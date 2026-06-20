package connectors

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// vertexSAJSON is a Google Cloud service-account key file. The Vertex connector
// accepts the entire JSON blob as the account's "API key"; when it parses as a
// service account, the connector mints a short-lived OAuth2 Bearer token from
// it (RS256 JWT assertion → token exchange), mirroring 9router's
// refreshVertexToken.
type vertexSAJSON struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// parseVertexSAJSON returns the parsed service account when apiKey is a valid SA
// JSON blob, or (nil, false) when it is a plain API key. Mirrors 9router's
// parseVertexSaJson: requires type=service_account plus client_email,
// private_key, and project_id.
func parseVertexSAJSON(apiKey string) (*vertexSAJSON, bool) {
	s := strings.TrimSpace(apiKey)
	if s == "" || s[0] != '{' {
		return nil, false
	}
	var sa vertexSAJSON
	if err := json.Unmarshal([]byte(s), &sa); err != nil {
		return nil, false
	}
	if sa.Type == "service_account" && sa.ClientEmail != "" && sa.PrivateKey != "" && sa.ProjectID != "" {
		return &sa, true
	}
	return nil, false
}

// vertexToken is a cached minted token.
type vertexToken struct {
	accessToken string
	expiresAt   time.Time
}

// vertexTokenCache caches minted Bearer tokens keyed by service-account email,
// refreshing 5 minutes before expiry (matching 9router's vertexTokenCache).
var (
	vertexTokenMu    sync.Mutex
	vertexTokenStore = map[string]vertexToken{}
)

// mintVertexToken mints (or returns a cached) OAuth2 Bearer token for Vertex AI
// from a service-account JSON, using the RS256 JWT-bearer assertion flow.
func mintVertexToken(ctx context.Context, sa *vertexSAJSON) (string, error) {
	vertexTokenMu.Lock()
	if cached, ok := vertexTokenStore[sa.ClientEmail]; ok {
		if time.Until(cached.expiresAt) > 5*time.Minute {
			vertexTokenMu.Unlock()
			return cached.accessToken, nil
		}
	}
	vertexTokenMu.Unlock()

	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}

	assertion, err := signVertexJWT(sa, tokenURI)
	if err != nil {
		return "", fmt.Errorf("vertex: sign jwt: %w", err)
	}

	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	body, err := doFormPOST(ctx, "vertex", "", tokenURI, form, nil)
	if err != nil {
		return "", err
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("vertex: decode token: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("vertex: empty access_token from token endpoint")
	}
	expiresIn := tok.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	vertexTokenMu.Lock()
	vertexTokenStore[sa.ClientEmail] = vertexToken{
		accessToken: tok.AccessToken,
		expiresAt:   time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
	vertexTokenMu.Unlock()

	return tok.AccessToken, nil
}

// signVertexJWT builds and signs an RS256 JWT assertion requesting the
// cloud-platform scope, audience = token endpoint, issuer = service account.
func signVertexJWT(sa *vertexSAJSON, audience string) (string, error) {
	priv, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return "", err
	}
	now := time.Now()
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   audience,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url(hb) + "." + b64url(cb)

	h := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// parseRSAPrivateKey decodes a PEM PKCS#8 (or PKCS#1) RSA private key, tolerating
// escaped "\n" sequences as they appear in JSON-embedded keys.
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	pemStr = strings.ReplaceAll(pemStr, "\\n", "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("vertex: no PEM block in private_key")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("vertex: private_key is not RSA")
	}
	// Fall back to PKCS#1.
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("vertex: parse private_key: %w", err)
	}
	return rsaKey, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
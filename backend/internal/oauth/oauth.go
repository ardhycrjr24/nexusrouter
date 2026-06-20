// Package oauth implements the OAuth flows NexusRouter uses to connect
// subscription/OAuth providers (Claude, Codex, Gemini CLI, GitHub Copilot,
// Qwen, xAI, ...) without an API key.
//
// Two canonical flows are supported, mirroring 9router:
//   - Authorization Code + PKCE: the dashboard opens a provider authorize URL,
//     the user signs in and is redirected back with a code, which is exchanged
//     for tokens (claude, codex, xai, gemini-cli, antigravity).
//   - Device Code: NexusRouter requests a device/user code, the user enters it on
//     the provider's verification page, and NexusRouter polls until tokens are
//     granted (github, qwen).
//
// Both flows end by sealing the resulting access/refresh tokens into an
// encrypted account record via the vault. Expired access tokens are refreshed
// on demand using the stored refresh token.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// FlowType discriminates the OAuth flow a provider uses.
type FlowType string

const (
	// FlowAuthCodePKCE is Authorization Code with PKCE (S256).
	FlowAuthCodePKCE FlowType = "authorization_code_pkce"
	// FlowAuthCode is plain Authorization Code (confidential client with secret).
	FlowAuthCode FlowType = "authorization_code"
	// FlowDeviceCode is the Device Authorization Grant.
	FlowDeviceCode FlowType = "device_code"
)

// PKCE holds a generated PKCE verifier/challenge pair plus a CSRF state.
type PKCE struct {
	Verifier  string
	Challenge string
	State     string
}

// GeneratePKCE produces a PKCE pair using the S256 method. bytes controls the
// verifier entropy (default 32; xAI uses 96).
func GeneratePKCE(bytes int) (PKCE, error) {
	if bytes <= 0 {
		bytes = 32
	}
	verifier, err := randomBase64URL(bytes)
	if err != nil {
		return PKCE{}, err
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return PKCE{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PKCE{Verifier: verifier, Challenge: challenge, State: state}, nil
}

// GenerateState produces a random CSRF state value.
func GenerateState() (string, error) {
	return randomBase64URL(32)
}

// Tokens is the normalized result of any OAuth flow.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	// IDToken is used transiently to derive labels/provider metadata. It is not
	// persisted because account metadata is stored as plaintext JSON.
	IDToken string
	// ExpiresIn is the access-token lifetime in seconds (0 if unknown).
	ExpiresIn int
	Scope     string
	// Email / DisplayName identify the connected account for the dashboard.
	Email       string
	DisplayName string
	// Extra carries provider-specific metadata to persist (project id, region,
	// AWS client credentials for refresh, etc.).
	Extra map[string]string
}

// randomBase64URL returns n random bytes encoded as unpadded base64url.
func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

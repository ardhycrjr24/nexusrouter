package connectors

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func TestParseVertexSAJSON(t *testing.T) {
	// A plain API key is not a service account.
	if _, ok := parseVertexSAJSON("AIzaSyPlainKey"); ok {
		t.Error("plain key must not parse as service account")
	}
	if _, ok := parseVertexSAJSON(""); ok {
		t.Error("empty must not parse")
	}

	sa := map[string]string{
		"type":         "service_account",
		"project_id":   "my-proj",
		"client_email": "svc@my-proj.iam.gserviceaccount.com",
		"private_key":  "-----BEGIN PRIVATE KEY-----\\nMII...\\n-----END PRIVATE KEY-----\\n",
	}
	b, _ := json.Marshal(sa)
	parsed, ok := parseVertexSAJSON(string(b))
	if !ok {
		t.Fatal("valid SA JSON must parse")
	}
	if parsed.ProjectID != "my-proj" || parsed.ClientEmail != "svc@my-proj.iam.gserviceaccount.com" {
		t.Errorf("parsed SA fields wrong: %+v", parsed)
	}
}

func TestVertexBuildURL(t *testing.T) {
	v := NewVertex("vertex", "https://aiplatform.googleapis.com")

	// SA / bearer mode → project-scoped path.
	saURL := v.buildURL("https://aiplatform.googleapis.com", "gemini-2.0-flash", "generateContent",
		"ya29.token", "", "my-proj", "us-central1", false)
	want := "https://aiplatform.googleapis.com/v1/projects/my-proj/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"
	if saURL != want {
		t.Errorf("SA url:\n got %q\nwant %q", saURL, want)
	}

	// SA / streaming → ?alt=sse.
	saStream := v.buildURL("https://aiplatform.googleapis.com", "gemini-2.0-flash", "streamGenerateContent",
		"ya29.token", "", "my-proj", "us-central1", true)
	if !strings.HasSuffix(saStream, ":streamGenerateContent?alt=sse") {
		t.Errorf("SA stream url missing ?alt=sse: %q", saStream)
	}

	// Raw key mode → global publishers endpoint with ?key=.
	rawURL := v.buildURL("https://aiplatform.googleapis.com", "gemini-2.0-flash", "generateContent",
		"", "AIzaKey", "", "us-central1", false)
	wantRaw := "https://aiplatform.googleapis.com/v1/publishers/google/models/gemini-2.0-flash:generateContent?key=AIzaKey"
	if rawURL != wantRaw {
		t.Errorf("raw url:\n got %q\nwant %q", rawURL, wantRaw)
	}

	// Raw key + streaming → ?alt=sse then &key=.
	rawStream := v.buildURL("https://aiplatform.googleapis.com", "gemini-2.0-flash", "streamGenerateContent",
		"", "AIzaKey", "", "us-central1", true)
	if !strings.Contains(rawStream, "?alt=sse&key=AIzaKey") {
		t.Errorf("raw stream url wrong: %q", rawStream)
	}
}

func TestSignVertexJWT(t *testing.T) {
	// Generate a throwaway RSA key and embed it as PKCS#8 PEM in an SA blob.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	sa := &vertexSAJSON{
		Type:        "service_account",
		ProjectID:   "p",
		ClientEmail: "svc@p.iam.gserviceaccount.com",
		PrivateKey:  string(pemBytes),
	}
	jwt, err := signVertexJWT(sa, "https://oauth2.googleapis.com/token")
	if err != nil {
		t.Fatalf("signVertexJWT: %v", err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt must have 3 segments, got %d", len(parts))
	}

	// Header should declare RS256.
	hdr, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hdr), `"RS256"`) {
		t.Errorf("jwt header missing RS256: %s", hdr)
	}

	// Claims should carry the cloud-platform scope and issuer.
	claims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claims)
	if !strings.Contains(cs, "cloud-platform") || !strings.Contains(cs, sa.ClientEmail) {
		t.Errorf("jwt claims missing scope/issuer: %s", cs)
	}
}
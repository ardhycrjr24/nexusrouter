package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	p, err := GeneratePKCE(32)
	if err != nil {
		t.Fatal(err)
	}
	if p.Verifier == "" || p.Challenge == "" || p.State == "" {
		t.Fatal("expected non-empty verifier/challenge/state")
	}
	// Challenge must be S256(verifier) base64url.
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Fatalf("challenge mismatch: got %q want %q", p.Challenge, want)
	}
	// base64url must not contain padding or +/.
	if strings.ContainsAny(p.Verifier, "+/=") {
		t.Errorf("verifier is not base64url: %q", p.Verifier)
	}
}

func TestGeneratePKCEUnique(t *testing.T) {
	a, _ := GeneratePKCE(32)
	b, _ := GeneratePKCE(32)
	if a.Verifier == b.Verifier || a.State == b.State {
		t.Fatal("expected unique PKCE values across calls")
	}
}

func TestConfigFor(t *testing.T) {
	for _, id := range []string{"claude", "codex", "github", "qwen", "xai", "gemini-cli"} {
		cfg, ok := ConfigFor(id)
		if !ok {
			t.Errorf("expected OAuth config for %q", id)
			continue
		}
		if cfg.Provider != id {
			t.Errorf("config %q has wrong Provider %q", id, cfg.Provider)
		}
	}
	if _, ok := ConfigFor("does-not-exist"); ok {
		t.Error("expected no config for unknown provider")
	}
}

func TestAuthURLPKCE(t *testing.T) {
	cfg, _ := ConfigFor("claude")
	url := cfg.AuthURL("http://localhost:20180/callback", "state123", "challenge456")
	for _, want := range []string{
		"https://claude.ai/oauth/authorize?",
		"client_id=9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		"code_challenge=challenge456",
		"code_challenge_method=S256",
		"state=state123",
		"response_type=code",
	} {
		if !strings.Contains(url, want) {
			t.Errorf("auth URL missing %q\ngot: %s", want, url)
		}
	}
}

func TestCodexAuthURLMatchesCLIFlow(t *testing.T) {
	cfg, _ := ConfigFor("codex")
	redirectURI := cfg.ResolveRedirectURI("http://localhost:20180/oauth/callback")
	if redirectURI != "http://localhost:1455/auth/callback" {
		t.Fatalf("codex redirect mismatch: got %q", redirectURI)
	}

	authURL := cfg.AuthURL(redirectURI, "state123", "challenge456")
	for _, want := range []string{
		"https://auth.openai.com/oauth/authorize?",
		"response_type=code",
		"client_id=app_EMoamEEZ73f0CkXaXp7hrann",
		"redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback",
		"scope=openid%20profile%20email%20offline_access",
		"code_challenge=challenge456",
		"code_challenge_method=S256",
		"id_token_add_organizations=true",
		"codex_cli_simplified_flow=true",
		"originator=codex_cli_rs",
		"state=state123",
	} {
		if !strings.Contains(authURL, want) {
			t.Errorf("codex auth URL missing %q\ngot: %s", want, authURL)
		}
	}
	if strings.Contains(authURL, "scope=openid+profile") {
		t.Fatalf("codex scope must use %%20 encoding, got: %s", authURL)
	}
}

func TestXAIAuthURLMatchesCLIFlow(t *testing.T) {
	cfg, _ := ConfigFor("xai")
	redirectURI := cfg.ResolveRedirectURI("http://localhost:20180/oauth/callback")
	if redirectURI != "http://127.0.0.1:56121/callback" {
		t.Fatalf("xai redirect mismatch: got %q", redirectURI)
	}

	authURL := cfg.AuthURL(redirectURI, "state123", "challenge456")
	for _, want := range []string{
		"https://auth.x.ai/oauth2/authorize?",
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A56121%2Fcallback",
		"scope=openid%20profile%20email%20offline_access%20grok-cli%3Aaccess%20api%3Aaccess",
		"state=state123",
		"plan=generic",
		"referrer=cli-proxy-api",
	} {
		if !strings.Contains(authURL, want) {
			t.Errorf("xai auth URL missing %q\ngot: %s", want, authURL)
		}
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	if nonce := parsed.Query().Get("nonce"); len(nonce) != 32 {
		t.Fatalf("expected 16-byte hex nonce, got %q", nonce)
	}
}

func TestAuthURLDeviceCodeFlowDistinct(t *testing.T) {
	cfg, _ := ConfigFor("github")
	if cfg.Flow != FlowDeviceCode {
		t.Fatalf("github should be device-code flow, got %q", cfg.Flow)
	}
}

func TestRefreshURLFallback(t *testing.T) {
	claude, _ := ConfigFor("claude")
	if claude.refreshURL() != claude.TokenURL {
		t.Error("refresh URL should default to token URL when unset")
	}
	cline, _ := ConfigFor("cline")
	if cline.refreshURL() != "https://api.cline.bot/api/v1/auth/refresh" {
		t.Errorf("cline refresh URL should use explicit RefreshURL, got %q", cline.refreshURL())
	}
}

func TestSessionStore(t *testing.T) {
	s := NewSessionStore()
	s.Put("k1", &Session{Provider: "claude", State: "k1", Verifier: "v"})
	got, ok := s.Get("k1")
	if !ok || got.Verifier != "v" {
		t.Fatal("expected to retrieve stored session")
	}
	s.Delete("k1")
	if _, ok := s.Get("k1"); ok {
		t.Fatal("expected session deleted")
	}
}

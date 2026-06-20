package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/config"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/oauth"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// TestOAuthCallback_NoFrontendDir verifies that /oauth/callback works even when
// the dashboard asset directory is missing. This is the bug from GitHub issue
// #1 — a binary running without bundled frontend/dist used to return chi's
// plain-text "404 page not found" for the provider's GET redirect, leaving the
// dashboard modal stuck on "Waiting for sign-in to complete…".
func TestOAuthCallback_NoFrontendDir(t *testing.T) {
	gw := New(Deps{
		Config:      config.Default(),
		Codecs:      transform.DefaultRegistry(),
		FrontendDir: "", // explicitly empty — mirrors the bug environment
	})
	handler := gw.Handler()

	t.Run("missing state renders inline error html", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback", nil)
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, "callback must be handled, not 404")
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		body := rec.Body.String()
		require.NotContains(t, body, "404 page not found")
		require.Contains(t, body, `"type":"oauth-callback"`)
		require.Contains(t, body, `"status":"error"`)
		require.Contains(t, body, "missing code or state parameter")
	})

	t.Run("unknown state renders session-expired html", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=unknown&code=x", nil)
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		body := rec.Body.String()
		require.NotContains(t, body, "404 page not found")
		require.Contains(t, body, "session expired or invalid")
	})

	t.Run("auth/callback alias is also handled", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=unknown&code=x", nil)
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
		require.NotContains(t, rec.Body.String(), "404 page not found")
	})
}

// TestOAuthCallback_ProviderMismatch verifies that mismatched provider hints
// surface a clean error page via the inline HTML response rather than relying
// on the SPA to render the error.
func TestOAuthCallback_ProviderMismatch(t *testing.T) {
	gw := New(Deps{
		Config: config.Default(),
		Codecs: transform.DefaultRegistry(),
	})
	gw.oauthSessions.Put("state-abc", &oauth.Session{Provider: "gemini-cli"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=state-abc&provider=codex&code=x", nil)
	gw.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body := rec.Body.String()
	require.Contains(t, body, "provider mismatch")
	require.True(t, strings.Contains(body, `"status":"error"`))
}

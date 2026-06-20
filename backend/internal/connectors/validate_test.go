package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

// validator is the optional interface connectors implement to probe credentials.
type validator interface {
	Validate(ctx context.Context, creds core.Credentials) error
}

// modelsServer returns a test server that responds to GET requests with the
// given status. A 200 returns a minimal OpenAI-style models list.
func modelsServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status >= 400 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"m1"}]}`))
	}))
}

func TestGemini_Validate(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewGemini("gemini", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{APIKey: "k", BaseURL: srv.URL}))
	})
	t.Run("rejected key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close()
		c := NewGemini("gemini", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{APIKey: "bad", BaseURL: srv.URL}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewGemini("gemini", "https://example.com")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestOpenAIResponses_Validate(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewOpenAIResponses("codex", srv.URL+"/responses")
		require.NoError(t, c.Validate(context.Background(), core.Credentials{AccessToken: "t", BaseURL: srv.URL + "/responses"}))
	})
	t.Run("rejected token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusForbidden)
		defer srv.Close()
		c := NewOpenAIResponses("codex", srv.URL+"/responses")
		require.Error(t, c.Validate(context.Background(), core.Credentials{AccessToken: "bad", BaseURL: srv.URL + "/responses"}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewOpenAIResponses("codex", "https://example.com/responses")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestQwen_Validate(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewQwen("qwen", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{AccessToken: "t", BaseURL: srv.URL + "/chat/completions"}))
	})
	t.Run("rejected token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close()
		c := NewQwen("qwen", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{AccessToken: "bad", BaseURL: srv.URL + "/chat/completions"}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewQwen("qwen", "https://example.com")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestIFlow_Validate(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewIFlow("iflow", srv.URL+"/chat/completions")
		require.NoError(t, c.Validate(context.Background(), core.Credentials{APIKey: "k", BaseURL: srv.URL + "/chat/completions"}))
	})
	t.Run("rejected key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close()
		c := NewIFlow("iflow", srv.URL+"/chat/completions")
		require.Error(t, c.Validate(context.Background(), core.Credentials{APIKey: "bad", BaseURL: srv.URL + "/chat/completions"}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewIFlow("iflow", "https://example.com/chat/completions")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestGitHubCopilot_Validate(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewGitHubCopilot("github", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{AccessToken: "t", BaseURL: srv.URL}))
	})
	t.Run("rejected token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close()
		c := NewGitHubCopilot("github", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{AccessToken: "bad", BaseURL: srv.URL}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewGitHubCopilot("github", "https://example.com")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestCommandCode_Validate(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewCommandCode("command-code", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{AccessToken: "t", BaseURL: srv.URL}))
	})
	t.Run("rejected token", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close()
		c := NewCommandCode("command-code", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{AccessToken: "bad", BaseURL: srv.URL}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewCommandCode("command-code", "https://example.com/generate")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

func TestVertex_Validate(t *testing.T) {
	t.Run("valid raw key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close()
		c := NewVertex("vertex", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{APIKey: "k", BaseURL: srv.URL}))
	})
	t.Run("rejected key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusForbidden)
		defer srv.Close()
		c := NewVertex("vertex", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{APIKey: "bad", BaseURL: srv.URL}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewVertex("vertex", "https://example.com")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}

// TestTokenPresenceValidators covers connectors whose upstreams have no cheap
// probe endpoint: validation is a credential-presence check.
func TestTokenPresenceValidators(t *testing.T) {
	cases := []struct {
		name string
		v    validator
	}{
		{"gemini-cli", NewGeminiCLI("gemini-cli", "https://example.com/v1internal")},
		{"antigravity", NewAntigravity("antigravity", "https://example.com")},
		{"cursor", NewCursor("cursor", "https://example.com")},
	}
	for _, tc := range cases {
		t.Run(tc.name+" with token", func(t *testing.T) {
			require.NoError(t, tc.v.Validate(context.Background(), core.Credentials{AccessToken: "t"}))
		})
		t.Run(tc.name+" without token", func(t *testing.T) {
			require.Error(t, tc.v.Validate(context.Background(), core.Credentials{}))
		})
	}
}
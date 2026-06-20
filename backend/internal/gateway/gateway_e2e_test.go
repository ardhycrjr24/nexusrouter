package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/budget"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/config"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/connectors"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/crypto"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/dispatch"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/identity"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/meter"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/pipeline"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/vault"
)

// e2eHarness wires a full gateway against an in-memory store and a fake upstream.
type e2eHarness struct {
	server   *httptest.Server
	apiKey   string
	upstream *httptest.Server
}

func newE2E(t *testing.T, upstreamHandler http.HandlerFunc) *e2eHarness {
	t.Helper()
	ctx := context.Background()

	// Fake upstream provider.
	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	// In-memory store.
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	// Crypto + vault.
	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)
	v := vault.New(sealer)

	// Seed an account for the "openai" provider pointing at the fake upstream.
	acc := store.Account{
		ID:        "acc-test",
		TenantID:  store.DefaultTenantID,
		Provider:  "openai",
		Label:     "test",
		AuthKind:  store.AuthAPIKey,
		Priority:  10,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, v.Seal(&acc, vault.NewSecret{
		APIKey:   "sk-upstream",
		Metadata: map[string]string{"base_url": upstream.URL},
	}))
	require.NoError(t, db.Accounts().Create(ctx, acc))

	// Issue an API key.
	idSvc := identity.New(db.APIKeys())
	issued, err := idSvc.Create(ctx, store.DefaultTenantID, "", "test-key")
	require.NoError(t, err)

	// Wire pipeline + gateway.
	connRegistry := connectors.DefaultRegistry()
	disp := dispatch.New(connRegistry, db.Accounts(), v)
	mtr := meter.New(db.Usage(), nil, nil)
	bud := budget.New(db.Budgets(), db.Usage())
	pipe := pipeline.New(pipeline.Deps{Dispatcher: disp, Meter: mtr, Budget: bud})

	gw := New(Deps{
		Config:   config.Default(),
		Identity: idSvc,
		Pipeline: pipe,
		Chains:   db.Chains(),
		Codecs:   transform.DefaultRegistry(),
	})

	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	return &e2eHarness{server: srv, apiKey: issued.Plaintext, upstream: upstream}
}

func (h *e2eHarness) post(t *testing.T, path, body, auth string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.server.URL+path, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// openAIUpstream returns a handler that responds as an OpenAI chat endpoint.
func openAIUpstream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"up1","model":"gpt-4o","choices":[{"message":{"role":"assistant","content":"hello from upstream"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
	}
}

func TestE2E_OpenAIChat_DirectProviderModel(t *testing.T) {
	h := newE2E(t, openAIUpstream())

	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	resp := h.post(t, "/v1/chat/completions", body, h.apiKey)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "openai", resp.Header.Get("X-NexusRouter-Provider"))

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out.Choices, 1)
	require.Equal(t, "hello from upstream", out.Choices[0].Message.Content)
}

func TestE2E_RejectsMissingAuth(t *testing.T) {
	h := newE2E(t, openAIUpstream())
	resp := h.post(t, "/v1/chat/completions", `{"model":"openai/gpt-4o","messages":[]}`, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestE2E_RejectsBadKey(t *testing.T) {
	h := newE2E(t, openAIUpstream())
	resp := h.post(t, "/v1/chat/completions", `{"model":"openai/gpt-4o","messages":[]}`, "kr_invalid")
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestE2E_UnknownModelIsBadRequest(t *testing.T) {
	h := newE2E(t, openAIUpstream())
	resp := h.post(t, "/v1/chat/completions", `{"model":"unknown-bare-name","messages":[{"role":"user","content":"hi"}]}`, h.apiKey)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Cross-dialect: a client speaks Anthropic (/v1/messages) but routes to an
// OpenAI-dialect provider. The gateway must translate both ways.
func TestE2E_AnthropicClientToOpenAIProvider(t *testing.T) {
	h := newE2E(t, openAIUpstream())

	body := `{"model":"openai/gpt-4o","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	resp := h.post(t, "/v1/messages", body, h.apiKey)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Response must be in Anthropic shape (content blocks + stop_reason).
	var out struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "message", out.Type)
	require.Len(t, out.Content, 1)
	require.Equal(t, "hello from upstream", out.Content[0].Text)
	require.Equal(t, "end_turn", out.StopReason)
}

func TestE2E_StreamingChat(t *testing.T) {
	streamUpstream := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		for _, l := range []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"par"}}]}`,
			`data: {"choices":[{"delta":{"content":"tial"}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		} {
			fmt.Fprintf(w, "%s\n\n", l)
			if flush != nil {
				flush.Flush()
			}
		}
	}
	h := newE2E(t, streamUpstream)

	body := `{"model":"openai/gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	resp := h.post(t, "/v1/chat/completions", body, h.apiKey)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	out := string(raw)

	require.Contains(t, out, `"content":"par"`)
	require.Contains(t, out, `"content":"tial"`)
	require.Contains(t, out, "[DONE]")
}
package gateway

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestE2E_DoubleV1Prefix verifies the collapseDoubleV1 middleware rewrites a
// duplicated /v1/v1 prefix down to /v1. The Anthropic SDK (Claude Code) appends
// /v1/messages to ANTHROPIC_BASE_URL, so a base URL ending in /v1 yields
// /v1/v1/messages. Without the rewrite the request would fall through to the
// SPA file server and break the client.
func TestE2E_DoubleV1Prefix(t *testing.T) {
	h := newE2E(t, openAIUpstream())

	body := `{"model":"openai/gpt-4o","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	resp := h.post(t, "/v1/v1/messages", body, h.apiKey)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Response must reach the Anthropic handler and render Anthropic shape.
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

// TestE2E_DoubleV1Prefix_ChatCompletions confirms the rewrite is generic across
// /v1/* routes, not just /v1/messages.
func TestE2E_DoubleV1Prefix_ChatCompletions(t *testing.T) {
	h := newE2E(t, openAIUpstream())

	body := `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	resp := h.post(t, "/v1/v1/chat/completions", body, h.apiKey)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "openai", resp.Header.Get("X-NexusRouter-Provider"))
}

// TestCollapseDoubleV1 unit-tests the path rewrite in isolation.
func TestCollapseDoubleV1(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/v1/v1/messages", "/v1/messages"},
		{"/v1/v1/chat/completions", "/v1/chat/completions"},
		{"/v1/v1", "/v1"},
		{"/v1/messages", "/v1/messages"},
		{"/v1", "/v1"},
		{"/healthz", "/healthz"},
		{"/v1beta/models/x", "/v1beta/models/x"},
	}
	for _, c := range cases {
		var got string
		h := collapseDoubleV1(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = r.URL.Path
		}))
		req, err := http.NewRequest(http.MethodGet, "http://x"+c.in, nil)
		require.NoError(t, err)
		h.ServeHTTP(nil, req)
		require.Equal(t, c.want, got, "input %s", c.in)
	}
}
package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

func TestCommandCodeHeaders(t *testing.T) {
	c := NewCommandCode("commandcode", "https://example.com/alpha/generate")

	headers := c.headers(core.Credentials{APIKey: "user_test"})

	require.Equal(t, "Bearer user_test", headers["Authorization"])
	require.Equal(t, commandCodeVersion, headers[commandCodeVersionHeader])
	require.Equal(t, commandCodeCLIEnv, headers[commandCodeCLIEnvHeader])
	require.NotEmpty(t, headers[commandCodeSessionHeader])
	_, err := uuid.Parse(headers[commandCodeSessionHeader])
	require.NoError(t, err)
}

func TestCommandCodeHeadersPreferAccessTokenAndAllowOverrides(t *testing.T) {
	c := NewCommandCode("commandcode", "https://example.com/alpha/generate")

	headers := c.headers(core.Credentials{
		APIKey:      "user_api",
		AccessToken: "user_access",
		Headers: map[string]string{
			commandCodeVersionHeader: "custom-version",
			commandCodeSessionHeader: "custom-session",
		},
	})

	require.Equal(t, "Bearer user_access", headers["Authorization"])
	require.Equal(t, "custom-version", headers[commandCodeVersionHeader])
	require.Equal(t, "custom-session", headers[commandCodeSessionHeader])
}

func TestCommandCodeChatForcesUpstreamStream(t *testing.T) {
	var gotStream bool
	var gotSession string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer user_test", r.Header.Get("Authorization"))
		require.Equal(t, commandCodeVersion, r.Header.Get(commandCodeVersionHeader))
		require.Equal(t, commandCodeCLIEnv, r.Header.Get(commandCodeCLIEnvHeader))
		gotSession = r.Header.Get(commandCodeSessionHeader)

		var body struct {
			Params struct {
				Stream bool `json:"stream"`
			} `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		gotStream = body.Params.Stream

		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"type":"text-delta","text":"hello"}` + "\n" + `{"type":"finish","finishReason":"stop","totalUsage":{"inputTokens":1,"outputTokens":2,"totalTokens":3}}` + "\n"))
	}))
	defer srv.Close()

	c := NewCommandCode("commandcode", srv.URL)
	resp, err := c.Chat(context.Background(), &core.ChatRequest{
		Model: "deepseek/deepseek-v4-pro",
		Messages: []core.Message{{
			Role: core.RoleUser,
			Content: []core.ContentPart{{
				Type: core.PartText,
				Text: "hi",
			}},
		}},
		Stream: false,
	}, core.Credentials{APIKey: "user_test"})
	require.NoError(t, err)

	require.True(t, gotStream)
	require.NotEmpty(t, gotSession)
	_, err = uuid.Parse(gotSession)
	require.NoError(t, err)
	require.Equal(t, "deepseek/deepseek-v4-pro", resp.Model)
	require.Equal(t, core.FinishStop, resp.FinishReason)
	require.Len(t, resp.Message.Content, 1)
	require.Equal(t, "hello", resp.Message.Content[0].Text)
	require.Equal(t, 1, resp.Usage.PromptTokens)
	require.Equal(t, 2, resp.Usage.CompletionTokens)
	require.Equal(t, 3, resp.Usage.TotalTokens)
}

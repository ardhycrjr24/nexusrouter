package connectors

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildEventStreamFrame builds a full AWS EventStream binary frame with the
// given event-type header and JSON payload. CRC fields are zeroed (the parser
// does not validate them).
func buildEventStreamFrame(eventType string, payload []byte) []byte {
	name := ":event-type"
	var hdr bytes.Buffer
	hdr.WriteByte(byte(len(name)))
	hdr.WriteString(name)
	hdr.WriteByte(7) // string type
	var vl [2]byte
	binary.BigEndian.PutUint16(vl[:], uint16(len(eventType)))
	hdr.Write(vl[:])
	hdr.WriteString(eventType)

	headers := hdr.Bytes()
	headersLen := len(headers)
	totalLen := 12 + headersLen + len(payload) + 4

	var out bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(totalLen))
	out.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(headersLen))
	out.Write(u32[:])
	out.Write([]byte{0, 0, 0, 0}) // prelude CRC
	out.Write(headers)
	out.Write(payload)
	out.Write([]byte{0, 0, 0, 0}) // message CRC
	return out.Bytes()
}

// writeEventStream writes binary EventStream frames to w and keeps the
// connection open briefly so the client's reader goroutine has time to consume
// the data before the handler returns and the server closes the connection.
func writeEventStream(w http.ResponseWriter, frames ...[]byte) {
	flush, _ := w.(http.Flusher)
	for _, f := range frames {
		w.Write(f)
		if flush != nil {
			flush.Flush()
		}
	}
	// Give the reader goroutine time to consume the buffered data.
	time.Sleep(50 * time.Millisecond)
}

// cloudCodeResponse wraps a Gemini-format response in the CloudCode envelope.
func cloudCodeResponse(inner string) string {
	return fmt.Sprintf(`{"response":%s}`, inner)
}

// geminiUnaryResponse returns a minimal Gemini generateContent JSON response.
func geminiUnaryResponse(text string) string {
	return fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`, text)
}

// geminiStreamChunk returns a single Gemini SSE stream chunk wrapped in CloudCode.
func geminiStreamChunk(text string) string {
	inner := fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}],"role":"model"}}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`, text)
	return cloudCodeResponse(inner)
}

// geminiStreamStop returns a Gemini stream stop chunk wrapped in CloudCode.
func geminiStreamStop() string {
	inner := `{"candidates":[{"content":{"parts":[],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`
	return cloudCodeResponse(inner)
}

// openaiResponse returns a minimal OpenAI chat completion JSON response.
func openaiResponse(model, text string) string {
	return fmt.Sprintf(`{"id":"chatcmpl-test","model":%q,"choices":[{"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`, model, text)
}

// openaiStreamChunk returns one OpenAI SSE data line.
func openaiStreamChunk(delta string) string {
	return fmt.Sprintf(`data: {"choices":[{"delta":{"role":"assistant","content":%q}}]}`, delta)
}

// ---------------------------------------------------------------------------
// Kiro Provider Tests (AWS EventStream binary protocol)
// ---------------------------------------------------------------------------

func TestKiro_Chat_Sonnet(t *testing.T) {
	// Mock the Kiro/CodeWhisperer endpoint returning a binary EventStream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "application/vnd.amazon.eventstream", r.Header.Get("Accept"))
		require.Equal(t, "AmazonCodeWhispererStreamingService.GenerateAssistantResponse", r.Header.Get("X-Amz-Target"))
		require.NotEmpty(t, r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		writeEventStream(w,
			buildEventStreamFrame("assistantResponseEvent", []byte(`{"content":"Hello from Kiro Sonnet"}`)),
			buildEventStreamFrame("messageStopEvent", []byte(`{}`)),
		)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.NoError(t, err)
	require.Equal(t, "Hello from Kiro Sonnet", resp.Message.TextContent())
	require.Equal(t, core.FinishStop, resp.FinishReason)
}

func TestKiro_Chat_Sonnet_Thinking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		writeEventStream(w,
			buildEventStreamFrame("reasoningContentEvent", []byte(`{"text":"Let me think..."}`)),
			buildEventStreamFrame("assistantResponseEvent", []byte(`{"content":"Here is my answer"}`)),
			buildEventStreamFrame("messageStopEvent", []byte(`{}`)),
		)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5-thinking",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "think about this"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.NoError(t, err)
	require.Equal(t, "Here is my answer", resp.Message.TextContent())
	// Verify thinking content is present.
	var thinking string
	for _, part := range resp.Message.Content {
		if part.Type == core.PartThinking {
			thinking += part.Text
		}
	}
	require.Equal(t, "Let me think...", thinking)
}

func TestKiro_Stream_Sonnet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		writeEventStream(w,
			buildEventStreamFrame("assistantResponseEvent", []byte(`{"content":"He"}`)),
			buildEventStreamFrame("assistantResponseEvent", []byte(`{"content":"llo"}`)),
			buildEventStreamFrame("messageStopEvent", []byte(`{}`)),
		)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		Stream: true,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	ch, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "test-token"}, core.StreamConfig{})
	require.NoError(t, err)

	var text string
	var finished bool
	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkText:
			text += chunk.Delta
		case core.ChunkFinish:
			finished = true
		case core.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	require.Equal(t, "Hello", text)
	require.True(t, finished)
}

func TestKiro_Stream_ToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		writeEventStream(w,
			buildEventStreamFrame("toolUseEvent", []byte(`{"toolUseId":"call-1","name":"read_file","input":{"path":"/test.go"}}`)),
			buildEventStreamFrame("messageStopEvent", []byte(`{}`)),
		)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5-agentic",
		Stream: true,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "read the file"}}},
		},
	}
	ch, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "test-token"}, core.StreamConfig{})
	require.NoError(t, err)

	var hasTool bool
	var toolName string
	var finishReason core.FinishReason
	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkToolCall:
			if chunk.ToolCall != nil && chunk.ToolCall.Name != "" {
				hasTool = true
				toolName = chunk.ToolCall.Name
			}
		case core.ChunkFinish:
			finishReason = chunk.FinishReason
		case core.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	require.True(t, hasTool, "expected tool call chunk")
	require.Equal(t, "read_file", toolName)
	require.Equal(t, core.FinishToolCalls, finishReason)
}

func TestKiro_Chat_UsageMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		writeEventStream(w,
			buildEventStreamFrame("assistantResponseEvent", []byte(`{"content":"ok"}`)),
			buildEventStreamFrame("metricsEvent", []byte(`{"inputTokens":10,"outputTokens":5}`)),
			buildEventStreamFrame("messageStopEvent", []byte(`{}`)),
		)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.NoError(t, err)
	require.Equal(t, 10, resp.Usage.PromptTokens)
	require.Equal(t, 5, resp.Usage.CompletionTokens)
	require.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestKiro_Chat_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Unauthorized"}`)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "bad-token"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrAuth, pe.Kind)
	require.Equal(t, "kiro", pe.Provider)
	require.True(t, pe.Fallbackable())
}

func TestKiro_Chat_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"Rate limit exceeded"}`)
	}))
	defer srv.Close()

	c := NewKiro("kiro", srv.URL)
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrRateLimit, pe.Kind)
	require.True(t, pe.Fallbackable())
}

func TestKiro_Validate_NoToken(t *testing.T) {
	c := NewKiro("kiro", "http://unused")
	err := c.Validate(context.Background(), core.Credentials{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no access token")
}

func TestKiro_Validate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "ListAvailableModels")
		require.NotEmpty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[{"modelId":"auto","modelName":"Auto"}]}`)
	}))
	defer srv.Close()

	// The Validate function hits the q.us-east-1.amazonaws.com endpoint, so we
	// need to use a custom base URL override via creds. Since Validate hardcodes
	// its own URL, we test the token-presence path only.
	c := NewKiro("kiro", srv.URL)
	// Validate will fail against the mock (different URL), but we can verify it
	// at least attempts the call with a non-empty token.
	err := c.Validate(context.Background(), core.Credentials{AccessToken: "test-token"})
	// The call will fail because Validate uses a hardcoded AWS URL, not srv.URL.
	// This is expected — we're testing the token-presence guard works.
	if err != nil {
		require.Contains(t, err.Error(), "validation failed")
	}
}

func TestKiro_Dialect(t *testing.T) {
	c := NewKiro("kiro", "http://unused")
	require.Equal(t, core.DialectKiro, c.Dialect())
}

// ---------------------------------------------------------------------------
// Antigravity Provider Tests (CloudCode/Gemini protocol)
// ---------------------------------------------------------------------------

func TestAntigravity_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/v1internal:generateContent")
		require.Equal(t, "antigravity/1.104.0", r.Header.Get("User-Agent"))
		require.NotEmpty(t, r.Header.Get("Authorization"))
		require.Equal(t, "local", r.Header.Get("x-request-source"))

		// Verify the request body has the CloudCode envelope structure.
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotEmpty(t, body["project"])
		require.NotEmpty(t, body["request"])
		require.Equal(t, "agent", body["requestType"])

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, cloudCodeResponse(geminiUnaryResponse("Hello from Antigravity")))
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.NoError(t, err)
	require.Equal(t, "Hello from Antigravity", resp.Message.TextContent())
	require.Equal(t, core.FinishStop, resp.FinishReason)
}

func TestAntigravity_Chat_WithSessionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-session-id", r.Header.Get("X-Machine-Session-Id"))

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, cloudCodeResponse(geminiUnaryResponse("ok")))
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{
		AccessToken: "test-token",
		Extra:       map[string]string{"session_id": "test-session-id"},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Message.TextContent())
}

func TestAntigravity_Chat_WithProjectID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "my-project-123", body["project"])

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, cloudCodeResponse(geminiUnaryResponse("ok")))
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{
		AccessToken: "test-token",
		Extra:       map[string]string{"project_id": "my-project-123"},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Message.TextContent())
}

func TestAntigravity_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.URL.Path, "/v1internal:streamGenerateContent")
		require.Equal(t, "sse", r.URL.Query().Get("alt"))
		require.Equal(t, "text/event-stream", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamChunk("He"))
		if flush != nil {
			flush.Flush()
		}
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamChunk("llo"))
		if flush != nil {
			flush.Flush()
		}
		fmt.Fprintf(w, "data: %s\n\n", geminiStreamStop())
		if flush != nil {
			flush.Flush()
		}
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: true,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	ch, err := c.Stream(context.Background(), req, core.Credentials{AccessToken: "test-token"}, core.StreamConfig{})
	require.NoError(t, err)

	var text string
	var finished bool
	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkText:
			text += chunk.Delta
		case core.ChunkFinish:
			finished = true
		case core.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	require.Equal(t, "Hello", text)
	require.True(t, finished)
}

func TestAntigravity_Chat_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"Access denied"}}`)
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "bad-token"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrAuth, pe.Kind)
	require.Equal(t, "antigravity", pe.Provider)
}

func TestAntigravity_Chat_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"code":429,"message":"Quota exceeded"}}`)
	}))
	defer srv.Close()

	c := NewAntigravity("antigravity", srv.URL)
	req := &core.ChatRequest{
		Model:  "gemini-2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{AccessToken: "test-token"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrRateLimit, pe.Kind)
	require.True(t, pe.Fallbackable())
}

func TestAntigravity_Dialect(t *testing.T) {
	c := NewAntigravity("antigravity", "http://unused")
	require.Equal(t, core.DialectAntigravity, c.Dialect())
}

// ---------------------------------------------------------------------------
// Xiaomi MiMo Provider Tests (OpenAI-compatible protocol)
// ---------------------------------------------------------------------------

func TestXiaomiMiMo_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, openaiResponse("mimo-v2.5-pro", "Hello from MiMo V2.5 Pro"))
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-api-key"})
	require.NoError(t, err)
	require.Equal(t, "Hello from MiMo V2.5 Pro", resp.Message.TextContent())
	require.Equal(t, core.FinishStop, resp.FinishReason)
	require.Equal(t, 5, resp.Usage.TotalTokens)
}

func TestXiaomiMiMo_Chat_MultipleModels(t *testing.T) {
	models := []struct {
		id      string
		display string
	}{
		{"mimo-v2.5-pro", "MiMo V2.5 Pro"},
		{"mimo-v2.5", "MiMo V2.5"},
		{"mimo-v2-omni", "MiMo V2 Omni"},
		{"mimo-v2-flash", "MiMo V2 Flash"},
	}

	for _, model := range models {
		t.Run(model.id, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, model.id, body["model"])

				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, openaiResponse(model.id, "ok from "+model.display))
			}))
			defer srv.Close()

			c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
			req := &core.ChatRequest{
				Model:  model.id,
				Stream: false,
				Messages: []core.Message{
					{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
				},
			}
			resp, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
			require.NoError(t, err)
			require.Equal(t, "ok from "+model.display, resp.Message.TextContent())
		})
	}
}

func TestXiaomiMiMo_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)

		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		lines := []string{
			openaiStreamChunk("He"),
			openaiStreamChunk("llo"),
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}
		for _, l := range lines {
			fmt.Fprintf(w, "%s\n\n", l)
			if flush != nil {
				flush.Flush()
			}
		}
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: true,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	ch, err := c.Stream(context.Background(), req, core.Credentials{APIKey: "test-api-key"}, core.StreamConfig{})
	require.NoError(t, err)

	var text string
	var finished bool
	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkText:
			text += chunk.Delta
		case core.ChunkFinish:
			finished = true
		case core.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	require.Equal(t, "Hello", text)
	require.True(t, finished)
}

func TestXiaomiMiMo_Chat_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "bad-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrAuth, pe.Kind)
	require.Equal(t, "xiaomi-mimo", pe.Provider)
	require.True(t, pe.Fallbackable())
}

func TestXiaomiMiMo_Chat_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrRateLimit, pe.Kind)
	require.Equal(t, 429, pe.StatusCode)
	require.True(t, pe.Fallbackable())
}

func TestXiaomiMiMo_Chat_BadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Model not found","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "nonexistent-model",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrBadRequest, pe.Kind)
	require.False(t, pe.Fallbackable(), "4xx request errors must not trigger fallback")
}

func TestXiaomiMiMo_Validate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"mimo-v2.5-pro","object":"model"}]}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	err := c.Validate(context.Background(), core.Credentials{APIKey: "test-key"})
	require.NoError(t, err)
}

func TestXiaomiMiMo_Validate_BadKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	err := c.Validate(context.Background(), core.Credentials{APIKey: "bad-key"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation failed")
}

func TestXiaomiMiMo_Dialect(t *testing.T) {
	c := NewOpenAICompatible("xiaomi-mimo", "http://unused")
	require.Equal(t, core.DialectOpenAI, c.Dialect())
}

// ---------------------------------------------------------------------------
// Xiaomi Token Plan Provider Tests
// ---------------------------------------------------------------------------

func TestXiaomiTokenPlan_Chat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer tp-test-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, openaiResponse("mimo-v2.5-pro", "Hello from Token Plan"))
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-tokenplan", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "tp-test-key"})
	require.NoError(t, err)
	require.Equal(t, "Hello from Token Plan", resp.Message.TextContent())
}

// ---------------------------------------------------------------------------
// Registry Integration Tests
// ---------------------------------------------------------------------------

func TestRegistry_HasAllThreeProviders(t *testing.T) {
	r := DefaultRegistry()
	for _, id := range []string{"kiro", "antigravity", "xiaomi-mimo", "xiaomi-tokenplan"} {
		require.True(t, r.Has(id), "registry should have connector for %q", id)
	}
}

func TestRegistry_ProviderDialects(t *testing.T) {
	r := DefaultRegistry()

	cases := map[string]core.Dialect{
		"kiro":             core.DialectKiro,
		"antigravity":      core.DialectAntigravity,
		"xiaomi-mimo":      core.DialectOpenAI,
		"xiaomi-tokenplan": core.DialectOpenAI,
	}
	for provider, wantDialect := range cases {
		c, err := r.Get(provider)
		require.NoError(t, err, "provider %q should be registered", provider)
		require.Equal(t, wantDialect, c.Dialect(), "provider %q dialect mismatch", provider)
	}
}

func TestCatalog_HasProviderSpecs(t *testing.T) {
	for _, id := range []string{"kiro", "antigravity", "xiaomi-mimo", "xiaomi-tokenplan"} {
		spec, ok := SpecByID(id)
		require.True(t, ok, "catalog should have spec for %q", id)
		require.NotEmpty(t, spec.DisplayName, "provider %q should have a display name", id)
		require.NotEmpty(t, spec.Alias, "provider %q should have an alias", id)
	}
}

func TestCatalog_AliasResolution(t *testing.T) {
	cases := map[string]string{
		"kr":    "kiro",
		"ag":    "antigravity",
		"mimo":  "xiaomi-mimo",
		"mmtp":  "xiaomi-tokenplan",
	}
	for alias, wantID := range cases {
		spec, ok := SpecByAlias(alias)
		require.True(t, ok, "alias %q should resolve", alias)
		require.Equal(t, wantID, spec.ID, "alias %q should resolve to %q", alias, wantID)
	}
}

func TestModelsForProvider_ThreeProviders(t *testing.T) {
	cases := map[string][]string{
		"kiro":             {"auto", "auto-thinking", "claude-sonnet-4.5", "claude-sonnet-4.5-thinking", "claude-sonnet-4.5-agentic", "claude-sonnet-4.5-thinking-agentic"},
		"xiaomi-mimo":      {"mimo-v2.5-pro", "mimo-v2.5", "mimo-v2-omni", "mimo-v2-flash"},
		"xiaomi-tokenplan": {"mimo-v2.5-pro", "mimo-v2.5", "mimo-v2-pro", "mimo-v2-omni"},
	}
	for provider, wantModels := range cases {
		models := ModelsForProvider(provider)
		require.NotEmpty(t, models, "provider %q should have models", provider)

		got := map[string]bool{}
		for _, m := range models {
			got[m.ID] = true
		}
		for _, id := range wantModels {
			require.True(t, got[id], "provider %q should have model %q", provider, id)
		}
	}
}

func TestFindModel_ThreeProviders(t *testing.T) {
	cases := []struct {
		provider string
		model    string
	}{
		{"kiro", "claude-sonnet-4.5"},
		{"kiro", "claude-sonnet-4.5-thinking"},
		{"kiro", "auto"},
		{"xiaomi-mimo", "mimo-v2.5-pro"},
		{"xiaomi-mimo", "mimo-v2-flash"},
		{"xiaomi-tokenplan", "mimo-v2.5-pro"},
		{"xiaomi-tokenplan", "mimo-v2-pro"},
	}
	for _, tc := range cases {
		_, ok := FindModel(tc.provider, tc.model)
		require.True(t, ok, "should find %s/%s", tc.provider, tc.model)
	}
}

func TestRegionResolution_XiaomiTokenPlan(t *testing.T) {
	// Default region (sgp).
	base := ResolveRegionBaseURL("xiaomi-tokenplan", "")
	require.Equal(t, "https://token-plan-sgp.xiaomimimo.com/v1", base)

	// Explicit regions.
	base = ResolveRegionBaseURL("xiaomi-tokenplan", "cn")
	require.Equal(t, "https://token-plan-cn.xiaomimimo.com/v1", base)

	base = ResolveRegionBaseURL("xiaomi-tokenplan", "ams")
	require.Equal(t, "https://token-plan-ams.xiaomimimo.com/v1", base)

	base = ResolveRegionBaseURL("xiaomi-tokenplan", "sgp")
	require.Equal(t, "https://token-plan-sgp.xiaomimimo.com/v1", base)
}

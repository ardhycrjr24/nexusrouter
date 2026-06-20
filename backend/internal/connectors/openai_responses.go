package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// OpenAIResponses drives the OpenAI Responses API (/v1/responses), the dialect
// used by Codex and Responses-native clients. The base URL points directly at
// the responses endpoint (e.g. https://chatgpt.com/backend-api/codex/responses
// for Codex, or https://api.openai.com/v1/responses for the public API), so the
// connector POSTs to the base URL itself rather than appending a path. It reads
// the rich Responses SSE event stream via the codec.
type OpenAIResponses struct {
	id          string
	defaultBase string
	codec       transform.OpenAIResponsesCodec
}

// NewOpenAIResponses builds a Responses connector.
func NewOpenAIResponses(id, defaultBaseURL string) *OpenAIResponses {
	return &OpenAIResponses{id: id, defaultBase: defaultBaseURL}
}

func (c *OpenAIResponses) ID() string            { return c.id }
func (c *OpenAIResponses) Dialect() core.Dialect { return core.DialectOpenAIResponses }

func (c *OpenAIResponses) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// endpoint returns the responses URL. If the configured base already ends in
// "/responses" it is used as-is; otherwise "/responses" is appended (so a plain
// "https://api.openai.com/v1" base also works).
func (c *OpenAIResponses) endpoint(creds core.Credentials) string {
	base := c.baseURL(creds)
	if hasResponsesSuffix(base) {
		return base
	}
	return joinURL(base, "responses")
}

func hasResponsesSuffix(u string) bool {
	const suf = "/responses"
	if len(u) < len(suf) {
		return false
	}
	return u[len(u)-len(suf):] == suf
}

// Validate probes the upstream by listing models. The base usually points at
// ".../responses", so the models endpoint is derived by trimming that suffix
// and appending "/models". A 401/403 means the key/token is rejected.
func (c *OpenAIResponses) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.APIKey == "" && creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no API key or access token", c.id)
	}
	base := strings.TrimRight(c.baseURL(creds), "/")
	base = strings.TrimSuffix(base, "/responses")
	modelsURL := base + "/models"
	// Codex's ChatGPT backend rejects the models probe without a client_version
	// query param (mirrors the codex CLI).
	if c.id == "codex" {
		modelsURL += "?client_version=1.0.0"
	}
	if _, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", modelsURL, nil, c.headers(creds)); err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

func (c *OpenAIResponses) headers(creds core.Credentials) map[string]string {
	h := map[string]string{}
	switch {
	case creds.AccessToken != "":
		h["Authorization"] = bearer(creds.AccessToken)
	case creds.APIKey != "":
		h["Authorization"] = bearer(creds.APIKey)
	}
	return mergeHeaders(h, creds.Headers)
}

// Chat performs a non-streaming Responses call.
func (c *OpenAIResponses) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	respBody, err := doJSON(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping.
func (c *OpenAIResponses) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// Stream performs a streaming Responses call, reading the typed SSE event stream.
func (c *OpenAIResponses) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		ttft := newTTFTTracker(cfg)
		terminalSeen := false

		scanner := sseScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			payload, ok := parseSSEData(scanner.Text())
			if !ok {
				continue
			}
			chunks, perr := c.codec.ParseStreamLine([]byte(payload), req.Model)
			if perr != nil {
				// Check if this was a terminal event that failed to parse.
				if isResponsesTerminalPayload(payload) {
					terminalSeen = true
				}
				continue
			}
			// Track whether we received a terminal event.
			if isResponsesTerminalPayload(payload) {
				terminalSeen = true
			}
			for _, ch := range chunks {
				ttft.maybeReport(ch)
				select {
				case out <- ch:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			terminalSeen = true // error is itself terminal
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err},
			}
		}

		// If the stream closed without a terminal event (response.completed,
		// response.failed, or error), synthesize response.failed + [DONE]
		// so Codex clients don't hang waiting for a terminal event.
		if !terminalSeen {
			out <- core.StreamChunk{
				Type: core.ChunkText,
				Delta: formatResponsesFailureAndDone(),
			}
		}
	}()
	return out, nil
}

// isResponsesTerminalPayload checks if an SSE data payload contains a terminal
// Responses API event: response.completed, response.failed, or error.
func isResponsesTerminalPayload(payload string) bool {
	var event struct {
		Type     string `json:"type"`
		Response *struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return false
	}
	switch event.Type {
	case "response.completed", "response.failed", "error":
		return true
	}
	if event.Response != nil {
		if event.Response.Status == "completed" || event.Response.Status == "failed" {
			return true
		}
	}
	return false
}

// formatResponsesFailureAndDone returns a synthetic response.failed SSE event
// followed by [DONE], for streams that close before the upstream sent a
// terminal event. This prevents Codex clients from hanging.
func formatResponsesFailureAndDone() string {
	failureEvent := map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":     fmt.Sprintf("resp_%d", time.Now().UnixMilli()),
			"status": "failed",
			"error": map[string]any{
				"type":    "stream_error",
				"code":    "stream_disconnected",
				"message": "stream closed before response.completed",
			},
		},
	}
	b, _ := json.Marshal(failureEvent)
	return "event: response.failed\ndata: " + string(b) + "\n\ndata: [DONE]\n\n"
}

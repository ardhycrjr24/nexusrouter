package connectors

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// ---- Qwen (portal.qwen.ai) --------------------------------------------------

// Qwen client fingerprint, mirroring 9router's qwen executor.
const qwenUserAgent = "QwenCode/0.12.3 (linux; x64)"

// Qwen drives portal.qwen.ai's OpenAI-compatible /chat/completions endpoint.
// Qwen OAuth tokens are bound to a per-account resource_url (host shard), so the
// connector resolves the host from creds.Extra["resource_url"] when present.
// It also injects an ephemeral cache-control system message and stream usage
// options, matching 9router's QwenExecutor.
type Qwen struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec
}

// NewQwen builds a Qwen connector.
func NewQwen(id, defaultBaseURL string) *Qwen {
	return &Qwen{id: id, defaultBase: defaultBaseURL}
}

func (c *Qwen) ID() string            { return c.id }
func (c *Qwen) Dialect() core.Dialect { return core.DialectOpenAI }

// endpoint resolves the chat/completions URL, honoring a token-bound resource
// URL host when supplied (using portal.qwen.ai for a foreign shard yields 401).
func (c *Qwen) endpoint(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	host := "portal.qwen.ai"
	if ru := creds.Extra["resource_url"]; ru != "" {
		host = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(ru, "https://"), "http://"), "/")
	}
	return "https://" + host + "/v1/chat/completions"
}

func (c *Qwen) headers(creds core.Credentials, stream bool) map[string]string {
	token := creds.APIKey
	if token == "" {
		token = creds.AccessToken
	}
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	h := map[string]string{
		"Authorization":               bearer(token),
		"User-Agent":                  qwenUserAgent,
		"X-DashScope-AuthType":        "qwen-oauth",
		"X-DashScope-CacheControl":    "enable",
		"X-DashScope-UserAgent":       qwenUserAgent,
		"X-Stainless-Arch":            "x64",
		"X-Stainless-Lang":            "js",
		"X-Stainless-Os":              "Linux",
		"X-Stainless-Package-Version": "5.11.0",
		"X-Stainless-Retry-Count":     "1",
		"X-Stainless-Runtime":         "node",
		"X-Stainless-Runtime-Version": "v18.19.1",
		"Accept":                      accept,
	}
	return mergeHeaders(h, creds.Headers)
}

// transformBody injects the Qwen ephemeral system message + stream usage opts.
func (c *Qwen) transformBody(body []byte, stream bool) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	// Prepend the ephemeral cache-control system message.
	sysMsg := map[string]any{
		"role":    "system",
		"content": []map[string]any{{"type": "text", "text": "", "cache_control": map[string]any{"type": "ephemeral"}}},
	}
	if msgs, ok := m["messages"].([]any); ok {
		m["messages"] = append([]any{sysMsg}, msgs...)
	} else {
		m["messages"] = []any{sysMsg}
	}
	// Request usage in stream.
	if stream {
		if _, ok := m["stream_options"]; !ok {
			m["stream_options"] = map[string]any{"include_usage": true}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// Validate confirms the Qwen token is accepted by listing models on the
// token-bound host. A 401/403 means the credential is rejected.
func (c *Qwen) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.APIKey == "" && creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no API key or access token", c.id)
	}
	endpoint := c.endpoint(creds)
	modelsURL := strings.TrimSuffix(endpoint, "/chat/completions") + "/models"
	if _, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", modelsURL, nil, c.headers(creds, false)); err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

func (c *Qwen) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	body = c.transformBody(body, false)

	respBody, err := doJSON(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, false))
	if err != nil {
		return nil, err
	}
	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

func (c *Qwen) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	body = c.transformBody(body, true)

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, true))
	if err != nil {
		return nil, err
	}
	return scanOpenAISSE(ctx, c.id, req.Model, resp, c.codec, cfg), nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping.
func (c *Qwen) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	body = c.transformBody(body, true)

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, true))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// ---- iFlow (apis.iflow.cn) --------------------------------------------------

const iflowUserAgent = "iFlow-Cli"

// IFlow drives apis.iflow.cn's OpenAI-compatible /chat/completions endpoint.
// iFlow requires a per-request HMAC-SHA256 signature over
// "<user-agent>:<session-id>:<timestamp>" keyed by the API key, plus the
// session-id and timestamp headers. Mirrors 9router's IFlowExecutor.
type IFlow struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec
}

// NewIFlow builds an iFlow connector.
func NewIFlow(id, defaultBaseURL string) *IFlow {
	return &IFlow{id: id, defaultBase: defaultBaseURL}
}

func (c *IFlow) ID() string            { return c.id }
func (c *IFlow) Dialect() core.Dialect { return core.DialectOpenAI }

func (c *IFlow) endpoint(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

func (c *IFlow) headers(creds core.Credentials, stream bool) map[string]string {
	apiKey := creds.APIKey
	if apiKey == "" {
		apiKey = creds.AccessToken
	}
	sessionID := "session-" + uuid.NewString()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := iflowSignature(iflowUserAgent, sessionID, timestamp, apiKey)

	h := map[string]string{
		"Content-Type":      "application/json",
		"User-Agent":        iflowUserAgent,
		"session-id":        sessionID,
		"x-iflow-timestamp": timestamp,
		"x-iflow-signature": signature,
	}
	if creds.APIKey != "" {
		h["Authorization"] = bearer(creds.APIKey)
	} else if creds.AccessToken != "" {
		h["Authorization"] = bearer(creds.AccessToken)
	}
	if stream {
		h["Accept"] = "text/event-stream"
	}
	return mergeHeaders(h, creds.Headers)
}

// iflowSignature computes the HMAC-SHA256 hex signature over the iFlow payload.
func iflowSignature(userAgent, sessionID, timestamp, apiKey string) string {
	if apiKey == "" {
		return ""
	}
	payload := userAgent + ":" + sessionID + ":" + timestamp
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *IFlow) transformBody(body []byte, stream bool) []byte {
	if !stream {
		return body
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if _, ok := m["messages"]; ok {
		if _, has := m["stream_options"]; !has {
			m["stream_options"] = map[string]any{"include_usage": true}
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}

// Validate confirms the iFlow credential is accepted by listing models. A
// 401/403 means the key/token is rejected.
func (c *IFlow) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.APIKey == "" && creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no API key or access token", c.id)
	}
	endpoint := c.endpoint(creds)
	modelsURL := strings.TrimSuffix(endpoint, "/chat/completions") + "/models"
	if _, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", modelsURL, nil, c.headers(creds, false)); err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

func (c *IFlow) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	respBody, err := doJSON(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, false))
	if err != nil {
		return nil, err
	}
	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

func (c *IFlow) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	body = c.transformBody(body, true)

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, true))
	if err != nil {
		return nil, err
	}
	return scanOpenAISSE(ctx, c.id, req.Model, resp, c.codec, cfg), nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping.
func (c *IFlow) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	body = c.transformBody(body, true)

	resp, err := openStream(ctx, c.id, req.Model, c.endpoint(creds), body, c.headers(creds, true))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}
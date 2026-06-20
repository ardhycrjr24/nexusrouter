package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	qoderlib "github.com/ardhycrjr24/nexusrouter/backend/internal/qoder"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// Qoder drives Qoder's COSY-signed inference endpoint at api3.qoder.sh.
// Unlike standard OpenAI-compatible providers, every Qoder request requires:
//   - A custom payload shape (chat_context, model_config, business blocks)
//   - WAF-bypass body encoding (&Encode=1)
//   - COSY signing (RSA+AES+MD5 with ~17 Cosy-* headers)
//   - SSE envelope unwrapping ({statusCodeValue, body} → plain OpenAI chunks)
type Qoder struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec

	// Model catalog cache (COSY-signed /algo/api/v2/model/list).
	mu      sync.RWMutex
	catalog map[string]*qoderCatalogEntry // keyed by user_id
}

// qoderCatalogEntry is a cached model catalog for one Qoder account.
type qoderCatalogEntry struct {
	fetchedAt  time.Time
	rawConfigs map[string]json.RawMessage // key → full model_config JSON
}

const qoderCatalogTTL = 1 * time.Hour

// NewQoder builds a Qoder connector.
func NewQoder(id, defaultBaseURL string) *Qoder {
	return &Qoder{
		id:          id,
		defaultBase: defaultBaseURL,
		catalog:     make(map[string]*qoderCatalogEntry),
	}
}

func (c *Qoder) ID() string            { return c.id }
func (c *Qoder) Dialect() core.Dialect { return core.DialectQoder }

// --- Credential helpers -----------------------------------------------------

// cosyCreds extracts the COSY signing material from core.Credentials.
// The Qoder OAuth flow stores user_id and machine_id in creds.Extra at
// connect time; the vault surfaces them here.
func (c *Qoder) cosyCreds(creds core.Credentials) qoderlib.CosyCreds {
	return qoderlib.CosyCreds{
		UserID:    creds.Extra["user_id"],
		AuthToken: creds.AccessToken,
		Name:      creds.Extra["email"], // best-effort; COSY name is optional
		Email:     creds.Extra["email"],
		MachineID: creds.Extra["machine_id"],
	}
}

// validateCreds returns an error when the credential is missing the fields
// required for COSY signing.
func (c *Qoder) validateCreds(creds core.Credentials) error {
	cc := c.cosyCreds(creds)
	if cc.UserID == "" {
		return &core.ProviderError{Kind: core.ErrAuth, Provider: c.id, Message: "qoder credential is missing user_id; reconnect the account"}
	}
	if cc.AuthToken == "" {
		return &core.ProviderError{Kind: core.ErrAuth, Provider: c.id, Message: "qoder credential is missing access token; reconnect the account"}
	}
	return nil
}

// --- Request body building --------------------------------------------------

// qoderPayload is the exact JSON shape Qoder's chat endpoint expects.
type qoderPayload struct {
	RequestID    string            `json:"request_id"`
	RequestSetID string            `json:"request_set_id"`
	ChatRecordID string            `json:"chat_record_id"`
	SessionID    string            `json:"session_id"`
	Stream       bool              `json:"stream"`
	ChatTask     string            `json:"chat_task"`
	IsReply      bool              `json:"is_reply"`
	IsRetry      bool              `json:"is_retry"`
	Source       int               `json:"source"`
	Version      string            `json:"version"`
	SessionType  string            `json:"session_type"`
	AgentID      string            `json:"agent_id"`
	TaskID       string            `json:"task_id"`
	CodeLanguage string            `json:"code_language"`
	ChatPrompt   string            `json:"chat_prompt"`
	ImageURLs    any               `json:"image_urls"`
	AliyunUser   string            `json:"aliyun_user_type"`
	System       string            `json:"system"`
	Messages     []qoderMessage    `json:"messages"`
	Tools        []json.RawMessage `json:"tools"`
	Parameters   qoderParams       `json:"parameters"`
	ChatContext  qoderChatContext  `json:"chat_context"`
	ModelConfig  json.RawMessage   `json:"model_config"`
	Business     qoderBusiness     `json:"business"`
}

type qoderMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qoderParams struct {
	MaxTokens int `json:"max_tokens"`
}

type qoderChatContext struct {
	ChatPrompt string            `json:"chatPrompt"`
	ImageURLs  any               `json:"imageUrls"`
	Extra      qoderChatExtra    `json:"extra"`
	Features   []json.RawMessage `json:"features"`
	Text       string            `json:"text"`
}

type qoderChatExtra struct {
	Context         []json.RawMessage `json:"context"`
	ModelConfig     qoderModelRef     `json:"modelConfig"`
	OriginalContent string            `json:"originalContent"`
}

type qoderModelRef struct {
	Key         string `json:"key"`
	IsReasoning bool   `json:"is_reasoning"`
}

type qoderBusiness struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Type    string `json:"type"`
	Stage   string `json:"stage"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	BeginAt int64  `json:"begin_at"`
}

// normalizeMessages hoists system messages out and flattens content to plain
// text.
func normalizeQoderMessages(msgs []core.Message) (out []qoderMessage, systemText string) {
	var sysParts []string
	for _, m := range msgs {
		text := m.TextContent()
		if m.Role == core.RoleSystem {
			if text != "" {
				sysParts = append(sysParts, text)
			}
			continue
		}
		out = append(out, qoderMessage{
			Role:    string(m.Role),
			Content: text,
		})
	}
	return out, strings.Join(sysParts, "\n\n")
}

// lastUserText returns the text of the last user message (for chat_context).
func lastUserText(msgs []qoderMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && msgs[i].Content != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// stableHash produces a deterministic short hex digest for session/record ids.
func stableHash(prefix string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(prefix))
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// stableChatRecordID generates a deterministic id from the request content.
func stableChatRecordID(model string, msgs []qoderMessage, tools []json.RawMessage, maxTokens int) string {
	h := sha256.New()
	h.Write([]byte("qoder-record"))
	h.Write([]byte{0})
	h.Write([]byte(model))
	for _, m := range msgs {
		h.Write([]byte{0})
		h.Write([]byte(m.Role))
		if m.Content != "" {
			h.Write([]byte{0})
			h.Write([]byte(m.Content))
		}
	}
	if len(tools) > 0 {
		h.Write([]byte{0})
		for _, t := range tools {
			h.Write(t)
		}
	}
	h.Write([]byte(fmt.Sprintf("\x00mt=%d", maxTokens)))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// resolveModelKey strips a "qoder/" prefix from the model name.
func resolveModelKey(model string) string {
	return strings.TrimPrefix(model, "qoder/")
}

// resolveMaxTokens determines the output token limit from the request and
// model config, defaulting to 32768.
func resolveMaxTokens(req *core.ChatRequest, modelConfig json.RawMessage) int {
	maxTokens := 32768

	// Extract max_output_tokens from model config if available.
	if modelConfig != nil {
		var cfg struct {
			MaxOutputTokens int `json:"max_output_tokens"`
		}
		if json.Unmarshal(modelConfig, &cfg) == nil && cfg.MaxOutputTokens > 0 {
			maxTokens = cfg.MaxOutputTokens
		}
	}

	if req.MaxTokens != nil && *req.MaxTokens > 0 && *req.MaxTokens < maxTokens {
		maxTokens = *req.MaxTokens
	}
	return maxTokens
}

// serializeTools converts core.Tool slice to Qoder's OpenAI-compatible tool
// format. Qoder validates that every tool has a function object, but also
// rejects malformed/empty parameter schemas.
func serializeTools(tools []core.Tool) []json.RawMessage {
	if len(tools) == 0 {
		return nil
	}
	out := make([]json.RawMessage, 0, len(tools))
	for _, t := range tools {
		toolJSON := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  normalizeQoderToolSchema(t.Parameters),
			},
		}
		b, err := json.Marshal(toolJSON)
		if err != nil {
			continue
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeQoderToolSchema(raw json.RawMessage) json.RawMessage {
	defaultSchema := json.RawMessage(`{"type":"object","properties":{}}`)
	if len(raw) == 0 {
		return defaultSchema
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return defaultSchema
	}
	if schema["type"] == nil {
		schema["type"] = "object"
	}
	if schema["properties"] == nil {
		schema["properties"] = map[string]any{}
	}
	out, err := json.Marshal(schema)
	if err != nil {
		return defaultSchema
	}
	return out
}

// buildPayload constructs the Qoder chat request payload from a canonical
// ChatRequest and the cached model config.
func (c *Qoder) buildPayload(req *core.ChatRequest, modelKey string, modelConfig json.RawMessage) qoderPayload {
	msgs, systemText := normalizeQoderMessages(req.Messages)
	tools := serializeTools(req.Tools)
	maxTokens := resolveMaxTokens(req, modelConfig)
	lastUser := lastUserText(msgs)

	sessionKey := req.Metadata.ContextAffinityKey
	if sessionKey == "" {
		sessionKey = modelKey
	}
	sessionID := stableHash("qoder-session", sessionKey, modelKey)
	recordID := stableChatRecordID(modelKey, msgs, tools, maxTokens)

	// Determine is_reasoning from model_config.
	var isReasoning bool
	if modelConfig != nil {
		var cfg struct {
			IsReasoning bool `json:"is_reasoning"`
		}
		_ = json.Unmarshal(modelConfig, &cfg)
		isReasoning = cfg.IsReasoning
	}

	// Default model_config when the live catalog hasn't been fetched yet.
	mc := modelConfig
	if mc == nil {
		mc, _ = json.Marshal(map[string]any{
			"key":          modelKey,
			"is_reasoning": isReasoning,
		})
	}

	return qoderPayload{
		RequestID:    uuid.NewString(),
		RequestSetID: recordID,
		ChatRecordID: recordID,
		SessionID:    sessionID,
		Stream:       true,
		ChatTask:     "FREE_INPUT",
		IsReply:      true,
		IsRetry:      false,
		Source:       1,
		Version:      "3",
		SessionType:  "qodercli",
		AgentID:      "agent_common",
		TaskID:       "common",
		CodeLanguage: "",
		ChatPrompt:   "",
		ImageURLs:    nil,
		AliyunUser:   "",
		System:       systemText,
		Messages:     msgs,
		Tools:        tools,
		Parameters:   qoderParams{MaxTokens: maxTokens},
		ChatContext: qoderChatContext{
			ChatPrompt: "",
			ImageURLs:  nil,
			Extra: qoderChatExtra{
				Context:         []json.RawMessage{},
				ModelConfig:     qoderModelRef{Key: modelKey, IsReasoning: isReasoning},
				OriginalContent: lastUser,
			},
			Features: []json.RawMessage{},
			Text:     lastUser,
		},
		ModelConfig: mc,
		Business: qoderBusiness{
			Product: "cli",
			Version: "1.0.0",
			Type:    "agent",
			Stage:   "start",
			ID:      uuid.NewString(),
			Name:    truncate(lastUser, 30),
			BeginAt: time.Now().UnixMilli(),
		},
	}
}

// --- Model catalog ----------------------------------------------------------

// fetchModelCatalog fetches the live model list from api3.qoder.sh and caches
// the raw model_config blocks by key. This is required because Qoder silently
// downgrades to a different model when the wrong model_config is sent.
func (c *Qoder) fetchModelCatalog(ctx context.Context, creds core.Credentials) (map[string]json.RawMessage, error) {
	cc := c.cosyCreds(creds)
	cacheKey := cc.UserID

	// Check cache first.
	c.mu.RLock()
	entry, ok := c.catalog[cacheKey]
	c.mu.RUnlock()
	if ok && time.Since(entry.fetchedAt) < qoderCatalogTTL {
		return entry.rawConfigs, nil
	}

	// Fetch fresh catalog.
	cosyHeaders, err := qoderlib.BuildCosyHeaders(nil, qoderlib.ModelListURL, cc)
	if err != nil {
		return nil, fmt.Errorf("qoder: build model list headers: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qoderlib.ModelListURL, nil)
	if err != nil {
		return nil, fmt.Errorf("qoder: build model list request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	for k, v := range cosyHeaders {
		req.Header.Set(k, v)
	}

	resp, err := proxyClient(ctx).Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder: model list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("qoder: model list returned %d: %s", resp.StatusCode, truncateError(body))
	}

	var catalog struct {
		Chat []json.RawMessage `json:"chat"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, fmt.Errorf("qoder: parse model list: %w", err)
	}

	configs := make(map[string]json.RawMessage, len(catalog.Chat))
	for _, entry := range catalog.Chat {
		var key struct {
			Key string `json:"key"`
		}
		if json.Unmarshal(entry, &key) != nil || key.Key == "" {
			continue
		}
		configs[key.Key] = entry
	}

	// Update cache.
	c.mu.Lock()
	c.catalog[cacheKey] = &qoderCatalogEntry{
		fetchedAt:  time.Now(),
		rawConfigs: configs,
	}
	c.mu.Unlock()

	return configs, nil
}

// modelConfigForKey resolves the model_config block for a given model key,
// fetching the catalog if needed.
func (c *Qoder) modelConfigForKey(ctx context.Context, creds core.Credentials, modelKey string) (json.RawMessage, error) {
	configs, err := c.fetchModelCatalog(ctx, creds)
	if err != nil {
		return nil, err
	}
	cfg, ok := configs[modelKey]
	if !ok {
		return nil, fmt.Errorf("qoder: model_config for %q not found in live catalog", modelKey)
	}
	return cfg, nil
}

// --- Chat / Stream ----------------------------------------------------------

// signedRequest builds the COSY-signed, WAF-encoded request for the Qoder chat
// endpoint. Returns the URL, headers, and encoded body ready for sending.
func (c *Qoder) signedRequest(payload qoderPayload, creds core.Credentials) (url string, headers map[string]string, body []byte, err error) {
	cc := c.cosyCreds(creds)
	url = qoderlib.ChatURLEncoded

	plainBody, err := json.Marshal(payload)
	if err != nil {
		return "", nil, nil, fmt.Errorf("qoder: marshal payload: %w", err)
	}

	// Apply WAF-bypass encoding.
	encodedBody := qoderlib.EncodeBody(plainBody)

	// Build COSY headers over the encoded body bytes.
	cosyHeaders, err := qoderlib.BuildCosyHeaders(encodedBody, url, cc)
	if err != nil {
		return "", nil, nil, fmt.Errorf("qoder: COSY signing: %w", err)
	}

	headers = map[string]string{
		"Content-Type":    "application/json",
		"Accept":          "text/event-stream",
		"Cache-Control":   "no-cache",
		"Accept-Encoding": "identity", // gzip breaks CDN signature validation
	}
	for k, v := range cosyHeaders {
		headers[k] = v
	}

	return url, headers, encodedBody, nil
}

// Stream opens a COSY-signed SSE connection and unwraps Qoder's
// {statusCodeValue, body} envelope into canonical OpenAI chunks.
func (c *Qoder) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	if err := c.validateCreds(creds); err != nil {
		return nil, err
	}

	modelKey := resolveModelKey(req.Model)
	modelConfig, err := c.modelConfigForKey(ctx, creds, modelKey)
	if err != nil {
		// Non-fatal: use a minimal model_config and let upstream decide.
		modelConfig = nil
	}

	payload := c.buildPayload(req, modelKey, modelConfig)
	payload.Stream = true

	url, headers, body, err := c.signedRequest(payload, creds)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	resp, err := openStream(ctx, c.id, req.Model, url, body, headers)
	if err != nil {
		return nil, err
	}

	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		ttft := newTTFTTracker(cfg)

		scanner := sseScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			inner, ok, qerr := unwrapQoderSSELineWithError(line, c.id, req.Model)
			if qerr != nil {
				out <- core.StreamChunk{Type: core.ChunkError, Err: qerr}
				return
			}
			if !ok {
				continue
			}
			if inner == "[DONE]" {
				return
			}

			// Sanitize embedded newlines so the SSE frame is a single event.
			inner = strings.ReplaceAll(inner, "\n", "")
			inner = strings.ReplaceAll(inner, "\r", "")

			chunks, perr := c.codec.ParseStreamLine([]byte(inner), req.Model)
			if perr != nil {
				continue // skip malformed chunk
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
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err},
			}
		}
	}()
	return out, nil
}

// Chat performs a non-streaming completion by collecting the streaming
// response. Qoder's chat endpoint is SSE-only.
func (c *Qoder) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = true // Qoder is SSE-only

	chunks, err := c.Stream(ctx, req, creds, core.StreamConfig{})
	if err != nil {
		return nil, err
	}

	// Collect all chunks into a complete response.
	var (
		textBuf      strings.Builder
		thinkingBuf  strings.Builder
		toolCalls    []*core.ToolCall
		finishReason core.FinishReason
		usage        core.Usage
	)

	for ch := range chunks {
		switch ch.Type {
		case core.ChunkText:
			textBuf.WriteString(ch.Delta)
		case core.ChunkThinking:
			thinkingBuf.WriteString(ch.Delta)
		case core.ChunkToolCall:
			if ch.ToolCall != nil {
				toolCalls = append(toolCalls, ch.ToolCall)
			}
		case core.ChunkFinish:
			finishReason = ch.FinishReason
		case core.ChunkUsage:
			if ch.Usage != nil {
				usage = *ch.Usage
			}
		case core.ChunkError:
			if ch.Err != nil {
				return nil, ch.Err
			}
		}
	}

	msg := core.Message{
		Role: core.RoleAssistant,
	}
	if text := textBuf.String(); text != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: text})
	}
	if thinking := thinkingBuf.String(); thinking != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: thinking})
	}
	for _, tc := range toolCalls {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartToolCall, ToolCall: tc})
	}

	return &core.ChatResponse{
		ID:           uuid.NewString(),
		Model:        req.Model,
		Message:      msg,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}

// Validate probes the Qoder model list endpoint to confirm the COSY signing
// and credentials are accepted.
func (c *Qoder) Validate(ctx context.Context, creds core.Credentials) error {
	if err := c.validateCreds(creds); err != nil {
		return err
	}
	_, err := c.fetchModelCatalog(ctx, creds)
	if err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

// --- SSE envelope unwrapping ------------------------------------------------

// qoderSSEEnvelope is the wrapper Qoder puts around each OpenAI chunk.
type qoderSSEEnvelope struct {
	StatusCodeValue int    `json:"statusCodeValue"`
	Body            string `json:"body"`
}

// unwrapQoderSSELine extracts the inner OpenAI JSON from a Qoder SSE line.
// Returns ("", false) for non-data lines or empty payloads.
// Returns ("[DONE]", true) when the stream ends.
func unwrapQoderSSELine(line string) (string, bool) {
	inner, ok, _ := unwrapQoderSSELineWithError(line, "", "")
	return inner, ok
}

func unwrapQoderSSELineWithError(line, provider, model string) (string, bool, error) {
	line = strings.TrimRight(line, "\r")
	if !strings.HasPrefix(line, "data:") {
		return "", false, nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" {
		return "", false, nil
	}
	if data == "[DONE]" {
		return "[DONE]", true, nil
	}

	var env qoderSSEEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return "", false, nil
	}
	if env.StatusCodeValue != 0 && env.StatusCodeValue != 200 {
		kind := core.ErrUpstream
		switch {
		case env.StatusCodeValue == http.StatusUnauthorized || env.StatusCodeValue == http.StatusForbidden:
			kind = core.ErrAuth
		case env.StatusCodeValue == http.StatusTooManyRequests:
			kind = core.ErrRateLimit
		case env.StatusCodeValue >= 400 && env.StatusCodeValue < 500:
			kind = core.ErrBadRequest
		}
		return "", false, &core.ProviderError{
			Kind:       kind,
			Provider:   provider,
			Model:      model,
			StatusCode: env.StatusCodeValue,
			Message:    truncateError([]byte(env.Body)),
		}
	}
	if env.Body == "" || env.Body == "[DONE]" {
		if env.Body == "[DONE]" {
			return "[DONE]", true, nil
		}
		return "", false, nil
	}
	return env.Body, true, nil
}

// ListModels implements live model discovery by fetching the COSY-signed
// model list endpoint.
func (c *Qoder) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	if err := c.validateCreds(creds); err != nil {
		return nil, err
	}

	configs, err := c.fetchModelCatalog(ctx, creds)
	if err != nil {
		return nil, err
	}

	out := make([]ModelSpec, 0, len(configs))
	for key, raw := range configs {
		var entry struct {
			Enable         *bool  `json:"enable"`
			DisplayName    string `json:"display_name"`
			MaxInputTokens int    `json:"max_input_tokens"`
		}
		_ = json.Unmarshal(raw, &entry)

		// Include all models (even disabled ones — upstream still accepts them).
		name := entry.DisplayName
		if name == "" {
			name = key
		}
		out = append(out, ModelSpec{
			ID:   key,
			Name: name,
			Kind: core.ServiceLLM,
		})
	}
	return out, nil
}

// --- Helpers ----------------------------------------------------------------

// Compile-time interface checks.
var (
	_ core.Connector = (*Qoder)(nil)
	_ core.Validator = (*Qoder)(nil)
)

// QoderModelSource implements LiveModelSource for Qoder's COSY-signed model
// list endpoint.
type QoderModelSource struct {
	connector *Qoder
}

// NewQoderModelSource builds a live model source backed by the Qoder connector.
func NewQoderModelSource(conn *Qoder) *QoderModelSource {
	return &QoderModelSource{connector: conn}
}

// ListModels delegates to the connector's ListModels.
func (s *QoderModelSource) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	return s.connector.ListModels(ctx, creds)
}

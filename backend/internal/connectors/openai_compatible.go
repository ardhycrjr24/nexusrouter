package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// OpenAICompatible drives any endpoint that speaks the OpenAI Chat Completions
// API: OpenAI itself, plus GLM, MiniMax, DeepSeek, Groq, Together, and custom
// gateways. The provider id and default base URL are supplied at construction
// so one implementation backs many registered providers.
type OpenAICompatible struct {
	id          string
	defaultBase string
	codec       transform.OpenAICodec
}

// NewOpenAICompatible builds a connector for an OpenAI-compatible provider.
func NewOpenAICompatible(id, defaultBaseURL string) *OpenAICompatible {
	return &OpenAICompatible{id: id, defaultBase: defaultBaseURL}
}

func (c *OpenAICompatible) ID() string            { return c.id }
func (c *OpenAICompatible) Dialect() core.Dialect { return core.DialectOpenAI }

func (c *OpenAICompatible) baseURL(creds core.Credentials) string {
	u := c.defaultBase
	if creds.BaseURL != "" {
		u = creds.BaseURL
	}
	// Resolve template placeholders like {accountId} from creds.Extra.
	// Cloudflare Workers AI uses: /accounts/{accountId}/ai/v1/chat/completions
	for key, val := range creds.Extra {
		u = strings.ReplaceAll(u, "{"+key+"}", val)
	}
	return u
}

func (c *OpenAICompatible) headers(creds core.Credentials) map[string]string {
	h := map[string]string{}
	if c.id == "azure" {
		switch {
		case creds.AccessToken != "":
			h["Authorization"] = bearer(creds.AccessToken)
		case creds.APIKey != "":
			h["api-key"] = creds.APIKey
		}
		if org := creds.Extra["organization"]; org != "" {
			h["OpenAI-Organization"] = org
		}
		return mergeHeaders(h, creds.Headers)
	}

	// Cline requires a workos: prefix on the access token and custom headers.
	if c.id == "cline" {
		tok := creds.AccessToken
		if tok == "" {
			tok = creds.APIKey
		}
		if tok != "" && !strings.HasPrefix(tok, "workos:") {
			tok = "workos:" + tok
		}
		h["Authorization"] = bearer(tok)
		h["HTTP-Referer"] = "https://cline.bot"
		h["X-Title"] = "Cline"
		h["X-CLIENT-TYPE"] = "nexusrouter"
		h["X-PLATFORM"] = "unknown"
		h["X-IS-MULTIROOT"] = "false"
		return mergeHeaders(h, creds.Headers)
	}

	switch {
	case creds.AccessToken != "":
		h["Authorization"] = bearer(creds.AccessToken)
	case creds.APIKey != "":
		h["Authorization"] = bearer(creds.APIKey)
	}
	return mergeHeaders(h, creds.Headers)
}

func (c *OpenAICompatible) chatCompletionsURL(creds core.Credentials, model string) string {
	if c.id == "azure" {
		endpoint := strings.TrimRight(creds.Extra["azure_endpoint"], "/")
		if endpoint == "" {
			endpoint = strings.TrimRight(creds.BaseURL, "/")
		}
		deployment := creds.Extra["deployment"]
		if deployment == "" {
			deployment = model
		}
		if deployment == "" {
			deployment = "gpt-4"
		}
		apiVersion := creds.Extra["api_version"]
		if apiVersion == "" {
			apiVersion = "2024-10-01-preview"
		}
		return endpoint + "/openai/deployments/" + url.PathEscape(deployment) +
			"/chat/completions?api-version=" + url.QueryEscape(apiVersion)
	}
	return joinURL(c.baseURL(creds), "chat/completions")
}

// Chat performs a non-streaming completion.
func (c *OpenAICompatible) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := c.chatCompletionsURL(creds, req.Model)

	// Use streaming JSON decode when the codec supports it — avoids buffering
	// the entire response body into a []byte before parsing.
	if sc, ok := interface{}(c.codec).(transform.StreamingResponseCodec); ok {
		_, respBody, decErr := doJSONDecode(ctx, c.id, req.Model, url, body, c.headers(creds))
		if decErr != nil {
			return nil, decErr
		}
		defer respBody.Close()
		resp, perr := sc.ParseResponseFrom(respBody, req.Model)
		if perr != nil {
			return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: perr.Error(), Cause: perr}
		}
		return resp, nil
	}

	// Fallback: buffer the entire response body.
	respBody, err := doJSON(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

// Validate probes the upstream /models endpoint to confirm the credentials are
// accepted. Returns nil on success.
func (c *OpenAICompatible) Validate(ctx context.Context, creds core.Credentials) error {
	if c.id == "azure" {
		body := []byte(`{"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`)
		err := validateProbe(ctx, c.id, c.chatCompletionsURL(creds, "validate"), body, c.headers(creds))
		if err != nil {
			return fmt.Errorf("validation failed for %s: %w", c.id, err)
		}
		return nil
	}

	url := joinURL(c.baseURL(creds), "models")
	_, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", url, nil, c.headers(creds))
	if err == nil {
		return nil
	}
	if c.id == "xai" {
		pe := core.AsProviderError(err)
		if pe.StatusCode == http.StatusForbidden {
			return nil
		}
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	if strictModelsValidation(c.id) {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	if validationAuthError(err) || !validationReachedUpstream(err) {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}

	// Many 9router-compatible providers either omit /models or reject unknown
	// probe models with 400/404 while still accepting the credential. Fall back
	// to a minimal chat request and treat any non-auth HTTP response as proof
	// that the connection reached the provider.
	probeModel := firstCatalogModel(c.id)
	body, _ := json.Marshal(map[string]any{
		"model": probeModel,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	})
	if err := validateProbe(ctx, c.id, c.chatCompletionsURL(creds, probeModel), body, c.headers(creds)); err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

func validateProbe(ctx context.Context, provider, endpoint string, body []byte, headers map[string]string) error {
	_, err := doJSON(ctx, provider, "validate", endpoint, body, headers)
	if err == nil {
		return nil
	}
	if validationAuthError(err) || !validationReachedUpstream(err) {
		return err
	}
	return nil
}

func validationAuthError(err error) bool {
	return core.AsProviderError(err).Kind == core.ErrAuth
}

func validationReachedUpstream(err error) bool {
	return core.AsProviderError(err).StatusCode > 0
}

func firstCatalogModel(provider string) string {
	for _, m := range ModelsForProvider(provider) {
		if m.Kind == core.ServiceLLM {
			return m.ID
		}
	}
	return "test"
}

func strictModelsValidation(provider string) bool {
	switch provider {
	case "openai", "openrouter", "vercel-ai-gateway",
		"deepseek", "groq", "mistral", "perplexity", "together",
		"fireworks", "cerebras", "cohere", "nebius", "siliconflow",
		"hyperbolic", "chutes", "nvidia", "xiaomi-mimo", "xiaomi-tokenplan":
		return true
	default:
		return false
	}
}

// OpenAICompatibleModelSource implements LiveModelSource by fetching the
// upstream GET /models endpoint — the standard discovery API for OpenAI-
// compatible providers. This auto-discovers models at runtime using the
// connected account's credentials.
type OpenAICompatibleModelSource struct {
	provider    string
	defaultBase string
}

// ListModels fetches GET /models from the upstream and returns ModelSpecs.
func (s *OpenAICompatibleModelSource) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	base := s.defaultBase
	if creds.BaseURL != "" {
		base = creds.BaseURL
	}
	// Resolve template placeholders (e.g. cloudflare {accountId}).
	for key, val := range creds.Extra {
		base = strings.ReplaceAll(base, "{"+key+"}", val)
	}

	url := joinURL(base, "models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	switch {
	case creds.AccessToken != "":
		req.Header.Set("Authorization", bearer(creds.AccessToken))
	case creds.APIKey != "":
		req.Header.Set("Authorization", bearer(creds.APIKey))
	}
	req.Header.Set("Accept", "application/json")

	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("GET /models returned %d: %s", resp.StatusCode, truncateError(body))
	}

	// Parse the standard OpenAI models response shape.
	var envelope struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decode /models response: %w", err)
	}

	out := make([]ModelSpec, 0, len(envelope.Data))
	for _, entry := range envelope.Data {
		if entry.ID == "" {
			continue
		}
		out = append(out, ModelSpec{
			ID:   entry.ID,
			Name: entry.ID, // best-effort; static catalog may have a better name
			Kind: core.ServiceLLM,
		})
	}
	return out, nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping. The caller must close body when done.
func (c *OpenAICompatible) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := c.chatCompletionsURL(creds, req.Model)
	resp, err := openStream(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// Stream performs a streaming completion, emitting canonical chunks.
func (c *OpenAICompatible) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequestForProvider(req, c.id)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := c.chatCompletionsURL(creds, req.Model)
	resp, err := openStream(ctx, c.id, req.Model, url, body, c.headers(creds))
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

			payload, ok := parseSSEData(scanner.Text())
			if !ok {
				continue
			}
			chunks, perr := c.codec.ParseStreamLine([]byte(payload), req.Model)
			if perr != nil {
				// Skip a single malformed chunk rather than aborting the stream.
				continue
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

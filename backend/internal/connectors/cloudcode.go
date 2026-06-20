package connectors

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// CloudCode drives Google's internal CloudCode Assist endpoints used by the
// Gemini CLI and Antigravity. These speak the Gemini generateContent format but
// wrap it: the request body is {project, model, request: <gemini>} and each
// streamed response chunk is {response: <gemini-chunk>}. This connector reuses
// the Gemini codec for the inner format and adds the CloudCode wrap/unwrap, plus
// the per-provider quirks (gemini-cli vs antigravity) of 9router's executors.
type CloudCode struct {
	id          string
	defaultBase string
	variant     cloudCodeVariant
	codec       transform.GeminiCodec
}

type cloudCodeVariant int

const (
	variantGeminiCLI cloudCodeVariant = iota
	variantAntigravity
)

// Gemini CLI client fingerprint, mirroring 9router's appConstants.js.
const (
	geminiCLIVersion   = "0.34.0"
	geminiCLIAPIClient = "google-genai-sdk/1.41.0 gl-node/v22.19.0"
	antigravityUA      = "antigravity/1.104.0"
)

// NewGeminiCLI builds a CloudCode connector for the Gemini CLI provider.
func NewGeminiCLI(id, defaultBaseURL string) *CloudCode {
	return &CloudCode{id: id, defaultBase: defaultBaseURL, variant: variantGeminiCLI}
}

// NewAntigravity builds a CloudCode connector for the Antigravity provider.
func NewAntigravity(id, defaultBaseURL string) *CloudCode {
	return &CloudCode{id: id, defaultBase: defaultBaseURL, variant: variantAntigravity}
}

func (c *CloudCode) ID() string { return c.id }
func (c *CloudCode) Dialect() core.Dialect {
	if c.variant == variantAntigravity {
		return core.DialectAntigravity
	}
	return core.DialectGeminiCLI
}

func (c *CloudCode) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// url builds the CloudCode endpoint. Gemini CLI appends the action directly to
// the base (which already ends in /v1internal); Antigravity appends
// "/v1internal:<action>" to a bare host base.
func (c *CloudCode) url(creds core.Credentials, stream bool) string {
	base := strings.TrimRight(c.baseURL(creds), "/")
	action := "generateContent"
	if stream {
		action = "streamGenerateContent?alt=sse"
	}
	if c.variant == variantAntigravity {
		return base + "/v1internal:" + action
	}
	return base + ":" + action
}

func (c *CloudCode) headers(creds core.Credentials, stream bool) map[string]string {
	accept := "application/json"
	if stream {
		accept = "text/event-stream"
	}
	h := map[string]string{
		"Authorization": bearer(creds.AccessToken),
		"Accept":        accept,
	}
	if c.variant == variantAntigravity {
		h["User-Agent"] = antigravityUA
		h["x-request-source"] = "local" // INTERNAL_REQUEST_HEADER (anti-loop)
		if sid := creds.Extra["session_id"]; sid != "" {
			h["X-Machine-Session-Id"] = sid
		}
	} else {
		h["User-Agent"] = "GeminiCLI/" + geminiCLIVersion
		h["X-Goog-Api-Client"] = geminiCLIAPIClient
	}
	return mergeHeaders(h, creds.Headers)
}

// Validate confirms an OAuth access token is present. CloudCode endpoints only
// expose a generate path (no cheap models/userinfo probe) and a live probe
// would consume quota, so this is a presence check.
func (c *CloudCode) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no access token", c.id)
	}
	return nil
}

// wrapRequest renders the canonical request to the inner Gemini body, then wraps
// it in the CloudCode envelope expected by the provider.
func (c *CloudCode) wrapRequest(req *core.ChatRequest, creds core.Credentials) ([]byte, error) {
	inner, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, err
	}
	var innerObj map[string]any
	if err := json.Unmarshal(inner, &innerObj); err != nil {
		return nil, err
	}

	projectID := creds.Extra["project_id"]
	if projectID == "" {
		projectID = generateCloudCodeProjectID()
	}

	if c.variant == variantAntigravity {
		// Antigravity adds session id + agent metadata to the inner request.
		sessionID := req.Metadata.ContextAffinityKey
		if sessionID != "" {
			sessionID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(sessionID)).String()
		}
		if sessionID == "" {
			sessionID = creds.Extra["session_id"]
		}
		if sessionID == "" {
			sessionID = deriveCloudCodeSession(creds)
		}
		innerObj["sessionId"] = sessionID
		if tools, ok := innerObj["tools"].([]any); ok && len(tools) > 0 {
			innerObj["toolConfig"] = map[string]any{
				"functionCallingConfig": map[string]any{"mode": "VALIDATED"},
			}
		}
		envelope := map[string]any{
			"project":     projectID,
			"model":       req.Model,
			"userAgent":   "antigravity",
			"requestType": "agent",
			"requestId":   "agent-" + uuid.NewString(),
			"request":     innerObj,
		}
		return json.Marshal(envelope)
	}

	envelope := map[string]any{
		"project": projectID,
		"model":   req.Model,
		"request": innerObj,
	}
	return json.Marshal(envelope)
}

// unwrapResponse extracts the inner Gemini body from a CloudCode unary response
// ({response: <gemini>}), falling back to the body itself when not wrapped.
func unwrapCloudCodeResponse(body []byte) []byte {
	var wrapper struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Response) > 0 {
		return wrapper.Response
	}
	return body
}

// Chat performs a non-streaming CloudCode call.
func (c *CloudCode) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.wrapRequest(req, creds)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	respBody, err := doJSON(ctx, c.id, req.Model, c.url(creds, false), body, c.headers(creds, false))
	if err != nil {
		return nil, err
	}

	inner := unwrapCloudCodeResponse(respBody)
	resp, err := c.codec.ParseResponse(inner, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

// Stream performs a streaming CloudCode call, unwrapping {response: ...} from
// each SSE chunk before handing the inner Gemini chunk to the codec.
func (c *CloudCode) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.wrapRequest(req, creds)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	resp, err := openStream(ctx, c.id, req.Model, c.url(creds, true), body, c.headers(creds, true))
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
			inner := unwrapCloudCodeResponse([]byte(payload))
			chunks, perr := c.codec.ParseStreamLine(inner, req.Model)
			if perr != nil {
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

// generateCloudCodeProjectID mirrors 9router's generateProjectId:
// "<adj>-<noun>-<5 hex>".
func generateCloudCodeProjectID() string {
	adjs := []string{"useful", "bright", "swift", "calm", "bold"}
	nouns := []string{"fuze", "wave", "spark", "flow", "core"}
	return adjs[randIndex(len(adjs))] + "-" + nouns[randIndex(len(nouns))] + "-" + randomHex(3)[:5]
}

// deriveCloudCodeSession derives a stable session id from the account identity
// (email/connection), mirroring 9router's deriveSessionId intent.
func deriveCloudCodeSession(creds core.Credentials) string {
	seed := creds.Extra["email"]
	if seed == "" {
		seed = creds.AccountID
	}
	if seed == "" {
		return uuid.NewString()
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(seed)).String()
}

func randIndex(n int) int {
	if n <= 0 {
		return 0
	}
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return int(b[0]) % n
}

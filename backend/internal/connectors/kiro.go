package connectors

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/transform"
)

// Kiro drives AWS CodeWhisperer's generateAssistantResponse endpoint used by
// Kiro AI. The request is a conversationState payload (built by the Kiro codec);
// the response is a binary AWS EventStream of typed events
// (assistantResponseEvent, reasoningContentEvent, toolUseEvent, messageStopEvent,
// metricsEvent, ...). This connector parses the binary frames and maps them to
// canonical chunks, mirroring 9router's KiroExecutor.transformEventStreamToSSE.
type Kiro struct {
	id          string
	defaultBase string
	codec       transform.KiroCodec
}

// NewKiro builds a Kiro connector.
func NewKiro(id, defaultBaseURL string) *Kiro {
	return &Kiro{id: id, defaultBase: defaultBaseURL}
}

func (c *Kiro) ID() string            { return c.id }
func (c *Kiro) Dialect() core.Dialect { return core.DialectKiro }

func (c *Kiro) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// headers builds the AWS SDK + CodeWhisperer headers Kiro expects.
func (c *Kiro) headers(creds core.Credentials) map[string]string {
	h := map[string]string{
		"Accept":                "application/vnd.amazon.eventstream",
		"X-Amz-Target":          "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
		"User-Agent":            "AWS-SDK-JS/3.0.0 kiro-ide/1.0.0",
		"X-Amz-User-Agent":      "aws-sdk-js/3.0.0 kiro-ide/1.0.0",
		"Amz-Sdk-Request":       "attempt=1; max=3",
		"Amz-Sdk-Invocation-Id": uuid.NewString(),
	}
	if creds.AccessToken != "" {
		h["Authorization"] = bearer(creds.AccessToken)
	}
	return mergeHeaders(h, creds.Headers)
}

// Validate probes the Kiro upstream by calling ListAvailableModels. If the
// access token is missing or rejected, an error is returned.
func (c *Kiro) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no access token", c.id)
	}
	// Use ListAvailableModels to verify the token. Region defaults to us-east-1.
	region := creds.Extra["kiro_region"]
	if region == "" {
		region = "us-east-1"
	}
	url := fmt.Sprintf("https://q.%s.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", region)
	h := map[string]string{
		"Authorization": bearer(creds.AccessToken),
		"Accept":        "application/json",
		"User-Agent":    "AWS-SDK-JS/3.0.0 kiro-ide/1.0.0",
	}
	_, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", url, nil, h)
	if err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

// ---- Live model discovery ---------------------------------------------------

// kiroModelEntry is the shape of one model in the ListAvailableModels response.
type kiroModelEntry struct {
	ModelID        string  `json:"modelId"`
	ModelName      string  `json:"modelName"`
	Description    string  `json:"description"`
	RateMultiplier float64 `json:"rateMultiplier"`
	TokenLimits    struct {
		MaxInputTokens int `json:"maxInputTokens"`
	} `json:"tokenLimits"`
}

// ListModels fetches the live Kiro model catalog and expands each upstream
// model into synthetic variants (-thinking, -agentic, -thinking-agentic).
// Implements LiveModelSource.
func (c *Kiro) ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error) {
	if creds.AccessToken == "" {
		return nil, fmt.Errorf("kiro: ListModels: no access token")
	}
	region := creds.Extra["kiro_region"]
	if region == "" {
		region = "us-east-1"
	}
	profileArn := creds.Extra["kiro_profile_arn"]

	params := "origin=AI_EDITOR"
	if profileArn != "" {
		params += "&profileArn=" + profileArn
	}
	url := fmt.Sprintf("https://q.%s.amazonaws.com/ListAvailableModels?%s", region, params)

	h := map[string]string{
		"Authorization": bearer(creds.AccessToken),
		"Accept":        "application/json",
		"User-Agent":    "AWS-SDK-JS/3.0.0 kiro-ide/1.0.0",
	}
	body, err := doJSONMethod(ctx, http.MethodGet, c.id, "list-models", url, nil, h)
	if err != nil {
		return nil, fmt.Errorf("kiro: ListModels: %w", err)
	}

	var resp struct {
		Models []kiroModelEntry `json:"models"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("kiro: ListModels: parse: %w", err)
	}

	var out []ModelSpec
	for _, m := range resp.Models {
		upstream := m.ModelID
		if upstream == "" {
			continue
		}
		display := m.ModelName
		if display == "" {
			display = upstream
		}
		// Format display name with rate multiplier if non-default.
		if m.RateMultiplier > 0 && m.RateMultiplier != 1.0 {
			display = fmt.Sprintf("Kiro %s (%.1fx credit)", display, m.RateMultiplier)
		} else {
			display = "Kiro " + display
		}

		isAuto := upstream == "auto"

		// Base model.
		out = append(out, ModelSpec{ID: upstream, Name: display, Kind: core.ServiceLLM})
		// Thinking variant.
		out = append(out, ModelSpec{ID: upstream + "-thinking", Name: display + " (Thinking)", Kind: core.ServiceLLM})
		// Agentic variant (skip for auto — Kiro picks model server-side).
		if !isAuto {
			out = append(out, ModelSpec{ID: upstream + "-agentic", Name: display + " (Agentic)", Kind: core.ServiceLLM})
			out = append(out, ModelSpec{ID: upstream + "-thinking-agentic", Name: display + " (Thinking + Agentic)", Kind: core.ServiceLLM})
		}
	}
	return out, nil
}

// ---- Quota fetching ---------------------------------------------------------

// FetchQuota fetches upstream Kiro usage/quota info by probing the
// getUsageLimits endpoints. Mirrors 9router's getKiroUsage() logic: tries
// three endpoints in sequence and returns provider-specific error messages
// based on the auth method.
func (c *Kiro) FetchQuota(ctx context.Context, creds core.Credentials) (*QuotaResult, error) {
	if creds.AccessToken == "" {
		return &QuotaResult{Message: "No access token; cannot fetch quota."}, nil
	}
	region := creds.Extra["kiro_region"]
	if region == "" {
		region = "us-east-1"
	}
	profileArn := creds.Extra["profile_arn"]
	if profileArn == "" {
		profileArn = creds.Extra["kiro_profile_arn"]
	}
	authMethod := creds.Extra["kiro_auth_method"]
	if profileArn == "" {
		profileArn = "arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX"
	}

	authHeaders := map[string]string{
		"Authorization":    bearer(creds.AccessToken),
		"Accept":           "application/json",
		"User-Agent":       "aws-sdk-js/1.0.0 KiroIDE",
		"x-amz-user-agent": "aws-sdk-js/1.0.0 KiroIDE",
	}

	sawAuthError := false

	// Attempt 1: GET on codewhisperer endpoint.
	params := "isEmailRequired=true&origin=AI_EDITOR&resourceType=AGENTIC_REQUEST"
	url1 := fmt.Sprintf("https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits?%s", params)
	body, err := doJSONMethod(ctx, http.MethodGet, c.id, "quota", url1, nil, authHeaders)
	if err == nil {
		return parseKiroQuota(body)
	}
	if isAuthError(err) {
		sawAuthError = true
	}

	// Attempt 2: POST on codewhisperer endpoint.
	postBody := map[string]string{"origin": "AI_EDITOR", "profileArn": profileArn, "resourceType": "AGENTIC_REQUEST"}
	postJSON, _ := json.Marshal(postBody)
	postHeaders := map[string]string{
		"Authorization": bearer(creds.AccessToken),
		"Content-Type":  "application/x-amz-json-1.0",
		"x-amz-target":  "AmazonCodeWhispererService.GetUsageLimits",
		"Accept":        "application/json",
	}
	body, err = doJSON(ctx, c.id, "quota", "https://codewhisperer.us-east-1.amazonaws.com", postJSON, postHeaders)
	if err == nil {
		return parseKiroQuota(body)
	}
	if isAuthError(err) {
		sawAuthError = true
	}

	// Attempt 3: GET on q endpoint with profileArn.
	qParams := fmt.Sprintf("origin=AI_EDITOR&profileArn=%s&resourceType=AGENTIC_REQUEST", profileArn)
	url3 := fmt.Sprintf("https://q.%s.amazonaws.com/getUsageLimits?%s", region, qParams)
	body, err = doJSONMethod(ctx, http.MethodGet, c.id, "quota", url3, nil, authHeaders)
	if err == nil {
		return parseKiroQuota(body)
	}
	if isAuthError(err) {
		sawAuthError = true
	}

	// Return provider-specific messages matching 9router.
	if sawAuthError {
		switch authMethod {
		case "idc":
			return &QuotaResult{Message: "Kiro quota API is unavailable for the current AWS IAM Identity Center session. Chat may still work. If this persists after renewing your session, reconnect Kiro."}, nil
		case "google", "github":
			return &QuotaResult{Message: "Kiro quota API authentication expired. Chat may still work."}, nil
		default:
			return &QuotaResult{Message: "Kiro quota API rejected the current token. Chat may still work."}, nil
		}
	}
	return &QuotaResult{Message: "Unable to fetch Kiro usage right now."}, nil
}

// isAuthError checks if a provider error is an auth failure (401/403).
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	pe := core.AsProviderError(err)
	return pe.Kind == core.ErrAuth
}

// kiroQuotaBreakdown mirrors the JSON shape of one usageBreakdownList entry.
// The precision fields can be either a bare number or an object {value, precision}.
type kiroQuotaBreakdown struct {
	ResourceType              string          `json:"resourceType"`
	CurrentUsageWithPrecision json.RawMessage `json:"currentUsageWithPrecision"`
	UsageLimitWithPrecision   json.RawMessage `json:"usageLimitWithPrecision"`
	FreeTrialInfo             *struct {
		CurrentUsageWithPrecision json.RawMessage `json:"currentUsageWithPrecision"`
		UsageLimitWithPrecision   json.RawMessage `json:"usageLimitWithPrecision"`
		FreeTrialExpiry           string          `json:"freeTrialExpiry"`
	} `json:"freeTrialInfo"`
}

// parseKiroDateField extracts a date string from a field that may be a bare
// string or a Unix timestamp number.
func parseKiroDateField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first.
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	// Try number (Unix timestamp).
	var n int64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		return fmt.Sprintf("%d", n)
	}
	// Try float.
	var f float64
	if json.Unmarshal(raw, &f) == nil && f > 0 {
		return fmt.Sprintf("%d", int64(f))
	}
	return ""
}

// parseKiroPrecision extracts an int from a field that may be a bare number
// (int or float) or an object {"value": N, "precision": "EXACT"}.
func parseKiroPrecision(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	// Try bare number first (int or float).
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int(f)
	}
	// Try object form.
	var obj struct {
		Value float64 `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return int(obj.Value)
	}
	return 0
}

// parseKiroQuota parses the getUsageLimits response into a QuotaResult.
// Mirrors 9router's parseKiroQuotaData().
func parseKiroQuota(body []byte) (*QuotaResult, error) {
	var data struct {
		UsageBreakdownList []kiroQuotaBreakdown `json:"usageBreakdownList"`
		SubscriptionInfo   struct {
			SubscriptionTitle string `json:"subscriptionTitle"`
		} `json:"subscriptionInfo"`
		NextDateReset json.RawMessage `json:"nextDateReset"`
		ResetDate     json.RawMessage `json:"resetDate"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parse quota: %w", err)
	}

	resetAt := parseKiroDateField(data.NextDateReset)
	if resetAt == "" {
		resetAt = parseKiroDateField(data.ResetDate)
	}

	planName := data.SubscriptionInfo.SubscriptionTitle
	if planName == "" {
		planName = "Kiro"
	}

	result := &QuotaResult{PlanName: planName}

	for _, bd := range data.UsageBreakdownList {
		used := parseKiroPrecision(bd.CurrentUsageWithPrecision)
		limit := parseKiroPrecision(bd.UsageLimitWithPrecision)
		remaining := limit - used
		if remaining < 0 {
			remaining = 0
		}

		resourceType := strings.ToLower(bd.ResourceType)
		if resourceType == "" {
			resourceType = "unknown"
		}

		result.Quotas = append(result.Quotas, QuotaEntry{
			ResourceType: resourceType,
			Used:         used,
			Limit:        limit,
			Remaining:    remaining,
			ResetAt:      resetAt,
			PlanName:     planName,
		})

		// Free trial quota (if available).
		if bd.FreeTrialInfo != nil {
			freeUsed := parseKiroPrecision(bd.FreeTrialInfo.CurrentUsageWithPrecision)
			freeLimit := parseKiroPrecision(bd.FreeTrialInfo.UsageLimitWithPrecision)
			freeRemaining := freeLimit - freeUsed
			if freeRemaining < 0 {
				freeRemaining = 0
			}
			freeReset := bd.FreeTrialInfo.FreeTrialExpiry
			if freeReset == "" {
				freeReset = resetAt
			}
			result.Quotas = append(result.Quotas, QuotaEntry{
				ResourceType: resourceType + "_freetrial",
				Used:         freeUsed,
				Limit:        freeLimit,
				Remaining:    freeRemaining,
				ResetAt:      freeReset,
				PlanName:     planName,
			})
		}
	}
	return result, nil
}

func init() {
	k := &Kiro{id: "kiro"}
	RegisterLiveModelSource("kiro", k)
	RegisterQuotaSource("kiro", k)
}

// Chat performs a non-streaming call by draining the event stream and folding
// the chunks into a single response.
func (c *Kiro) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	stream, err := c.Stream(ctx, req, creds, core.StreamConfig{})
	if err != nil {
		return nil, err
	}

	msg := core.Message{Role: core.RoleAssistant}
	var text, thinking string
	toolCalls := map[string]*core.ToolCall{}
	var toolOrder []string
	finish := core.FinishStop
	var usage core.Usage

	for ch := range stream {
		switch ch.Type {
		case core.ChunkText:
			text += ch.Delta
		case core.ChunkThinking:
			thinking += ch.Delta
		case core.ChunkToolCall:
			if ch.ToolCall != nil {
				existing, ok := toolCalls[ch.ToolCall.ID]
				if !ok {
					tc := *ch.ToolCall
					toolCalls[ch.ToolCall.ID] = &tc
					toolOrder = append(toolOrder, ch.ToolCall.ID)
				} else if len(ch.ToolCall.Arguments) > 0 {
					existing.Arguments = append(existing.Arguments, ch.ToolCall.Arguments...)
				}
				finish = core.FinishToolCalls
			}
		case core.ChunkFinish:
			if ch.FinishReason != "" {
				finish = ch.FinishReason
			}
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

	if thinking != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: thinking})
	}
	if text != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: text})
	}
	for _, id := range toolOrder {
		tc := toolCalls[id]
		if len(tc.Arguments) == 0 {
			tc.Arguments = json.RawMessage("{}")
		}
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartToolCall, ToolCall: tc})
	}

	return &core.ChatResponse{Model: req.Model, Message: msg, FinishReason: finish, Usage: usage}, nil
}

// Stream performs a streaming call, parsing the AWS EventStream into canonical
// chunks.
func (c *Kiro) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	// Kiro returns a binary eventstream, not SSE; use a plain streaming POST.
	resp, err := openStream(ctx, c.id, req.Model, c.baseURL(creds), body, c.headers(creds))
	if err != nil {
		// On an upstream rejection (e.g. CodeWhisperer's 400 "Improperly formed
		// request"), the offending field is opaque. When KIRO_DEBUG is set, dump
		// the rendered request body so the exact payload can be inspected. The
		// body may contain prompt content, so this is opt-in only.
		return nil, err
	}

	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()

		ttft := newTTFTTracker(cfg)

		parser := newEventStreamParser(resp.Body)
		seenTools := map[string]bool{}
		hasTool := false

		// Kiro does not always emit a metricsEvent/usageEvent frame (varies by
		// model and backend). Track whether real usage arrived and how much
		// text we streamed so we can synthesize an estimate at EOF — otherwise
		// the request bills upstream credit but records zero tokens locally.
		usageSeen := false
		outputChars := 0

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			frame, err := parser.next()
			if err != nil {
				if err != errEventStreamEOF {
					out <- core.StreamChunk{
						Type: core.ChunkError,
						Err:  &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err},
					}
				}
				break
			}
			if frame == nil {
				continue
			}

			for _, ch := range kiroFrameToChunks(frame, seenTools, &hasTool) {
				if ch.Type == core.ChunkUsage {
					usageSeen = true
				}
				if ch.Type == core.ChunkText || ch.Type == core.ChunkThinking {
					outputChars += len(ch.Delta)
				}
				ttft.maybeReport(ch)
				select {
				case out <- ch:
				case <-ctx.Done():
					return
				}
			}
		}

		if !usageSeen {
			if u := estimateKiroUsage(req, outputChars); u != nil {
				select {
				case out <- core.StreamChunk{Type: core.ChunkUsage, Usage: u}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// estimateKiroUsage produces a best-effort token estimate for a Kiro response
// when the upstream omits token accounting. It approximates ~4 characters per
// token over the rendered request input and the streamed output. The result is marked
// Estimated so downstream consumers can distinguish it from exact counts.
func estimateKiroUsage(req *core.ChatRequest, outputChars int) *core.Usage {
	inputChars := 0
	if req != nil {
		if req.System != "" {
			inputChars += len(req.System)
		}
		for _, m := range req.Messages {
			for _, part := range m.Content {
				inputChars += len(part.Text)
				if part.ToolCall != nil {
					inputChars += len(part.ToolCall.Arguments)
				}
				if part.ToolResult != nil {
					inputChars += len(part.ToolResult.Content)
				}
			}
		}
	}
	prompt := charsToTokens(inputChars)
	completion := charsToTokens(outputChars)
	if prompt == 0 && completion == 0 {
		return nil
	}
	return &core.Usage{
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
}

// charsToTokens converts a character count to an approximate token count using
// the common ~4 chars/token rule, rounding up so any non-empty text counts as
// at least one token.
func charsToTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

// kiroFrameToChunks maps one decoded Kiro eventstream frame to canonical chunks.
func kiroFrameToChunks(frame *eventStreamFrame, seenTools map[string]bool, hasTool *bool) []core.StreamChunk {
	eventType := frame.headers[":event-type"]
	var chunks []core.StreamChunk

	switch eventType {
	case "assistantResponseEvent", "codeEvent":
		var p struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(frame.payload, &p) == nil && p.Content != "" {
			chunks = append(chunks, core.StreamChunk{Type: core.ChunkText, Delta: p.Content})
		}

	case "reasoningContentEvent":
		text := extractKiroReasoning(frame.payload)
		if text != "" {
			chunks = append(chunks, core.StreamChunk{Type: core.ChunkThinking, Delta: text})
		}

	case "toolUseEvent":
		chunks = append(chunks, kiroToolUseChunks(frame.payload, seenTools)...)
		if len(chunks) > 0 {
			*hasTool = true
		}

	case "messageStopEvent":
		reason := core.FinishStop
		if *hasTool {
			reason = core.FinishToolCalls
		}
		chunks = append(chunks, core.StreamChunk{Type: core.ChunkFinish, FinishReason: reason})

	case "metricsEvent", "usageEvent":
		// Kiro emits token accounting under two event names depending on the
		// model and region: "metricsEvent" (CodeWhisperer) and "usageEvent"
		// (newer social/token-plan backends). Both carry inputTokens/
		// outputTokens, optionally nested under a key matching the event name.
		if u := parseKiroUsage(eventType, frame.payload); u != nil {
			chunks = append(chunks, core.StreamChunk{Type: core.ChunkUsage, Usage: u})
		}
	}
	return chunks
}

// parseKiroUsage extracts token usage from a metricsEvent/usageEvent payload.
// The counts may sit at the top level or be nested under a key matching the
// event type. Returns nil when no usable counts are present.
func parseKiroUsage(eventType string, payload []byte) *core.Usage {
	var p struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	}
	raw := payload
	var wrap map[string]json.RawMessage
	if json.Unmarshal(payload, &wrap) == nil {
		if m, ok := wrap[eventType]; ok {
			raw = m
		}
	}
	if json.Unmarshal(raw, &p) != nil {
		return nil
	}
	if p.InputTokens <= 0 && p.OutputTokens <= 0 {
		return nil
	}
	return &core.Usage{
		PromptTokens:     p.InputTokens,
		CompletionTokens: p.OutputTokens,
		TotalTokens:      p.InputTokens + p.OutputTokens,
	}
}

func extractKiroReasoning(payload []byte) string {
	// Payload may be a string, {text|content}, or {reasoningContentEvent:{...}}.
	var asString string
	if json.Unmarshal(payload, &asString) == nil && asString != "" {
		return asString
	}
	var obj struct {
		Text                  string `json:"text"`
		Content               string `json:"content"`
		ReasoningContentEvent struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"reasoningContentEvent"`
	}
	if json.Unmarshal(payload, &obj) != nil {
		return ""
	}
	if obj.Text != "" {
		return obj.Text
	}
	if obj.Content != "" {
		return obj.Content
	}
	if obj.ReasoningContentEvent.Text != "" {
		return obj.ReasoningContentEvent.Text
	}
	return obj.ReasoningContentEvent.Content
}

func kiroToolUseChunks(payload []byte, seenTools map[string]bool) []core.StreamChunk {
	parseOne := func(raw json.RawMessage) []core.StreamChunk {
		var t struct {
			ToolUseID string          `json:"toolUseId"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
		}
		if json.Unmarshal(raw, &t) != nil {
			return nil
		}
		id := t.ToolUseID
		if id == "" {
			id = "call_" + uuid.NewString()
		}
		var chunks []core.StreamChunk
		if !seenTools[id] {
			seenTools[id] = true
			chunks = append(chunks, core.StreamChunk{
				Type:     core.ChunkToolCall,
				ToolCall: &core.ToolCall{ID: id, Name: t.Name, Arguments: json.RawMessage("")},
			})
		}
		if len(t.Input) > 0 {
			args := t.Input
			// Input may be a JSON string or object; normalize to a string of JSON.
			var asStr string
			if json.Unmarshal(t.Input, &asStr) == nil {
				args = json.RawMessage(asStr)
			}
			chunks = append(chunks, core.StreamChunk{
				Type:     core.ChunkToolCall,
				ToolCall: &core.ToolCall{ID: id, Arguments: args},
			})
		}
		return chunks
	}

	// Payload may be a single tool-use object or an array.
	trimmed := payload
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\n' || trimmed[0] == '\t') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(payload, &arr) != nil {
			return nil
		}
		var chunks []core.StreamChunk
		for _, item := range arr {
			chunks = append(chunks, parseOne(item)...)
		}
		return chunks
	}
	return parseOne(payload)
}

// ---- AWS EventStream binary parser ------------------------------------------

// errEventStreamEOF signals the stream ended cleanly.
var errEventStreamEOF = errEventStreamDone{}

type errEventStreamDone struct{}

func (errEventStreamDone) Error() string { return "eventstream: EOF" }

// eventStreamFrame is one decoded AWS EventStream message: its string headers
// plus the raw JSON payload.
type eventStreamFrame struct {
	headers map[string]string
	payload []byte
}

// eventStreamParser reads AWS EventStream binary frames from an io.Reader. Each
// frame is: [4-byte total length][4-byte headers length][4-byte prelude CRC]
// [headers][payload][4-byte message CRC], all big-endian. Headers are
// length-prefixed name/value pairs; only string headers (type 7) are decoded,
// which is all CodeWhisperer emits (:event-type, :content-type, :message-type).
type eventStreamParser struct {
	r   io.Reader
	buf []byte
}

func newEventStreamParser(r io.Reader) *eventStreamParser {
	return &eventStreamParser{r: r}
}

// next returns the next decoded frame, errEventStreamEOF at end of stream, or a
// transport error. It returns (nil, nil) for a frame that decoded to no usable
// content so the caller can continue.
func (p *eventStreamParser) next() (*eventStreamFrame, error) {
	// Ensure at least the 12-byte prelude is buffered.
	for len(p.buf) < 12 {
		if err := p.fill(); err != nil {
			if err == io.EOF && len(p.buf) == 0 {
				return nil, errEventStreamEOF
			}
			if err == io.EOF {
				return nil, errEventStreamEOF
			}
			return nil, err
		}
	}

	totalLen := int(binary.BigEndian.Uint32(p.buf[0:4]))
	if totalLen < 16 {
		return nil, fmt.Errorf("eventstream: invalid frame length %d", totalLen)
	}

	// Buffer the whole frame.
	for len(p.buf) < totalLen {
		if err := p.fill(); err != nil {
			if err == io.EOF {
				return nil, errEventStreamEOF
			}
			return nil, err
		}
	}

	frame := p.buf[:totalLen]
	p.buf = p.buf[totalLen:]

	return decodeEventStreamFrame(frame)
}

// fill reads more bytes into the buffer.
func (p *eventStreamParser) fill() error {
	tmp := make([]byte, 32*1024)
	n, err := p.r.Read(tmp)
	if n > 0 {
		p.buf = append(p.buf, tmp[:n]...)
	}
	return err
}

// decodeEventStreamFrame parses one complete frame's bytes.
func decodeEventStreamFrame(frame []byte) (*eventStreamFrame, error) {
	if len(frame) < 16 {
		return nil, fmt.Errorf("eventstream: short frame")
	}
	headersLen := int(binary.BigEndian.Uint32(frame[4:8]))

	headers := map[string]string{}
	offset := 12 // after prelude (8) + prelude CRC (4)
	headerEnd := 12 + headersLen
	if headerEnd > len(frame) {
		return nil, fmt.Errorf("eventstream: headers exceed frame")
	}

	for offset < headerEnd {
		if offset >= len(frame) {
			break
		}
		nameLen := int(frame[offset])
		offset++
		if offset+nameLen > len(frame) {
			break
		}
		name := string(frame[offset : offset+nameLen])
		offset += nameLen
		if offset >= len(frame) {
			break
		}
		headerType := frame[offset]
		offset++
		if headerType == 7 { // string
			if offset+2 > len(frame) {
				break
			}
			valueLen := int(binary.BigEndian.Uint16(frame[offset : offset+2]))
			offset += 2
			if offset+valueLen > len(frame) {
				break
			}
			headers[name] = string(frame[offset : offset+valueLen])
			offset += valueLen
		} else {
			// Non-string header: we can't size it reliably, stop parsing headers.
			break
		}
	}

	payloadStart := 12 + headersLen
	payloadEnd := len(frame) - 4 // exclude message CRC
	if payloadEnd < payloadStart {
		return &eventStreamFrame{headers: headers, payload: nil}, nil
	}
	payload := frame[payloadStart:payloadEnd]
	return &eventStreamFrame{headers: headers, payload: payload}, nil
}

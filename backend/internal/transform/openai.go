package transform

import (
	json "github.com/ardhycrjr24/nexusrouter/backend/internal/fastjson"
	"fmt"
	"io"
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// OpenAICodec handles the OpenAI Chat Completions wire format, the de-facto
// standard most CLI tools speak.
type OpenAICodec struct{}

func (OpenAICodec) Dialect() core.Dialect { return core.DialectOpenAI }

// ---- wire types -------------------------------------------------------------

type oaiRequest struct {
	Model       string        `json:"model"`
	Messages    []oaiMessage  `json:"messages"`
	Tools       []oaiTool     `json:"tools,omitempty"`
	ToolChoice  any           `json:"tool_choice,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	StreamOpts  *oaiStreamOpt `json:"stream_options,omitempty"`
	ResponseFormat json.RawMessage `json:"response_format,omitempty"`
}

type oaiStreamOpt struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	Name       string        `json:"name,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	// ReasoningContent carries thinking/reasoning text that must be echoed
	// back on follow-up turns for DeepSeek, MiniMax, and similar providers.
	// Omitted when empty to avoid 400 errors on providers that don't support it.
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

// ---- request parsing --------------------------------------------------------

func (OpenAICodec) ParseRequest(body []byte) (*core.ChatRequest, error) {
	var raw oaiRequest
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openai: parse request: %w", err)
	}

	req := &core.ChatRequest{
		Model:       raw.Model,
		Temperature: raw.Temperature,
		TopP:        raw.TopP,
		MaxTokens:   raw.MaxTokens,
		Stop:        raw.Stop,
		Stream:      raw.Stream,
		ToolChoice:  raw.ToolChoice,
	}

	for _, t := range raw.Tools {
		if t.Type != "function" && t.Type != "" {
			continue
		}
		req.Tools = append(req.Tools, core.Tool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}

	for _, m := range raw.Messages {
		msg, isSystem, sysText := parseOAIMessage(m)
		if isSystem {
			// Hoist system content to the top-level field; concatenate multiples.
			if req.System != "" {
				req.System += "\n\n"
			}
			req.System += sysText
			continue
		}
		req.Messages = append(req.Messages, msg)
	}
	return req, nil
}

// parseOAIMessage converts one OpenAI message to canonical form. System and
// developer roles are reported separately so the caller can hoist them.
func parseOAIMessage(m oaiMessage) (msg core.Message, isSystem bool, sysText string) {
	role := m.Role
	// OpenAI's "developer" role is a system-equivalent for newer models.
	if role == "system" || role == "developer" {
		return core.Message{}, true, decodeOAIContentText(m.Content)
	}

	msg.Role = mapOAIRole(role)
	msg.Name = m.Name

	// Preserve reasoning_content as a thinking part so it gets echoed back
	// on follow-up turns (required by DeepSeek, MiniMax, etc.).
	if m.ReasoningContent != "" && role == "assistant" {
		msg.Content = append(msg.Content, core.ContentPart{
			Type: core.PartThinking,
			Text: m.ReasoningContent,
		})
	}

	// Assistant tool calls.
	for _, tc := range m.ToolCalls {
		msg.Content = append(msg.Content, core.ContentPart{
			Type: core.PartToolCall,
			ToolCall: &core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	// Tool result message.
	if role == "tool" {
		msg.Content = append(msg.Content, core.ContentPart{
			Type: core.PartToolResult,
			ToolResult: &core.ToolResult{
				CallID:  m.ToolCallID,
				Content: decodeOAIContentText(m.Content),
			},
		})
		return msg, false, ""
	}

	// Text / multimodal content parts.
	msg.Content = append(msg.Content, decodeOAIContentParts(m.Content)...)
	return msg, false, ""
}

func mapOAIRole(role string) core.Role {
	switch role {
	case "user":
		return core.RoleUser
	case "assistant":
		return core.RoleAssistant
	case "tool":
		return core.RoleTool
	default:
		return core.RoleUser
	}
}

// decodeOAIContentText extracts plain text from an OpenAI content field, which
// may be a string or an array of typed parts.
func decodeOAIContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var b strings.Builder
	for _, p := range decodeOAIContentParts(raw) {
		if p.Type == core.PartText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// decodeOAIContentParts extracts canonical content parts from an OpenAI content
// field (string or array of {type,text|image_url}).
func decodeOAIContentParts(raw json.RawMessage) []core.ContentPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []core.ContentPart{{Type: core.PartText, Text: s}}
	}

	var arr []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}

	var parts []core.ContentPart
	for _, p := range arr {
		switch p.Type {
		case "text":
			parts = append(parts, core.ContentPart{Type: core.PartText, Text: p.Text})
		case "image_url":
			media := parseOAIImageURL(p.ImageURL.URL)
			parts = append(parts, core.ContentPart{Type: core.PartImage, Media: media})
		}
	}
	return parts
}

// parseOAIImageURL decomposes an OpenAI image_url value into a MediaPayload.
// If the URL is a data URI (data:<mime>;base64,<data>), MIMEType and Data are
// populated; otherwise the URL is kept as-is for cross-dialect forwarding.
func parseOAIImageURL(rawURL string) *core.MediaPayload {
	if strings.HasPrefix(rawURL, "data:") {
		// Format: data:<mediatype>;base64,<data>
		rest, ok := strings.CutPrefix(rawURL, "data:")
		if ok {
			if idx := strings.Index(rest, ";base64,"); idx > 0 {
				return &core.MediaPayload{
					MIMEType: rest[:idx],
					Data:     rest[idx+8:],
				}
			}
		}
	}
	return &core.MediaPayload{URL: rawURL}
}

// mediaToDataURL converts a MediaPayload into a URL suitable for the OpenAI
// image_url format. If the media has inline base64 data, it is wrapped in a
// data URI. If it already has a URL, that URL is returned as-is.
func mediaToDataURL(m *core.MediaPayload) string {
	if m.Data != "" {
		mime := m.MIMEType
		if mime == "" {
			mime = "image/png"
		}
		return "data:" + mime + ";base64," + m.Data
	}
	return m.URL
}

// ---- request rendering ------------------------------------------------------

// needsJSONSchemaFallback checks if response_format uses json_schema and the
// provider doesn't support native structured output. Providers that support
// json_schema natively (like OpenAI, Azure) are excluded.
func needsJSONSchemaFallback(providerID string, responseFormat json.RawMessage) bool {
	if len(responseFormat) == 0 {
		return false
	}
	var rf struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(responseFormat, &rf); err != nil || rf.Type != "json_schema" {
		return false
	}
	// Providers that support native json_schema.
	switch providerID {
	case "openai", "azure":
		return false
	}
	return true
}

// fallbackResponseFormat converts json_schema to json_object for providers
// that don't support native structured output.
func fallbackResponseFormat(responseFormat json.RawMessage) json.RawMessage {
	if len(responseFormat) == 0 {
		return responseFormat
	}
	var rf struct {
		Type       string `json:"type"`
		JSONSchema any    `json:"json_schema"`
	}
	if err := json.Unmarshal(responseFormat, &rf); err != nil {
		return responseFormat
	}
	if rf.Type != "json_schema" {
		return responseFormat
	}
	// Fall back to plain json_object mode.
	out, _ := json.Marshal(map[string]string{"type": "json_object"})
	return out
}

// RenderRequestForProvider renders an OpenAI request for a specific provider,
// applying provider-specific fallbacks (e.g. json_schema → json_object).
func (c OpenAICodec) RenderRequestForProvider(req *core.ChatRequest, providerID string) ([]byte, error) {
	if needsJSONSchemaFallback(providerID, req.ResponseFormat) {
		clone := *req
		clone.ResponseFormat = fallbackResponseFormat(req.ResponseFormat)
		return c.RenderRequest(&clone)
	}
	return c.RenderRequest(req)
}

func (OpenAICodec) RenderRequest(req *core.ChatRequest) ([]byte, error) {
	out := oaiRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
		Stream:      req.Stream,
		ToolChoice:  req.ToolChoice,
		ResponseFormat: req.ResponseFormat,
	}
	// Note: stream_options with include_usage is intentionally omitted. Many
	// OpenAI-compatible providers (MiMo, Volcengine, etc.) reject this field
	// with 400. Usage is captured from the final stream chunk instead.

	if req.System != "" {
		sysContent, _ := json.Marshal(req.System)
		out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: sysContent})
	}

	for _, m := range req.Messages {
		out.Messages = append(out.Messages, renderOAIMessage(m))
	}

	for _, t := range req.Tools {
		var tool oaiTool
		tool.Type = "function"
		tool.Function.Name = t.Name
		tool.Function.Description = t.Description
		tool.Function.Parameters = t.Parameters
		out.Tools = append(out.Tools, tool)
	}

	return json.Marshal(out)
}

func renderOAIMessage(m core.Message) oaiMessage {
	out := oaiMessage{Role: string(m.Role), Name: m.Name}

	var textParts []string
	var thinkingParts []string
	var hasMedia bool
	var contentParts []map[string]any

	for _, p := range m.Content {
		switch p.Type {
		case core.PartThinking:
			// Collect thinking/reasoning content for echoing back as
			// reasoning_content on DeepSeek, MiniMax, and similar providers.
			thinkingParts = append(thinkingParts, p.Text)
		case core.PartText:
			textParts = append(textParts, p.Text)
		case core.PartImage:
			hasMedia = true
			if p.Media != nil {
				url := mediaToDataURL(p.Media)
				contentParts = append(contentParts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": url},
				})
			}
		case core.PartToolCall:
			var tc oaiToolCall
			tc.ID = p.ToolCall.ID
			tc.Type = "function"
			tc.Function.Name = p.ToolCall.Name
			tc.Function.Arguments = string(p.ToolCall.Arguments)
			out.ToolCalls = append(out.ToolCalls, tc)
		case core.PartToolResult:
			out.Role = "tool"
			out.ToolCallID = p.ToolResult.CallID
			content, _ := json.Marshal(p.ToolResult.Content)
			out.Content = content
			return out
		}
	}

	// Echo reasoning_content on assistant messages. DeepSeek, MiniMax, and
	// similar providers require this field on follow-up/tool-call turns to
	// avoid 400 errors ("reasoning_content must be passed back").
	if m.Role == core.RoleAssistant && len(thinkingParts) > 0 {
		out.ReasoningContent = strings.Join(thinkingParts, "")
	}

	// When images are present, use the array-of-parts content format (OpenAI
	// requires this for multimodal messages). Text-only messages use a plain
	// string for backward compatibility.
	if hasMedia {
		// Prepend any accumulated text as a text part.
		if len(textParts) > 0 {
			contentParts = append([]map[string]any{{
				"type": "text",
				"text": strings.Join(textParts, ""),
			}}, contentParts...)
		}
		b, _ := json.Marshal(contentParts)
		out.Content = b
	} else if len(textParts) > 0 {
		content, _ := json.Marshal(strings.Join(textParts, ""))
		out.Content = content
	}
	return out
}

// ---- response parsing -------------------------------------------------------

type oaiResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string        `json:"role"`
			Content   string        `json:"content"`
			// ReasoningContent carries thinking/reasoning text from models
			// that expose it as a structured field (DeepSeek, some MiMo).
			ReasoningContent string        `json:"reasoning_content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *oaiUsage `json:"usage"`
}

type oaiUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	PromptTokensDetails     *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
}

func (c OpenAICodec) ParseResponse(body []byte, model string) (*core.ChatResponse, error) {
	var raw oaiResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	return c.buildResponse(raw, model)
}

// ParseResponseFrom implements StreamingResponseCodec. It decodes directly from
// an io.Reader, avoiding the intermediate []byte allocation.
func (c OpenAICodec) ParseResponseFrom(r io.Reader, model string) (*core.ChatResponse, error) {
	var raw oaiResponse
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}
	return c.buildResponse(raw, model)
}

// buildResponse converts a parsed oaiResponse into a canonical ChatResponse.
func (OpenAICodec) buildResponse(raw oaiResponse, model string) (*core.ChatResponse, error) {
	if len(raw.Choices) == 0 {
		return nil, fmt.Errorf("openai: response has no choices")
	}

	choice := raw.Choices[0]
	msg := core.Message{Role: core.RoleAssistant}

	// Extract thinking content: prefer structured reasoning_content field,
	// fall back to <think> tag extraction from content.
	thinkingText := choice.Message.ReasoningContent
	contentText := choice.Message.Content
	if thinkingText == "" && contentText != "" {
		thinkingChunks, clean := StripThinkTags(contentText)
		if len(thinkingChunks) > 0 {
			// Store thinking as the first content part.
			for _, tc := range thinkingChunks {
				msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: tc.Delta})
			}
			contentText = clean
		}
	} else if thinkingText != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartThinking, Text: thinkingText})
	}

	if contentText != "" {
		msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: contentText})
	}
	for _, tc := range choice.Message.ToolCalls {
		msg.Content = append(msg.Content, core.ContentPart{
			Type: core.PartToolCall,
			ToolCall: &core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	resp := &core.ChatResponse{
		ID:           raw.ID,
		Model:        firstNonEmpty(raw.Model, model),
		Message:      msg,
		FinishReason: mapOAIFinish(choice.FinishReason),
	}
	if raw.Usage != nil {
		var cached int
		if raw.Usage.PromptTokensDetails != nil {
			cached = raw.Usage.PromptTokensDetails.CachedTokens
		}
		resp.Usage = core.Usage{
			PromptTokens:     raw.Usage.PromptTokens,
			CompletionTokens: raw.Usage.CompletionTokens,
			TotalTokens:      raw.Usage.TotalTokens,
			CachedTokens:     cached,
		}
	}
	return resp, nil
}

func (OpenAICodec) RenderResponse(resp *core.ChatResponse) ([]byte, error) {
	out := map[string]any{
		"id":      firstNonEmpty(resp.ID, "chatcmpl-"+resp.Model),
		"object":  "chat.completion",
		"model":   resp.Model,
		"choices": []map[string]any{renderOAIChoice(resp)},
		"usage": map[string]int{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		},
	}
	return json.Marshal(out)
}

func renderOAIChoice(resp *core.ChatResponse) map[string]any {
	message := map[string]any{"role": "assistant"}
	var text strings.Builder
	var toolCalls []map[string]any
	for _, p := range resp.Message.Content {
		switch p.Type {
		case core.PartText:
			text.WriteString(p.Text)
		case core.PartToolCall:
			toolCalls = append(toolCalls, map[string]any{
				"id":   p.ToolCall.ID,
				"type": "function",
				"function": map[string]string{
					"name":      p.ToolCall.Name,
					"arguments": string(p.ToolCall.Arguments),
				},
			})
		}
	}
	if text.Len() > 0 {
		message["content"] = text.String()
	} else {
		message["content"] = nil
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	return map[string]any{
		"index":         0,
		"message":       message,
		"finish_reason": string(resp.FinishReason),
	}
}

func mapOAIFinish(r string) core.FinishReason {
	switch r {
	case "stop":
		return core.FinishStop
	case "length":
		return core.FinishLength
	case "tool_calls", "function_call":
		return core.FinishToolCalls
	case "content_filter":
		return core.FinishFilter
	default:
		return core.FinishStop
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

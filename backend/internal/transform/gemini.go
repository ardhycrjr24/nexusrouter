package transform

import (
	json "github.com/ardhycrjr24/nexusrouter/backend/internal/fastjson"
	"fmt"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// GeminiCodec handles Google's Gemini generateContent wire format. Gemini
// groups turns under "contents" with role "user"/"model", carries system text
// in a separate "systemInstruction", and nests tool calls as functionCall /
// functionResponse parts.
type GeminiCodec struct{}

func (GeminiCodec) Dialect() core.Dialect { return core.DialectGemini }

// ---- wire types -------------------------------------------------------------

type gemRequest struct {
	Contents          []gemContent    `json:"contents"`
	SystemInstruction *gemContent     `json:"systemInstruction,omitempty"`
	Tools             []gemTool       `json:"tools,omitempty"`
	GenerationConfig  *gemGenConfig   `json:"generationConfig,omitempty"`
}

type gemGenConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
}

type gemContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []gemPart `json:"parts"`
}

type gemPart struct {
	Text             string             `json:"text,omitempty"`
	FunctionCall     *gemFunctionCall   `json:"functionCall,omitempty"`
	FunctionResponse *gemFunctionResult `json:"functionResponse,omitempty"`
	InlineData       *gemInlineData     `json:"inlineData,omitempty"`
}

type gemInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type gemFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type gemFunctionResult struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type gemTool struct {
	FunctionDeclarations []gemFuncDecl `json:"functionDeclarations"`
}

type gemFuncDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ---- request parsing --------------------------------------------------------

func (GeminiCodec) ParseRequest(body []byte) (*core.ChatRequest, error) {
	var raw gemRequest
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("gemini: parse request: %w", err)
	}

	req := &core.ChatRequest{}
	if raw.SystemInstruction != nil {
		for _, p := range raw.SystemInstruction.Parts {
			req.System += p.Text
		}
	}
	if raw.GenerationConfig != nil {
		req.Temperature = raw.GenerationConfig.Temperature
		req.TopP = raw.GenerationConfig.TopP
		req.MaxTokens = raw.GenerationConfig.MaxOutputTokens
		req.Stop = raw.GenerationConfig.StopSequences
	}
	for _, t := range raw.Tools {
		for _, fd := range t.FunctionDeclarations {
			req.Tools = append(req.Tools, core.Tool{Name: fd.Name, Description: fd.Description, Parameters: fd.Parameters})
		}
	}
	for _, c := range raw.Contents {
		req.Messages = append(req.Messages, parseGemContent(c))
	}
	return req, nil
}

func parseGemContent(c gemContent) core.Message {
	msg := core.Message{Role: mapGemRole(c.Role)}
	for _, p := range c.Parts {
		switch {
		case p.FunctionCall != nil:
			msg.Content = append(msg.Content, core.ContentPart{
				Type:     core.PartToolCall,
				ToolCall: &core.ToolCall{Name: p.FunctionCall.Name, Arguments: p.FunctionCall.Args},
			})
		case p.FunctionResponse != nil:
			msg.Content = append(msg.Content, core.ContentPart{
				Type: core.PartToolResult,
				ToolResult: &core.ToolResult{
					CallID:  p.FunctionResponse.Name,
					Content: string(p.FunctionResponse.Response),
				},
			})
		case p.InlineData != nil:
			msg.Content = append(msg.Content, core.ContentPart{
				Type:  core.PartImage,
				Media: &core.MediaPayload{MIMEType: p.InlineData.MIMEType, Data: p.InlineData.Data},
			})
		case p.Text != "":
			msg.Content = append(msg.Content, core.ContentPart{Type: core.PartText, Text: p.Text})
		}
	}
	return msg
}

func mapGemRole(role string) core.Role {
	if role == "model" {
		return core.RoleAssistant
	}
	return core.RoleUser
}

// ---- request rendering ------------------------------------------------------

func (GeminiCodec) RenderRequest(req *core.ChatRequest) ([]byte, error) {
	out := gemRequest{}
	if req.System != "" {
		out.SystemInstruction = &gemContent{Parts: []gemPart{{Text: req.System}}}
	}
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil || len(req.Stop) > 0 {
		out.GenerationConfig = &gemGenConfig{
			Temperature:     req.Temperature,
			TopP:            req.TopP,
			MaxOutputTokens: req.MaxTokens,
			StopSequences:   req.Stop,
		}
	}
	if len(req.Tools) > 0 {
		var decls []gemFuncDecl
		for _, t := range req.Tools {
			decls = append(decls, gemFuncDecl{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
		}
		out.Tools = []gemTool{{FunctionDeclarations: decls}}
	}
	for _, m := range req.Messages {
		out.Contents = append(out.Contents, renderGemContent(m))
	}
	return json.Marshal(out)
}

func renderGemContent(m core.Message) gemContent {
	role := "user"
	if m.Role == core.RoleAssistant {
		role = "model"
	}
	c := gemContent{Role: role}
	for _, p := range m.Content {
		switch p.Type {
		case core.PartText:
			c.Parts = append(c.Parts, gemPart{Text: p.Text})
		case core.PartToolCall:
			c.Parts = append(c.Parts, gemPart{FunctionCall: &gemFunctionCall{Name: p.ToolCall.Name, Args: p.ToolCall.Arguments}})
		case core.PartToolResult:
			c.Parts = append(c.Parts, gemPart{FunctionResponse: &gemFunctionResult{
				Name:     p.ToolResult.CallID,
				Response: json.RawMessage(quoteIfNotJSON(p.ToolResult.Content)),
			}})
		case core.PartImage:
			if p.Media != nil {
				c.Parts = append(c.Parts, gemPart{InlineData: &gemInlineData{MIMEType: p.Media.MIMEType, Data: p.Media.Data}})
			}
		}
	}
	if len(c.Parts) == 0 {
		c.Parts = append(c.Parts, gemPart{Text: ""})
	}
	return c
}

// quoteIfNotJSON wraps a tool-result string as a JSON value if it isn't already
// valid JSON, since Gemini's functionResponse.response expects a JSON object.
func quoteIfNotJSON(s string) string {
	var probe any
	if json.Unmarshal([]byte(s), &probe) == nil {
		return s
	}
	b, _ := json.Marshal(map[string]string{"result": s})
	return string(b)
}

// ---- response parsing -------------------------------------------------------

type gemResponse struct {
	Candidates []struct {
		Content      gemContent `json:"content"`
		FinishReason string     `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
}

func (GeminiCodec) ParseResponse(body []byte, model string) (*core.ChatResponse, error) {
	var raw gemResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}
	if len(raw.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: response has no candidates")
	}
	cand := raw.Candidates[0]
	msg := parseGemContent(cand.Content)
	msg.Role = core.RoleAssistant

	return &core.ChatResponse{
		Model:        model,
		Message:      msg,
		FinishReason: mapGemFinish(cand.FinishReason),
		Usage: core.Usage{
			PromptTokens:     raw.UsageMetadata.PromptTokenCount,
			CompletionTokens: raw.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      raw.UsageMetadata.TotalTokenCount,
			CachedTokens:     raw.UsageMetadata.CachedContentTokenCount,
		},
	}, nil
}

func (GeminiCodec) RenderResponse(resp *core.ChatResponse) ([]byte, error) {
	content := renderGemContent(resp.Message)
	content.Role = "model"
	out := map[string]any{
		"candidates": []map[string]any{{
			"content":      content,
			"finishReason": renderGemFinish(resp.FinishReason),
			"index":        0,
		}},
		"usageMetadata": map[string]int{
			"promptTokenCount":     resp.Usage.PromptTokens,
			"candidatesTokenCount": resp.Usage.CompletionTokens,
			"totalTokenCount":      resp.Usage.TotalTokens,
		},
	}
	return json.Marshal(out)
}

func mapGemFinish(r string) core.FinishReason {
	switch r {
	case "STOP":
		return core.FinishStop
	case "MAX_TOKENS":
		return core.FinishLength
	case "SAFETY", "RECITATION":
		return core.FinishFilter
	default:
		return core.FinishStop
	}
}

func renderGemFinish(r core.FinishReason) string {
	switch r {
	case core.FinishLength:
		return "MAX_TOKENS"
	case core.FinishFilter:
		return "SAFETY"
	default:
		return "STOP"
	}
}
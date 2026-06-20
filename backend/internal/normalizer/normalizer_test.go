package normalizer

import (
	"encoding/json"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestSanitizeToolCallIDs_ValidPassthrough(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "call_msg0_tc0_Read",
						Name:      "Read",
						Arguments: json.RawMessage(`{"file_path":"/tmp/a.txt"}`),
					}},
				},
			},
			{
				Role: core.RoleTool,
				Content: []core.ContentPart{
					{Type: core.PartToolResult, ToolResult: &core.ToolResult{
						CallID:  "call_msg0_tc0_Read",
						Content: "file content",
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	if req.Messages[0].Content[0].ToolCall.ID != "call_msg0_tc0_Read" {
		t.Errorf("valid ID was modified: %s", req.Messages[0].Content[0].ToolCall.ID)
	}
}

func TestSanitizeToolCallIDs_InvalidCharsStripped(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "call.msg@0#tc$0",
						Name:      "Read",
						Arguments: json.RawMessage(`{}`),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := req.Messages[0].Content[0].ToolCall.ID
	if got != "callmsg0tc0" {
		t.Errorf("expected 'callmsg0tc0', got %q", got)
	}
}

func TestSanitizeToolCallIDs_EmptyIDGenerated(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "",
						Name:      "WebSearch",
						Arguments: json.RawMessage(`{}`),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := req.Messages[0].Content[0].ToolCall.ID
	if got == "" {
		t.Error("expected generated ID, got empty")
	}
	if !toolIDPattern.MatchString(got) {
		t.Errorf("generated ID %q doesn't match pattern", got)
	}
}

func TestSanitizeToolCallIDs_EmptyArgsNormalized(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "tc1",
						Name:      "Read",
						Arguments: json.RawMessage(""),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := string(req.Messages[0].Content[0].ToolCall.Arguments)
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestSanitizeToolCallIDs_MalformedArgsNormalized(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "tc1",
						Name:      "Read",
						Arguments: json.RawMessage("{invalid json"),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := string(req.Messages[0].Content[0].ToolCall.Arguments)
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestFixMissingToolResults_AlreadyPresent(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tc1", Content: "ok"}},
				},
			},
		},
	}
	FixMissingToolResults(req)
	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}
}

func TestFixMissingToolResults_InsertsEmptyResults(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc2", Name: "Write"}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartText, Text: "Continue"},
				},
			},
		},
	}
	FixMissingToolResults(req)

	// The next message should now have tool_result parts.
	next := req.Messages[1]
	resultIDs := collectToolResultIDs(next)
	if len(resultIDs) != 2 {
		t.Errorf("expected 2 tool results, got %d: %v", len(resultIDs), resultIDs)
	}
}

func TestFixMissingToolResults_NoNextMessage(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
				},
			},
		},
	}
	FixMissingToolResults(req)

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages (synthetic tool result added), got %d", len(req.Messages))
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != core.RoleTool {
		t.Errorf("expected role 'tool', got %q", last.Role)
	}
	if len(last.Content) != 1 || last.Content[0].ToolResult == nil {
		t.Error("expected tool_result part in synthetic message")
	}
}

func TestApply_Idempotent(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "bad@id!",
						Name:      "Read",
						Arguments: json.RawMessage(""),
					}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartText, Text: "Continue"},
				},
			},
		},
	}

	Apply(req)
	msgs1 := len(req.Messages)
	id1 := req.Messages[0].Content[0].ToolCall.ID

	Apply(req)
	msgs2 := len(req.Messages)
	id2 := req.Messages[0].Content[0].ToolCall.ID

	if msgs1 != msgs2 {
		t.Errorf("message count changed: %d vs %d", msgs1, msgs2)
	}
	if id1 != id2 {
		t.Errorf("ID changed between runs: %q vs %q", id1, id2)
	}
}

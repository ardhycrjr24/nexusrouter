// Package normalizer validates and repairs tool call structures in chat
// requests before they are sent upstream. It ensures tool call IDs are
// Anthropic-compatible, arguments are valid JSON, and every tool_use has a
// matching tool_result.
//
// This mirrors 9router's ensureToolCallIds + fixMissingToolResponses logic.
package normalizer

import (
	json "github.com/ardhycrjr24/nexusrouter/backend/internal/fastjson"
	"fmt"
	"regexp"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// toolIDPattern matches the Anthropic tool_use.id character set.
var toolIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Apply runs all normalizers on the request in place. It is safe to call
// multiple times (idempotent).
func Apply(req *core.ChatRequest) {
	if req == nil {
		return
	}
	SanitizeToolCallIDs(req)
	FixMissingToolResults(req)
}

// SanitizeToolCallIDs walks every message and repairs tool call/result IDs
// that don't match the Anthropic-required pattern [a-zA-Z0-9_-]+.
// Invalid characters are stripped; if nothing remains, a deterministic ID
// is generated. Arguments are also normalized to valid JSON.
func SanitizeToolCallIDs(req *core.ChatRequest) {
	for mi := range req.Messages {
		for pi := range req.Messages[mi].Content {
			p := &req.Messages[mi].Content[pi]

			switch p.Type {
			case core.PartToolCall:
				if p.ToolCall == nil {
					continue
				}
				p.ToolCall.ID = sanitizeID(p.ToolCall.ID, mi, pi, p.ToolCall.Name)
				normalizeArguments(p.ToolCall)

			case core.PartToolResult:
				if p.ToolResult == nil {
					continue
				}
				p.ToolResult.CallID = sanitizeID(p.ToolResult.CallID, mi, pi, "")
			}
		}
	}
}

// FixMissingToolResults ensures every tool_use in an assistant message has a
// corresponding tool_result in the immediately following message. If not, it
// inserts empty tool_result parts.
func FixMissingToolResults(req *core.ChatRequest) {
	var newMessages []core.Message

	for i, msg := range req.Messages {
		newMessages = append(newMessages, msg)

		// Collect tool call IDs from this message.
		toolCallIDs := collectToolCallIDs(msg)
		if len(toolCallIDs) == 0 {
			continue
		}

		// Check if the next message has matching tool results.
		if i+1 < len(req.Messages) {
			resultIDs := collectToolResultIDs(req.Messages[i+1])
			missing := difference(toolCallIDs, resultIDs)
			if len(missing) == 0 {
				continue
			}

			// Insert empty tool results into the next message.
			for _, id := range missing {
				req.Messages[i+1].Content = append(req.Messages[i+1].Content, core.ContentPart{
					Type: core.PartToolResult,
					ToolResult: &core.ToolResult{
						CallID:  id,
						Content: "",
					},
				})
			}
		} else {
			// No next message — create a synthetic tool-result message.
			var parts []core.ContentPart
			for _, id := range toolCallIDs {
				parts = append(parts, core.ContentPart{
					Type: core.PartToolResult,
					ToolResult: &core.ToolResult{
						CallID:  id,
						Content: "",
					},
				})
			}
			newMessages = append(newMessages, core.Message{
				Role:    core.RoleTool,
				Content: parts,
			})
		}
	}

	req.Messages = newMessages
}

// --- helpers ---

// sanitizeID strips characters not matching [a-zA-Z0-9_-]. If the result is
// empty, a deterministic ID is generated.
func sanitizeID(id string, msgIdx, partIdx int, toolName string) string {
	if id == "" {
		return generateID(msgIdx, partIdx, toolName)
	}
	cleaned := cleanID(id)
	if cleaned == "" {
		return generateID(msgIdx, partIdx, toolName)
	}
	return cleaned
}

// cleanID removes characters outside [a-zA-Z0-9_-].
func cleanID(id string) string {
	buf := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			buf = append(buf, c)
		}
	}
	return string(buf)
}

// generateID creates a deterministic tool call ID from position + name.
func generateID(msgIdx, partIdx int, toolName string) string {
	if toolName != "" {
		return fmt.Sprintf("call_msg%d_tc%d_%s", msgIdx, partIdx, cleanID(toolName))
	}
	return fmt.Sprintf("call_msg%d_tc%d", msgIdx, partIdx)
}

// normalizeArguments ensures ToolCall.Arguments is valid JSON.
func normalizeArguments(tc *core.ToolCall) {
	if len(tc.Arguments) == 0 {
		tc.Arguments = json.RawMessage("{}")
		return
	}
	// Verify it's valid JSON.
	var raw any
	if err := json.Unmarshal(tc.Arguments, &raw); err != nil {
		tc.Arguments = json.RawMessage("{}")
	}
}

// collectToolCallIDs returns all tool call IDs from a message's content.
func collectToolCallIDs(msg core.Message) []string {
	var ids []string
	for _, p := range msg.Content {
		if p.Type == core.PartToolCall && p.ToolCall != nil && p.ToolCall.ID != "" {
			ids = append(ids, p.ToolCall.ID)
		}
	}
	return ids
}

// collectToolResultIDs returns all tool result call IDs from a message's content.
func collectToolResultIDs(msg core.Message) []string {
	var ids []string
	for _, p := range msg.Content {
		if p.Type == core.PartToolResult && p.ToolResult != nil && p.ToolResult.CallID != "" {
			ids = append(ids, p.ToolResult.CallID)
		}
	}
	return ids
}

// difference returns elements in a that are not in b.
func difference(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	var result []string
	for _, v := range a {
		if !set[v] {
			result = append(result, v)
		}
	}
	return result
}

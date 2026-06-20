package transform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestKiro_RenderRequest_ConversationState(t *testing.T) {
	req := &core.ChatRequest{
		Model:  "claude-sonnet-4.5",
		System: "be precise",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs, ok := env["conversationState"].(map[string]any)
	if !ok {
		t.Fatal("missing conversationState")
	}
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	content := cm["content"].(string)
	// System folds into the user content; a context marker is prepended.
	if !strings.Contains(content, "be precise") {
		t.Errorf("system should fold into content: %q", content)
	}
	if !strings.Contains(content, "[Context: Current time") {
		t.Errorf("context marker missing: %q", content)
	}
	if cm["modelId"] != "claude-sonnet-4.5" {
		t.Errorf("modelId wrong: %v", cm["modelId"])
	}
}

func TestKiro_RenderRequest_ThinkingSuffix(t *testing.T) {
	// The synthetic -thinking suffix injects the thinking_mode prefix and is
	// stripped from the upstream modelId.
	req := &core.ChatRequest{
		Model: "claude-sonnet-4.5-thinking",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cm := env["conversationState"].(map[string]any)["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	content := cm["content"].(string)
	if !strings.Contains(content, "<thinking_mode>enabled</thinking_mode>") {
		t.Errorf("thinking prefix missing: %q", content)
	}
	if cm["modelId"] != "claude-sonnet-4.5" {
		t.Errorf("upstream modelId should strip -thinking, got %v", cm["modelId"])
	}
}

func TestKiro_RenderRequest_ToolsAndHistory(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "first"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: "ok"}}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "second"}}},
		},
		Tools: []core.Tool{{Name: "get_weather", Description: "weather", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)

	// History holds the earlier user + assistant turns; the last user message is
	// promoted to currentMessage.
	history := cs["history"].([]any)
	if len(history) < 2 {
		t.Fatalf("expected history with prior turns, got %d", len(history))
	}

	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	if !strings.Contains(cm["content"].(string), "second") {
		t.Errorf("current message should be the last user turn: %v", cm["content"])
	}
	// Tools attach to the current message context.
	ctx, ok := cm["userInputMessageContext"].(map[string]any)
	if !ok || ctx["tools"] == nil {
		t.Errorf("tools should attach to current message context: %v", cm)
	}
}

// When the client sends NO tools but the history references tool calls/results,
// the structured tool content must be flattened to text. Leaving structured
// tool references without a tools array makes Kiro return 400 "Improperly
// formed request".
func TestKiro_RenderRequest_FlattensToolsWhenClientSentNone(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-opus-4.8-thinking",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "list files"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "calling tool"},
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "call_1", Name: "ls", Arguments: json.RawMessage(`{"path":"."}`)}},
			}},
			{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "call_1", Content: "a.go\nb.go"}},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "thanks"}}},
		},
		// No Tools sent by the client.
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	// No structured tool content (toolUses/toolResults/tools) may survive
	// anywhere in the payload.
	raw := string(body)
	for _, banned := range []string{`"toolUses"`, `"toolResults"`, `"toolSpecification"`, `"tools"`} {
		if strings.Contains(raw, banned) {
			t.Errorf("flattened payload must not contain %s: %s", banned, raw)
		}
	}

	// The tool call/result content should survive as readable text in history.
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)
	var allText strings.Builder
	for _, h := range cs["history"].([]any) {
		hm := h.(map[string]any)
		if uim, ok := hm["userInputMessage"].(map[string]any); ok {
			allText.WriteString(uim["content"].(string))
			allText.WriteString("\n")
		}
		if arm, ok := hm["assistantResponseMessage"].(map[string]any); ok {
			allText.WriteString(arm["content"].(string))
			allText.WriteString("\n")
		}
	}
	// The last user turn is promoted to currentMessage, so the flattened tool
	// result (folded into the final user turn) lives there.
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	allText.WriteString(cm["content"].(string))
	text := allText.String()
	if !strings.Contains(text, "[Tool call: ls(") {
		t.Errorf("tool call should be flattened to text: %q", text)
	}
	if !strings.Contains(text, "[Tool result: a.go") {
		t.Errorf("tool result should be flattened to text: %q", text)
	}
}

// When the client DOES send tools but a tool_result references a tool_use that
// was dropped by client-side compaction, the orphaned result must be folded
// back into user text instead of left as a dangling structured reference
// (which makes Kiro return 400).
func TestKiro_RenderRequest_ReconcilesOrphanedToolResults(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-opus-4.8-thinking",
		Messages: []core.Message{
			// Assistant message that WOULD have contained the matching tool_use
			// has been compacted away — only its text remains.
			{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: "earlier reply"}}},
			// Orphaned tool result: no assistant toolUse has id "orphan_1".
			{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "orphan_1", Content: "stale output"}},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "continue please"}}},
		},
		Tools: []core.Tool{{Name: "noop", Description: "noop", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)

	// The orphaned toolResult must not survive as a structured reference.
	walk := func(uim map[string]any) {
		if ctx, ok := uim["userInputMessageContext"].(map[string]any); ok {
			if trs, ok := ctx["toolResults"].([]any); ok {
				for _, tr := range trs {
					if id, _ := tr.(map[string]any)["toolUseId"].(string); id == "orphan_1" {
						t.Errorf("orphaned toolResult should be removed, found: %v", tr)
					}
				}
			}
		}
	}
	collected := strings.Builder{}
	for _, h := range cs["history"].([]any) {
		if uim, ok := h.(map[string]any)["userInputMessage"].(map[string]any); ok {
			walk(uim)
			collected.WriteString(uim["content"].(string))
			collected.WriteString("\n")
		}
	}
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	walk(cm)
	collected.WriteString(cm["content"].(string))

	// The salvaged content must survive as text somewhere.
	if !strings.Contains(collected.String(), "stale output") {
		t.Errorf("orphaned tool result content should be salvaged as text: %q", collected.String())
	}
}

// Claude Code (Anthropic dialect) carries tool results as tool_result blocks
// inside a user message, so after parsing they arrive as a RoleUser message
// holding a PartToolResult — not as a RoleTool message. buildKiroHistory must
// still surface those into Kiro toolResults; otherwise the result is dropped,
// the assistant's matching toolUse is orphaned, and CodeWhisperer rejects the
// conversation with "Improperly formed request" (HTTP 400). This reproduces the
// exact shape that 400s only on Claude Code and not on OpenAI-dialect tools.
func TestKiro_RenderRequest_UserEmbeddedToolResult(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-opus-4.8-thinking",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "list files"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "calling tool"},
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "call_1", Name: "ls", Arguments: json.RawMessage(`{"path":"."}`)}},
			}},
			// Anthropic shape: tool_result lives inside a USER message, not a tool role.
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "call_1", Content: "a.go\nb.go"}},
			}},
		},
		Tools: []core.Tool{{Name: "ls", Description: "list", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)

	// Collect every toolResult across history + currentMessage.
	var resultIDs []string
	collect := func(uim map[string]any) {
		ctx, ok := uim["userInputMessageContext"].(map[string]any)
		if !ok {
			return
		}
		trs, ok := ctx["toolResults"].([]any)
		if !ok {
			return
		}
		for _, tr := range trs {
			if id, _ := tr.(map[string]any)["toolUseId"].(string); id != "" {
				resultIDs = append(resultIDs, id)
			}
		}
	}
	for _, h := range cs["history"].([]any) {
		if uim, ok := h.(map[string]any)["userInputMessage"].(map[string]any); ok {
			collect(uim)
		}
	}
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	collect(cm)

	// The tool result must survive as a structured toolResult referencing the
	// assistant's toolUse — proving it was not dropped.
	found := false
	for _, id := range resultIDs {
		if id == "call_1" {
			found = true
		}
	}
	if !found {
		t.Errorf("tool_result inside user message must surface as a Kiro toolResult for call_1; got %v\npayload: %s", resultIDs, body)
	}

	// The assistant toolUse must not be orphaned: its id must have a matching
	// result somewhere in the payload.
	var toolUseIDs []string
	for _, h := range cs["history"].([]any) {
		if arm, ok := h.(map[string]any)["assistantResponseMessage"].(map[string]any); ok {
			for _, tu := range asAnySlice(arm["toolUses"]) {
				if id, _ := tu.(map[string]any)["toolUseId"].(string); id != "" {
					toolUseIDs = append(toolUseIDs, id)
				}
			}
		}
	}
	for _, tuID := range toolUseIDs {
		matched := false
		for _, rID := range resultIDs {
			if rID == tuID {
				matched = true
			}
		}
		if !matched {
			t.Errorf("assistant toolUse %q has no matching toolResult — orphaned, will 400", tuID)
		}
	}
}

// CodeWhisperer rejects tool names that don't match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$
// with 400 "Improperly formed request". MCP tool names (dots, hyphens, long)
// must be coerced into the accepted shape, consistently across the tool spec
// and the tool_use reference in assistant history.
func TestKiro_RenderRequest_SanitizesToolNames(t *testing.T) {
	longName := strings.Repeat("a", 80)
	req := &core.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "go"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "c1", Name: "mcp__server__do-thing", Arguments: json.RawMessage(`{"x":1}`)}},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "again"}}},
		},
		Tools: []core.Tool{
			{Name: "mcp__server__do-thing", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: longName, Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)},
			{Name: "123start", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)

	// Collect every tool name from the tool spec and from history toolUses.
	var names []string
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	if ctx, ok := cm["userInputMessageContext"].(map[string]any); ok {
		for _, tt := range ctx["tools"].([]any) {
			spec := tt.(map[string]any)["toolSpecification"].(map[string]any)
			names = append(names, spec["name"].(string))
		}
	}
	for _, h := range cs["history"].([]any) {
		if arm, ok := h.(map[string]any)["assistantResponseMessage"].(map[string]any); ok {
			if tus, ok := arm["toolUses"].([]any); ok {
				for _, tu := range tus {
					names = append(names, tu.(map[string]any)["name"].(string))
				}
			}
		}
	}

	if len(names) == 0 {
		t.Fatal("expected sanitized tool names in payload")
	}
	for _, n := range names {
		if !isValidKiroToolName(n) {
			t.Errorf("tool name %q is not a valid CodeWhisperer name", n)
		}
	}

	// The MCP-style name must appear identically in both the spec and the
	// toolUse reference, so the call still resolves.
	sanitized := sanitizeKiroToolName("mcp__server__do-thing")
	specHas, useHas := false, false
	for _, tt := range cm["userInputMessageContext"].(map[string]any)["tools"].([]any) {
		if tt.(map[string]any)["toolSpecification"].(map[string]any)["name"] == sanitized {
			specHas = true
		}
	}
	for _, h := range cs["history"].([]any) {
		if arm, ok := h.(map[string]any)["assistantResponseMessage"].(map[string]any); ok {
			for _, tu := range arm["toolUses"].([]any) {
				if tu.(map[string]any)["name"] == sanitized {
					useHas = true
				}
			}
		}
	}
	if !specHas || !useHas {
		t.Errorf("sanitized MCP name %q must match in spec (%v) and toolUse (%v)", sanitized, specHas, useHas)
	}
}

// A tool call with empty arguments must serialize input as an object {}, never
// null — CodeWhisperer rejects a null input with 400.
func TestKiro_RenderRequest_EmptyToolInputIsObject(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-sonnet-4.5",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "go"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "c1", Name: "ping", Arguments: nil}},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "again"}}},
		},
		Tools: []core.Tool{{Name: "ping", Description: "d", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)
	found := false
	for _, h := range cs["history"].([]any) {
		if arm, ok := h.(map[string]any)["assistantResponseMessage"].(map[string]any); ok {
			if tus, ok := arm["toolUses"].([]any); ok {
				for _, tu := range tus {
					input := tu.(map[string]any)["input"]
					if input == nil {
						t.Errorf("tool input must be an object, got null")
					}
					if _, ok := input.(map[string]any); !ok {
						t.Errorf("tool input must be an object, got %T", input)
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected a toolUse with input in history")
	}
}

// TestKiro_RenderRequest_ClaudeCodeScenario reproduces the exact conversation
// shape captured from a KIRO_DEBUG dump of Claude Code talking to Kiro Opus 4.8
// (thinking): several alternating turns, a final assistant turn carrying a
// toolUse (Bash), and a trailing tool result promoted to the current message,
// with the client's full tool set attached. It asserts the rendered payload is
// structurally valid (alternation, current message tools + tool result), so we
// can distinguish a codec defect from an upstream model-policy rejection.
func TestKiro_RenderRequest_ClaudeCodeScenario(t *testing.T) {
	build := func(model string) *core.ChatRequest {
		return &core.ChatRequest{
			Model:  model,
			System: "You are Claude Code.",
			Messages: []core.Message{
				{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "bro"}}},
				{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: "Yo. What we building?"}}},
				{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "improve UI/UX for passphrase import/export"}}},
				{Role: core.RoleAssistant, Content: []core.ContentPart{
					{Type: core.PartText, Text: "Need find export/import passphrase UI first. Let me locate it."},
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tooluse_abc", Name: "Bash", Arguments: json.RawMessage(`{"command":"grep -rln passphrase frontend/src","description":"find passphrase UI"}`)}},
				}},
				{Role: core.RoleTool, Content: []core.ContentPart{
					{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tooluse_abc", Content: "frontend/src/pages/Settings.tsx"}},
				}},
			},
			Tools: []core.Tool{
				{Name: "Bash", Description: "run a shell command", Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
				{Name: "Read", Description: "read a file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
			},
		}
	}

	render := func(model string) map[string]any {
		body, err := KiroCodec{}.RenderRequest(build(model))
		if err != nil {
			t.Fatalf("render %s: %v", model, err)
		}
		var env map[string]any
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("unmarshal %s: %v", model, err)
		}
		return env
	}

	env := render("claude-opus-4.8-thinking")
	cs := env["conversationState"].(map[string]any)

	// History must alternate strictly and end with an assistant turn (the
	// trailing user tool-result is promoted to currentMessage).
	hist := cs["history"].([]any)
	if len(hist) == 0 {
		t.Fatal("expected non-empty history")
	}
	prevUser := false
	for i, h := range hist {
		hm := h.(map[string]any)
		_, isUser := hm["userInputMessage"]
		_, isAsst := hm["assistantResponseMessage"]
		if !isUser && !isAsst {
			t.Fatalf("history[%d] is neither user nor assistant: %v", i, hm)
		}
		if i == 0 && !isUser {
			t.Fatalf("history must start with a user turn, got %v", hm)
		}
		if i > 0 && isUser == prevUser {
			t.Fatalf("history[%d] breaks alternation (consecutive %s)", i, map[bool]string{true: "user", false: "assistant"}[isUser])
		}
		prevUser = isUser
	}
	if _, lastIsAsst := hist[len(hist)-1].(map[string]any)["assistantResponseMessage"]; !lastIsAsst {
		t.Fatalf("history should end with an assistant turn, got %v", hist[len(hist)-1])
	}

	// Current message must carry the tools array and the tool result.
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	ctx, ok := cm["userInputMessageContext"].(map[string]any)
	if !ok {
		t.Fatalf("current message missing userInputMessageContext: %v", cm)
	}
	if ctx["tools"] == nil {
		t.Errorf("current message must carry tools array: %v", ctx)
	}
	trs := asAnySlice(ctx["toolResults"])
	if len(trs) == 0 {
		t.Errorf("current message must carry the trailing tool result: %v", ctx)
	} else if id, _ := trs[0].(map[string]any)["toolUseId"].(string); id != "tooluse_abc" {
		t.Errorf("tool result should reference tooluse_abc, got %q", id)
	}

	// The assistant toolUse in history must have a non-null object input.
	var sawToolUse bool
	for _, h := range hist {
		if arm, ok := h.(map[string]any)["assistantResponseMessage"].(map[string]any); ok {
			for _, tu := range asAnySlice(arm["toolUses"]) {
				sawToolUse = true
				if _, ok := tu.(map[string]any)["input"].(map[string]any); !ok {
					t.Errorf("toolUse input must be an object, got %T", tu.(map[string]any)["input"])
				}
			}
		}
	}
	if !sawToolUse {
		t.Error("expected an assistant toolUse in history")
	}

	// The only difference between Opus 4.8 and a known-good model must be the
	// modelId string — proving the codec treats them identically and any 400 is
	// an upstream model-policy decision, not a rendering defect.
	opus := render("claude-opus-4.8-thinking")
	sonnet := render("claude-sonnet-4.5-thinking")
	normalizeKiroModelIds(opus)
	normalizeKiroModelIds(sonnet)
	stripVolatile(opus)
	stripVolatile(sonnet)
	ob, _ := json.Marshal(opus)
	sb, _ := json.Marshal(sonnet)
	if string(ob) != string(sb) {
		t.Errorf("Opus and Sonnet payloads differ beyond modelId:\nopus:   %s\nsonnet: %s", ob, sb)
	}
}

// asAnySlice coerces a JSON-decoded value into []any (nil-safe).
func asAnySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// normalizeKiroModelIds rewrites every modelId in a decoded payload to a fixed
// sentinel so two payloads can be compared independent of model name.
func normalizeKiroModelIds(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "modelId" {
				t[k] = "MODEL"
				continue
			}
			normalizeKiroModelIds(val)
		}
	case []any:
		for _, e := range t {
			normalizeKiroModelIds(e)
		}
	}
}

// stripVolatile removes fields that vary per render (conversationId, the
// time-context marker) so structural comparison is stable.
func stripVolatile(env map[string]any) {
	cs, ok := env["conversationState"].(map[string]any)
	if !ok {
		return
	}
	delete(cs, "conversationId")
	cm, ok := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	if ok {
		if content, ok := cm["content"].(string); ok {
			cm["content"] = stripTimeContext(content)
		}
	}
}

func stripTimeContext(s string) string {
	start := strings.Index(s, "[Context: Current time")
	if start < 0 {
		return s
	}
	end := strings.Index(s[start:], "]")
	if end < 0 {
		return s
	}
	return s[:start] + s[start+end+1:]
}

// TestKiro_RenderRequest_NormalizesToolSchema verifies the tool schema is passed
// through to Kiro as-is (matching 9router's proven-good translator), with only a
// "required" array guaranteed present. Kiro accepts full JSON-Schema draft
// documents, so client keywords like "$schema" and "additionalProperties" are
// preserved rather than stripped — stripping diverged from the working shape and
// was unnecessary.
func TestKiro_RenderRequest_NormalizesToolSchema(t *testing.T) {
	req := &core.ChatRequest{
		Model: "claude-opus-4.8-thinking",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "go"}}},
		},
		Tools: []core.Tool{
			{
				Name:        "EnterPlanMode",
				Description: "plan",
				Parameters: json.RawMessage(`{
					"$schema":"https://json-schema.org/draft/2020-12/schema",
					"additionalProperties":false,
					"properties":{"x":{"type":"string"}},
					"type":"object"
				}`),
			},
			{
				Name:        "EmptySchema",
				Description: "no params",
				Parameters:  nil,
			},
		},
	}
	body, err := KiroCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatal(err)
	}

	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	cs := env["conversationState"].(map[string]any)
	cm := cs["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
	ctx := cm["userInputMessageContext"].(map[string]any)
	tools := asAnySlice(ctx["tools"])
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	schemaOf := func(i int) map[string]any {
		spec := tools[i].(map[string]any)["toolSpecification"].(map[string]any)
		return spec["inputSchema"].(map[string]any)["json"].(map[string]any)
	}

	// First tool: client schema preserved verbatim, including $schema, and a
	// required array is present.
	s0 := schemaOf(0)
	if s0["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Errorf("client $schema should be preserved, got %v", s0["$schema"])
	}
	if s0["type"] != "object" {
		t.Errorf("type should be preserved, got %v", s0["type"])
	}
	if _, ok := s0["required"]; !ok {
		t.Errorf("required array should be present, got %v", s0)
	}

	// Second tool: empty/absent schema becomes a minimal object schema.
	s1 := schemaOf(1)
	if s1["type"] != "object" {
		t.Errorf("empty schema should default to type=object, got %v", s1["type"])
	}
	if _, ok := s1["required"]; !ok {
		t.Errorf("empty schema should carry required array, got %v", s1)
	}
}

// TestKiro_RenderRequest_StripsBracketSuffix verifies that bracket-suffix
// annotations (e.g. "[1m]" for 1M-context markers) are stripped from the
// model name before it reaches Kiro. CodeWhisperer rejects any modelId
// containing literal bracket segments with "Improperly formed request"
// (HTTP 400).
func TestKiro_RenderRequest_StripsBracketSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"claude-opus-4.8-thinking[1m]", "claude-opus-4.8"},
		{"claude-sonnet-4.5[1m]", "claude-sonnet-4.5"},
		{"claude-opus-4.7-thinking-agentic[1m]", "claude-opus-4.7"},
		{"claude-sonnet-4.5", "claude-sonnet-4.5"},
	}
	for _, tc := range cases {
		req := &core.ChatRequest{
			Model: tc.in,
			Messages: []core.Message{
				{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
			},
		}
		body, err := KiroCodec{}.RenderRequest(req)
		if err != nil {
			t.Fatalf("render %q: %v", tc.in, err)
		}
		var env map[string]any
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.in, err)
		}
		cm := env["conversationState"].(map[string]any)["currentMessage"].(map[string]any)["userInputMessage"].(map[string]any)
		if got := cm["modelId"]; got != tc.want {
			t.Errorf("model %q -> modelId %v, want %q", tc.in, got, tc.want)
		}
		// The bracket segment must not survive anywhere in the payload.
		if strings.Contains(string(body), "[1m]") {
			t.Errorf("payload for %q still contains [1m]: %s", tc.in, body)
		}
	}
}

func TestStripKiroBracketSuffix(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4.8[1m]":                  "claude-opus-4.8",
		"claude-opus-4.8-thinking[1m]":         "claude-opus-4.8-thinking",
		"claude-opus-4.8-thinking-agentic[1m]": "claude-opus-4.8-thinking-agentic",
		"claude-sonnet-4.5":                    "claude-sonnet-4.5",
		"model[200k]":                          "model",
		"model[1m] ":                           "model",
		"":                                     "",
	}
	for in, want := range cases {
		if got := stripKiroBracketSuffix(in); got != want {
			t.Errorf("stripKiroBracketSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeKiroToolName(t *testing.T) {
	cases := map[string]string{
		"Read":                 "Read",
		"mcp__server__do":      "mcp__server__do",
		"do-thing":             "do_thing",
		"weird.name":           "weird_name",
		"123abc":               "t_123abc",
		"":                     "tool",
		strings.Repeat("x", 80): strings.Repeat("x", 64),
	}
	for in, want := range cases {
		if got := sanitizeKiroToolName(in); got != want {
			t.Errorf("sanitizeKiroToolName(%q) = %q, want %q", in, got, want)
		}
		if got := sanitizeKiroToolName(in); !isValidKiroToolName(got) {
			t.Errorf("sanitizeKiroToolName(%q) = %q is not a valid name", in, got)
		}
	}
}

// isValidKiroToolName reports whether a name matches CodeWhisperer's accepted
// format ^[a-zA-Z][a-zA-Z0-9_]{0,63}$. Test helper.
func isValidKiroToolName(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isLetter {
			return false
		}
		if !isLetter && !isDigit && r != '_' {
			return false
		}
	}
	return true
}

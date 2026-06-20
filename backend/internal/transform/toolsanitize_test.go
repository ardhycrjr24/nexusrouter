package transform

import (
	"encoding/json"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestToolArgSanitizer_PassesThroughNonToolChunks(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk

	s.Process(core.StreamChunk{Type: core.ChunkText, Delta: "hello"}, func(c core.StreamChunk) {
		emitted = append(emitted, c)
	})

	if len(emitted) != 1 || emitted[0].Delta != "hello" {
		t.Errorf("non-tool chunk not passed through: %+v", emitted)
	}
}

func TestToolArgSanitizer_BuffersAndFlushes(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk

	// First chunk: tool call start with ID.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID:        "tc1",
			Name:      "Read",
			Arguments: json.RawMessage("{}"),
		},
	}, func(c core.StreamChunk) { emitted = append(emitted, c) })

	// Second chunk: argument delta.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			Arguments: json.RawMessage(`{"file_path":"/tmp/a.txt"}`),
		},
	}, func(c core.StreamChunk) { emitted = append(emitted, c) })

	// Nothing emitted yet (buffering).
	if len(emitted) != 0 {
		t.Errorf("expected 0 emitted during buffering, got %d", len(emitted))
	}

	// Flush.
	s.Flush(func(c core.StreamChunk) { emitted = append(emitted, c) })

	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted on flush, got %d", len(emitted))
	}
	if emitted[0].ToolCall.ID != "tc1" {
		t.Errorf("expected ID 'tc1', got %q", emitted[0].ToolCall.ID)
	}
	if emitted[0].ToolCall.Name != "Read" {
		t.Errorf("expected Name 'Read', got %q", emitted[0].ToolCall.Name)
	}
}

func TestToolArgSanitizer_SanitizesReadLimit(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":"100"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if args["limit"] != float64(100) {
		t.Errorf("expected limit 100 (float64), got %v (%T)", args["limit"], args["limit"])
	}
}

func TestToolArgSanitizer_ClampsReadLimit(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":5000}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if args["limit"] != float64(2000) {
		t.Errorf("expected limit clamped to 2000, got %v", args["limit"])
	}
}

func TestToolArgSanitizer_ClampsReadLimitMin(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","limit":0}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if args["limit"] != float64(1) {
		t.Errorf("expected limit clamped to 1, got %v", args["limit"])
	}
}

func TestToolArgSanitizer_SanitizesReadOffset(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","offset":"-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if args["offset"] != float64(0) {
		t.Errorf("expected offset clamped to 0, got %v", args["offset"])
	}
}

func TestToolArgSanitizer_RemovesPagesForNonPdf(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/a.txt","pages":"1-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if _, ok := args["pages"]; ok {
		t.Error("expected 'pages' to be removed for non-PDF file")
	}
}

func TestToolArgSanitizer_KeepsPagesForPdf(t *testing.T) {
	sanitized := sanitizeToolArgs("Read", `{"file_path":"/tmp/doc.pdf","pages":"1-5"}`)
	var args map[string]any
	json.Unmarshal([]byte(sanitized), &args)

	if args["pages"] != "1-5" {
		t.Errorf("expected pages '1-5' for PDF, got %v", args["pages"])
	}
}

func TestToolArgSanitizer_FlushOnNewToolAtSameIndex(t *testing.T) {
	s := NewToolArgSanitizer()
	var emitted []core.StreamChunk
	emit := func(c core.StreamChunk) { emitted = append(emitted, c) }

	// First tool call at index 0.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID: "tc1", Name: "Read",
			Arguments: json.RawMessage(`{"file_path":"/a"}`),
		},
	}, emit)

	// Second tool call at same index 0 — should flush first.
	s.Process(core.StreamChunk{
		Type:  core.ChunkToolCall,
		Index: 0,
		ToolCall: &core.ToolCall{
			ID: "tc2", Name: "Write",
			Arguments: json.RawMessage(`{"file_path":"/b","content":"hi"}`),
		},
	}, emit)

	if len(emitted) != 1 {
		t.Fatalf("expected 1 flushed tool call, got %d", len(emitted))
	}
	if emitted[0].ToolCall.ID != "tc1" {
		t.Errorf("expected first tool 'tc1', got %q", emitted[0].ToolCall.ID)
	}

	// Flush remaining.
	s.Flush(emit)
	if len(emitted) != 2 {
		t.Fatalf("expected 2 total, got %d", len(emitted))
	}
	if emitted[1].ToolCall.ID != "tc2" {
		t.Errorf("expected second tool 'tc2', got %q", emitted[1].ToolCall.ID)
	}
}

func TestSanitizeToolArgs_InvalidJSON(t *testing.T) {
	// Should return original if unparseable.
	got := sanitizeToolArgs("Read", "not json")
	if got != "not json" {
		t.Errorf("expected passthrough for invalid JSON, got %q", got)
	}
}

func TestSanitizeToolArgs_PassthroughValidArgs(t *testing.T) {
	input := `{"file_path":"/tmp/a.txt","limit":50}`
	got := sanitizeToolArgs("Read", input)
	var args map[string]any
	json.Unmarshal([]byte(got), &args)

	if args["limit"] != float64(50) {
		t.Errorf("valid limit should pass through, got %v", args["limit"])
	}
}

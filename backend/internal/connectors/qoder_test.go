package connectors

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

func TestQoderSerializeTools_UsesOpenAIFunctionWrapper(t *testing.T) {
	tools := serializeTools([]core.Tool{
		{
			Name:        "read_file",
			Description: "Read a file",
			Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	})

	if len(tools) != 1 {
		t.Fatalf("expected 1 serialized tool, got %d", len(tools))
	}

	var got map[string]any
	if err := json.Unmarshal(tools[0], &got); err != nil {
		t.Fatalf("unmarshal serialized tool: %v", err)
	}

	if got["type"] != "function" {
		t.Fatalf("tool type = %v, want function", got["type"])
	}
	fn, ok := got["function"].(map[string]any)
	if !ok {
		t.Fatalf("function missing or wrong type: %v", got["function"])
	}
	if fn["name"] != "read_file" {
		t.Fatalf("tool name = %v, want read_file", fn["name"])
	}
	if fn["description"] != "Read a file" {
		t.Fatalf("tool description = %v, want Read a file", fn["description"])
	}

	schema, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters missing or wrong type: %v", fn["parameters"])
	}
	if schema["type"] != "object" {
		t.Fatalf("parameters.type = %v, want object", schema["type"])
	}
}

func TestQoderSerializeTools_DefaultsInvalidSchema(t *testing.T) {
	tools := serializeTools([]core.Tool{
		{Name: "empty"},
		{Name: "bad", Parameters: json.RawMessage(`not-json`)},
	})

	if len(tools) != 2 {
		t.Fatalf("expected 2 serialized tools, got %d", len(tools))
	}

	for i, raw := range tools {
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal serialized tool %d: %v", i, err)
		}
		fn, ok := got["function"].(map[string]any)
		if !ok {
			t.Fatalf("function missing or wrong type for tool %d: %v", i, got)
		}
		schema, ok := fn["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("parameters missing or wrong type for tool %d: %v", i, got)
		}
		if schema["type"] != "object" {
			t.Fatalf("parameters.type for tool %d = %v, want object", i, schema["type"])
		}
		if _, ok := schema["properties"].(map[string]any); !ok {
			t.Fatalf("parameters.properties missing or wrong type for tool %d: %v", i, schema)
		}
	}
}

func TestUnwrapQoderSSELineWithError_SurfacesEnvelopeError(t *testing.T) {
	line := `data: {"statusCodeValue":400,"body":"Invalid tool parameters"}`

	inner, ok, err := unwrapQoderSSELineWithError(line, "qoder", "claude-sonnet-4")
	if inner != "" || ok {
		t.Fatalf("expected no inner payload, got inner=%q ok=%v", inner, ok)
	}

	var pe *core.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ProviderError, got %T %v", err, err)
	}
	if pe.Kind != core.ErrBadRequest {
		t.Fatalf("error kind = %v, want %v", pe.Kind, core.ErrBadRequest)
	}
	if pe.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", pe.StatusCode)
	}
	if pe.Message != "Invalid tool parameters" {
		t.Fatalf("message = %q, want Invalid tool parameters", pe.Message)
	}
}

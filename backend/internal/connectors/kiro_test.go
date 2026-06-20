package connectors

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// encodeEventStreamFrame builds a minimal AWS EventStream frame with a single
// ":event-type" string header and a JSON payload. CRC fields are zeroed (the
// parser does not validate them). This mirrors the frame layout the Kiro
// connector decodes.
func encodeEventStreamFrame(eventType string, payload []byte) []byte {
	// Header: 1-byte name len, name, 1-byte type(7), 2-byte value len, value.
	name := ":event-type"
	var hdr bytes.Buffer
	hdr.WriteByte(byte(len(name)))
	hdr.WriteString(name)
	hdr.WriteByte(7) // string type
	var vl [2]byte
	binary.BigEndian.PutUint16(vl[:], uint16(len(eventType)))
	hdr.Write(vl[:])
	hdr.WriteString(eventType)

	headers := hdr.Bytes()
	headersLen := len(headers)
	totalLen := 12 + headersLen + len(payload) + 4 // prelude(8)+preludeCRC(4)+headers+payload+msgCRC(4)

	var out bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(totalLen))
	out.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(headersLen))
	out.Write(u32[:])
	out.Write([]byte{0, 0, 0, 0}) // prelude CRC (ignored)
	out.Write(headers)
	out.Write(payload)
	out.Write([]byte{0, 0, 0, 0}) // message CRC (ignored)
	return out.Bytes()
}

func TestEventStreamParser_DecodesFrames(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(encodeEventStreamFrame("assistantResponseEvent", []byte(`{"content":"Hello"}`)))
	stream.Write(encodeEventStreamFrame("assistantResponseEvent", []byte(`{"content":" world"}`)))
	stream.Write(encodeEventStreamFrame("messageStopEvent", []byte(`{}`)))

	parser := newEventStreamParser(&stream)
	var events []string
	for {
		frame, err := parser.next()
		if err == errEventStreamEOF {
			break
		}
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if frame == nil {
			continue
		}
		events = append(events, frame.headers[":event-type"])
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 frames, got %d: %v", len(events), events)
	}
	if events[0] != "assistantResponseEvent" || events[2] != "messageStopEvent" {
		t.Errorf("unexpected event order: %v", events)
	}
}

func TestKiroFrameToChunks_TextAndStop(t *testing.T) {
	seen := map[string]bool{}
	hasTool := false

	frame := mustDecode(t, encodeEventStreamFrame("assistantResponseEvent", []byte(`{"content":"hi"}`)))
	chunks := kiroFrameToChunks(frame, seen, &hasTool)
	if len(chunks) != 1 || chunks[0].Type != core.ChunkText || chunks[0].Delta != "hi" {
		t.Fatalf("expected text chunk 'hi', got %+v", chunks)
	}

	stop := mustDecode(t, encodeEventStreamFrame("messageStopEvent", []byte(`{}`)))
	chunks = kiroFrameToChunks(stop, seen, &hasTool)
	if len(chunks) != 1 || chunks[0].Type != core.ChunkFinish || chunks[0].FinishReason != core.FinishStop {
		t.Fatalf("expected finish=stop, got %+v", chunks)
	}
}

func TestKiroFrameToChunks_ToolUse(t *testing.T) {
	seen := map[string]bool{}
	hasTool := false

	frame := mustDecode(t, encodeEventStreamFrame("toolUseEvent",
		[]byte(`{"toolUseId":"t1","name":"get_weather","input":{"city":"SF"}}`)))
	chunks := kiroFrameToChunks(frame, seen, &hasTool)
	if !hasTool {
		t.Error("hasTool should be set")
	}
	// First chunk announces the tool, second carries its arguments.
	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	if chunks[0].ToolCall.Name != "get_weather" {
		t.Errorf("tool name wrong: %v", chunks[0].ToolCall.Name)
	}
	if !bytes.Contains(chunks[1].ToolCall.Arguments, []byte("SF")) {
		t.Errorf("tool args missing input: %s", chunks[1].ToolCall.Arguments)
	}
}

func TestKiroFrameToChunks_UsageEvent(t *testing.T) {
	seen := map[string]bool{}
	hasTool := false

	// Top-level inputTokens/outputTokens under usageEvent name.
	frame := mustDecode(t, encodeEventStreamFrame("usageEvent",
		[]byte(`{"inputTokens":120,"outputTokens":34}`)))
	chunks := kiroFrameToChunks(frame, seen, &hasTool)
	if len(chunks) != 1 || chunks[0].Type != core.ChunkUsage || chunks[0].Usage == nil {
		t.Fatalf("expected usage chunk, got %+v", chunks)
	}
	if chunks[0].Usage.PromptTokens != 120 || chunks[0].Usage.CompletionTokens != 34 {
		t.Errorf("usage tokens wrong: %+v", chunks[0].Usage)
	}
	if chunks[0].Usage.TotalTokens != 154 {
		t.Errorf("total tokens wrong: %d", chunks[0].Usage.TotalTokens)
	}
}

func TestKiroFrameToChunks_MetricsEventNested(t *testing.T) {
	seen := map[string]bool{}
	hasTool := false

	// Counts nested under a key matching the event type.
	frame := mustDecode(t, encodeEventStreamFrame("metricsEvent",
		[]byte(`{"metricsEvent":{"inputTokens":10,"outputTokens":5}}`)))
	chunks := kiroFrameToChunks(frame, seen, &hasTool)
	if len(chunks) != 1 || chunks[0].Type != core.ChunkUsage {
		t.Fatalf("expected usage chunk, got %+v", chunks)
	}
	if chunks[0].Usage.PromptTokens != 10 || chunks[0].Usage.CompletionTokens != 5 {
		t.Errorf("nested usage wrong: %+v", chunks[0].Usage)
	}
}

func TestParseKiroUsage_ZeroReturnsNil(t *testing.T) {
	if u := parseKiroUsage("usageEvent", []byte(`{"inputTokens":0,"outputTokens":0}`)); u != nil {
		t.Errorf("expected nil for zero usage, got %+v", u)
	}
	if u := parseKiroUsage("usageEvent", []byte(`not json`)); u != nil {
		t.Errorf("expected nil for bad json, got %+v", u)
	}
}

func TestEstimateKiroUsage(t *testing.T) {
	req := &core.ChatRequest{
		System: "you are helpful", // 15 chars
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "hello there"}, // 11 chars
			}},
		},
	}
	// input chars = 15 + 11 = 26 → ceil(26/4) = 7 prompt tokens
	// output chars = 40 → ceil(40/4) = 10 completion tokens
	u := estimateKiroUsage(req, 40)
	if u == nil {
		t.Fatal("expected non-nil estimate")
	}
	if u.PromptTokens != 7 {
		t.Errorf("prompt tokens = %d, want 7", u.PromptTokens)
	}
	if u.CompletionTokens != 10 {
		t.Errorf("completion tokens = %d, want 10", u.CompletionTokens)
	}
	if u.TotalTokens != 17 {
		t.Errorf("total tokens = %d, want 17", u.TotalTokens)
	}

	// Empty request + zero output → nil (nothing to record).
	if got := estimateKiroUsage(&core.ChatRequest{}, 0); got != nil {
		t.Errorf("expected nil for empty request, got %+v", got)
	}
}

func TestCharsToTokens(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0}, {-5, 0}, {1, 1}, {4, 1}, {5, 2}, {8, 2}, {9, 3},
	}
	for _, c := range cases {
		if got := charsToTokens(c.in); got != c.want {
			t.Errorf("charsToTokens(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func mustDecode(t *testing.T, frame []byte) *eventStreamFrame {
	t.Helper()
	f, err := decodeEventStreamFrame(frame)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return f
}

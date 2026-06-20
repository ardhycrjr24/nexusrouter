package connectors

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
)

// Cursor speaks a Connect-RPC protobuf wire format with no published .proto.
// This is a faithful port of 9router's cursorProtobuf.js: every field number is
// hardcoded, varints are written manually, and the request/response are wrapped
// in Connect-RPC frames (1 flag byte + 4-byte big-endian length + payload).

// protobuf wire types.
const (
	wireVarint = 0
	wireLen    = 2
)

// Cursor enum values.
const (
	cursorRoleUser      = 1
	cursorRoleAssistant = 2
	cursorUnifiedChat   = 1
	cursorUnifiedAgent  = 2
	cursorThinkingNone  = 0
	cursorThinkingMed   = 1
	cursorThinkingHigh  = 2
	cursorToolV2MCP     = 19
)

// Field numbers (mirrors the FIELD map in cursorProtobuf.js).
const (
	// StreamUnifiedChatRequestWithTools
	fREQUEST = 1

	// StreamUnifiedChatRequest
	fMESSAGES           = 1
	fUNKNOWN_2          = 2
	fINSTRUCTION        = 3
	fUNKNOWN_4          = 4
	fMODEL              = 5
	fWEB_TOOL           = 8
	fUNKNOWN_13         = 13
	fCURSOR_SETTING     = 15
	fUNKNOWN_19         = 19
	fCONVERSATION_ID    = 23
	fMETADATA           = 26
	fIS_AGENTIC         = 27
	fSUPPORTED_TOOLS    = 29
	fMESSAGE_IDS        = 30
	fMCP_TOOLS          = 34
	fLARGE_CONTEXT      = 35
	fUNKNOWN_38         = 38
	fUNIFIED_MODE       = 46
	fUNKNOWN_47         = 47
	fSHOULD_DISABLE     = 48
	fTHINKING_LEVEL     = 49
	fUNKNOWN_51         = 51
	fUNKNOWN_53         = 53
	fUNIFIED_MODE_NAME  = 54

	// ConversationMessage
	fMSG_CONTENT        = 1
	fMSG_ROLE           = 2
	fMSG_ID             = 13
	fMSG_IS_AGENTIC     = 29
	fMSG_UNIFIED_MODE   = 47
	fMSG_SUPPORTED_TOOL = 51

	// Model / Instruction
	fMODEL_NAME       = 1
	fMODEL_EMPTY      = 4
	fINSTRUCTION_TEXT = 1

	// CursorSetting
	fSETTING_PATH = 1
	fSETTING_U3   = 3
	fSETTING_U6   = 6
	fSETTING_U8   = 8
	fSETTING_U9   = 9
	fSETTING6_1   = 1
	fSETTING6_2   = 2

	// Metadata
	fMETA_PLATFORM  = 1
	fMETA_ARCH      = 2
	fMETA_VERSION   = 3
	fMETA_CWD       = 4
	fMETA_TIMESTAMP = 5

	// MessageId
	fMSGID_ID   = 1
	fMSGID_ROLE = 3

	// MCPTool
	fMCP_TOOL_NAME   = 1
	fMCP_TOOL_DESC   = 2
	fMCP_TOOL_PARAMS = 3
	fMCP_TOOL_SERVER = 4

	// Response
	fTOOL_CALL      = 1
	fRESPONSE       = 2
	fTOOL_ID        = 3
	fTOOL_NAME      = 9
	fTOOL_RAW_ARGS  = 10
	fTOOL_IS_LAST   = 11
	fTOOL_MCP_PARAM = 27
	fMCP_TOOLS_LIST = 1
	fMCP_NESTED_NAM = 1
	fMCP_NESTED_PRM = 3
	fRESPONSE_TEXT  = 1
	fTHINKING       = 25
	fTHINKING_TEXT  = 1
)

// ---- primitive encoding -----------------------------------------------------

func encodeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v&0x7F) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

// encodeTag writes a field tag (field number + wire type).
func encodeTag(buf *bytes.Buffer, field, wire int) {
	encodeVarint(buf, uint64(field<<3|wire))
}

// pbVarint encodes a varint field.
func pbVarint(field int, v uint64) []byte {
	var b bytes.Buffer
	encodeTag(&b, field, wireVarint)
	encodeVarint(&b, v)
	return b.Bytes()
}

// pbLen encodes a length-delimited field (string or bytes).
func pbLen(field int, data []byte) []byte {
	var b bytes.Buffer
	encodeTag(&b, field, wireLen)
	encodeVarint(&b, uint64(len(data)))
	b.Write(data)
	return b.Bytes()
}

func pbStr(field int, s string) []byte { return pbLen(field, []byte(s)) }

// ---- request building -------------------------------------------------------

// cursorMessage is the intermediate per-turn form (content + role flag).
type cursorMessage struct {
	content string
	role    int
}

// buildCursorBody renders a canonical request into the framed Connect-RPC
// protobuf body Cursor expects. Mirrors generateCursorBody → buildChatRequest.
func buildCursorBody(req *core.ChatRequest, forceAgentMode bool) []byte {
	hasTools := len(req.Tools) > 0
	isAgentic := hasTools || forceAgentMode

	// Flatten canonical messages to cursor messages (text content + role).
	var msgs []cursorMessage
	// Prepend system as a leading user turn (Cursor folds it in).
	if req.System != "" {
		msgs = append(msgs, cursorMessage{content: req.System, role: cursorRoleUser})
	}
	for _, m := range req.Messages {
		role := cursorRoleUser
		if m.Role == core.RoleAssistant {
			role = cursorRoleAssistant
		}
		text := m.TextContent()
		// Tool results fold into the user turn text (NexusRouter keeps tool
		// results as text content for the Cursor transport).
		for _, p := range m.Content {
			if p.Type == core.PartToolResult && p.ToolResult != nil {
				if text != "" {
					text += "\n"
				}
				text += p.ToolResult.Content
			}
		}
		msgs = append(msgs, cursorMessage{content: text, role: role})
	}

	thinkingLevel := cursorThinkingNone
	if req.Reasoning != nil {
		switch strings.ToLower(req.Reasoning.Effort) {
		case "medium":
			thinkingLevel = cursorThinkingMed
		case "high":
			thinkingLevel = cursorThinkingHigh
		}
	}

	var inner bytes.Buffer
	var messageIDs []struct {
		id   string
		role int
	}

	for i, m := range msgs {
		id := uuid.NewString()
		isLast := i == len(msgs)-1
		inner.Write(pbLen(fMESSAGES, encodeCursorMessage(m.content, m.role, id, isLast, hasTools)))
		messageIDs = append(messageIDs, struct {
			id   string
			role int
		}{id, m.role})
	}

	// Static fields (order + values match encodeRequest exactly).
	inner.Write(pbVarint(fUNKNOWN_2, 1))
	inner.Write(pbLen(fINSTRUCTION, nil))
	inner.Write(pbVarint(fUNKNOWN_4, 1))
	inner.Write(pbLen(fMODEL, encodeCursorModel(req.Model)))
	inner.Write(pbStr(fWEB_TOOL, ""))
	inner.Write(pbVarint(fUNKNOWN_13, 1))
	inner.Write(pbLen(fCURSOR_SETTING, encodeCursorSetting()))
	inner.Write(pbVarint(fUNKNOWN_19, 1))
	inner.Write(pbStr(fCONVERSATION_ID, uuid.NewString()))
	inner.Write(pbLen(fMETADATA, encodeCursorMetadata()))

	if isAgentic {
		inner.Write(pbVarint(fIS_AGENTIC, 1))
		var sv bytes.Buffer
		encodeVarint(&sv, 1)
		inner.Write(pbLen(fSUPPORTED_TOOLS, sv.Bytes()))
	} else {
		inner.Write(pbVarint(fIS_AGENTIC, 0))
	}

	for _, mid := range messageIDs {
		inner.Write(pbLen(fMESSAGE_IDS, encodeCursorMessageID(mid.id, mid.role)))
	}

	for _, t := range req.Tools {
		inner.Write(pbLen(fMCP_TOOLS, encodeCursorMcpTool(t)))
	}

	inner.Write(pbVarint(fLARGE_CONTEXT, 0))
	inner.Write(pbVarint(fUNKNOWN_38, 0))
	if isAgentic {
		inner.Write(pbVarint(fUNIFIED_MODE, cursorUnifiedAgent))
	} else {
		inner.Write(pbVarint(fUNIFIED_MODE, cursorUnifiedChat))
	}
	inner.Write(pbStr(fUNKNOWN_47, ""))
	if isAgentic {
		inner.Write(pbVarint(fSHOULD_DISABLE, 0))
	} else {
		inner.Write(pbVarint(fSHOULD_DISABLE, 1))
	}
	inner.Write(pbVarint(fTHINKING_LEVEL, uint64(thinkingLevel)))
	inner.Write(pbVarint(fUNKNOWN_51, 0))
	inner.Write(pbVarint(fUNKNOWN_53, 1))
	if isAgentic {
		inner.Write(pbStr(fUNIFIED_MODE_NAME, "Agent"))
	} else {
		inner.Write(pbStr(fUNIFIED_MODE_NAME, "Ask"))
	}

	// Wrap in StreamUnifiedChatRequestWithTools.request (field 1).
	requestField := pbLen(fREQUEST, inner.Bytes())
	return wrapConnectRPCFrame(requestField)
}

func encodeCursorMessage(content string, role int, messageID string, isLast, hasTools bool) []byte {
	var b bytes.Buffer
	b.Write(pbStr(fMSG_CONTENT, content))
	b.Write(pbVarint(fMSG_ROLE, uint64(role)))
	b.Write(pbStr(fMSG_ID, messageID))
	agentic := uint64(0)
	if hasTools {
		agentic = 1
	}
	b.Write(pbVarint(fMSG_IS_AGENTIC, agentic))
	mode := uint64(cursorUnifiedChat)
	if hasTools {
		mode = cursorUnifiedAgent
	}
	b.Write(pbVarint(fMSG_UNIFIED_MODE, mode))
	if isLast && hasTools {
		var sv bytes.Buffer
		encodeVarint(&sv, 1)
		b.Write(pbLen(fMSG_SUPPORTED_TOOL, sv.Bytes()))
	}
	return b.Bytes()
}

func encodeCursorModel(name string) []byte {
	var b bytes.Buffer
	b.Write(pbStr(fMODEL_NAME, name))
	b.Write(pbLen(fMODEL_EMPTY, nil))
	return b.Bytes()
}

func encodeCursorSetting() []byte {
	var u6 bytes.Buffer
	u6.Write(pbLen(fSETTING6_1, nil))
	u6.Write(pbLen(fSETTING6_2, nil))

	var b bytes.Buffer
	b.Write(pbStr(fSETTING_PATH, "cursor\\aisettings"))
	b.Write(pbLen(fSETTING_U3, nil))
	b.Write(pbLen(fSETTING_U6, u6.Bytes()))
	b.Write(pbVarint(fSETTING_U8, 1))
	b.Write(pbVarint(fSETTING_U9, 1))
	return b.Bytes()
}

func encodeCursorMetadata() []byte {
	var b bytes.Buffer
	b.Write(pbStr(fMETA_PLATFORM, "linux"))
	b.Write(pbStr(fMETA_ARCH, "x64"))
	b.Write(pbStr(fMETA_VERSION, "v20.0.0"))
	b.Write(pbStr(fMETA_CWD, "/"))
	b.Write(pbStr(fMETA_TIMESTAMP, nowISO()))
	return b.Bytes()
}

func encodeCursorMessageID(id string, role int) []byte {
	var b bytes.Buffer
	b.Write(pbStr(fMSGID_ID, id))
	b.Write(pbVarint(fMSGID_ROLE, uint64(role)))
	return b.Bytes()
}

func encodeCursorMcpTool(t core.Tool) []byte {
	var b bytes.Buffer
	if t.Name != "" {
		b.Write(pbStr(fMCP_TOOL_NAME, t.Name))
	}
	if t.Description != "" {
		b.Write(pbStr(fMCP_TOOL_DESC, t.Description))
	}
	if len(t.Parameters) > 0 {
		b.Write(pbStr(fMCP_TOOL_PARAMS, string(t.Parameters)))
	}
	b.Write(pbStr(fMCP_TOOL_SERVER, "custom"))
	return b.Bytes()
}

// wrapConnectRPCFrame prefixes a payload with the 5-byte Connect-RPC header
// (1 flag byte, 4-byte big-endian length). Cursor requests are never compressed.
func wrapConnectRPCFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0x00 // no compression
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

// ---- primitive decoding -----------------------------------------------------

func decodeVarintAt(buf []byte, offset int) (uint64, int) {
	var result uint64
	var shift uint
	pos := offset
	for pos < len(buf) {
		b := buf[pos]
		result |= uint64(b&0x7F) << shift
		pos++
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result, pos
}

// pbField is one decoded protobuf field.
type pbField struct {
	wire  int
	value []byte // for LEN fields
	num   uint64 // for VARINT fields
}

// decodeProtoMessage decodes a flat protobuf message into field number → fields.
func decodeProtoMessage(data []byte) map[int][]pbField {
	fields := map[int][]pbField{}
	pos := 0
	for pos < len(data) {
		tag, p1 := decodeVarintAt(data, pos)
		if p1 == pos {
			break
		}
		fieldNum := int(tag >> 3)
		wire := int(tag & 0x07)
		pos = p1

		switch wire {
		case wireVarint:
			v, p2 := decodeVarintAt(data, pos)
			fields[fieldNum] = append(fields[fieldNum], pbField{wire: wire, num: v})
			pos = p2
		case wireLen:
			length, p2 := decodeVarintAt(data, pos)
			end := p2 + int(length)
			if end > len(data) {
				return fields
			}
			fields[fieldNum] = append(fields[fieldNum], pbField{wire: wire, value: data[p2:end]})
			pos = end
		case 1: // FIXED64
			pos += 8
		case 5: // FIXED32
			pos += 4
		default:
			return fields
		}
	}
	return fields
}

// cursorResult is the decoded content of one Cursor response frame payload.
type cursorResult struct {
	text     string
	thinking string
	toolCall *cursorToolCall
}

type cursorToolCall struct {
	id     string
	name   string
	args   string
	isLast bool
}

// parseConnectRPCFrame reads one Connect-RPC frame from buf, returning the
// decompressed payload and the number of bytes consumed (0 if incomplete).
func parseConnectRPCFrame(buf []byte) (payload []byte, consumed int, ok bool) {
	if len(buf) < 5 {
		return nil, 0, false
	}
	flags := buf[0]
	length := int(binary.BigEndian.Uint32(buf[1:5]))
	if len(buf) < 5+length {
		return nil, 0, false
	}
	payload = buf[5 : 5+length]
	if flags == 0x01 {
		if dec, err := gunzip(payload); err == nil {
			payload = dec
		}
	}
	return payload, 5 + length, true
}

// extractCursorResult decodes one response payload into text / thinking / tool.
func extractCursorResult(payload []byte) cursorResult {
	fields := decodeProtoMessage(payload)

	// Field 1: ClientSideToolV2Call.
	if tc := fields[fTOOL_CALL]; len(tc) > 0 && tc[0].wire == wireLen {
		if call := extractCursorToolCall(tc[0].value); call != nil {
			return cursorResult{toolCall: call}
		}
	}

	// Field 2: StreamUnifiedChatResponse.
	if rs := fields[fRESPONSE]; len(rs) > 0 && rs[0].wire == wireLen {
		nested := decodeProtoMessage(rs[0].value)
		var res cursorResult
		if t := nested[fRESPONSE_TEXT]; len(t) > 0 && t[0].wire == wireLen {
			res.text = string(t[0].value)
		}
		if th := nested[fTHINKING]; len(th) > 0 && th[0].wire == wireLen {
			tm := decodeProtoMessage(th[0].value)
			if tt := tm[fTHINKING_TEXT]; len(tt) > 0 && tt[0].wire == wireLen {
				res.thinking = string(tt[0].value)
			}
		}
		return res
	}
	return cursorResult{}
}

func extractCursorToolCall(data []byte) *cursorToolCall {
	tc := decodeProtoMessage(data)
	call := &cursorToolCall{}

	if v := tc[fTOOL_ID]; len(v) > 0 && v[0].wire == wireLen {
		full := string(v[0].value)
		call.id = strings.SplitN(full, "\n", 2)[0]
	}
	if v := tc[fTOOL_NAME]; len(v) > 0 && v[0].wire == wireLen {
		call.name = string(v[0].value)
	}
	if v := tc[fTOOL_IS_LAST]; len(v) > 0 && v[0].wire == wireVarint {
		call.isLast = v[0].num != 0
	}
	if v := tc[fTOOL_MCP_PARAM]; len(v) > 0 && v[0].wire == wireLen {
		mcp := decodeProtoMessage(v[0].value)
		if list := mcp[fMCP_TOOLS_LIST]; len(list) > 0 && list[0].wire == wireLen {
			tool := decodeProtoMessage(list[0].value)
			if n := tool[fMCP_NESTED_NAM]; len(n) > 0 && n[0].wire == wireLen {
				call.name = string(n[0].value)
			}
			if p := tool[fMCP_NESTED_PRM]; len(p) > 0 && p[0].wire == wireLen {
				call.args = string(p[0].value)
			}
		}
	}
	if call.args == "" {
		if v := tc[fTOOL_RAW_ARGS]; len(v) > 0 && v[0].wire == wireLen {
			call.args = string(v[0].value)
		}
	}
	if call.args == "" {
		call.args = "{}"
	}

	if call.id != "" && call.name != "" {
		return call
	}
	return nil
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

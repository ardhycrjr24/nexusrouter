package connectors

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// Cursor uses a Connect-RPC protobuf transport authenticated by a set of
// derived headers, the most important being x-cursor-checksum (the "Jyh
// cipher"). This is a faithful port of 9router's cursorChecksum.js.

// urlSafeAlphabet is the URL-safe base64 alphabet (no padding) used by the
// checksum encoder.
const urlSafeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// generateHashed64Hex returns sha256(input+salt) as a 64-char hex string.
func generateHashed64Hex(input, salt string) string {
	sum := sha256.Sum256([]byte(input + salt))
	return hex.EncodeToString(sum[:])
}

// generateCursorChecksum builds the x-cursor-checksum value via the Jyh cipher:
// a 6-byte big-endian timestamp (ms/1e6), XOR/add byte obfuscation, URL-safe
// base64, suffixed with the machine id.
func generateCursorChecksum(machineID string) string {
	timestamp := time.Now().UnixMilli() / 1000000

	bytesArr := []byte{
		byte((timestamp >> 40) & 0xFF),
		byte((timestamp >> 32) & 0xFF),
		byte((timestamp >> 24) & 0xFF),
		byte((timestamp >> 16) & 0xFF),
		byte((timestamp >> 8) & 0xFF),
		byte(timestamp & 0xFF),
	}

	// Jyh cipher: t starts at 165; each byte XORs with t then adds (i%256).
	t := byte(165)
	for i := 0; i < len(bytesArr); i++ {
		bytesArr[i] = (bytesArr[i] ^ t) + byte(i%256)
		t = bytesArr[i]
	}

	encoded := urlSafeBase64NoPad(bytesArr)
	return encoded + machineID
}

// urlSafeBase64NoPad encodes bytes using the URL-safe alphabet without padding,
// matching the hand-rolled encoder in cursorChecksum.js exactly.
func urlSafeBase64NoPad(b []byte) string {
	var out []byte
	for i := 0; i < len(b); i += 3 {
		a := b[i]
		var bb, cc byte
		if i+1 < len(b) {
			bb = b[i+1]
		}
		if i+2 < len(b) {
			cc = b[i+2]
		}
		out = append(out, urlSafeAlphabet[a>>2])
		out = append(out, urlSafeAlphabet[((a&3)<<4)|(bb>>4)])
		if i+1 < len(b) {
			out = append(out, urlSafeAlphabet[((bb&15)<<2)|(cc>>6)])
		}
		if i+2 < len(b) {
			out = append(out, urlSafeAlphabet[cc&63])
		}
	}
	return string(out)
}

// cleanCursorToken strips a "prefix::" segment from a Cursor token if present.
func cleanCursorToken(token string) string {
	for i := 0; i+1 < len(token); i++ {
		if token[i] == ':' && token[i+1] == ':' {
			return token[i+2:]
		}
	}
	return token
}

// buildCursorHeaders assembles the full Cursor API header set, including the
// checksum, derived client key, and UUID-v5 session id. Mirrors 9router's
// buildCursorHeaders. osName/arch describe the spoofed client platform.
func buildCursorHeaders(accessToken, machineID, osName, arch string, ghostMode bool) map[string]string {
	clean := cleanCursorToken(accessToken)
	if machineID == "" {
		machineID = generateHashed64Hex(clean, "machineId")
	}
	sessionID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(clean)).String()
	clientKey := generateHashed64Hex(clean, "")
	checksum := generateCursorChecksum(machineID)

	if osName == "" {
		osName = "linux"
	}
	if arch == "" {
		arch = "x64"
	}

	ghost := "false"
	if ghostMode {
		ghost = "true"
	}

	return map[string]string{
		"authorization":                bearer(clean),
		"connect-accept-encoding":      "gzip",
		"connect-protocol-version":     "1",
		"content-type":                 "application/connect+proto",
		"user-agent":                   "connect-es/1.6.1",
		"x-amzn-trace-id":              "Root=" + uuid.NewString(),
		"x-client-key":                 clientKey,
		"x-cursor-checksum":            checksum,
		"x-cursor-client-version":      "3.1.0",
		"x-cursor-client-type":         "ide",
		"x-cursor-client-os":           osName,
		"x-cursor-client-arch":         arch,
		"x-cursor-client-device-type":  "desktop",
		"x-cursor-config-version":      uuid.NewString(),
		"x-cursor-timezone":            "UTC",
		"x-ghost-mode":                 ghost,
		"x-request-id":                 uuid.NewString(),
		"x-session-id":                 sessionID,
	}
}
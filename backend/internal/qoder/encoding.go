package qoder

import "encoding/base64"

// WAF-bypass body encoding for Qoder's inference API.
//
// Algorithm:
//  1. Base64-encode the plaintext bytes (standard alphabet).
//  2. Rearrange: split into thirds, reorder as [tail][mid][head].
//  3. Substitute each character via a custom alphabet mapping.
//
// The encoded body must be sent with &Encode=1 appended to the URL so the
// server decodes in reverse. The obfuscation prevents Alibaba Cloud WAF from
// pattern-matching the plaintext request body.

const (
	stdAlphabet    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	customAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"
)

// s2c maps standard base64 characters to their custom-alphabet substitutes.
// Built once at package init. Padding '=' maps to '$'.
var s2c [128]byte

func init() {
	for i := range s2c {
		s2c[i] = byte(i) // identity for unmapped bytes
	}
	for i := 0; i < 64; i++ {
		s2c[stdAlphabet[i]] = customAlphabet[i]
	}
	s2c['='] = '$'
}

// EncodeBody applies Qoder's WAF-bypass encoding to plaintext. The returned
// bytes should be sent as-is in the HTTP body with Content-Type: application/json
// and &Encode=1 in the URL. The output is latin1-safe (all bytes < 128).
func EncodeBody(plaintext []byte) []byte {
	if len(plaintext) == 0 {
		return nil
	}

	// Step 1: standard base64 encode.
	encoded := []byte(base64.StdEncoding.EncodeToString(plaintext))
	n := len(encoded)
	if n == 0 {
		return nil
	}

	// Step 2: rearrange — split into thirds, reorder [tail][mid][head].
	// JS: std.slice(n-a) + std.slice(a, n-a) + std.slice(0, a)
	a := n / 3
	rearranged := make([]byte, 0, n)
	rearranged = append(rearranged, encoded[n-a:]...)  // tail: last a chars
	rearranged = append(rearranged, encoded[a:n-a]...) // middle: a to n-a
	rearranged = append(rearranged, encoded[:a]...)    // head: first a chars

	// Step 3: substitute each character through the mapping table.
	out := make([]byte, n)
	for i, c := range rearranged {
		if c < 128 {
			out[i] = s2c[c]
		} else {
			out[i] = c
		}
	}
	return out
}

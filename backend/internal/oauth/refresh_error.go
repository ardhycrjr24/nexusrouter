package oauth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RefreshError is returned by ProviderConfig.Refresh when the token endpoint
// rejects a refresh request. The Permanent flag distinguishes unrecoverable
// failures (revoked tokens, invalid grant) from transient ones (rate limits,
// server errors) so callers can decide whether to mark the account for
// re-authentication.
type RefreshError struct {
	Code       string // provider error code (e.g. "token_revoked", "invalid_grant")
	Message    string // human-readable provider message
	HTTPStatus int    // HTTP status from the token endpoint
	Permanent  bool   // true when re-authentication is required
}

func (e *RefreshError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("oauth refresh: %s: %s (status=%d permanent=%v)", e.Code, e.Message, e.HTTPStatus, e.Permanent)
	}
	return fmt.Sprintf("oauth refresh: %s (status=%d permanent=%v)", e.Code, e.HTTPStatus, e.Permanent)
}

// IsPermanentRefresh returns true when err is a RefreshError that requires
// re-authentication (the refresh token itself is dead or revoked).
func IsPermanentRefresh(err error) bool {
	if err == nil {
		return false
	}
	re, ok := err.(*RefreshError)
	return ok && re.Permanent
}

// classifyRefreshError inspects the token endpoint response body and HTTP
// status to decide whether a refresh failure is permanent. Mirrors the
// classification logic used by 9router (classifyOAuthRefreshError).
func classifyRefreshError(body []byte, status int) *RefreshError {
	// The "error" field is polymorphic: standard OAuth uses a string
	// ("error": "invalid_grant"), while OpenAI uses a nested object
	// ("error": { "message": "...", "code": "token_revoked" }).
	var raw struct {
		Error            json.RawMessage `json:"error"`
		ErrorDescription string          `json:"error_description"`
	}
	_ = json.Unmarshal(body, &raw)

	var code, msg string
	msg = raw.ErrorDescription

	if len(raw.Error) > 0 {
		// Try string first.
		var s string
		if json.Unmarshal(raw.Error, &s) == nil {
			code = s
		} else {
			// Try object.
			var obj struct {
				Message string `json:"message"`
				Code    string `json:"code"`
				Type    string `json:"type"`
			}
			if json.Unmarshal(raw.Error, &obj) == nil {
				if obj.Code != "" {
					code = obj.Code
				} else if obj.Type != "" {
					code = obj.Type
				}
				if obj.Message != "" {
					msg = obj.Message
				}
			}
		}
	}

	// permanentCodes are OAuth error codes that indicate the refresh token
	// itself is invalid and cannot be recovered.
	permanentCodes := map[string]bool{
		"token_revoked":       true,
		"token_invalidated":   true,
		"invalid_grant":       true,
		"invalid_token":       true,
		"unauthorized_client": true,
		"access_denied":       true,
	}

	permanent := permanentCodes[strings.ToLower(code)]

	// 401/403 with a refresh error is almost always permanent.
	if !permanent && (status == 401 || status == 403) {
		permanent = true
	}

	// Transient: rate limiting or server errors.
	if status == 429 || status >= 500 {
		permanent = false
	}

	return &RefreshError{
		Code:       code,
		Message:    msg,
		HTTPStatus: status,
		Permanent:  permanent,
	}
}

package gateway

import (
	"log/slog"
	"strings"
)

// sanitizeError converts an internal error into a safe message for API responses.
// It logs the full error server-side and returns a generic message to the client.
//
// This prevents leakage of:
// - Filesystem paths
// - Database internals
// - Upstream provider error details
// - Stack traces
func sanitizeError(log *slog.Logger, err error, context string) string {
	if err == nil {
		return "an internal error occurred"
	}

	// Log the full error server-side for debugging
	if log != nil {
		log.Error(context, "error", err)
	}

	msg := err.Error()

	// Check for known safe error patterns that can be exposed
	safePatterns := []struct {
		prefix  string
		message string
	}{
		{"invalid password", "invalid password"},
		{"invalid API key", "invalid API key"},
		{"missing API key", "missing API key"},
		{"dashboard session required", "dashboard session required"},
		{"dashboard is restricted to loopback access", "dashboard is restricted to loopback access"},
		{"rate limit exceeded", "rate limit exceeded"},
		{"URL blocked", "request blocked by security policy"},
		{"invalid JSON", "invalid request format"},
		{"missing required field", "missing required field"},
		{"invalid provider", "invalid provider"},
		{"invalid model", "invalid model"},
		{"provider not found", "provider not found"},
		{"account not found", "account not found"},
		{"key not found", "key not found"},
		{"chain not found", "chain not found"},
		{"budget not found", "budget not found"},
		{"pool not found", "pool not found"},
		{"skill not found", "skill not found"},
		{"alias not found", "alias not found"},
		{"quota exceeded", "quota exceeded"},
		{"rate limited", "rate limited by upstream provider"},
		{"unauthorized", "unauthorized"},
		{"forbidden", "forbidden"},
	}

	for _, p := range safePatterns {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(p.prefix)) {
			return p.message
		}
	}

	// For validation errors, try to extract a safe message
	if strings.Contains(msg, "validation failed") {
		return "validation failed: please check your input"
	}

	if strings.Contains(msg, "already exists") {
		return "resource already exists"
	}

	if strings.Contains(msg, "cannot be empty") {
		return "required field cannot be empty"
	}

	// Default: return generic message
	return "an internal error occurred"
}

// sanitizeValidationError converts validation errors to safe messages.
func sanitizeValidationError(err error) string {
	if err == nil {
		return "validation failed"
	}

	msg := err.Error()

	// Common validation patterns
	if strings.Contains(msg, "required") {
		return "missing required field"
	}
	if strings.Contains(msg, "invalid") {
		return "invalid field value"
	}
	if strings.Contains(msg, "too long") {
		return "field value too long"
	}
	if strings.Contains(msg, "too short") {
		return "field value too short"
	}

	return "validation failed"
}

// sanitizeUpstreamError converts upstream provider errors to safe messages.
func sanitizeUpstreamError(err error) string {
	if err == nil {
		return "upstream provider error"
	}

	msg := err.Error()

	// Rate limit errors
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") {
		return "upstream provider rate limit exceeded"
	}

	// Auth errors
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") {
		return "upstream provider authentication failed"
	}

	// Quota errors
	if strings.Contains(msg, "402") || strings.Contains(msg, "quota") {
		return "upstream provider quota exceeded"
	}

	// Server errors
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") {
		return "upstream provider is experiencing issues"
	}

	return "upstream provider error"
}

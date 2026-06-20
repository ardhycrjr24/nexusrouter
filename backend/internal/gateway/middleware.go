package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/identity"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// ctxKey is an unexported context key type to avoid collisions.
type ctxKey int

const apiKeyCtxKey ctxKey = iota

// authedKey returns the authenticated API key record from the request context.
func authedKey(ctx context.Context) (store.APIKey, bool) {
	k, ok := ctx.Value(apiKeyCtxKey).(store.APIKey)
	return k, ok
}

// authMiddleware authenticates the inbound API key from the Authorization
// header (Bearer) or the x-api-key header, attaching the key record to context.
// It rejects unauthenticated requests with 401.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		key, err := s.identity.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, identity.ErrUnauthorized) {
				s.consoleLog.Logf("WARN", "auth rejected: invalid key for %s %s", r.Method, r.URL.Path)
				writeError(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			s.log.Error("auth lookup failed", "err", err)
			s.consoleLog.Logf("ERROR", "auth lookup failed: %v", err)
			writeError(w, http.StatusInternalServerError, "authentication error")
			return
		}
		s.consoleLog.Logf("DEBUG", "auth ok: key=%s (%s) · %s %s", key.Name, key.ID, r.Method, r.URL.Path)

		ctx := context.WithValue(r.Context(), apiKeyCtxKey, key)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractToken pulls the API key from standard header locations.
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
		return strings.TrimSpace(auth)
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}

// loopbackOnly rejects non-loopback clients when bind-loopback-only is set. This
// guards the dashboard/admin surface in local single-user mode.
func (s *Server) loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Security.BindLoopbackOnly {
			next.ServeHTTP(w, r)
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeError(w, http.StatusForbidden, "dashboard is restricted to loopback access")
			return
		}
		next.ServeHTTP(w, r)
	})
}
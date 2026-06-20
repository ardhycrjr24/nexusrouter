package gateway

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// sessionCookie is the name of the dashboard session cookie.
const sessionCookie = "kr_session"

// mountAuth registers unauthenticated auth endpoints and the session-protected
// dashboard status endpoint.
// Note: login is registered separately in server.go with rate limiting.
func (s *Server) mountAuth(r chi.Router) {
	r.Post("/logout", s.handleLogout)
	// Status reports onboarding/default-password state so the UI can decide
	// whether to show the onboarding flow. Safe to expose unauthenticated: it
	// reveals only booleans, never secrets.
	r.Get("/status", s.handleAuthStatus)
}

// mountAuthenticated registers session-protected auth actions.
func (s *Server) mountAuthenticatedAuth(r chi.Router) {
	r.Post("/password", s.handleChangePassword)
	r.Post("/onboarding/complete", s.handleCompleteOnboarding)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	ok, err := s.auth.VerifyPassword(r.Context(), body.Password)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	token, err := s.auth.IssueSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                  true,
		"using_default":       s.auth.UsingDefaultPassword(r.Context()),
		"onboarding_complete": s.auth.OnboardingComplete(r.Context()),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	authed := false
	if c, err := r.Cookie(sessionCookie); err == nil {
		authed = s.auth.VerifySession(c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       authed,
		"using_default":       s.auth.UsingDefaultPassword(r.Context()),
		"onboarding_complete": s.auth.OnboardingComplete(r.Context()),
	})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.auth.SetPassword(r.Context(), body.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, sanitizeError(s.log, err, "password change failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.CompleteOnboarding(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "onboarding completion failed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// setSessionCookie writes the session cookie with the configured lifetime.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(s.auth.TTL()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// sessionMiddleware protects the admin API: it requires a valid session cookie.
// It runs after loopbackOnly, so local access still needs an authenticated
// dashboard session — credentials and routing config are never exposed to an
// unauthenticated caller, even on loopback.
func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.auth.VerifySession(c.Value) {
			writeError(w, http.StatusUnauthorized, "dashboard session required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
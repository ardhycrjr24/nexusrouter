package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/oauth"
)

var fixedOAuthCallbacks = struct {
	sync.Mutex
	servers map[string]*http.Server
}{servers: map[string]*http.Server{}}

func (s *Server) ensureFixedOAuthCallbackServer(cfg oauth.ProviderConfig) error {
	if cfg.FixedLoopbackPort <= 0 {
		return nil
	}

	fixedOAuthCallbacks.Lock()
	if fixedOAuthCallbacks.servers[cfg.Provider] != nil {
		fixedOAuthCallbacks.Unlock()
		return nil
	}
	fixedOAuthCallbacks.Unlock()

	provider := cfg.Provider
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.oauthFixedLoopbackCallback(provider, w, r)
		}),
	}
	listeners, err := listenFixedOAuthLoopbacks(cfg)
	if err != nil {
		return err
	}

	fixedOAuthCallbacks.Lock()
	if fixedOAuthCallbacks.servers[provider] != nil {
		fixedOAuthCallbacks.Unlock()
		for _, ln := range listeners {
			_ = ln.Close()
		}
		return nil
	}
	fixedOAuthCallbacks.servers[provider] = srv
	fixedOAuthCallbacks.Unlock()

	for _, ln := range listeners {
		go func(ln net.Listener) {
			if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
				s.log.Warn("oauth fixed callback server stopped", "provider", provider, "addr", ln.Addr().String(), "error", err)
			}
			fixedOAuthCallbacks.Lock()
			if fixedOAuthCallbacks.servers[provider] == srv {
				delete(fixedOAuthCallbacks.servers, provider)
			}
			fixedOAuthCallbacks.Unlock()
		}(ln)
	}

	time.AfterFunc(5*time.Minute, func() {
		stopFixedOAuthCallbackServer(provider)
	})
	return nil
}

func listenFixedOAuthLoopbacks(cfg oauth.ProviderConfig) ([]net.Listener, error) {
	port := strconv.Itoa(cfg.FixedLoopbackPort)
	hosts := []string{cfg.LoopbackHost}
	if cfg.LoopbackHost == "" {
		hosts = []string{"127.0.0.1"}
	} else if cfg.LoopbackHost == "localhost" {
		hosts = []string{"127.0.0.1", "::1"}
	}

	var (
		listeners []net.Listener
		errs      []error
	)
	for _, host := range hosts {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		listeners = append(listeners, ln)
	}
	if len(listeners) > 0 {
		return listeners, nil
	}
	return nil, fmt.Errorf("OAuth callback port %d is unavailable for %s: %v", cfg.FixedLoopbackPort, cfg.Provider, errs)
}

func stopFixedOAuthCallbackServer(provider string) {
	fixedOAuthCallbacks.Lock()
	srv := fixedOAuthCallbacks.servers[provider]
	delete(fixedOAuthCallbacks.servers, provider)
	fixedOAuthCallbacks.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func (s *Server) oauthFixedLoopbackCallback(provider string, w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/callback" && r.URL.Path != "/auth/callback" {
		http.NotFound(w, r)
		return
	}
	defer func() {
		go stopFixedOAuthCallbackServer(provider)
	}()

	status := "success"
	message := ""
	if err := s.completeOAuthCallback(r, provider); err != nil {
		status = "error"
		message = err.Error()
		s.log.Warn("oauth fixed callback failed", "provider", provider, "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderOAuthPopupResult(provider, status, message)))
}

func (s *Server) completeOAuthCallback(r *http.Request, providerHint string) error {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		if desc := r.URL.Query().Get("error_description"); desc != "" {
			return fmt.Errorf("%s: %s", errParam, desc)
		}
		return fmt.Errorf("%s", errParam)
	}
	if code == "" || state == "" {
		return fmt.Errorf("missing code or state parameter")
	}

	sess, ok := s.oauthSessions.Get(state)
	if !ok {
		return fmt.Errorf("session expired or invalid; please restart the sign-in flow")
	}

	provider := providerHint
	if provider == "" {
		provider = r.URL.Query().Get("provider")
	}
	if provider == "" {
		provider = sess.Provider
	}
	if sess.Provider != provider {
		return fmt.Errorf("provider mismatch")
	}

	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	var tokens *oauth.Tokens
	var err error
	if provider == "cline" {
		tokens, err = cfg.ExchangeClineCode(r.Context(), code, sess.RedirectURI)
	} else {
		tokens, err = cfg.ExchangeCode(r.Context(), code, sess.RedirectURI, sess.Verifier)
	}
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	s.oauthSessions.Delete(state)

	if _, err := s.persistOAuthAccount(r, provider, "", tokens); err != nil {
		return fmt.Errorf("failed to save account: %w", err)
	}
	return nil
}

func renderOAuthPopupResult(provider, status, message string) string {
	ok := status == "success"
	color := "#22c55e"
	icon := "&#10003;"
	title := "Authentication Successful"
	if !ok {
		color = "#ef4444"
		icon = "&#10007;"
		title = "Authentication Failed"
	}
	if message == "" && ok {
		message = "You can close this window."
	}
	payload, _ := json.Marshal(map[string]string{
		"type":     "oauth-callback",
		"status":   status,
		"provider": provider,
		"message":  message,
	})
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;background:#f5f5f5}.c{text-align:center;padding:2rem;background:#fff;border-radius:8px;box-shadow:0 2px 10px rgba(0,0,0,.1);max-width:420px}.i{color:%s;font-size:3rem}h1{margin:1rem 0}.m{color:#666;line-height:1.45}</style>
</head><body><div class="c"><div class="i">%s</div><h1>%s</h1><p class="m">%s</p></div>
<script>
const payload = %s;
try { if (window.opener) window.opener.postMessage(payload, "*"); } catch (_) {}
setTimeout(() => { try { window.close(); } catch (_) {} }, 900);
</script>
</body></html>`, html.EscapeString(title), color, icon, html.EscapeString(title), html.EscapeString(message), string(payload))
}

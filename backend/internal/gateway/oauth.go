package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/connectors"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/httputil"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/oauth"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/vault"
)

// errVaultUnconfigured is returned when an OAuth flow needs the vault but it is
// not wired (should never happen in normal operation).
var errVaultUnconfigured = errors.New("vault not configured")

// mountOAuth registers the OAuth connection endpoints. They are part of the
// dashboard admin API (loopback + session guarded), since starting a flow and
// persisting tokens are privileged operations.
func (s *Server) mountOAuth(r chi.Router) {
	r.Get("/oauth/providers", s.oauthListProviders)
	r.Post("/oauth/{provider}/authorize", s.oauthAuthorize)
	r.Post("/oauth/{provider}/exchange", s.oauthExchange)
	r.Post("/oauth/{provider}/device-code", s.oauthDeviceCode)
	r.Post("/oauth/{provider}/device-code-submit", s.oauthDeviceCodeSubmit)
	r.Post("/oauth/{provider}/poll", s.oauthPoll)
}

// oauthListProviders reports which catalog providers support an OAuth flow.
func (s *Server) oauthListProviders(w http.ResponseWriter, _ *http.Request) {
	out := make([]map[string]any, 0)
	for _, id := range oauth.SupportedProviders() {
		cfg, _ := oauth.ConfigFor(id)
		spec, _ := connectors.SpecByID(id)
		out = append(out, map[string]any{
			"provider":      id,
			"display_name":  spec.DisplayName,
			"flow":          cfg.Flow,
			"icon":          "/providers/" + id + ".png",
			"color":         spec.Color,
			"callback_path": cfg.CallbackPath,
			"fixed_port":    cfg.FixedLoopbackPort,
			"loopback_host": cfg.LoopbackHost,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

// oauthAuthorize starts an authorization-code(+PKCE) flow. It returns the
// provider authorize URL the dashboard should open, and stores the PKCE
// verifier + state server-side keyed by state.
func (s *Server) oauthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no OAuth config for provider: "+provider)
		return
	}
	if cfg.Flow == oauth.FlowDeviceCode {
		writeError(w, http.StatusBadRequest, "provider uses the device-code flow; call /device-code")
		return
	}

	var body struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.RedirectURI == "" {
		writeError(w, http.StatusBadRequest, "redirect_uri is required")
		return
	}
	redirectURI := cfg.ResolveRedirectURI(body.RedirectURI)

	// SSRF Protection: Validate redirect_uri to prevent open redirect attacks.
	// OAuth callbacks legitimately target localhost/loopback, so we use a
	// dedicated validator that permits those while still blocking private
	// networks, cloud metadata, and other dangerous destinations.
	if err := httputil.ValidateOAuthRedirectURI(redirectURI); err != nil {
		s.log.Warn("blocked suspicious redirect_uri", "uri", redirectURI, "error", err)
		writeError(w, http.StatusBadRequest, "invalid redirect_uri: URL blocked by security policy")
		return
	}
	if err := s.ensureFixedOAuthCallbackServer(cfg); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	pkce, err := oauth.GeneratePKCE(cfg.PKCEVerifierBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "PKCE generation failed"))
		return
	}

	challenge := ""
	if cfg.Flow == oauth.FlowAuthCodePKCE {
		challenge = pkce.Challenge
	}
	authURL := cfg.AuthURL(redirectURI, pkce.State, challenge)

	s.oauthSessions.Put(pkce.State, &oauth.Session{
		Provider:    provider,
		Flow:        cfg.Flow,
		State:       pkce.State,
		Verifier:    pkce.Verifier,
		RedirectURI: redirectURI,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"authorize_url": authURL,
		"state":         pkce.State,
		"redirect_uri":  redirectURI,
	})
}

// oauthExchange completes an authorization-code flow: it exchanges the pasted
// code for tokens and persists them as an OAuth account.
func (s *Server) oauthExchange(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no OAuth config for provider: "+provider)
		return
	}

	var body struct {
		Code  string `json:"code"`
		State string `json:"state"`
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Code == "" || body.State == "" {
		writeError(w, http.StatusBadRequest, "code and state are required")
		return
	}

	sess, ok := s.oauthSessions.Get(body.State)
	if !ok || sess.Provider != provider {
		writeError(w, http.StatusBadRequest, "OAuth session not found or expired; restart the flow")
		return
	}

	var tokens *oauth.Tokens
	var err error
	if provider == "cline" {
		// Cline uses a non-standard exchange (base64-embedded tokens or
		// JSON body with client_type=extension).
		tokens, err = cfg.ExchangeClineCode(r.Context(), body.Code, sess.RedirectURI)
	} else {
		tokens, err = cfg.ExchangeCode(r.Context(), body.Code, sess.RedirectURI, sess.Verifier)
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.oauthSessions.Delete(body.State)

	id, perr := s.persistOAuthAccount(r, provider, body.Label, tokens)
	if perr != nil {
		writeError(w, http.StatusInternalServerError, perr.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "provider": provider, "email": tokens.Email})
}

// oauthCallback handles the GET redirect from OAuth providers after the user
// authorizes. It exchanges the code for tokens and writes a self-contained HTML
// page that postMessages the opener tab and closes the popup. The HTML response
// is rendered inline (no SPA dependency) so the flow also works on minimal
// deployments where the dashboard assets are not bundled alongside the binary.
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	provider := r.URL.Query().Get("provider")

	writeResult := func(status, msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(renderOAuthPopupResult(provider, status, msg)))
	}

	if state == "" {
		writeResult("error", "missing code or state parameter")
		return
	}

	sess, ok := s.oauthSessions.Get(state)
	if !ok {
		writeResult("error", "session expired or invalid; please restart the sign-in flow")
		return
	}

	// If provider was not in the query string, infer it from the session.
	if provider == "" {
		provider = sess.Provider
	} else if sess.Provider != provider {
		writeResult("error", "provider mismatch")
		return
	}

	if err := s.completeOAuthCallback(r, provider); err != nil {
		s.log.Warn("oauth callback failed", "provider", provider, "error", err)
		writeResult("error", err.Error())
		return
	}

	s.log.Info("oauth callback success", "provider", provider)
	writeResult("success", "")
}

// oauthDeviceCode starts a device-authorization flow and returns the user code
// and verification URL for the dashboard to display.
func (s *Server) oauthDeviceCode(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no OAuth config for provider: "+provider)
		return
	}
	if cfg.Flow != oauth.FlowDeviceCode {
		writeError(w, http.StatusBadRequest, "provider does not use the device-code flow")
		return
	}

	// Some device-code providers (Qwen) require PKCE on the device-code
	// request and the subsequent token poll.
	var challenge, verifier string
	if cfg.DeviceCodePKCE {
		pkce, perr := oauth.GeneratePKCE(cfg.PKCEVerifierBytes)
		if perr != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate PKCE: "+perr.Error())
			return
		}
		challenge = pkce.Challenge
		verifier = pkce.Verifier
	}

	// ClientDeviceCode providers (Qwen) sit behind a WAF that uses TLS
	// fingerprinting to block non-browser HTTP clients.  The Go backend
	// cannot reach their device-code endpoint at all, so we delegate the
	// HTTP call to the browser (frontend) which has a legitimate browser
	// TLS fingerprint.
	//
	// Step 1: return PKCE challenge + endpoint metadata so the frontend
	//         can make the device-code request itself.
	// Step 2 (device-code-submit): the frontend sends back the device-code
	//         response it received; we store the session for polling.
	if cfg.ClientDeviceCode {
		nonce := uuid.NewString()
		s.oauthSessions.Put(nonce, &oauth.Session{
			Provider: provider,
			Flow:     cfg.Flow,
			Verifier: verifier,
			Interval: 5,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"_client_device_code": true,
			"_pkce_challenge":     challenge,
			"_pkce_nonce":         nonce,
			"_device_code_url":    cfg.DeviceCodeURL,
			"_client_id":          cfg.ClientID,
			"_scopes":             cfg.Scopes,
			"_pkce_method":        "S256",
		})
		return
	}

	dc, err := cfg.RequestDeviceCode(r.Context(), challenge)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Key the session by the device code so the poll step can recover it.
	s.oauthSessions.Put(dc.DeviceCode, &oauth.Session{
		Provider:   provider,
		Flow:       cfg.Flow,
		DeviceCode: dc.DeviceCode,
		Verifier:   verifier,
		Interval:   dc.Interval,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               dc.DeviceCode,
		"user_code":                 dc.UserCode,
		"verification_uri":          dc.VerificationURI,
		"verification_uri_complete": dc.VerificationURIComplete,
		"expires_in":                dc.ExpiresIn,
		"interval":                  dc.Interval,
	})
}

// oauthDeviceCodeSubmit is step 2 of the ClientDeviceCode flow. The frontend
// has made the device-code HTTP request in the browser and now submits the
// response so the backend can store the session for polling.
func (s *Server) oauthDeviceCodeSubmit(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no OAuth config for provider: "+provider)
		return
	}

	var body struct {
		Nonce                   string `json:"nonce"`
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Nonce == "" || body.DeviceCode == "" {
		writeError(w, http.StatusBadRequest, "nonce and device_code are required")
		return
	}

	// Look up the pending session created in step 1.
	sess, ok := s.oauthSessions.Get(body.Nonce)
	if !ok || sess.Provider != provider {
		writeError(w, http.StatusBadRequest, "PKCE session not found or expired; restart the flow")
		return
	}
	s.oauthSessions.Delete(body.Nonce)

	if body.Interval <= 0 {
		body.Interval = 5
	}

	// Re-key the session by the device code for polling.
	s.oauthSessions.Put(body.DeviceCode, &oauth.Session{
		Provider:   provider,
		Flow:       cfg.Flow,
		DeviceCode: body.DeviceCode,
		Verifier:   sess.Verifier,
		Interval:   body.Interval,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               body.DeviceCode,
		"user_code":                 body.UserCode,
		"verification_uri":          body.VerificationURI,
		"verification_uri_complete": body.VerificationURIComplete,
		"expires_in":                body.ExpiresIn,
		"interval":                  body.Interval,
	})
}

// oauthPoll performs one device-code poll. On success it persists the account;
// otherwise it reports pending/slow_down so the dashboard keeps polling.
func (s *Server) oauthPoll(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	cfg, ok := oauth.ConfigFor(provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no OAuth config for provider: "+provider)
		return
	}

	var body struct {
		DeviceCode string `json:"device_code"`
		Label      string `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DeviceCode == "" {
		writeError(w, http.StatusBadRequest, "device_code is required")
		return
	}

	sess, ok := s.oauthSessions.Get(body.DeviceCode)
	if !ok || sess.Provider != provider {
		writeError(w, http.StatusBadRequest, "OAuth session not found or expired; restart the flow")
		return
	}

	result := cfg.PollDeviceToken(r.Context(), body.DeviceCode, sess.Verifier)
	if result.Err != nil {
		s.oauthSessions.Delete(body.DeviceCode)
		writeError(w, http.StatusBadGateway, result.Err.Error())
		return
	}
	if result.Pending {
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending", "slow_down": result.SlowDown})
		return
	}

	s.oauthSessions.Delete(body.DeviceCode)
	id, perr := s.persistOAuthAccount(r, provider, body.Label, result.Tokens)
	if perr != nil {
		writeError(w, http.StatusInternalServerError, perr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "id": id, "provider": provider})
}

// persistOAuthAccount seals OAuth tokens into a new account record.
func (s *Server) persistOAuthAccount(r *http.Request, provider, label string, tokens *oauth.Tokens) (string, error) {
	if s.vault == nil {
		return "", errVaultUnconfigured
	}
	now := time.Now()
	acc := store.Account{
		ID:        uuid.NewString(),
		TenantID:  adminTenant,
		Provider:  provider,
		Label:     defaultStr(label, oauthLabel(provider, tokens)),
		AuthKind:  store.AuthOAuth,
		Priority:  100,
		CreatedAt: now,
		UpdatedAt: now,
	}

	var expiresAt *time.Time
	if tokens.ExpiresIn > 0 {
		t := now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	meta := map[string]string{}
	for k, v := range tokens.Extra {
		meta[k] = v
	}
	if tokens.Email != "" {
		meta["email"] = tokens.Email
	}

	if err := s.vault.Seal(&acc, vault.NewSecret{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		Metadata:     meta,
	}); err != nil {
		return "", err
	}

	// Validate the token against the upstream before persisting.
	if verr := s.validateAccountCredentials(r.Context(), acc); verr != nil {
		return "", fmt.Errorf("token validation failed: %w", verr)
	}

	if err := s.accounts.Create(r.Context(), acc); err != nil {
		return "", err
	}

	// Clear stale cooldowns on other accounts for the same provider/tenant.
	// When a user reconnects a provider, any previously-stuck accounts
	// should not block future requests.
	s.clearStaleProviderCooldowns(r.Context(), adminTenant, provider)

	return acc.ID, nil
}

// clearStaleProviderCooldowns removes expired cooldowns on all accounts for a
// provider within a tenant. Called after a successful reconnect so the dispatch
// layer can immediately use any previously-stuck accounts.
func (s *Server) clearStaleProviderCooldowns(ctx context.Context, tenantID, provider string) {
	if s.accounts == nil {
		return
	}
	if err := s.accounts.ClearProviderCooldowns(ctx, tenantID, provider); err != nil {
		s.log.Warn("failed to clear stale provider cooldowns", "provider", provider, "err", err)
	}
}

// oauthLabel derives a human label for an OAuth account.
func oauthLabel(provider string, tokens *oauth.Tokens) string {
	if tokens.DisplayName != "" {
		return tokens.DisplayName
	}
	if tokens.Email != "" {
		return tokens.Email
	}
	return provider + " (oauth)"
}

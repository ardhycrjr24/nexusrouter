package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/oauth"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// mountKiro registers the Kiro-specific connect endpoints. Kiro authenticates
// through AWS SSO OIDC (Builder ID / IAM Identity Center device flows) or by
// importing a refresh token from the Kiro IDE, mirroring 9router's flow.
func (s *Server) mountKiro(r chi.Router) {
	// Mounted under a dedicated /kiro prefix (not /oauth/kiro) so the static
	// segment never collides with the /oauth/{provider} param routes in the
	// chi radix tree, which would otherwise 404 these endpoints.
	r.Post("/kiro/device-start", s.kiroDeviceStart)
	r.Post("/kiro/device-poll", s.kiroDevicePoll)
	r.Post("/kiro/import", s.kiroImport)
	r.Get("/kiro/health", s.kiroHealth)
}

// kiroDeviceStart registers an SSO OIDC client and begins device authorization
// for either Builder ID (no start URL) or IAM Identity Center (custom start URL
// + region). It returns the user code + verification URL and a device_code the
// dashboard polls with.
func (s *Server) kiroDeviceStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Method   string `json:"method"`    // builder-id | idc
		StartURL string `json:"start_url"` // required for idc
		Region   string `json:"region"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	region := body.Region
	if region == "" {
		region = "us-east-1"
	}
	startURL := body.StartURL
	method := body.Method
	if method == "" {
		method = "builder-id"
	}
	if method == "idc" && startURL == "" {
		writeError(w, http.StatusBadRequest, "start_url is required for IAM Identity Center")
		return
	}

	client, err := oauth.KiroRegisterClient(r.Context(), region, startURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	device, err := client.StartDeviceAuth(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Persist the client credentials in the session keyed by device code so the
	// poll step can complete and the account can store them for refresh.
	s.oauthSessions.Put(device.DeviceCode, &oauth.Session{
		Provider:         "kiro",
		Flow:             oauth.FlowDeviceCode,
		DeviceCode:       device.DeviceCode,
		Interval:         device.Interval,
		KiroClientID:     client.ClientID,
		KiroClientSecret: client.ClientSecret,
		KiroRegion:       client.Region,
		KiroStartURL:     client.StartURL,
		KiroAuthMethod:   method,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"device_code":               device.DeviceCode,
		"user_code":                 device.UserCode,
		"verification_uri":          device.VerificationURI,
		"verification_uri_complete": device.VerificationURIComplete,
		"expires_in":                device.ExpiresIn,
		"interval":                  device.Interval,
	})
}

// kiroDevicePoll performs one poll of the SSO OIDC token endpoint. On success it
// persists a Kiro account with the tokens and the client credentials needed for
// later refresh.
func (s *Server) kiroDevicePoll(w http.ResponseWriter, r *http.Request) {
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
	if !ok || sess.Provider != "kiro" {
		writeError(w, http.StatusBadRequest, "Kiro session not found or expired; restart the flow")
		return
	}

	client := &oauth.KiroClient{
		ClientID:     sess.KiroClientID,
		ClientSecret: sess.KiroClientSecret,
		Region:       sess.KiroRegion,
		StartURL:     sess.KiroStartURL,
	}
	result := client.PollToken(r.Context(), body.DeviceCode)
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

	// Attach the SSO client credentials so the TokenManager can refresh later.
	if result.Tokens.Extra == nil {
		result.Tokens.Extra = map[string]string{}
	}
	result.Tokens.Extra["kiro_auth_method"] = sess.KiroAuthMethod
	result.Tokens.Extra["kiro_client_id"] = sess.KiroClientID
	result.Tokens.Extra["kiro_client_secret"] = sess.KiroClientSecret
	result.Tokens.Extra["kiro_region"] = sess.KiroRegion
	result.Tokens.Extra["kiro_start_url"] = sess.KiroStartURL

	label := defaultStr(body.Label, kiroLabel(sess.KiroAuthMethod))
	id, perr := s.persistOAuthAccount(r, "kiro", label, result.Tokens)
	if perr != nil {
		writeError(w, http.StatusInternalServerError, perr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "complete", "id": id, "provider": "kiro"})
}

// kiroImport validates and stores a refresh token exported from the Kiro IDE.
// Imported tokens refresh through the Kiro desktop social auth service, so no
// SSO client credentials are stored.
func (s *Server) kiroImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
		Label        string `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tokens, err := oauth.KiroValidateImportToken(r.Context(), body.RefreshToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if tokens.Extra == nil {
		tokens.Extra = map[string]string{}
	}
	tokens.Extra["kiro_auth_method"] = "imported"

	label := defaultStr(body.Label, "Kiro (imported)")
	id, perr := s.persistOAuthAccount(r, "kiro", label, tokens)
	if perr != nil {
		writeError(w, http.StatusInternalServerError, perr.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "provider": "kiro"})
}

// kiroHealth reports the connection status of all Kiro accounts. The dashboard
// uses this to show a "Reconnect" prompt when the SSO session has expired.
func (s *Server) kiroHealth(w http.ResponseWriter, r *http.Request) {
	tenantID := store.DefaultTenantID
	accs, err := s.accounts.ListByProvider(r.Context(), tenantID, "kiro")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type accountHealth struct {
		ID         string  `json:"id"`
		Label      string  `json:"label"`
		Status     string  `json:"status"` // connected | needs_reauth
		ExpiresAt  *string `json:"expires_at,omitempty"`
		AuthMethod string  `json:"auth_method"`
	}

	var results []accountHealth
	for _, acc := range accs {
		if acc.Disabled {
			continue
		}
		meta := map[string]string{}
		if acc.Metadata != "" {
			_ = json.Unmarshal([]byte(acc.Metadata), &meta)
		}

		h := accountHealth{
			ID:         acc.ID,
			Label:      acc.Label,
			Status:     "connected",
			AuthMethod: meta["kiro_auth_method"],
		}

		if acc.TokenExpiresAt != nil {
			t := acc.TokenExpiresAt.UTC().Format(time.RFC3339)
			h.ExpiresAt = &t
			if acc.TokenExpiresAt.Before(time.Now()) {
				if s.refresher != nil {
					if _, ferr := s.refresher.EnsureFresh(r.Context(), acc); ferr != nil {
						h.Status = "needs_reauth"
					}
				} else {
					h.Status = "needs_reauth"
				}
			}
		}
		results = append(results, h)
	}

	writeJSON(w, http.StatusOK, map[string]any{"accounts": results})
}

// kiroLabel produces a human label for a Kiro account by auth method.
func kiroLabel(method string) string {
	switch method {
	case "idc":
		return "Kiro (IAM Identity Center)"
	case "imported":
		return "Kiro (imported)"
	default:
		return "Kiro (Builder ID)"
	}
}

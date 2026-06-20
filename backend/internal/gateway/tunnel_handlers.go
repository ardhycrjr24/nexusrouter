package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/tunnel/cloudflare"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/tunnel/tailscale"
)

// adminTunnelStatus returns the combined tunnel + tailscale + download status.
func (s *Server) adminTunnelStatus(w http.ResponseWriter, r *http.Request) {
	tunnelStatus := s.cfManager.Status()
	tailscaleStatus := s.tsManager.Status()
	downloading, progress := cloudflare.GetDownloadStatus()

	writeJSON(w, http.StatusOK, map[string]any{
		"tunnel":    tunnelStatus,
		"tailscale": tailscaleStatus,
		"download": map[string]any{
			"downloading": downloading,
			"progress":    progress,
		},
	})
}

// adminTunnelEnable starts the Cloudflare quick tunnel.
func (s *Server) adminTunnelEnable(w http.ResponseWriter, r *http.Request) {
	result, err := s.cfManager.Enable(func(tunnelURL string) {
		// Update settings with tunnel URL.
		ctx := r.Context()
		current := s.loadAccessSettings(ctx)
		current.TunnelEnabled = true
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// DNS warmup delay (Cloudflare edge propagation).
	time.Sleep(8 * time.Second)

	writeJSON(w, http.StatusOK, result)
}

// adminTunnelDisable stops the Cloudflare tunnel.
func (s *Server) adminTunnelDisable(w http.ResponseWriter, r *http.Request) {
	s.cfManager.Disable(func() {
		ctx := r.Context()
		current := s.loadAccessSettings(ctx)
		current.TunnelEnabled = false
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// adminTailscaleCheck returns installation and state info for Tailscale.
func (s *Server) adminTailscaleCheck(w http.ResponseWriter, r *http.Request) {
	result := s.tsManager.Check("")
	writeJSON(w, http.StatusOK, result)
}

// adminTailscaleEnable starts the Tailscale funnel.
func (s *Server) adminTailscaleEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SudoPassword string `json:"sudoPassword"`
	}
	// Accept optional sudo password.
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}

	result, err := s.tsManager.Enable(body.SudoPassword, func(tunnelURL string) {
		ctx := r.Context()
		current := s.loadAccessSettings(ctx)
		current.Tailscale = true
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// adminTailscaleDisable stops the Tailscale funnel.
func (s *Server) adminTailscaleDisable(w http.ResponseWriter, r *http.Request) {
	s.tsManager.Disable(func() {
		ctx := r.Context()
		current := s.loadAccessSettings(ctx)
		current.Tailscale = false
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// adminTailscaleInstall handles Tailscale installation with SSE streaming
// progress events, matching 9router's tailscale-install endpoint.
func (s *Server) adminTailscaleInstall(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	var body struct {
		SudoPassword string `json:"sudoPassword"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sendEvent := func(event string, data any) {
		d, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, d)
		flusher.Flush()
	}

	onProgress := func(msg string) {
		sendEvent("progress", map[string]string{"message": msg})
	}

	// Validate sudo password before install for paths that require it
	// (Linux, macOS without brew). macOS+brew doesn't need sudo for install
	// but still needs it for TUN daemon later.
	needsSudoForInstall := !tailscale.HasBrew() || runtime.GOOS != "darwin"
	if needsSudoForInstall {
		if err := tailscale.ValidateSudoPassword(body.SudoPassword); err != nil {
			sendEvent("error", map[string]string{"error": fmt.Sprintf("sudo password validation failed: %s", err.Error())})
			return
		}
	}

	err := tailscale.InstallTailscale(s.dataDir, body.SudoPassword, onProgress)
	if err != nil {
		errMsg := err.Error()
		if contains(errMsg, "incorrect password") || contains(errMsg, "Sorry") {
			errMsg = "Wrong sudo password"
		}
		sendEvent("error", map[string]string{"error": errMsg})
		return
	}

	sendEvent("done", map[string]any{"success": true})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

func (s *Server) adminListAccountHealth(w http.ResponseWriter, r *http.Request) {
	if s.health == nil {
		writeError(w, http.StatusInternalServerError, "health repository not configured")
		return
	}
	rows, err := s.health.List(r.Context(), adminTenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, h := range rows {
		out = append(out, accountHealthJSON(h))
	}
	writeJSON(w, http.StatusOK, map[string]any{"health": out})
}

func (s *Server) adminRunHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.healthChecker == nil {
		writeError(w, http.StatusInternalServerError, "health checker not configured")
		return
	}
	ctx, cancel := contextWithTimeout(r, s.cfg.Health.Timeout*time.Duration(max(1, s.cfg.Health.MaxParallel)))
	defer cancel()
	s.healthChecker.CheckOnce(ctx, adminTenant)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func accountHealthJSON(h store.AccountHealth) map[string]any {
	row := map[string]any{
		"id":                    h.ID,
		"tenant_id":             h.TenantID,
		"account_id":            h.AccountID,
		"provider":              h.Provider,
		"model":                 h.Model,
		"status":                h.Status,
		"latency_ms":            h.LatencyMS,
		"consecutive_failures":  h.ConsecutiveFailures,
		"consecutive_successes": h.ConsecutiveSuccesses,
		"last_checked_at":       h.LastCheckedAt,
		"last_error":            h.LastError,
		"updated_at":            h.UpdatedAt,
	}
	if h.LastOKAt != nil {
		row["last_ok_at"] = *h.LastOKAt
	}
	return row
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 30 * time.Second
	}
	return context.WithTimeout(r.Context(), d)
}

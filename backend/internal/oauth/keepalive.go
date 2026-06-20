package oauth

import (
	"context"
	"log/slog"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

const (
	// DefaultKeepAliveInterval is how often the background refresher checks
	// for near-expiry OAuth tokens. 30 minutes keeps tokens warm within the
	// typical 1-hour access token lifetime used by AWS SSO OIDC.
	DefaultKeepAliveInterval = 30 * time.Minute
)

// KeepAlive runs a background loop that proactively refreshes near-expiry
// OAuth access tokens. It prevents request-time latency from just-in-time
// refresh and detects expired refresh tokens early so the dashboard can show
// a "Reconnect" prompt.
type KeepAlive struct {
	interval time.Duration
	tokenMgr *TokenManager
	accounts *store.AccountRepo
	tenantID string
	log      *slog.Logger
}

// NewKeepAlive builds a KeepAlive.
func NewKeepAlive(tm *TokenManager, accounts *store.AccountRepo, tenantID string, log *slog.Logger) *KeepAlive {
	return &KeepAlive{
		interval: DefaultKeepAliveInterval,
		tokenMgr: tm,
		accounts: accounts,
		tenantID: tenantID,
		log:      log,
	}
}

// SetInterval overrides the default check interval.
func (k *KeepAlive) SetInterval(d time.Duration) {
	if d > 0 {
		k.interval = d
	}
}

// Run starts the keepalive loop. It blocks until ctx is cancelled. Callers
// should launch it as a goroutine tied to the application context.
func (k *KeepAlive) Run(ctx context.Context) {
	k.log.Info("oauth keepalive started", "interval", k.interval)

	// Run once immediately on startup to catch stale tokens early.
	k.refreshAll(ctx)

	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			k.log.Info("oauth keepalive stopped")
			return
		case <-ticker.C:
			k.refreshAll(ctx)
		}
	}
}

// refreshAll lists all OAuth accounts for the tenant and refreshes those that
// are near expiry. Failures are logged but do not stop the loop.
func (k *KeepAlive) refreshAll(ctx context.Context) {
	accs, err := k.accounts.ListByTenant(ctx, k.tenantID)
	if err != nil {
		k.log.Error("oauth keepalive: list accounts", "err", err)
		return
	}

	var refreshed, skipped, failed, reconnect int
	for _, acc := range accs {
		if acc.AuthKind != store.AuthOAuth {
			continue
		}
		if acc.Disabled {
			continue
		}
		// Already flagged for reconnection; skip until the user re-authenticates.
		if acc.NeedsReconnect {
			reconnect++
			continue
		}
		// Only refresh tokens that are near expiry or expired.
		if acc.TokenExpiresAt != nil && time.Until(*acc.TokenExpiresAt) > refreshSkew {
			skipped++
			continue
		}

		_, err := k.tokenMgr.EnsureFresh(ctx, acc)
		if err != nil {
			failed++
			k.log.Warn("oauth keepalive: refresh failed",
				"account", acc.ID,
				"provider", acc.Provider,
				"err", err,
			)
			continue
		}
		refreshed++
		k.log.Debug("oauth keepalive: refreshed",
			"account", acc.ID,
			"provider", acc.Provider,
		)
	}

	if refreshed > 0 || failed > 0 || reconnect > 0 {
		k.log.Info("oauth keepalive pass complete",
			"refreshed", refreshed,
			"skipped", skipped,
			"failed", failed,
			"needs_reconnect", reconnect,
		)
	}
}

package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/vault"
)

// refreshSkew refreshes a token this long before its actual expiry, so an
// in-flight request never races a just-expired token.
const refreshSkew = 60 * time.Second

// TokenManager refreshes expiring OAuth access tokens just-in-time. It is
// consulted by the dispatcher before opening an account's credentials.
type TokenManager struct {
	vault    *vault.Vault
	accounts *store.AccountRepo
}

// NewTokenManager builds a TokenManager.
func NewTokenManager(v *vault.Vault, accounts *store.AccountRepo) *TokenManager {
	return &TokenManager{vault: v, accounts: accounts}
}

// EnsureFresh returns an account whose OAuth access token is valid, refreshing
// it in place (and persisting the new tokens) when it is expired or about to
// expire. Non-OAuth accounts and accounts without an expiry are returned
// unchanged. A refresh failure is returned so the dispatcher can skip the
// account and fall back.
func (m *TokenManager) EnsureFresh(ctx context.Context, acc store.Account) (store.Account, error) {
	if m == nil || m.vault == nil || m.accounts == nil {
		return acc, nil
	}
	if acc.AuthKind != store.AuthOAuth || acc.TokenExpiresAt == nil {
		return acc, nil
	}
	if time.Until(*acc.TokenExpiresAt) > refreshSkew {
		return acc, nil // still valid
	}

	refreshToken, err := m.vault.OpenRefreshToken(acc)
	if err != nil {
		return acc, fmt.Errorf("oauth: no refresh token for account %s: %w", acc.ID, err)
	}

	var tokens *Tokens
	if acc.Provider == "kiro" {
		// Kiro refreshes through AWS SSO OIDC (Builder ID / IDC, using the stored
		// client credentials) or the Kiro desktop social auth service (imported).
		tokens, err = refreshKiro(ctx, acc, refreshToken)
	} else {
		cfg, ok := ConfigFor(acc.Provider)
		if !ok {
			// No refresh config; let the dispatcher try the (possibly stale) token.
			return acc, nil
		}
		tokens, err = cfg.Refresh(ctx, refreshToken)
	}
	if err != nil {
		// Permanent refresh failures (token_revoked, invalid_grant, etc.) mean
		// the refresh token itself is dead. Mark the account so the dashboard
		// shows a "Reconnect Required" badge and the dispatcher skips it.
		if IsPermanentRefresh(err) {
			if setErr := m.accounts.SetNeedsReconnect(ctx, acc.ID, true); setErr != nil {
				return acc, fmt.Errorf("oauth: mark reconnect: %w (original: %w)", setErr, err)
			}
		}
		return acc, fmt.Errorf("oauth: refresh failed for account %s: %w", acc.ID, err)
	}

	var expiresAt *time.Time
	if tokens.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	// Seal the new tokens into the account. Passing nil Metadata preserves the
	// existing provider metadata.
	if err := m.vault.Seal(&acc, vault.NewSecret{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return acc, fmt.Errorf("oauth: seal refreshed token: %w", err)
	}
	acc.TokenExpiresAt = expiresAt

	if err := m.accounts.UpdateTokens(ctx, acc); err != nil {
		return acc, fmt.Errorf("oauth: persist refreshed token: %w", err)
	}
	return acc, nil
}

// ForceRefresh unconditionally refreshes an OAuth account's access token,
// bypassing the local expiry check. Used when the upstream API rejects the
// current token even though it hasn't reached its local expiry (tokens can be
// invalidated server-side before expiry).
func (m *TokenManager) ForceRefresh(ctx context.Context, acc store.Account) (store.Account, error) {
	if m == nil || m.vault == nil || m.accounts == nil {
		return acc, fmt.Errorf("oauth: token manager not configured")
	}
	if acc.AuthKind != store.AuthOAuth {
		return acc, fmt.Errorf("oauth: account %s is not OAuth", acc.ID)
	}

	refreshToken, err := m.vault.OpenRefreshToken(acc)
	if err != nil {
		return acc, fmt.Errorf("oauth: no refresh token for account %s: %w", acc.ID, err)
	}

	var tokens *Tokens
	if acc.Provider == "kiro" {
		tokens, err = refreshKiro(ctx, acc, refreshToken)
	} else {
		cfg, ok := ConfigFor(acc.Provider)
		if !ok {
			return acc, fmt.Errorf("oauth: no refresh config for provider %s", acc.Provider)
		}
		tokens, err = cfg.Refresh(ctx, refreshToken)
	}
	if err != nil {
		if IsPermanentRefresh(err) {
			if setErr := m.accounts.SetNeedsReconnect(ctx, acc.ID, true); setErr != nil {
				return acc, fmt.Errorf("oauth: mark reconnect: %w (original: %w)", setErr, err)
			}
		}
		return acc, fmt.Errorf("oauth: refresh failed for account %s: %w", acc.ID, err)
	}

	var expiresAt *time.Time
	if tokens.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	if err := m.vault.Seal(&acc, vault.NewSecret{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return acc, fmt.Errorf("oauth: seal refreshed token: %w", err)
	}
	acc.TokenExpiresAt = expiresAt

	if err := m.accounts.UpdateTokens(ctx, acc); err != nil {
		return acc, fmt.Errorf("oauth: persist refreshed token: %w", err)
	}
	return acc, nil
}

// refreshKiro renews a Kiro account's token. Builder ID / IDC accounts carry the
// SSO OIDC client credentials in their metadata; imported accounts refresh
// through the Kiro desktop social auth service.
func refreshKiro(ctx context.Context, acc store.Account, refreshToken string) (*Tokens, error) {
	meta := map[string]string{}
	if acc.Metadata != "" {
		_ = json.Unmarshal([]byte(acc.Metadata), &meta)
	}
	clientID := meta["kiro_client_id"]
	clientSecret := meta["kiro_client_secret"]
	if clientID != "" && clientSecret != "" {
		client := &KiroClient{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Region:       meta["kiro_region"],
			StartURL:     meta["kiro_start_url"],
		}
		return client.Refresh(ctx, refreshToken)
	}
	// Imported token: refresh via the social auth service.
	return KiroSocialRefresh(ctx, refreshToken)
}

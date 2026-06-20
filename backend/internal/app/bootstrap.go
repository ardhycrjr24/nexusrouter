package app

import (
	"context"
	"fmt"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/config"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/identity"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// Bootstrap creates an initial API key for local use and returns its plaintext.
// It is invoked by `nexusrouter -bootstrap` so a fresh install is immediately
// usable without a dashboard. The plaintext is shown once and never recoverable.
func Bootstrap(ctx context.Context, cfg config.Config, name string) (string, error) {
	dataDir, err := resolveDataDir(cfg)
	if err != nil {
		return "", err
	}

	db, err := store.Open(ctx, cfg.Database, dataDir)
	if err != nil {
		return "", err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return "", fmt.Errorf("bootstrap: migrate: %w", err)
	}
	if err := db.Tenants().EnsureDefault(ctx); err != nil {
		return "", fmt.Errorf("bootstrap: ensure tenant: %w", err)
	}

	if name == "" {
		name = "default"
	}
	issued, err := identity.New(db.APIKeys()).Create(ctx, store.DefaultTenantID, "", name)
	if err != nil {
		return "", fmt.Errorf("bootstrap: create key: %w", err)
	}
	return issued.Plaintext, nil
}
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TenantRepo persists tenants and projects.
type TenantRepo struct{ db *DB }

// Tenants returns the tenant repository.
func (db *DB) Tenants() *TenantRepo { return &TenantRepo{db: db} }

// EnsureDefault creates the implicit default tenant if it does not exist. Called
// once on startup so local single-user mode has a tenant to attach data to.
func (r *TenantRepo) EnsureDefault(ctx context.Context) error {
	return r.Upsert(ctx, Tenant{ID: DefaultTenantID, Name: "Default", CreatedAt: time.Now()})
}

// Upsert inserts a tenant or leaves an existing one untouched.
func (r *TenantRepo) Upsert(ctx context.Context, t Tenant) error {
	q := r.db.rebind(`INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)
		ON CONFLICT (id) DO NOTHING`)
	_, err := r.db.sql.ExecContext(ctx, q, t.ID, t.Name, formatTime(t.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: upsert tenant: %w", err)
	}
	return nil
}

// Get returns one tenant by id.
func (r *TenantRepo) Get(ctx context.Context, id string) (Tenant, error) {
	q := r.db.rebind(`SELECT id, name, created_at FROM tenants WHERE id = ?`)
	var (
		t       Tenant
		created string
	)
	err := r.db.sql.QueryRowContext(ctx, q, id).Scan(&t.ID, &t.Name, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("store: get tenant: %w", err)
	}
	t.CreatedAt = parseTime(created)
	return t, nil
}

// SettingsRepo is a simple key/value configuration store.
type SettingsRepo struct{ db *DB }

// Settings returns the settings repository.
func (db *DB) Settings() *SettingsRepo { return &SettingsRepo{db: db} }

// Get returns a setting value, or ErrNotFound if the key is absent.
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	q := r.db.rebind(`SELECT value FROM settings WHERE key = ?`)
	var v string
	err := r.db.sql.QueryRowContext(ctx, q, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: get setting: %w", err)
	}
	return v, nil
}

// Set writes (inserts or updates) a setting value.
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	q := r.db.rebind(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`)
	_, err := r.db.sql.ExecContext(ctx, q, key, value, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set setting: %w", err)
	}
	return nil
}

// AuditRepo appends and reads audit entries.
type AuditRepo struct{ db *DB }

// Audit returns the audit repository.
func (db *DB) Audit() *AuditRepo { return &AuditRepo{db: db} }

// Append writes one audit entry. Audit is append-only; there is no update/delete.
func (r *AuditRepo) Append(ctx context.Context, e AuditEntry) error {
	q := r.db.rebind(`INSERT INTO audit_log (id, tenant_id, actor, action, target, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.sql.ExecContext(ctx, q,
		e.ID, nullString(e.TenantID), e.Actor, e.Action, nullString(e.Target),
		e.Detail, formatTime(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: append audit: %w", err)
	}
	return nil
}

// List returns recent audit entries for a tenant, newest first, capped by limit.
func (r *AuditRepo) List(ctx context.Context, tenantID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := r.db.rebind(`SELECT id, tenant_id, actor, action, target, detail, created_at
		FROM audit_log WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ?`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list audit: %w", err)
	}
	defer rows.Close()

	var out []AuditEntry
	for rows.Next() {
		var (
			e         AuditEntry
			tenant    sql.NullString
			target    sql.NullString
			createdAt string
		)
		if err := rows.Scan(&e.ID, &tenant, &e.Actor, &e.Action, &target, &e.Detail, &createdAt); err != nil {
			return nil, err
		}
		e.TenantID = tenant.String
		e.Target = target.String
		e.CreatedAt = parseTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}
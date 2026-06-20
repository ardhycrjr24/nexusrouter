package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PlanRepo persists plan templates.
type PlanRepo struct{ db *DB }

// Plans returns the plan repository.
func (db *DB) Plans() *PlanRepo { return &PlanRepo{db: db} }

const planSelectCols = `id, tenant_id, name, description, limit_micros, limit_tokens, rpm_limit, tpm_limit, concurrency_limit, period, alert_pct, hard_cutoff, allowed_models, created_at, updated_at`

// Create inserts a new plan.
func (r *PlanRepo) Create(ctx context.Context, p Plan) error {
	return r.insert(ctx, r.db.sql, p)
}

// CreateOnTx inserts a plan within an existing transaction.
func (r *PlanRepo) CreateOnTx(ctx context.Context, tx *sql.Tx, p Plan) error {
	return r.insert(ctx, tx, p)
}

func (r *PlanRepo) insert(ctx context.Context, ex sqlExec, p Plan) error {
	q := r.db.rebind(`INSERT INTO plans
		(id, tenant_id, name, description, limit_micros, limit_tokens, rpm_limit, tpm_limit, concurrency_limit, period, alert_pct, hard_cutoff, allowed_models, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := ex.ExecContext(ctx, q,
		p.ID, p.TenantID, p.Name, p.Description, p.LimitMicros, p.LimitTokens, p.RPMLimit, p.TPMLimit, p.ConcurrencyLimit, p.Period,
		p.AlertPct, boolToInt(p.HardCutoff), p.AllowedModels, formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create plan: %w", err)
	}
	return nil
}

// Get returns a single plan by id.
func (r *PlanRepo) Get(ctx context.Context, id string) (Plan, error) {
	q := r.db.rebind(`SELECT ` + planSelectCols + ` FROM plans WHERE id = ?`)
	p, err := scanPlan(r.db.sql.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	return p, err
}

// List returns all plans for a tenant, newest first.
func (r *PlanRepo) List(ctx context.Context, tenantID string) ([]Plan, error) {
	q := r.db.rebind(`SELECT ` + planSelectCols + ` FROM plans WHERE tenant_id = ? ORDER BY created_at DESC`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("store: list plans: %w", err)
	}
	defer rows.Close()

	var out []Plan
	for rows.Next() {
		p, err := scanPlanRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update modifies an existing plan's mutable fields.
func (r *PlanRepo) Update(ctx context.Context, p Plan) error {
	q := r.db.rebind(`UPDATE plans SET name = ?, description = ?, limit_micros = ?, limit_tokens = ?, rpm_limit = ?, tpm_limit = ?, concurrency_limit = ?, period = ?, alert_pct = ?, hard_cutoff = ?, allowed_models = ?, updated_at = ? WHERE id = ?`)
	res, err := r.db.sql.ExecContext(ctx, q, p.Name, p.Description, p.LimitMicros, p.LimitTokens, p.RPMLimit, p.TPMLimit, p.ConcurrencyLimit, p.Period,
		p.AlertPct, boolToInt(p.HardCutoff), p.AllowedModels, formatTime(p.UpdatedAt), p.ID)
	if err != nil {
		return fmt.Errorf("store: update plan: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a plan. The caller should verify no API keys reference it.
func (r *PlanRepo) Delete(ctx context.Context, id string) error {
	q := r.db.rebind(`DELETE FROM plans WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, id)
	return err
}

// CountKeys returns the number of API keys assigned to a plan.
func (r *PlanRepo) CountKeys(ctx context.Context, planID string) (int, error) {
	q := r.db.rebind(`SELECT COUNT(*) FROM api_keys WHERE plan_id = ?`)
	var n int
	if err := r.db.sql.QueryRowContext(ctx, q, planID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count plan keys: %w", err)
	}
	return n, nil
}

func scanPlan(scan func(dest ...any) error) (Plan, error) {
	var (
		p       Plan
		hardCut int
		created string
		updated string
	)
	err := scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.LimitMicros, &p.LimitTokens,
		&p.RPMLimit, &p.TPMLimit, &p.ConcurrencyLimit, &p.Period, &p.AlertPct, &hardCut, &p.AllowedModels, &created, &updated)
	if err != nil {
		return Plan{}, err
	}
	p.HardCutoff = hardCut != 0
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return p, nil
}

func scanPlanRows(rows *sql.Rows) (Plan, error) {
	var (
		p       Plan
		hardCut int
		created string
		updated string
	)
	err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.LimitMicros, &p.LimitTokens,
		&p.RPMLimit, &p.TPMLimit, &p.ConcurrencyLimit, &p.Period, &p.AlertPct, &hardCut, &p.AllowedModels, &created, &updated)
	if err != nil {
		return Plan{}, fmt.Errorf("store: scan plan: %w", err)
	}
	p.HardCutoff = hardCut != 0
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return p, nil
}

// GetPlanAllowedModels returns the allowed models for a plan as a slice.
func GetPlanAllowedModels(p Plan) []string {
	if p.AllowedModels == "" {
		return nil
	}
	var out []string
	for _, m := range strings.Split(p.AllowedModels, ",") {
		if trimmed := strings.TrimSpace(m); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// SetPlanAllowedModels serializes a model list into a comma-separated string.
func SetPlanAllowedModels(models []string) string {
	if len(models) == 0 {
		return ""
	}
	return strings.Join(models, ",")
}

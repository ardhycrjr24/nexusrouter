package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AgentRepo persists custom agents.
type AgentRepo struct{ db *DB }

// Agents returns the agent repository.
func (db *DB) Agents() *AgentRepo { return &AgentRepo{db: db} }

const agentSelectCols = `id, tenant_id, name, description, system_prompt, model, skills, created_at, updated_at`

// Create inserts a new agent.
func (r *AgentRepo) Create(ctx context.Context, a Agent) error {
	q := r.db.rebind(`INSERT INTO agents
		(id, tenant_id, name, description, system_prompt, model, skills, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	_, err := r.db.sql.ExecContext(ctx, q,
		a.ID, a.TenantID, a.Name, a.Description, a.SystemPrompt, a.Model, a.Skills, formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create agent: %w", err)
	}
	return nil
}

// Get returns a single agent by id.
func (r *AgentRepo) Get(ctx context.Context, id string) (Agent, error) {
	q := r.db.rebind(`SELECT ` + agentSelectCols + ` FROM agents WHERE id = ?`)
	a, err := scanAgent(r.db.sql.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	return a, err
}

// List returns all agents for a tenant, newest first.
func (r *AgentRepo) List(ctx context.Context, tenantID string) ([]Agent, error) {
	q := r.db.rebind(`SELECT ` + agentSelectCols + ` FROM agents WHERE tenant_id = ? ORDER BY created_at DESC`)
	rows, err := r.db.sql.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("store: list agents: %w", err)
	}
	defer rows.Close()

	var out []Agent
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Update modifies an existing agent.
func (r *AgentRepo) Update(ctx context.Context, a Agent) error {
	q := r.db.rebind(`UPDATE agents SET name = ?, description = ?, system_prompt = ?, model = ?, skills = ?, updated_at = ? WHERE id = ?`)
	res, err := r.db.sql.ExecContext(ctx, q, a.Name, a.Description, a.SystemPrompt, a.Model, a.Skills, formatTime(a.UpdatedAt), a.ID)
	if err != nil {
		return fmt.Errorf("store: update agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an agent.
func (r *AgentRepo) Delete(ctx context.Context, id string) error {
	q := r.db.rebind(`DELETE FROM agents WHERE id = ?`)
	_, err := r.db.sql.ExecContext(ctx, q, id)
	return err
}

func scanAgent(scan func(dest ...any) error) (Agent, error) {
	var (
		a       Agent
		created string
		updated string
	)
	err := scan(&a.ID, &a.TenantID, &a.Name, &a.Description, &a.SystemPrompt, &a.Model, &a.Skills, &created, &updated)
	if err != nil {
		return Agent{}, err
	}
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return a, nil
}

func scanAgentRows(rows *sql.Rows) (Agent, error) {
	var (
		a       Agent
		created string
		updated string
	)
	err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Description, &a.SystemPrompt, &a.Model, &a.Skills, &created, &updated)
	if err != nil {
		return Agent{}, fmt.Errorf("store: scan agent: %w", err)
	}
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return a, nil
}

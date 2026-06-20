package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AliasRepo persists model aliases (bare name → provider/model).
type AliasRepo struct{ db *DB }

// Aliases returns the alias repository.
func (db *DB) Aliases() *AliasRepo { return &AliasRepo{db: db} }

// ModelAlias maps a bare name to a provider/model target.
type ModelAlias struct {
	Alias  string
	Target string
}

// List returns all aliases.
func (r *AliasRepo) List(ctx context.Context) ([]ModelAlias, error) {
	rows, err := r.db.sql.QueryContext(ctx, "SELECT alias, target FROM model_aliases ORDER BY alias")
	if err != nil {
		return nil, fmt.Errorf("store: list aliases: %w", err)
	}
	defer rows.Close()

	var out []ModelAlias
	for rows.Next() {
		var a ModelAlias
		if err := rows.Scan(&a.Alias, &a.Target); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get returns one alias by name.
func (r *AliasRepo) Get(ctx context.Context, alias string) (ModelAlias, error) {
	if r == nil || r.db == nil {
		return ModelAlias{}, ErrNotFound
	}
	q := r.db.rebind("SELECT alias, target FROM model_aliases WHERE alias = ?")
	var a ModelAlias
	err := r.db.sql.QueryRowContext(ctx, q, alias).Scan(&a.Alias, &a.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelAlias{}, ErrNotFound
	}
	if err != nil {
		return ModelAlias{}, fmt.Errorf("store: get alias: %w", err)
	}
	return a, nil
}

// Set upserts an alias.
func (r *AliasRepo) Set(ctx context.Context, alias, target string) error {
	q := r.db.rebind(`INSERT INTO model_aliases (alias, target) VALUES (?, ?)
		ON CONFLICT (alias) DO UPDATE SET target = excluded.target`)
	_, err := r.db.sql.ExecContext(ctx, q, alias, target)
	if err != nil {
		return fmt.Errorf("store: set alias: %w", err)
	}
	return nil
}

// Delete removes an alias.
func (r *AliasRepo) Delete(ctx context.Context, alias string) error {
	q := r.db.rebind("DELETE FROM model_aliases WHERE alias = ?")
	_, err := r.db.sql.ExecContext(ctx, q, alias)
	return err
}

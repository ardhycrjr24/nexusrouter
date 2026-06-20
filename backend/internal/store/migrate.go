package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsTable tracks which migrations have been applied.
const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);`

// Migrate applies every embedded migration that has not yet run, in filename
// order. Each migration runs inside a transaction so a failure leaves the
// schema untouched. It is safe to call on every startup.
func (db *DB) Migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, migrationsTable); err != nil {
		return fmt.Errorf("store: create migrations table: %w", err)
	}

	applied, err := db.appliedVersions(ctx)
	if err != nil {
		return err
	}

	files, err := migrationFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")
		if _, done := applied[version]; done {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + f)
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", f, err)
		}
		if err := db.runMigration(ctx, version, string(body)); err != nil {
			return fmt.Errorf("store: apply migration %s: %w", f, err)
		}
	}
	return nil
}

func (db *DB) runMigration(ctx context.Context, version, body string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range splitStatements(body) {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("statement failed: %w\n%s", err, stmt)
		}
	}

	insert := db.rebind("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)")
	if _, err := tx.ExecContext(ctx, insert, version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) appliedVersions(ctx context.Context) (map[string]struct{}, error) {
	rows, err := db.sql.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = struct{}{}
	}
	return applied, rows.Err()
}

func migrationFiles() ([]string, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: list migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// splitStatements breaks a SQL file into individual statements on semicolons.
//
// Comments are stripped from the whole body *before* splitting, because a "--"
// line comment may itself contain a semicolon ("...stored; plaintext is shown")
// which would otherwise split a statement incorrectly. The schema avoids
// semicolons inside string literals, so splitting the comment-free SQL on ";"
// is sufficient and keeps the runner dependency-free.
func splitStatements(body string) []string {
	clean := stripSQLComments(body)
	parts := strings.Split(clean, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// stripSQLComments removes whole-line "--" comments. It intentionally only
// handles line comments (the schema uses no inline trailing comments), which is
// enough to keep semicolons inside prose from corrupting statement splitting.
func stripSQLComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// Package store is NexusRouter's persistence layer. It opens a database
// connection (SQLite by default, Postgres optionally), applies embedded
// migrations on startup, and exposes typed repositories for each domain.
//
// The same SQL runs against both engines wherever possible; dialect-specific
// statements are selected via the Dialect value. This keeps the local
// single-binary experience zero-config while letting team/VPS deployments point
// at Postgres without code changes.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver registered as "pgx"
	_ "modernc.org/sqlite"             // pure-Go SQLite driver, no CGO

	"github.com/ardhycrjr24/nexusrouter/backend/internal/config"
)

// Dialect identifies the active SQL engine.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// DB wraps the sql.DB handle together with its dialect so repositories can
// adapt placeholder syntax and engine-specific statements.
type DB struct {
	sql        *sql.DB
	dialect    Dialect
	sqlitePath string
}

// Open connects using the given database configuration and verifies the
// connection. The caller is responsible for calling Close.
func Open(ctx context.Context, cfg config.DatabaseConfig, dataDir string) (*DB, error) {
	dialect := Dialect(cfg.Driver)

	var (
		driverName     string
		dsn            string
		sqliteFilePath string
	)
	switch dialect {
	case DialectSQLite:
		driverName = "sqlite"
		sqliteFilePath = sqlitePath(cfg, dataDir)
		dsn = sqliteDSNFromPath(sqliteFilePath)
	case DialectPostgres:
		driverName = "pgx"
		dsn = cfg.DSN
	default:
		return nil, fmt.Errorf("store: unsupported driver %q", cfg.Driver)
	}

	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", dialect, err)
	}

	// SQLite is single-writer; cap connections to avoid SQLITE_BUSY. Postgres
	// uses the configured pool.
	if dialect == DialectSQLite {
		sqlDB.SetMaxOpenConns(1)
	} else {
		if cfg.MaxOpenConns > 0 {
			sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.MaxIdleConns > 0 {
			sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
		}
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: ping %s: %w", dialect, err)
	}

	db := &DB{sql: sqlDB, dialect: dialect, sqlitePath: sqliteFilePath}
	if dialect == DialectSQLite {
		if err := db.applySQLitePragmas(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}
	return db, nil
}

// sqliteDSN builds the SQLite connection string. An empty DSN defaults to a
// file under the data directory; ":memory:" is honored for tests.
func sqlitePath(cfg config.DatabaseConfig, dataDir string) string {
	path := cfg.DSN
	if path == "" {
		path = dataDir + "/nexusrouter.db"
	}
	return path
}

func sqliteDSN(cfg config.DatabaseConfig, dataDir string) string {
	return sqliteDSNFromPath(sqlitePath(cfg, dataDir))
}

func sqliteDSNFromPath(path string) string {
	if path == ":memory:" {
		// Shared-cache in-memory keeps the schema visible across the pool.
		return "file::memory:?cache=shared"
	}
	// WAL + busy timeout improve concurrent read/write behavior.
	return "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
}

func (db *DB) applySQLitePragmas(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, p := range pragmas {
		if _, err := db.sql.ExecContext(ctx, p); err != nil {
			return fmt.Errorf("store: pragma %q: %w", p, err)
		}
	}
	return nil
}

// SQL exposes the underlying handle for repositories within this package.
func (db *DB) SQL() *sql.DB { return db.sql }

// Dialect reports the active engine.
func (db *DB) Dialect() Dialect { return db.dialect }

// SQLitePath reports the SQLite database file path when the active engine is SQLite.
func (db *DB) SQLitePath() string { return db.sqlitePath }

// BeginTx starts a database transaction. Callers use this for multi-table
// writes that must succeed or fail atomically (e.g. key + budget creation).
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.sql.BeginTx(ctx, opts)
}

// Close releases the connection pool.
func (db *DB) Close() error { return db.sql.Close() }

// epochExpr returns a dialect-specific SQL expression that converts a TEXT
// RFC3339 timestamp (column or placeholder) to unix-epoch seconds. SQLite has
// strftime; Postgres needs an explicit cast to timestamptz before EXTRACT.
func (db *DB) epochExpr(operand string) string {
	if db.dialect == DialectPostgres {
		return "EXTRACT(EPOCH FROM " + operand + "::timestamptz)"
	}
	return "strftime('%s', " + operand + ")"
}

// dateExpr returns a dialect-specific SQL expression that yields the YYYY-MM-DD
// calendar date (as text) of a TEXT RFC3339 timestamp. SQLite's DATE() accepts
// text directly and returns text; Postgres needs a cast through timestamptz
// and an explicit format so the scanned value is always a string.
func (db *DB) dateExpr(operand string) string {
	if db.dialect == DialectPostgres {
		return "TO_CHAR(" + operand + "::timestamptz, 'YYYY-MM-DD')"
	}
	return "DATE(" + operand + ")"
}

// rebind converts '?' placeholders to the engine's native form. SQLite accepts
// '?' directly; Postgres needs $1, $2, ... positional placeholders.
func (db *DB) rebind(query string) string {
	if db.dialect != DialectPostgres {
		return query
	}
	var b []byte
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b = append(b, '$')
			b = strconv.AppendInt(b, int64(n), 10)
			continue
		}
		b = append(b, query[i])
	}
	return string(b)
}

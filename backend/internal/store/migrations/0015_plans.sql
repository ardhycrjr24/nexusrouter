-- Plans: reusable budget/model templates that API keys can inherit from.

CREATE TABLE plans (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    description   TEXT DEFAULT '',
    limit_micros  INTEGER NOT NULL DEFAULT 0,
    limit_tokens  INTEGER NOT NULL DEFAULT 0,
    period        TEXT NOT NULL DEFAULT 'monthly',
    alert_pct     INTEGER NOT NULL DEFAULT 80,
    hard_cutoff   INTEGER NOT NULL DEFAULT 1,
    allowed_models TEXT DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

-- Seed a "Default" plan for existing data. Use a fixed RFC3339 literal so the
-- statement is portable across SQLite and Postgres (datetime('now') is
-- SQLite-only). The rest of the app writes RFC3339 strings to these TEXT
-- columns via formatTime, so this matches the on-disk shape.
INSERT INTO plans (id, tenant_id, name, description, limit_micros, limit_tokens, period, alert_pct, hard_cutoff, allowed_models, created_at, updated_at)
VALUES ('default', 'default', 'Default', 'Default plan for existing keys', 0, 0, 'monthly', 80, 1, '', '2026-06-11T00:00:00Z', '2026-06-11T00:00:00Z');

-- Add plan_id column to api_keys.
ALTER TABLE api_keys ADD COLUMN plan_id TEXT NOT NULL DEFAULT '';

-- Assign all existing keys to the default plan.
UPDATE api_keys SET plan_id = 'default' WHERE plan_id = '';
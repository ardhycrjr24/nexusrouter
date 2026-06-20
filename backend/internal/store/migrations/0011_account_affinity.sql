CREATE TABLE IF NOT EXISTS account_affinity (
    scope_key  TEXT PRIMARY KEY,
    account_id TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_account_affinity_expires ON account_affinity(expires_at);

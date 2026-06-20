-- Background health probe status per account/model.
CREATE TABLE IF NOT EXISTS account_health (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    status TEXT NOT NULL,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_ok_at TEXT,
    last_checked_at TEXT NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    UNIQUE(account_id, model)
);

CREATE INDEX IF NOT EXISTS idx_account_health_tenant_status ON account_health(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_account_health_account_model ON account_health(account_id, model);
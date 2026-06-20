-- Token budget support: extend budgets table with token limit.
ALTER TABLE budgets ADD COLUMN limit_tokens INTEGER NOT NULL DEFAULT 0;

-- Per-API-key model access control. When rows exist for a key, only those
-- models are allowed. Empty = all models permitted (backward compatible).
CREATE TABLE IF NOT EXISTS api_key_model_access (
    api_key_id  TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    model       TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (api_key_id, model)
);
CREATE INDEX IF NOT EXISTS idx_akma_key ON api_key_model_access(api_key_id);
-- Exponential backoff level per account (used for adaptive cooldowns).
ALTER TABLE accounts ADD COLUMN backoff_level INTEGER NOT NULL DEFAULT 0;

-- Model-level cooldowns: locks a specific model on an account so other
-- models on the same account remain available.
CREATE TABLE IF NOT EXISTS model_cooldowns (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model       TEXT NOT NULL,
    cooldown_until TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(account_id, model)
);

CREATE INDEX IF NOT EXISTS idx_model_cooldowns_account
    ON model_cooldowns(account_id);

CREATE INDEX IF NOT EXISTS idx_model_cooldowns_lookup
    ON model_cooldowns(account_id, model);

-- Round-robin rotation state per chain (in-memory is sufficient, but we
-- persist the last-used index so restarts don't reset distribution).
CREATE TABLE IF NOT EXISTS chain_rotation (
    chain_id    TEXT PRIMARY KEY REFERENCES chains(id) ON DELETE CASCADE,
    last_index  INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL
);
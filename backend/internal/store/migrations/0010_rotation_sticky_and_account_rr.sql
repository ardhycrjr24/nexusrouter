-- Track sticky round-robin counts for combo rotation.
ALTER TABLE chain_rotation ADD COLUMN hit_count INTEGER NOT NULL DEFAULT 0;

-- Round-robin rotation state per tenant/provider/model target. This lets
-- direct provider/model routes distribute load across accounts for the same
-- provider without changing the requested model.
CREATE TABLE IF NOT EXISTS target_rotation (
    scope_key  TEXT PRIMARY KEY,
    last_index INTEGER NOT NULL DEFAULT 0,
    hit_count  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

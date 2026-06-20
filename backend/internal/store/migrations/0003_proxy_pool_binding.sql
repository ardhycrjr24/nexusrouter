-- Proxy pool binding: accounts can be bound to a proxy pool.
ALTER TABLE accounts ADD COLUMN proxy_pool_id TEXT NOT NULL DEFAULT '';

-- Proxy pools table (if not already created by the app layer).
CREATE TABLE IF NOT EXISTS proxy_pools (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'http',
    proxy_url   TEXT NOT NULL,
    no_proxy    TEXT NOT NULL DEFAULT '',
    strict      INTEGER NOT NULL DEFAULT 0,
    is_active   INTEGER NOT NULL DEFAULT 1,
    test_status TEXT NOT NULL DEFAULT 'unknown',
    last_tested TEXT,
    last_error  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pools_active ON proxy_pools(is_active);

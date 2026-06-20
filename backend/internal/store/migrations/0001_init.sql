-- NexusRouter initial schema.
-- Portable across SQLite and Postgres: TEXT for ids/timestamps (RFC3339),
-- INTEGER for counters/flags, no engine-specific types.

-- Tenants scope all data for multi-tenant deployments. Local single-user mode
-- uses a single implicit "default" tenant.
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

-- Projects partition usage and budgets within a tenant.
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_projects_tenant ON projects(tenant_id);

-- API keys authenticate inbound proxy requests. Only the argon2 hash and a
-- fast sha-256 lookup index are stored; plaintext is shown once at creation.
CREATE TABLE IF NOT EXISTS api_keys (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id    TEXT REFERENCES projects(id) ON DELETE SET NULL,
    name          TEXT NOT NULL,
    key_hash      TEXT NOT NULL,
    lookup_hash   TEXT NOT NULL,
    display       TEXT NOT NULL,
    scopes        TEXT NOT NULL DEFAULT '',
    disabled      INTEGER NOT NULL DEFAULT 0,
    last_used_at  TEXT,
    created_at    TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_lookup ON api_keys(lookup_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);

-- Provider accounts hold the encrypted upstream credentials. The secret columns
-- carry envelope-encrypted blobs (wrapped DEK + ciphertext), never plaintext.
CREATE TABLE IF NOT EXISTS accounts (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider           TEXT NOT NULL,
    label              TEXT NOT NULL,
    auth_kind          TEXT NOT NULL,          -- api_key | oauth | none
    secret_wrapped_dek TEXT,                   -- envelope: wrapped data key
    secret_ciphertext  TEXT,                   -- envelope: encrypted secret
    token_wrapped_dek  TEXT,                   -- oauth access token (sealed)
    token_ciphertext   TEXT,
    refresh_wrapped_dek TEXT,                  -- oauth refresh token (sealed)
    refresh_ciphertext TEXT,
    token_expires_at   TEXT,
    metadata           TEXT NOT NULL DEFAULT '{}', -- JSON: base_url, region, project_id...
    priority           INTEGER NOT NULL DEFAULT 100,
    disabled           INTEGER NOT NULL DEFAULT 0,
    cooldown_until     TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_accounts_tenant_provider ON accounts(tenant_id, provider);

-- Chains are ordered fallback definitions (NexusRouter's routing chains).
CREATE TABLE IF NOT EXISTS chains (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    strategy    TEXT NOT NULL DEFAULT 'priority', -- priority | round_robin | latency | cost
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chains_tenant ON chains(tenant_id);

-- Chain steps are the ordered candidate targets within a chain.
CREATE TABLE IF NOT EXISTS chain_steps (
    id          TEXT PRIMARY KEY,
    chain_id    TEXT NOT NULL REFERENCES chains(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    provider    TEXT NOT NULL,
    model       TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chain_steps_chain ON chain_steps(chain_id, position);

-- Usage records meter every completed request for analytics and billing.
CREATE TABLE IF NOT EXISTS usage_records (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    project_id        TEXT,
    api_key_id        TEXT,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    account_id        TEXT,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    cached_tokens     INTEGER NOT NULL DEFAULT 0,
    cost_micros       INTEGER NOT NULL DEFAULT 0, -- cost in millionths of USD
    cache_hit         INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_usage_tenant_time ON usage_records(tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_key_time ON usage_records(api_key_id, created_at);

-- Budgets enforce hard spend limits per scope (tenant/project/key).
CREATE TABLE IF NOT EXISTS budgets (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scope_kind      TEXT NOT NULL, -- tenant | project | api_key
    scope_id        TEXT NOT NULL,
    limit_micros    INTEGER NOT NULL,  -- hard cap in millionths of USD
    period          TEXT NOT NULL,     -- daily | weekly | monthly | total
    alert_pct       INTEGER NOT NULL DEFAULT 80,
    hard_cutoff     INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_budgets_scope ON budgets(scope_kind, scope_id);

-- Audit log is append-only: who did what, when.
CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT,
    actor       TEXT NOT NULL,   -- api_key id, dashboard user, or "system"
    action      TEXT NOT NULL,   -- e.g. proxy.request, key.create, account.delete
    target      TEXT,
    detail      TEXT NOT NULL DEFAULT '{}', -- JSON metadata (no secrets)
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_log(tenant_id, created_at);

-- Settings is a simple key/value store for runtime configuration owned by the UI.
CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
-- Guardrails: pluggable safety policies that gate inbound prompts and outbound
-- responses. Policies are layered by scope (global < provider < model < chain
-- < apikey) and merged at request time by the resolver; the most specific
-- non-nil field wins.

CREATE TABLE IF NOT EXISTS guardrail_policies (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scope       TEXT NOT NULL,           -- global | provider | model | chain | apikey
    scope_id    TEXT NOT NULL DEFAULT '',-- '' for global, else identifier (provider alias, model id, chain id, api key id)
    name        TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    config      TEXT NOT NULL DEFAULT '{}',  -- JSON: {pii: {...}, injection: {...}, topics: {...}, toxicity: {...}, bias: {...}}
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
-- Each (tenant, scope, scope_id) maps to at most one policy. Soft-bound by id
-- so providers/models/chains can be renamed without orphaning configs.
CREATE UNIQUE INDEX IF NOT EXISTS idx_guardrail_policies_scope
    ON guardrail_policies(tenant_id, scope, scope_id);
CREATE INDEX IF NOT EXISTS idx_guardrail_policies_tenant
    ON guardrail_policies(tenant_id);

-- Audit trail. Every detector decision is appended here so users can debug
-- false-positives and prove compliance. Findings is a JSON array per detector
-- and is opaque at the SQL layer.
CREATE TABLE IF NOT EXISTS guardrail_logs (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    request_id      TEXT NOT NULL DEFAULT '',
    api_key_id      TEXT NOT NULL DEFAULT '',
    provider        TEXT NOT NULL DEFAULT '',
    model           TEXT NOT NULL DEFAULT '',
    chain_id        TEXT NOT NULL DEFAULT '',
    detector        TEXT NOT NULL,       -- pii | injection | toxicity | topics | bias
    direction       TEXT NOT NULL,       -- inbound | outbound
    action          TEXT NOT NULL,       -- allow | warn | mask | block | log_only
    severity        TEXT NOT NULL DEFAULT '',  -- low | medium | high
    reason          TEXT NOT NULL DEFAULT '',
    findings        TEXT NOT NULL DEFAULT '[]', -- JSON array of {entity, score, start, end, redacted}
    created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_guardrail_logs_tenant_time
    ON guardrail_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_guardrail_logs_apikey
    ON guardrail_logs(api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_guardrail_logs_detector
    ON guardrail_logs(tenant_id, detector, created_at DESC);

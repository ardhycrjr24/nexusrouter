-- Project-scoped budget checks filter usage_records by project_id every request.
-- Without this index that path is a full table scan; tenant/key scopes already
-- have covering indexes from 0001_init.
CREATE INDEX IF NOT EXISTS idx_usage_project_time ON usage_records(project_id, created_at);

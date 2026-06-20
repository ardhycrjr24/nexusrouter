-- Add cache_write_tokens column to usage_records for accurate prompt-cache
-- write billing (e.g. Anthropic charges 1.25x standard input for cache writes).
ALTER TABLE usage_records ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;

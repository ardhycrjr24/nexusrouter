-- Add the calling-client column to usage_records so optimization savings can
-- be attributed per client (claude-code, codex, cline, ...) instead of being
-- counted only in aggregate. Existing rows default to '' and are reported as
-- "unknown" by SavingsByClient.
ALTER TABLE usage_records ADD COLUMN client TEXT NOT NULL DEFAULT '';
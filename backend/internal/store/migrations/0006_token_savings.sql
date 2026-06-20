-- Add token-saving analytics columns to usage_records.
-- Tracks RTK (slimmer) input-side compression savings and Caveman/Terse
-- output-side feature activation for detailed savings reporting.
ALTER TABLE usage_records ADD COLUMN slim_bytes_saved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN slim_tokens_saved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN slim_rules TEXT NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN caveman_active INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN terse_active INTEGER NOT NULL DEFAULT 0;

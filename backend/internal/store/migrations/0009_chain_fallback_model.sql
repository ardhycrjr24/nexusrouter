-- Add fallback provider/model columns to chains table.
-- When set, these are appended as the last-resort target after all chain steps
-- are exhausted, ensuring a combo never fails completely.

ALTER TABLE chains ADD COLUMN fallback_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE chains ADD COLUMN fallback_model    TEXT NOT NULL DEFAULT '';
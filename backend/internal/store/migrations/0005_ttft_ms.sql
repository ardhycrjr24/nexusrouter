-- Add ttft_ms column to usage_records for time-to-first-token tracking.
-- TTFT measures the elapsed time from stream start to the first meaningful
-- chunk (text, thinking, or tool_call). Zero for non-streaming requests.
ALTER TABLE usage_records ADD COLUMN ttft_ms INTEGER NOT NULL DEFAULT 0;

-- Add needs_reconnect flag so the dashboard can show "Reconnect Required"
-- when an OAuth refresh token is permanently revoked by the upstream.
ALTER TABLE accounts ADD COLUMN needs_reconnect INTEGER NOT NULL DEFAULT 0;

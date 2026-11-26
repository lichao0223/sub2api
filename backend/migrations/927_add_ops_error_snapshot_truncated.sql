-- Request snapshots are now size-capped and may be payload-truncated or
-- payload-purged (retention); keep an explicit flag so the UI never mistakes a
-- partial capture for a complete one.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS request_snapshot_truncated BOOLEAN NOT NULL DEFAULT FALSE;

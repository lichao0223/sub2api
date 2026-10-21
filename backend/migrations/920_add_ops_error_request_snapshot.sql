-- Error-only diagnostic payloads; deleting an error also deletes its snapshot.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS request_snapshot_gzip BYTEA,
    ADD COLUMN IF NOT EXISTS request_snapshot_bytes BIGINT NOT NULL DEFAULT 0;

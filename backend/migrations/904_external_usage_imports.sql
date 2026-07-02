-- External usage import batches and daily user stats for Token Ranking.

CREATE TABLE IF NOT EXISTS external_usage_import_batches (
    id BIGSERIAL PRIMARY KEY,
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    file_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'imported',
    total_rows INT NOT NULL DEFAULT 0,
    matched_rows INT NOT NULL DEFAULT 0,
    unmatched_rows INT NOT NULL DEFAULT 0,
    conflict_rows INT NOT NULL DEFAULT 0,
    invalid_rows INT NOT NULL DEFAULT 0,
    overwritten_rows INT NOT NULL DEFAULT 0,
    imported_rows INT NOT NULL DEFAULT 0,
    created_by BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    imported_at TIMESTAMPTZ,
    voided_at TIMESTAMPTZ,
    voided_by BIGINT,
    note TEXT NOT NULL DEFAULT '',
    CONSTRAINT external_usage_import_batches_status_check CHECK (
        status IN ('previewed', 'imported', 'voided', 'failed')
    )
);

CREATE INDEX IF NOT EXISTS idx_external_usage_import_batches_created_at
    ON external_usage_import_batches (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_external_usage_import_batches_file_sha256
    ON external_usage_import_batches (file_sha256);

CREATE TABLE IF NOT EXISTS external_usage_daily_user_stats (
    id BIGSERIAL PRIMARY KEY,
    batch_id BIGINT NOT NULL REFERENCES external_usage_import_batches(id) ON DELETE RESTRICT,
    bucket_date DATE NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    username_snapshot VARCHAR(100) NOT NULL DEFAULT '',
    requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    active_ms BIGINT NOT NULL DEFAULT 0,
    nonwork_tokens BIGINT NOT NULL DEFAULT 0,
    nonwork_active_ms BIGINT NOT NULL DEFAULT 0,
    raw_row_number INT NOT NULL DEFAULT 0,
    raw_username VARCHAR(255) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT external_usage_daily_user_stats_nonnegative_check CHECK (
        requests >= 0
        AND input_tokens >= 0
        AND output_tokens >= 0
        AND cache_creation_tokens >= 0
        AND cache_read_tokens >= 0
        AND total_tokens >= 0
        AND actual_cost >= 0
        AND active_ms >= 0
        AND nonwork_tokens >= 0
        AND nonwork_active_ms >= 0
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_external_usage_daily_user_stats_date_user_unique
    ON external_usage_daily_user_stats (bucket_date, user_id);

CREATE INDEX IF NOT EXISTS idx_external_usage_daily_user_stats_batch
    ON external_usage_daily_user_stats (batch_id);

CREATE INDEX IF NOT EXISTS idx_external_usage_daily_user_stats_user_date
    ON external_usage_daily_user_stats (user_id, bucket_date);

COMMENT ON TABLE external_usage_import_batches IS 'Audit batches for externally imported Token Ranking usage summaries.';
COMMENT ON TABLE external_usage_daily_user_stats IS 'Per-day per-user external usage summaries merged into Token Ranking without writing usage_logs.';

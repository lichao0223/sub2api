-- Non-work time usage ranking calendar and aggregation tables.

CREATE TABLE IF NOT EXISTS calendar_days (
    date DATE NOT NULL,
    country VARCHAR(8) NOT NULL DEFAULT 'CN',
    is_workday BOOLEAN NOT NULL,
    is_offday BOOLEAN NOT NULL,
    is_weekend BOOLEAN NOT NULL,
    day_type VARCHAR(32) NOT NULL,
    holiday_name VARCHAR(64),
    source VARCHAR(64) NOT NULL,
    source_version VARCHAR(128),
    confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    manual_override BOOLEAN NOT NULL DEFAULT FALSE,
    raw JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (country, date),
    CONSTRAINT calendar_days_workday_offday_check CHECK (is_workday <> is_offday),
    CONSTRAINT calendar_days_day_type_check CHECK (
        day_type IN (
            'normal_workday',
            'normal_weekend',
            'holiday_offday',
            'makeup_workday',
            'manual_workday',
            'manual_offday',
            'predicted_workday',
            'predicted_weekend'
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_calendar_days_country_confirmed
    ON calendar_days (country, confirmed);

COMMENT ON TABLE calendar_days IS 'Local business calendar dimension used for workday/non-work-time analytics.';
COMMENT ON COLUMN calendar_days.is_weekend IS 'Natural Saturday/Sunday flag. A weekend can still be a workday when it is a makeup workday.';
COMMENT ON COLUMN calendar_days.confirmed IS 'False means the day was generated from predicted/default rules and may be replaced by official holiday data.';
COMMENT ON COLUMN calendar_days.manual_override IS 'Manual overrides must not be replaced by automatic holiday sync.';

CREATE TABLE IF NOT EXISTS calendar_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    country VARCHAR(8) NOT NULL DEFAULT 'CN',
    year INT NOT NULL,
    source VARCHAR(64) NOT NULL,
    source_url TEXT,
    source_version VARCHAR(128),
    status VARCHAR(32) NOT NULL,
    days_inserted INT NOT NULL DEFAULT 0,
    days_updated INT NOT NULL DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    CONSTRAINT calendar_sync_runs_status_check CHECK (
        status IN ('success', 'failed', 'skipped', 'predicted')
    )
);

CREATE INDEX IF NOT EXISTS idx_calendar_sync_runs_year
    ON calendar_sync_runs (country, year, started_at DESC);

COMMENT ON TABLE calendar_sync_runs IS 'Audit log for automatic business calendar synchronization.';

CREATE TABLE IF NOT EXISTS usage_nonwork_daily_user_stats (
    bucket_date DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    user_id BIGINT NOT NULL,
    segment VARCHAR(32) NOT NULL,
    requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    active_ms BIGINT NOT NULL DEFAULT 0,
    active_sessions BIGINT NOT NULL DEFAULT 0,
    calendar_confirmed BOOLEAN NOT NULL DEFAULT TRUE,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, timezone, user_id, segment),
    CONSTRAINT usage_nonwork_daily_user_stats_segment_check CHECK (
        segment IN ('work_hours', 'after_hours', 'offday')
    )
);

CREATE INDEX IF NOT EXISTS idx_usage_nonwork_daily_user_stats_user_date
    ON usage_nonwork_daily_user_stats (user_id, bucket_date DESC);

CREATE INDEX IF NOT EXISTS idx_usage_nonwork_daily_user_stats_segment_date
    ON usage_nonwork_daily_user_stats (segment, bucket_date DESC);

COMMENT ON TABLE usage_nonwork_daily_user_stats IS 'Daily per-user usage aggregates split by work-hours, after-hours, and offday segments.';
COMMENT ON COLUMN usage_nonwork_daily_user_stats.active_ms IS 'Active usage duration calculated from request gaps, not API response duration.';

-- Tracks which non-work daily usage buckets have been computed.
-- This prevents zero-usage days from being mistaken for missing aggregation.

CREATE TABLE IF NOT EXISTS usage_nonwork_daily_stat_runs (
    bucket_date DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, timezone)
);

CREATE INDEX IF NOT EXISTS idx_usage_nonwork_daily_stat_runs_timezone_date
    ON usage_nonwork_daily_stat_runs (timezone, bucket_date DESC);

COMMENT ON TABLE usage_nonwork_daily_stat_runs IS 'Daily completion markers for non-work usage aggregation, including days with zero usage.';

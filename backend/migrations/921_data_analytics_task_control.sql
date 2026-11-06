-- Keep an existing cost installation running; fresh installations start paused.
INSERT INTO settings (key, value, updated_at)
SELECT 'data_analytics.tasks_enabled',
       jsonb_build_object('enabled',EXISTS(
           SELECT 1 FROM cost_jobs WHERE job_key='incremental' AND last_success_at IS NOT NULL
       ),'version',1,'updated_by',0,'updated_at',NOW())::text, NOW()
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS analytics_task_workers (
    instance_id VARCHAR(256) PRIMARY KEY,
    running INTEGER NOT NULL DEFAULT 0 CHECK (running >= 0),
    observed_version BIGINT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    last_success_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS analytics_task_workers_last_seen_idx
    ON analytics_task_workers (last_seen_at);

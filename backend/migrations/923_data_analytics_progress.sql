ALTER TABLE analytics_task_workers
    ADD COLUMN IF NOT EXISTS analytics_day TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS analytics_started_at TIMESTAMPTZ;

ALTER TABLE analytics_dirty_days ADD COLUMN IF NOT EXISTS retry_after TIMESTAMPTZ;

-- Retry days canceled by the old 30-second limit on the next aggregation run.
UPDATE analytics_dirty_days SET retry_after=NOW(),
    last_error='上次核算查询被取消，已安排重试'
WHERE last_error='pq: canceling statement due to user request';
UPDATE analytics_runtime_state SET last_error='上次核算查询被取消，已安排重试'
WHERE last_error='pq: canceling statement due to user request';

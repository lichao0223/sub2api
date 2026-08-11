ALTER TABLE ai_work_insight_samples
    ADD COLUMN IF NOT EXISTS truncated BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE ai_work_insight_samples
    DROP CONSTRAINT IF EXISTS chk_ai_work_insight_samples_reason;

ALTER TABLE ai_work_insight_samples
    ADD CONSTRAINT chk_ai_work_insight_samples_reason
    CHECK (sample_reason IN ('session_coverage','rate','compact'));

CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_pending_dedupe
    ON ai_work_insight_samples(user_id,local_date,prompt_hash,id DESC)
    WHERE status IN ('staging','pending_batch');

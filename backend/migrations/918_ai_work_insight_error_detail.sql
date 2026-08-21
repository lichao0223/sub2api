ALTER TABLE ai_work_insight_batches
    ADD COLUMN IF NOT EXISTS error_detail TEXT NOT NULL DEFAULT '';

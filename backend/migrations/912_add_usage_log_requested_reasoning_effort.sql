ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS requested_reasoning_effort VARCHAR(20);

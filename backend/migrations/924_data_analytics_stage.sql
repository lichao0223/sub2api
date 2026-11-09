ALTER TABLE analytics_task_workers ADD COLUMN IF NOT EXISTS analytics_stage TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_account_cost_configs_plan_mode_effective
    ON account_cost_configs (plan_id, cost_mode, account_id, effective_from, effective_to);

CREATE INDEX IF NOT EXISTS idx_cost_jobs_updated
    ON cost_jobs (updated_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_cost_jobs_active_recalculation_range
    ON cost_jobs (start_date, end_date)
    WHERE kind = 'recalculation' AND status IN ('queued', 'running');

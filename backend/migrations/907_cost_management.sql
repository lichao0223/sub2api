-- Isolated CNY cost management. This is reporting-only and does not alter gateway billing.

CREATE TABLE IF NOT EXISTS cost_plans (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    plan_type VARCHAR(16) NOT NULL,
    fixed_category VARCHAR(24),
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_plans_type_check CHECK (plan_type IN ('metered', 'fixed')),
    CONSTRAINT cost_plans_category_check CHECK (
        (plan_type = 'metered' AND fixed_category IS NULL)
        OR (plan_type = 'fixed' AND fixed_category IS NOT NULL
            AND fixed_category IN ('coding_plan', 'self_hosted', 'other'))
    ),
    CONSTRAINT cost_plans_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX IF NOT EXISTS idx_cost_plans_type_status
    ON cost_plans (plan_type, status, id DESC);

CREATE TABLE IF NOT EXISTS cost_plan_versions (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES cost_plans(id) ON DELETE RESTRICT,
    version_no INT NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    monthly_unit_cost_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    purchase_quantity INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_plan_versions_period_check CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT cost_plan_versions_amount_check CHECK (monthly_unit_cost_cny >= 0),
    CONSTRAINT cost_plan_versions_quantity_check CHECK (purchase_quantity > 0),
    CONSTRAINT cost_plan_versions_plan_version_unique UNIQUE (plan_id, version_no)
);

CREATE INDEX IF NOT EXISTS idx_cost_plan_versions_effective
    ON cost_plan_versions (plan_id, effective_from, effective_to);

CREATE TABLE IF NOT EXISTS cost_model_prices (
    id BIGSERIAL PRIMARY KEY,
    plan_version_id BIGINT NOT NULL REFERENCES cost_plan_versions(id) ON DELETE RESTRICT,
    upstream_model VARCHAR(255) NOT NULL,
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'token',
    input_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    output_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    cache_write_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    cache_read_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    image_input_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    image_output_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    per_request_price_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_model_prices_model_check CHECK (BTRIM(upstream_model) <> ''),
    CONSTRAINT cost_model_prices_billing_mode_check CHECK (billing_mode IN ('token', 'request', 'hybrid')),
    CONSTRAINT cost_model_prices_nonnegative_check CHECK (
        input_price_cny >= 0 AND output_price_cny >= 0
        AND cache_write_price_cny >= 0 AND cache_read_price_cny >= 0
        AND image_input_price_cny >= 0 AND image_output_price_cny >= 0
        AND per_request_price_cny >= 0
    ),
    CONSTRAINT cost_model_prices_version_model_unique UNIQUE (plan_version_id, upstream_model)
);

CREATE TABLE IF NOT EXISTS account_cost_configs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    cost_mode VARCHAR(16) NOT NULL,
    plan_id BIGINT REFERENCES cost_plans(id) ON DELETE RESTRICT,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    exclude_reason VARCHAR(255) NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_cost_configs_mode_check CHECK (cost_mode IN ('metered', 'fixed', 'excluded')),
    CONSTRAINT account_cost_configs_plan_check CHECK (
        (cost_mode = 'excluded' AND plan_id IS NULL AND BTRIM(exclude_reason) <> '')
        OR (cost_mode IN ('metered', 'fixed') AND plan_id IS NOT NULL)
    ),
    CONSTRAINT account_cost_configs_period_check CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_account_cost_configs_effective
    ON account_cost_configs (account_id, effective_from, effective_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_cost_configs_one_open
    ON account_cost_configs (account_id) WHERE effective_to IS NULL;

CREATE TABLE IF NOT EXISTS cost_daily_aggregates (
    id BIGSERIAL PRIMARY KEY,
    bucket_date DATE NOT NULL,
    aggregate_scope VARCHAR(32) NOT NULL,
    calculation_status VARCHAR(16) NOT NULL DEFAULT 'calculated',
    issue_code VARCHAR(64) NOT NULL DEFAULT '',
    account_cost_config_id BIGINT REFERENCES account_cost_configs(id) ON DELETE RESTRICT,
    account_id BIGINT REFERENCES accounts(id) ON DELETE RESTRICT,
    user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    plan_id BIGINT REFERENCES cost_plans(id) ON DELETE RESTRICT,
    plan_version_id BIGINT REFERENCES cost_plan_versions(id) ON DELETE RESTRICT,
    upstream_model VARCHAR(255) NOT NULL DEFAULT '',
    request_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    image_input_tokens BIGINT NOT NULL DEFAULT 0,
    image_output_tokens BIGINT NOT NULL DEFAULT 0,
    image_count BIGINT NOT NULL DEFAULT 0,
    amount_cny NUMERIC(20,12) NOT NULL DEFAULT 0,
    pending_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_daily_aggregates_scope_check CHECK (aggregate_scope IN ('usage', 'fixed_plan_total', 'fixed_user_allocation')),
    CONSTRAINT cost_daily_aggregates_status_check CHECK (calculation_status IN ('calculated', 'pending', 'error')),
    CONSTRAINT cost_daily_aggregates_nonnegative_check CHECK (
        request_count >= 0 AND input_tokens >= 0 AND output_tokens >= 0
        AND cache_write_tokens >= 0 AND cache_read_tokens >= 0
        AND image_input_tokens >= 0 AND image_output_tokens >= 0 AND image_count >= 0
        AND amount_cny >= 0 AND pending_count >= 0 AND error_count >= 0
    ),
    CONSTRAINT cost_daily_aggregates_dimension_unique UNIQUE NULLS NOT DISTINCT (
        bucket_date, aggregate_scope, calculation_status, issue_code,
        account_cost_config_id, account_id, user_id, plan_id, plan_version_id, upstream_model
    )
);

CREATE INDEX IF NOT EXISTS idx_cost_daily_scope_date ON cost_daily_aggregates (aggregate_scope, bucket_date);
CREATE INDEX IF NOT EXISTS idx_cost_daily_user_date ON cost_daily_aggregates (user_id, bucket_date);
CREATE INDEX IF NOT EXISTS idx_cost_daily_account_date ON cost_daily_aggregates (account_id, bucket_date);
CREATE INDEX IF NOT EXISTS idx_cost_daily_plan_date ON cost_daily_aggregates (plan_id, bucket_date);
CREATE INDEX IF NOT EXISTS idx_cost_daily_status_date ON cost_daily_aggregates (calculation_status, bucket_date);

CREATE TABLE IF NOT EXISTS cost_jobs (
    id BIGSERIAL PRIMARY KEY,
    kind VARCHAR(20) NOT NULL,
    job_key VARCHAR(64),
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    start_date DATE,
    end_date DATE,
    last_usage_log_id BIGINT NOT NULL DEFAULT 0,
    total_days INT NOT NULL DEFAULT 0,
    completed_days INT NOT NULL DEFAULT 0,
    requested_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    last_success_at TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_jobs_kind_check CHECK (kind IN ('incremental', 'recalculation')),
    CONSTRAINT cost_jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    CONSTRAINT cost_jobs_progress_check CHECK (last_usage_log_id >= 0 AND total_days >= 0 AND completed_days >= 0),
    CONSTRAINT cost_jobs_range_check CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cost_jobs_job_key
    ON cost_jobs (job_key) WHERE job_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cost_jobs_kind_status_created
    ON cost_jobs (kind, status, created_at DESC);

INSERT INTO cost_jobs (kind, job_key, status)
VALUES ('incremental', 'incremental', 'succeeded')
ON CONFLICT (job_key) WHERE job_key IS NOT NULL DO NOTHING;

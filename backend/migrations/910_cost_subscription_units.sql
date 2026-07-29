-- A subscription unit represents one purchased fixed-cost subscription.
-- Multiple platform accounts may share it and are charged only once.

CREATE TABLE IF NOT EXISTS cost_subscription_units (
    id BIGSERIAL PRIMARY KEY,
    plan_id BIGINT NOT NULL REFERENCES cost_plans(id) ON DELETE RESTRICT,
    name VARCHAR(120) NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cost_subscription_units_name_check CHECK (BTRIM(name) <> ''),
    CONSTRAINT cost_subscription_units_period_check CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_cost_subscription_units_plan_effective
    ON cost_subscription_units (plan_id, effective_from, effective_to);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cost_subscription_units_open_name
    ON cost_subscription_units (plan_id, LOWER(name)) WHERE effective_to IS NULL;

ALTER TABLE account_cost_configs
    ADD COLUMN IF NOT EXISTS subscription_unit_id BIGINT
        REFERENCES cost_subscription_units(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_account_cost_configs_subscription_unit
    ON account_cost_configs (subscription_unit_id, effective_from, effective_to)
    WHERE subscription_unit_id IS NOT NULL;

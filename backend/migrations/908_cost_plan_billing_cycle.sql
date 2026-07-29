-- Preserve the entered fixed-plan billing cycle and amount while keeping a normalized monthly cost.

ALTER TABLE cost_plan_versions
    ADD COLUMN IF NOT EXISTS billing_cycle VARCHAR(10) NOT NULL DEFAULT 'monthly',
    ADD COLUMN IF NOT EXISTS fixed_unit_cost_cny NUMERIC(20,12) NOT NULL DEFAULT 0;

UPDATE cost_plan_versions
SET fixed_unit_cost_cny = monthly_unit_cost_cny
WHERE fixed_unit_cost_cny = 0 AND monthly_unit_cost_cny <> 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'cost_plan_versions_billing_cycle_check'
    ) THEN
        ALTER TABLE cost_plan_versions
            ADD CONSTRAINT cost_plan_versions_billing_cycle_check
            CHECK (billing_cycle IN ('monthly', 'yearly'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'cost_plan_versions_fixed_unit_cost_check'
    ) THEN
        ALTER TABLE cost_plan_versions
            ADD CONSTRAINT cost_plan_versions_fixed_unit_cost_check
            CHECK (fixed_unit_cost_cny >= 0);
    END IF;
END $$;

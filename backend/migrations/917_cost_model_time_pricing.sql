ALTER TABLE cost_model_prices
    ADD COLUMN IF NOT EXISTS time_pricing JSONB NULL;

COMMENT ON COLUMN cost_model_prices.time_pricing IS
    'Optional timezone-aware daily pricing periods; each period supplies a multiplier.';

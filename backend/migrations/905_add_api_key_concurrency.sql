ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency_limit INT NOT NULL DEFAULT 0;

ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_concurrency_limit_non_negative CHECK (concurrency_limit >= 0);

COMMENT ON COLUMN api_keys.concurrency_limit IS 'Concurrent request limit for this API key (0 = unlimited)';

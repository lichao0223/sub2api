ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency INT NOT NULL DEFAULT 0 CHECK (concurrency >= 0);

COMMENT ON COLUMN api_keys.concurrency IS 'Concurrent request limit for this API key (0 = unlimited)';

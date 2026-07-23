CREATE TABLE IF NOT EXISTS user_usage_migrations (
    source_user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    target_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (source_user_id <> target_user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_usage_migrations_target
    ON user_usage_migrations (target_user_id);

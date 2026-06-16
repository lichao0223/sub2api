CREATE TABLE IF NOT EXISTS external_user_mappings (
    id BIGSERIAL PRIMARY KEY,
    external_user_id VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    username_snapshot VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS external_user_mappings_external_user_id_active_uidx
    ON external_user_mappings (external_user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS external_user_mappings_user_id_idx
    ON external_user_mappings (user_id);

CREATE INDEX IF NOT EXISTS external_user_mappings_api_key_id_idx
    ON external_user_mappings (api_key_id);

ALTER TABLE external_user_mappings
    ADD COLUMN IF NOT EXISTS external_organization_id VARCHAR(255) NOT NULL DEFAULT '';

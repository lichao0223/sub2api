-- Asynchronous AI work insights. Raw request text lives in Redis only.

CREATE TABLE IF NOT EXISTS ai_work_insight_samples (
    id                    BIGSERIAL PRIMARY KEY,
    request_fingerprint   VARCHAR(64) NOT NULL UNIQUE,
    request_id            VARCHAR(128) NOT NULL DEFAULT '',
    user_id               BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot     VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot   VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id            BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
    group_id              BIGINT REFERENCES groups(id) ON DELETE SET NULL,
    provider              VARCHAR(64) NOT NULL DEFAULT '',
    protocol              VARCHAR(64) NOT NULL DEFAULT '',
    endpoint              VARCHAR(128) NOT NULL DEFAULT '',
    requested_model       VARCHAR(255) NOT NULL DEFAULT '',
    local_date            DATE NOT NULL,
    active_session_id     VARCHAR(64) NOT NULL,
    batch_id              BIGINT,
    sample_reason         VARCHAR(32) NOT NULL,
    prompt_hash           VARCHAR(64) NOT NULL,
    estimated_tokens      INT NOT NULL DEFAULT 0,
    prompt_chars          INT NOT NULL DEFAULT 0,
    analyzed_chars        INT NOT NULL DEFAULT 0,
    redacted_preview      TEXT NOT NULL DEFAULT '',
    status                VARCHAR(32) NOT NULL DEFAULT 'pending_batch',
    attempts              INT NOT NULL DEFAULT 0,
    next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_version         BIGINT NOT NULL DEFAULT 0,
    error_code            VARCHAR(64) NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ai_work_insight_samples_status
        CHECK (status IN ('staging','pending_batch','batched','analyzed','failed','dropped')),
    CONSTRAINT chk_ai_work_insight_samples_reason
        CHECK (sample_reason IN ('session_coverage','rate')),
    CONSTRAINT chk_ai_work_insight_samples_nonnegative
        CHECK (estimated_tokens >= 0 AND prompt_chars >= 0 AND analyzed_chars >= 0 AND attempts >= 0 AND claim_version >= 0)
);

CREATE TABLE IF NOT EXISTS ai_work_insight_batches (
    id                       BIGSERIAL PRIMARY KEY,
    user_id                  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot        VARCHAR(255) NOT NULL DEFAULT '',
    local_date               DATE NOT NULL,
    active_session_id        VARCHAR(64) NOT NULL,
    first_sample_id          BIGINT NOT NULL,
    last_sample_id           BIGINT NOT NULL,
    sample_count             INT NOT NULL,
    estimated_input_tokens   INT NOT NULL DEFAULT 0,
    trigger_reason           VARCHAR(32) NOT NULL,
    status                   VARCHAR(32) NOT NULL DEFAULT 'queued',
    attempts                 INT NOT NULL DEFAULT 0,
    next_attempt_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_version            BIGINT NOT NULL DEFAULT 0,
    base_summary_version     BIGINT NOT NULL DEFAULT 0,
    work_summary             TEXT NOT NULL DEFAULT '',
    task_categories          JSONB NOT NULL DEFAULT '[]'::jsonb,
    explicit_projects        JSONB NOT NULL DEFAULT '[]'::jsonb,
    explicit_modules         JSONB NOT NULL DEFAULT '[]'::jsonb,
    change_types             JSONB NOT NULL DEFAULT '[]'::jsonb,
    business_topics          JSONB NOT NULL DEFAULT '[]'::jsonb,
    representative_items     JSONB NOT NULL DEFAULT '[]'::jsonb,
    evidence_level           VARCHAR(16) NOT NULL DEFAULT 'unknown',
    analyzer_model           VARCHAR(255) NOT NULL DEFAULT '',
    analyzer_input_tokens    BIGINT NOT NULL DEFAULT 0,
    analyzer_output_tokens   BIGINT NOT NULL DEFAULT 0,
    chunk_count              INT NOT NULL DEFAULT 0,
    analyzed_at              TIMESTAMPTZ,
    error_code               VARCHAR(64) NOT NULL DEFAULT '',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ai_work_insight_batch_watermark
        UNIQUE (user_id, local_date, first_sample_id, last_sample_id),
    CONSTRAINT chk_ai_work_insight_batches_status
        CHECK (status IN ('queued','processing','retry','done','failed','dropped')),
    CONSTRAINT chk_ai_work_insight_batches_trigger
        CHECK (trigger_reason IN ('idle','max_wait','sample_limit','token_limit','fixed','manual','finalize')),
    CONSTRAINT chk_ai_work_insight_batches_evidence
        CHECK (evidence_level IN ('explicit','unknown')),
    CONSTRAINT chk_ai_work_insight_batches_json
        CHECK (jsonb_typeof(task_categories)='array' AND jsonb_typeof(explicit_projects)='array'
            AND jsonb_typeof(explicit_modules)='array' AND jsonb_typeof(change_types)='array'
            AND jsonb_typeof(business_topics)='array' AND jsonb_typeof(representative_items)='array'),
    CONSTRAINT chk_ai_work_insight_batches_nonnegative
        CHECK (first_sample_id > 0 AND last_sample_id >= first_sample_id AND sample_count > 0
            AND estimated_input_tokens >= 0 AND attempts >= 0 AND claim_version >= 0
            AND base_summary_version >= 0 AND analyzer_input_tokens >= 0
            AND analyzer_output_tokens >= 0 AND chunk_count >= 0)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_ai_work_insight_samples_batch'
    ) THEN
        ALTER TABLE ai_work_insight_samples
            ADD CONSTRAINT fk_ai_work_insight_samples_batch
            FOREIGN KEY (batch_id) REFERENCES ai_work_insight_batches(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS ai_user_daily_work_insights (
    id                             BIGSERIAL PRIMARY KEY,
    user_id                        BIGINT REFERENCES users(id) ON DELETE SET NULL,
    username_snapshot              VARCHAR(255) NOT NULL DEFAULT '',
    insight_date                   DATE NOT NULL,
    business_request_count         BIGINT NOT NULL DEFAULT 0,
    business_total_tokens          BIGINT NOT NULL DEFAULT 0,
    business_input_tokens          BIGINT NOT NULL DEFAULT 0,
    business_output_tokens         BIGINT NOT NULL DEFAULT 0,
    business_cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    business_cache_read_tokens     BIGINT NOT NULL DEFAULT 0,
    business_error_count           BIGINT NOT NULL DEFAULT 0,
    average_duration_ms            BIGINT NOT NULL DEFAULT 0,
    p95_duration_ms                BIGINT NOT NULL DEFAULT 0,
    model_usage                    JSONB NOT NULL DEFAULT '{}'::jsonb,
    sample_count                   INT NOT NULL DEFAULT 0,
    failed_sample_count            INT NOT NULL DEFAULT 0,
    eligible_active_session_count  INT NOT NULL DEFAULT 0,
    covered_active_session_count   INT NOT NULL DEFAULT 0,
    task_category_stats            JSONB NOT NULL DEFAULT '{}'::jsonb,
    explicit_projects              JSONB NOT NULL DEFAULT '[]'::jsonb,
    explicit_modules               JSONB NOT NULL DEFAULT '[]'::jsonb,
    change_types                   JSONB NOT NULL DEFAULT '[]'::jsonb,
    business_topics                JSONB NOT NULL DEFAULT '[]'::jsonb,
    daily_summary                  TEXT NOT NULL DEFAULT '',
    last_analyzed_sample_id        BIGINT NOT NULL DEFAULT 0,
    summary_version                BIGINT NOT NULL DEFAULT 0,
    last_analyzed_at               TIMESTAMPTZ,
    finalized_at                   TIMESTAMPTZ,
    computed_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_ai_user_daily_work_insights UNIQUE (user_id, insight_date),
    CONSTRAINT chk_ai_user_daily_work_insights_json
        CHECK (jsonb_typeof(model_usage)='object' AND jsonb_typeof(task_category_stats)='object'
            AND jsonb_typeof(explicit_projects)='array' AND jsonb_typeof(explicit_modules)='array'
            AND jsonb_typeof(change_types)='array' AND jsonb_typeof(business_topics)='array'),
    CONSTRAINT chk_ai_user_daily_work_insights_nonnegative
        CHECK (business_request_count >= 0 AND business_total_tokens >= 0
            AND business_input_tokens >= 0 AND business_output_tokens >= 0
            AND business_cache_creation_tokens >= 0 AND business_cache_read_tokens >= 0
            AND business_error_count >= 0 AND average_duration_ms >= 0 AND p95_duration_ms >= 0
            AND sample_count >= 0
            AND failed_sample_count >= 0 AND eligible_active_session_count >= 0
            AND covered_active_session_count >= 0
            AND covered_active_session_count <= eligible_active_session_count
            AND last_analyzed_sample_id >= 0 AND summary_version >= 0)
);

-- Keep pre-release installations of this idempotent migration upgradeable.
ALTER TABLE ai_user_daily_work_insights
    ADD COLUMN IF NOT EXISTS business_input_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS business_output_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS business_cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS business_cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS business_error_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS average_duration_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS p95_duration_ms BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS model_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS business_topics JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_pending
    ON ai_work_insight_samples(id) WHERE status='pending_batch';
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_expiry
    ON ai_work_insight_samples(created_at, id) WHERE status IN ('staging','pending_batch');
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_cleanup
    ON ai_work_insight_samples(created_at, id);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_user_date
    ON ai_work_insight_samples(user_id, local_date, id);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_session
    ON ai_work_insight_samples(user_id, local_date, active_session_id, id) WHERE status='pending_batch';
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_samples_batch
    ON ai_work_insight_samples(batch_id, id);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_batches_schedule
    ON ai_work_insight_batches(status, next_attempt_at, id);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_batches_date_status
    ON ai_work_insight_batches(local_date, status);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_batches_cleanup
    ON ai_work_insight_batches(created_at, id);
CREATE INDEX IF NOT EXISTS idx_ai_work_insight_batches_user_date
    ON ai_work_insight_batches(user_id, local_date, id);
CREATE INDEX IF NOT EXISTS idx_ai_user_daily_work_insights_date
    ON ai_user_daily_work_insights(insight_date DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_ai_user_daily_work_insights_categories
    ON ai_user_daily_work_insights USING GIN(task_category_stats);

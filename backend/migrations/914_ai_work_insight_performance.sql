-- Apply production-safe defaults to existing AI work insight configurations.
UPDATE settings
SET value = (value::jsonb || jsonb_build_object(
        'sample_retention_days', LEAST(COALESCE((value::jsonb ->> 'sample_retention_days')::int, 30), 30),
        'user_daily_limit', GREATEST(COALESCE((value::jsonb ->> 'user_daily_limit')::int, 0), 5000),
        'global_daily_limit', GREATEST(COALESCE((value::jsonb ->> 'global_daily_limit')::int, 0), 200000),
        'analysis_idle_minutes', GREATEST(COALESCE((value::jsonb ->> 'analysis_idle_minutes')::int, 0), 15),
        'analysis_max_wait_minutes', GREATEST(COALESCE((value::jsonb ->> 'analysis_max_wait_minutes')::int, 0), 60),
        'max_samples_per_batch', GREATEST(COALESCE((value::jsonb ->> 'max_samples_per_batch')::int, 0), 50),
        'max_input_tokens', CASE
            WHEN COALESCE((value::jsonb ->> 'context_window_tokens')::int, 128000)
                    - COALESCE((value::jsonb ->> 'reserved_output_tokens')::int, 4000) - 4000
                    - COALESCE((value::jsonb ->> 'context_window_tokens')::int, 128000) * 15 / 100 >= 64000
                THEN GREATEST(COALESCE((value::jsonb ->> 'max_input_tokens')::int, 0), 64000)
            ELSE COALESCE((value::jsonb ->> 'max_input_tokens')::int, 24000)
        END,
        'analysis_timeout_seconds', GREATEST(COALESCE((value::jsonb ->> 'analysis_timeout_seconds')::int, 0), 60)
    ))::text,
    updated_at = NOW()
WHERE key = 'ai_work_insight_config';

-- Upgrade only the previous product default; preserve administrator-selected rates.
UPDATE settings
SET value = jsonb_set(value::jsonb, '{sample_rate}', '20'::jsonb, true)::text,
    updated_at = NOW()
WHERE key = 'ai_work_insight_config'
  AND COALESCE(value::jsonb ->> 'sample_rate', '2') = '2';

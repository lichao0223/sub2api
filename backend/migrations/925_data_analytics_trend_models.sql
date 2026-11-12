ALTER TABLE analytics_org_daily ADD COLUMN model_tokens JSONB;
ALTER TABLE analytics_org_daily_versions ADD COLUMN model_tokens JSONB;

CREATE INDEX analytics_org_daily_versions_scope_idx
  ON analytics_org_daily_versions(month,generation,org_id,day);

-- One retained day per month republishes the whole month with model snapshots.
INSERT INTO analytics_dirty_days(day,reason)
SELECT MIN(s.day),'trend'
FROM analytics_bucket_state s
WHERE NOT s.detail_pruned
  AND EXISTS(SELECT 1 FROM analytics_publications p WHERE p.month=DATE_TRUNC('month',s.day)::date)
  AND NOT EXISTS(SELECT 1 FROM analytics_bucket_state pruned
    WHERE pruned.detail_pruned AND DATE_TRUNC('month',pruned.day)=DATE_TRUNC('month',s.day))
GROUP BY DATE_TRUNC('month',s.day)
ON CONFLICT(day) DO NOTHING;

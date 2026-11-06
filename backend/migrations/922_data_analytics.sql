CREATE TABLE analytics_runtime_state (
    id INTEGER PRIMARY KEY CHECK(id=1), activated BOOLEAN NOT NULL DEFAULT FALSE,
    seeded BOOLEAN NOT NULL DEFAULT FALSE, started_at TIMESTAMPTZ, directory_synced_at TIMESTAMPTZ, directory_revision BIGINT NOT NULL DEFAULT 0,
    last_success_at TIMESTAMPTZ, raw_history_start DATE, last_error TEXT NOT NULL DEFAULT ''
);
INSERT INTO analytics_runtime_state(id) VALUES(1);

CREATE TABLE analytics_directory_imports (
    id TEXT PRIMARY KEY, digest TEXT NOT NULL, actor_id BIGINT NOT NULL,
    data JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW()+INTERVAL '24 hours',
    applied_at TIMESTAMPTZ
);
CREATE INDEX ON analytics_directory_imports(expires_at);
CREATE TABLE analytics_directory_versions (
    id BIGSERIAL PRIMARY KEY, digest TEXT NOT NULL, data JSONB NOT NULL,
    effective_at TIMESTAMPTZ NOT NULL UNIQUE, actor_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE analytics_identity_mappings (
    user_id BIGINT PRIMARY KEY, person_id TEXT NOT NULL UNIQUE,
    basis TEXT NOT NULL, actor_id BIGINT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE analytics_assignments (
    id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, person_id TEXT NOT NULL,
    name TEXT NOT NULL, org_ids TEXT[] NOT NULL, org_names TEXT[] NOT NULL,
    basis TEXT NOT NULL, effective_from TIMESTAMPTZ NOT NULL, effective_to TIMESTAMPTZ,
    CHECK(effective_to IS NULL OR effective_to>effective_from)
);
CREATE UNIQUE INDEX ON analytics_assignments(user_id) WHERE effective_to IS NULL;
CREATE INDEX ON analytics_assignments(user_id,effective_from,effective_to);

CREATE TABLE analytics_dirty_days (
    day DATE PRIMARY KEY, version BIGINT NOT NULL DEFAULT 1,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), reason TEXT NOT NULL DEFAULT 'usage',
    attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', source_change_from TIMESTAMPTZ
);
CREATE TABLE analytics_bucket_state (
    day DATE PRIMARY KEY, computed_at TIMESTAMPTZ NOT NULL,
    request_count BIGINT NOT NULL, pending_count BIGINT NOT NULL,
    archived_count BIGINT NOT NULL DEFAULT 0, archived_until TIMESTAMPTZ, detail_pruned BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE analytics_daily (
    day DATE NOT NULL, user_id BIGINT NOT NULL, attribution BIGINT NOT NULL,
    model TEXT NOT NULL, mode TEXT NOT NULL, source_id BIGINT NOT NULL,
    source TEXT NOT NULL, issue TEXT NOT NULL,
    input BIGINT NOT NULL, output BIGINT NOT NULL, cache_write BIGINT NOT NULL, cache_read BIGINT NOT NULL,
    requests BIGINT NOT NULL, valid_requests BIGINT NOT NULL,
    input_cost NUMERIC(20,12) NOT NULL, output_cost NUMERIC(20,12) NOT NULL,
    write_cost NUMERIC(20,12) NOT NULL, read_cost NUMERIC(20,12) NOT NULL,
    image_cost NUMERIC(20,12) NOT NULL, request_cost NUMERIC(20,12) NOT NULL,
    PRIMARY KEY(day,user_id,attribution,model,mode,source_id,issue)
);
CREATE INDEX ON analytics_daily(user_id,day);
CREATE TABLE analytics_archived_daily (LIKE analytics_daily INCLUDING ALL);
CREATE TABLE analytics_monthly (
    LIKE analytics_daily INCLUDING DEFAULTS,
    generation BIGINT NOT NULL, fixed NUMERIC(20,12) NOT NULL DEFAULT 0,
    PRIMARY KEY(day,generation,user_id,attribution,model,mode,source_id,issue)
);
CREATE INDEX ON analytics_monthly(user_id,day,generation);
CREATE INDEX ON analytics_monthly(day,generation,mode,source_id,user_id);
CREATE INDEX ON analytics_monthly(day,attribution,generation);
CREATE TABLE analytics_publications (
    month DATE NOT NULL, generation BIGSERIAL NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(month,generation)
);
CREATE TABLE analytics_org_daily (
    day DATE NOT NULL, org_id TEXT NOT NULL, level INTEGER NOT NULL, tokens BIGINT NOT NULL,
    PRIMARY KEY(day,org_id,level)
);
CREATE TABLE analytics_org_daily_versions (
    month DATE NOT NULL, generation BIGINT NOT NULL, day DATE NOT NULL,
    org_id TEXT NOT NULL, level INTEGER NOT NULL, tokens BIGINT NOT NULL,
    PRIMARY KEY(month,generation,day,org_id,level)
);
CREATE TABLE analytics_fixed_totals (
    month DATE NOT NULL, generation BIGINT NOT NULL, unit_id BIGINT NOT NULL,
    name TEXT NOT NULL, total NUMERIC(20,12) NOT NULL, participants INTEGER NOT NULL,
    public_cost NUMERIC(20,12) NOT NULL, PRIMARY KEY(month,generation,unit_id)
);
CREATE TABLE analytics_fixed_allocations (
    month DATE NOT NULL, generation BIGINT NOT NULL, unit_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL, amount NUMERIC(20,12) NOT NULL,
    PRIMARY KEY(month,generation,unit_id,user_id)
);
CREATE TABLE analytics_rebuild_audit (
    id BIGSERIAL PRIMARY KEY, start_day DATE NOT NULL, end_day DATE NOT NULL,
    actor_id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A SQL table function is inlined into both aggregation and retention queries.
-- Deleted request rows are folded into a bounded daily baseline, never copied.
CREATE FUNCTION analytics_resolve_log(ul usage_logs) RETURNS SETOF analytics_daily
LANGUAGE sql STABLE AS $$
WITH resolved AS (
    SELECT COALESCE(m.target_user_id,ul.user_id) uid, a.id aid,
      COALESCE(NULLIF(BTRIM(ul.upstream_model),''),NULLIF(BTRIM(ul.model),''),'(unknown)') model,
      COALESCE(c.cost_mode,'metered') mode,
      CASE WHEN c.cost_mode='fixed' THEN COALESCE(c.subscription_unit_id,0) ELSE COALESCE(c.plan_id,0) END source_id,
      COALESCE(su.name,p.name,CASE WHEN c.cost_mode='excluded' THEN '不计成本' ELSE '待配置' END) source,
      CASE WHEN c.cost_mode='excluded' THEN ''
        WHEN c.id IS NULL THEN 'missing_account_config'
        WHEN c.cost_mode='fixed' AND (su.id IS NULL OR ul.created_at<su.effective_from OR (su.effective_to IS NOT NULL AND ul.created_at>=su.effective_to)) THEN 'missing_subscription'
        WHEN c.cost_mode='fixed' AND uv.id IS NULL THEN 'missing_subscription_version'
        WHEN c.cost_mode='fixed' THEN ''
        WHEN v.id IS NULL THEN 'missing_plan_version'
        WHEN mp.id IS NULL THEN 'missing_model_price' ELSE '' END issue,
      mp.*,COALESCE((SELECT NULLIF(period->>'multiplier','')::numeric
        FROM jsonb_array_elements(COALESCE(mp.time_pricing->'periods','[]'::jsonb)) period
        WHERE (COALESCE((mp.time_pricing->>'weekdays_only')::boolean,FALSE)=FALSE
          OR EXTRACT(ISODOW FROM (ul.created_at AT TIME ZONE COALESCE(mp.time_pricing->>'timezone','Asia/Shanghai'))) BETWEEN 1 AND 5)
          AND (ul.created_at AT TIME ZONE COALESCE(mp.time_pricing->>'timezone','Asia/Shanghai'))::time >= (period->>'start_time')::time
          AND ((period->>'end_time') IN ('00:00','00:00:00')
            OR (ul.created_at AT TIME ZONE COALESCE(mp.time_pricing->>'timezone','Asia/Shanghai'))::time < (period->>'end_time')::time)
        LIMIT 1),1) multiplier
    FROM (SELECT 1) seed
    LEFT JOIN user_usage_migrations m ON m.source_user_id=ul.user_id
    LEFT JOIN LATERAL(SELECT * FROM analytics_assignments x WHERE x.user_id=COALESCE(m.target_user_id,ul.user_id)
        AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)
        ORDER BY x.effective_from DESC LIMIT 1)a ON TRUE
    LEFT JOIN LATERAL(SELECT * FROM account_cost_configs x WHERE x.account_id=ul.account_id
        AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)
        ORDER BY x.effective_from DESC LIMIT 1)c ON TRUE
    LEFT JOIN cost_plans p ON p.id=c.plan_id
    LEFT JOIN cost_subscription_units su ON su.id=c.subscription_unit_id
    LEFT JOIN LATERAL(SELECT x.id FROM cost_subscription_unit_versions x WHERE x.subscription_unit_id=c.subscription_unit_id
        AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)
        ORDER BY x.effective_from DESC LIMIT 1)uv ON TRUE
    LEFT JOIN LATERAL(SELECT * FROM cost_plan_versions x WHERE x.plan_id=c.plan_id
        AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)
        ORDER BY x.version_no DESC LIMIT 1)v ON TRUE
    LEFT JOIN cost_model_prices mp ON mp.plan_version_id=v.id
        AND mp.upstream_model=COALESCE(NULLIF(BTRIM(ul.upstream_model),''),ul.model)
)
SELECT (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date,uid,COALESCE(aid,0),model,mode,source_id,source,issue,
  GREATEST(ul.input_tokens,0)::bigint,GREATEST(ul.output_tokens,0)::bigint,
  GREATEST(ul.cache_creation_tokens,0)::bigint,GREATEST(ul.cache_read_tokens,0)::bigint,1::bigint,
  CASE WHEN COALESCE(ul.input_tokens,0)::bigint+COALESCE(ul.output_tokens,0)+COALESCE(ul.cache_creation_tokens,0)+COALESCE(ul.cache_read_tokens,0)>0
    OR COALESCE(ul.image_count,0)>0 OR COALESCE(ul.video_count,0)>0
    OR GREATEST(ul.image_input_tokens,0)>0 OR GREATEST(ul.image_output_tokens,0)>0
    OR ul.billing_mode IN('per_request','request','hybrid') OR billing_mode IN('request','hybrid') THEN 1 ELSE 0 END::bigint,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('token','hybrid') THEN ROUND(GREATEST(ul.input_tokens,0)*input_price_cny/1000000*multiplier,12) ELSE 0 END,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('token','hybrid') THEN ROUND(GREATEST(ul.output_tokens,0)*output_price_cny/1000000*multiplier,12) ELSE 0 END,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('token','hybrid') THEN ROUND(GREATEST(ul.cache_creation_tokens,0)*cache_write_price_cny/1000000*multiplier,12) ELSE 0 END,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('token','hybrid') THEN ROUND(GREATEST(ul.cache_read_tokens,0)*cache_read_price_cny/1000000*multiplier,12) ELSE 0 END,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('token','hybrid') THEN
    ROUND((GREATEST(ul.image_input_tokens,0)*image_input_price_cny+GREATEST(ul.image_output_tokens,0)*image_output_price_cny)/1000000*multiplier,12) ELSE 0 END,
  CASE WHEN mode='metered' AND issue='' AND billing_mode IN('request','hybrid') THEN ROUND(per_request_price_cny*multiplier,12) ELSE 0 END
FROM resolved
$$;

CREATE FUNCTION analytics_mark_usage_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO analytics_dirty_days(day)
  SELECT DISTINCT (created_at AT TIME ZONE 'Asia/Shanghai')::date FROM analytics_new
    WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date
      AND (created_at AT TIME ZONE 'Asia/Shanghai')::date<=(NOW() AT TIME ZONE 'Asia/Shanghai')::date
  ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,changed_at=NOW(),attempts=0,last_error='';
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_usage_insert AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS analytics_new FOR EACH STATEMENT EXECUTE FUNCTION analytics_mark_usage_insert();

CREATE FUNCTION analytics_mark_usage_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO analytics_dirty_days(day)
  SELECT DISTINCT (created_at AT TIME ZONE 'Asia/Shanghai')::date FROM(
    SELECT created_at FROM analytics_new UNION SELECT created_at FROM analytics_old) x
    WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date
      AND (created_at AT TIME ZONE 'Asia/Shanghai')::date<=(NOW() AT TIME ZONE 'Asia/Shanghai')::date
  ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,changed_at=NOW(),attempts=0,last_error='';
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_usage_update AFTER UPDATE ON usage_logs
REFERENCING NEW TABLE AS analytics_new OLD TABLE AS analytics_old
FOR EACH STATEMENT EXECUTE FUNCTION analytics_mark_usage_update();

CREATE FUNCTION analytics_archive_deleted() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT (SELECT activated FROM analytics_runtime_state WHERE id=1) THEN RETURN NULL; END IF;
  IF NOT pg_try_advisory_xact_lock(9072028) THEN RAISE EXCEPTION '数据分析正在核算，稍后重试日志清理'; END IF;
  IF EXISTS(SELECT 1 FROM (SELECT DISTINCT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day FROM analytics_old) x
    LEFT JOIN analytics_bucket_state s USING(day)
    WHERE x.day>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date
      AND (s.day IS NULL OR s.pending_count>0 OR EXISTS(SELECT 1 FROM analytics_dirty_days d WHERE d.day=x.day))) THEN
    RAISE EXCEPTION '数据分析尚未完整核算，暂不能清理该范围的请求日志';
  END IF;
  IF current_setting('sub2api.analytics_delete_mode',TRUE)='correction' THEN
    IF EXISTS(SELECT 1 FROM analytics_bucket_state s WHERE s.detail_pruned AND DATE_TRUNC('month',s.day) IN
      (SELECT DISTINCT DATE_TRUNC('month',created_at AT TIME ZONE 'Asia/Shanghai') FROM analytics_old)) THEN
      RAISE EXCEPTION '历史日明细已清理，无法精确修正已发布统计';
    END IF;
    INSERT INTO analytics_dirty_days(day,reason)
    SELECT DISTINCT (created_at AT TIME ZONE 'Asia/Shanghai')::date,'correction' FROM analytics_old
      WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date
      AND (created_at AT TIME ZONE 'Asia/Shanghai')::date<=(NOW() AT TIME ZONE 'Asia/Shanghai')::date
    ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,changed_at=NOW(),attempts=0,last_error='';
    RETURN NULL;
  END IF;
  -- Outside daily retention no baseline is kept: freeze only after the entire
  -- month is accounted, before any raw subset can disappear.
  IF EXISTS(
    SELECT 1 FROM (SELECT DISTINCT DATE_TRUNC('month',created_at AT TIME ZONE 'Asia/Shanghai')::date AS month FROM analytics_old
      WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date<(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '12 months')::date
        AND (created_at AT TIME ZONE 'Asia/Shanghai')::date>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date) x
    WHERE EXISTS(SELECT 1 FROM analytics_dirty_days d WHERE DATE_TRUNC('month',d.day)::date=x.month)
      OR (SELECT COUNT(*) FROM analytics_bucket_state s WHERE s.day>=x.month AND s.day<(x.month+INTERVAL '1 month')::date AND s.pending_count=0)
        <>EXTRACT(DAY FROM x.month+INTERVAL '1 month'-INTERVAL '1 day')
  ) THEN RAISE EXCEPTION '历史月份尚未整月完成核算，暂不能清理原始请求'; END IF;
  UPDATE analytics_bucket_state s SET detail_pruned=TRUE
    WHERE DATE_TRUNC('month',s.day)::date IN(SELECT DISTINCT DATE_TRUNC('month',created_at AT TIME ZONE 'Asia/Shanghai')::date FROM analytics_old
      WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date<(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '12 months')::date);
  INSERT INTO analytics_archived_daily
  SELECT f.day,f.user_id,f.attribution,f.model,f.mode,f.source_id,MAX(f.source),f.issue,
    SUM(f.input),SUM(f.output),SUM(f.cache_write),SUM(f.cache_read),SUM(f.requests),SUM(f.valid_requests),
    SUM(f.input_cost),SUM(f.output_cost),SUM(f.write_cost),SUM(f.read_cost),SUM(f.image_cost),SUM(f.request_cost)
  FROM analytics_old o CROSS JOIN LATERAL analytics_resolve_log(o::usage_logs) f
  WHERE f.day>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '12 months')::date
  GROUP BY f.day,f.user_id,f.attribution,f.model,f.mode,f.source_id,f.issue
  ON CONFLICT(day,user_id,attribution,model,mode,source_id,issue) DO UPDATE SET
    input=analytics_archived_daily.input+EXCLUDED.input,output=analytics_archived_daily.output+EXCLUDED.output,
    cache_write=analytics_archived_daily.cache_write+EXCLUDED.cache_write,cache_read=analytics_archived_daily.cache_read+EXCLUDED.cache_read,
    requests=analytics_archived_daily.requests+EXCLUDED.requests,valid_requests=analytics_archived_daily.valid_requests+EXCLUDED.valid_requests,
    input_cost=analytics_archived_daily.input_cost+EXCLUDED.input_cost,output_cost=analytics_archived_daily.output_cost+EXCLUDED.output_cost,
    write_cost=analytics_archived_daily.write_cost+EXCLUDED.write_cost,read_cost=analytics_archived_daily.read_cost+EXCLUDED.read_cost,
    image_cost=analytics_archived_daily.image_cost+EXCLUDED.image_cost,request_cost=analytics_archived_daily.request_cost+EXCLUDED.request_cost;
  UPDATE analytics_bucket_state s SET archived_count=s.archived_count+x.n,archived_until=GREATEST(s.archived_until,x.last_at)
  FROM(SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date AS day,COUNT(*) n,MAX(created_at) last_at FROM analytics_old GROUP BY 1)x WHERE s.day=x.day;
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_usage_delete AFTER DELETE ON usage_logs
REFERENCING OLD TABLE AS analytics_old FOR EACH STATEMENT EXECUTE FUNCTION analytics_archive_deleted();

-- Invalidate the affected effective interval only; closing a price version
-- today must not enqueue already archived months whose price did not change.
CREATE FUNCTION analytics_dirty_cost_interval(starts TIMESTAMPTZ, stops TIMESTAMPTZ, fill_gaps BOOLEAN) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF starts IS NULL OR starts>NOW() OR stops<=starts THEN RETURN; END IF;
  PERFORM pg_advisory_xact_lock(9072028);
  IF EXISTS(SELECT 1 FROM analytics_bucket_state WHERE archived_until>=starts
      AND day<=((stops-INTERVAL '1 microsecond') AT TIME ZONE 'Asia/Shanghai')::date) THEN
    RAISE EXCEPTION '生效范围内已有请求归档，无法完整修订历史成本；请选择新的生效时间';
  END IF;
  INSERT INTO analytics_dirty_days(day,reason,source_change_from)
  SELECT d::date,'cost',starts FROM generate_series(
    GREATEST((starts AT TIME ZONE 'Asia/Shanghai')::date,
      (DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date),
    LEAST(((stops-INTERVAL '1 microsecond') AT TIME ZONE 'Asia/Shanghai')::date,(NOW() AT TIME ZONE 'Asia/Shanghai')::date), INTERVAL '1 day') d
  WHERE fill_gaps OR EXISTS(SELECT 1 FROM analytics_bucket_state s WHERE s.day=d::date)
  ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,reason='cost',changed_at=NOW(),attempts=0,last_error='',
    source_change_from=LEAST(analytics_dirty_days.source_change_from,EXCLUDED.source_change_from);
END $$;
CREATE FUNCTION analytics_mark_cost_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n JSONB; o JSONB; starts TIMESTAMPTZ; stops TIMESTAMPTZ;
BEGIN
  IF TG_OP<>'DELETE' THEN n=to_jsonb(NEW); END IF;
  IF TG_OP<>'INSERT' THEN o=to_jsonb(OLD); END IF;
  IF TG_TABLE_NAME='cost_model_prices' THEN
    SELECT MIN(effective_from),MAX(COALESCE(effective_to,'infinity'::timestamptz)) INTO starts,stops
      FROM cost_plan_versions WHERE id IN((n->>'plan_version_id')::bigint,(o->>'plan_version_id')::bigint);
  ELSE
    starts=LEAST((n->>'effective_from')::timestamptz,(o->>'effective_from')::timestamptz);
    stops=CASE TG_OP WHEN 'INSERT' THEN COALESCE((n->>'effective_to')::timestamptz,'infinity') WHEN 'DELETE' THEN COALESCE((o->>'effective_to')::timestamptz,'infinity') ELSE GREATEST(COALESCE((n->>'effective_to')::timestamptz,'infinity'),COALESCE((o->>'effective_to')::timestamptz,'infinity')) END;
    IF TG_OP='UPDATE' AND (n-'effective_to'-'updated_at')=(o-'effective_to'-'updated_at') THEN
      starts=LEAST(COALESCE((n->>'effective_to')::timestamptz,'infinity'),COALESCE((o->>'effective_to')::timestamptz,'infinity'));
    END IF;
  END IF;
  PERFORM analytics_dirty_cost_interval(starts,stops,TG_TABLE_NAME='cost_subscription_units');
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_cost_config AFTER INSERT OR UPDATE OR DELETE ON account_cost_configs
FOR EACH ROW EXECUTE FUNCTION analytics_mark_cost_change();
CREATE TRIGGER analytics_cost_price AFTER INSERT OR UPDATE OR DELETE ON cost_model_prices
FOR EACH ROW EXECUTE FUNCTION analytics_mark_cost_change();
CREATE TRIGGER analytics_cost_version AFTER INSERT OR UPDATE OR DELETE ON cost_plan_versions
FOR EACH ROW EXECUTE FUNCTION analytics_mark_cost_change();
CREATE TRIGGER analytics_fixed_version AFTER INSERT OR UPDATE OR DELETE ON cost_subscription_unit_versions
FOR EACH ROW EXECUTE FUNCTION analytics_mark_cost_change();
CREATE TRIGGER analytics_fixed_unit AFTER INSERT OR UPDATE OR DELETE ON cost_subscription_units
FOR EACH ROW EXECUTE FUNCTION analytics_mark_cost_change();

-- Identity merges only change canonical ownership, so retained monthly facts
-- suffice even after source and daily detail expiration.
CREATE FUNCTION analytics_mark_identity_merge() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(9072028);
  UPDATE analytics_identity_mappings i SET user_id=m.target_user_id,updated_at=NOW()
    FROM user_usage_migrations m WHERE i.user_id=m.source_user_id
      AND NOT EXISTS(SELECT 1 FROM analytics_identity_mappings t WHERE t.user_id=m.target_user_id)
      AND 1=(SELECT COUNT(*) FROM user_usage_migrations x JOIN analytics_identity_mappings a ON a.user_id=x.source_user_id WHERE x.target_user_id=m.target_user_id);
  DELETE FROM analytics_identity_mappings i USING user_usage_migrations m WHERE i.user_id=m.source_user_id;
  INSERT INTO analytics_dirty_days(day,reason)
  SELECT MIN(day),'identity' FROM analytics_bucket_state
    WHERE day>=(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date
    GROUP BY DATE_TRUNC('month',day)
  ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,changed_at=NOW(),attempts=0,last_error='',
    reason=CASE WHEN analytics_dirty_days.reason IN('cost','directory','rebuild') THEN analytics_dirty_days.reason ELSE 'identity' END;
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_identity_merge AFTER INSERT OR UPDATE OR DELETE ON user_usage_migrations
FOR EACH STATEMENT EXECUTE FUNCTION analytics_mark_identity_merge();

-- TRUNCATE cannot preserve per-request accounting or fire the archive trigger.
CREATE FUNCTION analytics_guard_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (SELECT activated FROM analytics_runtime_state WHERE id=1) THEN
    RAISE EXCEPTION '数据分析启用后请使用分批日志清理，不能TRUNCATE请求表';
  END IF;
  RETURN NULL;
END $$;
CREATE TRIGGER analytics_usage_truncate BEFORE TRUNCATE ON usage_logs
FOR EACH STATEMENT EXECUTE FUNCTION analytics_guard_truncate();

-- Keep archive validation serialized with aggregation without waiting for a whole batch.
CREATE OR REPLACE FUNCTION analytics_dirty_cost_interval(starts TIMESTAMPTZ, stops TIMESTAMPTZ, fill_gaps BOOLEAN) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF starts IS NULL OR starts>NOW() OR stops<=starts THEN RETURN; END IF;
  IF NOT pg_try_advisory_xact_lock(9072028) THEN
    RAISE EXCEPTION '后台正在核算或清理数据，本次未保存，请稍后重试；也可先暂停后台统计后再保存'
      USING ERRCODE='55P03';
  END IF;
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

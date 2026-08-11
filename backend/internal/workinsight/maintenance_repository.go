package workinsight

import (
	"context"
	"time"
)

type CleanupResult struct{ Samples, Batches, Daily int64 }

type ClearLogsResult struct {
	Samples int64 `json:"samples"`
	Batches int64 `json:"batches"`
}

func (r *Repository) ClearTerminalLogs(ctx context.Context) (ClearLogsResult, error) {
	result := ClearLogsResult{}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted, err := tx.ExecContext(ctx, `DELETE FROM ai_work_insight_samples WHERE status IN ('analyzed','failed','dropped')`)
	if err != nil {
		return result, err
	}
	result.Samples, _ = deleted.RowsAffected()
	deleted, err = tx.ExecContext(ctx, `DELETE FROM ai_work_insight_batches WHERE status IN ('done','failed','dropped')`)
	if err != nil {
		return result, err
	}
	result.Batches, _ = deleted.RowsAffected()
	return result, tx.Commit()
}

func (r *Repository) RuntimeStats(ctx context.Context, date time.Time) (Runtime, error) {
	var result Runtime
	err := r.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM ai_work_insight_samples WHERE status='pending_batch'),
		COUNT(*) FILTER (WHERE status='queued'),COUNT(*) FILTER (WHERE status='processing'),COUNT(*) FILTER (WHERE status='retry'),
		COUNT(*) FILTER (WHERE status='done'),COUNT(*) FILTER (WHERE status IN ('failed','dropped')),
		COALESCE(SUM(analyzer_input_tokens),0),COALESCE(SUM(analyzer_output_tokens),0),COALESCE(SUM(chunk_count),0),
		(SELECT COUNT(*) FROM ai_user_daily_work_insights WHERE insight_date=$1 AND business_request_count>0),
		(SELECT COALESCE(SUM(eligible_active_session_count),0) FROM ai_user_daily_work_insights WHERE insight_date=$1),
		(SELECT COALESCE(SUM(covered_active_session_count),0) FROM ai_user_daily_work_insights WHERE insight_date=$1)
		FROM ai_work_insight_batches WHERE local_date=$1`, date).Scan(&result.WaitingSamples, &result.QueuedBatches, &result.ProcessingBatches,
		&result.RetryBatches, &result.DoneBatches, &result.FailedBatches, &result.AnalyzerInputTokens, &result.AnalyzerOutputTokens,
		&result.AnalyzerCalls, &result.ActiveUsers, &result.ActiveSessions, &result.CoveredSessions)
	return result, err
}

func (r *Repository) ReconcileUsage(ctx context.Context, date time.Time, timezone string) (int64, error) {
	start, end, err := usageDayBounds(date, timezone)
	if err != nil {
		return 0, err
	}
	result, err := r.db.ExecContext(ctx, `WITH usage_base AS (
		SELECT ul.user_id,u.username,ul.input_tokens,ul.output_tokens,ul.cache_creation_tokens,ul.cache_read_tokens,ul.duration_ms,
			COALESCE(NULLIF(ul.requested_model,''),NULLIF(ul.model,''),'未知') AS model
		FROM usage_logs ul LEFT JOIN users u ON u.id=ul.user_id
		WHERE ul.user_id IS NOT NULL AND ul.created_at >= $2 AND ul.created_at < $3),
	usage_agg AS (
		SELECT user_id,COALESCE(MAX(username),'') AS username,COUNT(*) AS requests,
			COALESCE(SUM(input_tokens),0) AS input_tokens,COALESCE(SUM(output_tokens),0) AS output_tokens,
			COALESCE(SUM(cache_creation_tokens),0) AS cache_creation_tokens,COALESCE(SUM(cache_read_tokens),0) AS cache_read_tokens,
			COALESCE(AVG(duration_ms) FILTER (WHERE duration_ms IS NOT NULL),0)::BIGINT AS average_duration_ms,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) FILTER (WHERE duration_ms IS NOT NULL),0)::BIGINT AS p95_duration_ms
		FROM usage_base GROUP BY user_id),
	model_counts AS (SELECT user_id,model,COUNT(*) AS requests FROM usage_base GROUP BY user_id,model),
	model_ranked AS (SELECT *,ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY requests DESC,model) AS rank FROM model_counts),
	model_buckets AS (SELECT user_id,CASE WHEN rank<=10 THEN model ELSE '其他' END AS model,SUM(requests) AS requests FROM model_ranked GROUP BY user_id,CASE WHEN rank<=10 THEN model ELSE '其他' END),
	model_usage AS (SELECT user_id,jsonb_object_agg(model,requests) AS models FROM model_buckets GROUP BY user_id),
	errors AS (SELECT user_id,COUNT(*) AS errors FROM ops_error_logs WHERE user_id IS NOT NULL AND created_at >= $2 AND created_at < $3
		AND COALESCE(status_code,0)>=400 AND COALESCE(is_count_tokens,FALSE)=FALSE GROUP BY user_id),
	daily_users AS (SELECT user_id FROM usage_agg UNION SELECT user_id FROM errors)
	INSERT INTO ai_user_daily_work_insights
		(user_id,username_snapshot,insight_date,business_request_count,business_total_tokens,business_input_tokens,
		business_output_tokens,business_cache_creation_tokens,business_cache_read_tokens,business_error_count,
		average_duration_ms,p95_duration_ms,model_usage,computed_at)
	SELECT d.user_id,COALESCE(NULLIF(a.username,''),u.username,''),$1,COALESCE(a.requests,0),
		COALESCE(a.input_tokens,0)+COALESCE(a.output_tokens,0)+COALESCE(a.cache_creation_tokens,0)+COALESCE(a.cache_read_tokens,0),
		COALESCE(a.input_tokens,0),COALESCE(a.output_tokens,0),COALESCE(a.cache_creation_tokens,0),COALESCE(a.cache_read_tokens,0),
		COALESCE(e.errors,0),COALESCE(a.average_duration_ms,0),COALESCE(a.p95_duration_ms,0),COALESCE(m.models,'{}'::jsonb),NOW()
	FROM daily_users d LEFT JOIN usage_agg a ON a.user_id=d.user_id LEFT JOIN model_usage m ON m.user_id=d.user_id
		LEFT JOIN errors e ON e.user_id=d.user_id LEFT JOIN users u ON u.id=d.user_id
		ON CONFLICT (user_id,insight_date) DO UPDATE SET
		username_snapshot=CASE WHEN EXCLUDED.username_snapshot<>'' THEN EXCLUDED.username_snapshot ELSE ai_user_daily_work_insights.username_snapshot END,
		business_request_count=EXCLUDED.business_request_count,business_total_tokens=EXCLUDED.business_total_tokens,
		business_input_tokens=EXCLUDED.business_input_tokens,business_output_tokens=EXCLUDED.business_output_tokens,
		business_cache_creation_tokens=EXCLUDED.business_cache_creation_tokens,business_cache_read_tokens=EXCLUDED.business_cache_read_tokens,
		business_error_count=EXCLUDED.business_error_count,average_duration_ms=EXCLUDED.average_duration_ms,
		p95_duration_ms=EXCLUDED.p95_duration_ms,model_usage=EXCLUDED.model_usage,computed_at=NOW()`, date, start, end)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func usageDayBounds(date time.Time, timezone string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location)
	return start.UTC(), start.AddDate(0, 0, 1).UTC(), nil
}

func (r *Repository) CoveredSessions(ctx context.Context, userID int64, date time.Time) (int, string, error) {
	var covered int
	var username string
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT b.active_session_id),COALESCE(MAX(b.username_snapshot),'')
		FROM ai_work_insight_batches b WHERE b.user_id=$1 AND b.local_date=$2 AND b.status='done'`, userID, date).Scan(&covered, &username)
	return covered, username, err
}

func (r *Repository) Finalize(ctx context.Context, date time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE ai_user_daily_work_insights d SET finalized_at=COALESCE(d.finalized_at,NOW()),computed_at=NOW()
		WHERE d.insight_date=$1 AND NOT EXISTS (SELECT 1 FROM ai_work_insight_batches b WHERE b.user_id=d.user_id AND b.local_date=d.insight_date
			AND b.status IN ('queued','processing','retry'))`, date)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *Repository) HasOpenBatches(ctx context.Context, date time.Time) (bool, error) {
	var pending bool
	err := r.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM ai_work_insight_samples WHERE local_date=$1 AND status IN ('staging','pending_batch'))
		OR EXISTS(SELECT 1 FROM ai_work_insight_batches WHERE local_date=$1 AND status IN ('queued','processing','retry'))`, date).Scan(&pending)
	return pending, err
}

func (r *Repository) Cleanup(ctx context.Context, now time.Time, cfg Config) (CleanupResult, error) {
	result := CleanupResult{}
	sampleCutoff := now.AddDate(0, 0, -cfg.SampleRetentionDays)
	dailyCutoff := now.AddDate(0, 0, -cfg.InsightRetentionDays)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	deleted, err := tx.ExecContext(ctx, `DELETE FROM ai_work_insight_samples WHERE id IN (SELECT id FROM ai_work_insight_samples WHERE created_at<$1 ORDER BY created_at,id LIMIT $2)`, sampleCutoff, cfg.CleanupBatchSize)
	if err != nil {
		return result, err
	}
	result.Samples, _ = deleted.RowsAffected()
	deleted, err = tx.ExecContext(ctx, `DELETE FROM ai_work_insight_batches WHERE id IN (SELECT id FROM ai_work_insight_batches WHERE created_at<$1 ORDER BY created_at,id LIMIT $2)`, sampleCutoff, cfg.CleanupBatchSize)
	if err != nil {
		return result, err
	}
	result.Batches, _ = deleted.RowsAffected()
	deleted, err = tx.ExecContext(ctx, `DELETE FROM ai_user_daily_work_insights WHERE id IN (SELECT id FROM ai_user_daily_work_insights WHERE insight_date<$1 ORDER BY insight_date,id LIMIT $2)`, dailyCutoff, cfg.CleanupBatchSize)
	if err != nil {
		return result, err
	}
	result.Daily, _ = deleted.RowsAffected()
	return result, tx.Commit()
}

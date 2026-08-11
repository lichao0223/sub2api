package workinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/lib/pq"
)

type sampleBucket struct {
	userID    int64
	username  string
	date      time.Time
	sessionID string
	samples   []Sample
}

func (r *Repository) DropExpiredSamples(ctx context.Context, cutoff time.Time) ([]BatchSample, error) {
	rows, err := r.db.QueryContext(ctx, `WITH expired AS (
		SELECT id FROM ai_work_insight_samples WHERE status IN ('staging','pending_batch') AND created_at <= $1
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 500)
		UPDATE ai_work_insight_samples s SET status='dropped',error_code='job_expired',updated_at=NOW()
		FROM expired WHERE s.id=expired.id RETURNING s.id,s.estimated_tokens,s.created_at`, cutoff.UTC())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var samples []BatchSample
	for rows.Next() {
		var sample BatchSample
		if err := rows.Scan(&sample.ID, &sample.EstimatedTokens, &sample.CreatedAt); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (r *Repository) CreateDueBatches(ctx context.Context, now time.Time, cfg Config, forceReason string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `WITH due_sessions AS (
		SELECT user_id,local_date,active_session_id FROM ai_work_insight_samples
		WHERE status='pending_batch' AND user_id IS NOT NULL
		GROUP BY user_id,local_date,active_session_id
		HAVING COUNT(*) >= $1 OR COALESCE(SUM(estimated_tokens),0) >= $2
			OR ($3 AND (MIN(created_at) <= $4 OR MAX(created_at) <= $5))
			OR ($6 AND MIN(created_at) <= $7) OR $8
		ORDER BY MIN(id) LIMIT 500)
		SELECT s.id,s.user_id,s.username_snapshot,s.local_date,s.active_session_id,s.estimated_tokens,s.created_at
		FROM ai_work_insight_samples s JOIN due_sessions d
			ON d.user_id=s.user_id AND d.local_date=s.local_date AND d.active_session_id=s.active_session_id
		WHERE s.status='pending_batch' ORDER BY s.id FOR UPDATE OF s SKIP LOCKED LIMIT 5000`,
		cfg.MaxSamplesPerBatch, cfg.MaxInputTokens, cfg.AnalysisTriggerMode == "hybrid",
		now.Add(-time.Duration(cfg.AnalysisMaxWaitMinutes)*time.Minute), now.Add(-time.Duration(cfg.AnalysisIdleMinutes)*time.Minute),
		cfg.AnalysisTriggerMode == "fixed_interval", now.Add(-time.Duration(cfg.FixedIntervalMinutes)*time.Minute), forceReason != "")
	if err != nil {
		return 0, err
	}
	buckets := map[string]*sampleBucket{}
	orderedBuckets := make([]*sampleBucket, 0)
	for rows.Next() {
		var sample Sample
		if err := rows.Scan(&sample.ID, &sample.UserID, &sample.Username, &sample.LocalDate, &sample.SessionID, &sample.EstimatedTokens, &sample.CreatedAt); err != nil {
			_ = rows.Close()
			return 0, err
		}
		key := strconv.FormatInt(sample.UserID, 10) + "\x00" + sample.LocalDate.Format("2006-01-02") + "\x00" + sample.SessionID
		bucket := buckets[key]
		if bucket == nil {
			bucket = &sampleBucket{userID: sample.UserID, username: sample.Username, date: sample.LocalDate, sessionID: sample.SessionID}
			buckets[key] = bucket
			orderedBuckets = append(orderedBuckets, bucket)
		}
		bucket.samples = append(bucket.samples, sample)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	created := 0
	for _, bucket := range orderedBuckets {
		if bucket.userID == 0 || len(bucket.samples) == 0 {
			continue
		}
		reason := batchTrigger(bucket.samples, now, cfg, forceReason)
		if reason == "" {
			continue
		}
		selected := selectBatchSamples(bucket.samples, cfg.MaxSamplesPerBatch, cfg.MaxInputTokens)
		ids := make([]int64, len(selected))
		tokens := 0
		for i, sample := range selected {
			ids[i], tokens = sample.ID, tokens+sample.EstimatedTokens
		}
		var id int64
		err := tx.QueryRowContext(ctx, `INSERT INTO ai_work_insight_batches
			(user_id,username_snapshot,local_date,active_session_id,first_sample_id,last_sample_id,sample_count,estimated_input_tokens,trigger_reason,status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'queued') ON CONFLICT (user_id,local_date,first_sample_id,last_sample_id) DO NOTHING RETURNING id`,
			bucket.userID, bucket.username, bucket.date, bucket.sessionID, ids[0], ids[len(ids)-1], len(ids), tokens, reason).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, `UPDATE ai_work_insight_samples SET status='batched',batch_id=$1,updated_at=$2 WHERE id=ANY($3) AND status='pending_batch'`, id, now.UTC(), pq.Array(ids))
		if err != nil {
			return 0, err
		}
		if affected, _ := result.RowsAffected(); affected != int64(len(ids)) {
			return 0, errors.New("work insight batch sample claim conflict")
		}
		created++
	}
	return created, tx.Commit()
}

func batchTrigger(samples []Sample, now time.Time, cfg Config, forceReason string) string {
	if len(samples) == 0 {
		return ""
	}
	if forceReason != "" {
		return forceReason
	}
	tokens := 0
	for _, sample := range samples {
		tokens += sample.EstimatedTokens
	}
	if len(samples) >= cfg.MaxSamplesPerBatch {
		return "sample_limit"
	}
	if tokens >= cfg.MaxInputTokens {
		return "token_limit"
	}
	if cfg.AnalysisTriggerMode == "fixed_interval" && !now.Before(samples[0].CreatedAt.Add(time.Duration(cfg.FixedIntervalMinutes)*time.Minute)) {
		return "fixed"
	}
	if cfg.AnalysisTriggerMode != "hybrid" {
		return ""
	}
	if !now.Before(samples[0].CreatedAt.Add(time.Duration(cfg.AnalysisMaxWaitMinutes) * time.Minute)) {
		return "max_wait"
	}
	if !now.Before(samples[len(samples)-1].CreatedAt.Add(time.Duration(cfg.AnalysisIdleMinutes) * time.Minute)) {
		return "idle"
	}
	return ""
}

type BatchSummary struct {
	ID                   int64      `json:"id"`
	UserID               *int64     `json:"user_id"`
	Username             string     `json:"username"`
	LocalDate            time.Time  `json:"local_date"`
	SessionID            string     `json:"active_session_id"`
	SampleCount          int        `json:"sample_count"`
	TriggerReason        string     `json:"trigger_reason"`
	Status               string     `json:"status"`
	Attempts             int        `json:"attempts"`
	ErrorCode            string     `json:"error_code"`
	AnalyzerModel        string     `json:"analyzer_model"`
	AnalyzerInputTokens  int64      `json:"analyzer_input_tokens"`
	AnalyzerOutputTokens int64      `json:"analyzer_output_tokens"`
	AnalyzedAt           *time.Time `json:"analyzed_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (r *Repository) ListBatches(ctx context.Context, size int) ([]BatchSummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,username_snapshot,local_date,active_session_id,sample_count,
		trigger_reason,status,attempts,error_code,analyzer_model,analyzer_input_tokens,analyzer_output_tokens,analyzed_at,created_at,updated_at
		FROM ai_work_insight_batches ORDER BY id DESC LIMIT $1`, size)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]BatchSummary, 0, size)
	for rows.Next() {
		var item BatchSummary
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.LocalDate, &item.SessionID, &item.SampleCount,
			&item.TriggerReason, &item.Status, &item.Attempts, &item.ErrorCode, &item.AnalyzerModel, &item.AnalyzerInputTokens,
			&item.AnalyzerOutputTokens, &item.AnalyzedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func selectBatchSamples(samples []Sample, maxSamples, maxTokens int) []Sample {
	selected, tokens := make([]Sample, 0, min(len(samples), maxSamples)), 0
	for _, sample := range samples {
		if len(selected) >= maxSamples || (len(selected) > 0 && tokens+sample.EstimatedTokens > maxTokens) {
			break
		}
		selected, tokens = append(selected, sample), tokens+sample.EstimatedTokens
	}
	return selected
}

func (r *Repository) ClaimBatch(ctx context.Context, now time.Time, lease time.Duration) (*Batch, bool, error) {
	leaseCutoff := now.Add(-lease).UTC()
	row := r.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT b.id FROM ai_work_insight_batches b
		WHERE ((b.status IN ('queued','retry') AND b.next_attempt_at <= $1) OR (b.status='processing' AND b.updated_at <= $2))
		AND NOT EXISTS (SELECT 1 FROM ai_work_insight_batches earlier WHERE earlier.user_id=b.user_id AND earlier.local_date=b.local_date
			AND earlier.first_sample_id < b.first_sample_id AND earlier.status IN ('queued','processing','retry'))
		AND NOT EXISTS (SELECT 1 FROM ai_work_insight_batches active WHERE active.user_id=b.user_id AND active.local_date=b.local_date
			AND active.status='processing' AND active.updated_at > $2 AND active.id<>b.id)
		ORDER BY b.next_attempt_at,b.id FOR UPDATE SKIP LOCKED LIMIT 1)
		UPDATE ai_work_insight_batches b SET status='processing',attempts=b.attempts+1,claim_version=b.claim_version+1,error_code='',
		base_summary_version=COALESCE((SELECT d.summary_version FROM ai_user_daily_work_insights d WHERE d.user_id=b.user_id AND d.insight_date=b.local_date),0),updated_at=$1
		FROM candidate WHERE b.id=candidate.id RETURNING b.id,COALESCE(b.user_id,0),b.username_snapshot,b.local_date,b.active_session_id,
		b.first_sample_id,b.last_sample_id,b.sample_count,b.estimated_input_tokens,b.trigger_reason,b.attempts,b.claim_version,b.base_summary_version,b.created_at`, now.UTC(), leaseCutoff)
	var batch Batch
	err := row.Scan(&batch.ID, &batch.UserID, &batch.Username, &batch.LocalDate, &batch.SessionID, &batch.FirstSampleID, &batch.LastSampleID,
		&batch.SampleCount, &batch.EstimatedInputTokens, &batch.TriggerReason, &batch.Attempts, &batch.ClaimVersion, &batch.BaseSummaryVersion, &batch.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return &batch, err == nil, err
}

func (r *Repository) LoadBatchSamples(ctx context.Context, batchID int64) ([]BatchSample, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,estimated_tokens,created_at FROM ai_work_insight_samples WHERE batch_id=$1 AND status='batched' ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var samples []BatchSample
	for rows.Next() {
		var sample BatchSample
		if err := rows.Scan(&sample.ID, &sample.EstimatedTokens, &sample.CreatedAt); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (r *Repository) RetryBatch(ctx context.Context, batch Batch, next time.Time, code string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ai_work_insight_batches SET status='retry',next_attempt_at=$3,error_code=$4,updated_at=NOW() WHERE id=$1 AND status='processing' AND claim_version=$2`, batch.ID, batch.ClaimVersion, next.UTC(), code)
	return requireAffected(result, err)
}

func (r *Repository) LoadDroppedBatchSamples(ctx context.Context, batchID int64) ([]BatchSample, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.id,s.estimated_tokens,s.created_at
		FROM ai_work_insight_samples s JOIN ai_work_insight_batches b ON b.id=s.batch_id
		WHERE b.id=$1 AND b.status IN ('failed','dropped') AND s.status IN ('failed','dropped') ORDER BY s.id`, batchID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var samples []BatchSample
	for rows.Next() {
		var sample BatchSample
		if err := rows.Scan(&sample.ID, &sample.EstimatedTokens, &sample.CreatedAt); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (r *Repository) RequeueDroppedBatch(ctx context.Context, batchID int64, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE ai_work_insight_batches SET status='queued',attempts=0,next_attempt_at=$2,
		trigger_reason='manual',error_code='',analyzer_model='',analyzer_input_tokens=0,analyzer_output_tokens=0,
		chunk_count=0,analyzed_at=NULL,created_at=$2,updated_at=$2 WHERE id=$1 AND status IN ('failed','dropped')`, batchID, now.UTC())
	if err := requireAffected(result, err); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE ai_work_insight_samples SET status='batched',error_code='',updated_at=$2
		WHERE batch_id=$1 AND status IN ('failed','dropped')`, batchID, now.UTC())
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errors.New("work insight retry samples missing")
	}
	return tx.Commit()
}

func (r *Repository) DropBatch(ctx context.Context, batch Batch, code string) ([]BatchSample, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE ai_work_insight_batches SET status='dropped',error_code=$3,updated_at=NOW() WHERE id=$1 AND status='processing' AND claim_version=$2`, batch.ID, batch.ClaimVersion, code)
	if err := requireAffected(result, err); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,estimated_tokens,created_at FROM ai_work_insight_samples WHERE batch_id=$1 ORDER BY id`, batch.ID)
	if err != nil {
		return nil, err
	}
	var samples []BatchSample
	for rows.Next() {
		var sample BatchSample
		if err := rows.Scan(&sample.ID, &sample.EstimatedTokens, &sample.CreatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ai_work_insight_samples SET status='dropped',error_code=$2,updated_at=NOW() WHERE batch_id=$1 AND status='batched'`, batch.ID, code); err != nil {
		return nil, err
	}
	return samples, tx.Commit()
}

func requireAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("work insight claim lost")
	}
	return nil
}

func mergeUnique(base []string, additions ...[]string) []string {
	seen := make(map[string]struct{}, len(base))
	result := append([]string(nil), base...)
	for _, value := range base {
		seen[value] = struct{}{}
	}
	for _, values := range additions {
		for _, value := range values {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}

func jsonValue(value any) []byte { raw, _ := json.Marshal(value); return raw }

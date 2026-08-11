package workinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

var ErrConfigConflict = errors.New("work insight config version conflict")

func (r *Repository) SaveConfigCAS(ctx context.Context, raw string, expectedVersion int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR UPDATE`, SettingKey).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion != DefaultConfig().ConfigVersion {
			return ErrConfigConflict
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES ($1,$2,NOW())`, SettingKey, raw); err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				return ErrConfigConflict
			}
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	var version struct {
		ConfigVersion int64 `json:"config_version"`
	}
	if json.Unmarshal([]byte(current), &version) != nil || version.ConfigVersion != expectedVersion {
		return ErrConfigConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE settings SET value=$2,updated_at=NOW() WHERE key=$1`, SettingKey, raw); err != nil {
		return err
	}
	return tx.Commit()
}

type SampleInput struct {
	Fingerprint, RequestID, Username, UserEmail, Provider, Protocol, Endpoint, Model string
	UserID, APIKeyID                                                                 int64
	GroupID                                                                          *int64
	LocalDate                                                                        time.Time
	SessionID, Reason, PromptHash                                                    string
	RedactedPreview                                                                  string
	EstimatedTokens, PromptChars, AnalyzedChars                                      int
}

func (r *Repository) CreateStaging(ctx context.Context, in SampleInput) (int64, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, errors.New("work insight database unavailable")
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO ai_work_insight_samples (
			request_fingerprint,request_id,user_id,username_snapshot,user_email_snapshot,api_key_id,group_id,
			provider,protocol,endpoint,requested_model,local_date,active_session_id,sample_reason,prompt_hash,
			estimated_tokens,prompt_chars,analyzed_chars,redacted_preview,status)
		VALUES ($1,$2,NULLIF($3,0),$4,$5,NULLIF($6,0),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'staging')
		ON CONFLICT (request_fingerprint) DO NOTHING RETURNING id`,
		in.Fingerprint, in.RequestID, in.UserID, in.Username, in.UserEmail, in.APIKeyID, in.GroupID,
		in.Provider, in.Protocol, in.Endpoint, in.Model, in.LocalDate, in.SessionID, in.Reason, in.PromptHash,
		in.EstimatedTokens, in.PromptChars, in.AnalyzedChars, in.RedactedPreview).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	err = r.db.QueryRowContext(ctx, `SELECT id FROM ai_work_insight_samples WHERE request_fingerprint=$1`, in.Fingerprint).Scan(&id)
	return id, false, err
}

func (r *Repository) Publish(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ai_work_insight_samples SET status='pending_batch',updated_at=NOW() WHERE id=$1 AND status='staging'`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("work insight sample is not staging")
	}
	return nil
}

func (r *Repository) Drop(ctx context.Context, id int64, code string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ai_work_insight_samples SET status='dropped',error_code=$2,updated_at=NOW() WHERE id=$1 AND status NOT IN ('analyzed','failed','dropped')`, id, code)
	return err
}

type DailyFilter struct {
	Start, End time.Time
	Search     string
	Category   string
	Project    string
	Page, Size int
}

type SampleSummary struct {
	ID              int64     `json:"id"`
	RequestID       string    `json:"request_id"`
	UserID          *int64    `json:"user_id"`
	Username        string    `json:"username"`
	Provider        string    `json:"provider"`
	Protocol        string    `json:"protocol"`
	Endpoint        string    `json:"endpoint"`
	RequestedModel  string    `json:"requested_model"`
	LocalDate       time.Time `json:"local_date"`
	ActiveSessionID string    `json:"active_session_id"`
	SampleReason    string    `json:"sample_reason"`
	EstimatedTokens int       `json:"estimated_tokens"`
	PromptChars     int       `json:"prompt_chars"`
	AnalyzedChars   int       `json:"analyzed_chars"`
	RedactedPreview string    `json:"redacted_preview,omitempty"`
	Status          string    `json:"status"`
	Attempts        int       `json:"attempts"`
	ErrorCode       string    `json:"error_code"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	BatchID         *int64    `json:"batch_id"`
}

func (r *Repository) ListSamples(ctx context.Context, beforeID int64, size int) ([]SampleSummary, int64, bool, error) {
	query := `SELECT id,request_id,user_id,username_snapshot,provider,protocol,endpoint,requested_model,
		local_date,active_session_id,sample_reason,estimated_tokens,prompt_chars,analyzed_chars,redacted_preview,status,
		attempts,error_code,created_at,updated_at,batch_id FROM ai_work_insight_samples`
	args := []any{size + 1}
	if beforeID > 0 {
		query += ` WHERE id<$1 ORDER BY id DESC LIMIT $2`
		args = []any{beforeID, size + 1}
	} else {
		query += ` ORDER BY id DESC LIMIT $1`
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()
	items := make([]SampleSummary, 0, size+1)
	for rows.Next() {
		var item SampleSummary
		if err := rows.Scan(&item.ID, &item.RequestID, &item.UserID, &item.Username, &item.Provider, &item.Protocol, &item.Endpoint,
			&item.RequestedModel, &item.LocalDate, &item.ActiveSessionID, &item.SampleReason, &item.EstimatedTokens, &item.PromptChars,
			&item.AnalyzedChars, &item.RedactedPreview, &item.Status, &item.Attempts, &item.ErrorCode, &item.CreatedAt, &item.UpdatedAt, &item.BatchID); err != nil {
			return nil, 0, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	hasMore := len(items) > size
	if hasMore {
		items = items[:size]
	}
	var nextCursor int64
	if hasMore && len(items) > 0 {
		nextCursor = items[len(items)-1].ID
	}
	return items, nextCursor, hasMore, nil
}

func (r *Repository) GetSample(ctx context.Context, id int64) (*SampleSummary, error) {
	var item SampleSummary
	err := r.db.QueryRowContext(ctx, `SELECT id,request_id,user_id,username_snapshot,provider,protocol,endpoint,requested_model,
		local_date,active_session_id,sample_reason,estimated_tokens,prompt_chars,analyzed_chars,redacted_preview,status,
		attempts,error_code,created_at,updated_at,batch_id FROM ai_work_insight_samples WHERE id=$1`, id).Scan(
		&item.ID, &item.RequestID, &item.UserID, &item.Username, &item.Provider, &item.Protocol, &item.Endpoint,
		&item.RequestedModel, &item.LocalDate, &item.ActiveSessionID, &item.SampleReason, &item.EstimatedTokens, &item.PromptChars,
		&item.AnalyzedChars, &item.RedactedPreview, &item.Status, &item.Attempts, &item.ErrorCode, &item.CreatedAt, &item.UpdatedAt, &item.BatchID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

type Overview struct {
	ActiveUsers          int64 `json:"active_users"`
	InsightUsers         int64 `json:"insight_users"`
	ActiveSessions       int64 `json:"active_sessions"`
	CoveredSessions      int64 `json:"covered_sessions"`
	SampleRequests       int64 `json:"sample_requests"`
	BusinessTokens       int64 `json:"business_tokens"`
	FailedSamples        int64 `json:"failed_samples"`
	AnalyzerInputTokens  int64 `json:"analyzer_input_tokens"`
	AnalyzerOutputTokens int64 `json:"analyzer_output_tokens"`
}

type DailyInsight struct {
	ID                          int64           `json:"id"`
	UserID                      *int64          `json:"user_id"`
	Username                    string          `json:"username"`
	Date                        time.Time       `json:"insight_date"`
	BusinessRequestCount        int64           `json:"business_request_count"`
	BusinessTotalTokens         int64           `json:"business_total_tokens"`
	BusinessInputTokens         int64           `json:"business_input_tokens"`
	BusinessOutputTokens        int64           `json:"business_output_tokens"`
	BusinessCacheCreationTokens int64           `json:"business_cache_creation_tokens"`
	BusinessCacheReadTokens     int64           `json:"business_cache_read_tokens"`
	BusinessErrorCount          int64           `json:"business_error_count"`
	AverageDurationMS           int64           `json:"average_duration_ms"`
	P95DurationMS               int64           `json:"p95_duration_ms"`
	ModelUsage                  json.RawMessage `json:"model_usage"`
	SampleCount                 int             `json:"sample_count"`
	FailedSampleCount           int             `json:"failed_sample_count"`
	EligibleActiveSessionCount  int             `json:"eligible_active_session_count"`
	CoveredActiveSessionCount   int             `json:"covered_active_session_count"`
	TaskCategoryStats           json.RawMessage `json:"task_category_stats"`
	ExplicitProjects            json.RawMessage `json:"explicit_projects"`
	ExplicitModules             json.RawMessage `json:"explicit_modules"`
	ChangeTypes                 json.RawMessage `json:"change_types"`
	BusinessTopics              json.RawMessage `json:"business_topics"`
	DailySummary                string          `json:"daily_summary"`
	LastAnalyzedAt              *time.Time      `json:"last_analyzed_at"`
	FinalizedAt                 *time.Time      `json:"finalized_at"`
}

func (r *Repository) ListDaily(ctx context.Context, f DailyFilter) ([]DailyInsight, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, errors.New("work insight database unavailable")
	}
	where, args := dailyWhere(f)
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_user_daily_work_insights d`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Size, (f.Page-1)*f.Size)
	rows, err := r.db.QueryContext(ctx, `SELECT d.id,d.user_id,d.username_snapshot,d.insight_date,
		d.business_request_count,d.business_total_tokens,d.business_input_tokens,d.business_output_tokens,
		d.business_cache_creation_tokens,d.business_cache_read_tokens,d.business_error_count,
		d.average_duration_ms,d.p95_duration_ms,d.model_usage,d.sample_count,d.failed_sample_count,
		d.eligible_active_session_count,d.covered_active_session_count,d.task_category_stats,
		d.explicit_projects,d.explicit_modules,d.change_types,d.business_topics,d.daily_summary,d.last_analyzed_at,d.finalized_at
		FROM ai_user_daily_work_insights d`+where+` ORDER BY d.insight_date DESC,d.id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]DailyInsight, 0, f.Size)
	for rows.Next() {
		var item DailyInsight
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Date, &item.BusinessRequestCount,
			&item.BusinessTotalTokens, &item.BusinessInputTokens, &item.BusinessOutputTokens,
			&item.BusinessCacheCreationTokens, &item.BusinessCacheReadTokens, &item.BusinessErrorCount,
			&item.AverageDurationMS, &item.P95DurationMS, &item.ModelUsage, &item.SampleCount, &item.FailedSampleCount,
			&item.EligibleActiveSessionCount, &item.CoveredActiveSessionCount, &item.TaskCategoryStats,
			&item.ExplicitProjects, &item.ExplicitModules, &item.ChangeTypes, &item.BusinessTopics, &item.DailySummary,
			&item.LastAnalyzedAt, &item.FinalizedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func dailyWhere(f DailyFilter) (string, []any) {
	clauses, args := []string{"1=1"}, []any{}
	if !f.Start.IsZero() {
		args = append(args, f.Start)
		clauses = append(clauses, fmt.Sprintf("d.insight_date >= $%d", len(args)))
	}
	if !f.End.IsZero() {
		args = append(args, f.End)
		clauses = append(clauses, fmt.Sprintf("d.insight_date <= $%d", len(args)))
	}
	if search := strings.TrimSpace(f.Search); search != "" {
		args = append(args, "%"+search+"%")
		clauses = append(clauses, fmt.Sprintf(`(d.username_snapshot ILIKE $%[1]d OR d.daily_summary ILIKE $%[1]d
			OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(d.explicit_projects) value WHERE value ILIKE $%[1]d)
			OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(d.explicit_modules) value WHERE value ILIKE $%[1]d))`, len(args)))
	}
	if category := strings.TrimSpace(f.Category); category != "" {
		args = append(args, category)
		clauses = append(clauses, fmt.Sprintf("d.task_category_stats ? $%d", len(args)))
	}
	if project := strings.TrimSpace(f.Project); project != "" {
		args = append(args, "%"+project+"%")
		clauses = append(clauses, fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements_text(d.explicit_projects) value WHERE value ILIKE $%d)", len(args)))
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) Overview(ctx context.Context, f DailyFilter) (Overview, error) {
	where, args := dailyWhere(f)
	var result Overview
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT d.user_id),COUNT(DISTINCT d.user_id) FILTER (WHERE d.last_analyzed_at IS NOT NULL),
		COALESCE(SUM(d.eligible_active_session_count),0),COALESCE(SUM(d.covered_active_session_count),0),COALESCE(SUM(d.sample_count),0),
		COALESCE(SUM(d.business_total_tokens),0),COALESCE(SUM(d.failed_sample_count),0)
		FROM ai_user_daily_work_insights d`+where, args...).Scan(&result.ActiveUsers, &result.InsightUsers, &result.ActiveSessions,
		&result.CoveredSessions, &result.SampleRequests, &result.BusinessTokens, &result.FailedSamples)
	if err != nil {
		return result, err
	}
	batchClauses, batchArgs := []string{"b.status='done'"}, []any{}
	if !f.Start.IsZero() {
		batchArgs = append(batchArgs, f.Start)
		batchClauses = append(batchClauses, fmt.Sprintf("b.local_date >= $%d", len(batchArgs)))
	}
	if !f.End.IsZero() {
		batchArgs = append(batchArgs, f.End)
		batchClauses = append(batchClauses, fmt.Sprintf("b.local_date <= $%d", len(batchArgs)))
	}
	err = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(b.analyzer_input_tokens),0),COALESCE(SUM(b.analyzer_output_tokens),0) FROM ai_work_insight_batches b WHERE `+strings.Join(batchClauses, " AND "), batchArgs...).Scan(&result.AnalyzerInputTokens, &result.AnalyzerOutputTokens)
	return result, err
}

func (r *Repository) GetDaily(ctx context.Context, id int64) (*DailyInsight, error) {
	var item DailyInsight
	err := r.db.QueryRowContext(ctx, `SELECT id,user_id,username_snapshot,insight_date,business_request_count,
		business_total_tokens,business_input_tokens,business_output_tokens,business_cache_creation_tokens,
		business_cache_read_tokens,business_error_count,average_duration_ms,p95_duration_ms,model_usage,
		sample_count,failed_sample_count,eligible_active_session_count,
		covered_active_session_count,task_category_stats,explicit_projects,explicit_modules,change_types,business_topics,
		daily_summary,last_analyzed_at,finalized_at FROM ai_user_daily_work_insights WHERE id=$1`, id).Scan(
		&item.ID, &item.UserID, &item.Username, &item.Date, &item.BusinessRequestCount, &item.BusinessTotalTokens,
		&item.BusinessInputTokens, &item.BusinessOutputTokens, &item.BusinessCacheCreationTokens,
		&item.BusinessCacheReadTokens, &item.BusinessErrorCount, &item.AverageDurationMS, &item.P95DurationMS, &item.ModelUsage,
		&item.SampleCount, &item.FailedSampleCount, &item.EligibleActiveSessionCount, &item.CoveredActiveSessionCount,
		&item.TaskCategoryStats, &item.ExplicitProjects, &item.ExplicitModules, &item.ChangeTypes,
		&item.BusinessTopics, &item.DailySummary, &item.LastAnalyzedAt, &item.FinalizedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) ListRepresentativeItems(ctx context.Context, userID *int64, date time.Time, limit, offset int) ([]RepresentativeItem, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(jsonb_array_length(representative_items)),0)
		FROM ai_work_insight_batches WHERE user_id IS NOT DISTINCT FROM $1 AND local_date=$2 AND status='done'`, userID, date).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT item FROM ai_work_insight_batches b
		CROSS JOIN LATERAL jsonb_array_elements(b.representative_items) item
		WHERE b.user_id IS NOT DISTINCT FROM $1 AND b.local_date=$2 AND b.status='done'
		ORDER BY b.last_sample_id DESC LIMIT $3 OFFSET $4`, userID, date, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]RepresentativeItem, 0, limit)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var item RepresentativeItem
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *Repository) PersistCoverage(ctx context.Context, userID int64, username string, date time.Time, eligible, covered int) error {
	if eligible < covered {
		eligible = covered
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO ai_user_daily_work_insights
		(user_id,username_snapshot,insight_date,eligible_active_session_count,covered_active_session_count,computed_at)
		VALUES ($1,$2,$3,$4,$5,NOW())
		ON CONFLICT (user_id,insight_date) DO UPDATE SET
		username_snapshot=CASE WHEN EXCLUDED.username_snapshot<>'' THEN EXCLUDED.username_snapshot ELSE ai_user_daily_work_insights.username_snapshot END,
		eligible_active_session_count=GREATEST(ai_user_daily_work_insights.eligible_active_session_count,EXCLUDED.eligible_active_session_count),
		covered_active_session_count=GREATEST(ai_user_daily_work_insights.covered_active_session_count,EXCLUDED.covered_active_session_count),
		computed_at=NOW()`, userID, username, date, eligible, covered)
	return err
}

package workinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type dailyState struct {
	summary        string
	categoryStats  map[string]int
	projects       []string
	modules        []string
	changeTypes    []string
	businessTopics []string
	version        int64
	sampleCount    int
	failedCount    int
}

func (r *Repository) DailySummary(ctx context.Context, userID int64, date time.Time) (string, int64, error) {
	var summary string
	var version int64
	err := r.db.QueryRowContext(ctx, `SELECT daily_summary,summary_version FROM ai_user_daily_work_insights WHERE user_id=$1 AND insight_date=$2`, userID, date).Scan(&summary, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return summary, version, err
}

func (r *Repository) CompleteBatch(ctx context.Context, batch Batch, result BatchResult, model string, inputTokens, outputTokens int64, chunks, eligible int, timezone string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	update, err := tx.ExecContext(ctx, `UPDATE ai_work_insight_batches SET status='done',work_summary=$3,task_categories=$4,
		explicit_projects=$5,explicit_modules=$6,change_types=$7,business_topics=$8,representative_items=$9,evidence_level=$10,
		analyzer_model=$11,analyzer_input_tokens=$12,analyzer_output_tokens=$13,chunk_count=$14,analyzed_at=NOW(),error_code='',updated_at=NOW()
		WHERE id=$1 AND status='processing' AND claim_version=$2`, batch.ID, batch.ClaimVersion, result.WorkSummary,
		jsonValue(result.TaskCategories), jsonValue(result.ExplicitProjects), jsonValue(result.ExplicitModules), jsonValue(result.ChangeTypes),
		jsonValue(result.BusinessTopics), jsonValue(result.RepresentativeItems), result.EvidenceLevel, model, inputTokens, outputTokens, chunks)
	if err := requireAffected(update, err); err != nil {
		return err
	}
	state, err := loadDailyState(ctx, tx, batch.UserID, batch.LocalDate)
	if err != nil {
		return err
	}
	result.WorkSummary = mergeWorkSummaries(result.WorkSummary, state.summary)
	for _, category := range result.TaskCategories {
		state.categoryStats[category]++
	}
	state.projects = mergeUnique(state.projects, result.ExplicitProjects)
	state.modules = mergeUnique(state.modules, result.ExplicitModules)
	state.changeTypes = mergeUnique(state.changeTypes, result.ChangeTypes)
	state.businessTopics = mergeUnique(state.businessTopics, result.BusinessTopics)
	start, end, err := usageDayBounds(batch.LocalDate, timezone)
	if err != nil {
		return err
	}
	var requests, tokens int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(input_tokens+output_tokens+cache_creation_tokens+cache_read_tokens),0)
		FROM usage_logs WHERE user_id=$1 AND created_at >= $2 AND created_at < $3`, batch.UserID, start, end).Scan(&requests, &tokens); err != nil {
		return err
	}
	samples := state.sampleCount + batch.SampleCount
	var failures, covered int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_work_insight_samples
		WHERE user_id=$1 AND local_date=$2 AND status IN ('failed','dropped')`, batch.UserID, batch.LocalDate).Scan(&failures); err != nil {
		return err
	}
	failures = max(failures, state.failedCount)
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT active_session_id) FROM ai_work_insight_batches WHERE user_id=$1 AND local_date=$2 AND status='done'`, batch.UserID, batch.LocalDate).Scan(&covered); err != nil {
		return err
	}
	if covered > eligible {
		eligible = covered
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO ai_user_daily_work_insights
		(user_id,username_snapshot,insight_date,business_request_count,business_total_tokens,sample_count,failed_sample_count,
		eligible_active_session_count,covered_active_session_count,task_category_stats,explicit_projects,explicit_modules,change_types,business_topics,
		daily_summary,last_analyzed_sample_id,summary_version,last_analyzed_at,computed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW(),NOW())
		ON CONFLICT (user_id,insight_date) DO UPDATE SET username_snapshot=EXCLUDED.username_snapshot,
		business_request_count=EXCLUDED.business_request_count,business_total_tokens=EXCLUDED.business_total_tokens,
		sample_count=EXCLUDED.sample_count,failed_sample_count=EXCLUDED.failed_sample_count,
		eligible_active_session_count=GREATEST(ai_user_daily_work_insights.eligible_active_session_count,EXCLUDED.eligible_active_session_count),
		covered_active_session_count=EXCLUDED.covered_active_session_count,task_category_stats=EXCLUDED.task_category_stats,
		explicit_projects=EXCLUDED.explicit_projects,explicit_modules=EXCLUDED.explicit_modules,change_types=EXCLUDED.change_types,
		business_topics=EXCLUDED.business_topics,
		daily_summary=EXCLUDED.daily_summary,last_analyzed_sample_id=EXCLUDED.last_analyzed_sample_id,
		summary_version=EXCLUDED.summary_version,last_analyzed_at=NOW(),finalized_at=NULL,computed_at=NOW()`,
		batch.UserID, batch.Username, batch.LocalDate, requests, tokens, samples, failures, eligible, covered,
		jsonValue(state.categoryStats), jsonValue(state.projects), jsonValue(state.modules), jsonValue(state.changeTypes),
		jsonValue(state.businessTopics), result.WorkSummary, batch.LastSampleID, state.version+1)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ai_work_insight_samples WHERE batch_id=$1 AND status='batched'`, batch.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func loadDailyState(ctx context.Context, tx *sql.Tx, userID int64, date time.Time) (dailyState, error) {
	state := dailyState{categoryStats: map[string]int{}}
	var categoryRaw, projectsRaw, modulesRaw, changesRaw, topicsRaw []byte
	err := tx.QueryRowContext(ctx, `SELECT daily_summary,task_category_stats,explicit_projects,explicit_modules,change_types,business_topics,summary_version,sample_count,failed_sample_count
		FROM ai_user_daily_work_insights WHERE user_id=$1 AND insight_date=$2 FOR UPDATE`, userID, date).Scan(&state.summary, &categoryRaw, &projectsRaw, &modulesRaw, &changesRaw, &topicsRaw, &state.version, &state.sampleCount, &state.failedCount)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(categoryRaw, &state.categoryStats); err != nil {
		return state, err
	}
	if err := json.Unmarshal(projectsRaw, &state.projects); err != nil {
		return state, err
	}
	if err := json.Unmarshal(modulesRaw, &state.modules); err != nil {
		return state, err
	}
	if err := json.Unmarshal(changesRaw, &state.changeTypes); err != nil {
		return state, err
	}
	if err := json.Unmarshal(topicsRaw, &state.businessTopics); err != nil {
		return state, err
	}
	return state, nil
}

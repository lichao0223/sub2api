package workinsight

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPreviousFinalizeTargetRunsOnceAfterConfiguredTime(t *testing.T) {
	cfg := storedConfig{Config: DefaultConfig()}
	before := time.Date(2026, 8, 11, 0, 14, 0, 0, time.FixedZone("CST", 8*60*60))
	_, _, due := previousFinalizeTarget(before, cfg)
	require.False(t, due)

	key, date, due := previousFinalizeTarget(before.Add(time.Minute), cfg)
	require.True(t, due)
	require.Equal(t, "2026-08-10", key)
	require.Equal(t, "2026-08-10", date.Format("2006-01-02"))
}

func TestCreateDueBatchesFiltersDueSessionsInSQL(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	cfg := DefaultConfig()
	now := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("WITH due_sessions").WithArgs(
		cfg.MaxSamplesPerBatch, cfg.MaxInputTokens, true,
		now.Add(-time.Duration(cfg.AnalysisMaxWaitMinutes)*time.Minute),
		now.Add(-time.Duration(cfg.AnalysisIdleMinutes)*time.Minute),
		false, now.Add(-time.Duration(cfg.FixedIntervalMinutes)*time.Minute), false,
	).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "username_snapshot", "local_date", "active_session_id", "estimated_tokens", "created_at"}))
	mock.ExpectCommit()

	created, err := repo.CreateDueBatches(context.Background(), now, cfg, "")
	require.NoError(t, err)
	require.Zero(t, created)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListSamplesReturnsPageAndTotal(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ai_work_insight_samples`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(41))
	mock.ExpectQuery(`ORDER BY id DESC LIMIT \$1 OFFSET \$2`).WithArgs(20, 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "username_snapshot", "provider", "protocol", "endpoint", "requested_model",
			"local_date", "active_session_id", "sample_reason", "estimated_tokens", "prompt_chars", "analyzed_chars",
			"truncated", "redacted_preview", "status", "attempts", "error_code", "created_at", "updated_at", "batch_id",
		}))

	items, total, err := repo.ListSamples(context.Background(), 2, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(41), total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListBatchesTreatsAdminStoppedAsPending(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*status='queued'.*admin_stopped`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)FROM ai_work_insight_batches WHERE.*status='queued'.*admin_stopped.*ORDER BY id DESC`).
		WithArgs(20, 0).WillReturnRows(sqlmock.NewRows([]string{
		"id", "user_id", "username_snapshot", "local_date", "active_session_id", "sample_count", "trigger_reason", "status",
		"attempts", "error_code", "analyzer_model", "analyzer_input_tokens", "analyzer_output_tokens", "analyzed_at", "created_at", "updated_at",
	}))

	items, total, err := NewRepository(db).ListBatches(context.Background(), 1, 20, "pending")
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(1), total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClaimBatchClearsPreviousRetryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)UPDATE ai_work_insight_batches b SET status='processing'.*error_code=''`).
		WithArgs(now.UTC(), now.Add(-time.Minute).UTC()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "username_snapshot", "local_date", "active_session_id", "first_sample_id", "last_sample_id",
			"sample_count", "estimated_input_tokens", "trigger_reason", "attempts", "claim_version", "base_summary_version", "created_at",
		}).AddRow(1, 2, "用户", now, "session", 3, 4, 2, 100, "manual", 2, 2, 0, now))

	batch, claimed, err := NewRepository(db).ClaimBatch(context.Background(), now, time.Minute)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, int64(1), batch.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClearTerminalLogsKeepsActiveWork(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)DELETE FROM ai_work_insight_samples WHERE status IN.*admin_stopped`).WillReturnResult(sqlmock.NewResult(0, 9))
	mock.ExpectExec(`(?s)DELETE FROM ai_work_insight_batches WHERE status IN.*admin_stopped`).WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()

	result, err := NewRepository(db).ClearTerminalLogs(context.Background())
	require.NoError(t, err)
	require.Equal(t, ClearLogsResult{Samples: 9, Batches: 4}, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequeueBatchResetsBatchAndSamples(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ai_work_insight_batches SET status='queued'`).WithArgs(int64(12), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE ai_work_insight_samples SET status='batched'`).WithArgs(int64(12), now).
		WillReturnResult(sqlmock.NewResult(0, 8))
	mock.ExpectCommit()

	require.NoError(t, NewRepository(db).RequeueBatch(context.Background(), 12, now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRequeueBatchesReturnsActualConcurrentResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ai_work_insight_batches SET status='queued'`).
		WithArgs(sqlmock.AnyArg(), now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE ai_work_insight_samples SET status='batched'`).
		WithArgs(sqlmock.AnyArg(), now).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	retried, err := NewRepository(db).RequeueBatches(context.Background(), []int64{12, 13}, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), retried)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStopBatchInvalidatesWorkerClaimAndKeepsPayloadRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ai_work_insight_batches SET status='dropped'.*claim_version=claim_version\+1`).
		WithArgs(int64(12), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE ai_work_insight_samples SET status='dropped'`).
		WithArgs(int64(12), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	service := &Service{repo: NewRepository(db)}
	workerCtx, cancel := context.WithCancel(context.Background())
	service.batchCancels.Store(int64(12), cancel)
	require.NoError(t, service.StopBatchNow(context.Background(), 12))
	select {
	case <-workerCtx.Done():
	default:
		t.Fatal("stopping a processing batch must cancel its worker")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteOlderPendingDuplicatesKeepsNewestExactPrompt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	date := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`DELETE FROM ai_work_insight_samples WHERE user_id=\$1 AND local_date=\$2 AND prompt_hash=\$3`).
		WithArgs(int64(9), date, "same-hash", int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(28).AddRow(29))

	ids, err := NewRepository(db).DeleteOlderPendingDuplicates(context.Background(), 30, 9, date, "same-hash")
	require.NoError(t, err)
	require.Equal(t, []int64{28, 29}, ids)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListUserRankingAggregatesBeforePagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM \(SELECT 1 FROM ai_user_daily_work_insights`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`GROUP BY COALESCE\(d.user_id::text,'deleted:'\|\|d.username_snapshot\).*ORDER BY SUM\(d.business_total_tokens\) DESC`).
		WithArgs(20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"latest_insight_id", "user_id", "username", "start_date", "end_date", "insight_days",
			"business_request_count", "business_total_tokens", "sample_count", "failed_sample_count",
			"eligible_active_session_count", "covered_active_session_count", "latest_summary", "last_analyzed_at",
		}))

	items, total, err := repo.ListUserRanking(context.Background(), DailyFilter{Page: 1, Size: 20})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(1), total)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDailyWhereExcludesUsageOnlyRows(t *testing.T) {
	where, args := dailyWhere(DailyFilter{})
	require.Contains(t, where, "d.sample_count > 0")
	require.Contains(t, where, "u.role='admin'")
	require.Empty(t, args)
}

func TestReconcileUsageReusesExistingUsageAndErrorLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	date := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	start, end, err := usageDayBounds(date, "Asia/Shanghai")
	require.NoError(t, err)

	mock.ExpectExec(`WITH usage_base AS`).WithArgs(date, start, end).WillReturnResult(sqlmock.NewResult(0, 1))
	updated, err := repo.ReconcileUsage(context.Background(), date, "Asia/Shanghai")
	require.NoError(t, err)
	require.Equal(t, int64(1), updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

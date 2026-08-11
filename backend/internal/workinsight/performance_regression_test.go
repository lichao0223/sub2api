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

	created, err := repo.CreateDueBatches(context.Background(), now, cfg, false)
	require.NoError(t, err)
	require.Zero(t, created)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListSamplesUsesCursorWithoutCountOrOffset(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	mock.ExpectQuery(`WHERE id<\$1 ORDER BY id DESC LIMIT \$2`).WithArgs(int64(100), 21).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "username_snapshot", "provider", "protocol", "endpoint", "requested_model",
			"local_date", "active_session_id", "sample_reason", "estimated_tokens", "prompt_chars", "analyzed_chars",
			"redacted_preview", "status", "attempts", "error_code", "created_at", "updated_at", "batch_id",
		}))

	items, next, hasMore, err := repo.ListSamples(context.Background(), 100, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, next)
	require.False(t, hasMore)
	require.NoError(t, mock.ExpectationsWereMet())
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

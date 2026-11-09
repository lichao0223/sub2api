package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsControlReadRejectsMissingOrInvalidConfiguration(t *testing.T) {
	for _, value := range []string{"", `{"enabled":true}`, `{"version":1}`, `{"enabled":null,"version":1}`, `{"enabled":"yes","version":1}`} {
		t.Run(value, func(t *testing.T) {
			db, mock := newSQLMock(t)
			mock.ExpectQuery("SELECT value FROM settings").WithArgs(service.AnalyticsTaskSettingKey).
				WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(value))
			_, err := (&analyticsTaskRepository{db: db}).GetAnalyticsTaskControl(context.Background())
			require.Error(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAnalyticsControlCompareAndSwapConflict(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery("UPDATE settings SET value").WithArgs(service.AnalyticsTaskSettingKey, false, int64(3), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	_, err := (&analyticsTaskRepository{db: db}).UpdateAnalyticsTaskControl(context.Background(), false, 3, 7)
	require.ErrorIs(t, err, service.ErrAnalyticsControlConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsControlCompareAndSwapReturnsPersistedVersion(t *testing.T) {
	db, mock := newSQLMock(t)
	mock.ExpectQuery("UPDATE settings SET value").WithArgs(service.AnalyticsTaskSettingKey, true, int64(3), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{"enabled":true,"version":4,"updated_by":7,"updated_at":"2026-11-06T10:00:00Z"}`))
	control, err := (&analyticsTaskRepository{db: db}).UpdateAnalyticsTaskControl(context.Background(), true, 3, 7)
	require.NoError(t, err)
	require.True(t, control.Enabled)
	require.EqualValues(t, 4, control.Version)
	require.EqualValues(t, 7, control.UpdatedBy)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsPublicationGuardRejectsPauseOrChangedVersion(t *testing.T) {
	for _, value := range []string{`{"enabled":false,"version":3}`, `{"enabled":true,"version":4}`} {
		t.Run(value, func(t *testing.T) {
			db, mock := newSQLMock(t)
			mock.ExpectQuery("SELECT value FROM settings.*FOR SHARE").WithArgs(service.AnalyticsTaskSettingKey).
				WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(value))
			repo := &analyticsGuardRepo{}
			controller := service.NewAnalyticsTaskService(repo, "test")
			err := controller.Run(context.Background(), func(ctx context.Context) error {
				return checkAnalyticsPublication(ctx, db)
			})
			require.ErrorIs(t, err, service.ErrAnalyticsTasksPaused)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

type analyticsGuardRepo struct{}

func (*analyticsGuardRepo) GetAnalyticsTaskControl(context.Context) (service.AnalyticsTaskControl, error) {
	return service.AnalyticsTaskControl{Enabled: true, Version: 3}, nil
}
func (*analyticsGuardRepo) UpdateAnalyticsTaskControl(context.Context, bool, int64, int64) (service.AnalyticsTaskControl, error) {
	panic("unused")
}
func (*analyticsGuardRepo) RecordAnalyticsTaskWorker(context.Context, service.AnalyticsTaskWorker) error {
	return nil
}
func (*analyticsGuardRepo) ListAnalyticsTaskWorkers(context.Context) ([]service.AnalyticsTaskWorker, error) {
	return nil, nil
}
func (*analyticsGuardRepo) GetAnalyticsProgress(context.Context) (*service.AnalyticsProgress, error) {
	return &service.AnalyticsProgress{}, nil
}

func TestAnalyticsWorkerPersistenceKeepsLatestSuccessAndPrunesOldInstances(t *testing.T) {
	db, mock := newSQLMock(t)
	now := time.Now().UTC()
	mock.ExpectExec("(?s)WITH expired AS.*DELETE FROM analytics_task_workers.*INSERT INTO analytics_task_workers.*COALESCE").
		WithArgs("a", 1, int64(3), now, nil, "2026-11-01", now).WillReturnResult(sqlmock.NewResult(0, 1))
	err := (&analyticsTaskRepository{db: db}).RecordAnalyticsTaskWorker(context.Background(),
		service.AnalyticsTaskWorker{InstanceID: "a", Running: 1, ObservedVersion: 3, LastSeenAt: now, AnalyticsDay: "2026-11-01", AnalyticsStartedAt: &now})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsWorkerRegistrationRejectsPauseBetweenReadAndStart(t *testing.T) {
	db, mock := newSQLMock(t)
	now := time.Now().UTC()
	mock.ExpectExec("(?s)WITH expired AS.*FOR SHARE.*INSERT INTO analytics_task_workers.*WHERE .*EXISTS").
		WithArgs("a", 1, int64(3), now, nil, "", nil).WillReturnResult(sqlmock.NewResult(0, 0))
	err := (&analyticsTaskRepository{db: db}).RecordAnalyticsTaskWorker(context.Background(),
		service.AnalyticsTaskWorker{InstanceID: "a", Running: 1, ObservedVersion: 3, LastSeenAt: now})
	require.ErrorIs(t, err, service.ErrAnalyticsTasksPaused)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsPauseRequeuesRecalculation(t *testing.T) {
	for _, err := range []error{service.ErrAnalyticsTasksPaused, context.Canceled, context.DeadlineExceeded} {
		status, retry := recalculationFailureStatus(err)
		require.Equal(t, "queued", status)
		require.True(t, retry)
	}
}

func TestAnalyticsPublicationWithoutControllerKeepsExistingRepositoryCallers(t *testing.T) {
	db, mock := newSQLMock(t)
	require.NoError(t, checkAnalyticsPublication(context.Background(), db))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAnalyticsRecalculationRollsBackCandidateAndProgressOnPause(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "paused", true: "enabled"}[enabled], func(t *testing.T) {
			db, mock := newSQLMock(t)
			day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM cost_daily_aggregates WHERE bucket_date").
				WithArgs(day).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("WITH resolved AS").WithArgs(day, day.AddDate(0, 0, 1)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM cost_daily_aggregates WHERE bucket_date").
				WithArgs(day).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT u.id,u.plan_id,v.id").WillReturnRows(sqlmock.NewRows([]string{
				"unit_id", "plan_id", "version_id", "monthly", "unit_from", "unit_to", "effective_from", "effective_to",
			}))
			mock.ExpectExec("UPDATE cost_jobs SET completed_days").WithArgs(int64(7), 1).
				WillReturnResult(sqlmock.NewResult(0, 1))
			value := `{"enabled":false,"version":4}`
			if enabled {
				value = `{"enabled":true,"version":3}`
			}
			mock.ExpectQuery("SELECT value FROM settings.*FOR SHARE").WithArgs(service.AnalyticsTaskSettingKey).
				WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(value))
			if enabled {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			controller := service.NewAnalyticsTaskService(&analyticsGuardRepo{}, "test")
			err := controller.Run(context.Background(), func(ctx context.Context) error {
				return (&costManagementRepository{db: db}).rebuildCostDay(ctx, 7, day, 1)
			})
			if enabled {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, service.ErrAnalyticsTasksPaused)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

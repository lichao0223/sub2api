//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsTaskControlMigrationDefaultsAndIdempotency(t *testing.T) {
	ctx := context.Background()
	migration, err := migrations.FS.ReadFile("921_data_analytics_task_control.sql")
	require.NoError(t, err)
	for _, existingCost := range []bool{false, true} {
		t.Run(fmt.Sprint(existingCost), func(t *testing.T) {
			tx, err := integrationDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			schema := fmt.Sprintf("analytics_migration_%d", time.Now().UnixNano())
			_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schema)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `CREATE TABLE settings(key text PRIMARY KEY,value text,updated_at timestamptz);
				CREATE TABLE cost_jobs(job_key text,last_success_at timestamptz);`)
			require.NoError(t, err)
			if existingCost {
				_, err = tx.ExecContext(ctx, `INSERT INTO cost_jobs VALUES('incremental',NOW())`)
				require.NoError(t, err)
			}
			_, err = tx.ExecContext(ctx, string(migration))
			require.NoError(t, err)
			var value string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, service.AnalyticsTaskSettingKey).Scan(&value))
			control, err := decodeAnalyticsControl(value)
			require.NoError(t, err)
			require.Equal(t, existingCost, control.Enabled)
			_, err = tx.ExecContext(ctx, `UPDATE settings SET value='{"enabled":false,"version":9}' WHERE key=$1`, service.AnalyticsTaskSettingKey)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, string(migration))
			require.NoError(t, err)
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, service.AnalyticsTaskSettingKey).Scan(&value))
			require.JSONEq(t, `{"enabled":false,"version":9}`, value)
		})
	}
}

func TestAnalyticsPublicationOrdersCommitBeforePauseAndRejectsOldVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var original string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, service.AnalyticsTaskSettingKey).Scan(&original))
	suffix := fmt.Sprint(time.Now().UnixNano())
	workerA, workerB, probe := "test-a-"+suffix, "test-b-"+suffix, "analytics-probe-"+suffix
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `UPDATE settings SET value=$2 WHERE key=$1`, service.AnalyticsTaskSettingKey, original)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM analytics_task_workers WHERE instance_id IN($1,$2)`, workerA, workerB)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM settings WHERE key=$1`, probe)
	})
	repo := &analyticsTaskRepository{db: integrationDB}
	a, b := service.NewAnalyticsTaskService(repo, workerA), service.NewAnalyticsTaskService(repo, workerB)
	control, err := repo.GetAnalyticsTaskControl(ctx)
	require.NoError(t, err)
	control, err = a.SetEnabled(ctx, true, control.Version, 7)
	require.NoError(t, err)
	err = a.Run(ctx, func(taskCtx context.Context) error {
		tx, err := integrationDB.BeginTx(taskCtx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := checkAnalyticsPublication(taskCtx, tx); err != nil {
			return err
		}
		// A pause cannot be acknowledged while an already checked commit
		// still holds the settings row's shared lock.
		blockedCtx, unblock := context.WithTimeout(ctx, 100*time.Millisecond)
		defer unblock()
		_, err = b.SetEnabled(blockedCtx, false, control.Version, 7)
		require.Error(t, err)
		_, err = tx.ExecContext(taskCtx, `INSERT INTO settings(key,value,updated_at) VALUES($1,'committed',NOW())`, probe)
		if err != nil {
			return err
		}
		return tx.Commit()
	})
	require.NoError(t, err)
	var value string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, probe).Scan(&value))
	require.Equal(t, "committed", value)

	// Pause wins before publication: roll back both the candidate value and
	// its progress, even when the remote worker has not refreshed yet.
	err = a.Run(ctx, func(taskCtx context.Context) error {
		_, err := b.SetEnabled(ctx, false, control.Version, 7)
		if err != nil {
			return err
		}
		tx, err := integrationDB.BeginTx(taskCtx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err = tx.ExecContext(taskCtx, `UPDATE settings SET value='unpublished' WHERE key=$1`, probe); err != nil {
			return err
		}
		return checkAnalyticsPublication(taskCtx, tx)
	})
	require.ErrorIs(t, err, service.ErrAnalyticsTasksPaused)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, probe).Scan(&value))
	require.Equal(t, "committed", value)
	require.ErrorIs(t, repo.RecordAnalyticsTaskWorker(ctx, service.AnalyticsTaskWorker{
		InstanceID: workerA, Running: 1, ObservedVersion: control.Version, LastSeenAt: time.Now(),
	}), service.ErrAnalyticsTasksPaused)
}

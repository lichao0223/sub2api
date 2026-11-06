package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type analyticsTaskRepository struct{ db *sql.DB }

func NewAnalyticsTaskRepository(db *sql.DB) service.AnalyticsTaskRepository {
	return &analyticsTaskRepository{db: db}
}

func decodeAnalyticsControl(value string) (service.AnalyticsTaskControl, error) {
	var control service.AnalyticsTaskControl
	raw := struct {
		*service.AnalyticsTaskControl
		Enabled *bool `json:"enabled"`
	}{AnalyticsTaskControl: &control}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return control, fmt.Errorf("invalid analytics task setting: %w", err)
	}
	if raw.Enabled == nil || control.Version < 1 {
		return control, errors.New("invalid analytics task control")
	}
	control.Enabled = *raw.Enabled
	return control, nil
}

func (r *analyticsTaskRepository) GetAnalyticsTaskControl(ctx context.Context) (service.AnalyticsTaskControl, error) {
	var value string
	if err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1`, service.AnalyticsTaskSettingKey).Scan(&value); err != nil {
		return service.AnalyticsTaskControl{}, err
	}
	return decodeAnalyticsControl(value)
}

func (r *analyticsTaskRepository) UpdateAnalyticsTaskControl(ctx context.Context, enabled bool, version, actorID int64) (service.AnalyticsTaskControl, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `UPDATE settings SET value=jsonb_build_object(
		'enabled',$2::boolean,'version',$3::bigint+1,'updated_by',$4::bigint,'updated_at',NOW())::text,updated_at=NOW()
		WHERE key=$1 AND (value::jsonb->>'version')::bigint=$3 RETURNING value`,
		service.AnalyticsTaskSettingKey, enabled, version, actorID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return service.AnalyticsTaskControl{}, service.ErrAnalyticsControlConflict
	}
	if err != nil {
		return service.AnalyticsTaskControl{}, err
	}
	return decodeAnalyticsControl(value)
}

func (r *analyticsTaskRepository) RecordAnalyticsTaskWorker(ctx context.Context, worker service.AnalyticsTaskWorker) error {
	// Keep seven days of heartbeats and the most recent successful worker.
	result, err := r.db.ExecContext(ctx, `WITH expired AS (
		DELETE FROM analytics_task_workers WHERE last_seen_at<NOW()-INTERVAL '7 days' AND instance_id<>$1
		  AND instance_id<>COALESCE((SELECT instance_id FROM analytics_task_workers
		    WHERE last_success_at IS NOT NULL ORDER BY last_success_at DESC,instance_id LIMIT 1),'')
	), control AS MATERIALIZED (
	  SELECT value::jsonb config FROM settings WHERE key='data_analytics.tasks_enabled' FOR SHARE
	)
	INSERT INTO analytics_task_workers(instance_id,running,observed_version,last_seen_at,last_success_at)
	SELECT $1::varchar,$2::integer,$3::bigint,$4::timestamptz,$5::timestamptz WHERE $2::integer=0 OR EXISTS(
	  SELECT 1 FROM control WHERE (config->>'enabled')::boolean AND (config->>'version')::bigint=$3
	) ON CONFLICT(instance_id) DO UPDATE SET
	  running=EXCLUDED.running,observed_version=EXCLUDED.observed_version,last_seen_at=EXCLUDED.last_seen_at,
	  last_success_at=COALESCE(EXCLUDED.last_success_at,analytics_task_workers.last_success_at)`,
		worker.InstanceID, worker.Running, worker.ObservedVersion, worker.LastSeenAt, worker.LastSuccessAt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return service.ErrAnalyticsTasksPaused
	}
	return err
}

func (r *analyticsTaskRepository) ListAnalyticsTaskWorkers(ctx context.Context) ([]service.AnalyticsTaskWorker, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT instance_id,running,observed_version,last_seen_at,last_success_at FROM analytics_task_workers`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	workers := make([]service.AnalyticsTaskWorker, 0)
	for rows.Next() {
		var worker service.AnalyticsTaskWorker
		if err := rows.Scan(&worker.InstanceID, &worker.Running, &worker.ObservedVersion, &worker.LastSeenAt, &worker.LastSuccessAt); err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}
	return workers, rows.Err()
}

type analyticsControlQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Call inside the publishing transaction, after the expensive work. The row
// lock orders commit against pause without blocking pause during the scan.
func checkAnalyticsPublication(ctx context.Context, tx analyticsControlQuerier) error {
	version, controlled := service.AnalyticsTaskVersion(ctx)
	if !controlled {
		return nil
	}
	var value string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=$1 FOR SHARE`, service.AnalyticsTaskSettingKey).Scan(&value); err != nil {
		return err
	}
	control, err := decodeAnalyticsControl(value)
	if err != nil {
		return err
	}
	if !control.Enabled || control.Version != version {
		return service.ErrAnalyticsTasksPaused
	}
	return nil
}

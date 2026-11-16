//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Exercise both the upstream group-rollup migrations and local analytics
// triggers on real PostgreSQL partitions, using an isolated committed schema.
func analyticsPartitionRetentionFixture(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	schema := fmt.Sprintf("analytics_retention_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+pq.QuoteIdentifier(schema)+" CASCADE")
	})
	db := openGroupUsageRollupSchemaDB(t, schema, "Asia/Shanghai")
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	seedAnalyticsFixture(t, ctx, tx, true)
	_, err = tx.ExecContext(ctx, "ALTER TABLE usage_logs ADD COLUMN group_id BIGINT")
	require.NoError(t, err)
	for _, name := range []string{
		"222_group_usage_daily_rollups.sql",
		"223_group_usage_rollup_timezone.sql",
		"251_group_usage_rollup_nonblocking_build.sql",
	} {
		migration, readErr := migrations.FS.ReadFile(name)
		require.NoError(t, readErr)
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_logs(id,user_id,account_id,model,created_at,input_tokens)
		VALUES(1,1,1,'retention-model',NOW()-INTERVAL '2 days',1000);
		UPDATE analytics_runtime_state SET activated=TRUE;
	`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	return db
}

func TestAnalyticsPartitionRetentionArchivesBeforeDropping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := analyticsPartitionRetentionFixture(t, ctx)
	repo := newDashboardAggregationRepositoryWithSQL(db)

	// A DROP would bypass analytics' statement DELETE trigger. Retain the
	// nonempty partition, then refuse row cleanup until accounting is complete.
	require.NoError(t, repo.dropUsageLogsPartition(ctx, "usage_logs_all"))
	err := repo.cleanupUsageLogsBatches(ctx, time.Now())
	require.ErrorContains(t, err, "数据分析尚未完整核算")
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs").Scan(&count))
	require.Equal(t, 1, count, "failed accounting must roll back deletion")

	_, err = db.ExecContext(ctx, `
		INSERT INTO analytics_daily
		SELECT f.* FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f;
		INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count)
		SELECT day,NOW(),SUM(requests),0 FROM analytics_daily GROUP BY day;
		DELETE FROM analytics_dirty_days;
	`)
	require.NoError(t, err)
	require.NoError(t, repo.cleanupUsageLogsBatches(ctx, time.Now()))
	var tokens, requests, archived int64
	require.NoError(t, db.QueryRowContext(ctx, "SELECT SUM(input)::bigint,SUM(requests)::bigint FROM analytics_archived_daily").Scan(&tokens, &requests))
	require.EqualValues(t, 1000, tokens)
	require.EqualValues(t, 1, requests)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT SUM(archived_count)::bigint FROM analytics_bucket_state").Scan(&archived))
	require.EqualValues(t, 1, archived)

	// After parent-table DELETE has archived the facts, an empty partition is
	// safe to remove. A retry must not duplicate archived accounting.
	require.NoError(t, repo.dropUsageLogsPartition(ctx, "usage_logs_all"))
	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT to_regclass('usage_logs_all') IS NOT NULL").Scan(&exists))
	require.False(t, exists)
	require.NoError(t, repo.cleanupUsageLogsBatches(ctx, time.Now()))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT SUM(requests)::bigint FROM analytics_archived_daily").Scan(&requests))
	require.EqualValues(t, 1, requests)
}

func TestAnalyticsPartitionRetentionWaitsForAggregation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := analyticsPartitionRetentionFixture(t, ctx)
	blocker, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	_, err = blocker.ExecContext(ctx, "SELECT pg_advisory_xact_lock(9072028)")
	require.NoError(t, err)

	repo := newDashboardAggregationRepositoryWithSQL(db)
	err = repo.dropUsageLogsPartition(ctx, "usage_logs_all")
	require.ErrorIs(t, err, service.ErrCostAggregationBusy)
	var count int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_logs").Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, blocker.Rollback())
	require.NoError(t, repo.dropUsageLogsPartition(ctx, "usage_logs_all"))
}

//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestCostSaveFailsFastWhileAnalyticsOwnsLock(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	blocker, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(9072028)`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SAVEPOINT cost_save`)
	require.NoError(t, err)
	// A blocking regression must fail this test instead of waiting for the batch limit.
	fastCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = tx.ExecContext(fastCtx, `INSERT INTO account_cost_configs
		VALUES(1,1,'excluded',NULL,NULL,NOW()-INTERVAL '1 minute',NULL)`)
	var pgErr *pq.Error
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, pq.ErrorCode("55P03"), pgErr.Code)
	require.Contains(t, pgErr.Message, "本次未保存")
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT cost_save`)
	require.NoError(t, err)
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_cost_configs`).Scan(&count))
	require.Zero(t, count, "failed save must not leave a partial configuration")
	require.NoError(t, blocker.Rollback())
	_, err = tx.ExecContext(ctx, `INSERT INTO account_cost_configs
		VALUES(1,1,'excluded',NULL,NULL,NOW()-INTERVAL '1 minute',NULL)`)
	require.NoError(t, err, "saving must succeed once the batch releases its lock")
}

func TestCostRecalculationEarliestDatePreservesBeijingBoundary(t *testing.T) {
	for _, hasLogs := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty history", true: "UTC date differs from Beijing"}[hasLogs], func(t *testing.T) {
			tx, ctx := analyticsMigrationFixture(t, false)
			_, err := tx.ExecContext(ctx, `ALTER TABLE cost_jobs ADD COLUMN kind TEXT,ADD COLUMN status TEXT,
				ADD COLUMN start_date DATE,ADD COLUMN end_date DATE,ADD COLUMN total_days INTEGER`)
			require.NoError(t, err)
			expected := "2025-12-01"
			if hasLogs {
				_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(id,created_at)
					VALUES(1,'2025-12-31T16:01:00Z'),(2,'2026-01-02T00:00:00Z')`)
				require.NoError(t, err)
				expected = "2026-01-01"
			}
			require.NoError(t, enqueueCostRecalculationTx(ctx, tx, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)))
			var start string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT start_date::text FROM cost_jobs WHERE kind='recalculation'`).Scan(&start))
			require.Equal(t, expected, start)
		})
	}
}

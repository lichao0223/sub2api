//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserTokenRankingPreservesTotalsAcrossDeleteAndMigration(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)

	source := mustCreateUser(t, client, &service.User{Email: "intern-ranking@test.com", Username: "Intern"})
	target := mustCreateUser(t, client, &service.User{Email: "employee-ranking@test.com", Username: "Employee"})
	sourceKey := mustCreateApiKey(t, client, &service.APIKey{UserID: source.ID, Key: "sk-intern-ranking", Name: "intern"})
	targetKey := mustCreateApiKey(t, client, &service.APIKey{UserID: target.ID, Key: "sk-employee-ranking", Name: "employee"})
	account := mustCreateAccount(t, client, &service.Account{Name: "ranking-migration-account"})
	start := time.Date(2098, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	nextStart := end
	nextEnd := nextStart.AddDate(0, 1, 0)

	createUsageLogForMigrationTest(t, repo, source, sourceKey, account, 100, 50, 1.25, start.Add(time.Hour))
	createUsageLogForMigrationTest(t, repo, target, targetKey, account, 20, 10, 0.25, start.Add(2*time.Hour))
	createUsageLogForMigrationTest(t, repo, source, sourceKey, account, 60, 40, 0.75, nextStart.Add(time.Hour))
	batchID := insertExternalUsageForMigrationTest(t, ctx, tx, source.ID, start, 30)

	_, err := client.User.UpdateOneID(source.ID).SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	before, err := repo.GetUserTokenRanking(ctx, start, end, 0)
	require.NoError(t, err)
	requireRankingItem(t, before.Ranking, source.ID, 180)
	requireRankingItem(t, before.Ranking, target.ID, 30)
	require.Equal(t, int64(210), before.TotalTokens)

	_, err = tx.ExecContext(ctx, `INSERT INTO user_usage_migrations (source_user_id, target_user_id) VALUES ($1, $2)`, source.ID, target.ID)
	require.NoError(t, err)

	after, err := repo.GetUserTokenRanking(ctx, start, end, 0)
	require.NoError(t, err)
	requireNoRankingItem(t, after.Ranking, source.ID)
	requireRankingItem(t, after.Ranking, target.ID, 210)
	require.Equal(t, before.TotalRequests, after.TotalRequests)
	require.Equal(t, before.TotalTokens, after.TotalTokens)
	require.InDelta(t, before.TotalActualCost, after.TotalActualCost, 0.000001)

	nextMonth, err := repo.GetUserTokenRanking(ctx, nextStart, nextEnd, 0)
	require.NoError(t, err)
	requireNoRankingItem(t, nextMonth.Ranking, source.ID)
	requireRankingItem(t, nextMonth.Ranking, target.ID, 100)
	require.Equal(t, int64(100), nextMonth.TotalTokens)

	require.NoError(t, repo.VoidExternalUsageImportBatch(ctx, batchID, target.ID))
	afterVoid, err := repo.GetUserTokenRanking(ctx, start, end, 0)
	require.NoError(t, err)
	requireRankingItem(t, afterVoid.Ranking, target.ID, 180)
}

func TestNonworkRankingMigratesDeletedUserWithoutChangingTotals(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()
	repo := newUsageLogRepositoryWithSQL(client, tx)
	source := mustCreateUser(t, client, &service.User{Email: "intern-nonwork@test.com", Username: "Intern"})
	target := mustCreateUser(t, client, &service.User{Email: "employee-nonwork@test.com", Username: "Employee"})
	day := time.Date(2098, 8, 5, 0, 0, 0, 0, time.UTC)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO usage_nonwork_daily_stat_runs (bucket_date, timezone, computed_at)
		VALUES ($1, 'Asia/Shanghai', NOW())
	`, day)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_nonwork_daily_user_stats
			(bucket_date, timezone, user_id, segment, requests, total_tokens, actual_cost, active_ms, active_sessions)
		VALUES
			($1, 'Asia/Shanghai', $2, 'after_hours', 2, 100, 1.00, 60000, 1),
			($1, 'Asia/Shanghai', $3, 'after_hours', 1, 40, 0.40, 20000, 1)
	`, day, source.ID, target.ID)
	require.NoError(t, err)
	_, err = client.User.UpdateOneID(source.ID).SetDeletedAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	before, err := repo.GetUserNonworkTokenRanking(ctx, day, day, usagestats.NonworkRankingScopeAll, usagestats.NonworkRankingRankByTokens, "desc", "Asia/Shanghai", nil, "", 50)
	require.NoError(t, err)
	requireNonworkRankingItem(t, before.Ranking, source.ID, 100)
	require.Equal(t, int64(140), before.TotalTokens)

	_, err = tx.ExecContext(ctx, `INSERT INTO user_usage_migrations (source_user_id, target_user_id) VALUES ($1, $2)`, source.ID, target.ID)
	require.NoError(t, err)

	after, err := repo.GetUserNonworkTokenRanking(ctx, day, day, usagestats.NonworkRankingScopeAll, usagestats.NonworkRankingRankByTokens, "desc", "Asia/Shanghai", nil, "", 50)
	require.NoError(t, err)
	requireNoNonworkRankingItem(t, after.Ranking, source.ID)
	requireNonworkRankingItem(t, after.Ranking, target.ID, 140)
	require.Equal(t, before.TotalTokens, after.TotalTokens)
	require.Equal(t, before.TotalActiveDurationMs, after.TotalActiveDurationMs)
}

func createUsageLogForMigrationTest(t *testing.T, repo *usageLogRepository, user *service.User, key *service.APIKey, account *service.Account, input, output int, cost float64, createdAt time.Time) {
	t.Helper()
	created, err := repo.Create(context.Background(), &service.UsageLog{
		UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
		RequestID: key.Key + "-" + createdAt.Format(time.RFC3339Nano), Model: "migration-test", InputTokens: input, OutputTokens: output,
		TotalCost: cost, ActualCost: cost, CreatedAt: createdAt,
	})
	require.NoError(t, err)
	require.True(t, created)
}

func insertExternalUsageForMigrationTest(t *testing.T, ctx context.Context, tx interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, userID int64, day time.Time, tokens int64) int64 {
	t.Helper()
	rows, err := tx.QueryContext(ctx, `INSERT INTO external_usage_import_batches (status) VALUES ('imported') RETURNING id`)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var batchID int64
	require.NoError(t, rows.Scan(&batchID))
	require.NoError(t, rows.Close())
	_, err = tx.ExecContext(ctx, `
		INSERT INTO external_usage_daily_user_stats
			(batch_id, bucket_date, user_id, total_tokens, requests)
		VALUES ($1, $2, $3, $4, 1)
	`, batchID, day, userID, tokens)
	require.NoError(t, err)
	return batchID
}

func requireRankingItem(t *testing.T, items []usagestats.UserTokenRankingItem, userID, tokens int64) {
	t.Helper()
	for _, item := range items {
		if item.UserID == userID {
			require.Equal(t, tokens, item.Tokens)
			return
		}
	}
	t.Fatalf("ranking item for user %d not found", userID)
}

func requireNoRankingItem(t *testing.T, items []usagestats.UserTokenRankingItem, userID int64) {
	t.Helper()
	for _, item := range items {
		require.NotEqual(t, userID, item.UserID)
	}
}

func requireNonworkRankingItem(t *testing.T, items []usagestats.UserNonworkTokenRankingItem, userID, tokens int64) {
	t.Helper()
	for _, item := range items {
		if item.UserID == userID {
			require.Equal(t, tokens, item.Tokens)
			return
		}
	}
	t.Fatalf("nonwork ranking item for user %d not found", userID)
}

func requireNoNonworkRankingItem(t *testing.T, items []usagestats.UserNonworkTokenRankingItem, userID int64) {
	t.Helper()
	for _, item := range items {
		require.NotEqual(t, userID, item.UserID)
	}
}

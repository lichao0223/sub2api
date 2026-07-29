//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCostIncrementalIsIdempotent(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var originalCheckpoint int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT last_usage_log_id FROM cost_jobs WHERE job_key='incremental'`).Scan(&originalCheckpoint))
	var existingMaxID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM usage_logs`).Scan(&existingMaxID))
	_, err := integrationDB.ExecContext(ctx, `UPDATE cost_jobs SET last_usage_log_id=$1 WHERE job_key='incremental'`, existingMaxID)
	require.NoError(t, err)

	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("cost-%d@test.local", suffix), Username: "cost-test",
	})
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{
		UserID: user.ID, Key: fmt.Sprintf("sk-cost-%d", suffix), Name: "cost-test",
	})
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("cost-%d", suffix)})
	repo := &costManagementRepository{db: integrationDB}
	effective := time.Now().Add(-time.Hour)
	plan, err := repo.CreateCostPlan(ctx, service.CostPlanInput{
		Name: "集成测试按量方案", PlanType: "metered", EffectiveFrom: effective,
		Prices: []service.CostModelPrice{{
			UpstreamModel: "qwen-cost-test", BillingMode: "token",
			InputPriceCNY: "1", OutputPriceCNY: "2",
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_daily_aggregates WHERE plan_id=$1 OR account_id=$2 OR user_id=$3`, plan.ID, account.ID, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_cost_configs WHERE account_id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_model_prices WHERE plan_version_id IN (SELECT id FROM cost_plan_versions WHERE plan_id=$1)`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_plan_versions WHERE plan_id=$1`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_plans WHERE id=$1`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id=$1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1`, key.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id=$1`, account.ID)
		_, _ = integrationDB.ExecContext(ctx, `UPDATE cost_jobs SET last_usage_log_id=$1 WHERE job_key='incremental'`, originalCheckpoint)
	})
	require.NoError(t, repo.SaveAccountCost(ctx, service.AccountCostInput{
		AccountID: account.ID, CostMode: "metered", PlanID: &plan.ID, EffectiveFrom: effective,
	}))

	model := "qwen-cost-test"
	usageRepo := newUsageLogRepositoryWithSQL(integrationEntClient, integrationDB)
	created, err := usageRepo.Create(ctx, &service.UsageLog{
		UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
		RequestID: fmt.Sprintf("cost-request-%d", suffix), Model: "client-model", UpstreamModel: &model,
		InputTokens: 1_000_000, OutputTokens: 1_000_000, CreatedAt: time.Now(),
	})
	require.NoError(t, err)
	require.True(t, created)

	for {
		processed, runErr := repo.RunCostIncremental(ctx, 2000)
		require.NoError(t, runErr)
		if !processed {
			break
		}
	}
	var amount string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_cny),0)::text
		FROM cost_daily_aggregates
		WHERE aggregate_scope='usage' AND user_id=$1 AND account_id=$2 AND upstream_model=$3
	`, user.ID, account.ID, model).Scan(&amount))
	value, err := decimal.NewFromString(amount)
	require.NoError(t, err)
	require.True(t, value.Equal(decimal.NewFromInt(3)))

	processed, err := repo.RunCostIncremental(ctx, 2000)
	require.NoError(t, err)
	require.False(t, processed)
	var after string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_cny),0)::text
		FROM cost_daily_aggregates
		WHERE aggregate_scope='usage' AND user_id=$1 AND account_id=$2 AND upstream_model=$3
	`, user.ID, account.ID, model).Scan(&after))
	require.Equal(t, amount, after)
}

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

func TestFixedCostCountsSharedSubscriptionOnce(t *testing.T) {
	ctx := context.Background()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(loc)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	daysInMonth := monthStart.AddDate(0, 1, -1).Day()
	suffix := time.Now().UnixNano()

	user := mustCreateUser(t, integrationEntClient, &service.User{
		Email: fmt.Sprintf("fixed-cost-%d@test.local", suffix), Username: "fixed-cost-test",
	})
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{
		UserID: user.ID, Key: fmt.Sprintf("sk-fixed-cost-%d", suffix), Name: "fixed-cost-test",
	})
	accountA := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("fixed-a-%d", suffix)})
	accountB := mustCreateAccount(t, integrationEntClient, &service.Account{Name: fmt.Sprintf("fixed-b-%d", suffix)})
	repo := &costManagementRepository{db: integrationDB}
	plan, err := repo.CreateCostPlan(ctx, service.CostPlanInput{
		Name: "共享订阅集成测试", PlanType: "fixed", FixedCategory: "coding_plan",
		EffectiveFrom: monthStart, BillingCycle: "monthly",
		FixedUnitCostCNY: fmt.Sprintf("%d", daysInMonth*10),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_daily_aggregates WHERE plan_id=$1 OR account_id IN($2,$3) OR user_id=$4`, plan.ID, accountA.ID, accountB.ID, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM usage_logs WHERE user_id=$1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM account_cost_configs WHERE account_id IN($1,$2)`, accountA.ID, accountB.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_subscription_unit_versions WHERE subscription_unit_id IN(SELECT id FROM cost_subscription_units WHERE plan_id=$1)`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_subscription_units WHERE plan_id=$1`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_plan_versions WHERE plan_id=$1`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM cost_plans WHERE id=$1`, plan.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM api_keys WHERE id=$1`, key.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, user.ID)
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE id IN($1,$2)`, accountA.ID, accountB.ID)
	})
	require.NoError(t, repo.SaveAccountCost(ctx, service.AccountCostInput{
		AccountID: accountA.ID, CostMode: "fixed", PlanID: &plan.ID,
		NewSubscriptionUnitName: "订阅 #1", EffectiveFrom: monthStart,
	}))
	var unitID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM cost_subscription_units WHERE plan_id=$1`, plan.ID).Scan(&unitID))
	require.NoError(t, repo.SaveAccountCost(ctx, service.AccountCostInput{
		AccountID: accountB.ID, CostMode: "fixed", PlanID: &plan.ID,
		SubscriptionUnitID: &unitID, EffectiveFrom: monthStart,
	}))

	usageRepo := newUsageLogRepositoryWithSQL(integrationEntClient, integrationDB)
	for i, accountID := range []int64{accountA.ID, accountB.ID} {
		created, createErr := usageRepo.Create(ctx, &service.UsageLog{
			UserID: user.ID, APIKeyID: key.ID, AccountID: accountID,
			RequestID: fmt.Sprintf("fixed-cost-request-%d-%d", suffix, i), Model: "fixed-cost-model",
			InputTokens: 1, OutputTokens: 1, CreatedAt: now,
		})
		require.NoError(t, createErr)
		require.True(t, created)
	}
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.rebuildFixedDay(ctx, tx, now))
	require.NoError(t, tx.Commit())

	var amount string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT amount_cny::text FROM cost_daily_aggregates
		WHERE bucket_date=$1::date AND aggregate_scope='fixed_plan_total' AND plan_id=$2
	`, now, plan.ID).Scan(&amount))
	value, err := decimal.NewFromString(amount)
	require.NoError(t, err)
	require.True(t, value.Equal(decimal.NewFromInt(10)), "shared subscription charged more than once: %s", amount)

	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at=NOW() WHERE id IN($1,$2)`, accountA.ID, accountB.ID)
	require.NoError(t, err)
	tx, err = integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.rebuildFixedDay(ctx, tx, now))
	require.NoError(t, tx.Commit())
	var afterDelete string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT amount_cny::text FROM cost_daily_aggregates
		WHERE bucket_date=$1::date AND aggregate_scope='fixed_plan_total' AND subscription_unit_id=$2
	`, now, unitID).Scan(&afterDelete))
	afterDeleteValue, err := decimal.NewFromString(afterDelete)
	require.NoError(t, err)
	require.True(t, afterDeleteValue.Equal(decimal.NewFromInt(10)), "soft-deleted account changed historical cost: %s", afterDelete)
}

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFixedBillingPeriodPreservesOriginalAnchor(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	anchor := time.Date(2026, time.January, 31, 0, 0, 0, 0, loc)

	start, end := fixedBillingPeriod(time.Date(2026, time.February, 15, 0, 0, 0, 0, loc), anchor)
	require.Equal(t, "2026-01-31", start.Format("2006-01-02"))
	require.Equal(t, "2026-02-28", end.Format("2006-01-02"))

	start, end = fixedBillingPeriod(time.Date(2026, time.March, 15, 0, 0, 0, 0, loc), anchor)
	require.Equal(t, "2026-02-28", start.Format("2006-01-02"))
	require.Equal(t, "2026-03-31", end.Format("2006-01-02"))
}

func TestFixedBillingPeriodUsesCostTimezoneForDatabaseTimestamps(t *testing.T) {
	day := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	anchor := time.Date(2026, time.June, 30, 16, 0, 0, 0, time.UTC)
	start, end := fixedBillingPeriod(day, anchor)
	require.Equal(t, "2026-07-01", start.Format("2006-01-02"))
	require.Equal(t, "2026-08-01", end.Format("2006-01-02"))
}

func TestCostAnalysisRangeUsesExactBucketCounts(t *testing.T) {
	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	start, grain := costAnalysisRange("month", now)
	require.Equal(t, "2025-08-01", start.Format("2006-01-02"))
	require.Equal(t, "month", grain)
	start, grain = costAnalysisRange("year", now)
	require.Equal(t, "2022-01-01", start.Format("2006-01-02"))
	require.Equal(t, "year", grain)
}

func TestPreviousCostRangePreservesWholeMonths(t *testing.T) {
	start := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	previousStart, previousEnd := previousCostRange(start, end)
	require.Equal(t, "2026-01-01", previousStart.Format("2006-01-02"))
	require.Equal(t, "2026-02-01", previousEnd.Format("2006-01-02"))
}

func TestNormalizeFixedCostConvertsYearlyToMonthly(t *testing.T) {
	cycle, unit, monthly, err := normalizeFixedCost("yearly", "1200", "")
	require.NoError(t, err)
	require.Equal(t, "yearly", cycle)
	require.Equal(t, "1200", unit)
	require.Equal(t, "100", monthly)
}

func TestNormalizeCostAmountRemovesDatabaseScale(t *testing.T) {
	require.Equal(t, "4500", normalizeCostAmount("4500.000000000000"))
	require.Equal(t, "1.25", normalizeCostAmount("1.250000000000"))
}

func TestUpdateCostPlanOnlyChangesBasicInformation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	effective := time.Date(2026, 1, 1, 10, 12, 0, 0, time.UTC)
	mock.ExpectExec("UPDATE cost_plans").WithArgs(int64(3), "GLM MAX 订阅", "coding_plan", "备注").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT p.name,p.plan_type").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "plan_type", "fixed_category", "status", "note", "version_id", "version_no",
			"effective_from", "effective_to", "billing_cycle", "fixed_unit_cost_cny",
			"monthly_unit_cost_cny", "subscription_unit_count", "account_count",
		}).AddRow("GLM MAX 订阅", "fixed", "coding_plan", "active", "备注", 21, 1, effective, nil, "yearly", "4500.000000000000", "375.000000000000", 0, 0))
	mock.ExpectQuery("SELECT upstream_model").
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{
			"upstream_model", "billing_mode", "input_price_cny", "output_price_cny",
			"cache_write_price_cny", "cache_read_price_cny", "image_input_price_cny",
			"image_output_price_cny", "per_request_price_cny",
		}))

	plan, err := (&costManagementRepository{db: db}).UpdateCostPlan(context.Background(), 3, service.CostPlanBasicInput{
		Name: "GLM MAX 订阅", FixedCategory: "coding_plan", Note: "备注",
	})
	require.NoError(t, err)
	require.Equal(t, effective, plan.EffectiveFrom)
	require.Equal(t, "4500", plan.FixedUnitCostCNY)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAppendSubscriptionUnitPriceCreatesVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	current := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	next := current.AddDate(0, 1, 0)
	mock.ExpectQuery("SELECT effective_from,effective_to FROM cost_subscription_units").WithArgs(int64(9), int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"effective_from", "effective_to"}).AddRow(current, nil))
	mock.ExpectQuery("SELECT id,version_no,effective_from FROM cost_subscription_unit_versions").WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version_no", "effective_from"}).AddRow(17, 1, current))
	mock.ExpectExec("UPDATE cost_subscription_unit_versions").WithArgs(int64(17), next).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO cost_subscription_unit_versions").WithArgs(int64(9), 2, next, "monthly", "160", "160").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, appendSubscriptionUnitVersionTx(context.Background(), tx, 3, 9, next, "monthly", "160"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMeteredPriceChangeCreatesNewPlanVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	current := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	next := current.AddDate(0, 1, 0)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type,status FROM cost_plans").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "status"}).AddRow("metered", "active"))
	mock.ExpectQuery("SELECT id,version_no,effective_from FROM cost_plan_versions").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version_no", "effective_from"}).AddRow(17, 1, current))
	mock.ExpectExec("UPDATE cost_plan_versions SET effective_to").WithArgs(int64(17), next).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO cost_plan_versions").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(18))
	mock.ExpectExec("INSERT INTO cost_model_prices").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO cost_jobs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = (&costManagementRepository{db: db}).ChangeCostPlanPrice(context.Background(), 3, service.CostPriceChangeInput{
		EffectiveFrom: next,
		Prices: []service.CostModelPrice{{
			UpstreamModel: "qwen3", BillingMode: "token", InputPriceCNY: "1", OutputPriceCNY: "2",
		}},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFixedDefaultPriceChangeDoesNotQueueRecalculation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	current := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	next := current.AddDate(0, 1, 0)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type,status FROM cost_plans").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "status"}).AddRow("fixed", "active"))
	mock.ExpectQuery("SELECT id,version_no,effective_from FROM cost_plan_versions").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version_no", "effective_from"}).AddRow(17, 1, current))
	mock.ExpectExec("UPDATE cost_plan_versions SET effective_to").WithArgs(int64(17), next).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("INSERT INTO cost_plan_versions").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(18))
	mock.ExpectCommit()

	err = (&costManagementRepository{db: db}).ChangeCostPlanPrice(context.Background(), 3, service.CostPriceChangeInput{
		EffectiveFrom: next, UpdateDefault: true, BillingCycle: "monthly", FixedUnitCostCNY: "160",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertFixedCostPlanIgnoresModelPrices(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	mock.ExpectQuery("INSERT INTO cost_plan_versions").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	err = (&costManagementRepository{db: db}).insertCostPlanVersion(
		context.Background(), tx, 1, 1,
		service.CostPlanInput{
			PlanType:         "fixed",
			BillingCycle:     "yearly",
			FixedUnitCostCNY: "4500",
			Prices:           []service.CostModelPrice{{}},
		},
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareSubscriptionUnitCreatesOnePurchasedInstance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	planID := int64(8)
	effective := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("INSERT INTO cost_subscription_units").
		WithArgs(planID, "ChatGPT Plus #3", effective).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(29))
	mock.ExpectQuery("SELECT billing_cycle,fixed_unit_cost_cny").WithArgs(planID, effective).
		WillReturnRows(sqlmock.NewRows([]string{"billing_cycle", "fixed_unit_cost_cny"}).AddRow("monthly", "140"))
	mock.ExpectExec("INSERT INTO cost_subscription_unit_versions").WithArgs(int64(29), 1, effective, "monthly", "140", "140").WillReturnResult(sqlmock.NewResult(1, 1))

	input := service.AccountCostInput{
		CostMode: "fixed", PlanID: &planID, EffectiveFrom: effective,
		NewSubscriptionUnitName: " ChatGPT Plus #3 ",
	}
	require.NoError(t, prepareSubscriptionUnitTx(context.Background(), tx, &input))
	require.Equal(t, int64(29), *input.SubscriptionUnitID)
	require.Empty(t, input.NewSubscriptionUnitName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEndCostSubscriptionUnitEndsCurrentAccountBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT effective_from,effective_to").
		WithArgs(int64(31)).
		WillReturnRows(sqlmock.NewRows([]string{"effective_from", "effective_to"}).AddRow(start, nil))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(int64(31), end).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("UPDATE cost_subscription_units").
		WithArgs(int64(31), end).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE cost_subscription_unit_versions").
		WithArgs(int64(31), end).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE account_cost_configs").
		WithArgs(int64(31), end).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO cost_jobs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, (&costManagementRepository{db: db}).EndCostSubscriptionUnit(context.Background(), 31, end))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveAccountCostCorrectsOnlyExistingConfigurationStartTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	accountID, planID, unitID := int64(7), int64(11), int64(31)
	original := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	corrected := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT id,effective_from").WithArgs(accountID, corrected).
		WillReturnRows(sqlmock.NewRows([]string{"id", "effective_from", "count", "cost_mode", "plan_id", "subscription_unit_id", "effective_to"}).
			AddRow(41, original, 1, "fixed", planID, unitID, nil))
	mock.ExpectQuery("SELECT plan_type,status").WithArgs(planID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "status"}).AddRow("fixed", "active"))
	mock.ExpectQuery("SELECT plan_id,effective_from,effective_to").WithArgs(unitID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "effective_from", "effective_to"}).AddRow(planID, corrected, nil))
	mock.ExpectExec("UPDATE account_cost_configs SET cost_mode").
		WithArgs(int64(41), "fixed", &planID, &unitID, corrected, nil, "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := saveAccountCostTx(context.Background(), tx, service.AccountCostInput{
		AccountID: accountID, CostMode: "fixed", PlanID: &planID, SubscriptionUnitID: &unitID, EffectiveFrom: corrected,
	})
	require.NoError(t, err)
	require.True(t, changed)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveAccountCostSkipsRecalculationWhenCostConfigIsUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	accountID, planID, unitID := int64(7), int64(11), int64(31)
	effective := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	in := service.AccountCostInput{
		AccountID: accountID, CostMode: "fixed", PlanID: &planID, SubscriptionUnitID: &unitID, EffectiveFrom: effective,
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id,effective_from").WithArgs(accountID, effective).
		WillReturnRows(sqlmock.NewRows([]string{"id", "effective_from", "count", "cost_mode", "plan_id", "subscription_unit_id", "effective_to"}).
			AddRow(41, effective, 1, "fixed", planID, unitID, nil))
	mock.ExpectQuery("SELECT plan_type,status").WithArgs(planID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type", "status"}).AddRow("fixed", "active"))
	mock.ExpectQuery("SELECT plan_id,effective_from,effective_to").WithArgs(unitID).
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "effective_from", "effective_to"}).AddRow(planID, effective, nil))
	mock.ExpectExec("UPDATE account_cost_configs SET cost_mode").
		WithArgs(int64(41), "fixed", &planID, &unitID, effective, nil, "", "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, (&costManagementRepository{db: db}).SaveAccountCost(context.Background(), in))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListAccountCostsUsesTheOverviewDateRangeForPendingCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM accounts").WithArgs("", "").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("d.bucket_date>=\\$3::date AND d.bucket_date<\\$4::date").
		WithArgs("", "", start, end, 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "platform", "status", "cost_mode", "plan_id", "plan_name", "subscription_unit_id", "subscription_unit_name", "effective_from", "effective_to", "exclude_reason", "pending_count"}).
			AddRow(7, "GPT", "openai", "active", "fixed", 11, "GPT 订阅", 31, "订阅 #1", start, nil, "", 9))

	items, total, err := (&costManagementRepository{db: db}).ListAccountCosts(context.Background(), 1, 20, "", "", start, end)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, int64(9), items[0].PendingCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCostOverviewPendingCountExcludesDeletedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SUM\\(pending_count\\) FILTER\\(WHERE calculation_status='pending' AND EXISTS\\(SELECT 1 FROM accounts a WHERE a.id=cost_daily_aggregates.account_id AND a.deleted_at IS NULL\\)\\)").
		WillReturnRows(sqlmock.NewRows([]string{"dynamic", "fixed", "total", "pending", "errors", "eligible", "calculated"}).AddRow("0", "0", "0", 1445, 0, 3651, 0))
	mock.ExpectQuery("SELECT last_success_at FROM cost_jobs").WillReturnRows(sqlmock.NewRows([]string{"last_success_at"}).AddRow(nil))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(amount_cny\\)").WillReturnRows(sqlmock.NewRows([]string{"previous_total"}).AddRow("0"))
	mock.ExpectQuery("SELECT MIN\\(ul.created_at\\),MAX\\(ul.created_at\\)").WillReturnRows(sqlmock.NewRows([]string{"min", "max", "exists"}).AddRow(nil, nil, false))

	overview, err := (&costManagementRepository{db: db}).GetCostOverview(context.Background(), time.Now().AddDate(0, 0, -1), time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1445), overview.PendingCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCostBreakdownMatchesSelectedScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("CASE d.aggregate_scope WHEN 'usage' THEN 'metered' ELSE 'fixed' END").
		WithArgs(start, end, "fixed").
		WillReturnRows(sqlmock.NewRows([]string{"mode", "plan", "account", "unit", "model", "billing_mode", "input_price", "output_price", "cache_write_price", "cache_read_price", "request_price", "billing_cycle", "fixed_cost", "monthly_cost", "requests", "input", "output", "cache_write", "cache_read", "amount", "total"}).
			AddRow("fixed", "Kimi 套餐 199", "", "Kimi 199", "", "", "0", "0", "0", "0", "0", "monthly", "375", "375", 0, 0, 0, 0, 0, "375", "375"))

	result, err := (&costManagementRepository{db: db}).GetCostBreakdown(context.Background(), start, end, "fixed")
	require.NoError(t, err)
	require.Equal(t, "375", result.TotalCostCNY)
	require.Equal(t, "Kimi 199", result.Items[0].SubscriptionUnitName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPendingCostDetailsExposeReasonAndModel(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery("SUM\\(SUM\\(d.pending_count\\)\\) OVER").
		WithArgs(start, end, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "account", "start", "end", "issue", "model", "pending", "total"}).
			AddRow(7, "DeepSeek", start, start.AddDate(0, 0, 2), "missing_model_price", "deepseek-v4-pro", 12, 12))

	result, err := (&costManagementRepository{db: db}).GetPendingCostDetails(context.Background(), start, end, 7)
	require.NoError(t, err)
	require.Equal(t, int64(12), result.TotalCount)
	require.Equal(t, "missing_model_price", result.Items[0].IssueCode)
	require.Equal(t, "deepseek-v4-pro", result.Items[0].UpstreamModel)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateCostRecalculationRejectsOverlappingActiveJob(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err = (&costManagementRepository{db: db}).CreateCostRecalculation(
		context.Background(),
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
		1,
	)
	require.EqualError(t, err, "所选日期范围已有补算任务排队或运行中")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCancelCostRecalculation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectExec("UPDATE cost_jobs").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, (&costManagementRepository{db: db}).CancelCostRecalculation(context.Background(), 7))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAutomaticCostRecalculationUsesTheSharedJobLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO cost_jobs").WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, enqueueCostRecalculationTx(context.Background(), tx, time.Now().AddDate(0, -1, 0)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecalculationTimeoutIsRetried(t *testing.T) {
	status, retry := recalculationFailureStatus(context.DeadlineExceeded)
	require.Equal(t, "queued", status)
	require.True(t, retry)

	status, retry = recalculationFailureStatus(errors.New("invalid cost data"))
	require.Equal(t, "failed", status)
	require.False(t, retry)
}

func TestCostAnalysisEmptyCollectionsMarshalAsArrays(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT TO_CHAR").
		WillReturnRows(sqlmock.NewRows([]string{"bucket", "dynamic", "fixed", "total"}))
	mock.ExpectQuery("SELECT p.id").
		WillReturnRows(sqlmock.NewRows([]string{"plan_id", "plan_name", "amount", "total"}))

	analysis, err := (&costManagementRepository{db: db}).GetCostAnalysis(context.Background(), "day", time.Now())
	require.NoError(t, err)
	body, err := json.Marshal(analysis)
	require.NoError(t, err)
	require.JSONEq(t, `{"period":"day","total_cost_cny":"0","trend":[],"top":[]}`, string(body))
	require.NoError(t, mock.ExpectationsWereMet())
}

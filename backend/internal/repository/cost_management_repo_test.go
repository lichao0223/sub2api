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

func TestUpdateCostPlanRevisesTheOnlyVersionInPlace(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	original := time.Date(2026, 7, 29, 10, 12, 0, 0, time.UTC)
	revised := time.Date(2026, 1, 1, 10, 12, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT plan_type").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"plan_type"}).AddRow("fixed"))
	mock.ExpectQuery("SELECT id,version_no,effective_from").WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version_no", "effective_from"}).AddRow(21, 1, original))
	mock.ExpectExec("UPDATE cost_plans").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE cost_plan_versions SET effective_from").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO cost_jobs").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT p.name,p.plan_type").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{
			"name", "plan_type", "fixed_category", "status", "note", "version_id", "version_no",
			"effective_from", "effective_to", "billing_cycle", "fixed_unit_cost_cny",
			"monthly_unit_cost_cny", "purchase_quantity", "subscription_unit_count",
			"unassigned_account_count", "account_count",
		}).AddRow("GLM MAX 订阅", "fixed", "coding_plan", "active", "", 21, 1, revised, nil, "yearly", "4500.000000000000", "375.000000000000", 1, 0, 0, 0))
	mock.ExpectQuery("SELECT upstream_model").
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{
			"upstream_model", "billing_mode", "input_price_cny", "output_price_cny",
			"cache_write_price_cny", "cache_read_price_cny", "image_input_price_cny",
			"image_output_price_cny", "per_request_price_cny",
		}))

	plan, err := (&costManagementRepository{db: db}).UpdateCostPlan(context.Background(), 3, service.CostPlanInput{
		Name: "GLM MAX 订阅", PlanType: "fixed", FixedCategory: "coding_plan",
		EffectiveFrom: revised, BillingCycle: "yearly", FixedUnitCostCNY: "4500", PurchaseQuantity: 1,
	})
	require.NoError(t, err)
	require.Equal(t, revised, plan.EffectiveFrom)
	require.Equal(t, "4500", plan.FixedUnitCostCNY)
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
			PurchaseQuantity: 1,
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

	input := service.AccountCostInput{
		CostMode: "fixed", PlanID: &planID, EffectiveFrom: effective,
		NewSubscriptionUnitName: " ChatGPT Plus #3 ",
	}
	require.NoError(t, prepareSubscriptionUnitTx(context.Background(), tx, &input))
	require.Equal(t, int64(29), *input.SubscriptionUnitID)
	require.Empty(t, input.NewSubscriptionUnitName)
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

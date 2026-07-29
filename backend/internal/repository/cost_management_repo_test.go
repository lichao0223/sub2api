package repository

import (
	"context"
	"encoding/json"
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

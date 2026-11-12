package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsTrendPointCombinesModelsAndModesExactly(t *testing.T) {
	point, err := analyticsTrendPoint("2026-01-01", "9007199254741110", []service.AnalyticsTrendModel{
		{Model: "shared", Mode: "metered", Tokens: "9007199254740993"},
		{Model: "shared", Mode: "metered", Tokens: "7"},
		{Model: "shared", Mode: "fixed", Tokens: "100"},
		{Model: "free", Mode: "excluded", Tokens: "10"},
	})
	require.NoError(t, err)
	require.Equal(t, "9007199254741110", point.Tokens)
	require.Equal(t, "9007199254741000", point.MeteredTokens)
	require.Equal(t, "100", point.FixedTokens)
	require.Equal(t, "10", point.ExcludedTokens)
	require.Equal(t, []service.AnalyticsTrendModel{
		{Model: "free", Mode: "excluded", Tokens: "10"},
		{Model: "shared", Mode: "fixed", Tokens: "100"},
		{Model: "shared", Mode: "metered", Tokens: "9007199254741000"},
	}, point.Models)
	legacy, err := analyticsTrendPoint("2026-01-01", "100", nil)
	require.NoError(t, err)
	require.Empty(t, legacy.MeteredTokens)
	_, err = analyticsTrendPoint("2026-01-01", "100", []service.AnalyticsTrendModel{{Tokens: "invalid"}})
	require.Error(t, err)
}

func TestAnalyticsReportTrendUsesPinnedVersionsAndFillsEmptyBuckets(t *testing.T) {
	for _, grain := range []string{"day", "month"} {
		t.Run(grain, func(t *testing.T) {
			db, mock := newSQLMock(t)
			mock.ExpectBegin()
			tx, err := db.Begin()
			require.NoError(t, err)
			query := service.AnalyticsQuery{OrgID: "dept", Period: service.AnalyticsPeriod{
				Year: 2025, Months: []int{1, 3}, Keys: []string{"2025-01-01", "2025-03-01"}, Grain: grain,
			}}
			report := &service.AnalyticsReport{Versions: map[string]int64{"2025-01-01": 8, "2025-03-01": 9}}
			rows := sqlmock.NewRows([]string{"bucket", "tokens", "models"}).
				AddRow("2025-01-01", "1000", `[{"model":"model","mode":"fixed","tokens":"1000"}]`)
			if grain == "day" {
				query.Period.Months, query.Period.Keys = []int{1}, []string{"2025-01-01"}
				rows.AddRow("2025-01-02", "50", nil)
				mock.ExpectQuery(`(?s)SELECT day::text,SUM\(tokens\).*FROM analytics_org_daily_versions.*generation=\$2 AND org_id=\$3`).
					WithArgs("2025-01-01", int64(8), "dept").WillReturnRows(rows)
			} else {
				mock.ExpectQuery(`(?s)SELECT bucket.*FROM analytics_monthly.*m.generation=\(\$2::jsonb`).
					WithArgs(sqlmock.AnyArg(), service.AnalyticsJSON(report.Versions), "dept").WillReturnRows(rows)
			}
			require.NoError(t, analyticsReportTrend(context.Background(), tx, query, report))
			require.Equal(t, "1000", report.Trend[0].FixedTokens)
			require.Equal(t, "0", report.Trend[0].MeteredTokens)
			if grain == "day" {
				require.Len(t, report.Trend, 31)
				require.Equal(t, "50", report.Trend[1].Tokens)
				require.Empty(t, report.Trend[1].FixedTokens)
				require.Equal(t, "0", report.Trend[2].FixedTokens)
			} else {
				require.Len(t, report.Trend, 2)
				require.Equal(t, "2025-03", report.Trend[1].Bucket)
				require.Equal(t, "0", report.Trend[1].Tokens)
			}
			mock.ExpectRollback()
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

//go:build integration

package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsTrendSnapshotsPreserveModelsScopesAndVersions(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	_, err := tx.ExecContext(ctx, `INSERT INTO analytics_assignments(id,user_id,person_id,name,org_ids,org_names,basis,effective_from)
		VALUES(1,1,'one','One','{company,dept-a}','{Company,A}','manual','2025-01-01'),
		      (2,2,'two','Two','{other,dept-b}','{Other,B}','manual','2025-01-01');
		INSERT INTO analytics_daily
		SELECT '2025-01-01',uid,attribution,model,mode,1,'Plan','',tokens,0,0,0,1,1,0,0,0,0,0,0
		FROM (VALUES(1,1,'shared','metered',1000),(1,1,'shared','fixed',2000),
			(2,2,'shared','metered',3000),(2,2,'free','excluded',4000),(3,0,'free','excluded',5000))
			v(uid,attribution,model,mode,tokens);
		INSERT INTO analytics_org_daily(day,org_id,level,tokens)
		SELECT d.day,COALESCE(a.org_ids[x.n],'0'),x.n,SUM(d.input+d.output+d.cache_write+d.cache_read)
		FROM analytics_daily d LEFT JOIN analytics_assignments a ON a.id=d.attribution
		CROSS JOIN generate_series(1,3) x(n)
		WHERE a.org_ids[x.n] IS NOT NULL OR x.n=1 GROUP BY d.day,COALESCE(a.org_ids[x.n],'0'),x.n;`)
	require.NoError(t, err)
	month := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, (&dataAnalyticsRepository{}).publishAnalyticsMonth(ctx, tx, month, month.AddDate(0, 1, 0), false))
	var generation int64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT MAX(generation) FROM analytics_publications`).Scan(&generation))
	for _, scope := range []struct{ org, total, metered, fixed, excluded string }{
		{"", "15000", "4000", "2000", "9000"},
		{"dept-a", "3000", "1000", "2000", "0"},
		{"0", "5000", "0", "0", "5000"},
	} {
		for _, grain := range []string{"day", "month"} {
			query := service.AnalyticsQuery{OrgID: scope.org, Period: service.AnalyticsPeriod{
				Year: 2025, Months: []int{1}, Keys: []string{"2025-01-01"}, Grain: grain,
			}}
			report := &service.AnalyticsReport{Versions: map[string]int64{"2025-01-01": generation}}
			require.NoError(t, analyticsReportTrend(ctx, tx, query, report))
			require.Equal(t, scope.total, report.Trend[0].Tokens, "%s/%s", scope.org, grain)
			require.Equal(t, scope.metered, report.Trend[0].MeteredTokens)
			require.Equal(t, scope.fixed, report.Trend[0].FixedTokens)
			require.Equal(t, scope.excluded, report.Trend[0].ExcludedTokens)
			if scope.org == "" {
				require.Len(t, report.Trend[0].Models, 3, "same model and mode merge across top-level organizations")
			}
		}
	}
	// Identity-only republication after daily retention must copy the pinned snapshot.
	_, err = tx.ExecContext(ctx, `UPDATE analytics_daily SET input=999999;
		UPDATE analytics_org_daily SET tokens=999999,model_tokens=NULL`)
	require.NoError(t, err)
	require.NoError(t, publishAnalyticsTrend(ctx, tx, "2025-01-01", "2025-02-01", generation+1, generation))
	var original, copied string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT model_tokens::text FROM analytics_org_daily_versions
		WHERE generation=$1 AND org_id='dept-a'`, generation).Scan(&original))
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT model_tokens::text FROM analytics_org_daily_versions
		WHERE generation=$1 AND org_id='dept-a'`, generation+1).Scan(&copied))
	require.Equal(t, original, copied)
}

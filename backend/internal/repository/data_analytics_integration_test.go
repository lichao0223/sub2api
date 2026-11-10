//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// An isolated rollback-only schema exercises migration functions on ordinary
// and partitioned logs without touching shared fixtures or real directories.
func analyticsMigrationFixture(t *testing.T, partitioned bool) (*sql.Tx, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	schema := fmt.Sprintf("analytics_fixture_%d", time.Now().UnixNano())
	_, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema)
	require.NoError(t, err)
	seedAnalyticsFixture(t, ctx, tx, partitioned)
	return tx, ctx
}

func seedAnalyticsFixture(t *testing.T, ctx context.Context, tx *sql.Tx, partitioned bool) {
	t.Helper()
	suffix := ""
	if partitioned {
		suffix = " PARTITION BY RANGE(created_at)"
	}
	_, err := tx.ExecContext(ctx, `CREATE TABLE usage_logs(
      id bigint,user_id bigint,account_id bigint,model text,upstream_model text,created_at timestamptz,
      input_tokens integer DEFAULT 0,output_tokens integer DEFAULT 0,cache_creation_tokens integer DEFAULT 0,cache_read_tokens integer DEFAULT 0,
      image_input_tokens integer DEFAULT 0,image_output_tokens integer DEFAULT 0,image_count integer DEFAULT 0,video_count integer DEFAULT 0,
      billing_mode text,actual_cost numeric DEFAULT 0)`+suffix)
	require.NoError(t, err)
	if partitioned {
		_, err = tx.ExecContext(ctx, `CREATE TABLE usage_logs_all PARTITION OF usage_logs FOR VALUES FROM(MINVALUE) TO(MAXVALUE)`)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `
      CREATE TABLE user_usage_migrations(source_user_id bigint,target_user_id bigint);
      CREATE TABLE account_cost_configs(id bigint,account_id bigint,cost_mode text,plan_id bigint,subscription_unit_id bigint,effective_from timestamptz,effective_to timestamptz);
      CREATE TABLE cost_plans(id bigint,name text);
      CREATE TABLE cost_plan_versions(id bigint,plan_id bigint,version_no integer,effective_from timestamptz,effective_to timestamptz);
      CREATE TABLE cost_subscription_units(id bigint,plan_id bigint,name text,effective_from timestamptz,effective_to timestamptz);
      CREATE TABLE cost_subscription_unit_versions(id bigint,subscription_unit_id bigint,monthly_unit_cost_cny numeric,effective_from timestamptz,effective_to timestamptz);
      CREATE TABLE cost_model_prices(id bigint,plan_version_id bigint,upstream_model text,billing_mode text,
        input_price_cny numeric DEFAULT 0,output_price_cny numeric DEFAULT 0,cache_write_price_cny numeric DEFAULT 0,cache_read_price_cny numeric DEFAULT 0,
        image_input_price_cny numeric DEFAULT 0,image_output_price_cny numeric DEFAULT 0,per_request_price_cny numeric DEFAULT 0,time_pricing jsonb);
    `)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("922_data_analytics.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT,updated_at TIMESTAMPTZ);
		CREATE TABLE cost_jobs(job_key TEXT,last_success_at TIMESTAMPTZ)`)
	require.NoError(t, err)
	for _, name := range []string{"921_data_analytics_task_control.sql", "923_data_analytics_progress.sql", "924_data_analytics_stage.sql"} {
		migration, err = migrations.FS.ReadFile(name)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
}

func TestAnalyticsHistoryPublishesWhileTodayInsertIsUncommitted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	schema := fmt.Sprintf("analytics_concurrent_%d", time.Now().UnixNano())
	_, err := integrationDB.ExecContext(ctx, `CREATE SCHEMA `+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	dsn, err := url.Parse(integrationPostgresDSN)
	require.NoError(t, err)
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	db, err := sql.Open("postgres", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	seedAnalyticsFixture(t, ctx, tx, false)
	_, err = tx.ExecContext(ctx, `UPDATE settings SET value='{"enabled":true,"version":1}';
		UPDATE analytics_runtime_state SET seeded=TRUE,directory_synced_at=NOW();
		INSERT INTO analytics_directory_versions(digest,data,effective_at,actor_id) VALUES('test','{}',NOW(),0);
		INSERT INTO analytics_dirty_days(day) SELECT (NOW() AT TIME ZONE 'Asia/Shanghai')::date-i FROM generate_series(1,2) i`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	writer, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Rollback() })
	_, err = writer.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day) VALUES((NOW() AT TIME ZONE 'Asia/Shanghai')::date)`)
	require.NoError(t, err)

	// Hold publication to check the stage through a separate, real database connection.
	blocker, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback() })
	_, err = blocker.ExecContext(ctx, `LOCK TABLE analytics_monthly IN ACCESS EXCLUSIVE MODE`)
	require.NoError(t, err)
	tasks := service.NewAnalyticsTaskService(NewAnalyticsTaskRepository(db), "concurrent-test")
	repo := &dataAnalyticsRepository{db: db}
	finished := make(chan error, 1)
	go func() {
		finished <- tasks.Run(ctx, func(ctx context.Context) error {
			more, err := repo.RunAnalyticsBatch(ctx)
			if err == nil && !more {
				return fmt.Errorf("historical batch did not run")
			}
			return err
		})
	}()
	var live *service.AnalyticsTaskStatus
	require.Eventually(t, func() bool {
		live, err = tasks.Status(ctx)
		return err == nil && live.AnalyticsStage == "publish"
	}, 5*time.Second, 20*time.Millisecond, "history must reach publication without waiting for today's insert")
	require.NotEmpty(t, live.AnalyticsDay)
	require.NotNil(t, live.AnalyticsStartedAt)
	require.NoError(t, blocker.Rollback())
	require.NoError(t, <-finished)
	progress, err := tasks.Status(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, progress.Progress.CompletedDays)
	require.EqualValues(t, 1, progress.Progress.BacklogDays)
	require.Empty(t, progress.AnalyticsStage)
	require.Empty(t, progress.AnalyticsDay)
	// A second historical day also publishes while today's transaction remains open.
	more, err := repo.RunAnalyticsBatch(ctx)
	require.NoError(t, err)
	require.True(t, more)
	progress, err = tasks.Status(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, progress.Progress.CompletedDays)
	require.Zero(t, progress.Progress.BacklogDays)
	require.NoError(t, writer.Rollback())
	// An idle day still accrues fixed purchases without any requests.
	more, err = repo.RunAnalyticsBatch(ctx)
	require.NoError(t, err)
	require.True(t, more)
	more, err = repo.RunAnalyticsBatch(ctx)
	require.NoError(t, err)
	require.False(t, more, "today must retain its five-minute refresh cooldown")
}

func TestAnalyticsOverallProgressCountsDirtyPublishedDaysOnce(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	_, err := tx.ExecContext(ctx, `
      UPDATE analytics_runtime_state SET seeded=TRUE;
      INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count)
        VALUES('2026-11-01',NOW(),1,10),('2026-11-02',NOW(),1,20);
      INSERT INTO analytics_dirty_days(day,attempts,last_error,retry_after)
        VALUES('2026-11-02',1,'timeout',NOW()+INTERVAL '5 minutes'),('2026-11-03',0,'',NULL);`)
	require.NoError(t, err)
	progress := &service.AnalyticsProgress{}
	require.NoError(t, tx.QueryRowContext(ctx, analyticsProgressQuery).Scan(
		&progress.DirectoryReady, &progress.Seeded, &progress.TotalDays, &progress.CompletedDays,
		&progress.BacklogDays, &progress.FailedDays, &progress.PendingRequests, &progress.RetryAt,
		&progress.LastSuccessAt, &progress.LastError))
	require.True(t, progress.Seeded)
	require.EqualValues(t, 3, progress.TotalDays)
	require.EqualValues(t, 1, progress.CompletedDays)
	require.EqualValues(t, 2, progress.BacklogDays)
	require.EqualValues(t, 1, progress.FailedDays)
	require.EqualValues(t, 30, progress.PendingRequests)
	require.NotNil(t, progress.RetryAt)
}

func TestAnalyticsNextDayLetsHistoryAdvanceDuringLiveTraffic(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	_, err := tx.ExecContext(ctx, `
	  INSERT INTO analytics_dirty_days(day) VALUES('2026-10-31'),('2026-11-09');
	  INSERT INTO analytics_dirty_days(day,attempts,retry_after) VALUES('2026-11-01',1,NOW()+INTERVAL '1 hour');
	  INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count)
	    VALUES('2026-11-09',NOW()-INTERVAL '6 minutes',10,0);`)
	require.NoError(t, err)
	next := func() (string, error) {
		var day time.Time
		var version int64
		var reason string
		var changeFrom sql.NullTime
		err := tx.QueryRowContext(ctx, analyticsNextDayQuery, "2026-11-09").Scan(&day, &version, &reason, &changeFrom)
		return day.Format("2006-01-02"), err
	}
	day, err := next()
	require.NoError(t, err)
	require.Equal(t, "2026-10-31", day)
	_, err = tx.ExecContext(ctx, `DELETE FROM analytics_dirty_days WHERE day='2026-10-31'`)
	require.NoError(t, err)
	day, err = next()
	require.NoError(t, err)
	require.Equal(t, "2026-11-09", day)
	_, err = tx.ExecContext(ctx, `UPDATE analytics_bucket_state SET computed_at=NOW() WHERE day='2026-11-09'`)
	require.NoError(t, err)
	_, err = next()
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = tx.ExecContext(ctx, `UPDATE analytics_dirty_days SET reason='cost' WHERE day='2026-11-09'`)
	require.NoError(t, err)
	_, err = next()
	require.ErrorIs(t, err, sql.ErrNoRows, "a live cost refresh must share today's cooldown")
	_, err = tx.ExecContext(ctx, `UPDATE analytics_dirty_days SET reason='rebuild' WHERE day='2026-11-09'`)
	require.NoError(t, err)
	day, err = next()
	require.NoError(t, err)
	require.Equal(t, "2026-11-09", day)
}

func TestAnalyticsProgressMigrationRequeuesOldCanceledDaysAndIsIdempotent(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	_, err := tx.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day,attempts,last_error)
      VALUES('2026-11-01',1,'pq: canceling statement due to user request'),('2026-11-02',1,'invalid price');`)
	require.NoError(t, err)
	migration, err := migrations.FS.ReadFile("923_data_analytics_progress.sql")
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = tx.ExecContext(ctx, string(migration))
		require.NoError(t, err)
	}
	var retry time.Time
	var message string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT retry_after,last_error FROM analytics_dirty_days WHERE day='2026-11-01'`).Scan(&retry, &message))
	require.Contains(t, message, "已安排重试")
	var untouched bool
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT retry_after IS NULL AND last_error='invalid price' FROM analytics_dirty_days WHERE day='2026-11-02'`).Scan(&untouched))
	require.True(t, untouched)
}

func TestAnalyticsAllModesAndRetentionPreserveFacts(t *testing.T) {
	for _, partitioned := range []bool{false, true} {
		t.Run(fmt.Sprint(partitioned), func(t *testing.T) {
			tx, ctx := analyticsMigrationFixture(t, partitioned)
			_, err := tx.ExecContext(ctx, `
          INSERT INTO cost_plans VALUES(1,'采购方案');
          INSERT INTO cost_plan_versions VALUES(1,1,1,NOW()-INTERVAL '1 day',NULL);
          INSERT INTO cost_model_prices(id,plan_version_id,upstream_model,billing_mode,input_price_cny,output_price_cny,cache_write_price_cny,cache_read_price_cny)
            VALUES(1,1,'model','token',1,2,3,4);
          INSERT INTO account_cost_configs VALUES(1,1,'metered',1,NULL,NOW()-INTERVAL '1 day',NULL),(2,2,'excluded',NULL,NULL,NOW()-INTERVAL '1 day',NULL);
          INSERT INTO usage_logs(id,user_id,account_id,model,created_at,input_tokens,output_tokens,cache_creation_tokens,cache_read_tokens,actual_cost)
            VALUES(1,1,1,'model',NOW()-INTERVAL '1 second',1000,2000,3000,4000,999),(2,2,2,'excluded',NOW()-INTERVAL '1 second',500,0,0,0,999);
        `)
			require.NoError(t, err)
			var total, cost, mode string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT (input+output+cache_write+cache_read)::text,(input_cost+output_cost+write_cost+read_cost)::text FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f WHERE u.id=1`).Scan(&total, &cost))
			require.Equal(t, "10000", total)
			require.True(t, decimal.RequireFromString(cost).Equal(decimal.RequireFromString("0.03")))
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT mode FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f WHERE u.id=2`).Scan(&mode))
			require.Equal(t, "excluded", mode)
			_, err = tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET activated=TRUE; SAVEPOINT unready`)
			require.NoError(t, err)
			_, err = tx.ExecContext(ctx, `DELETE FROM usage_logs WHERE id=1`)
			require.Error(t, err)
			_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT unready;
          INSERT INTO analytics_daily SELECT f.* FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f;
          INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count) SELECT day,NOW(),SUM(requests),0 FROM analytics_daily GROUP BY day;
          DELETE FROM analytics_dirty_days;
          DELETE FROM usage_logs WHERE id=1;
        `)
			require.NoError(t, err)
			var archivedCount int64
			var archivedUntil time.Time
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT archived_count,archived_until FROM analytics_bucket_state WHERE archived_count>0`).Scan(&archivedCount, &archivedUntil))
			require.Equal(t, int64(1), archivedCount)
			require.False(t, archivedUntil.IsZero())
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT (input_cost+output_cost+write_cost+read_cost)::text FROM analytics_archived_daily`).Scan(&cost))
			require.True(t, decimal.RequireFromString(cost).Equal(decimal.RequireFromString("0.03")))
			var combined string
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT SUM(input+output+cache_write+cache_read)::text FROM(
          SELECT f.* FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f UNION ALL SELECT * FROM analytics_archived_daily) x`).Scan(&combined))
			require.Equal(t, "10500", combined)
			_, err = tx.ExecContext(ctx, `INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count) VALUES((NOW() AT TIME ZONE 'Asia/Shanghai')::date-1,NOW(),0,0) ON CONFLICT DO NOTHING;
          UPDATE cost_plan_versions SET effective_to=NOW() WHERE id=1`)
			require.NoError(t, err)
			var oldDirty bool
			require.NoError(t, tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_dirty_days WHERE day<(NOW() AT TIME ZONE 'Asia/Shanghai')::date)`).Scan(&oldDirty))
			require.False(t, oldDirty, "closing a price interval today must preserve yesterday")
		})
	}
}

func TestAnalyticsMonthlyFixedUsesDistinctUsersAndBalancesModelParts(t *testing.T) {
	tx, ctx := analyticsMigrationFixture(t, false)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	_, err := tx.ExecContext(ctx, `INSERT INTO cost_plans VALUES(1,'订阅采购')`)
	require.NoError(t, err)
	for _, statement := range []string{
		`INSERT INTO cost_subscription_units VALUES(1,1,'共享订阅',$1,NULL),(2,1,'无人订阅',$1,NULL)`,
		`INSERT INTO cost_subscription_unit_versions VALUES(1,1,310,$1,NULL),(2,2,20,$1,NULL)`,
		`INSERT INTO account_cost_configs VALUES(1,1,'fixed',1,1,$1,NULL),(2,2,'fixed',1,1,$1,NULL)`,
	} {
		_, err = tx.ExecContext(ctx, statement, month)
		require.NoError(t, err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO usage_logs(id,user_id,account_id,model,created_at,input_tokens)
      VALUES(1,1,1,'A',NOW(),1000),(2,1,2,'B',NOW(),3000),(3,2,1,'A',NOW(),100),(4,3,1,'failed',NOW(),0);
      INSERT INTO analytics_daily SELECT f.* FROM usage_logs u CROSS JOIN LATERAL analytics_resolve_log(u) f`)
	require.NoError(t, err)
	repo := &dataAnalyticsRepository{}
	require.NoError(t, repo.publishAnalyticsMonth(ctx, tx, month, now, false))
	var participants int
	var total, assigned string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT total::text,participants FROM analytics_fixed_totals WHERE unit_id=1`).Scan(&total, &participants))
	require.Equal(t, 2, participants)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT SUM(amount)::text FROM analytics_fixed_allocations WHERE unit_id=1`).Scan(&assigned))
	require.True(t, decimal.RequireFromString(total).Equal(decimal.RequireFromString(assigned)))
	var modelTotal, personal string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT SUM(fixed)::text FROM analytics_monthly WHERE user_id=1`).Scan(&modelTotal))
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT amount::text FROM analytics_fixed_allocations WHERE user_id=1 AND unit_id=1`).Scan(&personal))
	require.True(t, decimal.RequireFromString(modelTotal).Equal(decimal.RequireFromString(personal)))
	var a, b string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT fixed::text FROM analytics_monthly WHERE user_id=1 AND model='A'`).Scan(&a))
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT fixed::text FROM analytics_monthly WHERE user_id=1 AND model='B'`).Scan(&b))
	require.True(t, decimal.RequireFromString(b).Sub(decimal.RequireFromString(a).Mul(decimal.NewFromInt(3))).Abs().LessThanOrEqual(decimal.New(4, -12)))
	var nonuser int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_fixed_allocations WHERE user_id=3`).Scan(&nonuser))
	require.Zero(t, nonuser)
	var public, unused string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT total::text,public_cost::text FROM analytics_fixed_totals WHERE unit_id=2`).Scan(&unused, &public))
	require.True(t, decimal.RequireFromString(unused).Equal(decimal.RequireFromString(public)))
	_, err = tx.ExecContext(ctx, `CREATE TABLE cost_daily_aggregates(bucket_date date,aggregate_scope text,subscription_unit_id bigint,user_id bigint,calculation_status text DEFAULT 'calculated',amount_cny numeric);
      INSERT INTO cost_daily_aggregates(bucket_date,aggregate_scope,subscription_unit_id,user_id,amount_cny)
      VALUES((NOW() AT TIME ZONE 'Asia/Shanghai')::date,'fixed_plan_total',1,NULL,`+total+`),
        ((NOW() AT TIME ZONE 'Asia/Shanghai')::date,'fixed_user_allocation',1,99,999),
        ((NOW() AT TIME ZONE 'Asia/Shanghai')::date,'usage',NULL,1,10)`)
	require.NoError(t, err)
	rows, err := tx.QueryContext(ctx, analyticsCompatibleUserCostsSQL, month, month.AddDate(0, 1, 0))
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	seen := map[int64]string{}
	for rows.Next() {
		var id int64
		var dynamic, fixed, total string
		require.NoError(t, rows.Scan(&id, &dynamic, &fixed, &total))
		seen[id] = fixed
		if id == 1 {
			require.Equal(t, "10", dynamic)
			require.True(t, decimal.RequireFromString(fixed).Equal(decimal.RequireFromString(personal)))
		}
	}
	require.NoError(t, rows.Err())
	require.NotContains(t, seen, int64(99))
	require.Len(t, seen, 2)

	// The drawer must retain the selected publication even if a new month
	// generation has been published before its yearly breakdown is queried.
	var generation int64
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT MAX(generation) FROM analytics_publications WHERE month=$1::date`, month).Scan(&generation))
	_, err = tx.ExecContext(ctx, `INSERT INTO analytics_publications(month) VALUES($1::date)`, month)
	require.NoError(t, err)
	period, err := service.NewAnalyticsPeriod(now.Year(), "month", []int{int(now.Month())}, now)
	require.NoError(t, err)
	monthly, err := analyticsPersonMonths(ctx, tx, service.AnalyticsQuery{Period: period, UserID: 1}, map[string]int64{month.Format("2006-01-02"): generation})
	require.NoError(t, err)
	var found bool
	for _, row := range monthly {
		if row.Month == month.Format("2006-01-02") {
			found = true
			require.Equal(t, generation, row.Generation)
			require.Equal(t, "4000", row.Tokens)
			require.True(t, decimal.RequireFromString(row.Fixed).Equal(decimal.RequireFromString(personal)))
		} else {
			require.Zero(t, row.Generation)
			require.Equal(t, "0", row.Tokens)
		}
	}
	require.True(t, found)
	require.LessOrEqual(t, len(monthly), 12)
}

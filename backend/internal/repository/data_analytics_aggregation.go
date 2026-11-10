package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

const analyticsDimensions = "day,user_id,attribution,model,mode,source_id,source,issue"
const analyticsMeasures = "input,output,cache_write,cache_read,requests,valid_requests,input_cost,output_cost,write_cost,read_cost,image_cost,request_cost"

// shortcut: live cost changes may lag up to 5 minutes; use incremental buckets for tighter freshness.
const analyticsNextDayQuery = `SELECT d.day,d.version,d.reason,d.source_change_from
	FROM analytics_dirty_days d LEFT JOIN analytics_bucket_state b ON b.day=d.day
	WHERE (d.attempts=0 OR COALESCE(d.retry_after,d.changed_at+INTERVAL '1 hour')<=NOW())
	  AND (d.day<>$1::date OR d.reason NOT IN('usage','cost') OR b.computed_at IS NULL OR b.computed_at<=NOW()-INTERVAL '5 minutes')
	ORDER BY (d.day=$1::date),d.attempts>0,(d.day>=DATE_TRUNC('month',$1::date::timestamp)::date) DESC,d.day LIMIT 1`

func analyticsSums(prefix string) string {
	var fields []string
	for _, field := range strings.Split(analyticsMeasures, ",") {
		fields = append(fields, "SUM("+prefix+field+")")
	}
	return strings.Join(fields, ",")
}

func (r *dataAnalyticsRepository) RunAnalyticsBatch(ctx context.Context) (more bool, runErr error) {
	stage := "prepare"
	if err := service.ReportAnalyticsStage(ctx, stage, ""); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	var failedDay string
	var failedVersion int64
	defer func() {
		_ = tx.Rollback()
		if runErr != nil && ctx.Err() != nil {
			runErr = ctx.Err()
		}
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, service.ErrAnalyticsTasksPaused) && !errors.Is(runErr, service.ErrCostAggregationBusy) {
			message, retryDelay := analyticsBatchFailure(runErr)
			if failedDay == "" {
				message = "准备核算队列或同步目录失败：" + runErr.Error()
			} else if stage == "publish" {
				message = failedDay + " 发布统计：" + message
			} else {
				message = failedDay + " 汇总日用量：" + message
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = r.db.ExecContext(cleanup, `UPDATE analytics_runtime_state SET last_error=$1 WHERE id=1`, message)
			if failedDay != "" {
				_, _ = r.db.ExecContext(cleanup, `UPDATE analytics_dirty_days SET attempts=attempts+1,changed_at=NOW(),last_error=$3,retry_after=$4 WHERE day=$1::date AND version=$2`, failedDay, failedVersion, message, time.Now().Add(retryDelay))
			}
		}
	}()
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(9072028)`).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, service.ErrCostAggregationBusy
	}
	var seeded bool
	if err := tx.QueryRowContext(ctx, `SELECT seeded FROM analytics_runtime_state WHERE id=1`).Scan(&seeded); err != nil {
		return false, err
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	if !seeded {
		var ready bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_directory_versions)`).Scan(&ready); err != nil {
			return false, err
		}
		if !ready {
			return false, tx.Commit()
		}
		var earliest time.Time
		if err := tx.QueryRowContext(ctx, `SELECT GREATEST(((DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months') AT TIME ZONE 'Asia/Shanghai'),
			LEAST(COALESCE((SELECT MIN(created_at) FROM usage_logs),NOW()),
			  COALESCE((SELECT MIN(effective_from) FROM cost_subscription_units),NOW())))`).Scan(&earliest); err != nil {
			return false, err
		}
		earliest = earliest.In(loc)
		earliest = time.Date(earliest.Year(), earliest.Month(), 1, 0, 0, 0, 0, loc)
		if err := analyticsDirtyRange(ctx, tx, earliest, now, "initial"); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET seeded=TRUE,activated=TRUE,started_at=NOW(),
          raw_history_start=COALESCE(((SELECT MIN(created_at) FROM usage_logs) AT TIME ZONE 'Asia/Shanghai')::date,(NOW() AT TIME ZONE 'Asia/Shanghai')::date) WHERE id=1`); err != nil {
			return false, err
		}
		// Publish cleanup protection before starting the potentially expensive first day.
		if err := checkAnalyticsPublication(ctx, tx); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day,reason)
      SELECT d::date,'catchup' FROM generate_series(
        GREATEST((DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date,
          COALESCE((SELECT MAX(day)+1 FROM analytics_bucket_state),(NOW() AT TIME ZONE 'Asia/Shanghai')::date)),
        (NOW() AT TIME ZONE 'Asia/Shanghai')::date-1,INTERVAL '1 day') d
      WHERE NOT EXISTS(SELECT 1 FROM analytics_bucket_state s WHERE s.day=d::date)
        AND NOT EXISTS(SELECT 1 FROM analytics_dirty_days q WHERE q.day=d::date) ON CONFLICT DO NOTHING`); err != nil {
		return false, err
	}
	var syncDirectory bool
	if err := tx.QueryRowContext(ctx, `SELECT directory_synced_at IS NULL OR directory_synced_at<NOW()-INTERVAL '5 minutes' FROM analytics_runtime_state WHERE id=1`).Scan(&syncDirectory); err != nil {
		return false, err
	}
	if syncDirectory {
		var data []byte
		err := tx.QueryRowContext(ctx, `SELECT data FROM analytics_directory_versions ORDER BY effective_at DESC LIMIT 1`).Scan(&data)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if len(data) > 0 {
			var dir service.AnalyticsDirectory
			if err := json.Unmarshal(data, &dir); err != nil {
				return false, err
			}
			effective := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			var archived bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_bucket_state WHERE day=$1::date AND archived_count>0)`, now.Format("2006-01-02")).Scan(&archived); err != nil {
				return false, err
			}
			if archived {
				effective = now
			}
			if err := analyticsUpdateAssignments(ctx, tx, dir, effective, false, -1); err != nil {
				return false, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET directory_synced_at=NOW() WHERE id=1`); err != nil {
			return false, err
		}
	}
	var day time.Time
	var version int64
	var reason string
	var changeFrom sql.NullTime
	today := now.Format("2006-01-02")
	nextDay := func() error {
		return tx.QueryRowContext(ctx, analyticsNextDayQuery, today).Scan(&day, &version, &reason, &changeFrom)
	}
	err = nextDay()
	if errors.Is(err, sql.ErrNoRows) {
		// Do not touch today's queue row while historical work can proceed: live writes may hold it.
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day) SELECT $1::date
			WHERE NOT EXISTS(SELECT 1 FROM analytics_dirty_days WHERE day=$1::date)
			  AND NOT EXISTS(SELECT 1 FROM analytics_bucket_state WHERE day=$1::date AND computed_at>NOW()-INTERVAL '5 minutes')
			ON CONFLICT DO NOTHING`, today); err != nil {
			return false, err
		}
		err = nextDay()
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	date := day.Format("2006-01-02")
	failedDay, failedVersion = date, version
	stage = "aggregate"
	if err := service.ReportAnalyticsStage(ctx, stage, date); err != nil {
		return false, err
	}
	var pruned bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_bucket_state WHERE detail_pruned AND DATE_TRUNC('month',day)=DATE_TRUNC('month',$1::date))`, date).Scan(&pruned); err != nil {
		return false, err
	}
	if pruned && reason == "identity" {
		month := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, loc)
		stage = "publish"
		if err := service.ReportAnalyticsStage(ctx, stage, date); err != nil {
			return false, err
		}
		if err := r.publishAnalyticsMonth(ctx, tx, month, now, true); err != nil {
			return false, err
		}
		if err := checkAnalyticsPublication(ctx, tx); err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_dirty_days WHERE day=$1::date AND version=$2`, date, version); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if pruned {
		_, err = tx.ExecContext(ctx, `UPDATE analytics_dirty_days SET attempts=attempts+1,changed_at=NOW(),retry_after=NOW()+INTERVAL '1 hour',last_error='日明细已到期，不能重新发布该月；保留原月统计' WHERE day=$1::date`, date)
		if err != nil {
			return false, err
		}
		if err := checkAnalyticsPublication(ctx, tx); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	var archived int64
	var archivedUntil sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT archived_count FROM analytics_bucket_state WHERE day=$1::date),0),
      (SELECT archived_until FROM analytics_bucket_state WHERE day=$1::date)`, date).Scan(&archived, &archivedUntil); err != nil {
		return false, err
	}
	if archived > 0 && (reason == "cost" || reason == "directory" || reason == "rebuild") && (!changeFrom.Valid || !archivedUntil.Valid || !changeFrom.Time.After(archivedUntil.Time)) {
		_, err = tx.ExecContext(ctx, `UPDATE analytics_dirty_days SET attempts=attempts+1,changed_at=NOW(),retry_after=NOW()+INTERVAL '1 hour',
			last_error='源数据已清理，历史价格或归属不能完整重算；保留原统计' WHERE day=$1::date`, date)
		if err != nil {
			return false, err
		}
		if err := checkAnalyticsPublication(ctx, tx); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_daily WHERE day=$1::date`, date); err != nil {
		return false, err
	}
	query := `INSERT INTO analytics_daily
		SELECT day,user_id,attribution,model,mode,source_id,MAX(source),issue,` + analyticsSums("") + `
		FROM(
		  SELECT f.* FROM usage_logs ul CROSS JOIN LATERAL analytics_resolve_log(ul) f
		  WHERE ul.created_at>=($1::date::timestamp AT TIME ZONE 'Asia/Shanghai')
		    AND ul.created_at<(($1::date+1)::timestamp AT TIME ZONE 'Asia/Shanghai')
		  UNION ALL SELECT b.day,COALESCE(m.target_user_id,b.user_id),b.attribution,b.model,b.mode,b.source_id,b.source,b.issue,
          b.` + strings.ReplaceAll(analyticsMeasures, ",", ",b.") + ` FROM analytics_archived_daily b
          LEFT JOIN user_usage_migrations m ON m.source_user_id=b.user_id WHERE b.day=$1::date
		) facts GROUP BY day,user_id,attribution,model,mode,source_id,issue`
	result, err := tx.ExecContext(ctx, query, date)
	if err != nil {
		return false, err
	}
	if n, err := result.RowsAffected(); err != nil {
		return false, err
	} else if n > 100000 {
		return false, errors.New("日模型汇总超过100000行，请整理模型命名后补算")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_org_daily WHERE day=$1::date`, date); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_org_daily(day,org_id,level,tokens)
		SELECT d.day,COALESCE(a.org_ids[x.n],'0'),x.n,SUM(d.input+d.output+d.cache_write+d.cache_read)
		FROM analytics_daily d LEFT JOIN analytics_assignments a ON a.id=d.attribution
		CROSS JOIN generate_series(1,3) x(n)
		WHERE d.day=$1::date AND (a.org_ids[x.n] IS NOT NULL OR x.n=1)
		GROUP BY d.day,COALESCE(a.org_ids[x.n],'0'),x.n`, date); err != nil {
		return false, err
	}
	month := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, loc)
	stage = "publish"
	if err := service.ReportAnalyticsStage(ctx, stage, date); err != nil {
		return false, err
	}
	if err := r.publishAnalyticsMonth(ctx, tx, month, now, false); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_bucket_state(day,computed_at,request_count,pending_count,archived_count)
	SELECT $1::date,clock_timestamp(),COALESCE(SUM(requests),0),COALESCE(SUM(requests) FILTER(WHERE issue<>''),0),$2
	FROM analytics_daily WHERE day=$1::date ON CONFLICT(day) DO UPDATE SET
	computed_at=EXCLUDED.computed_at,request_count=EXCLUDED.request_count,pending_count=EXCLUDED.pending_count`, date, archived); err != nil {
		return false, err
	}
	if err := checkAnalyticsPublication(ctx, tx); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_dirty_days WHERE day=$1::date AND version=$2`, date, version); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET last_success_at=clock_timestamp(),last_error='' WHERE id=1`); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func analyticsBatchFailure(err error) (string, time.Duration) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "单日核算超过4分钟，已回滚本批结果，5分钟后自动重试；持续超时请检查数据库负载与锁等待", 5 * time.Minute
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "57014" {
		return "核算查询被取消，已回滚本批结果，5分钟后自动重试", 5 * time.Minute
	}
	return err.Error(), time.Hour
}

func (r *dataAnalyticsRepository) publishAnalyticsMonth(ctx context.Context, tx *sql.Tx, month, now time.Time, fromPublished bool) error {
	key, end := month.Format("2006-01-02"), month.AddDate(0, 1, 0).Format("2006-01-02")
	var previous int64
	if fromPublished {
		if err := tx.QueryRowContext(ctx, `SELECT MAX(generation) FROM analytics_publications WHERE month=$1::date`, key).Scan(&previous); err != nil {
			return err
		}
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO analytics_publications(month) VALUES($1::date) RETURNING generation`, key).Scan(&generation); err != nil {
		return err
	}
	source, extra := "analytics_daily", ""
	args := []any{key, end, generation}
	if fromPublished {
		source = "analytics_monthly"
		extra = " AND d.generation=$4"
		args = append(args, previous)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO analytics_monthly(`+analyticsDimensions+`,`+analyticsMeasures+`,generation)
		SELECT $1::date,COALESCE(m.target_user_id,d.user_id),d.attribution,d.model,d.mode,d.source_id,MAX(d.source),d.issue,`+analyticsSums("d.")+`,$3
		FROM `+source+` d LEFT JOIN user_usage_migrations m ON m.source_user_id=d.user_id
          WHERE d.day>=$1::date AND d.day<$2::date`+extra+`
		GROUP BY COALESCE(m.target_user_id,d.user_id),d.attribution,d.model,d.mode,d.source_id,d.issue`, args...)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n > 100000 {
		return errors.New("月模型汇总超过100000行，请整理模型命名或缩小组织规模")
	}
	if err := r.allocateAnalyticsFixed(ctx, tx, month, now, generation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_org_daily_versions
		SELECT $1::date,$3,day,org_id,level,tokens FROM analytics_org_daily WHERE day>=$1::date AND day<$2::date`, key, end, generation); err != nil {
		return err
	}
	// Readers can pin either of the last two complete, atomic publications.
	for _, table := range []string{"analytics_monthly", "analytics_fixed_totals", "analytics_fixed_allocations", "analytics_org_daily_versions"} {
		column := "month"
		if table == "analytics_monthly" {
			column = "day"
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE `+column+`=$1::date AND generation NOT IN(
			SELECT generation FROM analytics_publications WHERE month=$1::date ORDER BY generation DESC LIMIT 2)`, key); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM analytics_publications WHERE month=$1::date AND generation NOT IN(
		SELECT generation FROM analytics_publications WHERE month=$1::date ORDER BY generation DESC LIMIT 2)`, key)
	return err
}

func (r *dataAnalyticsRepository) allocateAnalyticsFixed(ctx context.Context, tx *sql.Tx, month, now time.Time, generation int64) error {
	type unitVersion struct {
		unit, version  int64
		name, monthly  string
		unitFrom, from time.Time
		unitTo, to     sql.NullTime
	}
	key := month.Format("2006-01-02")
	end := month.AddDate(0, 1, 0)
	rows, err := tx.QueryContext(ctx, `SELECT u.id,v.id,u.name,v.monthly_unit_cost_cny::text,u.effective_from,u.effective_to,v.effective_from,v.effective_to
		FROM cost_subscription_units u JOIN cost_subscription_unit_versions v ON v.subscription_unit_id=u.id
		WHERE u.effective_from<$2 AND(u.effective_to IS NULL OR u.effective_to>$1)
		  AND v.effective_from<$2 AND(v.effective_to IS NULL OR v.effective_to>$1) ORDER BY u.id,v.id LIMIT 100001`, month, end)
	if err != nil {
		return err
	}
	var versions []unitVersion
	for rows.Next() {
		var v unitVersion
		if err := rows.Scan(&v.unit, &v.version, &v.name, &v.monthly, &v.unitFrom, &v.unitTo, &v.from, &v.to); err != nil {
			_ = rows.Close()
			return err
		}
		versions = append(versions, v)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if len(versions) > 100000 {
		return errors.New("订阅版本过多，请清理到期配置")
	}
	totals, names := map[int64]decimal.Decimal{}, map[int64]string{}
	for _, v := range versions {
		monthly, err := decimal.NewFromString(v.monthly)
		if err != nil {
			return err
		}
		for d := month; d.Before(end) && !d.After(now); d = d.AddDate(0, 0, 1) {
			amount := fixedDailyCost(d, monthly, v.unitFrom, v.unitTo, v.from, v.to).Round(12)
			totals[v.unit] = totals[v.unit].Add(amount)
		}
		names[v.unit] = v.name
	}
	var units []int64
	for id := range totals {
		units = append(units, id)
	}
	sort.Slice(units, func(i, j int) bool { return units[i] < units[j] })
	for _, unit := range units {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT user_id FROM analytics_monthly
			WHERE day=$1::date AND generation=$2 AND mode='fixed' AND source_id=$3 AND valid_requests>0 AND issue='' ORDER BY user_id LIMIT 100001`, key, generation, unit)
		if err != nil {
			return err
		}
		var users []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			users = append(users, id)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if len(users) > 100000 {
			return errors.New("订阅参与人员超过100000")
		}
		public := decimal.Zero
		if len(users) == 0 {
			public = totals[unit]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_fixed_totals VALUES($1::date,$2,$3,$4,$5,$6,$7)`,
			key, generation, unit, names[unit], totals[unit].String(), len(users), public.String()); err != nil {
			return err
		}
		equal := make([]decimal.Decimal, len(users))
		for i := range equal {
			equal[i] = decimal.NewFromInt(1)
		}
		shares := service.AllocateAnalyticsCost(totals[unit], equal)
		amounts := make([]string, len(shares))
		for i := range shares {
			amounts[i] = shares[i].String()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_fixed_allocations(month,generation,unit_id,user_id,amount)
          SELECT $1::date,$2,$3,user_id,amount FROM UNNEST($4::bigint[],$5::numeric[]) AS shares(user_id,amount)`, key, generation, unit, pq.Array(users), pq.Array(amounts)); err != nil {
			return err
		}
		// Allocate to historical organisation/model rows in one indexed statement per subscription.
		if _, err := tx.ExecContext(ctx, `WITH parts AS(
          SELECT m.ctid,m.user_id,m.attribution,m.model,m.issue,a.amount,m.valid_requests,
            CASE WHEN m.valid_requests>0 AND m.issue='' THEN m.input+m.output+m.cache_write+m.cache_read ELSE 0 END tokens
          FROM analytics_monthly m JOIN analytics_fixed_allocations a
            ON a.month=m.day AND a.generation=m.generation AND a.unit_id=m.source_id AND a.user_id=m.user_id
          WHERE m.day=$1::date AND m.generation=$2 AND m.mode='fixed' AND m.source_id=$3
        ), weights AS(
          SELECT *,CASE WHEN SUM(tokens) OVER(PARTITION BY user_id)>0 THEN tokens
            WHEN issue='' THEN valid_requests ELSE 0 END weight FROM parts
        ), amounts AS(
          SELECT *,TRUNC(amount*weight/NULLIF(SUM(weight) OVER(PARTITION BY user_id),0),12) base FROM weights
        ), balanced AS(
          SELECT *,ROW_NUMBER() OVER(PARTITION BY user_id ORDER BY weight>0 DESC,attribution,model,issue) rn,
            ROUND((amount-SUM(base) OVER(PARTITION BY user_id))*1000000000000) remainder FROM amounts
        ) UPDATE analytics_monthly m SET fixed=b.base+CASE WHEN b.rn<=b.remainder AND b.weight>0 THEN 0.000000000001 ELSE 0 END
          FROM balanced b WHERE m.ctid=b.ctid`, key, generation, unit); err != nil {
			return err
		}
	}
	return nil
}

func (r *dataAnalyticsRepository) PurgeAnalyticsPreviews(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM analytics_directory_imports WHERE id IN(
      SELECT id FROM analytics_directory_imports WHERE (applied_at IS NULL AND expires_at<NOW()) OR created_at<NOW()-INTERVAL '90 days' ORDER BY created_at LIMIT 1000)`)
	return err
}

func (r *dataAnalyticsRepository) AnalyticsMaintenance(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var locked bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(9072028)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return service.ErrCostAggregationBusy
	}
	// Mark the whole expired month before deleting pieces, so it can never be republished from partial daily data.
	if _, err := tx.ExecContext(ctx, `UPDATE analytics_bucket_state s SET detail_pruned=TRUE
      WHERE NOT detail_pruned AND day<(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '12 months')::date
        AND EXISTS(SELECT 1 FROM analytics_publications p WHERE p.month=DATE_TRUNC('month',s.day)::date)
        AND NOT EXISTS(SELECT 1 FROM analytics_dirty_days d WHERE DATE_TRUNC('month',d.day)=DATE_TRUNC('month',s.day))`); err != nil {
		return err
	}
	for _, table := range []string{"analytics_daily", "analytics_archived_daily"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE ctid IN(SELECT d.ctid FROM `+table+` d
          WHERE EXISTS(SELECT 1 FROM analytics_bucket_state s WHERE s.day=d.day AND s.detail_pruned) LIMIT 5000)`); err != nil {
			return err
		}
	}
	for table, column := range map[string]string{"analytics_monthly": "day", "analytics_org_daily": "day", "analytics_org_daily_versions": "month", "analytics_bucket_state": "day", "analytics_dirty_days": "day", "analytics_fixed_totals": "month", "analytics_fixed_allocations": "month", "analytics_publications": "month"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE ctid IN(SELECT ctid FROM `+table+` WHERE `+column+`<(DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '59 months')::date LIMIT 5000)`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_rebuild_audit WHERE id IN(SELECT id FROM analytics_rebuild_audit WHERE created_at<NOW()-INTERVAL '60 months' LIMIT 1000)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_assignments WHERE id IN(SELECT a.id FROM analytics_assignments a WHERE effective_to<NOW()-INTERVAL '60 months'
      AND NOT EXISTS(SELECT 1 FROM analytics_monthly m WHERE m.attribution=a.id) AND NOT EXISTS(SELECT 1 FROM analytics_daily d WHERE d.attribution=a.id)
      AND NOT EXISTS(SELECT 1 FROM analytics_archived_daily b WHERE b.attribution=a.id) LIMIT 1000)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM analytics_directory_versions WHERE id IN(SELECT id FROM analytics_directory_versions WHERE effective_at<NOW()-INTERVAL '60 months'
      AND id<>(SELECT id FROM analytics_directory_versions ORDER BY effective_at DESC LIMIT 1)
      AND id<>(SELECT id FROM analytics_directory_versions WHERE effective_at<NOW()-INTERVAL '60 months' ORDER BY effective_at DESC LIMIT 1) LIMIT 1000)`); err != nil {
		return err
	}
	if err := checkAnalyticsPublication(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

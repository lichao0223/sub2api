package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

var analyticsValueKeys = []string{"input", "output", "cache_write", "cache_read", "tokens", "requests", "metered", "fixed", "cost", "pending", "input_cost", "output_cost", "write_cost", "read_cost", "image_cost", "request_cost", "excluded_tokens"}

func analyticsValueMap(v service.AnalyticsValues) map[string]decimal.Decimal {
	raw := []string{v.Input, v.Output, v.CacheWrite, v.CacheRead, v.Tokens, v.Requests, v.Metered, v.Fixed, v.Cost, v.Pending, v.InputCost, v.OutputCost, v.WriteCost, v.ReadCost, v.ImageCost, v.RequestCost, v.Excluded}
	out := map[string]decimal.Decimal{}
	for i, key := range analyticsValueKeys {
		out[key], _ = decimal.NewFromString(raw[i])
	}
	return out
}

func analyticsValues(v map[string]decimal.Decimal) service.AnalyticsValues {
	return service.AnalyticsValues{
		Input: v["input"].String(), Output: v["output"].String(), CacheWrite: v["cache_write"].String(), CacheRead: v["cache_read"].String(),
		Tokens: v["tokens"].String(), Requests: v["requests"].String(), Metered: v["metered"].String(), Fixed: v["fixed"].String(),
		Cost: v["cost"].String(), Pending: v["pending"].String(), InputCost: v["input_cost"].String(), OutputCost: v["output_cost"].String(),
		WriteCost: v["write_cost"].String(), ReadCost: v["read_cost"].String(), ImageCost: v["image_cost"].String(), RequestCost: v["request_cost"].String(), Excluded: v["excluded_tokens"].String(),
	}
}

func analyticsAdd(total map[string]decimal.Decimal, values service.AnalyticsValues) {
	for key, value := range analyticsValueMap(values) {
		total[key] = total[key].Add(value)
	}
}

func (r *dataAnalyticsRepository) AnalyticsReport(ctx context.Context, q service.AnalyticsQuery) (*service.AnalyticsReport, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	locked := false
	for slot := int64(9072031); slot < 9072033; slot++ {
		if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock($1)`, slot).Scan(&locked); err != nil {
			return nil, err
		}
		if locked {
			break
		}
	}
	if !locked {
		return nil, errors.New("分析查询繁忙，请稍后重试")
	}
	if q.Export {
		// A transaction-scoped per-export lock bounds global and actor work.
		var lock bool
		if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(9072030)`).Scan(&lock); err != nil {
			return nil, err
		}
		if !lock {
			return nil, errors.New("已有报表正在准备，请稍后重试")
		}
	}
	report := &service.AnalyticsReport{
		Period: q.Period, ScopeOrgID: q.OrgID, Versions: map[string]int64{}, Coverage: map[string]any{},
		People: []service.AnalyticsRow{}, Models: []service.AnalyticsRow{}, Departments: []service.AnalyticsDepartment{},
		Trend: []service.AnalyticsTrend{}, FixedShares: []service.AnalyticsFixedShare{}, PublicCost: "0",
	}
	rows, err := tx.QueryContext(ctx, `SELECT month::text,MAX(generation) FROM analytics_publications WHERE month=ANY($1::date[]) GROUP BY month`, pq.Array(q.Period.Keys))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		var generation int64
		if err := rows.Scan(&key, &generation); err != nil {
			_ = rows.Close()
			return nil, err
		}
		report.Versions[key] = generation
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for _, key := range q.Period.Keys {
		if _, ok := report.Versions[key]; !ok {
			report.Versions[key] = 0
		}
	}
	if len(q.Versions) > 0 && len(q.Versions) != len(q.Period.Keys) {
		return nil, errors.New("发布版本必须覆盖全部所选月份")
	}
	for key, generation := range q.Versions {
		valid := false
		for _, selected := range q.Period.Keys {
			valid = valid || key == selected
		}
		if !valid {
			return nil, errors.New("发布版本与所选月份不一致")
		}
		if generation == 0 && report.Versions[key] == 0 {
			continue
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_publications WHERE month=$1::date AND generation=$2)`, key, generation).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, service.ErrAnalyticsControlConflict
		}
		report.Versions[key] = generation
	}
	versions := service.AnalyticsJSON(report.Versions)
	rows, err = tx.QueryContext(ctx, `SELECT m.user_id,m.attribution,COALESCE(a.person_id,''),COALESCE(a.name,u.username,'用户#'||m.user_id),
		COALESCE(a.org_ids,'{}'::text[]),COALESCE(a.org_names,'{}'::text[]),m.model,m.mode,m.source_id,MAX(m.source),m.issue,
		SUM(input)::text,SUM(output)::text,SUM(cache_write)::text,SUM(cache_read)::text,SUM(requests)::text,
		SUM(input_cost)::text,SUM(output_cost)::text,SUM(write_cost)::text,SUM(read_cost)::text,SUM(image_cost)::text,SUM(request_cost)::text,SUM(fixed)::text
		FROM analytics_monthly m LEFT JOIN analytics_assignments a ON a.id=m.attribution LEFT JOIN users u ON u.id=m.user_id
		WHERE m.day=ANY($1::date[]) AND m.generation=($2::jsonb->>m.day::text)::bigint
		  AND ($3::bigint=0 OR m.user_id=$3)
          AND ($4='' OR $4=ANY(a.org_ids) OR ($4='0' AND (a.id IS NULL OR CARDINALITY(a.org_ids)=0)))
		GROUP BY m.user_id,m.attribution,a.person_id,a.name,u.username,a.org_ids,a.org_names,m.model,m.mode,m.source_id,m.issue
		ORDER BY m.user_id,m.attribution,m.model,m.mode,m.source_id,m.issue LIMIT 100001`, pq.Array(q.Period.Keys), versions, q.UserID, q.OrgID)
	if err != nil {
		return nil, err
	}
	var facts []service.AnalyticsRow
	for rows.Next() {
		var row service.AnalyticsRow
		var ids, names pq.StringArray
		err := rows.Scan(&row.UserID, &row.Attribution, &row.PersonID, &row.Name, &ids, &names, &row.Model, &row.Mode, &row.SourceID, &row.Source, &row.Issue,
			&row.Input, &row.Output, &row.CacheWrite, &row.CacheRead, &row.Requests, &row.InputCost, &row.OutputCost, &row.WriteCost, &row.ReadCost, &row.ImageCost, &row.RequestCost, &row.Fixed)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		row.OrgIDs, row.OrgNames = []string(ids), []string(names)
		if row.OrgIDs == nil {
			row.OrgIDs, row.OrgNames = []string{}, []string{}
		}
		v := analyticsValueMap(row.AnalyticsValues)
		v["tokens"] = v["input"].Add(v["output"]).Add(v["cache_write"]).Add(v["cache_read"])
		v["metered"] = v["input_cost"].Add(v["output_cost"]).Add(v["write_cost"]).Add(v["read_cost"]).Add(v["image_cost"]).Add(v["request_cost"])
		v["cost"] = v["metered"].Add(v["fixed"])
		if row.Issue != "" {
			v["pending"] = v["requests"]
		}
		if row.Mode == "excluded" {
			v["excluded_tokens"] = v["tokens"]
		}
		row.AnalyticsValues = analyticsValues(v)
		facts = append(facts, row)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(facts) > 100000 {
		return nil, errors.New("明细超过100000行，请减少月份或按部门拆分")
	}
	total := map[string]decimal.Decimal{}
	people := map[int64]*service.AnalyticsRow{}
	personValues := map[int64]map[string]decimal.Decimal{}
	scopeUsers := map[int64]bool{}
	departments := map[string]*service.AnalyticsDepartment{}
	deptValues, deptUsers := map[string]map[string]decimal.Decimal{}, map[string]map[int64]bool{}
	matching := map[int64]bool{}
	for _, row := range facts {
		if q.Search == "" || strings.Contains(strings.ToLower(row.Name), strings.ToLower(q.Search)) {
			matching[row.UserID] = true
		}
	}
	for _, row := range facts {
		inOrg := q.OrgID == ""
		for _, id := range row.OrgIDs {
			inOrg = inOrg || id == q.OrgID
		}
		inOrg = inOrg || (q.OrgID == "0" && len(row.OrgIDs) == 0)
		if !inOrg {
			continue
		}
		analyticsAdd(total, row.AnalyticsValues)
		report.Models = append(report.Models, row)
		scopeUsers[row.UserID] = true
		if matching[row.UserID] {
			if people[row.UserID] == nil {
				copy := row
				people[row.UserID] = &copy
				personValues[row.UserID] = map[string]decimal.Decimal{}
			} else if strings.Join(people[row.UserID].OrgIDs, "/") != strings.Join(row.OrgIDs, "/") {
				people[row.UserID].OrgIDs, people[row.UserID].OrgNames = []string{}, []string{"期间涉及多个组织（见个人明细）"}
			}
			analyticsAdd(personValues[row.UserID], row.AnalyticsValues)
		}
		ids, names := row.OrgIDs, row.OrgNames
		if len(ids) == 0 {
			ids, names = []string{"0"}, []string{"待确认归属"}
		}
		for level, id := range ids {
			if level >= 3 {
				break
			}
			if departments[id] == nil {
				parent := ""
				if level > 0 {
					parent = ids[level-1]
				}
				departments[id] = &service.AnalyticsDepartment{ID: id, Name: names[level], Parent: parent, Level: level + 1}
				deptValues[id], deptUsers[id] = map[string]decimal.Decimal{}, map[int64]bool{}
			}
			analyticsAdd(deptValues[id], row.AnalyticsValues)
			deptUsers[id][row.UserID] = true
		}
	}
	for id, p := range people {
		p.AnalyticsValues = analyticsValues(personValues[id])
		report.People = append(report.People, *p)
	}
	report.PeopleTotal, report.Users = len(people), len(scopeUsers)
	sortKey := "tokens"
	if q.Sort == "cost" {
		sortKey = "cost"
	}
	sort.Slice(report.People, func(i, j int) bool {
		if q.Sort == "name" && report.People[i].Name != report.People[j].Name {
			return report.People[i].Name < report.People[j].Name
		}
		a, b := personValues[report.People[i].UserID][sortKey], personValues[report.People[j].UserID][sortKey]
		if a.Equal(b) {
			return report.People[i].UserID < report.People[j].UserID
		}
		return a.GreaterThan(b)
	})
	if !q.Export {
		page, size := q.Page, q.PageSize
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 100 {
			size = 20
		}
		start := (page - 1) * size
		if start > len(report.People) {
			start = len(report.People)
		}
		end := start + size
		if end > len(report.People) {
			end = len(report.People)
		}
		report.People = report.People[start:end]
		if q.UserID == 0 {
			report.Models = []service.AnalyticsRow{}
		}
	}
	for id, d := range departments {
		d.AnalyticsValues = analyticsValues(deptValues[id])
		d.Users = len(deptUsers[id])
		report.Departments = append(report.Departments, *d)
	}
	sort.Slice(report.Departments, func(i, j int) bool {
		a, b := report.Departments[i], report.Departments[j]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		if a.Tokens == b.Tokens {
			return a.ID < b.ID
		}
		return deptValues[a.ID]["tokens"].GreaterThan(deptValues[b.ID]["tokens"])
	})
	rows, err = tx.QueryContext(ctx, `SELECT t.month::text,t.unit_id,t.name,COALESCE(a.user_id,0),t.total::text,t.participants,
		COALESCE(a.amount,0)::text,t.public_cost::text
		FROM analytics_fixed_totals t LEFT JOIN analytics_fixed_allocations a ON a.month=t.month AND a.generation=t.generation AND a.unit_id=t.unit_id AND ($4::boolean OR $3::bigint>0)
		WHERE t.month=ANY($1::date[]) AND t.generation=($2::jsonb->>t.month::text)::bigint
		  AND ($3::bigint=0 OR a.user_id=$3)
          AND ($5='' OR a.user_id=ANY($6::bigint[]))
		ORDER BY t.month,t.unit_id,a.user_id LIMIT 100001`, pq.Array(q.Period.Keys), versions, q.UserID, q.Export, q.OrgID, pq.Array(analyticsUserIDs(scopeUsers)))
	if err != nil {
		return nil, err
	}
	seenUnits := map[string]bool{}
	fixedRows := 0
	public := decimal.Zero
	for rows.Next() {
		fixedRows++
		var share service.AnalyticsFixedShare
		if err := rows.Scan(&share.Month, &share.UnitID, &share.Name, &share.UserID, &share.Total, &share.Participants, &share.Amount, &share.Public); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if share.UserID == 0 && q.OrgID == "" && q.UserID == 0 {
			key := share.Month + "/" + strconv.FormatInt(share.UnitID, 10)
			if !seenUnits[key] {
				public = public.Add(decimal.RequireFromString(share.Public))
				seenUnits[key] = true
			}
		}
		if q.UserID > 0 || q.Export {
			if share.UserID == 0 || people[share.UserID] != nil {
				report.FixedShares = append(report.FixedShares, share)
			}
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if fixedRows > 100000 {
		return nil, errors.New("固定分摊明细过多，请拆分导出")
	}
	total["fixed"] = total["fixed"].Add(public)
	total["cost"] = total["cost"].Add(public)
	report.Summary, report.PublicCost = analyticsValues(total), public.String()
	if err := analyticsReportTrend(ctx, tx, q, report); err != nil {
		return nil, err
	}
	if q.UserID > 0 {
		report.PersonMonths, err = analyticsPersonMonths(ctx, tx, q, report.Versions)
		if err != nil {
			return nil, err
		}
	}
	var directoryReady bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_directory_versions)`).Scan(&directoryReady); err != nil {
		return nil, err
	}
	var backlog, failed, pending int64
	var first, last *time.Time
	var lastError string
	var rawStart *time.Time
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE last_error<>''),MIN(day) FROM analytics_dirty_days`).Scan(&backlog, &failed, &first); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT last_success_at,last_error,raw_history_start FROM analytics_runtime_state WHERE id=1`).Scan(&last, &lastError, &rawStart); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(pending_count),0) FROM analytics_bucket_state WHERE DATE_TRUNC('month',day)::date=ANY($1::date[])`, pq.Array(q.Period.Keys)).Scan(&pending); err != nil {
		return nil, err
	}
	var completed int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_bucket_state
		WHERE DATE_TRUNC('month',day)::date=ANY($1::date[]) AND NOT EXISTS(SELECT 1 FROM analytics_dirty_days d WHERE d.day=analytics_bucket_state.day)`, pq.Array(q.Period.Keys)).Scan(&completed); err != nil {
		return nil, err
	}
	expected := 0
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	for _, month := range q.Period.Months {
		start := time.Date(q.Period.Year, time.Month(month), 1, 0, 0, 0, 0, loc)
		stop := start.AddDate(0, 1, 0)
		for d := start; d.Before(stop) && !d.After(now); d = d.AddDate(0, 0, 1) {
			expected++
		}
	}
	issues := []map[string]string{}
	rows, err = tx.QueryContext(ctx, `SELECT day::text,last_error FROM analytics_dirty_days WHERE last_error<>'' ORDER BY day LIMIT 5`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var day, message string
		if err := rows.Scan(&day, &message); err != nil {
			_ = rows.Close()
			return nil, err
		}
		issues = append(issues, map[string]string{"day": day, "message": message})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	historyGap := rawStart != nil && q.Period.Keys[0] < rawStart.In(loc).Format("2006-01-02")
	report.Coverage = map[string]any{"directory_ready": directoryReady, "issues": issues, "raw_history_start": rawStart, "history_gap": historyGap, "backlog_days": backlog, "failed_days": failed, "earliest_pending": first,
		"last_updated": last, "last_error": lastError, "pending_requests": pending, "completed_days": completed, "expected_days": expected,
		"complete": completed == expected && pending == 0 && !historyGap, "history_basis": "首次目录按当前组织回溯；后续按实际生效时间归属", "provisional": q.Period.Year == now.Year() && analyticsContainsMonth(q.Period.Months, int(now.Month()))}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return report, nil
}

// One indexed yearly query serves the drawer, with selected months pinned to
// the same publications as the visible report.
func analyticsPersonMonths(ctx context.Context, tx *sql.Tx, q service.AnalyticsQuery, versions map[string]int64) ([]service.AnalyticsPersonMonth, error) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	period, err := service.NewAnalyticsPeriod(q.Period.Year, "year", nil, time.Now())
	if err != nil {
		return nil, err
	}
	start := time.Date(q.Period.Year, 1, 1, 0, 0, 0, 0, loc)
	rows, err := tx.QueryContext(ctx, `WITH publications AS (
		SELECT month,COALESCE(($3::jsonb->>month::text)::bigint,MAX(generation)) AS generation
		FROM analytics_publications WHERE month >= $1::date AND month < $2::date GROUP BY month
	), facts AS (
		SELECT m.* FROM analytics_monthly m LEFT JOIN analytics_assignments a ON a.id=m.attribution
		WHERE m.user_id=$4 AND m.day >= $1::date AND m.day < $2::date
		  AND ($5='' OR $5=ANY(a.org_ids) OR ($5='0' AND (a.id IS NULL OR CARDINALITY(a.org_ids)=0)))
	)
	SELECT p.month::text,p.generation,
		COALESCE(SUM(m.input),0)::text,COALESCE(SUM(m.output),0)::text,
		COALESCE(SUM(m.cache_write),0)::text,COALESCE(SUM(m.cache_read),0)::text,COALESCE(SUM(m.requests),0)::text,
		COALESCE(SUM(m.input_cost),0)::text,COALESCE(SUM(m.output_cost),0)::text,
		COALESCE(SUM(m.write_cost),0)::text,COALESCE(SUM(m.read_cost),0)::text,
		COALESCE(SUM(m.image_cost),0)::text,COALESCE(SUM(m.request_cost),0)::text,COALESCE(SUM(m.fixed),0)::text,
		COALESCE(SUM(CASE WHEN m.mode='excluded' THEN m.input+m.output+m.cache_write+m.cache_read ELSE 0 END),0)::text,
		COALESCE(SUM(CASE WHEN m.issue<>'' THEN m.requests ELSE 0 END),0)::text
	FROM publications p LEFT JOIN facts m ON m.day=p.month AND m.generation=p.generation
	GROUP BY p.month,p.generation ORDER BY p.month`,
		start, start.AddDate(1, 0, 0), service.AnalyticsJSON(versions), q.UserID, q.OrgID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	byMonth := map[string]service.AnalyticsPersonMonth{}
	for rows.Next() {
		var month service.AnalyticsPersonMonth
		if err := rows.Scan(&month.Month, &month.Generation, &month.Input, &month.Output, &month.CacheWrite, &month.CacheRead, &month.Requests,
			&month.InputCost, &month.OutputCost, &month.WriteCost, &month.ReadCost, &month.ImageCost, &month.RequestCost, &month.Fixed, &month.Excluded, &month.Pending); err != nil {
			return nil, err
		}
		values := analyticsValueMap(month.AnalyticsValues)
		values["tokens"] = values["input"].Add(values["output"]).Add(values["cache_write"]).Add(values["cache_read"])
		values["metered"] = values["input_cost"].Add(values["output_cost"]).Add(values["write_cost"]).Add(values["read_cost"]).Add(values["image_cost"]).Add(values["request_cost"])
		values["cost"] = values["metered"].Add(values["fixed"])
		month.AnalyticsValues = analyticsValues(values)
		byMonth[month.Month] = month
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]service.AnalyticsPersonMonth, 0, len(period.Keys))
	for _, key := range period.Keys {
		month, ok := byMonth[key]
		if !ok {
			month = service.AnalyticsPersonMonth{Month: key, AnalyticsValues: analyticsValues(map[string]decimal.Decimal{})}
		}
		result = append(result, month)
	}
	return result, nil
}

func analyticsReportTrend(ctx context.Context, tx *sql.Tx, q service.AnalyticsQuery, report *service.AnalyticsReport) error {
	if q.UserID > 0 {
		return nil
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	if q.Period.Grain == "day" {
		rows, err := tx.QueryContext(ctx, `SELECT day::text,SUM(tokens)::text FROM analytics_org_daily_versions
			WHERE month=$1::date AND generation=$3 AND (($2='' AND level=1) OR org_id=$2) GROUP BY day ORDER BY day`, q.Period.Keys[0], q.OrgID, report.Versions[q.Period.Keys[0]])
		if err != nil {
			return err
		}
		values := map[string]string{}
		for rows.Next() {
			var date, tokens string
			if err := rows.Scan(&date, &tokens); err != nil {
				_ = rows.Close()
				return err
			}
			values[date] = tokens
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		start := time.Date(q.Period.Year, time.Month(q.Period.Months[0]), 1, 0, 0, 0, 0, loc)
		for d := start; d.Before(start.AddDate(0, 1, 0)) && !d.After(now); d = d.AddDate(0, 0, 1) {
			value := values[d.Format("2006-01-02")]
			if value == "" {
				value = "0"
			}
			report.Trend = append(report.Trend, service.AnalyticsTrend{Bucket: d.Format("2006-01-02"), Tokens: value})
		}
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT m.day::text,SUM(input+output+cache_write+cache_read)::text
		FROM analytics_monthly m LEFT JOIN analytics_assignments a ON a.id=m.attribution LEFT JOIN users u ON u.id=m.user_id
		WHERE m.day=ANY($1::date[]) AND m.generation=($2::jsonb->>m.day::text)::bigint
		  AND ($3='' OR $3=ANY(a.org_ids) OR ($3='0' AND (a.id IS NULL OR CARDINALITY(a.org_ids)=0)))
		  AND ($4::bigint=0 OR m.user_id=$4)
		GROUP BY 1 ORDER BY 1`, pq.Array(q.Period.Keys), service.AnalyticsJSON(report.Versions), q.OrgID, q.UserID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		values[key] = value
	}
	for _, key := range q.Period.Keys {
		value := values[key]
		if value == "" {
			value = "0"
		}
		report.Trend = append(report.Trend, service.AnalyticsTrend{Bucket: key[:7], Tokens: value})
	}
	return rows.Err()
}

func analyticsUserIDs(users map[int64]bool) []int64 {
	ids := make([]int64, 0, len(users))
	for id := range users {
		ids = append(ids, id)
	}
	return ids
}
func analyticsContainsMonth(months []int, month int) bool {
	for _, m := range months {
		if m == month {
			return true
		}
	}
	return false
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type UserTokenRankingItem = usagestats.UserTokenRankingItem
type UserTokenRankingResponse = usagestats.UserTokenRankingResponse
type UserNonworkTokenRankingItem = usagestats.UserNonworkTokenRankingItem
type UserNonworkTokenRankingResponse = usagestats.UserNonworkTokenRankingResponse

// GetUserTokenRanking returns user ranking aggregated by token usage within the time range.
func (r *usageLogRepository) GetUserTokenRanking(ctx context.Context, startTime, endTime time.Time, limit int) (result *UserTokenRankingResponse, err error) {
	query := `
		WITH internal_stats AS (
			SELECT
				u.user_id,
				COALESCE(SUM(u.actual_cost), 0) as actual_cost,
				COALESCE(COUNT(u.id), 0) as requests,
				COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) as tokens
			FROM usage_logs u
			WHERE u.created_at >= $1 AND u.created_at < $2
			GROUP BY u.user_id
		),
		external_stats AS (
			SELECT
				s.user_id,
				COALESCE(SUM(s.actual_cost), 0) as actual_cost,
				COALESCE(SUM(s.requests), 0) as requests,
				COALESCE(SUM(s.total_tokens), 0) as tokens
			FROM external_usage_daily_user_stats s
			JOIN external_usage_import_batches b ON b.id = s.batch_id AND b.status = 'imported'
			WHERE s.bucket_date >= ($1 AT TIME ZONE 'Asia/Shanghai')::date
			  AND s.bucket_date < ($2 AT TIME ZONE 'Asia/Shanghai')::date
			GROUP BY s.user_id
		),
		user_tokens AS (
			SELECT
				us.id as user_id,
				COALESCE(us.email, '') as email,
				COALESCE(us.username, '') as username,
				COALESCE(i.actual_cost, 0) + COALESCE(e.actual_cost, 0) as actual_cost,
				COALESCE(i.requests, 0) + COALESCE(e.requests, 0) as requests,
				COALESCE(i.tokens, 0) + COALESCE(e.tokens, 0) as tokens
			FROM users us
			LEFT JOIN internal_stats i ON i.user_id = us.id
			LEFT JOIN external_stats e ON e.user_id = us.id
			WHERE us.deleted_at IS NULL AND us.role <> $3
		),
		ranked AS (
			SELECT
				user_id,
				email,
				username,
				actual_cost,
				requests,
				tokens,
				COALESCE(SUM(actual_cost) OVER (), 0) as total_actual_cost,
				COALESCE(SUM(requests) OVER (), 0) as total_requests,
				COALESCE(SUM(tokens) OVER (), 0) as total_tokens
			FROM user_tokens
			ORDER BY tokens DESC, actual_cost DESC, user_id ASC
			LIMIT NULLIF($4, 0)
		)
		SELECT
			user_id,
			email,
			username,
			actual_cost,
			requests,
			tokens,
			total_actual_cost,
			total_requests,
			total_tokens
		FROM ranked
		ORDER BY tokens DESC, actual_cost DESC, user_id ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, startTime, endTime, service.RoleAdmin, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	ranking := make([]UserTokenRankingItem, 0)
	totalActualCost := 0.0
	totalRequests := int64(0)
	totalTokens := int64(0)
	for rows.Next() {
		var row UserTokenRankingItem
		if err = rows.Scan(&row.UserID, &row.Email, &row.Username, &row.ActualCost, &row.Requests, &row.Tokens, &totalActualCost, &totalRequests, &totalTokens); err != nil {
			return nil, err
		}
		ranking = append(ranking, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &UserTokenRankingResponse{
		Ranking:         ranking,
		TotalActualCost: totalActualCost,
		TotalRequests:   totalRequests,
		TotalTokens:     totalTokens,
	}, nil
}

func (r *usageLogRepository) GetUserNonworkTokenRanking(ctx context.Context, startDate, endDate time.Time, scope, rankBy, sortOrder, tz string, externalOrganizationIDs []string, username string, limit int) (result *UserNonworkTokenRankingResponse, err error) {
	if limit <= 0 {
		limit = 50
	}
	if strings.TrimSpace(tz) == "" {
		tz = "Asia/Shanghai"
	}
	externalOrganizationIDs = strings.FieldsFunc(strings.Join(externalOrganizationIDs, ","), func(r rune) bool {
		return r == ','
	})
	for i := range externalOrganizationIDs {
		externalOrganizationIDs[i] = strings.TrimSpace(externalOrganizationIDs[i])
	}
	username = strings.TrimSpace(username)
	coverage, err := r.GetNonworkStatsCoverage(ctx, startDate, endDate, tz)
	if err != nil {
		return nil, err
	}
	segments := nonworkRankingSegments(scope)
	innerOrderExpr, outerOrderExpr := nonworkRankingOrderExprs(rankBy)
	innerDirection, outerDirection := nonworkRankingDirections(sortOrder)

	query := fmt.Sprintf(`
		WITH filtered_users AS (
			SELECT u.id, u.email, u.username
			FROM users u
			WHERE u.deleted_at IS NULL
			  AND u.role <> $5
			  AND (
				  cardinality($6::text[]) = 0
				  OR EXISTS (
					  SELECT 1
					  FROM external_user_mappings eum
					  WHERE eum.user_id = u.id
					    AND eum.deleted_at IS NULL
					    AND eum.external_organization_id = ANY($6::text[])
				  )
			  )
			  AND (
				  $7 = ''
				  OR u.username ILIKE '%%' || $7 || '%%'
				  OR EXISTS (
					  SELECT 1
					  FROM external_user_mappings eum_name
					  WHERE eum_name.user_id = u.id
					    AND eum_name.deleted_at IS NULL
					    AND eum_name.username_snapshot ILIKE '%%' || $7 || '%%'
				  )
			  )
		),
		stats AS (
			SELECT
				user_id,
				COALESCE(SUM(requests), 0) AS requests,
				COALESCE(SUM(total_tokens), 0) AS tokens,
				COALESCE(SUM(total_tokens) FILTER (WHERE segment IN ('offday', 'after_hours')), 0) AS nonwork_tokens,
				COALESCE(SUM(active_ms), 0) AS active_duration_ms,
				COALESCE(SUM(active_ms) FILTER (WHERE segment IN ('offday', 'after_hours')), 0) AS nonwork_active_ms,
				COALESCE(SUM(actual_cost), 0) AS actual_cost,
				COALESCE(BOOL_AND(calendar_confirmed), TRUE) AS calendar_confirmed
			FROM usage_nonwork_daily_user_stats st
			WHERE bucket_date >= $1::date
			  AND bucket_date <= $2::date
			  AND timezone = $3
			  AND segment = ANY($4)
			  AND EXISTS (SELECT 1 FROM filtered_users fu WHERE fu.id = st.user_id)
			GROUP BY user_id
		),
		totals AS (
			SELECT
				COALESCE(SUM(total_tokens), 0) AS total_all_tokens,
				COALESCE(SUM(total_tokens) FILTER (WHERE segment IN ('offday', 'after_hours')), 0) AS total_nonwork_tokens
			FROM usage_nonwork_daily_user_stats st
			WHERE bucket_date >= $1::date
			  AND bucket_date <= $2::date
			  AND timezone = $3
			  AND EXISTS (SELECT 1 FROM filtered_users fu WHERE fu.id = st.user_id)
		),
		external_stats AS (
			SELECT
				s.user_id,
				COALESCE(SUM(s.requests), 0) AS requests,
				COALESCE(SUM(s.total_tokens), 0) AS tokens,
				COALESCE(SUM(s.nonwork_tokens), 0) AS nonwork_tokens,
				COALESCE(SUM(s.active_ms), 0) AS active_duration_ms,
				COALESCE(SUM(s.nonwork_active_ms), 0) AS nonwork_active_ms,
				COALESCE(SUM(s.actual_cost), 0) AS actual_cost
			FROM external_usage_daily_user_stats s
			JOIN external_usage_import_batches b ON b.id = s.batch_id AND b.status = 'imported'
			WHERE s.bucket_date >= $1::date
			  AND s.bucket_date <= $2::date
			  AND EXISTS (SELECT 1 FROM filtered_users fu WHERE fu.id = s.user_id)
			GROUP BY s.user_id
		),
		external_totals AS (
			SELECT
				COALESCE(SUM(s.total_tokens), 0) AS total_all_tokens,
				COALESCE(SUM(s.nonwork_tokens), 0) AS total_nonwork_tokens
			FROM external_usage_daily_user_stats s
			JOIN external_usage_import_batches b ON b.id = s.batch_id AND b.status = 'imported'
			WHERE s.bucket_date >= $1::date
			  AND s.bucket_date <= $2::date
			  AND EXISTS (SELECT 1 FROM filtered_users fu WHERE fu.id = s.user_id)
		),
		metric AS (
			SELECT
				u.id AS user_id,
				COALESCE(u.email, '') AS email,
				COALESCE(u.username, '') AS username,
				COALESCE(s.actual_cost, 0) + COALESCE(e.actual_cost, 0) AS actual_cost,
				COALESCE(s.requests, 0) + COALESCE(e.requests, 0) AS requests,
				COALESCE(s.tokens, 0) + CASE WHEN $9 = 'nonwork' THEN COALESCE(e.nonwork_tokens, 0) ELSE COALESCE(e.tokens, 0) END AS tokens,
				COALESCE(s.nonwork_tokens, 0) + COALESCE(e.nonwork_tokens, 0) AS nonwork_tokens,
				COALESCE(s.active_duration_ms, 0) + CASE WHEN $9 = 'nonwork' THEN COALESCE(e.nonwork_active_ms, 0) ELSE COALESCE(e.active_duration_ms, 0) END AS active_duration_ms,
				COALESCE(s.nonwork_active_ms, 0) + COALESCE(e.nonwork_active_ms, 0) AS nonwork_active_ms,
				COALESCE(s.calendar_confirmed, TRUE) AS calendar_confirmed
			FROM filtered_users u
			LEFT JOIN stats s ON s.user_id = u.id
			LEFT JOIN external_stats e ON e.user_id = u.id
		),
		ranked AS (
			SELECT
				m.user_id,
				m.email,
				m.username,
				m.actual_cost,
				m.requests,
				m.tokens,
				m.nonwork_tokens,
				m.active_duration_ms,
				m.nonwork_active_ms,
				m.calendar_confirmed,
				COALESCE(SUM(m.actual_cost) OVER (), 0) AS total_actual_cost,
				COALESCE(SUM(m.requests) OVER (), 0) AS total_requests,
				COALESCE(SUM(m.tokens) OVER (), 0) AS total_tokens,
				COALESCE(SUM(m.nonwork_tokens) OVER (), 0) AS total_nonwork_tokens,
				t.total_all_tokens + et.total_all_tokens AS total_all_tokens,
				CASE WHEN t.total_all_tokens + et.total_all_tokens > 0
					THEN (t.total_nonwork_tokens + et.total_nonwork_tokens)::double precision / (t.total_all_tokens + et.total_all_tokens)::double precision
					ELSE 0
				END AS nonwork_token_ratio,
				COALESCE(SUM(m.active_duration_ms) OVER (), 0) AS total_active_duration_ms,
				COALESCE(BOOL_AND(m.calendar_confirmed) OVER (), TRUE) AS all_calendar_confirmed
			FROM metric m
			CROSS JOIN totals t
			CROSS JOIN external_totals et
			ORDER BY %s %s, tokens %s, active_duration_ms %s, user_id ASC
			LIMIT $8
		)
		SELECT
			user_id,
			email,
			username,
			actual_cost,
			requests,
			tokens,
			nonwork_tokens,
			active_duration_ms,
			nonwork_active_ms,
			calendar_confirmed,
			total_actual_cost,
			total_requests,
			total_tokens,
			total_nonwork_tokens,
			total_all_tokens,
			nonwork_token_ratio,
			total_active_duration_ms,
			all_calendar_confirmed
		FROM ranked
		ORDER BY %s %s, tokens %s, active_duration_ms %s, user_id ASC
	`, innerOrderExpr, innerDirection, innerDirection, innerDirection, outerOrderExpr, outerDirection, outerDirection, outerDirection)

	rows, err := r.sql.QueryContext(ctx, query, startDate, endDate, tz, pq.Array(segments), service.RoleAdmin, pq.Array(compactStrings(externalOrganizationIDs)), username, limit, strings.TrimSpace(scope))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	ranking := make([]UserNonworkTokenRankingItem, 0)
	var totalActualCost, nonworkTokenRatio float64
	var totalRequests, totalTokens, totalNonworkTokens, totalAllTokens, totalActiveDurationMs int64
	calendarConfirmed := true
	for rows.Next() {
		var row UserNonworkTokenRankingItem
		if err = rows.Scan(
			&row.UserID,
			&row.Email,
			&row.Username,
			&row.ActualCost,
			&row.Requests,
			&row.Tokens,
			&row.NonworkTokens,
			&row.ActiveDurationMs,
			&row.NonworkActiveMs,
			&row.CalendarConfirmed,
			&totalActualCost,
			&totalRequests,
			&totalTokens,
			&totalNonworkTokens,
			&totalAllTokens,
			&nonworkTokenRatio,
			&totalActiveDurationMs,
			&calendarConfirmed,
		); err != nil {
			return nil, err
		}
		ranking = append(ranking, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &UserNonworkTokenRankingResponse{
		Ranking:               ranking,
		TotalActualCost:       totalActualCost,
		TotalRequests:         totalRequests,
		TotalTokens:           totalTokens,
		TotalNonworkTokens:    totalNonworkTokens,
		TotalAllTokens:        totalAllTokens,
		NonworkTokenRatio:     nonworkTokenRatio,
		TotalActiveDurationMs: totalActiveDurationMs,
		CalendarConfirmed:     calendarConfirmed,
		StatsCoverage:         coverage,
		StatsComplete:         coverage.Complete,
	}, nil
}

func compactStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (r *usageLogRepository) GetNonworkStatsCoverage(ctx context.Context, startDate, endDate time.Time, tz string) (result usagestats.NonworkStatsCoverage, err error) {
	if r == nil || r.sql == nil || endDate.Before(startDate) {
		return usagestats.NonworkStatsCoverage{}, nil
	}
	if strings.TrimSpace(tz) == "" {
		tz = "Asia/Shanghai"
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH days AS (
			SELECT generate_series($1::date, $2::date, interval '1 day')::date AS bucket_date
		),
		marked AS (
			SELECT bucket_date, computed_at
			FROM usage_nonwork_daily_stat_runs
			WHERE timezone = $3
			  AND bucket_date >= $1::date
			  AND bucket_date <= $2::date
		)
		SELECT d.bucket_date::text, (m.bucket_date IS NOT NULL) AS aggregated, m.computed_at
		FROM days d
		LEFT JOIN marked m ON m.bucket_date = d.bucket_date
		ORDER BY d.bucket_date ASC
	`, startDate, endDate, tz)
	if err != nil {
		return result, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	result = usagestats.NonworkStatsCoverage{
		StartDate:     startDate.Format("2006-01-02"),
		EndDate:       endDate.Format("2006-01-02"),
		Timezone:      tz,
		MissingRanges: make([]usagestats.NonworkMissingDateRange, 0),
		Complete:      true,
	}
	var openMissingStart string
	var lastMissing string
	var lastComputedAt time.Time
	for rows.Next() {
		var date string
		var aggregated bool
		var computedAt sql.NullTime
		if err = rows.Scan(&date, &aggregated, &computedAt); err != nil {
			return result, err
		}
		result.TotalDays++
		if aggregated {
			result.AggregatedDays++
			if computedAt.Valid && computedAt.Time.After(lastComputedAt) {
				lastComputedAt = computedAt.Time
			}
			if openMissingStart != "" {
				result.MissingRanges = append(result.MissingRanges, usagestats.NonworkMissingDateRange{
					StartDate: openMissingStart,
					EndDate:   lastMissing,
				})
				openMissingStart = ""
				lastMissing = ""
			}
			continue
		}
		result.MissingDays++
		result.Complete = false
		if openMissingStart == "" {
			openMissingStart = date
		}
		lastMissing = date
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if openMissingStart != "" {
		result.MissingRanges = append(result.MissingRanges, usagestats.NonworkMissingDateRange{
			StartDate: openMissingStart,
			EndDate:   lastMissing,
		})
	}
	if !lastComputedAt.IsZero() {
		result.LastComputedAt = &lastComputedAt
	}
	return result, nil
}

func nonworkRankingSegments(scope string) []string {
	switch strings.TrimSpace(scope) {
	case usagestats.NonworkRankingScopeAll:
		return []string{"work_hours", "after_hours", "offday"}
	default:
		return []string{"after_hours", "offday"}
	}
}

func nonworkRankingOrderExprs(rankBy string) (string, string) {
	switch strings.TrimSpace(rankBy) {
	case usagestats.NonworkRankingRankByRequests:
		return "requests", "requests"
	case usagestats.NonworkRankingRankByActiveDuration:
		return "active_duration_ms", "active_duration_ms"
	case usagestats.NonworkRankingRankByNonworkActive:
		return "nonwork_active_ms", "nonwork_active_ms"
	case usagestats.NonworkRankingRankByActualCost:
		return "actual_cost", "actual_cost"
	case usagestats.NonworkRankingRankByNonworkTokens:
		return "nonwork_tokens", "nonwork_tokens"
	default:
		return "tokens", "tokens"
	}
}

func nonworkRankingDirections(sortOrder string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(sortOrder)) {
	case "desc":
		return "ASC", "ASC"
	default:
		return "DESC", "DESC"
	}
}

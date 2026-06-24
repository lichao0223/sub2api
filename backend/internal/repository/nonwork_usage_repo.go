package repository

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/nonworktime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type nonworkUsageRepository struct {
	sql sqlExecutor
}

// NewNonworkUsageRepository creates the repository backing non-work usage analytics.
func NewNonworkUsageRepository(sqlDB *sql.DB) service.NonworkUsageRepository {
	if sqlDB == nil {
		return nil
	}
	if !isPostgresDriver(sqlDB) {
		log.Printf("[NonworkUsage] 检测到非 PostgreSQL 驱动，已自动禁用非工作时段聚合")
		return nil
	}
	return newNonworkUsageRepositoryWithSQL(sqlDB)
}

func newNonworkUsageRepositoryWithSQL(sqlq sqlExecutor) *nonworkUsageRepository {
	return &nonworkUsageRepository{sql: sqlq}
}

func (r *nonworkUsageRepository) UpsertCalendarDays(ctx context.Context, days []nonworktime.CalendarDay) (int, int, error) {
	if r == nil || r.sql == nil || len(days) == 0 {
		return 0, 0, nil
	}
	inserted := 0
	updated := 0
	for _, day := range days {
		raw := any(nil)
		if len(day.Raw) > 0 {
			raw = string(day.Raw)
		}
		var wasInsert bool
		err := scanSingleRow(ctx, r.sql, `
			WITH upserted AS (
				INSERT INTO calendar_days (
					date, country, is_workday, is_offday, is_weekend, day_type,
					holiday_name, source, source_version, confirmed, manual_override, raw, updated_at
				)
				VALUES ($1::date, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''), $10, $11, $12::jsonb, NOW())
				ON CONFLICT (country, date)
				DO UPDATE SET
					is_workday = EXCLUDED.is_workday,
					is_offday = EXCLUDED.is_offday,
					is_weekend = EXCLUDED.is_weekend,
					day_type = EXCLUDED.day_type,
					holiday_name = EXCLUDED.holiday_name,
					source = EXCLUDED.source,
					source_version = EXCLUDED.source_version,
					confirmed = EXCLUDED.confirmed,
					raw = EXCLUDED.raw,
					updated_at = NOW()
				WHERE calendar_days.manual_override = FALSE
				  AND (EXCLUDED.confirmed = TRUE OR calendar_days.confirmed = FALSE)
				  AND (
					calendar_days.is_workday IS DISTINCT FROM EXCLUDED.is_workday OR
					calendar_days.is_offday IS DISTINCT FROM EXCLUDED.is_offday OR
					calendar_days.is_weekend IS DISTINCT FROM EXCLUDED.is_weekend OR
					calendar_days.day_type IS DISTINCT FROM EXCLUDED.day_type OR
					calendar_days.holiday_name IS DISTINCT FROM EXCLUDED.holiday_name OR
					calendar_days.source IS DISTINCT FROM EXCLUDED.source OR
					calendar_days.source_version IS DISTINCT FROM EXCLUDED.source_version OR
					calendar_days.confirmed IS DISTINCT FROM EXCLUDED.confirmed OR
					calendar_days.raw IS DISTINCT FROM EXCLUDED.raw
				  )
				RETURNING (xmax = 0) AS was_insert
			)
			SELECT was_insert FROM upserted
		`, []any{day.Date, day.Country, day.IsWorkday, day.IsOffday, day.IsWeekend, day.DayType, day.HolidayName,
			day.Source, day.SourceVersion, day.Confirmed, day.ManualOverride, raw}, &wasInsert)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return inserted, updated, err
		}
		if wasInsert {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, nil
}

func (r *nonworkUsageRepository) RecordCalendarSyncRun(ctx context.Context, run service.CalendarSyncRun) error {
	if r == nil || r.sql == nil {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO calendar_sync_runs (
			country, year, source, source_url, source_version, status,
			days_inserted, days_updated, error_message, started_at, finished_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, NULLIF($9, ''), $10, $11)
	`, run.Country, run.Year, run.Source, run.SourceURL, run.SourceVersion, run.Status, run.DaysInserted, run.DaysUpdated, run.ErrorMessage, run.StartedAt, run.FinishedAt)
	return err
}

func (r *nonworkUsageRepository) GetCalendarStatus(ctx context.Context, country string, years []int) ([]service.CalendarYearStatus, error) {
	if r == nil || r.sql == nil || len(years) == 0 {
		return nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		WITH requested_years AS (
			SELECT unnest($2::int[]) AS year
		),
		day_stats AS (
			SELECT
				EXTRACT(YEAR FROM date)::int AS year,
				country,
				COUNT(*)::int AS total_days,
				COUNT(*) FILTER (WHERE confirmed)::int AS confirmed_days,
				COUNT(*) FILTER (WHERE manual_override)::int AS manual_overrides,
				MIN(date)::text AS first_date,
				MAX(date)::text AS last_date,
				BOOL_AND(confirmed) AS confirmed
			FROM calendar_days
			WHERE country = $1
			  AND EXTRACT(YEAR FROM date)::int = ANY($2::int[])
			GROUP BY 1, 2
		),
		latest_sync AS (
			SELECT DISTINCT ON (year)
				year,
				status,
				finished_at,
				source,
				source_version
			FROM calendar_sync_runs
			WHERE country = $1
			  AND year = ANY($2::int[])
			ORDER BY year, started_at DESC
		)
		SELECT
			ry.year,
			COALESCE(ds.country, $1) AS country,
			COALESCE(ds.total_days, 0) AS total_days,
			COALESCE(ds.confirmed_days, 0) AS confirmed_days,
			COALESCE(ds.manual_overrides, 0) AS manual_overrides,
			COALESCE(ds.first_date, '') AS first_date,
			COALESCE(ds.last_date, '') AS last_date,
			COALESCE(ls.status, '') AS last_sync_status,
			COALESCE(ls.finished_at, 'epoch'::timestamptz) AS last_sync_at,
			COALESCE(ls.source, '') AS last_source,
			COALESCE(ls.source_version, '') AS last_source_version,
			COALESCE(ds.confirmed, false) AS confirmed
		FROM requested_years ry
		LEFT JOIN day_stats ds ON ds.year = ry.year
		LEFT JOIN latest_sync ls ON ls.year = ry.year
		ORDER BY ry.year ASC
	`, country, pq.Array(years))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]service.CalendarYearStatus, 0, len(years))
	for rows.Next() {
		var item service.CalendarYearStatus
		if err := rows.Scan(
			&item.Year,
			&item.Country,
			&item.TotalDays,
			&item.ConfirmedDays,
			&item.ManualOverrides,
			&item.FirstDate,
			&item.LastDate,
			&item.LastSyncStatus,
			&item.LastSyncAt,
			&item.LastSource,
			&item.LastSourceVersion,
			&item.Confirmed,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *nonworkUsageRepository) GetCalendarDays(ctx context.Context, country string, startDate, endDate time.Time) ([]nonworktime.CalendarDay, error) {
	if r == nil || r.sql == nil {
		return nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT date, country, is_workday, is_offday, is_weekend, day_type,
		       COALESCE(holiday_name, ''), source, COALESCE(source_version, ''),
		       confirmed, manual_override, COALESCE(raw::text, '')
		FROM calendar_days
		WHERE country = $1
		  AND date >= $2::date
		  AND date <= $3::date
		ORDER BY date ASC
	`, country, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []nonworktime.CalendarDay
	for rows.Next() {
		var day nonworktime.CalendarDay
		var raw string
		if err := rows.Scan(
			&day.Date,
			&day.Country,
			&day.IsWorkday,
			&day.IsOffday,
			&day.IsWeekend,
			&day.DayType,
			&day.HolidayName,
			&day.Source,
			&day.SourceVersion,
			&day.Confirmed,
			&day.ManualOverride,
			&raw,
		); err != nil {
			return nil, err
		}
		if raw != "" {
			day.Raw = []byte(raw)
		}
		out = append(out, day)
	}
	return out, rows.Err()
}

func (r *nonworkUsageRepository) UpsertManualCalendarDay(ctx context.Context, day nonworktime.CalendarDay) error {
	if r == nil || r.sql == nil {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO calendar_days (
			date, country, is_workday, is_offday, is_weekend, day_type,
			holiday_name, source, source_version, confirmed, manual_override, raw, updated_at
		)
		VALUES ($1::date, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''), $10, TRUE, NULL, NOW())
		ON CONFLICT (country, date)
		DO UPDATE SET
			is_workday = EXCLUDED.is_workday,
			is_offday = EXCLUDED.is_offday,
			is_weekend = EXCLUDED.is_weekend,
			day_type = EXCLUDED.day_type,
			holiday_name = EXCLUDED.holiday_name,
			source = EXCLUDED.source,
			source_version = EXCLUDED.source_version,
			confirmed = EXCLUDED.confirmed,
			manual_override = TRUE,
			raw = NULL,
			updated_at = NOW()
	`, day.Date, day.Country, day.IsWorkday, day.IsOffday, day.IsWeekend, day.DayType, day.HolidayName, day.Source, day.SourceVersion, day.Confirmed)
	return err
}

func (r *nonworkUsageRepository) GetUsageEvents(ctx context.Context, start, end time.Time) ([]service.NonworkUsageEvent, error) {
	if r == nil || r.sql == nil {
		return nil, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			user_id,
			COALESCE(request_id, ''),
			created_at,
			COALESCE(input_tokens, 0),
			COALESCE(output_tokens, 0),
			COALESCE(cache_creation_tokens, 0),
			COALESCE(cache_read_tokens, 0),
			COALESCE(input_tokens, 0) + COALESCE(output_tokens, 0) + COALESCE(cache_creation_tokens, 0) + COALESCE(cache_read_tokens, 0) AS total_tokens,
			COALESCE(actual_cost, 0)
		FROM usage_logs
		WHERE created_at >= $1
		  AND created_at < $2
		  AND user_id IS NOT NULL
		ORDER BY user_id ASC, created_at ASC, request_id ASC
	`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []service.NonworkUsageEvent
	for rows.Next() {
		var ev service.NonworkUsageEvent
		if err := rows.Scan(
			&ev.UserID,
			&ev.RequestID,
			&ev.CreatedAt,
			&ev.InputTokens,
			&ev.OutputTokens,
			&ev.CacheCreationTokens,
			&ev.CacheReadTokens,
			&ev.TotalTokens,
			&ev.ActualCost,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (r *nonworkUsageRepository) CleanupDailyUserStats(ctx context.Context, cutoffDate time.Time, tz string) error {
	if r == nil || r.sql == nil {
		return nil
	}
	_, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_nonwork_daily_user_stats
		WHERE bucket_date < $1::date
		  AND timezone = $2
	`, cutoffDate, tz)
	return err
}

func (r *nonworkUsageRepository) ReplaceDailyUserStats(ctx context.Context, startDate, endDate time.Time, tz string, rows []service.NonworkDailyUserStat) error {
	if r == nil || r.sql == nil {
		return nil
	}
	if db, ok := r.sql.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		txRepo := newNonworkUsageRepositoryWithSQL(tx)
		if err := txRepo.replaceDailyUserStatsInTx(ctx, startDate, endDate, tz, rows); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	return r.replaceDailyUserStatsInTx(ctx, startDate, endDate, tz, rows)
}

func (r *nonworkUsageRepository) replaceDailyUserStatsInTx(ctx context.Context, startDate, endDate time.Time, tz string, rows []service.NonworkDailyUserStat) error {
	if _, err := r.sql.ExecContext(ctx, `
		DELETE FROM usage_nonwork_daily_user_stats
		WHERE bucket_date >= $1::date
		  AND bucket_date <= $2::date
		  AND timezone = $3
	`, startDate, endDate, tz); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := r.sql.ExecContext(ctx, `
			INSERT INTO usage_nonwork_daily_user_stats (
				bucket_date, timezone, user_id, segment, requests,
				input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
				total_tokens, actual_cost, active_ms, active_sessions, calendar_confirmed, computed_at
			)
			VALUES ($1::date, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
			ON CONFLICT (bucket_date, timezone, user_id, segment)
			DO UPDATE SET
				requests = EXCLUDED.requests,
				input_tokens = EXCLUDED.input_tokens,
				output_tokens = EXCLUDED.output_tokens,
				cache_creation_tokens = EXCLUDED.cache_creation_tokens,
				cache_read_tokens = EXCLUDED.cache_read_tokens,
				total_tokens = EXCLUDED.total_tokens,
				actual_cost = EXCLUDED.actual_cost,
				active_ms = EXCLUDED.active_ms,
				active_sessions = EXCLUDED.active_sessions,
				calendar_confirmed = EXCLUDED.calendar_confirmed,
				computed_at = EXCLUDED.computed_at
		`, row.BucketDate, row.Timezone, row.UserID, row.Segment, row.Requests,
			row.InputTokens, row.OutputTokens, row.CacheCreationTokens, row.CacheReadTokens,
			row.TotalTokens, row.ActualCost, row.ActiveMs, row.ActiveSessions, row.CalendarConfirmed); err != nil {
			return err
		}
	}
	return nil
}

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const externalUsageDateLayout = "2006-01-02"

func checkedRowsClose(rows *sql.Rows, errp *error) {
	if closeErr := rows.Close(); closeErr != nil && errp != nil && *errp == nil {
		*errp = closeErr
	}
}

type externalUsageMatchedRow struct {
	row      usagestats.ExternalUsageImportRow
	date     time.Time
	userID   int64
	email    string
	username string
	status   string
}

type externalUsageUserMatch struct {
	id       int64
	email    string
	username string
}

func (r *usageLogRepository) PreviewExternalUsageImport(ctx context.Context, input usagestats.ExternalUsageImportInput) (*usagestats.ExternalUsageImportPreview, error) {
	preview, _, err := r.previewExternalUsageImport(ctx, input)
	return preview, err
}

func (r *usageLogRepository) ImportExternalUsage(ctx context.Context, input usagestats.ExternalUsageImportInput) (result *usagestats.ExternalUsageImportResult, err error) {
	preview, rows, err := r.previewExternalUsageImport(ctx, input)
	if err != nil {
		return nil, err
	}
	if preview.Summary.InvalidRows > 0 || preview.Summary.ConflictRows > 0 {
		return nil, infraerrors.New(http.StatusBadRequest, "EXTERNAL_USAGE_IMPORT_INVALID", "external usage import contains invalid rows")
	}

	batchID, err := r.insertExternalUsageImportBatch(ctx, input, preview.Summary)
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		if _, err = r.sql.ExecContext(ctx, `
			INSERT INTO external_usage_daily_user_stats (
				batch_id, bucket_date, user_id, username_snapshot, requests,
				input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
				total_tokens, actual_cost, active_ms, nonwork_tokens, nonwork_active_ms,
				raw_row_number, raw_username, note, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9,
				$10, $11, $12, $13, $14,
				$15, $16, $17, NOW()
			)
			ON CONFLICT (bucket_date, user_id) DO UPDATE SET
				batch_id = EXCLUDED.batch_id,
				username_snapshot = EXCLUDED.username_snapshot,
				requests = EXCLUDED.requests,
				input_tokens = EXCLUDED.input_tokens,
				output_tokens = EXCLUDED.output_tokens,
				cache_creation_tokens = EXCLUDED.cache_creation_tokens,
				cache_read_tokens = EXCLUDED.cache_read_tokens,
				total_tokens = EXCLUDED.total_tokens,
				actual_cost = EXCLUDED.actual_cost,
				active_ms = EXCLUDED.active_ms,
				nonwork_tokens = EXCLUDED.nonwork_tokens,
				nonwork_active_ms = EXCLUDED.nonwork_active_ms,
				raw_row_number = EXCLUDED.raw_row_number,
				raw_username = EXCLUDED.raw_username,
				note = EXCLUDED.note,
				updated_at = NOW()
		`, batchID, row.date, row.userID, row.username, row.row.Requests,
			row.row.InputTokens, row.row.OutputTokens, row.row.CacheCreationTokens, row.row.CacheReadTokens,
			row.row.TotalTokens, row.row.ActualCost, row.row.ActiveDurationMs, row.row.NonworkTokens, row.row.NonworkActiveMs,
			row.row.RowNumber, row.row.Username, row.row.Note); err != nil {
			return nil, err
		}
	}

	preview.Summary.ImportedRows = len(rows)
	if _, err = r.sql.ExecContext(ctx, `
		UPDATE external_usage_import_batches
		SET status = 'imported', imported_rows = $2, imported_at = NOW()
		WHERE id = $1
	`, batchID, len(rows)); err != nil {
		return nil, err
	}

	return &usagestats.ExternalUsageImportResult{
		BatchID:                    batchID,
		ExternalUsageImportPreview: *preview,
	}, nil
}

func (r *usageLogRepository) ListExternalUsageImportBatches(ctx context.Context, params pagination.PaginationParams) (result []usagestats.ExternalUsageImportBatch, page *pagination.PaginationResult, err error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}

	var total int64
	countRows, err := r.sql.QueryContext(ctx, `SELECT COUNT(*) FROM external_usage_import_batches`)
	if err != nil {
		return nil, nil, err
	}
	if countRows.Next() {
		if err = countRows.Scan(&total); err != nil {
			if closeErr := countRows.Close(); closeErr != nil {
				return nil, nil, closeErr
			}
			return nil, nil, err
		}
	}
	if closeErr := countRows.Close(); closeErr != nil {
		return nil, nil, closeErr
	}

	offset := (params.Page - 1) * params.PageSize
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, file_name, file_sha256, status, total_rows, matched_rows, unmatched_rows,
			conflict_rows, invalid_rows, overwritten_rows, imported_rows, created_by,
			created_at, imported_at, voided_at, voided_by, note
		FROM external_usage_import_batches
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, params.PageSize, offset)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
			page = nil
		}
	}()

	result = make([]usagestats.ExternalUsageImportBatch, 0)
	for rows.Next() {
		var item usagestats.ExternalUsageImportBatch
		if err = rows.Scan(
			&item.ID, &item.FileName, &item.FileSHA256, &item.Status, &item.TotalRows, &item.MatchedRows, &item.UnmatchedRows,
			&item.ConflictRows, &item.InvalidRows, &item.OverwrittenRows, &item.ImportedRows, &item.CreatedBy,
			&item.CreatedAt, &item.ImportedAt, &item.VoidedAt, &item.VoidedBy, &item.Note,
		); err != nil {
			return nil, nil, err
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}

	return result, &pagination.PaginationResult{
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
		Pages:    int((total + int64(params.PageSize) - 1) / int64(params.PageSize)),
	}, nil
}

func (r *usageLogRepository) VoidExternalUsageImportBatch(ctx context.Context, batchID, voidedBy int64) error {
	if _, err := r.sql.ExecContext(ctx, `
		UPDATE external_usage_import_batches
		SET status = 'voided', voided_at = NOW(), voided_by = $2
		WHERE id = $1 AND status = 'imported'
	`, batchID, voidedBy); err != nil {
		return err
	}
	_, err := r.sql.ExecContext(ctx, `DELETE FROM external_usage_daily_user_stats WHERE batch_id = $1`, batchID)
	return err
}

func (r *usageLogRepository) ExportExternalUsageRows(ctx context.Context, startDate, endDate time.Time, includeNonwork bool) (result []usagestats.ExternalUsageImportRow, err error) {
	rows, err := r.sql.QueryContext(ctx, `
		WITH native_stats AS (
			SELECT
				(ul.created_at AT TIME ZONE 'Asia/Shanghai')::date AS bucket_date,
				ul.user_id,
				COUNT(*)::bigint AS requests,
				COALESCE(SUM(ul.input_tokens), 0)::bigint AS input_tokens,
				COALESCE(SUM(ul.output_tokens), 0)::bigint AS output_tokens,
				COALESCE(SUM(ul.cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
				COALESCE(SUM(ul.cache_read_tokens), 0)::bigint AS cache_read_tokens,
				COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0)::bigint AS total_tokens,
				COALESCE(SUM(ul.actual_cost), 0) AS actual_cost,
				COALESCE(SUM(ul.duration_ms), 0)::bigint AS response_duration_ms
			FROM usage_logs ul
			WHERE (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date >= $1::date
			  AND (ul.created_at AT TIME ZONE 'Asia/Shanghai')::date <= $2::date
			GROUP BY bucket_date, ul.user_id
		),
		nonwork_stats AS (
			SELECT
				user_id,
				bucket_date,
				COALESCE(SUM(active_ms), 0)::bigint AS active_ms,
				COALESCE(SUM(total_tokens) FILTER (WHERE segment IN ('offday', 'after_hours')), 0)::bigint AS nonwork_tokens,
				COALESCE(SUM(active_ms) FILTER (WHERE segment IN ('offday', 'after_hours')), 0)::bigint AS nonwork_active_ms
			FROM usage_nonwork_daily_user_stats
			WHERE bucket_date >= $1::date AND bucket_date <= $2::date AND timezone = 'Asia/Shanghai'
			GROUP BY user_id, bucket_date
		)
		SELECT
			ns.bucket_date::text,
			COALESCE(u.username, '') AS username,
			ns.requests,
			ns.total_tokens,
			ns.input_tokens,
			ns.output_tokens,
			ns.cache_creation_tokens,
			ns.cache_read_tokens,
			ns.actual_cost,
			COALESCE(nws.active_ms, ns.response_duration_ms) AS active_ms,
			CASE WHEN $3 THEN LEAST(COALESCE(nws.nonwork_tokens, 0), ns.total_tokens) ELSE 0 END AS nonwork_tokens,
			CASE WHEN $3 THEN LEAST(COALESCE(nws.nonwork_active_ms, 0), COALESCE(nws.active_ms, ns.response_duration_ms)) ELSE 0 END AS nonwork_active_ms
		FROM native_stats ns
		JOIN users u ON u.id = ns.user_id
		LEFT JOIN nonwork_stats nws ON nws.user_id = ns.user_id AND nws.bucket_date = ns.bucket_date
		WHERE u.deleted_at IS NULL AND u.role <> $4
		ORDER BY ns.bucket_date ASC, u.username ASC, ns.user_id ASC
	`, startDate, endDate, includeNonwork, service.RoleAdmin)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	result = make([]usagestats.ExternalUsageImportRow, 0)
	rowNumber := 2
	for rows.Next() {
		var row usagestats.ExternalUsageImportRow
		if err = rows.Scan(&row.Date, &row.Username, &row.Requests, &row.TotalTokens, &row.InputTokens, &row.OutputTokens, &row.CacheCreationTokens, &row.CacheReadTokens, &row.ActualCost, &row.ActiveDurationMs, &row.NonworkTokens, &row.NonworkActiveMs); err != nil {
			return nil, err
		}
		row.RowNumber = rowNumber
		result = append(result, row)
		rowNumber++
	}
	return result, rows.Err()
}

func (r *usageLogRepository) previewExternalUsageImport(ctx context.Context, input usagestats.ExternalUsageImportInput) (*usagestats.ExternalUsageImportPreview, []externalUsageMatchedRow, error) {
	rowsForPreview := input.Rows
	if !externalUsageRowsHaveValidationErrors(input.Rows) {
		rowsForPreview = aggregateExternalUsageRows(input.Rows)
	}
	preview := &usagestats.ExternalUsageImportPreview{
		FileSHA256: input.FileSHA256,
		Rows:       make([]usagestats.ExternalUsageImportPreviewRow, 0, len(rowsForPreview)),
	}
	preview.Summary.TotalRows = len(input.Rows)

	usernames := make([]string, 0, len(rowsForPreview))
	seenUsername := map[string]bool{}
	for _, row := range rowsForPreview {
		username := strings.TrimSpace(row.Username)
		if username != "" && !seenUsername[username] {
			seenUsername[username] = true
			usernames = append(usernames, username)
		}
	}
	matches, err := r.loadExternalUsageUserMatches(ctx, usernames)
	if err != nil {
		return nil, nil, err
	}

	matched := make([]externalUsageMatchedRow, 0, len(input.Rows))
	for index, raw := range rowsForPreview {
		row := raw
		if row.RowNumber == 0 {
			row.RowNumber = index + 2
		}
		row.Date = strings.TrimSpace(row.Date)
		row.Username = strings.TrimSpace(row.Username)
		previewRow := usagestats.ExternalUsageImportPreviewRow{ExternalUsageImportRow: row}

		parsedDate, errorsForRow := validateExternalUsageRow(row)

		userMatches := matches[row.Username]
		switch {
		case len(errorsForRow) > 0:
			previewRow.Status = "invalid"
			previewRow.Errors = errorsForRow
			preview.Summary.InvalidRows++
		case len(userMatches) == 0:
			previewRow.Status = "unmatched"
			previewRow.Errors = []usagestats.ExternalUsageImportRowError{{Field: "username", Code: "USER_NOT_FOUND", Message: "未找到同名用户"}}
			preview.Summary.UnmatchedRows++
		case len(userMatches) > 1:
			previewRow.Status = "conflict"
			previewRow.Errors = []usagestats.ExternalUsageImportRowError{{Field: "username", Code: "USER_NOT_UNIQUE", Message: "存在多个同名用户"}}
			preview.Summary.ConflictRows++
		default:
			match := userMatches[0]
			previewRow.Status = "matched"
			previewRow.MatchedUserID = match.id
			previewRow.MatchedEmail = match.email
			previewRow.MatchedName = match.username
			preview.Summary.MatchedRows++
			matched = append(matched, externalUsageMatchedRow{row: row, date: parsedDate, userID: match.id, email: match.email, username: match.username, status: "matched"})
		}
		preview.Rows = append(preview.Rows, previewRow)
	}

	existing, err := r.loadExistingExternalUsageRows(ctx, matched)
	if err != nil {
		return nil, nil, err
	}
	for i := range preview.Rows {
		row := &preview.Rows[i]
		if row.Status != "matched" {
			continue
		}
		key := row.Date + "\x00" + fmt.Sprint(row.MatchedUserID)
		if existing[key] {
			row.Status = "overwrite"
			preview.Summary.OverwrittenRows++
			preview.Summary.MatchedRows--
		}
	}
	for i := range matched {
		key := matched[i].row.Date + "\x00" + fmt.Sprint(matched[i].userID)
		if existing[key] {
			matched[i].status = "overwrite"
		}
	}

	return preview, matched, nil
}

func externalUsageRowsHaveValidationErrors(rows []usagestats.ExternalUsageImportRow) bool {
	for _, raw := range rows {
		row := raw
		row.Date = strings.TrimSpace(row.Date)
		row.Username = strings.TrimSpace(row.Username)
		_, errs := validateExternalUsageRow(row)
		if len(errs) > 0 {
			return true
		}
	}
	return false
}

func aggregateExternalUsageRows(rows []usagestats.ExternalUsageImportRow) []usagestats.ExternalUsageImportRow {
	out := make([]usagestats.ExternalUsageImportRow, 0, len(rows))
	indexByKey := make(map[string]int, len(rows))
	for _, raw := range rows {
		row := raw
		row.Date = strings.TrimSpace(row.Date)
		row.Username = strings.TrimSpace(row.Username)
		if row.Date == "" || row.Username == "" {
			out = append(out, row)
			continue
		}
		key := row.Date + "\x00" + row.Username
		if idx, ok := indexByKey[key]; ok {
			current := &out[idx]
			current.Requests += row.Requests
			current.TotalTokens += row.TotalTokens
			current.InputTokens += row.InputTokens
			current.OutputTokens += row.OutputTokens
			current.CacheCreationTokens += row.CacheCreationTokens
			current.CacheReadTokens += row.CacheReadTokens
			current.ActualCost += row.ActualCost
			current.ActiveDurationMs += row.ActiveDurationMs
			current.NonworkTokens += row.NonworkTokens
			current.NonworkActiveMs += row.NonworkActiveMs
			if strings.TrimSpace(row.Note) != "" {
				if strings.TrimSpace(current.Note) != "" {
					current.Note += "；"
				}
				current.Note += strings.TrimSpace(row.Note)
			}
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, row)
	}
	return out
}

func validateExternalUsageRow(row usagestats.ExternalUsageImportRow) (time.Time, []usagestats.ExternalUsageImportRowError) {
	errs := make([]usagestats.ExternalUsageImportRowError, 0)
	var parsed time.Time
	if row.Date == "" {
		errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "date", Code: "REQUIRED", Message: "日期必填"})
	} else {
		var err error
		parsed, err = time.ParseInLocation(externalUsageDateLayout, row.Date, time.Local)
		if err != nil {
			errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "date", Code: "INVALID_DATE", Message: "日期格式应为 YYYY-MM-DD"})
		}
	}
	if row.Username == "" {
		errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "username", Code: "REQUIRED", Message: "用户中文名必填"})
	}
	checkNonNegative := func(field string, value int64) {
		if value < 0 {
			errs = append(errs, usagestats.ExternalUsageImportRowError{Field: field, Code: "NEGATIVE", Message: "数值不能为负数"})
		}
	}
	checkNonNegative("requests", row.Requests)
	checkNonNegative("total_tokens", row.TotalTokens)
	checkNonNegative("input_tokens", row.InputTokens)
	checkNonNegative("output_tokens", row.OutputTokens)
	checkNonNegative("cache_creation_tokens", row.CacheCreationTokens)
	checkNonNegative("cache_read_tokens", row.CacheReadTokens)
	checkNonNegative("active_duration_ms", row.ActiveDurationMs)
	checkNonNegative("nonwork_tokens", row.NonworkTokens)
	checkNonNegative("nonwork_active_ms", row.NonworkActiveMs)
	if row.ActualCost < 0 {
		errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "actual_cost", Code: "NEGATIVE", Message: "费用不能为负数"})
	}
	if row.NonworkTokens > row.TotalTokens {
		errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "nonwork_tokens", Code: "OUT_OF_RANGE", Message: "非工作时间 Token 不能大于总 Token"})
	}
	if row.NonworkActiveMs > row.ActiveDurationMs && row.ActiveDurationMs > 0 {
		errs = append(errs, usagestats.ExternalUsageImportRowError{Field: "nonwork_active_ms", Code: "OUT_OF_RANGE", Message: "非工作时间活跃时长不能大于总活跃时长"})
	}
	return parsed, errs
}

func (r *usageLogRepository) loadExternalUsageUserMatches(ctx context.Context, usernames []string) (map[string][]externalUsageUserMatch, error) {
	out := make(map[string][]externalUsageUserMatch, len(usernames))
	if len(usernames) == 0 {
		return out, nil
	}
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, COALESCE(email, ''), COALESCE(username, '')
		FROM users
		WHERE deleted_at IS NULL
		  AND role <> $1
		  AND username = ANY($2::text[])
		ORDER BY id ASC
	`, service.RoleAdmin, pq.Array(usernames))
	if err != nil {
		return nil, err
	}
	defer checkedRowsClose(rows, &err)
	for rows.Next() {
		var item externalUsageUserMatch
		if err = rows.Scan(&item.id, &item.email, &item.username); err != nil {
			return nil, err
		}
		out[item.username] = append(out[item.username], item)
	}
	return out, rows.Err()
}

func (r *usageLogRepository) loadExistingExternalUsageRows(ctx context.Context, rowsToCheck []externalUsageMatchedRow) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(rowsToCheck) == 0 {
		return out, nil
	}
	values := make([]string, 0, len(rowsToCheck))
	args := make([]any, 0, len(rowsToCheck)*2)
	for i, row := range rowsToCheck {
		values = append(values, fmt.Sprintf("($%d::date, $%d::bigint)", i*2+1, i*2+2))
		args = append(args, row.date, row.userID)
	}
	query := `
		WITH incoming(bucket_date, user_id) AS (VALUES ` + strings.Join(values, ",") + `)
		SELECT s.bucket_date::text, s.user_id
		FROM external_usage_daily_user_stats s
		JOIN external_usage_import_batches b ON b.id = s.batch_id AND b.status = 'imported'
		JOIN incoming i ON i.bucket_date = s.bucket_date AND i.user_id = s.user_id
	`
	rows, err := r.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer checkedRowsClose(rows, &err)
	for rows.Next() {
		var date string
		var userID int64
		if err = rows.Scan(&date, &userID); err != nil {
			return nil, err
		}
		out[date+"\x00"+fmt.Sprint(userID)] = true
	}
	return out, rows.Err()
}

func (r *usageLogRepository) insertExternalUsageImportBatch(ctx context.Context, input usagestats.ExternalUsageImportInput, summary usagestats.ExternalUsageImportSummary) (int64, error) {
	rows, err := r.sql.QueryContext(ctx, `
		INSERT INTO external_usage_import_batches (
			file_name, file_sha256, status, total_rows, matched_rows, unmatched_rows,
			conflict_rows, invalid_rows, overwritten_rows, imported_rows, created_by, imported_at, note
		) VALUES (
			$1, $2, 'imported', $3, $4, $5,
			$6, $7, $8, 0, $9, NOW(), $10
		)
		RETURNING id
	`, input.FileName, input.FileSHA256, summary.TotalRows, summary.MatchedRows, summary.UnmatchedRows,
		summary.ConflictRows, summary.InvalidRows, summary.OverwrittenRows, input.CreatedBy, input.Note)
	if err != nil {
		return 0, err
	}
	defer checkedRowsClose(rows, &err)
	var id int64
	if rows.Next() {
		if err = rows.Scan(&id); err != nil {
			return 0, err
		}
	}
	if id == 0 {
		return 0, sql.ErrNoRows
	}
	return id, rows.Err()
}

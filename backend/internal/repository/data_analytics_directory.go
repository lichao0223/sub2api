package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type dataAnalyticsRepository struct{ db *sql.DB }

func NewDataAnalyticsRepository(db *sql.DB) service.DataAnalyticsRepository {
	return &dataAnalyticsRepository{db: db}
}

type analyticsDirectoryDB interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func analyticsIdentities(ctx context.Context, db analyticsDirectoryDB, directory service.AnalyticsDirectory) ([]service.AnalyticsIdentity, error) {
	rows, err := db.QueryContext(ctx, `SELECT u.id,COALESCE(u.username,''),COALESCE(i.person_id,e.external_user_id,''),
		CASE WHEN i.user_id IS NOT NULL THEN i.basis WHEN e.matches>1 THEN 'conflict' WHEN e.matches=1 THEN 'external_id' ELSE '' END
		FROM users u LEFT JOIN analytics_identity_mappings i ON i.user_id=u.id
		LEFT JOIN LATERAL(SELECT CASE WHEN COUNT(DISTINCT external_user_id)=1 THEN MIN(external_user_id) END external_user_id,COUNT(DISTINCT external_user_id) matches
          FROM external_user_mappings WHERE user_id=u.id AND deleted_at IS NULL)e ON TRUE
		WHERE u.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM user_usage_migrations m WHERE m.source_user_id=u.id)
		ORDER BY u.id LIMIT 100001`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	people := map[string]service.AnalyticsPerson{}
	names := map[string][]string{}
	for _, p := range directory.People {
		people[p.ID] = p
		names[service.NormalizeAnalyticsName(p.Name)] = append(names[service.NormalizeAnalyticsName(p.Name)], p.ID)
	}
	var identities []service.AnalyticsIdentity
	userNames, stableCounts := map[string]int{}, map[string]int{}
	for rows.Next() {
		var i service.AnalyticsIdentity
		if err := rows.Scan(&i.UserID, &i.Username, &i.PersonID, &i.Basis); err != nil {
			return nil, err
		}
		if _, ok := people[i.PersonID]; !ok {
			i.PersonID, i.Basis = "", ""
		}
		identities = append(identities, i)
		userNames[service.NormalizeAnalyticsName(i.Username)]++
		if i.PersonID != "" {
			stableCounts[i.PersonID]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(identities) > 100000 {
		return nil, errors.New("平台人员超过100000，需分批整理")
	}
	claimed := map[string]bool{}
	for _, i := range identities {
		if i.PersonID != "" {
			claimed[i.PersonID] = true
		}
	}
	for n := range identities {
		i := &identities[n]
		if i.PersonID != "" && stableCounts[i.PersonID] > 1 {
			i.PersonID, i.Basis = "", "conflict"
			continue
		}
		key := service.NormalizeAnalyticsName(i.Username)
		if i.PersonID == "" && i.Basis != "conflict" && userNames[key] == 1 && len(names[key]) == 1 && !claimed[names[key][0]] {
			i.PersonID, i.Basis = names[key][0], "unique_name"
			claimed[i.PersonID] = true
		}
	}
	return identities, nil
}

func (r *dataAnalyticsRepository) PreviewAnalyticsDirectory(ctx context.Context, directory service.AnalyticsDirectory, digest string, actor int64) (*service.AnalyticsDirectoryPreview, error) {
	identities, err := analyticsIdentities(ctx, r.db, directory)
	if err != nil {
		return nil, err
	}
	preview := &service.AnalyticsDirectoryPreview{ID: uuid.NewString(), Digest: digest, Directory: directory, ExpiresAt: time.Now().Add(24 * time.Hour)}
	names, orgs := map[string]int{}, map[string]bool{}
	for _, o := range directory.Orgs {
		orgs[o.ID] = true
	}
	for _, p := range directory.People {
		names[service.NormalizeAnalyticsName(p.Name)]++
		if !orgs[p.Org] {
			preview.MissingOrgs++
		}
	}
	for _, n := range names {
		if n > 1 {
			preview.DuplicateNames++
		}
	}
	for _, i := range identities {
		if i.PersonID != "" {
			preview.Matches++
		} else {
			preview.Unmatched++
		}
	}
	err = r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_directory_versions WHERE digest=$1)`, digest).Scan(&preview.Applied)
	if err != nil {
		return nil, err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO analytics_directory_imports(id,digest,actor_id,data,expires_at) VALUES($1,$2,$3,$4::jsonb,$5)`,
		preview.ID, digest, actor, service.AnalyticsJSON(directory), preview.ExpiresAt)
	return preview, err
}

func (r *dataAnalyticsRepository) AnalyticsDirectory(ctx context.Context) (*service.AnalyticsDirectoryState, error) {
	state := &service.AnalyticsDirectoryState{Directory: service.AnalyticsDirectory{Orgs: []service.AnalyticsOrg{}, People: []service.AnalyticsPerson{}}, Imports: []map[string]interface{}{}}
	var data []byte
	var effective time.Time
	err := r.db.QueryRowContext(ctx, `SELECT (SELECT directory_revision FROM analytics_runtime_state WHERE id=1),effective_at,data FROM analytics_directory_versions ORDER BY effective_at DESC LIMIT 1`).
		Scan(&state.Version, &effective, &data)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		state.EffectiveAt = &effective
		if err := json.Unmarshal(data, &state.Directory); err != nil {
			return nil, err
		}
	}
	state.Identities, err = analyticsIdentities(ctx, r.db, state.Directory)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,created_at,applied_at FROM analytics_directory_imports ORDER BY created_at DESC LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var created time.Time
		var applied *time.Time
		if err := rows.Scan(&id, &created, &applied); err != nil {
			return nil, err
		}
		state.Imports = append(state.Imports, map[string]interface{}{"id": id, "created_at": created, "applied_at": applied})
	}
	return state, rows.Err()
}

func analyticsLock(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(9072028)`)
	return err
}

func analyticsCheckDirectoryVersion(ctx context.Context, tx *sql.Tx, version int64) error {
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT directory_revision FROM analytics_runtime_state WHERE id=1`).Scan(&current); err != nil {
		return err
	}
	if current != version {
		return service.ErrAnalyticsControlConflict
	}
	return nil
}

func analyticsUpdateAssignments(ctx context.Context, tx *sql.Tx, directory service.AnalyticsDirectory, effective time.Time, first bool, onlyUser int64) error {
	identities, err := analyticsIdentities(ctx, tx, directory)
	if err != nil {
		return err
	}
	people, orgs := map[string]service.AnalyticsPerson{}, map[string]service.AnalyticsOrg{}
	for _, p := range directory.People {
		people[p.ID] = p
	}
	for _, o := range directory.Orgs {
		orgs[o.ID] = o
	}
	type assignment struct {
		id           int64
		person, name string
		ids, names   pq.StringArray
		from         time.Time
	}
	previous := map[int64]assignment{}
	rows, err := tx.QueryContext(ctx, `SELECT user_id,id,person_id,name,org_ids,org_names,effective_from FROM analytics_assignments WHERE effective_to IS NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var user int64
		var a assignment
		if err := rows.Scan(&user, &a.id, &a.person, &a.name, &a.ids, &a.names, &a.from); err != nil {
			_ = rows.Close()
			return err
		}
		previous[user] = a
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, i := range identities {
		if onlyUser > 0 && i.UserID != onlyUser {
			continue
		}
		var oldID int64
		var oldFrom time.Time
		var oldPerson, oldName string
		var oldIDs, oldNames pq.StringArray
		old := previous[i.UserID]
		if onlyUser == -1 && old.id != 0 {
			continue
		}
		oldID, oldPerson, oldName, oldIDs, oldNames, oldFrom = old.id, old.person, old.name, old.ids, old.names, old.from

		p, matched := people[i.PersonID]
		o := orgs[p.Org]
		ids, names := o.Path, o.Names
		if ids == nil {
			ids, names = []string{}, []string{}
		}
		if !matched && oldID == 0 {
			continue
		}
		if matched && oldPerson == p.ID && oldName == p.Name && service.AnalyticsJSON(oldIDs) == service.AnalyticsJSON(ids) && service.AnalyticsJSON(oldNames) == service.AnalyticsJSON(names) {
			continue
		}
		if oldID != 0 {
			if !effective.After(oldFrom) {
				_, err = tx.ExecContext(ctx, `DELETE FROM analytics_assignments WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM analytics_daily WHERE attribution=$1) AND NOT EXISTS(SELECT 1 FROM analytics_monthly WHERE attribution=$1) AND NOT EXISTS(SELECT 1 FROM analytics_archived_daily WHERE attribution=$1)`, oldID)
				if err == nil {
					var exists bool
					err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_assignments WHERE id=$1)`, oldID).Scan(&exists)
					if exists {
						return errors.New("生效时间须晚于上一版本；已有统计不能覆盖")
					}
				}
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE analytics_assignments SET effective_to=$2 WHERE id=$1`, oldID, effective)
			}
			if err != nil {
				return err
			}
		}
		if !matched {
			continue
		}
		basis := i.Basis
		if first {
			basis += ":current_directory_backfill"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_assignments(user_id,person_id,name,org_ids,org_names,basis,effective_from)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, i.UserID, p.ID, p.Name, pq.Array(ids), pq.Array(names), basis, effective); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_identity_mappings(user_id,person_id,basis,actor_id) VALUES($1,$2,$3,0)
			ON CONFLICT(user_id) DO UPDATE SET person_id=EXCLUDED.person_id,basis=EXCLUDED.basis,updated_at=NOW()`, i.UserID, p.ID, i.Basis); err != nil {
			return err
		}
	}
	return nil
}

func analyticsDirtyRange(ctx context.Context, tx *sql.Tx, start, end time.Time, reason string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day,reason,source_change_from)
		SELECT d::date,$3,CASE WHEN $3 IN('directory','rebuild') THEN $4::timestamptz END FROM generate_series($1::date,$2::date,INTERVAL '1 day') d
		ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,reason=EXCLUDED.reason,changed_at=NOW(),attempts=0,last_error='',source_change_from=LEAST(analytics_dirty_days.source_change_from,EXCLUDED.source_change_from)`,
		start.Format("2006-01-02"), end.Format("2006-01-02"), reason, start)
	return err
}

func (r *dataAnalyticsRepository) ApplyAnalyticsDirectory(ctx context.Context, id string, effective time.Time, version, actor int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := analyticsLock(ctx, tx); err != nil {
		return err
	}
	if err := analyticsCheckDirectoryVersion(ctx, tx, version); err != nil {
		return err
	}
	var data []byte
	var digest string
	if err := tx.QueryRowContext(ctx, `SELECT data,digest FROM analytics_directory_imports WHERE id=$1 AND actor_id=$2 AND expires_at>NOW() AND data IS NOT NULL FOR UPDATE`, id, actor).Scan(&data, &digest); err != nil {
		return errors.New("预检不存在、已过期或不属于当前管理员")
	}
	var same bool
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT digest=$1 FROM analytics_directory_versions ORDER BY effective_at DESC LIMIT 1),FALSE)`, digest).Scan(&same); err != nil {
		return err
	}
	var directory service.AnalyticsDirectory
	if err := json.Unmarshal(data, &directory); err != nil {
		return err
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -59, 0)
	if version == 0 {
		effective = cutoff
	} else {
		if same {
			effective = now
		}
		var last time.Time
		if err := tx.QueryRowContext(ctx, `SELECT MAX(effective_at) FROM analytics_directory_versions`).Scan(&last); err != nil {
			return err
		}
		if !effective.After(last) {
			return errors.New("生效时间须晚于上一目录版本")
		}
		var archived bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_bucket_state WHERE archived_until>=$1)`, effective).Scan(&archived); err != nil {
			return err
		}
		if archived {
			return errors.New("生效范围已有源数据清理，无法精确恢复日中归属；请选择新的生效时间")
		}
	}
	if !same {
		if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_directory_versions(digest,data,effective_at,actor_id) VALUES($1,$2,$3,$4)`, digest, string(data), effective, actor); err != nil {
			return err
		}
	}
	if err := analyticsUpdateAssignments(ctx, tx, directory, effective, version == 0, 0); err != nil {
		return err
	}
	start := effective.In(loc)
	if start.Before(cutoff) {
		start = cutoff
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_dirty_days(day,reason,source_change_from)
        SELECT day,'directory',$2::timestamptz FROM analytics_bucket_state WHERE day >= $1::date
        UNION SELECT (NOW() AT TIME ZONE 'Asia/Shanghai')::date,'directory',$2::timestamptz
        ON CONFLICT(day) DO UPDATE SET version=analytics_dirty_days.version+1,reason='directory',changed_at=NOW(),attempts=0,last_error='',
          source_change_from=LEAST(analytics_dirty_days.source_change_from,EXCLUDED.source_change_from)`, start.Format("2006-01-02"), effective); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE analytics_directory_imports SET data=NULL,applied_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	if err := checkAnalyticsPublication(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET directory_revision=directory_revision+1 WHERE id=1`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *dataAnalyticsRepository) MapAnalyticsIdentity(ctx context.Context, userID int64, person string, version, actor int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := analyticsLock(ctx, tx); err != nil {
		return err
	}
	if err := analyticsCheckDirectoryVersion(ctx, tx, version); err != nil {
		return err
	}
	var data []byte
	if err := tx.QueryRowContext(ctx, `SELECT data FROM analytics_directory_versions ORDER BY effective_at DESC LIMIT 1`).Scan(&data); err != nil {
		return err
	}
	var directory service.AnalyticsDirectory
	if err := json.Unmarshal(data, &directory); err != nil {
		return err
	}
	found := false
	for _, p := range directory.People {
		found = found || p.ID == person
	}
	if !found || userID <= 0 {
		return errors.New("请选择有效的人员")
	}
	var valid bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM user_usage_migrations WHERE source_user_id=$1)) AND NOT EXISTS(
		SELECT 1 FROM analytics_identity_mappings WHERE person_id=$2 AND user_id<>$1)`, userID, person).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("账号不存在或人员已绑定其他账号")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_identity_mappings(user_id,person_id,basis,actor_id) VALUES($1,$2,'manual',$3)
		ON CONFLICT(user_id) DO UPDATE SET person_id=EXCLUDED.person_id,basis='manual',actor_id=$3,updated_at=NOW()`, userID, person, actor); err != nil {
		return err
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().UTC()
	effective := now
	var assigned bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_assignments WHERE user_id=$1)`, userID).Scan(&assigned); err != nil {
		return err
	}
	if !assigned {
		// Backfill only the source-complete window. Older published attribution remains explicit.
		if err := tx.QueryRowContext(ctx, `SELECT GREATEST(
          COALESCE((SELECT MIN(created_at) FROM usage_logs WHERE user_id=$1),NOW()),
          ((DATE_TRUNC('month',NOW() AT TIME ZONE 'Asia/Shanghai')-INTERVAL '12 months') AT TIME ZONE 'Asia/Shanghai'),
          COALESCE((SELECT ((MAX(day)+1)::timestamp AT TIME ZONE 'Asia/Shanghai') FROM analytics_bucket_state WHERE archived_count>0),'-infinity'::timestamptz))`, userID).Scan(&effective); err != nil {
			return err
		}
		if effective.After(now) {
			effective = now
		}
	}
	if err := analyticsUpdateAssignments(ctx, tx, directory, effective, false, userID); err != nil {
		return err
	}
	if err := analyticsDirtyRange(ctx, tx, effective.In(loc), now.In(loc), "directory"); err != nil {
		return err
	}
	if err := checkAnalyticsPublication(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE analytics_runtime_state SET directory_revision=directory_revision+1 WHERE id=1`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *dataAnalyticsRepository) QueueAnalyticsRebuild(ctx context.Context, start, end time.Time, actor int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var archived bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM analytics_bucket_state WHERE day BETWEEN $1::date AND $2::date AND archived_count>0)`,
		start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&archived); err != nil {
		return err
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -12, 0)
	if archived || start.Before(cutoff) {
		return errors.New("源数据已清理或超出日明细保留范围，不能完整重算")
	}
	if err := analyticsDirtyRange(ctx, tx, start, end, "rebuild"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO analytics_rebuild_audit(start_day,end_day,actor_id) VALUES($1::date,$2::date,$3)`, start.Format("2006-01-02"), end.Format("2006-01-02"), actor); err != nil {
		return fmt.Errorf("record rebuild: %w", err)
	}
	if err := checkAnalyticsPublication(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

type costManagementRepository struct{ db *sql.DB }

func NewCostManagementRepository(db *sql.DB) service.CostManagementRepository {
	return &costManagementRepository{db: db}
}

func (r *costManagementRepository) ListCostPlans(ctx context.Context, page, pageSize int, kind, search string) ([]service.CostPlan, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	where, args := "WHERE ($1 = '' OR p.plan_type = $1) AND ($2 = '' OR p.name ILIKE '%' || $2 || '%')", []any{kind, search}
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cost_plans p "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id,p.name,p.plan_type,COALESCE(p.fixed_category,''),p.status,p.note,
		       COALESCE(v.version_no,0),COALESCE(v.effective_from,'epoch'),v.effective_to,
		       COALESCE(v.billing_cycle,'monthly'),COALESCE(v.fixed_unit_cost_cny,v.monthly_unit_cost_cny,0)::text,
		       COALESCE(v.monthly_unit_cost_cny,0)::text,COALESCE(v.purchase_quantity,1),
		       (SELECT COUNT(*) FROM cost_subscription_units u WHERE u.plan_id=p.id AND u.effective_from<=NOW() AND(u.effective_to IS NULL OR u.effective_to>NOW())),
		       (SELECT COUNT(*) FROM account_cost_configs ac WHERE ac.plan_id=p.id AND ac.cost_mode='fixed' AND ac.subscription_unit_id IS NULL AND ac.effective_from<=NOW() AND(ac.effective_to IS NULL OR ac.effective_to>NOW())),
		       (SELECT COUNT(*) FROM cost_model_prices mp WHERE mp.plan_version_id=v.id),
		       (SELECT COUNT(*) FROM account_cost_configs ac WHERE ac.plan_id=p.id AND ac.effective_from<=NOW() AND(ac.effective_to IS NULL OR ac.effective_to>NOW()))
		FROM cost_plans p
		LEFT JOIN LATERAL (
		  SELECT * FROM cost_plan_versions WHERE plan_id=p.id ORDER BY version_no DESC LIMIT 1
		) v ON TRUE `+where+` ORDER BY p.id DESC LIMIT $3 OFFSET $4`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.CostPlan, 0)
	for rows.Next() {
		var p service.CostPlan
		var end sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &p.PlanType, &p.FixedCategory, &p.Status, &p.Note, &p.VersionNo, &p.EffectiveFrom, &end, &p.BillingCycle, &p.FixedUnitCostCNY, &p.MonthlyUnitCostCNY, &p.PurchaseQuantity, &p.SubscriptionUnits, &p.UnassignedAccounts, &p.ModelCount, &p.AccountCount); err != nil {
			return nil, 0, err
		}
		if end.Valid {
			p.EffectiveTo = &end.Time
		}
		p.FixedUnitCostCNY = normalizeCostAmount(p.FixedUnitCostCNY)
		p.MonthlyUnitCostCNY = normalizeCostAmount(p.MonthlyUnitCostCNY)
		out = append(out, p)
	}
	return out, total, rows.Err()
}

func (r *costManagementRepository) GetCostPlan(ctx context.Context, id int64) (*service.CostPlan, error) {
	p := &service.CostPlan{ID: id, Prices: make([]service.CostModelPrice, 0)}
	var versionID int64
	var end sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT p.name,p.plan_type,COALESCE(p.fixed_category,''),p.status,p.note,
		       v.id,v.version_no,v.effective_from,v.effective_to,v.billing_cycle,v.fixed_unit_cost_cny::text,
		       v.monthly_unit_cost_cny::text,v.purchase_quantity,
		       (SELECT COUNT(*) FROM cost_subscription_units u WHERE u.plan_id=p.id AND u.effective_from<=NOW() AND(u.effective_to IS NULL OR u.effective_to>NOW())),
		       (SELECT COUNT(*) FROM account_cost_configs ac WHERE ac.plan_id=p.id AND ac.cost_mode='fixed' AND ac.subscription_unit_id IS NULL AND ac.effective_from<=NOW() AND(ac.effective_to IS NULL OR ac.effective_to>NOW())),
		       (SELECT COUNT(*) FROM account_cost_configs ac WHERE ac.plan_id=p.id AND ac.effective_from<=NOW() AND(ac.effective_to IS NULL OR ac.effective_to>NOW()))
		FROM cost_plans p JOIN LATERAL (
		  SELECT * FROM cost_plan_versions WHERE plan_id=p.id ORDER BY version_no DESC LIMIT 1
		) v ON TRUE WHERE p.id=$1`, id).Scan(&p.Name, &p.PlanType, &p.FixedCategory, &p.Status, &p.Note, &versionID, &p.VersionNo, &p.EffectiveFrom, &end, &p.BillingCycle, &p.FixedUnitCostCNY, &p.MonthlyUnitCostCNY, &p.PurchaseQuantity, &p.SubscriptionUnits, &p.UnassignedAccounts, &p.AccountCount)
	if err != nil {
		return nil, err
	}
	if end.Valid {
		p.EffectiveTo = &end.Time
	}
	p.FixedUnitCostCNY = normalizeCostAmount(p.FixedUnitCostCNY)
	p.MonthlyUnitCostCNY = normalizeCostAmount(p.MonthlyUnitCostCNY)
	rows, err := r.db.QueryContext(ctx, `SELECT upstream_model,billing_mode,input_price_cny::text,output_price_cny::text,cache_write_price_cny::text,cache_read_price_cny::text,image_input_price_cny::text,image_output_price_cny::text,per_request_price_cny::text FROM cost_model_prices WHERE plan_version_id=$1 ORDER BY upstream_model`, versionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var price service.CostModelPrice
		if err := rows.Scan(&price.UpstreamModel, &price.BillingMode, &price.InputPriceCNY, &price.OutputPriceCNY, &price.CacheWritePriceCNY, &price.CacheReadPriceCNY, &price.ImageInputPriceCNY, &price.ImageOutputPriceCNY, &price.PerRequestPriceCNY); err != nil {
			return nil, err
		}
		price.InputPriceCNY = normalizeCostAmount(price.InputPriceCNY)
		price.OutputPriceCNY = normalizeCostAmount(price.OutputPriceCNY)
		price.CacheWritePriceCNY = normalizeCostAmount(price.CacheWritePriceCNY)
		price.CacheReadPriceCNY = normalizeCostAmount(price.CacheReadPriceCNY)
		price.ImageInputPriceCNY = normalizeCostAmount(price.ImageInputPriceCNY)
		price.ImageOutputPriceCNY = normalizeCostAmount(price.ImageOutputPriceCNY)
		price.PerRequestPriceCNY = normalizeCostAmount(price.PerRequestPriceCNY)
		p.Prices = append(p.Prices, price)
	}
	p.ModelCount = len(p.Prices)
	return p, rows.Err()
}

func (r *costManagementRepository) CreateCostPlan(ctx context.Context, in service.CostPlanInput) (*service.CostPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	category := any(nil)
	if in.PlanType == "fixed" {
		category = in.FixedCategory
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO cost_plans(name,plan_type,fixed_category,note) VALUES($1,$2,$3,$4) RETURNING id`, strings.TrimSpace(in.Name), in.PlanType, category, in.Note).Scan(&id); err != nil {
		return nil, err
	}
	if err = r.insertCostPlanVersion(ctx, tx, id, 1, in); err != nil {
		return nil, err
	}
	if in.PlanType == "fixed" {
		if err = enqueueCostRecalculationTx(ctx, tx, in.EffectiveFrom); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetCostPlan(ctx, id)
}

func (r *costManagementRepository) UpdateCostPlan(ctx context.Context, id int64, in service.CostPlanInput) (*service.CostPlan, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var planType string
	if err = tx.QueryRowContext(ctx, `SELECT plan_type FROM cost_plans WHERE id=$1 FOR UPDATE`, id).Scan(&planType); err != nil {
		return nil, err
	}
	if planType != in.PlanType {
		return nil, errors.New("成本方案类型不能修改")
	}
	var latestVersionID int64
	var latestVersion int
	var latestEffectiveFrom time.Time
	if err = tx.QueryRowContext(ctx, `SELECT id,version_no,effective_from FROM cost_plan_versions WHERE plan_id=$1 ORDER BY version_no DESC LIMIT 1 FOR UPDATE`, id).Scan(&latestVersionID, &latestVersion, &latestEffectiveFrom); err != nil {
		return nil, err
	}
	category := any(nil)
	if in.PlanType == "fixed" {
		category = in.FixedCategory
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_plans SET name=$2,fixed_category=$3,note=$4,updated_at=NOW() WHERE id=$1`, id, strings.TrimSpace(in.Name), category, in.Note); err != nil {
		return nil, err
	}
	recalculationStart := in.EffectiveFrom
	if !in.EffectiveFrom.After(latestEffectiveFrom) {
		if latestVersion > 1 {
			var previousVersionID int64
			var previousEffectiveFrom time.Time
			if err = tx.QueryRowContext(ctx, `SELECT id,effective_from FROM cost_plan_versions WHERE plan_id=$1 AND version_no<$2 ORDER BY version_no DESC LIMIT 1`, id, latestVersion).Scan(&previousVersionID, &previousEffectiveFrom); err != nil {
				return nil, err
			}
			if !in.EffectiveFrom.After(previousEffectiveFrom) {
				return nil, errors.New("生效时间必须晚于上一个成本方案版本")
			}
			if _, err = tx.ExecContext(ctx, `UPDATE cost_plan_versions SET effective_to=$2 WHERE id=$1`, previousVersionID, in.EffectiveFrom); err != nil {
				return nil, err
			}
		}
		if latestEffectiveFrom.Before(recalculationStart) {
			recalculationStart = latestEffectiveFrom
		}
		if err = r.updateCostPlanVersion(ctx, tx, latestVersionID, in); err != nil {
			return nil, err
		}
	} else {
		if _, err = tx.ExecContext(ctx, `UPDATE cost_plan_versions SET effective_to=$2 WHERE id=$1`, latestVersionID, in.EffectiveFrom); err != nil {
			return nil, err
		}
		if err = r.insertCostPlanVersion(ctx, tx, id, latestVersion+1, in); err != nil {
			return nil, err
		}
	}
	if err = enqueueCostRecalculationTx(ctx, tx, recalculationStart); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetCostPlan(ctx, id)
}

func (r *costManagementRepository) insertCostPlanVersion(ctx context.Context, tx *sql.Tx, planID int64, version int, in service.CostPlanInput) error {
	cycle, unit, monthly, err := normalizeFixedCost(in.BillingCycle, in.FixedUnitCostCNY, in.MonthlyUnitCostCNY)
	if err != nil {
		return err
	}
	qty := in.PurchaseQuantity
	if qty < 1 {
		qty = 1
	}
	var versionID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO cost_plan_versions(plan_id,version_no,effective_from,effective_to,billing_cycle,fixed_unit_cost_cny,monthly_unit_cost_cny,purchase_quantity) VALUES($1,$2,$3,$4,$5,$6::numeric,$7::numeric,$8) RETURNING id`, planID, version, in.EffectiveFrom, in.EffectiveTo, cycle, unit, monthly, qty).Scan(&versionID); err != nil {
		return err
	}
	if in.PlanType != "metered" {
		return nil
	}
	return insertCostModelPrices(ctx, tx, versionID, in.Prices)
}

func (r *costManagementRepository) updateCostPlanVersion(ctx context.Context, tx *sql.Tx, versionID int64, in service.CostPlanInput) error {
	cycle, unit, monthly, err := normalizeFixedCost(in.BillingCycle, in.FixedUnitCostCNY, in.MonthlyUnitCostCNY)
	if err != nil {
		return err
	}
	qty := in.PurchaseQuantity
	if qty < 1 {
		qty = 1
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_plan_versions SET effective_from=$2,effective_to=$3,billing_cycle=$4,fixed_unit_cost_cny=$5::numeric,monthly_unit_cost_cny=$6::numeric,purchase_quantity=$7 WHERE id=$1`, versionID, in.EffectiveFrom, in.EffectiveTo, cycle, unit, monthly, qty); err != nil {
		return err
	}
	if in.PlanType != "metered" {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM cost_model_prices WHERE plan_version_id=$1`, versionID); err != nil {
		return err
	}
	return insertCostModelPrices(ctx, tx, versionID, in.Prices)
}

func insertCostModelPrices(ctx context.Context, tx *sql.Tx, versionID int64, prices []service.CostModelPrice) error {
	for _, p := range prices {
		vals := []string{p.InputPriceCNY, p.OutputPriceCNY, p.CacheWritePriceCNY, p.CacheReadPriceCNY, p.ImageInputPriceCNY, p.ImageOutputPriceCNY, p.PerRequestPriceCNY}
		for i := range vals {
			if vals[i] == "" {
				vals[i] = "0"
			}
		}
		mode := p.BillingMode
		if mode == "" {
			mode = "token"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cost_model_prices(plan_version_id,upstream_model,billing_mode,input_price_cny,output_price_cny,cache_write_price_cny,cache_read_price_cny,image_input_price_cny,image_output_price_cny,per_request_price_cny) VALUES($1,$2,$3,$4::numeric,$5::numeric,$6::numeric,$7::numeric,$8::numeric,$9::numeric,$10::numeric)`, versionID, strings.TrimSpace(p.UpstreamModel), mode, vals[0], vals[1], vals[2], vals[3], vals[4], vals[5], vals[6]); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFixedCost(cycle, unit, legacyMonthly string) (string, string, string, error) {
	if cycle == "" {
		cycle = "monthly"
	}
	if unit == "" {
		unit = legacyMonthly
	}
	if unit == "" {
		unit = "0"
	}
	amount, err := decimal.NewFromString(unit)
	if err != nil {
		return "", "", "", err
	}
	monthly := amount
	if cycle == "yearly" {
		monthly = monthly.Div(decimal.NewFromInt(12))
	}
	return cycle, amount.String(), monthly.String(), nil
}

func normalizeCostAmount(value string) string {
	amount, err := decimal.NewFromString(value)
	if err != nil {
		return value
	}
	return amount.String()
}

func (r *costManagementRepository) DisableCostPlan(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE cost_plans SET status='disabled',updated_at=NOW() WHERE id=$1`)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_plan_versions SET effective_to=NOW() WHERE plan_id=$1 AND effective_from<NOW() AND(effective_to IS NULL OR effective_to>NOW())`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *costManagementRepository) ListAccountCosts(ctx context.Context, page, pageSize int, mode, search string) ([]service.AccountCostRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	where := `WHERE a.deleted_at IS NULL AND ($1='' OR COALESCE(c.cost_mode,'')=$1) AND ($2='' OR a.name ILIKE '%'||$2||'%' OR a.platform ILIKE '%'||$2||'%')`
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts a LEFT JOIN LATERAL(SELECT * FROM account_cost_configs WHERE account_id=a.id AND effective_from<=NOW() AND (effective_to IS NULL OR effective_to>NOW()) ORDER BY effective_from DESC LIMIT 1)c ON TRUE `+where, mode, search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.name,a.platform,a.status,COALESCE(c.cost_mode,''),c.plan_id,COALESCE(p.name,''),c.subscription_unit_id,COALESCE(u.name,''),c.effective_from,c.effective_to,COALESCE(c.exclude_reason,''),COALESCE((SELECT SUM(pending_count) FROM cost_daily_aggregates d WHERE d.account_id=a.id AND d.calculation_status='pending'),0) FROM accounts a LEFT JOIN LATERAL(SELECT * FROM account_cost_configs WHERE account_id=a.id AND effective_from<=NOW() AND (effective_to IS NULL OR effective_to>NOW()) ORDER BY effective_from DESC LIMIT 1)c ON TRUE LEFT JOIN cost_plans p ON p.id=c.plan_id LEFT JOIN cost_subscription_units u ON u.id=c.subscription_unit_id `+where+` ORDER BY a.id DESC LIMIT $3 OFFSET $4`, mode, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.AccountCostRow, 0)
	for rows.Next() {
		var x service.AccountCostRow
		var pid, unitID sql.NullInt64
		var from, to sql.NullTime
		if err := rows.Scan(&x.AccountID, &x.AccountName, &x.Platform, &x.AccountStatus, &x.CostMode, &pid, &x.PlanName, &unitID, &x.SubscriptionUnitName, &from, &to, &x.ExcludeReason, &x.PendingCount); err != nil {
			return nil, 0, err
		}
		if pid.Valid {
			x.PlanID = &pid.Int64
		}
		if unitID.Valid {
			x.SubscriptionUnitID = &unitID.Int64
		}
		if from.Valid {
			x.EffectiveFrom = &from.Time
		}
		if to.Valid {
			x.EffectiveTo = &to.Time
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

func (r *costManagementRepository) ListCostSubscriptionUnits(ctx context.Context, planID int64) ([]service.CostSubscriptionUnit, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id,u.plan_id,u.name,u.effective_from,u.effective_to,
		       COUNT(c.id) FILTER(WHERE c.effective_from<=NOW() AND(c.effective_to IS NULL OR c.effective_to>NOW()))
		FROM cost_subscription_units u
		LEFT JOIN account_cost_configs c ON c.subscription_unit_id=u.id
		WHERE u.plan_id=$1 AND(u.effective_to IS NULL OR u.effective_to>NOW())
		GROUP BY u.id ORDER BY u.id`, planID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CostSubscriptionUnit, 0)
	for rows.Next() {
		var item service.CostSubscriptionUnit
		var end sql.NullTime
		if err = rows.Scan(&item.ID, &item.PlanID, &item.Name, &item.EffectiveFrom, &end, &item.AccountCount); err != nil {
			return nil, err
		}
		if end.Valid {
			item.EffectiveTo = &end.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *costManagementRepository) ListCostModelOptions(ctx context.Context, page, pageSize int, search string) ([]service.CostModelOption, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `WITH models AS(SELECT DISTINCT COALESCE(NULLIF(BTRIM(upstream_model),''),model) AS model FROM usage_logs) SELECT COUNT(*) FROM models WHERE $1='' OR model ILIKE '%'||$1||'%'`, search).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `WITH models AS(SELECT DISTINCT COALESCE(NULLIF(BTRIM(upstream_model),''),model) AS model FROM usage_logs) SELECT model FROM models WHERE $1='' OR model ILIKE '%'||$1||'%' ORDER BY model LIMIT $2 OFFSET $3`, search, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.CostModelOption, 0)
	for rows.Next() {
		var item service.CostModelOption
		if err = rows.Scan(&item.Model); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *costManagementRepository) SaveAccountCost(ctx context.Context, in service.AccountCostInput) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = prepareSubscriptionUnitTx(ctx, tx, &in); err != nil {
		return err
	}
	if err = saveAccountCostTx(ctx, tx, in); err != nil {
		return err
	}
	if err = enqueueCostRecalculationTx(ctx, tx, in.EffectiveFrom); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *costManagementRepository) SaveAccountCosts(ctx context.Context, inputs []service.AccountCostInput) error {
	if len(inputs) == 0 {
		return errors.New("at least one account is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	first := inputs[0]
	createsUnit := strings.TrimSpace(first.NewSubscriptionUnitName) != ""
	if err = prepareSubscriptionUnitTx(ctx, tx, &first); err != nil {
		return err
	}
	if createsUnit {
		for i := range inputs {
			inputs[i].SubscriptionUnitID = first.SubscriptionUnitID
			inputs[i].NewSubscriptionUnitName = ""
		}
	}
	for _, in := range inputs {
		if err = saveAccountCostTx(ctx, tx, in); err != nil {
			return err
		}
	}
	if err = enqueueCostRecalculationTx(ctx, tx, inputs[0].EffectiveFrom); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *costManagementRepository) EndAccountCost(ctx context.Context, accountID int64, end time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE account_cost_configs SET effective_to=$2,updated_at=NOW()
		WHERE id=(
		  SELECT id FROM account_cost_configs
		  WHERE account_id=$1 AND effective_from<$2 AND(effective_to IS NULL OR effective_to>$2)
		  ORDER BY effective_from DESC FOR UPDATE LIMIT 1
		)`, accountID, end)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func enqueueCostRecalculationTx(ctx context.Context, tx *sql.Tx, effectiveFrom time.Time) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(9072027)`); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO cost_jobs(kind,status,start_date,end_date,total_days)
		SELECT 'recalculation','queued',start_date,(NOW() AT TIME ZONE 'Asia/Shanghai')::date-1,
		       ((NOW() AT TIME ZONE 'Asia/Shanghai')::date-1-start_date)+1
		FROM (
		  SELECT GREATEST(
		    $1::date,
		    COALESCE((SELECT MIN((created_at AT TIME ZONE 'Asia/Shanghai')::date) FROM usage_logs),$1::date)
		  ) start_date
		) x
		WHERE start_date<=(NOW() AT TIME ZONE 'Asia/Shanghai')::date-1
		  AND NOT EXISTS (
		    SELECT 1 FROM cost_jobs j
		    WHERE j.kind='recalculation' AND j.status IN ('queued','running')
		      AND j.start_date<=(NOW() AT TIME ZONE 'Asia/Shanghai')::date-1
		      AND j.end_date>=x.start_date
		  )`, effectiveFrom)
	return err
}

func prepareSubscriptionUnitTx(ctx context.Context, tx *sql.Tx, in *service.AccountCostInput) error {
	name := strings.TrimSpace(in.NewSubscriptionUnitName)
	if in.CostMode != "fixed" || name == "" {
		return nil
	}
	if in.PlanID == nil || in.SubscriptionUnitID != nil {
		return errors.New("订阅实例配置无效")
	}
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO cost_subscription_units(plan_id,name,effective_from)
		SELECT id,$2,$3 FROM cost_plans WHERE id=$1 AND plan_type='fixed' AND status='active'
		RETURNING id`, *in.PlanID, name, in.EffectiveFrom).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("固定成本方案不存在或已停用")
		}
		return err
	}
	in.SubscriptionUnitID = &id
	in.NewSubscriptionUnitName = ""
	return nil
}

func saveAccountCostTx(ctx context.Context, tx *sql.Tx, in service.AccountCostInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,effective_from FROM account_cost_configs WHERE account_id=$1 AND (effective_to IS NULL OR effective_to>$2) ORDER BY effective_from FOR UPDATE`, in.AccountID, in.EffectiveFrom)
	if err != nil {
		return err
	}
	var closeIDs []int64
	for rows.Next() {
		var id int64
		var from time.Time
		if err = rows.Scan(&id, &from); err != nil {
			_ = rows.Close()
			return err
		}
		if !from.Before(in.EffectiveFrom) {
			_ = rows.Close()
			return errors.New("a future or overlapping account cost configuration exists")
		}
		closeIDs = append(closeIDs, id)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, id := range closeIDs {
		if _, err = tx.ExecContext(ctx, `UPDATE account_cost_configs SET effective_to=$2,updated_at=NOW() WHERE id=$1`, id, in.EffectiveFrom); err != nil {
			return err
		}
	}
	if in.PlanID != nil {
		var kind, status string
		if err = tx.QueryRowContext(ctx, `SELECT plan_type,status FROM cost_plans WHERE id=$1`, *in.PlanID).Scan(&kind, &status); err != nil {
			return err
		}
		if kind != in.CostMode || status != "active" {
			return errors.New("cost plan type or status does not match")
		}
	}
	if in.CostMode == "fixed" {
		if in.SubscriptionUnitID == nil {
			return errors.New("固定成本账号必须选择一个订阅实例")
		}
		var unitPlanID int64
		var unitFrom time.Time
		var unitTo sql.NullTime
		if err = tx.QueryRowContext(ctx, `SELECT plan_id,effective_from,effective_to FROM cost_subscription_units WHERE id=$1`, *in.SubscriptionUnitID).Scan(&unitPlanID, &unitFrom, &unitTo); err != nil {
			return err
		}
		if in.PlanID == nil || unitPlanID != *in.PlanID || in.EffectiveFrom.Before(unitFrom) || unitTo.Valid && (in.EffectiveTo == nil || in.EffectiveTo.After(unitTo.Time)) {
			return errors.New("订阅实例与固定成本方案或生效时间不匹配")
		}
	} else if in.SubscriptionUnitID != nil {
		return errors.New("只有固定成本账号可以选择订阅实例")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO account_cost_configs(account_id,cost_mode,plan_id,subscription_unit_id,effective_from,effective_to,exclude_reason,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.AccountID, in.CostMode, in.PlanID, in.SubscriptionUnitID, in.EffectiveFrom, in.EffectiveTo, in.ExcludeReason, in.Note)
	if err != nil {
		return err
	}
	return nil
}

func (r *costManagementRepository) GetCostOverview(ctx context.Context, start, end time.Time) (*service.CostOverview, error) {
	o := &service.CostOverview{}
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='usage' AND calculation_status='calculated'),0)::text,COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='fixed_plan_total' AND calculation_status='calculated'),0)::text,COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope IN('usage','fixed_plan_total') AND calculation_status='calculated'),0)::text,COALESCE(SUM(pending_count),0),COALESCE(SUM(error_count),0),COALESCE(SUM(request_count) FILTER(WHERE aggregate_scope='usage'),0),COALESCE(SUM(request_count) FILTER(WHERE aggregate_scope='usage' AND calculation_status='calculated'),0) FROM cost_daily_aggregates WHERE bucket_date >= $1::date AND bucket_date < $2::date`, start, end).Scan(&o.DynamicCostCNY, &o.FixedCostCNY, &o.TotalCostCNY, &o.PendingCount, &o.ErrorCount, &o.EligibleCount, &o.CalculatedCount)
	if err != nil {
		return nil, err
	}
	var last sql.NullTime
	if err = r.db.QueryRowContext(ctx, `SELECT last_success_at FROM cost_jobs WHERE job_key='incremental'`).Scan(&last); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if last.Valid {
		o.LastSuccessAt = &last.Time
	}
	previousStart, previousEnd := previousCostRange(start, end)
	if err = r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope IN('usage','fixed_plan_total') AND calculation_status='calculated'),0)::text FROM cost_daily_aggregates WHERE bucket_date >= $1::date AND bucket_date < $2::date`, previousStart, previousEnd).Scan(&o.PreviousTotalCostCNY); err != nil {
		return nil, err
	}
	var coverageStart, coverageEnd sql.NullTime
	var hasUsage bool
	if err = r.db.QueryRowContext(ctx, `SELECT MIN(ul.created_at),MAX(ul.created_at),EXISTS(SELECT 1 FROM usage_logs) FROM usage_logs ul WHERE ul.id <= COALESCE((SELECT last_usage_log_id FROM cost_jobs WHERE job_key='incremental'),0)`).Scan(&coverageStart, &coverageEnd, &hasUsage); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	var firstDay, lastDay time.Time
	if coverageStart.Valid {
		o.CoverageStart = &coverageStart.Time
		value := coverageStart.Time.In(loc)
		firstDay = time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, loc)
	}
	if coverageEnd.Valid {
		o.CoverageEnd = &coverageEnd.Time
		value := coverageEnd.Time.In(loc)
		lastDay = time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	}
	o.CoverageComplete = !hasUsage || coverageStart.Valid && !start.Before(firstDay) && !end.After(lastDay)
	o.PreviousCoverageComplete = !hasUsage || coverageStart.Valid && !previousStart.Before(firstDay) && !previousEnd.After(lastDay)
	return o, nil
}

func previousCostRange(start, end time.Time) (time.Time, time.Time) {
	if start.Day() == 1 && end.Day() == 1 {
		months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
		if months > 0 {
			return start.AddDate(0, -months, 0), start
		}
	}
	return start.Add(-end.Sub(start)), start
}

func costAnalysisRange(period string, now time.Time) (time.Time, string) {
	switch period {
	case "week":
		return now.AddDate(0, 0, -6), "day"
	case "month":
		return time.Date(now.Year(), now.Month()-11, 1, 0, 0, 0, 0, now.Location()), "month"
	case "year":
		return time.Date(now.Year()-4, time.January, 1, 0, 0, 0, 0, now.Location()), "year"
	default:
		return now.AddDate(0, 0, -29), "day"
	}
}

func (r *costManagementRepository) GetCostAnalysis(ctx context.Context, period string, now time.Time) (*service.CostAnalysis, error) {
	start, grain := costAnalysisRange(period, now)
	trunc := "day"
	format := "YYYY-MM-DD"
	switch grain {
	case "month":
		trunc = "month"
		format = "YYYY-MM"
	case "year":
		trunc = "year"
		format = "YYYY"
	}
	rows, err := r.db.QueryContext(ctx, `SELECT TO_CHAR(DATE_TRUNC($1,bucket_date),$2),COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='usage' AND calculation_status='calculated'),0)::text,COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='fixed_plan_total' AND calculation_status='calculated'),0)::text,COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope IN('usage','fixed_plan_total') AND calculation_status='calculated'),0)::text FROM cost_daily_aggregates WHERE bucket_date >= $3::date AND bucket_date <= $4::date GROUP BY 1 ORDER BY 1`, trunc, format, start, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	a := &service.CostAnalysis{
		Period:       period,
		TotalCostCNY: "0",
		Trend:        make([]service.CostTrendPoint, 0),
		Top:          make([]service.CostPlanShare, 0),
	}
	for rows.Next() {
		var p service.CostTrendPoint
		if err = rows.Scan(&p.Bucket, &p.DynamicCostCNY, &p.FixedCostCNY, &p.TotalCostCNY); err != nil {
			return nil, err
		}
		a.Trend = append(a.Trend, p)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	top, err := r.db.QueryContext(ctx, `SELECT p.id,p.name,SUM(d.amount_cny)::text,SUM(SUM(d.amount_cny)) OVER()::text FROM cost_daily_aggregates d JOIN cost_plans p ON p.id=d.plan_id WHERE d.bucket_date >= $1::date AND d.bucket_date <= $2::date AND d.aggregate_scope IN('usage','fixed_plan_total') AND d.calculation_status='calculated' GROUP BY p.id,p.name ORDER BY SUM(d.amount_cny) DESC LIMIT 5`, start, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = top.Close() }()
	for top.Next() {
		var x service.CostPlanShare
		if err = top.Scan(&x.PlanID, &x.PlanName, &x.AmountCNY, &a.TotalCostCNY); err != nil {
			return nil, err
		}
		a.Top = append(a.Top, x)
	}
	return a, top.Err()
}

func (r *costManagementRepository) GetUserCosts(ctx context.Context, start, end time.Time) ([]service.UserCost, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(m.target_user_id,d.user_id),
		       COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='usage'),0)::text,
		       COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope='fixed_user_allocation'),0)::text,
		       COALESCE(SUM(amount_cny) FILTER(WHERE aggregate_scope IN('usage','fixed_user_allocation')),0)::text
		FROM cost_daily_aggregates d
		LEFT JOIN user_usage_migrations m ON m.source_user_id=d.user_id
		WHERE d.user_id IS NOT NULL AND calculation_status='calculated'
		  AND bucket_date >= $1::date AND bucket_date < $2::date
		GROUP BY COALESCE(m.target_user_id,d.user_id)`, start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.UserCost, 0)
	for rows.Next() {
		var x service.UserCost
		if err = rows.Scan(&x.UserID, &x.DynamicCostCNY, &x.FixedCostCNY, &x.TotalCostCNY); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *costManagementRepository) ListCostJobs(ctx context.Context, page, pageSize int) ([]service.CostJob, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_jobs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,kind,status,start_date,end_date,total_days,completed_days,error_message,created_at,finished_at FROM cost_jobs ORDER BY updated_at DESC,id DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]service.CostJob, 0)
	for rows.Next() {
		var x service.CostJob
		var s, e, f sql.NullTime
		if err = rows.Scan(&x.ID, &x.Kind, &x.Status, &s, &e, &x.TotalDays, &x.CompletedDays, &x.ErrorMessage, &x.CreatedAt, &f); err != nil {
			return nil, 0, err
		}
		if s.Valid {
			x.StartDate = &s.Time
		}
		if e.Valid {
			x.EndDate = &e.Time
		}
		if f.Valid {
			x.FinishedAt = &f.Time
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}
func (r *costManagementRepository) CreateCostRecalculation(ctx context.Context, start, end time.Time, userID int64) (*service.CostJob, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(9072027)`); err != nil {
		return nil, err
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM cost_jobs
		WHERE kind='recalculation' AND status IN ('queued','running')
		  AND start_date <= $2::date AND end_date >= $1::date
	)`, start, end).Scan(&active); err != nil {
		return nil, err
	}
	if active {
		return nil, errors.New("所选日期范围已有补算任务排队或运行中")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	job := &service.CostJob{}
	var startDate, endDate sql.NullTime
	if err = tx.QueryRowContext(ctx, `
		INSERT INTO cost_jobs(kind,status,start_date,end_date,total_days,requested_by)
		VALUES('recalculation','queued',$1::date,$2::date,$3,NULLIF($4,0))
		RETURNING id,kind,status,start_date,end_date,total_days,completed_days,error_message,created_at`,
		start, end, days, userID,
	).Scan(&job.ID, &job.Kind, &job.Status, &startDate, &endDate, &job.TotalDays, &job.CompletedDays, &job.ErrorMessage, &job.CreatedAt); err != nil {
		return nil, err
	}
	if startDate.Valid {
		job.StartDate = &startDate.Time
	}
	if endDate.Valid {
		job.EndDate = &endDate.Time
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return job, nil
}

func (r *costManagementRepository) CancelCostRecalculation(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE cost_jobs
		SET status='cancelled',error_message='用户取消',finished_at=NOW(),updated_at=NOW()
		WHERE id=$1 AND kind='recalculation' AND status IN ('queued','running')`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("任务不存在或当前状态不可取消")
	}
	return nil
}

func (r *costManagementRepository) RunCostIncremental(ctx context.Context, limit int) (processed bool, runErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	defer func() {
		if runErr != nil && !errors.Is(runErr, service.ErrCostAggregationBusy) {
			_, _ = r.db.ExecContext(context.Background(), `UPDATE cost_jobs SET status='failed',error_message=$1,updated_at=NOW() WHERE job_key='incremental'`, runErr.Error())
		}
	}()
	var locked bool
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(9072026)`).Scan(&locked); err != nil || !locked {
		if err != nil {
			return false, err
		}
		return false, service.ErrCostAggregationBusy
	}
	var checkpoint int64
	if err = tx.QueryRowContext(ctx, `SELECT last_usage_log_id FROM cost_jobs WHERE job_key='incremental' FOR UPDATE`).Scan(&checkpoint); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_jobs SET status='running',started_at=NOW(),updated_at=NOW() WHERE job_key='incremental'`); err != nil {
		return false, err
	}
	var maxID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		WITH batch AS (
		  SELECT ul.*,COALESCE(NULLIF(BTRIM(ul.upstream_model),''),ul.model) AS final_model
		  FROM usage_logs ul WHERE ul.id>$1 ORDER BY ul.id LIMIT $2
		), resolved AS (
		  SELECT b.*,c.id config_id,c.cost_mode,c.plan_id,v.id version_id,mp.id price_id,
		    mp.billing_mode AS cost_billing_mode,
		    mp.input_price_cny,mp.output_price_cny,mp.cache_write_price_cny,mp.cache_read_price_cny,
		    mp.image_input_price_cny,mp.image_output_price_cny,mp.per_request_price_cny
		  FROM batch b
		  LEFT JOIN LATERAL(SELECT * FROM account_cost_configs x WHERE x.account_id=b.account_id AND x.effective_from<=b.created_at AND(x.effective_to IS NULL OR x.effective_to>b.created_at)ORDER BY x.effective_from DESC LIMIT 1)c ON TRUE
		  LEFT JOIN LATERAL(SELECT * FROM cost_plan_versions x WHERE x.plan_id=c.plan_id AND x.effective_from<=b.created_at AND(x.effective_to IS NULL OR x.effective_to>b.created_at)ORDER BY x.version_no DESC LIMIT 1)v ON TRUE
		  LEFT JOIN cost_model_prices mp ON mp.plan_version_id=v.id AND mp.upstream_model=b.final_model
		), grouped AS (
		  SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date bucket_date,
		    CASE WHEN config_id IS NULL OR final_model='' OR version_id IS NULL OR price_id IS NULL THEN 'pending' ELSE 'calculated' END calc_status,
		    CASE WHEN config_id IS NULL THEN 'missing_account_config' WHEN final_model='' THEN 'missing_upstream_model' WHEN version_id IS NULL THEN 'missing_plan_version' WHEN price_id IS NULL THEN 'missing_model_price' ELSE '' END issue,
		    config_id,account_id,user_id,plan_id,version_id,final_model,
		    COUNT(*)::bigint requests,SUM(input_tokens)::bigint inputs,SUM(output_tokens)::bigint outputs,SUM(cache_creation_tokens)::bigint cache_writes,SUM(cache_read_tokens)::bigint cache_reads,
		    SUM(COALESCE(image_input_tokens,0))::bigint image_inputs,SUM(COALESCE(image_output_tokens,0))::bigint image_outputs,SUM(COALESCE(image_count,0))::bigint images,
		    SUM(CASE WHEN price_id IS NULL THEN 0 ELSE
		      CASE WHEN cost_billing_mode IN('token','hybrid') THEN
		        input_tokens*input_price_cny/1000000+output_tokens*output_price_cny/1000000+
		        cache_creation_tokens*cache_write_price_cny/1000000+cache_read_tokens*cache_read_price_cny/1000000+
		        COALESCE(image_input_tokens,0)*image_input_price_cny/1000000+COALESCE(image_output_tokens,0)*image_output_price_cny/1000000
		      ELSE 0 END+
		      CASE WHEN cost_billing_mode IN('request','hybrid') THEN per_request_price_cny ELSE 0 END
		    END) amount,
		    COUNT(*) FILTER(WHERE config_id IS NULL OR final_model='' OR version_id IS NULL OR price_id IS NULL)::bigint pending
		  FROM resolved WHERE COALESCE(cost_mode,'metered')='metered'
		  GROUP BY 1,2,3,config_id,account_id,user_id,plan_id,version_id,final_model
		), upserted AS (
		  INSERT INTO cost_daily_aggregates(bucket_date,aggregate_scope,calculation_status,issue_code,account_cost_config_id,account_id,user_id,plan_id,plan_version_id,upstream_model,request_count,input_tokens,output_tokens,cache_write_tokens,cache_read_tokens,image_input_tokens,image_output_tokens,image_count,amount_cny,pending_count)
		  SELECT bucket_date,'usage',calc_status,issue,config_id,account_id,user_id,plan_id,version_id,final_model,requests,inputs,outputs,cache_writes,cache_reads,image_inputs,image_outputs,images,amount,pending FROM grouped
		  ON CONFLICT ON CONSTRAINT cost_daily_aggregates_dimension_unique DO UPDATE SET request_count=cost_daily_aggregates.request_count+EXCLUDED.request_count,input_tokens=cost_daily_aggregates.input_tokens+EXCLUDED.input_tokens,output_tokens=cost_daily_aggregates.output_tokens+EXCLUDED.output_tokens,cache_write_tokens=cost_daily_aggregates.cache_write_tokens+EXCLUDED.cache_write_tokens,cache_read_tokens=cost_daily_aggregates.cache_read_tokens+EXCLUDED.cache_read_tokens,image_input_tokens=cost_daily_aggregates.image_input_tokens+EXCLUDED.image_input_tokens,image_output_tokens=cost_daily_aggregates.image_output_tokens+EXCLUDED.image_output_tokens,image_count=cost_daily_aggregates.image_count+EXCLUDED.image_count,amount_cny=cost_daily_aggregates.amount_cny+EXCLUDED.amount_cny,pending_count=cost_daily_aggregates.pending_count+EXCLUDED.pending_count,updated_at=NOW()
		) SELECT MAX(id) FROM batch`, checkpoint, limit).Scan(&maxID)
	if err != nil {
		return false, err
	}
	if maxID.Valid {
		_, err = tx.ExecContext(ctx, `UPDATE cost_jobs SET last_usage_log_id=$1,status='succeeded',last_success_at=NOW(),error_message='',updated_at=NOW() WHERE job_key='incremental'`, maxID.Int64)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE cost_jobs SET status='succeeded',last_success_at=NOW(),error_message='',updated_at=NOW() WHERE job_key='incremental'`)
	}
	if err != nil {
		return false, err
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return false, err
	}
	days := map[string]time.Time{}
	today := time.Now().In(loc)
	days[today.Format("2006-01-02")] = today
	if maxID.Valid {
		rows, queryErr := tx.QueryContext(ctx, `SELECT DISTINCT(created_at AT TIME ZONE 'Asia/Shanghai')::date FROM usage_logs WHERE id>$1 AND id<=$2`, checkpoint, maxID.Int64)
		if queryErr != nil {
			return false, queryErr
		}
		for rows.Next() {
			var day time.Time
			if queryErr = rows.Scan(&day); queryErr != nil {
				_ = rows.Close()
				return false, queryErr
			}
			days[day.Format("2006-01-02")] = day.In(loc)
		}
		if queryErr = rows.Err(); queryErr != nil {
			_ = rows.Close()
			return false, queryErr
		}
		_ = rows.Close()
		rows, queryErr = tx.QueryContext(ctx, `
			SELECT DISTINCT DATE_TRUNC('month',ul.created_at AT TIME ZONE 'Asia/Shanghai')::date
			FROM usage_logs ul
			JOIN LATERAL (
			  SELECT plan_id FROM account_cost_configs c
			  WHERE c.account_id=ul.account_id AND c.cost_mode='fixed'
			    AND c.effective_from<=ul.created_at AND(c.effective_to IS NULL OR c.effective_to>ul.created_at)
			  ORDER BY c.effective_from DESC LIMIT 1
			) c ON TRUE
			WHERE ul.id>$1 AND ul.id<=$2
			  AND NOT EXISTS (
			    SELECT 1 FROM cost_daily_aggregates d
			    WHERE d.aggregate_scope='fixed_plan_total' AND d.plan_id=c.plan_id
			      AND d.bucket_date>=DATE_TRUNC('month',ul.created_at AT TIME ZONE 'Asia/Shanghai')::date
			      AND d.bucket_date<(DATE_TRUNC('month',ul.created_at AT TIME ZONE 'Asia/Shanghai')+INTERVAL '1 month')::date
			  )`, checkpoint, maxID.Int64)
		if queryErr != nil {
			return false, queryErr
		}
		for rows.Next() {
			var monthStart time.Time
			if queryErr = rows.Scan(&monthStart); queryErr != nil {
				_ = rows.Close()
				return false, queryErr
			}
			for day := monthStart.In(loc); day.Before(monthStart.AddDate(0, 1, 0)) && !day.After(today); day = day.AddDate(0, 0, 1) {
				days[day.Format("2006-01-02")] = day
			}
		}
		if queryErr = rows.Err(); queryErr != nil {
			_ = rows.Close()
			return false, queryErr
		}
		_ = rows.Close()
	}
	for _, day := range days {
		if err = r.rebuildFixedDay(ctx, tx, day); err != nil {
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return maxID.Valid, nil
}

func (r *costManagementRepository) rebuildFixedDay(ctx context.Context, tx *sql.Tx, day time.Time) error {
	bucket := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	monthStart := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)
	if _, err := tx.ExecContext(ctx, `DELETE FROM cost_daily_aggregates WHERE bucket_date=$1::date AND aggregate_scope IN ('fixed_plan_total','fixed_user_allocation')`, bucket); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id,v.id,v.monthly_unit_cost_cny::text,
		       CASE WHEN EXISTS (
		         SELECT 1 FROM account_cost_configs legacy
		         WHERE legacy.cost_mode='fixed' AND legacy.plan_id=p.id
		           AND legacy.subscription_unit_id IS NULL
		           AND legacy.effective_from<$4 AND(legacy.effective_to IS NULL OR legacy.effective_to>$3)
		       ) THEN v.purchase_quantity ELSE (
		         SELECT COUNT(*)::int FROM cost_subscription_units u
		         WHERE u.plan_id=p.id AND u.effective_from<$4 AND(u.effective_to IS NULL OR u.effective_to>$3)
		           AND EXISTS (
		             SELECT 1 FROM account_cost_configs c
		             JOIN usage_logs ul ON ul.account_id=c.account_id
		               AND c.effective_from<=ul.created_at AND(c.effective_to IS NULL OR c.effective_to>ul.created_at)
		             WHERE c.cost_mode='fixed' AND c.plan_id=p.id AND c.subscription_unit_id=u.id
		               AND u.effective_from<=ul.created_at AND(u.effective_to IS NULL OR u.effective_to>ul.created_at)
		               AND ul.created_at>=$3 AND ul.created_at<$4
		           )
		       ) END,
		       (SELECT MIN(anchor.effective_from) FROM cost_plan_versions anchor WHERE anchor.plan_id=p.id),
		       v.effective_from,v.effective_to
		FROM cost_plans p
		JOIN cost_plan_versions v ON v.plan_id=p.id
		  AND v.effective_from<$2 AND(v.effective_to IS NULL OR v.effective_to>$1)
		WHERE p.plan_type='fixed'
		  AND(p.status='active' OR v.effective_from<p.updated_at)
		  AND EXISTS (
		    SELECT 1
		    FROM account_cost_configs c
		    WHERE c.cost_mode='fixed' AND c.plan_id=p.id
		      AND EXISTS (
		        SELECT 1 FROM usage_logs ul
		        WHERE ul.account_id=c.account_id
		          AND c.effective_from<=ul.created_at AND(c.effective_to IS NULL OR c.effective_to>ul.created_at)
		          AND ul.created_at>=$3 AND ul.created_at<$4
		      )
		  )
		`, bucket, bucket.AddDate(0, 0, 1), monthStart, monthEnd)
	if err != nil {
		return err
	}
	type fixedPlan struct {
		planID, versionID int64
		monthly           string
		quantity          int
		anchor            time.Time
		effectiveFrom     time.Time
		effectiveTo       sql.NullTime
	}
	var plans []fixedPlan
	for rows.Next() {
		var p fixedPlan
		if err = rows.Scan(&p.planID, &p.versionID, &p.monthly, &p.quantity, &p.anchor, &p.effectiveFrom, &p.effectiveTo); err != nil {
			_ = rows.Close()
			return err
		}
		plans = append(plans, p)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, p := range plans {
		start, end := fixedBillingPeriod(bucket, p.anchor)
		daily, err := decimal.NewFromString(p.monthly)
		if err != nil {
			return err
		}
		activeStart, activeEnd := bucket, bucket.AddDate(0, 0, 1)
		if p.effectiveFrom.After(activeStart) {
			activeStart = p.effectiveFrom
		}
		if p.effectiveTo.Valid && p.effectiveTo.Time.Before(activeEnd) {
			activeEnd = p.effectiveTo.Time
		}
		if !activeEnd.After(activeStart) {
			continue
		}
		daily = daily.Mul(decimal.NewFromInt(int64(p.quantity))).
			Mul(decimal.NewFromInt(activeEnd.Unix() - activeStart.Unix())).
			Div(decimal.NewFromInt(end.Unix() - start.Unix()))
		if _, err = tx.ExecContext(ctx, `INSERT INTO cost_daily_aggregates(bucket_date,aggregate_scope,plan_id,plan_version_id,amount_cny) VALUES($1,'fixed_plan_total',$2,$3,$4::numeric)`, bucket, p.planID, p.versionID, daily.String()); err != nil {
			return err
		}
		weights, err := tx.QueryContext(ctx, `
			SELECT ul.user_id,COUNT(*)::bigint,
			       SUM(ul.input_tokens+ul.output_tokens+ul.cache_creation_tokens+ul.cache_read_tokens)::bigint
			FROM usage_logs ul JOIN account_cost_configs c ON c.account_id=ul.account_id AND c.cost_mode='fixed' AND c.plan_id=$2
			  AND c.effective_from<=ul.created_at AND(c.effective_to IS NULL OR c.effective_to>ul.created_at)
			WHERE ul.created_at >= $1 AND ul.created_at < $1::timestamptz+INTERVAL '1 day'
			GROUP BY ul.user_id`, bucket, p.planID)
		if err != nil {
			return err
		}
		type weight struct{ userID, requests, tokens int64 }
		var list []weight
		var totalTokens, totalRequests int64
		for weights.Next() {
			var w weight
			if err = weights.Scan(&w.userID, &w.requests, &w.tokens); err != nil {
				_ = weights.Close()
				return err
			}
			list = append(list, w)
			totalTokens += w.tokens
			totalRequests += w.requests
		}
		if err = weights.Close(); err != nil {
			return err
		}
		for _, w := range list {
			den, num := totalTokens, w.tokens
			if den == 0 {
				den, num = totalRequests, w.requests
			}
			if den == 0 {
				continue
			}
			amount := daily.Mul(decimal.NewFromInt(num)).Div(decimal.NewFromInt(den))
			if _, err = tx.ExecContext(ctx, `INSERT INTO cost_daily_aggregates(bucket_date,aggregate_scope,user_id,plan_id,plan_version_id,request_count,amount_cny) VALUES($1,'fixed_user_allocation',$2,$3,$4,$5,$6::numeric)`, bucket, w.userID, p.planID, p.versionID, w.requests, amount.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

func fixedBillingPeriod(day, anchor time.Time) (time.Time, time.Time) {
	loc := day.Location()
	periodStart := anchoredMonthDate(day.Year(), day.Month(), anchor.Day(), loc)
	if periodStart.After(day) {
		prev := day.AddDate(0, -1, 0)
		periodStart = anchoredMonthDate(prev.Year(), prev.Month(), anchor.Day(), loc)
	}
	nextMonth := time.Date(periodStart.Year(), periodStart.Month()+1, 1, 0, 0, 0, 0, loc)
	periodEnd := anchoredMonthDate(nextMonth.Year(), nextMonth.Month(), anchor.Day(), loc)
	return periodStart, periodEnd
}
func anchoredMonthDate(year int, month time.Month, day int, loc *time.Location) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func (r *costManagementRepository) RunNextCostRecalculation(ctx context.Context) (processed bool, runErr error) {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE cost_jobs duplicate
		SET status='cancelled',error_message='已由较早创建的相同范围任务覆盖',finished_at=NOW(),updated_at=NOW()
		WHERE duplicate.kind='recalculation' AND duplicate.status='queued'
		  AND EXISTS (
		    SELECT 1 FROM cost_jobs original
		    WHERE original.kind='recalculation' AND original.status IN ('queued','running')
		      AND original.id<duplicate.id
		      AND original.start_date=duplicate.start_date AND original.end_date=duplicate.end_date
		  )`); err != nil {
		return false, err
	}
	var id int64
	var start, end time.Time
	var completed, total int
	err := r.db.QueryRowContext(ctx, `UPDATE cost_jobs SET status='running',started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE id=(SELECT id FROM cost_jobs WHERE kind='recalculation' AND(status='queued' OR(status='running' AND updated_at<NOW()-INTERVAL '10 minutes')) ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,start_date,end_date,completed_days,total_days`).Scan(&id, &start, &end, &completed, &total)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() {
		if runErr != nil {
			status, retry := recalculationFailureStatus(runErr)
			_, _ = r.db.ExecContext(context.Background(), `
				UPDATE cost_jobs
				SET status=$2,error_message=$3,finished_at=CASE WHEN $4 THEN NULL ELSE NOW() END,updated_at=NOW()
				WHERE id=$1 AND status='running'`, id, status, runErr.Error(), retry)
		}
	}()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return true, err
	}
	day := start.In(loc).AddDate(0, 0, completed)
	for range 7 {
		if day.After(end.In(loc)) {
			break
		}
		var cancelled bool
		if err = r.db.QueryRowContext(ctx, `SELECT status='cancelled' FROM cost_jobs WHERE id=$1`, id).Scan(&cancelled); err != nil {
			return true, err
		}
		if cancelled {
			return true, nil
		}
		completed++
		if err = r.rebuildCostDay(ctx, id, day, completed); err != nil {
			return true, err
		}
		day = day.AddDate(0, 0, 1)
	}
	status := "queued"
	if completed >= total {
		status = "succeeded"
	}
	if _, err = r.db.ExecContext(ctx, `UPDATE cost_jobs SET status=$2,finished_at=CASE WHEN $2='succeeded' THEN NOW() ELSE NULL END,updated_at=NOW() WHERE id=$1 AND status='running'`, id, status); err != nil {
		return true, err
	}
	return true, nil
}

func recalculationFailureStatus(err error) (string, bool) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "queued", true
	}
	return "failed", false
}

func (r *costManagementRepository) rebuildCostDay(ctx context.Context, jobID int64, day time.Time, completed int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(9072026)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM cost_daily_aggregates WHERE bucket_date=$1::date`, day); err != nil {
		return err
	}
	if err = rebuildUsageRange(ctx, tx, day, day.AddDate(0, 0, 1)); err != nil {
		return err
	}
	if err = r.rebuildFixedDay(ctx, tx, day); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cost_jobs SET completed_days=$2,updated_at=NOW() WHERE id=$1`, jobID, completed); err != nil {
		return err
	}
	return tx.Commit()
}

func rebuildUsageRange(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	_, err := tx.ExecContext(ctx, `
		WITH resolved AS (
		  SELECT ul.*,COALESCE(NULLIF(BTRIM(ul.upstream_model),''),ul.model) AS final_model,
		    c.id config_id,c.cost_mode,c.plan_id,v.id version_id,mp.id price_id,
		    mp.billing_mode AS cost_billing_mode,
		    mp.input_price_cny,mp.output_price_cny,mp.cache_write_price_cny,mp.cache_read_price_cny,
		    mp.image_input_price_cny,mp.image_output_price_cny,mp.per_request_price_cny
		  FROM usage_logs ul
		  LEFT JOIN LATERAL(SELECT * FROM account_cost_configs x WHERE x.account_id=ul.account_id AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)ORDER BY x.effective_from DESC LIMIT 1)c ON TRUE
		  LEFT JOIN LATERAL(SELECT * FROM cost_plan_versions x WHERE x.plan_id=c.plan_id AND x.effective_from<=ul.created_at AND(x.effective_to IS NULL OR x.effective_to>ul.created_at)ORDER BY x.version_no DESC LIMIT 1)v ON TRUE
		  LEFT JOIN cost_model_prices mp ON mp.plan_version_id=v.id AND mp.upstream_model=COALESCE(NULLIF(BTRIM(ul.upstream_model),''),ul.model)
		  WHERE ul.created_at >= ($1::date::timestamp AT TIME ZONE 'Asia/Shanghai')
		    AND ul.created_at < ($2::date::timestamp AT TIME ZONE 'Asia/Shanghai')
		), grouped AS (
		  SELECT (created_at AT TIME ZONE 'Asia/Shanghai')::date bucket_date,
		    CASE WHEN config_id IS NULL OR final_model='' OR version_id IS NULL OR price_id IS NULL THEN 'pending' ELSE 'calculated' END calc_status,
		    CASE WHEN config_id IS NULL THEN 'missing_account_config' WHEN final_model='' THEN 'missing_upstream_model' WHEN version_id IS NULL THEN 'missing_plan_version' WHEN price_id IS NULL THEN 'missing_model_price' ELSE '' END issue,
		    config_id,account_id,user_id,plan_id,version_id,final_model,
		    COUNT(*)::bigint requests,SUM(input_tokens)::bigint inputs,SUM(output_tokens)::bigint outputs,SUM(cache_creation_tokens)::bigint cache_writes,SUM(cache_read_tokens)::bigint cache_reads,
		    SUM(COALESCE(image_input_tokens,0))::bigint image_inputs,SUM(COALESCE(image_output_tokens,0))::bigint image_outputs,SUM(COALESCE(image_count,0))::bigint images,
		    SUM(CASE WHEN price_id IS NULL THEN 0 ELSE
		      CASE WHEN cost_billing_mode IN('token','hybrid') THEN
		        input_tokens*input_price_cny/1000000+output_tokens*output_price_cny/1000000+
		        cache_creation_tokens*cache_write_price_cny/1000000+cache_read_tokens*cache_read_price_cny/1000000+
		        COALESCE(image_input_tokens,0)*image_input_price_cny/1000000+COALESCE(image_output_tokens,0)*image_output_price_cny/1000000
		      ELSE 0 END+
		      CASE WHEN cost_billing_mode IN('request','hybrid') THEN per_request_price_cny ELSE 0 END
		    END) amount,
		    COUNT(*) FILTER(WHERE config_id IS NULL OR final_model='' OR version_id IS NULL OR price_id IS NULL)::bigint pending
		  FROM resolved WHERE COALESCE(cost_mode,'metered')='metered'
		  GROUP BY 1,2,3,config_id,account_id,user_id,plan_id,version_id,final_model
		)
		INSERT INTO cost_daily_aggregates(bucket_date,aggregate_scope,calculation_status,issue_code,account_cost_config_id,account_id,user_id,plan_id,plan_version_id,upstream_model,request_count,input_tokens,output_tokens,cache_write_tokens,cache_read_tokens,image_input_tokens,image_output_tokens,image_count,amount_cny,pending_count)
		SELECT bucket_date,'usage',calc_status,issue,config_id,account_id,user_id,plan_id,version_id,final_model,requests,inputs,outputs,cache_writes,cache_reads,image_inputs,image_outputs,images,amount,pending FROM grouped`, start, end)
	return err
}

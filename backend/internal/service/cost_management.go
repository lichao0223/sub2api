package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/shopspring/decimal"
)

const (
	costAggregationInterval = 5 * time.Minute
	costAggregationTimeout  = 4 * time.Minute
)

var ErrCostAggregationBusy = errors.New("cost aggregation is already running")

type CostModelPrice struct {
	UpstreamModel       string `json:"upstream_model"`
	BillingMode         string `json:"billing_mode"`
	InputPriceCNY       string `json:"input_price_cny"`
	OutputPriceCNY      string `json:"output_price_cny"`
	CacheWritePriceCNY  string `json:"cache_write_price_cny"`
	CacheReadPriceCNY   string `json:"cache_read_price_cny"`
	ImageInputPriceCNY  string `json:"image_input_price_cny"`
	ImageOutputPriceCNY string `json:"image_output_price_cny"`
	PerRequestPriceCNY  string `json:"per_request_price_cny"`
}

func (p *CostModelPrice) UnmarshalJSON(data []byte) error {
	type alias CostModelPrice
	raw := struct {
		*alias
		InputPriceCNY       json.RawMessage `json:"input_price_cny"`
		OutputPriceCNY      json.RawMessage `json:"output_price_cny"`
		CacheWritePriceCNY  json.RawMessage `json:"cache_write_price_cny"`
		CacheReadPriceCNY   json.RawMessage `json:"cache_read_price_cny"`
		ImageInputPriceCNY  json.RawMessage `json:"image_input_price_cny"`
		ImageOutputPriceCNY json.RawMessage `json:"image_output_price_cny"`
		PerRequestPriceCNY  json.RawMessage `json:"per_request_price_cny"`
	}{alias: (*alias)(p)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, field := range []struct {
		target *string
		value  json.RawMessage
	}{
		{&p.InputPriceCNY, raw.InputPriceCNY},
		{&p.OutputPriceCNY, raw.OutputPriceCNY},
		{&p.CacheWritePriceCNY, raw.CacheWritePriceCNY},
		{&p.CacheReadPriceCNY, raw.CacheReadPriceCNY},
		{&p.ImageInputPriceCNY, raw.ImageInputPriceCNY},
		{&p.ImageOutputPriceCNY, raw.ImageOutputPriceCNY},
		{&p.PerRequestPriceCNY, raw.PerRequestPriceCNY},
	} {
		if err := unmarshalCostAmount(field.value, field.target); err != nil {
			return err
		}
	}
	return nil
}

type CostPlanInput struct {
	Name               string           `json:"name"`
	PlanType           string           `json:"plan_type"`
	FixedCategory      string           `json:"fixed_category"`
	EffectiveFrom      time.Time        `json:"effective_from"`
	EffectiveTo        *time.Time       `json:"effective_to"`
	BillingCycle       string           `json:"billing_cycle"`
	FixedUnitCostCNY   string           `json:"fixed_unit_cost_cny"`
	MonthlyUnitCostCNY string           `json:"monthly_unit_cost_cny"`
	Note               string           `json:"note"`
	Prices             []CostModelPrice `json:"prices"`
}

func (in *CostPlanInput) UnmarshalJSON(data []byte) error {
	type alias CostPlanInput
	raw := struct {
		*alias
		FixedUnitCostCNY   json.RawMessage `json:"fixed_unit_cost_cny"`
		MonthlyUnitCostCNY json.RawMessage `json:"monthly_unit_cost_cny"`
	}{alias: (*alias)(in)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := unmarshalCostAmount(raw.FixedUnitCostCNY, &in.FixedUnitCostCNY); err != nil {
		return err
	}
	return unmarshalCostAmount(raw.MonthlyUnitCostCNY, &in.MonthlyUnitCostCNY)
}

func unmarshalCostAmount(raw json.RawMessage, target *string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		return json.Unmarshal(raw, target)
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return fmt.Errorf("cost amount must be a number or string: %w", err)
	}
	*target = number.String()
	return nil
}

type CostPlan struct {
	ID                 int64            `json:"id"`
	Name               string           `json:"name"`
	PlanType           string           `json:"plan_type"`
	FixedCategory      string           `json:"fixed_category"`
	Status             string           `json:"status"`
	VersionNo          int              `json:"version_no"`
	EffectiveFrom      time.Time        `json:"effective_from"`
	EffectiveTo        *time.Time       `json:"effective_to"`
	BillingCycle       string           `json:"billing_cycle"`
	FixedUnitCostCNY   string           `json:"fixed_unit_cost_cny"`
	MonthlyUnitCostCNY string           `json:"monthly_unit_cost_cny"`
	SubscriptionUnits  int              `json:"subscription_unit_count"`
	ModelCount         int              `json:"model_count"`
	AccountCount       int              `json:"account_count"`
	Note               string           `json:"note"`
	Prices             []CostModelPrice `json:"prices,omitempty"`
}

type CostPlanBasicInput struct {
	Name          string `json:"name"`
	FixedCategory string `json:"fixed_category"`
	Note          string `json:"note"`
}

type CostPriceChangeInput struct {
	EffectiveFrom       time.Time        `json:"effective_from"`
	BillingCycle        string           `json:"billing_cycle"`
	FixedUnitCostCNY    string           `json:"fixed_unit_cost_cny"`
	UpdateDefault       bool             `json:"update_default"`
	SubscriptionUnitIDs []int64          `json:"subscription_unit_ids"`
	Prices              []CostModelPrice `json:"prices"`
}

func (in *CostPriceChangeInput) UnmarshalJSON(data []byte) error {
	type alias CostPriceChangeInput
	raw := struct {
		*alias
		FixedUnitCostCNY json.RawMessage `json:"fixed_unit_cost_cny"`
	}{alias: (*alias)(in)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return unmarshalCostAmount(raw.FixedUnitCostCNY, &in.FixedUnitCostCNY)
}

type CostPriceVersion struct {
	ID                   int64            `json:"id"`
	PlanID               int64            `json:"plan_id"`
	SubscriptionUnitID   *int64           `json:"subscription_unit_id"`
	SubscriptionUnitName string           `json:"subscription_unit_name"`
	VersionNo            int              `json:"version_no"`
	EffectiveFrom        time.Time        `json:"effective_from"`
	EffectiveTo          *time.Time       `json:"effective_to"`
	BillingCycle         string           `json:"billing_cycle"`
	FixedUnitCostCNY     string           `json:"fixed_unit_cost_cny"`
	MonthlyUnitCostCNY   string           `json:"monthly_unit_cost_cny"`
	Prices               []CostModelPrice `json:"prices,omitempty"`
}

type AccountCostInput struct {
	AccountID               int64      `json:"account_id"`
	CostMode                string     `json:"cost_mode"`
	PlanID                  *int64     `json:"plan_id"`
	SubscriptionUnitID      *int64     `json:"subscription_unit_id"`
	NewSubscriptionUnitName string     `json:"new_subscription_unit_name"`
	EffectiveFrom           time.Time  `json:"effective_from"`
	EffectiveTo             *time.Time `json:"effective_to"`
	ExcludeReason           string     `json:"exclude_reason"`
	Note                    string     `json:"note"`
}

type AccountCostRow struct {
	AccountID            int64      `json:"account_id"`
	AccountName          string     `json:"account_name"`
	Platform             string     `json:"platform"`
	AccountStatus        string     `json:"account_status"`
	CostMode             string     `json:"cost_mode"`
	PlanID               *int64     `json:"plan_id"`
	PlanName             string     `json:"plan_name"`
	SubscriptionUnitID   *int64     `json:"subscription_unit_id"`
	SubscriptionUnitName string     `json:"subscription_unit_name"`
	EffectiveFrom        *time.Time `json:"effective_from"`
	EffectiveTo          *time.Time `json:"effective_to"`
	PendingCount         int64      `json:"pending_count"`
	ExcludeReason        string     `json:"exclude_reason"`
}

type CostSubscriptionUnit struct {
	ID                 int64      `json:"id"`
	PlanID             int64      `json:"plan_id"`
	Name               string     `json:"name"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveTo        *time.Time `json:"effective_to"`
	BillingCycle       string     `json:"billing_cycle"`
	FixedUnitCostCNY   string     `json:"fixed_unit_cost_cny"`
	MonthlyUnitCostCNY string     `json:"monthly_unit_cost_cny"`
	VersionNo          int        `json:"version_no"`
	PriceEffectiveFrom time.Time  `json:"price_effective_from"`
	AccountCount       int        `json:"account_count"`
}

type CostSubscriptionUnitInput struct {
	PlanID           int64     `json:"plan_id"`
	Name             string    `json:"name"`
	EffectiveFrom    time.Time `json:"effective_from"`
	BillingCycle     string    `json:"billing_cycle"`
	FixedUnitCostCNY string    `json:"fixed_unit_cost_cny"`
}

func (in *CostSubscriptionUnitInput) UnmarshalJSON(data []byte) error {
	type alias CostSubscriptionUnitInput
	raw := struct {
		*alias
		FixedUnitCostCNY json.RawMessage `json:"fixed_unit_cost_cny"`
	}{alias: (*alias)(in)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return unmarshalCostAmount(raw.FixedUnitCostCNY, &in.FixedUnitCostCNY)
}

type CostOverview struct {
	DynamicCostCNY           string     `json:"dynamic_cost_cny"`
	FixedCostCNY             string     `json:"fixed_cost_cny"`
	TotalCostCNY             string     `json:"total_cost_cny"`
	PendingCount             int64      `json:"pending_count"`
	ErrorCount               int64      `json:"error_count"`
	EligibleCount            int64      `json:"eligible_count"`
	CalculatedCount          int64      `json:"calculated_count"`
	LastSuccessAt            *time.Time `json:"last_success_at"`
	CoverageStart            *time.Time `json:"coverage_start"`
	CoverageEnd              *time.Time `json:"coverage_end"`
	CoverageComplete         bool       `json:"coverage_complete"`
	PreviousCoverageComplete bool       `json:"previous_coverage_complete"`
	PreviousTotalCostCNY     string     `json:"previous_total_cost_cny"`
}

type CostTrendPoint struct {
	Bucket         string `json:"bucket"`
	DynamicCostCNY string `json:"dynamic_cost_cny"`
	FixedCostCNY   string `json:"fixed_cost_cny"`
	TotalCostCNY   string `json:"total_cost_cny"`
}

type CostPlanShare struct {
	PlanID    int64  `json:"plan_id"`
	PlanName  string `json:"plan_name"`
	AmountCNY string `json:"amount_cny"`
}

type CostModelOption struct {
	Model string `json:"model"`
}

type CostAnalysis struct {
	Period       string           `json:"period"`
	TotalCostCNY string           `json:"total_cost_cny"`
	Trend        []CostTrendPoint `json:"trend"`
	Top          []CostPlanShare  `json:"top"`
}

type UserCost struct {
	UserID         int64  `json:"user_id"`
	DynamicCostCNY string `json:"dynamic_cost_cny"`
	FixedCostCNY   string `json:"fixed_cost_cny"`
	TotalCostCNY   string `json:"total_cost_cny"`
}

type UserCostResult struct {
	Items                   []UserCost `json:"items"`
	UnallocatedFixedCostCNY string     `json:"unallocated_fixed_cost_cny"`
	PlatformTotalCostCNY    string     `json:"platform_total_cost_cny"`
}

type CostJob struct {
	ID            int64      `json:"id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
	TotalDays     int        `json:"total_days"`
	CompletedDays int        `json:"completed_days"`
	ErrorMessage  string     `json:"error_message"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at"`
}

type CostManagementRepository interface {
	ListCostPlans(context.Context, int, int, string, string) ([]CostPlan, int64, error)
	GetCostPlan(context.Context, int64) (*CostPlan, error)
	CreateCostPlan(context.Context, CostPlanInput) (*CostPlan, error)
	UpdateCostPlan(context.Context, int64, CostPlanBasicInput) (*CostPlan, error)
	ChangeCostPlanPrice(context.Context, int64, CostPriceChangeInput) error
	ListCostPriceHistory(context.Context, int64) ([]CostPriceVersion, error)
	DisableCostPlan(context.Context, int64) error
	ListAccountCosts(context.Context, int, int, string, string, time.Time, time.Time) ([]AccountCostRow, int64, error)
	ListCostSubscriptionUnits(context.Context, int64) ([]CostSubscriptionUnit, error)
	CreateCostSubscriptionUnit(context.Context, CostSubscriptionUnitInput) (*CostSubscriptionUnit, error)
	RenameCostSubscriptionUnit(context.Context, int64, string) error
	EndCostSubscriptionUnit(context.Context, int64, time.Time) error
	ListCostModelOptions(context.Context, int, int, string) ([]CostModelOption, int64, error)
	SaveAccountCost(context.Context, AccountCostInput) error
	SaveAccountCosts(context.Context, []AccountCostInput) error
	EndAccountCost(context.Context, int64, time.Time) error
	GetCostOverview(context.Context, time.Time, time.Time) (*CostOverview, error)
	GetCostAnalysis(context.Context, string, time.Time) (*CostAnalysis, error)
	GetUserCosts(context.Context, time.Time, time.Time) ([]UserCost, error)
	ListCostJobs(context.Context, int, int) ([]CostJob, int64, error)
	CreateCostRecalculation(context.Context, time.Time, time.Time, int64) (*CostJob, error)
	CancelCostRecalculation(context.Context, int64) error
	RunCostIncremental(context.Context, int) (bool, error)
	RunNextCostRecalculation(context.Context) (bool, error)
}

type CostManagementService struct {
	repo        CostManagementRepository
	timingWheel *TimingWheelService
	running     int32
}

func NewCostManagementService(repo CostManagementRepository, timingWheel *TimingWheelService) *CostManagementService {
	return &CostManagementService{repo: repo, timingWheel: timingWheel}
}

func ProvideCostManagementService(repo CostManagementRepository, timingWheel *TimingWheelService) *CostManagementService {
	s := NewCostManagementService(repo, timingWheel)
	s.Start()
	return s
}

func (s *CostManagementService) Start() {
	if s == nil || s.repo == nil || s.timingWheel == nil {
		return
	}
	go s.runAggregation()
	s.timingWheel.ScheduleRecurring("cost:incremental-aggregation", costAggregationInterval, s.runAggregation)
}

func (s *CostManagementService) runAggregation() {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.running, 0)
	ctx, cancel := context.WithTimeout(context.Background(), costAggregationTimeout)
	for {
		more, err := s.repo.RunCostIncremental(ctx, 2000)
		if err != nil {
			if !errors.Is(err, ErrCostAggregationBusy) {
				logger.LegacyPrintf("service.cost_management", "[CostManagement] incremental aggregation failed: %v", err)
			}
			break
		}
		if !more {
			break
		}
	}
	cancel()
	recalculationCtx, cancelRecalculation := context.WithTimeout(context.Background(), costAggregationTimeout)
	defer cancelRecalculation()
	for {
		processed, err := s.repo.RunNextCostRecalculation(recalculationCtx)
		if err != nil {
			logger.LegacyPrintf("service.cost_management", "[CostManagement] recalculation failed: %v", err)
			break
		}
		if !processed {
			break
		}
	}
}

func (s *CostManagementService) ListPlans(ctx context.Context, page, pageSize int, kind, search string) ([]CostPlan, int64, error) {
	return s.repo.ListCostPlans(ctx, page, pageSize, kind, search)
}
func (s *CostManagementService) GetPlan(ctx context.Context, id int64) (*CostPlan, error) {
	return s.repo.GetCostPlan(ctx, id)
}
func (s *CostManagementService) CreatePlan(ctx context.Context, in CostPlanInput) (*CostPlan, error) {
	if err := validateCostPlanInput(in); err != nil {
		return nil, err
	}
	return s.repo.CreateCostPlan(ctx, in)
}
func (s *CostManagementService) UpdatePlan(ctx context.Context, id int64, in CostPlanBasicInput) (*CostPlan, error) {
	if id <= 0 || strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("成本方案名称不能为空")
	}
	if in.FixedCategory != "" && in.FixedCategory != "coding_plan" && in.FixedCategory != "self_hosted" && in.FixedCategory != "other" {
		return nil, errors.New("固定成本分类无效")
	}
	return s.repo.UpdateCostPlan(ctx, id, in)
}
func (s *CostManagementService) ChangePlanPrice(ctx context.Context, id int64, in CostPriceChangeInput) error {
	if id <= 0 || in.EffectiveFrom.IsZero() {
		return errors.New("价格生效时间不能为空")
	}
	plan, err := s.repo.GetCostPlan(ctx, id)
	if err != nil {
		return err
	}
	if plan.PlanType == "metered" {
		if err = validateModelPrices(in.Prices); err != nil {
			return err
		}
	} else {
		if in.BillingCycle != "monthly" && in.BillingCycle != "yearly" {
			return errors.New("付费周期无效")
		}
		if err = validateNonnegativeCost(in.FixedUnitCostCNY); err != nil {
			return err
		}
		if !in.UpdateDefault && len(in.SubscriptionUnitIDs) == 0 {
			return errors.New("请选择要改价的订阅实例或更新新实例默认价")
		}
	}
	return s.repo.ChangeCostPlanPrice(ctx, id, in)
}
func (s *CostManagementService) ListPriceHistory(ctx context.Context, id int64) ([]CostPriceVersion, error) {
	if id <= 0 {
		return nil, errors.New("成本方案无效")
	}
	return s.repo.ListCostPriceHistory(ctx, id)
}
func (s *CostManagementService) DisablePlan(ctx context.Context, id int64) error {
	return s.repo.DisableCostPlan(ctx, id)
}
func (s *CostManagementService) ListAccounts(ctx context.Context, page, pageSize int, mode, search string, start, end time.Time) ([]AccountCostRow, int64, error) {
	return s.repo.ListAccountCosts(ctx, page, pageSize, mode, search, start, end)
}
func (s *CostManagementService) ListSubscriptionUnits(ctx context.Context, planID int64) ([]CostSubscriptionUnit, error) {
	if planID <= 0 {
		return nil, errors.New("invalid plan_id")
	}
	return s.repo.ListCostSubscriptionUnits(ctx, planID)
}
func (s *CostManagementService) CreateSubscriptionUnit(ctx context.Context, in CostSubscriptionUnitInput) (*CostSubscriptionUnit, error) {
	if in.PlanID <= 0 {
		return nil, errors.New("固定成本方案无效")
	}
	if err := validateSubscriptionUnitName(in.Name); err != nil {
		return nil, err
	}
	if in.EffectiveFrom.IsZero() {
		return nil, errors.New("订阅开始时间不能为空")
	}
	if in.BillingCycle != "monthly" && in.BillingCycle != "yearly" {
		return nil, errors.New("付费周期无效")
	}
	if err := validateNonnegativeCost(in.FixedUnitCostCNY); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	return s.repo.CreateCostSubscriptionUnit(ctx, in)
}
func (s *CostManagementService) RenameSubscriptionUnit(ctx context.Context, id int64, name string) error {
	if id <= 0 {
		return errors.New("订阅实例无效")
	}
	if err := validateSubscriptionUnitName(name); err != nil {
		return err
	}
	return s.repo.RenameCostSubscriptionUnit(ctx, id, strings.TrimSpace(name))
}
func (s *CostManagementService) EndSubscriptionUnit(ctx context.Context, id int64, end time.Time) error {
	if id <= 0 || end.IsZero() {
		return errors.New("订阅实例无效")
	}
	return s.repo.EndCostSubscriptionUnit(ctx, id, end)
}
func (s *CostManagementService) ListModelOptions(ctx context.Context, page, pageSize int, search string) ([]CostModelOption, int64, error) {
	return s.repo.ListCostModelOptions(ctx, page, pageSize, search)
}
func (s *CostManagementService) SaveAccount(ctx context.Context, in AccountCostInput) error {
	if err := validateAccountCostInput(in); err != nil {
		return err
	}
	return s.repo.SaveAccountCost(ctx, in)
}
func (s *CostManagementService) SaveAccounts(ctx context.Context, inputs []AccountCostInput) error {
	if len(inputs) == 0 {
		return errors.New("at least one account is required")
	}
	seen := make(map[int64]struct{}, len(inputs))
	for _, in := range inputs {
		if err := validateAccountCostInput(in); err != nil {
			return err
		}
		if _, ok := seen[in.AccountID]; ok {
			return errors.New("duplicate account_id")
		}
		seen[in.AccountID] = struct{}{}
	}
	return s.repo.SaveAccountCosts(ctx, inputs)
}
func (s *CostManagementService) EndAccount(ctx context.Context, accountID int64, end time.Time) error {
	if accountID <= 0 || end.IsZero() {
		return errors.New("invalid account cost end time")
	}
	return s.repo.EndAccountCost(ctx, accountID, end)
}
func validateAccountCostInput(in AccountCostInput) error {
	if in.AccountID <= 0 || in.EffectiveFrom.IsZero() || in.EffectiveTo != nil && !in.EffectiveTo.After(in.EffectiveFrom) {
		return errors.New("账号成本生效时间无效")
	}
	if in.CostMode == "excluded" {
		if strings.TrimSpace(in.ExcludeReason) == "" || in.PlanID != nil || in.SubscriptionUnitID != nil || strings.TrimSpace(in.NewSubscriptionUnitName) != "" {
			return errors.New("不纳入核算的账号必须填写排除原因且不能选择成本方案")
		}
	} else if (in.CostMode != "metered" && in.CostMode != "fixed") || in.PlanID == nil {
		return errors.New("请选择成本方案")
	} else if in.CostMode == "fixed" {
		name := strings.TrimSpace(in.NewSubscriptionUnitName)
		hasExisting := in.SubscriptionUnitID != nil && *in.SubscriptionUnitID > 0
		hasNew := name != ""
		if hasExisting == hasNew {
			return errors.New("固定成本账号必须选择一个订阅实例")
		}
		if hasNew {
			return validateSubscriptionUnitName(name)
		}
	} else if in.SubscriptionUnitID != nil || strings.TrimSpace(in.NewSubscriptionUnitName) != "" {
		return errors.New("按量账号不能选择订阅实例")
	}
	return nil
}

func validateSubscriptionUnitName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("订阅实例名称不能为空")
	}
	if utf8.RuneCountInString(name) > 120 {
		return errors.New("订阅实例名称不能超过 120 个字符")
	}
	return nil
}
func (s *CostManagementService) Overview(ctx context.Context, start, end time.Time) (*CostOverview, error) {
	return s.repo.GetCostOverview(ctx, start, end)
}
func (s *CostManagementService) Analysis(ctx context.Context, period string, now time.Time) (*CostAnalysis, error) {
	switch period {
	case "week", "day", "month", "year":
	default:
		period = "day"
	}
	return s.repo.GetCostAnalysis(ctx, period, now)
}
func (s *CostManagementService) UserCosts(ctx context.Context, start, end time.Time) (*UserCostResult, error) {
	items, err := s.repo.GetUserCosts(ctx, start, end)
	if err != nil {
		return nil, err
	}
	overview, err := s.repo.GetCostOverview(ctx, start, end)
	if err != nil {
		return nil, err
	}
	allocated := decimal.Zero
	for _, item := range items {
		value, parseErr := decimal.NewFromString(item.FixedCostCNY)
		if parseErr != nil {
			return nil, parseErr
		}
		allocated = allocated.Add(value)
	}
	fixed, err := decimal.NewFromString(overview.FixedCostCNY)
	if err != nil {
		return nil, err
	}
	unallocated := fixed.Sub(allocated)
	if unallocated.IsNegative() {
		unallocated = decimal.Zero
	}
	return &UserCostResult{
		Items:                   items,
		UnallocatedFixedCostCNY: unallocated.String(),
		PlatformTotalCostCNY:    overview.TotalCostCNY,
	}, nil
}
func (s *CostManagementService) ListJobs(ctx context.Context, page, pageSize int) ([]CostJob, int64, error) {
	return s.repo.ListCostJobs(ctx, page, pageSize)
}
func (s *CostManagementService) CreateRecalculation(ctx context.Context, start, end time.Time, userID int64) (*CostJob, error) {
	if end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("recalculation range must be between 1 and 366 days")
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	if !end.Before(today) {
		return nil, errors.New("历史补算的结束日期不能晚于昨天")
	}
	return s.repo.CreateCostRecalculation(ctx, start, end, userID)
}
func (s *CostManagementService) CancelRecalculation(ctx context.Context, id int64) error {
	return s.repo.CancelCostRecalculation(ctx, id)
}

func validateCostPlanInput(in CostPlanInput) error {
	if strings.TrimSpace(in.Name) == "" || in.EffectiveFrom.IsZero() {
		return errors.New("name and effective_from are required")
	}
	if in.EffectiveTo != nil && !in.EffectiveTo.After(in.EffectiveFrom) {
		return errors.New("effective_to must be after effective_from")
	}
	if in.PlanType == "metered" {
		return validateModelPrices(in.Prices)
	}
	if in.PlanType != "fixed" ||
		(in.FixedCategory != "coding_plan" && in.FixedCategory != "self_hosted" && in.FixedCategory != "other") {
		return errors.New("invalid fixed cost plan")
	}
	if in.BillingCycle != "" && in.BillingCycle != "monthly" && in.BillingCycle != "yearly" {
		return errors.New("invalid billing_cycle")
	}
	amount := in.FixedUnitCostCNY
	if amount == "" {
		amount = in.MonthlyUnitCostCNY
	}
	return validateNonnegativeCost(amount)
}

func validateModelPrices(prices []CostModelPrice) error {
	if len(prices) == 0 {
		return errors.New("metered plan requires at least one model price")
	}
	seen := make(map[string]struct{}, len(prices))
	for _, p := range prices {
		model := strings.TrimSpace(p.UpstreamModel)
		if model == "" {
			return errors.New("upstream_model is required")
		}
		if _, ok := seen[model]; ok {
			return errors.New("duplicate upstream_model")
		}
		seen[model] = struct{}{}
		if p.BillingMode != "" && p.BillingMode != "token" && p.BillingMode != "request" && p.BillingMode != "hybrid" {
			return errors.New("invalid billing_mode")
		}
		for _, value := range []string{p.InputPriceCNY, p.OutputPriceCNY, p.CacheWritePriceCNY, p.CacheReadPriceCNY, p.ImageInputPriceCNY, p.ImageOutputPriceCNY, p.PerRequestPriceCNY} {
			if err := validateNonnegativeCost(value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNonnegativeCost(value string) error {
	if value == "" {
		return nil
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || amount.IsNegative() {
		return errors.New("cost values must be nonnegative numbers")
	}
	return nil
}

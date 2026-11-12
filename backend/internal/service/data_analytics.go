package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/shopspring/decimal"
)

type AnalyticsOrg struct {
	ID     string   `json:"id"`
	Parent string   `json:"parent"`
	Name   string   `json:"name"`
	Path   []string `json:"path"`
	Names  []string `json:"names"`
	Level  int      `json:"level"`
}

type AnalyticsPerson struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Org    string `json:"org"`
	Active bool   `json:"active"`
}

type AnalyticsDirectory struct {
	Orgs   []AnalyticsOrg    `json:"orgs"`
	People []AnalyticsPerson `json:"people"`
}

type AnalyticsDirectoryPreview struct {
	ID             string             `json:"id"`
	Digest         string             `json:"digest"`
	Directory      AnalyticsDirectory `json:"directory"`
	DuplicateNames int                `json:"duplicate_names"`
	MissingOrgs    int                `json:"missing_orgs"`
	Matches        int                `json:"matches"`
	Unmatched      int                `json:"unmatched"`
	ExpiresAt      time.Time          `json:"expires_at"`
	Applied        bool               `json:"applied"`
}

type AnalyticsIdentity struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	PersonID string `json:"person_id"`
	Basis    string `json:"basis"`
}

type AnalyticsDirectoryState struct {
	Version     int64               `json:"version"`
	EffectiveAt *time.Time          `json:"effective_at"`
	Directory   AnalyticsDirectory  `json:"directory"`
	Identities  []AnalyticsIdentity `json:"identities"`
	Imports     []map[string]any    `json:"imports"`
}

type AnalyticsPeriod struct {
	Year   int      `json:"year"`
	Mode   string   `json:"mode"`
	Months []int    `json:"months"`
	Keys   []string `json:"keys"`
	Grain  string   `json:"grain"`
}

func NewAnalyticsPeriod(year int, mode string, months []int, now time.Time) (AnalyticsPeriod, error) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now = now.In(loc)
	p := AnalyticsPeriod{Year: year, Mode: mode, Grain: "month"}
	if year < now.Year()-5 || year > now.Year() {
		return p, errors.New("年份超出保留范围")
	}
	if mode == "year" {
		months = nil
		for m := 1; m <= 12; m++ {
			if year < now.Year() || m <= int(now.Month()) {
				months = append(months, m)
			}
		}
	} else if mode != "month" && mode != "months" {
		return p, errors.New("时间方式必须为 year、month 或 months")
	}
	seen := map[int]bool{}
	cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -59, 0)
	for _, month := range months {
		if month < 1 || month > 12 || (year == now.Year() && month > int(now.Month())) {
			return p, errors.New("月份无效或尚未发生")
		}
		if time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc).Before(cutoff) {
			if mode == "year" {
				continue
			}
			return p, errors.New("月份已超出60个月在线保留范围")
		}
		if !seen[month] {
			seen[month] = true
			p.Months = append(p.Months, month)
		}
	}
	sort.Ints(p.Months)
	if len(p.Months) == 0 || (mode == "month" && len(p.Months) != 1) {
		return p, errors.New("请选择有效月份")
	}
	if mode == "month" {
		p.Grain = "day"
	}
	for _, m := range p.Months {
		p.Keys = append(p.Keys, time.Date(year, time.Month(m), 1, 0, 0, 0, 0, loc).Format("2006-01-02"))
	}
	return p, nil
}

// Split at storage precision and distribute the remainder in stable input order.
func AllocateAnalyticsCost(total decimal.Decimal, weights []decimal.Decimal) []decimal.Decimal {
	out := make([]decimal.Decimal, len(weights))
	denominator := decimal.Zero
	for _, w := range weights {
		denominator = denominator.Add(w)
	}
	if denominator.IsZero() {
		return out
	}
	remaining := total
	for i, w := range weights {
		out[i], _ = total.Mul(w).QuoRem(denominator, 12)
		remaining = remaining.Sub(out[i])
	}
	unit := decimal.New(1, -12)
	for i := 0; remaining.IsPositive(); i = (i + 1) % len(out) {
		if weights[i].IsPositive() {
			out[i] = out[i].Add(unit)
			remaining = remaining.Sub(unit)
		}
	}
	return out
}

type AnalyticsQuery struct {
	Period   AnalyticsPeriod  `json:"period"`
	OrgID    string           `json:"org_id"`
	Search   string           `json:"search"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Sort     string           `json:"sort"`
	Versions map[string]int64 `json:"versions"`
	UserID   int64            `json:"user_id"`
	Export   bool             `json:"export"`
}

// Numeric aggregate fields stay decimal strings all the way to the browser.
type AnalyticsValues struct {
	Input       string `json:"input"`
	Output      string `json:"output"`
	CacheWrite  string `json:"cache_write"`
	CacheRead   string `json:"cache_read"`
	Tokens      string `json:"tokens"`
	Requests    string `json:"requests"`
	Metered     string `json:"metered"`
	Fixed       string `json:"fixed"`
	Cost        string `json:"cost"`
	Pending     string `json:"pending"`
	InputCost   string `json:"input_cost"`
	OutputCost  string `json:"output_cost"`
	WriteCost   string `json:"write_cost"`
	ReadCost    string `json:"read_cost"`
	ImageCost   string `json:"image_cost"`
	RequestCost string `json:"request_cost"`
	Excluded    string `json:"excluded_tokens"`
}

type AnalyticsRow struct {
	AnalyticsValues
	UserID      int64    `json:"user_id"`
	Name        string   `json:"name"`
	PersonID    string   `json:"person_id"`
	OrgIDs      []string `json:"org_ids"`
	OrgNames    []string `json:"org_names"`
	Model       string   `json:"model"`
	Mode        string   `json:"mode"`
	SourceID    int64    `json:"source_id"`
	Source      string   `json:"source"`
	Issue       string   `json:"issue"`
	Attribution int64    `json:"attribution"`
}

type AnalyticsDepartment struct {
	AnalyticsValues
	ID     string `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
	Level  int    `json:"level"`
	Users  int    `json:"users"`
}

type AnalyticsTrend struct {
	Bucket         string                `json:"bucket"`
	Tokens         string                `json:"tokens"`
	MeteredTokens  string                `json:"metered_tokens,omitempty"`
	FixedTokens    string                `json:"fixed_tokens,omitempty"`
	ExcludedTokens string                `json:"excluded_tokens,omitempty"`
	Models         []AnalyticsTrendModel `json:"models,omitempty"`
}

type AnalyticsTrendModel struct {
	Model  string `json:"model"`
	Mode   string `json:"mode"`
	Tokens string `json:"tokens"`
}

type AnalyticsFixedShare struct {
	Month        string `json:"month"`
	UnitID       int64  `json:"unit_id"`
	Name         string `json:"name"`
	UserID       int64  `json:"user_id"`
	Total        string `json:"total"`
	Participants int    `json:"participants"`
	Amount       string `json:"amount"`
	Public       string `json:"public"`
}

type AnalyticsPersonMonth struct {
	AnalyticsValues
	Month      string `json:"month"`
	Generation int64  `json:"generation"`
}

type AnalyticsReport struct {
	Period       AnalyticsPeriod        `json:"period"`
	ScopeOrgID   string                 `json:"scope_org_id"`
	Summary      AnalyticsValues        `json:"summary"`
	Users        int                    `json:"users"`
	People       []AnalyticsRow         `json:"people"`
	PeopleTotal  int                    `json:"people_total"`
	Departments  []AnalyticsDepartment  `json:"departments"`
	Trend        []AnalyticsTrend       `json:"trend"`
	Models       []AnalyticsRow         `json:"models"`
	FixedShares  []AnalyticsFixedShare  `json:"fixed_shares"`
	PersonMonths []AnalyticsPersonMonth `json:"person_months,omitempty"`
	PublicCost   string                 `json:"public_cost"`
	Versions     map[string]int64       `json:"versions"`
	Coverage     map[string]any         `json:"coverage"`
	TaskStatus   *AnalyticsTaskStatus   `json:"task_status,omitempty"`
}

type DataAnalyticsRepository interface {
	PreviewAnalyticsDirectory(context.Context, AnalyticsDirectory, string, int64) (*AnalyticsDirectoryPreview, error)
	AnalyticsDirectory(context.Context) (*AnalyticsDirectoryState, error)
	ApplyAnalyticsDirectory(context.Context, string, time.Time, int64, int64) error
	MapAnalyticsIdentity(context.Context, int64, string, int64, int64) error
	AnalyticsReport(context.Context, AnalyticsQuery) (*AnalyticsReport, error)
	RunAnalyticsBatch(context.Context) (bool, error)
	QueueAnalyticsRebuild(context.Context, time.Time, time.Time, int64) error
	PurgeAnalyticsPreviews(context.Context) error
	AnalyticsMaintenance(context.Context) error
}

type DataAnalyticsService struct {
	repo       DataAnalyticsRepository
	tasks      *AnalyticsTaskService
	running    atomic.Bool
	previewing atomic.Bool
}

func NewDataAnalyticsService(repo DataAnalyticsRepository, tasks *AnalyticsTaskService) *DataAnalyticsService {
	return &DataAnalyticsService{repo: repo, tasks: tasks}
}

func ProvideDataAnalyticsService(repo DataAnalyticsRepository, tasks *AnalyticsTaskService, wheel *TimingWheelService) *DataAnalyticsService {
	s := NewDataAnalyticsService(repo, tasks)
	run := func() {
		if !s.running.CompareAndSwap(false, true) {
			return
		}
		defer s.running.Store(false)
		err := tasks.Run(context.Background(), s.runBatches)
		if err != nil && !errors.Is(err, ErrAnalyticsTasksPaused) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCostAggregationBusy) {
			logger.LegacyPrintf("service.data_analytics", "aggregation stopped: %v", err)
		}
	}
	maintenance := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := repo.PurgeAnalyticsPreviews(ctx); err != nil {
			logger.LegacyPrintf("service.data_analytics", "preview cleanup failed: %v", err)
		}
		err := tasks.Run(ctx, repo.AnalyticsMaintenance)
		if err != nil && !errors.Is(err, ErrAnalyticsTasksPaused) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrCostAggregationBusy) {
			logger.LegacyPrintf("service.data_analytics", "maintenance failed: %v", err)
		}
	}
	go run()
	wheel.ScheduleRecurring("analytics:aggregation", 30*time.Second, run)
	wheel.ScheduleRecurring("analytics:retention", time.Hour, maintenance)
	return s
}

const analyticsBatchTimeout = 4 * time.Minute

func (s *DataAnalyticsService) runBatches(ctx context.Context) error {
	started := time.Now()
	for i := 0; i < 31; i++ {
		// Give each day a full window; a nearly exhausted run must not cancel the next day.
		batchCtx, cancel := context.WithTimeout(ctx, analyticsBatchTimeout)
		more, err := s.repo.RunAnalyticsBatch(batchCtx)
		cancel()
		if err != nil {
			return err
		}
		if !more || time.Since(started) >= analyticsBatchTimeout {
			break
		}
	}
	return nil
}

func (s *DataAnalyticsService) Directory(ctx context.Context) (*AnalyticsDirectoryState, error) {
	return s.repo.AnalyticsDirectory(ctx)
}

func (s *DataAnalyticsService) Preview(ctx context.Context, orgSQL, personSQL string, actor int64) (*AnalyticsDirectoryPreview, error) {
	if !s.previewing.CompareAndSwap(false, true) {
		return nil, errors.New("已有SQL预检正在进行，请稍后重试")
	}
	defer s.previewing.Store(false)
	directory, digest, err := ParseAnalyticsDirectory(orgSQL, personSQL)
	if err != nil {
		return nil, err
	}
	return s.repo.PreviewAnalyticsDirectory(ctx, directory, digest, actor)
}

func (s *DataAnalyticsService) Apply(ctx context.Context, id string, effective time.Time, version, actor int64) error {
	if effective.After(time.Now()) {
		return errors.New("目录生效时间不能在未来")
	}
	return s.tasks.Run(ctx, func(ctx context.Context) error {
		return s.repo.ApplyAnalyticsDirectory(ctx, id, effective, version, actor)
	})
}

func (s *DataAnalyticsService) Map(ctx context.Context, user int64, person string, version, actor int64) error {
	return s.tasks.Run(ctx, func(ctx context.Context) error {
		return s.repo.MapAnalyticsIdentity(ctx, user, person, version, actor)
	})
}

func (s *DataAnalyticsService) Report(ctx context.Context, query AnalyticsQuery) (*AnalyticsReport, error) {
	var report *AnalyticsReport
	var err error
	if query.Export {
		err = s.tasks.Run(ctx, func(ctx context.Context) error {
			report, err = s.repo.AnalyticsReport(ctx, query)
			return err
		})
	} else {
		report, err = s.repo.AnalyticsReport(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	report.TaskStatus, err = s.tasks.Status(ctx)
	return report, err
}

func (s *DataAnalyticsService) Rebuild(ctx context.Context, start, end time.Time, actor int64) error {
	if start.IsZero() || end.Before(start) || end.Sub(start) > 30*24*time.Hour || end.After(time.Now()) {
		return errors.New("补算范围必须是已发生的1至31天")
	}
	return s.tasks.Run(ctx, func(ctx context.Context) error {
		return s.repo.QueueAnalyticsRebuild(ctx, start, end, actor)
	})
}

func AnalyticsJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

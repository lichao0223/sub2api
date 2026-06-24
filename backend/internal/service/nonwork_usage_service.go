package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/nonworktime"
)

const (
	defaultNonworkJobTimeout          = 5 * time.Minute
	defaultNonworkCalendarSyncPeriod  = 24 * time.Hour
	defaultNonworkAggregationInterval = 10 * time.Minute
	holidayCNURLFormat                = "https://raw.githubusercontent.com/NateScarlet/holiday-cn/master/%d.json"
)

type NonworkUsageRepository interface {
	UpsertCalendarDays(ctx context.Context, days []nonworktime.CalendarDay) (inserted int, updated int, err error)
	RecordCalendarSyncRun(ctx context.Context, run CalendarSyncRun) error
	GetCalendarStatus(ctx context.Context, country string, years []int) ([]CalendarYearStatus, error)
	GetCalendarDays(ctx context.Context, country string, startDate, endDate time.Time) ([]nonworktime.CalendarDay, error)
	UpsertManualCalendarDay(ctx context.Context, day nonworktime.CalendarDay) error
	GetUsageEvents(ctx context.Context, start, end time.Time) ([]NonworkUsageEvent, error)
	ReplaceDailyUserStats(ctx context.Context, startDate, endDate time.Time, timezone string, rows []NonworkDailyUserStat) error
	CleanupDailyUserStats(ctx context.Context, cutoffDate time.Time, timezone string) error
}

type CalendarSyncRun struct {
	Country       string
	Year          int
	Source        string
	SourceURL     string
	SourceVersion string
	Status        string
	DaysInserted  int
	DaysUpdated   int
	ErrorMessage  string
	StartedAt     time.Time
	FinishedAt    time.Time
}

type CalendarYearStatus struct {
	Year              int       `json:"year"`
	Country           string    `json:"country"`
	TotalDays         int       `json:"total_days"`
	ConfirmedDays     int       `json:"confirmed_days"`
	ManualOverrides   int       `json:"manual_overrides"`
	FirstDate         string    `json:"first_date"`
	LastDate          string    `json:"last_date"`
	LastSyncStatus    string    `json:"last_sync_status"`
	LastSyncAt        time.Time `json:"last_sync_at"`
	LastSource        string    `json:"last_source"`
	LastSourceVersion string    `json:"last_source_version"`
	Confirmed         bool      `json:"confirmed"`
}

type NonworkUsageEvent struct {
	UserID              int64
	RequestID           string
	CreatedAt           time.Time
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	ActualCost          float64
}

type NonworkDailyUserStat struct {
	BucketDate          time.Time
	Timezone            string
	UserID              int64
	Segment             string
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	TotalTokens         int64
	ActualCost          float64
	ActiveMs            int64
	ActiveSessions      int64
	CalendarConfirmed   bool
}

type UsageNonworkAggregationService struct {
	repo        NonworkUsageRepository
	timingWheel *TimingWheelService
	cfg         config.NonworkUsageConfig
	httpClient  *http.Client
	running     int32
}

type ManualCalendarDayInput struct {
	Date        time.Time
	IsWorkday   bool
	HolidayName string
}

func NewUsageNonworkAggregationService(repo NonworkUsageRepository, timingWheel *TimingWheelService, cfg *config.Config) *UsageNonworkAggregationService {
	var nonworkCfg config.NonworkUsageConfig
	if cfg != nil {
		nonworkCfg = cfg.NonworkUsage
	}
	return &UsageNonworkAggregationService{
		repo:        repo,
		timingWheel: timingWheel,
		cfg:         nonworkCfg,
		httpClient:  &http.Client{Timeout: 20 * time.Second},
	}
}

func ProvideUsageNonworkAggregationService(repo NonworkUsageRepository, timingWheel *TimingWheelService, cfg *config.Config) *UsageNonworkAggregationService {
	svc := NewUsageNonworkAggregationService(repo, timingWheel, cfg)
	svc.Start()
	return svc
}

func (s *UsageNonworkAggregationService) Start() {
	if s == nil || s.repo == nil || !s.cfg.Enabled {
		return
	}

	go s.runStartupJobs()

	if s.timingWheel == nil {
		return
	}
	if s.cfg.Calendar.SyncEnabled {
		s.timingWheel.ScheduleRecurring("nonwork:calendar-sync", defaultNonworkCalendarSyncPeriod, func() {
			s.runCalendarSync()
		})
	}
	if s.cfg.Aggregation.Enabled {
		interval := parseSimpleInterval(s.cfg.Aggregation.Schedule, defaultNonworkAggregationInterval)
		s.timingWheel.ScheduleRecurring("nonwork:usage-aggregation", interval, func() {
			s.runRecentAggregation()
		})
	}
}

func (s *UsageNonworkAggregationService) Stop() {
	if s == nil || s.timingWheel == nil {
		return
	}
	s.timingWheel.Cancel("nonwork:calendar-sync")
	s.timingWheel.Cancel("nonwork:usage-aggregation")
}

func (s *UsageNonworkAggregationService) runStartupJobs() {
	s.runCalendarSync()
	s.runRecentAggregation()
}

func (s *UsageNonworkAggregationService) runCalendarSync() {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.running, 0)

	ctx, cancel := context.WithTimeout(context.Background(), defaultNonworkJobTimeout)
	defer cancel()
	if err := s.SyncCalendars(ctx, time.Now()); err != nil {
		logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 日历同步失败: %v", err)
	}
}

func (s *UsageNonworkAggregationService) runRecentAggregation() {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.running, 0)

	ctx, cancel := context.WithTimeout(context.Background(), defaultNonworkJobTimeout)
	defer cancel()
	days := s.cfg.Aggregation.RecomputeDays
	if days <= 0 {
		days = 3
	}
	cfg := s.workdayConfig()
	now := time.Now().In(cfg.Location)
	start := dateOnlyService(now.AddDate(0, 0, -days+1), cfg.Location)
	end := dateOnlyService(now, cfg.Location).AddDate(0, 0, 1)
	if err := s.AggregateRange(ctx, start, end); err != nil {
		logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 非工作时段聚合失败: %v", err)
	}
	if s.cfg.Aggregation.RetentionDays > 0 {
		cutoff := dateOnlyService(now.AddDate(0, 0, -s.cfg.Aggregation.RetentionDays), cfg.Location)
		if err := s.repo.CleanupDailyUserStats(ctx, cutoff, cfg.Location.String()); err != nil {
			logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 非工作时段聚合保留清理失败: %v", err)
		}
	}
}

func (s *UsageNonworkAggregationService) SyncCalendars(ctx context.Context, now time.Time) error {
	if s == nil || s.repo == nil {
		return nil
	}
	cfg := s.workdayConfig()
	currentYear := now.In(cfg.Location).Year()
	ahead := s.cfg.Calendar.SyncYearsAhead
	if ahead < 0 {
		ahead = 0
	}
	for year := currentYear - 1; year <= currentYear+ahead; year++ {
		if err := s.syncCalendarYear(ctx, year, cfg); err != nil {
			return err
		}
	}
	return nil
}

func (s *UsageNonworkAggregationService) TriggerBackfill(start, end time.Time) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("非工作时段聚合服务未初始化")
	}
	if !end.After(start) {
		return fmt.Errorf("回填时间范围无效")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultNonworkJobTimeout)
		defer cancel()
		if err := s.AggregateRange(ctx, start, end); err != nil {
			logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 手动回填失败: %v", err)
		}
	}()
	return nil
}

func (s *UsageNonworkAggregationService) TriggerCalendarSync(years []int) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("非工作时段聚合服务未初始化")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), defaultNonworkJobTimeout)
		defer cancel()
		cfg := s.workdayConfig()
		if len(years) == 0 {
			if err := s.SyncCalendars(ctx, time.Now()); err != nil {
				logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 手动日历同步失败: %v", err)
			}
			return
		}
		for _, year := range years {
			if err := s.syncCalendarYear(ctx, year, cfg); err != nil {
				logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 手动日历同步失败(year=%d): %v", year, err)
				return
			}
		}
	}()
	return nil
}

func (s *UsageNonworkAggregationService) GetCalendarStatus(ctx context.Context, years []int) ([]CalendarYearStatus, error) {
	if s == nil || s.repo == nil {
		return nil, nil
	}
	cfg := s.workdayConfig()
	if len(years) == 0 {
		now := time.Now().In(cfg.Location)
		years = []int{now.Year() - 1, now.Year(), now.Year() + 1}
	}
	return s.repo.GetCalendarStatus(ctx, cfg.DefaultCountry, years)
}

func (s *UsageNonworkAggregationService) OverrideCalendarDay(ctx context.Context, input ManualCalendarDayInput) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("非工作时段聚合服务未初始化")
	}
	cfg := s.workdayConfig()
	localDate := dateOnlyService(input.Date, cfg.Location)
	weekend := localDate.Weekday() == time.Saturday || localDate.Weekday() == time.Sunday
	dayType := nonworktime.DayTypeManualOffday
	if input.IsWorkday {
		dayType = nonworktime.DayTypeManualWorkday
	}
	day := nonworktime.CalendarDay{
		Date:           localDate,
		Country:        cfg.DefaultCountry,
		IsWorkday:      input.IsWorkday,
		IsOffday:       !input.IsWorkday,
		IsWeekend:      weekend,
		DayType:        dayType,
		HolidayName:    strings.TrimSpace(input.HolidayName),
		Source:         "manual",
		SourceVersion:  "manual",
		Confirmed:      true,
		ManualOverride: true,
	}
	if err := s.repo.UpsertManualCalendarDay(ctx, day); err != nil {
		return err
	}
	start := localDate
	end := localDate.AddDate(0, 0, 1)
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultNonworkJobTimeout)
		defer cancel()
		if err := s.AggregateRange(bgCtx, start, end); err != nil {
			logger.LegacyPrintf("service.nonwork_usage", "[NonworkUsage] 日历手工修正后重算失败: %v", err)
		}
	}()
	return nil
}

func (s *UsageNonworkAggregationService) syncCalendarYear(ctx context.Context, year int, cfg nonworktime.WorkdayConfig) error {
	started := time.Now().UTC()
	run := CalendarSyncRun{
		Country:   cfg.DefaultCountry,
		Year:      year,
		Source:    strings.TrimSpace(s.cfg.Calendar.Source),
		SourceURL: fmt.Sprintf(holidayCNURLFormat, year),
		StartedAt: started,
		Status:    "predicted",
	}
	if run.Source == "" {
		run.Source = "holiday-cn"
	}

	var days []nonworktime.CalendarDay
	if s.cfg.Calendar.SyncEnabled && strings.EqualFold(run.Source, "holiday-cn") {
		data, err := s.fetchHolidayCN(ctx, year)
		if err == nil {
			parsed, parseErr := nonworktime.ParseHolidayCNCalendar(data, cfg.Location)
			if parseErr == nil && parsed.Year == year {
				sourceHash := sha256.Sum256(data)
				cfg.DefaultSource = "holiday-cn"
				cfg.DefaultSourceVers = parsed.SourceVersion
				days = nonworktime.BuildYearCalendar(year, parsed.Days, true, cfg)
				for i := range days {
					if days[i].SourceVersion == "" {
						days[i].SourceVersion = parsed.SourceVersion
					}
				}
				run.SourceVersion = parsed.SourceVersion + "#" + hex.EncodeToString(sourceHash[:8])
				run.Status = "success"
			} else {
				run.ErrorMessage = fmt.Sprintf("parse holiday-cn failed: %v", parseErr)
			}
		} else {
			run.ErrorMessage = err.Error()
		}
	}
	if len(days) == 0 {
		cfg.DefaultSource = "week_rule"
		cfg.DefaultSourceVers = fmt.Sprintf("predicted:%d", year)
		days = nonworktime.BuildYearCalendar(year, nil, false, cfg)
	}

	inserted, updated, err := s.repo.UpsertCalendarDays(ctx, days)
	run.DaysInserted = inserted
	run.DaysUpdated = updated
	run.FinishedAt = time.Now().UTC()
	if err != nil {
		run.Status = "failed"
		run.ErrorMessage = err.Error()
		_ = s.repo.RecordCalendarSyncRun(ctx, run)
		return err
	}
	if recordErr := s.repo.RecordCalendarSyncRun(ctx, run); recordErr != nil {
		return recordErr
	}
	return nil
}

func (s *UsageNonworkAggregationService) AggregateRange(ctx context.Context, start, end time.Time) error {
	if s == nil || s.repo == nil || !end.After(start) {
		return nil
	}
	cfg := s.workdayConfig()
	tz := cfg.Location.String()
	localStart := dateOnlyService(start.In(cfg.Location), cfg.Location)
	localEndExclusive := dateOnlyService(end.In(cfg.Location), cfg.Location)
	if end.In(cfg.Location).After(localEndExclusive) {
		localEndExclusive = localEndExclusive.AddDate(0, 0, 1)
	}
	localEndDate := localEndExclusive.AddDate(0, 0, -1)
	if localEndDate.Before(localStart) {
		return nil
	}

	for year := localStart.Year(); year <= localEndDate.Year(); year++ {
		if err := s.syncCalendarYear(ctx, year, cfg); err != nil {
			return err
		}
	}

	calendarDays, err := s.repo.GetCalendarDays(ctx, cfg.DefaultCountry, localStart.AddDate(0, 0, -1), localEndDate.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	calendar := nonworktime.CalendarMap(calendarDays, cfg.Location)

	queryStart := localStart.Add(-cfg.ActiveGap).UTC()
	queryEnd := localEndExclusive.Add(cfg.ActiveGap)
	if cfg.MinSession > cfg.ActiveGap {
		queryEnd = localEndExclusive.Add(cfg.MinSession)
	}
	events, err := s.repo.GetUsageEvents(ctx, queryStart, queryEnd.UTC())
	if err != nil {
		return err
	}

	stats := make(map[string]*NonworkDailyUserStat)
	for _, ev := range dedupeNonworkUsageEvents(events) {
		localCreated := ev.CreatedAt.In(cfg.Location)
		if localCreated.Before(localStart) || !localCreated.Before(localEndExclusive) {
			continue
		}
		segment, confirmed := nonworktime.SegmentAt(ev.CreatedAt, calendar, cfg)
		date := dateOnlyService(localCreated, cfg.Location)
		row := getNonworkStat(stats, date, tz, ev.UserID, segment)
		row.Requests++
		row.InputTokens += ev.InputTokens
		row.OutputTokens += ev.OutputTokens
		row.CacheCreationTokens += ev.CacheCreationTokens
		row.CacheReadTokens += ev.CacheReadTokens
		row.TotalTokens += ev.TotalTokens
		row.ActualCost += ev.ActualCost
		row.CalendarConfirmed = row.CalendarConfirmed && confirmed
	}

	points := make([]nonworktime.RequestPoint, 0, len(events))
	for _, ev := range events {
		points = append(points, nonworktime.RequestPoint{
			UserID:    ev.UserID,
			RequestID: ev.RequestID,
			CreatedAt: ev.CreatedAt,
		})
	}
	for _, seg := range nonworktime.BuildActiveSegments(points, cfg) {
		for _, part := range nonworktime.SplitActiveSegment(seg, calendar, cfg) {
			if part.Date.Before(localStart) || part.Date.After(localEndDate) {
				continue
			}
			_, confirmed := nonworktime.SegmentAt(part.Date, calendar, cfg)
			row := getNonworkStat(stats, part.Date, tz, seg.UserID, part.Segment)
			row.ActiveMs += part.Millis
			row.CalendarConfirmed = row.CalendarConfirmed && confirmed
		}
		for _, sessionPart := range splitSessionForCounting(seg, calendar, cfg) {
			if sessionPart.Date.Before(localStart) || sessionPart.Date.After(localEndDate) {
				continue
			}
			row := getNonworkStat(stats, sessionPart.Date, tz, seg.UserID, sessionPart.Segment)
			row.ActiveSessions++
		}
	}

	rows := make([]NonworkDailyUserStat, 0, len(stats))
	for _, row := range stats {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].BucketDate.Equal(rows[j].BucketDate) {
			return rows[i].BucketDate.Before(rows[j].BucketDate)
		}
		if rows[i].UserID != rows[j].UserID {
			return rows[i].UserID < rows[j].UserID
		}
		return rows[i].Segment < rows[j].Segment
	})
	return s.repo.ReplaceDailyUserStats(ctx, localStart, localEndDate, tz, rows)
}

func (s *UsageNonworkAggregationService) fetchHolidayCN(ctx context.Context, year int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(holidayCNURLFormat, year), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("holiday-cn returned status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func (s *UsageNonworkAggregationService) workdayConfig() nonworktime.WorkdayConfig {
	cfg := nonworktime.DefaultConfig()
	if loc, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil {
		cfg.Location = loc
	}
	if d, err := parseHHMMDuration(s.cfg.WorkStart); err == nil {
		cfg.WorkStart = d
	}
	if d, err := parseHHMMDuration(s.cfg.WorkEnd); err == nil {
		cfg.WorkEnd = d
	}
	if s.cfg.ActiveGapMinutes > 0 {
		cfg.ActiveGap = time.Duration(s.cfg.ActiveGapMinutes) * time.Minute
	}
	if s.cfg.MinSessionMinutes >= 0 {
		cfg.MinSession = time.Duration(s.cfg.MinSessionMinutes) * time.Minute
	}
	if strings.TrimSpace(s.cfg.Calendar.Country) != "" {
		cfg.DefaultCountry = strings.TrimSpace(s.cfg.Calendar.Country)
	}
	return cfg.Normalize()
}

func dedupeNonworkUsageEvents(events []NonworkUsageEvent) []NonworkUsageEvent {
	sort.Slice(events, func(i, j int) bool {
		if events[i].UserID != events[j].UserID {
			return events[i].UserID < events[j].UserID
		}
		if !events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].CreatedAt.Before(events[j].CreatedAt)
		}
		return events[i].RequestID < events[j].RequestID
	})
	out := make([]NonworkUsageEvent, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, ev := range events {
		if ev.RequestID != "" {
			key := fmt.Sprintf("%d:%s", ev.UserID, ev.RequestID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, ev)
	}
	return out
}

func getNonworkStat(rows map[string]*NonworkDailyUserStat, date time.Time, tz string, userID int64, segment string) *NonworkDailyUserStat {
	key := fmt.Sprintf("%s:%d:%s", date.Format("2006-01-02"), userID, segment)
	if row, ok := rows[key]; ok {
		return row
	}
	row := &NonworkDailyUserStat{
		BucketDate:        date,
		Timezone:          tz,
		UserID:            userID,
		Segment:           segment,
		CalendarConfirmed: true,
	}
	rows[key] = row
	return row
}

func splitSessionForCounting(seg nonworktime.ActiveSegment, calendar map[string]nonworktime.CalendarDay, cfg nonworktime.WorkdayConfig) []nonworktime.SegmentDuration {
	parts := nonworktime.SplitActiveSegment(seg, calendar, cfg)
	if len(parts) == 0 {
		return nil
	}
	seen := make(map[string]nonworktime.SegmentDuration)
	for _, part := range parts {
		key := part.Date.Format("2006-01-02") + ":" + part.Segment
		if _, ok := seen[key]; !ok {
			seen[key] = nonworktime.SegmentDuration{Date: part.Date, Segment: part.Segment}
		}
	}
	out := make([]nonworktime.SegmentDuration, 0, len(seen))
	for _, part := range seen {
		out = append(out, part)
	}
	return out
}

func parseHHMMDuration(value string) (time.Duration, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, nil
}

func parseSimpleInterval(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d
	}
	if strings.HasPrefix(value, "*/") && strings.HasSuffix(value, " * * * *") {
		mins := strings.TrimSuffix(strings.TrimPrefix(value, "*/"), " * * * *")
		if parsed, err := time.ParseDuration(mins + "m"); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func dateOnlyService(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

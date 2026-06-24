package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/nonworktime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type fakeNonworkUsageRepo struct {
	calendar []nonworktime.CalendarDay
	events   []NonworkUsageEvent
	rows     []NonworkDailyUserStat
	runs     []CalendarSyncRun
}

func (r *fakeNonworkUsageRepo) UpsertCalendarDays(ctx context.Context, days []nonworktime.CalendarDay) (int, int, error) {
	r.calendar = days
	return len(days), 0, nil
}

func (r *fakeNonworkUsageRepo) RecordCalendarSyncRun(ctx context.Context, run CalendarSyncRun) error {
	r.runs = append(r.runs, run)
	return nil
}

func (r *fakeNonworkUsageRepo) GetCalendarStatus(ctx context.Context, country string, years []int) ([]CalendarYearStatus, error) {
	return nil, nil
}

func (r *fakeNonworkUsageRepo) GetStatsCoverage(ctx context.Context, startDate, endDate time.Time, timezone string) (usagestats.NonworkStatsCoverage, error) {
	return usagestats.NonworkStatsCoverage{Complete: true}, nil
}

func (r *fakeNonworkUsageRepo) GetFirstUsageDate(ctx context.Context, timezone string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (r *fakeNonworkUsageRepo) GetCalendarDays(ctx context.Context, country string, startDate, endDate time.Time) ([]nonworktime.CalendarDay, error) {
	return r.calendar, nil
}

func (r *fakeNonworkUsageRepo) UpsertManualCalendarDay(ctx context.Context, day nonworktime.CalendarDay) error {
	return nil
}

func (r *fakeNonworkUsageRepo) GetUsageEvents(ctx context.Context, start, end time.Time) ([]NonworkUsageEvent, error) {
	return r.events, nil
}

func (r *fakeNonworkUsageRepo) ReplaceDailyUserStats(ctx context.Context, startDate, endDate time.Time, timezone string, rows []NonworkDailyUserStat) error {
	r.rows = rows
	return nil
}

func (r *fakeNonworkUsageRepo) CleanupDailyUserStats(ctx context.Context, cutoffDate time.Time, timezone string) error {
	return nil
}

func TestUsageNonworkAggregationServiceAggregateRangeSplitsActiveDuration(t *testing.T) {
	loc := mustLoadLocation(t, "Asia/Shanghai")
	day := time.Date(2026, 6, 24, 0, 0, 0, 0, loc)
	repo := &fakeNonworkUsageRepo{
		calendar: []nonworktime.CalendarDay{
			{
				Date:      day,
				Country:   nonworktime.CountryCN,
				IsWorkday: true,
				IsOffday:  false,
				IsWeekend: false,
				DayType:   nonworktime.DayTypeNormalWorkday,
				Source:    "test",
				Confirmed: true,
			},
		},
		events: []NonworkUsageEvent{
			{
				UserID:      10,
				RequestID:   "a",
				CreatedAt:   time.Date(2026, 6, 24, 17, 58, 0, 0, loc).UTC(),
				InputTokens: 100,
				TotalTokens: 100,
				ActualCost:  1,
			},
			{
				UserID:       10,
				RequestID:    "b",
				CreatedAt:    time.Date(2026, 6, 24, 18, 3, 0, 0, loc).UTC(),
				OutputTokens: 200,
				TotalTokens:  200,
				ActualCost:   2,
			},
		},
	}
	svc := NewUsageNonworkAggregationService(repo, nil, &config.Config{
		NonworkUsage: config.NonworkUsageConfig{
			Enabled:           true,
			Timezone:          "Asia/Shanghai",
			WorkStart:         "08:30",
			WorkEnd:           "18:00",
			ActiveGapMinutes:  5,
			MinSessionMinutes: 1,
			Calendar: config.NonworkUsageCalendarConfig{
				Country:     "CN",
				Source:      "week_rule",
				SyncEnabled: false,
			},
			Aggregation: config.NonworkUsageAggregationConfig{Enabled: true, RecomputeDays: 3, RetentionDays: 3650},
		},
	})

	err := svc.AggregateRange(context.Background(), day, day.AddDate(0, 0, 1))
	require.NoError(t, err)

	bySegment := map[string]NonworkDailyUserStat{}
	for _, row := range repo.rows {
		bySegment[row.Segment] = row
	}
	require.Equal(t, int64(1), bySegment[nonworktime.SegmentWorkHours].Requests)
	require.Equal(t, int64(100), bySegment[nonworktime.SegmentWorkHours].TotalTokens)
	require.Equal(t, int64(2*60*1000), bySegment[nonworktime.SegmentWorkHours].ActiveMs)
	require.Equal(t, int64(1), bySegment[nonworktime.SegmentAfterHours].Requests)
	require.Equal(t, int64(200), bySegment[nonworktime.SegmentAfterHours].TotalTokens)
	require.Equal(t, int64(3*60*1000), bySegment[nonworktime.SegmentAfterHours].ActiveMs)
}

func TestUsageNonworkAggregationServiceSyncCalendarUsesPredictionForEmptyHolidayCN(t *testing.T) {
	repo := &fakeNonworkUsageRepo{}
	svc := NewUsageNonworkAggregationService(repo, nil, &config.Config{
		NonworkUsage: config.NonworkUsageConfig{
			Timezone: "Asia/Shanghai",
			Calendar: config.NonworkUsageCalendarConfig{
				Country:     "CN",
				Source:      "holiday-cn",
				SyncEnabled: true,
			},
		},
	})
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"year":2027,"papers":[],"days":[]}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	cfg := svc.workdayConfig()
	err := svc.syncCalendarYear(context.Background(), 2027, cfg)
	require.NoError(t, err)
	require.Len(t, repo.calendar, 365)
	require.NotEmpty(t, repo.runs)
	require.Equal(t, "predicted", repo.runs[len(repo.runs)-1].Status)
	require.Equal(t, "holiday-cn has no confirmed day entries", repo.runs[len(repo.runs)-1].ErrorMessage)

	for _, day := range repo.calendar {
		require.False(t, day.Confirmed)
		require.Equal(t, "week_rule", day.Source)
	}
}

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

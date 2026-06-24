package nonworktime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildYearCalendarHolidayOverride(t *testing.T) {
	cfg := DefaultConfig()
	holidays := []HolidayDay{
		{Name: "元旦", Date: mustDate(t, "2026-01-01"), IsOffDay: true},
		{Name: "元旦", Date: mustDate(t, "2026-01-04"), IsOffDay: false},
	}

	days := BuildYearCalendar(2026, holidays, true, cfg)
	byDate := CalendarMap(days, cfg.Location)

	require.Len(t, days, 365)
	require.False(t, byDate["2026-01-01"].IsWorkday)
	require.True(t, byDate["2026-01-01"].IsOffday)
	require.Equal(t, DayTypeHolidayOffday, byDate["2026-01-01"].DayType)

	require.True(t, byDate["2026-01-04"].IsWeekend)
	require.True(t, byDate["2026-01-04"].IsWorkday)
	require.False(t, byDate["2026-01-04"].IsOffday)
	require.Equal(t, DayTypeMakeupWorkday, byDate["2026-01-04"].DayType)
}

func TestSegmentAtUsesShanghaiWorkHours(t *testing.T) {
	cfg := DefaultConfig()
	calendar := CalendarMap(BuildYearCalendar(2026, nil, true, cfg), cfg.Location)

	seg, confirmed := SegmentAt(mustTime(t, "2026-06-18T08:29:00+08:00"), calendar, cfg)
	require.Equal(t, SegmentAfterHours, seg)
	require.True(t, confirmed)

	seg, confirmed = SegmentAt(mustTime(t, "2026-06-18T08:30:00+08:00"), calendar, cfg)
	require.Equal(t, SegmentWorkHours, seg)
	require.True(t, confirmed)

	seg, confirmed = SegmentAt(mustTime(t, "2026-06-18T18:00:00+08:00"), calendar, cfg)
	require.Equal(t, SegmentAfterHours, seg)
	require.True(t, confirmed)
}

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", value, DefaultConfig().Location)
	require.NoError(t, err)
	return d
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return d
}

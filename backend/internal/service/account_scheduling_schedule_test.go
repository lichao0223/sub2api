package service

import (
	"fmt"
	"testing"
	"time"
)

func scheduleExtra(enabled bool, timezone string, windows map[string]any) map[string]any {
	return map[string]any{SchedulingScheduleExtraKey: map[string]any{
		"enabled": enabled, "timezone": timezone, "weekly_windows": windows,
	}}
}

func TestSchedulingScheduleBoundariesAndWeekdays(t *testing.T) {
	schedule, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Shanghai", map[string]any{
		"1": []any{[]any{"09:00", "12:00"}, []any{"13:30", "18:00"}},
		"5": []any{[]any{"23:00", "02:00"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	tests := []struct {
		name, value string
		want        bool
	}{
		{"start inclusive", "2026-08-24 09:00", true},
		{"end exclusive", "2026-08-24 12:00", false},
		{"lunch gap", "2026-08-24 12:30", false},
		{"second window", "2026-08-24 17:59", true},
		{"weekend", "2026-08-29 10:00", false},
		{"cross midnight start", "2026-08-28 23:30", true},
		{"cross midnight next day", "2026-08-29 01:59", true},
		{"cross midnight end", "2026-08-29 02:00", false},
	}
	for _, test := range tests {
		at, parseErr := time.ParseInLocation("2006-01-02 15:04", test.value, loc)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got := schedule.IsWithin(at); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestSchedulingScheduleValidationAndNextStart(t *testing.T) {
	if _, err := ParseSchedulingSchedule(map[string]any{SchedulingScheduleExtraKey: "bad"}); err == nil {
		t.Fatal("expected malformed schedule rejection")
	}
	if _, err := ParseSchedulingSchedule(scheduleExtra(true, "Not/AZone", nil)); err == nil {
		t.Fatal("expected invalid timezone")
	}
	if _, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Shanghai", map[string]any{
		"1": []any{[]any{"09:00", "12:00"}, []any{"11:00", "13:00"}},
	})); err == nil {
		t.Fatal("expected overlapping windows")
	}
	if _, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Shanghai", map[string]any{
		"1": []any{[]any{"00:00", "00:00"}},
	})); err == nil {
		t.Fatal("expected empty window rejection")
	}

	schedule, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Shanghai", map[string]any{
		"1": []any{[]any{"13:00", "14:00"}, []any{"09:00", "12:00"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-08-24 10:00", loc)
	next := schedule.NextStart(at)
	if next == nil || !next.Equal(time.Date(2026, 8, 24, 13, 0, 0, 0, loc)) {
		t.Fatalf("unexpected next start: %v", next)
	}
	at, _ = time.ParseInLocation("2006-01-02 15:04", "2026-08-24 14:00", loc)
	next = schedule.NextStart(at)
	if next == nil || !next.Equal(time.Date(2026, 8, 31, 9, 0, 0, 0, loc)) {
		t.Fatalf("unexpected next weekly start: %v", next)
	}

	withoutConfig, err := ParseSchedulingSchedule(nil)
	if err != nil || !withoutConfig.IsWithin(time.Now()) {
		t.Fatal("missing configuration must remain unrestricted")
	}

	tokyo, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Tokyo", map[string]any{
		"1": []any{[]any{"09:00", "24:00"}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	utcMorning := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	if !tokyo.IsWithin(utcMorning) {
		t.Fatal("timezone conversion should evaluate in the configured location")
	}
}

func TestSchedulingScheduleMidnightEndDoesNotOverlapNextMorning(t *testing.T) {
	windows := map[string]any{}
	for day := 1; day <= 5; day++ {
		windows[fmt.Sprint(day)] = []any{
			[]any{"09:00", "12:00"},
			[]any{"14:00", "18:00"},
			[]any{"22:00", "00:00"},
			[]any{"00:00", "08:00"},
		}
	}
	schedule, err := ParseSchedulingSchedule(scheduleExtra(true, "Asia/Shanghai", windows))
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-08-25 15:22", loc)
	if !schedule.IsWithin(at) {
		t.Fatal("Tuesday afternoon should be within the configured window")
	}
}

func TestAccountScheduleStatusPriority(t *testing.T) {
	at := time.Date(2026, 8, 24, 20, 0, 0, 0, time.UTC)
	account := &Account{Status: StatusActive, Schedulable: true, Extra: scheduleExtra(true, "UTC", map[string]any{
		"1": []any{[]any{"09:00", "10:00"}},
	})}
	if got := account.ScheduleStatus(at); got != ScheduleStatusOutside {
		t.Fatalf("got %s", got)
	}
	account.Schedulable = false
	if got := account.ScheduleStatus(at); got != ScheduleStatusManual {
		t.Fatalf("manual status got %s", got)
	}
	account.Status = "inactive"
	if got := account.ScheduleStatus(at); got != ScheduleStatusInactive {
		t.Fatalf("inactive status got %s", got)
	}
}

package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	SchedulingScheduleExtraKey = "scheduling_schedule"
	DefaultSchedulingTimezone  = "Asia/Shanghai"
	ScheduleStatusAvailable    = "available"
	ScheduleStatusOutside      = "outside_window"
	ScheduleStatusManual       = "manual_disabled"
	ScheduleStatusInactive     = "inactive"
	ScheduleStatusExpired      = "expired"
	ScheduleStatusRateLimited  = "rate_limited"
	ScheduleStatusOverloaded   = "overloaded"
	ScheduleStatusTemporary    = "temporarily_unschedulable"
)

type SchedulingSchedule struct {
	Enabled       bool                  `json:"enabled"`
	Timezone      string                `json:"timezone"`
	WeeklyWindows map[string][][]string `json:"weekly_windows"`
}

type schedulingWindow struct{ start, end int }

func ParseSchedulingSchedule(extra map[string]any) (*SchedulingSchedule, error) {
	raw, ok := extra[SchedulingScheduleExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", SchedulingScheduleExtraKey)
	}
	schedule := &SchedulingSchedule{WeeklyWindows: map[string][][]string{}}
	if enabled, exists := value["enabled"]; exists {
		var valid bool
		schedule.Enabled, valid = enabled.(bool)
		if !valid {
			return nil, fmt.Errorf("scheduling_schedule.enabled must be boolean")
		}
	}
	schedule.Timezone = DefaultSchedulingTimezone
	if timezone, exists := value["timezone"]; exists {
		var valid bool
		schedule.Timezone, valid = timezone.(string)
		if !valid || strings.TrimSpace(schedule.Timezone) == "" {
			return nil, fmt.Errorf("scheduling_schedule.timezone must be a valid IANA timezone")
		}
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil {
		return nil, fmt.Errorf("scheduling_schedule.timezone: %w", err)
	}
	if windows, exists := value["weekly_windows"]; exists {
		object, valid := windows.(map[string]any)
		if !valid {
			return nil, fmt.Errorf("scheduling_schedule.weekly_windows must be an object")
		}
		for day, rawWindows := range object {
			dayNumber, err := strconv.Atoi(day)
			if err != nil || dayNumber < 1 || dayNumber > 7 {
				return nil, fmt.Errorf("invalid scheduling weekday %q", day)
			}
			list, valid := rawWindows.([]any)
			if !valid {
				return nil, fmt.Errorf("scheduling weekday %s must be an array", day)
			}
			for _, rawWindow := range list {
				pair, valid := rawWindow.([]any)
				if !valid || len(pair) != 2 {
					return nil, fmt.Errorf("scheduling weekday %s window must contain start and end", day)
				}
				start, startOK := pair[0].(string)
				end, endOK := pair[1].(string)
				if !startOK || !endOK {
					return nil, fmt.Errorf("scheduling weekday %s times must be strings", day)
				}
				schedule.WeeklyWindows[day] = append(schedule.WeeklyWindows[day], []string{start, end})
			}
		}
	}
	if err := schedule.validate(); err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *SchedulingSchedule) validate() error {
	if s == nil || !s.Enabled {
		return nil
	}
	byDay := make(map[int][]schedulingWindow, 7)
	for day, windows := range s.WeeklyWindows {
		dayNumber, _ := strconv.Atoi(day)
		for _, pair := range windows {
			if pair[0] == pair[1] {
				return fmt.Errorf("scheduling weekday %s cannot use an empty window", day)
			}
			start, err := parseScheduleMinute(pair[0], false)
			if err != nil {
				return fmt.Errorf("scheduling weekday %s: %w", day, err)
			}
			end, err := parseScheduleMinute(pair[1], true)
			if err != nil {
				return fmt.Errorf("scheduling weekday %s: %w", day, err)
			}
			if end < start {
				byDay[dayNumber] = append(byDay[dayNumber], schedulingWindow{start, 1440})
				byDay[dayNumber%7+1] = append(byDay[dayNumber%7+1], schedulingWindow{0, end})
			} else {
				byDay[dayNumber] = append(byDay[dayNumber], schedulingWindow{start, end})
			}
		}
	}
	for day, windows := range byDay {
		sort.Slice(windows, func(i, j int) bool { return windows[i].start < windows[j].start })
		for i := 1; i < len(windows); i++ {
			if windows[i].start < windows[i-1].end {
				return fmt.Errorf("scheduling weekday %d contains overlapping windows", day)
			}
		}
	}
	return nil
}

func parseScheduleMinute(value string, allow24 bool) (int, error) {
	if allow24 && value == "00:00" {
		return 1440, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || minute < 0 || minute > 59 || hour < 0 || hour > 24 || (hour == 24 && (!allow24 || minute != 0)) {
		return 0, fmt.Errorf("invalid time %q", value)
	}
	return hour*60 + minute, nil
}

func (s *SchedulingSchedule) windowsForDay(day int) []schedulingWindow {
	if s == nil || !s.Enabled {
		return nil
	}
	result := make([]schedulingWindow, 0)
	for offset := 0; offset <= 1; offset++ {
		sourceDay := day - offset
		if sourceDay <= 0 {
			sourceDay += 7
		}
		for _, pair := range s.WeeklyWindows[strconv.Itoa(sourceDay)] {
			start, _ := parseScheduleMinute(pair[0], false)
			end, _ := parseScheduleMinute(pair[1], true)
			if offset == 0 && end > start {
				result = append(result, schedulingWindow{start, end})
			}
			if offset == 0 && end < start {
				result = append(result, schedulingWindow{start, 1440})
			}
			if offset == 1 && end < start {
				result = append(result, schedulingWindow{0, end})
			}
		}
	}
	return result
}

func (s *SchedulingSchedule) IsWithin(now time.Time) bool {
	if s == nil || !s.Enabled {
		return true
	}
	location, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return false
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	for _, window := range s.windowsForDay(int(local.Weekday()+6)%7 + 1) {
		if minute >= window.start && minute < window.end {
			return true
		}
	}
	return false
}

func (s *SchedulingSchedule) NextStart(now time.Time) *time.Time {
	if s == nil || !s.Enabled {
		return nil
	}
	location, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil
	}
	local := now.In(location)
	var next *time.Time
	for dayOffset := 0; dayOffset <= 7; dayOffset++ {
		date := local.AddDate(0, 0, dayOffset)
		day := int(date.Weekday()+6)%7 + 1
		for _, pair := range s.WeeklyWindows[strconv.Itoa(day)] {
			start, _ := parseScheduleMinute(pair[0], false)
			candidate := time.Date(date.Year(), date.Month(), date.Day(), start/60, start%60, 0, 0, location)
			if candidate.After(local) && (next == nil || candidate.Before(*next)) {
				candidateCopy := candidate
				next = &candidateCopy
			}
		}
	}
	return next
}

func (a *Account) SchedulingSchedule() (*SchedulingSchedule, error) {
	return ParseSchedulingSchedule(a.Extra)
}

func (a *Account) IsWithinSchedulingWindow(now time.Time) bool {
	schedule, err := a.SchedulingSchedule()
	return err == nil && schedule.IsWithin(now)
}

func (a *Account) NextSchedulingWindowStart(now time.Time) *time.Time {
	schedule, err := a.SchedulingSchedule()
	if err != nil {
		return nil
	}
	return schedule.NextStart(now)
}

func (a *Account) ScheduleStatus(now time.Time) string {
	if !a.IsActive() {
		return ScheduleStatusInactive
	}
	if !a.Schedulable {
		return ScheduleStatusManual
	}
	if a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return ScheduleStatusExpired
	}
	if a.RateLimitResetAt != nil && now.Before(*a.RateLimitResetAt) {
		return ScheduleStatusRateLimited
	}
	if a.OverloadUntil != nil && now.Before(*a.OverloadUntil) {
		return ScheduleStatusOverloaded
	}
	if a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil) {
		return ScheduleStatusTemporary
	}
	if !a.IsWithinSchedulingWindow(now) {
		return ScheduleStatusOutside
	}
	return ScheduleStatusAvailable
}

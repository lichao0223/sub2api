package nonworktime

import (
	"sort"
	"time"
)

func BuildYearCalendar(year int, holidays []HolidayDay, confirmed bool, cfg WorkdayConfig) []CalendarDay {
	cfg = cfg.Normalize()
	loc := cfg.Location
	start := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(1, 0, 0)
	byDate := make(map[string]CalendarDay)

	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		key := dateKey(d)
		weekend := isWeekend(d)
		dayType := DayTypeNormalWorkday
		if weekend {
			dayType = DayTypeNormalWeekend
		}
		if !confirmed {
			if weekend {
				dayType = DayTypePredictedWeekend
			} else {
				dayType = DayTypePredictedWorkday
			}
		}
		byDate[key] = CalendarDay{
			Date:          dateOnly(d, loc),
			Country:       cfg.DefaultCountry,
			IsWorkday:     !weekend,
			IsOffday:      weekend,
			IsWeekend:     weekend,
			DayType:       dayType,
			Source:        cfg.DefaultSource,
			SourceVersion: cfg.DefaultSourceVers,
			Confirmed:     confirmed,
		}
	}

	if confirmed {
		for _, h := range holidays {
			d := dateOnly(h.Date.In(loc), loc)
			if d.Year() != year {
				continue
			}
			key := dateKey(d)
			current, ok := byDate[key]
			if !ok {
				continue
			}
			current.HolidayName = h.Name
			current.Source = "holiday-cn"
			current.Raw = h.Raw
			current.Confirmed = true
			if h.IsOffDay {
				current.IsWorkday = false
				current.IsOffday = true
				current.DayType = DayTypeHolidayOffday
			} else {
				current.IsWorkday = true
				current.IsOffday = false
				current.DayType = DayTypeMakeupWorkday
			}
			byDate[key] = current
		}
	}

	keys := make([]string, 0, len(byDate))
	for key := range byDate {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]CalendarDay, 0, len(keys))
	for _, key := range keys {
		out = append(out, byDate[key])
	}
	return out
}

func CalendarMap(days []CalendarDay, loc *time.Location) map[string]CalendarDay {
	out := make(map[string]CalendarDay, len(days))
	for _, d := range days {
		out[dateKey(d.Date.In(loc))] = d
	}
	return out
}

func SegmentAt(t time.Time, calendar map[string]CalendarDay, cfg WorkdayConfig) (string, bool) {
	cfg = cfg.Normalize()
	local := t.In(cfg.Location)
	day, ok := calendar[dateKey(local)]
	if !ok {
		weekend := isWeekend(local)
		day = CalendarDay{
			Date:      dateOnly(local, cfg.Location),
			Country:   cfg.DefaultCountry,
			IsWorkday: !weekend,
			IsOffday:  weekend,
			IsWeekend: weekend,
			DayType:   DayTypePredictedWorkday,
			Confirmed: false,
		}
		if weekend {
			day.DayType = DayTypePredictedWeekend
		}
	}
	if day.IsOffday {
		return SegmentOffday, day.Confirmed
	}
	clock := time.Duration(local.Hour())*time.Hour + time.Duration(local.Minute())*time.Minute + time.Duration(local.Second())*time.Second + time.Duration(local.Nanosecond())
	if clock < cfg.WorkStart || clock >= cfg.WorkEnd {
		return SegmentAfterHours, day.Confirmed
	}
	return SegmentWorkHours, day.Confirmed
}

func dateOnly(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func dateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

package nonworktime

import "time"

const (
	CountryCN = "CN"

	DayTypeNormalWorkday    = "normal_workday"
	DayTypeNormalWeekend    = "normal_weekend"
	DayTypeHolidayOffday    = "holiday_offday"
	DayTypeMakeupWorkday    = "makeup_workday"
	DayTypeManualWorkday    = "manual_workday"
	DayTypeManualOffday     = "manual_offday"
	DayTypePredictedWorkday = "predicted_workday"
	DayTypePredictedWeekend = "predicted_weekend"

	SegmentWorkHours  = "work_hours"
	SegmentAfterHours = "after_hours"
	SegmentOffday     = "offday"
)

type HolidayDay struct {
	Name     string
	Date     time.Time
	IsOffDay bool
	Raw      []byte
}

type CalendarDay struct {
	Date           time.Time
	Country        string
	IsWorkday      bool
	IsOffday       bool
	IsWeekend      bool
	DayType        string
	HolidayName    string
	Source         string
	SourceVersion  string
	Confirmed      bool
	ManualOverride bool
	Raw            []byte
}

type WorkdayConfig struct {
	Location          *time.Location
	WorkStart        time.Duration
	WorkEnd          time.Duration
	ActiveGap        time.Duration
	MinSession       time.Duration
	DefaultCountry    string
	DefaultSource     string
	DefaultSourceVers string
}

func DefaultConfig() WorkdayConfig {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return WorkdayConfig{
		Location:       loc,
		WorkStart:     8*time.Hour + 30*time.Minute,
		WorkEnd:       18 * time.Hour,
		ActiveGap:     5 * time.Minute,
		MinSession:    time.Minute,
		DefaultCountry: CountryCN,
		DefaultSource:  "week_rule",
	}
}

func (c WorkdayConfig) Normalize() WorkdayConfig {
	def := DefaultConfig()
	if c.Location == nil {
		c.Location = def.Location
	}
	if c.WorkEnd <= c.WorkStart {
		c.WorkStart = def.WorkStart
		c.WorkEnd = def.WorkEnd
	}
	if c.ActiveGap <= 0 {
		c.ActiveGap = def.ActiveGap
	}
	if c.MinSession < 0 {
		c.MinSession = def.MinSession
	}
	if c.DefaultCountry == "" {
		c.DefaultCountry = def.DefaultCountry
	}
	if c.DefaultSource == "" {
		c.DefaultSource = def.DefaultSource
	}
	return c
}

type RequestPoint struct {
	UserID    int64
	RequestID string
	CreatedAt time.Time
}

type ActiveSegment struct {
	UserID int64
	Start  time.Time
	End    time.Time
}

type SegmentDuration struct {
	Date    time.Time
	Segment string
	Millis  int64
}

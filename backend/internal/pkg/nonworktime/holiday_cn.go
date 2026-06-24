package nonworktime

import (
	"encoding/json"
	"fmt"
	"time"
)

type holidayCNFile struct {
	Year   int            `json:"year"`
	Papers []holidayPaper `json:"papers"`
	Days   []holidayCNDay `json:"days"`
}

type holidayPaper struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type holidayCNDay struct {
	Name     string `json:"name"`
	Date     string `json:"date"`
	IsOffDay bool   `json:"isOffDay"`
}

type HolidayCNCalendar struct {
	Year          int
	Days          []HolidayDay
	SourceVersion string
	Raw           []byte
}

func ParseHolidayCNCalendar(data []byte, loc *time.Location) (HolidayCNCalendar, error) {
	if loc == nil {
		loc = DefaultConfig().Location
	}
	var file holidayCNFile
	if err := json.Unmarshal(data, &file); err != nil {
		return HolidayCNCalendar{}, fmt.Errorf("parse holiday-cn json: %w", err)
	}
	if file.Year <= 0 {
		return HolidayCNCalendar{}, fmt.Errorf("holiday-cn year is missing")
	}
	out := HolidayCNCalendar{
		Year:          file.Year,
		SourceVersion: holidayCNSourceVersion(file),
		Raw:           append([]byte(nil), data...),
		Days:          make([]HolidayDay, 0, len(file.Days)),
	}
	for _, d := range file.Days {
		parsed, err := time.ParseInLocation("2006-01-02", d.Date, loc)
		if err != nil {
			return HolidayCNCalendar{}, fmt.Errorf("parse holiday-cn day %q: %w", d.Date, err)
		}
		raw, _ := json.Marshal(d)
		out.Days = append(out.Days, HolidayDay{
			Name:     d.Name,
			Date:     parsed,
			IsOffDay: d.IsOffDay,
			Raw:      raw,
		})
	}
	return out, nil
}

func holidayCNSourceVersion(file holidayCNFile) string {
	if len(file.Papers) == 0 {
		return fmt.Sprintf("holiday-cn:%d", file.Year)
	}
	if file.Papers[0].URL != "" {
		return file.Papers[0].URL
	}
	if file.Papers[0].Name != "" {
		return file.Papers[0].Name
	}
	return fmt.Sprintf("holiday-cn:%d", file.Year)
}

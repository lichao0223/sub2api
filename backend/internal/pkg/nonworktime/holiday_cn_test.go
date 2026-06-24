package nonworktime

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHolidayCNCalendarAcceptsStringPapers(t *testing.T) {
	cfg := DefaultConfig()
	data := []byte(`{
		"year": 2026,
		"papers": ["https://www.gov.cn/zhengce/content/202511/content_123456.htm"],
		"days": [
			{"name": "元旦", "date": "2026-01-01", "isOffDay": true},
			{"name": "春节调休", "date": "2026-02-14", "isOffDay": false}
		]
	}`)

	calendar, err := ParseHolidayCNCalendar(data, cfg.Location)
	require.NoError(t, err)
	require.Equal(t, 2026, calendar.Year)
	require.Equal(t, "https://www.gov.cn/zhengce/content/202511/content_123456.htm", calendar.SourceVersion)
	require.Len(t, calendar.Days, 2)
	require.True(t, calendar.Days[0].IsOffDay)
	require.False(t, calendar.Days[1].IsOffDay)
}

func TestParseHolidayCNCalendarAcceptsObjectPapers(t *testing.T) {
	cfg := DefaultConfig()
	data := []byte(`{
		"year": 2026,
		"papers": [{"name": "国务院办公厅通知", "url": "https://www.gov.cn/object-paper"}],
		"days": [{"name": "元旦", "date": "2026-01-01", "isOffDay": true}]
	}`)

	calendar, err := ParseHolidayCNCalendar(data, cfg.Location)
	require.NoError(t, err)
	require.Equal(t, "https://www.gov.cn/object-paper", calendar.SourceVersion)
	require.Len(t, calendar.Days, 1)
}

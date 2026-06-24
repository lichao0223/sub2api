package nonworktime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildActiveSegmentsGapAndSingleRequest(t *testing.T) {
	cfg := DefaultConfig()
	points := []RequestPoint{
		{UserID: 1, RequestID: "a", CreatedAt: mustTime(t, "2026-06-18T08:00:00+08:00")},
		{UserID: 1, RequestID: "b", CreatedAt: mustTime(t, "2026-06-18T08:03:00+08:00")},
		{UserID: 1, RequestID: "c", CreatedAt: mustTime(t, "2026-06-18T08:10:00+08:00")},
		{UserID: 1, RequestID: "d", CreatedAt: mustTime(t, "2026-06-18T08:13:00+08:00")},
		{UserID: 2, RequestID: "single", CreatedAt: mustTime(t, "2026-06-18T20:00:00+08:00")},
	}

	segments := BuildActiveSegments(points, cfg)

	require.Len(t, segments, 3)
	require.Equal(t, 3*time.Minute, segments[0].End.Sub(segments[0].Start))
	require.Equal(t, 3*time.Minute, segments[1].End.Sub(segments[1].Start))
	require.Equal(t, time.Minute, segments[2].End.Sub(segments[2].Start))
}

func TestBuildActiveSegmentsDedupesRequestID(t *testing.T) {
	cfg := DefaultConfig()
	points := []RequestPoint{
		{UserID: 1, RequestID: "same", CreatedAt: mustTime(t, "2026-06-18T08:00:00+08:00")},
		{UserID: 1, RequestID: "same", CreatedAt: mustTime(t, "2026-06-18T08:01:00+08:00")},
	}

	segments := BuildActiveSegments(points, cfg)

	require.Len(t, segments, 1)
	require.Equal(t, time.Minute, segments[0].End.Sub(segments[0].Start))
}

func TestSplitActiveSegmentAcrossWorkEnd(t *testing.T) {
	cfg := DefaultConfig()
	calendar := CalendarMap(BuildYearCalendar(2026, nil, true, cfg), cfg.Location)
	seg := ActiveSegment{
		UserID: 1,
		Start:  mustTime(t, "2026-06-18T17:58:00+08:00"),
		End:    mustTime(t, "2026-06-18T18:03:00+08:00"),
	}

	parts := SplitActiveSegment(seg, calendar, cfg)

	require.Len(t, parts, 2)
	require.Equal(t, SegmentWorkHours, parts[0].Segment)
	require.Equal(t, int64((2 * time.Minute).Milliseconds()), parts[0].Millis)
	require.Equal(t, SegmentAfterHours, parts[1].Segment)
	require.Equal(t, int64((3 * time.Minute).Milliseconds()), parts[1].Millis)
}

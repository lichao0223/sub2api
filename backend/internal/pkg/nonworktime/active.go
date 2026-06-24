package nonworktime

import (
	"sort"
	"strconv"
	"time"
)

func BuildActiveSegments(points []RequestPoint, cfg WorkdayConfig) []ActiveSegment {
	cfg = cfg.Normalize()
	if len(points) == 0 {
		return nil
	}
	points = dedupeAndSortPoints(points)

	segments := make([]ActiveSegment, 0)
	var currentUser int64
	var sessionStart time.Time
	var last time.Time
	requestCount := 0

	flush := func() {
		if requestCount == 0 {
			return
		}
		end := last
		if requestCount == 1 && cfg.MinSession > 0 {
			end = sessionStart.Add(cfg.MinSession)
		}
		if end.After(sessionStart) {
			segments = append(segments, ActiveSegment{
				UserID: currentUser,
				Start:  sessionStart,
				End:    end,
			})
		}
	}

	for _, p := range points {
		if p.CreatedAt.IsZero() {
			continue
		}
		if requestCount == 0 || p.UserID != currentUser {
			flush()
			currentUser = p.UserID
			sessionStart = p.CreatedAt
			last = p.CreatedAt
			requestCount = 1
			continue
		}
		if !p.CreatedAt.Before(last) && p.CreatedAt.Sub(last) <= cfg.ActiveGap {
			last = p.CreatedAt
			requestCount++
			continue
		}
		flush()
		sessionStart = p.CreatedAt
		last = p.CreatedAt
		requestCount = 1
	}
	flush()

	return segments
}

func SplitActiveSegment(seg ActiveSegment, calendar map[string]CalendarDay, cfg WorkdayConfig) []SegmentDuration {
	cfg = cfg.Normalize()
	if !seg.End.After(seg.Start) {
		return nil
	}
	out := make([]SegmentDuration, 0)
	cur := seg.Start.In(cfg.Location)
	end := seg.End.In(cfg.Location)
	for cur.Before(end) {
		next := nextBoundary(cur, end, cfg)
		segment, _ := SegmentAt(cur, calendar, cfg)
		out = append(out, SegmentDuration{
			Date:    dateOnly(cur, cfg.Location),
			Segment: segment,
			Millis:  next.Sub(cur).Milliseconds(),
		})
		cur = next
	}
	return out
}

func dedupeAndSortPoints(points []RequestPoint) []RequestPoint {
	sort.Slice(points, func(i, j int) bool {
		if points[i].UserID != points[j].UserID {
			return points[i].UserID < points[j].UserID
		}
		if !points[i].CreatedAt.Equal(points[j].CreatedAt) {
			return points[i].CreatedAt.Before(points[j].CreatedAt)
		}
		return points[i].RequestID < points[j].RequestID
	})

	out := make([]RequestPoint, 0, len(points))
	seen := make(map[string]struct{}, len(points))
	for _, p := range points {
		if p.RequestID != "" {
			key := strconv.FormatInt(p.UserID, 10) + ":" + p.RequestID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, p)
	}
	return out
}

func nextBoundary(cur, end time.Time, cfg WorkdayConfig) time.Time {
	localDate := dateOnly(cur, cfg.Location)
	candidates := []time.Time{
		localDate.AddDate(0, 0, 1),
		localDate.Add(cfg.WorkStart),
		localDate.Add(cfg.WorkEnd),
	}
	next := end
	for _, c := range candidates {
		if c.After(cur) && c.Before(next) {
			next = c
		}
	}
	return next
}

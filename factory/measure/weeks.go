package main

import (
	"slices"
	"time"
)

const week = 7 * 24 * time.Hour

// weekStart is the Monday 00:00 UTC that opens t's week, the same week the
// ledger's measures use (GET /work-items/measures).
func weekStart(t time.Time) time.Time {
	u := t.UTC()
	day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	back := (int(day.Weekday()) + 6) % 7 // Monday 0 … Sunday 6
	return day.AddDate(0, 0, -back)
}

// Window is the weeks a report covers: [Start, End), both Mondays.
type Window struct{ Start, End time.Time }

// newWindow opens on the week holding since. With weeks > 0 it covers that
// many; otherwise it runs through the week holding now.
func newWindow(since time.Time, weeks int, now time.Time) Window {
	start := weekStart(since)
	end := weekStart(now).Add(week)
	if weeks > 0 {
		end = start.Add(time.Duration(weeks) * week)
	}
	return Window{Start: start, End: end}
}

func (w Window) contains(t time.Time) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

// weeks lists the window's weeks, newest first, as the console does.
func (w Window) weeks() []time.Time {
	var out []time.Time
	for s := w.End.Add(-week); !s.Before(w.Start); s = s.Add(-week) {
		out = append(out, s)
	}
	return out
}

// percentile is the nearest-rank percentile of d: the smallest value with at
// least p percent of the values at or below it. The median of an even count
// is therefore the lower middle value, never an average of two stays.
func percentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sorted := slices.Clone(d)
	slices.Sort(sorted)
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank, 1)-1]
}

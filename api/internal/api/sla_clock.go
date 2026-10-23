package api

import (
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

// maxClockDays bounds the walk over the calendar; a target that needs more open days than this
// falls back to plain time.
const maxClockDays = 100_000

type clockSpan struct{ start, end int }

// businessClock is an inbox's weekly business hours in its timezone, as minutes from local
// midnight per weekday; plain means every second counts.
type businessClock struct {
	loc   *time.Location
	days  [7][]clockSpan
	plain bool
}

func clockMinutes(v string) (int, bool) {
	if len(v) != 5 || v[2] != ':' {
		return 0, false
	}
	h, err1 := strconv.Atoi(v[:2])
	m, err2 := strconv.Atoi(v[3:])
	if err1 != nil || err2 != nil || h > 24 || m > 59 || (h == 24 && m != 0) {
		return 0, false
	}
	return h*60 + m, true
}

func inboxClock(in store.Inbox) businessClock {
	var h oas.BusinessHours
	if json.Unmarshal(in.BusinessHours, &h) != nil || !h.Enabled {
		return businessClock{plain: true}
	}
	loc, err := time.LoadLocation(in.Timezone)
	if err != nil {
		loc = time.UTC
	}
	c := businessClock{loc: loc}
	open := false
	for _, iv := range h.Intervals {
		day := slices.Index(weekdayNames[:], iv.Day)
		s, ok1 := clockMinutes(iv.Start)
		e, ok2 := clockMinutes(iv.End)
		if day < 0 || !ok1 || !ok2 || e <= s {
			continue
		}
		c.days[day] = append(c.days[day], clockSpan{s, e})
		open = true
	}
	if !open {
		return businessClock{plain: true}
	}
	for d := range c.days {
		spans := c.days[d]
		slices.SortFunc(spans, func(a, b clockSpan) int { return a.start - b.start })
		var merged []clockSpan
		for _, sp := range spans {
			if n := len(merged); n > 0 && sp.start <= merged[n-1].end {
				merged[n-1].end = max(merged[n-1].end, sp.end)
				continue
			}
			merged = append(merged, sp)
		}
		c.days[d] = merged
	}
	return c
}

// wallTime is the instant the wall clock in loc shows y-m-d plus mins; a wall time that does not
// exist (skipped by a clock change) moves forward to the instant the clock jumps.
func wallTime(y int, m time.Month, d, mins int, loc *time.Location) time.Time {
	t := time.Date(y, m, d, 0, mins, 0, 0, loc)
	want := time.Date(y, m, d, 0, mins, 0, 0, time.UTC)
	got := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	switch {
	case got.After(want):
		start, _ := t.ZoneBounds()
		return start
	case got.Before(want):
		_, end := t.ZoneBounds()
		return end
	}
	return t
}

// spans calls fn with each open span of the civil days from the one holding from onwards, as
// absolute instants, until fn returns false. Days are counted on the calendar, not by adding
// hours, so a day whose midnight does not exist is neither skipped nor counted twice.
func (c businessClock) spans(from time.Time, fn func(s, e time.Time) bool) bool {
	y, m, d := from.In(c.loc).Date()
	first := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	for i := 0; i < maxClockDays; i++ {
		day := first.AddDate(0, 0, i)
		dy, dm, dd := day.Date()
		for _, sp := range c.days[day.Weekday()] {
			s := wallTime(dy, dm, dd, sp.start, c.loc)
			e := wallTime(dy, dm, dd, sp.end, c.loc)
			if !e.After(s) {
				continue
			}
			if !fn(s, e) {
				return true
			}
		}
	}
	return false
}

func (c businessClock) add(from time.Time, d time.Duration) time.Time {
	if c.plain || d <= 0 {
		return from.Add(max(d, 0))
	}
	var at time.Time
	left := d
	if !c.spans(from, func(s, e time.Time) bool {
		if !e.After(from) {
			return true
		}
		if s.Before(from) {
			s = from
		}
		if avail := e.Sub(s); avail < left {
			left -= avail
			return true
		}
		at = s.Add(left)
		return false
	}) {
		return from.Add(d)
	}
	return at
}

func (c businessClock) between(a, b time.Time) time.Duration {
	if !b.After(a) {
		return 0
	}
	if c.plain {
		return b.Sub(a)
	}
	var sum time.Duration
	c.spans(a, func(s, e time.Time) bool {
		if !s.Before(b) {
			return false
		}
		if s.Before(a) {
			s = a
		}
		if e.After(b) {
			e = b
		}
		if e.After(s) {
			sum += e.Sub(s)
		}
		return true
	})
	return sum
}

// addBusinessSeconds is when secs seconds of the inbox's business hours have passed after from,
// counted in its timezone; with business hours off it is plain time. Seconds are real elapsed
// time, so a span that crosses a clock change counts what really passed.
func addBusinessSeconds(in store.Inbox, from time.Time, secs int64) time.Time {
	return inboxClock(in).add(from, time.Duration(secs)*time.Second)
}

// businessSecondsBetween counts the seconds of the inbox's business hours from a until b.
func businessSecondsBetween(in store.Inbox, a, b time.Time) int64 {
	return int64(inboxClock(in).between(a, b) / time.Second)
}

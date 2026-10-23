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

// spans calls fn with each open span of the local days from the one holding from onwards, as
// absolute instants, until fn returns false.
func (c businessClock) spans(from time.Time, fn func(s, e time.Time) bool) bool {
	t := from.In(c.loc)
	y, m, d := t.Date()
	for i := 0; i < maxClockDays; i++ {
		day := time.Date(y, m, d+i, 0, 0, 0, 0, c.loc)
		for _, sp := range c.days[day.Weekday()] {
			dy, dm, dd := day.Date()
			s := time.Date(dy, dm, dd, 0, sp.start, 0, 0, c.loc)
			e := time.Date(dy, dm, dd, 0, sp.end, 0, 0, c.loc)
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

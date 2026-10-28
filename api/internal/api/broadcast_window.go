package api

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const minSendWindow = 30

var (
	zoneNamePattern  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+-]*(/[A-Za-z][A-Za-z0-9_+-]*)*$`)
	windowStartClock = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	localDateTime    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T([01][0-9]|2[0-3]):[0-5][0-9]$`)
	zones            sync.Map
)

// zone loads an IANA zone from the embedded database once per name. Every path segment starts with a
// letter, so spellings of one file such as Europe//Berlin never reach the cache.
func zone(name string) (*time.Location, error) {
	if l, ok := zones.Load(name); ok {
		return l.(*time.Location), nil
	}
	if name == "" || len(name) > 64 || name == "Local" || !zoneNamePattern.MatchString(name) {
		return nil, fmt.Errorf("not a zone name: %q", name)
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	zones.Store(name, l)
	return l, nil
}

func checkZone(name string) (string, error) {
	if _, err := zone(name); err != nil {
		return "", errValidation("timezone must be an IANA time zone name, such as Europe/Istanbul")
	}
	return name, nil
}

func zoneOf(b store.Broadcast) *time.Location {
	if l, err := zone(b.Timezone); err == nil {
		return l
	}
	return time.UTC
}

type sendWindow struct {
	loc        *time.Location
	start, end int
}

func windowOf(b store.Broadcast) (sendWindow, bool) {
	if b.SendWindowStart == nil || b.SendWindowEnd == nil {
		return sendWindow{}, false
	}
	return sendWindow{loc: zoneOf(b), start: int(*b.SendWindowStart), end: int(*b.SendWindowEnd)}, true
}

// day is the window that opens on the local date y-m-d.
func (w sendWindow) day(y int, m time.Month, d int) (opens, closes time.Time) {
	end := w.end
	if end <= w.start {
		end += 1440
	}
	return resolveLocal(w.loc, y, m, d, w.start, false), resolveLocal(w.loc, y, m, d, end, true)
}

// at says whether now is inside a window, and gives that window, or the next one when it is closed.
func (w sendWindow) at(now time.Time) (open bool, opens, closes time.Time) {
	l := now.In(w.loc)
	for i := -1; i <= 2; i++ {
		o, c := w.day(l.Year(), l.Month(), l.Day()+i)
		if !now.Before(o) && now.Before(c) {
			return true, o, c
		}
		if o.After(now) {
			return false, o, c
		}
	}
	o, c := w.day(l.Year(), l.Month(), l.Day()+3)
	return false, o, c
}

// length is the real length of the current or next window: an hour more or less when the clocks
// change inside it.
func (w sendWindow) length(now time.Time) time.Duration {
	_, o, c := w.at(now)
	return c.Sub(o)
}

func (w sendWindow) body() *oas.BroadcastSendWindow {
	return &oas.BroadcastSendWindow{Start: clockOf(w.start), End: clockOf(w.end)}
}

func clockOf(minute int) string {
	return fmt.Sprintf("%02d:%02d", minute/60, minute%60)
}

// resolveLocal is the instant of a wall-clock time, minute minutes after local midnight of y-m-d
// (1440 and more run into the next days). A time the clocks skip moves forward by the jump; a time
// that happens twice is the first occurrence, or the second when last is set. time.Date leaves
// that choice unspecified.
func resolveLocal(loc *time.Location, y int, m time.Month, d, minute int, last bool) time.Time {
	wall := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Add(time.Duration(minute) * time.Minute)
	offset := func(t time.Time) time.Duration {
		_, o := t.In(loc).Zone()
		return time.Duration(o) * time.Second
	}
	before, after := offset(wall.Add(-24*time.Hour)), offset(wall.Add(24*time.Hour))
	var found []time.Time
	for _, o := range []time.Duration{before, after} {
		t := wall.Add(-o)
		if offset(t) == o && (len(found) == 0 || !found[0].Equal(t)) {
			found = append(found, t)
		}
	}
	switch len(found) {
	case 0:
		return wall.Add(-before).In(loc)
	case 1:
		return found[0].In(loc)
	}
	first, second := found[0], found[1]
	if second.Before(first) {
		first, second = second, first
	}
	if last {
		return second.In(loc)
	}
	return first.In(loc)
}

func parseLocalDateTime(s string, loc *time.Location) (time.Time, error) {
	if !localDateTime.MatchString(s) {
		return time.Time{}, errValidation("send_at_local must be a local time such as 2026-11-02T09:00")
	}
	t, err := time.Parse("2006-01-02T15:04", s)
	if err != nil {
		return time.Time{}, errValidation("send_at_local must be a local time such as 2026-11-02T09:00")
	}
	return resolveLocal(loc, t.Year(), t.Month(), t.Day(), t.Hour()*60+t.Minute(), false), nil
}

// checkSendWindow reads a window's HH:MM times as minutes after local midnight.
func checkSendWindow(in oas.BroadcastSendWindow) (start, end int16, err error) {
	bad := errValidation("send_window needs start and end as HH:MM (end 24:00 at the latest), at least 30 minutes apart")
	if !windowStartClock.MatchString(in.Start) || !clockPattern.MatchString(in.End) {
		return 0, 0, bad
	}
	minutes := func(s string) int {
		h, _ := strconv.Atoi(s[:2])
		m, _ := strconv.Atoi(s[3:])
		return h*60 + m
	}
	s, e := minutes(in.Start), minutes(in.End)
	if e == 0 {
		e = 1440
	}
	if s == e%1440 || (e-s+1440)%1440 < minSendWindow {
		return 0, 0, bad
	}
	return int16(s), int16(e), nil
}

// windowOpensAt is when a closed window opens next for a broadcast that is waiting to send: from
// now, or from its start while that is later.
func windowOpensAt(b store.Broadcast, now time.Time) *time.Time {
	w, ok := windowOf(b)
	if !ok {
		return nil
	}
	switch b.State {
	case bcScheduled, bcPreparing, bcSending, bcActive:
	default:
		return nil
	}
	if b.State == bcScheduled && b.ScheduledAt != nil && b.ScheduledAt.After(now) {
		now = *b.ScheduledAt
	}
	if open, opens, _ := w.at(now); !open {
		return &opens
	}
	return nil
}

// gmtOffset names a zone's offset at an instant as browsers do: GMT, GMT+3, GMT-3:30.
func gmtOffset(t time.Time) string {
	_, secs := t.Zone()
	if secs == 0 {
		return "GMT"
	}
	sign := "+"
	if secs < 0 {
		sign, secs = "-", -secs
	}
	out := "GMT" + sign + strconv.Itoa(secs/3600)
	if m := secs % 3600 / 60; m > 0 {
		out += fmt.Sprintf(":%02d", m)
	}
	return out
}

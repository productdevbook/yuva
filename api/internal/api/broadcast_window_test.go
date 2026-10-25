package api

import (
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func mustZone(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := zone(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestResolveLocalAcrossClockChanges(t *testing.T) {
	berlin := mustZone(t, "Europe/Berlin")
	for _, c := range []struct {
		name   string
		y      int
		m      time.Month
		d, min int
		last   bool
		want   string
	}{
		{"spring forward gap moves on by the jump", 2027, time.March, 28, 2*60 + 30, false, "2027-03-28T01:30:00Z"},
		{"gap ignores last", 2027, time.March, 28, 2*60 + 30, true, "2027-03-28T01:30:00Z"},
		{"just before the gap", 2027, time.March, 28, 1*60 + 59, false, "2027-03-28T00:59:00Z"},
		{"just after the gap", 2027, time.March, 28, 3 * 60, false, "2027-03-28T01:00:00Z"},
		{"fall back first occurrence", 2026, time.October, 25, 2*60 + 30, false, "2026-10-25T00:30:00Z"},
		{"fall back second occurrence", 2026, time.October, 25, 2*60 + 30, true, "2026-10-25T01:30:00Z"},
		{"ordinary winter day", 2027, time.January, 10, 9 * 60, false, "2027-01-10T08:00:00Z"},
		{"ordinary summer day", 2027, time.July, 10, 9 * 60, true, "2027-07-10T07:00:00Z"},
		{"24:00 is the next midnight", 2027, time.March, 27, 1440, false, "2027-03-27T23:00:00Z"},
	} {
		if got := resolveLocal(berlin, c.y, c.m, c.d, c.min, c.last); !got.Equal(utc(c.want)) {
			t.Errorf("%s: %v, want %s", c.name, got.UTC(), c.want)
		}
	}
	if got := resolveLocal(berlin, 2027, time.March, 28, 2*60+30, false); got.Format("15:04 MST") != "03:30 CEST" {
		t.Errorf("02:30 on the spring-forward day reads %s, want 03:30 CEST", got.Format("15:04 MST"))
	}
}

func TestResolveLocalWithoutDST(t *testing.T) {
	istanbul := mustZone(t, "Europe/Istanbul")
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for day := 0; day < 366; day += 7 {
		d := start.AddDate(0, 0, day)
		for minute := 0; minute < 1440; minute += 37 {
			got := resolveLocal(istanbul, d.Year(), d.Month(), d.Day(), minute, minute%2 == 0)
			l := got.In(istanbul)
			if l.Year() != d.Year() || l.Month() != d.Month() || l.Day() != d.Day() || l.Hour()*60+l.Minute() != minute {
				t.Fatalf("%s minute %d came back as %v", d.Format("2006-01-02"), minute, l)
			}
			if got.Sub(time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).Add(time.Duration(minute)*time.Minute)) != -3*time.Hour {
				t.Fatalf("%s minute %d is not UTC+3: %v", d.Format("2006-01-02"), minute, got.UTC())
			}
		}
	}
	at, err := parseLocalDateTime("2026-11-02T09:00", istanbul)
	if err != nil || !at.Equal(utc("2026-11-02T06:00:00Z")) {
		t.Fatalf("send_at_local 2026-11-02T09:00 Istanbul: %v %v", at.UTC(), err)
	}
	for _, bad := range []string{"2026-11-02 09:00", "2026-11-02T9:00", "2026-11-02T24:00", "2026-02-30T09:00", "2026-11-02T09:00:00", "2026-11-02T09:00Z"} {
		if _, err := parseLocalDateTime(bad, istanbul); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestSendWindowAt(t *testing.T) {
	berlin := mustZone(t, "Europe/Berlin")
	type check struct {
		now          string
		open         bool
		opens, close string
	}
	for _, c := range []struct {
		name       string
		start, end int
		checks     []check
	}{
		{"crosses midnight", 22 * 60, 6 * 60, []check{
			{"2027-01-10T22:00:00Z", true, "2027-01-10T21:00:00Z", "2027-01-11T05:00:00Z"},
			{"2027-01-11T02:00:00Z", true, "2027-01-10T21:00:00Z", "2027-01-11T05:00:00Z"},
			{"2027-01-11T05:00:00Z", false, "2027-01-11T21:00:00Z", "2027-01-12T05:00:00Z"},
			{"2027-01-11T12:00:00Z", false, "2027-01-11T21:00:00Z", "2027-01-12T05:00:00Z"},
			{"2027-01-11T20:59:59Z", false, "2027-01-11T21:00:00Z", "2027-01-12T05:00:00Z"},
		}},
		{"morning across spring forward", 5 * 60, 12 * 60, []check{
			{"2027-03-27T03:59:00Z", false, "2027-03-27T04:00:00Z", "2027-03-27T11:00:00Z"},
			{"2027-03-27T10:59:00Z", true, "2027-03-27T04:00:00Z", "2027-03-27T11:00:00Z"},
			{"2027-03-27T11:00:00Z", false, "2027-03-28T03:00:00Z", "2027-03-28T10:00:00Z"},
			{"2027-03-28T03:00:00Z", true, "2027-03-28T03:00:00Z", "2027-03-28T10:00:00Z"},
		}},
		{"spans the spring-forward jump", 1 * 60, 4 * 60, []check{
			{"2027-03-27T23:30:00Z", false, "2027-03-28T00:00:00Z", "2027-03-28T02:00:00Z"},
			{"2027-03-28T01:30:00Z", true, "2027-03-28T00:00:00Z", "2027-03-28T02:00:00Z"},
		}},
		{"opens inside the spring-forward gap", 2*60 + 30, 5 * 60, []check{
			{"2027-03-28T01:00:00Z", false, "2027-03-28T01:30:00Z", "2027-03-28T03:00:00Z"},
			{"2027-03-28T01:30:00Z", true, "2027-03-28T01:30:00Z", "2027-03-28T03:00:00Z"},
		}},
		{"spans the fall-back hour", 1 * 60, 4 * 60, []check{
			{"2026-10-24T22:00:00Z", false, "2026-10-24T23:00:00Z", "2026-10-25T03:00:00Z"},
			{"2026-10-25T02:30:00Z", true, "2026-10-24T23:00:00Z", "2026-10-25T03:00:00Z"},
		}},
		{"closes at a doubled time", 0, 2*60 + 30, []check{
			{"2026-10-25T01:15:00Z", true, "2026-10-24T22:00:00Z", "2026-10-25T01:30:00Z"},
			{"2026-10-25T01:30:00Z", false, "2026-10-25T23:00:00Z", "2026-10-26T01:30:00Z"},
		}},
	} {
		w := sendWindow{loc: berlin, start: c.start, end: c.end}
		for _, k := range c.checks {
			open, opens, closes := w.at(utc(k.now))
			if open != k.open || !opens.Equal(utc(k.opens)) || !closes.Equal(utc(k.close)) {
				t.Errorf("%s at %s: open %v %v–%v, want %v %s–%s", c.name, k.now, open, opens.UTC(), closes.UTC(), k.open, k.opens, k.close)
			}
		}
	}
	for _, c := range []struct {
		start, end int
		at         string
		want       time.Duration
	}{
		{60, 240, "2027-03-27T12:00:00Z", 2 * time.Hour},
		{60, 240, "2026-10-24T12:00:00Z", 4 * time.Hour},
		{300, 720, "2027-03-27T12:00:00Z", 7 * time.Hour},
		{0, 150, "2026-10-24T12:00:00Z", 3*time.Hour + 30*time.Minute},
	} {
		if got := (sendWindow{loc: berlin, start: c.start, end: c.end}).length(utc(c.at)); got != c.want {
			t.Errorf("window %d–%d next after %s lasts %v, want %v", c.start, c.end, c.at, got, c.want)
		}
	}
}

func TestCheckSendWindow(t *testing.T) {
	for _, c := range []struct {
		start, end string
		ok         bool
	}{
		{"05:00", "12:00", true}, {"05:00", "05:30", true}, {"05:00", "05:29", false}, {"00:00", "24:00", false},
		{"22:00", "06:00", true}, {"23:45", "00:15", true}, {"23:45", "00:10", false}, {"10:00", "10:00", false},
		{"24:00", "06:00", false}, {"5:00", "12:00", false}, {"05:00", "24:01", false}, {"00:30", "24:00", true},
		{"05:00", "00:00", true}, {"00:00", "00:00", false}, {"23:45", "00:00", false},
	} {
		_, _, err := checkSendWindow(oas.BroadcastSendWindow{Start: c.start, End: c.end})
		if (err == nil) != c.ok {
			t.Errorf("%s–%s: %v", c.start, c.end, err)
		}
	}
	if _, end, err := checkSendWindow(oas.BroadcastSendWindow{Start: "18:00", End: "00:00"}); err != nil || end != 1440 {
		t.Errorf("end 00:00 is stored as %d (%v), want 1440", end, err)
	}
	for _, name := range []string{"", "Local", "+03:00", "Mars/Olympus", "../etc/passwd", "Europe/Istanbul ", "Europe//Istanbul", "Europe/Istanbul/", "/Europe/Istanbul", "Europe/_Istanbul"} {
		if _, err := checkZone(name); err == nil {
			t.Errorf("zone %q was accepted", name)
		}
	}
	for _, name := range []string{"UTC", "Europe/Istanbul", "America/Argentina/Buenos_Aires", "Etc/GMT+3"} {
		if _, err := checkZone(name); err != nil {
			t.Errorf("zone %q refused: %v", name, err)
		}
	}
}

func TestPaceInsideSendWindow(t *testing.T) {
	start, end, perDay := int16(300), int16(720), int32(100)
	b := store.Broadcast{Timezone: "Europe/Berlin", SendWindowStart: &start, SendWindowEnd: &end, PacePerDay: &perDay}
	now := utc("2027-03-25T00:00:00Z")
	if g := paceGap(b, now); g != 252*time.Second {
		t.Fatalf("gap with a 7-hour window and 100 a day: %v", g)
	}
	wait := float64(4 * 3600)
	secs, days := sendingSeconds(b, 500, 100, now)
	if days != 5 || secs != wait+4*86400+99*252 {
		t.Fatalf("500 mails: %v seconds over %d days", secs, days)
	}
	if secs, days := sendingSeconds(b, 100, 100, now); days != 1 || secs != wait+99*252 {
		t.Fatalf("100 mails: %v seconds over %d days", secs, days)
	}
	later := utc("2027-03-26T11:00:00Z")
	b.ScheduledAt = &later
	if secs, _ := sendingSeconds(b, 100, 100, now); secs != 17*3600+99*252 {
		t.Fatalf("scheduled at 12:00, when the window closes: %v seconds", secs)
	}
	b.ScheduledAt = nil
	shortEnd := int16(330)
	narrow := store.Broadcast{Timezone: "Europe/Berlin", SendWindowStart: &start, SendWindowEnd: &shortEnd}
	if secs, _ := sendingSeconds(narrow, 1_000_000, 1, now); !paceLimited(narrow, now) || secs <= maxPaceDuration.Seconds() {
		t.Fatalf("a 30-minute window without a pace at one mail a second: limited %v, %v seconds", paceLimited(narrow, now), secs)
	}
	if paceLimited(store.Broadcast{Timezone: "Europe/Berlin"}, now) {
		t.Fatal("no pace and no window is not limited")
	}
	b.SendWindowStart, b.SendWindowEnd = nil, nil
	if g := paceGap(b, now); g != 864*time.Second {
		t.Fatalf("gap without a window: %v", g)
	}
	for _, c := range []struct {
		zone, at, want string
	}{
		{"Europe/Istanbul", "2026-11-02T06:00:00Z", "GMT+3"}, {"Asia/Kolkata", "2026-11-02T06:00:00Z", "GMT+5:30"},
		{"UTC", "2026-11-02T06:00:00Z", "GMT"}, {"America/New_York", "2027-01-10T06:00:00Z", "GMT-5"},
		{"America/St_Johns", "2027-01-10T06:00:00Z", "GMT-3:30"},
	} {
		if got := gmtOffset(utc(c.at).In(mustZone(t, c.zone))); got != c.want {
			t.Errorf("%s: %s, want %s", c.zone, got, c.want)
		}
	}
}

package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func hoursInbox(t *testing.T, tz string, enabled bool, ivs ...oas.BusinessHoursInterval) store.Inbox {
	t.Helper()
	b, err := json.Marshal(oas.BusinessHours{Enabled: enabled, Intervals: ivs})
	if err != nil {
		t.Fatal(err)
	}
	return store.Inbox{Timezone: tz, BusinessHours: b}
}

func officeDays(start, end string) []oas.BusinessHoursInterval {
	var out []oas.BusinessHoursInterval
	for _, d := range []oas.Weekday{oas.Mon, oas.Tue, oas.Wed, oas.Thu, oas.Fri} {
		out = append(out, oas.BusinessHoursInterval{Day: d, Start: start, End: end})
	}
	return out
}

func localAt(t *testing.T, tz, v string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	r, err := time.ParseInLocation("2006-01-02 15:04", v, loc)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAddBusinessSeconds(t *testing.T) {
	const ist, ny, berlin = "Europe/Istanbul", "America/New_York", "Europe/Berlin"
	office := hoursInbox(t, ist, true, officeDays("09:00", "17:00")...)
	cases := []struct {
		name string
		in   store.Inbox
		tz   string
		from string
		secs int64
		want string
	}{
		{"inside hours", office, ist, "2026-10-21 10:00", 3600, "2026-10-21 11:00"},
		{"before opening starts at opening", office, ist, "2026-10-21 07:30", 1800, "2026-10-21 09:30"},
		{"runs over the evening into the next morning", office, ist, "2026-10-21 16:00", 2 * 3600, "2026-10-22 10:00"},
		{"lands exactly on closing", office, ist, "2026-10-21 16:00", 3600, "2026-10-21 17:00"},
		{"after closing starts next morning", office, ist, "2026-10-21 18:00", 60, "2026-10-22 09:01"},
		{"friday evening skips the weekend", office, ist, "2026-10-23 16:30", 3600, "2026-10-26 09:30"},
		{"saturday starts on monday", office, ist, "2026-10-24 12:00", 8 * 3600, "2026-10-26 17:00"},
		{"several days", office, ist, "2026-10-21 09:00", 3 * 8 * 3600, "2026-10-23 17:00"},
		{"zero seconds", office, ist, "2026-10-24 12:00", 0, "2026-10-24 12:00"},
		{"hours off is plain time", hoursInbox(t, ist, false, officeDays("09:00", "17:00")...), ist, "2026-10-24 12:00", 3600, "2026-10-24 13:00"},
		{"enabled without intervals is plain time", hoursInbox(t, ist, true), ist, "2026-10-24 12:00", 3600, "2026-10-24 13:00"},
		{"overlapping and split intervals", hoursInbox(t, ist, true,
			oas.BusinessHoursInterval{Day: oas.Wed, Start: "09:00", End: "12:00"},
			oas.BusinessHoursInterval{Day: oas.Wed, Start: "11:00", End: "13:00"},
			oas.BusinessHoursInterval{Day: oas.Wed, Start: "14:00", End: "18:00"},
		), ist, "2026-10-21 08:00", 5 * 3600, "2026-10-21 15:00"},
		{"to midnight", hoursInbox(t, ist, true, oas.BusinessHoursInterval{Day: oas.Wed, Start: "20:00", End: "24:00"},
			oas.BusinessHoursInterval{Day: oas.Thu, Start: "00:00", End: "02:00"}), ist, "2026-10-21 23:00", 2 * 3600, "2026-10-22 01:00"},
		{"timezone of the inbox, not UTC", hoursInbox(t, ny, true, officeDays("09:00", "17:00")...), ny, "2026-10-21 16:30", 3600, "2026-10-22 09:30"},
		// Berlin moves to summer time on 2026-03-29 at 02:00: that Sunday has 23 hours.
		{"spring forward day counts real time", hoursInbox(t, berlin, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "00:00", End: "24:00"}),
			berlin, "2026-03-29 00:00", 23 * 3600, "2026-03-30 00:00"},
		{"across the spring change", hoursInbox(t, berlin, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "01:00", End: "04:00"}),
			berlin, "2026-03-29 01:00", 2 * 3600, "2026-03-29 04:00"},
		// and back to winter time on 2026-10-25 at 03:00: that Sunday has 25 hours.
		{"fall back day counts real time", hoursInbox(t, berlin, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "00:00", End: "24:00"}),
			berlin, "2026-10-25 00:00", 24 * 3600, "2026-10-25 23:00"},
		{"office hours keep their local time after the change", hoursInbox(t, berlin, true, officeDays("09:00", "17:00")...),
			berlin, "2026-10-23 16:00", 2 * 3600, "2026-10-26 10:00"},
		{"plain time across a change", hoursInbox(t, berlin, false), berlin, "2026-10-25 00:00", 24 * 3600, "2026-10-25 23:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := addBusinessSeconds(c.in, localAt(t, c.tz, c.from), c.secs)
			if want := localAt(t, c.tz, c.want); !got.Equal(want) {
				t.Fatalf("got %s, want %s", got.In(want.Location()), want)
			}
		})
	}
}

func TestBusinessSecondsBetween(t *testing.T) {
	const ist, berlin = "Europe/Istanbul", "Europe/Berlin"
	office := hoursInbox(t, ist, true, officeDays("09:00", "17:00")...)
	cases := []struct {
		name     string
		in       store.Inbox
		tz       string
		from, to string
		want     int64
	}{
		{"inside hours", office, ist, "2026-10-21 10:00", "2026-10-21 11:30", 5400},
		{"overnight", office, ist, "2026-10-21 16:00", "2026-10-22 10:00", 2 * 3600},
		{"weekend counts nothing", office, ist, "2026-10-24 00:00", "2026-10-26 00:00", 0},
		{"a working week", office, ist, "2026-10-19 00:00", "2026-10-26 00:00", 5 * 8 * 3600},
		{"backwards is zero", office, ist, "2026-10-22 10:00", "2026-10-21 10:00", 0},
		{"hours off", hoursInbox(t, ist, false), ist, "2026-10-24 00:00", "2026-10-26 00:00", 48 * 3600},
		{"fall back day", hoursInbox(t, berlin, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "00:00", End: "24:00"}),
			berlin, "2026-10-25 00:00", "2026-10-26 00:00", 25 * 3600},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := businessSecondsBetween(c.in, localAt(t, c.tz, c.from), localAt(t, c.tz, c.to)); got != c.want {
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
}

func TestBusinessClockRoundTrip(t *testing.T) {
	in := hoursInbox(t, "America/New_York", true, append(officeDays("08:30", "12:00"), officeDays("13:00", "18:15")...)...)
	from := localAt(t, "America/New_York", "2026-03-05 11:17")
	for secs := int64(0); secs < 30*24*3600; secs += 7919 {
		due := addBusinessSeconds(in, from, secs)
		if got := businessSecondsBetween(in, from, due); got != secs {
			t.Fatalf("%d seconds: due %s counts back %d", secs, due, got)
		}
	}
}

// Chile moves to summer time when Saturday 2026-09-05 ends, Cuba on Sunday 2026-03-08: on those
// Sundays local midnight does not exist.
func TestBusinessClockMissingMidnight(t *testing.T) {
	const santiago, havana = "America/Santiago", "America/Havana"
	sat := hoursInbox(t, santiago, true, oas.BusinessHoursInterval{Day: oas.Sat, Start: "09:00", End: "17:00"})
	if got := businessSecondsBetween(sat, localAt(t, santiago, "2026-09-05 08:00"), localAt(t, santiago, "2026-09-07 08:00")); got != 28800 {
		t.Fatalf("Santiago Saturday counted %d seconds, want 28800", got)
	}
	sun := hoursInbox(t, santiago, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "10:00", End: "12:00"})
	if got, want := addBusinessSeconds(sun, localAt(t, santiago, "2026-09-05 12:00"), 3600), localAt(t, santiago, "2026-09-06 11:00"); !got.Equal(want) {
		t.Fatalf("Santiago Saturday noon + 1h is %s, want %s", got, want)
	}
	from := localAt(t, santiago, "2026-09-05 12:00")
	for secs := int64(0); secs <= 2*3600; secs += 600 {
		if back := businessSecondsBetween(sun, from, addBusinessSeconds(sun, from, secs)); back != secs {
			t.Fatalf("Santiago %d seconds count back %d", secs, back)
		}
	}
	weekend := hoursInbox(t, havana, true,
		oas.BusinessHoursInterval{Day: oas.Sat, Start: "00:00", End: "24:00"},
		oas.BusinessHoursInterval{Day: oas.Sun, Start: "00:00", End: "24:00"})
	if got := businessSecondsBetween(weekend, localAt(t, havana, "2026-03-07 00:00"), localAt(t, havana, "2026-03-09 00:00")); got != 47*3600 {
		t.Fatalf("Havana weekend counted %d seconds, want 47h", got)
	}
	if got, want := addBusinessSeconds(weekend, localAt(t, havana, "2026-03-07 23:00"), 2*3600), localAt(t, havana, "2026-03-08 02:00"); !got.Equal(want) {
		t.Fatalf("Havana Saturday 23:00 + 2h is %s, want %s", got, want)
	}
	sunday := hoursInbox(t, havana, true, oas.BusinessHoursInterval{Day: oas.Sun, Start: "00:00", End: "02:00"})
	if got := businessSecondsBetween(sunday, localAt(t, havana, "2026-03-07 12:00"), localAt(t, havana, "2026-03-09 12:00")); got != 3600 {
		t.Fatalf("Havana Sunday 00:00-02:00 counted %d seconds, want 3600", got)
	}
}

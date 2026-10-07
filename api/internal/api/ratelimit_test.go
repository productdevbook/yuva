package api

import (
	"fmt"
	"testing"
	"time"
)

func TestRateLimiterBounds(t *testing.T) {
	l := newRateLimiter()
	now := time.Unix(1_800_000_000, 0)
	lim := limit{2, time.Minute}
	if !l.allow("a", lim, now) || !l.allow("a", lim, now) || l.allow("a", lim, now) {
		t.Fatal("limit not applied")
	}
	if !l.allow("a", lim, now.Add(time.Minute)) {
		t.Fatal("new window refused")
	}
	for i := range maxBuckets {
		l.allow(fmt.Sprint("k", i), lim, now.Add(time.Minute))
	}
	if l.allow("fresh", lim, now.Add(time.Minute)) {
		t.Fatal("a new key was let in past the cap")
	}
	if !l.allow("a", lim, now.Add(time.Minute)) {
		t.Fatal("an existing key was refused at the cap")
	}
	if !l.allow("fresh", lim, now.Add(2*time.Minute)) || len(l.buckets) != 1 {
		t.Fatalf("finished windows not swept: %d buckets", len(l.buckets))
	}
}

func TestRateIP(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.9":          "203.0.113.9",
		"::ffff:203.0.113.9":   "203.0.113.9",
		"2001:db8:1:2:3:4:5:6": "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1": "2001:db8:1:2::/64",
		"not an address":       "not an address",
	} {
		if got := rateIP(in); got != want {
			t.Errorf("rateIP(%q) = %q, want %q", in, got, want)
		}
	}
}

package api

import "testing"

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

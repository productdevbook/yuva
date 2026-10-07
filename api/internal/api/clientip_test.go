package api

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("fd00::/8")}
	cases := []struct {
		name    string
		header  string
		trusted []netip.Prefix
		peer    string
		xff     []string
		want    string
	}{
		{"no header setting", "", nil, "203.0.113.9:4000", []string{"1.1.1.1"}, "203.0.113.9"},
		{"rightmost without trusted proxies", "X-Forwarded-For", nil, "10.0.0.1:4000", []string{"6.6.6.6, 198.51.100.7"}, "198.51.100.7"},
		{"skips trusted hops", "X-Forwarded-For", proxies, "10.0.0.1:4000", []string{"6.6.6.6, 198.51.100.7, 10.1.2.3"}, "198.51.100.7"},
		{"several header lines", "X-Forwarded-For", proxies, "10.0.0.1:4000", []string{"6.6.6.6", "198.51.100.7", "10.1.2.3"}, "198.51.100.7"},
		{"ipv6 proxy", "X-Forwarded-For", proxies, "[fd00::1]:4000", []string{"2001:db8::5, fd00::2"}, "2001:db8::5"},
		{"all trusted", "X-Forwarded-For", proxies, "10.0.0.1:4000", []string{"10.9.9.9, 10.1.2.3"}, "10.9.9.9"},
		{"untrusted peer ignores the header", "X-Forwarded-For", proxies, "203.0.113.9:4000", []string{"198.51.100.7"}, "203.0.113.9"},
		{"empty header", "X-Real-IP", nil, "203.0.113.9:4000", nil, "203.0.113.9"},
	}
	for _, tc := range cases {
		s := &Server{auth: AuthSettings{ClientIPHeader: tc.header, TrustedProxies: tc.trusted}}
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		for _, v := range tc.xff {
			r.Header.Add(tc.header, v)
		}
		if got := s.clientIP(r); got != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, got, tc.want)
		}
	}
}

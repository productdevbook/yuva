package webhook

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// The vector of the Standard Webhooks reference libraries (libraries/go/webhook_test.go).
func TestSignStandardVector(t *testing.T) {
	key, err := base64.StdEncoding.DecodeString("MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
	if err != nil {
		t.Fatal(err)
	}
	got := Sign(key, "msg_p5jXN8AQM9LWM0D4loKWxJek", time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))
	if want := "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="; got != want {
		t.Fatalf("signature %s, want %s", got, want)
	}
}

func TestNewSecret(t *testing.T) {
	s, key := NewSecret()
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, SecretPrefix))
	if !strings.HasPrefix(s, SecretPrefix) || err != nil || string(raw) != string(key) || len(key) != 32 {
		t.Fatalf("secret %q", s)
	}
}

func TestBlocked(t *testing.T) {
	for _, ip := range []string{
		"127.0.0.1", "10.0.0.1", "10.255.1.2", "172.16.5.4", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0",
		"224.0.0.1", "255.255.255.255", "::1", "::", "fe80::1", "fc00::1", "fd12::1", "::ffff:10.0.0.1", "::ffff:127.0.0.1",
		"64:ff9b::a00:1", "ff02::1",
	} {
		if !Blocked(netip.MustParseAddr(ip)) {
			t.Errorf("%s is not blocked", ip)
		}
	}
	for _, ip := range []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if Blocked(netip.MustParseAddr(ip)) {
			t.Errorf("%s is blocked", ip)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for _, u := range []string{
		"http://127.0.0.1/hook", "http://10.1.2.3:8080/", "http://[::1]/",
		"http://localhost:3000/", "http://api.localhost/", "http://[::ffff:192.168.0.1]/",
	} {
		if _, err := CheckURL(u, false); !errors.Is(err, ErrRefusedAddress) {
			t.Errorf("%s: %v", u, err)
		}
		if _, err := CheckURL(u, true); err != nil {
			t.Errorf("%s with private allowed: %v", u, err)
		}
	}
	for _, u := range []string{"ftp://example.com/", "https://user:pw@example.com/", "example.com/hook", "https:///x"} {
		if _, err := CheckURL(u, true); err == nil || errors.Is(err, ErrRefusedAddress) {
			t.Errorf("%s: %v", u, err)
		}
	}
	if _, err := CheckURL("https://hooks.example.com/yuva?x=1", false); err != nil {
		t.Fatal(err)
	}
}

func TestNeverAllowed(t *testing.T) {
	for _, ip := range []string{
		"169.254.169.254", "169.254.0.1", "169.254.255.255", "::ffff:169.254.169.254", "fe80::1", "fe80::1%eth0",
		"febf::1", "fd00:ec2::254",
	} {
		if !NeverAllowed(netip.MustParseAddr(ip)) {
			t.Errorf("%s is allowed", ip)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "fd12::1", "fec0::1", "169.253.255.255", "169.255.0.1", "1.1.1.1"} {
		if NeverAllowed(netip.MustParseAddr(ip)) {
			t.Errorf("%s is never allowed", ip)
		}
	}
}

func TestCheckURLRefusesLinkLocalWhenPrivateAllowed(t *testing.T) {
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data", "http://169.254.1.1:8080/", "http://[fe80::1]/",
		"http://[fe80::1%25eth0]/", "http://[::ffff:169.254.169.254]/", "http://[::ffff:a9fe:a9fe]/", "http://[fd00:ec2::254]/",
	} {
		for _, allow := range []bool{false, true} {
			if _, err := CheckURL(u, allow); !errors.Is(err, ErrRefusedAddress) {
				t.Errorf("%s (private allowed %v): %v", u, allow, err)
			}
		}
	}
}

func TestSendRefusesLinkLocalWhenPrivateAllowed(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	c := NewClient(true, fakeResolver{
		"metadata.example": {netip.MustParseAddr("169.254.169.254")},
		"mapped.example":   {netip.MustParseAddr("::ffff:169.254.169.254")},
		"v6ll.example":     {netip.MustParseAddr("fe80::1")},
		"mixed.example":    {netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("169.254.10.10")},
		"loop.example":     {netip.MustParseAddr("127.0.0.1")},
	})
	for _, u := range []string{
		"http://metadata.example" + port, "http://mapped.example" + port, "http://v6ll.example" + port, "http://mixed.example" + port,
		"http://169.254.169.254" + port,
	} {
		res := c.Send(context.Background(), Request{URL: u, MessageID: "msg_1", Timestamp: time.Now(), Body: []byte("{}"), Keys: [][]byte{[]byte("k")}})
		if !errors.Is(res.Err, ErrRefusedAddress) || res.OK() {
			t.Errorf("%s: %+v", u, res)
		}
	}
	if hit {
		t.Fatal("a never allowed target was reached")
	}
	res := c.Send(context.Background(), Request{URL: "http://loop.example" + port, MessageID: "msg_2", Timestamp: time.Now(), Body: []byte("{}"), Keys: [][]byte{[]byte("k")}})
	if !res.OK() || !hit {
		t.Fatalf("loopback with private allowed: %+v", res)
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return f[host], nil
}

func TestSendRefusesPrivateTargets(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	c := NewClient(false, fakeResolver{
		"internal.example": {netip.MustParseAddr("10.0.0.5")},
		"loop.example":     {netip.MustParseAddr("127.0.0.1")},
		"mixed.example":    {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
		"metadata.example": {netip.MustParseAddr("169.254.169.254")},
		"v6.example":       {netip.MustParseAddr("::1")},
	})
	for _, u := range []string{
		srv.URL, "http://[::1]" + port, "http://internal.example" + port, "http://loop.example" + port,
		"http://mixed.example" + port, "http://metadata.example/", "http://v6.example" + port,
	} {
		res := c.Send(context.Background(), Request{URL: u, MessageID: "msg_1", Timestamp: time.Now(), Body: []byte("{}"), Keys: [][]byte{[]byte("k")}})
		if !errors.Is(res.Err, ErrRefusedAddress) || res.OK() {
			t.Errorf("%s: %+v", u, res)
		}
	}
	if hit {
		t.Fatal("a refused target was reached")
	}
}

func TestSendSignsAndDoesNotFollowRedirects(t *testing.T) {
	var got http.Header
	var redirected bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			redirected = true
			return
		}
		got = r.Header.Clone()
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", 4000)))
	}))
	defer srv.Close()
	c := NewClient(true, nil)
	ts := time.Unix(1700000000, 0)
	res := c.Send(context.Background(), Request{
		URL: srv.URL + "/ok", MessageID: "msg_a", Timestamp: ts, Body: []byte(`{"a":1}`), Keys: [][]byte{[]byte("new"), []byte("old")},
	})
	if !res.OK() || res.StatusCode != 200 || len(res.Body) != KeptBodyBytes {
		t.Fatalf("result %+v", res)
	}
	want := Sign([]byte("new"), "msg_a", ts, []byte(`{"a":1}`)) + " " + Sign([]byte("old"), "msg_a", ts, []byte(`{"a":1}`))
	if got.Get("webhook-id") != "msg_a" || got.Get("webhook-timestamp") != "1700000000" || got.Get("webhook-signature") != want {
		t.Fatalf("headers %v", got)
	}
	res = c.Send(context.Background(), Request{URL: srv.URL + "/redirect", MessageID: "msg_b", Timestamp: ts, Body: []byte("{}")})
	if res.OK() || res.StatusCode != http.StatusTemporaryRedirect || redirected {
		t.Fatalf("redirect: %+v, followed %v", res, redirected)
	}
}

func TestRetrySchedule(t *testing.T) {
	var total time.Duration
	prev := time.Duration(0)
	for n := 1; ; n++ {
		d, ok := NextDelay(n, []byte("delivery"))
		if !ok {
			if n != len(RetryDelays)+1 {
				t.Fatalf("schedule ends after %d attempts", n)
			}
			break
		}
		base := RetryDelays[n-1]
		if d < base || d > base+base/10 {
			t.Fatalf("attempt %d: delay %s outside %s + 10%%", n, d, base)
		}
		if again, _ := NextDelay(n, []byte("delivery")); again != d {
			t.Fatalf("attempt %d: delay not stable", n)
		}
		if base < prev {
			t.Fatalf("attempt %d: delay shrinks", n)
		}
		prev = base
		total += base
	}
	if total < 24*time.Hour || total > 26*time.Hour {
		t.Fatalf("retries span %s, want about 24h", total)
	}
}

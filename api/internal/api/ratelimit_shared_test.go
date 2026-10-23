package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

// twoServers runs a second server on the harness's database, clock and settings, as a second
// replica would; h.url can point at either.
func twoServers(t *testing.T, configure func(*api.Deps)) (*harness, *api.Server, []string) {
	t.Helper()
	var deps *api.Deps
	h := newHarnessWith(t, func(d *api.Deps) {
		if configure != nil {
			configure(d)
		}
		deps = d
	})
	other := api.New(*deps)
	ts := httptest.NewServer(other.Handler())
	t.Cleanup(ts.Close)
	return h, other, []string{h.url, ts.URL}
}

func expectLimited(t *testing.T, r response) {
	t.Helper()
	if r.status != http.StatusTooManyRequests || r.str("code") != "rate_limited" || r.header.Get("Retry-After") == "" {
		t.Fatalf("want 429 rate_limited with Retry-After, got %d %s", r.status, r.raw)
	}
}

func TestSessionLimitSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, _, urls := twoServers(t, nil)
	ct := newChatTeam(t, h, "live", false)
	c := h.client()
	c.origin = ct.origin
	start := func() response {
		return c.do("POST", "/client/v1/session", map[string]any{"channel_key": ct.key, "identity_token": ct.token(h, map[string]any{"sub": unique("user")})})
	}
	for i := range 30 {
		h.url = urls[i%2]
		if r := start(); r.status != http.StatusCreated {
			t.Fatalf("session %d on server %d: %d %s", i+1, i%2+1, r.status, r.raw)
		}
	}
	for _, u := range urls {
		h.url = u
		expectLimited(t, start())
	}
	other := h.client()
	other.origin = ct.origin
	if r := other.do("POST", "/client/v1/session", map[string]any{"channel_key": ct.key, "identity_token": ct.token(h, map[string]any{"sub": unique("user")})}); r.status != http.StatusCreated {
		t.Fatalf("another address was refused: %d %s", r.status, r.raw)
	}
	h.clock.Advance(time.Minute)
	h.url = urls[1]
	if r := start(); r.status != http.StatusCreated {
		t.Fatalf("a new window was refused: %d %s", r.status, r.raw)
	}
}

func TestBearerLimitSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, _, urls := twoServers(t, nil)
	tm := newTeam(t, h)
	grant := h.client()
	grant.bearer = h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin, []string{"conversations:read"})
	for i := range 600 {
		h.url = urls[i%2]
		grant.expect(http.StatusOK, "GET", "/v1/conversations", nil)
	}
	for _, u := range urls {
		h.url = u
		expectLimited(t, grant.do("GET", "/v1/conversations", nil))
	}
	h.url = urls[0]
	other := h.client()
	other.bearer = h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin, []string{"conversations:read"})
	other.expect(http.StatusOK, "GET", "/v1/conversations", nil)
}

func TestMonitorLimitSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, _, urls := twoServers(t, nil)
	st := newStatusTeam(t, h)
	_, token := st.monitor(t, map[string]any{"kind": "heartbeat", "name": "Backup", "component_id": st.api})
	for i := range 60 {
		h.url = urls[i%2]
		if r := h.client().do("GET", "/ingress/heartbeat/"+token, nil); r.status != http.StatusOK {
			t.Fatalf("beat %d: %d %s", i+1, r.status, r.raw)
		}
	}
	for _, u := range urls {
		h.url = u
		expectLimited(t, h.client().do("GET", "/ingress/heartbeat/"+token, nil))
	}
}

func TestAnalyticsAddressLimitSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, _, urls := twoServers(t, nil)
	tm := newTeam(t, h)
	_, key := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	req := collectReq{origin: "https://example.com", ip: "203.0.113.40", body: pageview("https://example.com/")}
	for i := range 120 {
		h.url = urls[i%2]
		if r := collect(h, key, req); r.status != http.StatusAccepted {
			t.Fatalf("event %d: %d %s", i+1, r.status, r.raw)
		}
	}
	for _, u := range urls {
		h.url = u
		expectLimited(t, collect(h, key, req))
	}
}

func TestAnalyticsPropertyLimitSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, other, urls := twoServers(t, func(d *api.Deps) { d.Analytics.PropertyEventsPerMinute = 5 })
	tm := newTeam(t, h)
	id, _ := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	backend := tm.apiKey(h)
	batch := func() []map[string]any {
		var out []map[string]any
		for i := range 3 {
			out = append(out, map[string]any{"name": "Signup", "url": "https://example.com/signup", "ip": "198.51.100." + strconv.Itoa(i+1), "user_agent": testUA})
		}
		return out
	}
	for _, u := range urls {
		h.url = u
		serverEvents(backend, http.StatusAccepted, id, batch()...)
	}
	h.flushAnalytics()
	if err := other.FlushAnalytics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM analytics_events WHERE workspace_id = $1 AND property_id = $2", tm.ws, id); n != 5 {
		t.Fatalf("%d events kept, want the property's 5 a minute", n)
	}
	if n := h.count("SELECT coalesce(sum(events), 0) FROM analytics_ingest_stats WHERE workspace_id = $1 AND property_id = $2 AND reason = 'rate'", tm.ws, id); n != 1 {
		t.Fatalf("%d events dropped for the rate, want 1", n)
	}
}

func TestRateWindowsAcrossServers(t *testing.T) {
	t.Parallel()
	h, other, _ := twoServers(t, nil)
	ctx := context.Background()
	key := unique("window")
	take := func(s *api.Server, n int) (int, time.Duration) {
		t.Helper()
		return s.TakeRate(ctx, key, n, 5, time.Minute)
	}
	if n, _ := take(h.srv, 3); n != 3 {
		t.Fatalf("first server took %d of 3", n)
	}
	if n, wait := take(other, 3); n != 2 || wait <= 0 || wait > time.Minute {
		t.Fatalf("second server took %d of 3 (wait %s), want the 2 left", n, wait)
	}
	if n, _ := take(h.srv, 1); n != 0 {
		t.Fatal("a full window let one more through")
	}
	h.clock.Advance(30 * time.Second)
	if n, wait := take(other, 1); n != 0 || wait > 31*time.Second {
		t.Fatalf("took %d, wait %s halfway through the window", n, wait)
	}
	h.clock.Advance(30 * time.Second)
	if n, _ := take(other, 1); n != 1 {
		t.Fatal("the next window was refused")
	}
	h.clock.Advance(-30 * time.Second)
	if n, _ := take(h.srv, 5); n != 4 {
		t.Fatalf("a clock slightly behind took %d, want the window's 4 left", n)
	}
	h.clock.Advance(-5 * time.Minute)
	if n, _ := take(h.srv, 5); n != 5 {
		t.Fatalf("a clock set back past the window took %d, want a fresh window", n)
	}
}

func TestRateTakesOfOneKeyCountTogether(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()
	key, other := unique("dup"), unique("dup")
	got, wait := h.srv.TakeRates(ctx, nil,
		api.RateTake{Key: key, N: 2, Lim: 3, Window: time.Minute},
		api.RateTake{Key: other, N: 1, Lim: 3, Window: time.Minute},
		api.RateTake{Key: key, N: 2, Lim: 3, Window: time.Minute})
	if got[0] != 2 || got[1] != 1 || got[2] != 1 || wait[0] != 0 || wait[2] <= 0 {
		t.Fatalf("takes sharing a key got %v (waits %v), want 2, 1 and the 1 left", got, wait)
	}
	if n, _ := h.srv.TakeRate(ctx, key, 1, 3, time.Minute); n != 0 {
		t.Fatal("the shared key's window was not full")
	}
}

func TestRateLimitFailsOpen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()
	tx, err := h.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1/0"); err == nil {
		t.Fatal("the transaction did not fail")
	}
	key := unique("broken")
	if got, _ := h.srv.TakeRates(api.MemberTraffic(ctx), h.st.WithTx(tx), api.RateTake{Key: key, N: 3, Lim: 1, Window: time.Minute}); got[0] != 3 {
		t.Fatalf("a failing check let %d of 3 member requests through, want all", got[0])
	}
	if got, _ := h.srv.TakeRates(ctx, h.st.WithTx(tx), api.RateTake{Key: key, N: 3, Lim: 1, Window: time.Minute}); got[0] != 1 {
		t.Fatalf("a failing check let %d of 3 anonymous requests through, want the local limit's 1", got[0])
	}
}

func TestQuerySlotsSharedAcrossServers(t *testing.T) {
	t.Parallel()
	h, other, _ := twoServers(t, nil)
	ctx := context.Background()
	ws := uuid.New()
	var releases []func()
	for i := range 4 {
		s := h.srv
		if i%2 == 1 {
			s = other
		}
		release, err := s.AcquireQuerySlot(ctx, ws)
		if err != nil {
			t.Fatalf("slot %d: %v", i+1, err)
		}
		releases = append(releases, release)
	}
	if _, err := other.AcquireQuerySlot(ctx, ws); err == nil || !strings.HasPrefix(err.Error(), "429 rate_limited") {
		t.Fatalf("a fifth query was let through: %v", err)
	}
	if release, err := h.srv.AcquireQuerySlot(ctx, uuid.New()); err != nil {
		t.Fatalf("another workspace waited: %v", err)
	} else {
		release()
	}
	releases[0]()
	release, err := other.AcquireQuerySlot(ctx, ws)
	if err != nil {
		t.Fatalf("a freed slot was not reused: %v", err)
	}
	release()
	h.clock.Advance(2 * time.Minute)
	if release, err = h.srv.AcquireQuerySlot(ctx, ws); err != nil {
		t.Fatalf("a slot whose lease ended was not reused: %v", err)
	}
	release()
	for _, r := range releases[1:] {
		r()
	}
}

func TestRateLimitSweep(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	short, long, slots := unique("short"), unique("long"), uuid.New()
	for _, k := range []struct {
		key    string
		window time.Duration
	}{{short, time.Millisecond}, {long, time.Hour}} {
		h.srv.TakeRate(ctx, k.key, 1, 1, k.window)
	}
	if _, err := h.srv.AcquireQuerySlot(ctx, slots); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(2 * time.Minute)
	n, err := h.srv.SweepRateLimits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("swept %d rows, want at least the ended window and the ended lease", n)
	}
	if h.count("SELECT count(*) FROM rate_limits WHERE key = $1", h.srv.RateKeyHash(short)) != 0 {
		t.Fatal("an ended window was kept")
	}
	if h.count("SELECT count(*) FROM rate_limits WHERE key = $1", h.srv.RateKeyHash(long)) != 1 {
		t.Fatal("an open window was swept")
	}
	if h.count("SELECT count(*) FROM rate_slots WHERE key = $1", h.srv.RateKeyHash("analytics-query:"+slots.String())) != 0 {
		t.Fatal("an ended lease was kept")
	}
}

func TestChannelBudgetSurvivesAFlood(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	start := func(ip string, identified bool) response {
		c := h.client()
		c.ip, c.origin = ip, ct.origin
		req := map[string]any{"channel_key": ct.key}
		if identified {
			req["identity_token"] = ct.token(h, map[string]any{"sub": unique("user")})
		}
		return c.do("POST", "/client/v1/session", req)
	}
	refused := 0
	for i := range 5 {
		ip := "2001:db8:1:" + strconv.Itoa(i) + "::1"
		for range 30 {
			if start(ip, false).status == http.StatusTooManyRequests {
				refused++
			}
		}
	}
	if refused != 30 {
		t.Fatalf("five /64s of one /48 had %d of 150 session starts refused, want the 30 over the network's 120", refused)
	}
	for i := range 20 {
		ip := "2001:db8:" + strconv.Itoa(100+i) + "::1"
		for j := range 30 {
			if r := start(ip, false); r.status == http.StatusTooManyRequests {
				t.Fatalf("source %d, start %d: %d %s", i+1, j+1, r.status, r.raw)
			}
		}
		expectLimited(t, start(ip, false))
	}
	if r := start("203.0.113.9", true); r.status != http.StatusCreated {
		t.Fatalf("a 21st source after the flood: %d %s", r.status, r.raw)
	}
	if r := start("2001:db8:1:99::1", true); r.status != http.StatusTooManyRequests {
		t.Fatalf("a new /64 in the flooding /48: %d %s", r.status, r.raw)
	}
}

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/store"
)

// rateLimited is the 429 for a refused window: the detail names the wait and Retry-After carries
// it in seconds.
func rateLimited(wait time.Duration) *apiError {
	secs := max(int(math.Ceil(wait.Seconds())), 1)
	var in string
	switch mins := (secs + 59) / 60; {
	case secs < 60:
		in = fmt.Sprintf("%d seconds", secs)
	case mins == 1:
		in = "a minute"
	case mins < 60:
		in = fmt.Sprintf("%d minutes", mins)
	case mins == 60:
		in = "an hour"
	default:
		in = fmt.Sprintf("%d hours", (mins+59)/60)
	}
	e := problem(http.StatusTooManyRequests, "rate_limited", "too many requests; try again in "+in)
	e.RetryAfter = secs
	return e
}

type limit struct {
	n      int
	window time.Duration
}

var (
	limitSessionPerIP      = limit{30, time.Minute}
	limitChannelPerIP      = limit{120, time.Minute}
	limitSessionPerChannel = limit{600, time.Minute}
	limitWritePerIP        = limit{60, time.Minute}
	limitWritePerChannel   = limit{1200, time.Minute}
	limitTypingPerSession  = limit{60, time.Minute}
	limitBearer            = limit{600, time.Minute}
	limitRegisterPerIP     = limit{20, time.Hour}
	limitAuthorizePerIP    = limit{60, time.Minute}
)

const (
	defaultAnonymousContactsPerHour = 20
	rateSlotPoll                    = 100 * time.Millisecond
)

// rateLimiter counts requests per key in fixed windows that start with a key's first request, in
// Postgres, so every replica counts against the same windows. Keys are stored as a keyed hash: they
// hold client addresses. Finished windows are deleted by the rate limit sweep.
type rateLimiter struct {
	st  *store.Store
	mac []byte
}

func newRateLimiter(st *store.Store, secrets *secret.Key) *rateLimiter {
	l := &rateLimiter{st: st}
	if secrets != nil {
		l.mac = secrets.Derive("yuva rate limit keys")
	}
	return l
}

func (l *rateLimiter) hash(key string) []byte {
	m := hmac.New(sha256.New, l.mac)
	m.Write([]byte(key))
	return m.Sum(nil)[:16]
}

type rateTake struct {
	key string
	lim limit
	n   int
}

type rateGrant struct {
	n    int
	wait time.Duration
}

// take counts n requests against each key in one statement and says how many fit; when not all
// did, wait is how long until that key's window ends. Keys must be distinct.
func (l *rateLimiter) take(ctx context.Context, now time.Time, takes ...rateTake) ([]rateGrant, error) {
	p := store.TakeRateLimitsParams{Now: now, Keys: make([][]byte, len(takes)), Ends: make([]time.Time, len(takes)),
		Lims: make([]int32, len(takes)), Wants: make([]int32, len(takes))}
	at := make(map[string]int, len(takes))
	for i, t := range takes {
		p.Keys[i], p.Ends[i], p.Lims[i], p.Wants[i] = l.hash(t.key), now.Add(t.lim.window), int32(max(t.lim.n, 0)), int32(t.n)
		at[string(p.Keys[i])] = i
	}
	rows, err := l.st.TakeRateLimits(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]rateGrant, len(takes))
	for _, r := range rows {
		i, ok := at[string(r.Key)]
		if !ok {
			continue
		}
		out[i].n = int(r.Granted)
		if out[i].n < takes[i].n {
			out[i].wait = min(max(r.EndsAt.Sub(now), time.Second), takes[i].lim.window)
		}
	}
	return out, nil
}

// allowWait counts one request against key and, when refused, says how long until the window ends.
func (l *rateLimiter) allowWait(ctx context.Context, key string, lim limit, now time.Time) (bool, time.Duration, error) {
	g, err := l.take(ctx, now, rateTake{key, lim, 1})
	if err != nil {
		return false, 0, err
	}
	return g[0].n == 1, g[0].wait, nil
}

func (l *rateLimiter) allow(ctx context.Context, key string, lim limit, now time.Time) (bool, error) {
	ok, _, err := l.allowWait(ctx, key, lim, now)
	return ok, err
}

// acquireSlot holds one of n slots of key for at most lease, waiting up to wait for one to free.
// It reports false when none freed in time; release gives the slot back.
func (l *rateLimiter) acquireSlot(ctx context.Context, key string, n int, lease, wait time.Duration, now func() time.Time) (func(), bool, error) {
	h, holder := l.hash(key), newID()
	deadline := time.Now().Add(wait)
	for {
		at := now()
		slot, err := l.st.TakeRateSlot(ctx, store.TakeRateSlotParams{Key: h, Holder: holder, ExpiresAt: at.Add(lease), Last: int32(n - 1), Now: at})
		if err == nil {
			return func() { l.releaseSlot(ctx, h, slot, holder) }, true, nil
		}
		if !store.IsNotFound(err) {
			return nil, false, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, false, nil
		}
		t := time.NewTimer(min(rateSlotPoll, left))
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, false, ctx.Err()
		}
	}
}

func (l *rateLimiter) releaseSlot(ctx context.Context, key []byte, slot int32, holder uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = l.st.ReleaseRateSlot(ctx, store.ReleaseRateSlotParams{Key: key, Slot: slot, Holder: holder})
}

type rateLimitSweepWorker struct {
	river.WorkerDefaults[jobs.RateLimitSweepArgs]
	s *Server
}

func (w *rateLimitSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.RateLimitSweepArgs]) error {
	_, err := w.s.SweepRateLimits(ctx)
	return err
}

// SweepRateLimits deletes the windows and slot leases that ended and says how many went.
func (s *Server) SweepRateLimits(ctx context.Context) (int64, error) {
	now := s.now()
	n, err := s.st.SweepRateLimits(ctx, now)
	if err != nil {
		return 0, err
	}
	m, err := s.st.SweepRateSlots(ctx, now)
	return n + m, err
}

// rateIP is the client address as a rate limit key: IPv6 clients usually hold a whole /64.
func rateIP(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	if a = a.Unmap(); a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}

// rateLimit counts the request against each check in turn and stops at the first that refuses, so
// a refused address never spends a channel's budget.
func (s *Server) rateLimit(ctx context.Context, checks ...rateCheck) error {
	now := s.now()
	for _, c := range checks {
		ok, wait, err := s.limits.allowWait(ctx, c.key, c.lim, now)
		if err != nil {
			return err
		}
		if !ok {
			return rateLimited(wait)
		}
	}
	return nil
}

type rateCheck struct {
	key string
	lim limit
}

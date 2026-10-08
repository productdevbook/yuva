package api

import (
	"net/http"
	"net/netip"
	"sync"
	"time"
)

var errClientRateLimited = problem(http.StatusTooManyRequests, "rate_limited", "too many requests; try again in a minute")

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
	maxBuckets                      = 100_000
	defaultAnonymousContactsPerHour = 20
)

type bucket struct {
	start  time.Time
	window time.Duration
	n      int
}

// rateLimiter counts requests per key in fixed windows, in this process only. Finished windows
// are swept every minute; when maxBuckets windows are open, new keys are refused until a sweep
// frees room, so rotating source addresses cannot grow it without bound.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{buckets: map[string]*bucket{}} }

func (l *rateLimiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.start) >= b.window || now.Before(b.start) {
			delete(l.buckets, k)
		}
	}
	l.swept = now
}

func (l *rateLimiter) allow(key string, lim limit, now time.Time) bool {
	ok, _ := l.allowWait(key, lim, now)
	return ok
}

// allowWait is allow that also says, when refused, how long until the window ends.
func (l *rateLimiter) allowWait(key string, lim limit, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) >= time.Minute || now.Before(l.swept) {
		l.sweep(now)
	}
	b := l.buckets[key]
	if b == nil || now.Sub(b.start) >= lim.window || now.Before(b.start) {
		if b == nil && len(l.buckets) >= maxBuckets {
			return false, time.Minute
		}
		b = &bucket{start: now, window: lim.window}
		l.buckets[key] = b
	}
	if b.n >= lim.n {
		return false, b.start.Add(b.window).Sub(now)
	}
	b.n++
	return true, 0
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

func (s *Server) rateLimit(checks ...rateCheck) error {
	now := s.now()
	for _, c := range checks {
		if !s.limits.allow(c.key, c.lim, now) {
			return errClientRateLimited
		}
	}
	return nil
}

type rateCheck struct {
	key string
	lim limit
}

package api

import (
	"net/http"
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
	limitSessionPerChannel = limit{600, time.Minute}
	limitWritePerIP        = limit{60, time.Minute}
	limitWritePerChannel   = limit{1200, time.Minute}
	limitTypingPerSession  = limit{60, time.Minute}
)

type bucket struct {
	start time.Time
	n     int
}

// rateLimiter counts requests per key in fixed windows, in this process only.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

func newRateLimiter() *rateLimiter { return &rateLimiter{buckets: map[string]*bucket{}} }

func (l *rateLimiter) allow(key string, lim limit, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.start) > time.Hour {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	b := l.buckets[key]
	if b == nil || now.Sub(b.start) >= lim.window || now.Before(b.start) {
		b = &bucket{start: now}
		l.buckets[key] = b
	}
	if b.n >= lim.n {
		return false
	}
	b.n++
	return true
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

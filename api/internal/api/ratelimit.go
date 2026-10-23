package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"sync"
	"sync/atomic"
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
	limitSessionPerIP     = limit{30, time.Minute}
	limitChannelPerIP     = limit{120, time.Minute}
	limitWritePerIP       = limit{60, time.Minute}
	limitTypingPerSession = limit{60, time.Minute}
	limitBearer           = limit{600, time.Minute}
	limitRegisterPerIP    = limit{20, time.Hour}
	limitAuthorizePerIP   = limit{60, time.Minute}
)

// budget is a limit that every visitor of one channel, page or centre shares. One network
// (rateNet) may use `network` of it, and `ceiling` only caps the total load: it is far above what a
// few networks reach, so a flood from some sources never locks the others out.
type budget struct{ network, ceiling limit }

var (
	budgetSessions = budget{limit{120, time.Minute}, limit{6000, time.Minute}}
	budgetWrites   = budget{limit{240, time.Minute}, limit{12000, time.Minute}}
)

// sharedChecks are the checks for a request from address ip against the budget named key; the
// caller checks the address's own limit first.
func sharedChecks(ip, key string, b budget) []rateCheck {
	return []rateCheck{{key + ":net:" + rateNet(ip), b.network}, {key, b.ceiling}}
}

const (
	defaultAnonymousContactsPerHour = 20
	rateSlotFirstPoll               = 25 * time.Millisecond
	rateSlotMaxPoll                 = 400 * time.Millisecond
	rateFailureLogEvery             = time.Minute
	localWindowsMax                 = 100_000
)

// rateLimiter counts requests per key in fixed windows that start with a key's first request, in
// Postgres and by the database's clock, so every replica counts against the same windows. Keys are
// stored as a keyed hash: they hold client addresses. When the database fails, a check for a
// signed-in member, API key or grant lets the request through; every other check (anonymous and
// contact traffic, jobs) counts in this process's own windows instead, with the same limits. The
// failure is logged at most once a minute. Finished windows are deleted by the rate limit sweep.
type rateLimiter struct {
	st     *store.Store
	mac    []byte
	log    *slog.Logger
	clock  func() time.Time
	sleep  func(ctx context.Context, d time.Duration) error
	logged atomic.Int64
	local  localWindows
	broken func() error
}

type localWindow struct {
	ends time.Time
	n    int
}

// localWindows are the fixed windows a rateLimiter counts in while the database fails. When full,
// ended windows go; a key that still finds no room is refused.
type localWindows struct {
	mu sync.Mutex
	m  map[string]*localWindow
}

func (lw *localWindows) take(key string, lim limit, n int, now time.Time) (int, time.Duration) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	if lw.m == nil {
		lw.m = map[string]*localWindow{}
	}
	w := lw.m[key]
	if w == nil || !now.Before(w.ends) {
		if w == nil && len(lw.m) >= localWindowsMax {
			for k, old := range lw.m {
				if !now.Before(old.ends) {
					delete(lw.m, k)
				}
			}
			if len(lw.m) >= localWindowsMax {
				return 0, lim.window
			}
		}
		w = &localWindow{ends: now.Add(lim.window)}
		lw.m[key] = w
	}
	got := min(n, max(lim.n-w.n, 0))
	w.n += got
	return got, w.ends.Sub(now)
}

type memberTrafficKey struct{}

// memberTraffic marks ctx as a signed-in member's, an API key's or a grant's before the principal
// is in it, so a failing limiter lets it through.
func memberTraffic(ctx context.Context) context.Context {
	return context.WithValue(ctx, memberTrafficKey{}, true)
}

func failsOpen(ctx context.Context) bool {
	if ctx.Value(memberTrafficKey{}) != nil {
		return true
	}
	p := principalFrom(ctx)
	return p.personID != uuid.Nil() || p.workspaceID != uuid.Nil() || p.keyID != uuid.Nil()
}

func newRateLimiter(st *store.Store, secrets *secret.Key, log *slog.Logger, clock func() time.Time, sleep func(ctx context.Context, d time.Duration) error) *rateLimiter {
	l := &rateLimiter{st: st, log: log, clock: clock, sleep: sleep}
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

// at is the time the statements measure windows and leases with: nil, the database's clock, except
// in tests.
func (l *rateLimiter) at() *time.Time {
	if l.clock == nil {
		return nil
	}
	return new(l.clock())
}

func (l *rateLimiter) failed(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	now := time.Now().UnixNano()
	last := l.logged.Load()
	if now-last < int64(rateFailureLogEvery) || !l.logged.CompareAndSwap(last, now) {
		return
	}
	l.log.ErrorContext(ctx, "rate limit check failed; counting anonymous requests in this process", slog.Any("error", err))
}

func (l *rateLimiter) now() time.Time {
	if l.clock == nil {
		return time.Now()
	}
	return l.clock()
}

func (l *rateLimiter) queries(q *store.Queries) *store.Queries {
	if q == nil {
		return l.st.Queries
	}
	return q
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
// did, wait is how long until that key's window ends. Takes of one key are counted together, in
// order. q is the transaction to count in, or nil; in a transaction the statement runs in a
// savepoint so a failure leaves the transaction usable.
func (l *rateLimiter) take(ctx context.Context, q *store.Queries, takes ...rateTake) []rateGrant {
	out := make([]rateGrant, len(takes))
	p := store.TakeRateLimitsParams{At: l.at()}
	at := make(map[string]int, len(takes))
	for _, t := range takes {
		h := l.hash(t.key)
		if i, ok := at[string(h)]; ok {
			p.Wants[i] += int32(t.n)
			continue
		}
		at[string(h)] = len(p.Keys)
		p.Keys = append(p.Keys, h)
		p.WindowMs = append(p.WindowMs, t.lim.window.Milliseconds())
		p.Lims = append(p.Lims, int32(max(t.lim.n, 0)))
		p.Wants = append(p.Wants, int32(t.n))
	}
	var rows []store.TakeRateLimitsRow
	err := l.queries(q).Savepoint(ctx, func(q *store.Queries) error {
		if l.broken != nil {
			if err := l.broken(); err != nil {
				return err
			}
		}
		var err error
		rows, err = q.TakeRateLimits(ctx, p)
		return err
	})
	if err != nil {
		l.failed(ctx, err)
		open, now := failsOpen(ctx), l.now()
		for i, t := range takes {
			if open {
				out[i].n = t.n
				continue
			}
			out[i].n, out[i].wait = l.local.take(string(l.hash(t.key)), t.lim, t.n, now)
			if out[i].n < t.n {
				out[i].wait = min(max(out[i].wait, time.Second), t.lim.window)
			}
		}
		return out
	}
	granted := make([]int, len(p.Keys))
	waits := make([]time.Duration, len(p.Keys))
	for _, r := range rows {
		if i, ok := at[string(r.Key)]; ok {
			granted[i], waits[i] = int(r.Granted), r.EndsAt.Sub(r.CheckedAt)
		}
	}
	for i, t := range takes {
		k := at[string(l.hash(t.key))]
		out[i].n = min(t.n, granted[k])
		granted[k] -= out[i].n
		if out[i].n < t.n {
			out[i].wait = min(max(waits[k], time.Second), t.lim.window)
		}
	}
	return out
}

// allowWait counts one request against key and, when refused, says how long until the window ends.
func (l *rateLimiter) allowWait(ctx context.Context, q *store.Queries, key string, lim limit) (bool, time.Duration) {
	g := l.take(ctx, q, rateTake{key, lim, 1})
	return g[0].n == 1, g[0].wait
}

// takeAll counts n requests against key only when all of them fit in the window, and otherwise
// counts none and says how long until the window ends. When the database fails it lets them through.
func (l *rateLimiter) takeAll(ctx context.Context, key string, lim limit, n int) (bool, time.Duration) {
	r, err := l.st.TakeRateLimitAll(ctx, store.TakeRateLimitAllParams{
		Key: l.hash(key), WindowMs: lim.window.Milliseconds(), Lim: int32(max(lim.n, 0)), Want: int32(n), At: l.at(),
	})
	if err != nil {
		l.failed(ctx, err)
		return true, 0
	}
	if int(r.Granted) == n {
		return true, 0
	}
	return false, min(max(r.EndsAt.Sub(r.CheckedAt), time.Second), lim.window)
}

func (l *rateLimiter) allow(ctx context.Context, q *store.Queries, key string, lim limit) bool {
	ok, _ := l.allowWait(ctx, q, key, lim)
	return ok
}

// acquireSlot holds one of n slots of key for lease, by the same clock as the windows, and polls
// with a growing pause for up to wait for one to free. It reports false when none freed in time;
// release gives the slot back. When the database fails, it lets the caller through without a slot.
func (l *rateLimiter) acquireSlot(ctx context.Context, key string, n int, lease, wait time.Duration) (func(), bool, error) {
	h, holder := l.hash(key), newID()
	pause, waited := rateSlotFirstPoll, time.Duration(0)
	for {
		slot, err := l.st.TakeRateSlot(ctx, store.TakeRateSlotParams{Key: h, Holder: holder, LeaseSeconds: lease.Seconds(), At: l.at(), Last: int32(n - 1)})
		if err == nil {
			return func() { l.releaseSlot(ctx, h, slot, holder) }, true, nil
		}
		if !store.IsNotFound(err) {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			l.failed(ctx, err)
			return func() {}, true, nil
		}
		if waited >= wait {
			return nil, false, nil
		}
		d := min(pause+rand.N(pause/2), wait-waited)
		if err := l.sleep(ctx, d); err != nil {
			return nil, false, err
		}
		waited += d
		pause = min(2*pause, rateSlotMaxPoll)
	}
}

func (l *rateLimiter) releaseSlot(ctx context.Context, key []byte, slot int32, holder uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := l.st.ReleaseRateSlot(ctx, store.ReleaseRateSlotParams{Key: key, Slot: slot, Holder: holder}); err != nil {
		l.failed(ctx, err)
	}
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
	at := s.limits.at()
	n, err := s.st.SweepRateLimits(ctx, at)
	if err != nil {
		return 0, err
	}
	m, err := s.st.SweepRateSlots(ctx, at)
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

// rateNet is the network a client address belongs to: a /24 for IPv4 and a /48 for IPv6, what one
// site or customer usually holds, so the cheap addresses of one provider count together.
func rateNet(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	bits := 48
	if a = a.Unmap(); a.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(a, bits).Masked().String()
}

// rateLimit counts the request against each check in turn and stops at the first that refuses, so
// a refused address never spends a channel's budget. It is never called inside a transaction: the
// check takes a connection of its own; rateLimitTx counts on the transaction's.
func (s *Server) rateLimit(ctx context.Context, checks ...rateCheck) error {
	return s.rateLimitTx(ctx, nil, checks...)
}

// rateLimitTx is rateLimit on transaction q, for a check that depends on what the transaction read;
// the count goes with the transaction.
func (s *Server) rateLimitTx(ctx context.Context, q *store.Queries, checks ...rateCheck) error {
	for _, c := range checks {
		if ok, wait := s.limits.allowWait(ctx, q, c.key, c.lim); !ok {
			return rateLimited(wait)
		}
	}
	return nil
}

type rateCheck struct {
	key string
	lim limit
}

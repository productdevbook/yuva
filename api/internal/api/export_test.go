package api

import (
	"context"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

type RateTake struct {
	Key    string
	N, Lim int
	Window time.Duration
}

// TakeRates counts takes in one statement, in transaction q when it is not nil, and returns what
// each got and its wait.
func (s *Server) TakeRates(ctx context.Context, q *store.Queries, takes ...RateTake) ([]int, []time.Duration) {
	in := make([]rateTake, len(takes))
	for i, t := range takes {
		in[i] = rateTake{t.Key, limit{t.Lim, t.Window}, t.N}
	}
	g := s.limits.take(ctx, q, in...)
	n, wait := make([]int, len(g)), make([]time.Duration, len(g))
	for i := range g {
		n[i], wait[i] = g[i].n, g[i].wait
	}
	return n, wait
}

func (s *Server) TakeRate(ctx context.Context, key string, n, lim int, window time.Duration) (int, time.Duration) {
	got, wait := s.TakeRates(ctx, nil, RateTake{key, n, lim, window})
	return got[0], wait[0]
}

func (s *Server) RateKeyHash(key string) []byte { return s.limits.hash(key) }

func (s *Server) AcquireQuerySlot(ctx context.Context, ws uuid.UUID) (func(), error) {
	return s.analytics.slots.acquire(ctx, ws)
}

func SetMaxIncidentLinks(n int) (restore func()) {
	old := maxIncidentLinks
	maxIncidentLinks = n
	return func() { maxIncidentLinks = old }
}

func SetStatusReplyBatchSize(n int) (restore func()) {
	old := statusReplyBatchSize
	statusReplyBatchSize = n
	return func() { statusReplyBatchSize = old }
}

func (s *Server) ManageURL(ws, list, contact uuid.UUID) string {
	return s.manageURL(ws, list, contact, uuid.Nil())
}

func SetStatusClaimBudget(n int32) (restore func()) {
	old := claimBudget
	claimBudget = n
	return func() { claimBudget = old }
}

func (s *Server) CleanWorkspaceImages(ctx context.Context, ws uuid.UUID) error {
	return s.cleanUnusedImages(ctx, ws)
}

func (s *Server) CountHelpView(ws, center, article uuid.UUID, locale string) {
	s.countHelpArticle(store.HelpCenter{WorkspaceID: ws, ID: center}, article, locale, helpArticleCount{views: 1})
}

func (s *Server) CountHelpSearch(ws, center uuid.UUID, locale, query string, found bool) {
	s.countHelpSearch(store.HelpCenter{WorkspaceID: ws, ID: center}, locale, query, found)
}

func SetMaxIdeaLinks(n int) (restore func()) {
	old := maxIdeaLinks
	maxIdeaLinks = n
	return func() { maxIdeaLinks = old }
}

func SetRoadmapBatchSize(n int) (restore func()) {
	old := roadmapBatchSize
	roadmapBatchSize = n
	return func() { roadmapBatchSize = old }
}

func SetMaxPendingIdeas(n int) (restore func()) {
	old := maxPendingIdeas
	maxPendingIdeas = n
	return func() { maxPendingIdeas = old }
}

func SetIssueSweepLimits(batch, perIntegration int) (restore func()) {
	oldBatch, oldPer := issueSweepBatch, issueSweepPerIntegration
	issueSweepBatch, issueSweepPerIntegration = batch, perIntegration
	return func() { issueSweepBatch, issueSweepPerIntegration = oldBatch, oldPer }
}

func MemberTraffic(ctx context.Context) context.Context { return memberTraffic(ctx) }

// BreakRateLimits makes every database rate limit check fail with err.
func (s *Server) BreakRateLimits(err error) { s.limits.broken = func() error { return err } }

// TakeRateAs counts n requests against key as member traffic or as anonymous traffic.
func (s *Server) TakeRateAs(member bool, key string, n, lim int, window time.Duration) int {
	ctx := context.Background()
	if member {
		ctx = memberTraffic(ctx)
	}
	return s.limits.take(ctx, nil, rateTake{key, limit{lim, window}, n})[0].n
}

func (s *Server) AnalyticsKeyKnown(ctx context.Context, key string) (bool, error) {
	cp, err := s.analytics.props.get(ctx, s.st, key, s.now())
	return cp != nil, err
}

func NewAnalyticsKey() string { return newAnalyticsKey() }

const PublicCachePerPage = publicCachePerPage

type PublicCache struct{ c *publicCache }

func NewPublicCache(max int) PublicCache { return PublicCache{newPublicCache(time.Hour, max)} }

func (p PublicCache) Put(key string, at time.Time) { p.c.put(key, publicEntry{at: at}) }

func (p PublicCache) Has(key string, now time.Time) bool {
	_, ok := p.c.get(key, time.Time{}, now)
	return ok
}

func (p PublicCache) Len() int {
	p.c.mu.Lock()
	defer p.c.mu.Unlock()
	return len(p.c.entries)
}

// ForgetStatusDomains empties the own-domain cache, as its 30 seconds running out would.
func (s *Server) ForgetStatusDomains() {
	c := s.statusDomainCache
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.verified, c.misses = map[string]statusDomainHit{}, map[string]time.Time{}
}

func ConfirmAddressKey(addr string) string { return confirmAddressKey(addr) }

func SetTypedConfirmWorkspaceLimit(n int) (restore func()) {
	old := limitTypedConfirmPerWorkspace
	limitTypedConfirmPerWorkspace = limit{n, old.window}
	return func() { limitTypedConfirmPerWorkspace = old }
}

type Sockets struct{ c *clientSockets }

type Socket struct{ s *clientSocket }

func NewSockets() Sockets { return Sockets{newClientSockets()} }

func (x Sockets) Open(session, contact uuid.UUID, addr string) Socket {
	return Socket{x.c.open(session, contact, addr)}
}

func (x Sockets) Ready(s Socket) bool { return x.c.ready(s.s) }

func (x Sockets) Close(s Socket) bool { return x.c.close(s.s) }

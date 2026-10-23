package api

import (
	"context"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) TakeRate(ctx context.Context, key string, n, lim int, window time.Duration) (int, time.Duration, error) {
	g, err := s.limits.take(ctx, s.now(), rateTake{key, limit{lim, window}, n})
	if err != nil {
		return 0, 0, err
	}
	return g[0].n, g[0].wait, nil
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

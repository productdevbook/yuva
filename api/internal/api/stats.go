package api

import (
	"cmp"
	"context"
	"slices"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxStatsWindow = 366 * 24 * time.Hour

func (s *Server) GetStats(ctx context.Context, req oas.GetStatsRequestObject) (oas.GetStatsResponseObject, error) {
	p := principalFrom(ctx)
	now := s.now()
	since, err := statsSince(req.Params, now)
	if err != nil {
		return nil, err
	}
	if req.Params.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *req.Params.InboxId); err != nil {
			return nil, err
		}
	}
	arg := store.StatsRepliesParams{
		WorkspaceID: p.workspaceID, Since: since, Until: now, InboxID: req.Params.InboxId,
		AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	}
	replies, err := s.st.StatsReplies(ctx, arg)
	if err != nil {
		return nil, err
	}
	closed, err := s.st.StatsClosed(ctx, store.StatsClosedParams(arg))
	if err != nil {
		return nil, err
	}
	firsts, err := s.st.StatsFirstReplies(ctx, store.StatsFirstRepliesParams(arg))
	if err != nil {
		return nil, err
	}
	ratings, err := s.st.StatsRatings(ctx, store.StatsRatingsParams(arg))
	if err != nil {
		return nil, err
	}
	out := oas.GetStats200JSONResponse{
		Since: since, Until: now, FirstReplies: int64(len(firsts)), Members: []oas.MemberStats{},
		Ratings: oas.RatingStats{Inboxes: []oas.InboxRatingStats{}},
	}
	for _, r := range ratings {
		if n := len(out.Ratings.Inboxes); n == 0 || out.Ratings.Inboxes[n-1].InboxId != r.InboxID {
			out.Ratings.Inboxes = append(out.Ratings.Inboxes, oas.InboxRatingStats{InboxId: r.InboxID})
		}
		per := &out.Ratings.Inboxes[len(out.Ratings.Inboxes)-1]
		if r.Rating == string(oas.Good) {
			out.Ratings.Good, per.Good = out.Ratings.Good+r.N, per.Good+r.N
		} else {
			out.Ratings.Bad, per.Bad = out.Ratings.Bad+r.N, per.Bad+r.N
		}
	}
	perMember := map[uuid.UUID]*oas.MemberStats{}
	member := func(id uuid.UUID) *oas.MemberStats {
		if m := perMember[id]; m != nil {
			return m
		}
		m := &oas.MemberStats{MemberId: id}
		perMember[id] = m
		return m
	}
	for _, r := range replies {
		out.Replies += r.N
		if id := cmp.Or(r.SentByMemberID, r.AuthorMemberID); id != nil {
			member(*id).Replies += r.N
		}
	}
	for _, r := range closed {
		out.Closed += r.N
		if r.AuthorMemberID != nil {
			member(*r.AuthorMemberID).Closed += r.N
		}
	}
	for _, m := range perMember {
		out.Members = append(out.Members, *m)
	}
	slices.SortFunc(out.Members, func(a, b oas.MemberStats) int { return a.MemberId.Compare(b.MemberId) })
	if len(firsts) > 0 {
		out.MedianFirstReplySeconds = new(medianSeconds(firsts))
	}
	return out, nil
}

func statsSince(params oas.GetStatsParams, now time.Time) (time.Time, error) {
	tz := "UTC"
	if params.Timezone != nil {
		tz = *params.Timezone
	}
	if err := validTimezone(tz); err != nil {
		return time.Time{}, err
	}
	if params.Since != nil {
		since := *params.Since
		if !since.Before(now) {
			return time.Time{}, errValidation("since must be in the past")
		}
		if now.Sub(since) > maxStatsWindow {
			return time.Time{}, errValidation("since must be at most 366 days ago")
		}
		return since, nil
	}
	loc, _ := time.LoadLocation(tz)
	t := now.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), nil
}

func medianSeconds(rows []store.StatsFirstRepliesRow) int64 {
	d := make([]time.Duration, len(rows))
	for i, r := range rows {
		d[i] = r.RepliedAt.Sub(r.AskedAt)
	}
	slices.Sort(d)
	mid := len(d) / 2
	if len(d)%2 == 1 {
		return int64(d[mid] / time.Second)
	}
	return int64((d[mid-1] + d[mid]) / 2 / time.Second)
}

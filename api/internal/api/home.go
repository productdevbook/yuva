package api

import (
	"cmp"
	"context"
	"slices"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	homeWaitingShown  = 5
	homeIdeasShown    = 5
	homeUpcomingShown = 5
	homeDayFormat     = "2006-01-02"
)

type homeScope struct {
	p                     principal
	inbox                 *uuid.UUID
	days                  int
	tz                    string
	loc                   *time.Location
	now, since, prevSince time.Time
	sinceDay              string
	dayIndex              map[string]int
}

func (s *Server) GetHome(ctx context.Context, req oas.GetHomeRequestObject) (oas.GetHomeResponseObject, error) {
	p := principalFrom(ctx)
	sc, err := s.homeScope(ctx, p, req.Params)
	if err != nil {
		return nil, err
	}
	out := oas.GetHome200JSONResponse{Period: oas.HomePeriod{Days: sc.days, Timezone: sc.tz, Since: sc.since, Until: sc.now.In(sc.loc), PreviousSince: sc.prevSince}}
	if out.Inbox, err = s.homeInbox(ctx, sc); err != nil {
		return nil, err
	}
	products, err := s.homeOpenLoad(ctx, sc, &out)
	if err != nil {
		return nil, err
	}
	if out.Activity, err = s.homeActivity(ctx, sc, products); err != nil {
		return nil, err
	}
	if out.StatusPages, err = s.homeStatusPages(ctx, sc); err != nil {
		return nil, err
	}
	if !s.roadmap.Off {
		ideas, err := s.homeIdeas(ctx, sc)
		if err != nil {
			return nil, err
		}
		out.Ideas = &ideas
	}
	if out.Upcoming, err = s.homeUpcoming(ctx, sc); err != nil {
		return nil, err
	}
	if s.analytics != nil {
		if out.Analytics, err = s.homeAnalytics(ctx, sc); err != nil {
			return nil, err
		}
	}
	if s.broadcastMode() != broadcastsOff {
		if out.Broadcasts, err = s.homeBroadcasts(ctx, sc); err != nil {
			return nil, err
		}
	}
	if p.seesAllInboxes() {
		parts, err := s.setupParts(ctx, p, setupWant{inboxes: true, channels: true, properties: true, lists: true})
		if err != nil {
			return nil, err
		}
		out.Setup = &oas.HomeSetup{Items: parts.checklist()}
	}
	return out, nil
}

func (s *Server) homeScope(ctx context.Context, p principal, prm oas.GetHomeParams) (homeScope, error) {
	sc := homeScope{p: p, days: int(oas.HomeDays14), tz: "UTC", inbox: prm.InboxId}
	if prm.Days != nil {
		if !prm.Days.Valid() {
			return sc, errValidation("days must be 7, 14 or 30")
		}
		sc.days = int(*prm.Days)
	}
	if prm.Timezone != nil {
		sc.tz = *prm.Timezone
	}
	if err := validTimezone(sc.tz); err != nil {
		return sc, err
	}
	if sc.inbox != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *sc.inbox); err != nil {
			return sc, err
		}
	}
	sc.loc, _ = time.LoadLocation(sc.tz)
	sc.now = s.now()
	local := sc.now.In(sc.loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, sc.loc)
	sc.since = today.AddDate(0, 0, 1-sc.days)
	sc.prevSince = sc.since.AddDate(0, 0, -sc.days)
	sc.sinceDay = sc.since.Format(homeDayFormat)
	sc.dayIndex = make(map[string]int, sc.days)
	for i := range sc.days {
		sc.dayIndex[sc.since.AddDate(0, 0, i).Format(homeDayFormat)] = i
	}
	return sc, nil
}

func (s *Server) homeInbox(ctx context.Context, sc homeScope) (oas.HomeInbox, error) {
	p := sc.p
	rows, err := s.st.HomeInbox(ctx, store.HomeInboxParams{WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(), WaitingLimit: homeWaitingShown})
	if err != nil {
		return oas.HomeInbox{}, err
	}
	out := oas.HomeInbox{WaitingConversations: []oas.HomeWaitingConversation{}}
	for i, r := range rows {
		if i == 0 {
			out.Open, out.Unassigned, out.Waiting = r.Open, r.Unassigned, r.Waiting
		}
		if r.OldestID == nil || r.OldestContactID == nil {
			continue
		}
		w := oas.HomeWaitingConversation{
			Id: *r.OldestID, InboxId: *r.OldestInboxID, Subject: derefString(r.OldestSubject), Since: *r.OldestSince, Preview: previewText(r.OldestPreview),
			Contact: oas.HomeContact{Id: *r.OldestContactID, Name: derefString(r.OldestContactName)},
		}
		if r.OldestChannelKind != nil {
			w.ChannelKind = new(oas.ChannelKind(*r.OldestChannelKind))
		}
		if r.OldestContactEmail != nil {
			w.Contact.Email = new(openapi_types.Email(*r.OldestContactEmail))
		}
		out.WaitingConversations = append(out.WaitingConversations, w)
	}
	if len(out.WaitingConversations) > 0 {
		out.OldestWaiting = &out.WaitingConversations[0]
	}
	return out, nil
}

func (s *Server) homeOpenLoad(ctx context.Context, sc homeScope, out *oas.GetHome200JSONResponse) (map[uuid.UUID]*oas.HomeProduct, error) {
	p := sc.p
	rows, err := s.st.HomeOpenLoad(ctx, store.HomeOpenLoadParams{WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef()})
	if err != nil {
		return nil, err
	}
	out.Products = []oas.HomeProduct{}
	index := map[uuid.UUID]int{}
	perMember := map[uuid.UUID]int64{}
	for _, r := range rows {
		i, ok := index[r.InboxID]
		if !ok {
			i = len(out.Products)
			index[r.InboxID] = i
			out.Products = append(out.Products, oas.HomeProduct{InboxId: r.InboxID})
		}
		out.Products[i].Open += r.N
		if r.Feedback {
			out.Inbox.Feedback += r.N
		}
		if r.AssigneeID != nil && r.N > 0 {
			perMember[*r.AssigneeID] += r.N
		}
	}
	out.Team = oas.HomeTeam{Members: make([]oas.HomeMemberLoad, 0, len(perMember))}
	for id, n := range perMember {
		out.Team.Members = append(out.Team.Members, oas.HomeMemberLoad{MemberId: id, Open: n})
	}
	slices.SortFunc(out.Team.Members, func(a, b oas.HomeMemberLoad) int {
		return cmp.Or(cmp.Compare(b.Open, a.Open), a.MemberId.Compare(b.MemberId))
	})
	products := make(map[uuid.UUID]*oas.HomeProduct, len(index))
	for id, i := range index {
		products[id] = &out.Products[i]
	}
	return products, nil
}

func (s *Server) homeActivity(ctx context.Context, sc homeScope, products map[uuid.UUID]*oas.HomeProduct) (oas.HomeActivity, error) {
	p := sc.p
	out := oas.HomeActivity{Days: make([]oas.HomeActivityDay, sc.days)}
	for i := range out.Days {
		out.Days[i] = oas.HomeActivityDay{Day: openapi_types.Date{Time: sc.since.AddDate(0, 0, i)}, Inboxes: []oas.HomeInboxConversations{}}
	}
	day := func(d string) *oas.HomeActivityDay {
		if i, ok := sc.dayIndex[d]; ok {
			return &out.Days[i]
		}
		return nil
	}
	conversations, err := s.st.HomeConversationsByDay(ctx, store.HomeConversationsByDayParams{
		Timezone: sc.tz, WorkspaceID: p.workspaceID, Since: sc.prevSince, Until: sc.now, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(),
	})
	if err != nil {
		return out, err
	}
	slices.SortFunc(conversations, func(a, b store.HomeConversationsByDayRow) int {
		return cmp.Or(a.Day.Compare(b.Day), cmp.Compare(b.N, a.N), a.InboxID.Compare(b.InboxID))
	})
	for _, r := range conversations {
		d := r.Day.Format(homeDayFormat)
		if d < sc.sinceDay {
			out.Previous.Conversations += r.N
			continue
		}
		if at := day(d); at != nil {
			at.Conversations += r.N
			at.Inboxes = append(at.Inboxes, oas.HomeInboxConversations{InboxId: r.InboxID, Conversations: r.N})
			out.Conversations += r.N
			if pr := products[r.InboxID]; pr != nil {
				pr.Conversations += r.N
			}
		}
	}
	ratings, err := s.st.HomeRatingsByDay(ctx, store.HomeRatingsByDayParams{
		Timezone: sc.tz, WorkspaceID: p.workspaceID, Since: sc.prevSince, Until: sc.now, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return out, err
	}
	for _, r := range ratings {
		good := r.Rating == string(oas.Good)
		count := func(g, b *int64) {
			if good {
				*g += r.N
			} else {
				*b += r.N
			}
		}
		d := r.Day.Format(homeDayFormat)
		if d < sc.sinceDay {
			count(&out.Previous.Good, &out.Previous.Bad)
			continue
		}
		if at := day(d); at != nil {
			count(&at.Good, &at.Bad)
			count(&out.Good, &out.Bad)
			if pr := products[r.InboxID]; pr != nil {
				count(&pr.Good, &pr.Bad)
			}
		}
	}
	firsts, err := s.st.StatsFirstReplies(ctx, store.StatsFirstRepliesParams{
		WorkspaceID: p.workspaceID, Since: sc.prevSince, Until: sc.now, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return out, err
	}
	var previous, current []store.StatsFirstRepliesRow
	perDay := make([][]store.StatsFirstRepliesRow, sc.days)
	for _, r := range firsts {
		if r.RepliedAt.Before(sc.since) {
			previous = append(previous, r)
			continue
		}
		if i, ok := sc.dayIndex[r.RepliedAt.In(sc.loc).Format(homeDayFormat)]; ok {
			current = append(current, r)
			perDay[i] = append(perDay[i], r)
		}
	}
	median := func(rows []store.StatsFirstRepliesRow) (int64, *int64) {
		if len(rows) == 0 {
			return 0, nil
		}
		return int64(len(rows)), new(medianSeconds(rows))
	}
	out.FirstReplies, out.MedianFirstReplySeconds = median(current)
	out.Previous.FirstReplies, out.Previous.MedianFirstReplySeconds = median(previous)
	for i, rows := range perDay {
		out.Days[i].FirstReplies, out.Days[i].MedianFirstReplySeconds = median(rows)
	}
	return out, nil
}

func (s *Server) homeStatusPages(ctx context.Context, sc homeScope) ([]oas.HomeStatusPage, error) {
	p := sc.p
	rows, err := s.st.HomeStatusComponents(ctx, store.HomeStatusComponentsParams{WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, r := range rows {
		if r.ComponentID != nil {
			ids = append(ids, *r.ComponentID)
		}
	}
	by := map[uuid.UUID][]stateSpan{}
	if len(ids) > 0 {
		spans, err := s.st.StatusComponentSpans(ctx, store.StatusComponentSpansParams{WorkspaceID: p.workspaceID, ComponentIds: ids, Since: sc.now.AddDate(0, 0, -sc.days-1)})
		if err != nil {
			return nil, err
		}
		for _, sp := range spans {
			var end time.Time
			if sp.EndedAt != nil {
				end = *sp.EndedAt
			}
			by[sp.ComponentID] = append(by[sp.ComponentID], stateSpan{state: sp.State, start: sp.StartedAt, end: end})
		}
	}
	out := []oas.HomeStatusPage{}
	var merged []uptimeDay
	var states []string
	finish := func() {
		if n := len(out); n > 0 {
			out[n-1].State = oas.StatusComponentStateValue(worstState(states))
			out[n-1].Days = homeStatusDays(merged)
		}
	}
	for _, r := range rows {
		if n := len(out); n == 0 || out[n-1].Id != r.PageID {
			finish()
			out = append(out, oas.HomeStatusPage{Id: r.PageID, InboxId: r.InboxID, Name: r.PageName, Published: r.Published, Components: []oas.HomeStatusComponent{}})
			merged, states = make([]uptimeDay, sc.days), nil
		}
		if r.ComponentID == nil {
			continue
		}
		days := uptimeDays(by[*r.ComponentID], sc.days, propertyLocation(r.Timezone), sc.now)
		for i, d := range days {
			m := &merged[i]
			if !m.known {
				m.date, m.state = d.date, stOperational
			}
			if d.known {
				m.known = true
				m.state = worseState(m.state, d.state)
				m.counts += d.counts
				m.down += d.down
			}
		}
		states = append(states, *r.ComponentState)
		page := &out[len(out)-1]
		page.Components = append(page.Components, oas.HomeStatusComponent{
			Id: *r.ComponentID, Name: derefString(r.ComponentName), State: oas.StatusComponentStateValue(*r.ComponentState), Days: homeStatusDays(days),
		})
	}
	finish()
	return out, nil
}

func homeStatusDays(days []uptimeDay) []oas.PublicStatusDay {
	out := []oas.PublicStatusDay{}
	for _, d := range days {
		if d.known {
			out = append(out, oas.PublicStatusDay{Date: openapi_types.Date{Time: d.date}, Uptime: d.uptime(), State: oas.StatusComponentStateValue(d.state)})
		}
	}
	return out
}

func (s *Server) homeIdeas(ctx context.Context, sc homeScope) ([]oas.HomeIdea, error) {
	p := sc.p
	rows, err := s.st.HomeIdeas(ctx, store.HomeIdeasParams{WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), Lim: homeIdeasShown})
	if err != nil {
		return nil, err
	}
	out := make([]oas.HomeIdea, len(rows))
	for i, r := range rows {
		out[i] = oas.HomeIdea{Id: r.ID, InboxId: r.InboxID, Title: r.Title, Votes: int64(r.VoteCount), Stage: oas.IdeaStage(r.Stage)}
	}
	return out, nil
}

func (s *Server) homeUpcoming(ctx context.Context, sc homeScope) ([]oas.HomeUpcoming, error) {
	p := sc.p
	rows, err := s.st.HomeUpcoming(ctx, store.HomeUpcomingParams{
		WithBroadcasts: s.broadcastMode() != broadcastsOff, WithWhatsNew: !s.changelog.Off, Lim: homeUpcomingShown,
		WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]oas.HomeUpcoming, len(rows))
	for i, r := range rows {
		u := oas.HomeUpcoming{Kind: oas.HomeUpcomingKind(r.Kind), Id: r.ID, InboxId: r.InboxID, At: r.At, Title: r.Title}
		switch u.Kind {
		case oas.HomeUpcomingBroadcast:
			u.Recipients = new(r.Recipients)
		case oas.HomeUpcomingWhatsNew:
			u.ChangelogId, u.Locale = r.ParentID, new(r.Locale)
		default:
			u.StatusPageId = r.ParentID
		}
		out[i] = u
	}
	return out, nil
}

func (s *Server) homeAnalytics(ctx context.Context, sc homeScope) (*oas.HomeAnalytics, error) {
	p := sc.p
	props, err := s.st.ListAnalyticsProperties(ctx, store.ListAnalyticsPropertiesParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	if sc.inbox != nil {
		props = slices.DeleteFunc(props, func(prop store.AnalyticsProperty) bool { return prop.InboxID == nil || *prop.InboxID != *sc.inbox })
	}
	if len(props) == 0 && !p.seesAllInboxes() {
		return nil, nil
	}
	want := make([]analytics.HomeProperty, len(props))
	for i, prop := range props {
		want[i] = analytics.HomeProperty{ID: prop.ID, Loc: propertyLocation(prop.Timezone)}
	}
	visitors, err := s.analytics.store.Home(ctx, p.workspaceID, want, s.rawFrom(), sc.now)
	if err != nil {
		return nil, err
	}
	out := &oas.HomeAnalytics{Properties: make([]oas.HomeProperty, len(props))}
	for i, prop := range props {
		v := visitors[prop.ID]
		hp := oas.HomeProperty{
			Id: prop.ID, InboxId: prop.InboxID, Name: prop.Name, Timezone: prop.Timezone, Domains: make([]oas.AnalyticsDomain, len(prop.Domains)),
			Platforms: make([]oas.AnalyticsPlatform, len(prop.Platforms)), RealtimeVisitors: v.Realtime, Days: make([]oas.HomeDay, len(v.Days)),
		}
		for j, d := range prop.Domains {
			hp.Domains[j] = oas.AnalyticsDomain(d)
		}
		for j, pl := range prop.Platforms {
			hp.Platforms[j] = oas.AnalyticsPlatform(pl)
		}
		for j, d := range v.Days {
			hp.Days[j] = oas.HomeDay{Day: openapi_types.Date{Time: d}, Visitors: v.Visitors[j]}
		}
		if n := len(v.Visitors); n > 0 {
			hp.VisitorsToday = v.Visitors[n-1]
		}
		out.Properties[i] = hp
	}
	return out, nil
}

func (s *Server) homeBroadcasts(ctx context.Context, sc homeScope) (*oas.HomeBroadcasts, error) {
	p := sc.p
	b, err := s.st.HomeLastBroadcast(ctx, store.HomeLastBroadcastParams{WorkspaceID: p.workspaceID, InboxID: sc.inbox, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if store.IsNotFound(err) {
		return &oas.HomeBroadcasts{}, nil
	}
	if err != nil {
		return nil, err
	}
	c := parseCounts(b.Counts)
	sentAt := b.UpdatedAt
	if b.FinishedAt != nil {
		sentAt = *b.FinishedAt
	}
	return &oas.HomeBroadcasts{LastSent: &oas.HomeBroadcast{
		Id: b.ID, InboxId: b.InboxID, Name: b.Name, Subject: b.Subject, SentAt: sentAt, ClickTracking: b.ClickTracking,
		Recipients: c.Recipients, Sent: c.Sent, Clicked: c.Clicked, Replied: c.Replied,
	}}, nil
}

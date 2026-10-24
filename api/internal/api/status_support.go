package api

import (
	"context"
	"encoding/json"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const statusReplyAttempts = 8

func (s *Server) clientStatus(ctx context.Context, q *store.Queries, inboxID, ws uuid.UUID) (*oas.ClientInboxStatus, error) {
	pg, err := q.GetInboxStatusPage(ctx, store.GetInboxStatusPageParams{WorkspaceID: ws, InboxID: inboxID})
	if store.IsNotFound(err) || (err == nil && !pg.Published) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	key := pg.ID.String() + "|client"
	if e, ok := s.statusCache.get(key, pg.ChangedAt, now); ok {
		var out oas.ClientInboxStatus
		if err := json.Unmarshal(e.body, &out); err == nil {
			return &out, nil
		}
	}
	comps, err := q.ListStatusComponents(ctx, store.ListStatusComponentsParams{WorkspaceID: ws, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	incs, err := q.ClientStatusIncidents(ctx, store.ClientStatusIncidentsParams{WorkspaceID: ws, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	out := oas.ClientInboxStatus{Slug: &pg.Slug, Url: s.pageURL(pg, ""), State: oas.StatusComponentStateValue(pageState(comps)), Open: []oas.ClientStatusIncident{}}
	for _, i := range incs {
		out.Open = append(out.Open, oas.ClientStatusIncident{
			Id: i.ID, Kind: oas.StatusIncidentKind(i.Kind), Title: i.Title, Impact: oas.StatusIncidentImpact(i.Impact),
			State: oas.StatusIncidentState(i.State), UpdatedAt: i.LastUpdateAt, Url: s.pageURL(pg, "/incidents/"+i.ID.String()),
		})
	}
	s.statusCache.put(key, statusCacheEntry{changed: pg.ChangedAt, at: now, ctype: "application/json", body: mustJSON(out)})
	return &out, nil
}

func clientStatusEvent(typ string) bool {
	switch typ {
	case statusEventComponent, statusEventCreated, statusEventUpdated, statusEventResolved, realtime.StatusHint:
		return true
	}
	return false
}

// statusHint lets the inbox's contact connections compare their status line after a change that
// stores no event, such as publishing or deleting the page.
func (s *Server) statusHint(ctx context.Context, ws, inboxID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: realtime.StatusHint, WorkspaceID: ws, InboxID: &inboxID, Data: json.RawMessage("{}")})
}

func (s *Server) statusPageSignal(ctx context.Context, ws, inboxID, pageID uuid.UUID) {
	s.signal(ctx, realtime.Event{Type: eventStatusPage, WorkspaceID: ws, InboxID: &inboxID, Data: mustJSON(map[string]uuid.UUID{"page_id": pageID})})
}

func statusLinkTargets(ctx context.Context, q *store.Queries, p principal, pageID, incidentID, conversationID uuid.UUID) (store.StatusIncident, store.Conversation, error) {
	pg, err := visibleStatusPage(ctx, q, p, pageID, true)
	if err != nil {
		return store.StatusIncident{}, store.Conversation{}, err
	}
	inc, err := lockedStatusIncident(ctx, q, pg, incidentID)
	if err != nil {
		return inc, store.Conversation{}, err
	}
	c, err := visibleConversation(ctx, q, p, conversationID, true)
	return inc, c, err
}

func (s *Server) LinkStatusIncidentConversation(ctx context.Context, req oas.LinkStatusIncidentConversationRequestObject) (oas.LinkStatusIncidentConversationResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		inc, c, err := statusLinkTargets(ctx, q, p, req.PageId, req.IncidentId, req.ConversationId)
		if err != nil {
			return err
		}
		linked, err := q.StatusIncidentConversationLinked(ctx, store.StatusIncidentConversationLinkedParams{
			WorkspaceID: p.workspaceID, IncidentID: inc.ID, ConversationID: c.ID,
		})
		if err != nil || linked {
			return err
		}
		n, err := q.CountStatusIncidentConversations(ctx, store.CountStatusIncidentConversationsParams{WorkspaceID: p.workspaceID, IncidentID: inc.ID})
		if err != nil {
			return err
		}
		if int(n) >= maxIncidentLinks {
			return errTooManyLinks()
		}
		member, key := statusAuthor(p)
		return q.LinkStatusIncidentConversation(ctx, store.LinkStatusIncidentConversationParams{
			WorkspaceID: p.workspaceID, IncidentID: inc.ID, ConversationID: c.ID, LinkedByMemberID: member, LinkedByApiKeyID: key, Now: s.now(),
		})
	})
	if err != nil {
		return nil, err
	}
	return oas.LinkStatusIncidentConversation204Response{}, nil
}

func (s *Server) UnlinkStatusIncidentConversation(ctx context.Context, req oas.UnlinkStatusIncidentConversationRequestObject) (oas.UnlinkStatusIncidentConversationResponseObject, error) {
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		inc, c, err := statusLinkTargets(ctx, q, p, req.PageId, req.IncidentId, req.ConversationId)
		if err != nil {
			return err
		}
		return q.UnlinkStatusIncidentConversation(ctx, store.UnlinkStatusIncidentConversationParams{
			WorkspaceID: p.workspaceID, IncidentID: inc.ID, ConversationID: c.ID,
		})
	})
	if err != nil {
		return nil, err
	}
	return oas.UnlinkStatusIncidentConversation204Response{}, nil
}

func visibleStatusIncident(ctx context.Context, q *store.Queries, p principal, pageID, incidentID uuid.UUID) (store.StatusIncident, error) {
	pg, err := visibleStatusPage(ctx, q, p, pageID, false)
	if err != nil {
		return store.StatusIncident{}, err
	}
	inc, err := q.GetStatusIncident(ctx, store.GetStatusIncidentParams{WorkspaceID: p.workspaceID, PageID: pg.ID, ID: incidentID})
	if store.IsNotFound(err) {
		return inc, errStatusIncidentGone
	}
	return inc, err
}

func (s *Server) ListStatusIncidentConversations(ctx context.Context, req oas.ListStatusIncidentConversationsRequestObject) (oas.ListStatusIncidentConversationsResponseObject, error) {
	p := principalFrom(ctx)
	limit, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	inc, err := visibleStatusIncident(ctx, s.st.Queries, p, req.PageId, req.IncidentId)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListStatusIncidentConversations(ctx, store.ListStatusIncidentConversationsParams{
		WorkspaceID: p.workspaceID, IncidentID: inc.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		CursorAt: at, CursorID: id, Lim: limit + 1,
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListStatusIncidentConversations200JSONResponse{Items: []oas.StatusIncidentConversation{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		cur := encodeCursor(last.LinkedAt, last.Conversation.ID)
		out.NextCursor = &cur
	}
	convs := make([]store.Conversation, len(rows))
	for i, r := range rows {
		convs[i] = r.Conversation
	}
	items, err := s.listItems(ctx, p, convs)
	if err != nil {
		return nil, err
	}
	for i, c := range items {
		out.Items = append(out.Items, oas.StatusIncidentConversation{
			Id: c.Id, InboxId: c.InboxId, InboxPurpose: c.InboxPurpose, ContactId: c.ContactId, ChannelId: c.ChannelId, ChannelKind: c.ChannelKind, Kind: c.Kind, Feedback: c.Feedback, Page: c.Page, Subject: c.Subject,
			Status: c.Status, SnoozeUntil: c.SnoozeUntil, Priority: c.Priority, Spam: c.Spam, AssigneeId: c.AssigneeId, TeamId: c.TeamId, AssignWaitingSince: c.AssignWaitingSince, Labels: c.Labels,
			LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Starred: c.Starred,
			RelatedConversationId: c.RelatedConversationId, ClosedAt: c.ClosedAt, Rating: c.Rating, LastReadByContactAt: c.LastReadByContactAt, BroadcastId: c.BroadcastId,
			Imported: c.Imported, Contact: c.Contact, LastMessage: c.LastMessage, Unread: c.Unread, PinnedAt: c.PinnedAt, LinkedAt: rows[i].LinkedAt,
		})
	}
	return out, nil
}

func (s *Server) ListConversationIncidents(ctx context.Context, req oas.ListConversationIncidentsRequestObject) (oas.ListConversationIncidentsResponseObject, error) {
	p := principalFrom(ctx)
	c, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListConversationIncidents(ctx, store.ListConversationIncidentsParams{
		WorkspaceID: p.workspaceID, ConversationID: c.ID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListConversationIncidents200JSONResponse{Items: []oas.ConversationIncident{}}
	for _, r := range rows {
		i := r.StatusIncident
		out.Items = append(out.Items, oas.ConversationIncident{
			Id: i.ID, Page: oas.StatusPageRef{Id: i.PageID, Name: r.PageName, Slug: r.PageSlug}, Kind: oas.StatusIncidentKind(i.Kind),
			Title: i.Title, Impact: oas.StatusIncidentImpact(i.Impact), State: oas.StatusIncidentState(i.State),
			Url: s.statusURLFor(r.PageSlug, r.PageDomain, r.PageDomainState, r.PageCfStatus, r.PageCfSslStatus, "/incidents/"+i.ID.String()), PublishedAt: i.PublishedAt, ResolvedAt: i.ResolvedAt, LinkedAt: r.LinkedAt,
		})
	}
	return out, nil
}

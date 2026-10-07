package api

import (
	"context"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	assigneeMe         = "me"
	assigneeUnassigned = "unassigned"
	maxLabels          = 50
)

var errAssigneeAccess = errValidation("assignee_id must be a member with access to the inbox")

func searchQuery(q *string) (*string, error) {
	if q == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*q)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > 200 {
		return nil, errValidation("q must be at most 200 characters")
	}
	return &v, nil
}

func conversationBody(c store.Conversation, labels []uuid.UUID) oas.Conversation {
	if labels == nil {
		labels = []uuid.UUID{}
	}
	return oas.Conversation{
		Id: c.ID, InboxId: c.InboxID, ContactId: c.ContactID, ChannelId: c.ChannelID, Subject: c.Subject,
		Status: oas.ConversationStatus(c.Status), SnoozeUntil: c.SnoozeUntil, Priority: oas.Priority(c.Priority),
		AssigneeId: c.AssigneeID, Labels: labels, LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func conversationLabels(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := q.ListConversationLabels(ctx, store.ListConversationLabelsParams{WorkspaceID: workspaceID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]uuid.UUID{}
	for _, r := range rows {
		out[r.ConversationID] = append(out[r.ConversationID], r.LabelID)
	}
	return out, nil
}

func oneConversation(ctx context.Context, q *store.Queries, c store.Conversation) (oas.Conversation, error) {
	labels, err := conversationLabels(ctx, q, c.WorkspaceID, []uuid.UUID{c.ID})
	if err != nil {
		return oas.Conversation{}, err
	}
	return conversationBody(c, labels[c.ID]), nil
}

func labelSet(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, in []uuid.UUID) ([]uuid.UUID, error) {
	if len(in) > maxLabels {
		return nil, errValidation("at most 50 labels")
	}
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return a.Compare(b) })
	out = slices.Compact(out)
	if len(out) == 0 {
		return out, nil
	}
	n, err := q.CountLabels(ctx, store.CountLabelsParams{WorkspaceID: workspaceID, Ids: out})
	if err != nil {
		return nil, err
	}
	if int(n) != len(out) {
		return nil, errValidation("labels names a label that does not exist")
	}
	return out, nil
}

func validPriority(p oas.Priority) bool { return p.Valid() }

func (s *Server) writeEvent(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, ev oas.MessageEvent, at time.Time) error {
	author, member := authorFor(p)
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: p.workspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindEvent),
		AuthorType: author, AuthorMemberID: member, Event: mustJSON(ev), CreatedAt: at,
	})
	if err != nil {
		return err
	}
	events.conversation(realtime.MessageCreated, c, messageBody(msg, nil))
	return nil
}

func authorFor(p principal) (string, *uuid.UUID) {
	if p.isKey() {
		return string(oas.AuthorTypeSystem), nil
	}
	id := p.memberID
	return string(oas.AuthorTypeMember), &id
}

func (s *Server) recordChanges(ctx context.Context, q *store.Queries, events *eventBatch, p principal, before, after store.Conversation, added, removed []uuid.UUID, at time.Time) error {
	if !sameID(before.AssigneeID, after.AssigneeID) {
		ev := oas.MessageEvent{Type: oas.Unassigned, PreviousAssigneeId: before.AssigneeID}
		if after.AssigneeID != nil {
			ev = oas.MessageEvent{Type: oas.Assigned, AssigneeId: after.AssigneeID, PreviousAssigneeId: before.AssigneeID}
		}
		if err := s.writeEvent(ctx, q, events, p, after, ev, at); err != nil {
			return err
		}
	}
	if before.Status != after.Status {
		st, prev := oas.ConversationStatus(after.Status), oas.ConversationStatus(before.Status)
		if err := s.writeEvent(ctx, q, events, p, after, oas.MessageEvent{Type: oas.StatusChanged, Status: &st, PreviousStatus: &prev}, at); err != nil {
			return err
		}
	}
	if len(added) > 0 || len(removed) > 0 {
		if added == nil {
			added = []uuid.UUID{}
		}
		if removed == nil {
			removed = []uuid.UUID{}
		}
		if err := s.writeEvent(ctx, q, events, p, after, oas.MessageEvent{Type: oas.LabelsChanged, AddedLabels: &added, RemovedLabels: &removed}, at); err != nil {
			return err
		}
	}
	return nil
}

func conversationChanged(a, b store.Conversation) bool {
	return a.Subject != b.Subject || a.Status != b.Status || a.Priority != b.Priority ||
		!sameID(a.AssigneeID, b.AssigneeID) || !sameTime(a.SnoozeUntil, b.SnoozeUntil)
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Server) ListConversations(ctx context.Context, req oas.ListConversationsRequestObject) (oas.ListConversationsResponseObject, error) {
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	at, cid, err := decodeCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	q, err := searchQuery(prm.Q)
	if err != nil {
		return nil, err
	}
	arg := store.ListConversationsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), MemberID: p.memberID,
		InboxID: prm.InboxId, LabelID: prm.LabelId, Q: q, CursorAt: at, CursorID: cid, Lim: lim + 1,
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	if prm.Status != nil {
		if !prm.Status.Valid() {
			return nil, errValidation("status must be open, pending, snoozed or closed")
		}
		st := string(*prm.Status)
		arg.Status = &st
	}
	if prm.Assignee != nil {
		switch a := strings.TrimSpace(*prm.Assignee); a {
		case assigneeMe:
			if p.isKey() {
				return nil, errValidation("assignee=me needs a member session")
			}
			arg.AssigneeID = &p.memberID
		case assigneeUnassigned:
			arg.Unassigned = true
		default:
			id, err := uuid.Parse(a)
			if err != nil {
				return nil, errValidation("assignee must be a member id, me or unassigned")
			}
			arg.AssigneeID = &id
		}
	}
	rows, err := s.st.ListConversations(ctx, arg)
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].LastActivityAt, rows[lim-1].ID)
		next = &c
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	labels, err := conversationLabels(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	out := oas.ListConversations200JSONResponse{Items: make([]oas.Conversation, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = conversationBody(r, labels[r.ID])
	}
	return out, nil
}

func (s *Server) CreateConversation(ctx context.Context, req oas.CreateConversationRequestObject) (oas.CreateConversationResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	subject := ""
	var err error
	if b.Subject != nil {
		if subject, err = trimmed(*b.Subject, 0, 500, "subject"); err != nil {
			return nil, err
		}
	}
	priority := oas.Normal
	if b.Priority != nil {
		if !validPriority(*b.Priority) {
			return nil, errValidation("priority must be low, normal, high or urgent")
		}
		priority = *b.Priority
	}
	var out oas.Conversation
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if _, err := visibleInbox(ctx, q, p, b.InboxId); err != nil {
			return err
		}
		if _, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: b.ContactId}); store.IsNotFound(err) {
			return errContactGone
		} else if err != nil {
			return err
		}
		if b.ChannelId != nil {
			ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: p.workspaceID, ID: *b.ChannelId})
			if store.IsNotFound(err) || (err == nil && ch.InboxID != b.InboxId) {
				return errValidation("channel_id must be a channel of the inbox")
			}
			if err != nil {
				return err
			}
		}
		if b.AssigneeId != nil {
			ok, err := memberHasInbox(ctx, q, p.workspaceID, b.InboxId, *b.AssigneeId)
			if err != nil {
				return err
			}
			if !ok {
				return errAssigneeAccess
			}
		}
		var labels []uuid.UUID
		if b.Labels != nil {
			if labels, err = labelSet(ctx, q, p.workspaceID, *b.Labels); err != nil {
				return err
			}
		}
		now := s.now()
		c, err := q.CreateConversation(ctx, store.CreateConversationParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: b.InboxId, ContactID: b.ContactId, ChannelID: b.ChannelId,
			Subject: subject, Priority: string(priority), AssigneeID: b.AssigneeId, Now: now,
		})
		if err != nil {
			return err
		}
		for _, l := range labels {
			if err := q.AddConversationLabel(ctx, store.AddConversationLabelParams{WorkspaceID: p.workspaceID, ConversationID: c.ID, LabelID: l}); err != nil {
				return err
			}
		}
		out = conversationBody(c, labels)
		events.conversation(realtime.ConversationCreated, c, out)
		before := c
		before.AssigneeID = nil
		if err := s.recordChanges(ctx, q, events, p, before, c, labels, nil, now); err != nil {
			return err
		}
		return s.addUsage(ctx, q, p.workspaceID, 1, 0, 0)
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateConversation201JSONResponse(out), nil
}

func (s *Server) GetConversation(ctx context.Context, req oas.GetConversationRequestObject) (oas.GetConversationResponseObject, error) {
	p := principalFrom(ctx)
	c, err := visibleConversation(ctx, s.st.Queries, p, req.ConversationId, false)
	if err != nil {
		return nil, err
	}
	out, err := oneConversation(ctx, s.st.Queries, c)
	if err != nil {
		return nil, err
	}
	return oas.GetConversation200JSONResponse(out), nil
}

func (s *Server) UpdateConversation(ctx context.Context, req oas.UpdateConversationRequestObject) (oas.UpdateConversationResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	var out oas.Conversation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := visibleConversation(ctx, q, p, req.ConversationId, true)
		if err != nil {
			return err
		}
		now := s.now()
		next := cur
		if b.Subject != nil {
			if next.Subject, err = trimmed(*b.Subject, 0, 500, "subject"); err != nil {
				return err
			}
		}
		if b.Priority != nil {
			if !validPriority(*b.Priority) {
				return errValidation("priority must be low, normal, high or urgent")
			}
			next.Priority = string(*b.Priority)
		}
		if b.Status != nil {
			if !b.Status.Valid() {
				return errValidation("status must be open, pending, snoozed or closed")
			}
			next.Status = string(*b.Status)
		}
		if next.Status == string(oas.Snoozed) {
			if b.SnoozeUntil != nil {
				until := *b.SnoozeUntil
				next.SnoozeUntil = &until
			}
			if next.SnoozeUntil == nil || !next.SnoozeUntil.After(now) {
				return errValidation("snoozed needs a snooze_until in the future")
			}
		} else {
			if b.SnoozeUntil != nil {
				return errValidation("snooze_until is only allowed with status snoozed")
			}
			next.SnoozeUntil = nil
		}
		if b.AssigneeId.IsSpecified() {
			next.AssigneeID = nil
			if !b.AssigneeId.IsNull() {
				id := b.AssigneeId.MustGet()
				ok, err := memberHasInbox(ctx, q, p.workspaceID, cur.InboxID, id)
				if err != nil {
					return err
				}
				if !ok {
					return errAssigneeAccess
				}
				next.AssigneeID = &id
			}
		}
		labelMap, err := conversationLabels(ctx, q, p.workspaceID, []uuid.UUID{cur.ID})
		if err != nil {
			return err
		}
		labels := labelMap[cur.ID]
		var added, removed []uuid.UUID
		if b.Labels != nil {
			want, err := labelSet(ctx, q, p.workspaceID, *b.Labels)
			if err != nil {
				return err
			}
			for _, l := range want {
				if !slices.Contains(labels, l) {
					added = append(added, l)
				}
			}
			for _, l := range labels {
				if !slices.Contains(want, l) {
					removed = append(removed, l)
				}
			}
			for _, l := range added {
				if err := q.AddConversationLabel(ctx, store.AddConversationLabelParams{WorkspaceID: p.workspaceID, ConversationID: cur.ID, LabelID: l}); err != nil {
					return err
				}
			}
			for _, l := range removed {
				if err := q.RemoveConversationLabel(ctx, store.RemoveConversationLabelParams{WorkspaceID: p.workspaceID, ConversationID: cur.ID, LabelID: l}); err != nil {
					return err
				}
			}
			labels = want
		}
		updated, err := q.UpdateConversation(ctx, store.UpdateConversationParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Subject: next.Subject, Status: next.Status, SnoozeUntil: next.SnoozeUntil,
			Priority: next.Priority, AssigneeID: next.AssigneeID, Now: now,
		})
		if err != nil {
			return err
		}
		out = conversationBody(updated, labels)
		if conversationChanged(cur, updated) || len(added) > 0 || len(removed) > 0 {
			events.conversation(realtime.ConversationUpdated, updated, out)
		}
		return s.recordChanges(ctx, q, events, p, cur, updated, added, removed, now)
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateConversation200JSONResponse(out), nil
}

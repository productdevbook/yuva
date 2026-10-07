package api

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	assigneeMe         = "me"
	assigneeUnassigned = "unassigned"
	maxLabels          = 50
	previewRunes       = 140
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

// splitSearch separates the negated terms of a websearch query, so that a conversation is
// excluded when any of its parts contains one, not only when a single message does.
func splitSearch(q *string) (pos, neg *string) {
	if q == nil {
		return nil, nil
	}
	var keep, drop []string
	rs := []rune(*q)
	for i := 0; i < len(rs); {
		if unicode.IsSpace(rs[i]) {
			i++
			continue
		}
		negated := rs[i] == '-'
		if negated {
			i++
		}
		var tok string
		if i < len(rs) && rs[i] == '"' {
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			if inner := strings.TrimSpace(string(rs[i+1 : j])); inner != "" {
				tok = `"` + inner + `"`
			}
			i = j + 1
		} else {
			j := i
			for j < len(rs) && !unicode.IsSpace(rs[j]) && rs[j] != '"' {
				j++
			}
			tok = string(rs[i:j])
			i = j
		}
		switch {
		case tok == "":
		case negated:
			drop = append(drop, tok)
		default:
			keep = append(keep, tok)
		}
	}
	if !slices.ContainsFunc(keep, func(t string) bool { return !strings.EqualFold(t, "or") }) {
		keep = nil
	}
	if len(keep) > 0 {
		v := strings.Join(keep, " ")
		pos = &v
	}
	if len(drop) > 0 {
		v := strings.Join(drop, " or ")
		neg = &v
	}
	return pos, neg
}

func previewText(body string) string {
	text := strings.Join(strings.Fields(body), " ")
	if r := []rune(text); len(r) > previewRunes {
		text = strings.TrimSpace(string(r[:previewRunes])) + "…"
	}
	return text
}

func (s *Server) listItems(ctx context.Context, p principal, rows []store.Conversation) ([]oas.ConversationListItem, error) {
	ids := make([]uuid.UUID, len(rows))
	contactIDs := make([]uuid.UUID, 0, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		if !slices.Contains(contactIDs, r.ContactID) {
			contactIDs = append(contactIDs, r.ContactID)
		}
	}
	labels, err := conversationLabels(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	contactRows, err := s.st.ListContactSummaries(ctx, store.ListContactSummariesParams{WorkspaceID: p.workspaceID, Ids: contactIDs})
	if err != nil {
		return nil, err
	}
	contacts := make(map[uuid.UUID]oas.ConversationContact, len(contactRows))
	for _, c := range contactRows {
		cc := oas.ConversationContact{Id: c.ID, Name: c.Name}
		if c.Email != "" {
			e := oas.Email(c.Email)
			cc.Email = &e
		}
		contacts[c.ID] = cc
	}
	previewRows, err := s.st.ListConversationPreviews(ctx, store.ListConversationPreviewsParams{WorkspaceID: p.workspaceID, ConversationIds: ids})
	if err != nil {
		return nil, err
	}
	previews := make(map[uuid.UUID]*oas.MessagePreview, len(previewRows))
	for _, m := range previewRows {
		previews[m.ConversationID] = &oas.MessagePreview{
			Id: m.ID, Kind: oas.MessageKind(m.Kind), AuthorType: oas.AuthorType(m.AuthorType), Text: previewText(m.Body), CreatedAt: m.CreatedAt,
		}
	}
	unread := map[uuid.UUID]bool{}
	if !p.isKey() {
		ur, err := s.st.ListUnreadConversations(ctx, store.ListUnreadConversationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationIds: ids})
		if err != nil {
			return nil, err
		}
		for _, id := range ur {
			unread[id] = true
		}
	}
	out := make([]oas.ConversationListItem, len(rows))
	for i, r := range rows {
		c := conversationBody(r, labels[r.ID])
		out[i] = oas.ConversationListItem{
			Id: c.Id, InboxId: c.InboxId, ContactId: c.ContactId, ChannelId: c.ChannelId, Kind: c.Kind, Feedback: c.Feedback, Subject: c.Subject,
			Status: c.Status, SnoozeUntil: c.SnoozeUntil, Priority: c.Priority, Spam: c.Spam, AssigneeId: c.AssigneeId, Labels: c.Labels,
			LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
			Contact: contacts[r.ContactID], LastMessage: previews[r.ID], Unread: unread[r.ID],
		}
	}
	return out, nil
}

func conversationBody(c store.Conversation, labels []uuid.UUID) oas.Conversation {
	if labels == nil {
		labels = []uuid.UUID{}
	}
	return oas.Conversation{
		Id: c.ID, InboxId: c.InboxID, ContactId: c.ContactID, ChannelId: c.ChannelID, Subject: c.Subject,
		Kind: oas.ConversationKind(c.Kind), Feedback: conversationFeedback(c), Status: oas.ConversationStatus(c.Status), SnoozeUntil: c.SnoozeUntil, Priority: oas.Priority(c.Priority), Spam: c.Spam,
		AssigneeId: c.AssigneeID, Labels: labels, LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, RelatedConversationId: c.RelatedConversationID,
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
	return a.Subject != b.Subject || a.Status != b.Status || a.Priority != b.Priority || a.Spam != b.Spam ||
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
	pos, neg := splitSearch(q)
	arg := store.ListConversationsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), MemberID: p.memberID,
		InboxID: prm.InboxId, ContactID: prm.ContactId, LabelID: prm.LabelId, Q: pos, QNot: neg,
		CursorAt: at, CursorID: cid, Lim: lim + 1, Spam: prm.Spam != nil && *prm.Spam,
	}
	if prm.Kind != nil {
		if !prm.Kind.Valid() {
			return nil, errValidation("kind must be conversation or feedback")
		}
		k := string(*prm.Kind)
		arg.Kind = &k
	}
	if prm.Category != nil {
		if !prm.Category.Valid() {
			return nil, errValidation("category must be bug, idea, praise or other")
		}
		k, c := string(oas.ConversationKindFeedback), string(*prm.Category)
		if arg.Kind != nil && *arg.Kind != k {
			return nil, errValidation("category is only for kind=feedback")
		}
		arg.Kind, arg.Category = &k, &c
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return nil, err
		}
	}
	if prm.ContactId != nil {
		found, err := s.st.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: p.workspaceID, ID: *prm.ContactId})
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errContactGone
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
	items, err := s.listItems(ctx, p, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListConversations200JSONResponse{Items: items, NextCursor: next}, nil
}

func (s *Server) GetConversationCounts(ctx context.Context, _ oas.GetConversationCountsRequestObject) (oas.GetConversationCountsResponseObject, error) {
	p := principalFrom(ctx)
	var member *uuid.UUID
	if !p.isKey() {
		member = &p.memberID
	}
	rows, err := s.st.CountOpenConversations(ctx, store.CountOpenConversationsParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), MemberID: member})
	if err != nil {
		return nil, err
	}
	byLabel, err := s.st.CountOpenConversationsByLabel(ctx, store.CountOpenConversationsByLabelParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), MemberID: member})
	if err != nil {
		return nil, err
	}
	feedback, err := s.st.CountOpenFeedback(ctx, store.CountOpenFeedbackParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), MemberID: member})
	if err != nil {
		return nil, err
	}
	out := oas.GetConversationCounts200JSONResponse{
		Inboxes: []oas.CountByID{}, Labels: make([]oas.CountByID, 0, len(byLabel)), FeedbackCategories: make([]oas.FeedbackCount, 0, len(feedback)),
	}
	for _, r := range feedback {
		out.Feedback += r.N
		out.FeedbackCategories = append(out.FeedbackCategories, oas.FeedbackCount{Category: oas.FeedbackCategory(r.Category), Count: r.N})
	}
	perInbox := map[uuid.UUID]int64{}
	for _, r := range rows {
		if r.Spam {
			out.Spam += r.N
			continue
		}
		out.All += r.N
		if r.Mine {
			out.Mine += r.N
		}
		if r.Unassigned {
			out.Unassigned += r.N
		}
		perInbox[r.InboxID] += r.N
	}
	for id, n := range perInbox {
		out.Inboxes = append(out.Inboxes, oas.CountByID{Id: id, Count: n})
	}
	slices.SortFunc(out.Inboxes, func(a, b oas.CountByID) int { return a.Id.Compare(b.Id) })
	for _, r := range byLabel {
		out.Labels = append(out.Labels, oas.CountByID{Id: r.LabelID, Count: r.N})
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
		if b.Spam != nil {
			next.Spam = *b.Spam
		}
		if b.Status != nil {
			if !b.Status.Valid() {
				return errValidation("status must be open, pending, snoozed or closed")
			}
			next.Status = string(*b.Status)
		}
		if next.Status == string(oas.ConversationStatusSnoozed) {
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
			Priority: next.Priority, AssigneeID: next.AssigneeID, Spam: next.Spam, Now: now,
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

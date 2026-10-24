package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"uuid"

	"github.com/oapi-codegen/nullable"
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

var (
	errAssigneeAccess = errValidation("assignee_id must be a member with access to the inbox")
	errNotFeedback    = problem(http.StatusConflict, "not_feedback", "feedback_progress is only for feedback conversations")
)

func searchQuery(q *string) (*string, error) {
	if q == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*q)
	if v == "" {
		return nil, nil
	}
	if len([]rune(v)) > 200 {
		e := errValidation("q must be at most 200 characters")
		e.Field = "q"
		return nil, e
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
	return excerpt(body, previewRunes)
}

func excerpt(body string, n int) string {
	text := strings.Join(strings.Fields(body), " ")
	if r := []rune(text); len(r) > n {
		text = strings.TrimSpace(string(r[:n])) + "…"
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
			Id: m.ID, Kind: oas.MessageKind(m.Kind), AuthorType: oas.AuthorType(m.AuthorType), Text: previewText(m.Body), Redacted: trueOrNil(m.Redacted), CreatedAt: m.CreatedAt,
		}
	}
	unread := map[uuid.UUID]bool{}
	pinned := map[uuid.UUID]*time.Time{}
	if !p.isKey() {
		pins, err := s.st.ListPinnedConversations(ctx, store.ListPinnedConversationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationIds: ids})
		if err != nil {
			return nil, err
		}
		for _, r := range pins {
			pinned[r.ConversationID] = &r.PinnedAt
		}
		ur, err := s.st.ListUnreadConversations(ctx, store.ListUnreadConversationsParams{WorkspaceID: p.workspaceID, MemberID: p.memberID, ConversationIds: ids})
		if err != nil {
			return nil, err
		}
		for _, id := range ur {
			unread[id] = true
		}
	}
	attRows, err := s.st.ListConversationsWithAttachments(ctx, store.ListConversationsWithAttachmentsParams{WorkspaceID: p.workspaceID, ConversationIds: ids, IncludeNotes: !p.isKey()})
	if err != nil {
		return nil, err
	}
	withAtts := make(map[uuid.UUID]bool, len(attRows))
	for _, id := range attRows {
		withAtts[id] = true
	}
	bodies := make([]oas.Conversation, len(rows))
	refs := make([]*oas.Conversation, len(rows))
	for i, r := range rows {
		bodies[i] = conversationFields(r, labels[r.ID])
		refs[i] = &bodies[i]
	}
	if err := fillPurposes(ctx, s.st.Queries, p.workspaceID, refs...); err != nil {
		return nil, err
	}
	starred, err := memberStarred(ctx, s.st.Queries, p, ids)
	if err != nil {
		return nil, err
	}
	if err := fillImportSources(ctx, s.st.Queries, p.workspaceID, refs...); err != nil {
		return nil, err
	}
	if err := fillIdeas(ctx, s.st.Queries, p.workspaceID, refs...); err != nil {
		return nil, err
	}
	if err := fillIssues(ctx, s.st.Queries, p.workspaceID, refs...); err != nil {
		return nil, err
	}
	if err := s.fillSLA(ctx, s.st.Queries, p.workspaceID, refs...); err != nil {
		return nil, err
	}
	out := make([]oas.ConversationListItem, len(rows))
	for i, r := range rows {
		c := bodies[i]
		out[i] = oas.ConversationListItem{
			Id: c.Id, InboxId: c.InboxId, InboxPurpose: c.InboxPurpose, ContactId: c.ContactId, ChannelId: c.ChannelId, ChannelKind: c.ChannelKind, Kind: c.Kind, Feedback: c.Feedback, Page: c.Page, Subject: c.Subject,
			Status: c.Status, SnoozeUntil: c.SnoozeUntil, Priority: c.Priority, Spam: c.Spam, AssigneeId: c.AssigneeId, TeamId: c.TeamId, AssignWaitingSince: c.AssignWaitingSince, Labels: c.Labels,
			LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Starred: starredFor(p, starred[r.ID]),
			RelatedConversationId: c.RelatedConversationId, ClosedAt: c.ClosedAt, Rating: c.Rating, LastReadByContactAt: c.LastReadByContactAt, BroadcastId: c.BroadcastId,
			Imported: c.Imported, HelpArticleId: c.HelpArticleId, Idea: c.Idea, Issues: c.Issues, Contact: contacts[r.ContactID], LastMessage: previews[r.ID], Unread: unread[r.ID], HasAttachments: withAtts[r.ID], PinnedAt: pinned[r.ID],
			TrashedAt: c.TrashedAt, TrashedBy: c.TrashedBy, DeletesAt: c.DeletesAt, Fields: c.Fields, Sla: c.Sla,
		}
	}
	return out, nil
}

func conversationFields(c store.Conversation, labels []uuid.UUID) oas.Conversation {
	if labels == nil {
		labels = []uuid.UUID{}
	}
	out := oas.Conversation{
		Id: c.ID, InboxId: c.InboxID, ContactId: c.ContactID, ChannelId: c.ChannelID, ChannelKind: (*oas.ChannelKind)(c.ChannelKind), Subject: c.Subject,
		Kind: oas.ConversationKind(c.Kind), Feedback: conversationFeedback(c), Page: conversationPage(c), Status: oas.ConversationStatus(c.Status), SnoozeUntil: c.SnoozeUntil, Priority: oas.Priority(c.Priority), Spam: c.Spam,
		AssigneeId: c.AssigneeID, TeamId: c.TeamID, Labels: labels, LastMessageAt: c.LastMessageAt, LastActivityAt: c.LastActivityAt,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, RelatedConversationId: c.RelatedConversationID, BroadcastId: c.BroadcastID,
		ClosedAt: c.ClosedAt, Rating: conversationRating(c), LastReadByContactAt: c.LastReadByContactAt, Imported: conversationImport(c),
		HelpArticleId: c.HelpArticleID, TrashedAt: c.TrashedAt, Fields: decodeFields(c.Fields), AssignWaitingSince: c.AssignWaitingSince,
	}
	if c.TrashedAt != nil {
		out.TrashedBy = &oas.ConversationTrashedBy{MemberId: c.TrashedByMemberID, ApiKeyId: c.TrashedByApiKeyID}
		out.DeletesAt = new(trashExpiry(*c.TrashedAt))
	}
	return out
}

func conversationImport(c store.Conversation) *oas.ConversationImport {
	if c.ImportedAt == nil {
		return nil
	}
	return &oas.ConversationImport{At: *c.ImportedAt, MigrationId: c.MigrationID}
}

// fillImportSources names the migration each imported conversation came from.
func fillImportSources(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, convs ...*oas.Conversation) error {
	var ids []uuid.UUID
	for _, c := range convs {
		if c.Imported != nil && c.Imported.MigrationId != nil && !slices.Contains(ids, *c.Imported.MigrationId) {
			ids = append(ids, *c.Imported.MigrationId)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.ListMigrationSources(ctx, store.ListMigrationSourcesParams{WorkspaceID: workspaceID, Ids: ids})
	if err != nil {
		return err
	}
	for _, c := range convs {
		if c.Imported == nil || c.Imported.MigrationId == nil {
			continue
		}
		for _, r := range rows {
			if r.ID == *c.Imported.MigrationId {
				c.Imported.Source = new(oas.MigrationSource(r.Source))
				c.Imported.Preset = new(oas.MigrationPresetId(r.Preset))
			}
		}
	}
	return nil
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
	out, err := conversationBody(ctx, q, c, labels[c.ID])
	if err != nil {
		return out, err
	}
	if err := fillImportSources(ctx, q, c.WorkspaceID, &out); err != nil {
		return out, err
	}
	if err := fillIdeas(ctx, q, c.WorkspaceID, &out); err != nil {
		return out, err
	}
	return out, fillIssues(ctx, q, c.WorkspaceID, &out)
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
	author, member, key := authorFor(p)
	msg, err := q.CreateMessage(ctx, store.CreateMessageParams{
		ID: newID(), WorkspaceID: p.workspaceID, ConversationID: c.ID, Kind: string(oas.MessageKindEvent),
		AuthorType: author, AuthorMemberID: member, AuthorApiKeyID: key, Event: mustJSON(ev), CreatedAt: at, Via: p.viaClient(),
		Rule: p.ruleJSON(),
	})
	if err != nil {
		return err
	}
	events.conversation(realtime.MessageCreated, c, messageBody(msg, nil))
	return nil
}

func authorFor(p principal) (string, *uuid.UUID, *uuid.UUID) {
	if p.rule != nil || p.auto {
		return string(oas.AuthorTypeSystem), nil, nil
	}
	if p.isKey() {
		id := p.keyID
		return string(oas.AuthorTypeBot), nil, &id
	}
	id := p.memberID
	return string(oas.AuthorTypeMember), &id, nil
}

func (s *Server) recordChanges(ctx context.Context, q *store.Queries, events *eventBatch, p principal, before, after store.Conversation, added, removed []uuid.UUID, at time.Time) error {
	if !sameID(before.AssigneeID, after.AssigneeID) {
		ev := oas.MessageEvent{Type: oas.Unassigned, PreviousAssigneeId: before.AssigneeID}
		if after.AssigneeID != nil {
			ev = oas.MessageEvent{Type: oas.Assigned, AssigneeId: after.AssigneeID, PreviousAssigneeId: before.AssigneeID}
		}
		if p.auto {
			ev.Auto = new(true)
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
	if !sameString(before.FeedbackProgress, after.FeedbackProgress) {
		ev := oas.MessageEvent{Type: oas.EventTypeFeedbackProgress, FeedbackProgress: nullable.NewNullNullable[oas.FeedbackProgress](),
			PreviousFeedbackProgress: (*oas.FeedbackProgress)(before.FeedbackProgress), IdeaId: ideaProgressFrom(ctx), IssueLinkId: issueProgressFrom(ctx)}
		if after.FeedbackProgress != nil {
			ev.FeedbackProgress = nullable.NewNullableWithValue(oas.FeedbackProgress(*after.FeedbackProgress))
		}
		if err := s.writeEvent(ctx, q, events, p, after, ev, at); err != nil {
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
		!sameID(a.AssigneeID, b.AssigneeID) || !sameID(a.TeamID, b.TeamID) || !sameTime(a.SnoozeUntil, b.SnoozeUntil) || !sameString(a.FeedbackProgress, b.FeedbackProgress) ||
		!sameFields(a.Fields, b.Fields)
}

func sameString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
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
	arg, err := s.conversationFilter(ctx, s.st.Queries, p, prm, fieldFilterPairs(ctx))
	if err != nil {
		return nil, err
	}
	if arg.BySla || arg.ScheduledBy != nil {
		arg.CursorDue, arg.CursorAt, arg.CursorID, err = decodeSLACursor(prm.Cursor)
	} else {
		arg.CursorAt, arg.CursorID, err = decodeCursor(prm.Cursor)
	}
	if err != nil {
		return nil, err
	}
	arg.Lim = lim + 1
	list := s.st.ListConversations
	if arg.HasOperators() {
		list = s.st.ListConversationsPlanned
	}
	rows, err := list(ctx, arg)
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		last := rows[lim-1].Conversation
		c := encodeCursor(last.LastActivityAt, last.ID)
		switch {
		case arg.ScheduledBy != nil:
			c = encodeSLACursor(rows[lim-1].NextScheduledAt, last.LastActivityAt, last.ID)
		case arg.BySla:
			c = encodeSLACursor(last.SlaDueAt, last.LastActivityAt, last.ID)
		}
		next = &c
	}
	convs := make([]store.Conversation, len(rows))
	for i, r := range rows {
		convs[i] = r.Conversation
	}
	items, err := s.listItems(ctx, p, convs)
	if err != nil {
		return nil, err
	}
	return oas.ListConversations200JSONResponse{Items: items, NextCursor: next}, nil
}

// listFilter is a parsed list query; addresses are the from:, to: and cc: operators of q that
// resolveAddresses still has to look up.
type listFilter struct {
	store.ListConversationsParams
	addresses []searchOperator
}

func (s *Server) conversationFilter(ctx context.Context, q *store.Queries, p principal, prm oas.ListConversationsParams, fields map[string][]string) (store.ListConversationsParams, error) {
	f, err := s.parseConversationFilter(ctx, q, p, prm, fields)
	if err != nil {
		return store.ListConversationsParams{}, err
	}
	if err := resolveAddresses(ctx, q, &f.ListConversationsParams, f.addresses); err != nil {
		return store.ListConversationsParams{}, err
	}
	return f.ListConversationsParams, nil
}

// parseConversationFilter reads the filters of ListConversations, everything but the page, for p. It is
// the one parser of those parameters: saved views check and count with it too. A refused value
// comes back as a listFilterError naming its parameter.
func (s *Server) parseConversationFilter(ctx context.Context, q *store.Queries, p principal, prm oas.ListConversationsParams, fields map[string][]string) (listFilter, error) {
	var none listFilter
	loc, err := searchLocation(prm.Timezone)
	if err != nil {
		return none, onParam("timezone", err)
	}
	search, err := searchQuery(prm.Q)
	if err != nil {
		return none, onParam("q", err)
	}
	var ops []searchOperator
	if search != nil {
		text, parsed, err := parseSearchOperators(*search, loc)
		if err != nil {
			return none, onParam("q", err)
		}
		ops = parsed
		search = nil
		if text != "" {
			search = &text
		}
	}
	pos, neg := splitSearch(search)
	if prm.Sort != nil && !prm.Sort.Valid() {
		return none, onParam("sort", errValidation("sort must be last_activity or sla_due"))
	}
	arg := store.ListConversationsParams{
		WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(),
		InboxID: prm.InboxId, ContactID: prm.ContactId, BroadcastID: prm.BroadcastId, LabelID: prm.LabelId, Q: pos, QNot: neg,
		Spam: prm.Spam, Trashed: prm.Trashed != nil && *prm.Trashed,
		BySla: prm.Sort != nil && *prm.Sort == oas.ListConversationsSortSlaDue, SlaNow: s.now(),
	}
	if arg.SlaStates, err = enumList(prm.SlaState, "sla_state must be on_track, due_soon, breached, met or paused"); err != nil {
		return none, onParam("sla_state", err)
	}
	addresses, err := applySearchOperators(&arg, p, ops, loc)
	if err != nil {
		return none, onParam("q", err)
	}
	if arg.Trashed {
		if err := requireTrashManager(p); err != nil {
			return none, onParam("trashed", err)
		}
	} else if arg.Spam == nil {
		arg.Spam = new(false)
	}
	if arg.Kinds, err = enumList(prm.Kind, "kind must be conversation or feedback"); err != nil {
		return none, onParam("kind", err)
	}
	if arg.ChannelKinds, err = enumList(prm.ChannelKind, "channel_kind must be chat, email, app or api"); err != nil {
		return none, onParam("channel_kind", err)
	}
	if prm.Category != nil {
		if !categoryKeyPattern.MatchString(*prm.Category) {
			return none, onParam("category", errValidation("category must be a feedback category key"))
		}
		k, c := string(oas.ConversationKindFeedback), string(*prm.Category)
		if arg.Kinds != nil && !slices.Contains(arg.Kinds, k) {
			return none, onParam("category", errValidation("category is only for kind=feedback"))
		}
		arg.Kinds, arg.Category = []string{k}, &c
	}
	if prm.Site != nil {
		site, ok := normalizeOrigin(*prm.Site)
		if !ok {
			return none, onParam("site", errValidation("site must be an origin, scheme://host[:port]"))
		}
		arg.Site = &site
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, q, p, *prm.InboxId); err != nil {
			return none, onParam("inbox_id", err)
		}
	}
	if prm.Page != nil {
		page, _, err := pageURL(*prm.Page)
		if err != nil {
			return none, onParam("page", err)
		}
		arg.Page = &page
	}
	if prm.ContactId != nil {
		found, err := q.ContactExists(ctx, store.ContactExistsParams{WorkspaceID: p.workspaceID, ID: *prm.ContactId})
		if err != nil {
			return none, err
		}
		if !found {
			return none, onParam("contact_id", errContactGone)
		}
	}
	if prm.IdeaId != nil {
		if _, err := visibleIdea(ctx, q, p, *prm.IdeaId, false); err != nil {
			return none, onParam("idea_id", err)
		}
		arg.IdeaID = prm.IdeaId
	}
	if prm.Idea != nil {
		if *prm.Idea != oas.ListConversationsParamsIdeaNone {
			return none, onParam("idea", errValidation("idea must be none"))
		}
		arg.NoIdea = true
	}
	if prm.Pinned != nil {
		if p.isKey() {
			return none, onParam("pinned", errValidation("pinned needs a member session"))
		}
		arg.Pinned, arg.PinnedBy = prm.Pinned, &p.memberID
	}
	if prm.Purpose != nil {
		if !prm.Purpose.Valid() {
			return none, onParam("purpose", errValidation("purpose must be support or mail"))
		}
		arg.Purpose = new(string(*prm.Purpose))
	}
	if prm.Starred != nil {
		if p.isKey() {
			return none, onParam("starred", errValidation("starred needs a member session"))
		}
		arg.Starred, arg.StarredBy = prm.Starred, &p.memberID
	}
	if prm.Sent != nil {
		if *prm.Sent != oas.ListConversationsSentMine {
			return none, onParam("sent", errValidation("sent must be mine"))
		}
		if p.isKey() {
			return none, onParam("sent", errValidation("sent=mine needs a member session"))
		}
		arg.SentBy = &p.memberID
	}
	if prm.Scheduled != nil {
		if *prm.Scheduled != oas.ListConversationsScheduledMine {
			return none, onParam("scheduled", errValidation("scheduled must be mine"))
		}
		if p.memberRef() == nil {
			return none, onParam("scheduled", errValidation("scheduled=mine needs a member session"))
		}
		if arg.BySla {
			return none, onParam("scheduled", errValidation("scheduled=mine lists by the next scheduled send and takes no sort=sla_due"))
		}
		arg.ScheduledBy = &p.memberID
	}
	if arg.Statuses, err = enumList(prm.Status, "status must be open, pending, snoozed or closed"); err != nil {
		return none, onParam("status", err)
	}
	if prm.Assignee != nil {
		switch a := strings.TrimSpace(*prm.Assignee); a {
		case assigneeMe:
			if p.isKey() {
				return none, onParam("assignee", errValidation("assignee=me needs a member session"))
			}
			arg.AssigneeID = &p.memberID
		case assigneeUnassigned:
			arg.Unassigned = true
		default:
			id, err := uuid.Parse(a)
			if err != nil {
				return none, onParam("assignee", errValidation("assignee must be a member id, me or unassigned"))
			}
			arg.AssigneeID = &id
		}
	}
	arg.TeamID = prm.TeamId
	if prm.Team != nil {
		if *prm.Team != oas.ListConversationsTeamMine {
			return none, onParam("team", errValidation("team must be mine"))
		}
		if p.isKey() {
			return none, onParam("team", errValidation("team=mine needs a member session"))
		}
		arg.TeamsOf = &p.memberID
	}
	if arg.Fields, err = fieldFilter(ctx, q, p.workspaceID, fields); err != nil {
		var e *apiError
		if errors.As(err, &e) && e.Field != "" {
			return none, onParam(fieldFilterPrefix+e.Field, err)
		}
		return none, err
	}
	return listFilter{arg, addresses}, nil
}

// listFilterError names the parameter of ListConversations whose value was refused; it answers as
// the error it wraps.
type listFilterError struct {
	param string
	err   error
}

func (e *listFilterError) Error() string { return e.err.Error() }
func (e *listFilterError) Unwrap() error { return e.err }

func onParam(param string, err error) error {
	var e *apiError
	if !errors.As(err, &e) {
		return err
	}
	return &listFilterError{param: param, err: err}
}

func (s *Server) GetConversationCounts(ctx context.Context, req oas.GetConversationCountsRequestObject) (oas.GetConversationCountsResponseObject, error) {
	p := principalFrom(ctx)
	kinds, err := enumList(req.Params.Kind, "kind must be conversation or feedback")
	if err != nil {
		return nil, err
	}
	var purpose *string
	if req.Params.Purpose != nil {
		if !req.Params.Purpose.Valid() {
			return nil, errValidation("purpose must be support or mail")
		}
		purpose = new(string(*req.Params.Purpose))
	}
	var member *uuid.UUID
	if !p.isKey() {
		member = &p.memberID
	}
	rows, err := s.st.CountOpenConversations(ctx, store.CountOpenConversationsParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(), Kinds: kinds, Purpose: purpose})
	if err != nil {
		return nil, err
	}
	byLabel, err := s.st.CountOpenConversationsByLabel(ctx, store.CountOpenConversationsByLabelParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(), Kinds: kinds, Purpose: purpose})
	if err != nil {
		return nil, err
	}
	feedback, err := s.st.CountOpenFeedback(ctx, store.CountOpenFeedbackParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(), Kinds: kinds, Purpose: purpose})
	if err != nil {
		return nil, err
	}
	var starred int64
	if member != nil {
		if starred, err = s.st.CountStarredConversations(ctx, store.CountStarredConversationsParams{
			WorkspaceID: p.workspaceID, MemberID: *member, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), ViewerMemberID: p.memberRef(), Kinds: kinds, Purpose: purpose,
		}); err != nil {
			return nil, err
		}
	}
	var myTeams []uuid.UUID
	if member != nil {
		if myTeams, err = s.st.ListMemberTeamIDs(ctx, store.ListMemberTeamIDsParams{WorkspaceID: p.workspaceID, MemberID: *member}); err != nil {
			return nil, err
		}
	}
	out := oas.GetConversationCounts200JSONResponse{
		Inboxes: []oas.CountByID{}, Assignees: []oas.CountByID{}, Teams: []oas.CountByID{}, Labels: make([]oas.CountByID, 0, len(byLabel)), FeedbackCategories: make([]oas.FeedbackCount, 0, len(feedback)),
		Starred: starred,
	}
	for _, r := range feedback {
		out.Feedback += r.N
		out.FeedbackCategories = append(out.FeedbackCategories, oas.FeedbackCount{Category: oas.FeedbackCategory(r.Category), Count: r.N})
	}
	perInbox, perAssignee, perTeam := map[uuid.UUID]int64{}, map[uuid.UUID]int64{}, map[uuid.UUID]int64{}
	for _, r := range rows {
		if r.Spam {
			out.Spam += r.N
			continue
		}
		out.All += r.N
		switch {
		case r.AssigneeID == nil:
			out.Unassigned += r.N
		default:
			perAssignee[*r.AssigneeID] += r.N
			if member != nil && *r.AssigneeID == *member {
				out.Mine += r.N
			}
		}
		if r.TeamID != nil {
			perTeam[*r.TeamID] += r.N
			if slices.Contains(myTeams, *r.TeamID) {
				out.MineTeams += r.N
			}
		}
		perInbox[r.InboxID] += r.N
	}
	out.Inboxes, out.Assignees, out.Teams = sortedCounts(perInbox), sortedCounts(perAssignee), sortedCounts(perTeam)
	for _, r := range byLabel {
		out.Labels = append(out.Labels, oas.CountByID{Id: r.LabelID, Count: r.N})
	}
	return out, nil
}

func sortedCounts(m map[uuid.UUID]int64) []oas.CountByID {
	out := make([]oas.CountByID, 0, len(m))
	for id, n := range m {
		out = append(out, oas.CountByID{Id: id, Count: n})
	}
	slices.SortFunc(out, func(a, b oas.CountByID) int { return a.Id.Compare(b.Id) })
	return out
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
		if err := visibleContact(ctx, q, p, b.ContactId); err != nil {
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
		if out, err = conversationBody(ctx, q, c, labels); err != nil {
			return err
		}
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
	if err := s.fillSLA(ctx, s.st.Queries, p.workspaceID, &out); err != nil {
		return nil, err
	}
	return oas.CreateConversation201JSONResponse(out), nil
}

func (s *Server) GetConversation(ctx context.Context, req oas.GetConversationRequestObject) (oas.GetConversationResponseObject, error) {
	p := principalFrom(ctx)
	c, err := readableConversation(ctx, s.st.Queries, p, req.ConversationId)
	if err != nil {
		return nil, err
	}
	out, err := oneConversation(ctx, s.st.Queries, c)
	if err != nil {
		return nil, err
	}
	if err := s.fillSLA(ctx, s.st.Queries, p.workspaceID, &out); err != nil {
		return nil, err
	}
	starred, err := memberStarred(ctx, s.st.Queries, p, []uuid.UUID{c.ID})
	if err != nil {
		return nil, err
	}
	out.Starred = starredFor(p, starred[c.ID])
	return oas.GetConversation200JSONResponse(out), nil
}

func (s *Server) UpdateConversation(ctx context.Context, req oas.UpdateConversationRequestObject) (oas.UpdateConversationResponseObject, error) {
	p := principalFrom(ctx)
	var out oas.Conversation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		out, err = s.changeConversation(ctx, q, events, p, req.ConversationId, *req.Body, nil, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.fillSLA(ctx, s.st.Queries, p.workspaceID, &out); err != nil {
		return nil, err
	}
	return oas.UpdateConversation200JSONResponse(out), nil
}

// changeConversation applies b to the conversation and records it; addLabels and removeLabels
// change the labels when b.Labels does not replace them.
func (s *Server) changeConversation(ctx context.Context, q *store.Queries, events *eventBatch, p principal, id uuid.UUID, b oas.ConversationUpdate, addLabels, removeLabels []uuid.UUID) (oas.Conversation, error) {
	var out oas.Conversation
	if b.TeamId.IsSpecified() && !b.TeamId.IsNull() {
		if err := shareLockTeam(ctx, q, p.workspaceID, b.TeamId.MustGet()); err != nil {
			return out, err
		}
	}
	cur, err := visibleConversation(ctx, q, p, id, true)
	if err != nil {
		return out, err
	}
	now := s.now()
	next := cur
	if b.Subject != nil {
		if next.Subject, err = trimmed(*b.Subject, 0, 500, "subject"); err != nil {
			return out, err
		}
	}
	if b.Priority != nil {
		if !validPriority(*b.Priority) {
			return out, errValidation("priority must be low, normal, high or urgent")
		}
		next.Priority = string(*b.Priority)
	}
	if b.Spam != nil {
		next.Spam = *b.Spam
	}
	if b.Status != nil {
		if !b.Status.Valid() {
			return out, errValidation("status must be open, pending, snoozed or closed")
		}
		next.Status = string(*b.Status)
	}
	if next.Status == string(oas.ConversationStatusSnoozed) {
		if b.SnoozeUntil != nil {
			until := *b.SnoozeUntil
			next.SnoozeUntil = &until
		}
		if next.SnoozeUntil == nil || !next.SnoozeUntil.After(now) {
			return out, errValidation("snoozed needs a snooze_until in the future")
		}
	} else {
		if b.SnoozeUntil != nil {
			return out, errValidation("snooze_until is only allowed with status snoozed")
		}
		next.SnoozeUntil = nil
	}
	if b.AssigneeId.IsSpecified() {
		next.AssigneeID = nil
		if !b.AssigneeId.IsNull() {
			id := b.AssigneeId.MustGet()
			ok, err := memberHasInbox(ctx, q, p.workspaceID, cur.InboxID, id)
			if err != nil {
				return out, err
			}
			if !ok {
				return out, errAssigneeAccess
			}
			next.AssigneeID = &id
		}
	}
	if b.TeamId.IsSpecified() {
		next.TeamID = nil
		if !b.TeamId.IsNull() {
			id := b.TeamId.MustGet()
			ok, err := q.TeamHasInbox(ctx, store.TeamHasInboxParams{WorkspaceID: p.workspaceID, TeamID: id, InboxID: cur.InboxID})
			if err != nil {
				return out, err
			}
			if !ok {
				return out, errTeamAccess
			}
			next.TeamID = &id
		}
	}
	if b.FeedbackProgress.IsSpecified() {
		if cur.Kind != string(oas.ConversationKindFeedback) && !b.FeedbackProgress.IsNull() {
			return out, errNotFeedback
		}
		next.FeedbackProgress = nil
		if !b.FeedbackProgress.IsNull() {
			v := b.FeedbackProgress.MustGet()
			if !v.Valid() {
				return out, errValidation("feedback_progress must be planned, in_progress, done, wont_do or null")
			}
			next.FeedbackProgress = new(string(v))
		}
	}
	if err := applyFieldChange(ctx, q, p.workspaceID, cur, &next, b.Fields); err != nil {
		return out, err
	}
	labelMap, err := conversationLabels(ctx, q, p.workspaceID, []uuid.UUID{cur.ID})
	if err != nil {
		return out, err
	}
	labels := labelMap[cur.ID]
	var want []uuid.UUID
	switch {
	case b.Labels != nil:
		if want, err = labelSet(ctx, q, p.workspaceID, *b.Labels); err != nil {
			return out, err
		}
	case len(addLabels) > 0 || len(removeLabels) > 0:
		for _, l := range append(slices.Clone(labels), addLabels...) {
			if !slices.Contains(removeLabels, l) && !slices.Contains(want, l) {
				want = append(want, l)
			}
		}
	default:
		want = labels
	}
	var added, removed []uuid.UUID
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
			return out, err
		}
	}
	for _, l := range removed {
		if err := q.RemoveConversationLabel(ctx, store.RemoveConversationLabelParams{WorkspaceID: p.workspaceID, ConversationID: cur.ID, LabelID: l}); err != nil {
			return out, err
		}
	}
	if !sameID(cur.TeamID, next.TeamID) {
		if _, err := q.SetConversationTeam(ctx, store.SetConversationTeamParams{WorkspaceID: p.workspaceID, ID: cur.ID, TeamID: next.TeamID, Now: now}); err != nil {
			return out, err
		}
	}
	if !sameFields(cur.Fields, next.Fields) {
		if _, err := q.SetConversationFields(ctx, store.SetConversationFieldsParams{WorkspaceID: p.workspaceID, ID: cur.ID, Fields: next.Fields, Now: now}); err != nil {
			return out, err
		}
	}
	updated, err := q.UpdateConversation(ctx, store.UpdateConversationParams{
		WorkspaceID: p.workspaceID, ID: cur.ID, Subject: next.Subject, Status: next.Status, SnoozeUntil: next.SnoozeUntil,
		Priority: next.Priority, AssigneeID: next.AssigneeID, Spam: next.Spam, FeedbackProgress: next.FeedbackProgress, Now: now,
	})
	if err != nil {
		return out, err
	}
	if cur.Status != updated.Status || !sameTime(cur.SnoozeUntil, updated.SnoozeUntil) {
		var rule *uuid.UUID
		if p.rule != nil && updated.Status == string(oas.ConversationStatusSnoozed) {
			rule = &p.rule.Id
		}
		if !sameID(cur.SnoozeRuleID, rule) {
			if err := q.SetConversationSnoozeRule(ctx, store.SetConversationSnoozeRuleParams{WorkspaceID: p.workspaceID, ID: cur.ID, RuleID: rule}); err != nil {
				return out, err
			}
		}
	}
	if out, err = conversationBody(ctx, q, updated, want); err != nil {
		return out, err
	}
	if err := fillImportSources(ctx, q, p.workspaceID, &out); err != nil {
		return out, err
	}
	if err := fillIdeas(ctx, q, p.workspaceID, &out); err != nil {
		return out, err
	}
	if err := fillIssues(ctx, q, p.workspaceID, &out); err != nil {
		return out, err
	}
	s.scheduleRatingRequest(events, cur, updated, now)
	if conversationChanged(cur, updated) || len(added) > 0 || len(removed) > 0 {
		events.conversation(realtime.ConversationUpdated, updated, out)
	}
	return out, s.recordChanges(ctx, q, events, p, cur, updated, added, removed, now)
}

func enumList[T interface {
	~string
	Valid() bool
}](vals *[]T, msg string) ([]string, error) {
	if vals == nil {
		return nil, nil
	}
	var out []string
	for _, v := range *vals {
		for part := range strings.SplitSeq(string(v), ",") {
			part = strings.TrimSpace(part)
			if !T(part).Valid() {
				return nil, errValidation(msg)
			}
			if !slices.Contains(out, part) {
				out = append(out, part)
			}
		}
	}
	return out, nil
}

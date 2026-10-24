package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

var (
	errInboxGone        = problem(http.StatusNotFound, "not_found", "no such inbox")
	errChannelGone      = problem(http.StatusNotFound, "not_found", "no such channel")
	errContactGone      = problem(http.StatusNotFound, "not_found", "no such contact")
	errConversationGone = problem(http.StatusNotFound, "not_found", "no such conversation")
	errAttachmentGone   = problem(http.StatusNotFound, "not_found", "no such attachment")
	errLabelGone        = problem(http.StatusNotFound, "not_found", "no such label")
	errCannedReplyGone  = problem(http.StatusNotFound, "not_found", "no such canned reply")
	errInvalidCursor    = errValidation("cursor is not valid")
)

func (p principal) seesAllInboxes() bool {
	if p.rule != nil || p.auto {
		return true
	}
	if p.isKey() {
		return !p.limited
	}
	return p.role == roleOwner || p.role == roleAdmin
}

func requireManagerOrKey(p principal) error {
	if p.isKey() {
		return nil
	}
	if p.role != roleOwner && p.role != roleAdmin {
		return errForbidden
	}
	return nil
}

func errPersonalMailbox(detail string) *apiError {
	return problem(http.StatusConflict, "personal_mailbox", detail)
}

// canReadPersonal is who reads a personal mailbox's content: its owner, through a session or the
// owner's own OAuth grant, and the system's own jobs.
func canReadPersonal(p principal, in store.Inbox) bool {
	if in.OwnerMemberID == nil || p.auto {
		return true
	}
	return !p.isKey() && p.rule == nil && p.memberID != uuid.Nil() && p.memberID == *in.OwnerMemberID
}

// canSeeInbox reports whether p reads the inbox's content; seeing every inbox stops short of another
// member's personal mailbox.
func canSeeInbox(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) (bool, error) {
	if p.auto {
		return true, nil
	}
	return q.ViewerReadsInbox(ctx, store.ViewerReadsInboxParams{WorkspaceID: p.workspaceID, InboxID: inboxID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
}

func visibleInbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := visibleInboxMeta(ctx, q, p, id)
	if err == nil && !canReadPersonal(p, in) {
		return in, errInboxGone
	}
	return in, err
}

// visibleInboxMeta also returns another member's personal mailbox to those who see every inbox, for
// its metadata only.
func visibleInboxMeta(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return in, errInboxGone
	}
	if err != nil {
		return in, err
	}
	if p.seesAllInboxes() {
		return in, nil
	}
	ok, err := q.ViewerHasInbox(ctx, store.ViewerHasInboxParams{WorkspaceID: p.workspaceID, InboxID: in.ID, ViewerID: p.viewerID()})
	if err != nil {
		return in, err
	}
	if !ok {
		return in, errInboxGone
	}
	return in, nil
}

// hiddenInboxes remembers, for one principal, which inboxes are personal mailboxes it does not read.
// An inbox's owner is fixed when it is created, so an answer holds while the inbox exists.
type hiddenInboxes struct {
	p    principal
	seen map[uuid.UUID]bool
}

func (h *hiddenInboxes) hidden(ctx context.Context, q *store.Queries, id uuid.UUID) (bool, error) {
	if v, ok := h.seen[id]; ok {
		return v, nil
	}
	if h.seen == nil {
		h.seen = map[uuid.UUID]bool{}
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: h.p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	h.seen[id] = !canReadPersonal(h.p, in)
	return h.seen[id], nil
}

// visibleConversation is a conversation of an inbox the caller can see that is not in the trash.
func visibleConversation(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Conversation, error) {
	c, err := visibleConversationOrTrashed(ctx, q, p, id, lock)
	if err == nil && c.TrashedAt != nil {
		return c, errConversationGone
	}
	return c, err
}

func visibleConversationOrTrashed(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Conversation, error) {
	get := q.GetConversation
	if lock {
		get = func(ctx context.Context, arg store.GetConversationParams) (store.Conversation, error) {
			return q.LockConversation(ctx, store.LockConversationParams(arg))
		}
	}
	c, err := get(ctx, store.GetConversationParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return c, errConversationGone
	}
	if err != nil {
		return c, err
	}
	if conversationHidden(p, c) {
		return c, errConversationGone
	}
	ok, err := canSeeInbox(ctx, q, p, c.InboxID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, errConversationGone
	}
	return c, nil
}

// conversationHidden matches conversation_visible_to in SQL: a conversation a member's new mail
// draft made is theirs alone until it goes.
func conversationHidden(p principal, c store.Conversation) bool {
	if c.DraftOf == nil {
		return false
	}
	m := p.memberRef()
	return m == nil || *m != *c.DraftOf
}

func memberHasInbox(ctx context.Context, q *store.Queries, workspaceID, inboxID, memberID uuid.UUID) (bool, error) {
	return q.MemberReadsInbox(ctx, store.MemberReadsInboxParams{WorkspaceID: workspaceID, InboxID: inboxID, MemberID: memberID})
}

func pageSize(limit *int32) (int32, error) {
	if limit == nil {
		return defaultPageSize, nil
	}
	if *limit < 1 || *limit > maxPageSize {
		return 0, errValidation("limit must be 1 to 100")
	}
	return *limit, nil
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(t.UnixMicro(), 10) + "." + id.String()))
}

// encodeSLACursor continues sort=sla_due: the due time ("-" when none), then the activity and id.
func encodeSLACursor(due *time.Time, t time.Time, id uuid.UUID) string {
	d := "-"
	if due != nil {
		d = strconv.FormatInt(due.UnixMicro(), 10)
	}
	return base64.RawURLEncoding.EncodeToString([]byte("s." + d + "." + strconv.FormatInt(t.UnixMicro(), 10) + "." + id.String()))
}

func decodeSLACursor(s *string) (*time.Time, *time.Time, *uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return nil, nil, nil, errInvalidCursor
	}
	parts := strings.SplitN(string(raw), ".", 4)
	if len(parts) != 4 || parts[0] != "s" {
		return nil, nil, nil, errInvalidCursor
	}
	var due *time.Time
	if parts[1] != "-" {
		micros, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, nil, nil, errInvalidCursor
		}
		due = new(time.UnixMicro(micros).UTC())
	}
	rest := base64.RawURLEncoding.EncodeToString([]byte(parts[2] + "." + parts[3]))
	at, id, err := decodeCursor(&rest)
	if err != nil {
		return nil, nil, nil, err
	}
	return due, at, id, nil
}

func decodeCursor(s *string) (*time.Time, *uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*s)
	if err != nil {
		return nil, nil, errInvalidCursor
	}
	ts, idPart, ok := strings.Cut(string(raw), ".")
	if !ok {
		return nil, nil, errInvalidCursor
	}
	micros, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return nil, nil, errInvalidCursor
	}
	id, err := uuid.Parse(idPart)
	if err != nil {
		return nil, nil, errInvalidCursor
	}
	t := time.UnixMicro(micros).UTC()
	return &t, &id, nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func trimmed(s string, min, max int, field string) (string, error) {
	s = strings.TrimSpace(s)
	if n := len([]rune(s)); n < min || n > max {
		if min == 0 {
			return "", errValidation(field + " must be at most " + strconv.Itoa(max) + " characters")
		}
		return "", errValidation(field + " must be " + strconv.Itoa(min) + " to " + strconv.Itoa(max) + " characters")
	}
	return s, nil
}

func monthOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (s *Server) addUsage(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, conversations, messages, bytes int64) error {
	return q.AddUsage(ctx, store.AddUsageParams{
		WorkspaceID: workspaceID, Month: monthOf(s.now()),
		Conversations: conversations, Messages: messages, AttachmentBytes: bytes,
	})
}

func (s *Server) deleteObjects(ctx context.Context, keys []string) {
	for _, k := range keys {
		if err := s.objects.Delete(context.WithoutCancel(ctx), k); err != nil {
			s.log.ErrorContext(ctx, "delete object", "key", k, "error", err)
		}
	}
}

func newID() uuid.UUID { return uuid.NewV7() }

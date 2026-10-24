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

	"github.com/productdevbook/yuva/api/internal/oas"
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
	errReadDelegate     = problem(http.StatusForbidden, "forbidden", "you may read this personal mailbox, not write in it")
	errMoveFromMailbox  = problem(http.StatusForbidden, "forbidden", "only the mailbox's owner moves its conversations to another inbox")
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

// mailboxRight is what a principal may do in an inbox: a shared inbox, and the system's own jobs,
// leave it to the usual role and scope checks (rightFull).
type mailboxRight int

const (
	rightNone mailboxRight = iota
	rightRead
	rightSend
	rightFull
)

// ownsMailbox is the owner of a personal mailbox, through a session or the owner's own OAuth grant.
func ownsMailbox(p principal, in store.Inbox) bool {
	return in.OwnerMemberID != nil && !p.isKey() && p.rule == nil && p.memberID != uuid.Nil() && p.memberID == *in.OwnerMemberID
}

// rightIn is the owner's full right, a delegate's read or send, or nothing; keys, rules and other
// members' grants never reach a personal mailbox.
func rightIn(ctx context.Context, q *store.Queries, p principal, in store.Inbox) (mailboxRight, error) {
	if !in.Personal || p.auto || ownsMailbox(p, in) || (p.mailbox != uuid.Nil() && p.mailbox == in.ID) {
		return rightFull, nil
	}
	if p.isKey() || p.rule != nil || p.memberID == uuid.Nil() {
		return rightNone, nil
	}
	access, err := q.MailboxDelegateAccess(ctx, store.MailboxDelegateAccessParams{WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: p.memberID})
	if store.IsNotFound(err) {
		return rightNone, nil
	}
	if err != nil {
		return rightNone, err
	}
	if access == string(oas.DelegateSend) {
		return rightSend, nil
	}
	return rightRead, nil
}

// canReadPersonal is who reads a personal mailbox's content: its owner and delegates, through a
// session or their own OAuth grant, the system's own jobs, and the mailbox's own filters.
func canReadPersonal(ctx context.Context, q *store.Queries, p principal, in store.Inbox) (bool, error) {
	r, err := rightIn(ctx, q, p, in)
	return r != rightNone, err
}

// requireWrite refuses a delegate who only reads the personal mailbox inboxID any change in it.
func requireWrite(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) error {
	if p.auto || p.isKey() || p.rule != nil || p.memberID == uuid.Nil() {
		return nil
	}
	ok, err := q.MemberWritesInbox(ctx, store.MemberWritesInboxParams{WorkspaceID: p.workspaceID, InboxID: inboxID, MemberID: p.memberID})
	if store.IsNotFound(err) {
		return errInboxGone
	}
	if err != nil {
		return err
	}
	if !ok {
		return errReadDelegate
	}
	return nil
}

// canSeeInbox reports whether p reads the inbox's content; seeing every inbox stops short of another
// member's personal mailbox.
func canSeeInbox(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) (bool, error) {
	if p.auto || (p.mailbox != uuid.Nil() && p.mailbox == inboxID) {
		return true, nil
	}
	return q.ViewerReadsInbox(ctx, store.ViewerReadsInboxParams{WorkspaceID: p.workspaceID, InboxID: inboxID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
}

// inboxObjectsVisible is canSeeInbox for what belongs to an inbox (broadcasts, mailing lists,
// designs, analytics properties): in a personal mailbox those are its owner's alone.
func inboxObjectsVisible(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) (bool, error) {
	if p.auto {
		return true, nil
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inboxID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if in.Personal {
		return ownsMailbox(p, in), nil
	}
	return canSeeInbox(ctx, q, p, inboxID)
}

func visibleInbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := visibleInboxMeta(ctx, q, p, id)
	if err != nil {
		return in, err
	}
	ok, err := canReadPersonal(ctx, q, p, in)
	if err != nil {
		return in, err
	}
	if !ok {
		return in, errInboxGone
	}
	return in, nil
}

// ownedInbox is visibleInbox for what only a personal mailbox's owner does: its settings, channels,
// delegates, migrations, broadcasts, lists, designs and analytics properties. Its delegates get 404 like everyone else.
func ownedInbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := visibleInbox(ctx, q, p, id)
	if err == nil && in.Personal && !p.auto && !ownsMailbox(p, in) {
		return in, errInboxGone
	}
	return in, err
}

// writableInbox is visibleInbox for writing in it: a delegate who only reads gets 403.
func writableInbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := visibleInbox(ctx, q, p, id)
	if err != nil {
		return in, err
	}
	if r, err := rightIn(ctx, q, p, in); err != nil {
		return in, err
	} else if r == rightRead {
		return in, errReadDelegate
	}
	return in, nil
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

// hiddenInboxes remembers, for one principal, its right in each personal mailbox. A delegation or a
// hand-over sends inbox_access.changed to the members it concerns, whose filters start again.
type hiddenInboxes struct {
	p    principal
	seen map[uuid.UUID]mailboxRight
}

func (h *hiddenInboxes) right(ctx context.Context, q *store.Queries, id uuid.UUID) (mailboxRight, error) {
	if v, ok := h.seen[id]; ok {
		return v, nil
	}
	if h.seen == nil {
		h.seen = map[uuid.UUID]mailboxRight{}
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: h.p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return rightNone, nil
	}
	if err != nil {
		return rightNone, err
	}
	r, err := rightIn(ctx, q, h.p, in)
	if err != nil {
		return rightNone, err
	}
	h.seen[id] = r
	return r, nil
}

func (h *hiddenInboxes) hidden(ctx context.Context, q *store.Queries, id uuid.UUID) (bool, error) {
	r, err := h.right(ctx, q, id)
	return r == rightNone, err
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
	if lock {
		return c, requireWrite(ctx, q, p, c.InboxID)
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

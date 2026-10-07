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
	return p.isKey() || p.role == roleOwner || p.role == roleAdmin
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

func canSeeInbox(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) (bool, error) {
	if p.seesAllInboxes() {
		return true, nil
	}
	return q.HasInboxAccess(ctx, store.HasInboxAccessParams{WorkspaceID: p.workspaceID, InboxID: inboxID, MemberID: p.memberID})
}

func visibleInbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return in, errInboxGone
	}
	if err != nil {
		return in, err
	}
	ok, err := canSeeInbox(ctx, q, p, in.ID)
	if err != nil {
		return in, err
	}
	if !ok {
		return in, errInboxGone
	}
	return in, nil
}

func visibleConversation(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Conversation, error) {
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
	ok, err := canSeeInbox(ctx, q, p, c.InboxID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, errConversationGone
	}
	return c, nil
}

func memberHasInbox(ctx context.Context, q *store.Queries, workspaceID, inboxID, memberID uuid.UUID) (bool, error) {
	m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: workspaceID, ID: memberID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if m.Role == roleOwner || m.Role == roleAdmin {
		return true, nil
	}
	return q.HasInboxAccess(ctx, store.HasInboxAccessParams{WorkspaceID: workspaceID, InboxID: inboxID, MemberID: memberID})
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

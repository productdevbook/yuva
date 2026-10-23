package api

import (
	"context"
	"net/http"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const trashKept = 30 * 24 * time.Hour

var errTrashExpired = problem(http.StatusConflict, "trash_expired", "the conversation's 30 days in the trash are over; it is about to be deleted")

func trashExpiry(trashedAt time.Time) time.Time { return trashedAt.Add(trashKept) }

// requireTrashManager admits owners and admins, and keys and tokens with conversations:delete,
// never agents, so a mistake cannot be hidden.
func requireTrashManager(p principal) error {
	if err := requireManagerOrKey(p); err != nil {
		return err
	}
	return requireScope(p, oas.ConversationsDelete)
}

// readableConversation lets those who may see the trash read a trashed conversation.
func readableConversation(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Conversation, error) {
	if requireTrashManager(p) == nil {
		return visibleConversationOrTrashed(ctx, q, p, id, false)
	}
	return visibleConversation(ctx, q, p, id, false)
}

func (s *Server) TrashConversation(ctx context.Context, req oas.TrashConversationRequestObject) (oas.TrashConversationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireTrashManager(p); err != nil {
		return nil, err
	}
	var out oas.Conversation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		out, err = s.setTrashed(ctx, q, events, p, req.ConversationId, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.TrashConversation200JSONResponse(out), nil
}

func (s *Server) RestoreConversation(ctx context.Context, req oas.RestoreConversationRequestObject) (oas.RestoreConversationResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireTrashManager(p); err != nil {
		return nil, err
	}
	var out oas.Conversation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		out, err = s.setTrashed(ctx, q, events, p, req.ConversationId, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.RestoreConversation200JSONResponse(out), nil
}

// setTrashed moves a conversation to the trash or back; one already where it should be is
// answered unchanged, without an event or audit entry.
func (s *Server) setTrashed(ctx context.Context, q *store.Queries, events *eventBatch, p principal, id uuid.UUID, trash bool) (oas.Conversation, error) {
	cur, err := visibleConversationOrTrashed(ctx, q, p, id, true)
	if err != nil {
		return oas.Conversation{}, err
	}
	now := s.now()
	var next store.Conversation
	typ, action := realtime.ConversationTrashed, oas.AuditConversationTrashed
	switch {
	case trash && cur.TrashedAt == nil:
		arg := store.TrashConversationParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: now}
		if p.isKey() {
			arg.ApiKeyID = new(p.keyID)
		} else if p.memberID != uuid.Nil() {
			arg.MemberID = new(p.memberID)
		}
		next, err = q.TrashConversation(ctx, arg)
	case !trash && cur.TrashedAt != nil:
		if !now.Before(trashExpiry(*cur.TrashedAt)) {
			return oas.Conversation{}, errTrashExpired
		}
		typ, action = realtime.ConversationRestored, oas.AuditConversationRestored
		next, err = q.RestoreConversation(ctx, store.RestoreConversationParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: now})
	default:
		return oneConversation(ctx, q, cur)
	}
	if err != nil {
		return oas.Conversation{}, err
	}
	out, err := oneConversation(ctx, q, next)
	if err != nil {
		return out, err
	}
	events.conversation(typ, next, out)
	return out, s.audit(ctx, q, p, action, oas.AuditTargetConversation, next.ID, auditDetails{"inbox_id": next.InboxID})
}

// PurgeTrash deletes for good, in every workspace, the conversations that went to the trash more
// than 30 days ago, with their messages, attachments, raw e-mails and stored files.
func (s *Server) PurgeTrash(ctx context.Context) error {
	now := s.now()
	before := now.Add(-trashKept)
	workspaces, err := s.st.ListWorkspacesWithExpiredTrash(ctx, before)
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		for {
			n, _, err := s.deleteConversationBatch(ctx, ws, func(q *store.Queries, events *eventBatch) ([]uuid.UUID, error) {
				rows, err := q.ListExpiredTrash(ctx, store.ListExpiredTrashParams{WorkspaceID: ws, Before: before, MaxRows: retentionBatch})
				if err != nil {
					return nil, err
				}
				ids := make([]uuid.UUID, len(rows))
				for i, r := range rows {
					ids[i] = r.ID
					events.add(realtime.ConversationDeleted, new(r.InboxID), new(r.ID), oas.ConversationDeleted{Id: r.ID, InboxId: r.InboxID, ContactId: r.ContactID})
					if err := writeSystemAudit(ctx, q, ws, now, oas.AuditConversationDeleted, oas.AuditTargetConversation, r.ID, auditDetails{"inbox_id": r.InboxID}); err != nil {
						return nil, err
					}
				}
				return ids, nil
			})
			if err != nil {
				return err
			}
			if n < retentionBatch {
				break
			}
		}
	}
	return nil
}

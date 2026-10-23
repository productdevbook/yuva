package api

import (
	"context"
	"errors"
	"fmt"
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

// dbAt is the time trash windows are measured at: nil, the database's clock, except in tests.
func (s *Server) dbAt() *time.Time { return s.limits.at() }

// setTrashed moves a conversation to the trash or back; one already where it should be is
// answered unchanged, without an event or audit entry. Trashing deletes the conversation's earlier
// events but its trash and restore ones, so the event feed and realtime replays stop showing it;
// a webhook of such an event not yet fanned out is not sent.
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
		arg := store.TrashConversationParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: now, At: s.dbAt()}
		if p.isKey() {
			arg.ApiKeyID = new(p.keyID)
		} else if p.memberID != uuid.Nil() {
			arg.MemberID = new(p.memberID)
		}
		if next, err = q.TrashConversation(ctx, arg); err != nil {
			return oas.Conversation{}, err
		}
		err = q.DeleteConversationEvents(ctx, store.DeleteConversationEventsParams{WorkspaceID: p.workspaceID, ConversationIds: []uuid.UUID{cur.ID}, KeepTrash: true})
		if err == nil {
			err = s.scrubSlackConversations(ctx, q, events, p.workspaceID, slackScrubTrashed, []uuid.UUID{cur.ID})
		}
	case !trash && cur.TrashedAt != nil:
		typ, action = realtime.ConversationRestored, oas.AuditConversationRestored
		next, err = q.RestoreConversation(ctx, store.RestoreConversationParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: now, At: s.dbAt()})
		if store.IsNotFound(err) {
			return oas.Conversation{}, errTrashExpired
		}
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

// PurgeTrash deletes for good, in every workspace, the conversations that went to the trash 30
// days ago or more, with their messages, attachments, raw e-mails, stored files and events, for
// at most about budget; the next run goes on. A workspace that fails does not stop the others.
func (s *Server) PurgeTrash(ctx context.Context, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	workspaces, err := s.st.ListWorkspacesWithExpiredTrash(ctx, s.dbAt())
	if err != nil {
		return err
	}
	var errs []error
	for _, ws := range workspaces {
		if time.Now().After(deadline) {
			break
		}
		if err := s.purgeWorkspaceTrash(ctx, ws, deadline); err != nil {
			errs = append(errs, fmt.Errorf("workspace %s: %w", ws, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Server) purgeWorkspaceTrash(ctx context.Context, ws uuid.UUID, deadline time.Time) error {
	now := s.now()
	for {
		n, _, err := s.deleteConversationBatch(ctx, ws, true, func(q *store.Queries, events *eventBatch) ([]uuid.UUID, error) {
			rows, err := q.ListExpiredTrash(ctx, store.ListExpiredTrashParams{WorkspaceID: ws, At: s.dbAt(), MaxRows: retentionBatch})
			if err != nil || len(rows) == 0 {
				return nil, err
			}
			ids := make([]uuid.UUID, len(rows))
			for i, r := range rows {
				ids[i] = r.ID
			}
			for _, r := range rows {
				events.add(realtime.ConversationDeleted, new(r.InboxID), new(r.ID), oas.ConversationDeleted{Id: r.ID, InboxId: r.InboxID, ContactId: r.ContactID})
				if err := writeSystemAudit(ctx, q, ws, now, oas.AuditConversationDeleted, oas.AuditTargetConversation, r.ID, auditDetails{"inbox_id": r.InboxID}); err != nil {
					return nil, err
				}
			}
			return ids, nil
		})
		if err != nil || n < retentionBatch || time.Now().After(deadline) {
			return err
		}
	}
}

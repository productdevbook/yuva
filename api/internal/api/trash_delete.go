package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const trashEmptiesKept = 7 * 24 * time.Hour

var errNotTrashed = problem(http.StatusConflict, "not_trashed", "the conversation is not in the trash; move it there first")

func errCountChanged(n int64) *apiError {
	e := problem(http.StatusConflict, "count_changed", "the number of conversations in the trash changed; show the new count and ask again")
	e.Extra = map[string]any{"count": n}
	return e
}

// mayDeleteForever lets a personal mailbox's owner alone delete from it, and owners, admins and
// keys from every other inbox they see.
func mayDeleteForever(ctx context.Context, q *store.Queries, p principal, c store.Conversation) error {
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: c.InboxID})
	if store.IsNotFound(err) {
		return errConversationGone
	}
	if err != nil {
		return err
	}
	if in.Personal {
		if ownsMailbox(p, in) {
			return nil
		}
		return errForbidden
	}
	return requireManagerOrKey(p)
}

func conversationFailure(id uuid.UUID, ae *apiError) oas.ConversationBulkFailure {
	f := oas.ConversationBulkFailure{Id: id, Status: int32(ae.Status), Code: ae.Code}
	if ae.Detail != "" {
		f.Detail = new(ae.Detail)
	}
	if ae.Field != "" {
		f.Field = new(ae.Field)
	}
	return f
}

func (s *Server) DeleteConversations(ctx context.Context, req oas.DeleteConversationsRequestObject) (oas.DeleteConversationsResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireScope(p, oas.ConversationsDelete); err != nil {
		return nil, err
	}
	if len(req.Body.Ids) == 0 || len(req.Body.Ids) > maxBulkConversations {
		return nil, errValidation("ids must name 1 to 100 conversations")
	}
	ids := slices.Clone(req.Body.Ids)
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return a.Compare(b) })
	ids = slices.Compact(ids)
	var failed []oas.ConversationBulkFailure
	n, _, err := s.deleteConversationBatch(ctx, p.workspaceID, true, func(q *store.Queries, events *eventBatch) ([]uuid.UUID, error) {
		failed = []oas.ConversationBulkFailure{}
		var ok []uuid.UUID
		for _, id := range ids {
			c, err := visibleConversationOrTrashed(ctx, q, p, id, true)
			if err == nil && c.TrashedAt == nil {
				err = errNotTrashed
			}
			if err == nil {
				err = mayDeleteForever(ctx, q, p, c)
			}
			var ae *apiError
			switch {
			case err == nil:
				ok = append(ok, c.ID)
				events.add(realtime.ConversationDeleted, new(c.InboxID), new(c.ID), oas.ConversationDeleted{Id: c.ID, InboxId: c.InboxID, ContactId: c.ContactID})
			case errors.As(err, &ae) && ae.Status < http.StatusInternalServerError:
				failed = append(failed, conversationFailure(id, ae))
			default:
				return nil, err
			}
		}
		if len(ok) > 0 {
			if err := s.audit(ctx, q, p, oas.AuditConversationsDeleted, oas.AuditTargetConversations, p.workspaceID, auditDetails{"count": len(ok)}); err != nil {
				return nil, err
			}
		}
		return ok, nil
	})
	if err != nil {
		return nil, err
	}
	if failed == nil {
		failed = []oas.ConversationBulkFailure{}
	}
	order := map[uuid.UUID]int{}
	for i, id := range req.Body.Ids {
		if _, seen := order[id]; !seen {
			order[id] = i
		}
	}
	slices.SortFunc(failed, func(a, b oas.ConversationBulkFailure) int { return order[a.Id] - order[b.Id] })
	return oas.DeleteConversations200JSONResponse{Deleted: int32(n), Failed: failed}, nil
}

// TrashEmptyArgs deletes for good the conversations frozen in trash_empty_items when the caller
// asked, as far as the caller may still delete them; who asked is checked again on every run.
type TrashEmptyArgs struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	EmptyID     uuid.UUID  `json:"empty_id"`
	MemberID    uuid.UUID  `json:"member_id,omitzero"`
	KeyID       uuid.UUID  `json:"key_id,omitzero"`
	GrantID     uuid.UUID  `json:"grant_id,omitzero"`
	SSO         *uuid.UUID `json:"sso,omitempty"`
	Via         string     `json:"via,omitempty"`
	MCPOnly     bool       `json:"mcp_only,omitempty"`
}

func (TrashEmptyArgs) Kind() string { return "trash_empty" }

func (a TrashEmptyArgs) asked() principal {
	return principal{workspaceID: a.WorkspaceID, memberID: a.MemberID, keyID: a.KeyID, grantID: a.GrantID, via: a.Via}
}

func (a TrashEmptyArgs) caller(ctx context.Context, s *Server) (principal, bool, error) {
	ref := func(id uuid.UUID) *uuid.UUID {
		if id == uuid.Nil() {
			return nil
		}
		return new(id)
	}
	p, refused, err := s.callerPrincipal(ctx, a.WorkspaceID, ref(a.MemberID), ref(a.KeyID), ref(a.GrantID), a.SSO, a.Via, a.MCPOnly)
	if err != nil || refused != "" {
		return p, false, err
	}
	ok, _ := trashView(p)
	return p, ok, nil
}

type trashEmptyWorker struct {
	river.WorkerDefaults[TrashEmptyArgs]
	s *Server
}

func (w *trashEmptyWorker) Timeout(*river.Job[TrashEmptyArgs]) time.Duration {
	return purgeBudget + 90*time.Second
}

func (w *trashEmptyWorker) Work(ctx context.Context, job *river.Job[TrashEmptyArgs]) error {
	return w.s.EmptyTrashRun(ctx, job.Args, purgeBudget)
}

func trashScope(p principal) (owner *uuid.UUID, manager bool) {
	return p.memberRef(), requireManagerOrKey(p) == nil
}

// CleanTrashEmpties drops the frozen sets whose job was discarded or cancelled; a live job finishes
// long before trashEmptiesKept.
func (s *Server) CleanTrashEmpties(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := s.CleanWorkspaceTrashEmpties(ctx, ws); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) CleanWorkspaceTrashEmpties(ctx context.Context, ws uuid.UUID) error {
	_, err := s.st.DeleteStaleTrashEmpties(ctx, store.DeleteStaleTrashEmptiesParams{WorkspaceID: ws, Before: s.now().Add(-trashEmptiesKept)})
	return err
}

func (s *Server) EmptyTrash(ctx context.Context, req oas.EmptyTrashRequestObject) (oas.EmptyTrashResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireScope(p, oas.ConversationsDelete); err != nil {
		return nil, err
	}
	if req.Body.ConfirmCount < 0 {
		return nil, errValidation("confirm_count must be 0 or more")
	}
	owner, manager := trashScope(p)
	var count int64
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		id := newID()
		if err := q.CreateTrashEmpty(ctx, store.CreateTrashEmptyParams{WorkspaceID: p.workspaceID, ID: id, Now: s.now()}); err != nil {
			return err
		}
		n, err := q.FreezeTrashEmpty(ctx, store.FreezeTrashEmptyParams{
			EmptyID: id, WorkspaceID: p.workspaceID, ViewerMemberID: p.memberRef(), OwnerMemberID: owner, Manager: manager,
			AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
		})
		if err != nil {
			return err
		}
		if n != req.Body.ConfirmCount {
			return errCountChanged(n)
		}
		count = n
		if n == 0 {
			return q.DeleteTrashEmpty(ctx, store.DeleteTrashEmptyParams{WorkspaceID: p.workspaceID, ID: id})
		}
		events.job(TrashEmptyArgs{
			WorkspaceID: p.workspaceID, EmptyID: id, MemberID: p.memberID, KeyID: p.keyID, GrantID: p.grantID,
			SSO: p.ssoSettings, Via: p.via, MCPOnly: p.mcpOnly,
		}, nil)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.EmptyTrash202JSONResponse{Count: count}, nil
}

// EmptyTrashRun deletes batches of the frozen set until none is left or about budget is spent;
// the transaction of the last batch records trash.emptied with the count kept in trash_empties,
// or queues the next run.
func (s *Server) EmptyTrashRun(ctx context.Context, a TrashEmptyArgs, budget time.Duration) error {
	exists, err := s.st.TrashEmptyExists(ctx, store.TrashEmptyExistsParams{WorkspaceID: a.WorkspaceID, ID: a.EmptyID})
	if err != nil || !exists {
		return err
	}
	p, may, err := a.caller(ctx, s)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(budget)
	owner, manager := trashScope(p)
	for {
		more, done := false, false
		var deleted int64
		_, _, err := s.deleteConversationBatch(ctx, a.WorkspaceID, true, func(q *store.Queries, events *eventBatch) ([]uuid.UUID, error) {
			var rows []store.ListTrashEmptyBatchRow
			if may {
				var err error
				rows, err = q.ListTrashEmptyBatch(ctx, store.ListTrashEmptyBatchParams{
					WorkspaceID: a.WorkspaceID, EmptyID: a.EmptyID, ViewerMemberID: p.memberRef(), OwnerMemberID: owner, Manager: manager,
					AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), MaxRows: retentionBatch,
				})
				if err != nil {
					return nil, err
				}
			}
			ids := make([]uuid.UUID, len(rows))
			for i, r := range rows {
				ids[i] = r.ID
				events.add(realtime.ConversationDeleted, new(r.InboxID), new(r.ID), oas.ConversationDeleted{Id: r.ID, InboxId: r.InboxID, ContactId: r.ContactID})
			}
			var err error
			deleted, err = q.AddTrashEmptyDeleted(ctx, store.AddTrashEmptyDeletedParams{WorkspaceID: a.WorkspaceID, ID: a.EmptyID, N: int64(len(rows))})
			if store.IsNotFound(err) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			switch {
			case len(rows) < retentionBatch:
				done = true
				if err := q.DeleteTrashEmpty(ctx, store.DeleteTrashEmptyParams{WorkspaceID: a.WorkspaceID, ID: a.EmptyID}); err != nil {
					return nil, err
				}
				return ids, writeAudit(ctx, q, a.asked(), s.now(), oas.AuditTrashEmptied, oas.AuditTargetTrash, a.WorkspaceID, auditDetails{"count": deleted})
			case time.Now().After(deadline):
				events.job(a, nil)
			default:
				more = true
			}
			return ids, nil
		})
		if err != nil {
			return err
		}
		if done {
			s.log.InfoContext(ctx, "trash emptied", slog.String("workspace_id", a.WorkspaceID.String()), slog.Int64("conversations", deleted))
		}
		if !more {
			return nil
		}
	}
}

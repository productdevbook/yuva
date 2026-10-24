package api

import (
	"context"
	"fmt"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

// leaveChoices pairs each personal mailbox of a leaving member with what happens to it. Leaving one
// out answers 409 personal_mailbox with all of them, as a removal without a body does.
func leaveChoices(owned []uuid.UUID, body *oas.MemberRemoval, leaver uuid.UUID) ([]oas.MailboxOnLeave, error) {
	var named []oas.MailboxOnLeave
	if body != nil && body.Mailboxes != nil {
		named = *body.Mailboxes
	}
	seen := map[uuid.UUID]bool{}
	for i, c := range named {
		field := fmt.Sprintf("mailboxes[%d]", i)
		switch {
		case !slices.Contains(owned, c.InboxId):
			return nil, leaveFieldError(field+".inbox_id", "inbox_id must be one of the member's personal mailboxes")
		case seen[c.InboxId]:
			return nil, leaveFieldError(field+".inbox_id", "each personal mailbox is named once")
		case c.Action == oas.MailboxOnLeaveHandOver && c.ToMemberId == nil:
			return nil, leaveFieldError(field+".to_member_id", "hand_over needs to_member_id")
		case c.Action == oas.MailboxOnLeaveHandOver && *c.ToMemberId == leaver:
			return nil, leaveFieldError(field+".to_member_id", "to_member_id must be another member than the one leaving")
		case c.Action == oas.MailboxOnLeaveDelete && c.ToMemberId != nil:
			return nil, leaveFieldError(field+".to_member_id", "to_member_id goes with hand_over only")
		case c.Action != oas.MailboxOnLeaveDelete && c.Action != oas.MailboxOnLeaveHandOver:
			return nil, leaveFieldError(field+".action", "action must be delete or hand_over")
		}
		seen[c.InboxId] = true
	}
	if len(seen) < len(owned) {
		e := errPersonalMailbox("the member owns personal mailboxes: say for each whether it is deleted or handed over")
		e.Extra = map[string]any{"inbox_ids": owned}
		return nil, e
	}
	return named, nil
}

func leaveFieldError(field, detail string) *apiError {
	e := errValidation(detail)
	e.Field = field
	return e
}

// applyLeaveChoices deletes or hands over the leaving member's personal mailboxes inside the
// removal's transaction, and returns the status pages whose domains to forget after it.
func (s *Server) applyLeaveChoices(ctx context.Context, q *store.Queries, events *eventBatch, p principal, leaver uuid.UUID, choices []oas.MailboxOnLeave) ([]uuid.UUID, error) {
	if len(choices) == 0 {
		return nil, nil
	}
	if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
		return nil, err
	}
	var pages []uuid.UUID
	for _, c := range choices {
		in, err := q.LockInbox(ctx, store.LockInboxParams{WorkspaceID: p.workspaceID, ID: c.InboxId})
		if err != nil {
			return nil, err
		}
		if c.Action == oas.MailboxOnLeaveDelete {
			pg, err := s.deleteInboxTx(ctx, q, events, p, in)
			if err != nil {
				return nil, err
			}
			pages = append(pages, pg...)
			if err := s.audit(ctx, q, p, oas.AuditMailboxDeletedOnLeave, oas.AuditTargetMailbox, in.ID, auditDetails{"member_id": leaver, "name": in.Name}); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := s.handOverMailbox(ctx, q, events, p, in, *c.ToMemberId); err != nil {
			return nil, err
		}
	}
	return pages, nil
}

// handOverMailbox makes to the owner of the personal mailbox in and removes its delegates; the
// previous owner, when there is one, loses it with the delegates.
func (s *Server) handOverMailbox(ctx context.Context, q *store.Queries, events *eventBatch, p principal, in store.Inbox, to uuid.UUID) (store.Inbox, error) {
	if _, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: to}); store.IsNotFound(err) {
		return in, leaveFieldError("to_member_id", "to_member_id must be a member of the workspace")
	} else if err != nil {
		return in, err
	}
	delegates, err := q.ListMailboxDelegates(ctx, store.ListMailboxDelegatesParams{WorkspaceID: p.workspaceID, InboxID: in.ID})
	if err != nil {
		return in, err
	}
	members := []uuid.UUID{to}
	if in.OwnerMemberID != nil {
		members = append(members, *in.OwnerMemberID)
	}
	for _, d := range delegates {
		members = append(members, d.MemberID)
	}
	var out store.Inbox
	if _, err := changeAccess(ctx, q, events, p.workspaceID, members, func() (bool, error) {
		if _, err := q.DeleteInboxDelegates(ctx, store.DeleteInboxDelegatesParams{WorkspaceID: p.workspaceID, InboxID: in.ID}); err != nil {
			return false, err
		}
		out, err = q.SetMailboxOwner(ctx, store.SetMailboxOwnerParams{WorkspaceID: p.workspaceID, ID: in.ID, OwnerMemberID: to, Now: s.now()})
		return true, err
	}); err != nil {
		return in, err
	}
	details := auditDetails{"to_member_id": to}
	if in.OwnerMemberID != nil {
		details["from_member_id"] = *in.OwnerMemberID
	}
	if err := s.audit(ctx, q, p, oas.AuditMailboxHandedOver, oas.AuditTargetMailbox, in.ID, details); err != nil {
		return in, err
	}
	body, err := memberInbox(ctx, q, out)
	if err != nil {
		return in, err
	}
	events.add(realtime.InboxUpdated, &out.ID, nil, body)
	return out, nil
}

func (s *Server) HandOverMailbox(ctx context.Context, req oas.HandOverMailboxRequestObject) (oas.HandOverMailboxResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	var out store.Inbox
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		in, err := q.LockInbox(ctx, store.LockInboxParams{WorkspaceID: p.workspaceID, ID: req.InboxId})
		if store.IsNotFound(err) {
			return errInboxGone
		}
		if err != nil {
			return err
		}
		if !in.Personal || in.OwnerMemberID != nil {
			return errPersonalMailbox("only an orphaned personal mailbox is handed over here")
		}
		out, err = s.handOverMailbox(ctx, q, events, p, in, req.Body.ToMemberId)
		return err
	})
	if err != nil {
		return nil, err
	}
	items := []oas.Inbox{inboxBody(out)}
	if err := withImports(ctx, s.st.Queries, p.workspaceID, items); err != nil {
		return nil, err
	}
	if err := withSESTenants(ctx, s.st.Queries, p, items); err != nil {
		return nil, err
	}
	if err := withMailboxes(ctx, s.st.Queries, p, items); err != nil {
		return nil, err
	}
	return oas.HandOverMailbox200JSONResponse(items[0]), nil
}

// orphanMailboxes is what deleting an account does to the person's personal mailboxes in one
// workspace: their delegates go now, the owner link goes with the member row, and the mailboxes
// stay personal and unread until an owner or admin hands them over or deletes them.
func (s *Server) orphanMailboxes(ctx context.Context, q *store.Queries, ev *eventBatch, by principal, member uuid.UUID, ids []uuid.UUID) error {
	for _, id := range ids {
		gone, err := q.DeleteInboxDelegates(ctx, store.DeleteInboxDelegatesParams{WorkspaceID: by.workspaceID, InboxID: id})
		if err != nil {
			return err
		}
		accessChanged(ev, id, member, false)
		for _, d := range gone {
			accessChanged(ev, id, d, false)
		}
		if err := s.audit(ctx, q, by, oas.AuditMailboxOrphaned, oas.AuditTargetMailbox, id, auditDetails{"member_id": member}); err != nil {
			return err
		}
	}
	return nil
}

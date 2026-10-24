package api

import (
	"context"
	"net/http"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var errSelfDelegation = problem(http.StatusUnprocessableEntity, "self_delegation", "you own this mailbox: name another member")

// ownMailbox is a personal mailbox of the caller's own; everyone else gets 404, and a shared inbox
// 409 to those who see it.
func ownMailbox(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.Inbox, error) {
	in, err := visibleInbox(ctx, q, p, id)
	if err != nil {
		return in, err
	}
	if !in.Personal {
		return in, errPersonalMailbox("only a personal mailbox has delegates")
	}
	if !ownsMailbox(p, in) {
		return in, errInboxGone
	}
	return in, nil
}

// onBehalfOf is the owner of the personal mailbox inboxID when p writes in it as a delegate.
func onBehalfOf(ctx context.Context, q *store.Queries, p principal, inboxID uuid.UUID) (*uuid.UUID, error) {
	if p.isKey() || p.rule != nil || p.auto || p.memberID == uuid.Nil() {
		return nil, nil
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inboxID})
	if err != nil || in.OwnerMemberID == nil || *in.OwnerMemberID == p.memberID {
		return nil, err
	}
	return in.OwnerMemberID, nil
}

func (s *Server) ListMailboxDelegates(ctx context.Context, req oas.ListMailboxDelegatesRequestObject) (oas.ListMailboxDelegatesResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := ownMailbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	rows, err := s.st.ListMailboxDelegates(ctx, store.ListMailboxDelegatesParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
	if err != nil {
		return nil, err
	}
	out := oas.ListMailboxDelegates200JSONResponse{Items: make([]oas.MailboxDelegate, len(rows))}
	for i, r := range rows {
		out.Items[i] = oas.MailboxDelegate{MemberId: r.MemberID, Name: r.Name, Email: oas.Email(r.Email), Access: oas.MailboxDelegateAccess(r.Access), CreatedAt: r.CreatedAt}
	}
	return out, nil
}

func (s *Server) SetMailboxDelegate(ctx context.Context, req oas.SetMailboxDelegateRequestObject) (oas.SetMailboxDelegateResponseObject, error) {
	p := principalFrom(ctx)
	access := req.Body.Access
	if access != oas.DelegateRead && access != oas.DelegateSend {
		return nil, errValidation("access must be read or send")
	}
	in, err := ownMailbox(ctx, s.st.Queries, p, req.InboxId)
	if err != nil {
		return nil, err
	}
	if req.MemberId == *in.OwnerMemberID {
		return nil, errSelfDelegation
	}
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: req.MemberId})
	if store.IsNotFound(err) {
		return nil, errMemberGone
	}
	if err != nil {
		return nil, err
	}
	var out oas.MailboxDelegate
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		if _, err := ownMailbox(ctx, q, p, req.InboxId); err != nil {
			return err
		}
		key := store.LockMailboxDelegateParams{WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: m.ID}
		cur, err := q.LockMailboxDelegate(ctx, key)
		var row store.MailboxDelegate
		switch {
		case store.IsNotFound(err):
			if _, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{m.ID}, func() (bool, error) {
				row, err = q.InsertMailboxDelegate(ctx, store.InsertMailboxDelegateParams{
					WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: m.ID, Access: string(access), GrantedBy: &p.memberID, Now: s.now(),
				})
				return true, err
			}); err != nil {
				return err
			}
			if err := s.audit(ctx, q, p, oas.AuditMailboxDelegateGranted, oas.AuditTargetMailbox, in.ID, auditDetails{"member_id": m.ID, "access": access}); err != nil {
				return err
			}
		case err != nil:
			return err
		case cur.Access == string(access):
			row = cur
		default:
			if row, err = q.SetMailboxDelegateAccess(ctx, store.SetMailboxDelegateAccessParams{
				WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: m.ID, Access: string(access), GrantedBy: &p.memberID,
			}); err != nil {
				return err
			}
			if err := s.audit(ctx, q, p, oas.AuditMailboxDelegateGranted, oas.AuditTargetMailbox, in.ID, auditDetails{"member_id": m.ID, "access": access, "from": cur.Access}); err != nil {
				return err
			}
		}
		out = oas.MailboxDelegate{MemberId: m.ID, Name: m.Name, Email: oas.Email(m.Email), Access: oas.MailboxDelegateAccess(row.Access), CreatedAt: row.CreatedAt}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.SetMailboxDelegate200JSONResponse(out), nil
}

func (s *Server) RemoveMailboxDelegate(ctx context.Context, req oas.RemoveMailboxDelegateRequestObject) (oas.RemoveMailboxDelegateResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := ownMailbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		in, err := ownMailbox(ctx, q, p, req.InboxId)
		if err != nil {
			return err
		}
		cur, err := q.LockMailboxDelegate(ctx, store.LockMailboxDelegateParams{WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: req.MemberId})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{cur.MemberID}, func() (bool, error) {
			n, err := q.DeleteMailboxDelegate(ctx, store.DeleteMailboxDelegateParams{WorkspaceID: p.workspaceID, InboxID: in.ID, MemberID: cur.MemberID})
			return n > 0, err
		}); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMailboxDelegateRevoked, oas.AuditTargetMailbox, in.ID, auditDetails{"member_id": cur.MemberID, "access": cur.Access})
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveMailboxDelegate204Response{}, nil
}

package api

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxMemberTitle = 100

var (
	errLastOwner  = problem(http.StatusConflict, "last_owner", "a workspace needs at least one owner")
	errOwnerRole  = problem(http.StatusForbidden, "forbidden", "only an owner can grant, change or remove the owner role")
	errMemberGone = problem(http.StatusNotFound, "not_found", "no such member")
)

func memberBody(m store.GetMemberRow) oas.Member {
	return oas.Member{
		Id: m.ID, PersonId: m.PersonID, Email: oas.Email(m.Email), Name: m.Name, Title: m.Title,
		Role: oas.Role(m.Role), CreatedAt: m.CreatedAt,
	}
}

// withPresence fills in each member's availability and whether they are online.
func (s *Server) withPresence(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, items []oas.Member) error {
	ids := make([]uuid.UUID, len(items))
	for i, m := range items {
		ids[i] = m.Id
	}
	rows, err := q.ListMemberPresence(ctx, store.ListMemberPresenceParams{WorkspaceID: workspaceID, Ids: ids, FreshAfter: s.now().Add(-presenceFresh)})
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]store.ListMemberPresenceRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for i := range items {
		r := byID[items[i].Id]
		items[i].Availability, items[i].Online = oas.Availability(r.Availability), r.Online
		if items[i].Availability == "" {
			items[i].Availability = oas.Auto
		}
	}
	return nil
}

func (s *Server) GetWorkspace(ctx context.Context, _ oas.GetWorkspaceRequestObject) (oas.GetWorkspaceResponseObject, error) {
	w, err := s.st.GetWorkspace(ctx, principalFrom(ctx).workspaceID)
	if err != nil {
		return nil, err
	}
	body, err := s.workspaceBody(ctx, w)
	if err != nil {
		return nil, err
	}
	return oas.GetWorkspace200JSONResponse(body), nil
}

func (s *Server) ListMembers(ctx context.Context, _ oas.ListMembersRequestObject) (oas.ListMembersResponseObject, error) {
	rows, err := s.st.ListMembers(ctx, principalFrom(ctx).workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.ListMembers200JSONResponse{Items: make([]oas.Member, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, memberBody(store.GetMemberRow(r)))
	}
	if err := s.withPresence(ctx, s.st.Queries, principalFrom(ctx).workspaceID, out.Items); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) GetMember(ctx context.Context, req oas.GetMemberRequestObject) (oas.GetMemberResponseObject, error) {
	m, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: principalFrom(ctx).workspaceID, ID: req.MemberId})
	if store.IsNotFound(err) {
		return nil, errMemberGone
	}
	if err != nil {
		return nil, err
	}
	out := []oas.Member{memberBody(m)}
	if err := s.withPresence(ctx, s.st.Queries, m.WorkspaceID, out); err != nil {
		return nil, err
	}
	return oas.GetMember200JSONResponse(out[0]), nil
}

func (s *Server) UpdateMember(ctx context.Context, req oas.UpdateMemberRequestObject) (oas.UpdateMemberResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if b.Role == nil && b.Title == nil {
		return nil, errValidation("send role or title")
	}
	if b.Role != nil || req.MemberId != p.memberID {
		if err := requireManager(p); err != nil {
			return nil, err
		}
	}
	if b.Role != nil && !validRole(*b.Role) {
		return nil, errValidation("role must be owner, admin or agent")
	}
	var title string
	if b.Title != nil {
		title = strings.TrimSpace(*b.Title)
		if utf8.RuneCountInString(title) > maxMemberTitle {
			return nil, errValidation("title must be at most 100 characters")
		}
	}
	var out store.GetMemberRow
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		target, err := lockedMember(ctx, q, p, req.MemberId)
		if err != nil {
			return err
		}
		if b.Title != nil {
			if err := q.UpdateMemberTitle(ctx, store.UpdateMemberTitleParams{WorkspaceID: p.workspaceID, ID: target.ID, Title: title}); err != nil {
				return err
			}
			target.Title = title
		}
		out = target
		if b.Role == nil {
			return nil
		}
		role := *b.Role
		if (target.Role == roleOwner || role == oas.Owner) && p.role != roleOwner {
			return errOwnerRole
		}
		if target.Role == roleOwner && role != oas.Owner {
			if err := ensureAnotherOwner(ctx, q, p); err != nil {
				return err
			}
		}
		if err := q.UpdateMemberRole(ctx, store.UpdateMemberRoleParams{WorkspaceID: p.workspaceID, ID: target.ID, Role: string(role)}); err != nil {
			return err
		}
		if target.Role != string(role) {
			if err := s.audit(ctx, q, p, oas.AuditMemberRoleChanged, oas.AuditTargetMember, target.ID, auditDetails{"from": target.Role, "to": role}); err != nil {
				return err
			}
		}
		target.Role = string(role)
		out = target
		return nil
	})
	if err != nil {
		return nil, err
	}
	body := []oas.Member{memberBody(out)}
	if err := s.withPresence(ctx, s.st.Queries, p.workspaceID, body); err != nil {
		return nil, err
	}
	return oas.UpdateMember200JSONResponse(body[0]), nil
}

func (s *Server) RemoveMember(ctx context.Context, req oas.RemoveMemberRequestObject) (oas.RemoveMemberResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		target, err := lockedMember(ctx, q, p, req.MemberId)
		if err != nil {
			return err
		}
		if target.Role == roleOwner {
			if p.role != roleOwner {
				return errOwnerRole
			}
			if err := ensureAnotherOwner(ctx, q, p); err != nil {
				return err
			}
		}
		if err := s.deleteMemberDrafts(ctx, tx, q, p.workspaceID, target.ID); err != nil {
			return err
		}
		if err := q.DeleteMember(ctx, store.DeleteMemberParams{WorkspaceID: p.workspaceID, ID: target.ID}); err != nil {
			return err
		}
		if err := q.ClearMacroAssignee(ctx, store.ClearMacroAssigneeParams{WorkspaceID: p.workspaceID, MemberID: target.ID.String()}); err != nil {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberRemoved, oas.AuditTargetMember, target.ID, auditDetails{"role": target.Role})
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveMember204Response{}, nil
}

func lockedMember(ctx context.Context, q *store.Queries, p principal, id oas.MemberId) (store.GetMemberRow, error) {
	if _, err := q.LockWorkspace(ctx, p.workspaceID); err != nil {
		return store.GetMemberRow{}, err
	}
	m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return m, errMemberGone
	}
	return m, err
}

func ensureAnotherOwner(ctx context.Context, q *store.Queries, p principal) error {
	n, err := q.CountOwners(ctx, p.workspaceID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errLastOwner
	}
	return nil
}

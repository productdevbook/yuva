package api

import (
	"context"
	"net/http"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const inviteTTL = 7 * 24 * time.Hour

var errAlreadyMember = problem(http.StatusConflict, "already_member", "this address already belongs to a member")

func inviteBody(i store.Invite) oas.Invite {
	return oas.Invite{
		Id: i.ID, Email: oas.Email(i.Email), Role: oas.Role(i.Role), Locale: oas.Locale(i.Locale),
		InvitedBy: i.InvitedBy, CreatedAt: i.CreatedAt, ExpiresAt: i.ExpiresAt,
	}
}

func (s *Server) ListInvites(ctx context.Context, _ oas.ListInvitesRequestObject) (oas.ListInvitesResponseObject, error) {
	rows, err := s.st.ListInvites(ctx, store.ListInvitesParams{WorkspaceID: principalFrom(ctx).workspaceID, ExpiresAt: s.now()})
	if err != nil {
		return nil, err
	}
	out := oas.ListInvites200JSONResponse{Items: make([]oas.Invite, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, inviteBody(r))
	}
	return out, nil
}

func (s *Server) CreateInvite(ctx context.Context, req oas.CreateInviteRequestObject) (oas.CreateInviteResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	email, err := normalizeEmail(b.Email)
	if err != nil {
		return nil, err
	}
	if !validRole(b.Role) {
		return nil, errValidation("role must be owner, admin or agent")
	}
	if b.Role == oas.Owner && p.role != roleOwner {
		return nil, errOwnerRole
	}
	locale := oas.En
	if b.Locale != nil {
		if !validLocale(*b.Locale) {
			return nil, errValidation("locale must be en or tr")
		}
		locale = *b.Locale
	}
	member, err := s.st.MemberExistsByEmail(ctx, store.MemberExistsByEmailParams{WorkspaceID: p.workspaceID, Email: email})
	if err != nil {
		return nil, err
	}
	if member {
		return nil, errAlreadyMember
	}
	now := s.now()
	inv, err := s.st.UpsertInvite(ctx, store.UpsertInviteParams{
		ID: uuid.New(), WorkspaceID: p.workspaceID, Email: email, Role: string(b.Role), Locale: string(locale),
		InvitedBy: &p.memberID, CreatedAt: now, ExpiresAt: now.Add(inviteTTL),
	})
	if err != nil {
		return nil, err
	}
	ws, err := s.st.GetWorkspace(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	inviter, err := s.st.GetPerson(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	inviterName := inviter.Name
	if inviterName == "" {
		inviterName = inviter.Email
	}
	msg, err := mail.Render("invite", inv.Locale, map[string]any{
		"Workspace": ws.Name, "Inviter": inviterName, "Role": inv.Role, "URL": s.auth.PublicURL,
		"Days": int(inviteTTL / (24 * time.Hour)),
	})
	if err != nil {
		return nil, err
	}
	msg.To = email
	if err := s.mailer.Send(ctx, msg); err != nil {
		return nil, err
	}
	return oas.CreateInvite201JSONResponse(inviteBody(inv)), nil
}

func (s *Server) DeleteInvite(ctx context.Context, req oas.DeleteInviteRequestObject) (oas.DeleteInviteResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteInvite(ctx, store.DeleteInviteParams{WorkspaceID: p.workspaceID, ID: req.InviteId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errNotFound
	}
	return oas.DeleteInvite204Response{}, nil
}

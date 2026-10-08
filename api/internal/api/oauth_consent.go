package api

import (
	"context"
	"slices"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func (s *Server) GetOAuthRequest(ctx context.Context, req oas.GetOAuthRequestRequestObject) (oas.GetOAuthRequestResponseObject, error) {
	p := principalFrom(ctx)
	r, err := s.st.GetOAuthRequest(ctx, store.GetOAuthRequestParams{ID: req.OauthRequestId, Now: s.now()})
	if store.IsNotFound(err) {
		return nil, errOAuthRequestGone
	}
	if err != nil {
		return nil, err
	}
	memberships, err := s.st.ListMemberships(ctx, p.personID)
	if err != nil {
		return nil, err
	}
	out := oas.OAuthRequest{
		Id: r.ID, Client: oas.OAuthClient{ClientId: r.ClientKey, Name: r.ClientName, ClientUri: r.ClientUri},
		RedirectUri: r.RedirectUri, RedirectHost: redirectHost(r.RedirectUri), Resource: s.resourceKind(r.Resource),
		Workspaces: make([]oas.OAuthWorkspaceChoice, 0, len(memberships)), ExpiresAt: r.ExpiresAt,
	}
	if r.Scopes != nil {
		out.RequestedScopes = &r.Scopes
	}
	for _, m := range memberships {
		out.Workspaces = append(out.Workspaces, oas.OAuthWorkspaceChoice{
			WorkspaceId: m.WorkspaceID, Name: m.WorkspaceName, Role: oas.Role(m.Role), Scopes: offeredScopes(r.Scopes, m.Role),
		})
	}
	return oas.GetOAuthRequest200JSONResponse(out), nil
}

func (s *Server) ApproveOAuthRequest(ctx context.Context, req oas.ApproveOAuthRequestRequestObject) (oas.ApproveOAuthRequestResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if len(b.Scopes) == 0 {
		return nil, errValidation("choose at least one scope")
	}
	var scopes []string
	for _, sc := range b.Scopes {
		if !slices.Contains(scopes, string(sc)) {
			scopes = append(scopes, string(sc))
		}
	}
	now := s.now()
	var redirect string
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		r, err := q.GetOAuthRequest(ctx, store.GetOAuthRequestParams{ID: req.OauthRequestId, Now: now})
		if store.IsNotFound(err) {
			return errOAuthRequestGone
		}
		if err != nil {
			return err
		}
		m, err := q.GetMemberByPerson(ctx, store.GetMemberByPersonParams{WorkspaceID: b.WorkspaceId, PersonID: p.personID})
		if store.IsNotFound(err) {
			return errNotAMember
		}
		if err != nil {
			return err
		}
		offered := offeredScopes(r.Scopes, m.Role)
		for _, sc := range scopes {
			if !slices.Contains(offered, oas.ApiKeyScope(sc)) {
				return errValidation("the scope " + sc + " cannot be granted in this workspace")
			}
		}
		if n, err := q.DeleteOAuthRequest(ctx, r.ID); err != nil || n == 0 {
			if err == nil {
				err = errOAuthRequestGone
			}
			return err
		}
		g, err := q.UpsertOAuthGrant(ctx, store.UpsertOAuthGrantParams{
			WorkspaceID: m.WorkspaceID, ID: newID(), MemberID: m.ID, ClientID: r.ClientRef, Scopes: scopes,
			Resource: r.Resource, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		code := randomToken(32)
		if err := q.CreateOAuthCode(ctx, store.CreateOAuthCodeParams{
			WorkspaceID: g.WorkspaceID, ID: newID(), GrantID: g.ID, CodeHash: hashSecret(code), RedirectUri: r.RedirectUri,
			CodeChallenge: r.CodeChallenge, CreatedAt: now, ExpiresAt: now.Add(oauthCodeTTL),
		}); err != nil {
			return err
		}
		redirect = withQuery(r.RedirectUri, map[string]string{"code": code, "state": deref(r.State), "iss": s.auth.PublicURL})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.ApproveOAuthRequest200JSONResponse{RedirectUrl: redirect}, nil
}

func (s *Server) DenyOAuthRequest(ctx context.Context, req oas.DenyOAuthRequestRequestObject) (oas.DenyOAuthRequestResponseObject, error) {
	var redirect string
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		r, err := q.GetOAuthRequest(ctx, store.GetOAuthRequestParams{ID: req.OauthRequestId, Now: s.now()})
		if store.IsNotFound(err) {
			return errOAuthRequestGone
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteOAuthRequest(ctx, r.ID); err != nil {
			return err
		}
		redirect = withQuery(r.RedirectUri, map[string]string{
			"error": "access_denied", "error_description": "the person denied the request", "state": deref(r.State), "iss": s.auth.PublicURL,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.DenyOAuthRequest200JSONResponse{RedirectUrl: redirect}, nil
}

func (s *Server) ListOAuthGrants(ctx context.Context, _ oas.ListOAuthGrantsRequestObject) (oas.ListOAuthGrantsResponseObject, error) {
	p := principalFrom(ctx)
	arg := store.ListOAuthGrantsParams{WorkspaceID: p.workspaceID}
	if p.role == roleAgent {
		arg.MemberID = &p.memberID
	}
	rows, err := s.st.ListOAuthGrants(ctx, arg)
	if err != nil {
		return nil, err
	}
	month := monthOf(s.now())
	out := oas.ListOAuthGrants200JSONResponse{Items: make([]oas.OAuthGrant, 0, len(rows))}
	for _, g := range rows {
		item := oas.OAuthGrant{
			Id: g.ID, Client: oas.OAuthClient{ClientId: g.ClientKey, Name: g.ClientName, ClientUri: g.ClientUri},
			MemberId: g.MemberID, Scopes: make([]oas.ApiKeyScope, len(g.Scopes)), Resource: s.resourceKind(g.Resource),
			CreatedAt: g.CreatedAt, LastUsedAt: g.LastUsedAt,
		}
		for i, sc := range g.Scopes {
			item.Scopes[i] = oas.ApiKeyScope(sc)
		}
		if g.RequestMonth != nil && g.RequestMonth.Equal(month) {
			item.RequestsThisMonth = g.RequestCount
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Server) RevokeOAuthGrant(ctx context.Context, req oas.RevokeOAuthGrantRequestObject) (oas.RevokeOAuthGrantResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		g, err := q.GetOAuthGrant(ctx, store.GetOAuthGrantParams{WorkspaceID: p.workspaceID, ID: req.OauthGrantId})
		if store.IsNotFound(err) {
			return errOAuthGrantGone
		}
		if err != nil {
			return err
		}
		if p.role == roleAgent && g.MemberID != p.memberID {
			return errOAuthGrantGone
		}
		_, err = revokeGrant(ctx, q, p.workspaceID, g.ID, s.now())
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.RevokeOAuthGrant204Response{}, nil
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

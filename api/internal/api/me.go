package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func validLocale(l oas.Locale) bool { return l == oas.En || l == oas.Tr }

func validRole(r oas.Role) bool { return r == oas.Owner || r == oas.Admin || r == oas.Agent }

func buildMe(ctx context.Context, q *store.Queries, personID uuid.UUID) (oas.Me, error) {
	p, err := q.GetPerson(ctx, personID)
	if err != nil {
		return oas.Me{}, err
	}
	rows, err := q.ListMemberships(ctx, personID)
	if err != nil {
		return oas.Me{}, err
	}
	me := oas.Me{
		Person:      oas.Person{Id: p.ID, Email: oas.Email(p.Email), Name: p.Name, Locale: oas.Locale(p.Locale)},
		Memberships: make([]oas.Membership, 0, len(rows)),
	}
	for _, r := range rows {
		me.Memberships = append(me.Memberships, oas.Membership{
			MemberId:  r.MemberID,
			Role:      oas.Role(r.Role),
			Workspace: oas.Workspace{Id: r.WorkspaceID, Name: r.WorkspaceName, CreatedAt: r.WorkspaceCreatedAt},
		})
	}
	return me, nil
}

func (s *Server) GetMe(ctx context.Context, _ oas.GetMeRequestObject) (oas.GetMeResponseObject, error) {
	me, err := buildMe(ctx, s.st.Queries, principalFrom(ctx).personID)
	if err != nil {
		return nil, err
	}
	return oas.GetMe200JSONResponse(me), nil
}

func (s *Server) UpdateMe(ctx context.Context, req oas.UpdateMeRequestObject) (oas.UpdateMeResponseObject, error) {
	b := req.Body
	if b.Name == nil && b.Locale == nil {
		return nil, errValidation("send name or locale")
	}
	if b.Name != nil && len([]rune(*b.Name)) > 200 {
		return nil, errValidation("name is longer than 200 characters")
	}
	if b.Locale != nil && !validLocale(*b.Locale) {
		return nil, errValidation("locale must be en or tr")
	}
	personID := principalFrom(ctx).personID
	if _, err := s.st.UpdatePerson(ctx, store.UpdatePersonParams{ID: personID, Name: b.Name, Locale: (*string)(b.Locale)}); err != nil {
		return nil, err
	}
	me, err := buildMe(ctx, s.st.Queries, personID)
	if err != nil {
		return nil, err
	}
	return oas.UpdateMe200JSONResponse(me), nil
}

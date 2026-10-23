package api

import (
	"context"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func validLocale(l oas.Locale) bool { return l.Valid() }

func validRole(r oas.Role) bool { return r == oas.Owner || r == oas.Admin || r == oas.Agent }

func (s *Server) buildMe(ctx context.Context, q *store.Queries, personID uuid.UUID) (oas.Me, error) {
	p, err := q.GetPerson(ctx, personID)
	if err != nil {
		return oas.Me{}, err
	}
	rows, err := q.ListMemberships(ctx, personID)
	if err != nil {
		return oas.Me{}, err
	}
	me := oas.Me{
		Person: oas.Person{
			Id: p.ID, Email: oas.Email(p.Email), Name: p.Name, Locale: oas.Locale(p.Locale), Availability: oas.Availability(p.Availability),
		},
		Memberships: make([]oas.Membership, 0, len(rows)),
	}
	for _, r := range rows {
		w, err := q.GetWorkspace(ctx, r.WorkspaceID)
		if err != nil {
			return oas.Me{}, err
		}
		ws, err := s.workspaceBodyQ(ctx, q, w)
		if err != nil {
			return oas.Me{}, err
		}
		me.Memberships = append(me.Memberships, oas.Membership{MemberId: r.MemberID, Role: oas.Role(r.Role), Workspace: ws})
	}
	return me, nil
}

func (s *Server) GetMe(ctx context.Context, _ oas.GetMeRequestObject) (oas.GetMeResponseObject, error) {
	p := principalFrom(ctx)
	me, err := s.buildMe(ctx, s.st.Queries, p.personID)
	if err != nil {
		return nil, err
	}
	if p.ssoWorkspace != nil {
		me.Memberships = slices.DeleteFunc(me.Memberships, func(m oas.Membership) bool { return m.Workspace.Id != *p.ssoWorkspace })
	}
	return oas.GetMe200JSONResponse(me), nil
}

func (s *Server) UpdateMe(ctx context.Context, req oas.UpdateMeRequestObject) (oas.UpdateMeResponseObject, error) {
	if err := principalFrom(ctx).ssoScope(nil); err != nil {
		return nil, err
	}
	b := req.Body
	if b.Name == nil && b.Locale == nil && b.Availability == nil {
		return nil, errValidation("send name, locale or availability")
	}
	if b.Availability != nil && !b.Availability.Valid() {
		return nil, errValidation("availability must be auto or away")
	}
	if b.Name != nil && len([]rune(*b.Name)) > 200 {
		return nil, errValidation("name is longer than 200 characters")
	}
	if b.Locale != nil && !validLocale(*b.Locale) {
		return nil, errValidation("locale must be a supported language code")
	}
	personID := principalFrom(ctx).personID
	if _, err := s.st.UpdatePerson(ctx, store.UpdatePersonParams{ID: personID, Name: b.Name, Locale: (*string)(b.Locale)}); err != nil {
		return nil, err
	}
	if b.Availability != nil {
		if err := s.st.SetPersonAvailability(ctx, store.SetPersonAvailabilityParams{ID: personID, Availability: string(*b.Availability)}); err != nil {
			return nil, err
		}
		members, err := s.st.ListPersonMembers(ctx, personID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			s.presenceHint(ctx, m.WorkspaceID)
			s.memberPresence(ctx, m.WorkspaceID, m.ID)
			if *b.Availability == oas.Auto {
				s.wakeAssignWaiting(ctx, m.WorkspaceID)
			}
		}
	}
	me, err := s.buildMe(ctx, s.st.Queries, personID)
	if err != nil {
		return nil, err
	}
	return oas.UpdateMe200JSONResponse(me), nil
}

package api

import (
	"context"
	"errors"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type BootstrapInput struct {
	Email         string
	Workspace     string
	Name          string
	Locale        string
	AllowExisting bool
}

type BootstrapResult struct {
	WorkspaceID uuid.UUID
	MemberID    uuid.UUID
	PersonID    uuid.UUID
}

func Bootstrap(ctx context.Context, st *store.Store, in BootstrapInput) (BootstrapResult, error) {
	email, err := normalizeEmail(oas.Email(in.Email))
	if err != nil {
		return BootstrapResult{}, errors.New("--email is not a valid address")
	}
	name := strings.TrimSpace(in.Workspace)
	if name == "" || len([]rune(name)) > 200 {
		return BootstrapResult{}, errors.New("--workspace must be 1 to 200 characters")
	}
	if in.Locale == "" {
		in.Locale = string(oas.En)
	}
	if !validLocale(oas.Locale(in.Locale)) {
		return BootstrapResult{}, errors.New("--locale must be en or tr")
	}
	var res BootstrapResult
	err = st.InTx(ctx, func(q *store.Queries) error {
		if err := q.LockBootstrap(ctx); err != nil {
			return err
		}
		n, err := q.CountWorkspaces(ctx)
		if err != nil {
			return err
		}
		if n > 0 && !in.AllowExisting {
			return errors.New("a workspace already exists; pass --allow-existing to create another one")
		}
		ws, err := q.CreateWorkspace(ctx, store.CreateWorkspaceParams{ID: uuid.New(), Name: name})
		if err != nil {
			return err
		}
		person, err := q.GetPersonByEmail(ctx, email)
		if store.IsNotFound(err) {
			person, err = createPerson(ctx, q, email, strings.TrimSpace(in.Name), in.Locale)
		}
		if err != nil {
			return err
		}
		m, err := q.CreateMember(ctx, store.CreateMemberParams{ID: uuid.New(), WorkspaceID: ws.ID, PersonID: person.ID, Role: roleOwner})
		if err != nil {
			return err
		}
		res = BootstrapResult{WorkspaceID: ws.ID, MemberID: m.ID, PersonID: person.ID}
		return nil
	})
	return res, err
}

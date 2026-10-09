package api

import (
	"context"
	"net/http"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxContactNoteRunes = 10000

var (
	errContactNoteGone   = problem(http.StatusNotFound, "not_found", "no such note")
	errContactNoteAuthor = problem(http.StatusForbidden, "forbidden", "only the note's author or an owner or admin can delete it")
)

func contactNoteBody(r store.ListContactNotesRow) oas.ContactNote {
	author := oas.MessageAuthor{Type: oas.AuthorTypeMember, MemberId: r.AuthorMemberID}
	if r.AuthorApiKeyID != nil {
		author = botAuthor(r.AuthorApiKeyID, r.BotName, r.BotAvatarUrl)
	}
	return oas.ContactNote{Id: r.ID, ContactId: r.ContactID, Author: author, Body: r.Body, CreatedAt: r.CreatedAt}
}

// noteContact checks that the caller can see the contact the notes are about.
func noteContact(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) error {
	if _, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: id}); store.IsNotFound(err) {
		return errContactGone
	} else if err != nil {
		return err
	}
	return visibleContact(ctx, q, p, id)
}

func (s *Server) ListContactNotes(ctx context.Context, req oas.ListContactNotesRequestObject) (oas.ListContactNotesResponseObject, error) {
	p := principalFrom(ctx)
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	if err := noteContact(ctx, s.st.Queries, p, req.ContactId); err != nil {
		return nil, err
	}
	rows, err := s.st.ListContactNotes(ctx, store.ListContactNotesParams{
		WorkspaceID: p.workspaceID, ContactID: req.ContactId, CursorAt: at, CursorID: id, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		c := encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID)
		next = &c
	}
	out := oas.ListContactNotes200JSONResponse{Items: make([]oas.ContactNote, len(rows)), NextCursor: next}
	for i, r := range rows {
		out.Items[i] = contactNoteBody(r)
	}
	return out, nil
}

func (s *Server) CreateContactNote(ctx context.Context, req oas.CreateContactNoteRequestObject) (oas.CreateContactNoteResponseObject, error) {
	p := principalFrom(ctx)
	body := strings.TrimSpace(req.Body.Body)
	if body == "" || len([]rune(body)) > maxContactNoteRunes {
		return nil, errValidation("body must be 1 to 10000 characters")
	}
	if err := noteContact(ctx, s.st.Queries, p, req.ContactId); err != nil {
		return nil, err
	}
	_, member, key := authorFor(p)
	r, err := s.st.CreateContactNote(ctx, store.CreateContactNoteParams{
		ID: newID(), WorkspaceID: p.workspaceID, ContactID: req.ContactId, AuthorMemberID: member, AuthorApiKeyID: key,
		Body: body, Now: s.now(),
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateContactNote201JSONResponse(contactNoteBody(store.ListContactNotesRow(r))), nil
}

func (s *Server) DeleteContactNote(ctx context.Context, req oas.DeleteContactNoteRequestObject) (oas.DeleteContactNoteResponseObject, error) {
	p := principalFrom(ctx)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if err := noteContact(ctx, q, p, req.ContactId); err != nil {
			return err
		}
		n, err := q.LockContactNote(ctx, store.LockContactNoteParams{WorkspaceID: p.workspaceID, ContactID: req.ContactId, ID: req.NoteId})
		if store.IsNotFound(err) {
			return errContactNoteGone
		}
		if err != nil {
			return err
		}
		_, member, key := authorFor(p)
		own := (key != nil && sameID(n.AuthorApiKeyID, key)) || (key == nil && sameID(n.AuthorMemberID, member))
		if !own && requireManager(p) != nil {
			return errContactNoteAuthor
		}
		return q.DeleteContactNote(ctx, store.DeleteContactNoteParams{WorkspaceID: p.workspaceID, ID: n.ID})
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteContactNote204Response{}, nil
}

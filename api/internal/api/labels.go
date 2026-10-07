package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const defaultLabelColor = "#6b7280"

var (
	errLabelTaken    = problem(http.StatusConflict, "label_taken", "another label has this name")
	errShortcutTaken = problem(http.StatusConflict, "shortcut_taken", "another canned reply has this shortcut")
)

func labelBody(l store.Label) oas.Label {
	return oas.Label{Id: l.ID, Name: l.Name, Color: l.Color, CreatedAt: l.CreatedAt}
}

func labelColor(c string) (string, error) {
	if !colorPattern.MatchString(c) {
		return "", errValidation("color must be #rrggbb")
	}
	return strings.ToLower(c), nil
}

func (s *Server) ListLabels(ctx context.Context, _ oas.ListLabelsRequestObject) (oas.ListLabelsResponseObject, error) {
	rows, err := s.st.ListLabels(ctx, principalFrom(ctx).workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.ListLabels200JSONResponse{Items: make([]oas.Label, len(rows))}
	for i, r := range rows {
		out.Items[i] = labelBody(r)
	}
	return out, nil
}

func (s *Server) CreateLabel(ctx context.Context, req oas.CreateLabelRequestObject) (oas.CreateLabelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	name, err := trimmed(req.Body.Name, 1, 64, "name")
	if err != nil {
		return nil, err
	}
	color := defaultLabelColor
	if req.Body.Color != nil {
		if color, err = labelColor(*req.Body.Color); err != nil {
			return nil, err
		}
	}
	l, err := s.st.CreateLabel(ctx, store.CreateLabelParams{ID: newID(), WorkspaceID: p.workspaceID, Name: name, Color: color, Now: s.now()})
	if store.IsUniqueViolation(err) {
		return nil, errLabelTaken
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateLabel201JSONResponse(labelBody(l)), nil
}

func (s *Server) UpdateLabel(ctx context.Context, req oas.UpdateLabelRequestObject) (oas.UpdateLabelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	cur, err := s.st.GetLabel(ctx, store.GetLabelParams{WorkspaceID: p.workspaceID, ID: req.LabelId})
	if store.IsNotFound(err) {
		return nil, errLabelGone
	}
	if err != nil {
		return nil, err
	}
	if req.Body.Name != nil {
		if cur.Name, err = trimmed(*req.Body.Name, 1, 64, "name"); err != nil {
			return nil, err
		}
	}
	if req.Body.Color != nil {
		if cur.Color, err = labelColor(*req.Body.Color); err != nil {
			return nil, err
		}
	}
	l, err := s.st.UpdateLabel(ctx, store.UpdateLabelParams{WorkspaceID: p.workspaceID, ID: cur.ID, Name: cur.Name, Color: cur.Color})
	if store.IsUniqueViolation(err) {
		return nil, errLabelTaken
	}
	if store.IsNotFound(err) {
		return nil, errLabelGone
	}
	if err != nil {
		return nil, err
	}
	return oas.UpdateLabel200JSONResponse(labelBody(l)), nil
}

func (s *Server) DeleteLabel(ctx context.Context, req oas.DeleteLabelRequestObject) (oas.DeleteLabelResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteLabel(ctx, store.DeleteLabelParams{WorkspaceID: p.workspaceID, ID: req.LabelId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errLabelGone
	}
	return oas.DeleteLabel204Response{}, nil
}

func cannedBody(c store.CannedReply) oas.CannedReply {
	return oas.CannedReply{Id: c.ID, Shortcut: c.Shortcut, Title: c.Title, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

func validCanned(c *store.CannedReply) error {
	if err := validSlug(c.Shortcut, "shortcut"); err != nil {
		return err
	}
	var err error
	if c.Title, err = trimmed(c.Title, 1, 200, "title"); err != nil {
		return err
	}
	if strings.TrimSpace(c.Body) == "" || len([]rune(c.Body)) > maxMessageBodyRunes {
		return errValidation("body must be 1 to 65536 characters")
	}
	return nil
}

func (s *Server) ListCannedReplies(ctx context.Context, _ oas.ListCannedRepliesRequestObject) (oas.ListCannedRepliesResponseObject, error) {
	rows, err := s.st.ListCannedReplies(ctx, principalFrom(ctx).workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.ListCannedReplies200JSONResponse{Items: make([]oas.CannedReply, len(rows))}
	for i, r := range rows {
		out.Items[i] = cannedBody(r)
	}
	return out, nil
}

func (s *Server) CreateCannedReply(ctx context.Context, req oas.CreateCannedReplyRequestObject) (oas.CreateCannedReplyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	c := store.CannedReply{Shortcut: req.Body.Shortcut, Title: req.Body.Title, Body: req.Body.Body}
	if err := validCanned(&c); err != nil {
		return nil, err
	}
	out, err := s.st.CreateCannedReply(ctx, store.CreateCannedReplyParams{
		ID: newID(), WorkspaceID: p.workspaceID, Shortcut: c.Shortcut, Title: c.Title, Body: c.Body, Now: s.now(),
	})
	if store.IsUniqueViolation(err) {
		return nil, errShortcutTaken
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateCannedReply201JSONResponse(cannedBody(out)), nil
}

func (s *Server) UpdateCannedReply(ctx context.Context, req oas.UpdateCannedReplyRequestObject) (oas.UpdateCannedReplyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	c, err := s.st.GetCannedReply(ctx, store.GetCannedReplyParams{WorkspaceID: p.workspaceID, ID: req.CannedReplyId})
	if store.IsNotFound(err) {
		return nil, errCannedReplyGone
	}
	if err != nil {
		return nil, err
	}
	if req.Body.Shortcut != nil {
		c.Shortcut = *req.Body.Shortcut
	}
	if req.Body.Title != nil {
		c.Title = *req.Body.Title
	}
	if req.Body.Body != nil {
		c.Body = *req.Body.Body
	}
	if err := validCanned(&c); err != nil {
		return nil, err
	}
	out, err := s.st.UpdateCannedReply(ctx, store.UpdateCannedReplyParams{
		WorkspaceID: p.workspaceID, ID: c.ID, Shortcut: c.Shortcut, Title: c.Title, Body: c.Body, Now: s.now(),
	})
	if store.IsUniqueViolation(err) {
		return nil, errShortcutTaken
	}
	if store.IsNotFound(err) {
		return nil, errCannedReplyGone
	}
	if err != nil {
		return nil, err
	}
	return oas.UpdateCannedReply200JSONResponse(cannedBody(out)), nil
}

func (s *Server) DeleteCannedReply(ctx context.Context, req oas.DeleteCannedReplyRequestObject) (oas.DeleteCannedReplyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManager(p); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteCannedReply(ctx, store.DeleteCannedReplyParams{WorkspaceID: p.workspaceID, ID: req.CannedReplyId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errCannedReplyGone
	}
	return oas.DeleteCannedReply204Response{}, nil
}

func (s *Server) GetUsage(ctx context.Context, _ oas.GetUsageRequestObject) (oas.GetUsageResponseObject, error) {
	rows, err := s.st.ListUsage(ctx, principalFrom(ctx).workspaceID)
	if err != nil {
		return nil, err
	}
	out := oas.GetUsage200JSONResponse{Items: make([]oas.UsageMonth, len(rows))}
	for i, r := range rows {
		out.Items[i] = oas.UsageMonth{
			Month: r.Month.Format("2006-01"), Conversations: r.Conversations, Messages: r.Messages, AttachmentBytes: r.AttachmentBytes,
		}
	}
	return out, nil
}

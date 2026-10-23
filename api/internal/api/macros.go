package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const maxSnoozeMinutes = 525600

var (
	errMacroGone          = problem(http.StatusNotFound, "not_found", "no such macro")
	errMacroShortcutTaken = problem(http.StatusConflict, "shortcut_taken", "another macro has this shortcut")
	errMacroNeedsMember   = problem(http.StatusForbidden, "member_session_required", "a personal macro belongs to a member; an API key manages shared macros only")
)

func macroBody(m store.Macro) oas.Macro {
	out := oas.Macro{
		Id: m.ID, Visibility: oas.MacroVisibilityShared, OwnerMemberId: m.OwnerMemberID, Name: m.Name, Shortcut: m.Shortcut,
		Body: m.Body, Actions: macroActions(m), Position: m.Position, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
	if m.OwnerMemberID != nil {
		out.Visibility = oas.MacroVisibilityPersonal
	}
	return out
}

func macroActions(m store.Macro) oas.MacroActions {
	var a oas.MacroActions
	_ = json.Unmarshal(m.Actions, &a)
	if a.AddLabels != nil && len(*a.AddLabels) == 0 {
		a.AddLabels = nil
	}
	if a.RemoveLabels != nil && len(*a.RemoveLabels) == 0 {
		a.RemoveLabels = nil
	}
	return a
}

func emptyActions(a oas.MacroActions) bool {
	return a.AddLabels == nil && a.RemoveLabels == nil && !a.AssigneeId.IsSpecified() && a.Status == nil && a.SnoozeMinutes == nil && a.Priority == nil
}

func mayWriteMacro(p principal, personal bool) error {
	if personal {
		if p.isKey() {
			return errMacroNeedsMember
		}
		if p.memberID == uuid.Nil() {
			return errForbidden
		}
		return requireScope(p, oas.ConversationsWrite)
	}
	if err := requireScope(p, oas.CannedRepliesWrite); err != nil {
		return err
	}
	return requireManagerOrKey(p)
}

func (s *Server) validMacroActions(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, a *oas.MacroActions) error {
	var add, remove []uuid.UUID
	var err error
	if a.AddLabels != nil {
		if add, err = labelSet(ctx, q, workspaceID, *a.AddLabels); err != nil {
			return err
		}
		a.AddLabels = nil
		if len(add) > 0 {
			a.AddLabels = &add
		}
	}
	if a.RemoveLabels != nil {
		if remove, err = labelSet(ctx, q, workspaceID, *a.RemoveLabels); err != nil {
			return err
		}
		a.RemoveLabels = nil
		if len(remove) > 0 {
			a.RemoveLabels = &remove
		}
	}
	if slices.ContainsFunc(add, func(l uuid.UUID) bool { return slices.Contains(remove, l) }) {
		return errValidation("a label cannot be both added and removed")
	}
	if a.AssigneeId.IsSpecified() && !a.AssigneeId.IsNull() {
		if _, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: workspaceID, ID: a.AssigneeId.MustGet()}); store.IsNotFound(err) {
			return errValidation("actions.assignee_id must be a member of the workspace")
		} else if err != nil {
			return err
		}
	}
	if a.Priority != nil && !validPriority(*a.Priority) {
		return errValidation("actions.priority must be low, normal, high or urgent")
	}
	if a.Status != nil && !a.Status.Valid() {
		return errValidation("actions.status must be open, pending, snoozed or closed")
	}
	if a.SnoozeMinutes != nil {
		if *a.SnoozeMinutes < 1 || *a.SnoozeMinutes > maxSnoozeMinutes {
			return errValidation("actions.snooze_minutes must be 1 to 525600")
		}
		if a.Status != nil && *a.Status != oas.ConversationStatusSnoozed {
			return errValidation("actions.snooze_minutes goes with status snoozed")
		}
		a.Status = new(oas.ConversationStatusSnoozed)
	} else if a.Status != nil && *a.Status == oas.ConversationStatusSnoozed {
		return errValidation("actions.status snoozed needs snooze_minutes")
	}
	return nil
}

func validMacroFields(m *store.Macro) error {
	var err error
	if m.Name, err = trimmed(m.Name, 1, 200, "name"); err != nil {
		return err
	}
	if err := validSlug(m.Shortcut, "shortcut"); err != nil {
		return err
	}
	if m.Body != nil && (strings.TrimSpace(*m.Body) == "" || len([]rune(*m.Body)) > maxMessageBodyRunes) {
		return errValidation("body must be 1 to 65536 characters")
	}
	if m.Position < 0 || m.Position > 10000 {
		return errValidation("position must be 0 to 10000")
	}
	return nil
}

func (s *Server) ListMacros(ctx context.Context, _ oas.ListMacrosRequestObject) (oas.ListMacrosResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListMacrosFor(ctx, store.ListMacrosForParams{WorkspaceID: p.workspaceID, MemberID: p.memberRef()})
	if err != nil {
		return nil, err
	}
	out := oas.ListMacros200JSONResponse{Items: make([]oas.Macro, len(rows))}
	for i, r := range rows {
		out.Items[i] = macroBody(r)
	}
	return out, nil
}

func (s *Server) GetMacro(ctx context.Context, req oas.GetMacroRequestObject) (oas.GetMacroResponseObject, error) {
	p := principalFrom(ctx)
	m, err := s.st.GetMacroFor(ctx, store.GetMacroForParams{WorkspaceID: p.workspaceID, ID: req.MacroId, MemberID: p.memberRef()})
	if store.IsNotFound(err) {
		return nil, errMacroGone
	}
	if err != nil {
		return nil, err
	}
	return oas.GetMacro200JSONResponse(macroBody(m)), nil
}

func (s *Server) CreateMacro(ctx context.Context, req oas.CreateMacroRequestObject) (oas.CreateMacroResponseObject, error) {
	p := principalFrom(ctx)
	b := req.Body
	if !b.Visibility.Valid() {
		return nil, errValidation("visibility must be personal or shared")
	}
	personal := b.Visibility == oas.MacroVisibilityPersonal
	if err := mayWriteMacro(p, personal); err != nil {
		return nil, err
	}
	m := store.Macro{Name: b.Name, Shortcut: b.Shortcut, Body: b.Body}
	if b.Position != nil {
		m.Position = *b.Position
	}
	if personal {
		m.OwnerMemberID = new(p.memberID)
	}
	if err := validMacroFields(&m); err != nil {
		return nil, err
	}
	var actions oas.MacroActions
	if b.Actions != nil {
		actions = *b.Actions
	}
	if err := s.validMacroActions(ctx, s.st.Queries, p.workspaceID, &actions); err != nil {
		return nil, err
	}
	if m.Body == nil && emptyActions(actions) {
		return nil, errValidation("a macro needs a body, actions or both")
	}
	out, err := s.st.CreateMacro(ctx, store.CreateMacroParams{
		ID: newID(), WorkspaceID: p.workspaceID, OwnerMemberID: m.OwnerMemberID, Name: m.Name, Shortcut: m.Shortcut,
		Body: m.Body, Actions: mustJSON(actions), Position: m.Position, Now: s.now(),
	})
	if store.IsUniqueViolation(err) {
		return nil, errMacroShortcutTaken
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateMacro201JSONResponse(macroBody(out)), nil
}

func (s *Server) UpdateMacro(ctx context.Context, req oas.UpdateMacroRequestObject) (oas.UpdateMacroResponseObject, error) {
	p := principalFrom(ctx)
	m, err := s.st.GetMacroFor(ctx, store.GetMacroForParams{WorkspaceID: p.workspaceID, ID: req.MacroId, MemberID: p.memberRef()})
	if store.IsNotFound(err) {
		return nil, errMacroGone
	}
	if err != nil {
		return nil, err
	}
	if err := mayWriteMacro(p, m.OwnerMemberID != nil); err != nil {
		return nil, err
	}
	b := req.Body
	if b.Name != nil {
		m.Name = *b.Name
	}
	if b.Shortcut != nil {
		m.Shortcut = *b.Shortcut
	}
	if b.Body.IsSpecified() {
		m.Body = nil
		if !b.Body.IsNull() {
			m.Body = new(b.Body.MustGet())
		}
	}
	if b.Position != nil {
		m.Position = *b.Position
	}
	if err := validMacroFields(&m); err != nil {
		return nil, err
	}
	actions := macroActions(m)
	if b.Actions != nil {
		actions = *b.Actions
	}
	if err := s.validMacroActions(ctx, s.st.Queries, p.workspaceID, &actions); err != nil {
		return nil, err
	}
	if m.Body == nil && emptyActions(actions) {
		return nil, errValidation("a macro needs a body, actions or both")
	}
	out, err := s.st.UpdateMacro(ctx, store.UpdateMacroParams{
		WorkspaceID: p.workspaceID, ID: m.ID, Name: m.Name, Shortcut: m.Shortcut, Body: m.Body, Actions: mustJSON(actions),
		Position: m.Position, Now: s.now(),
	})
	if store.IsUniqueViolation(err) {
		return nil, errMacroShortcutTaken
	}
	if store.IsNotFound(err) {
		return nil, errMacroGone
	}
	if err != nil {
		return nil, err
	}
	return oas.UpdateMacro200JSONResponse(macroBody(out)), nil
}

func (s *Server) DeleteMacro(ctx context.Context, req oas.DeleteMacroRequestObject) (oas.DeleteMacroResponseObject, error) {
	p := principalFrom(ctx)
	m, err := s.st.GetMacroFor(ctx, store.GetMacroForParams{WorkspaceID: p.workspaceID, ID: req.MacroId, MemberID: p.memberRef()})
	if store.IsNotFound(err) {
		return nil, errMacroGone
	}
	if err != nil {
		return nil, err
	}
	if err := mayWriteMacro(p, m.OwnerMemberID != nil); err != nil {
		return nil, err
	}
	n, err := s.st.DeleteMacro(ctx, store.DeleteMacroParams{WorkspaceID: p.workspaceID, ID: m.ID})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errMacroGone
	}
	return oas.DeleteMacro204Response{}, nil
}

// macroChange turns a macro's actions and then into one change: the fields of then take the
// place of the macro's.
func macroChange(m *store.Macro, then *oas.ConversationUpdate, now time.Time) (oas.ConversationUpdate, []uuid.UUID, []uuid.UUID) {
	var upd oas.ConversationUpdate
	var add, remove []uuid.UUID
	if m != nil {
		a := macroActions(*m)
		upd.Status, upd.Priority, upd.AssigneeId = a.Status, a.Priority, a.AssigneeId
		if a.SnoozeMinutes != nil {
			upd.Status = new(oas.ConversationStatusSnoozed)
			upd.SnoozeUntil = new(now.Add(time.Duration(*a.SnoozeMinutes) * time.Minute))
		}
		if a.AddLabels != nil {
			add = *a.AddLabels
		}
		if a.RemoveLabels != nil {
			remove = *a.RemoveLabels
		}
	}
	if then == nil {
		return upd, add, remove
	}
	if then.Subject != nil {
		upd.Subject = then.Subject
	}
	if then.Status != nil {
		upd.Status, upd.SnoozeUntil = then.Status, then.SnoozeUntil
	} else if then.SnoozeUntil != nil {
		upd.SnoozeUntil = then.SnoozeUntil
	}
	if then.Priority != nil {
		upd.Priority = then.Priority
	}
	if then.Spam != nil {
		upd.Spam = then.Spam
	}
	if then.AssigneeId.IsSpecified() {
		upd.AssigneeId = then.AssigneeId
	}
	if then.Labels != nil {
		upd.Labels = then.Labels
	}
	if then.FeedbackProgress.IsSpecified() {
		upd.FeedbackProgress = then.FeedbackProgress
	}
	return upd, add, remove
}

func parseThen(v string) (*oas.ConversationUpdate, error) {
	var out oas.ConversationUpdate
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, errValidation("then must be a ConversationUpdate as JSON")
	}
	return &out, nil
}

func (in *messageInput) takeMacroFields() error {
	if v, ok := in.extra["macro_id"]; ok {
		id, err := uuid.Parse(strings.TrimSpace(v))
		if err != nil {
			return errValidation("macro_id must be a macro id")
		}
		in.macroID = &id
	}
	if v, ok := in.extra["then"]; ok {
		then, err := parseThen(v)
		if err != nil {
			return err
		}
		in.then = then
	}
	return nil
}

// applyAfterMessage applies a macro and then inside the message's transaction, after the message,
// so that either refusing rolls back both.
func (s *Server) applyAfterMessage(ctx context.Context, q *store.Queries, events *eventBatch, p principal, c store.Conversation, macro *store.Macro, then *oas.ConversationUpdate, now time.Time) error {
	if macro == nil && then == nil {
		return nil
	}
	if macro != nil {
		ev := oas.MessageEvent{Type: oas.MacroApplied, Macro: &oas.MacroRef{Id: macro.ID, Name: macro.Name}}
		if err := s.writeEvent(ctx, q, events, p, c, ev, now); err != nil {
			return err
		}
	}
	upd, add, remove := macroChange(macro, then, now)
	_, err := s.changeConversation(ctx, q, events, p, c.ID, upd, add, remove)
	return err
}

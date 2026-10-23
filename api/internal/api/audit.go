package api

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const defaultWorkspaceAuditDays = 365

type auditDetails map[string]any

// writeAudit records an action in the workspace's audit log, in the caller's transaction. A
// principal with neither member nor key nor grant is the operator's command line.
func writeAudit(ctx context.Context, q *store.Queries, p principal, now time.Time, action oas.AuditAction, target oas.AuditTargetType, targetID uuid.UUID, details auditDetails) error {
	arg := store.AddAuditEntryParams{
		ID: newID(), WorkspaceID: p.workspaceID, Action: string(action), TargetType: string(target), TargetID: targetID, Now: now,
	}
	switch {
	case p.isKey():
		arg.ActorKind, arg.ActorID = string(oas.AuditActorApiKey), new(p.keyID)
	case p.isGrant():
		arg.ActorKind, arg.ActorID, arg.ActorMemberID, arg.Via = string(oas.AuditActorOAuthGrant), new(p.grantID), new(p.memberID), p.viaClient()
	case p.memberID != uuid.Nil():
		arg.ActorKind, arg.ActorID, arg.ActorMemberID = string(oas.AuditActorMember), new(p.memberID), new(p.memberID)
	default:
		arg.ActorKind = string(oas.AuditActorOperator)
	}
	if details == nil {
		details = auditDetails{}
	}
	arg.Details = mustJSON(details)
	return q.AddAuditEntry(ctx, arg)
}

func (s *Server) audit(ctx context.Context, q *store.Queries, p principal, action oas.AuditAction, target oas.AuditTargetType, targetID uuid.UUID, details auditDetails) error {
	return writeAudit(ctx, q, p, s.now(), action, target, targetID, details)
}

func equalPtr[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (s *Server) ListAuditLog(ctx context.Context, req oas.ListAuditLogRequestObject) (oas.ListAuditLogResponseObject, error) {
	p := principalFrom(ctx)
	if p.isKey() {
		if p.limited {
			return nil, errForbidden
		}
	} else if err := requireOwner(p); err != nil {
		return nil, err
	}
	lim, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	actions, err := enumList(req.Params.Action, "action must name audit actions")
	if err != nil {
		return nil, err
	}
	if len(actions) > 50 {
		return nil, errValidation("action names at most 50 actions")
	}
	arg := store.ListAuditEntriesParams{
		WorkspaceID: p.workspaceID, ActorID: req.Params.ActorId, Actions: actions, Since: req.Params.Since, Until: req.Params.Until,
		CursorAt: at, CursorID: id, Lim: lim + 1,
	}
	if arg.Actions == nil {
		arg.Actions = []string{}
	}
	if k := req.Params.ActorKind; k != nil {
		if !k.Valid() {
			return nil, errValidation("actor_kind must be member, api_key, oauth_grant or operator")
		}
		arg.ActorKind = new(string(*k))
	}
	if arg.Since != nil && arg.Until != nil && !arg.Since.Before(*arg.Until) {
		return nil, errValidation("since must be before until")
	}
	rows, err := s.st.ListAuditEntries(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := oas.ListAuditLog200JSONResponse{Items: make([]oas.AuditEntry, 0, len(rows))}
	if len(rows) > int(lim) {
		rows = rows[:lim]
		out.NextCursor = new(encodeCursor(rows[lim-1].CreatedAt, rows[lim-1].ID))
	}
	for _, r := range rows {
		e, err := auditEntryBody(r)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, e)
	}
	return out, nil
}

func auditEntryBody(r store.AuditLog) (oas.AuditEntry, error) {
	out := oas.AuditEntry{
		Id: r.ID, Action: oas.AuditAction(r.Action), CreatedAt: r.CreatedAt,
		Actor:  oas.AuditActor{Kind: oas.AuditActorKind(r.ActorKind), Id: r.ActorID, MemberId: r.ActorMemberID, Via: r.Via},
		Target: oas.AuditTarget{Type: oas.AuditTargetType(r.TargetType), Id: r.TargetID},
	}
	if r.ActorKind != string(oas.AuditActorOperator) {
		out.Actor.Name = new(r.ActorName)
	}
	if err := json.Unmarshal(r.Details, &out.Details); err != nil {
		return out, err
	}
	return out, nil
}

// CleanAudit deletes audit entries older than the audit period.
func (s *Server) CleanAudit(ctx context.Context) error {
	before := s.now().AddDate(0, 0, -s.auditDays)
	rows, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range rows {
		if _, err := s.st.DeleteOldAuditEntries(ctx, store.DeleteOldAuditEntriesParams{WorkspaceID: ws, Before: before}); err != nil {
			return err
		}
	}
	return nil
}

package api

import (
	"context"
	"net/http"
	"slices"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxSLAPolicies     = 100
	maxSLATargetMins   = 525600
	maxSLAWarnMins     = 1440
	defaultSLAWarnMins = 15
)

var (
	errSLAPolicyGone  = problem(http.StatusNotFound, "not_found", "no such SLA policy")
	errSLAPolicyLimit = problem(http.StatusForbidden, "sla_policy_limit", "a workspace holds at most 100 SLA policies")
)

func slaPolicyBody(p store.SlaPolicy, seen map[uuid.UUID]bool) oas.SlaPolicy {
	out := oas.SlaPolicy{
		Id: p.ID, Name: p.Name, Position: p.Position, Enabled: p.Enabled, InboxIds: onlySeen(p.InboxIds, seen),
		ChannelKinds: make([]oas.ChannelKind, 0, len(p.ChannelKinds)), LabelIds: p.LabelIds, Priorities: make([]oas.Priority, 0, len(p.Priorities)),
		FirstReplyMinutes: p.FirstReplyMinutes, NextReplyMinutes: p.NextReplyMinutes, ResolutionMinutes: p.ResolutionMinutes,
		WarnMinutes: p.WarnMinutes, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
	if out.InboxIds == nil {
		out.InboxIds = []uuid.UUID{}
	}
	if out.LabelIds == nil {
		out.LabelIds = []uuid.UUID{}
	}
	for _, k := range p.ChannelKinds {
		out.ChannelKinds = append(out.ChannelKinds, oas.ChannelKind(k))
	}
	for _, v := range p.Priorities {
		out.Priorities = append(out.Priorities, oas.Priority(v))
	}
	return out
}

type slaPolicyFields struct {
	name                         string
	enabled                      bool
	inboxes, labels              []uuid.UUID
	kinds, priorities            []string
	firstReply, nextReply, solve *int32
	warn                         int32
}

func validTargetMinutes(v *int32, field string) error {
	if v != nil && (*v < 1 || *v > maxSLATargetMins) {
		return errValidation(field + " must be 1 to 525600")
	}
	return nil
}

func sortedIDs(in []uuid.UUID) []uuid.UUID {
	out := append([]uuid.UUID{}, in...)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return a.Compare(b) })
	return slices.Compact(out)
}

func enumSet[T interface {
	~string
	Valid() bool
}](in []T, max int, msg string) ([]string, error) {
	out := []string{}
	for _, v := range in {
		if !v.Valid() {
			return nil, errValidation(msg)
		}
		if !slices.Contains(out, string(v)) {
			out = append(out, string(v))
		}
	}
	if len(out) > max {
		return nil, errValidation(msg)
	}
	return out, nil
}

// check validates the fields and locks the inboxes and labels they name, so a delete running at
// the same time either strips them afterwards or makes this write fail.
func (f *slaPolicyFields) check(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	for field, v := range map[string]*int32{"first_reply_minutes": f.firstReply, "next_reply_minutes": f.nextReply, "resolution_minutes": f.solve} {
		if err := validTargetMinutes(v, field); err != nil {
			return err
		}
	}
	if f.firstReply == nil && f.nextReply == nil && f.solve == nil {
		return errValidation("a policy needs first_reply_minutes, next_reply_minutes or resolution_minutes")
	}
	if f.warn < 0 || f.warn > maxSLAWarnMins {
		return errValidation("warn_minutes must be 0 to 1440")
	}
	f.inboxes, f.labels = sortedIDs(f.inboxes), sortedIDs(f.labels)
	if len(f.inboxes) > 100 {
		return errValidation("inbox_ids holds at most 100 inboxes")
	}
	if len(f.labels) > maxLabels {
		return errValidation("label_ids holds at most 50 labels")
	}
	if len(f.inboxes) > 0 {
		found, err := q.LockInboxesShared(ctx, store.LockInboxesSharedParams{WorkspaceID: ws, Ids: f.inboxes})
		if err != nil {
			return err
		}
		if len(found) != len(f.inboxes) {
			return errValidation("inbox_ids names an inbox that does not exist")
		}
	}
	if len(f.labels) > 0 {
		found, err := q.LockLabels(ctx, store.LockLabelsParams{WorkspaceID: ws, Ids: f.labels})
		if err != nil {
			return err
		}
		if len(found) != len(f.labels) {
			return errValidation("label_ids names a label that does not exist")
		}
	}
	return nil
}

func (s *Server) ListSlaPolicies(ctx context.Context, _ oas.ListSlaPoliciesRequestObject) (oas.ListSlaPoliciesResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListSLAPolicies(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	seen, err := seenInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	out := oas.ListSlaPolicies200JSONResponse{Items: make([]oas.SlaPolicy, len(rows))}
	for i, r := range rows {
		out.Items[i] = slaPolicyBody(r, seen)
	}
	return out, nil
}

func (s *Server) GetSlaPolicy(ctx context.Context, req oas.GetSlaPolicyRequestObject) (oas.GetSlaPolicyResponseObject, error) {
	p := principalFrom(ctx)
	r, err := s.st.GetSLAPolicy(ctx, store.GetSLAPolicyParams{WorkspaceID: p.workspaceID, ID: req.SlaPolicyId})
	if store.IsNotFound(err) {
		return nil, errSLAPolicyGone
	}
	if err != nil {
		return nil, err
	}
	seen, err := seenInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	return oas.GetSlaPolicy200JSONResponse(slaPolicyBody(r, seen)), nil
}

func (s *Server) CreateSlaPolicy(ctx context.Context, req oas.CreateSlaPolicyRequestObject) (oas.CreateSlaPolicyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	f := slaPolicyFields{
		name: b.Name, enabled: b.Enabled == nil || *b.Enabled, firstReply: b.FirstReplyMinutes, nextReply: b.NextReplyMinutes,
		solve: b.ResolutionMinutes, warn: defaultSLAWarnMins,
	}
	if b.WarnMinutes != nil {
		f.warn = *b.WarnMinutes
	}
	if b.InboxIds != nil {
		f.inboxes = *b.InboxIds
	}
	if b.LabelIds != nil {
		f.labels = *b.LabelIds
	}
	var err error
	if f.kinds, err = enumSet(listOf(b.ChannelKinds), 4, "channel_kinds must be chat, email, app or api"); err != nil {
		return nil, err
	}
	if f.priorities, err = enumSet(listOf(b.Priorities), 4, "priorities must be low, normal, high or urgent"); err != nil {
		return nil, err
	}
	var out store.SlaPolicy
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockSLAPolicies(ctx, p.workspaceID); err != nil {
			return err
		}
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		n, err := q.CountSLAPolicies(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		if n >= maxSLAPolicies {
			return errSLAPolicyLimit
		}
		out, err = q.CreateSLAPolicy(ctx, store.CreateSLAPolicyParams{
			ID: newID(), WorkspaceID: p.workspaceID, Name: f.name, Enabled: f.enabled, InboxIds: f.inboxes, ChannelKinds: f.kinds,
			LabelIds: f.labels, Priorities: f.priorities, FirstReplyMinutes: f.firstReply, NextReplyMinutes: f.nextReply,
			ResolutionMinutes: f.solve, WarnMinutes: f.warn, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateSlaPolicy201JSONResponse(slaPolicyBody(out, nil)), nil
}

func listOf[T any](v *[]T) []T {
	if v == nil {
		return nil
	}
	return *v
}

func (s *Server) UpdateSlaPolicy(ctx context.Context, req oas.UpdateSlaPolicyRequestObject) (oas.UpdateSlaPolicyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out store.SlaPolicy
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		cur, err := q.LockSLAPolicy(ctx, store.LockSLAPolicyParams{WorkspaceID: p.workspaceID, ID: req.SlaPolicyId})
		if store.IsNotFound(err) {
			return errSLAPolicyGone
		}
		if err != nil {
			return err
		}
		f := slaPolicyFields{
			name: cur.Name, enabled: cur.Enabled, inboxes: cur.InboxIds, labels: cur.LabelIds, kinds: cur.ChannelKinds, priorities: cur.Priorities,
			firstReply: cur.FirstReplyMinutes, nextReply: cur.NextReplyMinutes, solve: cur.ResolutionMinutes, warn: cur.WarnMinutes,
		}
		if b.Name != nil {
			f.name = *b.Name
		}
		if b.Enabled != nil {
			f.enabled = *b.Enabled
		}
		if b.InboxIds != nil {
			f.inboxes = *b.InboxIds
		}
		if b.LabelIds != nil {
			f.labels = *b.LabelIds
		}
		if b.ChannelKinds != nil {
			if f.kinds, err = enumSet(*b.ChannelKinds, 4, "channel_kinds must be chat, email, app or api"); err != nil {
				return err
			}
		}
		if b.Priorities != nil {
			if f.priorities, err = enumSet(*b.Priorities, 4, "priorities must be low, normal, high or urgent"); err != nil {
				return err
			}
		}
		f.firstReply = nullableMinutes(b.FirstReplyMinutes.IsSpecified(), b.FirstReplyMinutes.IsNull(), b.FirstReplyMinutes.MustGet, f.firstReply)
		f.nextReply = nullableMinutes(b.NextReplyMinutes.IsSpecified(), b.NextReplyMinutes.IsNull(), b.NextReplyMinutes.MustGet, f.nextReply)
		f.solve = nullableMinutes(b.ResolutionMinutes.IsSpecified(), b.ResolutionMinutes.IsNull(), b.ResolutionMinutes.MustGet, f.solve)
		if b.WarnMinutes != nil {
			f.warn = *b.WarnMinutes
		}
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		out, err = q.UpdateSLAPolicy(ctx, store.UpdateSLAPolicyParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Enabled: f.enabled, InboxIds: f.inboxes, ChannelKinds: f.kinds,
			LabelIds: f.labels, Priorities: f.priorities, FirstReplyMinutes: f.firstReply, NextReplyMinutes: f.nextReply,
			ResolutionMinutes: f.solve, WarnMinutes: f.warn, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateSlaPolicy200JSONResponse(slaPolicyBody(out, nil)), nil
}

func nullableMinutes(specified, null bool, get func() int32, cur *int32) *int32 {
	switch {
	case !specified:
		return cur
	case null:
		return nil
	}
	v := get()
	return &v
}

func (s *Server) DeleteSlaPolicy(ctx context.Context, req oas.DeleteSlaPolicyRequestObject) (oas.DeleteSlaPolicyResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockSLAPolicies(ctx, p.workspaceID); err != nil {
			return err
		}
		if _, err := q.LockSLAPolicy(ctx, store.LockSLAPolicyParams{WorkspaceID: p.workspaceID, ID: req.SlaPolicyId}); store.IsNotFound(err) {
			return errSLAPolicyGone
		} else if err != nil {
			return err
		}
		if err := q.ClearSLADueOfPolicy(ctx, store.ClearSLADueOfPolicyParams{WorkspaceID: p.workspaceID, PolicyID: req.SlaPolicyId}); err != nil {
			return err
		}
		_, err := q.DeleteSLAPolicy(ctx, store.DeleteSLAPolicyParams{WorkspaceID: p.workspaceID, ID: req.SlaPolicyId})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteSlaPolicy204Response{}, nil
}

func (s *Server) OrderSlaPolicies(ctx context.Context, req oas.OrderSlaPoliciesRequestObject) (oas.OrderSlaPoliciesResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var rows []store.SlaPolicy
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockSLAPolicies(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.ListSLAPolicies(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		ids := req.Body.Ids
		if len(sortedIDs(ids)) != len(ids) || len(ids) != len(cur) {
			return errValidation("ids must name every SLA policy of the workspace once")
		}
		for _, c := range cur {
			if !slices.Contains(ids, c.ID) {
				return errValidation("ids must name every SLA policy of the workspace once")
			}
		}
		for i, id := range ids {
			if err := q.SetSLAPolicyPosition(ctx, store.SetSLAPolicyPositionParams{WorkspaceID: p.workspaceID, ID: id, Position: int32(i)}); err != nil {
				return err
			}
		}
		rows, err = q.ListSLAPolicies(ctx, p.workspaceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := oas.OrderSlaPolicies200JSONResponse{Items: make([]oas.SlaPolicy, len(rows))}
	for i, r := range rows {
		out.Items[i] = slaPolicyBody(r, nil)
	}
	return out, nil
}

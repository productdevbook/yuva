package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	eventTeamCreated = string(oas.TeamCreatedEvent)
	eventTeamUpdated = string(oas.TeamUpdatedEvent)
	eventTeamDeleted = string(oas.TeamDeletedEvent)

	maxTeamMembers = 500
	maxTeamInboxes = 100
)

var (
	errTeamGone        = problem(http.StatusNotFound, "not_found", "no such team")
	errTeamTaken       = problem(http.StatusConflict, "team_taken", "another team has this name")
	errTeamAccess      = problem(http.StatusUnprocessableEntity, "team_access", "team_id must be a team with access to the conversation's inbox")
	errTeamMemberLimit = problem(http.StatusForbidden, "team_limit", "a team holds at most 500 members")
	errTeamInboxLimit  = problem(http.StatusForbidden, "team_limit", "a team holds at most 100 inboxes")
)

func teamBodies(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, teams []store.Team) ([]oas.Team, error) {
	ids := make([]uuid.UUID, len(teams))
	for i, t := range teams {
		ids[i] = t.ID
	}
	members, err := q.ListTeamMemberIDs(ctx, store.ListTeamMemberIDsParams{WorkspaceID: workspaceID, TeamIds: ids})
	if err != nil {
		return nil, err
	}
	inboxes, err := q.ListTeamInboxIDs(ctx, store.ListTeamInboxIDsParams{WorkspaceID: workspaceID, TeamIds: ids})
	if err != nil {
		return nil, err
	}
	out := make([]oas.Team, len(teams))
	for i, t := range teams {
		out[i] = oas.Team{
			Id: t.ID, Name: t.Name, Description: t.Description, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
			AssignmentMode: oas.AssignmentMode(t.AssignmentMode), AssignmentCap: t.AssignmentCap,
			MemberIds: []uuid.UUID{}, InboxIds: []uuid.UUID{},
		}
		for _, m := range members {
			if m.TeamID == t.ID {
				out[i].MemberIds = append(out[i].MemberIds, m.MemberID)
			}
		}
		for _, in := range inboxes {
			if in.TeamID == t.ID {
				out[i].InboxIds = append(out[i].InboxIds, in.InboxID)
			}
		}
	}
	return out, nil
}

func teamBody(ctx context.Context, q *store.Queries, t store.Team) (oas.Team, error) {
	out, err := teamBodies(ctx, q, t.WorkspaceID, []store.Team{t})
	if err != nil {
		return oas.Team{}, err
	}
	return out[0], nil
}

func teamEvent(events *eventBatch, typ string, t oas.Team, removed []uuid.UUID) {
	d := oas.TeamEventData{
		Id: t.Id, Name: t.Name, Description: t.Description, MemberIds: t.MemberIds, InboxIds: t.InboxIds,
		AssignmentMode: t.AssignmentMode, AssignmentCap: t.AssignmentCap, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if len(removed) > 0 {
		d.RemovedMemberIds = &removed
	}
	events.add(typ, nil, nil, d)
}

// seenInboxes keeps the inboxes the caller sees; nil when they see every inbox.
func seenInboxes(ctx context.Context, q *store.Queries, p principal) (map[uuid.UUID]bool, error) {
	if p.seesAllInboxes() {
		return nil, nil
	}
	ids, err := q.ListViewerInboxIDs(ctx, store.ListViewerInboxIDsParams{WorkspaceID: p.workspaceID, ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func onlySeen(ids []uuid.UUID, seen map[uuid.UUID]bool) []uuid.UUID {
	if seen == nil {
		return ids
	}
	out := []uuid.UUID{}
	for _, id := range ids {
		if seen[id] {
			out = append(out, id)
		}
	}
	return out
}

// shareLockTeam is taken before a conversation's lock wherever a team is set or kept, in the same
// order as a team's delete and revoke (team, then conversations), so neither misses the other.
func shareLockTeam(ctx context.Context, q *store.Queries, workspaceID, teamID uuid.UUID) error {
	_, err := q.ShareLockTeam(ctx, store.ShareLockTeamParams{WorkspaceID: workspaceID, ID: teamID})
	if store.IsNotFound(err) {
		return errTeamAccess
	}
	return err
}

// clearTeam takes the team off its conversations (of one inbox when inboxID is set), with one
// conversation.updated each and no timeline entry.
func clearTeam(ctx context.Context, q *store.Queries, events *eventBatch, workspaceID, teamID uuid.UUID, inboxID *uuid.UUID, now time.Time) error {
	rows, err := q.ClearConversationsTeam(ctx, store.ClearConversationsTeamParams{WorkspaceID: workspaceID, TeamID: &teamID, InboxID: inboxID, Now: now})
	if err != nil || len(rows) == 0 {
		return err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	labels, err := conversationLabels(ctx, q, workspaceID, ids)
	if err != nil {
		return err
	}
	bodies := make([]oas.Conversation, len(rows))
	refs := make([]*oas.Conversation, len(rows))
	for i, r := range rows {
		bodies[i] = conversationFields(r, labels[r.ID])
		refs[i] = &bodies[i]
	}
	if err := fillPurposes(ctx, q, workspaceID, refs...); err != nil {
		return err
	}
	if err := fillImportSources(ctx, q, workspaceID, refs...); err != nil {
		return err
	}
	if err := fillIdeas(ctx, q, workspaceID, refs...); err != nil {
		return err
	}
	for i, r := range rows {
		events.conversation(realtime.ConversationUpdated, r, bodies[i])
	}
	return nil
}

func teamFields(name, description *string, t *store.Team) error {
	var err error
	if name != nil {
		if t.Name, err = trimmed(*name, 1, 64, "name"); err != nil {
			return err
		}
	}
	if description != nil {
		if t.Description, err = trimmed(*description, 0, 500, "description"); err != nil {
			return err
		}
	}
	return nil
}

type accessPair struct{ member, inbox uuid.UUID }

// memberAccess reads the members' effective inbox access; callers hold LockInboxAccess, so two
// changes never miss each other's flip.
func memberAccess(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, members []uuid.UUID) (map[accessPair]bool, error) {
	out := map[accessPair]bool{}
	if len(members) == 0 {
		return out, nil
	}
	rows, err := q.ListMembersInboxAccess(ctx, store.ListMembersInboxAccessParams{WorkspaceID: workspaceID, MemberIds: members})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[accessPair{r.MemberID, r.InboxID}] = true
	}
	return out, nil
}

// accessFlips sends inbox_access.changed for each member and inbox whose effective access differs
// between before and after.
func accessFlips(events *eventBatch, before, after map[accessPair]bool) {
	var flips []accessPair
	for k := range before {
		if !after[k] {
			flips = append(flips, k)
		}
	}
	for k := range after {
		if !before[k] {
			flips = append(flips, k)
		}
	}
	slices.SortFunc(flips, func(a, b accessPair) int {
		if c := a.member.Compare(b.member); c != 0 {
			return c
		}
		return a.inbox.Compare(b.inbox)
	})
	for _, f := range flips {
		accessChanged(events, f.inbox, f.member, after[f])
	}
}

// changeAccess runs change, which reports whether it changed anything, between two reads of the
// members' effective access and sends the flips.
func changeAccess(ctx context.Context, q *store.Queries, events *eventBatch, workspaceID uuid.UUID, members []uuid.UUID, change func() (bool, error)) (bool, error) {
	before, err := memberAccess(ctx, q, workspaceID, members)
	if err != nil {
		return false, err
	}
	changed, err := change()
	if err != nil || !changed {
		return changed, err
	}
	after, err := memberAccess(ctx, q, workspaceID, members)
	if err != nil {
		return false, err
	}
	accessFlips(events, before, after)
	return true, nil
}

func (s *Server) ListTeams(ctx context.Context, _ oas.ListTeamsRequestObject) (oas.ListTeamsResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListTeams(ctx, store.ListTeamsParams{WorkspaceID: p.workspaceID, AllTeams: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	items, err := teamBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	seen, err := seenInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].InboxIds = onlySeen(items[i].InboxIds, seen)
	}
	return oas.ListTeams200JSONResponse{Items: items}, nil
}

func (s *Server) GetTeam(ctx context.Context, req oas.GetTeamRequestObject) (oas.GetTeamResponseObject, error) {
	p := principalFrom(ctx)
	id := req.TeamId
	rows, err := s.st.ListTeams(ctx, store.ListTeamsParams{WorkspaceID: p.workspaceID, ID: &id, AllTeams: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errTeamGone
	}
	out, err := teamBody(ctx, s.st.Queries, rows[0])
	if err != nil {
		return nil, err
	}
	seen, err := seenInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	out.InboxIds = onlySeen(out.InboxIds, seen)
	return oas.GetTeam200JSONResponse(out), nil
}

func (s *Server) CreateTeam(ctx context.Context, req oas.CreateTeamRequestObject) (oas.CreateTeamResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	t := store.Team{AssignmentMode: string(oas.AssignmentOff)}
	if err := teamFields(&req.Body.Name, req.Body.Description, &t); err != nil {
		return nil, err
	}
	as := assignmentSettings{mode: oas.AssignmentOff, cap: req.Body.AssignmentCap}
	if req.Body.AssignmentMode != nil {
		as.mode = *req.Body.AssignmentMode
	}
	if err := as.validate(); err != nil {
		return nil, err
	}
	var out oas.Team
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		created, err := q.CreateTeam(ctx, store.CreateTeamParams{
			ID: newID(), WorkspaceID: p.workspaceID, Name: t.Name, Description: t.Description,
			AssignmentMode: string(as.mode), AssignmentCap: as.cap, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if out, err = teamBody(ctx, q, created); err != nil {
			return err
		}
		teamEvent(events, eventTeamCreated, out, nil)
		return s.audit(ctx, q, p, oas.AuditTeamCreated, oas.AuditTargetTeam, created.ID, auditDetails{"name": created.Name})
	})
	if store.IsUniqueViolation(err) {
		return nil, errTeamTaken
	}
	if err != nil {
		return nil, err
	}
	return oas.CreateTeam201JSONResponse(out), nil
}

func (s *Server) UpdateTeam(ctx context.Context, req oas.UpdateTeamRequestObject) (oas.UpdateTeamResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var out oas.Team
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		t, err := q.LockTeam(ctx, store.LockTeamParams{WorkspaceID: p.workspaceID, ID: req.TeamId})
		if store.IsNotFound(err) {
			return errTeamGone
		}
		if err != nil {
			return err
		}
		next := t
		if err := teamFields(req.Body.Name, req.Body.Description, &next); err != nil {
			return err
		}
		as := assignmentSettings{mode: oas.AssignmentMode(t.AssignmentMode), cap: t.AssignmentCap}
		as.apply(req.Body.AssignmentMode, req.Body.AssignmentCap)
		if err := as.validate(); err != nil {
			return err
		}
		if next.Name == t.Name && next.Description == t.Description && string(as.mode) == t.AssignmentMode && sameInt(as.cap, t.AssignmentCap) {
			out, err = teamBody(ctx, q, t)
			return err
		}
		updated, err := q.UpdateTeam(ctx, store.UpdateTeamParams{
			WorkspaceID: p.workspaceID, ID: t.ID, Name: next.Name, Description: next.Description,
			AssignmentMode: string(as.mode), AssignmentCap: as.cap, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if as.widened(t.AssignmentMode, t.AssignmentCap) {
			events.jobOnce(AssignWaitingArgs{WorkspaceID: p.workspaceID}, nil)
		}
		if out, err = teamBody(ctx, q, updated); err != nil {
			return err
		}
		teamEvent(events, eventTeamUpdated, out, nil)
		return nil
	})
	if store.IsUniqueViolation(err) {
		return nil, errTeamTaken
	}
	if err != nil {
		return nil, err
	}
	return oas.UpdateTeam200JSONResponse(out), nil
}

func (s *Server) DeleteTeam(ctx context.Context, req oas.DeleteTeamRequestObject) (oas.DeleteTeamResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		t, err := q.LockTeam(ctx, store.LockTeamParams{WorkspaceID: p.workspaceID, ID: req.TeamId})
		if store.IsNotFound(err) {
			return errTeamGone
		}
		if err != nil {
			return err
		}
		body, err := teamBody(ctx, q, t)
		if err != nil {
			return err
		}
		if err := clearTeam(ctx, q, events, p.workspaceID, t.ID, nil, s.now()); err != nil {
			return err
		}
		if _, err := changeAccess(ctx, q, events, p.workspaceID, body.MemberIds, func() (bool, error) {
			_, err := q.DeleteTeam(ctx, store.DeleteTeamParams{WorkspaceID: p.workspaceID, ID: t.ID})
			return true, err
		}); err != nil {
			return err
		}
		if err := q.DeleteAssignmentTurns(ctx, store.DeleteAssignmentTurnsParams{WorkspaceID: p.workspaceID, PoolID: t.ID}); err != nil {
			return err
		}
		if err := q.StripTeamFromSavedViews(ctx, store.StripTeamFromSavedViewsParams{WorkspaceID: p.workspaceID, TeamID: t.ID.String()}); err != nil {
			return err
		}
		teamEvent(events, eventTeamDeleted, body, nil)
		return s.audit(ctx, q, p, oas.AuditTeamDeleted, oas.AuditTargetTeam, t.ID, auditDetails{"name": t.Name})
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteTeam204Response{}, nil
}

// changeTeam locks inbox access and the team, runs change and, when it changed something, sends
// team.updated.
func (s *Server) changeTeam(ctx context.Context, p principal, teamID uuid.UUID, change func(q *store.Queries, events *eventBatch, t store.Team) (bool, []uuid.UUID, error)) error {
	if err := requireManagerOrKey(p); err != nil {
		return err
	}
	return s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		t, err := q.LockTeam(ctx, store.LockTeamParams{WorkspaceID: p.workspaceID, ID: teamID})
		if store.IsNotFound(err) {
			return errTeamGone
		}
		if err != nil {
			return err
		}
		changed, removed, err := change(q, events, t)
		if err != nil || !changed {
			return err
		}
		touched, err := q.TouchTeam(ctx, store.TouchTeamParams{WorkspaceID: p.workspaceID, ID: t.ID, Now: s.now()})
		if err != nil {
			return err
		}
		body, err := teamBody(ctx, q, touched)
		if err != nil {
			return err
		}
		teamEvent(events, eventTeamUpdated, body, removed)
		return nil
	})
}

func teamMember(ctx context.Context, q *store.Queries, workspaceID, memberID uuid.UUID) error {
	if _, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: workspaceID, ID: memberID}); store.IsNotFound(err) {
		return errMemberGone
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Server) AddTeamMember(ctx context.Context, req oas.AddTeamMemberRequestObject) (oas.AddTeamMemberResponseObject, error) {
	p := principalFrom(ctx)
	err := s.changeTeam(ctx, p, req.TeamId, func(q *store.Queries, events *eventBatch, t store.Team) (bool, []uuid.UUID, error) {
		if err := teamMember(ctx, q, p.workspaceID, req.MemberId); err != nil {
			return false, nil, err
		}
		changed, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{req.MemberId}, func() (bool, error) {
			n, err := q.AddTeamMember(ctx, store.AddTeamMemberParams{WorkspaceID: p.workspaceID, TeamID: t.ID, MemberID: req.MemberId, Now: s.now()})
			if err != nil || n == 0 {
				return false, err
			}
			count, err := q.CountTeamMembers(ctx, store.CountTeamMembersParams{WorkspaceID: p.workspaceID, TeamID: t.ID})
			if err != nil {
				return false, err
			}
			if count > maxTeamMembers {
				return false, errTeamMemberLimit
			}
			return true, nil
		})
		if err != nil || !changed {
			return false, nil, err
		}
		return true, nil, s.audit(ctx, q, p, oas.AuditTeamMemberAdded, oas.AuditTargetTeam, t.ID, auditDetails{"member_id": req.MemberId})
	})
	if err != nil {
		return nil, err
	}
	return oas.AddTeamMember204Response{}, nil
}

func (s *Server) RemoveTeamMember(ctx context.Context, req oas.RemoveTeamMemberRequestObject) (oas.RemoveTeamMemberResponseObject, error) {
	p := principalFrom(ctx)
	err := s.changeTeam(ctx, p, req.TeamId, func(q *store.Queries, events *eventBatch, t store.Team) (bool, []uuid.UUID, error) {
		if err := teamMember(ctx, q, p.workspaceID, req.MemberId); err != nil {
			return false, nil, err
		}
		changed, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{req.MemberId}, func() (bool, error) {
			n, err := q.RemoveTeamMember(ctx, store.RemoveTeamMemberParams{WorkspaceID: p.workspaceID, TeamID: t.ID, MemberID: req.MemberId})
			return n > 0, err
		})
		if err != nil || !changed {
			return false, nil, err
		}
		return true, []uuid.UUID{req.MemberId}, s.audit(ctx, q, p, oas.AuditTeamMemberRemoved, oas.AuditTargetTeam, t.ID, auditDetails{"member_id": req.MemberId})
	})
	if err != nil {
		return nil, err
	}
	return oas.RemoveTeamMember204Response{}, nil
}

func (s *Server) teamInboxChange(ctx context.Context, p principal, teamID, inboxID uuid.UUID, grant bool) error {
	return s.changeTeam(ctx, p, teamID, func(q *store.Queries, events *eventBatch, t store.Team) (bool, []uuid.UUID, error) {
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inboxID})
		if store.IsNotFound(err) {
			return false, nil, errInboxGone
		}
		if err != nil {
			return false, nil, err
		}
		if grant && in.OwnerMemberID != nil {
			return false, nil, errPersonalMailbox("only its owner reads a personal mailbox")
		}
		rows, err := q.ListTeamMemberIDs(ctx, store.ListTeamMemberIDsParams{WorkspaceID: p.workspaceID, TeamIds: []uuid.UUID{t.ID}})
		if err != nil {
			return false, nil, err
		}
		members := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			members[i] = r.MemberID
		}
		changed, err := changeAccess(ctx, q, events, p.workspaceID, members, func() (bool, error) {
			if !grant {
				n, err := q.RemoveTeamInbox(ctx, store.RemoveTeamInboxParams{WorkspaceID: p.workspaceID, TeamID: t.ID, InboxID: in.ID})
				if err != nil || n == 0 {
					return false, err
				}
				return true, clearTeam(ctx, q, events, p.workspaceID, t.ID, &in.ID, s.now())
			}
			n, err := q.AddTeamInbox(ctx, store.AddTeamInboxParams{WorkspaceID: p.workspaceID, TeamID: t.ID, InboxID: in.ID, Now: s.now()})
			if err != nil || n == 0 {
				return false, err
			}
			count, err := q.CountTeamInboxes(ctx, store.CountTeamInboxesParams{WorkspaceID: p.workspaceID, TeamID: t.ID})
			if err != nil {
				return false, err
			}
			if count > maxTeamInboxes {
				return false, errTeamInboxLimit
			}
			return true, nil
		})
		if err != nil || !changed {
			return false, nil, err
		}
		action := oas.AuditTeamInboxAccessRevoked
		if grant {
			action = oas.AuditTeamInboxAccessGranted
		}
		return true, nil, s.audit(ctx, q, p, action, oas.AuditTargetTeam, t.ID, auditDetails{"inbox_id": in.ID, "inbox_name": in.Name})
	})
}

func (s *Server) GrantTeamInboxAccess(ctx context.Context, req oas.GrantTeamInboxAccessRequestObject) (oas.GrantTeamInboxAccessResponseObject, error) {
	if err := s.teamInboxChange(ctx, principalFrom(ctx), req.TeamId, req.InboxId, true); err != nil {
		return nil, err
	}
	return oas.GrantTeamInboxAccess204Response{}, nil
}

func (s *Server) RevokeTeamInboxAccess(ctx context.Context, req oas.RevokeTeamInboxAccessRequestObject) (oas.RevokeTeamInboxAccessResponseObject, error) {
	if err := s.teamInboxChange(ctx, principalFrom(ctx), req.TeamId, req.InboxId, false); err != nil {
		return nil, err
	}
	return oas.RevokeTeamInboxAccess204Response{}, nil
}

// teamEventVisible tells whether a team event reaches the caller: every team for those who see
// every inbox, otherwise teams they belong to or that hold an inbox they see.
func (f *eventFilter) teamEventVisible(e realtime.Event) (bool, error) {
	if requireScope(f.p, oas.InboxesRead) != nil {
		return false, nil
	}
	if f.p.seesAllInboxes() {
		return true, nil
	}
	var t oas.TeamEventData
	if err := json.Unmarshal(e.Data, &t); err != nil {
		return false, err
	}
	if !f.p.isKey() && (slices.Contains(t.MemberIds, f.p.memberID) || (t.RemovedMemberIds != nil && slices.Contains(*t.RemovedMemberIds, f.p.memberID))) {
		return true, nil
	}
	return slices.ContainsFunc(t.InboxIds, func(id uuid.UUID) bool { return f.inboxes[id] }), nil
}

// teamView keeps only the inboxes the caller sees in a team event.
func (f *eventFilter) teamView(e realtime.Event) (realtime.Event, bool) {
	if f.p.seesAllInboxes() {
		return e, true
	}
	var t oas.TeamEventData
	if json.Unmarshal(e.Data, &t) != nil {
		return e, false
	}
	t.InboxIds = onlySeen(t.InboxIds, f.inboxes)
	b, err := json.Marshal(t)
	if err != nil {
		return e, false
	}
	e.Data = b
	return e, true
}

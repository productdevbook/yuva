package api

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const assignWaitingPage = 50

type assignmentSettings struct {
	mode oas.AssignmentMode
	cap  *int32
}

func (a *assignmentSettings) apply(mode *oas.AssignmentMode, limit nullable.Nullable[int32]) {
	if mode != nil {
		a.mode = *mode
	}
	if limit.IsSpecified() {
		a.cap = nil
		if !limit.IsNull() {
			a.cap = new(limit.MustGet())
		}
	}
}

func (a assignmentSettings) validate() error {
	if !a.mode.Valid() {
		return errValidation("assignment_mode must be off, round_robin or least_open")
	}
	if a.cap != nil && (*a.cap < 1 || *a.cap > 500) {
		return errValidation("assignment_cap must be 1 to 500 or null")
	}
	return nil
}

// widened reports a change that can make someone eligible for waiting conversations.
func (a assignmentSettings) widened(mode string, limit *int32) bool {
	if a.mode == oas.AssignmentOff || (string(a.mode) == mode && sameInt(a.cap, limit)) {
		return false
	}
	return mode == string(oas.AssignmentOff) || a.cap == nil || (limit != nil && *a.cap > *limit)
}

func assignJobStates() []rivertype.JobState {
	return []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateScheduled, rivertype.JobStateRetryable,
	}
}

// AutoAssignArgs gives one conversation a member of its pool. Reopen lets it take the conversation
// from an assignee who can no longer answer it.
type AutoAssignArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Reopen         bool      `json:"reopen,omitempty"`
}

func (AutoAssignArgs) Kind() string { return "auto_assign" }

func (AutoAssignArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: assignJobStates()}}
}

// AssignWaitingArgs assigns the workspace's waiting conversations, oldest first.
type AssignWaitingArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
}

func (AssignWaitingArgs) Kind() string { return "assign_waiting" }

func (AssignWaitingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: assignJobStates()}}
}

type assignTrigger struct {
	conversation uuid.UUID
	reopen       bool
}

// assignTriggers lists the conversations of a batch that enter a pool: created ones, and ones the
// contact or a snooze's end reopened.
func assignTriggers(items []pendingEvent) []assignTrigger {
	var out []assignTrigger
	add := func(id uuid.UUID, reopen bool) {
		for i, t := range out {
			if t.conversation == id {
				out[i].reopen = out[i].reopen && reopen
				return
			}
		}
		out = append(out, assignTrigger{conversation: id, reopen: reopen})
	}
	for _, e := range items {
		if e.conversationID == nil {
			continue
		}
		if trigger, _, ok := automationTriggerOf(e); ok && trigger == oas.AutomationTriggerConversationCreated {
			add(*e.conversationID, false)
			continue
		}
		if e.typ != realtime.MessageCreated {
			continue
		}
		var m struct {
			Kind   oas.MessageKind `json:"kind"`
			Author struct {
				Type oas.AuthorType `json:"type"`
			} `json:"author"`
			Event *oas.MessageEvent `json:"event"`
		}
		if json.Unmarshal(e.data, &m) != nil || m.Kind != oas.MessageKindEvent || m.Author.Type != oas.AuthorTypeSystem || m.Event == nil {
			continue
		}
		ev := m.Event
		if ev.Type == oas.StatusChanged && ev.Status != nil && *ev.Status == oas.ConversationStatusOpen &&
			ev.PreviousStatus != nil && *ev.PreviousStatus != oas.ConversationStatusOpen {
			add(*e.conversationID, true)
		}
	}
	return out
}

// freesMember reports an event after which a member may hold fewer open conversations.
func freesMember(e pendingEvent) bool {
	if e.typ != realtime.MessageCreated {
		return false
	}
	var m struct {
		Kind  oas.MessageKind   `json:"kind"`
		Event *oas.MessageEvent `json:"event"`
	}
	if json.Unmarshal(e.data, &m) != nil || m.Kind != oas.MessageKindEvent || m.Event == nil {
		return false
	}
	switch m.Event.Type {
	case oas.StatusChanged:
		return m.Event.PreviousStatus != nil && *m.Event.PreviousStatus == oas.ConversationStatusOpen
	case oas.Assigned, oas.Unassigned:
		return m.Event.PreviousAssigneeId != nil
	}
	return false
}

// queueAssignWaiting runs in the transaction that wrote the events: a member's conversation that
// left `open` or lost them may make room for a waiting one.
func (s *Server) queueAssignWaiting(ctx context.Context, tx pgx.Tx, q *store.Queries, workspaceID uuid.UUID, ev *eventBatch) error {
	if ev.assign || !slices.ContainsFunc(ev.items, freesMember) {
		return nil
	}
	waiting, err := q.HasAssignWaiting(ctx, workspaceID)
	if err != nil || !waiting {
		return err
	}
	_, err = s.jobs.InsertManyTx(ctx, tx, []river.InsertManyParams{{Args: AssignWaitingArgs{WorkspaceID: workspaceID}}})
	return err
}

// wakeAssignWaiting queues the waiting conversations of the workspace for a member who became
// available.
func (s *Server) wakeAssignWaiting(ctx context.Context, workspaceID uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	waiting, err := s.st.HasAssignWaiting(ctx, workspaceID)
	if err == nil && waiting {
		_, err = s.jobs.Insert(ctx, AssignWaitingArgs{WorkspaceID: workspaceID}, nil)
	}
	if err != nil {
		s.log.WarnContext(ctx, "assign waiting", "error", err)
	}
}

type autoAssignWorker struct {
	river.WorkerDefaults[AutoAssignArgs]
	s *Server
}

func (w *autoAssignWorker) Work(ctx context.Context, job *river.Job[AutoAssignArgs]) error {
	return w.s.AutoAssign(ctx, job.Args)
}

type assignWaitingWorker struct {
	river.WorkerDefaults[AssignWaitingArgs]
	s *Server
}

func (w *assignWaitingWorker) Work(ctx context.Context, job *river.Job[AssignWaitingArgs]) error {
	return w.s.AssignWaiting(ctx, job.Args)
}

type assignSweepWorker struct {
	river.WorkerDefaults[jobs.AssignSweepArgs]
	s *Server
}

func (w *assignSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.AssignSweepArgs]) error {
	return w.s.SweepAssignWaiting(ctx)
}

func (s *Server) liveWorkspace(ctx context.Context, ws uuid.UUID) (bool, error) {
	w, err := s.st.GetWorkspace(ctx, ws)
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return w.DeletedAt == nil, nil
}

// AutoAssign gives the conversation a member of its pool, or leaves it waiting. Every automatic
// assignment of a workspace holds the same lock while it counts and assigns, so no cap is passed.
func (s *Server) AutoAssign(ctx context.Context, a AutoAssignArgs) error {
	if ok, err := s.liveWorkspace(ctx, a.WorkspaceID); err != nil || !ok {
		return err
	}
	return s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, ev *eventBatch) error {
		ev.assign = true
		if err := q.LockAssignment(ctx, a.WorkspaceID); err != nil {
			return err
		}
		_, err := s.assignOne(ctx, q, ev, a.WorkspaceID, a.ConversationID, a.Reopen, false, s.now())
		return err
	})
}

type poolInbox struct{ pool, inbox uuid.UUID }

// AssignWaiting walks the waiting conversations oldest first, a page of ids at a time, to the end.
// Each conversation is decided in its own short transaction, so the run never holds more than one
// conversation's lock; a pool that had nobody eligible for an inbox is not asked again for that
// inbox in the same page.
func (s *Server) AssignWaiting(ctx context.Context, a AssignWaitingArgs) error {
	if ok, err := s.liveWorkspace(ctx, a.WorkspaceID); err != nil || !ok {
		return err
	}
	var afterSince time.Time
	var afterID uuid.UUID
	for {
		rows, err := s.st.ListAssignWaiting(ctx, store.ListAssignWaitingParams{
			WorkspaceID: a.WorkspaceID, AfterSince: afterSince, AfterID: afterID, Lim: assignWaitingPage,
		})
		if err != nil || len(rows) == 0 {
			return err
		}
		empty := map[poolInbox]bool{}
		for _, r := range rows {
			key := poolInbox{pool: r.InboxID, inbox: r.InboxID}
			if r.TeamID != nil {
				key.pool = *r.TeamID
			}
			if empty[key] {
				continue
			}
			var got assignResult
			err := s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, ev *eventBatch) error {
				ev.assign = true
				if err := q.LockAssignment(ctx, a.WorkspaceID); err != nil {
					return err
				}
				var err error
				got, err = s.assignOne(ctx, q, ev, a.WorkspaceID, r.ID, false, true, s.now())
				return err
			})
			if err != nil {
				return err
			}
			if got == assignNobody {
				empty[key] = true
			}
		}
		last := rows[len(rows)-1]
		afterSince, afterID = *last.AssignWaitingSince, last.ID
		if len(rows) < assignWaitingPage {
			return nil
		}
	}
}

// SweepAssignWaiting queues the waiting conversations of every workspace where a member is
// available: it covers business hours that opened and members who became available while a run
// was already under way.
func (s *Server) SweepAssignWaiting(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	fresh := s.now().Add(-presenceFresh)
	var queued []river.InsertManyParams
	for _, ws := range ids {
		ready, err := s.st.AssignWaitingReady(ctx, store.AssignWaitingReadyParams{WorkspaceID: ws, FreshAfter: fresh})
		if err != nil {
			return err
		}
		if ready {
			queued = append(queued, river.InsertManyParams{Args: AssignWaitingArgs{WorkspaceID: ws}})
		}
	}
	if len(queued) == 0 {
		return nil
	}
	_, err = s.jobs.InsertMany(ctx, queued)
	return err
}

type assignPool struct {
	id    uuid.UUID
	team  *uuid.UUID
	mode  oas.AssignmentMode
	cap   *int32
	inbox store.Inbox
}

func (s *Server) poolOf(ctx context.Context, q *store.Queries, c store.Conversation) (assignPool, error) {
	in, err := q.ShareInbox(ctx, store.ShareInboxParams{WorkspaceID: c.WorkspaceID, ID: c.InboxID})
	if err != nil {
		return assignPool{}, err
	}
	pool := assignPool{id: in.ID, mode: oas.AssignmentMode(in.AssignmentMode), cap: in.AssignmentCap, inbox: in}
	if in.Purpose == purposeMail {
		pool.mode = oas.AssignmentOff
		return pool, nil
	}
	if c.TeamID == nil {
		return pool, nil
	}
	t, err := q.GetTeam(ctx, store.GetTeamParams{WorkspaceID: c.WorkspaceID, ID: *c.TeamID})
	if store.IsNotFound(err) {
		return pool, nil
	}
	if err != nil {
		return pool, err
	}
	pool.id, pool.team, pool.mode, pool.cap = t.ID, &t.ID, oas.AssignmentMode(t.AssignmentMode), t.AssignmentCap
	return pool, nil
}

type assignResult int

const (
	// assignSkipped: the conversation is not the pool's to give (gone, closed, spam, trashed, has a
	// member, its pool is off, or its rules have not run yet).
	assignSkipped assignResult = iota
	assignNobody
	assignGiven
)

// assignOne decides one conversation under the assignment lock. waiting is a run over waiting
// conversations, which leaves alone one whose rules still have to run.
func (s *Server) assignOne(ctx context.Context, q *store.Queries, ev *eventBatch, ws, id uuid.UUID, reopen, waiting bool, now time.Time) (assignResult, error) {
	c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return assignSkipped, nil
	}
	if err != nil {
		return assignSkipped, err
	}
	if waiting && c.AssignAfterRules {
		return assignSkipped, nil
	}
	stop := func() (assignResult, error) {
		return assignSkipped, q.StopAssignWaiting(ctx, store.StopAssignWaitingParams{WorkspaceID: ws, ID: c.ID})
	}
	if c.TrashedAt != nil || c.Spam || c.Status != string(oas.ConversationStatusOpen) {
		return stop()
	}
	pool, err := s.poolOf(ctx, q, c)
	if err != nil {
		return assignSkipped, err
	}
	if pool.mode == oas.AssignmentOff {
		return stop()
	}
	p := principal{workspaceID: ws, auto: true}
	if c.AssigneeID != nil {
		if !reopen {
			return stop()
		}
		keep, err := assigneeKeeps(ctx, q, c)
		if err != nil || keep {
			if err != nil {
				return assignSkipped, err
			}
			return stop()
		}
	}
	member, err := s.pickMember(ctx, q, pool, now)
	if err != nil {
		return assignSkipped, err
	}
	if member == nil {
		if c.AssigneeID != nil {
			if _, err := s.changeConversation(ctx, q, ev, p, c.ID, oas.ConversationUpdate{AssigneeId: nullable.NewNullNullable[uuid.UUID]()}, nil, nil); err != nil {
				return assignSkipped, err
			}
		}
		started, err := q.StartAssignWaiting(ctx, store.StartAssignWaitingParams{WorkspaceID: ws, ID: c.ID, Now: now})
		if store.IsNotFound(err) {
			return assignNobody, nil
		}
		if err != nil {
			return assignSkipped, err
		}
		body, err := oneConversation(ctx, q, started)
		if err != nil {
			return assignSkipped, err
		}
		ev.conversation(realtime.ConversationUpdated, started, body)
		return assignNobody, nil
	}
	if err := q.TakeAssignmentTurn(ctx, store.TakeAssignmentTurnParams{WorkspaceID: ws, PoolID: pool.id, MemberID: *member, Now: now}); err != nil {
		return assignSkipped, err
	}
	if _, err := s.changeConversation(ctx, q, ev, p, c.ID, oas.ConversationUpdate{AssigneeId: nullable.NewNullableWithValue(*member)}, nil, nil); err != nil {
		return assignSkipped, err
	}
	return assignGiven, nil
}

// assigneeKeeps reports an assignee who can still answer a reopened conversation: access to its
// inbox and not away.
func assigneeKeeps(ctx context.Context, q *store.Queries, c store.Conversation) (bool, error) {
	ok, err := memberHasInbox(ctx, q, c.WorkspaceID, c.InboxID, *c.AssigneeID)
	if err != nil || !ok {
		return false, err
	}
	away, err := q.MemberAway(ctx, store.MemberAwayParams{WorkspaceID: c.WorkspaceID, ID: *c.AssigneeID})
	if store.IsNotFound(err) {
		return false, nil
	}
	return !away, err
}

// pickMember chooses among the pool's eligible members, or nil when there is none.
func (s *Server) pickMember(ctx context.Context, q *store.Queries, pool assignPool, now time.Time) (*uuid.UUID, error) {
	if !inboxOpen(pool.inbox, now) {
		return nil, nil
	}
	rows, err := q.ListAssignCandidates(ctx, store.ListAssignCandidatesParams{
		WorkspaceID: pool.inbox.WorkspaceID, PoolID: pool.id, InboxID: pool.inbox.ID, TeamID: pool.team, FreshAfter: now.Add(-presenceFresh),
	})
	if err != nil {
		return nil, err
	}
	rows = slices.DeleteFunc(rows, func(r store.ListAssignCandidatesRow) bool { return pool.cap != nil && r.OpenCount >= *pool.cap })
	if len(rows) == 0 {
		return nil, nil
	}
	turn := func(a, b store.ListAssignCandidatesRow) int {
		switch {
		case a.LastAt == nil && b.LastAt == nil:
			return a.ID.Compare(b.ID)
		case a.LastAt == nil:
			return -1
		case b.LastAt == nil:
			return 1
		}
		return cmp.Or(a.LastAt.Compare(*b.LastAt), a.ID.Compare(b.ID))
	}
	best := slices.MinFunc(rows, func(a, b store.ListAssignCandidatesRow) int {
		if pool.mode == oas.AssignmentLeastOpen {
			if c := cmp.Compare(a.OpenCount, b.OpenCount); c != 0 {
				return c
			}
		}
		return turn(a, b)
	})
	return &best.ID, nil
}

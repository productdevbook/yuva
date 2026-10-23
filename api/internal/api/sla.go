package api

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	eventSLAWarning  = string(oas.ConversationSlaWarningEvent)
	eventSLABreached = string(oas.ConversationSlaBreachedEvent)

	slaSweepBatch  = 500
	snoozeWakeSize = 500
)

type slaTarget struct {
	name     oas.SlaTargetName
	targetS  **int32
	started  **time.Time
	due      **time.Time
	remain   **int32
	met      **time.Time
	breached **time.Time
	warned   **time.Time
}

func slaTargets(r *store.ConversationSla) [3]slaTarget {
	return [3]slaTarget{
		{oas.SlaFirstReply, &r.FirstReplyTargetS, &r.FirstReplyStartedAt, &r.FirstReplyDueAt, &r.FirstReplyRemainingS, &r.FirstReplyMetAt, &r.FirstReplyBreachedAt, &r.FirstReplyWarnedAt},
		{oas.SlaNextReply, &r.NextReplyTargetS, &r.NextReplyStartedAt, &r.NextReplyDueAt, &r.NextReplyRemainingS, &r.NextReplyMetAt, &r.NextReplyBreachedAt, &r.NextReplyWarnedAt},
		{oas.SlaResolution, &r.ResolutionTargetS, &r.ResolutionStartedAt, &r.ResolutionDueAt, &r.ResolutionRemainingS, &r.ResolutionMetAt, &r.ResolutionBreachedAt, &r.ResolutionWarnedAt},
	}
}

func slaTargetNamed(r *store.ConversationSla, name oas.SlaTargetName) (slaTarget, bool) {
	for _, t := range slaTargets(r) {
		if t.name == name {
			return t, true
		}
	}
	return slaTarget{}, false
}

func policyMinutes(p store.SlaPolicy, name oas.SlaTargetName) *int32 {
	switch name {
	case oas.SlaFirstReply:
		return p.FirstReplyMinutes
	case oas.SlaNextReply:
		return p.NextReplyMinutes
	}
	return p.ResolutionMinutes
}

func slaMatchKey(c store.Conversation, labels []uuid.UUID) string {
	ids := make([]string, len(labels))
	for i, l := range labels {
		ids[i] = l.String()
	}
	slices.Sort(ids)
	return c.InboxID.String() + "|" + c.Priority + "|" + strings.Join(ids, ",")
}

func slaPolicyMatches(p store.SlaPolicy, c store.Conversation, labels []uuid.UUID) bool {
	switch {
	case !p.Enabled:
		return false
	case len(p.InboxIds) > 0 && !slices.Contains(p.InboxIds, c.InboxID):
		return false
	case len(p.ChannelKinds) > 0 && (c.ChannelKind == nil || !slices.Contains(p.ChannelKinds, *c.ChannelKind)):
		return false
	case len(p.Priorities) > 0 && !slices.Contains(p.Priorities, c.Priority):
		return false
	case len(p.LabelIds) > 0 && !slices.ContainsFunc(labels, func(l uuid.UUID) bool { return slices.Contains(p.LabelIds, l) }):
		return false
	}
	return true
}

func dueTime(t time.Time) *time.Time {
	v := t.Truncate(time.Microsecond).UTC()
	return &v
}

func earliest(a *time.Time, b time.Time) *time.Time {
	if a == nil || b.Before(*a) {
		return &b
	}
	return a
}

func slaTargetState(t slaTarget, warn time.Duration, now time.Time) oas.SlaTargetState {
	switch {
	case *t.breached != nil:
		return oas.SlaTargetBreached
	case *t.met != nil:
		return oas.SlaTargetMet
	case *t.due != nil && !(*t.due).After(now):
		return oas.SlaTargetBreached
	case *t.due != nil && warn > 0 && !(*t.due).Add(-warn).After(now):
		return oas.SlaTargetDueSoon
	case *t.due != nil:
		return oas.SlaTargetOnTrack
	case *t.remain != nil:
		return oas.SlaTargetPaused
	}
	return oas.SlaTargetStopped
}

// slaBody mirrors the state the conversation list filters on (ListConversations, sla_states).
func slaBody(r store.ConversationSla, now time.Time) *oas.ConversationSla {
	out := &oas.ConversationSla{Targets: []oas.ConversationSlaTarget{}}
	if r.PolicyID != nil {
		out.PolicyId = *r.PolicyID
	}
	warn := time.Duration(r.WarnMinutes) * time.Minute
	var due *time.Time
	var breached, met bool
	for _, t := range slaTargets(&r) {
		if *t.targetS == nil || *t.started == nil {
			continue
		}
		item := oas.ConversationSlaTarget{
			Target: t.name, State: slaTargetState(t, warn, now), StartedAt: **t.started, DueAt: *t.due, MetAt: *t.met,
			BreachedAt: *t.breached, WarnedAt: *t.warned,
		}
		if *t.remain != nil {
			item.RemainingSeconds = new(int64(**t.remain))
		}
		out.Targets = append(out.Targets, item)
		if *t.due != nil {
			due = earliest(due, **t.due)
		}
		breached = breached || *t.breached != nil
		met = met || *t.met != nil
	}
	out.DueAt = due
	switch {
	case due != nil && !due.After(now):
		out.State = oas.SlaStateBreached
	case due != nil && warn > 0 && !due.Add(-warn).After(now):
		out.State = oas.SlaStateDueSoon
	case due != nil:
		out.State = oas.SlaStateOnTrack
	case r.PausedAt != nil:
		out.State = oas.SlaStatePaused
	case breached:
		out.State = oas.SlaStateBreached
	case met:
		out.State = oas.SlaStateMet
	default:
		out.State = oas.SlaStateOnTrack
	}
	return out
}

// fillSLA sets sla on conversations read outside the write that last changed them.
func (s *Server) fillSLA(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, convs ...*oas.Conversation) error {
	ids := make([]uuid.UUID, len(convs))
	for i, c := range convs {
		ids[i] = c.Id
	}
	rows, err := q.ListConversationSLAs(ctx, store.ListConversationSLAsParams{WorkspaceID: workspaceID, ConversationIds: ids})
	if err != nil {
		return err
	}
	now := s.now()
	for _, c := range convs {
		c.Sla = nil
		for _, r := range rows {
			if r.ConversationID == c.Id && r.DroppedAt == nil {
				c.Sla = slaBody(r, now)
			}
		}
	}
	return nil
}

type slaOutcome struct {
	target  oas.SlaTargetName
	started time.Time
	outcome string
	at      time.Time
}

// slaRun applies what happened to one conversation's targets. historical is set while a row is
// built from the conversation's past messages: outcomes are then recorded on the row only.
type slaRun struct {
	row        *store.ConversationSla
	policy     store.SlaPolicy
	clock      businessClock
	now        time.Time
	historical bool
	outcomes   []slaOutcome
}

func (e *slaRun) start(t slaTarget, at time.Time) {
	if e.historical && at.Before(e.policy.CreatedAt) {
		at = e.policy.CreatedAt
	}
	*t.started = &at
	*t.met, *t.breached, *t.warned, *t.due, *t.remain, *t.targetS = nil, nil, nil, nil, nil, nil
	mins := policyMinutes(e.policy, t.name)
	if mins == nil {
		return
	}
	secs := *mins * 60
	*t.targetS = &secs
	*t.due = dueTime(e.clock.add(at, time.Duration(secs)*time.Second))
}

// finish ends a running or paused target at at, met in time or late.
func (e *slaRun) finish(t slaTarget, at time.Time) {
	*t.met = &at
	if *t.targetS != nil && *t.breached == nil && (*t.due != nil || *t.remain != nil) {
		late := (*t.due != nil && at.After(**t.due)) || (*t.remain != nil && **t.remain <= 0)
		switch {
		case late && *t.due != nil:
			*t.breached = *t.due
		case late:
			*t.breached = &at
		}
		if !e.historical {
			outcome := "met"
			if late {
				outcome = "breached"
			}
			e.outcomes = append(e.outcomes, slaOutcome{t.name, **t.started, outcome, at})
		}
	}
	*t.due, *t.remain = nil, nil
}

func (e *slaRun) current() slaTarget {
	ts := slaTargets(e.row)
	if *ts[1].started != nil {
		return ts[1]
	}
	return ts[0]
}

func (e *slaRun) contactMessage(at time.Time) {
	if e.row.AwaitingReply {
		return
	}
	e.row.AwaitingReply = true
	ts := slaTargets(e.row)
	if *ts[0].started == nil {
		e.start(ts[0], at)
		return
	}
	e.start(ts[1], at)
}

func (e *slaRun) reply(at time.Time) {
	if !e.row.AwaitingReply {
		return
	}
	e.row.AwaitingReply = false
	if t := e.current(); *t.met == nil {
		e.finish(t, at)
	}
}

// pending lists the targets still waiting for their outcome.
func (e *slaRun) pending() []slaTarget {
	var out []slaTarget
	if e.row.AwaitingReply {
		if t := e.current(); *t.met == nil && *t.breached == nil {
			out = append(out, t)
		}
	}
	if t := slaTargets(e.row)[2]; *t.started != nil && *t.met == nil && *t.breached == nil {
		out = append(out, t)
	}
	return out
}

// retarget moves the pending targets to another policy, keeping the business time they have
// already used.
func (e *slaRun) retarget(p store.SlaPolicy) {
	e.policy = p
	for _, t := range e.pending() {
		var used time.Duration
		switch {
		case *t.targetS != nil && *t.remain != nil:
			used = time.Duration(**t.targetS-**t.remain) * time.Second
		case *t.targetS != nil && *t.due != nil:
			used = time.Duration(**t.targetS)*time.Second - e.clock.between(e.now, **t.due)
		default:
			used = e.clock.between(**t.started, e.now)
		}
		*t.due, *t.remain, *t.warned, *t.targetS = nil, nil, nil, nil
		mins := policyMinutes(p, t.name)
		if mins == nil {
			continue
		}
		secs := *mins * 60
		*t.targetS = &secs
		*t.due = dueTime(e.clock.add(e.now, max(time.Duration(secs)*time.Second-max(used, 0), 0)))
	}
}

// revive brings back a row whose policy stopped applying at dropped: the time from then on does
// not count, so a target that ran keeps the business time it had left at the drop.
func (e *slaRun) revive(dropped time.Time) {
	for _, t := range slaTargets(e.row) {
		if *t.due == nil {
			continue
		}
		left := int32(e.clock.between(dropped, **t.due).Round(time.Second) / time.Second)
		if *t.targetS != nil {
			left = min(left, **t.targetS)
		}
		*t.remain, *t.due = &left, nil
	}
	e.row.DroppedAt = nil
}

// catchUp aligns a revived row with the messages written while it was dropped: a contact still
// waiting starts a target now, a reply given meanwhile ends the one that waited, without an
// outcome.
func (e *slaRun) catchUp(hist []store.ListSLAMessagesRow) {
	if len(hist) == 0 {
		return
	}
	last := hist[len(hist)-1]
	in := last.Direction != nil && *last.Direction == string(oas.In)
	e.historical = true
	switch {
	case in && !e.row.AwaitingReply:
		e.contactMessage(e.now)
	case !in && e.row.AwaitingReply:
		e.reply(last.CreatedAt)
	}
	e.historical = false
}

func (e *slaRun) settle(status string) {
	switch status {
	case string(oas.ConversationStatusOpen):
		for _, t := range slaTargets(e.row) {
			if *t.remain != nil {
				*t.due = dueTime(e.clock.add(e.now, time.Duration(**t.remain)*time.Second))
				*t.remain = nil
			}
		}
		e.row.PausedAt = nil
	case string(oas.ConversationStatusPending), string(oas.ConversationStatusSnoozed):
		for _, t := range slaTargets(e.row) {
			if *t.due != nil {
				left := int32(e.clock.between(e.now, **t.due).Round(time.Second) / time.Second)
				*t.remain, *t.due = &left, nil
			}
		}
	case string(oas.ConversationStatusClosed):
		if t := slaTargets(e.row)[2]; *t.started != nil && *t.met == nil {
			e.finish(t, e.now)
		}
		if e.row.AwaitingReply {
			t := e.current()
			*t.due, *t.remain = nil, nil
			e.row.AwaitingReply = false
		}
		e.row.PausedAt = nil
	}
	paused := false
	for _, t := range slaTargets(e.row) {
		paused = paused || *t.remain != nil
	}
	switch {
	case !paused:
		e.row.PausedAt = nil
	case e.row.PausedAt == nil:
		e.row.PausedAt = &e.now
	}
	var check *time.Time
	for _, t := range slaTargets(e.row) {
		if *t.due != nil && *t.breached == nil {
			check = earliest(check, **t.due)
		}
	}
	e.row.CheckAt = check
}

func slaDue(r *store.ConversationSla) *time.Time {
	var due *time.Time
	for _, t := range slaTargets(r) {
		if *t.due != nil {
			due = earliest(due, **t.due)
		}
	}
	return due
}

func sameInt(a, b *int32) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// slaShown tells whether what Conversation.sla shows differs between a and b.
func slaShown(a, b store.ConversationSla) bool {
	if !sameID(a.PolicyID, b.PolicyID) || a.WarnMinutes != b.WarnMinutes || !sameTime(a.PausedAt, b.PausedAt) || !sameTime(a.DroppedAt, b.DroppedAt) {
		return true
	}
	ta, tb := slaTargets(&a), slaTargets(&b)
	for i := range ta {
		x, y := ta[i], tb[i]
		if !sameInt(*x.targetS, *y.targetS) || !sameInt(*x.remain, *y.remain) || !sameTime(*x.started, *y.started) || !sameTime(*x.due, *y.due) ||
			!sameTime(*x.met, *y.met) || !sameTime(*x.breached, *y.breached) || !sameTime(*x.warned, *y.warned) {
			return true
		}
	}
	return false
}

func saveSLAParams(r store.ConversationSla, now time.Time) store.SaveConversationSLAParams {
	return store.SaveConversationSLAParams{
		WorkspaceID: r.WorkspaceID, ConversationID: r.ConversationID, PolicyID: r.PolicyID, MatchKey: r.MatchKey, DroppedAt: r.DroppedAt, WarnMinutes: r.WarnMinutes,
		AwaitingReply: r.AwaitingReply, PausedAt: r.PausedAt, CheckAt: r.CheckAt,
		FirstReplyTargetS: r.FirstReplyTargetS, FirstReplyStartedAt: r.FirstReplyStartedAt, FirstReplyDueAt: r.FirstReplyDueAt,
		FirstReplyRemainingS: r.FirstReplyRemainingS, FirstReplyMetAt: r.FirstReplyMetAt, FirstReplyBreachedAt: r.FirstReplyBreachedAt,
		FirstReplyWarnedAt: r.FirstReplyWarnedAt,
		NextReplyTargetS:   r.NextReplyTargetS, NextReplyStartedAt: r.NextReplyStartedAt, NextReplyDueAt: r.NextReplyDueAt,
		NextReplyRemainingS: r.NextReplyRemainingS, NextReplyMetAt: r.NextReplyMetAt, NextReplyBreachedAt: r.NextReplyBreachedAt,
		NextReplyWarnedAt: r.NextReplyWarnedAt,
		ResolutionTargetS: r.ResolutionTargetS, ResolutionStartedAt: r.ResolutionStartedAt, ResolutionDueAt: r.ResolutionDueAt,
		ResolutionRemainingS: r.ResolutionRemainingS, ResolutionMetAt: r.ResolutionMetAt, ResolutionBreachedAt: r.ResolutionBreachedAt,
		ResolutionWarnedAt: r.ResolutionWarnedAt, Now: now,
	}
}

type slaMessage struct {
	at time.Time
	in bool
}

// slaMessageOf reads a message.created event: a public message from the contact, or a sent public
// reply of a member or bot. Notes, drafts, events, imports, automatic replies and rule replies are not.
func slaMessageOf(e pendingEvent) (slaMessage, bool) {
	var m struct {
		Kind       oas.MessageKind `json:"kind"`
		Direction  *oas.Direction  `json:"direction"`
		Draft      bool            `json:"draft"`
		ImportedAt *time.Time      `json:"imported_at"`
		CreatedAt  time.Time       `json:"created_at"`
		Rule       *struct{}       `json:"rule"`
		Author     struct {
			Type oas.AuthorType `json:"type"`
		} `json:"author"`
	}
	if json.Unmarshal(e.data, &m) != nil || m.Kind != oas.MessageKindMessage || m.Direction == nil || m.Draft || m.ImportedAt != nil || m.Rule != nil {
		return slaMessage{}, false
	}
	if *m.Direction == oas.In {
		return slaMessage{at: m.CreatedAt, in: true}, true
	}
	if m.Author.Type == oas.AuthorTypeSystem {
		return slaMessage{}, false
	}
	return slaMessage{at: m.CreatedAt}, true
}

var slaConversationEvents = []string{realtime.ConversationCreated, realtime.ConversationUpdated, realtime.ConversationRestored, realtime.ConversationTrashed}

// syncSLA brings the SLA of every conversation the batch touched up to date, in the transaction
// that changed them and under the conversation's lock the change already holds: the state,
// policy and status after the change decide; the batch's messages, in order, start and meet
// targets.
func (s *Server) syncSLA(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, ev *eventBatch) error {
	var order []uuid.UUID
	msgs := map[uuid.UUID][]slaMessage{}
	for _, e := range ev.items {
		if e.conversationID == nil {
			continue
		}
		switch e.typ {
		case realtime.MessageCreated:
			if m, ok := slaMessageOf(e); ok {
				msgs[*e.conversationID] = append(msgs[*e.conversationID], m)
			}
		case realtime.ConversationCreated, realtime.ConversationUpdated, realtime.ConversationMoved, realtime.ConversationTrashed, realtime.ConversationRestored:
		default:
			continue
		}
		if !slices.Contains(order, *e.conversationID) {
			order = append(order, *e.conversationID)
		}
	}
	if len(order) == 0 {
		return nil
	}
	has, err := q.WorkspaceHasSLAPolicies(ctx, workspaceID)
	if err != nil || !has {
		return err
	}
	for _, id := range order {
		if err := s.syncConversationSLA(ctx, q, workspaceID, id, msgs[id], ev); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) syncConversationSLA(ctx context.Context, q *store.Queries, ws, id uuid.UUID, msgs []slaMessage, ev *eventBatch) error {
	c, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: id})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	row, err := q.LockConversationSLA(ctx, store.LockConversationSLAParams{WorkspaceID: ws, ConversationID: id})
	exists := err == nil
	if err != nil && !store.IsNotFound(err) {
		return err
	}
	now := s.now()
	dropped := exists && row.DroppedAt != nil
	var policy store.SlaPolicy
	found := false
	key := ""
	if !c.Spam && c.TrashedAt == nil {
		labelMap, err := conversationLabels(ctx, q, ws, []uuid.UUID{id})
		if err != nil {
			return err
		}
		labels := labelMap[id]
		key = slaMatchKey(c, labels)
		if exists && !dropped && row.MatchKey == key && row.PolicyID != nil {
			p, err := q.GetSLAPolicy(ctx, store.GetSLAPolicyParams{WorkspaceID: ws, ID: *row.PolicyID})
			if err != nil && !store.IsNotFound(err) {
				return err
			}
			if err == nil && slaPolicyMatches(p, c, labels) {
				policy, found = p, true
			}
		}
		if !found {
			policies, err := q.ListSLAPolicies(ctx, ws)
			if err != nil {
				return err
			}
			for _, p := range policies {
				if slaPolicyMatches(p, c, labels) {
					policy, found = p, true
					break
				}
			}
		}
	}
	if !found {
		if !exists || dropped {
			return nil
		}
		row.DroppedAt, row.CheckAt = &now, nil
		if err := q.SaveConversationSLA(ctx, saveSLAParams(row, now)); err != nil {
			return err
		}
		if err := q.SetConversationSLADue(ctx, store.SetConversationSLADueParams{WorkspaceID: ws, ID: id}); err != nil {
			return err
		}
		return s.showSLA(ctx, q, ev, c, nil)
	}
	in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: c.InboxID})
	if err != nil {
		return err
	}
	before := row
	run := &slaRun{row: &row, policy: policy, clock: inboxClock(in), now: now}
	switch {
	case !exists:
		row = store.ConversationSla{WorkspaceID: ws, ConversationID: id}
		if err := s.buildSLA(ctx, q, run, c); err != nil {
			return err
		}
	case dropped:
		run.revive(*row.DroppedAt)
		if row.PolicyID == nil || *row.PolicyID != policy.ID {
			run.retarget(policy)
		}
		hist, err := q.ListSLAMessages(ctx, store.ListSLAMessagesParams{WorkspaceID: ws, ConversationID: id})
		if err != nil {
			return err
		}
		run.catchUp(hist)
	default:
		if row.PolicyID == nil || *row.PolicyID != policy.ID {
			run.retarget(policy)
		}
		for _, m := range msgs {
			if m.in {
				run.contactMessage(m.at)
			} else {
				run.reply(m.at)
			}
		}
	}
	row.PolicyID, row.MatchKey, row.WarnMinutes = &policy.ID, key, policy.WarnMinutes
	run.settle(c.Status)
	if exists && !slaShown(before, row) && before.MatchKey == row.MatchKey && sameTime(before.CheckAt, row.CheckAt) && before.AwaitingReply == row.AwaitingReply {
		return nil
	}
	if err := q.SaveConversationSLA(ctx, saveSLAParams(row, now)); err != nil {
		return err
	}
	if err := q.SetConversationSLADue(ctx, store.SetConversationSLADueParams{WorkspaceID: ws, ID: id, DueAt: slaDue(&row)}); err != nil {
		return err
	}
	for _, o := range run.outcomes {
		if _, err := s.recordSLAOutcome(ctx, q, c, &policy.ID, o); err != nil {
			return err
		}
	}
	queueSLAChecks(ev, before, row, exists && !dropped, now)
	if exists && !slaShown(before, row) {
		return nil
	}
	return s.showSLA(ctx, q, ev, c, slaBody(row, now))
}

// slaConversationsOf lists, sorted, the conversations the given policies drive, or every policy
// for which keep returns true.
func slaConversationsOf(ctx context.Context, q *store.Queries, ws uuid.UUID, keep func(store.SlaPolicy) bool) ([]uuid.UUID, error) {
	policies, err := q.ListSLAPolicies(ctx, ws)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, p := range policies {
		if !keep(p) {
			continue
		}
		got, err := q.ListSLAConversationsOfPolicy(ctx, store.ListSLAConversationsOfPolicyParams{WorkspaceID: ws, PolicyID: &p.ID})
		if err != nil {
			return nil, err
		}
		ids = append(ids, got...)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return a.Compare(b) })
	return ids, nil
}

// lockSLAConversations locks conversations in id order before a policy, label or inbox change
// that alters which policy drives them, so that change waits for their writers and never the
// other way round.
func lockSLAConversations(ctx context.Context, q *store.Queries, ws uuid.UUID, ids []uuid.UUID) error {
	for _, id := range ids {
		if _, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: ws, ID: id}); err != nil && !store.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// syncSLAConversations evaluates the SLA of conversations locked by lockSLAConversations again.
func (s *Server) syncSLAConversations(ctx context.Context, q *store.Queries, ev *eventBatch, ws uuid.UUID, ids []uuid.UUID) error {
	for _, id := range ids {
		if err := s.syncConversationSLA(ctx, q, ws, id, nil, ev); err != nil {
			return err
		}
	}
	return nil
}

// buildSLA starts a conversation's targets from its past: the resolution target at its creation
// (or its import) unless it was ever closed, the reply targets from its public messages. No
// target starts before the policy was created, so a new policy never breaches old conversations
// at once.
func (s *Server) buildSLA(ctx context.Context, q *store.Queries, run *slaRun, c store.Conversation) error {
	hist, err := q.ListSLAMessages(ctx, store.ListSLAMessagesParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if err != nil {
		return err
	}
	run.historical = true
	if c.ClosedAt == nil {
		start := c.CreatedAt
		if c.ImportedAt != nil {
			start = *c.ImportedAt
		}
		run.start(slaTargets(run.row)[2], start)
	}
	for _, m := range hist {
		if m.Direction != nil && *m.Direction == string(oas.In) {
			run.contactMessage(m.CreatedAt)
		} else {
			run.reply(m.CreatedAt)
		}
	}
	run.historical = false
	return nil
}

// recordSLAOutcome reports false when the target already has its outcome.
func (s *Server) recordSLAOutcome(ctx context.Context, q *store.Queries, c store.Conversation, policyID *uuid.UUID, o slaOutcome) (bool, error) {
	n, err := q.InsertSLAOutcome(ctx, store.InsertSLAOutcomeParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ConversationID: c.ID, InboxID: c.InboxID, PolicyID: policyID,
		Target: string(o.target), StartedAt: o.started, Outcome: o.outcome, At: o.at, AssigneeID: c.AssigneeID, TeamID: c.TeamID,
	})
	return n > 0, err
}

// showSLA puts sla on the batch's conversation events of c, or sends conversation.updated when
// the batch has none.
func (s *Server) showSLA(ctx context.Context, q *store.Queries, ev *eventBatch, c store.Conversation, body *oas.ConversationSla) error {
	shown := false
	for i, e := range ev.items {
		if e.conversationID == nil || *e.conversationID != c.ID || !slices.Contains(slaConversationEvents, e.typ) {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(e.data, &m) != nil {
			continue
		}
		if body == nil {
			delete(m, "sla")
		} else {
			m["sla"] = mustJSON(body)
		}
		ev.items[i].data = mustJSON(m)
		shown = true
	}
	if shown || c.TrashedAt != nil {
		return nil
	}
	out, err := oneConversation(ctx, q, c)
	if err != nil {
		return err
	}
	out.Sla = body
	ev.conversation(realtime.ConversationUpdated, c, out)
	return nil
}

func slaCheckOpts(at time.Time) *river.InsertOpts {
	return &river.InsertOpts{ScheduledAt: at, MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}}
}

// queueSLAChecks queues the breach check of every target whose due time was set, and its warning
// while the warning time is still ahead.
func queueSLAChecks(ev *eventBatch, before, after store.ConversationSla, existed bool, now time.Time) {
	tb, ta := slaTargets(&before), slaTargets(&after)
	warn := time.Duration(after.WarnMinutes) * time.Minute
	for i, t := range ta {
		if *t.due == nil || *t.breached != nil || (existed && sameTime(*tb[i].due, *t.due) && sameTime(*tb[i].warned, *t.warned)) {
			continue
		}
		due := **t.due
		ev.jobOnce(SLACheckArgs{WorkspaceID: after.WorkspaceID, ConversationID: after.ConversationID, Target: string(t.name), DueAt: due}, slaCheckOpts(due))
		if at := due.Add(-warn); warn > 0 && *t.warned == nil && at.After(now) {
			ev.jobOnce(SLACheckArgs{WorkspaceID: after.WorkspaceID, ConversationID: after.ConversationID, Target: string(t.name), DueAt: due, Warn: true}, slaCheckOpts(at))
		}
	}
}

type SLACheckArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	ConversationID uuid.UUID `json:"conversation_id"`
	Target         string    `json:"target"`
	DueAt          time.Time `json:"due_at"`
	Warn           bool      `json:"warn,omitempty"`
}

func (SLACheckArgs) Kind() string { return "sla_check" }

type slaCheckWorker struct {
	river.WorkerDefaults[SLACheckArgs]
	s *Server
}

func (w *slaCheckWorker) Work(ctx context.Context, job *river.Job[SLACheckArgs]) error {
	return w.s.CheckSLA(ctx, job.Args)
}

// CheckSLA warns of or breaches one target, only while it still runs to the due time the check
// was queued for (compare-and-set on its due_at): a check made stale by a pause, a reply or another
// policy does nothing, and each warning and breach happens once.
func (s *Server) CheckSLA(ctx context.Context, a SLACheckArgs) error {
	if live, err := s.workspaceLive(ctx, a.WorkspaceID); err != nil || !live {
		return err
	}
	name := oas.SlaTargetName(a.Target)
	if !name.Valid() {
		return nil
	}
	return s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.now()
		if !a.Warn && a.DueAt.After(now) {
			return nil
		}
		if err := s.syncConversationSLA(ctx, q, a.WorkspaceID, a.ConversationID, nil, ev); err != nil {
			return err
		}
		arg := store.MarkSLABreachedParams{WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, Target: a.Target, DueAt: a.DueAt.UTC(), Now: now}
		var row store.ConversationSla
		if a.Warn {
			row, err = q.MarkSLAWarned(ctx, store.MarkSLAWarnedParams(arg))
		} else {
			row, err = q.MarkSLABreached(ctx, arg)
		}
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		typ := eventSLAWarning
		if !a.Warn {
			typ = eventSLABreached
			var check *time.Time
			for _, t := range slaTargets(&row) {
				if *t.due != nil && *t.breached == nil {
					check = earliest(check, **t.due)
				}
			}
			row.CheckAt = check
			if err := q.SaveConversationSLA(ctx, saveSLAParams(row, now)); err != nil {
				return err
			}
			t, _ := slaTargetNamed(&row, name)
			first, err := s.recordSLAOutcome(ctx, q, c, row.PolicyID, slaOutcome{name, **t.started, "breached", now})
			if err != nil || !first {
				return err
			}
		}
		conv, err := oneConversation(ctx, q, c)
		if err != nil {
			return err
		}
		conv.Sla = slaBody(row, now)
		ev.conversation(typ, c, oas.ConversationSlaEventData{Target: name, DueAt: a.DueAt.UTC(), Conversation: conv})
		if !a.Warn {
			ev.conversation(realtime.ConversationUpdated, c, conv)
		}
		ev.job(SLANotifyArgs{WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, Target: a.Target, DueAt: a.DueAt.UTC(), Breached: !a.Warn}, nil)
		return s.queueSlackSLA(ctx, q, ev, c, SlackSLAArgs{WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, Target: a.Target, DueAt: a.DueAt.UTC(), At: now.UTC(), Breached: !a.Warn})
	})
}

type slaSweepWorker struct {
	river.WorkerDefaults[jobs.SLASweepArgs]
	s *Server
}

func (w *slaSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.SLASweepArgs]) error {
	return w.s.SweepSLA(ctx)
}

// SweepSLA breaches the targets whose check was lost, per workspace.
func (s *Server) SweepSLA(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := s.SweepSLAIn(ctx, ws); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) SweepSLAIn(ctx context.Context, ws uuid.UUID) error {
	now := s.now()
	rows, err := s.st.ListSLAChecksDue(ctx, store.ListSLAChecksDueParams{WorkspaceID: ws, Now: now, Lim: slaSweepBatch})
	if err != nil {
		return err
	}
	for _, r := range rows {
		for _, t := range slaTargets(&r) {
			if *t.due == nil || *t.breached != nil || (*t.due).After(now) {
				continue
			}
			if err := s.CheckSLA(ctx, SLACheckArgs{WorkspaceID: ws, ConversationID: r.ConversationID, Target: string(t.name), DueAt: **t.due}); err != nil {
				return err
			}
		}
	}
	return nil
}

type snoozeWakeWorker struct {
	river.WorkerDefaults[jobs.SnoozeWakeArgs]
	s *Server
}

func (w *snoozeWakeWorker) Work(ctx context.Context, _ *river.Job[jobs.SnoozeWakeArgs]) error {
	return w.s.WakeSnoozed(ctx)
}

// WakeSnoozed reopens the snoozed conversations whose snooze_until passed, per workspace, with
// conversation.updated and a status_changed event each; their SLA clocks resume.
func (s *Server) WakeSnoozed(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := s.WakeSnoozedIn(ctx, ws); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) WakeSnoozedIn(ctx context.Context, ws uuid.UUID) error {
	for {
		n := 0
		err := s.inTx(ctx, ws, func(q *store.Queries, ev *eventBatch) error {
			now := s.now()
			due, err := q.ClaimDueSnoozed(ctx, store.ClaimDueSnoozedParams{WorkspaceID: ws, Now: now, Lim: snoozeWakeSize})
			if err != nil {
				return err
			}
			n = len(due)
			for _, d := range due {
				from := len(ev.items)
				if _, err := s.reopenConversation(ctx, q, ev, ws, d.ID, now); err != nil {
					return err
				}
				if d.SnoozeRuleID != nil {
					for i := from; i < len(ev.items); i++ {
						ev.items[i].noRules = true
					}
				}
			}
			return nil
		})
		if err != nil || n < snoozeWakeSize {
			return err
		}
	}
}

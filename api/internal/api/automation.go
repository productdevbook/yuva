package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/oapi-codegen/nullable"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	automationRunsKept   = 30 * 24 * time.Hour
	automationRunsBatch  = 5000
	automationIdleWindow = 15 * time.Minute
	automationIdleBatch  = 2000
	ruleReplyEvery       = time.Hour
	eventAutomation      = "automation.triggered"
)

var limitAutomationActions = limit{600, time.Minute}

var (
	errRuleSkip         = errors.New("automation rule does not apply")
	errRuleUncharged    = errors.New("automation rule matched before the rate limit counted it")
	errRuleLabelGone    = problem(http.StatusConflict, "label_gone", "a label the rule adds or removes was deleted")
	errRuleCannedGone   = problem(http.StatusConflict, "canned_reply_gone", "the rule's canned reply was deleted")
	errRuleWebhookGone  = problem(http.StatusConflict, "webhook_unavailable", "the rule's webhook endpoint was deleted, is turned off or is limited to another inbox")
	errRuleBotsMaySend  = problem(http.StatusForbidden, "bot_sending_disabled", "the rule was last saved by an API key or a connected app, and the workspace does not let bots deliver messages")
	errRuleReplyAuto    = problem(http.StatusConflict, "auto_reply", "the contact's message is an automatic reply or a bounce")
	errRuleReplyDMARC   = problem(http.StatusConflict, "auto_reply", "the contact's e-mail did not pass DMARC")
	errRuleReplyBlocked = problem(http.StatusConflict, "auto_reply", "the contact is blocked")
	errRuleReplySpam    = problem(http.StatusConflict, "auto_reply", "the conversation is spam")
	errRuleReplyTooSoon = problem(http.StatusConflict, "auto_reply", "a rule already replied in this conversation within the hour")
	errRuleReplyTooMany = problem(http.StatusConflict, "auto_reply", "rules already replied three times in this conversation within a day")
	errRuleCannedBot    = problem(http.StatusForbidden, "bot_sending_disabled", "the canned reply was last saved by an API key or a connected app, or before this was recorded, and the workspace does not let bots deliver messages")
)

const ruleRepliesPerDay = 3

// AutomationRunArgs runs the rules of one trigger on one conversation, in order.
type AutomationRunArgs struct {
	WorkspaceID    uuid.UUID  `json:"workspace_id"`
	ConversationID uuid.UUID  `json:"conversation_id"`
	Trigger        string     `json:"trigger"`
	EventID        int64      `json:"event_id,omitempty"`
	MessageID      *uuid.UUID `json:"message_id,omitempty"`
	IdleSince      *time.Time `json:"idle_since,omitempty"`
	IdleMinutes    int32      `json:"idle_minutes,omitempty"`
	// Assign queues automatic assignment once the rules ran; Reopen as AutoAssignArgs.Reopen.
	Assign bool `json:"assign,omitempty"`
	Reopen bool `json:"reopen,omitempty"`
}

func (AutomationRunArgs) Kind() string { return "automation_run" }

func (AutomationRunArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// triggerKey names the trigger a rule runs for at most once: the event, or one quiet period.
func (a AutomationRunArgs) triggerKey() string {
	if a.IdleSince != nil {
		return "idle:" + a.ConversationID.String() + ":" + a.IdleSince.UTC().Format(time.RFC3339Nano)
	}
	return "event:" + strconv.FormatInt(a.EventID, 10)
}

func (p principal) ruleJSON() []byte {
	if p.rule == nil {
		return nil
	}
	return mustJSON(p.rule)
}

// automationTriggerOf tells which trigger an event is, and the contact's message for
// message_received.
func automationTriggerOf(e pendingEvent) (oas.AutomationTrigger, *uuid.UUID, bool) {
	if e.conversationID == nil {
		return "", nil, false
	}
	switch e.typ {
	case realtime.ConversationCreated:
		var c struct {
			Imported *struct{} `json:"imported"`
		}
		if json.Unmarshal(e.data, &c) != nil || c.Imported != nil {
			return "", nil, false
		}
		return oas.AutomationTriggerConversationCreated, nil, true
	case realtime.MessageCreated:
		var m struct {
			ID         uuid.UUID       `json:"id"`
			Kind       oas.MessageKind `json:"kind"`
			Direction  *oas.Direction  `json:"direction"`
			Draft      bool            `json:"draft"`
			ImportedAt *time.Time      `json:"imported_at"`
			Event      *struct {
				Type oas.EventType `json:"type"`
			} `json:"event"`
		}
		if json.Unmarshal(e.data, &m) != nil || m.Draft || m.ImportedAt != nil {
			return "", nil, false
		}
		if m.Kind == oas.MessageKindMessage && m.Direction != nil && *m.Direction == oas.In {
			return oas.AutomationTriggerMessageReceived, &m.ID, true
		}
		if m.Kind == oas.MessageKindEvent && m.Event != nil && m.Event.Type == oas.StatusChanged {
			return oas.AutomationTriggerStatusChanged, nil, true
		}
	}
	return "", nil, false
}

// queueAutomations runs in the transaction that wrote the events, with their ids: a run job exists
// exactly when its trigger committed. Events a rule wrote queue nothing, so rules never loop.
// Automatic assignment of a conversation that enters a pool follows its rules: one of their jobs
// queues it when done, or it is queued here when no rule runs.
func (s *Server) queueAutomations(ctx context.Context, tx pgx.Tx, q *store.Queries, workspaceID uuid.UUID, ev *eventBatch, ids []int64) error {
	if ev.rule != nil {
		return nil
	}
	var (
		triggers []string
		loaded   bool
		runs     []AutomationRunArgs
		queued   []river.InsertManyParams
	)
	for i, e := range ev.items {
		trigger, msg, ok := automationTriggerOf(e)
		if !ok || e.noRules {
			continue
		}
		if !loaded {
			var err error
			if triggers, err = q.ListEnabledAutomationTriggers(ctx, workspaceID); err != nil {
				return err
			}
			loaded = true
		}
		if !slices.Contains(triggers, string(trigger)) {
			continue
		}
		runs = append(runs, AutomationRunArgs{
			WorkspaceID: workspaceID, ConversationID: *e.conversationID, Trigger: string(trigger), EventID: ids[i], MessageID: msg,
		})
	}
	if pools := assignTriggers(ev.items); len(pools) > 0 {
		enabled, err := q.AssignmentEnabled(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, t := range pools {
			if !enabled {
				break
			}
			if i := assignAfter(runs, t.conversation); i >= 0 {
				runs[i].Assign, runs[i].Reopen = true, t.reopen
				continue
			}
			queued = append(queued, river.InsertManyParams{Args: AutoAssignArgs{WorkspaceID: workspaceID, ConversationID: t.conversation, Reopen: t.reopen}})
		}
	}
	for _, r := range runs {
		queued = append(queued, river.InsertManyParams{Args: r})
	}
	if len(queued) == 0 {
		return nil
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, queued)
	return err
}

// assignAfter picks the run job of the conversation that queues its assignment: conversation_created
// first, then message_received, then status_changed; -1 when none runs.
func assignAfter(runs []AutomationRunArgs, conversation uuid.UUID) int {
	for _, trigger := range []oas.AutomationTrigger{oas.AutomationTriggerConversationCreated, oas.AutomationTriggerMessageReceived, oas.AutomationTriggerStatusChanged} {
		for i, r := range runs {
			if r.ConversationID == conversation && r.Trigger == string(trigger) {
				return i
			}
		}
	}
	return -1
}

type automationRunWorker struct {
	river.WorkerDefaults[AutomationRunArgs]
	s *Server
}

func (w *automationRunWorker) Work(ctx context.Context, job *river.Job[AutomationRunArgs]) error {
	return w.s.RunAutomations(ctx, job.Args)
}

// RunAutomations applies the enabled rules of the trigger to the conversation in order, each in
// its own transaction, at most once per rule and trigger, then queues automatic assignment when the
// trigger or a rule's assign_pool asks for it. The rate limit is checked before any transaction
// opens.
func (s *Server) RunAutomations(ctx context.Context, a AutomationRunArgs) error {
	if err := s.runAutomationRules(ctx, a); err != nil {
		var snooze *rivertype.JobSnoozeError
		if a.Assign && errors.As(err, &snooze) {
			if err := s.holdForRules(ctx, a); err != nil {
				return err
			}
		}
		return err
	}
	return s.assignAfterRules(ctx, a)
}

// holdForRules shows a conversation whose rules wait for the rate limit as waiting, and keeps the
// waiting runs from assigning it before its rules ran.
func (s *Server) holdForRules(ctx context.Context, a AutomationRunArgs) error {
	return s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, ev *eventBatch) error {
		ev.assign = true
		c, err := q.HoldForRules(ctx, store.HoldForRulesParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID, Now: s.now()})
		if store.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		body, err := oneConversation(ctx, q, c)
		if err != nil {
			return err
		}
		ev.conversation(realtime.ConversationUpdated, c, body)
		return nil
	})
}

// assignAfterRules reads what the trigger's applied runs did, so a retry decides the same: a rule
// that assigned or unassigned a member keeps its choice over a reopen's.
func (s *Server) assignAfterRules(ctx context.Context, a AutomationRunArgs) error {
	rows, err := s.st.ListAppliedRunActions(ctx, store.ListAppliedRunActionsParams{
		WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, TriggerKey: a.triggerKey(),
	})
	if err != nil {
		return err
	}
	pooled, reopen := false, a.Reopen
	for _, raw := range rows {
		var actions []oas.AutomationRunAction
		if json.Unmarshal(raw, &actions) != nil {
			continue
		}
		for _, act := range actions {
			if act.Result != oas.AutomationResultApplied {
				continue
			}
			switch act.Type {
			case oas.AutomationActionAssignPool:
				pooled, reopen = true, false
			case oas.AutomationActionAssignMember, oas.AutomationActionUnassign:
				reopen = false
			}
		}
	}
	if !a.Assign && !pooled {
		return nil
	}
	return s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if err := q.ReleaseForRules(ctx, store.ReleaseForRulesParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID}); err != nil {
			return err
		}
		_, err := s.jobs.InsertTx(ctx, tx, AutoAssignArgs{WorkspaceID: a.WorkspaceID, ConversationID: a.ConversationID, Reopen: reopen}, nil)
		return err
	})
}

func (s *Server) runAutomationRules(ctx context.Context, a AutomationRunArgs) error {
	w, err := s.st.GetWorkspace(ctx, a.WorkspaceID)
	if store.IsNotFound(err) || (err == nil && w.DeletedAt != nil) {
		return nil
	}
	if err != nil {
		return err
	}
	arg := store.ListEnabledAutomationRulesParams{WorkspaceID: a.WorkspaceID, Trigger: a.Trigger}
	if a.Trigger == string(oas.AutomationTriggerIdle) {
		arg.IdleMinutes = &a.IdleMinutes
	}
	rules, err := s.st.ListEnabledAutomationRules(ctx, arg)
	if err != nil || len(rules) == 0 {
		return err
	}
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID})
	if store.IsNotFound(err) || (err == nil && c.TrashedAt != nil) {
		return nil
	}
	if err != nil {
		return err
	}
	key := a.triggerKey()
	for _, r := range rules {
		if r.InboxID != nil && *r.InboxID != c.InboxID {
			continue
		}
		done, err := s.st.AutomationRunExists(ctx, store.AutomationRunExistsParams{WorkspaceID: a.WorkspaceID, RuleID: r.ID, TriggerKey: key})
		if err != nil {
			return err
		}
		if done {
			continue
		}
		charge := func() error {
			if ok, wait := s.limits.takeAll(ctx, "automation_actions:"+a.WorkspaceID.String(), limitAutomationActions, len(ruleActions(r))); !ok {
				return river.JobSnooze(wait)
			}
			return nil
		}
		match, err := s.ruleMatchesNow(ctx, a, r)
		if err != nil {
			return err
		}
		if match {
			if err := charge(); err != nil {
				return err
			}
		}
		err = s.runRule(ctx, w, a, r.ID, match)
		if errors.Is(err, errRuleUncharged) {
			if err := charge(); err != nil {
				return err
			}
			err = s.runRule(ctx, w, a, r.ID, true)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ruleMatchesNow evaluates a rule's conditions without a lock, to charge the rate limit only for
// rules that will apply; the rule's transaction decides again under the conversation's lock.
func (s *Server) ruleMatchesNow(ctx context.Context, a AutomationRunArgs, r store.AutomationRule) (bool, error) {
	c, err := s.st.GetConversation(ctx, store.GetConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.ruleMatches(ctx, s.st.Queries, c, a, ruleConditions(r), s.now())
}

// runRule applies one rule in one transaction. charged says the rate limit already counted its
// actions; a rule that matches uncharged rolls back with errRuleUncharged to be charged and run again.
func (s *Server) runRule(ctx context.Context, w store.Workspace, a AutomationRunArgs, ruleID uuid.UUID, charged bool) error {
	var (
		rule    store.AutomationRule
		planned []oas.AutomationAction
	)
	err := s.inTx(ctx, a.WorkspaceID, func(q *store.Queries, ev *eventBatch) error {
		r, err := q.ShareLockAutomationRule(ctx, store.ShareLockAutomationRuleParams{WorkspaceID: a.WorkspaceID, ID: ruleID})
		if store.IsNotFound(err) {
			return errRuleSkip
		}
		if err != nil {
			return err
		}
		if !r.Enabled || r.Trigger != a.Trigger || (a.IdleSince != nil && (r.IdleMinutes == nil || *r.IdleMinutes != a.IdleMinutes)) {
			return errRuleSkip
		}
		rule, planned = r, ruleActions(r)
		ev.rule = &oas.AutomationRuleRef{Id: r.ID, Name: r.Name}
		var teams []uuid.UUID
		for _, act := range planned {
			if act.TeamId != nil {
				teams = append(teams, *act.TeamId)
			}
		}
		if len(teams) > 0 {
			if _, err := q.ShareLockTeams(ctx, store.ShareLockTeamsParams{WorkspaceID: a.WorkspaceID, Ids: teams}); err != nil {
				return err
			}
		}
		c, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID})
		if store.IsNotFound(err) {
			return errRuleSkip
		}
		if err != nil {
			return err
		}
		if c.TrashedAt != nil || (r.InboxID != nil && *r.InboxID != c.InboxID) {
			return errRuleSkip
		}
		if a.IdleSince != nil && (!c.LastActivityAt.Equal(*a.IdleSince) || c.Spam ||
			(c.Status != string(oas.ConversationStatusOpen) && c.Status != string(oas.ConversationStatusPending))) {
			return errRuleSkip
		}
		now := s.now()
		runID := newID()
		if _, err := q.InsertAutomationRun(ctx, store.InsertAutomationRunParams{
			ID: runID, WorkspaceID: a.WorkspaceID, RuleID: r.ID, ConversationID: c.ID, Trigger: a.Trigger, TriggerKey: a.triggerKey(),
			Outcome: string(oas.AutomationRunApplied), Actions: []byte("[]"), Now: now,
		}); store.IsNotFound(err) {
			return errRuleSkip
		} else if err != nil {
			return err
		}
		match, err := s.ruleMatches(ctx, q, c, a, ruleConditions(r), now)
		if err != nil {
			return err
		}
		if !match {
			return q.SetAutomationRunOutcome(ctx, store.SetAutomationRunOutcomeParams{WorkspaceID: a.WorkspaceID, ID: runID, Outcome: string(oas.AutomationRunNotMatched)})
		}
		if !charged {
			return errRuleUncharged
		}
		p := principal{workspaceID: a.WorkspaceID, rule: ev.rule, botsMaySend: w.BotsMaySend}
		results, err := s.applyRule(ctx, q, ev, p, w, r, c, a, planned, now)
		if err != nil {
			return err
		}
		if err := q.SetAutomationRunActions(ctx, store.SetAutomationRunActionsParams{WorkspaceID: a.WorkspaceID, ID: runID, Actions: mustJSON(results)}); err != nil {
			return err
		}
		return s.writeEvent(ctx, q, ev, p, c, oas.MessageEvent{Type: oas.RuleApplied, Actions: &results}, now)
	})
	if err == nil || errors.Is(err, errRuleSkip) {
		return nil
	}
	if errors.Is(err, errRuleUncharged) {
		return err
	}
	var ae *apiError
	if !errors.As(err, &ae) {
		return err
	}
	return s.recordFailedRun(ctx, a, rule, planned, ae)
}

// recordFailedRun takes the conversation's lock first, as the rule's transaction does, so it never
// waits on a run row while holding what that transaction waits for.
func (s *Server) recordFailedRun(ctx context.Context, a AutomationRunArgs, r store.AutomationRule, planned []oas.AutomationAction, ae *apiError) error {
	results := make([]oas.AutomationRunAction, len(planned))
	for i, act := range planned {
		results[i] = oas.AutomationRunAction{Type: act.Type, Result: oas.AutomationResultNotRun}
	}
	code, detail := ae.Code, ae.Detail
	if ae.Field != "" && !strings.Contains(detail, ae.Field) {
		detail += " (" + ae.Field + ")"
	}
	return s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.LockConversation(ctx, store.LockConversationParams{WorkspaceID: a.WorkspaceID, ID: a.ConversationID}); store.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
		_, err := q.InsertAutomationRun(ctx, store.InsertAutomationRunParams{
			ID: newID(), WorkspaceID: a.WorkspaceID, RuleID: r.ID, ConversationID: a.ConversationID, Trigger: a.Trigger, TriggerKey: a.triggerKey(),
			Outcome: string(oas.AutomationRunFailed), Error: &code, Detail: &detail, Actions: mustJSON(results), Now: s.now(),
		})
		if store.IsNotFound(err) || store.IsForeignKeyViolation(err) {
			return nil
		}
		return err
	})
}

// ruleState reads, once each, what the conditions of a rule look at beyond the conversation row.
type ruleState struct {
	q      *store.Queries
	c      store.Conversation
	a      AutomationRunArgs
	labels []uuid.UUID
	attrs  map[string]any
	fields map[string]any
	body   *string
	inbox  *store.Inbox
	loaded map[oas.AutomationConditionField]bool
}

func (st *ruleState) load(ctx context.Context, f oas.AutomationConditionField) error {
	if st.loaded[f] {
		return nil
	}
	st.loaded[f] = true
	ws, q := st.c.WorkspaceID, st.q
	switch f {
	case oas.AutomationFieldLabel:
		m, err := conversationLabels(ctx, q, ws, []uuid.UUID{st.c.ID})
		st.labels = m[st.c.ID]
		return err
	case oas.AutomationFieldContactAttribute:
		ct, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: ws, ID: st.c.ContactID})
		if err != nil {
			return err
		}
		_ = json.Unmarshal(ct.Attributes, &st.attrs)
	case oas.AutomationFieldCustomField:
		_ = json.Unmarshal(st.c.Fields, &st.fields)
	case oas.AutomationFieldBody:
		m, err := triggeringMessage(ctx, q, st.c, st.a)
		if err != nil {
			return err
		}
		st.body = &m.Body
	case oas.AutomationFieldBusinessHours:
		in, err := q.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: st.c.InboxID})
		if err != nil {
			return err
		}
		st.inbox = &in
	}
	return nil
}

// triggeringMessage is the contact's message a rule answers: the one that triggered it, else the
// contact's latest. An empty row when the contact wrote nothing.
func triggeringMessage(ctx context.Context, q *store.Queries, c store.Conversation, a AutomationRunArgs) (store.LatestContactMessageRow, error) {
	if a.MessageID != nil {
		m, err := q.ContactMessageOf(ctx, store.ContactMessageOfParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID, ID: *a.MessageID})
		if err == nil || !store.IsNotFound(err) {
			return store.LatestContactMessageRow(m), err
		}
	}
	m, err := q.LatestContactMessage(ctx, store.LatestContactMessageParams{WorkspaceID: c.WorkspaceID, ConversationID: c.ID})
	if store.IsNotFound(err) {
		return m, nil
	}
	return m, err
}

// valueText is how a contact attribute or custom field value compares: strings as they are,
// anything else as its JSON text; absent, null and "" are not set.
func valueText(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	if s, ok := v.(string); ok {
		return s, s != ""
	}
	b, _ := json.Marshal(v)
	return string(b), true
}

func matchText(op oas.AutomationConditionOperator, got string, set bool, want *string) bool {
	switch op {
	case oas.AutomationOpIsSet:
		return set
	case oas.AutomationOpIsNotSet:
		return !set
	case oas.AutomationOpEquals:
		return set && got == *want
	case oas.AutomationOpNotEquals:
		return !set || got != *want
	case oas.AutomationOpContains:
		return set && strings.Contains(strings.ToLower(got), strings.ToLower(*want))
	case oas.AutomationOpNotContains:
		return !set || !strings.Contains(strings.ToLower(got), strings.ToLower(*want))
	}
	return false
}

func matchSet(op oas.AutomationConditionOperator, got *string, values *[]string) bool {
	in := got != nil && values != nil && slices.Contains(*values, *got)
	if op == oas.AutomationOpNotIn {
		return !in
	}
	return in
}

// ruleMatches evaluates the conditions on the conversation as it is now, under its lock, so a
// later rule sees what an earlier one changed.
func (s *Server) ruleMatches(ctx context.Context, q *store.Queries, c store.Conversation, a AutomationRunArgs, conds []oas.AutomationCondition, now time.Time) (bool, error) {
	st := &ruleState{q: q, c: c, a: a, loaded: map[oas.AutomationConditionField]bool{}}
	for _, cond := range conds {
		if err := st.load(ctx, cond.Field); err != nil {
			return false, err
		}
		var ok bool
		switch cond.Field {
		case oas.AutomationFieldChannelKind:
			ok = matchSet(cond.Operator, c.ChannelKind, cond.Values)
		case oas.AutomationFieldKind:
			ok = matchSet(cond.Operator, &c.Kind, cond.Values)
		case oas.AutomationFieldFeedbackCategory:
			var fb struct {
				Category string `json:"category"`
			}
			var got *string
			if json.Unmarshal(c.Feedback, &fb) == nil && fb.Category != "" {
				got = &fb.Category
			}
			ok = matchSet(cond.Operator, got, cond.Values)
		case oas.AutomationFieldPriority:
			ok = matchSet(cond.Operator, &c.Priority, cond.Values)
		case oas.AutomationFieldStatus:
			ok = matchSet(cond.Operator, &c.Status, cond.Values)
		case oas.AutomationFieldSubject:
			ok = matchText(cond.Operator, c.Subject, c.Subject != "", cond.Value)
		case oas.AutomationFieldBody:
			ok = matchText(cond.Operator, *st.body, *st.body != "", cond.Value)
		case oas.AutomationFieldContactAttribute:
			got, set := valueText(st.attrs, *cond.Key)
			ok = matchText(cond.Operator, got, set, cond.Value)
		case oas.AutomationFieldCustomField:
			got, set := valueText(st.fields, *cond.Key)
			ok = matchText(cond.Operator, got, set, cond.Value)
		case oas.AutomationFieldLabel:
			has := slices.ContainsFunc(st.labels, func(l uuid.UUID) bool { return slices.Contains(*cond.Values, l.String()) })
			ok = has == (cond.Operator == oas.AutomationOpIn)
		case oas.AutomationFieldTeam:
			switch cond.Operator {
			case oas.AutomationOpIsSet:
				ok = c.TeamID != nil
			case oas.AutomationOpIsNotSet:
				ok = c.TeamID == nil
			default:
				var got *string
				if c.TeamID != nil {
					got = new(c.TeamID.String())
				}
				ok = matchSet(cond.Operator, got, cond.Values)
			}
		case oas.AutomationFieldAssignee:
			ok = (c.AssigneeID != nil) == (cond.Operator == oas.AutomationOpIsSet)
		case oas.AutomationFieldBusinessHours:
			ok = inboxOpen(*st.inbox, now) == (*cond.Value == "inside")
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// applyRule applies every action of the rule through the code /v1 uses, or fails as a whole:
// notes and the reply first, then one conversation change, then the webhook with the result.
func (s *Server) applyRule(ctx context.Context, q *store.Queries, ev *eventBatch, p principal, w store.Workspace, r store.AutomationRule,
	c store.Conversation, a AutomationRunArgs, actions []oas.AutomationAction, now time.Time) ([]oas.AutomationRunAction, error) {
	results := make([]oas.AutomationRunAction, len(actions))
	var (
		upd           oas.ConversationUpdate
		add, remove   []uuid.UUID
		change        bool
		note          *string
		reply, hookID *uuid.UUID
	)
	for i, act := range actions {
		results[i] = oas.AutomationRunAction{Type: act.Type, Result: oas.AutomationResultApplied}
		switch act.Type {
		case oas.AutomationActionAddLabels, oas.AutomationActionRemoveLabels:
			found, err := q.LockLabels(ctx, store.LockLabelsParams{WorkspaceID: c.WorkspaceID, Ids: *act.LabelIds})
			if err != nil {
				return nil, err
			}
			if len(found) != len(*act.LabelIds) {
				return nil, errRuleLabelGone
			}
			if act.Type == oas.AutomationActionAddLabels {
				add = *act.LabelIds
			} else {
				remove = *act.LabelIds
			}
			change = true
		case oas.AutomationActionAssignMember:
			upd.AssigneeId, change = nullable.NewNullableWithValue(*act.MemberId), true
		case oas.AutomationActionUnassign:
			upd.AssigneeId, change = nullable.NewNullNullable[uuid.UUID](), true
		case oas.AutomationActionAssignTeam:
			upd.TeamId, change = nullable.NewNullableWithValue(*act.TeamId), true
		case oas.AutomationActionAssignPool:
			upd.AssigneeId, change = nullable.NewNullNullable[uuid.UUID](), true
		case oas.AutomationActionSetPriority:
			upd.Priority, change = act.Priority, true
		case oas.AutomationActionSetStatus:
			upd.Status, change = act.Status, true
		case oas.AutomationActionSnooze:
			until := now.Add(time.Duration(*act.SnoozeMinutes) * time.Minute)
			upd.Status, upd.SnoozeUntil, change = new(oas.ConversationStatusSnoozed), &until, true
		case oas.AutomationActionAddNote:
			note = act.Body
		case oas.AutomationActionSendReply:
			reply = act.CannedReplyId
		case oas.AutomationActionSendWebhook:
			hookID = act.WebhookId
		}
	}
	if note != nil {
		if err := s.ruleMessage(ctx, q, ev, p, c, string(oas.MessageKindNote), *note, now); err != nil {
			return nil, err
		}
	}
	if reply != nil {
		cr, err := q.GetCannedReply(ctx, store.GetCannedReplyParams{WorkspaceID: c.WorkspaceID, ID: *reply})
		if store.IsNotFound(err) {
			return nil, errRuleCannedGone
		}
		if err != nil {
			return nil, err
		}
		if err := s.ruleReplyAllowed(ctx, q, w, r, cr, c, a, now); err != nil {
			return nil, err
		}
		if err := s.ruleMessage(ctx, q, ev, p, c, string(oas.MessageKindMessage), cr.Body, now); err != nil {
			return nil, err
		}
	}
	if change {
		before := c
		labels, err := conversationLabels(ctx, q, c.WorkspaceID, []uuid.UUID{c.ID})
		if err != nil {
			return nil, err
		}
		if _, err := s.changeConversation(ctx, q, ev, p, c.ID, upd, add, remove); err != nil {
			return nil, err
		}
		for i, act := range actions {
			if unchangedBy(act, before, labels[c.ID]) {
				results[i].Result = oas.AutomationResultUnchanged
			}
		}
	}
	if hookID != nil {
		if err := s.ruleWebhook(ctx, q, ev, c, *hookID, a.Trigger, now); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func unchangedBy(act oas.AutomationAction, before store.Conversation, labels []uuid.UUID) bool {
	switch act.Type {
	case oas.AutomationActionAddLabels:
		return !slices.ContainsFunc(*act.LabelIds, func(l uuid.UUID) bool { return !slices.Contains(labels, l) })
	case oas.AutomationActionRemoveLabels:
		return !slices.ContainsFunc(*act.LabelIds, func(l uuid.UUID) bool { return slices.Contains(labels, l) })
	case oas.AutomationActionAssignMember:
		return sameID(before.AssigneeID, act.MemberId)
	case oas.AutomationActionUnassign:
		return before.AssigneeID == nil
	case oas.AutomationActionAssignTeam:
		return sameID(before.TeamID, act.TeamId)
	case oas.AutomationActionSetPriority:
		return before.Priority == string(*act.Priority)
	case oas.AutomationActionSetStatus:
		return before.Status == string(*act.Status)
	}
	return false
}

// ruleReplyAllowed keeps rule replies away from automatic mail, e-mail that did not pass DMARC,
// blocked contacts and spam, at
// most one an hour and three a day per conversation, and, while the workspace keeps bots from
// sending, sends only text an owner or admin approved: both the rule and the canned reply last saved
// by one. The conversation's lock orders every rule reply of the conversation.
func (s *Server) ruleReplyAllowed(ctx context.Context, q *store.Queries, w store.Workspace, r store.AutomationRule, cr store.CannedReply, c store.Conversation, a AutomationRunArgs, now time.Time) error {
	if c.Spam {
		return errRuleReplySpam
	}
	ct, err := q.GetContact(ctx, store.GetContactParams{WorkspaceID: c.WorkspaceID, ID: c.ContactID})
	if err != nil {
		return err
	}
	if ct.Blocked {
		return errRuleReplyBlocked
	}
	m, err := triggeringMessage(ctx, q, c, a)
	if err != nil {
		return err
	}
	if m.Auto {
		return errRuleReplyAuto
	}
	if m.Dmarc != "" && m.Dmarc != email.DMARCPass {
		return errRuleReplyDMARC
	}
	n, err := q.CountRuleReplies(ctx, store.CountRuleRepliesParams{
		WorkspaceID: c.WorkspaceID, ConversationID: c.ID, HourSince: now.Add(-ruleReplyEvery), DaySince: now.Add(-24 * time.Hour),
	})
	if err != nil {
		return err
	}
	if n.Hour > 0 {
		return errRuleReplyTooSoon
	}
	if n.Day >= ruleRepliesPerDay {
		return errRuleReplyTooMany
	}
	if w.BotsMaySend {
		return nil
	}
	if ok, err := savedByManager(ctx, q, c.WorkspaceID, &r.SavedByKind, r.SavedByMemberID); err != nil || !ok {
		if err == nil {
			err = errRuleBotsMaySend
		}
		return err
	}
	if ok, err := savedByManager(ctx, q, c.WorkspaceID, cr.SavedByKind, cr.SavedByMemberID); err != nil || !ok {
		if err == nil {
			err = errRuleCannedBot
		}
		return err
	}
	return nil
}

// savedByManager reports a member who is an owner or admin now; keys, connected apps, removed
// members and unknown authors are not.
func savedByManager(ctx context.Context, q *store.Queries, ws uuid.UUID, kind *string, member *uuid.UUID) (bool, error) {
	if kind == nil || *kind != string(oas.AutomationAuthorMember) || member == nil {
		return false, nil
	}
	m, err := q.GetMember(ctx, store.GetMemberParams{WorkspaceID: ws, ID: *member})
	if store.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return m.Role == roleOwner || m.Role == roleAdmin, nil
}

// ruleMessage writes a rule's note or reply as /v1 would, by the system and naming the rule. It
// does not move the conversation's last activity, so a rule never starts a new quiet period.
func (s *Server) ruleMessage(ctx context.Context, q *store.Queries, ev *eventBatch, p principal, c store.Conversation, kind, body string, now time.Time) error {
	in := &messageInput{kind: kind, body: body}
	if err := s.validateMessage(p, in); err != nil {
		return err
	}
	deliver := kind == string(oas.MessageKindMessage)
	var plan *emailPlan
	if deliver {
		var err error
		if plan, err = s.planEmail(ctx, q, c); err != nil {
			return err
		}
		if plan != nil {
			plan.auto = true
		}
	}
	arg := store.CreateMessageParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ConversationID: c.ID, Kind: kind, Body: in.body, CreatedAt: now, Rule: p.ruleJSON(),
	}
	arg.AuthorType, arg.AuthorMemberID, arg.AuthorApiKeyID = authorFor(p)
	if deliver {
		arg.Direction = &in.direction
	}
	if plan != nil {
		queued := deliveryQueued
		arg.DeliveryState = &queued
	}
	msg, err := q.CreateMessage(ctx, arg)
	if err != nil {
		return err
	}
	var summary *store.ListMessageEmailsRow
	if deliver {
		if summary, err = s.dispatchOutgoing(ctx, q, ev, p, c, plan, msg, now); err != nil {
			return err
		}
	}
	if err := q.TouchConversationByRule(ctx, store.TouchConversationByRuleParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: now, IsMessage: deliver}); err != nil {
		return err
	}
	if err := s.addUsage(ctx, q, c.WorkspaceID, 0, 1, 0); err != nil {
		return err
	}
	ev.conversation(realtime.MessageCreated, c, withEmail(messageBody(msg, nil), summary))
	return nil
}

// ruleWebhook sends automation.triggered to the endpoint through the usual delivery jobs, with the
// conversation as the rule left it.
func (s *Server) ruleWebhook(ctx context.Context, q *store.Queries, ev *eventBatch, c store.Conversation, endpointID uuid.UUID, trigger string, now time.Time) error {
	ws := c.WorkspaceID
	e, err := q.ShareLockWebhookEndpoint(ctx, store.ShareLockWebhookEndpointParams{WorkspaceID: ws, ID: endpointID})
	if store.IsNotFound(err) {
		return errRuleWebhookGone
	}
	if err != nil {
		return err
	}
	cur, err := q.GetConversation(ctx, store.GetConversationParams{WorkspaceID: ws, ID: c.ID})
	if err != nil {
		return err
	}
	if !e.Enabled || (e.InboxID != nil && *e.InboxID != cur.InboxID) {
		return errRuleWebhookGone
	}
	conv, err := oneConversation(ctx, q, cur)
	if err != nil {
		return err
	}
	if err := s.fillSLA(ctx, q, ws, &conv); err != nil {
		return err
	}
	contact, err := s.webhookContact(ctx, q, ws, cur.ContactID)
	if err != nil {
		return err
	}
	if contact == nil {
		return errContactGone
	}
	inbox := cur.InboxID
	body := mustJSON(webhookPayload{Type: eventAutomation, Timestamp: now.UTC(), WorkspaceID: ws, InboxID: &inbox, Data: oas.WebhookAutomationTriggeredData{
		Rule: *ev.rule, Trigger: oas.AutomationTrigger(trigger), Conversation: conv, Contact: *contact,
	}})
	id := newID()
	d, err := q.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{
		ID: id, WorkspaceID: ws, EndpointID: e.ID, MessageID: "msg_" + strings.ReplaceAll(id.String(), "-", ""),
		EventType: eventAutomation, Payload: string(body), Now: now,
	})
	if err != nil {
		return err
	}
	ev.job(WebhookDeliveryArgs{WorkspaceID: ws, DeliveryID: d.ID}, webhookDeliveryOpts())
	return nil
}

type automationIdleSweepWorker struct {
	river.WorkerDefaults[jobs.AutomationIdleSweepArgs]
	s *Server
}

func (w *automationIdleSweepWorker) Work(ctx context.Context, _ *river.Job[jobs.AutomationIdleSweepArgs]) error {
	return w.s.SweepIdleAutomations(ctx)
}

// SweepIdleAutomations queues the idle rules of the conversations that went quiet, per workspace.
func (s *Server) SweepIdleAutomations(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := s.SweepIdleAutomationsIn(ctx, ws); err != nil {
			return err
		}
	}
	return nil
}

// SweepIdleAutomationsIn queues one run per quiet period and idle_minutes for the conversations
// whose quiet period reached it within the last window; the job's unique arguments, kept while
// River keeps the finished job, make a later sweep queue the same period again for nothing.
func (s *Server) SweepIdleAutomationsIn(ctx context.Context, ws uuid.UUID) error {
	mins, err := s.st.ListIdleAutomationMinutes(ctx, ws)
	if err != nil || len(mins) == 0 {
		return err
	}
	now := s.now()
	var queued []river.InsertManyParams
	for _, m := range mins {
		before := now.Add(-time.Duration(m) * time.Minute)
		rows, err := s.st.ListIdleConversations(ctx, store.ListIdleConversationsParams{
			WorkspaceID: ws, IdleBefore: before, NotBefore: before.Add(-automationIdleWindow), Lim: automationIdleBatch,
		})
		if err != nil {
			return err
		}
		for _, r := range rows {
			since := r.LastActivityAt.UTC()
			queued = append(queued, river.InsertManyParams{Args: AutomationRunArgs{
				WorkspaceID: ws, ConversationID: r.ID, Trigger: string(oas.AutomationTriggerIdle), IdleSince: &since, IdleMinutes: m,
			}})
		}
	}
	if len(queued) == 0 {
		return nil
	}
	_, err = s.jobs.InsertMany(ctx, queued)
	return err
}

// CleanAutomationRuns deletes the runs older than 30 days.
func (s *Server) CleanAutomationRuns(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	before := s.now().Add(-automationRunsKept)
	for _, ws := range ids {
		for {
			n, err := s.st.DeleteOldAutomationRuns(ctx, store.DeleteOldAutomationRunsParams{WorkspaceID: ws, Before: before, Lim: automationRunsBatch})
			if err != nil {
				return err
			}
			if n < automationRunsBatch {
				break
			}
		}
	}
	return nil
}

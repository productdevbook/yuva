package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxAutomationRules      = 100
	maxAutomationConditions = 20
	maxAutomationActions    = 10
	maxAutomationNote       = 10000
)

var (
	errAutomationRuleGone  = problem(http.StatusNotFound, "not_found", "no such automation rule")
	errAutomationRuleLimit = problem(http.StatusForbidden, "automation_rule_limit", "a workspace holds at most 100 automation rules")
	feedbackCategoryKey    = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

var automationFieldOps = map[oas.AutomationConditionField][]oas.AutomationConditionOperator{
	oas.AutomationFieldChannelKind:      {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldKind:             {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldFeedbackCategory: {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldPriority:         {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldStatus:           {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldSubject:          {oas.AutomationOpContains, oas.AutomationOpNotContains},
	oas.AutomationFieldBody:             {oas.AutomationOpContains, oas.AutomationOpNotContains},
	oas.AutomationFieldContactAttribute: {oas.AutomationOpEquals, oas.AutomationOpNotEquals, oas.AutomationOpContains, oas.AutomationOpNotContains, oas.AutomationOpIsSet, oas.AutomationOpIsNotSet},
	oas.AutomationFieldCustomField:      {oas.AutomationOpEquals, oas.AutomationOpNotEquals, oas.AutomationOpContains, oas.AutomationOpNotContains, oas.AutomationOpIsSet, oas.AutomationOpIsNotSet},
	oas.AutomationFieldLabel:            {oas.AutomationOpIn, oas.AutomationOpNotIn},
	oas.AutomationFieldTeam:             {oas.AutomationOpIn, oas.AutomationOpNotIn, oas.AutomationOpIsSet, oas.AutomationOpIsNotSet},
	oas.AutomationFieldAssignee:         {oas.AutomationOpIsSet, oas.AutomationOpIsNotSet},
	oas.AutomationFieldBusinessHours:    {oas.AutomationOpEquals},
}

func automationRuleBody(r store.AutomationRule) oas.AutomationRule {
	out := oas.AutomationRule{
		Id: r.ID, Name: r.Name, Enabled: r.Enabled, Position: r.Position, InboxId: r.InboxID, Trigger: oas.AutomationTrigger(r.Trigger),
		IdleMinutes: r.IdleMinutes, Conditions: ruleConditions(r), Actions: ruleActions(r), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		SavedBy: oas.AutomationRuleAuthor{Type: oas.AutomationRuleAuthorType(r.SavedByKind), MemberId: r.SavedByMemberID, ApiKeyId: r.SavedByApiKeyID},
	}
	return out
}

func ruleConditions(r store.AutomationRule) []oas.AutomationCondition {
	out := []oas.AutomationCondition{}
	_ = json.Unmarshal(r.Conditions, &out)
	return out
}

func ruleActions(r store.AutomationRule) []oas.AutomationAction {
	out := []oas.AutomationAction{}
	_ = json.Unmarshal(r.Actions, &out)
	return out
}

func automationRuleVisible(r store.AutomationRule, seen map[uuid.UUID]bool) bool {
	return seen == nil || r.InboxID == nil || seen[*r.InboxID]
}

func savedBy(p principal) (string, *uuid.UUID, *uuid.UUID) {
	switch {
	case p.isKey():
		return string(oas.AutomationAuthorApiKey), nil, new(p.keyID)
	case p.isGrant():
		return string(oas.AutomationAuthorOAuthGrant), new(p.memberID), nil
	}
	return string(oas.AutomationAuthorMember), new(p.memberID), nil
}

type automationRuleFields struct {
	name       string
	enabled    bool
	inbox      *uuid.UUID
	trigger    oas.AutomationTrigger
	idle       *int32
	conditions []oas.AutomationCondition
	actions    []oas.AutomationAction
	hooks      []string
}

func (f *automationRuleFields) check(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	if !f.trigger.Valid() {
		return errValidation("trigger must be conversation_created, message_received, status_changed or idle")
	}
	if f.trigger == oas.AutomationTriggerIdle {
		if f.idle == nil || *f.idle < 5 || *f.idle > 43200 {
			return errValidation("idle_minutes must be 5 to 43200 for the idle trigger")
		}
	} else if f.idle != nil {
		return errValidation("idle_minutes goes with the idle trigger only")
	}
	if f.inbox != nil {
		found, err := q.LockInboxesShared(ctx, store.LockInboxesSharedParams{WorkspaceID: ws, Ids: []uuid.UUID{*f.inbox}})
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return errValidation("inbox_id must be an inbox of the workspace")
		}
	}
	if len(f.conditions) > maxAutomationConditions {
		return errValidation("a rule has at most 20 conditions")
	}
	for i := range f.conditions {
		if err := checkCondition(ctx, q, ws, &f.conditions[i], "conditions["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	if len(f.actions) == 0 || len(f.actions) > maxAutomationActions {
		return errValidation("a rule has 1 to 10 actions")
	}
	f.hooks, err = checkActions(ctx, q, ws, f.actions)
	return err
}

func checkCondition(ctx context.Context, q *store.Queries, ws uuid.UUID, c *oas.AutomationCondition, at string) error {
	ops, ok := automationFieldOps[c.Field]
	if !ok {
		return errValidation(at + ".field is not a condition field")
	}
	if !slices.Contains(ops, c.Operator) {
		return errValidation(at + ": " + string(c.Field) + " does not take " + string(c.Operator))
	}
	keyed := c.Field == oas.AutomationFieldContactAttribute || c.Field == oas.AutomationFieldCustomField
	if keyed != (c.Key != nil) {
		if keyed {
			return errValidation(at + ".key is required for " + string(c.Field))
		}
		return errValidation(at + ".key goes with contact_attribute and custom_field only")
	}
	switch c.Operator {
	case oas.AutomationOpIn, oas.AutomationOpNotIn:
		if c.Values == nil || len(*c.Values) == 0 || c.Value != nil {
			return errValidation(at + ": " + string(c.Operator) + " takes values")
		}
	case oas.AutomationOpIsSet, oas.AutomationOpIsNotSet:
		if c.Values != nil || c.Value != nil {
			return errValidation(at + ": " + string(c.Operator) + " takes neither value nor values")
		}
	default:
		if c.Value == nil || c.Values != nil {
			return errValidation(at + ": " + string(c.Operator) + " takes value")
		}
		v, err := trimmed(*c.Value, 1, 500, at+".value")
		if err != nil {
			return err
		}
		c.Value = &v
	}
	if c.Key != nil {
		k, err := trimmed(*c.Key, 1, 100, at+".key")
		if err != nil {
			return err
		}
		c.Key = &k
		if c.Field == oas.AutomationFieldCustomField {
			found, err := q.ListCustomFieldKeys(ctx, store.ListCustomFieldKeysParams{WorkspaceID: ws, Keys: []string{k}})
			if err != nil {
				return err
			}
			if len(found) == 0 {
				return errValidation(at + ".key must be a conversation custom field")
			}
		}
	}
	if c.Field == oas.AutomationFieldBusinessHours && *c.Value != "inside" && *c.Value != "outside" {
		return errValidation(at + ".value must be inside or outside")
	}
	if c.Values == nil {
		return nil
	}
	vals := []string{}
	for _, v := range *c.Values {
		v = strings.TrimSpace(v)
		if !slices.Contains(vals, v) {
			vals = append(vals, v)
		}
	}
	if len(vals) > 50 {
		return errValidation(at + ".values holds at most 50 entries")
	}
	c.Values = &vals
	bad := func(msg string) error { return errValidation(at + ".values " + msg) }
	switch c.Field {
	case oas.AutomationFieldChannelKind:
		for _, v := range vals {
			if !oas.ChannelKind(v).Valid() {
				return bad("must be chat, email, app or api")
			}
		}
	case oas.AutomationFieldKind:
		for _, v := range vals {
			if !oas.ConversationKind(v).Valid() {
				return bad("must be conversation or feedback")
			}
		}
	case oas.AutomationFieldFeedbackCategory:
		for _, v := range vals {
			if !feedbackCategoryKey.MatchString(v) {
				return bad("must be feedback category keys")
			}
		}
	case oas.AutomationFieldPriority:
		for _, v := range vals {
			if !oas.Priority(v).Valid() {
				return bad("must be low, normal, high or urgent")
			}
		}
	case oas.AutomationFieldStatus:
		for _, v := range vals {
			if !oas.ConversationStatus(v).Valid() {
				return bad("must be open, pending, snoozed or closed")
			}
		}
	case oas.AutomationFieldLabel, oas.AutomationFieldTeam:
		ids := make([]uuid.UUID, 0, len(vals))
		for _, v := range vals {
			id, err := uuid.Parse(v)
			if err != nil {
				return bad("must be ids")
			}
			ids = append(ids, id)
		}
		var n int
		if c.Field == oas.AutomationFieldLabel {
			found, err := q.LockLabels(ctx, store.LockLabelsParams{WorkspaceID: ws, Ids: ids})
			if err != nil {
				return err
			}
			n = len(found)
		} else {
			found, err := q.ShareLockTeams(ctx, store.ShareLockTeamsParams{WorkspaceID: ws, Ids: ids})
			if err != nil {
				return err
			}
			n = len(found)
		}
		if n != len(ids) {
			return bad("names a " + string(c.Field) + " that does not exist")
		}
		for i, id := range ids {
			vals[i] = id.String()
		}
	}
	return nil
}

var automationActionFields = map[oas.AutomationActionType]string{
	oas.AutomationActionAddLabels:    "label_ids",
	oas.AutomationActionRemoveLabels: "label_ids",
	oas.AutomationActionAssignMember: "member_id",
	oas.AutomationActionAssignTeam:   "team_id",
	oas.AutomationActionAssignPool:   "",
	oas.AutomationActionUnassign:     "",
	oas.AutomationActionSetPriority:  "priority",
	oas.AutomationActionSetStatus:    "status",
	oas.AutomationActionSnooze:       "snooze_minutes",
	oas.AutomationActionAddNote:      "body",
	oas.AutomationActionSendReply:    "canned_reply_id",
	oas.AutomationActionSendWebhook:  "webhook_id",
}

func actionFieldsSet(a oas.AutomationAction) []string {
	var out []string
	for name, set := range map[string]bool{
		"label_ids": a.LabelIds != nil, "member_id": a.MemberId != nil, "team_id": a.TeamId != nil, "priority": a.Priority != nil,
		"status": a.Status != nil, "snooze_minutes": a.SnoozeMinutes != nil, "body": a.Body != nil,
		"canned_reply_id": a.CannedReplyId != nil, "webhook_id": a.WebhookId != nil,
	} {
		if set {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// checkActions validates the actions and locks what they name until the rule is written; it
// returns the hosts of the webhook endpoints they send to.
func checkActions(ctx context.Context, q *store.Queries, ws uuid.UUID, actions []oas.AutomationAction) ([]string, error) {
	seen := map[oas.AutomationActionType]bool{}
	var hosts []string
	for i := range actions {
		a := &actions[i]
		at := "actions[" + strconv.Itoa(i) + "]"
		want, ok := automationActionFields[a.Type]
		if !ok {
			return nil, errValidation(at + ".type is not an action type")
		}
		if seen[a.Type] {
			return nil, errValidation(at + ": a rule has each action type once")
		}
		seen[a.Type] = true
		got := actionFieldsSet(*a)
		if want == "" && len(got) > 0 || want != "" && !slices.Equal(got, []string{want}) {
			if want == "" {
				return nil, errValidation(at + ": " + string(a.Type) + " takes no fields")
			}
			return nil, errValidation(at + ": " + string(a.Type) + " takes " + want + " and nothing else")
		}
		switch a.Type {
		case oas.AutomationActionAddLabels, oas.AutomationActionRemoveLabels:
			if len(*a.LabelIds) == 0 {
				return nil, errValidation(at + ".label_ids must name at least one label")
			}
			ids, err := lockedLabelSet(ctx, q, ws, *a.LabelIds)
			if err != nil {
				return nil, err
			}
			a.LabelIds = &ids
		case oas.AutomationActionAssignMember:
			found, err := q.LockMemberShared(ctx, store.LockMemberSharedParams{WorkspaceID: ws, ID: *a.MemberId})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errValidation(at + ".member_id must be a member of the workspace")
			}
		case oas.AutomationActionAssignTeam:
			found, err := q.ShareLockTeams(ctx, store.ShareLockTeamsParams{WorkspaceID: ws, Ids: []uuid.UUID{*a.TeamId}})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errValidation(at + ".team_id must be a team of the workspace")
			}
		case oas.AutomationActionSetPriority:
			if !a.Priority.Valid() {
				return nil, errValidation(at + ".priority must be low, normal, high or urgent")
			}
		case oas.AutomationActionSetStatus:
			if !a.Status.Valid() || *a.Status == oas.ConversationStatusSnoozed {
				return nil, errValidation(at + ".status must be open, pending or closed; snooze with the snooze action")
			}
		case oas.AutomationActionSnooze:
			if *a.SnoozeMinutes < 1 || *a.SnoozeMinutes > maxSnoozeMinutes {
				return nil, errValidation(at + ".snooze_minutes must be 1 to 525600")
			}
		case oas.AutomationActionAddNote:
			body, err := trimmed(*a.Body, 1, maxAutomationNote, at+".body")
			if err != nil {
				return nil, err
			}
			a.Body = &body
		case oas.AutomationActionSendReply:
			found, err := q.ListCannedRepliesByID(ctx, store.ListCannedRepliesByIDParams{WorkspaceID: ws, Ids: []uuid.UUID{*a.CannedReplyId}})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errValidation(at + ".canned_reply_id must be a canned reply of the workspace")
			}
		case oas.AutomationActionSendWebhook:
			found, err := q.ListWebhookEndpointsByID(ctx, store.ListWebhookEndpointsByIDParams{WorkspaceID: ws, Ids: []uuid.UUID{*a.WebhookId}})
			if err != nil {
				return nil, err
			}
			if len(found) == 0 {
				return nil, errValidation(at + ".webhook_id must be a webhook endpoint of the workspace")
			}
			hosts = append(hosts, urlHost(found[0].Url))
		}
	}
	for _, pair := range [][]oas.AutomationActionType{
		{oas.AutomationActionSetStatus, oas.AutomationActionSnooze},
		{oas.AutomationActionAssignMember, oas.AutomationActionAssignPool},
		{oas.AutomationActionAssignMember, oas.AutomationActionUnassign},
		{oas.AutomationActionAssignPool, oas.AutomationActionUnassign},
	} {
		if seen[pair[0]] && seen[pair[1]] {
			return nil, errValidation("actions: " + string(pair[0]) + " and " + string(pair[1]) + " exclude each other")
		}
	}
	var add, remove []uuid.UUID
	for _, a := range actions {
		switch a.Type {
		case oas.AutomationActionAddLabels:
			add = *a.LabelIds
		case oas.AutomationActionRemoveLabels:
			remove = *a.LabelIds
		}
	}
	if slices.ContainsFunc(add, func(l uuid.UUID) bool { return slices.Contains(remove, l) }) {
		return nil, errValidation("actions: a label cannot be both added and removed")
	}
	return hosts, nil
}

func ruleAuditDetails(r store.AutomationRule, hosts []string) auditDetails {
	types := []string{}
	for _, a := range ruleActions(r) {
		types = append(types, string(a.Type))
	}
	d := auditDetails{"name": r.Name, "trigger": r.Trigger, "action_types": types}
	if r.InboxID != nil {
		d["inbox_id"] = r.InboxID.String()
	}
	if len(hosts) > 0 {
		d["webhook_hosts"] = hosts
	}
	return d
}

func (s *Server) ListAutomationRules(ctx context.Context, _ oas.ListAutomationRulesRequestObject) (oas.ListAutomationRulesResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListAutomationRules(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	seen, err := seenInboxes(ctx, s.st.Queries, p)
	if err != nil {
		return nil, err
	}
	return oas.ListAutomationRules200JSONResponse{Items: automationRuleBodies(rows, seen)}, nil
}

func automationRuleBodies(rows []store.AutomationRule, seen map[uuid.UUID]bool) []oas.AutomationRule {
	out := []oas.AutomationRule{}
	for _, r := range rows {
		if automationRuleVisible(r, seen) {
			out = append(out, automationRuleBody(r))
		}
	}
	return out
}

// visibleAutomationRule is a rule of the caller's workspace that the caller sees.
func visibleAutomationRule(ctx context.Context, q *store.Queries, p principal, id uuid.UUID) (store.AutomationRule, map[uuid.UUID]bool, error) {
	r, err := q.GetAutomationRule(ctx, store.GetAutomationRuleParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return r, nil, errAutomationRuleGone
	}
	if err != nil {
		return r, nil, err
	}
	seen, err := seenInboxes(ctx, q, p)
	if err != nil {
		return r, nil, err
	}
	if !automationRuleVisible(r, seen) {
		return r, nil, errAutomationRuleGone
	}
	return r, seen, nil
}

func (s *Server) GetAutomationRule(ctx context.Context, req oas.GetAutomationRuleRequestObject) (oas.GetAutomationRuleResponseObject, error) {
	p := principalFrom(ctx)
	r, _, err := visibleAutomationRule(ctx, s.st.Queries, p, req.AutomationRuleId)
	if err != nil {
		return nil, err
	}
	return oas.GetAutomationRule200JSONResponse(automationRuleBody(r)), nil
}

func (s *Server) CreateAutomationRule(ctx context.Context, req oas.CreateAutomationRuleRequestObject) (oas.CreateAutomationRuleResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	f := automationRuleFields{
		name: b.Name, enabled: b.Enabled == nil || *b.Enabled, inbox: b.InboxId, trigger: b.Trigger, idle: b.IdleMinutes,
		conditions: listOf(b.Conditions), actions: b.Actions,
	}
	if f.conditions == nil {
		f.conditions = []oas.AutomationCondition{}
	}
	var out store.AutomationRule
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockAutomationRules(ctx, p.workspaceID); err != nil {
			return err
		}
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		n, err := q.CountAutomationRules(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		if n >= maxAutomationRules {
			return errAutomationRuleLimit
		}
		kind, member, key := savedBy(p)
		now := s.now()
		out, err = q.CreateAutomationRule(ctx, store.CreateAutomationRuleParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: f.inbox, Name: f.name, Enabled: f.enabled, Trigger: string(f.trigger),
			IdleMinutes: f.idle, Conditions: mustJSON(f.conditions), Actions: mustJSON(f.actions),
			SavedByKind: kind, SavedByMemberID: member, SavedByApiKeyID: key, Now: now,
		})
		if err != nil {
			return err
		}
		return writeAudit(ctx, q, p, now, oas.AuditAutomationRuleCreated, oas.AuditTargetAutomationRule, out.ID, ruleAuditDetails(out, f.hooks))
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateAutomationRule201JSONResponse(automationRuleBody(out)), nil
}

func (s *Server) UpdateAutomationRule(ctx context.Context, req oas.UpdateAutomationRuleRequestObject) (oas.UpdateAutomationRuleResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out store.AutomationRule
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockAutomationRules(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.LockAutomationRule(ctx, store.LockAutomationRuleParams{WorkspaceID: p.workspaceID, ID: req.AutomationRuleId})
		if store.IsNotFound(err) {
			return errAutomationRuleGone
		}
		if err != nil {
			return err
		}
		f := automationRuleFields{
			name: cur.Name, enabled: cur.Enabled, inbox: cur.InboxID, trigger: oas.AutomationTrigger(cur.Trigger), idle: cur.IdleMinutes,
			conditions: ruleConditions(cur), actions: ruleActions(cur),
		}
		var fields []string
		if b.Name != nil {
			f.name, fields = *b.Name, append(fields, "name")
		}
		if b.Enabled != nil {
			f.enabled, fields = *b.Enabled, append(fields, "enabled")
		}
		if b.InboxId.IsSpecified() {
			f.inbox, fields = nil, append(fields, "inbox_id")
			if !b.InboxId.IsNull() {
				f.inbox = new(b.InboxId.MustGet())
			}
		}
		if b.Trigger != nil {
			f.trigger, fields = *b.Trigger, append(fields, "trigger")
			if f.trigger != oas.AutomationTriggerIdle && !b.IdleMinutes.IsSpecified() {
				f.idle = nil
			}
		}
		if b.IdleMinutes.IsSpecified() {
			f.idle, fields = nil, append(fields, "idle_minutes")
			if !b.IdleMinutes.IsNull() {
				f.idle = new(b.IdleMinutes.MustGet())
			}
		}
		if b.Conditions != nil {
			f.conditions, fields = *b.Conditions, append(fields, "conditions")
		}
		if b.Actions != nil {
			f.actions, fields = *b.Actions, append(fields, "actions")
		}
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		kind, member, key := savedBy(p)
		now := s.now()
		out, err = q.UpdateAutomationRule(ctx, store.UpdateAutomationRuleParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, InboxID: f.inbox, Name: f.name, Enabled: f.enabled, Trigger: string(f.trigger),
			IdleMinutes: f.idle, Conditions: mustJSON(f.conditions), Actions: mustJSON(f.actions),
			SavedByKind: kind, SavedByMemberID: member, SavedByApiKeyID: key, Now: now,
		})
		if err != nil {
			return err
		}
		d := ruleAuditDetails(out, f.hooks)
		d["fields"] = fields
		return writeAudit(ctx, q, p, now, oas.AuditAutomationRuleUpdated, oas.AuditTargetAutomationRule, out.ID, d)
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateAutomationRule200JSONResponse(automationRuleBody(out)), nil
}

func (s *Server) DeleteAutomationRule(ctx context.Context, req oas.DeleteAutomationRuleRequestObject) (oas.DeleteAutomationRuleResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockAutomationRules(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.LockAutomationRule(ctx, store.LockAutomationRuleParams{WorkspaceID: p.workspaceID, ID: req.AutomationRuleId})
		if store.IsNotFound(err) {
			return errAutomationRuleGone
		}
		if err != nil {
			return err
		}
		var hosts []string
		var hooks []uuid.UUID
		for _, a := range ruleActions(cur) {
			if a.WebhookId != nil {
				hooks = append(hooks, *a.WebhookId)
			}
		}
		if len(hooks) > 0 {
			found, err := q.ListWebhookEndpointsByID(ctx, store.ListWebhookEndpointsByIDParams{WorkspaceID: p.workspaceID, Ids: hooks})
			if err != nil {
				return err
			}
			for _, e := range found {
				hosts = append(hosts, urlHost(e.Url))
			}
		}
		if _, err := q.DeleteAutomationRule(ctx, store.DeleteAutomationRuleParams{WorkspaceID: p.workspaceID, ID: cur.ID}); err != nil {
			return err
		}
		return writeAudit(ctx, q, p, s.now(), oas.AuditAutomationRuleDeleted, oas.AuditTargetAutomationRule, cur.ID, ruleAuditDetails(cur, hosts))
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteAutomationRule204Response{}, nil
}

func (s *Server) OrderAutomationRules(ctx context.Context, req oas.OrderAutomationRulesRequestObject) (oas.OrderAutomationRulesResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrFullKey(p); err != nil {
		return nil, err
	}
	var rows []store.AutomationRule
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if err := q.LockAutomationRules(ctx, p.workspaceID); err != nil {
			return err
		}
		cur, err := q.ListAutomationRules(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		ids := req.Body.Ids
		if len(sortedIDs(ids)) != len(ids) || len(ids) != len(cur) {
			return errValidation("ids must name every automation rule of the workspace once")
		}
		for _, c := range cur {
			if !slices.Contains(ids, c.ID) {
				return errValidation("ids must name every automation rule of the workspace once")
			}
		}
		for i, id := range ids {
			if err := q.SetAutomationRulePosition(ctx, store.SetAutomationRulePositionParams{WorkspaceID: p.workspaceID, ID: id, Position: int32(i)}); err != nil {
				return err
			}
		}
		rows, err = q.ListAutomationRules(ctx, p.workspaceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.OrderAutomationRules200JSONResponse{Items: automationRuleBodies(rows, nil)}, nil
}

func (s *Server) ListAutomationRuns(ctx context.Context, req oas.ListAutomationRunsRequestObject) (oas.ListAutomationRunsResponseObject, error) {
	p := principalFrom(ctx)
	r, seen, err := visibleAutomationRule(ctx, s.st.Queries, p, req.AutomationRuleId)
	if err != nil {
		return nil, err
	}
	limit, err := pageSize(req.Params.Limit)
	if err != nil {
		return nil, err
	}
	at, id, err := decodeCursor(req.Params.Cursor)
	if err != nil {
		return nil, err
	}
	arg := store.ListAutomationRunsParams{WorkspaceID: p.workspaceID, RuleID: r.ID, AllInboxes: seen == nil, CursorAt: at, CursorID: id, Lim: limit + 1}
	if req.Params.Outcome != nil {
		if !req.Params.Outcome.Valid() {
			return nil, errValidation("outcome must be applied or failed")
		}
		arg.Outcome = new(string(*req.Params.Outcome))
	}
	for inbox := range seen {
		arg.InboxIds = append(arg.InboxIds, inbox)
	}
	rows, err := s.st.ListAutomationRuns(ctx, arg)
	if err != nil {
		return nil, err
	}
	out := oas.AutomationRunPage{Items: []oas.AutomationRun{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		out.NextCursor = new(encodeCursor(last.CreatedAt, last.ID))
	}
	for _, row := range rows {
		run := oas.AutomationRun{
			Id: row.ID, RuleId: row.RuleID, ConversationId: row.ConversationID, Trigger: oas.AutomationTrigger(row.Trigger),
			Outcome: oas.AutomationRunOutcome(row.Outcome), Error: row.Error, Detail: row.Detail, CreatedAt: row.CreatedAt,
			Actions: []oas.AutomationRunAction{},
		}
		_ = json.Unmarshal(row.Actions, &run.Actions)
		out.Items = append(out.Items, run)
	}
	return oas.ListAutomationRuns200JSONResponse(out), nil
}

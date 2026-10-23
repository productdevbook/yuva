package api_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (h *harness) automationJobs(ws string) []api.AutomationRunArgs {
	h.t.Helper()
	var out []api.AutomationRunArgs
	for _, j := range h.jobs("automation_run", ws) {
		var a api.AutomationRunArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// runAutomations runs the queued automation jobs, as the job queue would, and drops them.
func (h *harness) runAutomations(ws string) int {
	h.t.Helper()
	jobs := h.jobs("automation_run", ws)
	for _, j := range jobs {
		var a api.AutomationRunArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.RunAutomations(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	return len(jobs)
}

func (tm team) rule(c *client, body map[string]any) string {
	tm.owner.h.t.Helper()
	return c.expect(http.StatusCreated, "POST", "/v1/automation-rules", body).str("id")
}

func ruleRuns(c *client, rule string) []map[string]any {
	return items(c.expect(http.StatusOK, "GET", "/v1/automation-rules/"+rule+"/runs", nil))
}

func byRule(msgs []map[string]any, kind string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		if m["rule"] != nil && m["kind"] == kind {
			out = append(out, m)
		}
	}
	return out
}

func labelsOf(r response) []string {
	var out []string
	for _, l := range r.body["labels"].([]any) {
		out = append(out, l.(string))
	}
	return out
}

func TestAutomationRuleEndpoints(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	otherLabel := other.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": "https://hooks.example.com/secret-path?token=abc", "events": []string{"conversation.created"}}).body["endpoint"].(map[string]any)["id"].(string)
	canned := tm.owner.expect(http.StatusCreated, "POST", "/v1/canned-replies", map[string]any{"shortcut": "thanks", "title": "Thanks", "body": "Thanks, we are on it."}).str("id")
	second := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)

	first := tm.owner.expect(http.StatusCreated, "POST", "/v1/automation-rules", map[string]any{
		"name": " Tag VIPs ", "trigger": "message_received",
		"conditions": []map[string]any{
			{"field": "contact_attribute", "operator": "equals", "key": "plan", "value": "pro"},
			{"field": "channel_kind", "operator": "in", "values": []string{"email", "chat", "email"}},
		},
		"actions": []map[string]any{
			{"type": "add_labels", "label_ids": []string{label}},
			{"type": "send_webhook", "webhook_id": hook},
			{"type": "send_reply", "canned_reply_id": canned},
		},
	})
	b := first.body
	if b["name"] != "Tag VIPs" || b["position"] != 0.0 || b["enabled"] != true || b["inbox_id"] != nil || b["idle_minutes"] != nil ||
		b["saved_by"].(map[string]any)["type"] != "member" || fmt.Sprint(b["conditions"].([]any)[1].(map[string]any)["values"]) != "[email chat]" {
		t.Fatalf("created %s", first.raw)
	}
	id := first.str("id")
	inboxRule := tm.rule(tm.owner, map[string]any{"name": "Billing idle", "inbox_id": second, "trigger": "idle", "idle_minutes": 30,
		"actions": []map[string]any{{"type": "add_note", "body": "Quiet for half an hour."}}})

	bad := map[string]map[string]any{
		"idle without minutes":   {"name": "x", "trigger": "idle", "actions": []map[string]any{{"type": "unassign"}}},
		"minutes without idle":   {"name": "x", "trigger": "conversation_created", "idle_minutes": 30, "actions": []map[string]any{{"type": "unassign"}}},
		"no actions":             {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{}},
		"operator for field":     {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "subject", "operator": "in", "values": []string{"a"}}}, "actions": []map[string]any{{"type": "unassign"}}},
		"value for in":           {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "priority", "operator": "in", "value": "high"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"unknown priority":       {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "priority", "operator": "in", "values": []string{"meh"}}}, "actions": []map[string]any{{"type": "unassign"}}},
		"key missing":            {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "contact_attribute", "operator": "is_set"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"unknown custom field":   {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "custom_field", "key": "nope", "operator": "is_set"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"label of another ws":    {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_labels", "label_ids": []string{otherLabel}}}},
		"label condition other":  {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "label", "operator": "in", "values": []string{otherLabel}}}, "actions": []map[string]any{{"type": "unassign"}}},
		"inbox of another ws":    {"name": "x", "inbox_id": other.inbox, "trigger": "conversation_created", "actions": []map[string]any{{"type": "unassign"}}},
		"twice the same type":    {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "unassign"}, {"type": "unassign"}}},
		"field of another type":  {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_labels", "label_ids": []string{label}, "member_id": tm.agentID}}},
		"status snoozed":         {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "set_status", "status": "snoozed"}}},
		"status and snooze":      {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "set_status", "status": "closed"}, {"type": "snooze", "snooze_minutes": 5}}},
		"added and removed":      {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_labels", "label_ids": []string{label}}, {"type": "remove_labels", "label_ids": []string{label}}}},
		"canned of another ws":   {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "send_reply", "canned_reply_id": uuid.New().String()}}},
		"member of another ws":   {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "assign_member", "member_id": uuid.New().String()}}},
		"blank name":             {"name": " ", "trigger": "conversation_created", "actions": []map[string]any{{"type": "unassign"}}},
		"business hours value":   {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "business_hours", "operator": "equals", "value": "lunch"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"assign and unassign":    {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "assign_member", "member_id": tm.agentID}, {"type": "unassign"}}},
		"webhook of another ws":  {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "send_webhook", "webhook_id": uuid.New().String()}}},
		"note too long":          {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": strings.Repeat("a", 10001)}}},
		"pool takes no field":    {"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "assign_pool", "member_id": tm.agentID}}},
		"value for is_set":       {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "assignee", "operator": "is_set", "value": "x"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"key on a plain field":   {"name": "x", "trigger": "conversation_created", "conditions": []map[string]any{{"field": "subject", "key": "x", "operator": "contains", "value": "x"}}, "actions": []map[string]any{{"type": "unassign"}}},
		"eleven actions (limit)": {"name": "x", "trigger": "conversation_created", "actions": slices.Repeat([]map[string]any{{"type": "unassign"}}, 11)},
	}
	for name, body := range bad {
		if r := tm.owner.do("POST", "/v1/automation-rules", body); r.status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/webhooks", map[string]any{"url": "https://hooks.example.com/x", "events": []string{"automation.triggered"}})

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/automation-rules", map[string]any{"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "unassign"}}})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/automation-rules/"+id, map[string]any{"name": "y"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/automation-rules/"+id, nil)
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)); !slices.Equal(got, []string{id}) {
		t.Fatalf("an agent without the billing inbox reads %v", got)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+inboxRule, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+inboxRule+"/runs", nil)

	reader := h.client()
	reader.bearer = apiKey(tm.owner, "reader", "automations:read")
	reader.expect(http.StatusOK, "GET", "/v1/automation-rules/"+id, nil)
	reader.expect(http.StatusOK, "GET", "/v1/automation-rules/"+id+"/runs", nil)
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "DELETE", "/v1/automation-rules/"+id, nil)
	unnamed := h.client()
	unnamed.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "everything"}).str("secret")
	unnamed.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)
	unnamed.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/automation-rules", map[string]any{"name": "x", "trigger": "conversation_created", "actions": []map[string]any{{"type": "unassign"}}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/api-keys", map[string]any{"name": "limited", "inbox_ids": []string{tm.inbox}, "scopes": []string{"automations:manage"}})
	limited := h.client()
	limited.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "limited", "inbox_ids": []string{tm.inbox}, "scopes": []string{"automations:read"}}).str("secret")
	if got := itemIDs(limited.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)); !slices.Equal(got, []string{id}) {
		t.Fatalf("a key limited to the support inbox reads %v", got)
	}
	manager := h.client()
	manager.bearer = apiKey(tm.owner, "manager", "automations:read", "automations:manage")
	byKey := manager.expect(http.StatusCreated, "POST", "/v1/automation-rules", map[string]any{"name": "By key", "trigger": "status_changed", "actions": []map[string]any{{"type": "set_priority", "priority": "high"}}})
	if sb := byKey.body["saved_by"].(map[string]any); sb["type"] != "api_key" || sb["api_key_id"] == nil || byKey.body["position"] != 2.0 {
		t.Fatalf("a key's rule %s", byKey.raw)
	}

	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+id+"/runs", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/automation-rules/"+id, map[string]any{"name": "mine"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/automation-rules/"+id, nil)
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/automation-rules/order", map[string]any{"ids": []string{id, inboxRule, byKey.str("id")}})
	if got := items(other.owner.expect(http.StatusOK, "GET", "/v1/automation-rules", nil)); len(got) != 0 {
		t.Fatalf("other workspace lists %v", got)
	}

	ordered := tm.owner.expect(http.StatusOK, "PUT", "/v1/automation-rules/order", map[string]any{"ids": []string{inboxRule, byKey.str("id"), id}})
	var got []string
	for _, it := range items(ordered) {
		got = append(got, it["id"].(string))
	}
	if !slices.Equal(got, []string{inboxRule, byKey.str("id"), id}) {
		t.Fatalf("order %v", got)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/automation-rules/order", map[string]any{"ids": []string{id, id, inboxRule}})

	patched := tm.owner.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+inboxRule, map[string]any{"inbox_id": nil, "trigger": "conversation_created", "enabled": false})
	if patched.body["inbox_id"] != nil || patched.body["idle_minutes"] != nil || patched.body["trigger"] != "conversation_created" || patched.body["enabled"] != false {
		t.Fatalf("patched %s", patched.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/automation-rules/"+inboxRule, map[string]any{"trigger": "idle"})
	manager.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+id, map[string]any{"name": "Tag VIPs by key"})
	if sb := tm.owner.expect(http.StatusOK, "GET", "/v1/automation-rules/"+id, nil).body["saved_by"].(map[string]any); sb["type"] != "member" {
		t.Fatalf("a rename made %v the author", sb)
	}
	manager.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+id, map[string]any{"enabled": false})
	if sb := tm.owner.expect(http.StatusOK, "GET", "/v1/automation-rules/"+id, nil).body["saved_by"].(map[string]any); sb["type"] != "api_key" {
		t.Fatalf("the last to change what it does is %v", sb)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/automation-rules/"+id, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+id, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/automation-rules/"+id, nil)

	entries := items(tm.owner.expect(http.StatusOK, "GET", "/v1/audit-log", nil))
	created := auditOf(entries, "automation_rule.created")
	if len(created) != 3 {
		t.Fatalf("%d created entries", len(created))
	}
	deleted := oneAudit(t, entries, "automation_rule.deleted")
	d := detailsOf(deleted)
	if fmt.Sprint(d["webhook_hosts"]) != "[hooks.example.com]" || d["trigger"] != "message_received" || targetOf(deleted)["type"] != "automation_rule" {
		t.Fatalf("deleted entry %v", deleted)
	}
	if len(auditOf(entries, "automation_rule.updated")) != 3 {
		t.Fatalf("updated entries %v", auditOf(entries, "automation_rule.updated"))
	}
	if rows := h.auditRows(tm.ws); strings.Contains(rows, "secret-path") || strings.Contains(rows, "token=abc") {
		t.Fatalf("the audit log holds a webhook URL: %s", rows)
	}
}

func TestAutomationRulesRunInOrderOncePerTrigger(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	vip := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	tagger := tm.rule(tm.owner, map[string]any{"name": "Tag", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "subject", "operator": "contains", "value": "URGENT"}},
		"actions":    []map[string]any{{"type": "add_labels", "label_ids": []string{vip}}, {"type": "set_priority", "priority": "high"}, {"type": "assign_pool"}}})
	router := tm.rule(tm.owner, map[string]any{"name": "Route VIPs", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "label", "operator": "in", "values": []string{vip}}, {"field": "priority", "operator": "in", "values": []string{"high"}}, {"field": "assignee", "operator": "is_not_set"}},
		"actions":    []map[string]any{{"type": "assign_member", "member_id": tm.agentID}, {"type": "add_note", "body": "VIP: answer first."}}})
	never := tm.rule(tm.owner, map[string]any{"name": "Untagged", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "label", "operator": "not_in", "values": []string{vip}}},
		"actions":    []map[string]any{{"type": "add_note", "body": "never"}}})
	ignored := tm.rule(tm.owner, map[string]any{"name": "Status", "trigger": "status_changed", "actions": []map[string]any{{"type": "set_priority", "priority": "low"}}})

	conv := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "urgent: help"}).str("id")
	jobs := h.automationJobs(tm.ws)
	if len(jobs) != 1 || jobs[0].Trigger != "conversation_created" || jobs[0].ConversationID.String() != conv {
		t.Fatalf("queued %+v", jobs)
	}
	after := lastEvent(tm.owner)
	h.runAutomations(tm.ws)
	if n := len(h.automationJobs(tm.ws)); n != 0 {
		t.Fatalf("the rules' own changes queued %d runs", n)
	}
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if !slices.Equal(labelsOf(got), []string{vip}) || got.str("priority") != "high" || got.str("assignee_id") != tm.agentID {
		t.Fatalf("after the rules %s", got.raw)
	}
	msgs := messages(tm.owner, conv)
	notes := byRule(msgs, "note")
	if len(notes) != 1 || notes[0]["body"] != "VIP: answer first." || notes[0]["author"].(map[string]any)["type"] != "system" ||
		notes[0]["rule"].(map[string]any)["id"] != router || notes[0]["rule"].(map[string]any)["name"] != "Route VIPs" {
		t.Fatalf("rule notes %v", notes)
	}
	var applied []string
	for _, m := range byRule(msgs, "event") {
		ev := m["event"].(map[string]any)
		if ev["type"] == "rule_applied" {
			applied = append(applied, m["rule"].(map[string]any)["name"].(string))
		}
		if ev["type"] == "labels_changed" && m["rule"].(map[string]any)["id"] != tagger {
			t.Fatalf("labels_changed by %v", m["rule"])
		}
	}
	if !slices.Equal(applied, []string{"Tag", "Route VIPs"}) {
		t.Fatalf("rule_applied events %v", applied)
	}
	for _, e := range feed(tm.owner, after) {
		if e["type"] == "message.created" {
			if d := e["data"].(map[string]any); d["rule"] == nil {
				t.Fatalf("a rule's event names no rule: %v", d)
			}
		}
	}
	runs := ruleRuns(tm.owner, tagger)
	if len(runs) != 1 || runs[0]["outcome"] != "applied" || runs[0]["conversation_id"] != conv || runs[0]["trigger"] != "conversation_created" {
		t.Fatalf("tagger runs %v", runs)
	}
	acts := runs[0]["actions"].([]any)
	if acts[2].(map[string]any)["result"] != "skipped" || acts[0].(map[string]any)["result"] != "applied" {
		t.Fatalf("tagger actions %v", acts)
	}
	if n := len(ruleRuns(tm.owner, router)); n != 1 {
		t.Fatalf("router ran %d times", n)
	}
	if r := ruleRuns(tm.owner, never); len(r) != 1 || r[0]["outcome"] != "not_matched" || len(r[0]["actions"].([]any)) != 0 {
		t.Fatalf("a rule whose conditions failed logs %v", r)
	}
	if n := len(ruleRuns(tm.owner, ignored)); n != 0 {
		t.Fatalf("a status_changed rule ran %d times on a new conversation", n)
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+tagger, map[string]any{"actions": []map[string]any{{"type": "add_labels", "label_ids": []string{vip}}}})
	again := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "URGENT"}).str("id")
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+again, map[string]any{"labels": []string{vip}})
	h.runAutomations(tm.ws)
	runs = ruleRuns(tm.owner, tagger)
	if runs[0]["actions"].([]any)[0].(map[string]any)["result"] != "unchanged" {
		t.Fatalf("adding a label it already has %v", runs[0])
	}
	if r := items(tm.owner.expect(http.StatusOK, "GET", "/v1/automation-rules/"+router+"/runs?outcome=applied", nil)); len(r) != 1 {
		t.Fatalf("a normal-priority conversation matched: %v", r)
	}
}

func TestAutomationRulesNeverLoop(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	reopen := tm.rule(tm.owner, map[string]any{"name": "Never close", "trigger": "status_changed",
		"conditions": []map[string]any{{"field": "status", "operator": "in", "values": []string{"closed"}}},
		"actions":    []map[string]any{{"type": "set_status", "status": "open"}, {"type": "add_note", "body": "Reopened."}}})
	flip := tm.rule(tm.owner, map[string]any{"name": "Close again", "trigger": "status_changed",
		"conditions": []map[string]any{{"field": "status", "operator": "in", "values": []string{"open"}}},
		"actions":    []map[string]any{{"type": "set_status", "status": "closed"}}})
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if n := h.runAutomations(tm.ws); n != 1 {
		t.Fatalf("closing queued %d runs", n)
	}
	for i := range 3 {
		if n := h.runAutomations(tm.ws); n != 0 {
			t.Fatalf("round %d: a rule's change queued %d runs", i, n)
		}
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); got != "closed" {
		t.Fatalf("status %s: the later rule must see the earlier rule's reopen", got)
	}
	if len(ruleRuns(tm.owner, reopen)) != 1 || len(ruleRuns(tm.owner, flip)) != 1 {
		t.Fatal("each rule runs once for the one trigger")
	}
	if n := len(byRule(messages(tm.owner, conv), "note")); n != 1 {
		t.Fatalf("%d notes", n)
	}
}

func TestAutomationAtMostOnceUnderRetriesAndParallelJobs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	note := tm.rule(tm.owner, map[string]any{"name": "Note", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "Hello"}}})
	failing := tm.rule(tm.owner, map[string]any{"name": "Assign", "trigger": "conversation_created", "actions": []map[string]any{{"type": "assign_member", "member_id": tm.agentID}}})

	conv := tm.conversation(tm.owner)
	jobs := h.automationJobs(tm.ws)
	for range 3 {
		if err := h.srv.RunAutomations(ctx, jobs[0]); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(byRule(messages(tm.owner, conv), "note")); n != 1 {
		t.Fatalf("%d notes after retries", n)
	}
	runs := ruleRuns(tm.owner, failing)
	if len(runs) != 1 || runs[0]["outcome"] != "failed" || runs[0]["error"] != "validation_failed" || runs[0]["actions"].([]any)[0].(map[string]any)["result"] != "not_run" {
		t.Fatalf("a member without access fails once: %v", runs)
	}
	for _, j := range h.jobs("automation_run", tm.ws) {
		h.dropJob(j.id)
	}

	parallel := tm.conversation(tm.owner)
	jobs = h.automationJobs(tm.ws)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { errs <- h.srv.RunAutomations(ctx, jobs[0]) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := len(byRule(messages(tm.owner, parallel), "note")); n != 1 {
		t.Fatalf("%d notes from parallel jobs", n)
	}
	if n := h.count("SELECT count(*) FROM automation_runs WHERE conversation_id = $1", parallel); n != 2 {
		t.Fatalf("%d runs for two rules", n)
	}
	if n := len(ruleRuns(tm.owner, note)); n != 2 {
		t.Fatalf("note rule ran %d times for two conversations", n)
	}
}

func TestAutomationFailedRunChangesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "reason", "name": "Reason", "type": "text", "required_on_close": true})
	done := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Done"}).str("id")
	closer := tm.rule(tm.owner, map[string]any{"name": "Close", "trigger": "conversation_created", "actions": []map[string]any{
		{"type": "add_labels", "label_ids": []string{done}}, {"type": "add_note", "body": "Closing."}, {"type": "set_status", "status": "closed"},
	}})
	conv := tm.conversation(tm.owner)
	h.runAutomations(tm.ws)
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if got.str("status") != "open" || len(labelsOf(got)) != 0 || len(byRule(messages(tm.owner, conv), "note")) != 0 {
		t.Fatalf("a failed rule left changes: %s", got.raw)
	}
	runs := ruleRuns(tm.owner, closer)
	if len(runs) != 1 || runs[0]["outcome"] != "failed" || runs[0]["error"] != "field_required" || !strings.Contains(runs[0]["detail"].(string), "reason") {
		t.Fatalf("runs %v", runs)
	}
	failed := items(tm.owner.expect(http.StatusOK, "GET", "/v1/automation-rules/"+closer+"/runs?outcome=failed", nil))
	if len(failed) != 1 || len(items(tm.owner.expect(http.StatusOK, "GET", "/v1/automation-rules/"+closer+"/runs?outcome=applied", nil))) != 0 {
		t.Fatalf("outcome filter %v", failed)
	}

	withReason := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+withReason, map[string]any{"fields": map[string]any{"reason": "spam"}})
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/labels/"+done, nil)
	h.runAutomations(tm.ws)
	if runs := ruleRuns(tm.owner, closer); runs[0]["error"] != "label_gone" {
		t.Fatalf("a deleted label %v", runs[0])
	}
}

func TestAutomationReplyGuards(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	canned := et.owner.expect(http.StatusCreated, "POST", "/v1/canned-replies", map[string]any{"shortcut": "ack", "title": "Ack", "body": "We got it."}).str("id")
	ack := et.rule(et.owner, map[string]any{"name": "Ack", "trigger": "message_received", "actions": []map[string]any{{"type": "send_reply", "canned_reply_id": canned}}})
	firstID := newMessageID()
	mail := func(from, subject string, headers map[string]string) string {
		id := newMessageID()
		if subject == "Help" {
			id = firstID
		}
		if strings.HasPrefix(subject, "Re: ") {
			headers = map[string]string{"In-Reply-To": "<" + firstID + ">", "References": "<" + firstID + ">"}
		}
		r := h.ingest(et.address, buildMail(mailOpts{from: from, to: et.address, subject: subject, messageID: id, headers: headers, body: "Hello"}), nil)
		if r.status != http.StatusAccepted {
			t.Fatalf("ingest %d %v", r.status, r.body)
		}
		h.runAutomations(et.ws)
		return r.str("conversation_id")
	}
	lastRun := func() map[string]any { return ruleRuns(et.owner, ack)[0] }

	customer := unique("customer") + "@example.net"
	conv := mail(customer, "Help", nil)
	replies := byRule(messages(et.owner, conv), "message")
	if len(replies) != 1 || replies[0]["body"] != "We got it." || replies[0]["direction"] != "out" || replies[0]["author"].(map[string]any)["type"] != "system" ||
		replies[0]["email"].(map[string]any)["auto"] != true {
		t.Fatalf("rule reply %v", replies)
	}
	if lastRun()["outcome"] != "applied" {
		t.Fatalf("run %v", lastRun())
	}

	h.clock.Advance(10 * time.Minute)
	mail(customer, "Re: Help", nil)
	if r := lastRun(); r["outcome"] != "failed" || r["error"] != "auto_reply" || !strings.Contains(r["detail"].(string), "hour") {
		t.Fatalf("second reply within the hour %v", r)
	}
	h.clock.Advance(51 * time.Minute)
	mail(customer, "Re: Help", nil)
	if r := lastRun(); r["outcome"] != "applied" || len(byRule(messages(et.owner, conv), "message")) != 2 {
		t.Fatalf("after an hour %v", r)
	}

	auto := mail(unique("away")+"@example.net", "Out of office", map[string]string{"Auto-Submitted": "auto-replied"})
	if r := lastRun(); r["outcome"] != "failed" || r["error"] != "auto_reply" || r["conversation_id"] != auto || len(byRule(messages(et.owner, auto), "message")) != 0 {
		t.Fatalf("an automatic mail got a rule reply: %v", r)
	}
	bounce := h.ingestFrom(et.address, "", buildMail(mailOpts{from: "mailer-daemon@example.net", to: et.address, subject: "Undeliverable", messageID: newMessageID(), body: "failed"}), nil)
	if bounce.status != http.StatusAccepted || bounce.str("conversation_id") == "" {
		t.Fatalf("bounce ingest %d %v", bounce.status, bounce.body)
	}
	h.runAutomations(et.ws)
	if r := lastRun(); r["conversation_id"] != bounce.str("conversation_id") || r["error"] != "auto_reply" {
		t.Fatalf("a bounce got a rule reply: %v", r)
	}

	greet := et.rule(et.owner, map[string]any{"name": "Greet", "trigger": "conversation_created", "actions": []map[string]any{{"type": "send_reply", "canned_reply_id": canned}}})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+ack, map[string]any{"enabled": false})
	spam := et.conversation(et.owner)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"spam": true})
	h.runAutomations(et.ws)
	if r := ruleRuns(et.owner, greet)[0]; r["error"] != "auto_reply" || !strings.Contains(r["detail"].(string), "spam") {
		t.Fatalf("spam %v", r)
	}
	blocked := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Troll", "emails": []string{unique("troll") + "@example.net"}}).str("id")
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": blocked, "subject": "x"})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+blocked, map[string]any{"blocked": true})
	h.runAutomations(et.ws)
	if r := ruleRuns(et.owner, greet)[0]; r["error"] != "auto_reply" || !strings.Contains(r["detail"].(string), "blocked") {
		t.Fatalf("blocked %v", r)
	}

	if w := et.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil); w.body["bots_may_send"] != false {
		t.Fatalf("bots_may_send starts %v", w.body["bots_may_send"])
	}
	manager := h.client()
	manager.bearer = apiKey(et.owner, "manager", "automations:read", "automations:manage", "canned_replies:write")
	expectGreet := func(label, want string) {
		t.Helper()
		conv := et.conversation(et.owner)
		h.runAutomations(et.ws)
		r := ruleRuns(et.owner, greet)[0]
		if r["conversation_id"] != conv {
			t.Fatalf("%s: last run %v", label, r)
		}
		switch want {
		case "applied":
			if r["outcome"] != "applied" {
				t.Fatalf("%s: %v", label, r)
			}
		default:
			if r["error"] != "bot_sending_disabled" || !strings.Contains(r["detail"].(string), want) {
				t.Fatalf("%s: want refused (%s), got %v", label, want, r)
			}
		}
	}
	cannedBy := func() any {
		for _, c := range items(et.owner.expect(http.StatusOK, "GET", "/v1/canned-replies", nil)) {
			if c["id"] == canned {
				return c["saved_by"].(map[string]any)["type"]
			}
		}
		return nil
	}
	if got := cannedBy(); got != "member" {
		t.Fatalf("canned reply saved by %v", got)
	}
	expectGreet("owner's rule and canned reply", "applied")
	manager.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+greet, map[string]any{"conditions": []map[string]any{{"field": "subject", "operator": "contains", "value": "help"}}})
	expectGreet("a key changed the rule's conditions", "rule was last saved")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+greet, map[string]any{"name": "Greet, renamed by the owner"})
	expectGreet("the owner only renamed the key's rule", "rule was last saved")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	expectGreet("a key's rule while bots may send", "applied")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": false})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/automation-rules/"+greet, map[string]any{"actions": []map[string]any{
		{"type": "send_reply", "canned_reply_id": canned}, {"type": "set_priority", "priority": "high"},
	}})
	expectGreet("the owner edited the actions", "applied")
	manager.expect(http.StatusOK, "PATCH", "/v1/canned-replies/"+canned, map[string]any{"title": "Renamed by a key"})
	expectGreet("a key renamed the canned reply", "applied")
	manager.expect(http.StatusOK, "PATCH", "/v1/canned-replies/"+canned, map[string]any{"body": "Text from a key."})
	if got := cannedBy(); got != "api_key" {
		t.Fatalf("canned reply saved by %v", got)
	}
	expectGreet("a key edited the owner's canned reply", "canned reply was last saved")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/canned-replies/"+canned, map[string]any{"body": "We got it, the owner says."})
	expectGreet("the owner saved the text again", "applied")
}

func TestAutomationReplyDoesNotMeetSLA(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 60})
	canned := ct.owner.expect(http.StatusCreated, "POST", "/v1/canned-replies", map[string]any{"shortcut": "hi", "title": "Hi", "body": "Someone will be with you shortly."}).str("id")
	ct.rule(ct.owner, map[string]any{"name": "Greet", "trigger": "message_received", "actions": []map[string]any{{"type": "send_reply", "canned_reply_id": canned}}})
	cs := ct.session(h, map[string]any{})
	conv := cs.start("Hello?")
	if n := h.runAutomations(ct.ws); n != 1 {
		t.Fatalf("%d runs", n)
	}
	if n := len(byRule(messages(ct.owner, conv), "message")); n != 1 {
		t.Fatalf("%d rule replies", n)
	}
	if fr := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply"); fr["met_at"] != nil || fr["state"] != "on_track" {
		t.Fatalf("a rule reply met the first reply: %v", fr)
	}
	reply(ct.owner, conv, nil)
	if fr := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply"); fr["state"] != "met" {
		t.Fatalf("a member reply after the rule reply: %v", fr)
	}
}

func TestAutomationIsolation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	ctx := context.Background()
	billing := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	scoped := tm.rule(tm.owner, map[string]any{"name": "Billing only", "inbox_id": billing, "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "billing"}}})
	everywhere := tm.rule(tm.owner, map[string]any{"name": "All", "trigger": "conversation_created", "actions": []map[string]any{{"type": "set_priority", "priority": "high"}}})
	other.rule(other.owner, map[string]any{"name": "Theirs", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "theirs"}}})

	support := tm.conversation(tm.owner)
	bill := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": billing, "contact_id": tm.contact, "subject": "Invoice"}).str("id")
	h.runAutomations(tm.ws)
	if n := len(byRule(messages(tm.owner, support), "note")); n != 0 {
		t.Fatalf("an inbox rule acted on another inbox: %d notes", n)
	}
	if n := len(byRule(messages(tm.owner, bill), "note")); n != 1 {
		t.Fatalf("%d billing notes", n)
	}
	if runs := ruleRuns(tm.agent, everywhere); len(runs) != 1 || runs[0]["conversation_id"] != support {
		t.Fatalf("an agent without billing sees runs %v", runs)
	}
	if runs := ruleRuns(tm.owner, everywhere); len(runs) != 2 {
		t.Fatalf("the owner sees %d runs", len(runs))
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/automation-rules/"+scoped+"/runs", nil)

	theirs := other.conversation(other.owner)
	for _, j := range h.jobs("automation_run", other.ws) {
		h.dropJob(j.id)
	}
	if err := h.srv.RunAutomations(ctx, api.AutomationRunArgs{WorkspaceID: uuid.MustParse(tm.ws), ConversationID: uuid.MustParse(theirs), Trigger: "conversation_created", EventID: 1}); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.RunAutomations(ctx, api.AutomationRunArgs{WorkspaceID: uuid.MustParse(other.ws), ConversationID: uuid.MustParse(support), Trigger: "conversation_created", EventID: 1}); err != nil {
		t.Fatal(err)
	}
	if got := other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+theirs, nil).str("priority"); got != "normal" {
		t.Fatalf("another workspace's rule acted: priority %s", got)
	}
	if n := len(byRule(messages(tm.owner, support), "note")); n != 0 {
		t.Fatalf("a rule of another workspace wrote %d notes", n)
	}
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1", other.ws); n != 0 {
		t.Fatalf("%d runs in the other workspace", n)
	}
}

func TestAutomationIdleTrigger(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	ws := uuid.MustParse(tm.ws)
	nudge := tm.rule(tm.owner, map[string]any{"name": "Nudge", "trigger": "idle", "idle_minutes": 30,
		"conditions": []map[string]any{{"field": "status", "operator": "in", "values": []string{"open"}}},
		"actions":    []map[string]any{{"type": "add_note", "body": "Quiet for 30 minutes."}, {"type": "set_priority", "priority": "high"}}})
	conv := tm.conversation(tm.owner)
	closed := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+closed, map[string]any{"status": "closed"})
	reply(tm.owner, conv, map[string]any{"kind": "note"})
	var before time.Time
	if err := h.st.Pool.QueryRow(ctx, "SELECT last_activity_at FROM conversations WHERE id = $1", conv).Scan(&before); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(20 * time.Minute)
	if err := h.srv.SweepIdleAutomationsIn(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if n := len(h.automationJobs(tm.ws)); n != 0 {
		t.Fatalf("%d idle runs before 30 minutes", n)
	}
	h.clock.Advance(11 * time.Minute)
	if err := h.srv.SweepIdleAutomationsIn(ctx, ws); err != nil {
		t.Fatal(err)
	}
	jobs := h.automationJobs(tm.ws)
	if len(jobs) != 1 || jobs[0].ConversationID.String() != conv || jobs[0].IdleMinutes != 30 {
		t.Fatalf("idle jobs %+v", jobs)
	}
	h.runAutomations(tm.ws)
	if err := h.srv.SweepIdleAutomationsIn(ctx, ws); err != nil {
		t.Fatal(err)
	}
	h.runAutomations(tm.ws)
	if n := len(byRule(messages(tm.owner, conv), "note")); n != 1 {
		t.Fatalf("%d idle notes for one quiet period", n)
	}
	var after time.Time
	if err := h.st.Pool.QueryRow(ctx, "SELECT last_activity_at FROM conversations WHERE id = $1", conv).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatalf("the rule's note moved the last activity from %v to %v", before, after)
	}
	h.clock.Advance(40 * time.Minute)
	if err := h.srv.SweepIdleAutomationsIn(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if n := len(h.automationJobs(tm.ws)); n != 0 {
		t.Fatalf("the rule's own note started a new quiet period: %d jobs", n)
	}
	reply(tm.owner, conv, map[string]any{"kind": "note"})
	h.clock.Advance(31 * time.Minute)
	if err := h.srv.SweepIdleAutomationsIn(ctx, ws); err != nil {
		t.Fatal(err)
	}
	h.runAutomations(tm.ws)
	if n := len(byRule(messages(tm.owner, conv), "note")); n != 2 {
		t.Fatalf("%d idle notes after a second quiet period", n)
	}
	if n := len(ruleRuns(tm.owner, nudge)); n != 2 {
		t.Fatalf("%d runs", n)
	}
}

func TestAutomationRateLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	a := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "A"}).str("id")
	b := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "B"}).str("id")
	for i := range 10 {
		tm.rule(tm.owner, map[string]any{"name": fmt.Sprint("Rule ", i), "trigger": "conversation_created", "actions": []map[string]any{
			{"type": "add_labels", "label_ids": []string{a}}, {"type": "remove_labels", "label_ids": []string{b}}, {"type": "set_priority", "priority": "high"},
			{"type": "add_note", "body": "x"}, {"type": "assign_pool"}, {"type": "set_status", "status": "pending"},
		}})
	}
	for range 11 {
		tm.conversation(tm.owner)
	}
	jobs := h.automationJobs(tm.ws)
	for i, j := range jobs {
		err := h.srv.RunAutomations(ctx, j)
		var snooze *river.JobSnoozeError
		switch {
		case i < 10 && err != nil:
			t.Fatalf("job %d: %v", i, err)
		case i == 10 && !errors.As(err, &snooze):
			t.Fatalf("the 601st action in a minute ran: %v", err)
		}
	}
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1", tm.ws); n != 100 {
		t.Fatalf("%d runs", n)
	}
	h.clock.Advance(61 * time.Second)
	if err := h.srv.RunAutomations(ctx, jobs[10]); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM automation_runs WHERE workspace_id = $1", tm.ws); n != 110 {
		t.Fatalf("%d runs after the window", n)
	}
}

func TestAutomationWebhookAction(t *testing.T) {
	t.Parallel()
	h := privateHarness(t)
	tm := newTeam(t, h)
	rcv := newReceiver(t)
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL, "events": []string{"conversation.rated"}}).body["endpoint"].(map[string]any)["id"].(string)
	billing := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	limitedHook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL + "/billing", "inbox_id": billing, "events": []string{"conversation.rated"}}).body["endpoint"].(map[string]any)["id"].(string)
	send := tm.rule(tm.owner, map[string]any{"name": "Notify CRM", "trigger": "conversation_created", "actions": []map[string]any{
		{"type": "set_priority", "priority": "urgent"}, {"type": "send_webhook", "webhook_id": hook},
	}})
	wrong := tm.rule(tm.owner, map[string]any{"name": "Billing hook", "trigger": "conversation_created", "actions": []map[string]any{{"type": "send_webhook", "webhook_id": limitedHook}}})
	conv := tm.conversation(tm.owner)
	h.runAutomations(tm.ws)
	h.runWebhooks(tm.ws)
	got := rcv.take()
	var triggered []receivedHook
	for _, g := range got {
		if g.typ == "automation.triggered" {
			triggered = append(triggered, g)
		}
	}
	if len(triggered) != 1 {
		t.Fatalf("hooks %v", got)
	}
	d := triggered[0].data
	if d["rule"].(map[string]any)["id"] != send || d["trigger"] != "conversation_created" || d["conversation"].(map[string]any)["id"] != conv ||
		d["conversation"].(map[string]any)["priority"] != "urgent" || d["contact"].(map[string]any)["id"] != tm.contact {
		t.Fatalf("payload %v", d)
	}
	if r := ruleRuns(tm.owner, wrong); len(r) != 1 || r[0]["error"] != "webhook_unavailable" {
		t.Fatalf("an endpoint limited to another inbox %v", r)
	}
}

func TestAutomationRunsKept30Days(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	rule := tm.rule(tm.owner, map[string]any{"name": "Note", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "x"}}})
	tm.conversation(tm.owner)
	tm.conversation(tm.owner)
	h.runAutomations(tm.ws)
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE automation_runs SET created_at = $2 WHERE id = (SELECT id FROM automation_runs WHERE rule_id = $1 LIMIT 1)",
		rule, h.clock.Now().Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.CleanAutomationRuns(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(ruleRuns(tm.owner, rule)); n != 1 {
		t.Fatalf("%d runs after the cleanup", n)
	}
}

func TestAutomationIdleRunRechecksTheConversation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	nudge := tm.rule(tm.owner, map[string]any{"name": "Nudge", "trigger": "idle", "idle_minutes": 30, "actions": []map[string]any{{"type": "add_note", "body": "Still quiet."}}})
	replied, closed, snoozed, quiet := tm.conversation(tm.owner), tm.conversation(tm.owner), tm.conversation(tm.owner), tm.conversation(tm.owner)
	h.clock.Advance(31 * time.Minute)
	if err := h.srv.SweepIdleAutomationsIn(ctx, uuid.MustParse(tm.ws)); err != nil {
		t.Fatal(err)
	}
	if n := len(h.automationJobs(tm.ws)); n != 4 {
		t.Fatalf("%d idle jobs", n)
	}
	backend := h.client()
	backend.bearer = apiKey(tm.owner, "backend", "conversations:read", "messages:write")
	backend.expect(http.StatusCreated, "POST", "/v1/conversations/"+replied+"/messages", map[string]any{"kind": "message", "direction": "in", "body": "Hello again"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+closed, map[string]any{"status": "closed"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+snoozed, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(time.Hour)})
	for _, j := range h.jobs("automation_run", tm.ws) {
		var a api.AutomationRunArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		if a.Trigger != "idle" {
			h.dropJob(j.id)
		}
	}
	h.runAutomations(tm.ws)
	runs := ruleRuns(tm.owner, nudge)
	if len(runs) != 1 || runs[0]["conversation_id"] != quiet {
		t.Fatalf("idle runs %v", runs)
	}
	for _, conv := range []string{replied, closed, snoozed} {
		if n := len(byRule(messages(tm.owner, conv), "note")); n != 0 {
			t.Fatalf("a conversation that stopped being idle got %d notes", n)
		}
	}
}

func TestAutomationSnoozeWakeAndReplyCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	canned := tm.owner.expect(http.StatusCreated, "POST", "/v1/canned-replies", map[string]any{"shortcut": "back", "title": "Back", "body": "We are back on it."}).str("id")
	again := tm.rule(tm.owner, map[string]any{"name": "Reply and wait", "trigger": "status_changed",
		"conditions": []map[string]any{{"field": "status", "operator": "in", "values": []string{"open"}}},
		"actions":    []map[string]any{{"type": "send_reply", "canned_reply_id": canned}, {"type": "snooze", "snooze_minutes": 60}}})
	conv := tm.conversation(tm.owner)
	reopen := func() {
		t.Helper()
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"})
		h.runAutomations(tm.ws)
	}
	reopen()
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); got != "snoozed" {
		t.Fatalf("status %s", got)
	}
	h.clock.Advance(61 * time.Minute)
	if err := h.srv.WakeSnoozedIn(ctx, uuid.MustParse(tm.ws)); err != nil {
		t.Fatal(err)
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); got != "open" {
		t.Fatalf("after the wake %s", got)
	}
	if n := len(h.automationJobs(tm.ws)); n != 0 {
		t.Fatalf("the end of the rule's snooze queued %d runs", n)
	}
	for range 3 {
		h.clock.Advance(61 * time.Minute)
		reopen()
	}
	if n := len(byRule(messages(tm.owner, conv), "message")); n != 3 {
		t.Fatalf("%d rule replies in a day", n)
	}
	var last map[string]any
	for _, r := range ruleRuns(tm.owner, again) {
		if r["outcome"] != "not_matched" {
			last = r
			break
		}
	}
	if last["error"] != "auto_reply" || !strings.Contains(last["detail"].(string), "three") {
		t.Fatalf("the fourth reply in a day %v", last)
	}

	member := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+member, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(time.Minute)})
	for _, j := range h.jobs("automation_run", tm.ws) {
		h.dropJob(j.id)
	}
	h.clock.Advance(2 * time.Minute)
	if err := h.srv.WakeSnoozedIn(ctx, uuid.MustParse(tm.ws)); err != nil {
		t.Fatal(err)
	}
	if n := len(h.automationJobs(tm.ws)); n != 1 {
		t.Fatalf("the end of a member's snooze queued %d runs", n)
	}
}

func TestAutomationRetryKeepsEarlierDecisions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	vip := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	first := tm.rule(tm.owner, map[string]any{"name": "VIPs", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "label", "operator": "in", "values": []string{vip}}},
		"actions":    []map[string]any{{"type": "add_note", "body": "one"}}})
	tm.rule(tm.owner, map[string]any{"name": "Everyone", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "two"}}})
	third := tm.rule(tm.owner, map[string]any{"name": "Broken", "trigger": "conversation_created", "actions": []map[string]any{{"type": "add_note", "body": "three"}}})
	conv := tm.conversation(tm.owner)
	jobs := h.automationJobs(tm.ws)

	b := make([]byte, 6)
	_, _ = rand.Read(b)
	constraint := "automation_block_" + hex.EncodeToString(b)
	if _, err := h.st.Pool.Exec(ctx, fmt.Sprintf("ALTER TABLE automation_runs ADD CONSTRAINT %s CHECK (rule_id <> '%s') NOT VALID", constraint, third)); err != nil {
		t.Fatal(err)
	}
	dropped := false
	drop := func() {
		if !dropped {
			dropped = true
			if _, err := h.st.Pool.Exec(context.Background(), "ALTER TABLE automation_runs DROP CONSTRAINT "+constraint); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(drop)
	if err := h.srv.RunAutomations(ctx, jobs[0]); err == nil {
		t.Fatal("the third rule's database error must fail the job")
	}
	drop()
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"labels": []string{vip}})
	if err := h.srv.RunAutomations(ctx, jobs[0]); err != nil {
		t.Fatal(err)
	}
	var notes []string
	for _, m := range byRule(messages(tm.owner, conv), "note") {
		notes = append(notes, m["body"].(string))
	}
	slices.Sort(notes)
	if !slices.Equal(notes, []string{"three", "two"}) {
		t.Fatalf("notes after the retry %v", notes)
	}
	if r := ruleRuns(tm.owner, first); len(r) != 1 || r[0]["outcome"] != "not_matched" {
		t.Fatalf("the retry evaluated the first rule again: %v", r)
	}
}

func TestAutomationRateLimitPerRule(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	a := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "A"}).str("id")
	b := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "B"}).str("id")
	team := tm.owner.expect(http.StatusCreated, "POST", "/v1/teams", map[string]any{"name": "Support"}).str("id")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+team+"/inboxes/"+tm.inbox, nil)
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": "https://hooks.example.com/x", "events": []string{"conversation.rated"}}).body["endpoint"].(map[string]any)["id"].(string)
	for i := range 60 {
		tm.rule(tm.owner, map[string]any{"name": fmt.Sprint("Big ", i), "trigger": "conversation_created",
			"conditions": []map[string]any{{"field": "subject", "operator": "contains", "value": "big"}},
			"actions": []map[string]any{
				{"type": "add_labels", "label_ids": []string{a}}, {"type": "remove_labels", "label_ids": []string{b}}, {"type": "set_priority", "priority": "high"},
				{"type": "set_status", "status": "pending"}, {"type": "add_note", "body": "x"}, {"type": "unassign"}, {"type": "assign_team", "team_id": team},
				{"type": "send_webhook", "webhook_id": hook},
			}})
	}
	tm.rule(tm.owner, map[string]any{"name": "Small", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "subject", "operator": "contains", "value": "small"}},
		"actions":    []map[string]any{{"type": "add_labels", "label_ids": []string{a}}, {"type": "set_priority", "priority": "low"}, {"type": "add_note", "body": "s"}, {"type": "unassign"}, {"type": "set_status", "status": "pending"}}})
	tm.rule(tm.owner, map[string]any{"name": "Tiny", "trigger": "conversation_created",
		"conditions": []map[string]any{{"field": "subject", "operator": "contains", "value": "tiny"}},
		"actions":    []map[string]any{{"type": "add_labels", "label_ids": []string{b}}, {"type": "set_priority", "priority": "urgent"}, {"type": "add_note", "body": "t"}}})
	conv := func(subject string) api.AutomationRunArgs {
		t.Helper()
		id := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": subject}).str("id")
		for _, j := range h.jobs("automation_run", tm.ws) {
			var args api.AutomationRunArgs
			if err := json.Unmarshal(j.args, &args); err != nil {
				t.Fatal(err)
			}
			h.dropJob(j.id)
			if args.ConversationID.String() == id {
				return args
			}
		}
		t.Fatal("no job")
		return api.AutomationRunArgs{}
	}
	snoozed := func(err error) bool {
		var s *river.JobSnoozeError
		return errors.As(err, &s)
	}
	runs := func(c uuid.UUID) int {
		return h.count("SELECT count(*) FROM automation_runs WHERE conversation_id = $1 AND outcome = 'applied'", c)
	}
	for range 25 {
		if err := h.srv.RunAutomations(ctx, conv("small")); err != nil {
			t.Fatal(err)
		}
	}
	big := conv("big")
	if err := h.srv.RunAutomations(ctx, big); !snoozed(err) {
		t.Fatalf("480 actions after 125 must not all fit: %v", err)
	}
	if n := runs(big.ConversationID); n != 59 {
		t.Fatalf("%d big rules ran before the limit", n)
	}
	tiny := conv("tiny")
	if err := h.srv.RunAutomations(ctx, tiny); err != nil {
		t.Fatalf("the 3 actions left were burnt by the refused rule: %v", err)
	}
	if err := h.srv.RunAutomations(ctx, conv("small")); !snoozed(err) {
		t.Fatalf("a full window let a small job through: %v", err)
	}
	h.clock.Advance(61 * time.Second)
	if err := h.srv.RunAutomations(ctx, big); err != nil {
		t.Fatal(err)
	}
	if n := runs(big.ConversationID); n != 60 {
		t.Fatalf("%d big rules after the window", n)
	}
	second := conv("big")
	if err := h.srv.RunAutomations(ctx, second); err != nil {
		t.Fatalf("the rules that already ran were charged again: %v", err)
	}
	if n := runs(second.ConversationID); n != 60 {
		t.Fatalf("%d rules on the second big conversation", n)
	}
}

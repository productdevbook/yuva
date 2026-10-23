package api_test

import (
	"net/http"
	"testing"
)

func TestSendingSettingsAreAudited(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": false, "broadcasts_by_keys": true})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/workspace", map[string]any{"broadcasts_by_keys": false})

	entries := auditLog(t, tm.owner, "")
	bots := auditOf(entries, "workspace.bots_may_send_changed")
	if len(bots) != 2 {
		t.Fatalf("%d bots_may_send entries: %v", len(bots), bots)
	}
	if d := detailsOf(bots[0]); d["from"] != true || d["to"] != false {
		t.Fatalf("latest bots_may_send entry %v", d)
	}
	if d := detailsOf(bots[1]); d["from"] != false || d["to"] != true {
		t.Fatalf("first bots_may_send entry %v", d)
	}
	keys := oneAudit(t, entries, "workspace.broadcasts_by_keys_changed")
	if d := detailsOf(keys); d["from"] != false || d["to"] != true || targetOf(keys)["id"] != tm.ws {
		t.Fatalf("broadcasts_by_keys entry %v", keys)
	}
	theirs := auditLog(t, other.owner, "")
	if n := len(auditOf(theirs, "workspace.bots_may_send_changed")) + len(auditOf(theirs, "workspace.broadcasts_by_keys_changed")); n != 0 {
		t.Fatalf("%d entries in another workspace", n)
	}
}

func TestRuleRunsHideTrashedConversations(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	rule := tm.rule(tm.owner, map[string]any{"name": "Prioritise", "trigger": "conversation_created",
		"actions": []map[string]any{{"type": "set_priority", "priority": "high"}}})
	kept := tm.conversation(tm.owner)
	trashed := tm.conversation(tm.owner)
	h.runAutomations(tm.ws)
	if n := len(ruleRuns(tm.owner, rule)); n != 2 {
		t.Fatalf("%d runs before the trash", n)
	}
	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+trashed, nil)
	runs := ruleRuns(tm.owner, rule)
	if len(runs) != 1 || runs[0]["conversation_id"] != kept {
		t.Fatalf("runs with one trashed conversation: %v", runs)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+trashed+"/restore", nil)
	if n := len(ruleRuns(tm.owner, rule)); n != 2 {
		t.Fatalf("%d runs after the restore", n)
	}
}

package api_test

import (
	"net/http"
	"testing"
)

func TestWorkspaceFocus(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)

	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil); r.body["focus"] != nil {
		t.Fatalf("a new workspace has a focus: %s", r.raw)
	}

	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/workspace", map[string]any{"focus": "sales"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/workspace", map[string]any{"focus": "mail"})
	tm.apiKey(h).expectProblem(http.StatusForbidden, "member_session_required", "PATCH", "/v1/workspace", map[string]any{"focus": "mail"})

	if r := tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"focus": "mail"}); r.body["focus"] != "mail" {
		t.Fatalf("set focus: %s", r.raw)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"focus": "mail"})
	if r := tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"focus": "both"}); r.body["focus"] != "both" {
		t.Fatalf("change focus: %s", r.raw)
	}
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil); r.body["focus"] != "both" {
		t.Fatalf("focus not kept: %s", r.raw)
	}
	if r := other.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil); r.body["focus"] != nil {
		t.Fatalf("another workspace got the focus: %s", r.raw)
	}

	entries := auditOf(auditLog(t, tm.owner, ""), "workspace.focus_changed")
	if len(entries) != 2 {
		t.Fatalf("focus audit entries: %v", entries)
	}
	if d := detailsOf(entries[0]); d["from"] != "mail" || d["to"] != "both" {
		t.Fatalf("latest focus change: %v", d)
	}
	if d := detailsOf(entries[1]); d["from"] != nil || d["to"] != "mail" {
		t.Fatalf("first focus change: %v", d)
	}
}

func TestSetupMailItem(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	mailCount := func(c *client) [2]any {
		t.Helper()
		return checklist(t, homeOf(c))["mail"]
	}

	mail := mailInbox(tm.owner)
	mailInbox(tm.owner)
	if got := mailCount(tm.owner); got != [2]any{false, float64(0)} {
		t.Fatalf("mail inboxes without e-mail channels: %v", got)
	}
	email := func(inbox string) {
		t.Helper()
		tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
			"kind": "email", "name": "Mail", "email": map[string]any{
				"address": unique("me") + "@example.com",
				"smtp":    map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
			},
		})
	}
	email(tm.inbox)
	email(mail)
	email(mail)
	got := checklist(t, homeOf(tm.owner))
	if got["mail"] != [2]any{true, float64(1)} || got["email"] != [2]any{true, float64(3)} {
		t.Fatalf("setup after e-mail channels: %v", got)
	}
	if got := mailCount(other.owner); got != [2]any{false, float64(0)} {
		t.Fatalf("another workspace counts the mail inbox: %v", got)
	}

	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "analytics:read", "lists:read"}))
	state, errText := callTool(t, cs, "get_setup_state", map[string]any{})
	if errText != "" {
		t.Fatal(errText)
	}
	if state["focus"] != nil {
		t.Fatalf("get_setup_state focus before it is set: %v", state["focus"])
	}
	items := map[string]any{}
	for _, it := range state["setup"].([]any) {
		m := it.(map[string]any)
		items[m["id"].(string)] = m["count"]
	}
	if items["mail"] != float64(1) || items["email"] != float64(3) {
		t.Fatalf("get_setup_state setup: %v", state["setup"])
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"focus": "both"})
	if state, _ = callTool(t, cs, "get_setup_state", map[string]any{}); state["focus"] != "both" {
		t.Fatalf("get_setup_state focus: %v", state["focus"])
	}

	limited := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read"}))
	if state, _ = callTool(t, limited, "get_setup_state", map[string]any{}); state["setup"] != nil || state["focus"] != "both" {
		t.Fatalf("get_setup_state without every part: %v", state)
	}
}

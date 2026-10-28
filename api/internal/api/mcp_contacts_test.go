package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

var (
	teamReadTools     = []string{"list_members", "search_contacts", "list_contact_notes", "list_drafts", "list_discussions", "get_discussion"}
	teamWriteTools    = []string{"create_contact", "update_contact", "add_contact_note", "start_conversation"}
	teamManageTools   = []string{"create_label", "create_canned_reply", "update_canned_reply"}
	setupProductTools = []string{
		"create_status_page", "create_status_component", "create_status_monitor", "update_incident", "create_help_center",
		"create_help_collection", "create_changelog", "create_roadmap_board", "create_funnel", "analytics_retention",
	}
)

func apiKey(owner *client, name string, scopes ...string) string {
	return owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": name, "scopes": scopes}).str("secret")
}

func TestMCPTeamAndSetupToolsByRoleAndScope(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	full := []string{
		"inboxes:read", "conversations:read", "conversations:write", "messages:write", "contacts:read", "contacts:write", "lists:read",
		"labels:write", "canned_replies:write", "status:manage", "help:write", "roadmap:write",
		"analytics:read", "analytics:manage",
	}

	owner := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", full))
	tools := toolNames(t, owner)
	for _, name := range slices.Concat(teamReadTools, teamWriteTools, teamManageTools, []string{
		"list_contact_subscriptions", "create_status_page", "create_status_component", "create_status_monitor", "update_incident",
		"create_help_collection", "create_funnel", "analytics_retention",
	}) {
		if !slices.Contains(tools, name) {
			t.Fatalf("owner with every scope lacks %s: %v", name, tools)
		}
	}
	for _, name := range []string{"create_help_center", "create_changelog", "create_roadmap_board", "reply_to_discussion"} {
		if slices.Contains(tools, name) {
			t.Fatalf("an OAuth connection lists %s", name)
		}
	}

	key := h.mcpSession(apiKey(tm.owner, "setup", "inboxes:read", "help:publish", "changelog:publish", "roadmap:write"))
	tools = toolNames(t, key)
	for _, name := range []string{"create_help_center", "create_changelog", "create_roadmap_board"} {
		if !slices.Contains(tools, name) {
			t.Fatalf("a key with the scope lacks %s: %v", name, tools)
		}
	}

	agentScopes := []string{"inboxes:read", "conversations:read", "conversations:write", "messages:write", "contacts:read", "contacts:write", "lists:read", "analytics:read"}
	agent := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", agentScopes))
	tools = toolNames(t, agent)
	for _, name := range slices.Concat(teamReadTools, teamWriteTools) {
		if !slices.Contains(tools, name) {
			t.Fatalf("agent lacks %s: %v", name, tools)
		}
	}
	for _, name := range slices.Concat(teamManageTools, []string{"create_status_page", "create_status_component", "create_status_monitor", "create_funnel"}) {
		if slices.Contains(tools, name) {
			t.Fatalf("agent lists %s", name)
		}
	}
	members, errText := callTool(t, agent, "list_members", map[string]any{})
	if errText != "" || len(members["items"].([]any)) != 2 {
		t.Fatalf("list_members: %v %s", members, errText)
	}
	for _, m := range members["items"].([]any) {
		if _, ok := m.(map[string]any)["email"]; ok {
			t.Fatalf("an agent sees an address: %v", m)
		}
	}
	ownerMembers, _ := callTool(t, owner, "list_members", map[string]any{})
	if m := ownerMembers["items"].([]any)[0].(map[string]any); m["email"] == nil {
		t.Fatalf("an owner sees no address: %v", m)
	}

	readOnly := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "conversations:read", "contacts:read"}))
	tools = toolNames(t, readOnly)
	for _, name := range slices.Concat(teamWriteTools, teamManageTools, setupProductTools) {
		if slices.Contains(tools, name) {
			t.Fatalf("a read-only connection lists %s", name)
		}
	}
	if _, errText := callTool(t, readOnly, "create_contact", map[string]any{"name": "X"}); !strings.Contains(errText, "insufficient_scope") || !strings.Contains(errText, "contacts:write") {
		t.Fatalf("create_contact without contacts:write: %q", errText)
	}
}

func TestMCPStartConversationDraftsUnlessAssistantsMayDeliver(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	scopes := []string{"conversations:read", "conversations:write", "messages:write", "contacts:read"}
	start := map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "subject": "Welcome", "body": "Hello Ayşe", "send": true}

	cs := h.mcpSession(h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", scopes))
	out, errText := callTool(t, cs, "start_conversation", start)
	if errText != "" {
		t.Fatal(errText)
	}
	msg := out["message"].(map[string]any)
	if msg["draft"] != true || out["note"] == nil || out["conversation"].(map[string]any)["inbox_id"] != et.inbox {
		t.Fatalf("send:true with assistants not allowed to deliver: %v", out)
	}
	if jobs := h.jobs("email_send", et.ws); len(jobs) != 0 {
		t.Fatalf("a draft was mailed: %d jobs", len(jobs))
	}
	drafts, errText := callTool(t, cs, "list_drafts", map[string]any{"inbox_id": et.inbox})
	if errText != "" || drafts["total"] != 1.0 {
		t.Fatalf("list_drafts: %v %s", drafts, errText)
	}
	item := drafts["items"].([]any)[0].(map[string]any)
	if item["draft"].(map[string]any)["id"] != msg["id"] || item["conversation"].(map[string]any)["customer_content"].(map[string]any)["subject"] != "Welcome" {
		t.Fatalf("list_drafts item: %v", item)
	}
	if at := outsideCustomerContent(drafts, "Ayşe", ""); len(at) > 0 {
		t.Fatalf("a contact name outside customer_content at %v", at)
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	cs = h.mcpSession(h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", scopes))
	start["subject"] = "Second"
	if out, errText = callTool(t, cs, "start_conversation", start); errText != "" || out["message"].(map[string]any)["draft"] == true || out["note"] != nil {
		t.Fatalf("send:true with delivery allowed: %v %s", out, errText)
	}
	if jobs := h.jobs("email_send", et.ws); len(jobs) != 1 {
		t.Fatalf("e-mail jobs after a sent first message: %d", len(jobs))
	}
	start["send"] = false
	if out, errText = callTool(t, cs, "start_conversation", start); errText != "" || out["message"].(map[string]any)["draft"] != true || out["note"] != nil {
		t.Fatalf("send omitted with delivery allowed: %v %s", out, errText)
	}
	delete(start, "body")
	if _, errText = callTool(t, cs, "start_conversation", start); errText == "" {
		t.Fatal("start_conversation without a body")
	}

	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", testOrigin+"/mcp", scopes))
	start["body"] = "Hi"
	if _, errText = callTool(t, ocs, "start_conversation", start); !strings.Contains(errText, "not_found") {
		t.Fatalf("start_conversation in another workspace's inbox: %q", errText)
	}
}

func TestMCPUpdateAndDiscardDrafts(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	scopes := []string{"conversations:read", "conversations:write", "messages:write", "contacts:read"}
	cs := h.mcpSession(h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", scopes))
	readOnly := h.mcpSession(h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read"}))
	for _, name := range []string{"update_draft", "discard_draft"} {
		if !slices.Contains(toolNames(t, cs), name) || slices.Contains(toolNames(t, readOnly), name) {
			t.Fatalf("%s is listed only with messages:write", name)
		}
	}
	if _, errText := callTool(t, readOnly, "discard_draft", map[string]any{"message_id": "00000000-0000-0000-0000-000000000000"}); !strings.Contains(errText, "messages:write") {
		t.Fatalf("discard_draft without messages:write: %q", errText)
	}

	started, errText := callTool(t, cs, "start_conversation", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "subject": "Welcome", "body": "Hello Ayşe"})
	if errText != "" {
		t.Fatal(errText)
	}
	draftID := started["message"].(map[string]any)["id"].(string)
	if _, errText = callTool(t, cs, "update_draft", map[string]any{"message_id": draftID}); errText == "" {
		t.Fatal("update_draft with nothing to change")
	}
	updated, errText := callTool(t, cs, "update_draft", map[string]any{"message_id": draftID, "body": "Hello Ayşe, welcome aboard"})
	if errText != "" || updated["id"] != draftID || updated["draft"] != true {
		t.Fatalf("update_draft: %v %s", updated, errText)
	}
	drafts, _ := callTool(t, cs, "list_drafts", map[string]any{"inbox_id": et.inbox})
	if drafts["total"] != 1.0 || !strings.Contains(drafts["items"].([]any)[0].(map[string]any)["draft"].(map[string]any)["customer_content"].(map[string]any)["body"].(string), "welcome aboard") {
		t.Fatalf("list_drafts after update_draft: %v", drafts)
	}
	discarded, errText := callTool(t, cs, "discard_draft", map[string]any{"message_id": draftID})
	if errText != "" || discarded["discarded"] != true || discarded["message_id"] != draftID {
		t.Fatalf("discard_draft: %v %s", discarded, errText)
	}
	if drafts, _ = callTool(t, cs, "list_drafts", map[string]any{"inbox_id": et.inbox}); drafts["total"] != 0.0 {
		t.Fatalf("list_drafts after discard_draft: %v", drafts)
	}
	if _, errText = callTool(t, cs, "discard_draft", map[string]any{"message_id": draftID}); !strings.Contains(errText, "not_found") {
		t.Fatalf("discard_draft twice: %q", errText)
	}

	conv := et.conversation(et.owner)
	sent := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Sent already"}).str("id")
	for tool, args := range map[string]map[string]any{"update_draft": {"message_id": sent, "body": "x"}, "discard_draft": {"message_id": sent}} {
		if _, errText = callTool(t, cs, tool, args); !strings.Contains(errText, "not_a_draft") {
			t.Fatalf("%s on a sent message: %q", tool, errText)
		}
	}

	mine := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"dora@example.org"}, "subject": "Plan", "body": "First", "draft": true,
	}).body["message"].(map[string]any)["id"].(string)
	agent := h.mcpSession(h.oauthToken(et.agent, et.ws, "Claude Code", testOrigin+"/mcp", scopes))
	for tool, args := range map[string]map[string]any{"update_draft": {"message_id": mine, "body": "Stolen"}, "discard_draft": {"message_id": mine}} {
		if _, errText = callTool(t, agent, tool, args); !strings.Contains(errText, "not_found") {
			t.Fatalf("%s on another member's draft: %q", tool, errText)
		}
	}
	if out, errText := callTool(t, cs, "update_draft", map[string]any{"message_id": mine, "subject": "Plan B", "body": "Second"}); errText != "" || out["id"] != mine {
		t.Fatalf("update_draft on the author's own draft: %v %s", out, errText)
	}
	if _, errText = callTool(t, cs, "discard_draft", map[string]any{"message_id": mine}); errText != "" {
		t.Fatal(errText)
	}

	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", testOrigin+"/mcp", scopes))
	again, _ := callTool(t, cs, "start_conversation", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "subject": "Again", "body": "Hi"})
	foreign := again["message"].(map[string]any)["id"].(string)
	for tool, args := range map[string]map[string]any{"update_draft": {"message_id": foreign, "body": "x"}, "discard_draft": {"message_id": foreign}} {
		if _, errText = callTool(t, ocs, tool, args); !strings.Contains(errText, "not_found") {
			t.Fatalf("%s on another workspace's draft: %q", tool, errText)
		}
	}
	if drafts, _ = callTool(t, cs, "list_drafts", map[string]any{"inbox_id": et.inbox}); drafts["total"] != 1.0 {
		t.Fatalf("the other workspace changed a draft: %v", drafts)
	}
}

func TestMCPContactNotesLabelsAndCannedReplies(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	scopes := []string{"inboxes:read", "conversations:read", "contacts:read", "contacts:write", "lists:read", "labels:write", "canned_replies:write"}
	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", scopes))

	ct, errText := callTool(t, cs, "create_contact", map[string]any{
		"name": "Mehmet Kaya", "emails": []string{"mehmet@example.com"}, "attributes": map[string]any{"plan": "pro"},
	})
	if errText != "" {
		t.Fatal(errText)
	}
	id := ct["id"].(string)
	if at := outsideCustomerContent(ct, "mehmet@example.com", ""); len(at) > 0 {
		t.Fatalf("an address outside customer_content at %v", at)
	}
	if _, errText = callTool(t, cs, "create_contact", map[string]any{"emails": []string{"mehmet@example.com"}}); errText == "" {
		t.Fatal("a second contact with the same address")
	}
	up, errText := callTool(t, cs, "update_contact", map[string]any{"contact_id": id, "name": "Mehmet K.", "attributes": map[string]any{"plan": "team"}})
	cc := up["customer_content"].(map[string]any)
	if errText != "" || cc["name"] != "Mehmet K." || cc["attributes"].(map[string]any)["plan"] != "team" {
		t.Fatalf("update_contact: %v %s", up, errText)
	}
	if _, errText = callTool(t, cs, "update_contact", map[string]any{"contact_id": id}); !strings.Contains(errText, "at least one field") {
		t.Fatalf("update_contact without a field: %q", errText)
	}
	found, errText := callTool(t, cs, "search_contacts", map[string]any{"query": "Mehmet"})
	if errText != "" || len(found["items"].([]any)) != 1 {
		t.Fatalf("search_contacts: %v %s", found, errText)
	}

	note, errText := callTool(t, cs, "add_contact_note", map[string]any{"contact_id": id, "body": "Prefers phone calls"})
	if errText != "" || note["customer_content"].(map[string]any)["body"] != "Prefers phone calls" {
		t.Fatalf("add_contact_note: %v %s", note, errText)
	}
	notes, errText := callTool(t, cs, "list_contact_notes", map[string]any{"contact_id": id})
	if errText != "" || len(notes["items"].([]any)) != 1 {
		t.Fatalf("list_contact_notes: %v %s", notes, errText)
	}
	if at := outsideCustomerContent(notes, "Prefers phone calls", ""); len(at) > 0 {
		t.Fatalf("a note outside customer_content at %v", at)
	}
	if subs, errText := callTool(t, cs, "list_contact_subscriptions", map[string]any{"contact_id": id}); errText != "" || len(subs["items"].([]any)) != 0 {
		t.Fatalf("list_contact_subscriptions: %v %s", subs, errText)
	}

	label, errText := callTool(t, cs, "create_label", map[string]any{"name": "Billing", "color": "#ff0000"})
	if errText != "" || label["name"] != "Billing" || label["color"] != "#ff0000" {
		t.Fatalf("create_label: %v %s", label, errText)
	}
	if _, errText = callTool(t, cs, "create_label", map[string]any{"name": "Billing"}); !strings.Contains(errText, "409") && !strings.Contains(errText, "taken") && !strings.Contains(errText, "exists") {
		t.Fatalf("a duplicate label: %q", errText)
	}
	reply, errText := callTool(t, cs, "create_canned_reply", map[string]any{"title": "Greeting", "shortcut": "hello", "body": "Hi there"})
	if errText != "" || reply["shortcut"] != "hello" {
		t.Fatalf("create_canned_reply: %v %s", reply, errText)
	}
	upd, errText := callTool(t, cs, "update_canned_reply", map[string]any{"canned_reply_id": reply["id"], "body": "Hello!"})
	if errText != "" || upd["body"] != "Hello!" || upd["title"] != "Greeting" {
		t.Fatalf("update_canned_reply: %v %s", upd, errText)
	}
	if _, errText = callTool(t, cs, "update_canned_reply", map[string]any{"canned_reply_id": reply["id"]}); !strings.Contains(errText, "at least one field") {
		t.Fatalf("update_canned_reply without a field: %q", errText)
	}
	if disc, errText := callTool(t, cs, "list_discussions", map[string]any{}); errText != "" || len(disc["items"].([]any)) != 0 {
		t.Fatalf("list_discussions: %v %s", disc, errText)
	}

	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", testOrigin+"/mcp", scopes))
	for name, args := range map[string]map[string]any{
		"update_contact":      {"contact_id": id, "name": "Evil"},
		"add_contact_note":    {"contact_id": id, "body": "x"},
		"list_contact_notes":  {"contact_id": id},
		"update_canned_reply": {"canned_reply_id": reply["id"], "body": "x"},
	} {
		if _, errText := callTool(t, ocs, name, args); !strings.Contains(errText, "not_found") && !strings.Contains(errText, "gone") {
			t.Fatalf("%s from another workspace: %q", name, errText)
		}
	}
}

func TestMCPSetupOtherProducts(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	key := h.mcpSession(apiKey(tm.owner, "setup", "inboxes:read", "inboxes:manage", "status:read", "status:manage", "help:read", "help:write", "help:publish",
		"changelog:publish", "roadmap:write", "analytics:read", "analytics:manage"))

	page, errText := callTool(t, key, "create_status_page", map[string]any{"inbox_id": tm.inbox, "name": "Acme", "slug": slugOf("acme")})
	if errText != "" || page["published"] != false {
		t.Fatalf("create_status_page: %v %s", page, errText)
	}
	comp, errText := callTool(t, key, "create_status_component", map[string]any{"page_id": page["id"], "name": "API"})
	if errText != "" || comp["state"] != "operational" {
		t.Fatalf("create_status_component: %v %s", comp, errText)
	}
	mon, errText := callTool(t, key, "create_status_monitor", map[string]any{"page_id": page["id"], "component_id": comp["id"], "kind": "heartbeat", "name": "Nightly job"})
	if errText != "" || mon["token"] == "" || mon["token"] == nil || mon["state"] != "pending" {
		t.Fatalf("create_status_monitor: %v %s", mon, errText)
	}
	listed, _ := callTool(t, key, "list_monitors", map[string]any{"page_id": page["id"]})
	if at := outsideCustomerContent(listed, mon["token"].(string), ""); len(listed["items"].([]any)) != 1 || len(at) > 0 {
		t.Fatalf("list_monitors: %v", listed)
	}
	inc, errText := callTool(t, key, "create_incident", map[string]any{"page_id": page["id"], "title": "Slow API", "message": "Looking into it"})
	if errText != "" {
		t.Fatal(errText)
	}
	upd, errText := callTool(t, key, "update_incident", map[string]any{"page_id": page["id"], "incident_id": inc["id"], "title": "API errors", "impact": "major"})
	if errText != "" || upd["title"] != "API errors" || upd["impact"] != "major" || upd["draft"] != true {
		t.Fatalf("update_incident: %v %s", upd, errText)
	}
	if _, errText = callTool(t, key, "update_incident", map[string]any{"page_id": page["id"], "incident_id": inc["id"]}); !strings.Contains(errText, "at least one field") {
		t.Fatalf("update_incident without a field: %q", errText)
	}

	hc, errText := callTool(t, key, "create_help_center", map[string]any{"inbox_id": tm.inbox, "name": "Help", "slug": slugOf("help")})
	if errText != "" || hc["published"] != false {
		t.Fatalf("create_help_center: %v %s", hc, errText)
	}
	col, errText := callTool(t, key, "create_help_collection", map[string]any{"center_id": hc["id"], "name": "Getting started"})
	if errText != "" || len(col["names"].([]any)) != 1 {
		t.Fatalf("create_help_collection: %v %s", col, errText)
	}
	cl, errText := callTool(t, key, "create_changelog", map[string]any{"inbox_id": tm.inbox, "name": "What's new", "slug": slugOf("news")})
	if errText != "" || cl["published"] != false {
		t.Fatalf("create_changelog: %v %s", cl, errText)
	}
	chat, errText := callTool(t, key, "create_chat_channel", map[string]any{"inbox_id": tm.inbox, "name": "site", "origins": []string{"https://example.com"}})
	if errText != "" {
		t.Fatal(errText)
	}
	board, errText := callTool(t, key, "create_roadmap_board", map[string]any{"inbox_id": tm.inbox, "channel_id": chat["id"], "name": "Roadmap", "slug": slugOf("road")})
	if errText != "" || board["public"] != false {
		t.Fatalf("create_roadmap_board: %v %s", board, errText)
	}

	prop, errText := callTool(t, key, "create_property", map[string]any{"name": "example.com", "domains": []string{"example.com"}, "platforms": []string{"web"}})
	if errText != "" {
		t.Fatal(errText)
	}
	var goals []any
	for _, name := range []string{"Visit", "Signup"} {
		g, errText := callTool(t, key, "create_goal", map[string]any{"property_id": prop["id"], "name": name, "kind": "event", "event_name": strings.ToLower(name)})
		if errText != "" {
			t.Fatal(errText)
		}
		goals = append(goals, g["id"])
	}
	fn, errText := callTool(t, key, "create_funnel", map[string]any{"property_id": prop["id"], "name": "Signup", "steps": goals})
	if errText != "" || len(fn["steps"].([]any)) != 2 {
		t.Fatalf("create_funnel: %v %s", fn, errText)
	}
	ret, errText := callTool(t, key, "analytics_retention", map[string]any{"property_id": prop["id"], "grain": "week"})
	if errText != "" || ret["grain"] != "week" || ret["cohorts"] == nil {
		t.Fatalf("analytics_retention: %v %s", ret, errText)
	}

	other := newTeam(t, h)
	okey := h.mcpSession(apiKey(other.owner, "x", "inboxes:read", "status:manage", "help:write", "analytics:read", "analytics:manage"))
	for name, args := range map[string]map[string]any{
		"create_status_component": {"page_id": page["id"], "name": "Evil"},
		"update_incident":         {"page_id": page["id"], "incident_id": inc["id"], "title": "x"},
		"create_help_collection":  {"center_id": hc["id"], "name": "Evil", "locale": "en"},
		"create_funnel":           {"property_id": prop["id"], "name": "Evil", "steps": goals},
		"analytics_retention":     {"property_id": prop["id"]},
	} {
		if _, errText := callTool(t, okey, name, args); !strings.Contains(errText, "not_found") && !strings.Contains(errText, "gone") {
			t.Fatalf("%s from another workspace: %q", name, errText)
		}
	}
}

func slugOf(prefix string) string {
	s := strings.ToLower(unique(prefix))
	return s[:min(len(s), 24)]
}

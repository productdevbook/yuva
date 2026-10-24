package api_test

import (
	"net/http"
	"slices"
	"testing"
)

func page(c *client, path string) response {
	c.h.t.Helper()
	return c.expect(http.StatusOK, "GET", path, nil)
}

func pageItems(r response) []map[string]any {
	var out []map[string]any
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func memberID(c *client) string {
	c.h.t.Helper()
	return c.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
}

func TestListMentions(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	ownerID := memberID(tm.owner)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "my refund?"})
	first := post(tm.owner, conv, map[string]any{"kind": "note", "body": "can you  take\nthis?", "mentions": []string{tm.agentID}})
	post(tm.owner, conv, map[string]any{"kind": "note", "body": "nobody named"})
	second := post(tm.owner, conv, map[string]any{"kind": "note", "body": "and this", "mentions": []string{tm.agentID, ownerID}})
	post(tm.agent, conv, map[string]any{"kind": "note", "body": "me myself", "mentions": []string{tm.agentID, ownerID}})
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)

	r := page(tm.agent, "/v1/me/mentions")
	items := pageItems(r)
	if len(items) != 2 || items[0]["note"].(map[string]any)["id"] != second || items[1]["note"].(map[string]any)["id"] != first {
		t.Fatalf("mentions: %s", r.raw)
	}
	if r.body["unseen"] != float64(0) {
		t.Fatalf("writing in the conversation leaves mentions unseen: %s", r.raw)
	}

	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/unread", nil)
	third := post(tm.owner, conv, map[string]any{"kind": "note", "body": "one more", "mentions": []string{tm.agentID}})
	r = page(tm.agent, "/v1/me/mentions?limit=1")
	items = pageItems(r)
	note := items[0]["note"].(map[string]any)
	author := note["author"].(map[string]any)
	cv := items[0]["conversation"].(map[string]any)
	if len(items) != 1 || note["id"] != third || note["text"] != "one more" || author["type"] != "member" || author["member_id"] != ownerID ||
		items[0]["seen"] != false || r.body["unseen"] != float64(1) || r.str("next_cursor") == "" {
		t.Fatalf("first page: %s", r.raw)
	}
	if cv["id"] != conv || cv["subject"] != "Help" || cv["inbox_id"] != tm.inbox || cv["status"] != "open" ||
		cv["contact"].(map[string]any)["name"] != "Ayşe Yılmaz" {
		t.Fatalf("conversation summary: %v", cv)
	}
	r = page(tm.agent, "/v1/me/mentions?limit=1&cursor="+r.str("next_cursor"))
	if items = pageItems(r); len(items) != 1 || items[0]["note"].(map[string]any)["id"] != second || items[0]["seen"] != true {
		t.Fatalf("second page: %s", r.raw)
	}

	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	r = page(tm.agent, "/v1/me/mentions")
	if r.body["unseen"] != float64(0) || pageItems(r)[0]["seen"] != true {
		t.Fatalf("after reading: %s", r.raw)
	}
	owned := pageItems(page(tm.owner, "/v1/me/mentions"))
	if len(owned) != 1 || owned[0]["note"].(map[string]any)["author"].(map[string]any)["member_id"] != tm.agentID {
		t.Fatalf("owner's mentions: %v", owned)
	}

	key.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/mentions", nil)
	token := h.client()
	token.bearer = h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin, []string{"conversations:read"})
	token.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/mentions", nil)
	if n := len(pageItems(page(other.owner, "/v1/me/mentions"))); n != 0 {
		t.Fatalf("another workspace's mentions: %d", n)
	}
	intruder := *other.owner
	intruder.workspace = tm.ws
	intruder.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/me/mentions", nil)

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if r = page(tm.agent, "/v1/me/mentions"); len(pageItems(r)) != 0 || r.body["unseen"] != float64(0) {
		t.Fatalf("mentions in an inbox the agent left: %s", r.raw)
	}
}

func TestMarkMentionsSeen(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	c1 := tm.conversation(tm.owner)
	c2 := tm.conversation(tm.owner)
	c3 := tm.conversation(tm.owner)
	unread := func(id string) bool { return itemByID(listItems(tm.agent, ""), id)["unread"].(bool) }
	seen := func(note string) bool {
		for _, it := range pageItems(page(tm.agent, "/v1/me/mentions")) {
			if it["note"].(map[string]any)["id"] == note {
				return it["seen"].(bool)
			}
		}
		t.Fatalf("mention %s not listed", note)
		return false
	}

	post(key, c1, map[string]any{"kind": "message", "direction": "in", "body": "one"})
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c1+"/read", nil)
	post(key, c1, map[string]any{"kind": "message", "direction": "in", "body": "two"})
	post(key, c1, map[string]any{"kind": "message", "direction": "in", "body": "three"})
	n1 := post(tm.owner, c1, map[string]any{"kind": "note", "body": "one", "mentions": []string{tm.agentID}})
	n2 := post(tm.owner, c1, map[string]any{"kind": "note", "body": "two", "mentions": []string{tm.agentID}})
	post(key, c2, map[string]any{"kind": "message", "direction": "in", "body": "hello"})
	n3 := post(tm.owner, c2, map[string]any{"kind": "note", "body": "three", "mentions": []string{tm.agentID}})
	plain := post(tm.owner, c1, map[string]any{"kind": "note", "body": "no mention"})
	post(key, c3, map[string]any{"kind": "message", "direction": "in", "body": "hi"})
	n4 := post(tm.owner, c3, map[string]any{"kind": "note", "body": "four", "mentions": []string{tm.agentID}})
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c3+"/read", nil)
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c3+"/unread", nil)
	if r := page(tm.agent, "/v1/me/mentions"); r.body["unseen"] != float64(3) || !seen(n4) {
		t.Fatalf("before: %s", r.raw)
	}

	r := tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/"+n1+"/seen", nil)
	if r.body["marked"] != float64(1) || r.body["unseen"] != float64(2) {
		t.Fatalf("mark one: %s", r.raw)
	}
	if !seen(n1) || seen(n2) || !unread(c1) {
		t.Fatalf("mark one: n1 %v n2 %v unread %v", seen(n1), seen(n2), unread(c1))
	}
	r = tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/"+n1+"/seen", nil)
	if r.body["marked"] != float64(0) || r.body["unseen"] != float64(2) {
		t.Fatalf("mark again: %s", r.raw)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/mentions/"+plain+"/seen", nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/mentions/00000000-0000-4000-8000-000000000001/seen", nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/mentions/"+n2+"/seen", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/mentions/"+n2+"/seen", nil)
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/me/mentions/seen", nil)
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/me/mentions/"+n2+"/seen", nil)

	r = tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/seen", nil)
	if r.body["marked"] != float64(2) || r.body["unseen"] != float64(0) || !seen(n2) || !seen(n3) {
		t.Fatalf("mark all: %s", r.raw)
	}
	if !unread(c1) || !unread(c2) || !unread(c3) {
		t.Fatalf("mark all left unread: %v %v %v", unread(c1), unread(c2), unread(c3))
	}
	if r = tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/seen", nil); r.body["marked"] != float64(0) {
		t.Fatalf("mark all twice: %s", r.raw)
	}
	if r = other.owner.expect(http.StatusOK, "POST", "/v1/me/mentions/seen", nil); r.body["marked"] != float64(0) {
		t.Fatalf("another workspace: %s", r.raw)
	}

	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c3+"/read", nil)
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c3+"/unread", nil)
	tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/"+n4+"/seen", nil)
	if !unread(c3) {
		t.Fatal("mark seen cleared the marked-unread flag")
	}

	later := post(tm.owner, c2, map[string]any{"kind": "note", "body": "later", "mentions": []string{tm.agentID}})
	tm.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+c2+"/read", nil)
	if !seen(later) {
		t.Fatal("opening the conversation still marks its mentions seen")
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/mentions/"+later+"/seen", nil)
	if r = tm.agent.expect(http.StatusOK, "POST", "/v1/me/mentions/seen", nil); r.body["marked"] != float64(0) {
		t.Fatalf("mark all in a hidden inbox: %s", r.raw)
	}
}

func TestListDrafts(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	billing := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": billing, "contact_id": tm.contact, "subject": "Invoice"}).str("id")
	assistant := h.client()
	assistant.bearer = h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin, []string{"conversations:read", "messages:write"})

	post(tm.owner, conv, map[string]any{"kind": "message", "body": "sent already"})
	bot := post(key, conv, map[string]any{"kind": "message", "body": "a bot's draft", "draft": true})
	byAssistant := post(assistant, conv, map[string]any{"kind": "message", "body": "an assistant's draft", "draft": true})
	byMember := post(tm.agent, conv, map[string]any{"kind": "message", "body": "a member's draft", "draft": true})
	inBilling := post(tm.owner, hidden, map[string]any{"kind": "message", "body": "billing draft", "draft": true})

	ids := func(r response) []string {
		var out []string
		for _, it := range pageItems(r) {
			out = append(out, it["draft"].(map[string]any)["id"].(string))
		}
		return out
	}
	r := page(tm.owner, "/v1/drafts")
	if got := ids(r); !slices.Equal(got, []string{inBilling, byAssistant, bot}) || r.body["total"] != float64(3) {
		t.Fatalf("owner's drafts: %s", r.raw)
	}
	it := pageItems(r)[1]
	d, cv := it["draft"].(map[string]any), it["conversation"].(map[string]any)
	if d["draft"] != true || d["body"] != "an assistant's draft" || d["author"].(map[string]any)["via"] != "Claude Code" ||
		cv["id"] != conv || cv["inbox_id"] != tm.inbox || cv["contact"].(map[string]any)["id"] != tm.contact {
		t.Fatalf("draft item: %v", it)
	}
	for kind, want := range map[string][]string{"bot": {bot}, "assistant": {byAssistant}, "member": {inBilling}} {
		if r := page(tm.owner, "/v1/drafts?author="+kind); !slices.Equal(ids(r), want) || r.body["total"] != float64(len(want)) {
			t.Fatalf("author=%s: %s", kind, r.raw)
		}
	}
	if got := ids(page(tm.owner, "/v1/drafts?inbox_id="+billing)); !slices.Equal(got, []string{inBilling}) {
		t.Fatalf("inbox filter: %v", got)
	}
	r = page(tm.owner, "/v1/drafts?limit=2")
	if r = page(tm.owner, "/v1/drafts?limit=2&cursor="+r.str("next_cursor")); !slices.Equal(ids(r), []string{bot}) {
		t.Fatalf("second page: %s", r.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/drafts?author=robot", nil)

	if r := page(tm.agent, "/v1/drafts"); !slices.Equal(ids(r), []string{byMember, bot}) || r.body["total"] != float64(2) {
		t.Fatalf("agent's drafts: %s", r.raw)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/drafts?inbox_id="+billing, nil)
	if got := ids(page(key, "/v1/drafts?author=bot")); !slices.Equal(got, []string{bot}) {
		t.Fatalf("key's drafts: %v", got)
	}
	reader := h.client()
	reader.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "writer", "scopes": []string{"messages:write"}}).str("secret")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/drafts", nil)

	if n := len(pageItems(page(other.owner, "/v1/drafts"))); n != 0 {
		t.Fatalf("another workspace's drafts: %d", n)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/drafts?inbox_id="+tm.inbox, nil)
	intruder := *other.owner
	intruder.workspace = tm.ws
	intruder.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/drafts", nil)

	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/messages/"+byMember+"/send", nil)
	tm.agent.expect(http.StatusOK, "POST", "/v1/messages/"+byMember+"/send", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+bot, nil)
	if r := page(tm.owner, "/v1/drafts"); !slices.Equal(ids(r), []string{inBilling, byAssistant}) || r.body["total"] != float64(2) {
		t.Fatalf("after sending and discarding: %s", r.raw)
	}
}

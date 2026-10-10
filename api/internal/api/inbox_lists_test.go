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
	if got := ids(r); !slices.Equal(got, []string{inBilling, byMember, byAssistant, bot}) || r.body["total"] != float64(4) {
		t.Fatalf("owner's drafts: %s", r.raw)
	}
	it := pageItems(r)[2]
	d, cv := it["draft"].(map[string]any), it["conversation"].(map[string]any)
	if d["draft"] != true || d["body"] != "an assistant's draft" || d["author"].(map[string]any)["via"] != "Claude Code" ||
		cv["id"] != conv || cv["inbox_id"] != tm.inbox || cv["contact"].(map[string]any)["id"] != tm.contact {
		t.Fatalf("draft item: %v", it)
	}
	for kind, want := range map[string][]string{"bot": {bot}, "assistant": {byAssistant}, "member": {inBilling, byMember}} {
		if r := page(tm.owner, "/v1/drafts?author="+kind); !slices.Equal(ids(r), want) || r.body["total"] != float64(len(want)) {
			t.Fatalf("author=%s: %s", kind, r.raw)
		}
	}
	if got := ids(page(tm.owner, "/v1/drafts?inbox_id="+billing)); !slices.Equal(got, []string{inBilling}) {
		t.Fatalf("inbox filter: %v", got)
	}
	r = page(tm.owner, "/v1/drafts?limit=3")
	if r = page(tm.owner, "/v1/drafts?limit=3&cursor="+r.str("next_cursor")); !slices.Equal(ids(r), []string{bot}) {
		t.Fatalf("second page: %s", r.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/drafts?author=robot", nil)

	if r := page(tm.agent, "/v1/drafts"); !slices.Equal(ids(r), []string{byMember, byAssistant, bot}) || r.body["total"] != float64(3) {
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

	tm.owner.expect(http.StatusOK, "POST", "/v1/messages/"+byMember+"/send", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+bot, nil)
	if r := page(tm.owner, "/v1/drafts"); !slices.Equal(ids(r), []string{inBilling, byAssistant}) || r.body["total"] != float64(2) {
		t.Fatalf("after sending and discarding: %s", r.raw)
	}
}

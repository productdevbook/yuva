package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func contactIDs(r response) []string {
	var out []string
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func TestContactDirectory(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)

	known := tm.contact
	visitor := tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Visitor 7"}).str("id")
	quiet := tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Quiet", "emails": []string{"quiet@example.com"}}).str("id")

	open := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": known}).str("id")
	post(key, open, map[string]any{"kind": "message", "direction": "in", "body": "hello"})
	h.clock.Advance(90 * time.Second)
	post(tm.agent, open, map[string]any{"kind": "message", "body": "hi"})
	closed := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": hidden, "contact_id": visitor}).str("id")
	post(key, closed, map[string]any{"kind": "message", "direction": "in", "body": "private"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+closed, map[string]any{"status": "closed"})
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE conversations SET rating = 'good', rated_at = now() WHERE id = $1", closed); err != nil {
		t.Fatal(err)
	}

	has := func(ids []string, id string) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	ids := contactIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?kind=known", nil))
	if !has(ids, known) || !has(ids, quiet) || has(ids, visitor) {
		t.Fatalf("known %v", ids)
	}
	if ids := contactIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?kind=visitor", nil)); len(ids) != 1 || ids[0] != visitor {
		t.Fatalf("visitors %v", ids)
	}
	if ids := contactIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?has_open=true", nil)); len(ids) != 1 || ids[0] != known {
		t.Fatalf("with an open conversation %v", ids)
	}
	if ids := contactIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?has_open=false", nil)); has(ids, known) || !has(ids, visitor) || !has(ids, quiet) {
		t.Fatalf("without an open conversation %v", ids)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/contacts?kind=robot", nil)

	first := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?sort=last_seen&limit=1", nil)
	if ids := contactIDs(first); len(ids) != 1 || ids[0] != visitor {
		t.Fatalf("most recently active first %v", ids)
	}
	second := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts?sort=last_seen&limit=1&cursor="+first.str("next_cursor"), nil)
	if ids := contactIDs(second); len(ids) != 1 || ids[0] != known {
		t.Fatalf("second page %v", ids)
	}

	a := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+known, nil).body["activity"].(map[string]any)
	if a["conversations"] != float64(1) || a["open_conversations"] != float64(1) || a["last_seen_at"] == nil || a["last_conversation_at"] == nil {
		t.Fatalf("activity %v", a)
	}
	for _, it := range tm.owner.expect(http.StatusOK, "GET", "/v1/contacts", nil).body["items"].([]any) {
		c := it.(map[string]any)
		act := c["activity"].(map[string]any)
		if c["id"] == quiet && (act["conversations"] != float64(0) || act["last_seen_at"] != nil || act["last_conversation_at"] != nil) {
			t.Fatalf("a quiet contact %v", act)
		}
	}
	if ids := contactIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/contacts", nil)); has(ids, visitor) {
		t.Fatalf("the agent sees a contact of an inbox they cannot access: %v", ids)
	}

	sum := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+known+"/summary", nil)
	if sum.body["first_replies"] != float64(1) || sum.body["median_first_reply_seconds"] != float64(91) {
		t.Fatalf("summary %s", sum.raw)
	}
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+visitor+"/summary", nil).body["ratings"].(map[string]any); r["good"] != float64(1) || r["bad"] != float64(0) {
		t.Fatalf("ratings %v", r)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+visitor+"/summary", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+known+"/summary", nil)

	if n := len(listItems(tm.owner, "?contact_id="+known+"&status=open")); n != 1 {
		t.Fatalf("open conversations of the contact %d", n)
	}
	if n := len(listItems(tm.owner, "?contact_id="+known+"&status=closed")); n != 0 {
		t.Fatalf("closed conversations of the contact %d", n)
	}
	if n := len(listItems(tm.owner, "?contact_id="+visitor+"&status=closed")); n != 1 {
		t.Fatalf("closed conversations of the visitor %d", n)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations?contact_id="+known, nil)
}

func TestContactNotes(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.conversation(tm.owner)
	notes := "/v1/contacts/" + tm.contact + "/notes"

	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", notes, map[string]any{"body": "   "})
	byOwner := tm.owner.expect(http.StatusCreated, "POST", notes, map[string]any{"body": "VIP since 2021"})
	if a := byOwner.body["author"].(map[string]any); a["type"] != "member" || a["member_id"] != tm.ownerMemberID(t) || byOwner.str("contact_id") != tm.contact {
		t.Fatalf("note %s", byOwner.raw)
	}
	h.clock.Advance(time.Second)
	byAgent := tm.agent.expect(http.StatusCreated, "POST", notes, map[string]any{"body": "prefers e-mail"}).str("id")
	h.clock.Advance(time.Second)
	byKey := key.expect(http.StatusCreated, "POST", notes, map[string]any{"body": "plan: pro"})
	if a := byKey.body["author"].(map[string]any); a["type"] != "bot" || a["api_key_id"] == nil {
		t.Fatalf("bot note %s", byKey.raw)
	}

	page := tm.agent.expect(http.StatusOK, "GET", notes+"?limit=2", nil)
	items := page.body["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["body"] != "plan: pro" || page.str("next_cursor") == "" {
		t.Fatalf("first page %s", page.raw)
	}
	rest := tm.agent.expect(http.StatusOK, "GET", notes+"?limit=2&cursor="+page.str("next_cursor"), nil).body["items"].([]any)
	if len(rest) != 1 || rest[0].(map[string]any)["body"] != "VIP since 2021" {
		t.Fatalf("second page %v", rest)
	}

	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", notes, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", notes, map[string]any{"body": "x"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", notes+"/"+byAgent, nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", notes+"/"+byOwner.str("id"), nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", notes+"/"+byKey.str("id"), nil)
	key.expect(http.StatusNoContent, "DELETE", notes+"/"+byKey.str("id"), nil)
	tm.agent.expect(http.StatusNoContent, "DELETE", notes+"/"+byAgent, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", notes+"/"+byAgent, nil)
	if left := tm.owner.expect(http.StatusOK, "GET", notes, nil).body["items"].([]any); len(left) != 1 {
		t.Fatalf("left %v", left)
	}

	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	secret := tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Secret"}).str("id")
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": hidden, "contact_id": secret})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/contacts/"+secret+"/notes", map[string]any{"body": "hush"})
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+secret+"/notes", nil)
}

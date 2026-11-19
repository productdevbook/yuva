package api_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"
	"uuid"
)

func activityOf(c *client, query string) []map[string]any {
	c.h.t.Helper()
	var out []map[string]any
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/activity"+query, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func activityKinds(items []map[string]any) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it["kind"].(string)+" "+it["target"].(map[string]any)["title"].(string))
	}
	return out
}

func countIDs(r response, key string) map[string]any {
	out := map[string]any{}
	for _, it := range r.body[key].([]any) {
		m := it.(map[string]any)
		out[m["id"].(string)] = m["count"]
	}
	return out
}

func TestActivityPermissionsAndCursor(t *testing.T) {
	h := newHarness(t)
	ht := newHelpTeam(t, h)
	tm := ht.team
	other := newTeam(t, h)
	o := tm.owner
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	billing := o.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	at := func(minutes int) { h.clock.Set(start.Add(time.Duration(minutes) * time.Minute)) }

	at(0)
	support := o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "Login fails"}).str("id")
	invoice := o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": billing, "contact_id": tm.contact, "subject": "Invoice"}).str("id")
	o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "Still open"})
	o.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": billing, "contact_id": tm.contact, "subject": "Refund"})
	other.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+other.conversation(other.owner), map[string]any{"status": "closed"})

	counts := o.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil)
	if got := countIDs(counts, "unassigned_inboxes"); len(got) != 2 || got[tm.inbox] != float64(2) || got[billing] != float64(2) {
		t.Fatalf("owner's unassigned_inboxes: %s", counts.raw)
	}
	if got := countIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil), "unassigned_inboxes"); len(got) != 1 || got[tm.inbox] != float64(2) {
		t.Fatalf("agent's unassigned_inboxes: %v", got)
	}

	at(1)
	o.expect(http.StatusOK, "PATCH", "/v1/conversations/"+support, map[string]any{"status": "closed"})
	at(2)
	o.expect(http.StatusOK, "PATCH", "/v1/conversations/"+invoice, map[string]any{"status": "closed"})
	at(3)
	if _, err := h.st.Pool.Exec(context.Background(), `INSERT INTO messages (id, workspace_id, conversation_id, kind, author_type, event, created_at)
		SELECT $1, workspace_id, id, 'event', 'contact', '{"type": "rated", "rating": "good"}', $3 FROM conversations WHERE id = $2`,
		uuid.New(), support, h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	at(4)
	article, _ := ht.article(o, "en", "Resetting a password", "Steps.")
	ht.publish(o, article, "en")
	at(5)
	page := func(inbox string) string {
		return o.expect(http.StatusCreated, "POST", "/v1/status-pages", map[string]any{"inbox_id": inbox, "name": "Status", "slug": unique("st")}).str("id")
	}
	o.expect(http.StatusCreated, "POST", "/v1/status-pages/"+page(billing)+"/incidents", map[string]any{"title": "Payments down", "message": "Looking.", "impact": "major", "publish": true})
	at(6)
	supportPage := page(tm.inbox)
	o.expect(http.StatusCreated, "POST", "/v1/status-pages/"+supportPage+"/incidents", map[string]any{"title": "Login slow", "message": "Looking.", "impact": "minor", "publish": true})
	o.expect(http.StatusCreated, "POST", "/v1/status-pages/"+supportPage+"/incidents", map[string]any{"title": "Draft", "message": "Not yet."})
	broadcast := o.expect(http.StatusCreated, "POST", "/v1/broadcasts", map[string]any{"inbox_id": tm.inbox, "name": "November news"}).str("id")
	setBroadcastSent(t, h, broadcast, start.Add(7*time.Minute), map[string]any{"recipients": 3})
	at(8)

	all := activityOf(o, "?limit=50")
	want := []string{
		"broadcast.sent November news", "incident.updated Login slow", "incident.updated Payments down", "article.published Resetting a password",
		"conversation.rated Login fails", "conversation.closed Invoice", "conversation.closed Login fails",
	}
	if got := activityKinds(all); !slices.Equal(got, want) {
		t.Fatalf("owner's activity: %v", got)
	}
	ownerID := all[6]["actor"].(map[string]any)["member_id"]
	if all[6]["inbox_id"] != tm.inbox || all[6]["target"].(map[string]any)["id"] != support || ownerID == nil || all[6]["detail"] != nil ||
		all[4]["actor"] != nil || all[4]["detail"].(map[string]any)["rating"] != "good" ||
		all[3]["detail"].(map[string]any)["locale"] != "en" || all[3]["actor"].(map[string]any)["member_id"] != ownerID ||
		all[1]["detail"].(map[string]any)["state"] != "investigating" || all[1]["target"].(map[string]any)["parent_id"] == nil {
		t.Fatalf("owner's items: %v", all)
	}

	var paged []map[string]any
	cursor := ""
	for range 10 {
		r := o.expect(http.StatusOK, "GET", "/v1/activity?limit=2"+cursor, nil)
		for _, it := range r.body["items"].([]any) {
			paged = append(paged, it.(map[string]any))
		}
		next, ok := r.body["next_cursor"].(string)
		if !ok {
			break
		}
		cursor = "&cursor=" + next
	}
	if got := activityKinds(paged); !slices.Equal(got, want) {
		t.Fatalf("paged activity: %v", got)
	}

	if got := activityKinds(activityOf(tm.agent, "")); !slices.Equal(got, []string{
		"broadcast.sent November news", "incident.updated Login slow", "article.published Resetting a password",
		"conversation.rated Login fails", "conversation.closed Login fails",
	}) {
		t.Fatalf("agent's activity: %v", got)
	}
	if got := activityKinds(activityOf(o, "?inbox_id="+billing)); !slices.Equal(got, []string{"incident.updated Payments down", "conversation.closed Invoice"}) {
		t.Fatalf("billing activity: %v", got)
	}
	if got := activityKinds(activityOf(other.owner, "")); !slices.Equal(got, []string{"conversation.closed Help"}) {
		t.Fatalf("other workspace's activity: %v", got)
	}

	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/activity?inbox_id="+billing, nil)
	o.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/activity?inbox_id="+other.inbox, nil)
	tm.apiKey(h).expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/activity", nil)
	h.client().expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/activity", nil)
	for _, q := range []string{"?limit=0", "?limit=51", "?cursor=nonsense"} {
		o.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/activity"+q, nil)
	}
}

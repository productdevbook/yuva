package api_test

import (
	"fmt"
	"net/http"
	"sync"
	"slices"
	"strings"
	"testing"
)

func (tm team) newView(c *client, body map[string]any) response {
	c.h.t.Helper()
	return c.expect(http.StatusCreated, "POST", "/v1/views", body)
}

func viewsOf(c *client) map[string]map[string]any {
	c.h.t.Helper()
	out := map[string]map[string]any{}
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/views", nil).body["items"].([]any) {
		v := it.(map[string]any)
		out[v["id"].(string)] = v
	}
	return out
}

func viewCount(t *testing.T, c *client, id string) any {
	t.Helper()
	v, ok := viewsOf(c)[id]
	if !ok {
		t.Fatalf("view %s not listed", id)
	}
	return v["count"]
}

func TestSavedViewFiltersUseTheListParser(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "billing"}).str("id")
	tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "select", "options": []string{"free", "pro"}})

	for param, filters := range map[string]map[string]string{
		"colour":       {"colour": "red"},
		"cursor":       {"cursor": "abc"},
		"limit":        {"limit": "10"},
		"status":       {"status": "open,maybe"},
		"assignee":     {"assignee": "someone"},
		"inbox_id":     {"inbox_id": "not-a-uuid"},
		"spam":         {"spam": "perhaps"},
		"category":     {"kind": "conversation", "category": "bug"},
		"site":         {"site": "docs.example.com"},
		"team":         {"team": "theirs"},
		"field.nope":   {"field.nope": "x"},
		"field.plan":   {"field.plan": "enterprise"},
		"label_id":     {"label_id": tm.inbox},
		"contact_id":   {"contact_id": tm.ws},
		"trashed":      {"trashed": "true"},
		"q":            {"q": strings.Repeat("a", 201)},
		"channel_kind": {"channel_kind": "fax"},
		"sla_state":    {"sla_state": "late"},
		"sort":         {"sort": "oldest"},
	} {
		c := tm.owner
		if param == "trashed" {
			c = tm.agent
		}
		r := c.do("POST", "/v1/views", map[string]any{"visibility": "personal", "name": "Bad", "filters": filters})
		expectField(t, r, http.StatusBadRequest, "validation_failed", param)
	}

	r := tm.newView(tm.owner, map[string]any{"visibility": "personal", "name": " Billing ", "filters": map[string]string{
		"status": "open,pending", "label_id": strings.ToUpper(label), "q": "refund -spam", "field.plan": "pro", "assignee": "me", "team": "mine",
	}})
	f := r.body["filters"].(map[string]any)
	if r.str("name") != "Billing" || f["label_id"] != label || f["status"] != "open,pending" || f["field.plan"] != "pro" || r.str("owner_member_id") == "" {
		t.Fatalf("saved view: %s", r.raw)
	}
	if r.body["count"] != float64(0) || r.body["count_capped"] != false {
		t.Fatalf("count: %s", r.raw)
	}
	sla := tm.newView(tm.owner, map[string]any{"visibility": "personal", "name": "Breached", "filters": map[string]string{"sort": "sla_due", "sla_state": "breached,due_soon"}})
	if sla.body["count"] != float64(0) {
		t.Fatalf("sla view: %s", sla.raw)
	}
	expectField(t, tm.owner.do("PATCH", "/v1/views/"+r.str("id"), map[string]any{"filters": map[string]string{"pinned": "yes"}}), http.StatusBadRequest, "validation_failed", "pinned")
	upd := tm.owner.expect(http.StatusOK, "PATCH", "/v1/views/"+r.str("id"), map[string]any{"filters": map[string]string{"pinned": "true"}, "name": "Pinned"})
	if f := upd.body["filters"].(map[string]any); len(f) != 1 || f["pinned"] != "true" || upd.str("name") != "Pinned" {
		t.Fatalf("updated: %s", upd.raw)
	}
}

func TestSavedViewPermissions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	open := map[string]string{"status": "open"}

	personal := tm.newView(tm.agent, map[string]any{"visibility": "personal", "name": "Mine", "filters": map[string]string{"assignee": "me"}}).str("id")
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/views", map[string]any{"visibility": "workspace", "name": "x", "filters": open})
	shared := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "Open", "filters": open}).str("id")
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/views/"+shared, map[string]any{"name": "x"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/views/"+shared, nil)

	squad := tm.newTeamOf("Billing", nil, nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/views", map[string]any{"visibility": "team", "name": "x", "filters": open})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/views", map[string]any{"visibility": "workspace", "team_id": squad, "name": "x", "filters": open})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/views", map[string]any{"visibility": "team", "team_id": other.inbox, "name": "x", "filters": open})
	teamView := tm.newView(tm.owner, map[string]any{"visibility": "team", "team_id": squad, "name": "Billing queue", "filters": map[string]string{"team_id": squad}}).str("id")
	if _, ok := viewsOf(tm.agent)[teamView]; ok {
		t.Fatal("team view listed to a member outside the team")
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views/"+teamView, nil)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+squad+"/members/"+tm.agentID, nil)
	if _, ok := viewsOf(tm.agent)[teamView]; !ok {
		t.Fatal("team view not listed to its member")
	}
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/views/"+teamView, map[string]any{"name": "x"})

	if _, ok := viewsOf(tm.owner)[personal]; ok {
		t.Fatal("owner lists an agent's personal view")
	}
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/views/"+personal, map[string]any{"name": "x"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/views/"+personal, nil)

	key := tm.scopedKey(h, "conversations:read", "views:write")
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/views", map[string]any{"visibility": "personal", "name": "x", "filters": open})
	keyView := tm.newView(key, map[string]any{"visibility": "workspace", "name": "From the backend", "filters": open}).str("id")
	if v := viewsOf(key); len(v) != 2 || v[personal] != nil || v[teamView] != nil {
		t.Fatalf("key lists %v", v)
	}
	if r := key.expect(http.StatusOK, "GET", "/v1/views?team_id="+squad, nil); len(r.body["items"].([]any)) != 1 {
		t.Fatalf("key lists the team's views: %s", r.raw)
	}
	writeOnly := tm.scopedKey(h, "views:write")
	writeOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/views", map[string]any{"visibility": "workspace", "name": "oracle", "filters": map[string]string{"q": "secret"}})
	writeOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "PATCH", "/v1/views/"+keyView, map[string]any{"filters": map[string]string{"contact_id": tm.contact}})
	writeOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/views", nil)
	writeOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/views/"+keyView, nil)
	limited := h.client()
	limited.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "limited", "inbox_ids": []string{tm.inbox}, "scopes": []string{"conversations:read", "views:write"}}).str("secret")
	limited.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views/"+teamView, nil)
	limited.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views?team_id="+squad, nil)
	limited.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/views", map[string]any{"visibility": "team", "team_id": squad, "name": "x", "filters": open})
	if v := viewsOf(limited); v[teamView] != nil {
		t.Fatal("team view listed to a key limited to inboxes")
	}
	elsewhere := tm.newTeamOf("Elsewhere", nil, nil)
	foreign := tm.newView(tm.owner, map[string]any{"visibility": "team", "team_id": elsewhere, "name": "Not my team", "filters": open}).str("id")
	if _, ok := viewsOf(tm.owner)[foreign]; ok {
		t.Fatal("owner's rail lists a team the owner is not in")
	}
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/views?team_id="+elsewhere, nil); len(r.body["items"].([]any)) != 1 {
		t.Fatalf("owner lists another team's views: %s", r.raw)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views?team_id="+elsewhere, nil)
	readOnly := tm.scopedKey(h, "conversations:read")
	readOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/views", map[string]any{"visibility": "workspace", "name": "x", "filters": open})
	readOnly.expectProblem(http.StatusForbidden, "insufficient_scope", "DELETE", "/v1/views/"+keyView, nil)
	plain := tm.apiKey(h)
	plain.expect(http.StatusCreated, "POST", "/v1/views", map[string]any{"visibility": "workspace", "name": "Implied", "filters": open})

	for _, c := range []*client{other.owner, other.agent} {
		for _, id := range []string{personal, shared, teamView} {
			c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views/"+id, nil)
			c.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/views/"+id, map[string]any{"name": "x"})
			c.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/views/"+id, nil)
		}
		if len(viewsOf(c)) != 0 {
			t.Fatal("views of another workspace listed")
		}
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/views/order", map[string]any{"ids": []string{shared}})
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/views", map[string]any{"visibility": "team", "team_id": squad, "name": "x", "filters": open})

	second := tm.newView(tm.agent, map[string]any{"visibility": "personal", "name": "Second", "filters": open}).str("id")
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/views/order", map[string]any{"ids": []string{second, shared}})
	tm.agent.expect(http.StatusNoContent, "PUT", "/v1/views/order", map[string]any{"ids": []string{second, personal}})
	v := viewsOf(tm.agent)
	if v[second]["position"] != float64(0) || v[personal]["position"] != float64(1) {
		t.Fatalf("order: %v %v", v[second], v[personal])
	}
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/views/order", map[string]any{"ids": []string{second, second}})

	tm.agent.expect(http.StatusNoContent, "DELETE", "/v1/views/"+second, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/views/"+shared, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views/"+shared, nil)
}

func TestSavedViewLimitPerOwner(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	empty := map[string]string{}
	for i := range 50 {
		tm.newView(tm.agent, map[string]any{"visibility": "personal", "name": fmt.Sprintf("v%d", i), "filters": empty})
	}
	tm.agent.expectProblem(http.StatusForbidden, "view_limit", "POST", "/v1/views", map[string]any{"visibility": "personal", "name": "one more", "filters": empty})
	tm.newView(tm.owner, map[string]any{"visibility": "personal", "name": "owner's", "filters": empty})
	for i := range 50 {
		tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": fmt.Sprintf("w%d", i), "filters": empty})
	}
	tm.owner.expectProblem(http.StatusForbidden, "view_limit", "POST", "/v1/views", map[string]any{"visibility": "workspace", "name": "one more", "filters": empty})
	squad := tm.newTeamOf("Billing", []string{tm.agentID}, nil)
	tm.newView(tm.owner, map[string]any{"visibility": "team", "team_id": squad, "name": "team", "filters": empty})
	if n := len(viewsOf(tm.agent)); n != 101 {
		t.Fatalf("agent lists %d views, want all 101", n)
	}
}

func TestSavedViewCountsFollowTheCallersAccess(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	inbox2 := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Sales", "slug": "sales"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+inbox2+"/members/"+tm.agentID, nil)
	tm.conversation(tm.owner)
	hidden := tm.conversation(tm.owner)
	seen := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox2, "contact_id": tm.contact, "subject": "Quote"}).str("id")

	all := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "All", "filters": map[string]string{}}).str("id")
	support := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "Support", "filters": map[string]string{"inbox_id": tm.inbox}}).str("id")
	closed := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "Closed", "filters": map[string]string{"status": "closed"}}).str("id")
	mine := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "Mine", "filters": map[string]string{"assignee": "me"}}).str("id")

	if n := viewCount(t, tm.owner, all); n != float64(3) {
		t.Fatalf("owner counts %v", n)
	}
	if n := viewCount(t, tm.agent, all); n != float64(1) {
		t.Fatalf("agent counts %v of what it cannot see", n)
	}
	if n := viewCount(t, tm.agent, support); n != nil {
		t.Fatalf("agent counts an inbox it cannot see: %v", n)
	}
	if n := viewCount(t, tm.owner, support); n != float64(2) {
		t.Fatalf("owner counts support %v", n)
	}
	if n := viewCount(t, tm.owner, closed); n != float64(0) {
		t.Fatalf("closed view counts %v", n)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+hidden, map[string]any{"status": "closed"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+seen, map[string]any{"assignee_id": tm.agentID})
	if n := viewCount(t, tm.owner, all); n != float64(2) {
		t.Fatalf("owner counts %v after closing one", n)
	}
	if a, o := viewCount(t, tm.agent, mine), viewCount(t, tm.owner, mine); a != float64(1) || o != float64(0) {
		t.Fatalf("assignee=me counts agent %v owner %v", a, o)
	}
	key := tm.scopedKey(h, "conversations:read")
	if n := viewCount(t, key, mine); n != nil {
		t.Fatalf("key counts assignee=me: %v", n)
	}
	if got := tm.agent.expect(http.StatusOK, "GET", "/v1/views/"+all, nil); got.body["count"] != float64(1) {
		t.Fatalf("get view: %s", got.raw)
	}
}

func TestSavedViewFiltersDropDeletedReferences(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "billing"}).str("id")
	squad := tm.newTeamOf("Billing", nil, nil)
	field := tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})
	filters := map[string]string{"status": "open", "label_id": label, "team_id": squad, "field.plan": "pro"}
	view := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "Billing", "filters": filters}).str("id")
	teamView := tm.newView(tm.owner, map[string]any{"visibility": "team", "team_id": squad, "name": "Team", "filters": map[string]string{}}).str("id")

	otherLabel := other.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "billing"}).str("id")
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/views", map[string]any{"visibility": "personal", "name": "x", "filters": map[string]string{"label_id": label}})
	otherView := tm.newView(other.owner, map[string]any{"visibility": "workspace", "name": "Theirs", "filters": map[string]string{"label_id": otherLabel}}).str("id")
	if _, err := h.st.Pool.Exec(t.Context(), "UPDATE saved_views SET filters = jsonb_set(filters, '{label_id}', to_jsonb($1::text)) WHERE id = $2", label, otherView); err != nil {
		t.Fatal(err)
	}

	keys := func() []string {
		t.Helper()
		f := tm.owner.expect(http.StatusOK, "GET", "/v1/views/"+view, nil).body["filters"].(map[string]any)
		var out []string
		for k := range f {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/labels/"+label, map[string]any{"name": "invoices"})
	if k := keys(); len(k) != 4 {
		t.Fatalf("rename changed filters: %v", k)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/labels/"+label, nil)
	if k := keys(); slices.Contains(k, "label_id") || len(k) != 3 {
		t.Fatalf("after label delete: %v", k)
	}
	if f := other.owner.expect(http.StatusOK, "GET", "/v1/views/"+otherView, nil).body["filters"].(map[string]any); f["label_id"] != label {
		t.Fatalf("another workspace's view stripped: %v", f)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+squad, nil)
	if k := keys(); slices.Contains(k, "team_id") || len(k) != 2 {
		t.Fatalf("after team delete: %v", k)
	}
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/views/"+teamView, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+field, nil)
	if k := keys(); len(k) != 1 || k[0] != "status" {
		t.Fatalf("after field delete: %v", k)
	}
}

func TestSavedViewUpdateKeepsAConcurrentStrip(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	for round := range 5 {
		label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": fmt.Sprintf("l%d", round)}).str("id")
		view := tm.newView(tm.owner, map[string]any{"visibility": "workspace", "name": "v", "filters": map[string]string{"label_id": label, "status": "open"}}).str("id")
		var wg sync.WaitGroup
		statuses := make(chan int, 21)
		for i := range 20 {
			wg.Go(func() { statuses <- tm.owner.do("PATCH", "/v1/views/"+view, map[string]any{"name": fmt.Sprintf("n%d", i)}).status })
		}
		wg.Go(func() { statuses <- tm.owner.do("DELETE", "/v1/labels/"+label, nil).status })
		wg.Wait()
		close(statuses)
		for st := range statuses {
			if st != http.StatusOK && st != http.StatusNoContent {
				t.Fatalf("status %d", st)
			}
		}
		f := tm.owner.expect(http.StatusOK, "GET", "/v1/views/"+view, nil).body["filters"].(map[string]any)
		if _, ok := f["label_id"]; ok || f["status"] != "open" {
			t.Fatalf("round %d: a rename undid the strip: %v", round, f)
		}
	}
}

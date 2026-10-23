package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

type accessFlip struct {
	member, inbox string
	granted       bool
}

func (tm team) newTeamOf(name string, members, inboxes []string) string {
	id := tm.owner.expect(http.StatusCreated, "POST", "/v1/teams", map[string]any{"name": name}).str("id")
	for _, m := range members {
		tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+id+"/members/"+m, nil)
	}
	for _, in := range inboxes {
		tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+id+"/inboxes/"+in, nil)
	}
	return id
}

func (tm team) newInbox(name string) string {
	return tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": name, "slug": strings.ToLower(name)}).body["inbox"].(map[string]any)["id"].(string)
}

func lastEvent(c *client) int64 {
	return int64(c.expect(http.StatusOK, "GET", "/v1/events/latest", nil).body["id"].(float64))
}

func feed(c *client, after int64) []map[string]any {
	r := c.expect(http.StatusOK, "GET", fmt.Sprintf("/v1/events?after=%d&limit=500", after), nil)
	var out []map[string]any
	for _, e := range r.body["events"].([]any) {
		out = append(out, e.(map[string]any))
	}
	return out
}

func flipsAfter(c *client, after int64) []accessFlip {
	var out []accessFlip
	for _, e := range feed(c, after) {
		if e["type"] != "inbox_access.changed" {
			continue
		}
		d := e["data"].(map[string]any)
		out = append(out, accessFlip{d["member_id"].(string), d["inbox_id"].(string), d["granted"].(bool)})
	}
	return out
}

func itemIDs(r response) []string {
	var out []string
	for _, it := range r.body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	slices.Sort(out)
	return out
}

func sorted(v ...string) []string {
	slices.Sort(v)
	return v
}

func TestInboxAccessIsUnionOfGrantsAndTeams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := tm.newInbox("Billing")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	support := tm.newTeamOf("Support", []string{tm.agentID}, []string{tm.inbox, other})

	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)); !slices.Equal(got, sorted(tm.inbox, other)) {
		t.Fatalf("agent's inboxes %v", got)
	}
	members := tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+other+"/members", nil)
	m := memberIn(t, members, tm.agentID)
	if m["via"] != "team" || fmt.Sprint(m["team_ids"]) != "["+support+"]" || len(members.body["items"].([]any)) != 1 {
		t.Fatalf("inbox members through a team: %s", members.raw)
	}
	m = memberIn(t, tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox+"/members", nil), tm.agentID)
	if m["via"] != "direct" || fmt.Sprint(m["team_ids"]) != "["+support+"]" {
		t.Fatalf("inbox members with both: %v", m)
	}

	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": tm.agentID})

	after := lastEvent(tm.owner)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if f := flipsAfter(tm.owner, after); len(f) != 0 {
		t.Fatalf("revoking a grant a team also gives flipped access: %v", f)
	}
	tm.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	m = memberIn(t, tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox+"/members", nil), tm.agentID)
	if m["via"] != "team" {
		t.Fatalf("after the direct grant is gone: %v", m)
	}
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations", nil)); !slices.Equal(got, []string{conv}) {
		t.Fatalf("agent lists %v", got)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+support+"/inboxes/"+tm.inbox, nil)
	if f := flipsAfter(tm.owner, after); !slices.Equal(f, []accessFlip{{tm.agentID, tm.inbox, false}}) {
		t.Fatalf("flips %v", f)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
}

func TestTeamAssignment(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	billing := tm.newInbox("Billing")
	third := tm.newInbox("Third")
	both := tm.newTeamOf("Both", nil, []string{tm.inbox, billing})
	elsewhere := tm.newTeamOf("Elsewhere", nil, []string{billing})
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv

	tm.owner.expectProblem(http.StatusUnprocessableEntity, "team_access", "PATCH", path, map[string]any{"team_id": elsewhere})
	tm.owner.expectProblem(http.StatusUnprocessableEntity, "team_access", "PATCH", path, map[string]any{"team_id": "0190d0a6-0000-7000-8000-000000000000"})
	r := tm.agent.expect(http.StatusOK, "PATCH", path, map[string]any{"team_id": both, "assignee_id": tm.agentID})
	if r.str("team_id") != both || r.str("assignee_id") != tm.agentID {
		t.Fatalf("assigned: %s", r.raw)
	}
	r = tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"assignee_id": nil})
	if r.str("team_id") != both || r.body["assignee_id"] != nil {
		t.Fatalf("unassigning the member keeps the team: %s", r.raw)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?assignee=unassigned", nil)); !slices.Equal(got, []string{conv}) {
		t.Fatalf("a team-only conversation is unassigned: %v", got)
	}

	moved := tm.owner.expect(http.StatusOK, "POST", path+"/move", map[string]any{"inbox_id": billing})
	if moved.str("team_id") != both {
		t.Fatalf("move to an inbox the team sees dropped it: %s", moved.raw)
	}
	moved = tm.owner.expect(http.StatusOK, "POST", path+"/move", map[string]any{"inbox_id": third})
	if _, ok := moved.body["team_id"]; ok {
		t.Fatalf("move to an inbox the team does not see kept it: %s", moved.raw)
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/move", map[string]any{"inbox_id": tm.inbox})
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"team_id": both})
	r = tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"team_id": nil})
	if _, ok := r.body["team_id"]; ok {
		t.Fatalf("team_id null kept the team: %s", r.raw)
	}

	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"team_id": both})
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+both, nil)
	if c := tm.owner.expect(http.StatusOK, "GET", path, nil); c.body["team_id"] != nil {
		t.Fatalf("deleting the team kept it on the conversation: %s", c.raw)
	}
}

func TestTeamFiltersAndCounts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	mine := tm.newTeamOf("Mine", []string{tm.agentID}, []string{tm.inbox})
	theirs := tm.newTeamOf("Theirs", nil, []string{tm.inbox})
	inMine, inTheirs, none := tm.conversation(tm.owner), tm.conversation(tm.owner), tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+inMine, map[string]any{"team_id": mine})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+inTheirs, map[string]any{"team_id": theirs, "assignee_id": tm.agentID})

	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations?team=mine", nil)); !slices.Equal(got, []string{inMine}) {
		t.Fatalf("team=mine: %v", got)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?team_id="+theirs, nil)); !slices.Equal(got, []string{inTheirs}) {
		t.Fatalf("team_id: %v", got)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?assignee=unassigned", nil)); !slices.Equal(got, sorted(inMine, none)) {
		t.Fatalf("unassigned: %v", got)
	}
	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?team=mine", nil)); len(got) != 0 {
		t.Fatalf("owner in no team: %v", got)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?team=all", nil)
	tm.apiKey(h).expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?team=mine", nil)

	counts := tm.agent.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil)
	teams := map[string]float64{}
	for _, c := range counts.body["teams"].([]any) {
		teams[c.(map[string]any)["id"].(string)] = c.(map[string]any)["count"].(float64)
	}
	if counts.body["mine_teams"] != float64(1) || counts.body["mine"] != float64(1) || counts.body["unassigned"] != float64(2) ||
		len(teams) != 2 || teams[mine] != 1 || teams[theirs] != 1 {
		t.Fatalf("agent's counts: %s", counts.raw)
	}
	if c := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil); c.body["mine_teams"] != float64(0) {
		t.Fatalf("owner's counts: %s", c.raw)
	}
}

func TestTeamVisibilityAndPermissions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	billing := tm.newInbox("Billing")
	member := tm.newTeamOf("Member", []string{tm.agentID}, nil)
	sharing := tm.newTeamOf("Sharing", nil, []string{tm.inbox})
	hidden := tm.newTeamOf("Hidden", nil, []string{billing})

	if got := itemIDs(tm.owner.expect(http.StatusOK, "GET", "/v1/teams", nil)); !slices.Equal(got, sorted(member, sharing, hidden)) {
		t.Fatalf("owner's teams %v", got)
	}
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/teams", nil)); !slices.Equal(got, []string{member}) {
		t.Fatalf("agent without inboxes sees %v", got)
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/teams", nil)); !slices.Equal(got, sorted(member, sharing)) {
		t.Fatalf("agent with an inbox sees %v", got)
	}
	tm.agent.expect(http.StatusOK, "GET", "/v1/teams/"+sharing, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/teams/"+hidden, nil)

	limited := h.client()
	limited.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{
		"name": "limited", "scopes": []string{"inboxes:read", "conversations:read"}, "inbox_ids": []string{billing},
	}).str("secret")
	if got := itemIDs(limited.expect(http.StatusOK, "GET", "/v1/teams", nil)); !slices.Equal(got, []string{hidden}) {
		t.Fatalf("limited key sees %v", got)
	}
	limited.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/teams", map[string]any{"name": "x"})

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/teams", map[string]any{"name": "x"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/teams/"+member, map[string]any{"name": "x"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/teams/"+member, nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/teams/"+member+"/members/"+tm.agentID, nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/teams/"+member+"/inboxes/"+billing, nil)

	tm.owner.expectProblem(http.StatusConflict, "team_taken", "POST", "/v1/teams", map[string]any{"name": "Member"})
	tm.owner.expectProblem(http.StatusConflict, "team_taken", "PATCH", "/v1/teams/"+sharing, map[string]any{"name": "Member"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/teams", map[string]any{"name": " "})
	r := tm.owner.expect(http.StatusOK, "PATCH", "/v1/teams/"+sharing, map[string]any{"name": " Shared ", "description": "Front line"})
	if r.str("name") != "Shared" || r.str("description") != "Front line" || fmt.Sprint(r.body["inbox_ids"]) != "["+tm.inbox+"]" {
		t.Fatalf("updated team: %s", r.raw)
	}

	audit := tm.owner.expect(http.StatusOK, "GET", "/v1/audit-log?limit=100", nil)
	var actions []string
	for _, e := range audit.body["items"].([]any) {
		if a := e.(map[string]any)["action"].(string); strings.HasPrefix(a, "team.") {
			actions = append(actions, a)
		}
	}
	slices.Sort(actions)
	if want := []string{"team.created", "team.created", "team.created", "team.inbox_access_granted", "team.inbox_access_granted", "team.member_added"}; !slices.Equal(actions, want) {
		t.Fatalf("audit %v", actions)
	}
}

func TestTeamAccessRevocationEvents(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	billing := tm.newInbox("Billing")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	team := tm.newTeamOf("Team", nil, []string{tm.inbox, billing})
	agentWS := tm.agent.dial("")
	agentWS.ready()

	after := lastEvent(tm.owner)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+team+"/members/"+tm.agentID, nil)
	if f := flipsAfter(tm.owner, after); !slices.Equal(f, []accessFlip{{tm.agentID, billing, true}}) {
		t.Fatalf("joining flips %v", f)
	}
	var change map[string]any
	if err := json.Unmarshal(agentWS.nextOf("inbox_access.changed").Data, &change); err != nil || change["inbox_id"] != billing || change["granted"] != true {
		t.Fatalf("agent's realtime: %v %v", change, err)
	}
	if m := agentWS.nextOf("team.updated"); !strings.Contains(string(m.Data), tm.agentID) {
		t.Fatalf("team.updated: %s", m.Data)
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+team+"/members/"+tm.agentID, nil)

	after = lastEvent(tm.owner)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team+"/inboxes/"+tm.inbox, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team, nil)
	if f := flipsAfter(tm.owner, after); !slices.Equal(f, []accessFlip{{tm.agentID, tm.inbox, false}, {tm.agentID, billing, false}}) {
		t.Fatalf("revoking flips %v", f)
	}
	var types []string
	for _, e := range feed(tm.owner, after) {
		types = append(types, e["type"].(string))
	}
	if want := []string{"team.updated", "inbox_access.changed", "inbox_access.changed", "team.deleted"}; !slices.Equal(types, want) {
		t.Fatalf("events %v", types)
	}
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)); len(got) != 0 {
		t.Fatalf("agent keeps %v", got)
	}
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/teams/"+team, nil)
}

// An agent whose only grant comes through a team loses everything of that inbox the moment the
// team lets them go, on every surface.
func TestTeamRemovalLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	team := tm.newTeamOf("Support", []string{tm.agentID}, []string{tm.inbox})
	key := tm.apiKey(h)
	word := strings.ReplaceAll(unique("kiwi"), "-", "")
	conv := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "About " + word}).str("id")
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "help with " + word})
	mention := map[string]any{"kind": "note", "body": "look", "mentions": []string{tm.agentID}}
	mcp := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "conversations:read", "contacts:read"}))

	visible := func(want bool) {
		t.Helper()
		status := map[bool]int{true: http.StatusOK, false: http.StatusNotFound}[want]
		n := map[bool]int{true: 1, false: 0}[want]
		tm.agent.expect(status, "GET", "/v1/conversations/"+conv, nil)
		tm.agent.expect(status, "GET", "/v1/contacts/"+tm.contact, nil)
		if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations", nil)); len(got) != n {
			t.Fatalf("list %v", got)
		}
		if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/conversations?q="+word, nil)); len(got) != n {
			t.Fatalf("search %v", got)
		}
		if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/contacts?limit=100", nil)); slices.Contains(got, tm.contact) != want {
			t.Fatalf("contacts %v", got)
		}
		if c := tm.agent.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil); c.body["all"] != float64(n) || c.body["mine_teams"] != float64(0) {
			t.Fatalf("counts %s", c.raw)
		}
		if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)); len(got) != n {
			t.Fatalf("inboxes %v", got)
		}
		if inbox := homeBlock(homeOf(tm.agent), "inbox"); inbox["open"] != float64(n) {
			t.Fatalf("home %v", inbox)
		}
		var convEvents int
		for _, e := range feed(tm.agent, 0) {
			if e["conversation_id"] == conv {
				convEvents++
			}
		}
		if (convEvents > 0) != want {
			t.Fatalf("/v1/events has %d events of the conversation", convEvents)
		}
		found, errText := callTool(t, mcp, "search_conversations", map[string]any{"query": word})
		if errText != "" || len(found["items"].([]any)) != n {
			t.Fatalf("mcp search %v %s", found, errText)
		}
		if _, errText := callTool(t, mcp, "get_conversation", map[string]any{"conversation_id": conv}); (errText == "") != want {
			t.Fatalf("mcp get_conversation: %q", errText)
		}
		if teams, _ := callTool(t, mcp, "list_teams", nil); len(teams["items"].([]any)) != n {
			t.Fatalf("mcp list_teams %v", teams)
		}
		tm.owner.expect(map[bool]int{true: http.StatusCreated, false: http.StatusBadRequest}[want], "POST", "/v1/conversations/"+conv+"/messages", mention)
	}

	visible(true)
	ws := tm.agent.dial("")
	ws.ready()
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team+"/members/"+tm.agentID, nil)
	var change map[string]any
	if err := json.Unmarshal(ws.nextOf("inbox_access.changed").Data, &change); err != nil || change["inbox_id"] != tm.inbox || change["granted"] != false {
		t.Fatalf("realtime: %v %v", change, err)
	}
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "still there?"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "high"})
	if m := ws.nextOf("team.updated"); !strings.Contains(string(m.Data), `"removed_member_ids":["`+tm.agentID+`"]`) || strings.Contains(string(m.Data), tm.inbox) {
		t.Fatalf("removed member's team.updated: %s", m.Data)
	}
	ws.quiet(500*time.Millisecond, "member.presence", "viewing", "contact.presence")
	visible(false)
}

func TestTeamsAcrossWorkspaces(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := newTeam(t, h), newTeam(t, h)
	teamA := a.newTeamOf("Alpha", []string{a.agentID}, []string{a.inbox})
	teamB := b.newTeamOf("Beta", nil, []string{b.inbox})
	convB := b.conversation(b.owner)

	for _, req := range []struct{ method, path string }{
		{"GET", "/v1/teams/" + teamA}, {"PATCH", "/v1/teams/" + teamA}, {"DELETE", "/v1/teams/" + teamA},
		{"PUT", "/v1/teams/" + teamA + "/members/" + b.agentID}, {"PUT", "/v1/teams/" + teamA + "/inboxes/" + b.inbox},
	} {
		b.owner.expectProblem(http.StatusNotFound, "not_found", req.method, req.path, map[string]any{"name": "taken over"})
	}
	b.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/teams/"+teamB+"/members/"+a.agentID, nil)
	b.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/teams/"+teamB+"/inboxes/"+a.inbox, nil)
	b.owner.expectProblem(http.StatusUnprocessableEntity, "team_access", "PATCH", "/v1/conversations/"+convB, map[string]any{"team_id": teamA})
	if got := itemIDs(b.owner.expect(http.StatusOK, "GET", "/v1/teams", nil)); !slices.Equal(got, []string{teamB}) {
		t.Fatalf("workspace B lists %v", got)
	}
	if got := itemIDs(b.owner.expect(http.StatusOK, "GET", "/v1/conversations?team_id="+teamA, nil)); len(got) != 0 {
		t.Fatalf("workspace B filters by A's team: %v", got)
	}
	keyA := a.apiKey(h)
	keyA.workspace = b.ws
	keyA.expectProblem(http.StatusForbidden, "workspace_mismatch", "GET", "/v1/teams", nil)
	if r := a.owner.expect(http.StatusOK, "GET", "/v1/teams/"+teamA, nil); r.str("name") != "Alpha" {
		t.Fatalf("team A changed: %s", r.raw)
	}
}

func TestTeamDeleteAndRevokeClearConversations(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	billing := tm.newInbox("Billing")
	team := tm.newTeamOf("Team", nil, []string{tm.inbox, billing})
	support := tm.conversation(tm.owner)
	other := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": billing, "contact_id": tm.contact}).str("id")
	for _, c := range []string{support, other} {
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+c, map[string]any{"team_id": team})
	}
	timeline := func(c string) int { return len(messages(tm.owner, c)) }
	before := timeline(support)

	updated := func(after int64) []string {
		var out []string
		for _, e := range feed(tm.owner, after) {
			if e["type"] == "conversation.updated" {
				if _, ok := e["data"].(map[string]any)["team_id"]; ok {
					t.Fatalf("conversation.updated still names the team: %v", e)
				}
				out = append(out, e["conversation_id"].(string))
			}
		}
		return out
	}
	after := lastEvent(tm.owner)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team+"/inboxes/"+tm.inbox, nil)
	if got := updated(after); !slices.Equal(got, []string{support}) {
		t.Fatalf("revoke updated %v", got)
	}
	if c := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+support, nil); c.body["team_id"] != nil {
		t.Fatalf("revoke kept the team: %s", c.raw)
	}
	if c := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+other, nil); c.str("team_id") != team {
		t.Fatalf("revoke of another inbox cleared: %s", c.raw)
	}

	after = lastEvent(tm.owner)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team, nil)
	if got := updated(after); !slices.Equal(got, []string{other}) {
		t.Fatalf("delete updated %v", got)
	}
	if timeline(support) != before {
		t.Fatal("clearing the team wrote to the timeline")
	}
	tm.owner.expectProblem(http.StatusUnprocessableEntity, "team_access", "PATCH", "/v1/conversations/"+other, map[string]any{"team_id": team})
}

func TestTeamPayloadsShowOnlySeenInboxes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	billing := tm.newInbox("Billing")
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	hook := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": "https://hooks.example.com/teams", "events": []string{"team.updated"}, "inbox_id": billing,
	}).body["endpoint"].(map[string]any)["id"].(string)
	team := tm.newTeamOf("Team", nil, []string{tm.inbox, billing})

	if r := tm.agent.expect(http.StatusOK, "GET", "/v1/teams/"+team, nil); fmt.Sprint(r.body["inbox_ids"]) != "["+tm.inbox+"]" {
		t.Fatalf("agent sees %s", r.raw)
	}
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/teams/"+team, nil); len(r.body["inbox_ids"].([]any)) != 2 {
		t.Fatalf("owner sees %s", r.raw)
	}
	for _, e := range feed(tm.agent, 0) {
		if strings.HasPrefix(e["type"].(string), "team.") && strings.Contains(fmt.Sprint(e["data"]), billing) {
			t.Fatalf("agent's feed names the hidden inbox: %v", e)
		}
	}

	ws := tm.agent.dial("")
	ws.ready()
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/teams/"+team, map[string]any{"name": "Renamed"})
	if m := ws.nextOf("team.updated"); strings.Contains(string(m.Data), billing) || !strings.Contains(string(m.Data), tm.inbox) {
		t.Fatalf("realtime names the hidden inbox: %s", m.Data)
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+team+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/teams/"+team+"/members/"+tm.agentID, nil)
	var removed map[string]any
	for removed == nil || removed["removed_member_ids"] == nil {
		if err := json.Unmarshal(ws.nextOf("team.updated").Data, &removed); err != nil {
			t.Fatal(err)
		}
	}
	if fmt.Sprint(removed["removed_member_ids"]) != "["+tm.agentID+"]" || strings.Contains(fmt.Sprint(removed), billing) {
		t.Fatalf("removed member's team.updated: %v", removed)
	}

	h.runWebhooks(tm.ws)
	var payloads []string
	rows, err := h.st.Pool.Query(context.Background(), "SELECT payload FROM webhook_deliveries WHERE endpoint_id = $1", hook)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, p)
	}
	rows.Close()
	if len(payloads) == 0 {
		t.Fatal("no team.updated delivery to the inbox endpoint")
	}
	for _, p := range payloads {
		if strings.Contains(p, tm.inbox) || !strings.Contains(p, billing) {
			t.Fatalf("inbox endpoint payload: %s", p)
		}
	}
}

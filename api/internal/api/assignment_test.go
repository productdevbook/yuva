package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (tm team) newAgent(inboxes ...string) (*client, string) {
	tm.owner.h.t.Helper()
	email := unique("agent") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": email, "role": "agent"})
	c := tm.owner.h.client()
	c.signIn(email)
	id := c.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	for _, in := range inboxes {
		tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+in+"/members/"+id, nil)
	}
	return c, id
}

// online records a live connection of the member, as the realtime endpoint does.
func (h *harness) online(ws, member string) {
	h.t.Helper()
	if _, err := h.st.Pool.Exec(context.Background(), "INSERT INTO realtime_connections (id, workspace_id, member_id, seen_at) VALUES ($1, $2, $3, $4)",
		uuid.New(), ws, member, h.clock.Now()); err != nil {
		h.t.Fatal(err)
	}
}

// heartbeat marks every connection of the workspace seen now.
func (h *harness) heartbeat(ws string) {
	h.t.Helper()
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE realtime_connections SET seen_at = $2 WHERE workspace_id = $1", ws, h.clock.Now()); err != nil {
		h.t.Fatal(err)
	}
}

// runAssignments runs the queued auto_assign and assign_waiting jobs, as the job queue would.
func (h *harness) runAssignments(ws string) (assign, waiting int) {
	h.t.Helper()
	ctx := context.Background()
	for _, j := range h.jobs("auto_assign", ws) {
		var a api.AutoAssignArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.AutoAssign(ctx, a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
		assign++
	}
	for _, j := range h.jobs("assign_waiting", ws) {
		var a api.AssignWaitingArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.AssignWaiting(ctx, a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
		waiting++
	}
	return assign, waiting
}

func assigneeOf(c *client, conv string) string {
	r := c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if r.body["assignee_id"] == nil {
		return ""
	}
	return r.str("assignee_id")
}

func autoEvents(c *client, conv string) []map[string]any {
	var out []map[string]any
	for _, m := range messages(c, conv) {
		if ev, ok := m["event"].(map[string]any); ok && ev["auto"] == true {
			out = append(out, m)
		}
	}
	return out
}

func TestAssignmentSettings(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)

	tm.conversation(tm.owner)
	if n := len(h.jobs("auto_assign", tm.ws)) + len(h.jobs("automation_run", tm.ws)); n != 0 {
		t.Fatalf("assignment off queued %d jobs", n)
	}

	r := tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox, nil)
	if r.body["assignment_mode"] != "off" || r.body["assignment_cap"] != nil {
		t.Fatalf("default inbox %s", r.raw)
	}
	r = tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin", "assignment_cap": 3})
	if r.body["assignment_mode"] != "round_robin" || r.body["assignment_cap"] != 3.0 {
		t.Fatalf("patched inbox %s", r.raw)
	}
	r = tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_cap": nil})
	if r.body["assignment_mode"] != "round_robin" || r.body["assignment_cap"] != nil {
		t.Fatalf("cap cleared %s", r.raw)
	}
	for _, bad := range []map[string]any{{"assignment_mode": "fifo"}, {"assignment_cap": 0}, {"assignment_cap": 501}} {
		tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/inboxes/"+tm.inbox, bad)
	}
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "off"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "off"})
	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Sales", "slug": "sales", "assignment_mode": "least_open", "assignment_cap": 5})
	if in := created.body["inbox"].(map[string]any); in["assignment_mode"] != "least_open" || in["assignment_cap"] != 5.0 {
		t.Fatalf("created inbox %s", created.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes", map[string]any{"name": "X", "slug": "x", "assignment_cap": 0})

	after := lastEvent(tm.owner)
	team := tm.owner.expect(http.StatusCreated, "POST", "/v1/teams", map[string]any{"name": "Tier 1", "assignment_mode": "round_robin", "assignment_cap": 2})
	if team.body["assignment_mode"] != "round_robin" || team.body["assignment_cap"] != 2.0 {
		t.Fatalf("created team %s", team.raw)
	}
	id := team.str("id")
	if r := tm.owner.expect(http.StatusCreated, "POST", "/v1/teams", map[string]any{"name": "Tier 2"}); r.body["assignment_mode"] != "off" || r.body["assignment_cap"] != nil {
		t.Fatalf("default team %s", r.raw)
	}
	r = tm.owner.expect(http.StatusOK, "PATCH", "/v1/teams/"+id, map[string]any{"assignment_mode": "least_open", "assignment_cap": nil})
	if r.body["assignment_mode"] != "least_open" || r.body["assignment_cap"] != nil {
		t.Fatalf("patched team %s", r.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/teams/"+id, map[string]any{"assignment_cap": 501})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/teams", map[string]any{"name": "Tier 3", "assignment_mode": "random"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/teams/"+id, map[string]any{"assignment_mode": "off"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/teams/"+id, map[string]any{"assignment_mode": "off"})
	var updated bool
	for _, e := range feed(tm.owner, after) {
		if d, ok := e["data"].(map[string]any); ok && e["type"] == "team.updated" && d["assignment_mode"] == "least_open" {
			updated = true
		}
	}
	if !updated {
		t.Fatal("team.updated carries no assignment_mode")
	}
}

func TestAutoAssignRoundRobinAndEligibility(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	_, second := tm.newAgent(tm.inbox)
	away, awayID := tm.newAgent(tm.inbox)
	_, offlineID := tm.newAgent(tm.inbox)
	_, noAccessID := tm.newAgent()
	ownerID := tm.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	away.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	for _, m := range []string{tm.agentID, second, awayID, noAccessID, ownerID} {
		h.online(tm.ws, m)
	}
	h.online(tm.ws, offlineID)
	h.clock.Advance(time.Hour)
	h.heartbeat(tm.ws)
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE realtime_connections SET seen_at = $2 WHERE member_id = $1", offlineID, h.clock.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin"})

	first, last := min(tm.agentID, second), max(tm.agentID, second)
	after := lastEvent(tm.owner)
	var got []string
	var convs []string
	for range 4 {
		conv := tm.conversation(tm.owner)
		if n, _ := h.runAssignments(tm.ws); n != 1 {
			t.Fatalf("ran %d auto_assign jobs", n)
		}
		convs = append(convs, conv)
		got = append(got, assigneeOf(tm.owner, conv))
	}
	if !slices.Equal(got, []string{first, last, first, last}) {
		t.Fatalf("round robin gave %v, want %v", got, []string{first, last, first, last})
	}
	evs := autoEvents(tm.owner, convs[0])
	if len(evs) != 1 || evs[0]["event"].(map[string]any)["type"] != "assigned" || evs[0]["author"].(map[string]any)["type"] != "system" ||
		evs[0]["event"].(map[string]any)["assignee_id"] != first {
		t.Fatalf("auto events %v", evs)
	}
	var updated, assigned int
	for _, e := range feed(tm.owner, after) {
		d, _ := e["data"].(map[string]any)
		switch {
		case e["type"] == "conversation.updated" && d["assignee_id"] != nil:
			updated++
		case e["type"] == "message.created" && d["event"] != nil && d["event"].(map[string]any)["auto"] == true:
			assigned++
		}
	}
	if updated != 4 || assigned != 4 {
		t.Fatalf("feed: %d conversation.updated with an assignee, %d auto events", updated, assigned)
	}
	if n := len(h.jobs("notify", tm.ws)); n != 4 {
		t.Fatalf("%d notification jobs for 4 automatic assignments", n)
	}
}

func TestAutoAssignLeastOpen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	_, second := tm.newAgent(tm.inbox)
	for range 2 {
		conv := tm.conversation(tm.owner)
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": tm.agentID})
	}
	h.online(tm.ws, tm.agentID)
	h.online(tm.ws, second)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "least_open"})
	var got []string
	for range 3 {
		conv := tm.conversation(tm.owner)
		h.runAssignments(tm.ws)
		got = append(got, assigneeOf(tm.owner, conv))
	}
	if !slices.Equal(got, []string{second, second, tm.agentID}) {
		t.Fatalf("least open gave %v (first agent %s, second %s)", got, tm.agentID, second)
	}
}

func TestAutoAssignWaitsForMembersAndCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin", "assignment_cap": 1})

	first := tm.conversation(tm.owner)
	h.runAssignments(tm.ws)
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+first, nil)
	if got.body["assignee_id"] != nil || got.body["assign_waiting_since"] == nil {
		t.Fatalf("nobody online: %s", got.raw)
	}

	ws := tm.agent.dial("")
	ws.ready()
	if n := len(h.jobs("assign_waiting", tm.ws)); n != 1 {
		t.Fatalf("connecting queued %d assign_waiting jobs", n)
	}
	if _, n := h.runAssignments(tm.ws); n != 1 {
		t.Fatalf("ran %d assign_waiting jobs", n)
	}
	got = tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+first, nil)
	if got.str("assignee_id") != tm.agentID || got.body["assign_waiting_since"] != nil {
		t.Fatalf("after connecting: %s", got.raw)
	}

	second := tm.conversation(tm.owner)
	h.runAssignments(tm.ws)
	if a := assigneeOf(tm.owner, second); a != "" {
		t.Fatalf("over the cap the second went to %s", a)
	}
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+first, map[string]any{"status": "closed"})
	if n := len(h.jobs("assign_waiting", tm.ws)); n != 1 {
		t.Fatalf("closing queued %d assign_waiting jobs", n)
	}
	h.runAssignments(tm.ws)
	if a := assigneeOf(tm.owner, second); a != tm.agentID {
		t.Fatalf("after closing the first, the second went to %q", a)
	}

	third := tm.conversation(tm.owner)
	h.runAssignments(tm.ws)
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+second, map[string]any{"assignee_id": nil})
	h.runAssignments(tm.ws)
	if a := assigneeOf(tm.owner, third); a != "" {
		t.Fatalf("an away member got %s", third)
	}
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "auto"})
	if n := len(h.jobs("assign_waiting", tm.ws)); n != 1 {
		t.Fatalf("availability auto queued %d assign_waiting jobs", n)
	}
	h.runAssignments(tm.ws)
	if a, b := assigneeOf(tm.owner, second), assigneeOf(tm.owner, third); a != "" || b != tm.agentID {
		t.Fatalf("the waiting third should go to the agent and the second, unassigned by hand, stay: second %q, third %q", a, b)
	}
	ws.close()
}

func TestAutoAssignBusinessHours(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	h.clock.Set(time.Date(2026, 10, 12, 8, 55, 0, 0, time.UTC))
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{
		"assignment_mode": "round_robin", "timezone": "UTC",
		"business_hours": map[string]any{"enabled": true, "intervals": []map[string]any{{"day": "mon", "start": "09:00", "end": "17:00"}}},
	})
	h.online(tm.ws, tm.agentID)
	conv := tm.conversation(tm.owner)
	h.runAssignments(tm.ws)
	if err := h.srv.SweepAssignWaiting(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.runAssignments(tm.ws)
	if a := assigneeOf(tm.owner, conv); a != "" {
		t.Fatalf("assigned before hours to %s", a)
	}
	h.clock.Advance(10 * time.Minute)
	h.heartbeat(tm.ws)
	if err := h.srv.SweepAssignWaiting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, n := h.runAssignments(tm.ws); n != 1 {
		t.Fatalf("the sweep queued %d assign_waiting jobs", n)
	}
	if a := assigneeOf(tm.owner, conv); a != tm.agentID {
		t.Fatalf("hours opened, assignee %q", a)
	}
}

func TestAutoAssignReopen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	other, otherID := tm.newAgent(tm.inbox)
	h.online(tm.ws, tm.agentID)
	h.online(tm.ws, otherID)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin"})
	conv := tm.conversation(tm.owner)
	h.runAssignments(tm.ws)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": tm.agentID})
	ws := uuid.MustParse(tm.ws)

	wake := func() {
		t.Helper()
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(time.Minute).Format(time.RFC3339Nano)})
		h.clock.Advance(2 * time.Minute)
		h.heartbeat(tm.ws)
		if err := h.srv.WakeSnoozedIn(context.Background(), ws); err != nil {
			t.Fatal(err)
		}
		jobs := h.jobs("auto_assign", tm.ws)
		if len(jobs) != 1 {
			t.Fatalf("a reopen queued %d auto_assign jobs", len(jobs))
		}
		var a api.AutoAssignArgs
		_ = json.Unmarshal(jobs[0].args, &a)
		if !a.Reopen {
			t.Fatalf("reopen job %s", jobs[0].args)
		}
		h.runAssignments(tm.ws)
	}

	before := len(autoEvents(tm.owner, conv))
	wake()
	if a := assigneeOf(tm.owner, conv); a != tm.agentID || len(autoEvents(tm.owner, conv)) != before {
		t.Fatalf("an eligible assignee lost the reopened conversation to %q", a)
	}

	tm.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	wake()
	if a := assigneeOf(tm.owner, conv); a != otherID {
		t.Fatalf("an away assignee's reopened conversation went to %q", a)
	}
	evs := autoEvents(tm.owner, conv)
	if ev := evs[len(evs)-1]["event"].(map[string]any); ev["type"] != "assigned" || ev["previous_assignee_id"] != tm.agentID {
		t.Fatalf("reassigned with %v", ev)
	}

	other.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	wake()
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if got.body["assignee_id"] != nil || got.body["assign_waiting_since"] == nil {
		t.Fatalf("nobody eligible on reopen: %s", got.raw)
	}
	evs = autoEvents(tm.owner, conv)
	if ev := evs[len(evs)-1]["event"].(map[string]any); ev["type"] != "unassigned" || ev["previous_assignee_id"] != otherID {
		t.Fatalf("unassigned with %v", ev)
	}
}

func TestAutoAssignTeamPoolByRule(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	_, inTeam := tm.newAgent()
	team := tm.newTeamOf("Billing", []string{inTeam}, []string{tm.inbox})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/teams/"+team, map[string]any{"assignment_mode": "round_robin"})
	h.online(tm.ws, tm.agentID)
	h.online(tm.ws, inTeam)
	rule := tm.rule(tm.owner, map[string]any{"name": "To billing", "trigger": "conversation_created",
		"actions": []map[string]any{{"type": "assign_team", "team_id": team}, {"type": "assign_pool"}}})

	conv := tm.conversation(tm.owner)
	if n := len(h.jobs("auto_assign", tm.ws)); n != 0 {
		t.Fatalf("auto_assign queued before the rules ran: %d", n)
	}
	if runs := h.automationJobs(tm.ws); len(runs) != 1 || !runs[0].Assign {
		t.Fatalf("automation jobs %+v", runs)
	}
	h.runAutomations(tm.ws)
	if n, _ := h.runAssignments(tm.ws); n != 1 {
		t.Fatalf("the rules queued %d auto_assign jobs", n)
	}
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if got.str("team_id") != team || got.str("assignee_id") != inTeam {
		t.Fatalf("team pool: %s (team member %s, inbox member %s)", got.raw, inTeam, tm.agentID)
	}
	runs := ruleRuns(tm.owner, rule)
	if acts := runs[0]["actions"].([]any); acts[1].(map[string]any)["result"] != "applied" {
		t.Fatalf("assign_pool result %v", acts)
	}
}

func TestAutoAssignAfterRules(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	_, second := tm.newAgent(tm.inbox)
	h.online(tm.ws, second)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "round_robin"})
	tm.rule(tm.owner, map[string]any{"name": "To the agent", "trigger": "conversation_created",
		"actions": []map[string]any{{"type": "assign_member", "member_id": tm.agentID}}})
	conv := tm.conversation(tm.owner)
	if n := len(h.jobs("auto_assign", tm.ws)); n != 0 {
		t.Fatalf("auto_assign queued beside the rule: %d", n)
	}
	h.runAutomations(tm.ws)
	if n, _ := h.runAssignments(tm.ws); n != 1 {
		t.Fatalf("ran %d auto_assign jobs after the rules", n)
	}
	if a := assigneeOf(tm.owner, conv); a != tm.agentID {
		t.Fatalf("automatic assignment overrode the rule: %q", a)
	}
}

func TestAutoAssignParallelCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	_, b := tm.newAgent(tm.inbox)
	_, c := tm.newAgent(tm.inbox)
	for _, m := range []string{tm.agentID, b, c} {
		h.online(tm.ws, m)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+tm.inbox, map[string]any{"assignment_mode": "least_open", "assignment_cap": 2})

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			r := tm.owner.do("POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "Rush"})
			if r.status != http.StatusCreated {
				errs <- fmt.Errorf("create: %d %s", r.status, r.raw)
			}
		})
	}
	wg.Wait()
	jobs := h.jobs("auto_assign", tm.ws)
	if len(jobs) != n {
		t.Fatalf("%d auto_assign jobs for %d conversations", len(jobs), n)
	}
	for _, j := range jobs {
		wg.Go(func() {
			var a api.AutoAssignArgs
			if err := json.Unmarshal(j.args, &a); err != nil {
				errs <- err
				return
			}
			if err := h.srv.AutoAssign(context.Background(), a); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	counts := map[string]int{}
	rows, err := h.st.Pool.Query(context.Background(),
		"SELECT coalesce(assignee_id::text, ''), assign_waiting_since IS NOT NULL FROM conversations WHERE workspace_id = $1", tm.ws)
	if err != nil {
		t.Fatal(err)
	}
	waiting := 0
	for rows.Next() {
		var who string
		var wait bool
		if err := rows.Scan(&who, &wait); err != nil {
			t.Fatal(err)
		}
		counts[who]++
		if wait {
			waiting++
		}
	}
	rows.Close()
	for _, m := range []string{tm.agentID, b, c} {
		if counts[m] != 2 {
			t.Fatalf("member %s holds %d (cap 2): %v", m, counts[m], counts)
		}
	}
	if counts[""] != n-6 || waiting != n-6 {
		t.Fatalf("%d unassigned, %d waiting, want %d", counts[""], waiting, n-6)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

type slaJob struct {
	id   int64
	at   time.Time
	args api.SLACheckArgs
}

func (h *harness) slaChecks(ws string) []slaJob {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(),
		"SELECT id, scheduled_at, args FROM river_job WHERE kind = 'sla_check' AND args->>'workspace_id' = $1 ORDER BY scheduled_at, id", ws)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []slaJob
	for rows.Next() {
		var j slaJob
		var raw []byte
		if err := rows.Scan(&j.id, &j.at, &raw); err != nil {
			h.t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &j.args); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

// runSLAChecks runs the SLA checks whose time has come, as the job queue would, and drops them.
func (h *harness) runSLAChecks(ws string) int {
	h.t.Helper()
	n := 0
	for _, j := range h.slaChecks(ws) {
		if j.at.After(h.clock.Now()) {
			continue
		}
		if err := h.srv.CheckSLA(context.Background(), j.args); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
		n++
	}
	return n
}

func (h *harness) runSLANotifications(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("sla_notify", ws) {
		var a api.SLANotifyArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.NotifySLA(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (h *harness) slaOutcomes(conv string) []string {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(), "SELECT target || ':' || outcome FROM sla_outcomes WHERE conversation_id = $1 ORDER BY at, target", conv)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func slaOf(t *testing.T, c *client, conv string) map[string]any {
	t.Helper()
	sla, _ := c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).body["sla"].(map[string]any)
	return sla
}

func slaTarget(t *testing.T, sla map[string]any, name string) map[string]any {
	t.Helper()
	for _, it := range sla["targets"].([]any) {
		if m := it.(map[string]any); m["target"] == name {
			return m
		}
	}
	t.Fatalf("no %s target in %v", name, sla)
	return nil
}

func slaTime(t *testing.T, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("not a time: %v", v)
	}
	r, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func near(t *testing.T, what string, got any, want time.Time) {
	t.Helper()
	v, ok := got.(time.Time)
	if !ok {
		v = slaTime(t, got)
	}
	if d := v.Sub(want); d < -time.Second || d > time.Second {
		t.Fatalf("%s is %v, want %s", what, got, want)
	}
}

func (ct chatTeam) policy(body map[string]any) string {
	return ct.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", body).str("id")
}

func (cs contactSession) say(conv, body string) {
	cs.h.t.Helper()
	r := cs.do("POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": body, "client_id": unique("c")})
	if r.status != http.StatusCreated && r.status != http.StatusOK {
		cs.h.t.Fatalf("contact message: %d %s", r.status, r.raw)
	}
}

func reply(c *client, conv string, extra map[string]any) {
	c.h.t.Helper()
	body := map[string]any{"kind": "message", "body": "On it."}
	for k, v := range extra {
		body[k] = v
	}
	c.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", body)
}

func eventsOf(c *client, after int64, typ string) []map[string]any {
	var out []map[string]any
	for _, e := range feed(c, after) {
		if e["type"] == typ {
			out = append(out, e)
		}
	}
	return out
}

func TestSLAPolicyEndpoints(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	otherLabel := other.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")

	first := tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{
		"name": "VIP", "label_ids": []string{label}, "priorities": []string{"high", "urgent", "high"}, "channel_kinds": []string{"chat"},
		"first_reply_minutes": 30, "resolution_minutes": 480,
	})
	if first.body["position"].(float64) != 0 || first.body["warn_minutes"].(float64) != 15 || first.body["enabled"] != true ||
		fmt.Sprint(first.body["priorities"]) != "[high urgent]" || first.body["next_reply_minutes"] != nil {
		t.Fatalf("created %s", first.raw)
	}
	id := first.str("id")
	second := tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{
		"name": "Everyone", "inbox_ids": []string{tm.inbox}, "next_reply_minutes": 60, "warn_minutes": 0,
	}).str("id")

	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": "No target"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 0})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 5, "warn_minutes": 1441})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 5, "label_ids": []string{otherLabel}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 5, "inbox_ids": []string{other.inbox}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sla-policies", map[string]any{"name": " ", "first_reply_minutes": 5})

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 5})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/sla-policies/"+id, map[string]any{"name": "y"})
	if got := itemIDs(tm.agent.expect(http.StatusOK, "GET", "/v1/sla-policies", nil)); !slices.Equal(got, []string{id}) {
		t.Fatalf("an agent without the inbox reads %v", got)
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/sla-policies/"+second, nil)
	reader := h.client()
	reader.bearer = apiKey(tm.owner, "reader", "inboxes:read")
	reader.expect(http.StatusOK, "GET", "/v1/sla-policies/"+id, nil)
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "DELETE", "/v1/sla-policies/"+id, nil)
	manager := h.client()
	manager.bearer = apiKey(tm.owner, "manager", "inboxes:read", "inboxes:manage")
	manager.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+second, map[string]any{"warn_minutes": 5})

	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/sla-policies/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/sla-policies/"+id, map[string]any{"name": "mine"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/sla-policies/"+id, nil)
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sla-policies/order", map[string]any{"ids": []string{id, second}})
	if got := other.owner.expect(http.StatusOK, "GET", "/v1/sla-policies", nil).body["items"].([]any); len(got) != 0 {
		t.Fatalf("other workspace lists %v", got)
	}

	ordered := tm.owner.expect(http.StatusOK, "PUT", "/v1/sla-policies/order", map[string]any{"ids": []string{second, id}})
	items := ordered.body["items"].([]any)
	if items[0].(map[string]any)["id"] != second || items[1].(map[string]any)["position"].(float64) != 1 {
		t.Fatalf("order %s", ordered.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sla-policies/order", map[string]any{"ids": []string{id}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sla-policies/order", map[string]any{"ids": []string{id, id}})

	patched := tm.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+id, map[string]any{"first_reply_minutes": nil, "next_reply_minutes": 15, "enabled": false})
	if patched.body["first_reply_minutes"] != nil || patched.body["next_reply_minutes"].(float64) != 15 || patched.body["resolution_minutes"].(float64) != 480 || patched.body["enabled"] != false {
		t.Fatalf("patched %s", patched.raw)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/sla-policies/"+id,
		map[string]any{"next_reply_minutes": nil, "resolution_minutes": nil})

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+id, map[string]any{"enabled": true})
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/labels/"+label, nil)
	stripped := tm.owner.expect(http.StatusOK, "GET", "/v1/sla-policies/"+id, nil)
	if len(stripped.body["label_ids"].([]any)) != 0 || stripped.body["enabled"] != false {
		t.Fatalf("a policy whose only label is deleted must turn off: %s", stripped.raw)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox, nil)
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/sla-policies/"+second, nil); got.body["enabled"] != false {
		t.Fatalf("a policy whose only inbox is deleted must turn off: %s", got.raw)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/sla-policies/"+id, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/sla-policies/"+id, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/sla-policies/"+id, nil)
}

func TestSLAReplyTargetsAndResolution(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "inbox_ids": []string{ct.chatInbox}, "first_reply_minutes": 60, "next_reply_minutes": 30, "resolution_minutes": 600, "warn_minutes": 10})
	cs := ct.session(h, map[string]any{})
	t0 := h.clock.Now()
	conv := cs.start("Hello, I need help")
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": ct.agentID})

	sla := slaOf(t, ct.owner, conv)
	if sla["state"] != "on_track" {
		t.Fatalf("new conversation %v", sla)
	}
	near(t, "first reply due", slaTarget(t, sla, "first_reply")["due_at"], t0.Add(time.Hour))
	near(t, "resolution due", slaTarget(t, sla, "resolution")["due_at"], t0.Add(10*time.Hour))
	near(t, "sla due", sla["due_at"], t0.Add(time.Hour))

	h.clock.Advance(10 * time.Minute)
	reply(ct.agent, conv, map[string]any{"kind": "note"})
	reply(ct.agent, conv, map[string]any{"draft": true})
	if got := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply"); got["state"] != "on_track" || got["met_at"] != nil {
		t.Fatalf("a note or a draft met the first reply: %v", got)
	}
	reply(ct.agent, conv, nil)
	fr := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")
	if fr["state"] != "met" || fr["due_at"] != nil {
		t.Fatalf("first reply after the reply: %v", fr)
	}

	h.clock.Advance(time.Minute)
	cs.say(conv, "One more thing")
	t1 := h.clock.Now()
	nr := slaTarget(t, slaOf(t, ct.owner, conv), "next_reply")
	near(t, "next reply due", nr["due_at"], t1.Add(30*time.Minute))
	cs.say(conv, "Hello?")
	near(t, "next reply due after a second message", slaTarget(t, slaOf(t, ct.owner, conv), "next_reply")["due_at"], t1.Add(30*time.Minute))

	h.clock.Advance(21 * time.Minute)
	after := lastEvent(ct.owner)
	if n := h.runSLAChecks(ct.ws); n != 1 {
		t.Fatalf("ran %d checks at the warning", n)
	}
	if ev := eventsOf(ct.owner, after, "conversation.sla_warning"); len(ev) != 1 || ev[0]["data"].(map[string]any)["target"] != "next_reply" {
		t.Fatalf("warning events %v", ev)
	}
	if got := slaOf(t, ct.owner, conv); got["state"] != "due_soon" {
		t.Fatalf("state before due %v", got)
	}

	h.clock.Advance(10 * time.Minute)
	jobs := h.slaChecks(ct.ws)
	h.runSLAChecks(ct.ws)
	for _, j := range jobs {
		if j.at.After(h.clock.Now()) {
			continue
		}
		if err := h.srv.CheckSLA(context.Background(), j.args); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	breaches := eventsOf(ct.owner, after, "conversation.sla_breached")
	if len(breaches) != 1 {
		t.Fatalf("%d breach events, want exactly one", len(breaches))
	}
	d := breaches[0]["data"].(map[string]any)
	if d["target"] != "next_reply" || d["conversation"].(map[string]any)["sla"].(map[string]any)["state"] != "breached" {
		t.Fatalf("breach event %v", d)
	}
	if got := slaOf(t, ct.owner, conv); got["state"] != "breached" || slaTarget(t, got, "next_reply")["state"] != "breached" {
		t.Fatalf("after the breach %v", got)
	}
	updated := eventsOf(ct.owner, after, "conversation.updated")
	if len(updated) == 0 || updated[len(updated)-1]["data"].(map[string]any)["sla"].(map[string]any)["state"] != "breached" {
		t.Fatalf("conversation.updated after the breach %v", updated)
	}

	h.clock.Advance(time.Minute)
	reply(ct.agent, conv, nil)
	got := slaOf(t, ct.owner, conv)
	if late := slaTarget(t, got, "next_reply"); late["state"] != "breached" || late["met_at"] == nil || late["due_at"] != nil {
		t.Fatalf("late reply %v", late)
	}
	if got["state"] != "on_track" {
		t.Fatalf("with only the resolution running %v", got)
	}

	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	got = slaOf(t, ct.owner, conv)
	if res := slaTarget(t, got, "resolution"); res["state"] != "met" || got["state"] != "breached" || got["due_at"] != nil {
		t.Fatalf("closed %v", got)
	}

	h.clock.Advance(time.Hour)
	cs.say(conv, "It broke again")
	got = slaOf(t, ct.owner, conv)
	if res := slaTarget(t, got, "resolution"); res["state"] != "met" || res["due_at"] != nil {
		t.Fatalf("a reopened conversation got a new resolution target: %v", got)
	}
	near(t, "next reply after reopening", slaTarget(t, got, "next_reply")["due_at"], h.clock.Now().Add(30*time.Minute))

	want := []string{"first_reply:met", "next_reply:breached", "resolution:met"}
	if got := h.slaOutcomes(conv); !slices.Equal(got, want) {
		t.Fatalf("outcomes %v, want %v", got, want)
	}

	stats := ct.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+t0.Add(-time.Minute).UTC().Format(time.RFC3339), nil).body["sla"].(map[string]any)
	if stats["met"].(float64) != 2 || stats["breached"].(float64) != 1 {
		t.Fatalf("stats %v", stats)
	}
	inboxes, members := stats["inboxes"].([]any), stats["members"].([]any)
	if len(inboxes) != 1 || inboxes[0].(map[string]any)["inbox_id"] != ct.chatInbox || inboxes[0].(map[string]any)["met"].(float64) != 2 {
		t.Fatalf("per inbox %v", inboxes)
	}
	if len(members) != 1 || members[0].(map[string]any)["member_id"] != ct.agentID || members[0].(map[string]any)["breached"].(float64) != 1 {
		t.Fatalf("per member %v", members)
	}
	other := newTeam(t, h)
	if s := other.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+t0.Add(-time.Minute).UTC().Format(time.RFC3339), nil).body["sla"].(map[string]any); s["met"].(float64) != 0 {
		t.Fatalf("another workspace counts %v", s)
	}
	if s := ct.agent.expect(http.StatusOK, "GET", "/v1/stats?inbox_id="+ct.chatInbox+"&since="+t0.Add(-time.Minute).UTC().Format(time.RFC3339), nil).body["sla"].(map[string]any); s["breached"].(float64) != 1 {
		t.Fatalf("agent's stats of the inbox %v", s)
	}
}

func TestSLAPauseSnoozeWakeAndRace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 60, "warn_minutes": 0})
	cs := ct.session(h, map[string]any{})
	conv := cs.start("Hello")

	h.clock.Advance(20 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(2 * time.Hour).Format(time.RFC3339Nano)})
	sla := slaOf(t, ct.owner, conv)
	fr := slaTarget(t, sla, "first_reply")
	if sla["state"] != "paused" || sla["due_at"] != nil || fr["state"] != "paused" || fr["remaining_seconds"].(float64) != 40*60 {
		t.Fatalf("snoozed %v", sla)
	}
	var due *time.Time
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT sla_due_at FROM conversations WHERE id = $1", conv).Scan(&due); err != nil || due != nil {
		t.Fatalf("sla_due_at while paused: %v %v", due, err)
	}

	h.clock.Advance(time.Hour)
	if err := h.srv.WakeSnoozedIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	if got := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); got != "snoozed" {
		t.Fatalf("woke before snooze_until: %s", got)
	}
	h.clock.Advance(90 * time.Minute)
	after := lastEvent(ct.owner)
	if err := h.srv.WakeSnoozedIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	woke := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if woke.str("status") != "open" || woke.body["snooze_until"] != nil {
		t.Fatalf("after snooze_until %s", woke.raw)
	}
	near(t, "first reply due after waking", slaTarget(t, woke.body["sla"].(map[string]any), "first_reply")["due_at"], h.clock.Now().Add(40*time.Minute))
	if len(eventsOf(ct.owner, after, "conversation.updated")) != 1 {
		t.Fatal("no conversation.updated for the wake")
	}
	var changed bool
	for _, e := range eventsOf(ct.owner, after, "message.created") {
		ev, _ := e["data"].(map[string]any)["event"].(map[string]any)
		changed = changed || (ev["type"] == "status_changed" && ev["status"] == "open" && ev["previous_status"] == "snoozed")
	}
	if !changed {
		t.Fatal("no status_changed event for the wake")
	}

	h.clock.Advance(30 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})
	if fr := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply"); fr["remaining_seconds"].(float64) != 10*60 {
		t.Fatalf("pending %v", fr)
	}
	stale := h.slaChecks(ct.ws)
	h.clock.Advance(time.Hour)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"})
	resumed := slaTime(t, slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")["due_at"])
	near(t, "first reply due after pending", resumed, h.clock.Now().Add(10*time.Minute))

	for _, j := range stale {
		if err := h.srv.CheckSLA(context.Background(), j.args); err != nil {
			t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	if got := slaOf(t, ct.owner, conv); got["state"] != "on_track" || len(h.slaOutcomes(conv)) != 0 {
		t.Fatalf("a stale check acted: %v", got)
	}

	h.clock.Advance(10 * time.Minute)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = h.srv.CheckSLA(context.Background(), api.SLACheckArgs{WorkspaceID: uuid.MustParse(ct.ws), ConversationID: uuid.MustParse(conv), Target: "first_reply", DueAt: resumed})
	}()
	var pauseStatus int
	go func() {
		defer wg.Done()
		pauseStatus = ct.agent.do("PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"}).status
	}()
	wg.Wait()
	if pauseStatus != http.StatusOK {
		t.Fatalf("pause %d", pauseStatus)
	}
	outcomes := h.slaOutcomes(conv)
	fr = slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")
	switch {
	case len(outcomes) == 1 && outcomes[0] == "first_reply:breached":
		if fr["state"] != "breached" {
			t.Fatalf("breached first, then paused: %v", fr)
		}
	case len(outcomes) == 0:
		if fr["state"] != "paused" || fr["remaining_seconds"].(float64) != 0 {
			t.Fatalf("paused first: %v", fr)
		}
	default:
		t.Fatalf("outcomes %v", outcomes)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"})
	h.runSLAChecks(ct.ws)
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	if got := h.slaOutcomes(conv); !slices.Equal(got, []string{"first_reply:breached"}) {
		t.Fatalf("the breach must happen exactly once: %v", got)
	}
}

func TestSLAResumeRacesBreach(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 30, "warn_minutes": 0})
	cs := ct.session(h, map[string]any{})
	for i := range 5 {
		conv := cs.start(fmt.Sprintf("Hello %d", i))
		old := slaTime(t, slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")["due_at"])
		h.clock.Advance(20 * time.Minute)
		ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})
		h.clock.Advance(15 * time.Minute)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = h.srv.CheckSLA(context.Background(), api.SLACheckArgs{WorkspaceID: uuid.MustParse(ct.ws), ConversationID: uuid.MustParse(conv), Target: "first_reply", DueAt: old})
		}()
		var status int
		go func() {
			defer wg.Done()
			status = ct.agent.do("PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"}).status
		}()
		wg.Wait()
		if status != http.StatusOK {
			t.Fatalf("resume %d", status)
		}
		fr := slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")
		if got := h.slaOutcomes(conv); len(got) != 0 || fr["state"] != "on_track" {
			t.Fatalf("the check for the due time before the pause acted: %v %v", got, fr)
		}
		near(t, "due after resuming", fr["due_at"], h.clock.Now().Add(10*time.Minute))
		h.clock.Advance(10 * time.Minute)
		h.runSLAChecks(ct.ws)
		h.runSLAChecks(ct.ws)
		if got := h.slaOutcomes(conv); !slices.Equal(got, []string{"first_reply:breached"}) {
			t.Fatalf("outcomes %v", got)
		}
	}
}

func TestSLAReevaluation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	urgent := ct.policy(map[string]any{"name": "Urgent", "priorities": []string{"urgent"}, "first_reply_minutes": 10})
	label := ct.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Enterprise"}).str("id")
	enterprise := ct.policy(map[string]any{"name": "Enterprise", "label_ids": []string{label}, "first_reply_minutes": 120, "resolution_minutes": 60})
	cs := ct.session(h, map[string]any{})
	t0 := h.clock.Now()
	conv := cs.start("Hello")
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("no policy matches yet: %v", sla)
	}

	h.clock.Advance(5 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"labels": []string{label}})
	sla := slaOf(t, ct.owner, conv)
	if sla["policy_id"] != enterprise {
		t.Fatalf("after the label %v", sla)
	}
	near(t, "first reply from the first message", slaTarget(t, sla, "first_reply")["due_at"], t0.Add(2*time.Hour))
	near(t, "resolution from creation", slaTarget(t, sla, "resolution")["due_at"], t0.Add(time.Hour))

	h.clock.Advance(3 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "urgent"})
	sla = slaOf(t, ct.owner, conv)
	if sla["policy_id"] != urgent {
		t.Fatalf("after the priority %v", sla)
	}
	near(t, "first reply keeps the time used", slaTarget(t, sla, "first_reply")["due_at"], h.clock.Now().Add(2*time.Minute))
	if res := slaTarget(t, sla, "first_reply"); len(sla["targets"].([]any)) != 1 || res == nil {
		t.Fatalf("the urgent policy tracks no resolution: %v", sla)
	}

	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": true})
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("spam keeps %v", sla)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": false})
	sla = slaOf(t, ct.owner, conv)
	if sla["policy_id"] != urgent {
		t.Fatalf("unflagged %v", sla)
	}
	near(t, "first reply rebuilt from the first message", slaTarget(t, sla, "first_reply")["due_at"], t0.Add(10*time.Minute))
	ct.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	if n := h.count("SELECT count(*) FROM conversation_sla WHERE conversation_id = $1 AND dropped_at IS NULL", conv); n != 0 {
		t.Fatalf("trash keeps %d live rows", n)
	}

	conv2 := cs.start("Second")
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv2, map[string]any{"priority": "urgent"})
	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/sla-policies/"+urgent, nil)
	if sla := slaOf(t, ct.owner, conv2); sla != nil {
		t.Fatalf("deleted policy kept %v", sla)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE id = $1 AND sla_due_at IS NOT NULL", conv2); n != 0 {
		t.Fatal("deleted policy kept sla_due_at")
	}

	old := cs.start("Old and unanswered")
	h.clock.Advance(5 * time.Hour)
	late := ct.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Late"}).str("id")
	ct.policy(map[string]any{"name": "Late", "label_ids": []string{late}, "first_reply_minutes": 30})
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+old, map[string]any{"labels": []string{late}})
	sla = slaOf(t, ct.owner, old)
	if sla["state"] != "on_track" {
		t.Fatalf("a new policy breached an old conversation at once: %v", sla)
	}
	near(t, "first reply of an older conversation counts from the policy", slaTarget(t, sla, "first_reply")["due_at"], h.clock.Now().Add(30*time.Minute))
}

func TestSLAListSortFilterAndMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 60, "warn_minutes": 15})
	ids := map[string]string{}
	for _, name := range []string{"a", "b", "c"} {
		ids[name] = ct.session(h, map[string]any{}).start("Hello " + name)
		h.clock.Advance(10 * time.Minute)
	}
	reply(ct.agent, ids["b"], nil)
	plain := ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": ct.inbox, "contact_id": ct.contact, "subject": "No SLA"}).str("id")
	_ = plain

	list := func(query string) []string {
		var out []string
		cursor := ""
		for {
			r := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations?inbox_id="+ct.chatInbox+"&limit=1"+query+cursor, nil)
			for _, it := range r.body["items"].([]any) {
				out = append(out, it.(map[string]any)["id"].(string))
			}
			next, _ := r.body["next_cursor"].(string)
			if next == "" {
				return out
			}
			cursor = "&cursor=" + next
		}
	}
	if got := list("&sort=sla_due"); !slices.Equal(got, []string{ids["a"], ids["c"], ids["b"]}) {
		t.Fatalf("sort=sla_due %v, want a c b", got)
	}
	if got := list(""); !slices.Equal(got, []string{ids["b"], ids["c"], ids["a"]}) {
		t.Fatalf("default order %v", got)
	}
	if got := list("&sla_state=met"); !slices.Equal(got, []string{ids["b"]}) {
		t.Fatalf("met %v", got)
	}
	if got := list("&sla_state=on_track,due_soon&sort=sla_due"); !slices.Equal(got, []string{ids["a"], ids["c"]}) {
		t.Fatalf("running %v", got)
	}
	h.clock.Advance(25 * time.Minute)
	if got := list("&sla_state=due_soon"); !slices.Equal(got, []string{ids["a"]}) {
		t.Fatalf("due soon %v", got)
	}
	h.clock.Advance(10 * time.Minute)
	if got := list("&sla_state=breached"); !slices.Equal(got, []string{ids["a"]}) {
		t.Fatalf("breached %v", got)
	}
	first := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations?inbox_id="+ct.chatInbox+"&limit=1&sort=sla_due", nil)
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?cursor="+first.str("next_cursor"), nil)
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?sla_state=late", nil)
	item := first.body["items"].([]any)[0].(map[string]any)
	if item["sla"].(map[string]any)["state"] != "breached" {
		t.Fatalf("list item sla %v", item)
	}

	token := h.oauthToken(ct.owner, ct.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read", "inboxes:read"})
	session := h.mcpSession(token)
	got, errText := callTool(t, session, "search_conversations", map[string]any{"sla_state": "breached"})
	if errText != "" {
		t.Fatal(errText)
	}
	items := got["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != ids["a"] || items[0].(map[string]any)["sla"].(map[string]any)["state"] != "breached" {
		t.Fatalf("search_conversations sla_state %v", got)
	}
	got, errText = callTool(t, session, "search_conversations", map[string]any{"sort": "sla_due", "status": "open", "limit": 2})
	if errText != "" {
		t.Fatal(errText)
	}
	if items := got["items"].([]any); items[0].(map[string]any)["id"] != ids["a"] || items[1].(map[string]any)["id"] != ids["c"] {
		t.Fatalf("search_conversations sort %v", got)
	}
	got, errText = callTool(t, session, "get_conversation", map[string]any{"conversation_id": ids["c"]})
	if errText != "" {
		t.Fatal(errText)
	}
	if sla := got["conversation"].(map[string]any)["sla"].(map[string]any); sla["state"] != "due_soon" || !strings.HasPrefix(fmt.Sprint(sla["targets"]), "[map[") {
		t.Fatalf("get_conversation sla %v", got["conversation"])
	}
}

func TestSLANotifications(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 30, "warn_minutes": 5})
	agentEmail := ct.agent.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
	ownerEmail := ct.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)

	assigned := ct.session(h, map[string]any{}).start("Assigned")
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+assigned, map[string]any{"assignee_id": ct.agentID})
	support := ct.owner.expect(http.StatusCreated, "POST", "/v1/teams", map[string]any{"name": "Support"}).str("id")
	ct.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+support+"/members/"+ct.agentID, nil)
	ct.owner.expect(http.StatusNoContent, "PUT", "/v1/teams/"+support+"/inboxes/"+ct.chatInbox, nil)
	teamed := ct.session(h, map[string]any{}).start("Team")
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+teamed, map[string]any{"team_id": support})
	nobody := ct.session(h, map[string]any{}).start("Nobody")
	_ = nobody

	h.clock.Advance(26 * time.Minute)
	h.runSLAChecks(ct.ws)
	if n := len(h.jobs("sla_notify", ct.ws)); n != 3 {
		t.Fatalf("%d warning notifications queued", n)
	}
	h.runSLANotifications(ct.ws)
	h.clock.Advance(5 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.runSLANotifications(ct.ws)
	slaMails := func(addr string) []string {
		var out []string
		for _, m := range h.mail.to(addr) {
			if strings.Contains(m.Subject, "SLA") {
				if !strings.Contains(m.Text, "first reply") {
					t.Fatalf("mail %q %q", m.Subject, m.Text)
				}
				out = append(out, m.Subject)
			}
		}
		return out
	}
	if got := slaMails(agentEmail); len(got) != 2 {
		t.Fatalf("agent got %v, want one per breached conversation of theirs or their team's", got)
	}
	if got := slaMails(ownerEmail); len(got) != 0 {
		t.Fatalf("owner got %v though nothing is theirs", got)
	}

	ct.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{"events": map[string]any{"sla_breached": map[string]any{"push": true, "email": false}}})
	again := ct.session(h, map[string]any{}).start("Again")
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+again, map[string]any{"assignee_id": ct.agentID})
	h.clock.Advance(31 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.runSLANotifications(ct.ws)
	if got := slaMails(agentEmail); len(got) != 2 {
		t.Fatalf("e-mail turned off, still %v", got)
	}
	settings := ct.agent.expect(http.StatusOK, "GET", "/v1/me/notifications", nil)
	if fmt.Sprint(settings.body["defaults"].(map[string]any)["sla_breached"]) != "map[email:true push:true]" {
		t.Fatalf("defaults %s", settings.raw)
	}
}

func TestSLAWebhooks(t *testing.T) {
	t.Parallel()
	h := privateHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 30, "warn_minutes": 10})
	hooks, other := newReceiver(t), newReceiver(t)
	ct.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": hooks.srv.URL, "events": []string{"conversation.sla_warning", "conversation.sla_breached"}})
	ct.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": other.srv.URL, "inbox_id": ct.inbox, "events": []string{"conversation.sla_breached"}})
	conv := ct.session(h, map[string]any{}).start("Hello")
	h.runWebhooks(ct.ws)
	h.clock.Advance(25 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.clock.Advance(10 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.runWebhooks(ct.ws)
	got := hooks.take()
	if len(got) != 2 || got[0].typ != "conversation.sla_warning" || got[1].typ != "conversation.sla_breached" {
		t.Fatalf("hooks %v", got)
	}
	d := got[1].data
	if d["target"] != "first_reply" || d["conversation"].(map[string]any)["id"] != conv || d["contact"] == nil || d["due_at"] == nil {
		t.Fatalf("breach payload %v", d)
	}
	if n := len(other.take()); n != 0 {
		t.Fatalf("an endpoint of another inbox got %d hooks", n)
	}
}

func TestSLADropKeepsTheClock(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Urgent", "priorities": []string{"urgent"}, "first_reply_minutes": 10, "warn_minutes": 0})
	cs := ct.session(h, map[string]any{})
	after := lastEvent(ct.owner)

	conv := cs.start("Down")
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "urgent"})
	h.clock.Advance(4 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "normal"})
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("normal priority keeps %v", sla)
	}
	h.clock.Advance(2 * time.Hour)
	h.runSLAChecks(ct.ws)
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "urgent"})
	sla := slaOf(t, ct.owner, conv)
	if sla["state"] != "on_track" {
		t.Fatalf("urgent again %v", sla)
	}
	near(t, "first reply keeps the 6 minutes left at the drop", slaTarget(t, sla, "first_reply")["due_at"], h.clock.Now().Add(6*time.Minute))
	h.runSLAChecks(ct.ws)
	if got := h.slaOutcomes(conv); len(got) != 0 {
		t.Fatalf("outcomes while the priority was normal: %v", got)
	}
	h.clock.Advance(7 * time.Minute)
	h.runSLAChecks(ct.ws)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "normal"})
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"priority": "urgent"})
	h.runSLAChecks(ct.ws)
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	if got := h.slaOutcomes(conv); !slices.Equal(got, []string{"first_reply:breached"}) {
		t.Fatalf("outcomes %v", got)
	}

	spam := cs.start("Wait")
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"priority": "urgent"})
	h.clock.Advance(2 * time.Minute)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"status": "pending"})
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"spam": true})
	h.clock.Advance(72 * time.Hour)
	h.runSLAChecks(ct.ws)
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"spam": false})
	fr := slaTarget(t, slaOf(t, ct.owner, spam), "first_reply")
	if fr["state"] != "paused" || fr["remaining_seconds"].(float64) != 8*60 {
		t.Fatalf("unflagged after days pending %v", fr)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"status": "open"})
	near(t, "resumed after spam", slaTarget(t, slaOf(t, ct.owner, spam), "first_reply")["due_at"], h.clock.Now().Add(8*time.Minute))
	h.runSLAChecks(ct.ws)
	if got := h.slaOutcomes(spam); len(got) != 0 {
		t.Fatalf("outcomes of the spam one %v", got)
	}
	breaches := eventsOf(ct.owner, after, "conversation.sla_breached")
	if len(breaches) != 1 || breaches[0]["conversation_id"] != conv {
		t.Fatalf("breach events %v", breaches)
	}
}

func TestSLAPolicyThatStopsApplying(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	vip := ct.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "VIP"}).str("id")
	other := ct.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Other"}).str("id")
	policy := ct.policy(map[string]any{"name": "VIP", "label_ids": []string{vip}, "first_reply_minutes": 30, "warn_minutes": 0})
	cs := ct.session(h, map[string]any{})
	conv := cs.start("Hello")
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"labels": []string{vip}})
	if slaOf(t, ct.owner, conv) == nil {
		t.Fatal("no SLA with the label")
	}

	h.clock.Advance(10 * time.Minute)
	after := lastEvent(ct.owner)
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+policy, map[string]any{"enabled": false})
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("a disabled policy still drives %v", sla)
	}
	if ev := eventsOf(ct.owner, after, "conversation.updated"); len(ev) != 1 || ev[0]["data"].(map[string]any)["sla"] != nil {
		t.Fatalf("conversation.updated for the disabled policy %v", ev)
	}
	h.clock.Advance(time.Hour)
	h.runSLAChecks(ct.ws)
	if err := h.srv.SweepSLAIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	if got := h.slaOutcomes(conv); len(got) != 0 {
		t.Fatalf("a disabled policy breached %v", got)
	}

	ct.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+policy, map[string]any{"enabled": true})
	reply(ct.agent, conv, map[string]any{"kind": "note"})
	near(t, "re-enabled keeps the 20 minutes left", slaTarget(t, slaOf(t, ct.owner, conv), "first_reply")["due_at"], h.clock.Now().Add(20*time.Minute))

	ct.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+policy, map[string]any{"label_ids": []string{other}})
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("a narrowed policy still drives %v", sla)
	}
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/sla-policies/"+policy, map[string]any{"label_ids": []string{vip}})
	reply(ct.agent, conv, map[string]any{"kind": "note"})
	if slaOf(t, ct.owner, conv) == nil {
		t.Fatal("no SLA once the policy matches again")
	}
	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/labels/"+vip, nil)
	if sla := slaOf(t, ct.owner, conv); sla != nil {
		t.Fatalf("the policy turned off by the label's deletion still drives %v", sla)
	}

	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE sla_policies SET enabled = true, label_ids = '{}' WHERE id = $1", policy); err != nil {
		t.Fatal(err)
	}
	reply(ct.agent, conv, map[string]any{"kind": "note"})
	sla := slaOf(t, ct.owner, conv)
	due := slaTime(t, slaTarget(t, sla, "first_reply")["due_at"])
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE sla_policies SET enabled = false WHERE id = $1", policy); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Hour)
	if err := h.srv.CheckSLA(context.Background(), api.SLACheckArgs{WorkspaceID: uuid.MustParse(ct.ws), ConversationID: uuid.MustParse(conv), Target: "first_reply", DueAt: due}); err != nil {
		t.Fatal(err)
	}
	if got := h.slaOutcomes(conv); len(got) != 0 || slaOf(t, ct.owner, conv) != nil {
		t.Fatalf("a check of a policy turned off without a resync breached: %v", got)
	}
}

func TestSLAPoliciesAndInboxLimitedKeys(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := tm.newInbox("Billing")
	everywhere := tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{"name": "All", "first_reply_minutes": 60}).str("id")
	both := tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{"name": "Both", "inbox_ids": []string{tm.inbox, other}, "first_reply_minutes": 60}).str("id")
	billing := tm.owner.expect(http.StatusCreated, "POST", "/v1/sla-policies", map[string]any{"name": "Billing", "inbox_ids": []string{other}, "first_reply_minutes": 60}).str("id")

	limited := h.client()
	r := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "limited", "inbox_ids": []string{tm.inbox}, "scopes": []string{"inboxes:read"}})
	limited.bearer = r.str("secret")
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE api_keys SET scopes = scopes || '{inboxes:manage}' WHERE id = $1", r.body["api_key"].(map[string]any)["id"]); err != nil {
		t.Fatal(err)
	}
	limited.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/sla-policies", map[string]any{"name": "x", "first_reply_minutes": 5})
	limited.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/sla-policies/"+both, map[string]any{"name": "x"})
	limited.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/sla-policies/"+both, nil)
	limited.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/sla-policies/order", map[string]any{"ids": []string{billing, both, everywhere}})

	list := limited.expect(http.StatusOK, "GET", "/v1/sla-policies", nil)
	if got := itemIDs(list); !slices.Equal(got, sorted(everywhere, both)) {
		t.Fatalf("limited key lists %v", got)
	}
	for _, it := range list.body["items"].([]any) {
		if ids := fmt.Sprint(it.(map[string]any)["inbox_ids"]); ids != "[]" && ids != "["+tm.inbox+"]" {
			t.Fatalf("limited key sees inboxes %s", ids)
		}
	}
	limited.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/sla-policies/"+billing, nil)
	if got := fmt.Sprint(limited.expect(http.StatusOK, "GET", "/v1/sla-policies/"+both, nil).body["inbox_ids"]); got != "["+tm.inbox+"]" {
		t.Fatalf("limited key reads inbox_ids %s", got)
	}
}

func TestSLAWarningOnlyAhead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Short", "first_reply_minutes": 10, "warn_minutes": 15})
	conv := ct.session(h, map[string]any{}).start("Hello")
	for _, j := range h.slaChecks(ct.ws) {
		if j.args.Warn {
			t.Fatalf("a warning queued at %s though its time had passed", j.at)
		}
	}
	h.clock.Advance(11 * time.Minute)
	after := lastEvent(ct.owner)
	h.runSLAChecks(ct.ws)
	if n := len(eventsOf(ct.owner, after, "conversation.sla_warning")); n != 0 {
		t.Fatalf("%d warnings", n)
	}
	if n := len(eventsOf(ct.owner, after, "conversation.sla_breached")); n != 1 {
		t.Fatalf("%d breaches", n)
	}

	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "snoozed", "snooze_until": h.clock.Now().Add(time.Hour).Format(time.RFC3339Nano)})
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": true})
	h.clock.Advance(2 * time.Hour)
	if err := h.srv.WakeSnoozedIn(context.Background(), uuid.MustParse(ct.ws)); err != nil {
		t.Fatal(err)
	}
	if got := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); got != "snoozed" {
		t.Fatalf("spam woke up: %s", got)
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/mail"
)

func (st statusTeam) monitor(t *testing.T, body map[string]any) (id, token string) {
	t.Helper()
	r := st.owner.expect(http.StatusCreated, "POST", st.path("/monitors"), body)
	if r.str("token") == "" || !strings.HasSuffix(r.str("url"), "/"+r.str("token")) || !strings.HasPrefix(r.str("token"), r.str("token_prefix")) {
		t.Fatalf("created monitor %s", r.raw)
	}
	return r.str("id"), r.str("token")
}

func (st statusTeam) monitorBody(t *testing.T, id string) response {
	t.Helper()
	return st.owner.expect(http.StatusOK, "GET", st.path("/monitors/"+id), nil)
}

func ingress(t *testing.T, h *harness, want int, method, path string, body any) response {
	t.Helper()
	r := h.client().do(method, path, body)
	if r.status != want {
		t.Fatalf("%s %s: %d %s", method, path, r.status, r.raw)
	}
	return r
}

func fixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "status", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (st statusTeam) incident(t *testing.T, id string) response {
	t.Helper()
	return st.owner.expect(http.StatusOK, "GET", st.path("/incidents/"+id), nil)
}

func TestStatusHeartbeatMissedAndBack(t *testing.T) {
	h := newHarness(t)
	st := newStatusTeam(t, h)
	id, token := st.monitor(t, map[string]any{"kind": "heartbeat", "name": "Nightly job", "component_id": st.api, "interval": 60, "grace": 30})
	ctx := context.Background()

	if r := ingress(t, h, http.StatusOK, "GET", "/ingress/heartbeat/"+token, nil); r.str("state") != "up" {
		t.Fatalf("first beat %s", r.raw)
	}
	h.clock.Advance(80 * time.Second)
	if err := h.srv.SweepStatusMonitors(ctx); err != nil {
		t.Fatal(err)
	}
	if m := st.monitorBody(t, id); m.str("state") != "up" {
		t.Fatalf("failed within the grace: %s", m.raw)
	}
	h.clock.Advance(15 * time.Second)
	if err := h.srv.SweepStatusMonitors(ctx); err != nil {
		t.Fatal(err)
	}
	m := st.monitorBody(t, id)
	if m.str("state") != "down" || m.str("down_state") != "major_outage" || m.str("incident_id") == "" || m.body["next_beat_due"] != nil {
		t.Fatalf("after the grace: %s", m.raw)
	}
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("component %s", got)
	}
	inc := st.incident(t, m.str("incident_id"))
	if inc.body["published_at"] != nil || inc.str("state") != "investigating" || len(inc.body["components"].([]any)) != 1 {
		t.Fatalf("draft incident %s", inc.raw)
	}
	if n := len(h.jobs("status_monitor_notify", st.ws)); n != 1 {
		t.Fatalf("%d notify jobs", n)
	}

	ingress(t, h, http.StatusOK, "POST", "/ingress/heartbeat/"+token, nil)
	m = st.monitorBody(t, id)
	if m.str("state") != "up" || m.body["incident_id"] != nil || st.componentState(t, st.api) != "operational" {
		t.Fatalf("after beating again: %s, component %s", m.raw, st.componentState(t, st.api))
	}
	if got := st.incident(t, inc.str("id")).str("state"); got != "resolved" {
		t.Fatalf("incident %s", got)
	}
	if r := ingress(t, h, http.StatusOK, "POST", "/ingress/heartbeat/"+token+"/fail", nil); r.str("state") != "down" {
		t.Fatalf("fail beat %s", r.raw)
	}
	ingress(t, h, http.StatusOK, "GET", "/ingress/heartbeat/"+token, nil)
	if r := ingress(t, h, http.StatusOK, "GET", "/ingress/heartbeat/"+token+"?fail=1", nil); r.str("state") != "down" {
		t.Fatalf("?fail=1 %s", r.raw)
	}

	checks := st.owner.expect(http.StatusOK, "GET", st.path("/monitors/"+id+"/checks?limit=2"), nil)
	items := checks.body["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["source"] != "fail" || checks.str("next_cursor") == "" {
		t.Fatalf("checks %s", checks.raw)
	}
	rest := st.owner.expect(http.StatusOK, "GET", st.path("/monitors/"+id+"/checks?cursor="+checks.str("next_cursor")), nil).body["items"].([]any)
	sources := []string{}
	for _, c := range rest {
		sources = append(sources, c.(map[string]any)["source"].(string))
	}
	if strings.Join(sources, ",") != "fail,beat,missed,beat" {
		t.Fatalf("older checks %v", sources)
	}

	events := st.owner.expect(http.StatusOK, "GET", "/v1/events?after=0", nil).body["events"].([]any)
	var seen []string
	for _, e := range events {
		if typ := e.(map[string]any)["type"].(string); strings.HasPrefix(typ, "status.monitor.") {
			seen = append(seen, typ)
		}
	}
	if strings.Join(seen, ",") != "status.monitor.down,status.monitor.up,status.monitor.down,status.monitor.up,status.monitor.down" {
		t.Fatalf("monitor events %v", seen)
	}
}

func TestStatusHeartbeatWaitsAfterServerStart(t *testing.T) {
	h := newHarness(t)
	st := newStatusTeam(t, h)
	started := h.clock.Now()
	h.clock.Set(started.Add(-10 * time.Minute))
	id, _ := st.monitor(t, map[string]any{"kind": "heartbeat", "name": "Cron", "component_id": st.api, "interval": 60, "grace": 30})
	h.clock.Set(started.Add(time.Minute))
	ctx := context.Background()
	if err := h.srv.SweepStatusMonitors(ctx); err != nil {
		t.Fatal(err)
	}
	m := st.monitorBody(t, id)
	due, _ := time.Parse(time.RFC3339Nano, m.str("next_beat_due"))
	if m.str("state") != "pending" || !due.Equal(started.Add(time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("beat expected before the server started: %s", m.raw)
	}
	h.clock.Set(started.Add(time.Minute + time.Second))
	if err := h.srv.SweepStatusMonitors(ctx); err != nil {
		t.Fatal(err)
	}
	if m := st.monitorBody(t, id); m.str("state") != "down" {
		t.Fatalf("one more interval only: %s", m.raw)
	}
}

func TestStatusMonitorThresholds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	_, token := st.monitor(t, map[string]any{"kind": "push", "name": "Prometheus", "component_id": st.web,
		"failure_threshold": 2, "recovery_threshold": 2, "on_failure": "none", "failure_state": "degraded"})
	path := "/ingress/status/" + token
	steps := []struct {
		body               map[string]any
		monitor, component string
	}{
		{map[string]any{"state": "major_outage", "message": "EU down"}, "pending", "operational"},
		{map[string]any{"state": "operational"}, "up", "operational"},
		{map[string]any{"state": "partial_outage"}, "up", "operational"},
		{map[string]any{"state": "partial_outage"}, "down", "partial_outage"},
		{map[string]any{"state": "major_outage"}, "down", "major_outage"},
		{map[string]any{"state": "operational"}, "down", "major_outage"},
		{map[string]any{"state": "operational"}, "up", "operational"},
	}
	for i, s := range steps {
		r := ingress(t, h, http.StatusOK, "POST", path, s.body)
		if r.str("state") != s.monitor || st.componentState(t, st.web) != s.component {
			t.Fatalf("step %d: %s, component %s", i, r.raw, st.componentState(t, st.web))
		}
	}
	if items := st.owner.expect(http.StatusOK, "GET", st.path("/incidents"), nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("on_failure none opened %v", items)
	}
	ingress(t, h, http.StatusBadRequest, "POST", path, map[string]any{"state": "maintenance"})
	ingress(t, h, http.StatusConflict, "GET", "/ingress/heartbeat/"+token, nil)
	ingress(t, h, http.StatusNotFound, "POST", "/ingress/status/nope", map[string]any{"state": "operational"})
}

func TestStatusMonitorMemberStateWins(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	id, token := st.monitor(t, map[string]any{"kind": "push", "name": "Uptime", "component_id": st.api, "on_failure": "publish"})
	path := "/ingress/status/" + token
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	m := st.monitorBody(t, id)
	if st.componentState(t, st.api) != "major_outage" || st.incident(t, m.str("incident_id")).body["published_at"] == nil {
		t.Fatalf("down: %s", m.raw)
	}
	h.clock.Advance(time.Minute)
	st.agent.expectProblem(http.StatusNotFound, "not_found", "GET", st.path("/monitors"), nil)
	st.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+st.inbox+"/members/"+st.agentID, nil)
	st.agent.expect(http.StatusOK, "GET", st.path("/monitors"), nil)
	st.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", st.path("/monitors"), map[string]any{"kind": "push", "name": "X", "component_id": st.api})
	st.agent.expect(http.StatusOK, "POST", st.path("/components/"+st.api+"/state"), map[string]any{"state": "degraded"})

	h.clock.Advance(time.Minute)
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "operational"})
	if got := st.componentState(t, st.api); got != "degraded" {
		t.Fatalf("the member's later state was replaced by %s", got)
	}
	if got := st.incident(t, m.str("incident_id")).str("state"); got != "resolved" {
		t.Fatalf("incident %s", got)
	}
	h.clock.Advance(time.Minute)
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("a new failure after the member's state: %s", got)
	}

	incidentID := st.monitorBody(t, id).str("incident_id")
	st.owner.expect(http.StatusCreated, "POST", st.path("/incidents/"+incidentID+"/updates"), map[string]any{"state": "resolved", "body": "Fixed"})
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("resolving the incident let go of a component the failing monitor holds: %s", got)
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path("/monitors/"+id), map[string]any{"paused": true})
	if got := st.componentState(t, st.api); got != "operational" {
		t.Fatalf("pausing kept %s", got)
	}
	if r := ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"}); r.str("state") != "pending" || r.body["paused"] != true {
		t.Fatalf("paused push %s", r.raw)
	}
}

func TestStatusMonitorAlertBodies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	_, am := st.monitor(t, map[string]any{"kind": "push", "name": "Alertmanager", "component_id": st.api, "on_failure": "none"})
	gid, gr := st.monitor(t, map[string]any{"kind": "push", "name": "Grafana", "component_id": st.web, "on_failure": "none", "failure_state": "partial_outage"})

	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+am, fixture(t, "alertmanager_firing.json"))
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("alertmanager firing: %s", got)
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+am, fixture(t, "alertmanager_resolved.json"))
	if got := st.componentState(t, st.api); got != "operational" {
		t.Fatalf("alertmanager resolved: %s", got)
	}

	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+gr, fixture(t, "grafana_firing.json"))
	m := st.monitorBody(t, gid)
	if st.componentState(t, st.web) != "partial_outage" || m.str("last_message") != "Checkout p95 latency is above 3 seconds" {
		t.Fatalf("grafana firing: %s", m.raw)
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+gr, fixture(t, "grafana_legacy_no_data.json"))
	if got := st.componentState(t, st.web); got != "partial_outage" {
		t.Fatalf("no_data changed the state to %s", got)
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+gr, fixture(t, "grafana_resolved.json"))
	if got := st.componentState(t, st.web); got != "operational" {
		t.Fatalf("grafana resolved: %s", got)
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+gr, fixture(t, "grafana_legacy_alerting.json"))
	if got := st.componentState(t, st.web); got != "partial_outage" {
		t.Fatalf("legacy alerting: %s", got)
	}
}

func TestStatusMonitorIsolationAndTokens(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) { d.Status.Monitors = 2 })
	st := newStatusTeam(t, h)
	other := newStatusTeam(t, h)
	id, token := st.monitor(t, map[string]any{"kind": "heartbeat", "name": "Backup", "component_id": st.api})

	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", st.path("/monitors"), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", st.path("/monitors/"+id), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", other.path("/monitors/"+id), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", other.path("/monitors/"+id), map[string]any{"name": "Mine"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", other.path("/monitors/"+id), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", other.path("/monitors/"+id+"/token"), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", other.path("/monitors/"+id+"/checks"), nil)
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", other.path("/monitors"), map[string]any{"kind": "push", "name": "X", "component_id": st.api})

	st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", st.path("/monitors"), map[string]any{"kind": "push", "name": "X", "component_id": st.api, "interval": 60})
	st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", st.path("/monitors"), map[string]any{"kind": "heartbeat", "name": "X", "component_id": st.api, "interval": 5})
	st.monitor(t, map[string]any{"kind": "push", "name": "Second", "component_id": st.web})
	st.owner.expectProblem(http.StatusForbidden, "status_monitor_limit", "POST", st.path("/monitors"), map[string]any{"kind": "push", "name": "Third", "component_id": st.web})

	rot := st.owner.expect(http.StatusOK, "POST", st.path("/monitors/"+id+"/token"), nil)
	ingress(t, h, http.StatusNotFound, "GET", "/ingress/heartbeat/"+token, nil)
	ingress(t, h, http.StatusOK, "GET", "/ingress/heartbeat/"+rot.str("token"), nil)
	for range 59 {
		ingress(t, h, http.StatusOK, "GET", "/ingress/heartbeat/"+rot.str("token"), nil)
	}
	if r := ingress(t, h, http.StatusTooManyRequests, "GET", "/ingress/heartbeat/"+rot.str("token"), nil); r.header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	h.clock.Advance(time.Minute)

	if err := h.srv.NotifyStatusMonitor(context.Background(), api.StatusMonitorNotifyArgs{WorkspaceID: uuid.MustParse(other.ws), MonitorID: uuid.MustParse(id), Down: true}); err != nil {
		t.Fatal(err)
	}
	st.owner.expect(http.StatusNoContent, "DELETE", st.path("/monitors/"+id), nil)
	ingress(t, h, http.StatusNotFound, "GET", "/ingress/heartbeat/"+rot.str("token"), nil)
}

func TestStatusMonitorNotifiesLikeNewConversation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	email := st.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
	_, token := st.monitor(t, map[string]any{"kind": "push", "name": "Uptime", "component_id": st.api, "on_failure": "none"})
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+token, map[string]any{"state": "partial_outage", "message": "EU workers lag"})
	fanOut := h.jobs("status_monitor_notify", st.ws)
	if len(fanOut) != 1 {
		t.Fatalf("%d notify jobs", len(fanOut))
	}
	var args api.StatusMonitorNotifyArgs
	if err := json.Unmarshal(fanOut[0].args, &args); err != nil {
		t.Fatal(err)
	}
	if !args.Down || args.State != "partial_outage" || args.Message != "EU workers lag" || args.At.IsZero() {
		t.Fatalf("the job does not carry the transition: %+v", args)
	}
	ctx := context.Background()
	memberJobs := func() []riverJob { return h.jobs("status_monitor_notify_member", st.ws) }
	monitorMails := func() []mail.Message {
		var out []mail.Message
		for _, m := range h.mail.to(email) {
			if strings.HasPrefix(m.Subject, "Support: ") {
				out = append(out, m)
			}
		}
		return out
	}
	if err := h.srv.NotifyStatusMonitor(ctx, args); err != nil {
		t.Fatal(err)
	}
	if n := len(memberJobs()); n != 0 {
		t.Fatalf("queued %d jobs for members who did not ask", n)
	}
	st.owner.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{
		"events": map[string]any{"new_async_conversation": map[string]any{"push": true, "email": true}},
	})
	for range 2 {
		if err := h.srv.NotifyStatusMonitor(ctx, args); err != nil {
			t.Fatal(err)
		}
	}
	jobs := memberJobs()
	if len(jobs) != 1 {
		t.Fatalf("a repeated fan-out queued %d member jobs", len(jobs))
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+token, map[string]any{"state": "operational", "message": "recovered"})
	var member api.StatusMonitorNotifyMemberArgs
	if err := json.Unmarshal(jobs[0].args, &member); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.NotifyStatusMonitorMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	got := monitorMails()
	if len(got) != 1 || !strings.Contains(got[0].Subject, "API") || !strings.Contains(got[0].Text, "EU workers lag") || !strings.Contains(got[0].Text, "Partial outage") {
		t.Fatalf("monitor e-mail %+v", got)
	}
	if err := h.srv.NotifyStatusMonitor(ctx, args); err != nil {
		t.Fatal(err)
	}
	if n := len(memberJobs()); n != 1 || len(monitorMails()) != 1 {
		t.Fatalf("a retried fan-out queued %d member jobs and sent %d mails", n, len(monitorMails()))
	}
}

func (st statusTeam) setComponent(t *testing.T, id, state string) {
	t.Helper()
	st.owner.expect(http.StatusOK, "POST", st.path("/components/"+id+"/state"), map[string]any{"state": state})
}

func TestStatusMonitorNeverOverridesLaterMemberState(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	_, token := st.monitor(t, map[string]any{"kind": "push", "name": "Uptime", "component_id": st.api, "on_failure": "none"})
	path := "/ingress/status/" + token
	push := func(state string) {
		t.Helper()
		h.clock.Advance(time.Minute)
		ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": state})
	}
	push("operational")
	h.clock.Advance(time.Minute)
	st.setComponent(t, st.api, "degraded")
	push("major_outage")
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("a new failure after the member's state: %s", got)
	}

	h.clock.Advance(time.Minute)
	st.setComponent(t, st.api, "degraded")
	push("partial_outage")
	if got := st.componentState(t, st.api); got != "degraded" {
		t.Fatalf("the monitor changed its claim over a later member state: %s", got)
	}
	h.clock.Advance(time.Minute)
	other := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"title": "Other", "message": "Looking.", "publish": true, "components": []map[string]any{{"component_id": st.api, "state": "major_outage"}},
	})
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("incident %s", got)
	}
	h.clock.Advance(time.Minute)
	st.owner.expect(http.StatusCreated, "POST", st.path("/incidents/"+other.str("id")+"/updates"), map[string]any{"state": "resolved", "body": "Fixed"})
	if got := st.componentState(t, st.api); got != "operational" {
		t.Fatalf("an unrelated settle applied the overridden monitor again: %s", got)
	}

	push("operational")
	push("major_outage")
	if got := st.componentState(t, st.api); got != "major_outage" {
		t.Fatalf("a later transition of the monitor: %s", got)
	}
}

func (st statusTeam) hasUpdate(t *testing.T, incident, word string) bool {
	t.Helper()
	for _, u := range st.incident(t, incident).body["updates"].([]any) {
		if strings.Contains(u.(map[string]any)["body"].(string), word) {
			return true
		}
	}
	return false
}

func TestStatusMonitorPausedOrDeletedResolvesItsIncident(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	id, token := st.monitor(t, map[string]any{"kind": "push", "name": "Uptime", "component_id": st.api, "on_failure": "publish"})
	path := "/ingress/status/" + token
	holder := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"title": "Slow", "message": "Looking.", "publish": true, "components": []map[string]any{{"component_id": st.api, "state": "degraded"}},
	})

	h.clock.Advance(time.Minute)
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	incident := st.monitorBody(t, id).str("incident_id")
	if st.componentState(t, st.api) != "major_outage" || incident == "" {
		t.Fatal("not down")
	}
	h.clock.Advance(time.Minute)
	st.owner.expect(http.StatusOK, "PATCH", st.path("/monitors/"+id), map[string]any{"paused": true})
	if got := st.incident(t, incident).str("state"); got != "resolved" || !st.hasUpdate(t, incident, "paused") {
		t.Fatalf("incident after pausing: %s", st.incident(t, incident).raw)
	}
	if got := st.componentState(t, st.api); got != "degraded" {
		t.Fatalf("the component is held by the other incident, got %s", got)
	}
	if m := st.monitorBody(t, id); m.body["incident_id"] != nil || m.str("state") != "pending" {
		t.Fatalf("paused monitor %s", m.raw)
	}

	h.clock.Advance(time.Minute)
	st.owner.expect(http.StatusOK, "PATCH", st.path("/monitors/"+id), map[string]any{"paused": false})
	h.clock.Advance(time.Minute)
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	incident = st.monitorBody(t, id).str("incident_id")
	if incident == "" || st.componentState(t, st.api) != "major_outage" {
		t.Fatal("not down again")
	}
	h.clock.Advance(time.Minute)
	st.owner.expect(http.StatusNoContent, "DELETE", st.path("/monitors/"+id), nil)
	if got := st.incident(t, incident).str("state"); got != "resolved" || !st.hasUpdate(t, incident, "removed") {
		t.Fatalf("incident after deleting: %s", st.incident(t, incident).raw)
	}
	if got := st.componentState(t, st.api); got != "degraded" {
		t.Fatalf("component after deleting %s", got)
	}
	if got := st.incident(t, holder.str("id")).str("state"); got == "resolved" {
		t.Fatal("resolved an incident the monitor did not open")
	}
}

func TestStatusMonitorIncidentFollowsTheClaim(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	id, token := st.monitor(t, map[string]any{"kind": "push", "name": "Uptime", "component_id": st.api, "on_failure": "publish"})
	path := "/ingress/status/" + token
	check := func(impact, state string) {
		t.Helper()
		inc := st.incident(t, st.monitorBody(t, id).str("incident_id"))
		comps := inc.body["components"].([]any)
		if inc.str("impact") != impact || len(comps) != 1 || comps[0].(map[string]any)["state"] != state || st.componentState(t, st.api) != state {
			t.Fatalf("want %s/%s: %s, component %s", impact, state, inc.raw, st.componentState(t, st.api))
		}
	}
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	check("major", "major_outage")
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "degraded"})
	check("minor", "degraded")
	ingress(t, h, http.StatusOK, "POST", path, map[string]any{"state": "major_outage"})
	check("major", "major_outage")
}

func TestStatusMonitorIngressLimits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	_, token := st.monitor(t, map[string]any{"kind": "heartbeat", "name": "Backup", "component_id": st.api})
	c := h.client()
	call := func(want int, method, path string, body any) response {
		t.Helper()
		r := c.do(method, path, body)
		if r.status != want {
			t.Fatalf("%s %s: %d %s", method, path, r.status, r.raw)
		}
		return r
	}

	for range 600 {
		call(http.StatusNotFound, "GET", "/ingress/heartbeat/"+unique("guess"), nil)
	}
	if r := call(http.StatusTooManyRequests, "GET", "/ingress/heartbeat/"+token, nil); r.header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	call(http.StatusTooManyRequests, "POST", "/ingress/status/guess", map[string]any{"state": "operational"})
	h.clock.Advance(time.Minute)
	call(http.StatusOK, "GET", "/ingress/heartbeat/"+token, nil)
}

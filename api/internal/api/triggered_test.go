package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jhillyerd/enmime/v2"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (bt broadcastTeam) triggered(t *testing.T, basis string, delay int, extra map[string]any) string {
	t.Helper()
	lists := []string{}
	if basis == "marketing" {
		lists = []string{bt.list}
	}
	body := map[string]any{
		"kind": "triggered", "trigger": map[string]any{"event": "contact_identified", "delay_minutes": delay, "basis": basis},
		"name": "Welcome " + unique("t"), "audience": map[string]any{"list_ids": lists},
	}
	for k, v := range extra {
		body[k] = v
	}
	return bt.draft(t, body)
}

func (bt broadcastTeam) start(t *testing.T, id string, body map[string]any) map[string]any {
	t.Helper()
	b := bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", body).body
	bt.runTriggered(t)
	return b
}

// runTriggered works the start, due and send jobs as the queue would, until nothing is due now.
func (bt broadcastTeam) runTriggered(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, j := range bt.h.jobs("broadcast_start", bt.ws) {
		var a api.BroadcastStartArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		for {
			wait, err := bt.h.srv.StartBroadcast(ctx, a.WorkspaceID, a.BroadcastID, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if wait != 0 {
				break
			}
		}
		bt.h.dropJob(j.id)
	}
	for _, j := range bt.h.jobs("triggered_due", bt.ws) {
		var a api.TriggeredDueArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		for {
			wait, err := bt.h.srv.ProcessTriggered(ctx, a.WorkspaceID, a.BroadcastID, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if wait != 0 {
				break
			}
		}
		bt.h.dropJob(j.id)
	}
	if _, err := bt.h.srv.SendBroadcasts(ctx, uuid.MustParse(bt.ws), uuid.MustParse(bt.emailChannel), time.Minute); err != nil {
		t.Fatal(err)
	}
	for _, j := range bt.h.jobs("broadcast_send", bt.ws) {
		bt.h.dropJob(j.id)
	}
}

// due queues triggered_due for every triggered e-mail, as the sweep does, and works the jobs.
func (bt broadcastTeam) due(t *testing.T, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		if _, err := bt.h.srv.ProcessTriggered(ctx, uuid.MustParse(bt.ws), uuid.MustParse(id), time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	bt.runTriggered(t)
}

func (bt broadcastTeam) identify(t *testing.T, sub, addr, locale string) string {
	t.Helper()
	claims := map[string]any{"sub": sub, "email": addr, "email_verified": true, "name": "User " + sub}
	if locale != "" {
		claims["locale"] = locale
	}
	return bt.session(bt.h, map[string]any{"identity_token": bt.token(bt.h, claims)}).contactID
}

func enrollments(t *testing.T, c *client, id, state string) []map[string]any {
	t.Helper()
	path := "/v1/broadcasts/" + id + "/enrollments?limit=100"
	if state != "" {
		path += "&state=" + state
	}
	var out []map[string]any
	for _, it := range c.expect(http.StatusOK, "GET", path, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func enrollmentOf(t *testing.T, c *client, id, contact string) map[string]any {
	t.Helper()
	for _, e := range enrollments(t, c, id, "") {
		if e["contact"].(map[string]any)["id"] == contact {
			return e
		}
	}
	return nil
}

func envelopeTo(t *testing.T, bs *broadcastSender, addr string) []*enmime.Envelope {
	t.Helper()
	bs.mu.Lock()
	defer bs.mu.Unlock()
	var out []*enmime.Envelope
	for _, m := range bs.sent {
		if m.to[0] == addr {
			env, err := enmime.ReadEnvelope(bytes.NewReader(m.raw))
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, env)
		}
	}
	return out
}

func TestTriggeredServiceFlow(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	setClock(h.clock, t0)
	bt := newBroadcastTeam(t, h, bs)
	bt.owner.expect(http.StatusOK, "PUT", "/v1/email-designs/"+bt.design, map[string]any{"name": "News", "document": versionedDoc(), "base_version": 1})

	before := bt.identify(t, unique("early"), unique("early")+"@example.org", "")
	id := bt.triggered(t, "service", 3*24*60, nil)
	b := bt.get(t, id)
	if b["kind"] != "triggered" || b["trigger"].(map[string]any)["basis"] != "service" || b["state"] != "draft" {
		t.Fatalf("draft: %v", b)
	}
	if item := reviewItem(bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body, "basis"); item["status"] != "warning" {
		t.Fatalf("basis review item: %v", item)
	}
	bt.owner.expectProblem(http.StatusBadRequest, "service_basis_unconfirmed", "POST", "/v1/broadcasts/"+id+"/schedule", nil)
	started := bt.start(t, id, map[string]any{"confirm_service": true})
	if started["service_confirmed"] == nil {
		t.Fatalf("service confirmation: %v", started)
	}
	if b := bt.get(t, id); b["state"] != "active" {
		t.Fatalf("started: %v", b["state"])
	}
	if rows := h.auditRows(bt.ws); !strings.Contains(rows, "broadcast.scheduled") {
		t.Fatalf("audit: %s", rows)
	}
	if e := enrollmentOf(t, bt.owner, id, before); e != nil {
		t.Fatalf("a contact identified before the start was enrolled: %v", e)
	}

	sub, addr := unique("ada"), unique("ada")+"@example.org"
	contact := bt.identify(t, sub, addr, "tr")
	waiting := enrollments(t, bt.owner, id, "waiting")
	if len(waiting) != 1 || waiting[0]["contact"].(map[string]any)["id"] != contact || !timeOf(t, waiting[0]["sends_at"]).Equal(t0.Add(72*time.Hour)) {
		t.Fatalf("waiting: %v", waiting)
	}
	bt.identify(t, sub, addr, "tr")
	bt.runTriggered(t)
	if bs.count() != 0 || len(enrollments(t, bt.owner, id, "")) != 1 {
		t.Fatalf("before the due time: %d mails, %v", bs.count(), enrollments(t, bt.owner, id, ""))
	}

	setClock(h.clock, t0.Add(72*time.Hour+time.Minute))
	bt.due(t, id)
	got := envelopeTo(t, bs, addr)
	if len(got) != 1 {
		t.Fatalf("mails to the contact: %d", len(got))
	}
	m := got[0]
	if !strings.Contains(m.HTML, "Merhaba") || !strings.Contains(m.HTML, "kullandığınız için") {
		t.Fatalf("not the Turkish version with the service footer: %s", m.HTML)
	}
	if m.GetHeader("List-Id") != "" || m.GetHeader("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" || m.GetHeader("List-Unsubscribe") == "" ||
		!strings.Contains(m.GetHeader("Feedback-Id"), ":00000000:") {
		t.Fatalf("headers: %v", m.Root.Header)
	}
	if e := enrollmentOf(t, bt.owner, id, contact); e["state"] != "queued" || e["delivery_id"] == nil {
		t.Fatalf("enrollment after sending: %v", e)
	}
	c := counts(bt.get(t, id))
	if c["enrolled"] != float64(1) || c["waiting"] != float64(0) || c["sent"] != float64(1) {
		t.Fatalf("counts: %v", c)
	}
	bt.identify(t, sub, addr, "tr")
	setClock(h.clock, t0.Add(10*24*time.Hour))
	bt.due(t, id)
	if len(envelopeTo(t, bs, addr)) != 1 {
		t.Fatal("identifying again mailed the contact again")
	}

	address := bt.owner.expect(http.StatusOK, "GET", "/v1/channels/"+bt.emailChannel, nil).body["email"].(map[string]any)["address"].(string)
	msgID := m.GetHeader("Message-Id")
	r := h.ingest(address, buildMail(mailOpts{from: addr, to: address, subject: "Re: Hoş geldiniz", messageID: newMessageID(), body: "Teşekkürler",
		headers: map[string]string{"In-Reply-To": msgID, "References": msgID}}), nil)
	if conv := bt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+r.str("conversation_id"), nil).body; conv["broadcast_id"] != id {
		t.Fatalf("reply conversation: %v", conv)
	}
	ooo := h.ingest(address, buildMail(mailOpts{from: addr, to: address, subject: "Out of office", messageID: newMessageID(), body: "Away",
		headers: map[string]string{"In-Reply-To": msgID, "Auto-Submitted": "auto-replied"}}), nil)
	if ooo.str("status") != "dropped" {
		t.Fatalf("out of office: %v", ooo.body)
	}
	if c := counts(bt.get(t, id)); c["replied"] != float64(1) || c["auto_replied"] != float64(1) {
		t.Fatalf("reply counts: %v", c)
	}

	stop := strings.TrimPrefix(strings.Trim(m.GetHeader("List-Unsubscribe"), "<>"), "http://localhost:8080")
	if st, body := pageAt(t, h, "GET", stop, nil); st != http.StatusOK || !strings.Contains(body, "Stop") && !strings.Contains(body, "durdur") {
		t.Fatalf("stop page: %d %s", st, body)
	}
	if st, _ := pageAt(t, h, "POST", stop, url.Values{"List-Unsubscribe": {"One-Click"}}); st != http.StatusOK {
		t.Fatalf("one-click: %d", st)
	}
	var source string
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT source FROM inbox_mail_optouts WHERE workspace_id = $1 AND contact_id = $2`, bt.ws, contact).Scan(&source); err != nil || source != "triggered" {
		t.Fatalf("inbox opt-out: %q %v", source, err)
	}
	if c := counts(bt.get(t, id)); c["unsubscribed"] != float64(1) {
		t.Fatalf("unsubscribed count: %v", c)
	}

	second := bt.triggered(t, "service", 4*24*60, map[string]any{"name": "Feedback"})
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+second+"/review", nil).body
	if review["recipients"] != float64(0) {
		t.Fatalf("without back-fill nobody is counted: %v", review["recipients"])
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+second, map[string]any{"trigger": map[string]any{"backfill_days": 30}})
	review = bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+second+"/review", nil).body
	if review["recipients"] != float64(2) {
		t.Fatalf("back-fill count over 30 days: %v", review["recipients"])
	}
	bt.start(t, second, map[string]any{"confirm_service": true, "confirm_recipients": 2})
	if e := enrollmentOf(t, bt.owner, second, contact); e == nil || e["state"] != "skipped" || e["skip_reason"] != "stopped" || e["source"] != "backfill" {
		t.Fatalf("a stopped contact in a second service mail: %v", e)
	}
	if len(envelopeTo(t, bs, addr)) != 1 {
		t.Fatal("a stopped contact got the second service mail")
	}
}

func TestTriggeredBackfill(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	setClock(h.clock, t0.Add(-10*24*time.Hour))
	bt := newBroadcastTeam(t, h, bs)
	old := bt.identify(t, unique("old"), unique("old")+"@example.org", "")
	setClock(h.clock, t0.Add(-2*24*time.Hour))
	recent := bt.identify(t, unique("recent"), unique("recent")+"@example.org", "")
	setClock(h.clock, t0)

	none := bt.triggered(t, "service", 60, nil)
	bt.start(t, none, map[string]any{"confirm_service": true})
	if got := enrollments(t, bt.owner, none, ""); len(got) != 0 {
		t.Fatalf("contacts identified before the start were enrolled: %v", got)
	}

	id := bt.triggered(t, "service", 60, map[string]any{"name": "Back-fill"})
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"trigger": map[string]any{"backfill_days": 7}})
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if review["recipients"] != float64(1) {
		t.Fatalf("review back-fill count: %v", review["recipients"])
	}
	bt.owner.expectProblem(http.StatusConflict, "audience_changed", "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true, "confirm_recipients": 2})
	bt.start(t, id, map[string]any{"confirm_service": true, "confirm_recipients": 1})
	got := enrollments(t, bt.owner, id, "")
	if len(got) != 1 || got[0]["contact"].(map[string]any)["id"] != recent || got[0]["source"] != "backfill" || enrollmentOf(t, bt.owner, id, old) != nil {
		t.Fatalf("back-fill: %v", got)
	}
	if got[0]["state"] != "queued" || bs.count() != 1 {
		t.Fatalf("a back-filled contact whose due time passed goes out at once: %v, %d mails", got[0], bs.count())
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", "/v1/broadcasts/"+id, map[string]any{"trigger": map[string]any{"basis": "marketing"}})
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", "/v1/broadcasts/"+id, map[string]any{"subject": "Changed"})
	if b := bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"trigger": map[string]any{"delay_minutes": 120}}).body; b["trigger"].(map[string]any)["delay_minutes"] != float64(120) {
		t.Fatalf("delay change: %v", b["trigger"])
	}
}

func TestTriggeredMarketing(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC))
	bt := newBroadcastTeam(t, h, bs)
	bt.owner.expectProblem(http.StatusBadRequest, "basis_lists", "POST", "/v1/broadcasts", map[string]any{
		"inbox_id": bt.chatInbox, "name": "No lists", "kind": "triggered", "audience": map[string]any{"list_ids": []string{}},
		"trigger": map[string]any{"event": "contact_identified", "delay_minutes": 0, "basis": "marketing"},
	})
	bt.owner.expectProblem(http.StatusBadRequest, "trigger_invalid", "POST", "/v1/broadcasts", map[string]any{
		"inbox_id": bt.chatInbox, "name": "Once", "trigger": map[string]any{"event": "contact_identified", "delay_minutes": 0, "basis": "service"},
	})
	id := bt.triggered(t, "marketing", 0, nil)
	bt.start(t, id, nil)

	subscribed := unique("sub") + "@example.org"
	bt.subscribe(t, subscribed)
	in := bt.identify(t, unique("s"), subscribed, "")
	out := bt.identify(t, unique("n"), unique("not")+"@example.org", "")
	bt.due(t, id)
	if e := enrollmentOf(t, bt.owner, id, out); e["skip_reason"] != "not_subscribed" {
		t.Fatalf("a contact not on the list: %v", e)
	}
	if e := enrollmentOf(t, bt.owner, id, in); e["state"] != "queued" {
		t.Fatalf("a subscribed contact: %v", e)
	}
	got := envelopeTo(t, bs, subscribed)
	if len(got) != 1 || got[0].GetHeader("List-Id") == "" {
		t.Fatalf("marketing mail: %d", len(got))
	}
	unsub := strings.TrimPrefix(strings.Trim(got[0].GetHeader("List-Unsubscribe"), "<>"), "http://localhost:8080")
	if st, _ := pageAt(t, h, "POST", unsub, url.Values{"List-Unsubscribe": {"One-Click"}}); st != http.StatusOK {
		t.Fatalf("one-click: %d", st)
	}
	if s := bt.subscription(t, subscribed); s["state"] != "unsubscribed" {
		t.Fatalf("subscription after one-click: %v", s["state"])
	}
	once := bt.draft(t, map[string]any{"name": "Later news"})
	if n := bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+once+"/audience", nil).body["recipients"]; n != float64(0) {
		t.Fatalf("a later broadcast to the list still counts the address: %v", n)
	}
}

func TestTriggeredPausedLate(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	t0 := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	setClock(h.clock, t0)
	bt := newBroadcastTeam(t, h, bs)
	id := bt.triggered(t, "service", 24*60, nil)
	bt.start(t, id, map[string]any{"confirm_service": true})
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/pause", nil)
	early := unique("early") + "@example.org"
	bt.identify(t, unique("e"), early, "")
	setClock(h.clock, t0.Add(5*24*time.Hour))
	later := unique("later") + "@example.org"
	bt.identify(t, unique("l"), later, "")
	if n := len(enrollments(t, bt.owner, id, "waiting")); n != 2 {
		t.Fatalf("pausing stops enrolling: %d waiting", n)
	}
	setClock(h.clock, t0.Add(8*24*time.Hour+time.Hour))
	bt.due(t, id)
	if bs.count() != 0 {
		t.Fatal("a paused triggered e-mail sent")
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"subject": "Welcome again"})
	if b := bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/resume", nil).body; b["state"] != "active" {
		t.Fatalf("resume: %v", b["state"])
	}
	bt.runTriggered(t)
	if len(envelopeTo(t, bs, early)) != 0 || len(envelopeTo(t, bs, later)) != 1 {
		t.Fatalf("after the pause: early %d, later %d", len(envelopeTo(t, bs, early)), len(envelopeTo(t, bs, later)))
	}
	if subj := envelopeTo(t, bs, later)[0].GetHeader("Subject"); subj != "Welcome again" {
		t.Fatalf("the subject changed while paused: %q", subj)
	}
	var lateN int
	for _, e := range enrollments(t, bt.owner, id, "skipped") {
		if e["skip_reason"] == "late" {
			lateN++
		}
	}
	if lateN != 1 || counts(bt.get(t, id))["skipped"].(map[string]any)["late"] != float64(1) {
		t.Fatalf("late: %d, %v", lateN, counts(bt.get(t, id)))
	}

	contact := bt.identify(t, unique("w"), unique("w")+"@example.org", "")
	path := "/v1/broadcasts/" + id + "/enrollments/" + contact
	bt.owner.expect(http.StatusNoContent, "DELETE", path, nil)
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "DELETE", path, nil)
	bt.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/broadcasts/"+id+"/enrollments/"+uuid.NewV7().String(), nil)
	if e := enrollmentOf(t, bt.owner, id, contact); e["skip_reason"] != "removed" {
		t.Fatalf("removed: %v", e)
	}
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/cancel", nil)
	if b := bt.get(t, id); b["state"] != "cancelled" {
		t.Fatalf("stopped: %v", b["state"])
	}
	stopped := bt.identify(t, unique("x"), unique("x")+"@example.org", "")
	if enrollmentOf(t, bt.owner, id, stopped) != nil {
		t.Fatal("a stopped triggered e-mail enrolled")
	}
}

func TestTriggeredAccessAndSkipTriggers(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	id := bt.triggered(t, "service", 60, nil)
	key := h.client()
	key.bearer = bt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "s", "scopes": []string{"broadcasts:read", "broadcasts:send", "contacts:read"}}).str("secret")
	key.expectProblem(http.StatusForbidden, "broadcasts_by_keys_disabled", "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true})
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"broadcasts_by_keys": true})
	key.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true})
	bt.runTriggered(t)

	skipped := bt.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Back-filled", "emails": []string{unique("bf") + "@example.org"}, "skip_triggers": true,
		"external_ids": []map[string]any{{"inbox_id": bt.chatInbox, "external_id": unique("bf")}},
	}).str("id")
	enrolled := bt.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "New user", "emails": []string{unique("nu") + "@example.org"},
		"external_ids": []map[string]any{{"inbox_id": bt.chatInbox, "external_id": unique("nu")}},
	}).str("id")
	if enrollmentOf(t, bt.owner, id, skipped) != nil || enrollmentOf(t, bt.owner, id, enrolled) == nil {
		t.Fatalf("skip_triggers: %v", enrollments(t, bt.owner, id, ""))
	}
	if items := enrollments(t, key, id, "waiting"); len(items) != 1 {
		t.Fatalf("a key reads the waiting list: %v", items)
	}

	other := newBroadcastTeam(t, h, bs)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/broadcasts/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/broadcasts/"+id+"/enrollments", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/broadcasts/"+id+"/enrollments/"+enrolled, nil)
	if items := other.owner.expect(http.StatusOK, "GET", "/v1/broadcasts?kind=triggered", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("another workspace lists the triggered e-mail: %v", items)
	}
	if items := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts?kind=triggered", nil).body["items"].([]any); len(items) != 1 {
		t.Fatalf("kind filter: %d", len(items))
	}
	once := bt.draft(t, nil)
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/broadcasts/"+once+"/enrollments", nil)
}

func TestTriggeredPausedAtStart(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	ctx := context.Background()
	t0 := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	setClock(h.clock, t0.Add(-10*24*time.Hour))
	bt := newBroadcastTeam(t, h, bs)
	old := unique("old") + "@example.org"
	bt.identify(t, unique("o"), old, "")
	setClock(h.clock, t0)

	id := bt.triggered(t, "service", 60, nil)
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"trigger": map[string]any{"backfill_days": 14}})
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true, "confirm_recipients": 1})
	if _, err := h.st.Pool.Exec(ctx, `UPDATE workspaces SET broadcasts_status = 'suspended' WHERE id = $1`, bt.ws); err != nil {
		t.Fatal(err)
	}
	bt.runTriggered(t)
	if b := bt.get(t, id); b["state"] != "paused" || b["pause_reason"] != "workspace_suspended" {
		t.Fatalf("started while suspended: %v %v", b["state"], b["pause_reason"])
	}
	if _, err := h.st.Pool.Exec(ctx, `UPDATE workspaces SET broadcasts_status = 'allowed' WHERE id = $1`, bt.ws); err != nil {
		t.Fatal(err)
	}
	setClock(h.clock, t0.Add(2*24*time.Hour))
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/resume", nil)
	bt.runTriggered(t)
	var started *time.Time
	var done bool
	if err := h.st.Pool.QueryRow(ctx, `SELECT started_at, snapshot_done FROM broadcasts WHERE workspace_id = $1 AND id = $2`, bt.ws, id).Scan(&started, &done); err != nil {
		t.Fatal(err)
	}
	if started == nil || !started.Equal(t0.Add(2*24*time.Hour)) || !done {
		t.Fatalf("resume did not finish the start: started %v, back-fill done %v", started, done)
	}
	if n := len(h.jobs("broadcast_start", bt.ws)); n != 0 {
		t.Fatalf("%d start jobs left after the back-fill", n)
	}
	if len(envelopeTo(t, bs, old)) != 1 {
		t.Fatalf("a contact back-filled from 12 days before goes out, not late: %v", enrollments(t, bt.owner, id, ""))
	}
}

func (bt broadcastTeam) triggeredOn(t *testing.T, basis string, delay int, trigger map[string]any) string {
	t.Helper()
	trigger["delay_minutes"], trigger["basis"] = delay, basis
	return bt.triggered(t, basis, delay, map[string]any{"trigger": trigger, "name": "On " + trigger["event"].(string) + " " + unique("t")})
}

func (bt broadcastTeam) identifyWith(t *testing.T, sub, addr string, attrs map[string]any) string {
	t.Helper()
	claims := map[string]any{"sub": sub, "email": addr, "email_verified": true, "name": "User " + sub, "attrs": attrs}
	return bt.session(bt.h, map[string]any{"identity_token": bt.token(bt.h, claims)}).contactID
}

func broadcastMails(t *testing.T, bs *broadcastSender, addr string) []*enmime.Envelope {
	t.Helper()
	var out []*enmime.Envelope
	for _, m := range envelopeTo(t, bs, addr) {
		if m.GetHeader("List-Id") != "" {
			out = append(out, m)
		}
	}
	return out
}

func TestTriggeredSubscribed(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Imports.Resolver = importMX{} })
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	setClock(h.clock, t0)
	bt := newBroadcastTeam(t, h, bs)
	other := newBroadcastTeam(t, h, bs)
	base := map[string]any{"inbox_id": bt.chatInbox, "name": "Bad", "kind": "triggered", "audience": map[string]any{"list_ids": []string{bt.list}}}
	for _, tc := range []struct {
		code    string
		trigger map[string]any
	}{
		{"trigger_invalid", map[string]any{"event": "subscribed"}},
		{"trigger_invalid", map[string]any{"event": "subscribed", "list_id": bt.list, "condition": map[string]any{"attribute": "plan", "op": "eq", "value": "pro"}}},
		{"trigger_invalid", map[string]any{"event": "contact_identified", "list_id": bt.list}},
		{"validation_failed", map[string]any{"event": "subscribed", "list_id": other.list}},
	} {
		tc.trigger["delay_minutes"], tc.trigger["basis"] = 0, "marketing"
		body := map[string]any{"trigger": tc.trigger}
		for k, v := range base {
			body[k] = v
		}
		bt.owner.expectProblem(http.StatusBadRequest, tc.code, "POST", "/v1/broadcasts", body)
	}

	early := unique("early") + "@example.org"
	bt.subscribe(t, early)
	setClock(h.clock, t0.Add(time.Hour))
	first := bt.triggeredOn(t, "marketing", 0, map[string]any{"event": "subscribed", "list_id": bt.list})
	if tr := bt.get(t, first)["trigger"].(map[string]any); tr["list_id"] != bt.list || tr["event"] != "subscribed" {
		t.Fatalf("trigger: %v", tr)
	}
	if item := reviewItem(bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+first+"/review", nil).body, "trigger"); !strings.Contains(item["detail"].(string), "subscribes to Product news") {
		t.Fatalf("trigger review item: %v", item)
	}
	second := bt.triggeredOn(t, "marketing", 3*24*60, map[string]any{"event": "subscribed", "list_id": bt.list})
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+second, map[string]any{"trigger": map[string]any{"backfill_days": 1}})
	if r := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+second+"/review", nil).body; r["recipients"] != float64(1) {
		t.Fatalf("back-fill count of confirmed subscriptions: %v", r["recipients"])
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+second, map[string]any{"trigger": map[string]any{"backfill_days": 0}})
	bt.start(t, first, nil)
	bt.start(t, second, nil)
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", "/v1/broadcasts/"+first, map[string]any{"trigger": map[string]any{"list_id": other.list}})
	if len(enrollments(t, bt.owner, first, "")) != 0 {
		t.Fatal("a subscription from before the start enrolled")
	}
	bt.owner.expectProblem(http.StatusConflict, "list_in_use", "DELETE", "/v1/lists/"+bt.list, nil)
	backdated := unique("backdated") + "@example.org"
	bt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+bt.list+"/subscriptions", map[string]any{
		"email": backdated, "consent": map[string]any{"confirmed": true, "obtained_at": h.clock.Now().Add(-20 * 24 * time.Hour).Format(time.RFC3339)},
	})
	bt.runTriggered(t)
	if got := broadcastMails(t, bs, backdated); len(got) != 1 {
		t.Fatalf("a subscription with an earlier consent time counts from the write, not as late: %d mails, %v", len(got), enrollments(t, bt.owner, first, ""))
	}

	c, token := bt.form(h)
	addr := unique("reader") + "@example.org"
	h.clock.Advance(3 * time.Second)
	c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+bt.key+"/subscriptions", bt.subscribeBody(addr, token))
	h.runListMail(bt.ws)
	mails := envelopeTo(t, bs, addr)
	if len(mails) != 1 {
		t.Fatalf("confirmation mails: %d", len(mails))
	}
	bt.runTriggered(t)
	if n := len(enrollments(t, bt.owner, first, "")); n != 1 || len(envelopeTo(t, bs, addr)) != 1 {
		t.Fatalf("a pending subscription enrolled: %d", n)
	}
	if st, _ := pageAt(t, h, "POST", linkIn(t, mails[0], "s"), url.Values{"action": {"confirm"}}); st != http.StatusOK {
		t.Fatalf("confirm: %d", st)
	}
	confirmedAt := h.clock.Now()
	bt.runTriggered(t)
	if got := broadcastMails(t, bs, addr); len(got) != 1 {
		t.Fatalf("the first mail after confirming: %d", len(got))
	}
	w := enrollments(t, bt.owner, second, "waiting")
	if len(w) != 2 || w[1]["contact"].(map[string]any)["email"] != addr || !timeOf(t, w[1]["triggered_at"]).Equal(confirmedAt) {
		t.Fatalf("the second waits from the confirmation: %v", w)
	}

	cs := h.mcpSession(h.oauthToken(bt.owner, bt.ws, "Claude Code", testOrigin+"/mcp", []string{"lists:read", "lists:write", "broadcasts:read", "broadcasts:write", "contacts:read"}))
	imported := unique("imported") + "@example.org"
	imp, errText := callTool(t, cs, "create_list_import", map[string]any{
		"list_id": bt.list, "csv": "email,name\n" + imported + ",Imp\n", "attestation": "They opted in.", "consent_source": "Sign-up form, 2026",
	})
	if errText != "" {
		t.Fatal(errText)
	}
	runListImports(t, h, bt.ws)
	if _, errText := callTool(t, cs, "start_list_import", map[string]any{"list_id": bt.list, "import_id": imp["id"], "confirm_count": 1}); errText != "" {
		t.Fatal(errText)
	}
	runListImports(t, h, bt.ws)
	if s := bt.subscription(t, imported); s["state"] != "subscribed" {
		t.Fatalf("imported subscription: %v", s)
	}
	if n := len(enrollments(t, bt.owner, first, "")); n != 2 {
		t.Fatalf("an import enrolled: %d enrollments", n)
	}

	setClock(h.clock, confirmedAt.Add(3*24*time.Hour+time.Minute))
	bt.due(t, second)
	if got := broadcastMails(t, bs, addr); len(got) != 2 {
		t.Fatalf("the second mail 3 days later: %d", len(got))
	}
	if len(broadcastMails(t, bs, imported)) != 0 || len(broadcastMails(t, bs, early)) != 0 {
		t.Fatal("an imported or earlier subscriber got a welcome mail")
	}

	cs = h.mcpSession(h.oauthToken(bt.owner, bt.ws, "Claude Code", testOrigin+"/mcp", []string{"broadcasts:read", "broadcasts:write"}))
	att, errText := callTool(t, cs, "create_triggered_email", map[string]any{
		"inbox_id": bt.chatInbox, "channel_id": bt.emailChannel, "name": "Pro welcome", "subject": "Welcome to Pro", "design_id": bt.design,
		"event": "attribute_changed", "trigger_condition": map[string]any{"attribute": "plan", "op": "eq", "value": "pro"},
		"delay": 0, "delay_unit": "minutes", "basis": "service",
	})
	if errText != "" {
		t.Fatal(errText)
	}
	if tr := att["trigger"].(map[string]any); tr["event"] != "attribute_changed" || tr["condition"].(map[string]any)["value"] != "pro" {
		t.Fatalf("create_triggered_email trigger: %v", tr)
	}
	sub, errText := callTool(t, cs, "create_triggered_email", map[string]any{
		"inbox_id": bt.chatInbox, "name": "Welcome", "subject": "Welcome", "event": "subscribed", "trigger_list_id": bt.list,
		"delay": 1, "delay_unit": "days", "basis": "marketing", "list_ids": []string{bt.list},
	})
	if errText != "" || sub["trigger"].(map[string]any)["list_id"] != bt.list {
		t.Fatalf("create_triggered_email subscribed: %v %s", sub, errText)
	}
}

func TestTriggeredAttributeChanged(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	setClock(h.clock, time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC))
	bt := newBroadcastTeam(t, h, bs)
	cond := map[string]any{"attribute": "plan", "op": "eq", "value": "pro"}
	bt.owner.expectProblem(http.StatusBadRequest, "trigger_invalid", "POST", "/v1/broadcasts", map[string]any{
		"inbox_id": bt.chatInbox, "name": "No condition", "kind": "triggered", "audience": map[string]any{"list_ids": []string{}},
		"trigger": map[string]any{"event": "attribute_changed", "delay_minutes": 0, "basis": "service"},
	})
	id := bt.triggeredOn(t, "service", 0, map[string]any{"event": "attribute_changed", "condition": cond})
	bt.owner.expectProblem(http.StatusBadRequest, "trigger_invalid", "PATCH", "/v1/broadcasts/"+id, map[string]any{"trigger": map[string]any{"backfill_days": 3}})
	bt.owner.expectProblem(http.StatusBadRequest, "trigger_invalid", "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true, "backfill_days": 3})

	alreadySub, alreadyAddr := unique("pro"), unique("pro")+"@example.org"
	already := bt.identifyWith(t, alreadySub, alreadyAddr, map[string]any{"plan": "pro"})
	bt.start(t, id, map[string]any{"confirm_service": true})
	bt.identifyWith(t, alreadySub, alreadyAddr, map[string]any{"plan": "pro"})
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_service": true, "backfill_days": 3})
	if enrollmentOf(t, bt.owner, id, already) != nil {
		t.Fatal("a contact already pro when it started was enrolled")
	}

	sub, addr := unique("u"), unique("u")+"@example.org"
	user := bt.identifyWith(t, sub, addr, map[string]any{"plan": "free"})
	if enrollmentOf(t, bt.owner, id, user) != nil {
		t.Fatal("a contact not matching was enrolled")
	}
	bt.identifyWith(t, sub, addr, map[string]any{"plan": "pro"})
	bt.runTriggered(t)
	if got := envelopeTo(t, bs, addr); len(got) != 1 {
		t.Fatalf("moving plan from free to pro: %d mails", len(got))
	}
	bt.identifyWith(t, sub, addr, map[string]any{"plan": "pro"})
	bt.identifyWith(t, sub, addr, map[string]any{"plan": "free"})
	bt.identifyWith(t, sub, addr, map[string]any{"plan": "pro"})
	bt.runTriggered(t)
	if got := envelopeTo(t, bs, addr); len(got) != 1 {
		t.Fatalf("setting pro again: %d mails", len(got))
	}

	quiet := bt.identifyWith(t, unique("q"), unique("q")+"@example.org", map[string]any{"plan": "free"})
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+quiet, map[string]any{"attributes": map[string]any{"plan": "pro"}, "skip_triggers": true})
	loud := bt.identifyWith(t, unique("l"), unique("l")+"@example.org", map[string]any{"plan": "free"})
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+loud, map[string]any{"attributes": map[string]any{"plan": "pro"}})
	created := bt.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Pro from the start", "emails": []string{unique("c") + "@example.org"}, "attributes": map[string]any{"plan": "pro"},
		"external_ids": []map[string]any{{"inbox_id": bt.chatInbox, "external_id": unique("c")}},
	}).str("id")
	outsider := bt.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Not of the inbox", "emails": []string{unique("o") + "@example.org"}, "attributes": map[string]any{"plan": "pro"},
	}).str("id")
	if enrollmentOf(t, bt.owner, id, quiet) != nil || enrollmentOf(t, bt.owner, id, outsider) != nil {
		t.Fatalf("skip_triggers or a contact outside the inbox enrolled: %v", enrollments(t, bt.owner, id, ""))
	}
	if enrollmentOf(t, bt.owner, id, loud) == nil || enrollmentOf(t, bt.owner, id, created) == nil {
		t.Fatalf("a /v1/contacts write that newly matches: %v", enrollments(t, bt.owner, id, ""))
	}

	other := newBroadcastTeam(t, h, bs)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/broadcasts/"+id+"/enrollments", nil)
	foreign := other.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Elsewhere", "emails": []string{unique("f") + "@example.org"}, "attributes": map[string]any{"plan": "pro"},
		"external_ids": []map[string]any{{"inbox_id": other.chatInbox, "external_id": unique("f")}},
	}).str("id")
	if enrollmentOf(t, bt.owner, id, foreign) != nil {
		t.Fatal("a contact of another workspace enrolled")
	}
}

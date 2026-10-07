package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

type receivedHook struct {
	header http.Header
	body   []byte
	typ    string
	data   map[string]any
}

type receiver struct {
	mu     sync.Mutex
	secret []byte
	status int
	hooks  []receivedHook
	srv    *httptest.Server
}

func newReceiver(t *testing.T) *receiver {
	r := &receiver{status: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var env struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(body, &env)
		r.mu.Lock()
		defer r.mu.Unlock()
		r.hooks = append(r.hooks, receivedHook{header: req.Header.Clone(), body: body, typ: env.Type, data: env.Data})
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) setStatus(s int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = s
}

func (r *receiver) take() []receivedHook {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.hooks
	r.hooks = nil
	return out
}

func verifySignature(t *testing.T, secret string, h receivedHook) {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(h.header.Get("webhook-id") + "." + h.header.Get("webhook-timestamp") + "."))
	mac.Write(h.body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, sig := range strings.Fields(h.header.Get("webhook-signature")) {
		if hmac.Equal([]byte(sig), []byte(want)) {
			return
		}
	}
	t.Fatalf("no valid signature in %q", h.header.Get("webhook-signature"))
}

func privateHarness(t *testing.T) *harness {
	return newHarnessWith(t, func(d *api.Deps) { d.Webhooks.AllowPrivate = true })
}

type riverJob struct {
	id      int64
	attempt int
	max     int
	args    []byte
}

func (h *harness) jobs(kind, ws string) []riverJob {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(),
		"SELECT id, attempt, max_attempts, args FROM river_job WHERE kind = $1 AND args->>'workspace_id' = $2 ORDER BY id", kind, ws)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var out []riverJob
	for rows.Next() {
		var j riverJob
		if err := rows.Scan(&j.id, &j.attempt, &j.max, &j.args); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, j)
	}
	return out
}

func (h *harness) dropJob(id int64) {
	if _, err := h.st.Pool.Exec(context.Background(), "DELETE FROM river_job WHERE id = $1", id); err != nil {
		h.t.Fatal(err)
	}
}

// runWebhooks runs the queued fan-outs and then one attempt of each queued delivery, as the job
// queue would, and reports how many attempts asked to be retried.
func (h *harness) runWebhooks(ws string) int {
	h.t.Helper()
	ctx := context.Background()
	for _, j := range h.jobs("webhook_fanout", ws) {
		var a api.WebhookFanoutArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.FanOutWebhooks(ctx, a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	retries := 0
	for _, j := range h.jobs("webhook_delivery", ws) {
		var a api.WebhookDeliveryArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		retry, err := h.srv.DeliverWebhook(ctx, a.WorkspaceID, a.DeliveryID, 1, j.max, a.Manual)
		if err != nil {
			h.t.Fatal(err)
		}
		if retry {
			retries++
		}
		h.dropJob(j.id)
	}
	return retries
}

type appTeam struct {
	team
	appInbox string
	secret   string
	channel  string
	key      string
}

func newAppTeam(t *testing.T, h *harness, anonymous bool) appTeam {
	t.Helper()
	tm := newTeam(t, h)
	in := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "App", "slug": "app", "mode": "async"})
	inbox := in.body["inbox"].(map[string]any)["id"].(string)
	body := map[string]any{"kind": "app", "name": "iOS and Android"}
	if anonymous {
		body["app"] = map[string]any{"allow_anonymous": true}
	}
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", body)
	app := ch.body["app"].(map[string]any)
	return appTeam{team: tm, appInbox: inbox, secret: in.str("identity_secret"), channel: ch.str("id"), key: app["public_key"].(string)}
}

func (at appTeam) session(h *harness, sub string) contactSession {
	h.t.Helper()
	c := h.client()
	claims := map[string]any{"sub": sub, "name": "App User", "exp": h.clock.Now().Add(5 * time.Minute).Unix()}
	tok := signToken(at.secret, map[string]any{"alg": "HS256", "typ": "JWT"}, claims)
	r := c.expect(http.StatusCreated, "POST", "/client/v1/session", map[string]any{"channel_key": at.key, "identity_token": tok})
	c.bearer = r.str("token")
	return contactSession{client: c, token: c.bearer, contactID: r.body["contact"].(map[string]any)["id"].(string), body: r.body}
}

func TestAppChannelNeedsNoOrigin(t *testing.T) {
	h := newHarness(t)
	at := newAppTeam(t, h, false)
	ch := at.owner.expect(http.StatusOK, "GET", "/v1/channels/"+at.channel, nil)
	app := ch.body["app"].(map[string]any)
	if app["allow_anonymous"] != false || len(app["platforms"].([]any)) != 2 || ch.body["chat"] != nil {
		t.Fatalf("app channel: %s", ch.raw)
	}
	at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/channels/"+at.channel, map[string]any{
		"chat": map[string]any{"allowed_origins": []string{"https://x.example"}},
	})
	upd := at.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+at.channel, map[string]any{"app": map[string]any{"platforms": []string{"android"}}})
	if p := upd.body["app"].(map[string]any)["platforms"].([]any); len(p) != 1 || p[0] != "android" {
		t.Fatalf("platforms: %s", upd.raw)
	}
	rotated := at.owner.expect(http.StatusOK, "POST", "/v1/channels/"+at.channel+"/public-key", nil)
	newKey := rotated.body["app"].(map[string]any)["public_key"].(string)
	if newKey == at.key {
		t.Fatal("key not rotated")
	}
	at.key = newKey

	c := h.client()
	r := c.expect(http.StatusOK, "GET", "/client/v1/channels/"+at.key, nil)
	if cats := r.body["feedback_categories"].([]any); len(cats) != 4 || cats[0] != "bug" {
		t.Fatalf("feedback categories: %s", r.raw)
	}
	c.expectProblem(http.StatusForbidden, "anonymous_not_allowed", "POST", "/client/v1/session", map[string]any{"channel_key": at.key})
	cs := at.session(h, "app-user-1")
	cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil)
	ws, status, err := dialContact(h, cs.token, "", "")
	if err != nil {
		t.Fatalf("app socket without Origin: %d %v", status, err)
	}
	ws.close()

	ct := newChatTeam(t, h, "async", true)
	noOrigin := h.client()
	noOrigin.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", "/client/v1/channels/"+ct.key, nil)
	noOrigin.expectProblem(http.StatusForbidden, "origin_not_allowed", "POST", "/client/v1/session", map[string]any{"channel_key": ct.key})
	chatSession := ct.session(h, map[string]any{})
	chatSession.origin = ""
	chatSession.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", "/client/v1/conversations", nil)
	if _, status, err := dialContact(h, chatSession.token, "", ""); err == nil || status != http.StatusForbidden {
		t.Fatalf("chat socket without Origin: %d %v", status, err)
	}
	chatSession.origin = ct.origin
	chatSession.expectProblem(http.StatusForbidden, "forbidden", "POST", "/client/v1/feedback", map[string]any{"category": "bug", "body": "x"})
}

func TestClientFeedback(t *testing.T) {
	h := newHarness(t)
	at := newAppTeam(t, h, false)
	cs := at.session(h, "user-42")
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/feedback", map[string]any{"category": "rant", "body": "x"})
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/feedback", map[string]any{"category": "bug"})
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/feedback", map[string]any{
		"category": "bug", "body": "x", "app_version": strings.Repeat("9", 51),
	})
	req := map[string]any{
		"category": "bug", "body": "The app crashes when I open settings", "client_id": "fb-1",
		"app_version": "2.3.1", "build": "231", "os": "iOS", "os_version": "18.1", "device_model": "iPhone17,1",
		"locale": "tr-TR", "screen": "Settings", "installation_id": "inst-1", "allow_email": true, "email": "success@simulator.amazonses.com",
	}
	r := cs.expect(http.StatusCreated, "POST", "/client/v1/feedback", req)
	conv := r.body["conversation"].(map[string]any)
	fb := conv["feedback"].(map[string]any)
	if conv["kind"] != "feedback" || fb["category"] != "bug" || fb["app_version"] != "2.3.1" || fb["device_model"] != "iPhone17,1" || fb["allow_email"] != true {
		t.Fatalf("feedback conversation: %s", r.raw)
	}
	again := cs.expect(http.StatusCreated, "POST", "/client/v1/feedback", req)
	if again.body["conversation"].(map[string]any)["id"] != conv["id"] {
		t.Fatalf("client_id is not idempotent: %s", again.raw)
	}
	sess := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil)
	if sess.body["contact"].(map[string]any)["typed_email"] != "success@simulator.amazonses.com" {
		t.Fatalf("allowed e-mail not kept: %s", sess.raw)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"category": "idea", "body": "Dark mode please", "os": "Android", "allow_email": "false", "screen": "Home"} {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("files", "shot.png")
	_, _ = fw.Write([]byte("\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 40)))
	_ = mw.Close()
	hr, _ := http.NewRequest("POST", h.url+"/client/v1/feedback", &buf)
	hr.Header.Set("Content-Type", mw.FormDataContentType())
	hr.Header.Set("Authorization", "Bearer "+cs.token)
	res, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("multipart feedback: %d %s", res.StatusCode, raw)
	}
	var mp map[string]any
	_ = json.Unmarshal(raw, &mp)
	if atts := mp["message"].(map[string]any)["attachments"].([]any); len(atts) != 1 || mp["conversation"].(map[string]any)["feedback"].(map[string]any)["category"] != "idea" {
		t.Fatalf("multipart feedback: %s", raw)
	}

	list := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil)
	if items := list.body["items"].([]any); len(items) != 2 || items[0].(map[string]any)["kind"] != "feedback" {
		t.Fatalf("client list: %s", list.raw)
	}
	all := at.owner.expect(http.StatusOK, "GET", "/v1/conversations?kind=feedback", nil)
	if len(all.body["items"].([]any)) != 2 {
		t.Fatalf("kind filter: %s", all.raw)
	}
	bugs := at.owner.expect(http.StatusOK, "GET", "/v1/conversations?category=bug", nil)
	items := bugs.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["feedback"].(map[string]any)["screen"] != "Settings" {
		t.Fatalf("category filter: %s", bugs.raw)
	}
	at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?kind=conversation&category=bug", nil)
	plain := at.owner.expect(http.StatusOK, "GET", "/v1/conversations?kind=conversation", nil)
	for _, it := range plain.body["items"].([]any) {
		if it.(map[string]any)["kind"] != "conversation" {
			t.Fatalf("kind=conversation: %s", plain.raw)
		}
	}
	counts := at.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil)
	if counts.body["feedback"] != float64(2) || len(counts.body["feedback_categories"].([]any)) != 2 {
		t.Fatalf("counts: %s", counts.raw)
	}
}

func TestAPIFeedbackAndDeleteByExternalID(t *testing.T) {
	h := privateHarness(t)
	tm := newTeam(t, h)
	key := tm.apiKey(h)
	rcv := newReceiver(t)
	hook := key.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "events": []string{"feedback.created", "contact.deleted", "conversation.created"},
	})
	secret := hook.str("secret")
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret %q", secret)
	}
	body := map[string]any{
		"inbox_id": tm.inbox, "category": "praise", "body": "Love the new release", "client_id": "form-1", "app_version": "web-5",
		"contact": map[string]any{"external_id": "acct-7", "email": "success+fb@simulator.amazonses.com", "name": "Form User"},
	}
	key.expectProblem(http.StatusBadRequest, "api_channel_required", "POST", "/v1/feedback", body)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{"kind": "api", "name": "Website form"})
	tm.owner.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/feedback", body)
	r := key.expect(http.StatusCreated, "POST", "/v1/feedback", body)
	contact := r.body["contact"].(map[string]any)
	conv := r.body["conversation"].(map[string]any)
	if conv["kind"] != "feedback" || contact["name"] != "Form User" || contact["external_ids"].([]any)[0].(map[string]any)["external_id"] != "acct-7" ||
		r.body["message"].(map[string]any)["direction"] != "in" {
		t.Fatalf("api feedback: %s", r.raw)
	}
	if again := key.expect(http.StatusOK, "POST", "/v1/feedback", body); again.body["conversation"].(map[string]any)["id"] != conv["id"] {
		t.Fatalf("idempotent api feedback: %s", again.raw)
	}
	body["client_id"] = "form-2"
	if second := key.expect(http.StatusCreated, "POST", "/v1/feedback", body); second.body["contact"].(map[string]any)["id"] != contact["id"] {
		t.Fatalf("same external id made a new contact: %s", second.raw)
	}
	h.runWebhooks(tm.ws)
	var feedbackHooks int
	for _, got := range rcv.take() {
		verifySignature(t, secret, got)
		if got.typ == "feedback.created" {
			feedbackHooks++
			c := got.data["contact"].(map[string]any)
			if c["online"] != false || c["external_ids"].([]any)[0].(map[string]any)["external_id"] != "acct-7" || got.data["message"] == nil {
				t.Fatalf("feedback.created: %s", got.body)
			}
		}
	}
	if feedbackHooks != 2 {
		t.Fatalf("%d feedback.created hooks", feedbackHooks)
	}

	other := newTeam(t, h)
	otherKey := other.apiKey(h)
	otherKey.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/contacts/by-external-id?inbox_id="+tm.inbox+"&external_id=acct-7", nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/contacts/by-external-id?inbox_id="+tm.inbox+"&external_id=acct-7", nil)
	key.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/contacts/by-external-id?inbox_id="+tm.inbox+"&external_id=nobody", nil)
	key.expect(http.StatusNoContent, "DELETE", "/v1/contacts/by-external-id?inbox_id="+tm.inbox+"&external_id=acct-7", nil)
	key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+contact["id"].(string), nil)
	key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv["id"].(string), nil)
	h.runWebhooks(tm.ws)
	got := rcv.take()
	if len(got) != 1 || got[0].typ != "contact.deleted" {
		t.Fatalf("after delete: %d hooks", len(got))
	}
	verifySignature(t, secret, got[0])
	dc := got[0].data["contact"].(map[string]any)
	if dc["id"] != contact["id"] || dc["external_ids"].([]any)[0].(map[string]any)["external_id"] != "acct-7" {
		t.Fatalf("contact.deleted: %s", got[0].body)
	}
}

func TestWebhookDeliveryRetriesAndDisable(t *testing.T) {
	h := privateHarness(t)
	at := newAppTeam(t, h, false)
	rcv := newReceiver(t)
	created := at.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "inbox_id": at.appInbox, "events": []string{"message.created"}, "description": "push",
	})
	hookID := created.body["endpoint"].(map[string]any)["id"].(string)
	secret := created.str("secret")
	at.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/webhooks", nil)
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/webhooks/"+hookID, nil)

	cs := at.session(h, "push-user")
	ws, _, err := dialContact(h, cs.token, "", "")
	if err != nil {
		t.Fatal(err)
	}
	fb := cs.expect(http.StatusCreated, "POST", "/client/v1/feedback", map[string]any{"category": "bug", "body": "broken"})
	conv := fb.body["conversation"].(map[string]any)["id"].(string)
	h.runWebhooks(at.ws)
	first := rcv.take()
	if len(first) != 1 || first[0].typ != "message.created" || first[0].data["contact"].(map[string]any)["online"] != true {
		t.Fatalf("contact message while connected: %v", first)
	}
	verifySignature(t, secret, first[0])
	ws.close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		_ = h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM realtime_connections WHERE contact_id = $1", cs.contactID).Scan(&n)
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection still open")
		}
		time.Sleep(20 * time.Millisecond)
	}
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal"})
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "out", "body": "Fixed in 2.3.2"})
	h.runWebhooks(at.ws)
	reply := rcv.take()
	if len(reply) != 1 || reply[0].data["message"].(map[string]any)["body"] != "Fixed in 2.3.2" ||
		reply[0].data["contact"].(map[string]any)["online"] != false {
		t.Fatalf("member reply: %v", reply)
	}
	presence := at.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+cs.contactID+"/presence", nil)
	if presence.body["online"] != false || presence.body["last_seen_at"] == nil {
		t.Fatalf("presence: %s", presence.raw)
	}

	rot := at.owner.expect(http.StatusOK, "POST", "/v1/webhooks/"+hookID+"/secret", nil)
	newSecret := rot.str("secret")
	rcv.setStatus(http.StatusInternalServerError)
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "out", "body": "Still there?"})
	if retries := h.runWebhooks(at.ws); retries != 1 {
		t.Fatalf("%d retries after a 500", retries)
	}
	failed := rcv.take()
	if len(failed) != 1 || len(strings.Fields(failed[0].header.Get("webhook-signature"))) != 2 {
		t.Fatalf("rotated signatures: %v", failed)
	}
	verifySignature(t, newSecret, failed[0])
	verifySignature(t, secret, failed[0])
	deliveries := at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/deliveries?state=pending", nil)
	items := deliveries.body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("pending deliveries: %s", deliveries.raw)
	}
	d := items[0].(map[string]any)
	last := d["last_attempt"].(map[string]any)
	if d["attempts"] != float64(1) || d["next_attempt_at"] == nil || last["status_code"] != float64(500) || last["success"] != false {
		t.Fatalf("pending delivery: %s", deliveries.raw)
	}
	deliveryID := uuid.MustParse(d["id"].(string))
	wsID := uuid.MustParse(at.ws)
	ctx := context.Background()
	for attempt := 2; attempt <= 9; attempt++ {
		retry, err := h.srv.DeliverWebhook(ctx, wsID, deliveryID, attempt, 10, false)
		if err != nil || !retry {
			t.Fatalf("attempt %d: retry %v, %v", attempt, retry, err)
		}
	}
	h.clock.Advance(25 * time.Hour)
	if retry, err := h.srv.DeliverWebhook(ctx, wsID, deliveryID, 10, 10, false); err != nil || retry {
		t.Fatalf("last attempt: retry %v, %v", retry, err)
	}
	if got := rcv.take(); len(got) != 9 {
		t.Fatalf("%d retried attempts reached the receiver", len(got))
	}
	ep := at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID, nil)
	if ep.body["enabled"] != false || !strings.Contains(ep.str("disabled_reason"), "failed") {
		t.Fatalf("endpoint after 24 hours of failures: %s", ep.raw)
	}
	detail := at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/deliveries/"+deliveryID.String(), nil)
	if detail.body["state"] != "failed" || len(detail.body["attempt_log"].([]any)) != 10 || detail.body["payload"].(map[string]any)["type"] != "message.created" {
		t.Fatalf("delivery detail: %s", detail.raw)
	}
	at.owner.expectProblem(http.StatusConflict, "webhook_disabled", "POST", "/v1/webhooks/"+hookID+"/deliveries/"+deliveryID.String()+"/redeliver", nil)
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "out", "body": "while disabled"})
	h.runWebhooks(at.ws)
	if got := rcv.take(); len(got) != 0 {
		t.Fatalf("disabled endpoint got %d hooks", len(got))
	}

	rcv.setStatus(http.StatusOK)
	ep = at.owner.expect(http.StatusOK, "PATCH", "/v1/webhooks/"+hookID, map[string]any{"enabled": true})
	if ep.body["disabled_reason"] != nil || ep.body["failing_since"] != nil {
		t.Fatalf("re-enabled: %s", ep.raw)
	}
	at.owner.expect(http.StatusAccepted, "POST", "/v1/webhooks/"+hookID+"/deliveries/"+deliveryID.String()+"/redeliver", nil)
	h.runWebhooks(at.ws)
	redelivered := rcv.take()
	if len(redelivered) != 1 || redelivered[0].header.Get("webhook-id") != failed[0].header.Get("webhook-id") {
		t.Fatalf("redeliver: %v", redelivered)
	}
	detail = at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/deliveries/"+deliveryID.String(), nil)
	if detail.body["state"] != "succeeded" || detail.body["attempt_log"].([]any)[0].(map[string]any)["manual"] != true {
		t.Fatalf("after redeliver: %s", detail.raw)
	}
	log := at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/attempts?limit=100", nil)
	if n := len(log.body["items"].([]any)); n < 12 {
		t.Fatalf("delivery log has %d attempts", n)
	}

	rcv.setStatus(http.StatusGone)
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "out", "body": "gone?"})
	if retries := h.runWebhooks(at.ws); retries != 0 {
		t.Fatalf("410 was retried")
	}
	ep = at.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID, nil)
	if ep.body["enabled"] != false || !strings.Contains(ep.str("disabled_reason"), "410") {
		t.Fatalf("after 410: %s", ep.raw)
	}
	at.owner.expect(http.StatusNoContent, "DELETE", "/v1/webhooks/"+hookID, nil)
	at.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/webhooks/"+hookID, nil)
}

func TestWebhookNotesAndInboxScope(t *testing.T) {
	h := privateHarness(t)
	tm := newTeam(t, h)
	other := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Other", "slug": "other"})
	otherInbox := other.body["inbox"].(map[string]any)["id"].(string)
	all, scoped, notes := newReceiver(t), newReceiver(t), newReceiver(t)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": all.srv.URL, "events": []string{"message.created", "conversation.created"}})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": scoped.srv.URL, "inbox_id": otherInbox, "events": []string{"message.created"}})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": notes.srv.URL, "events": []string{"message.created"}, "include_notes": true})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/webhooks", map[string]any{"url": all.srv.URL, "events": []string{"nope"}})
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "secret note"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	h.runWebhooks(tm.ws)
	if got := all.take(); len(got) != 1 || got[0].typ != "conversation.created" {
		t.Fatalf("workspace endpoint: %d hooks", len(got))
	}
	if got := scoped.take(); len(got) != 0 {
		t.Fatalf("other inbox's endpoint: %d hooks", len(got))
	}
	if got := notes.take(); len(got) != 1 || got[0].data["message"].(map[string]any)["kind"] != "note" {
		t.Fatalf("notes endpoint: %v", got)
	}
}

type staticResolver map[string]string

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if ip, ok := r[host]; ok {
		return []netip.Addr{netip.MustParseAddr(ip)}, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func TestWebhookLinkLocalRefusedWhenPrivateAllowed(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		d.Webhooks.Resolver = staticResolver{"hooks.example.com": "127.0.0.1", "metadata.example.com": "169.254.169.254"}
	})
	tm := newTeam(t, h)
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data/", "http://169.254.1.1:8080/hook", "http://[fe80::1]/hook",
		"http://[::ffff:169.254.169.254]/hook", "http://[fd00:ec2::254]/",
	} {
		tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/webhooks", map[string]any{"url": u, "events": []string{"message.created"}})
	}
	rcv := newReceiver(t)
	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": strings.Replace(rcv.srv.URL, "127.0.0.1", "hooks.example.com", 1), "events": []string{"conversation.created"},
	})
	hookID := created.body["endpoint"].(map[string]any)["id"].(string)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/webhooks/"+hookID, map[string]any{"url": "http://[fe80::1]/"})
	tm.conversation(tm.owner)
	h.runWebhooks(tm.ws)
	if got := rcv.take(); len(got) != 1 {
		t.Fatalf("loopback target with private allowed got %d deliveries", len(got))
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/webhooks/"+hookID, map[string]any{"url": "http://metadata.example.com/latest/meta-data/"})
	tm.conversation(tm.owner)
	h.runWebhooks(tm.ws)
	log := tm.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/attempts", nil)
	items := log.body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("attempt log: %s", log.raw)
	}
	if e, _ := items[0].(map[string]any)["error"].(string); !strings.Contains(e, "refused address 169.254.169.254") {
		t.Fatalf("metadata attempt not refused: %s", log.raw)
	}
}

func TestWebhookSSRFRefused(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.Resolver = staticResolver{"hooks.example.com": "127.0.0.1", "intranet.example.com": "10.1.2.3"}
	})
	tm := newTeam(t, h)
	for _, u := range []string{
		"http://127.0.0.1:8080/hook", "http://10.0.0.8/hook", "http://[::1]/hook", "http://169.254.169.254/latest/meta-data/",
		"http://localhost/hook", "ftp://example.com/",
	} {
		tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/webhooks", map[string]any{"url": u, "events": []string{"message.created"}})
	}
	rcv := newReceiver(t)
	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": strings.Replace(rcv.srv.URL, "127.0.0.1", "hooks.example.com", 1), "events": []string{"conversation.created"},
	})
	hookID := created.body["endpoint"].(map[string]any)["id"].(string)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/webhooks/"+hookID, map[string]any{"url": "http://192.168.1.1/"})
	tm.conversation(tm.owner)
	h.runWebhooks(tm.ws)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/webhooks/"+hookID, map[string]any{"url": "http://intranet.example.com/hook"})
	tm.conversation(tm.owner)
	h.runWebhooks(tm.ws)
	if got := rcv.take(); len(got) != 0 {
		t.Fatal("a private target was reached")
	}
	log := tm.owner.expect(http.StatusOK, "GET", "/v1/webhooks/"+hookID+"/attempts", nil)
	items := log.body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("attempt log: %s", log.raw)
	}
	for _, it := range items {
		if e, _ := it.(map[string]any)["error"].(string); !strings.Contains(e, "refused address") {
			t.Fatalf("attempt not refused: %s", log.raw)
		}
	}
}

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/api"
)

// leaks counts the rows that still hold text, by table.
func (h *harness) leaks(ws, text string) map[string]int {
	h.t.Helper()
	like := "%" + text + "%"
	out := map[string]int{
		"messages":           h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND (body LIKE $2 OR html LIKE $2 OR help_cards::text LIKE $2)", ws, like),
		"message_emails":     h.count("SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND (full_text LIKE $2 OR full_html LIKE $2 OR subject LIKE $2 OR headers::text LIKE $2)", ws, like),
		"attachments":        h.count("SELECT count(*) FROM attachments WHERE workspace_id = $1 AND filename LIKE $2", ws, like),
		"events":             h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND payload::text LIKE $2", ws, like),
		"webhook_deliveries": h.count("SELECT count(*) FROM webhook_deliveries WHERE workspace_id = $1 AND payload LIKE $2", ws, like),
		"idempotency_keys":   h.count("SELECT count(*) FROM idempotency_keys WHERE workspace_id = $1 AND position(convert_to($2, 'UTF8') IN coalesce(body, '')) > 0", ws, text),
		"audit_log":          h.count("SELECT count(*) FROM audit_log WHERE workspace_id = $1 AND details::text LIKE $2", ws, like),
		"river_job":          h.count("SELECT count(*) FROM river_job WHERE args::text LIKE $1", like),
	}
	for k, v := range out {
		if v == 0 {
			delete(out, k)
		}
	}
	return out
}

func (h *harness) runStorageDeletes(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("storage_delete", ws) {
		var a api.StorageDeleteArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.DeleteStoredObjects(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (h *harness) runNotifyOnly(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("notify", ws) {
		var a api.NotifyArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.Notify(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (c *client) postIdempotent(path, key string, body any) response {
	c.h.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", c.h.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("X-Forwarded-For", c.ip)
	req.Header.Set("Authorization", "Bearer "+c.bearer)
	res, err := c.http.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := response{status: res.StatusCode, raw: raw, header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func messageByID(c *client, conv, id string) map[string]any {
	c.h.t.Helper()
	for _, m := range messages(c, conv) {
		if m["id"] == id {
			return m
		}
	}
	c.h.t.Fatalf("message %s not in %s", id, conv)
	return nil
}

func expectRedacted(t *testing.T, m map[string]any) {
	t.Helper()
	if m["redacted_at"] == nil || m["redacted_by"] == nil || m["body"] != "" || m["html"] != nil || m["mentions"] != nil || m["cards"] != nil ||
		len(m["attachments"].([]any)) != 0 {
		t.Fatalf("not redacted: %v", m)
	}
}

func TestRedactWhoMayAndWhatIsLeft(t *testing.T) {
	t.Parallel()
	h, fake := pushHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	ownerID := tm.ownerMemberID(t)
	fake.subscribe(tm.owner)
	rcv := newReceiver(t)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "events": []string{"message.created", "message.redacted"}, "include_notes": true,
	})
	secret := "zq" + unique("secret")[7:]
	conv := tm.conversation(tm.owner)
	keep := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "kept note"}).str("id")
	sent := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "card " + secret + " for you"},
		[]file{{name: secret + ".txt", contentType: "text/plain", data: []byte("file " + secret)}})
	if sent.status != http.StatusCreated {
		t.Fatalf("upload: %s", sent.raw)
	}
	msg := sent.str("id")
	keys := h.storedKeys(tm.ws)
	if len(keys) != 1 {
		t.Fatalf("stored: %v", keys)
	}
	note := tm.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "note", "body": "note " + secret, "mentions": []string{ownerID},
	}).str("id")
	backend := tm.scopedKey(h, "conversations:read", "messages:write", "messages:redact")
	incoming := backend.postIdempotent("/v1/conversations/"+conv+"/messages", "idem-1", map[string]any{"kind": "message", "direction": "in", "body": "from contact " + secret})
	if incoming.status != http.StatusCreated {
		t.Fatalf("incoming: %s", incoming.raw)
	}
	in := incoming.str("id")
	draft := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "draft", "draft": true}).str("id")
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	event := lastOf(messages(tm.owner, conv), "event")["id"].(string)
	h.runWebhooks(tm.ws)
	rcv.take()
	h.runNotifyOnly(tm.ws)
	if len(h.jobs("push_send", tm.ws)) == 0 {
		t.Fatal("no push queued for the contact's message")
	}
	if l := h.leaks(tm.ws, secret); l["events"] == 0 || l["webhook_deliveries"] == 0 || l["idempotency_keys"] == 0 || l["river_job"] == 0 {
		t.Fatalf("the test does not reach every copy: %v", l)
	}

	redact := func(id string) string { return "/v1/messages/" + id + "/redact" }
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", redact(msg), nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", redact(in), nil)
	tm.apiKey(h).expectProblem(http.StatusForbidden, "insufficient_scope", "POST", redact(msg), nil)
	tm.scopedKey(h, "conversations:read", "messages:write").expectProblem(http.StatusForbidden, "insufficient_scope", "POST", redact(msg), nil)
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", redact(msg), nil)
	other.scopedKey(h, "conversations:read", "messages:redact").expectProblem(http.StatusNotFound, "not_found", "POST", redact(msg), nil)
	tm.owner.expectProblem(http.StatusConflict, "not_redactable", "POST", redact(draft), nil)
	tm.owner.expectProblem(http.StatusConflict, "not_redactable", "POST", redact(event), nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", redact("0195f0f0-0000-7000-8000-000000000000"), nil)

	byAgent := tm.agent.expect(http.StatusOK, "POST", redact(note), nil)
	if by := byAgent.body["redacted_by"].(map[string]any); by["member_id"] != tm.agentID || by["api_key_id"] != nil || byAgent.str("kind") != "note" {
		t.Fatalf("agent's own note: %s", byAgent.raw)
	}
	byKey := backend.expect(http.StatusOK, "POST", redact(msg), nil)
	if by := byKey.body["redacted_by"].(map[string]any); by["api_key_id"] == nil || by["member_id"] != nil {
		t.Fatalf("by key: %s", byKey.raw)
	}
	byOwner := tm.owner.expect(http.StatusOK, "POST", redact(in), nil)
	again := tm.owner.expect(http.StatusOK, "POST", redact(in), nil)
	if again.str("redacted_at") != byOwner.str("redacted_at") || again.body["redacted_by"].(map[string]any)["member_id"] != ownerID {
		t.Fatalf("second redaction changed it: %s then %s", byOwner.raw, again.raw)
	}
	for _, id := range []string{note, msg, in} {
		expectRedacted(t, messageByID(tm.owner, conv, id))
	}
	if m := messageByID(tm.owner, conv, keep); m["body"] != "kept note" || m["redacted_at"] != nil {
		t.Fatalf("other message changed: %v", m)
	}
	count := len(messages(tm.owner, conv))
	replay := backend.postIdempotent("/v1/conversations/"+conv+"/messages", "idem-1", map[string]any{"kind": "message", "direction": "in", "body": "from contact " + secret})
	if replay.status != http.StatusCreated || replay.str("id") != in || replay.str("redacted_at") == "" || strings.Contains(string(replay.raw), secret) || len(messages(tm.owner, conv)) != count {
		t.Fatalf("idempotent replay: %s", replay.raw)
	}

	if len(h.jobs("push_send", tm.ws)) != 0 {
		t.Fatal("pushes of the redacted message are still queued")
	}
	h.runStorageDeletes(tm.ws)
	if h.stored(keys[0]) {
		t.Fatal("attachment file still stored")
	}
	if l := h.leaks(tm.ws, secret); len(l) != 0 {
		t.Fatalf("text left in %v", l)
	}
	if ids := listedIDs(tm.owner, "&q="+secret); len(ids) != 0 {
		t.Fatalf("search still finds it: %v", ids)
	}
	for _, it := range tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?limit=100", nil).body["items"].([]any) {
		if c := it.(map[string]any); c["id"] == conv {
			if lm := c["last_message"].(map[string]any); lm["redacted"] != true || lm["text"] != "" {
				t.Fatalf("preview: %v", lm)
			}
		}
	}

	h.runWebhooks(tm.ws)
	hooks := rcv.take()
	redacted := map[string]bool{}
	for _, hk := range hooks {
		if strings.Contains(string(hk.body), secret) {
			t.Fatalf("webhook carries the text: %s", hk.body)
		}
		if hk.typ == "message.redacted" {
			m := hk.data["message"].(map[string]any)
			expectRedacted(t, m)
			redacted[m["id"].(string)] = true
		}
	}
	if len(redacted) != 3 || !redacted[note] {
		t.Fatalf("message.redacted hooks: %v", hooks)
	}
	var seen int
	for _, e := range tm.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=200", nil).body["events"].([]any) {
		if ev := e.(map[string]any); ev["type"] == "message.redacted" {
			seen++
			expectRedacted(t, ev["data"].(map[string]any))
		}
	}
	if seen != 3 {
		t.Fatalf("%d message.redacted events", seen)
	}

	log := auditLog(t, tm.owner, "&action=message.redacted")
	if len(log) != 3 {
		t.Fatalf("audit: %v", log)
	}
	if e := log[1]; actorOf(e)["kind"] != "api_key" || targetOf(e)["type"] != "message" || targetOf(e)["id"] != msg ||
		detailsOf(e)["conversation_id"] != conv || detailsOf(e)["inbox_id"] != tm.inbox || detailsOf(e)["kind"] != "message" || detailsOf(e)["files"] != float64(1) {
		t.Fatalf("audit entry %v", e)
	}
	if e := log[2]; actorOf(e)["kind"] != "member" || actorOf(e)["id"] != tm.agentID || detailsOf(e)["kind"] != "note" {
		t.Fatalf("agent's audit entry %v", e)
	}
	if len(auditLog(t, other.owner, "&action=message.redacted")) != 0 {
		t.Fatal("other workspace sees the entries")
	}

	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", redact(keep), nil)
}

func TestRedactSkipsNotificationsNotYetSent(t *testing.T) {
	t.Parallel()
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	fake.subscribe(nt.owner)
	secret := "zq" + unique("late")[5:]
	conv := nt.open(h, nt.live, "hello "+secret)
	id := lastOf(messages(nt.owner, conv), "message")["id"].(string)
	nt.owner.expect(http.StatusOK, "POST", "/v1/messages/"+id+"/redact", nil)
	h.runNotifications(nt.ws)
	for _, d := range fake.take() {
		if strings.Contains(fmt.Sprint(d.payload), secret) {
			t.Fatalf("push sent the text: %v", d.payload)
		}
	}
	if l := h.leaks(nt.ws, secret); len(l) != 0 {
		t.Fatalf("text left in %v", l)
	}
}

func TestRedactInWidget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	cs := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "user-1"})})
	secret := "zq" + unique("widget")[7:]
	started := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations", map[string]any{"body": "my code is " + secret, "client_id": "c-1"})
	conv := started.body["conversation"].(map[string]any)["id"].(string)
	first := started.body["message"].(map[string]any)["id"].(string)
	note := ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal " + secret}).str("id")
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.nextOf("ready")
	ct.owner.expect(http.StatusOK, "POST", "/v1/messages/"+note+"/redact", nil)
	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/messages/"+first+"/redact", nil)
	ct.owner.expect(http.StatusOK, "POST", "/v1/messages/"+first+"/redact", nil)
	m := ws.nextOf("message.redacted")
	var data map[string]any
	_ = json.Unmarshal(m.Data, &data)
	if m.ConversationID != conv || data["id"] != first || data["redacted"] != true || data["body"] != "" || len(data["attachments"].([]any)) != 0 || data["redacted_by"] != nil {
		t.Fatalf("frame: %s", m.Data)
	}
	ws.close()

	items := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages", nil).body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["redacted"] != true || items[0].(map[string]any)["body"] != "" {
		t.Fatalf("client messages: %v", items)
	}
	list := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil)
	if lm := list.body["items"].([]any)[0].(map[string]any)["last_message"].(map[string]any); lm["redacted"] != true || lm["text"] != "" {
		t.Fatalf("client preview: %v", lm)
	}
	if retry := cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "my code is " + secret, "client_id": "c-1"}); retry.str("id") != first || retry.body["redacted"] != true {
		t.Fatalf("retry of a redacted message: %s", retry.raw)
	}

	replay, _, err := dialContact(h, cs.token, ct.origin, "?last_event_id=0")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range replay.until(func(m wsMessage) bool { return m.Type == "ready" }) {
		if strings.Contains(string(f.Data), secret) {
			t.Fatalf("replay carries the text: %s %s", f.Type, f.Data)
		}
	}
	replay.close()
	if l := h.leaks(ct.ws, secret); len(l) != 0 {
		t.Fatalf("text left in %v", l)
	}
}

func TestRedactEmail(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	in := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	conv, inbound := in.str("conversation_id"), in.str("message_id")
	before := h.storedKeys(et.ws)
	if len(before) < 2 {
		t.Fatalf("stored: %v", before)
	}
	secret := "zq" + unique("mail")[5:]
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Your new password is " + secret + " and it works today"})
	h.sendQueued(t, et.ws, reply.str("id"))
	header := reply.body["email"].(map[string]any)["message_id"].(string)
	quoting := h.ingest(et.address, buildMail(mailOpts{
		from: "newcust@example.net", to: et.address, subject: "Re: Broken export", messageID: newMessageID(),
		body:    "Thanks, it works.\n\nOn Monday Acme Support wrote:\n> Your new password is " + secret + "\n> and it works today\n",
		headers: map[string]string{"In-Reply-To": "<" + header + ">", "References": "<" + header + ">"},
	}), nil)
	unrelated := h.ingest(et.address, buildMail(mailOpts{
		from: "newcust@example.net", to: et.address, subject: "Re: Broken export", messageID: newMessageID(),
		body:    "One more thing.\n\nOn Monday someone else wrote:\n> Something else entirely.\n",
		headers: map[string]string{"In-Reply-To": "<" + header + ">", "References": "<" + header + ">"},
	}), nil)
	if quoting.str("conversation_id") != conv || unrelated.str("conversation_id") != conv {
		t.Fatalf("replies threaded elsewhere: %v %v", quoting.body, unrelated.body)
	}
	if full := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+quoting.str("message_id")+"/email", nil); !strings.Contains(full.str("full_text"), secret) {
		t.Fatalf("the quoting reply does not quote: %s", full.raw)
	}

	queued := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "not yet sent " + secret}).str("id")
	r := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+queued+"/redact", nil)
	if d := r.body["delivery"].(map[string]any); d["state"] != "failed" {
		t.Fatalf("queued e-mail: %s", r.raw)
	}
	sentBefore := h.smtp.count()
	h.sendQueued(t, et.ws, queued)
	if h.smtp.count() != sentBefore {
		t.Fatal("a redacted queued e-mail was sent")
	}

	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+reply.str("id")+"/redact", nil)
	red := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+inbound+"/redact", nil)
	if em := red.body["email"].(map[string]any); em["subject"] != nil || em["raw"] != false || em["quoted"] != false || em["message_id"] == "" {
		t.Fatalf("inbound e-mail summary: %s", red.raw)
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+inbound+"/raw", nil)
	if full := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+inbound+"/email", nil); full.str("full_text") != "" || full.body["full_html"] != nil || len(full.body["headers"].(map[string]any)) != 0 {
		t.Fatalf("inbound e-mail detail: %s", full.raw)
	}
	q := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+quoting.str("message_id")+"/email", nil)
	if strings.Contains(q.str("full_text"), secret) || !strings.Contains(q.str("full_text"), "Thanks, it works.") {
		t.Fatalf("quoting reply: %s", q.raw)
	}
	if m := messageByID(et.owner, conv, quoting.str("message_id")); m["body"] != "Thanks, it works." || m["redacted_at"] != nil {
		t.Fatalf("quoting reply's message: %v", m)
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+quoting.str("message_id")+"/raw", nil)
	et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+unrelated.str("message_id")+"/raw", nil)
	if u := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+unrelated.str("message_id")+"/email", nil); !strings.Contains(u.str("full_text"), "Something else entirely.") {
		t.Fatalf("unrelated reply lost its quote: %s", u.raw)
	}

	h.runStorageDeletes(et.ws)
	left := map[string]bool{}
	for _, k := range h.storedKeys(et.ws) {
		left[k] = true
	}
	for _, k := range before {
		if left[k] || h.stored(k) {
			t.Fatalf("file %s of the redacted e-mail is still kept", k)
		}
	}
	if l := h.leaks(et.ws, secret); len(l) != 0 {
		t.Fatalf("text left in %v", l)
	}
	if l := h.leaks(et.ws, "The export is"); len(l) != 0 {
		t.Fatalf("inbound text left in %v", l)
	}
}

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jhillyerd/enmime/v2"

	"github.com/productdevbook/yuva/api/internal/email"
)

type formField struct{ name, value string }

func (c *client) uploadFields(path string, fields []formField, files []file) response {
	c.h.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range fields {
		_ = w.WriteField(f.name, f.value)
	}
	for _, f := range files {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename=%q`, f.name))
		hdr.Set("Content-Type", f.contentType)
		pw, _ := w.CreatePart(hdr)
		_, _ = pw.Write(f.data)
	}
	_ = w.Close()
	req, _ := http.NewRequest("POST", c.h.url+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
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

func TestComposeMail(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	ctx := context.Background()
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"purpose": "mail", "signature": "Best regards from Acme"})
	ownerID := et.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	d := unique("compose") + ".example.org"
	ann, ben, cem, hidden := "ann@"+d, "ben@"+d, "cem@"+d, "hidden@"+d
	known := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Ann Known", "emails": []string{ann}}).str("id")

	r := et.owner.uploadFields("/v1/conversations/mail", []formField{
		{"inbox_id", et.inbox}, {"to", strings.ToUpper(ann)}, {"to", ben}, {"cc", cem}, {"cc", ben}, {"bcc", hidden},
		{"subject", "Quarterly numbers"}, {"body", "Please find the numbers attached."},
	}, []file{{name: "numbers.txt", contentType: "text/plain", data: []byte("NUMBERS-CONTENT")}})
	if r.status != http.StatusCreated {
		t.Fatalf("compose: %d %s", r.status, r.raw)
	}
	conv := r.body["conversation"].(map[string]any)
	msg := r.body["message"].(map[string]any)
	convID, msgID := conv["id"].(string), msg["id"].(string)
	if conv["contact_id"] != known || conv["subject"] != "Quarterly numbers" || conv["channel_kind"] != "email" || conv["status"] != "open" || conv["assignee_id"] != ownerID {
		t.Fatalf("conversation: %v", conv)
	}
	em := msg["email"].(map[string]any)
	if !slices.Equal(strs(em["to"]), []string{ann, ben}) || !slices.Equal(strs(em["cc"]), []string{cem}) || !slices.Equal(strs(em["bcc"]), []string{hidden}) || em["subject"] != "Quarterly numbers" {
		t.Fatalf("message e-mail: %v", em)
	}
	if !strings.Contains(fmt.Sprint(msg["signature_html"]), "Best regards from Acme") || len(msg["attachments"].([]any)) != 1 {
		t.Fatalf("message: %s", r.raw)
	}
	if got := participantEmails(t, et.owner, convID); len(got) != 2 || !slices.Contains(got, ben) || !slices.Contains(got, cem) {
		t.Fatalf("participants %v", got)
	}
	for _, addr := range []string{ben, cem} {
		var n int
		if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = $2", et.ws, addr).Scan(&n); err != nil || n != 1 {
			t.Fatalf("contact for %s: %d %v", addr, n, err)
		}
	}
	var hiddenContacts int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = $2", et.ws, hidden).Scan(&hiddenContacts); err != nil || hiddenContacts != 0 {
		t.Fatalf("a bcc address became a contact: %d %v", hiddenContacts, err)
	}

	h.sendQueued(t, et.ws, msgID)
	sent, parsed := h.smtp.last(t)
	rcpts := slices.Clone(sent.to)
	slices.Sort(rcpts)
	if want := []string{ann, ben, cem, hidden}; !slices.Equal(rcpts, want) || sent.from != et.address {
		t.Fatalf("envelope %s -> %v", sent.from, sent.to)
	}
	if to := parsed.Header.Get("To"); !strings.Contains(to, ann) || !strings.Contains(to, ben) || strings.Contains(to, cem) {
		t.Fatalf("To %q", to)
	}
	if cc := parsed.Header.Get("Cc"); !strings.Contains(cc, cem) || strings.Contains(cc, ben) {
		t.Fatalf("Cc %q", cc)
	}
	if parsed.Header.Get("Subject") != "Quarterly numbers" || parsed.Header.Get("In-Reply-To") != "" || parsed.Header.Get("References") != "" {
		t.Fatalf("headers: %v", parsed.Header)
	}
	if bytes.Contains(sent.raw, []byte("hidden@")) {
		t.Fatal("the Bcc address is in the sent message")
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(sent.raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Attachments) != 1 || !bytes.Contains(env.Attachments[0].Content, []byte("NUMBERS-CONTENT")) || !strings.Contains(env.Text, "Best regards from Acme") {
		t.Fatalf("sent body %q, attachments %d", env.Text, len(env.Attachments))
	}
	var leaks int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE workspace_id = $1 AND payload::text LIKE '%hidden@%'", et.ws).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("%d events carry the Bcc (%v)", leaks, err)
	}
	var created, messaged int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE type = 'conversation.created'), count(*) FILTER (WHERE type = 'message.created') FROM events WHERE workspace_id = $1 AND conversation_id = $2", et.ws, convID).Scan(&created, &messaged); err != nil || created != 1 || messaged < 1 {
		t.Fatalf("events: %d created, %d messages (%v)", created, messaged, err)
	}

	answer := h.ingest(et.address, buildMail(mailOpts{
		from: cem, to: et.address, subject: "Re: Quarterly numbers", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + email.NormalizeID(parsed.Header.Get("Message-ID")) + ">", "Authentication-Results": dmarcPass(d)}, body: "Thanks, got them.",
	}), nil)
	if answer.status != http.StatusAccepted || answer.str("conversation_id") != convID {
		t.Fatalf("the Cc participant's answer did not join: %d %v", answer.status, answer.body)
	}

	dora, eli, eda, secret := "dora@"+d, "eli@"+d, "eda@"+d, "secretdraft@"+d
	draft := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora, eli}, "cc": []string{eda}, "bcc": []string{secret}, "subject": "Draft plan", "body": "Not yet.", "draft": true, "signature": false,
	})
	dm := draft.body["message"].(map[string]any)
	if dm["draft"] != true || dm["delivery"] != nil || dm["signature_html"] != nil || dm["email"] != nil {
		t.Fatalf("draft: %s", draft.raw)
	}
	draftConv := draft.body["conversation"].(map[string]any)["id"].(string)
	contactsOf := func(addrs ...string) int {
		var n int
		if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = ANY($2)", et.ws, addrs).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := participantEmails(t, et.owner, draftConv); len(got) != 0 || contactsOf(dora) != 1 || contactsOf(eli, eda, secret) != 0 {
		t.Fatalf("a draft made participants %v or contacts", got)
	}
	sentDraft := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+dm["id"].(string)+"/send", nil)
	if got := participantEmails(t, et.owner, draftConv); len(got) != 2 || !slices.Contains(got, eli) || !slices.Contains(got, eda) || contactsOf(eli, eda) != 2 || contactsOf(secret) != 0 {
		t.Fatalf("sent draft participants %v", got)
	}
	sem := sentDraft.body["email"].(map[string]any)
	if !slices.Equal(strs(sem["to"]), []string{dora, eli}) || !slices.Equal(strs(sem["cc"]), []string{eda}) || !slices.Equal(strs(sem["bcc"]), []string{secret}) || sem["subject"] != "Draft plan" {
		t.Fatalf("sent draft e-mail: %v", sem)
	}
	h.sendQueued(t, et.ws, dm["id"].(string))
	sent, parsed = h.smtp.last(t)
	rcpts = slices.Clone(sent.to)
	slices.Sort(rcpts)
	if want := []string{dora, eda, eli, secret}; !slices.Equal(rcpts, want) {
		t.Fatalf("draft envelope %v", sent.to)
	}
	if to := parsed.Header.Get("To"); !strings.Contains(to, dora) || !strings.Contains(to, eli) || strings.Contains(to, eda) {
		t.Fatalf("draft To %q", to)
	}
	if cc := parsed.Header.Get("Cc"); !strings.Contains(cc, eda) || strings.Contains(cc, eli) {
		t.Fatalf("draft Cc %q", cc)
	}
	if parsed.Header.Get("Subject") != "Draft plan" || parsed.Header.Get("In-Reply-To") != "" || parsed.Header.Get("References") != "" || bytes.Contains(sent.raw, []byte("secretdraft@")) {
		t.Fatalf("draft headers: %v", parsed.Header)
	}
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE workspace_id = $1 AND payload::text LIKE '%secretdraft@%'", et.ws).Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("%d events carry the draft's Bcc (%v)", leaks, err)
	}

	gus := "gus@" + d
	second := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora}, "cc": []string{eda}, "bcc": []string{secret}, "subject": "Second plan", "body": "x", "draft": true,
	})
	secondID := second.body["message"].(map[string]any)["id"].(string)
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/messages/"+secondID+"/send", map[string]any{"cc": []string{et.address}})
	replaced := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+secondID+"/send", map[string]any{"cc": []string{gus}, "bcc": []string{}})
	rem := replaced.body["email"].(map[string]any)
	if !slices.Equal(strs(rem["to"]), []string{dora}) || !slices.Equal(strs(rem["cc"]), []string{gus}) || rem["bcc"] != nil || rem["subject"] != "Second plan" {
		t.Fatalf("replaced lists: %v", rem)
	}
	h.sendQueued(t, et.ws, secondID)
	sent, _ = h.smtp.last(t)
	rcpts = slices.Clone(sent.to)
	slices.Sort(rcpts)
	if want := []string{dora, gus}; !slices.Equal(rcpts, want) {
		t.Fatalf("replaced envelope %v", sent.to)
	}
	if got := participantEmails(t, et.owner, second.body["conversation"].(map[string]any)["id"].(string)); !slices.Contains(got, gus) {
		t.Fatalf("the new Cc is no participant: %v", got)
	}

	later := "later@" + d
	third := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora}, "bcc": []string{later}, "subject": "Third", "body": "x", "draft": true,
	})
	if _, err := h.st.Pool.Exec(ctx, "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, 'bounce', 'all', now())", et.ws, later); err != nil {
		t.Fatal(err)
	}
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", "/v1/messages/"+third.body["message"].(map[string]any)["id"].(string)+"/send", nil)

	zed := "zed@" + d
	dropped := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora, zed}, "cc": []string{zed + ".cc"}, "bcc": []string{secret}, "subject": "Dropped", "body": "x", "draft": true,
	})
	droppedID := dropped.body["message"].(map[string]any)["id"].(string)
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+droppedID, nil)
	var leftover int
	if err := h.st.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM composed_mails WHERE workspace_id = $1 AND message_id = $2) + (SELECT count(*) FROM conversation_participants WHERE workspace_id = $1 AND conversation_id = $3)", et.ws, droppedID, dropped.body["conversation"].(map[string]any)["id"]).Scan(&leftover); err != nil || leftover != 0 || contactsOf(zed, zed+".cc") != 0 {
		t.Fatalf("a deleted draft left %d rows or contacts behind (%v)", leftover, err)
	}

	erased := "erased@" + d
	erasedContact := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Erased", "emails": []string{erased}}).str("id")
	gdpr := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora, erased}, "cc": []string{"kept@" + d}, "subject": "Erasure", "body": "x", "draft": true,
	})
	gdprID := gdpr.body["message"].(map[string]any)["id"].(string)
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+erasedContact, nil)
	gem := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+gdprID+"/send", nil).body["email"].(map[string]any)
	if !slices.Equal(strs(gem["to"]), []string{dora}) {
		t.Fatalf("an erased address is still a recipient: %v", gem)
	}
	h.sendQueued(t, et.ws, gdprID)
	sent, _ = h.smtp.last(t)
	if slices.Contains(sent.to, erased) || bytes.Contains(sent.raw, []byte(erased)) || !slices.Contains(sent.to, "kept@"+d) {
		t.Fatalf("an erased address was mailed: %v", sent.to)
	}
	orphan := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{dora}, "subject": "Orphan", "body": "x", "draft": true,
	})
	orphanID := orphan.body["message"].(map[string]any)["id"].(string)
	if _, err := h.st.Pool.Exec(ctx, "UPDATE composed_mails SET to_addresses = '{}' WHERE workspace_id = $1 AND message_id = $2", et.ws, orphanID); err != nil {
		t.Fatal(err)
	}
	et.owner.expectProblem(http.StatusConflict, "no_recipient", "POST", "/v1/messages/"+orphanID+"/send", nil)

	body := map[string]any{"inbox_id": et.inbox, "to": []string{"fay@" + d}, "subject": "Once", "body": "Only once.", "client_id": "compose-1"}
	first := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", body)
	again := et.owner.expect(http.StatusOK, "POST", "/v1/conversations/mail", body)
	if first.body["message"].(map[string]any)["id"] != again.body["message"].(map[string]any)["id"] {
		t.Fatalf("client_id repeat wrote again: %s / %s", first.raw, again.raw)
	}

	bounced := "bounced@" + d
	if _, err := h.st.Pool.Exec(ctx, "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, 'bounce', 'all', now())", et.ws, bounced); err != nil {
		t.Fatal(err)
	}
	post := func(b map[string]any) map[string]any {
		out := map[string]any{"inbox_id": et.inbox, "to": []string{"x@" + d}, "subject": "S", "body": "B"}
		for k, v := range b {
			out[k] = v
		}
		return out
	}
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", "/v1/conversations/mail", post(map[string]any{"cc": []string{bounced}}))
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"to": []string{et.address}}))
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"to": []string{strings.Replace(et.address, "@", "+tag@", 1)}}))
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"to": []string{"not an address"}}))
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"to": []string{}}))
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"subject": " "}))
	many := make([]string, 51)
	for i := range many {
		many[i] = fmt.Sprintf("m%d@%s", i, d)
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"to": many}))
	fifty := many[:50]
	full := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", post(map[string]any{"to": fifty[:25], "cc": fifty[25:]}))
	if got := participantEmails(t, et.owner, full.body["conversation"].(map[string]any)["id"].(string)); len(got) != 49 {
		t.Fatalf("%d participants, want 49", len(got))
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", post(map[string]any{"channel_id": other.channel}))

	chatInbox := et.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Chat", "slug": "chat"}).body["inbox"].(map[string]any)["id"].(string)
	et.owner.expectProblem(http.StatusConflict, "email_not_configured", "POST", "/v1/conversations/mail", post(map[string]any{"inbox_id": chatInbox}))

	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/mail", post(nil))
	et.agent.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/mail", post(nil))

	key := h.client()
	key.bearer = apiKey(et.owner, "Mailer", "conversations:write", "messages:write", "contacts:write")
	key.expectProblem(http.StatusForbidden, "bot_sending_disabled", "POST", "/v1/conversations/mail", post(nil))
	byKey := key.expect(http.StatusCreated, "POST", "/v1/conversations/mail", post(map[string]any{"draft": true}))
	if byKey.body["conversation"].(map[string]any)["assignee_id"] != nil {
		t.Fatalf("a key's mail was assigned: %s", byKey.raw)
	}
	idem := map[string]any{"inbox_id": et.inbox, "to": []string{"gul@" + d}, "subject": "Idem", "body": "Retry.", "draft": true}
	k1 := key.postIdempotent("/v1/conversations/mail", "compose-key-1", idem)
	k2 := key.postIdempotent("/v1/conversations/mail", "compose-key-1", idem)
	if k1.status != http.StatusCreated || k2.status != http.StatusCreated || !bytes.Equal(k1.raw, k2.raw) {
		t.Fatalf("Idempotency-Key: %d %d %s", k1.status, k2.status, k1.raw)
	}
	noScope := h.client()
	noScope.bearer = apiKey(et.owner, "Reader", "conversations:read")
	noScope.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/mail", post(nil))
	for _, scopes := range [][]string{{"messages:write", "contacts:write"}, {"conversations:write", "contacts:write"}, {"conversations:write", "messages:write"}} {
		partial := h.client()
		partial.bearer = apiKey(et.owner, "Partial", scopes...)
		partial.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/mail", post(map[string]any{"draft": true}))
	}

	narrow := h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read", "messages:write", "conversations:write"})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	if tools := toolNames(t, h.mcpSession(narrow)); slices.Contains(tools, "compose_mail") {
		t.Fatalf("compose_mail offered without contacts:write: %v", tools)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": false})

	token := h.oauthToken(et.owner, et.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read", "conversations:write", "messages:write", "contacts:read", "contacts:write"})
	if tools := toolNames(t, h.mcpSession(token)); slices.Contains(tools, "compose_mail") {
		t.Fatalf("compose_mail offered with bots_may_send off: %v", tools)
	}
	if _, errText := callTool(t, h.mcpSession(token), "compose_mail", map[string]any{"inbox_id": et.inbox, "to": []string{"mcp@" + d}, "subject": "S", "body": "B"}); errText == "" {
		t.Fatal("compose_mail ran without bots_may_send")
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	cs := h.mcpSession(token)
	if tools := toolNames(t, cs); !slices.Contains(tools, "compose_mail") {
		t.Fatalf("compose_mail missing with bots_may_send on: %v", tools)
	}
	got, errText := callTool(t, cs, "compose_mail", map[string]any{"inbox_id": et.inbox, "to": []string{"mcp@" + d}, "cc": []string{"mcp-cc@" + d}, "subject": "From the assistant", "body": "Hello."})
	if errText != "" {
		t.Fatal(errText)
	}
	if got["conversation"].(map[string]any)["customer_content"].(map[string]any)["subject"] != "From the assistant" || got["message"].(map[string]any)["kind"] != "message" {
		t.Fatalf("compose_mail: %v", got)
	}
}

func TestComposeBounces(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	signer := newSNSSigner(t, h.web)
	ctx := context.Background()
	d := unique("bounce") + ".example.org"
	fixture, err := os.ReadFile("../email/testdata/ses_bounce.json")
	if err != nil {
		t.Fatal(err)
	}
	bounce := func(conv, msgID, rcpt string) map[string]any {
		h.sendQueued(t, et.ws, msgID)
		ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)
		payload := strings.NewReplacer("{{MESSAGE_ID}}", ourID, "{{RECIPIENT}}", rcpt).Replace(string(fixture))
		note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: payload, Timestamp: time.Now().UTC().Format(time.RFC3339)}
		if st := h.postSNS(signer.sign(t, note)); st != http.StatusOK {
			t.Fatalf("bounce notification: %d", st)
		}
		return lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any)
	}
	suppressed := func(addr string) bool {
		var n int
		if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM email_suppressions WHERE workspace_id = $1 AND email = $2 AND scope = 'all'", et.ws, addr).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}

	ann, ben := "ann@"+d, "ben@"+d
	many := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{ann, ben}, "subject": "Two", "body": "x"})
	conv, msg := many.body["conversation"].(map[string]any)["id"].(string), many.body["message"].(map[string]any)["id"].(string)
	if dv := bounce(conv, msg, ben); dv["state"] == "failed" || !suppressed(ben) {
		t.Fatalf("one of two To bounced: delivery %v, suppressed %v", dv, suppressed(ben))
	}

	solo := "solo@" + d
	one := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{solo}, "cc": []string{"copy@" + d}, "subject": "One", "body": "x"})
	conv, msg = one.body["conversation"].(map[string]any)["id"].(string), one.body["message"].(map[string]any)["id"].(string)
	if dv := bounce(conv, msg, solo); dv["state"] != "failed" || !suppressed(solo) {
		t.Fatalf("the only To bounced: delivery %v", dv)
	}
}

func TestComposeRecipientBudget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	d := unique("budget") + ".example.org"
	mail := func(n int, extra map[string]any) map[string]any {
		hidden := make([]string, 50)
		for i := range hidden {
			hidden[i] = fmt.Sprintf("b%d-%d@%s", n, i, d)
		}
		b := map[string]any{"inbox_id": et.inbox, "to": []string{fmt.Sprintf("to%d@%s", n, d)}, "bcc": hidden, "subject": "Budget", "body": "x"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	first := mail(0, map[string]any{"client_id": "budget-0"})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", first)
	for i := 1; i < 5; i++ {
		et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", mail(i, nil))
	}
	draft := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", mail(5, map[string]any{"draft": true}))
	r := et.owner.do("POST", "/v1/conversations/mail", mail(6, nil))
	if r.status != http.StatusTooManyRequests || r.str("code") != "rate_limited" || !strings.Contains(r.str("detail"), "300 recipients an hour") {
		t.Fatalf("over the recipient budget: %d %s", r.status, r.raw)
	}
	et.owner.expect(http.StatusOK, "POST", "/v1/conversations/mail", first)
	et.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/messages/"+draft.body["message"].(map[string]any)["id"].(string)+"/send", nil)
}

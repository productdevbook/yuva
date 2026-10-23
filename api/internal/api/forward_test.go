package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jhillyerd/enmime/v2"

	"github.com/productdevbook/yuva/api/internal/email"
)

func mailWithFile(from, to, subject, messageID, body, filename, content string, headers map[string]string) []byte {
	var b strings.Builder
	b.WriteString("From: Customer <" + from + ">\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nMessage-ID: <" + messageID + ">\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	for k, v := range headers {
		b.WriteString(k + ": " + v + "\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n")
	b.WriteString("--b1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body + "\r\n")
	b.WriteString("--b1\r\nContent-Type: text/plain; name=\"" + filename + "\"\r\nContent-Disposition: attachment; filename=\"" + filename + "\"\r\n\r\n" + content + "\r\n")
	b.WriteString("--b1--\r\n")
	return []byte(b.String())
}

func TestForward(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	other := newEmailTeam(t, h, false)
	ctx := context.Background()
	d := unique("fwd") + ".example.org"
	cust, vendor, boss := "cust@"+d, "vendor@"+d, "boss@"+d
	et.owner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Deniz Kaya"})

	r := h.ingest(et.address, mailWithFile(cust, et.address, "Broken invoice", newMessageID(), "The invoice total is wrong.", "invoice.txt", "INVOICE-CONTENT",
		map[string]string{"Authentication-Results": dmarcPass(d)}), nil)
	if r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv := r.str("conversation_id")
	firstMsg := r.str("message_id")
	post := "/v1/conversations/" + conv + "/messages"
	reply := et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "message", "body": "We are looking into it."})
	h.sendQueued(t, et.ws, reply.str("id"))
	note := et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "note", "body": "internal-secret-note"})
	et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "message", "body": "draft-secret-text", "draft": true})
	fwdPath := "/v1/conversations/" + conv + "/forward"

	fw := et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{vendor, strings.ToUpper(boss)}, "note": "Can you check this?"})
	if fw.body["kind"] != "forward" || fw.body["direction"] != "out" || fw.body["body"] != "Can you check this?" {
		t.Fatalf("forward: %s", fw.raw)
	}
	spec := fw.body["forward"].(map[string]any)
	ids := strs(spec["message_ids"])
	if spec["scope"] != "conversation" || len(ids) != 2 || ids[0] != firstMsg || ids[1] != reply.str("id") || spec["attachments"] != true || !slices.Equal(strs(spec["to"]), []string{vendor, boss}) {
		t.Fatalf("forward spec: %v", spec)
	}
	if slices.Contains(ids, note.str("id")) {
		t.Fatal("a note was forwarded")
	}
	if fw.body["delivery"].(map[string]any)["state"] != "queued" || !slices.Equal(strs(fw.body["email"].(map[string]any)["to"]), []string{vendor, boss}) {
		t.Fatalf("forward delivery: %s", fw.raw)
	}
	var queue string
	var maxAttempts int
	if err := h.st.Pool.QueryRow(ctx, "SELECT queue, max_attempts FROM river_job WHERE kind = 'email_send' AND args->>'message_id' = $1", fw.str("id")).Scan(&queue, &maxAttempts); err != nil {
		t.Fatal(err)
	}
	if queue != "email" || maxAttempts != 8 {
		t.Fatalf("forward job on queue %q with %d attempts", queue, maxAttempts)
	}
	h.sendQueued(t, et.ws, fw.str("id"))
	sent, parsed := h.smtp.last(t)
	if !slices.Equal(sent.to, []string{vendor, boss}) || sent.from != et.address {
		t.Fatalf("envelope %s -> %v", sent.from, sent.to)
	}
	if got := parsed.Header.Get("From"); got != "Deniz Kaya <"+et.address+">" {
		t.Fatalf("From %q", got)
	}
	if got := parsed.Header.Get("Reply-To"); got != "Acme Support <"+et.address+">" {
		t.Fatalf("Reply-To %q", got)
	}
	if got := parsed.Header.Get("Subject"); got != "Fwd: Broken invoice" {
		t.Fatalf("Subject %q", got)
	}
	if parsed.Header.Get("In-Reply-To") != "" || parsed.Header.Get("References") != "" || parsed.Header.Get("Cc") != "" || parsed.Header.Get("Auto-Submitted") != "" {
		t.Fatalf("thread headers on a forward: %v", parsed.Header)
	}
	to := parsed.Header.Get("To")
	if !strings.Contains(to, vendor) || !strings.Contains(to, boss) || strings.Contains(to, cust) {
		t.Fatalf("To %q", to)
	}
	fwdID := email.NormalizeID(parsed.Header.Get("Message-ID"))
	if fwdID == "" || fwdID != email.NormalizeID(fw.body["email"].(map[string]any)["message_id"].(string)) {
		t.Fatalf("Message-ID %q", fwdID)
	}
	env, err := enmime.ReadEnvelope(bytes.NewReader(sent.raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Can you check this?", "The invoice total is wrong.", "We are looking into it.", "Forwarded message", "Subject: Broken invoice"} {
		if !strings.Contains(env.Text, want) {
			t.Fatalf("text misses %q:\n%s", want, env.Text)
		}
	}
	if strings.Contains(env.HTML, "<script") || !strings.Contains(env.HTML, "The invoice total is wrong.") {
		t.Fatalf("html: %s", env.HTML)
	}
	for _, secret := range []string{"internal-secret-note", "draft-secret-text"} {
		if bytes.Contains(sent.raw, []byte(secret)) {
			t.Fatalf("%s went out", secret)
		}
	}
	if len(env.Attachments) != 1 || env.Attachments[0].FileName != "invoice.txt" || !bytes.Contains(env.Attachments[0].Content, []byte("INVOICE-CONTENT")) {
		t.Fatalf("attachments: %v", env.Attachments)
	}

	one := et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{vendor}, "message_id": firstMsg, "attachments": false})
	if sp := one.body["forward"].(map[string]any); sp["scope"] != "message" || sp["attachments"] != false || !slices.Equal(strs(sp["message_ids"]), []string{firstMsg}) {
		t.Fatalf("one message: %s", one.raw)
	}
	h.sendQueued(t, et.ws, one.str("id"))
	sent, _ = h.smtp.last(t)
	env, err = enmime.ReadEnvelope(bytes.NewReader(sent.raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Attachments) != 0 || bytes.Contains(sent.raw, []byte("INVOICE-CONTENT")) || strings.Contains(env.Text, "We are looking into it.") || !strings.Contains(env.Text, "The invoice total is wrong.") {
		t.Fatalf("one message forward: %s", env.Text)
	}

	listed := messages(et.owner, conv)
	if f := lastOf(listed, "forward"); f == nil || f["id"] != one.str("id") {
		t.Fatalf("forward not in the timeline: %v", listed)
	}

	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{cust}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{other.address}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{et.address}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{"not an address"}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{}})
	many := make([]string, 11)
	for i := range many {
		many[i] = unique("m") + "@" + d
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": many})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{vendor}, "message_id": note.str("id")})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{vendor}, "message_id": fw.str("id")})
	bounced := "bounced@" + d
	if _, err := h.st.Pool.Exec(ctx, "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, 'bounce', 'all', now())", et.ws, bounced); err != nil {
		t.Fatal(err)
	}
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", fwdPath, map[string]any{"to": []string{vendor, bounced}})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", fwdPath, map[string]any{"to": []string{vendor}})

	stranger := h.ingest(et.address, buildMail(mailOpts{
		from: vendor, to: et.address, subject: "Re: Fwd: Broken invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + fwdID + ">"}, body: "Unverified answer.",
	}), nil)
	if stranger.str("conversation_id") == conv {
		t.Fatal("an answer without DMARC joined the conversation")
	}
	elsewhere := h.ingest(et.address, buildMail(mailOpts{
		from: vendor, to: et.address, subject: "Re: Broken invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + email.NormalizeID(reply.body["email"].(map[string]any)["message_id"].(string)) + ">", "Authentication-Results": dmarcPass(d)}, body: "Wrong thread.",
	}), nil)
	if elsewhere.str("conversation_id") == conv {
		t.Fatal("a forward recipient joined through a Message-ID that was not the forward's")
	}
	answer := h.ingest(et.address, buildMail(mailOpts{
		from: vendor, to: et.address, subject: "Re: Fwd: Broken invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + fwdID + ">", "Cc": "helper@" + d, "Authentication-Results": dmarcPass(d)}, body: "The tax rate is outdated.",
	}), nil)
	if answer.status != http.StatusAccepted || answer.str("conversation_id") != conv {
		t.Fatalf("verified answer did not join: %d %v", answer.status, answer.body)
	}
	last := lastOf(messages(et.owner, conv), "forward")
	if last["id"] != answer.str("message_id") || last["direction"] != "in" || last["body"] != "The tax rate is outdated." {
		t.Fatalf("answer: %v", last)
	}
	parts := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/participants", nil).body["items"].([]any)
	roles := map[string]string{}
	statuses := map[string]string{}
	for _, p := range parts {
		pm := p.(map[string]any)
		roles[pm["email"].(string)], statuses[pm["email"].(string)] = pm["role"].(string), pm["status"].(string)
	}
	if roles[vendor] != "forwarded" || statuses[vendor] != "active" || roles["helper@"+d] != "forwarded" || statuses["helper@"+d] != "suggested" {
		t.Fatalf("participants: %v %v", roles, statuses)
	}
	again := h.ingest(et.address, buildMail(mailOpts{
		from: vendor, to: et.address, subject: "Re: Fwd: Broken invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + fwdID + ">", "Authentication-Results": dmarcPass(d)}, body: "One more thing.",
	}), nil)
	if again.str("conversation_id") != conv || lastOf(messages(et.owner, conv), "forward")["id"] != again.str("message_id") {
		t.Fatalf("second answer: %v", again.body)
	}

	toCustomer := et.owner.expect(http.StatusCreated, "POST", post, map[string]any{"kind": "message", "body": "Fixed."})
	h.sendQueued(t, et.ws, toCustomer.str("id"))
	sent, parsed = h.smtp.last(t)
	if !slices.Equal(sent.to, []string{cust}) || parsed.Header.Get("Cc") != "" {
		t.Fatalf("customer reply copied a forward recipient: %v %q", sent.to, parsed.Header.Get("Cc"))
	}
	if strings.Contains(parsed.Header.Get("References"), fwdID) || strings.Contains(parsed.Header.Get("In-Reply-To"), fwdID) {
		t.Fatalf("customer reply names the forward: %v", parsed.Header)
	}

	draft := et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{boss}, "draft": true})
	if draft.body["draft"] != true || draft.body["delivery"] != nil {
		t.Fatalf("draft forward: %s", draft.raw)
	}
	before := h.smtp.count()
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/messages/"+draft.str("id")+"/send", map[string]any{"cc": []string{vendor}})
	sentDraft := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+draft.str("id")+"/send", nil)
	if sentDraft.body["kind"] != "forward" || sentDraft.body["draft"] != false || sentDraft.body["delivery"].(map[string]any)["state"] != "queued" {
		t.Fatalf("sent draft: %s", sentDraft.raw)
	}
	if h.smtp.count() != before {
		t.Fatal("a draft was mailed before it was sent")
	}
	h.sendQueued(t, et.ws, sentDraft.str("id"))
	if sent, _ = h.smtp.last(t); !slices.Equal(sent.to, []string{boss}) {
		t.Fatalf("draft envelope %v", sent.to)
	}

	vendorContact := last["author"].(map[string]any)["contact_id"].(string)
	pp := et.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+vendorContact+"/participations", nil).body["items"].([]any)
	if len(pp) != 1 || pp[0].(map[string]any)["conversation_id"] != conv {
		t.Fatalf("participations: %v", pp)
	}
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+vendorContact, nil)
	var left int
	if err := h.st.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND $2 = ANY(to_addresses))
		+ (SELECT count(*) FROM messages WHERE workspace_id = $1 AND forward->'to' ? $2)
		+ (SELECT count(*) FROM conversation_participants WHERE workspace_id = $1 AND email = $2)`, et.ws, vendor).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d rows still name the deleted forward recipient", left)
	}
}

func TestForwardHiddenFromContact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	cs := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "user-1"})})
	started := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations", map[string]any{"body": "Hello from the widget"})
	conv := started.body["conversation"].(map[string]any)["id"].(string)
	fwdPath := "/v1/conversations/" + conv + "/forward"
	ct.owner.expectProblem(http.StatusConflict, "email_not_configured", "POST", fwdPath, map[string]any{"to": []string{"ops@example.org"}})

	addr := unique("chatmail") + "@example.com"
	ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
		"kind": "email", "name": "Chat mail", "email": map[string]any{
			"address": addr, "display_name": "Chat Support",
			"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	fw := ct.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{"ops@example.org"}, "note": "forward-note-secret"})
	h.sendQueued(t, ct.ws, fw.str("id"))
	sent, parsed := h.smtp.last(t)
	if !slices.Equal(sent.to, []string{"ops@example.org"}) || !strings.Contains(parsed.Header.Get("From"), addr) {
		t.Fatalf("chat forward: %v %q", sent.to, parsed.Header.Get("From"))
	}
	body, _ := io.ReadAll(parsed.Body)
	if !bytes.Contains(sent.raw, []byte("Hello from the widget")) {
		t.Fatalf("chat forward body: %s", body)
	}

	client := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages", nil)
	if bytes.Contains(client.raw, []byte("forward-note-secret")) || bytes.Contains(client.raw, []byte(fw.str("id"))) || bytes.Contains(client.raw, []byte("ops@example.org")) {
		t.Fatalf("the contact sees the forward: %s", client.raw)
	}
	list := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil)
	if bytes.Contains(list.raw, []byte("forward-note-secret")) {
		t.Fatalf("the contact's list shows the forward: %s", list.raw)
	}
}

func TestForwardRateLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	r := h.ingest(et.address, buildMail(mailOpts{from: "cust@" + unique("rl") + ".example.org", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello."}), nil)
	fwdPath := "/v1/conversations/" + r.str("conversation_id") + "/forward"
	for i := 0; i < 20; i++ {
		et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{"ops@example.org"}})
	}
	et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{"ops@example.org"}, "draft": true})
	et.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", fwdPath, map[string]any{"to": []string{"ops@example.org"}})
}

func TestForwardAnswerFromParticipants(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	d := unique("fwp") + ".example.org"
	cust, sugg, vendor, col := "cust@"+d, "sugg@"+d, "vendor@"+d, "col@"+d

	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{
		from: cust, to: et.address, subject: "Order", messageID: first, body: "Where is my order?",
		headers: map[string]string{"Cc": sugg},
	}), nil)
	conv := r.str("conversation_id")
	fwdPath := "/v1/conversations/" + conv + "/forward"
	et.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/participants", map[string]any{"email": col})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{col}})

	forwardID := func(to string) string {
		t.Helper()
		fw := et.owner.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{to}, "note": "internal-note-" + to})
		h.sendQueued(t, et.ws, fw.str("id"))
		_, parsed := h.smtp.last(t)
		return email.NormalizeID(parsed.Header.Get("Message-ID"))
	}
	suggFwd := forwardID(sugg)
	vendorFwd := forwardID(vendor)
	et.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/participants", map[string]any{"email": vendor})

	answer := func(from, fwdID, body string) {
		t.Helper()
		res := h.ingest(et.address, buildMail(mailOpts{
			from: from, to: et.address, subject: "Re: Fwd: Order", messageID: newMessageID(),
			headers: map[string]string{"In-Reply-To": "<" + fwdID + ">", "Authentication-Results": dmarcPass(d)}, body: body,
		}), nil)
		if res.str("conversation_id") != conv {
			t.Fatalf("%s's answer did not join: %v", from, res.body)
		}
		last := messages(et.owner, conv)
		m := last[len(last)-1]
		if m["id"] != res.str("message_id") || m["kind"] != "forward" || m["direction"] != "in" {
			t.Fatalf("%s's answer to a forward is not members-only: %v", from, m)
		}
	}
	answer(vendor, vendorFwd, "Shipped yesterday.")
	answer(sugg, suggFwd, "I can confirm.")

	parts := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/participants", nil).body["items"].([]any)
	got := map[string]string{}
	for _, p := range parts {
		pm := p.(map[string]any)
		got[pm["email"].(string)] = pm["role"].(string) + "/" + pm["status"].(string)
	}
	if got[vendor] != "cc/active" || got[sugg] != "forwarded/active" || got[col] != "cc/active" {
		t.Fatalf("participants: %v", got)
	}
	for _, m := range messages(et.owner, conv) {
		if m["kind"] == "message" && m["direction"] == "in" && m["body"] != "Where is my order?" {
			t.Fatalf("an answer to a forward is a customer-visible message: %v", m)
		}
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "On its way."})
	h.sendQueued(t, et.ws, reply.str("id"))
	sent, _ := h.smtp.last(t)
	slices.Sort(sent.to)
	if want := []string{col, cust, vendor}; !slices.Equal(sent.to, want) {
		t.Fatalf("reply envelope %v, want %v", sent.to, want)
	}
}

func TestForwardAnswerNotifications(t *testing.T) {
	t.Parallel()
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	addr := unique("notify") + "@example.com"
	nt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+nt.async+"/channels", map[string]any{
		"kind": "email", "name": "Mail", "email": map[string]any{
			"address": addr, "display_name": "Acme Support",
			"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	ownerB, agentB := fake.subscribe(nt.owner), fake.subscribe(nt.agent)
	d := unique("fwn") + ".example.org"
	r := h.ingest(addr, buildMail(mailOpts{from: "cust@" + d, to: addr, subject: "Refund", messageID: newMessageID(), body: "Refund please."}), nil)
	conv := r.str("conversation_id")
	h.runNotifications(nt.ws)
	fake.take()
	h.emailChecks(nt.ws)

	fw := nt.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/forward", map[string]any{"to": []string{"finance@" + d}})
	h.sendQueued(t, nt.ws, fw.str("id"))
	_, parsed := h.smtp.last(t)
	fwdID := email.NormalizeID(parsed.Header.Get("Message-ID"))
	h.runNotifications(nt.ws)
	if got := fake.take(); len(got) != 0 {
		t.Fatalf("a forward notified: %v", events(got))
	}
	answer := func(body string) {
		t.Helper()
		h.clock.Advance(time.Minute)
		res := h.ingest(addr, buildMail(mailOpts{
			from: "finance@" + d, to: addr, subject: "Re: Fwd: Refund", messageID: newMessageID(),
			headers: map[string]string{"In-Reply-To": "<" + fwdID + ">", "Authentication-Results": dmarcPass(d)}, body: body,
		}), nil)
		if res.str("conversation_id") != conv {
			t.Fatalf("answer did not join: %v", res.body)
		}
		h.runNotifications(nt.ws)
	}

	answer("Approved.")
	got := fake.take()
	expectPushes(t, got, ownerB)
	if p := expectPushes(t, got, agentB, "forward_answer"); !strings.Contains(p[0].payload["body"].(string), "answered a forward: Approved.") {
		t.Fatalf("push: %v", p[0].payload)
	}
	checks := h.emailChecks(nt.ws)
	if len(checks) != 1 || checks[0].MemberID.String() != nt.agentID {
		t.Fatalf("e-mail checks %v", checks)
	}
	agentEmail := nt.agent.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
	before := len(h.mail.to(agentEmail))
	h.clock.Advance(time.Hour)
	if next, err := h.srv.SendNotificationEmail(context.Background(), checks[0]); err != nil || next != nil {
		t.Fatalf("e-mail check: next %v err %v", next, err)
	}
	mails := h.mail.to(agentEmail)[before:]
	if len(mails) != 1 || !strings.Contains(mails[0].Subject, "a forward was answered") || !strings.Contains(mails[0].Text, "answered a forward:\n> Approved.") {
		t.Fatalf("e-mail %v", mails)
	}

	nt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nt.ownerID})
	h.runNotifications(nt.ws)
	fake.take()
	h.emailChecks(nt.ws)
	answer("Paid out.")
	got = fake.take()
	expectPushes(t, got, ownerB, "forward_answer")
	expectPushes(t, got, agentB, "forward_answer")

	nt.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{"events": map[string]any{"forward_answer": map[string]any{"push": false, "email": false}}})
	answer("Done.")
	got = fake.take()
	expectPushes(t, got, ownerB, "forward_answer")
	expectPushes(t, got, agentB)
}

func TestForwardTypedAddressAndDraftSender(t *testing.T) {
	t.Parallel()
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	addr := unique("typed") + "@example.com"
	nt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+nt.async+"/channels", map[string]any{
		"kind": "email", "name": "Mail", "email": map[string]any{
			"address": addr, "display_name": "Acme Support",
			"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	nt.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Selin Ak"})
	ownerB, agentB := fake.subscribe(nt.owner), fake.subscribe(nt.agent)
	d := unique("fwt") + ".example.org"
	typed := "typed@" + d
	r := h.ingest(addr, buildMail(mailOpts{from: "cust@" + d, to: addr, subject: "Login", messageID: newMessageID(), body: "I cannot log in."}), nil)
	conv := r.str("conversation_id")
	fwdPath := "/v1/conversations/" + conv + "/forward"
	h.runNotifications(nt.ws)
	fake.take()
	h.emailChecks(nt.ws)

	draft := nt.key.expect(http.StatusCreated, "POST", fwdPath, map[string]any{"to": []string{typed}, "draft": true})
	sent := nt.agent.expect(http.StatusOK, "POST", "/v1/messages/"+draft.str("id")+"/send", nil)
	h.sendQueued(t, nt.ws, sent.str("id"))
	_, parsed := h.smtp.last(t)
	if got := parsed.Header.Get("From"); got != "Selin Ak <"+addr+">" {
		t.Fatalf("From of a draft sent by a member: %q", got)
	}
	fwdID := email.NormalizeID(parsed.Header.Get("Message-ID"))

	var contact string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT contact_id FROM conversations WHERE id = $1", conv).Scan(&contact); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE contacts SET typed_email = $1 WHERE id = $2", typed, contact); err != nil {
		t.Fatal(err)
	}
	nt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", fwdPath, map[string]any{"to": []string{typed}})

	h.clock.Advance(time.Minute)
	res := h.ingest(addr, buildMail(mailOpts{
		from: typed, to: addr, subject: "Re: Fwd: Login", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + fwdID + ">"}, body: "Reset the password.",
	}), nil)
	if res.str("conversation_id") != conv {
		t.Fatalf("typed-address answer: %v", res.body)
	}
	last := messages(nt.owner, conv)
	if m := last[len(last)-1]; m["id"] != res.str("message_id") || m["kind"] != "forward" || m["direction"] != "in" {
		t.Fatalf("an answer from the contact's typed address is customer-visible: %v", m)
	}
	h.runNotifications(nt.ws)
	got := fake.take()
	expectPushes(t, got, agentB, "forward_answer")
	expectPushes(t, got, ownerB)
}

package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/mail"
)

type confirmationView struct {
	email, state string
	expires      bool
}

func confirmationOf(t *testing.T, contact map[string]any) confirmationView {
	t.Helper()
	c, ok := contact["email_confirmation"].(map[string]any)
	if !ok {
		return confirmationView{}
	}
	_, expires := c["expires_at"].(string)
	return confirmationView{c["email"].(string), c["state"].(string), expires}
}

func sessionConfirmation(t *testing.T, cs contactSession) confirmationView {
	t.Helper()
	return confirmationOf(t, cs.expect(http.StatusOK, "GET", "/client/v1/session", nil).body["contact"].(map[string]any))
}

func suppressionOf(t *testing.T, h *harness, ws, addr string) (reason, scope string) {
	t.Helper()
	_ = h.st.Pool.QueryRow(t.Context(), "SELECT reason, scope FROM email_suppressions WHERE workspace_id = $1 AND email = $2", ws, addr).Scan(&reason, &scope)
	return reason, scope
}

func typeEmail(t *testing.T, h *harness, ct chatTeam, addr string) (contactSession, confirmationView) {
	t.Helper()
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	return cs, confirmationOf(t, cs.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": addr}).body)
}

func TestTypedEmailBounce(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	server := newEmailTeam(t, h, false)
	conversations := func() float64 {
		n, _ := server.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["all"].(float64)
		return n
	}
	before := conversations()

	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.nextNot("presence")
	typed := unique("gone") + "@example.net"
	r := cs.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": typed})
	if v := confirmationOf(t, r.body); v.email != typed || v.state != "sent" || !v.expires {
		t.Fatalf("after typing: %+v (%s)", v, r.raw)
	}
	msg := h.mail.wait(t, typed, 1)[0]
	if !strings.HasPrefix(msg.MessageID, "confirm.") {
		t.Fatalf("confirmation Message-ID %q", msg.MessageID)
	}
	if m := ws.nextNot("presence", "typing"); m.Type != "contact.updated" || m.ID == 0 {
		t.Fatalf("frame after typing: %s %s", m.Type, m.Data)
	}

	if b := h.ingestFrom(server.address, "", dsnMail(server.address, msg.MessageID, typed, "4.4.1"), nil); b.status != http.StatusAccepted || b.str("status") != "bounce" {
		t.Fatalf("temporary dsn: %d %v", b.status, b.body)
	}
	if v := sessionConfirmation(t, cs); v.state != "sent" {
		t.Fatalf("a temporary failure changed the state: %+v", v)
	}
	if reason, _ := suppressionOf(t, h, ct.ws, typed); reason != "" {
		t.Fatal("a temporary failure suppressed the address")
	}
	ws.quiet(300*time.Millisecond, "presence", "typing")

	if b := h.ingestFrom(server.address, "", dsnMail(server.address, msg.MessageID, typed, "5.1.1"), nil); b.status != http.StatusAccepted || b.str("status") != "bounce" {
		t.Fatalf("permanent dsn: %d %v", b.status, b.body)
	}
	if v := sessionConfirmation(t, cs); v.email != typed || v.state != "undeliverable" || v.expires {
		t.Fatalf("after the bounce: %+v", v)
	}
	m := ws.nextNot("presence", "typing")
	var frame map[string]any
	_ = json.Unmarshal(m.Data, &frame)
	if v := confirmationOf(t, frame); m.Type != "contact.updated" || v.state != "undeliverable" {
		t.Fatalf("frame after the bounce: %s %s", m.Type, m.Data)
	}
	if after := conversations(); after != before {
		t.Fatalf("the delivery report opened a conversation: %v -> %v", before, after)
	}
	var contacts int
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT count(*) FROM contact_emails WHERE workspace_id IN ($1, $2) AND (email = $3 OR email = 'mailer-daemon@mx.example.net')", ct.ws, server.ws, typed).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 0 {
		t.Fatalf("%d contacts have the bounced address or the reporter", contacts)
	}
	if reason, scope := suppressionOf(t, h, ct.ws, typed); reason != "bounce" || scope != "confirmations" {
		t.Fatalf("suppression %q %q, want bounce for confirmations only", reason, scope)
	}

	other, v := typeEmail(t, h, ct, typed)
	if v.email != typed || v.state != "undeliverable" {
		t.Fatalf("typing a bounced address: %+v", v)
	}
	if v := sessionConfirmation(t, other); v.state != "undeliverable" {
		t.Fatalf("session after typing a bounced address: %+v", v)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(h.mail.to(typed)); n != 1 {
		t.Fatalf("%d mails to the bounced address, want 1", n)
	}
	customer := ct.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Customer", "emails": []string{typed}})
	if und := customer.body["undeliverable"].([]any); len(und) != 0 {
		t.Fatalf("a confirmation bounce marks the customer undeliverable for replies: %v", und)
	}
}

func TestTypedEmailSuppressionOutcome(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	suppress := func(addr, reason, scope string) {
		t.Helper()
		if _, err := h.st.Pool.Exec(t.Context(), "INSERT INTO email_suppressions (workspace_id, email, reason, scope, created_at) VALUES ($1, $2, $3, $4, now())", ct.ws, addr, reason, scope); err != nil {
			t.Fatal(err)
		}
	}
	fresh := unique("fresh") + "@example.com"
	_, sent := typeEmail(t, h, ct, fresh)
	h.mail.wait(t, fresh, 1)

	complained := unique("complained") + "@example.com"
	suppress(complained, "complaint", "broadcasts")
	cs, v := typeEmail(t, h, ct, complained)
	if v.email != complained || v.state != sent.state || !v.expires {
		t.Fatalf("a complaint shows %+v, a clean address %+v", v, sent)
	}
	if v := sessionConfirmation(t, cs); v.state != "sent" {
		t.Fatalf("session for a complaint: %+v", v)
	}
	manual := unique("manual") + "@example.com"
	suppress(manual, "manual", "all")
	if _, v := typeEmail(t, h, ct, manual); v.state != "sent" {
		t.Fatalf("a manual suppression shows %+v", v)
	}
	bounced := unique("bounced") + "@example.com"
	suppress(bounced, "bounce", "broadcasts")
	if _, v := typeEmail(t, h, ct, bounced); v.state != "undeliverable" {
		t.Fatalf("a bounce of any scope shows %+v", v)
	}

	for addr, want := range map[string]string{complained: "sent", bounced: "undeliverable"} {
		fb := ct.session(h, map[string]any{})
		fb.expect(http.StatusCreated, "POST", "/client/v1/feedback", map[string]any{"body": "it breaks", "allow_email": true, "email": addr})
		if v := sessionConfirmation(t, fb); v.email != addr || v.state != want {
			t.Fatalf("feedback with %s: %+v, want %s", addr, v, want)
		}
	}
	time.Sleep(50 * time.Millisecond)
	for _, addr := range []string{complained, manual, bounced} {
		if n := len(h.mail.to(addr)); n != 0 {
			t.Fatalf("%d mails to suppressed %s", n, addr)
		}
	}
}

func TestTypedEmailLateBounce(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	server := newEmailTeam(t, h, false)
	bounce := func(id, addr string) {
		t.Helper()
		if b := h.ingestFrom(server.address, "", dsnMail(server.address, id, addr, "5.1.1"), nil); b.str("status") != "bounce" {
			t.Fatalf("dsn: %v", b.body)
		}
	}

	confirmed := unique("confirmed") + "@example.com"
	cs, _ := typeEmail(t, h, ct, confirmed)
	msg := h.mail.wait(t, confirmed, 1)[0]
	confirmTypedEmail(t, h, confirmLink(t, msg.Text))
	bounce(msg.MessageID, confirmed)
	if reason, scope := suppressionOf(t, h, ct.ws, confirmed); reason != "bounce" || scope != "confirmations" {
		t.Fatalf("a bounce after confirmation: suppression %q %q", reason, scope)
	}
	if v := sessionConfirmation(t, cs); v.email != confirmed || v.state != "confirmed" {
		t.Fatalf("after confirming: %+v", v)
	}
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+cs.contactID, map[string]any{"emails": []string{unique("new") + "@example.com"}})
	if v := sessionConfirmation(t, cs); v.state != "" {
		t.Fatalf("confirmed shown for an address the contact no longer has: %+v", v)
	}

	first := unique("first") + "@example.com"
	replaced, _ := typeEmail(t, h, ct, first)
	firstMsg := h.mail.wait(t, first, 1)[0]
	second := unique("second") + "@example.com"
	replaced.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": second})
	h.mail.wait(t, second, 1)
	bounce(firstMsg.MessageID, first)
	if reason, scope := suppressionOf(t, h, ct.ws, first); reason != "bounce" || scope != "confirmations" {
		t.Fatalf("a bounce for a replaced confirmation: suppression %q %q", reason, scope)
	}
	if v := sessionConfirmation(t, replaced); v.email != second || v.state != "sent" {
		t.Fatalf("after a report for the replaced address: %+v", v)
	}

	late := unique("late") + "@example.com"
	expired, _ := typeEmail(t, h, ct, late)
	lateMsg := h.mail.wait(t, late, 1)[0]
	h.clock.Advance(25 * time.Hour)
	if v := sessionConfirmation(t, expired); v.state != "" {
		t.Fatalf("an expired link still shows: %+v", v)
	}
	bounce(lateMsg.MessageID, strings.ToUpper(late))
	if reason, scope := suppressionOf(t, h, ct.ws, late); reason != "bounce" || scope != "confirmations" {
		t.Fatalf("a bounce for an expired confirmation, recipient in capitals: suppression %q %q", reason, scope)
	}
	if v := sessionConfirmation(t, expired); v.state != "" {
		t.Fatalf("a bounce for an expired confirmation changed the state: %+v", v)
	}
}

func TestTypedEmailSESBounce(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	signer := newSNSSigner(t, h.web)
	fixture, err := os.ReadFile("../email/testdata/ses_bounce.json")
	if err != nil {
		t.Fatal(err)
	}
	notify := func(id, rcpt, kind string) {
		t.Helper()
		payload := strings.NewReplacer("{{MESSAGE_ID}}", id, "{{RECIPIENT}}", rcpt, `"Permanent"`, `"`+kind+`"`).Replace(string(fixture))
		note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: payload, Timestamp: time.Now().UTC().Format(time.RFC3339)}
		if st := h.postSNS(signer.sign(t, note)); st != http.StatusOK {
			t.Fatalf("%s notification: %d", kind, st)
		}
	}

	typed := unique("gone") + "@example.net"
	cs, _ := typeEmail(t, h, ct, typed)
	id := h.mail.wait(t, typed, 1)[0].MessageID

	notify(id, typed, "Transient")
	if v := sessionConfirmation(t, cs); v.state != "sent" {
		t.Fatalf("a transient bounce changed the state: %+v", v)
	}
	notify(id, unique("someone")+"@example.org", "Permanent")
	if v := sessionConfirmation(t, cs); v.state != "sent" {
		t.Fatalf("a bounce for another recipient changed the state: %+v", v)
	}
	notify(id, typed, "Permanent")
	if v := sessionConfirmation(t, cs); v.email != typed || v.state != "undeliverable" {
		t.Fatalf("after the SES bounce: %+v", v)
	}
	if reason, scope := suppressionOf(t, h, ct.ws, typed); reason != "bounce" || scope != "confirmations" {
		t.Fatalf("suppression %q %q", reason, scope)
	}
	if _, err := h.st.Pool.Exec(t.Context(), "UPDATE email_confirmations SET bounced_at = NULL WHERE workspace_id = $1 AND email = $2", ct.ws, typed); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(t.Context(), "UPDATE email_suppressions SET scope = 'all' WHERE workspace_id = $1 AND email = $2", ct.ws, typed); err != nil {
		t.Fatal(err)
	}
	notify(id, typed, "Permanent")
	if _, scope := suppressionOf(t, h, ct.ws, typed); scope != "all" {
		t.Fatalf("a confirmation bounce narrowed a suppression of all mail to %q", scope)
	}
}

func TestTypedEmailRelayIDAndExpiry(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	server := newEmailTeam(t, h, false)
	typed := unique("gone") + "@example.net"
	cs, _ := typeEmail(t, h, ct, typed)
	msg := h.mail.wait(t, typed, 1)[0]
	relay := unique("0102019a") + "@eu-west-1.amazonses.com"
	msg.OnRelayed(t.Context(), relay)
	if b := h.ingestFrom(server.address, "", dsnMail(server.address, relay, typed, "5.1.1"), nil); b.str("status") != "bounce" {
		t.Fatalf("dsn naming the relay id: %v", b.body)
	}
	if v := sessionConfirmation(t, cs); v.state != "undeliverable" {
		t.Fatalf("after a dsn naming the relay id: %+v", v)
	}

	h.clock.Advance(31 * 24 * time.Hour)
	again, v := typeEmail(t, h, ct, typed)
	if v.state != "sent" {
		t.Fatalf("a confirmation bounce older than 30 days still blocks: %+v", v)
	}
	h.mail.wait(t, typed, 2)
	if v := sessionConfirmation(t, again); v.state != "sent" {
		t.Fatalf("session after the bounce expired: %+v", v)
	}
}

func TestTypedEmailComplaints(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	server := newEmailTeam(t, h, false)
	signer := newSNSSigner(t, h.web)

	viaSES := unique("ses") + "@example.net"
	sesSession, _ := typeEmail(t, h, ct, viaSES)
	sesMsg := h.mail.wait(t, viaSES, 1)[0]
	sesID := sesMsg.MessageID
	before := contactUpdates(t, h, ct.ws, sesSession.contactID)
	payload, _ := json.Marshal(map[string]any{
		"notificationType": "Complaint",
		"complaint":        map[string]any{"complainedRecipients": []map[string]any{{"emailAddress": viaSES}}, "complaintFeedbackType": "abuse"},
		"mail": map[string]any{"messageId": "x", "commonHeaders": map[string]any{"messageId": "<" + sesID + ">"},
			"tags": map[string]any{"ses:tenant-name": []string{"some-tenant"}}},
	})
	note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: string(payload), Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if st := h.postSNS(signer.sign(t, note)); st != http.StatusOK {
		t.Fatalf("complaint notification: %d", st)
	}

	if status, _ := confirmStatus(t, h, "GET", confirmLink(t, sesMsg.Text)); status != http.StatusNotFound {
		t.Fatalf("the link still opens after a complaint: %d", status)
	}
	if v := sessionConfirmation(t, sesSession); v.state != "sent" || !v.expires {
		t.Fatalf("after a complaint the visitor sees %+v, want what the silent case shows", v)
	}
	if after := contactUpdates(t, h, ct.ws, sesSession.contactID); after != before+1 {
		t.Fatalf("contact.updated events %d -> %d after the complaint", before, after)
	}

	viaARF := unique("arf") + "@example.net"
	typeEmail(t, h, ct, viaARF)
	arfID := h.mail.wait(t, viaARF, 1)[0].MessageID
	if r := h.ingest(server.address, arfMail("fbl@mailbox.example", server.address, "<"+arfID+">", viaARF), nil); r.str("status") != "bounce" {
		t.Fatalf("feedback report: %v", r.body)
	}

	for _, addr := range []string{viaSES, viaARF} {
		if reason, scope := suppressionOf(t, h, ct.ws, addr); reason != "complaint" || scope != "confirmations" {
			t.Fatalf("%s: suppression %q %q", addr, reason, scope)
		}
	}
	h.clock.Advance(31 * 24 * time.Hour)
	if _, v := typeEmail(t, h, ct, viaSES); v.state != "sent" {
		t.Fatalf("after a complaint: %+v", v)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(h.mail.to(viaSES)); n != 1 {
		t.Fatalf("%d mails after a complaint, want 1: complaints do not expire", n)
	}
}

func TestTypedEmailDiscardedConfirmation(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	typed := unique("taken") + "@example.com"
	cs, _ := typeEmail(t, h, ct, typed)
	token := confirmLink(t, h.mail.wait(t, typed, 1)[0].Text)
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.nextNot("presence")
	ct.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Owner", "emails": []string{typed}})
	if status, _ := confirmStatus(t, h, "POST", token); status != http.StatusNotFound {
		t.Fatalf("confirming an address another contact has: %d", status)
	}
	m := ws.nextNot("presence", "typing")
	var frame map[string]any
	_ = json.Unmarshal(m.Data, &frame)
	if v := confirmationOf(t, frame); m.Type != "contact.updated" || v.state != "" {
		t.Fatalf("frame after the discard: %s %s", m.Type, m.Data)
	}
}

func contactUpdates(t *testing.T, h *harness, ws, contact string) int {
	t.Helper()
	var n int
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'contact.updated' AND payload->>'id' = $2", ws, contact).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func typeFrom(t *testing.T, h *harness, ct chatTeam, ip, addr string) confirmationView {
	t.Helper()
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	cs.ip = ip
	return confirmationOf(t, cs.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": addr}).body)
}

func TestTypedEmailConfirmationCaps(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	long := "Claim your refund\n\nat evil.example " + strings.Repeat("x", 150)
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"name": long})
	user := unique("victim")
	victim := user + "@gmail.com"
	spellings := []string{victim, strings.ToUpper(user[:1]) + user[1:] + "@gmail.com", user + "+news@gmail.com", user[:3] + "." + user[3:] + "@googlemail.com"}
	for i := range 8 {
		addr := spellings[i%len(spellings)]
		want := "sent"
		if i >= 3 {
			want = "limited"
		}
		if v := typeFrom(t, h, ct, "10.31.1."+strconv.Itoa(i+1), addr); v.state != want || v.expires != (want == "sent") {
			t.Fatalf("visitor %d of one network typing %s: %+v, want %s", i+1, addr, v, want)
		}
	}
	if v := typeFrom(t, h, ct, "10.32.1.5", victim); v.state != "sent" {
		t.Fatalf("the address's owner from their own network after another network's 8 tries: %+v", v)
	}
	time.Sleep(100 * time.Millisecond)
	var msgs []mail.Message
	seen := map[string]bool{}
	for _, a := range spellings {
		if a = strings.ToLower(a); !seen[a] {
			seen[a] = true
			msgs = append(msgs, h.mail.to(a)...)
		}
	}
	if len(msgs) != 4 {
		t.Fatalf("%d confirmation mails reached one mailbox, want 3 from one network and 1 from another", len(msgs))
	}
	for _, m := range msgs {
		if strings.Contains(m.Subject, "\n") || strings.Contains(m.Text, "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx") || strings.Contains(m.HTML, "\n\nat evil") {
			t.Fatalf("the inbox name is not limited to one short line: %q\n%s", m.Subject, m.Text)
		}
		if !strings.Contains(m.Subject, "Claim your refund at evil.example x") || !strings.Contains(m.Subject, "…") {
			t.Fatalf("subject %q", m.Subject)
		}
	}

	attacker := newChatTeam(t, h, "live", true)
	for i := range 30 {
		if v := typeFrom(t, h, attacker, "10.33.1."+strconv.Itoa(i+1), unique("n")+"@example.net"); v.state != "sent" {
			t.Fatalf("visitor %d of one network in a workspace: %+v", i+1, v)
		}
	}
	for i := range 5 {
		if v := typeFrom(t, h, attacker, "10.33.1."+strconv.Itoa(100+i), victim); v.state != "limited" {
			t.Fatalf("past the network's 30 in the workspace: %+v", v)
		}
	}
	other := newChatTeam(t, h, "live", true)
	for n := range 2 {
		for i := range 3 {
			if v := typeFrom(t, h, other, "10.34."+strconv.Itoa(n)+"."+strconv.Itoa(i+1), victim); v.state != "sent" {
				t.Fatalf("network %d try %d: %+v (refused tries must not spend the address's budget)", n, i+1, v)
			}
		}
	}
	if v := typeFrom(t, h, other, "10.35.0.1", victim); v.state != "limited" {
		t.Fatalf("an 11th mail to one address in a day: %+v", v)
	}
}

func TestTypedEmailWorkspaceCap(t *testing.T) {
	defer api.SetTypedConfirmWorkspaceLimit(50)()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	sent := 0
	for i := range 100 {
		ch := ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
			"kind": "chat", "name": "Site " + strconv.Itoa(i),
			"chat": map[string]any{"allowed_origins": []string{ct.origin + "/"}, "allow_anonymous": true},
		})
		c := ct
		c.key, c.channel = ch.body["chat"].(map[string]any)["public_key"].(string), ch.str("id")
		if v := typeFrom(t, h, c, "10.40."+strconv.Itoa(i)+".1", unique("w")+"@example.net"); v.state == "sent" {
			sent++
		} else if v.state != "limited" {
			t.Fatalf("channel %d: %+v", i+1, v)
		}
	}
	if sent != 50 {
		t.Fatalf("%d confirmation mails from 100 channels of one workspace, want the workspace's 50", sent)
	}
}

func TestConfirmAddressKey(t *testing.T) {
	for _, c := range [][2]string{
		{"Jane.Doe+news@GoogleMail.com", "janedoe@gmail.com"},
		{"jane@bücher.de", "jane@xn--bcher-kva.de"},
		{"Jane@XN--BCHER-KVA.DE", "jane@xn--bcher-kva.de"},
		{"jane.doe+x@example.com", "jane.doe@example.com"},
	} {
		if got := api.ConfirmAddressKey(c[0]); got != c[1] {
			t.Fatalf("%s -> %s, want %s", c[0], got, c[1])
		}
	}
}

func TestIdentityTokenUnverifiedEmailGetsOneLink(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	addr := unique("unverified") + "@example.net"
	sub := unique("user")
	start := func() contactSession {
		return ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": sub, "email": addr})})
	}
	first := start()
	if v := confirmationOf(t, first.body["contact"].(map[string]any)); v.email != addr || v.state != "sent" {
		t.Fatalf("first session: %+v", v)
	}
	h.mail.wait(t, addr, 1)
	start()
	time.Sleep(100 * time.Millisecond)
	if n := len(h.mail.to(addr)); n != 1 {
		t.Fatalf("%d confirmation mails for one unverified token address, want 1", n)
	}
	verified := unique("verified") + "@example.net"
	cs := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": unique("user"), "email": verified, "email_verified": true})})
	time.Sleep(100 * time.Millisecond)
	if n := len(h.mail.to(verified)); n != 0 || cs.body["contact"].(map[string]any)["email"] != verified {
		t.Fatalf("a verified token address: %d mails, contact %v", n, cs.body["contact"])
	}
}

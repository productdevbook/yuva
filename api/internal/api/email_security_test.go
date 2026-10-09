package api_test

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/email"
)

func undeliverable(c *client, contactID string) []any {
	und, _ := c.expect(http.StatusOK, "GET", "/v1/contacts/"+contactID, nil).body["undeliverable"].([]any)
	return und
}

func TestThirdPartyCannotHijackReplies(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	customer, colleague := unique("customer")+"@example.net", unique("colleague")+"@example.org"
	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{
		from: customer, to: et.address, subject: "Invoice", messageID: first, body: "Where is my invoice?",
		headers: map[string]string{"Cc": "Colleague <" + colleague + ">"},
	}), nil)
	conv := r.str("conversation_id")
	in := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)
	if cc, _ := in["cc"].([]any); len(cc) != 1 || cc[0] != colleague {
		t.Fatalf("Cc not recorded: %v", in)
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Here it is."})
	h.sendQueued(t, et.ws, reply.str("id"))
	sent, parsed := h.smtp.last(t)
	if len(sent.to) != 1 || sent.to[0] != customer || parsed.Header.Get("Cc") != "" {
		t.Fatalf("first reply envelope %v, Cc %q", sent.to, parsed.Header.Get("Cc"))
	}
	ourID := email.NormalizeID(parsed.Header.Get("Message-ID"))
	token, _, _ := email.TokenFromID(ourID)
	customerContact := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
	before := len(messages(et.owner, conv))

	for _, attack := range []map[string]string{
		{"In-Reply-To": "<" + ourID + ">", "References": "<" + first + "> <" + ourID + ">"},
		{"In-Reply-To": "<" + strings.ToUpper(token) + ".AAAAAAAAAAAAAAAA@RELAY.EXAMPLE.ORG>"},
	} {
		fwd := h.ingest(et.address, buildMail(mailOpts{
			from: colleague, to: et.address, subject: "Fwd: Re: Invoice", messageID: newMessageID(), headers: attack,
			body: "Please send it to me instead.",
		}), nil)
		if fwd.status != http.StatusAccepted || fwd.str("status") != "stored" || fwd.str("conversation_id") == conv {
			t.Fatalf("forward joined the customer's conversation: %d %v", fwd.status, fwd.body)
		}
		side := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+fwd.str("conversation_id"), nil)
		if side.str("related_conversation_id") != conv || side.str("contact_id") == customerContact {
			t.Fatalf("new conversation: %s", side.raw)
		}
	}
	if n := len(messages(et.owner, conv)); n != before {
		t.Fatalf("customer's conversation got %d new messages", n-before)
	}

	again := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Did it arrive?"})
	h.sendQueued(t, et.ws, again.str("id"))
	sent, parsed = h.smtp.last(t)
	if len(sent.to) != 1 || sent.to[0] != customer || !strings.Contains(parsed.Header.Get("To"), customer) || parsed.Header.Get("Cc") != "" {
		t.Fatalf("member reply went to %v (To %q, Cc %q), want only %s", sent.to, parsed.Header.Get("To"), parsed.Header.Get("Cc"), customer)
	}
	if em := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any); em["to"].([]any)[0] != customer {
		t.Fatalf("stored recipient: %v", em)
	}

	sameContact := h.ingest(et.address, buildMail(mailOpts{
		from: customer, to: et.address, subject: "Re: Invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"}, body: "Got it, thanks.",
	}), nil)
	if sameContact.str("conversation_id") != conv {
		t.Fatalf("the contact's own reply did not thread: %v", sameContact.body)
	}

	other := newEmailTeam(t, h, false)
	cross := h.ingest(other.address, buildMail(mailOpts{
		from: customer, to: other.address, subject: "Re: Invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"}, body: "Wrong inbox.",
	}), nil)
	side := other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+cross.str("conversation_id"), nil)
	if _, ok := side.body["related_conversation_id"]; ok {
		t.Fatalf("a conversation of another workspace was referenced: %s", side.raw)
	}
}

func TestForgedDeliveryReports(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	victim := unique("victim") + "@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: victim, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	contactID := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	evil := unique("evil") + "@example.org"
	forged := strings.Replace(string(dsnMail(et.address, newMessageID(), victim, "5.1.1")),
		"From: Mail Delivery System <mailer-daemon@mx.example.net>", "From: <"+evil+">", 1)
	f := h.ingest(et.address, []byte(forged), nil)
	if f.status != http.StatusAccepted || f.str("status") != "stored" || f.str("conversation_id") == conv {
		t.Fatalf("forged report: %d %v", f.status, f.body)
	}
	if und := undeliverable(et.owner, contactID); len(und) != 0 {
		t.Fatalf("a forged report marked %s undeliverable: %v", victim, und)
	}
	if m := lastOf(messages(et.owner, f.str("conversation_id")), "message"); m["email"].(map[string]any)["auto"] != true {
		t.Fatalf("forged report stored as a normal mail: %v", m)
	}

	otherRcpt := unique("other") + "@example.net"
	o := h.ingest(et.address, buildMail(mailOpts{from: otherRcpt, to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hi"}), nil)
	otherContact := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+o.str("conversation_id"), nil).str("contact_id")
	wrongRcpt := h.ingestFrom(et.address, "", dsnMail(et.address, ourID, otherRcpt, "5.1.1"), nil)
	if wrongRcpt.str("status") == "bounce" || len(undeliverable(et.owner, otherContact)) != 0 {
		t.Fatalf("a report for someone who was not a recipient counted: %v", wrongRcpt.body)
	}
	if d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any); d["state"] != "sent" {
		t.Fatalf("a report for another recipient failed the message: %v", d)
	}

	foreign := newEmailTeam(t, h, false)
	cross := h.ingest(foreign.address, dsnMail(foreign.address, ourID, victim, "5.1.1"), nil)
	if cross.str("status") == "bounce" || len(undeliverable(et.owner, contactID)) != 0 {
		t.Fatalf("a report to another workspace counted: %v", cross.body)
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+otherContact, map[string]any{"blocked": true})
	blockedReport := strings.Replace(string(dsnMail(et.address, ourID, victim, "5.1.1")),
		"From: Mail Delivery System <mailer-daemon@mx.example.net>", "From: <"+otherRcpt+">", 1)
	if b := h.ingest(et.address, []byte(blockedReport), nil); b.status != http.StatusForbidden || b.str("code") != "blocked_sender" {
		t.Fatalf("report from a blocked sender: %d %v", b.status, b.body)
	}
	if len(undeliverable(et.owner, contactID)) != 0 {
		t.Fatal("a blocked sender's report counted")
	}

	third := strings.Replace(string(dsnMail(et.address, ourID, victim, "5.1.1")),
		"From: Mail Delivery System <mailer-daemon@mx.example.net>", "From: <third@example.com>", 1)
	sent := h.ingestFrom(et.address, "third@example.com", []byte(third), nil)
	if sent.status != http.StatusAccepted || sent.str("status") != "stored" || len(undeliverable(et.owner, contactID)) != 0 {
		t.Fatalf("a report with a sender counted: %d %v", sent.status, sent.body)
	}
	for _, m := range messages(et.owner, sent.str("conversation_id")) {
		if m["direction"] == "out" {
			t.Fatal("a report with a sender got an automatic reply")
		}
	}

	genuine := h.ingestFrom(et.address, "", []byte(third), nil)
	if genuine.str("status") != "bounce" || len(undeliverable(et.owner, contactID)) != 1 {
		t.Fatalf("genuine report: %v", genuine.body)
	}
}

func TestSESOnlyForOurRecipients(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	signer := newSNSSigner(t, h.web)
	rcpt, bystander := unique("rcpt")+"@example.net", unique("bystander")+"@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: rcpt, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	b := h.ingest(et.address, buildMail(mailOpts{from: bystander, to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hi"}), nil)
	bystanderContact := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+b.str("conversation_id"), nil).str("contact_id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	fixture, err := os.ReadFile("../email/testdata/ses_bounce.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, rcpt string }{{ourID, bystander}, {newMessageID(), rcpt}} {
		payload := strings.NewReplacer("{{MESSAGE_ID}}", c.id, "{{RECIPIENT}}", c.rcpt).Replace(string(fixture))
		note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: payload, Timestamp: time.Now().UTC().Format(time.RFC3339)}
		if st := h.postSNS(signer.sign(t, note)); st != http.StatusOK {
			t.Fatalf("notification: %d", st)
		}
	}
	if len(undeliverable(et.owner, bystanderContact)) != 0 {
		t.Fatal("an SES bounce for someone who was not a recipient counted")
	}
	if d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any); d["state"] != "sent" {
		t.Fatalf("an SES bounce that is not ours failed the message: %v", d)
	}
}

const inlineImageMail = "From: Customer <%[1]s>\r\nTo: %[2]s\r\nSubject: Screenshot\r\nMessage-ID: <%[3]s>\r\n" +
	"MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n" +
	"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
	`<p>See <img src="cid:shot1@client.example.net" alt="shot"> and <img src="https://tracker.example.com/p.gif"></p>` + "\r\n" +
	"--rel\r\nContent-Type: image/png\r\nContent-ID: <shot1@client.example.net>\r\nContent-Disposition: inline; filename=\"shot.png\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n" +
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==\r\n" +
	"--rel--\r\n"

func TestInlineImagesAndRemoteImageFlag(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	r := h.ingest(et.address, []byte(fmt.Sprintf(inlineImageMail, unique("pics")+"@example.net", et.address, newMessageID())), nil)
	if r.str("status") != "stored" {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
	html, _ := m["html"].(string)
	if !strings.Contains(html, `src="cid:shot1@client.example.net"`) || !strings.Contains(html, "tracker.example.com") {
		t.Fatalf("html: %q", html)
	}
	if m["email"].(map[string]any)["has_remote_images"] != true {
		t.Fatalf("remote images not flagged: %v", m["email"])
	}
	atts := m["attachments"].([]any)
	if len(atts) != 1 || atts[0].(map[string]any)["content_id"] != "shot1@client.example.net" || atts[0].(map[string]any)["inline"] != true {
		t.Fatalf("attachments: %v", atts)
	}
	detail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+r.str("message_id")+"/email", nil)
	if detail.body["has_remote_images"] != true {
		t.Fatalf("detail: %s", detail.raw)
	}

	plain := h.ingest(et.address, buildMail(mailOpts{from: unique("plain") + "@example.net", to: et.address, subject: "x", messageID: newMessageID(), body: "x"}), nil)
	pm := lastOf(messages(et.owner, plain.str("conversation_id")), "message")
	if pm["email"].(map[string]any)["has_remote_images"] != false || pm["attachments"] == nil {
		t.Fatalf("plain mail: %v", pm)
	}
}

const styledMailHTML = `<html><head><style>body { color: red } @import url(https://evil.example.com/x.css);</style>` +
	`<link rel="stylesheet" href="https://evil.example.com/y.css"></head><body>` +
	`<center><table bgcolor="#ffeecc" width="600" border="0" cellpadding="4" cellspacing="0" style="border-collapse: collapse; background: url(https://tracker.example.com/bg.png)">` +
	`<tr><td align="center" valign="top" style="color: #333333; font-family: Arial, sans-serif; padding: 8px 12px; position: absolute; background-image: url(https://tracker.example.com/td.png)">` +
	`<font color="#0066cc" face="Arial">Your order shipped</font></td></tr></table></center>` +
	`<p style="color: expression(alert(1)); margin: 0 0 12px 0">Track it below.</p>` +
	`<a href="javascript:alert(1)" style="color: #0066cc; text-decoration: underline">Track</a>` +
	`<div onclick="steal()" style="width: 100px; behavior: url(x.htc)">Box</div>` +
	`<img src="cid:logo@client.example.net" alt="logo">` +
	`<script>alert(1)</script><form action="https://evil.example.com"><input name="pw"></form>` +
	`<iframe src="https://evil.example.com"></iframe>` +
	`<blockquote type="cite">Earlier mail</blockquote></body></html>`

func TestOriginalHTMLKeepsAllowedStyles(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	raw := fmt.Sprintf("From: Shop <%s>\r\nTo: %s\r\nSubject: Shipped\r\nMessage-ID: <%s>\r\n"+
		"MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=alt\r\n\r\n"+
		"--alt\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nYour order shipped\r\n"+
		"--alt\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--alt--\r\n",
		unique("shop")+"@example.net", et.address, newMessageID(), styledMailHTML)
	r := h.ingest(et.address, []byte(raw), nil)
	if r.str("status") != "stored" {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
	html, _ := m["html"].(string)
	if !strings.Contains(html, "Your order shipped") || !strings.Contains(html, `src="cid:logo@client.example.net"`) ||
		strings.Contains(html, "style=") || strings.Contains(html, "bgcolor") || strings.Contains(html, "<center>") ||
		strings.Contains(html, "Earlier mail") {
		t.Fatalf("html changed: %q", html)
	}

	detail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+r.str("message_id")+"/email", nil)
	if full := detail.str("full_html"); strings.Contains(full, "style=") || strings.Contains(full, "bgcolor") {
		t.Fatalf("full_html keeps styling: %q", full)
	}
	orig := detail.str("original_html")
	for _, want := range []string{
		`<center>`, `bgcolor="#ffeecc"`, `width="600"`, `cellpadding="4"`, `cellspacing="0"`, `border="0"`,
		`style="border-collapse: collapse"`, `align="center"`, `valign="top"`,
		`style="color: #333333; font-family: Arial, sans-serif; padding: 8px 12px"`,
		`<font color="#0066cc" face="Arial">`, `style="margin: 0 0 12px 0"`,
		`style="color: #0066cc; text-decoration: underline"`, `style="width: 100px"`,
		`src="cid:logo@client.example.net"`, `Earlier mail`,
	} {
		if !strings.Contains(orig, want) {
			t.Errorf("original_html lacks %s: %q", want, orig)
		}
	}
	for _, refused := range []string{
		"<style", "@import", "evil.example.com", "url(", "expression", "position", "behavior",
		"javascript:", "onclick", "<script", "alert", "<form", "<input", "<iframe", "<link",
	} {
		if strings.Contains(orig, refused) {
			t.Errorf("original_html keeps %s: %q", refused, orig)
		}
	}

	plain := h.ingest(et.address, buildMail(mailOpts{from: unique("plain") + "@example.net", to: et.address, subject: "x", messageID: newMessageID(), body: "x"}), nil)
	pd := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+plain.str("message_id")+"/email", nil)
	if _, ok := pd.body["original_html"]; ok {
		t.Fatalf("plain mail has original_html: %s", pd.raw)
	}
}

package api_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/email"
)

type sentMail struct {
	cfg  email.SMTPConfig
	from string
	to   []string
	raw  []byte
}

type smtpCapture struct {
	mu   sync.Mutex
	sent []sentMail
	fail error
}

func (c *smtpCapture) Send(_ context.Context, cfg email.SMTPConfig, from string, to []string, msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.sent = append(c.sent, sentMail{cfg: cfg, from: from, to: to, raw: msg})
	return nil
}

func (c *smtpCapture) last(t *testing.T) (sentMail, *mail.Message) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) == 0 {
		t.Fatal("nothing was sent")
	}
	m := c.sent[len(c.sent)-1]
	parsed, err := mail.ReadMessage(bytes.NewReader(m.raw))
	if err != nil {
		t.Fatal(err)
	}
	return m, parsed
}

func (c *smtpCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sent)
}

type fakeWeb struct {
	mu       sync.Mutex
	pages    map[string][]byte
	requests []string
}

func newFakeWeb() *fakeWeb { return &fakeWeb{pages: map[string][]byte{}} }

func (f *fakeWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.String())
	body, ok := f.pages[r.URL.String()]
	status := http.StatusOK
	if !ok {
		status = http.StatusNotFound
	}
	return &http.Response{StatusCode: status, Status: strconv.Itoa(status), Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func (f *fakeWeb) fetched(url string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r == url {
			return true
		}
	}
	return false
}

type emailTeam struct {
	team
	channel string
	address string
}

func newEmailTeam(t *testing.T, h *harness, autoReply bool) emailTeam {
	t.Helper()
	tm := newTeam(t, h)
	addr := unique("support") + "@example.com"
	settings := map[string]any{
		"address": strings.ToUpper(addr), "display_name": "Acme Support",
		"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
	}
	if autoReply {
		settings["auto_reply"] = map[string]any{"enabled": true, "text": "Thanks, we got your message.", "interval_hours": 24}
	}
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Support mail", "email": settings,
	})
	return emailTeam{team: tm, channel: ch.str("id"), address: addr}
}

type ingressResult struct {
	status int
	body   map[string]any
}

func (r ingressResult) str(k string) string { s, _ := r.body[k].(string); return s }

func (h *harness) ingest(to string, raw []byte, mutate func(*http.Request)) ingressResult {
	h.t.Helper()
	ts := strconv.FormatInt(h.clock.Now().Unix(), 10)
	req, _ := http.NewRequest("POST", h.url+"/ingress/email", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "message/rfc822")
	req.Header.Set("X-Yuva-Envelope-To", to)
	req.Header.Set("X-Yuva-Envelope-From", "bounce@example.net")
	req.Header.Set("X-Yuva-Timestamp", ts)
	req.Header.Set("X-Yuva-Signature", email.Sign(testIngressSecret, ts, to, raw))
	if mutate != nil {
		mutate(req)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer res.Body.Close()
	out := ingressResult{status: res.StatusCode}
	b, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(b, &out.body)
	if ct := res.Header.Get("Content-Type"); res.StatusCode >= 400 && res.StatusCode < 500 && ct != "application/json" {
		h.t.Fatalf("refusal content type %q: %s", ct, b)
	}
	return out
}

type mailOpts struct {
	from, to, subject, messageID, body string
	headers                            map[string]string
}

func buildMail(o mailOpts) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: Customer <%s>\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\n",
		o.from, o.to, o.subject, o.messageID, time.Now().Format(time.RFC1123Z))
	for k, v := range o.headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(strings.ReplaceAll(o.body, "\n", "\r\n"))
	return []byte(b.String())
}

func newMessageID() string { return unique("msg") + "@client.example.net" }

func (h *harness) sendQueued(t *testing.T, ws, message string) {
	t.Helper()
	if err := h.srv.SendEmail(context.Background(), uuid.MustParse(ws), uuid.MustParse(message), false); err != nil {
		t.Fatal(err)
	}
}

func lastOf(items []map[string]any, kind string) map[string]any {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i]["kind"] == kind {
			return items[i]
		}
	}
	return nil
}

func TestEmailChannelSettings(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	o := et.owner
	ch := o.expect(http.StatusOK, "GET", "/v1/channels/"+et.channel, nil)
	if bytes.Contains(ch.raw, []byte("smtp-secret")) {
		t.Fatal("channel body shows the SMTP password")
	}
	em := ch.body["email"].(map[string]any)
	smtp := em["smtp"].(map[string]any)
	if em["address"] != et.address || smtp["password_set"] != true || smtp["port"] != float64(2525) {
		t.Fatalf("email settings: %s", ch.raw)
	}
	var sealed []byte
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT smtp_password FROM email_channels WHERE channel_id = $1", et.channel).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("smtp-secret")) {
		t.Fatal("SMTP password stored in the clear")
	}

	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{"kind": "email", "name": "x"})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "chat", "name": "x", "email": map[string]any{"address": "a@example.com"},
	})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "email", "name": "x", "email": map[string]any{"address": "b@example.com", "smtp": map[string]any{"host": "bad host"}},
	})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "email", "name": "x", "email": map[string]any{"address": "c@example.com", "auto_reply": map[string]any{"enabled": true}},
	})
	o.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/inboxes/"+et.inbox+"/channels", map[string]any{
		"kind": "email", "name": "x", "email": map[string]any{"address": "d@example.com", "smtp": map[string]any{"host": "relay.example.com", "tls": "none", "username": "u"}},
	})
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusConflict, "email_address_taken", "POST", "/v1/inboxes/"+other.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Taken", "email": map[string]any{"address": strings.ToUpper(et.address[:1]) + et.address[1:]},
	})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/channels/"+et.channel, nil)

	up := o.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"email": map[string]any{
		"address": et.address, "display_name": "Help", "smtp": map[string]any{"host": "smtp2.example.com", "tls": "tls"},
	}})
	smtp = up.body["email"].(map[string]any)["smtp"].(map[string]any)
	if smtp["password_set"] != true || smtp["port"] != float64(465) || smtp["host"] != "smtp2.example.com" {
		t.Fatalf("update kept the wrong settings: %s", up.raw)
	}
	up = o.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"email": map[string]any{
		"address": et.address, "smtp": map[string]any{"host": "smtp2.example.com", "password": ""},
	}})
	if up.body["email"].(map[string]any)["smtp"].(map[string]any)["password_set"] != false {
		t.Fatalf("empty password did not remove it: %s", up.raw)
	}
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/channels/"+et.channel, map[string]any{"name": "x"})
}

func TestIngressSignatureAndRefusals(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	raw := buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})

	r := h.ingest(et.address, raw, func(req *http.Request) { req.Header.Set("X-Yuva-Signature", "v1="+strings.Repeat("0", 64)) })
	if r.status != http.StatusUnauthorized || r.str("code") != "bad_signature" || r.str("reason") == "" {
		t.Fatalf("bad signature: %d %v", r.status, r.body)
	}
	r = h.ingest(et.address, raw, func(req *http.Request) {
		req.Header.Set("X-Yuva-Signature", strings.Replace(req.Header.Get("X-Yuva-Signature"), "v1=", "v2=", 1))
	})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("v2 prefix: %d", r.status)
	}
	r = h.ingest(et.address, raw, func(req *http.Request) { req.Header.Set("X-Yuva-Envelope-To", "other@example.com") })
	if r.status != http.StatusUnauthorized {
		t.Fatalf("signature not bound to the recipient: %d", r.status)
	}
	stale := strconv.FormatInt(h.clock.Now().Add(-6*time.Minute).Unix(), 10)
	r = h.ingest(et.address, raw, func(req *http.Request) {
		req.Header.Set("X-Yuva-Timestamp", stale)
		req.Header.Set("X-Yuva-Signature", email.Sign(testIngressSecret, stale, et.address, raw))
	})
	if r.status != http.StatusUnauthorized || r.str("code") != "stale_timestamp" {
		t.Fatalf("stale timestamp: %d %v", r.status, r.body)
	}
	unknown := unique("nobody") + "@example.com"
	if r = h.ingest(unknown, raw, nil); r.status != http.StatusNotFound || r.str("code") != "unknown_recipient" {
		t.Fatalf("unknown recipient: %d %v", r.status, r.body)
	}
	big := append(bytes.Clone(raw), bytes.Repeat([]byte("a"), 25<<20)...)
	if r = h.ingest(et.address, big, nil); r.status != http.StatusRequestEntityTooLarge || r.str("code") != "too_large" {
		t.Fatalf("too large: %d %v", r.status, r.body)
	}

	tagged := strings.Replace(et.address, "@", "+billing@", 1)
	if r = h.ingest(tagged, raw, nil); r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("plus address: %d %v", r.status, r.body)
	}

	blocked := buildMail(mailOpts{from: "ayse@example.com", to: et.address, subject: "Hi", messageID: newMessageID(), body: "x"})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+et.contact, map[string]any{"blocked": true})
	if r = h.ingest(et.address, blocked, nil); r.status != http.StatusForbidden || r.str("code") != "blocked_sender" {
		t.Fatalf("blocked: %d %v", r.status, r.body)
	}
}

const attachmentMail = "From: =?UTF-8?Q?Ay=C5=9Fe?= <newcust@example.net>\r\n" +
	"To: %s\r\nSubject: Broken export\r\nMessage-ID: <%s>\r\n" +
	"Authentication-Results: mx.example.com; spf=pass; dkim=pass; dmarc=pass header.from=example.net\r\n" +
	"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b1\r\n\r\n" +
	"--b1\r\nContent-Type: multipart/alternative; boundary=b2\r\n\r\n" +
	"--b2\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThe export is empty.\r\n\r\n-- \r\nAyse\r\n" +
	"--b2\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p onclick=\"x()\">The export is <b>empty</b>.</p><script>alert(1)</script>\r\n" +
	"--b2--\r\n" +
	"--b1\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=\"log.txt\"\r\n\r\nline one\r\n" +
	"--b1\r\nContent-Type: application/x-msdownload\r\nContent-Disposition: attachment; filename=\"evil.exe\"\r\n\r\nMZ\r\n" +
	"--b1--\r\n"

func TestInboundMailOpensConversation(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	id := newMessageID()
	raw := []byte(fmt.Sprintf(attachmentMail, et.address, id))
	r := h.ingest(et.address, raw, nil)
	if r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv, msgID := r.str("conversation_id"), r.str("message_id")
	c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if c.str("subject") != "Broken export" || c.str("channel_id") != et.channel || c.body["spam"] != false {
		t.Fatalf("conversation: %s", c.raw)
	}
	contact := et.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+c.str("contact_id"), nil)
	if contact.str("name") != "Ayşe" || contact.body["emails"].([]any)[0] != "newcust@example.net" {
		t.Fatalf("contact: %s", contact.raw)
	}
	msgs := messages(et.owner, conv)
	m := lastOf(msgs, "message")
	if m["body"] != "The export is empty." || m["direction"] != "in" {
		t.Fatalf("message body: %v", m)
	}
	if html, _ := m["html"].(string); strings.Contains(html, "script") || strings.Contains(html, "onclick") || !strings.Contains(html, "<b>empty</b>") {
		t.Fatalf("html not sanitized: %q", html)
	}
	em := m["email"].(map[string]any)
	if em["message_id"] != id || em["quoted"] != true || em["raw"] != true || em["dmarc"] != "pass" || em["auto"] != false {
		t.Fatalf("email summary: %v", em)
	}
	atts := m["attachments"].([]any)
	if len(atts) != 1 || atts[0].(map[string]any)["filename"] != "log.txt" {
		t.Fatalf("attachments: %v", atts)
	}
	detail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+msgID+"/email", nil)
	if !strings.Contains(detail.str("full_text"), "Ayse") || !strings.Contains(detail.str("authentication_results"), "dmarc=pass") {
		t.Fatalf("detail: %s", detail.raw)
	}
	rawRes := et.owner.do("GET", "/v1/messages/"+msgID+"/raw", nil)
	if rawRes.status != http.StatusOK || !bytes.Equal(rawRes.raw, raw) || rawRes.header.Get("Content-Type") != "message/rfc822" {
		t.Fatalf("raw download: %d %q", rawRes.status, rawRes.header.Get("Content-Type"))
	}
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+msgID+"/raw", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+msgID+"/email", nil)
	et.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+msgID+"/raw", nil)

	again := h.ingest(et.address, raw, nil)
	if again.status != http.StatusOK || again.str("status") != "duplicate" || again.str("message_id") != msgID {
		t.Fatalf("duplicate: %d %v", again.status, again.body)
	}
	if n := len(messages(et.owner, conv)); n != len(msgs) {
		t.Fatalf("duplicate stored again: %d messages, want %d", n, len(msgs))
	}
}

func TestReplyThreading(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	first := newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{from: "thread@example.net", to: et.address, subject: "Invoice", messageID: first, body: "Where is my invoice?"}), nil)
	conv := r.str("conversation_id")

	note := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal"})
	if note.body["delivery"] != nil || note.body["email"] != nil {
		t.Fatalf("note queued for e-mail: %s", note.raw)
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Here it is.", "html": "<p>Here it is.</p>",
	})
	delivery := reply.body["delivery"].(map[string]any)
	if delivery["state"] != "queued" {
		t.Fatalf("reply not queued: %s", reply.raw)
	}
	before := h.smtp.count()
	h.sendQueued(t, et.ws, reply.str("id"))
	if h.smtp.count() != before+1 {
		t.Fatal("note or reply count wrong")
	}
	sent, parsed := h.smtp.last(t)
	if sent.from != et.address || len(sent.to) != 1 || sent.to[0] != "thread@example.net" || sent.cfg.Host != "smtp.example.com" || sent.cfg.Password != "smtp-secret" {
		t.Fatalf("envelope: %+v", sent)
	}
	hdr := parsed.Header
	ourID := email.NormalizeID(hdr.Get("Message-ID"))
	token, domain, ok := email.TokenFromID(ourID)
	if !ok || domain != email.Domain(et.address) {
		t.Fatalf("Message-ID %q is not ours", ourID)
	}
	checks := map[string]string{
		"In-Reply-To": "<" + first + ">",
		"References":  "<" + first + ">",
		"Subject":     "Re: Invoice",
		"From":        `"Acme Support" <` + et.address + ">",
		"Reply-To":    `"Acme Support" <` + et.address + ">",
	}
	for k, want := range checks {
		if got := hdr.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if hdr.Get("Auto-Submitted") != "" {
		t.Error("a member reply is marked Auto-Submitted")
	}
	if !strings.HasPrefix(hdr.Get("Content-Type"), "multipart/alternative") {
		t.Errorf("Content-Type %q", hdr.Get("Content-Type"))
	}
	msgs := messages(et.owner, conv)
	sentMsg := lastOf(msgs, "message")
	if sentMsg["delivery"].(map[string]any)["state"] != "sent" || sentMsg["email"].(map[string]any)["message_id"] != ourID {
		t.Fatalf("after send: %v", sentMsg)
	}

	viaInReplyTo := h.ingest(et.address, buildMail(mailOpts{
		from: "thread@example.net", to: et.address, subject: "Re: Invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"},
		body:    "Thanks!\n\nOn Tue, Oct 6, 2026 at 4:02 PM Acme Support <" + et.address + "> wrote:\n> Here it is.\n",
	}), nil)
	if viaInReplyTo.str("conversation_id") != conv {
		t.Fatalf("In-Reply-To did not thread: %v", viaInReplyTo.body)
	}
	last := lastOf(messages(et.owner, conv), "message")
	if last["body"] != "Thanks!" || last["email"].(map[string]any)["quoted"] != true {
		t.Fatalf("quotes not stripped: %v", last)
	}

	viaReferences := h.ingest(et.address, buildMail(mailOpts{
		from: "thread@example.net", to: et.address, subject: "Re: Invoice", messageID: newMessageID(),
		headers: map[string]string{"References": "<unrelated@example.org> <" + first + ">"}, body: "Via references.",
	}), nil)
	if viaReferences.str("conversation_id") != conv {
		t.Fatalf("References did not thread: %v", viaReferences.body)
	}

	mangled := strings.ToUpper(token) + ".AAAAAAAAAAAAAAAA@RELAY.EXAMPLE.ORG"
	viaToken := h.ingest(et.address, buildMail(mailOpts{
		from: "thread@example.net", to: et.address, subject: "Invoice (fwd)", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + mangled + ">"}, body: "Via token.",
	}), nil)
	if viaToken.str("conversation_id") != conv {
		t.Fatalf("token did not thread: %v", viaToken.body)
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	reopened := h.ingest(et.address, buildMail(mailOpts{
		from: "thread@example.net", to: et.address, subject: "Re: Invoice", messageID: newMessageID(),
		headers: map[string]string{"In-Reply-To": "<" + ourID + ">"}, body: "One more thing.",
	}), nil)
	if reopened.str("conversation_id") != conv || et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status") != "open" {
		t.Fatalf("closed conversation not reopened: %v", reopened.body)
	}

	fresh := h.ingest(et.address, buildMail(mailOpts{from: "thread@example.net", to: et.address, subject: "Another topic", messageID: newMessageID(), body: "New."}), nil)
	if fresh.str("conversation_id") == conv {
		t.Fatal("unrelated mail joined the conversation")
	}

	second := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Second reply."})
	h.sendQueued(t, et.ws, second.str("id"))
	_, parsed = h.smtp.last(t)
	refs := email.ParseIDList(parsed.Header.Get("References"))
	if len(refs) != 2 || refs[0] != ourID || "<"+refs[1]+">" != parsed.Header.Get("In-Reply-To") || parsed.Header.Get("Subject") != "Re: Invoice" {
		t.Fatalf("second reply References %v Subject %q", refs, parsed.Header.Get("Subject"))
	}
}

func TestOutboundFailuresAndRefusals(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	r := h.ingest(et.address, buildMail(mailOpts{from: "fail@example.net", to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")

	h.smtp.fail = &email.PermanentError{Err: &textproto.Error{Code: 550, Msg: "mailbox unavailable"}}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	h.smtp.fail = nil
	m := lastOf(messages(et.owner, conv), "message")
	d := m["delivery"].(map[string]any)
	if d["state"] != "failed" || !strings.Contains(d["error"].(string), "550") {
		t.Fatalf("permanent failure: %v", d)
	}

	h.smtp.fail = errors.New("connection refused")
	retry := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "B."})
	if err := h.srv.SendEmail(context.Background(), uuid.MustParse(et.ws), uuid.MustParse(retry.str("id")), false); err == nil {
		t.Fatal("a temporary failure was not returned for a retry")
	}
	if err := h.srv.SendEmail(context.Background(), uuid.MustParse(et.ws), uuid.MustParse(retry.str("id")), true); err != nil {
		t.Fatal(err)
	}
	h.smtp.fail = nil
	if d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any); d["state"] != "failed" {
		t.Fatalf("last attempt not failed: %v", d)
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+et.channel, map[string]any{"email": map[string]any{"address": et.address}})
	et.owner.expectProblem(http.StatusConflict, "email_not_configured", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "C."})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "notes still work"})

	chat := et.conversation(et.owner)
	plain := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+chat+"/messages", map[string]any{"kind": "message", "body": "not mail"})
	if plain.body["delivery"] != nil {
		t.Fatalf("a conversation without an e-mail channel was queued: %s", plain.raw)
	}
}

func TestLoopProtection(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, true)
	autoReplies := func(conv string) int {
		n := 0
		for _, m := range messages(et.owner, conv) {
			if m["kind"] == "message" && m["direction"] == "out" {
				if e, _ := m["email"].(map[string]any); e != nil && e["auto"] == true {
					n++
				}
			}
		}
		return n
	}
	cases := map[string]map[string]string{
		"auto-submitted": {"Auto-Submitted": "auto-replied"},
		"precedence":     {"Precedence": "bulk"},
		"junk":           {"Precedence": "junk"},
		"list":           {"Precedence": "list"},
		"x-autoreply":    {"X-Autoreply": "yes"},
		"suppress":       {"X-Auto-Response-Suppress": "All"},
		"own-id":         {"Message-ID": "<abcdefghijklmnopqrstuvwx.abcdefghijklmnop@" + email.Domain(et.address) + ">"},
	}
	for name, hdrs := range cases {
		id := newMessageID()
		if v, ok := hdrs["Message-ID"]; ok {
			id = email.NormalizeID(v)
			delete(hdrs, "Message-ID")
		}
		r := h.ingest(et.address, buildMail(mailOpts{
			from: unique("auto") + "@example.net", to: et.address, subject: "Out of office", messageID: id, headers: hdrs, body: "I am away.",
		}), nil)
		if r.status != http.StatusAccepted {
			t.Fatalf("%s: %d %v", name, r.status, r.body)
		}
		if n := autoReplies(r.str("conversation_id")); n != 0 {
			t.Errorf("%s: %d auto-replies", name, n)
		}
	}

	sender := unique("human") + "@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: sender, to: et.address, subject: "Hello", messageID: newMessageID(), body: "Hi"}), nil)
	conv := r.str("conversation_id")
	if autoReplies(conv) != 1 {
		t.Fatal("no greeting for a new conversation")
	}
	greeting := lastOf(messages(et.owner, conv), "message")
	if greeting["author"].(map[string]any)["type"] != "system" || greeting["body"] != "Thanks, we got your message." {
		t.Fatalf("greeting: %v", greeting)
	}
	h.sendQueued(t, et.ws, greeting["id"].(string))
	_, parsed := h.smtp.last(t)
	if parsed.Header.Get("Auto-Submitted") != "auto-replied" || parsed.Header.Get("In-Reply-To") == "" {
		t.Fatalf("greeting headers: %v", parsed.Header)
	}
	again := h.ingest(et.address, buildMail(mailOpts{from: sender, to: et.address, subject: "Second topic", messageID: newMessageID(), body: "Hi again"}), nil)
	if autoReplies(again.str("conversation_id")) != 0 {
		t.Fatal("greeted the same contact twice within the interval")
	}

	own := h.ingest(et.address, buildMail(mailOpts{from: et.address, to: et.address, subject: "Loop", messageID: newMessageID(), body: "x"}), nil)
	if own.status != http.StatusAccepted || own.str("status") != "dropped" {
		t.Fatalf("own address: %d %v", own.status, own.body)
	}

	flood := unique("flood") + "@example.net"
	var last ingressResult
	for i := 0; i <= 20; i++ {
		last = h.ingest(et.address, buildMail(mailOpts{from: flood, to: et.address, subject: "Flood " + strconv.Itoa(i), messageID: newMessageID(), body: "x"}), nil)
	}
	if last.status != http.StatusTooManyRequests || last.str("code") != "rate_limited" {
		t.Fatalf("21st new conversation in an hour: %d %v", last.status, last.body)
	}
}

func TestSpamView(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, true)
	r := h.ingest(et.address, buildMail(mailOpts{
		from: "spoof@example.net", to: et.address, subject: "You won", messageID: newMessageID(), body: "Click",
		headers: map[string]string{"Authentication-Results": "mx.example.com; spf=fail; dkim=none; dmarc=fail (p=reject) header.from=example.net"},
	}), nil)
	conv := r.str("conversation_id")
	c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if c.body["spam"] != true {
		t.Fatalf("DMARC failure not flagged: %s", c.raw)
	}
	inList := func(q string) bool {
		for _, it := range et.owner.expect(http.StatusOK, "GET", "/v1/conversations?inbox_id="+et.inbox+q, nil).body["items"].([]any) {
			if it.(map[string]any)["id"] == conv {
				return true
			}
		}
		return false
	}
	if inList("") || !inList("&spam=true") {
		t.Fatal("spam conversation in the wrong view")
	}
	counts := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil)
	if counts.body["spam"] != float64(1) || counts.body["all"] != float64(0) {
		t.Fatalf("counts: %s", counts.raw)
	}
	for _, m := range messages(et.owner, conv) {
		if m["direction"] == "out" {
			t.Fatal("spam got an auto-reply")
		}
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": false})
	if !inList("") {
		t.Fatal("unflagged conversation not back in the inbox")
	}
}

func dsnMail(to, original, rcpt, status string) []byte {
	return []byte("From: Mail Delivery System <mailer-daemon@mx.example.net>\r\n" +
		"To: " + to + "\r\nSubject: Undelivered Mail Returned to Sender\r\nMessage-ID: <" + newMessageID() + ">\r\n" +
		"Auto-Submitted: auto-replied\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=r1\r\n\r\n" +
		"--r1\r\nContent-Type: text/plain\r\n\r\nYour message could not be delivered.\r\n" +
		"--r1\r\nContent-Type: message/delivery-status\r\n\r\n" +
		"Reporting-MTA: dns; mx.example.net\r\n\r\n" +
		"Final-Recipient: rfc822; " + rcpt + "\r\nAction: failed\r\nStatus: " + status + "\r\n" +
		"Diagnostic-Code: smtp; 550 5.1.1 user unknown\r\n\r\n" +
		"--r1\r\nContent-Type: text/rfc822-headers\r\n\r\n" +
		"From: Acme Support <" + to + ">\r\nTo: " + rcpt + "\r\nSubject: Re: Q\r\nMessage-ID: <" + original + ">\r\n" +
		"--r1--\r\n")
}

func TestBounceDSN(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	rcpt := unique("gone") + "@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: rcpt, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	b := h.ingest(et.address, dsnMail(et.address, ourID, rcpt, "5.1.1"), func(req *http.Request) { req.Header.Set("X-Yuva-Envelope-From", "") })
	if b.status != http.StatusAccepted || b.str("status") != "bounce" || b.str("message_id") != reply.str("id") {
		t.Fatalf("dsn: %d %v", b.status, b.body)
	}
	d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any)
	if d["state"] != "failed" || !strings.Contains(d["error"].(string), "5.1.1") {
		t.Fatalf("message not failed: %v", d)
	}
	contactID := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
	contact := et.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+contactID, nil)
	und := contact.body["undeliverable"].([]any)
	if len(und) != 1 || und[0].(map[string]any)["email"] != rcpt || und[0].(map[string]any)["reason"] != "bounce" {
		t.Fatalf("undeliverable: %s", contact.raw)
	}
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "again"})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/contacts/"+contactID, map[string]any{"clear_undeliverable": []string{"someone@example.org"}})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+contactID, map[string]any{"clear_undeliverable": []string{rcpt}})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "again"})

	if n := len(messages(et.owner, conv)); n == 0 {
		t.Fatal("no messages")
	}
	before, _ := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["all"].(float64)
	delayed := h.ingest(et.address, dsnMail(et.address, ourID, rcpt, "4.4.1"), nil)
	if delayed.str("status") != "bounce" {
		t.Fatalf("delayed dsn: %v", delayed.body)
	}
	after, _ := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["all"].(float64)
	if before != after {
		t.Fatal("a delivery report opened a conversation")
	}
	if und := et.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+contactID, nil).body["undeliverable"].([]any); len(und) != 0 {
		t.Fatal("a temporary failure marked the address undeliverable")
	}
}

type snsSigner struct {
	key     *rsa.PrivateKey
	certURL string
}

func newSNSSigner(t *testing.T, web *fakeWeb) *snsSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "sns.amazonaws.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	s := &snsSigner{key: key, certURL: "https://sns.eu-west-1.amazonaws.com/SimpleNotificationService-" + unique("cert") + ".pem"}
	web.mu.Lock()
	web.pages[s.certURL] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	web.mu.Unlock()
	return s
}

func (s *snsSigner) sign(t *testing.T, m *email.SNSMessage) []byte {
	t.Helper()
	m.SignatureVersion, m.SigningCertURL = "2", s.certURL
	sum := sha256.Sum256([]byte(m.StringToSign()))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	b, _ := json.Marshal(m)
	return b
}

func (h *harness) postSNS(body []byte) int {
	h.t.Helper()
	req, _ := http.NewRequest("POST", h.url+"/ingress/ses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain; charset=UTF-8")
	req.Header.Set("X-Amz-Sns-Message-Type", "Notification")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestSESNotifications(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	signer := newSNSSigner(t, h.web)

	subscribe := "https://sns.eu-west-1.amazonaws.com/?Action=ConfirmSubscription&TopicArn=" + testSESTopic + "&Token=" + unique("tok")
	h.web.mu.Lock()
	h.web.pages[subscribe] = []byte("<ConfirmSubscriptionResponse/>")
	h.web.mu.Unlock()
	confirm := &email.SNSMessage{Type: "SubscriptionConfirmation", MessageId: unique("m"), Token: "tok", TopicArn: testSESTopic,
		Message: "You have chosen to subscribe", SubscribeURL: subscribe, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if st := h.postSNS(signer.sign(t, confirm)); st != http.StatusOK || !h.web.fetched(subscribe) {
		t.Fatalf("subscription confirmation: %d fetched=%v", st, h.web.fetched(subscribe))
	}
	evil := "https://sns.eu-west-1.amazonaws.com.evil.example/confirm"
	bad := &email.SNSMessage{Type: "SubscriptionConfirmation", MessageId: unique("m"), Token: "tok", TopicArn: testSESTopic,
		Message: "x", SubscribeURL: evil, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if st := h.postSNS(signer.sign(t, bad)); st != http.StatusForbidden || h.web.fetched(evil) {
		t.Fatalf("foreign SubscribeURL: %d", st)
	}
	other := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: "arn:aws:sns:eu-west-1:999999999999:other", Message: "{}", Timestamp: "x"}
	if st := h.postSNS(signer.sign(t, other)); st != http.StatusForbidden {
		t.Fatalf("unknown topic: %d", st)
	}

	rcpt := unique("complainer") + "@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: rcpt, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	fixture, err := os.ReadFile("../email/testdata/ses_bounce.json")
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.NewReplacer("{{MESSAGE_ID}}", ourID, "{{RECIPIENT}}", rcpt).Replace(string(fixture))
	note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: payload, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	body := signer.sign(t, note)
	tampered := bytes.Replace(body, []byte("Permanent"), []byte("Transient"), 1)
	if st := h.postSNS(tampered); st != http.StatusForbidden {
		t.Fatalf("tampered notification: %d", st)
	}
	if st := h.postSNS(body); st != http.StatusOK {
		t.Fatalf("bounce notification: %d", st)
	}
	if d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any); d["state"] != "failed" {
		t.Fatalf("SES bounce did not fail the message: %v", d)
	}
	et.owner.expectProblem(http.StatusConflict, "email_undeliverable", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "again"})
}

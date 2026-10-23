package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/storage"
)

type countingBody struct{ read atomic.Int64 }

func (b *countingBody) Read(p []byte) (int, error) {
	n := copy(p, bytes.Repeat([]byte("a"), len(p)))
	b.read.Add(int64(n))
	return n, nil
}

func (b *countingBody) Close() error { return nil }

func serveIngress(h *harness, req *http.Request) (int, string) {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.srv.Handler().ServeHTTP(rec, req)
	var out struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out.Code
}

func TestIngressRefusesBeforeReadingTheBody(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	raw := buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})
	stale := strconv.FormatInt(h.clock.Now().Add(-10*time.Minute).Unix(), 10)
	for name, tc := range map[string]struct {
		mutate func(*http.Request)
		status int
		code   string
	}{
		"no signature": {func(r *http.Request) { r.Header.Del("X-Yuva-Signature") }, http.StatusUnauthorized, "bad_signature"},
		"no timestamp": {func(r *http.Request) { r.Header.Del("X-Yuva-Timestamp") }, http.StatusUnauthorized, "bad_signature"},
		"stale": {func(r *http.Request) {
			r.Header.Set("X-Yuva-Timestamp", stale)
			r.Header.Set("X-Yuva-Signature", email.Sign(testIngressSecret, stale, et.address, "bounce@example.net", raw))
		}, http.StatusUnauthorized, "stale_timestamp"},
		"v1": {func(r *http.Request) {
			r.Header.Set("X-Yuva-Signature", email.SignV1(testIngressSecret, r.Header.Get("X-Yuva-Timestamp"), et.address, raw))
		}, http.StatusUnauthorized, "bad_signature"},
		"declared too large": {func(r *http.Request) { r.ContentLength = api.MaxIngressBytes + 1 }, http.StatusRequestEntityTooLarge, "too_large"},
		"v3 forged header MAC": {func(r *http.Request) {
			sig := r.Header.Get("X-Yuva-Signature")
			r.Header.Set("X-Yuva-Signature", "v3="+strings.Repeat("0", 64)+sig[67:])
		}, http.StatusUnauthorized, "bad_signature"},
		"v3 other Content-Length": {func(r *http.Request) { r.ContentLength-- }, http.StatusUnauthorized, "bad_signature"},
		"v3 no Content-Length":    {func(r *http.Request) { r.ContentLength = -1 }, http.StatusUnauthorized, "bad_signature"},
		"v2 not accepted": {func(r *http.Request) {
			r.Header.Set("X-Yuva-Signature", email.SignV2(testIngressSecret, r.Header.Get("X-Yuva-Timestamp"), et.address, "bounce@example.net", raw))
		}, http.StatusUnauthorized, "bad_signature"},
	} {
		req := h.ingressRequest(et.address, "bounce@example.net", raw)
		body := &countingBody{}
		req.Body, req.ContentLength = body, int64(len(raw))
		tc.mutate(req)
		if status, code := serveIngress(h, req); status != tc.status || code != tc.code {
			t.Fatalf("%s: %d %s", name, status, code)
		}
		if n := body.read.Load(); n != 0 {
			t.Fatalf("%s: read %d bytes of the body before refusing", name, n)
		}
	}
}

// gatedStorage holds every raw message write after hold until release is closed.
type gatedStorage struct {
	storage.Storage
	held    chan struct{}
	release chan struct{}
	armed   atomic.Bool
}

func (g *gatedStorage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	if strings.Contains(key, "/raw/") && g.armed.CompareAndSwap(true, false) {
		g.held <- struct{}{}
		<-g.release
	}
	return g.Storage.Put(ctx, key, r, size, ct)
}

func TestIngressConcurrencyCap(t *testing.T) {
	gate := &gatedStorage{held: make(chan struct{}), release: make(chan struct{})}
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Ingress.MaxConcurrent = 1
		gate.Storage = d.Storage
		d.Storage = gate
	})
	et := newEmailTeam(t, h, false)
	raw := buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})

	gate.armed.Store(true)
	done := make(chan int, 1)
	go func() {
		status, _ := serveIngress(h, h.ingressRequest(et.address, "bounce@example.net", raw))
		done <- status
	}()
	<-gate.held

	second := buildMail(mailOpts{from: "other@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})
	if r := h.ingest(et.address, second, nil); r.status != http.StatusServiceUnavailable || r.str("code") != "unavailable" {
		t.Fatalf("over the cap: %d %v", r.status, r.body)
	}
	close(gate.release)
	if status := <-done; status != http.StatusAccepted {
		t.Fatalf("held request: %d", status)
	}
	if r := h.ingest(et.address, second, nil); r.status != http.StatusAccepted {
		t.Fatalf("after the slot is free: %d %v", r.status, r.body)
	}
}

func TestIngressForgedV3HoldsNoSlot(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.MaxConcurrent = 1 })
	et := newEmailTeam(t, h, false)
	raw := buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})

	pr, pw := io.Pipe()
	defer pw.Close()
	forged := h.ingressRequest(et.address, "bounce@example.net", raw)
	sig := forged.Header.Get("X-Yuva-Signature")
	forged.Header.Set("X-Yuva-Signature", "v3="+strings.Repeat("0", 64)+sig[67:])
	forged.Body, forged.ContentLength = pr, api.MaxIngressBytes
	done := make(chan int, 1)
	go func() {
		status, _ := serveIngress(h, forged)
		done <- status
	}()
	select {
	case status := <-done:
		if status != http.StatusUnauthorized {
			t.Fatalf("forged v3: %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a forged v3 request waited for its body")
	}
	if r := h.ingest(et.address, raw, nil); r.status != http.StatusAccepted {
		t.Fatalf("signed mail after a forged v3: %d %v", r.status, r.body)
	}
}

func TestIngressReadDeadlineFreesSlowV2Slot(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Ingress.MaxConcurrent, d.Ingress.AcceptV2, d.Ingress.ReadTimeout = 1, true, 500*time.Millisecond
	})
	et := newEmailTeam(t, h, false)
	raw := buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})

	pr, pw := io.Pipe()
	defer pw.Close()
	slow := h.ingressRequest(et.address, "bounce@example.net", raw)
	slow.Header.Set("X-Yuva-Signature", "v2="+strings.Repeat("0", 64))
	slow.Body, slow.ContentLength = pr, -1
	done := make(chan ingressResult, 1)
	go func() {
		res, err := http.DefaultClient.Do(slow)
		if err != nil {
			done <- ingressResult{}
			return
		}
		defer res.Body.Close()
		out := ingressResult{status: res.StatusCode}
		_ = json.NewDecoder(res.Body).Decode(&out.body)
		done <- out
	}()
	if _, err := pw.Write(raw[:10]); err != nil {
		t.Fatal(err)
	}

	if r := h.ingest(et.address, raw, nil); r.status != http.StatusServiceUnavailable || r.str("code") != "unavailable" {
		t.Fatalf("while a v2 body holds the slot: %d %v", r.status, r.body)
	}
	select {
	case r := <-done:
		if r.status != http.StatusServiceUnavailable || r.str("code") != "unavailable" {
			t.Fatalf("slow v2 body: %d %v", r.status, r.body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the read deadline did not end a slow v2 body")
	}
	if r := h.ingest(et.address, raw, nil); r.status != http.StatusAccepted {
		t.Fatalf("after the deadline freed the slot: %d %v", r.status, r.body)
	}
}

func TestIngressSignatureVersions(t *testing.T) {
	signV2 := func(req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
		req.Header.Set("X-Yuva-Signature", email.SignV2(testIngressSecret, req.Header.Get("X-Yuva-Timestamp"), req.Header.Get("X-Yuva-Envelope-To"), req.Header.Get("X-Yuva-Envelope-From"), raw))
	}
	for _, acceptV2 := range []bool{false, true} {
		h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.AcceptV2 = acceptV2 })
		et := newEmailTeam(t, h, false)
		mail := func() []byte {
			return buildMail(mailOpts{from: "cust@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "Hello"})
		}
		if r := h.ingest(et.address, mail(), nil); r.status != http.StatusAccepted || !strings.HasPrefix(h.ingressRequest(et.address, "", nil).Header.Get("X-Yuva-Signature"), "v3=") {
			t.Fatalf("v3 (accept v2 %v): %d %v", acceptV2, r.status, r.body)
		}
		r := h.ingest(et.address, mail(), signV2)
		if acceptV2 && r.status != http.StatusAccepted {
			t.Fatalf("v2 accepted by setting: %d %v", r.status, r.body)
		}
		if !acceptV2 && (r.status != http.StatusUnauthorized || r.str("code") != "bad_signature") {
			t.Fatalf("v2 without the setting: %d %v", r.status, r.body)
		}
	}
}

func TestIngressV1DoesNotTrustTheEnvelopeSender(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.AcceptV1 = true })
	et := newEmailTeam(t, h, false)
	rcpt := unique("gone") + "@example.net"
	r := h.ingest(et.address, buildMail(mailOpts{from: rcpt, to: et.address, subject: "Q", messageID: newMessageID(), body: "Q?"}), nil)
	conv := r.str("conversation_id")
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "A."})
	h.sendQueued(t, et.ws, reply.str("id"))
	ourID := lastOf(messages(et.owner, conv), "message")["email"].(map[string]any)["message_id"].(string)

	dsn := dsnMail(et.address, ourID, rcpt, "5.1.1")
	v1 := h.ingestFrom(et.address, "", dsn, func(req *http.Request) {
		req.Header.Set("X-Yuva-Signature", email.SignV1(testIngressSecret, req.Header.Get("X-Yuva-Timestamp"), et.address, dsn))
	})
	if v1.status != http.StatusAccepted || v1.str("status") == "bounce" {
		t.Fatalf("v1 with an empty envelope sender counted as a delivery report: %d %v", v1.status, v1.body)
	}
	if d := lastOf(messages(et.owner, conv), "message")["delivery"].(map[string]any); d["state"] != "sent" {
		t.Fatalf("a v1 report failed the message: %v", d)
	}
	v2 := h.ingestFrom(et.address, "", dsnMail(et.address, ourID, rcpt, "5.1.1"), nil)
	if v2.str("status") != "bounce" {
		t.Fatalf("v2 report: %v", v2.body)
	}
}

func TestIngressTrustsOnlyTheConfiguredAuthservID(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	forged := buildMail(mailOpts{
		from: unique("spoof") + "@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "x",
		headers: map[string]string{"Authentication-Results": "attacker.example; dmarc=fail header.from=example.net"},
	})
	r := h.ingest(et.address, forged, nil)
	c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+r.str("conversation_id"), nil)
	m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
	if c.body["spam"] != false || m["email"].(map[string]any)["dmarc"] != "unknown" {
		t.Fatalf("verdict of an untrusted authserv-id used: spam %v, %v", c.body["spam"], m["email"])
	}
	detail := et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+r.str("message_id")+"/email", nil)
	if detail.str("authentication_results") != "attacker.example; dmarc=fail header.from=example.net" {
		t.Fatalf("raw Authentication-Results not stored: %s", detail.raw)
	}

	h2 := newHarnessWith(t, func(d *api.Deps) { d.Ingress.AuthservID = "" })
	et2 := newEmailTeam(t, h2, false)
	r = h2.ingest(et2.address, buildMail(mailOpts{
		from: unique("spoof") + "@example.net", to: et2.address, subject: "Hi", messageID: newMessageID(), body: "x",
		headers: map[string]string{"Authentication-Results": testAuthservID + "; dmarc=fail header.from=example.net"},
	}), nil)
	if c := et2.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+r.str("conversation_id"), nil); c.body["spam"] != false {
		t.Fatalf("DMARC used without YUVA_INGRESS_AUTHSERV_ID: %s", c.raw)
	}
}

func TestIngressForgedHeaderOfTheTrustedAuthservID(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	forged := testAuthservID + "; spf=pass; dkim=pass header.d=example.net; dmarc=pass header.from=example.net"
	genuine := testAuthservID + "; spf=fail smtp.mailfrom=example.net; dkim=none; dmarc=fail (p=reject) header.from=example.net"
	for _, tc := range []struct{ name, top, below string }{
		{"forged above", forged, genuine},
		{"forged below", genuine, forged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := append([]byte("Authentication-Results: "+tc.top+"\r\n"), buildMail(mailOpts{
				from: unique("spoof") + "@example.net", to: et.address, subject: "Hi", messageID: newMessageID(), body: "x",
				headers: map[string]string{"Authentication-Results": tc.below},
			})...)
			r := h.ingest(et.address, raw, nil)
			c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+r.str("conversation_id"), nil)
			m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
			if c.body["spam"] != true || m["email"].(map[string]any)["dmarc"] != "fail" {
				t.Fatalf("forged dmarc=pass used: spam %v, %v", c.body["spam"], m["email"])
			}
		})
	}
}

func TestDMARCFailingReplyOpensSpamConversation(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	customer, first := unique("customer")+"@example.net", newMessageID()
	r := h.ingest(et.address, buildMail(mailOpts{from: customer, to: et.address, subject: "Invoice", messageID: first, body: "Where is it?"}), nil)
	conv := r.str("conversation_id")
	before := len(messages(et.owner, conv))

	forged := h.ingest(et.address, buildMail(mailOpts{
		from: customer, to: et.address, subject: "Re: Invoice", messageID: newMessageID(), body: "Pay to this account instead.",
		headers: map[string]string{
			"In-Reply-To": "<" + first + ">", "References": "<" + first + ">",
			"Authentication-Results": testAuthservID + "; spf=fail; dkim=none; dmarc=fail (p=reject) header.from=example.net",
		},
	}), nil)
	if forged.status != http.StatusAccepted || forged.str("conversation_id") == conv {
		t.Fatalf("a reply failing DMARC joined the conversation: %d %v", forged.status, forged.body)
	}
	side := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+forged.str("conversation_id"), nil)
	if side.body["spam"] != true || side.str("related_conversation_id") != conv {
		t.Fatalf("new conversation: %s", side.raw)
	}
	if n := len(messages(et.owner, conv)); n != before {
		t.Fatalf("the original conversation got %d messages", n-before)
	}

	genuine := h.ingest(et.address, buildMail(mailOpts{
		from: customer, to: et.address, subject: "Re: Invoice", messageID: newMessageID(), body: "Thanks",
		headers: map[string]string{
			"In-Reply-To":            "<" + first + ">",
			"Authentication-Results": testAuthservID + "; dmarc=pass header.from=example.net",
		},
	}), nil)
	if genuine.str("conversation_id") != conv {
		t.Fatalf("a passing reply did not join: %v", genuine.body)
	}
}

func TestSMTPHostCheckedOnSave(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	channel := func(c *client, host string) response {
		return c.do("POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
			"kind": "email", "name": "Mail",
			"email": map[string]any{"address": unique("support") + "@example.com", "smtp": map[string]any{"host": host, "tls": "starttls"}},
		})
	}
	for _, host := range []string{"127.0.0.1", "10.0.0.8", "192.168.1.10", "localhost", "relay.localhost", "169.254.169.254", "0.0.0.0"} {
		if r := channel(tm.owner, host); r.status != http.StatusBadRequest || r.str("code") != "validation_failed" {
			t.Fatalf("%s accepted: %d %s", host, r.status, r.raw)
		}
	}
	if r := channel(tm.owner, "smtp.example.com"); r.status != http.StatusCreated {
		t.Fatalf("public host: %d %s", r.status, r.raw)
	}
	ch := channel(tm.owner, "smtp2.example.com")
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/channels/"+ch.str("id"), map[string]any{
		"email": map[string]any{"address": ch.body["email"].(map[string]any)["address"], "smtp": map[string]any{"host": "10.1.2.3"}},
	})

	dev := newHarnessWith(t, func(d *api.Deps) { d.SMTPAllowPrivate = true })
	dt := newTeam(t, dev)
	devChannel := func(host string) int {
		return dt.owner.do("POST", "/v1/inboxes/"+dt.inbox+"/channels", map[string]any{
			"kind": "email", "name": "Mail",
			"email": map[string]any{"address": unique("support") + "@example.com", "smtp": map[string]any{"host": host, "tls": "none"}},
		}).status
	}
	if s := devChannel("127.0.0.1"); s != http.StatusCreated {
		t.Fatalf("YUVA_SMTP_ALLOW_PRIVATE: loopback refused with %d", s)
	}
	if s := devChannel("169.254.169.254"); s != http.StatusBadRequest {
		t.Fatalf("YUVA_SMTP_ALLOW_PRIVATE: metadata address answered %d", s)
	}
}

func TestAnonymousContactCap(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) { d.Chat.AnonymousContactsPerHour = 2 })
	ct := newChatTeam(t, h, "live", true)
	c := h.client()
	c.origin = ct.origin
	start := func(c *client, visitor string) response {
		req := map[string]any{"channel_key": ct.key}
		if visitor != "" {
			req["visitor_id"] = visitor
		}
		return c.do("POST", "/client/v1/session", req)
	}
	first := start(c, "")
	if first.status != http.StatusCreated || start(c, "").status != http.StatusCreated {
		t.Fatal("first visitors refused")
	}
	if r := start(c, ""); r.status != http.StatusTooManyRequests || r.str("code") != "anonymous_limit" {
		t.Fatalf("third new visitor from one address: %d %s", r.status, r.raw)
	}
	if r := start(c, first.str("visitor_id")); r.status != http.StatusCreated {
		t.Fatalf("resuming a visitor: %d %s", r.status, r.raw)
	}
	other := h.client()
	other.origin = ct.origin
	if r := start(other, ""); r.status != http.StatusCreated {
		t.Fatalf("another address: %d %s", r.status, r.raw)
	}
	h.clock.Advance(time.Hour + time.Second)
	if r := start(c, ""); r.status != http.StatusCreated {
		t.Fatalf("after an hour: %d %s", r.status, r.raw)
	}
}

func TestSenderCapCountsForgedMailApart(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(d *api.Deps) { d.Ingress.SenderHourlyCap = 2 })
	et := newEmailTeam(t, h, false)
	sender := unique("customer") + "@example.net"
	send := func(authResults string) ingressResult {
		return h.ingest(et.address, buildMail(mailOpts{
			from: sender, to: et.address, subject: "Order", messageID: newMessageID(), body: "x",
			headers: map[string]string{"Authentication-Results": authResults},
		}), nil)
	}
	forged := testAuthservID + "; dmarc=none header.from=example.net"
	for i := range 2 {
		if r := send(forged); r.status != http.StatusAccepted {
			t.Fatalf("forged mail %d: %d %v", i+1, r.status, r.body)
		}
	}
	if r := send(forged); r.status != http.StatusTooManyRequests {
		t.Fatalf("forged mail over the cap: %d %v", r.status, r.body)
	}
	for i := range 2 {
		if r := send(dmarcPass("example.net")); r.status != http.StatusAccepted || r.str("status") != "stored" {
			t.Fatalf("authenticated mail %d after a forged flood: %d %v", i+1, r.status, r.body)
		}
	}
	if r := send(dmarcPass("example.net")); r.status != http.StatusTooManyRequests || r.str("code") != "rate_limited" {
		t.Fatalf("authenticated mail over its own cap: %d %v", r.status, r.body)
	}
}

func TestLargeHTMLMailIsCheap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: Big\r\nMessage-ID: <%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hello there</p>\r\n",
		unique("big")+"@example.net", et.address, newMessageID())
	for b.Len() < 20<<20 {
		b.WriteString("<a><p>x</p></a><b><p>y</p></b>\r\n")
	}
	start := time.Now()
	r := h.ingest(et.address, []byte(b.String()), nil)
	took := time.Since(start)
	if r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	if took > 5*time.Second {
		t.Fatalf("a 20 MB HTML mail took %v", took)
	}
	m := lastOf(messages(et.owner, r.str("conversation_id")), "message")
	if m["html"] != nil || !strings.HasPrefix(m["body"].(string), "Hello there") {
		t.Fatalf("stored html %v, body %.40q", m["html"] != nil, m["body"])
	}
}

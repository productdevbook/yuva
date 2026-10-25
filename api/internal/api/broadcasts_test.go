package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/email"
	"github.com/productdevbook/yuva/api/internal/store"
)

// broadcastSender records sent mail like smtpCapture and lets a test refuse addresses or act in the
// middle of a batch.
type broadcastSender struct {
	mu      sync.Mutex
	sent    []sentMail
	refuse  func(to string) error
	onSend  func(n int)
	relay   func(to string) string
	sending int
}

func (b *broadcastSender) SendRelayed(ctx context.Context, cfg email.SMTPConfig, from string, to []string, msg []byte) (string, error) {
	if err := b.Send(ctx, cfg, from, to, msg); err != nil {
		return "", err
	}
	b.mu.Lock()
	relay := b.relay
	b.mu.Unlock()
	if relay == nil {
		return "", nil
	}
	return relay(to[0]), nil
}

func (b *broadcastSender) Send(_ context.Context, cfg email.SMTPConfig, from string, to []string, msg []byte) error {
	b.mu.Lock()
	refuse, onSend := b.refuse, b.onSend
	b.sending++
	n := b.sending
	b.mu.Unlock()
	if refuse != nil {
		if err := refuse(to[0]); err != nil {
			return err
		}
	}
	b.mu.Lock()
	b.sent = append(b.sent, sentMail{cfg: cfg, from: from, to: to, raw: msg})
	b.mu.Unlock()
	if onSend != nil {
		onSend(n)
	}
	return nil
}

func (b *broadcastSender) to(addr string) []*mail.Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*mail.Message
	for _, m := range b.sent {
		if m.to[0] == addr {
			parsed, err := mail.ReadMessage(bytes.NewReader(m.raw))
			if err == nil {
				out = append(out, parsed)
			}
		}
	}
	return out
}

func (b *broadcastSender) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sent)
}

func badMailbox(addr string) error {
	return &email.PermanentError{Err: &textproto.Error{Code: 550, Msg: "5.1.1 <" + addr + ">: user unknown"}}
}

type broadcastTeam struct {
	listTeam
	h      *harness
	sender *broadcastSender
	design string
}

func newBroadcastHarness(t *testing.T, configure func(*api.Deps)) (*harness, *broadcastSender) {
	t.Helper()
	bs := &broadcastSender{}
	h := newHarnessWith(t, func(d *api.Deps) {
		d.EmailSender = bs
		if configure != nil {
			configure(d)
		}
	})
	return h, bs
}

func newBroadcastTeam(t *testing.T, h *harness, bs *broadcastSender) broadcastTeam {
	t.Helper()
	lt := newListTeam(t, h)
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+lt.chatInbox, map[string]any{
		"sender": map[string]any{"legal_name": "Acme Ltd", "postal_address": "1 Main Street, Springfield"},
	})
	ch := lt.owner.expect(http.StatusOK, "GET", "/v1/channels/"+lt.emailChannel, nil)
	settings := ch.body["email"].(map[string]any)
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+lt.emailChannel, map[string]any{"email": map[string]any{
		"address": settings["address"], "display_name": "Acme News",
		"smtp":           map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "tls": "starttls"},
		"broadcast_smtp": map[string]any{"host": "bulk.example.com", "port": 587, "username": "bulk", "password": "bulk-secret"},
		"broadcast_rate": 100,
	}})
	d := lt.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "News", "document": designDoc(
		map[string]any{"id": "h1", "type": "heading", "level": 1, "text": "Hello {{ contact.first_name | \"there\" }}"},
		map[string]any{"id": "t1", "type": "text", "html": `<p>Read <a href="https://acme.example/news">the news</a>.</p>`},
		map[string]any{"id": "b1", "type": "button", "label": "Open", "url": "https://acme.example/open"},
	)})
	return broadcastTeam{listTeam: lt, h: h, sender: bs, design: d.str("id")}
}

func (bt broadcastTeam) subscribe(t *testing.T, addrs ...string) {
	t.Helper()
	for _, a := range addrs {
		bt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+bt.list+"/subscriptions", map[string]any{
			"email": a, "name": "Reader " + strings.Split(a, "@")[0], "consent": map[string]any{"confirmed": true},
		})
	}
}

// subscribeMany adds n subscribers straight into the database; prefix sorts them in order.
func (bt broadcastTeam) subscribeMany(t *testing.T, prefix string, n int) []string {
	t.Helper()
	var out []string
	var batch pgx.Batch
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("%s-%04d@example.org", prefix, i)
		id := uuid.NewV7()
		batch.Queue(`INSERT INTO contacts (id, workspace_id, name, attributes, created_at, updated_at) VALUES ($1, $2, '', '{}', now(), now())`, id, bt.ws)
		batch.Queue(`INSERT INTO contact_emails (workspace_id, contact_id, email, position) VALUES ($1, $2, $3, 0)`, bt.ws, id, addr)
		batch.Queue(`INSERT INTO list_subscriptions (id, workspace_id, list_id, contact_id, email, state, source, consent_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'subscribed', 'api', now(), now(), now())`, uuid.NewV7(), bt.ws, bt.list, id, addr)
		out = append(out, addr)
	}
	if err := bt.h.st.Pool.SendBatch(context.Background(), &batch).Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (bt broadcastTeam) draft(t *testing.T, extra map[string]any) string {
	t.Helper()
	body := map[string]any{
		"inbox_id": bt.chatInbox, "channel_id": bt.emailChannel, "name": "October news", "subject": "News for {{ contact.first_name | \"you\" }}",
		"design_id": bt.design, "audience": map[string]any{"list_ids": []string{bt.list}},
	}
	for k, v := range extra {
		body[k] = v
	}
	return bt.owner.expect(http.StatusCreated, "POST", "/v1/broadcasts", body).str("id")
}

func (bt broadcastTeam) get(t *testing.T, id string) map[string]any {
	t.Helper()
	return bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id, nil).body
}

func counts(b map[string]any) map[string]any { return b["counts"].(map[string]any) }

// run works the broadcast's start and send jobs as the queue would, until nothing is due now.
func (bt broadcastTeam) run(t *testing.T) time.Duration {
	t.Helper()
	ctx := context.Background()
	ws := uuid.MustParse(bt.ws)
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
	wait, err := bt.h.srv.SendBroadcasts(ctx, ws, uuid.MustParse(bt.emailChannel), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range bt.h.jobs("broadcast_send", bt.ws) {
		bt.h.dropJob(j.id)
	}
	return wait
}

func (bt broadcastTeam) schedule(t *testing.T, id string) map[string]any {
	t.Helper()
	return bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", nil).body
}

func deliveriesOf(t *testing.T, c *client, id, query string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/deliveries?limit=100"+query, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func deliveryOf(t *testing.T, c *client, id, addr string) map[string]any {
	t.Helper()
	for _, d := range deliveriesOf(t, c, id, "&q="+url.QueryEscape(addr)) {
		if d["email"] == addr {
			return d
		}
	}
	t.Fatalf("no delivery to %s", addr)
	return nil
}

var ourMessageID = regexp.MustCompile(`^<[a-z2-7]{24}\.[a-z2-7]{16}@example\.com>$`)

func TestBroadcastSendFlow(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "ada@example.org", "grace@example.org", "linus@example.org", "blocked@example.org", "gone@example.org", "later@example.org")
	sub := bt.subscription(t, "gone@example.org")
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+bt.list+"/subscriptions/"+sub["id"].(string), map[string]any{"state": "unsubscribed"})
	blocked := bt.subscription(t, "blocked@example.org")["contact"].(map[string]any)["id"].(string)
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+blocked, map[string]any{"blocked": true})
	if _, err := h.st.Pool.Exec(context.Background(), `INSERT INTO email_suppressions (workspace_id, email, reason, detail, scope, created_at) VALUES ($1, 'linus@example.org', 'bounce', '', 'all', now())`, bt.ws); err != nil {
		t.Fatal(err)
	}
	id := bt.draft(t, nil)

	count := bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/audience", nil).body
	ex := count["excluded"].(map[string]any)
	if count["recipients"] != float64(3) || ex["unsubscribed"] != float64(1) || ex["blocked"] != float64(1) || ex["suppressed"] != float64(1) {
		t.Fatalf("audience: %v", count)
	}
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if review["ready"] != true || review["recipients"] != float64(3) {
		t.Fatalf("review: %v", review)
	}

	bt.owner.expect(http.StatusAccepted, "POST", "/v1/broadcasts/"+id+"/test", nil)
	key := bt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "news", "scopes": []string{"broadcasts:read", "broadcasts:write", "broadcasts:send"}})
	kc := h.client()
	kc.bearer = key.str("secret")
	kc.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts/"+id+"/test", nil)
	kc.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts/"+id+"/test", map[string]any{"member_ids": []string{uuid.NewV7().String()}})
	for _, j := range h.jobs("broadcast_test", bt.ws) {
		var a api.BroadcastTestArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		if err := h.srv.SendBroadcastTestMail(context.Background(), a, false); err != nil {
			t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	if bs.count() != 1 || bs.sent[0].cfg.Host != "bulk.example.com" || bs.sent[0].cfg.Password != "bulk-secret" {
		t.Fatalf("test send: %d %+v", bs.count(), bs.sent)
	}
	test, _ := mail.ReadMessage(bytes.NewReader(bs.sent[0].raw))
	if !strings.HasPrefix(test.Header.Get("Subject"), "[Test] News for") {
		t.Fatalf("test subject: %q", test.Header.Get("Subject"))
	}
	testUnsub := strings.Trim(test.Header.Get("List-Unsubscribe"), "<>")
	if st, body := pageAt(t, h, "GET", strings.TrimPrefix(testUnsub, "http://localhost:8080"), nil); st != http.StatusOK || !strings.Contains(body, "test") {
		t.Fatalf("test unsubscribe page: %d", st)
	}

	scheduled := bt.schedule(t, id)
	if scheduled["state"] != "scheduled" || scheduled["design_version"] == nil {
		t.Fatalf("schedule: %v", scheduled)
	}
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "PATCH", "/v1/broadcasts/"+id, map[string]any{"name": "x"})
	bt.owner.expectProblem(http.StatusConflict, "broadcast_active", "DELETE", "/v1/broadcasts/"+id, nil)
	bt.owner.expectProblem(http.StatusConflict, "list_in_use", "DELETE", "/v1/lists/"+bt.list, nil)
	bt.subscribe(t, "late@example.org")
	if wait := bt.run(t); wait >= 0 {
		t.Fatalf("send left work: %v", wait)
	}
	b := bt.get(t, id)
	c := counts(b)
	if b["state"] != "sent" || c["recipients"] != float64(4) || c["sent"] != float64(4) || c["queued"] != float64(0) {
		t.Fatalf("after send: %v", b)
	}
	if got := bs.to("late@example.org"); len(got) != 1 {
		t.Fatalf("a subscriber added after scheduling but before the start gets it: %d", len(got))
	}
	msgs := bs.to("ada@example.org")
	if len(msgs) != 1 {
		t.Fatalf("ada got %d", len(msgs))
	}
	m := msgs[0]
	hd := m.Header
	if !ourMessageID.MatchString(hd.Get("Message-Id")) || hd.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" ||
		!strings.Contains(hd.Get("List-Id"), "<l") || !strings.HasSuffix(hd.Get("Feedback-Id"), ":yuvabroadcast") ||
		hd.Get("X-Auto-Response-Suppress") != "OOF, AutoReply" || hd.Get("Auto-Submitted") != "" || hd.Get("Subject") != "News for Reader" {
		t.Fatalf("headers: %v", hd)
	}
	if bs.to("blocked@example.org") != nil || bs.to("linus@example.org") != nil || bs.to("gone@example.org") != nil {
		t.Fatal("an excluded address got mail")
	}

	unsub := strings.Trim(hd.Get("List-Unsubscribe"), "<>")
	if st, _ := pageAt(t, h, "POST", strings.TrimPrefix(unsub, "http://localhost:8080"), url.Values{"List-Unsubscribe": {"One-Click"}}); st != http.StatusOK {
		t.Fatalf("one-click: %d", st)
	}
	if c := counts(bt.get(t, id)); c["unsubscribed"] != float64(1) {
		t.Fatalf("unsubscribed count: %v", c)
	}
	if d := deliveryOf(t, bt.owner, id, "ada@example.org"); d["unsubscribed_at"] == nil || d["state"] != "sent" {
		t.Fatalf("delivery: %v", d)
	}
	next := bt.draft(t, map[string]any{"name": "November"})
	if n := bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+next+"/audience", nil).body["recipients"]; n != float64(3) {
		t.Fatalf("the next broadcast still counts the unsubscribed address: %v", n)
	}
	report := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/report", nil).body
	if len(report["timeline"].([]any)) == 0 || report["rates"].(map[string]any)["unsubscribed"] != 0.25 {
		t.Fatalf("report: %v", report)
	}
	events := bt.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil).body["events"].([]any)
	var states []string
	for _, e := range events {
		ev := e.(map[string]any)
		if ev["type"] == "broadcast.updated" && ev["data"].(map[string]any)["broadcast"].(map[string]any)["id"] == id {
			states = append(states, ev["data"].(map[string]any)["broadcast"].(map[string]any)["state"].(string))
		}
	}
	if strings.Join(states, ",") != "scheduled,preparing,sending,sent" {
		t.Fatalf("broadcast.updated states: %v", states)
	}
	bt.owner.expect(http.StatusNoContent, "DELETE", "/v1/broadcasts/"+id, nil)
}

func TestBroadcastPauseCancelWithinBatch(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "r", 20)
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	bs.onSend = func(n int) {
		if n == 3 {
			bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/pause", nil)
		}
	}
	bt.run(t)
	b := bt.get(t, id)
	if b["state"] != "paused" || b["pause_reason"] != "member" || bs.count() != 3 || counts(b)["sent"] != float64(3) {
		t.Fatalf("paused: %d sent, %v", bs.count(), b)
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "POST", "/v1/broadcasts/"+id+"/pause", nil)
	bs.onSend = nil
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/resume", nil)
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sent" || bs.count() != 20 || counts(b)["sent"] != float64(20) {
		t.Fatalf("resumed: %d %v", bs.count(), b)
	}

	second := bt.draft(t, map[string]any{"name": "Second"})
	bt.schedule(t, second)
	bs.onSend = func(n int) {
		if n == 25 {
			bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+second+"/cancel", nil)
		}
	}
	bt.run(t)
	b = bt.get(t, second)
	c := counts(b)
	if b["state"] != "cancelled" || c["sent"] != float64(5) || c["skipped"].(map[string]any)["cancelled"] != float64(15) || bs.count() != 25 {
		t.Fatalf("cancelled: %d %v", bs.count(), b)
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "POST", "/v1/broadcasts/"+second+"/resume", nil)
}

func TestBroadcastRechecksEachAddress(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "stay@example.org", "leave@example.org", "bounced@example.org")
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	ctx := context.Background()
	if wait, err := h.srv.StartBroadcast(ctx, uuid.MustParse(bt.ws), uuid.MustParse(id), time.Minute); err != nil || wait >= 0 {
		t.Fatal(wait, err)
	}
	sub := bt.subscription(t, "leave@example.org")
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+bt.list+"/subscriptions/"+sub["id"].(string), map[string]any{"state": "unsubscribed"})
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO email_suppressions (workspace_id, email, reason, detail, scope, created_at) VALUES ($1, 'bounced@example.org', 'complaint', '', 'broadcasts', now())`, bt.ws); err != nil {
		t.Fatal(err)
	}
	bt.run(t)
	b := bt.get(t, id)
	sk := counts(b)["skipped"].(map[string]any)
	if b["state"] != "sent" || bs.count() != 1 || sk["unsubscribed"] != float64(1) || sk["suppressed"] != float64(1) {
		t.Fatalf("rechecks: %d %v", bs.count(), b)
	}
	if d := deliveryOf(t, bt.owner, id, "leave@example.org"); d["state"] != "skipped" || d["skip_reason"] != "unsubscribed" {
		t.Fatalf("delivery: %v", d)
	}
}

func TestBroadcastReplies(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "ada@example.org", "grace@example.org")
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	bt.run(t)
	address := bt.owner.expect(http.StatusOK, "GET", "/v1/channels/"+bt.emailChannel, nil).body["email"].(map[string]any)["address"].(string)
	adaID := bs.to("ada@example.org")[0].Header.Get("Message-Id")
	graceID := bs.to("grace@example.org")[0].Header.Get("Message-Id")

	r := h.ingest(address, buildMail(mailOpts{from: "ada@example.org", to: address, subject: "Re: News", messageID: newMessageID(), body: "Thanks!",
		headers: map[string]string{"In-Reply-To": adaID, "References": adaID}}), nil)
	conv := r.str("conversation_id")
	if r.str("status") != "stored" || conv == "" {
		t.Fatalf("reply: %v", r.body)
	}
	c := bt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).body
	if c["broadcast_id"] != id {
		t.Fatalf("conversation: %v", c)
	}
	ms := messages(bt.owner, conv)
	first := ms[0]
	if first["kind"] != "event" || first["event"].(map[string]any)["type"] != "broadcast_reply" ||
		first["event"].(map[string]any)["broadcast"].(map[string]any)["name"] != "October news" {
		t.Fatalf("first entry: %v", ms)
	}
	again := h.ingest(address, buildMail(mailOpts{from: "ada@example.org", to: address, subject: "Re: News", messageID: newMessageID(), body: "One more thing",
		headers: map[string]string{"In-Reply-To": adaID}}), nil)
	if again.str("conversation_id") != conv {
		t.Fatalf("second reply opened another conversation: %v", again.body)
	}
	ooo := h.ingest(address, buildMail(mailOpts{from: "grace@example.org", to: address, subject: "Out of office", messageID: newMessageID(), body: "Away",
		headers: map[string]string{"In-Reply-To": graceID, "Auto-Submitted": "auto-replied"}}), nil)
	if ooo.str("status") != "dropped" || ooo.str("conversation_id") != "" {
		t.Fatalf("out of office: %v", ooo.body)
	}
	fwd := h.ingest(address, buildMail(mailOpts{from: "boss@example.net", to: address, subject: "Fwd: News", messageID: newMessageID(), body: "Interesting",
		headers: map[string]string{"References": graceID}}), nil)
	other := bt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+fwd.str("conversation_id"), nil).body
	if other["broadcast_id"] != id || other["contact_id"] == c["contact_id"] {
		t.Fatalf("forward: %v", other)
	}
	b := bt.get(t, id)
	if cc := counts(b); cc["replied"] != float64(1) || cc["auto_replied"] != float64(1) {
		t.Fatalf("counts: %v", cc)
	}
	if d := deliveryOf(t, bt.owner, id, "grace@example.org"); d["replied_at"] != nil || d["auto_replied_at"] == nil {
		t.Fatalf("grace: %v", d)
	}
	list := bt.owner.expect(http.StatusOK, "GET", "/v1/conversations?broadcast_id="+id, nil).body["items"].([]any)
	if len(list) != 2 {
		t.Fatalf("filter: %d", len(list))
	}
	for _, it := range list {
		if it.(map[string]any)["broadcast_id"] != id {
			t.Fatalf("list item: %v", it)
		}
	}
	report := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/report", nil).body
	if rs := report["replies"].([]any); len(rs) != 1 || rs[0].(map[string]any)["conversation_id"] != conv {
		t.Fatalf("report replies: %v", report["replies"])
	}
	if jobs := h.jobs("broadcast_replies_notify", bt.ws); len(jobs) > 1 {
		t.Fatalf("reply notifications are not grouped: %d", len(jobs))
	}
}

func arfMail(from, to, original, rcpt string) []byte {
	return []byte("From: " + from + "\r\nTo: " + to + "\r\nSubject: Complaint\r\nMessage-ID: <" + newMessageID() + ">\r\n" +
		"Authentication-Results: " + testAuthservID + "; dmarc=pass header.from=" + email.Domain(from) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/report; report-type=feedback-report; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\nThis is an abuse report.\r\n" +
		"--b1\r\nContent-Type: message/feedback-report\r\n\r\nFeedback-Type: abuse\r\nUser-Agent: Test/1.0\r\nVersion: 1\r\nOriginal-Rcpt-To: " + rcpt + "\r\n\r\n" +
		"--b1\r\nContent-Type: text/rfc822-headers\r\n\r\nFrom: news@example.com\r\nTo: " + rcpt + "\r\nMessage-ID: " + original + "\r\nSubject: News\r\n\r\n" +
		"--b1--\r\n")
}

func TestBroadcastBouncesAndComplaints(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "unknown@example.org", "ses@example.org", "arf@example.org", "dsn@example.org")
	bs.refuse = func(to string) error {
		if to == "unknown@example.org" {
			return badMailbox(to)
		}
		return nil
	}
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	bt.run(t)
	if d := deliveryOf(t, bt.owner, id, "unknown@example.org"); d["state"] != "bounced" {
		t.Fatalf("SMTP 5.1.1: %v", d)
	}
	cid := bt.subscription(t, "unknown@example.org")["contact"].(map[string]any)["id"].(string)
	if und := bt.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+cid, nil).body["undeliverable"].([]any); len(und) != 1 {
		t.Fatalf("not suppressed: %v", und)
	}

	signer := newSNSSigner(t, h.web)
	sesID := strings.Trim(bs.to("ses@example.org")[0].Header.Get("Message-Id"), "<>")
	payload := `{"notificationType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"ses@example.org"}],"complaintFeedbackType":"abuse"},"mail":{"commonHeaders":{"messageId":"<` + sesID + `>"}}}`
	note := &email.SNSMessage{Type: "Notification", MessageId: unique("m"), TopicArn: testSESTopic, Message: payload, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	if st := h.postSNS(signer.sign(t, note)); st != http.StatusOK {
		t.Fatalf("ses: %d", st)
	}
	if d := deliveryOf(t, bt.owner, id, "ses@example.org"); d["state"] != "complained" {
		t.Fatalf("SES complaint: %v", d)
	}
	if s := bt.subscription(t, "ses@example.org"); s["state"] != "unsubscribed" || s["unsubscribe_reason"] != "complaint" {
		t.Fatalf("complaint did not unsubscribe: %v", s)
	}
	var scope string
	if err := h.st.Pool.QueryRow(context.Background(), `SELECT scope FROM email_suppressions WHERE workspace_id = $1 AND email = 'ses@example.org'`, bt.ws).Scan(&scope); err != nil || scope != "broadcasts" {
		t.Fatalf("complaint scope: %q %v", scope, err)
	}

	address := bt.owner.expect(http.StatusOK, "GET", "/v1/channels/"+bt.emailChannel, nil).body["email"].(map[string]any)["address"].(string)
	arfID := bs.to("arf@example.org")[0].Header.Get("Message-Id")
	untrusted := strings.Replace(string(arfMail("fbl@mailbox.example", address, arfID, "arf@example.org")), testAuthservID, "evil.example", 1)
	h.ingest(address, []byte(untrusted), nil)
	if d := deliveryOf(t, bt.owner, id, "arf@example.org"); d["state"] != "sent" {
		t.Fatalf("an untrusted report counted: %v", d)
	}
	if r := h.ingest(address, arfMail("fbl@mailbox.example", address, arfID, "dsn@example.org"), nil); r.str("status") == "bounce" {
		t.Fatalf("a report naming another recipient counted: %v", r.body)
	}
	if d := deliveryOf(t, bt.owner, id, "arf@example.org"); d["state"] != "sent" {
		t.Fatalf("a report naming another recipient counted: %v", d)
	}
	if r := h.ingest(address, arfMail("fbl@mailbox.example", address, arfID, "arf@example.org"), nil); r.str("status") != "bounce" {
		t.Fatalf("arf: %v", r.body)
	}
	if d := deliveryOf(t, bt.owner, id, "arf@example.org"); d["state"] != "complained" {
		t.Fatalf("ARF complaint: %v", d)
	}

	dsnID := strings.Trim(bs.to("dsn@example.org")[0].Header.Get("Message-Id"), "<>")
	if r := h.ingestFrom(address, "", dsnMail(address, dsnID, "dsn@example.org", "5.1.1"), nil); r.str("status") != "bounce" {
		t.Fatalf("dsn: %v", r.body)
	}
	if d := deliveryOf(t, bt.owner, id, "dsn@example.org"); d["state"] != "bounced" {
		t.Fatalf("DSN bounce: %v", d)
	}
	c := counts(bt.get(t, id))
	if c["bounced"] != float64(2) || c["complained"] != float64(2) {
		t.Fatalf("counts: %v", c)
	}
}

func TestBroadcastAutoPauseAtBounceRate(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "a-bad", 12)
	bt.subscribeMany(t, "b-good", 250)
	bs.refuse = func(to string) error {
		if strings.HasPrefix(to, "a-bad") {
			return badMailbox(to)
		}
		return nil
	}
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	bt.run(t)
	b := bt.get(t, id)
	if b["state"] != "paused" || b["pause_reason"] != "bounce_rate" || counts(b)["queued"] == float64(0) {
		t.Fatalf("bounce rate: %v", b)
	}
	h.clock.Advance(time.Minute)
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/resume", nil)
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sent" {
		t.Fatalf("resume after the rate pause: %v", b)
	}
}

func TestBroadcastDailyCapAndComplaintRate(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.DailyRecipients = 100 })
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "r", 250)
	id := bt.draft(t, nil)
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if review["days"] != float64(3) {
		t.Fatalf("review days: %v", review)
	}
	bt.schedule(t, id)
	for day := 1; day <= 3; day++ {
		wait := bt.run(t)
		b := bt.get(t, id)
		want := map[int]float64{1: 100, 2: 200, 3: 250}[day]
		if counts(b)["sent"] != want {
			t.Fatalf("day %d: %v", day, counts(b))
		}
		if day < 3 {
			if b["state"] != "sending" || b["waiting_until"] == nil || wait <= 0 {
				t.Fatalf("day %d waits: %v %v", day, wait, b)
			}
			h.clock.Advance(24 * time.Hour)
		} else if b["state"] != "sent" {
			t.Fatalf("day 3: %v", b)
		}
	}
	ws := bt.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).body["broadcasts"].(map[string]any)
	if ws["daily_limit"] != float64(100) || ws["sent_today"] != float64(50) {
		t.Fatalf("workspace broadcasts: %v", ws)
	}
	meAgreesWithWorkspace(t, bt)
}

func meAgreesWithWorkspace(t *testing.T, bt broadcastTeam) {
	t.Helper()
	ws := bt.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).body
	for _, m := range bt.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any) {
		mw := m.(map[string]any)["workspace"].(map[string]any)
		if mw["id"] != ws["id"] {
			continue
		}
		if !reflect.DeepEqual(mw, ws) {
			t.Fatalf("/v1/me workspace %v, /v1/workspace %v", mw, ws)
		}
		return
	}
	t.Fatalf("no membership for workspace %v", ws["id"])
}

func TestBroadcastComplaintRateSuspends(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.DailyRecipients = 1000 })
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "r", 1010)
	id := bt.draft(t, nil)
	bt.schedule(t, id)
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sending" || counts(b)["sent"] != float64(1000) {
		t.Fatalf("cap: %v", counts(b))
	}
	address := bt.owner.expect(http.StatusOK, "GET", "/v1/channels/"+bt.emailChannel, nil).body["email"].(map[string]any)["address"].(string)
	for i := 0; i < 3; i++ {
		rcpt := fmt.Sprintf("r-%04d@example.org", i)
		mid := bs.to(rcpt)[0].Header.Get("Message-Id")
		h.ingest(address, arfMail("fbl@mailbox.example", address, mid, rcpt), nil)
	}
	b := bt.get(t, id)
	if b["state"] != "paused" || (b["pause_reason"] != "complaint_rate" && b["pause_reason"] != "workspace_suspended") {
		t.Fatalf("complaint rate: %v", b)
	}
	ws := bt.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).body["broadcasts"].(map[string]any)
	if ws["status"] != "suspended" {
		t.Fatalf("workspace not suspended: %v", ws)
	}
	meAgreesWithWorkspace(t, bt)
	bt.owner.expectProblem(http.StatusForbidden, "broadcasts_suspended", "POST", "/v1/broadcasts/"+id+"/resume", nil)
	if err := h.srv.OperatorApproveBroadcasts(context.Background(), uuid.MustParse(bt.ws), ""); err != nil {
		t.Fatal(err)
	}
	bt.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/resume", nil)
}

func TestBroadcastApprovalGateAndWarmup(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.Mode = "approval" })
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "ada@example.org")
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, `UPDATE workspaces SET broadcasts_status = 'approval_required' WHERE id = $1`, bt.ws); err != nil {
		t.Fatal(err)
	}
	id := bt.draft(t, nil)
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if review["ready"] != false {
		t.Fatalf("review: %v", review)
	}
	bt.owner.expectProblem(http.StatusForbidden, "broadcasts_approval_required", "POST", "/v1/broadcasts/"+id+"/schedule", nil)
	bt.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/workspace/broadcast-approval", map[string]any{
		"website": "https://acme.example", "sender_description": "Product news", "consent_description": "A form on the site"})
	w := bt.owner.expect(http.StatusAccepted, "POST", "/v1/workspace/broadcast-approval", map[string]any{
		"website": "https://acme.example", "sender_description": "Product news", "consent_description": "A form on the site"}).body
	if w["broadcasts"].(map[string]any)["status"] != "approval_requested" || w["broadcasts"].(map[string]any)["daily_limit"] != float64(500) {
		t.Fatalf("requested: %v", w)
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "POST", "/v1/workspace/broadcast-approval", map[string]any{
		"website": "https://acme.example", "sender_description": "Product news", "consent_description": "A form on the site"})
	bt.owner.expectProblem(http.StatusForbidden, "broadcasts_approval_required", "POST", "/v1/broadcasts/"+id+"/schedule", nil)
	if err := h.srv.OperatorApproveBroadcasts(ctx, uuid.MustParse(bt.ws), ""); err != nil {
		t.Fatal(err)
	}
	bt.schedule(t, id)
	bt.run(t)
	if b := bt.get(t, id); b["state"] != "sent" {
		t.Fatalf("after approval: %v", b)
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO broadcast_sending_days (workspace_id, day, sent) VALUES ($1, $2, 400)`, bt.ws, yesterday); err != nil {
		t.Fatal(err)
	}
	if lim := bt.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).body["broadcasts"].(map[string]any)["daily_limit"]; lim != float64(1000) {
		t.Fatalf("warm-up after a clean day: %v", lim)
	}
}

func TestBroadcastScopesAndAccess(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "ada@example.org")
	id := bt.draft(t, nil)

	reader := h.client()
	reader.bearer = bt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "r", "scopes": []string{"broadcasts:read", "broadcasts:write"}}).str("secret")
	reader.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id, nil)
	r := reader.do("POST", "/v1/broadcasts/"+id+"/schedule", nil)
	if r.status != http.StatusForbidden || r.str("code") != "insufficient_scope" || r.str("scope") != "broadcasts:send" {
		t.Fatalf("schedule without send: %d %s", r.status, r.raw)
	}
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/broadcasts/"+id+"/deliveries", nil)
	def := bt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "default"}).body["api_key"].(map[string]any)
	for _, sc := range def["scopes"].([]any) {
		if sc == "broadcasts:send" {
			t.Fatal("broadcasts:send is implied")
		}
	}
	sender := h.client()
	sender.bearer = bt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "s", "scopes": []string{"broadcasts:read", "broadcasts:send"}}).str("secret")
	sender.expectProblem(http.StatusForbidden, "broadcasts_by_keys_disabled", "POST", "/v1/broadcasts/"+id+"/schedule", nil)
	sender.expectProblem(http.StatusForbidden, "insufficient_scope", "PATCH", "/v1/workspace", map[string]any{"broadcasts_by_keys": true})
	if w := bt.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"broadcasts_by_keys": true}).body; w["broadcasts"].(map[string]any)["by_keys"] != true {
		t.Fatalf("by_keys: %v", w)
	}
	scheduled := sender.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"send_at": time.Now().Add(time.Hour), "confirm_recipients": 1}).body
	if scheduled["scheduled_by_api_key"] == nil {
		t.Fatalf("scheduled by key: %v", scheduled)
	}
	sender.expect(http.StatusOK, "POST", "/v1/broadcasts/"+id+"/unschedule", nil)
	sender.expectProblem(http.StatusConflict, "audience_changed", "POST", "/v1/broadcasts/"+id+"/schedule", map[string]any{"confirm_recipients": 7})

	bt.agent.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id, nil)
	bt.agent.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/report", nil)
	bt.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/broadcasts/"+id+"/deliveries", nil)
	bt.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/broadcasts", map[string]any{"inbox_id": bt.chatInbox, "name": "x"})
	bt.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/broadcasts/"+id+"/pause", nil)

	other := newTeam(t, h)
	for _, p := range []string{"", "/report", "/review", "/deliveries"} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/broadcasts/"+id+p, nil)
	}
	for _, p := range []string{"/pause", "/cancel", "/audience", "/test"} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/broadcasts/"+id+p, nil)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/broadcasts/"+id, nil)
	if items := other.owner.expect(http.StatusOK, "GET", "/v1/broadcasts", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("another workspace lists %d broadcasts", len(items))
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/broadcasts", map[string]any{"inbox_id": bt.chatInbox, "name": "x"})
	other.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts", map[string]any{
		"inbox_id": other.inbox, "name": "x", "audience": map[string]any{"list_ids": []string{bt.list}}})
}

func TestBroadcastsOffAndClickTracking(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "ada@example.org")
	id := bt.draft(t, map[string]any{"click_tracking": true})
	bt.schedule(t, id)
	bt.run(t)
	raw := string(bs.sent[len(bs.sent)-1].raw)
	click := regexp.MustCompile(`http://localhost:8080(/c/[A-Za-z0-9_-]+)`).FindStringSubmatch(strings.ReplaceAll(strings.ReplaceAll(raw, "=\r\n", ""), "=3D", "="))
	if click == nil {
		t.Fatal("no tracked link in the mail")
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	head, _ := http.NewRequest("HEAD", h.url+click[1], nil)
	if res, err := noFollow.Do(head); err != nil || res.StatusCode != http.StatusFound {
		t.Fatalf("head: %v %v", res, err)
	}
	if c := counts(bt.get(t, id)); c["clicked"] != float64(0) {
		t.Fatalf("a HEAD counted: %v", c)
	}
	res, err := noFollow.Get(h.url + click[1])
	if err != nil || res.StatusCode != http.StatusFound || !strings.HasPrefix(res.Header.Get("Location"), "https://acme.example/") || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("click: %v %v", res, err)
	}
	if c := counts(bt.get(t, id)); c["clicked"] != float64(1) {
		t.Fatalf("click count: %v", c)
	}
	report := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/report", nil).body
	if links := report["links"].([]any); len(links) < 2 || links[0].(map[string]any)["clicks"] != float64(1) {
		t.Fatalf("links: %v", report["links"])
	}
	if res, _ := noFollow.Get(h.url + "/c/forged"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("forged token: %d", res.StatusCode)
	}

	off, _ := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.Mode = "off" })
	ot := newBroadcastTeam(t, off, bs)
	ot.owner.expectProblem(http.StatusForbidden, "broadcasts_disabled", "POST", "/v1/broadcasts", map[string]any{"inbox_id": ot.chatInbox, "name": "x"})
	if m := ot.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).body["broadcasts"].(map[string]any)["mode"]; m != "off" {
		t.Fatalf("mode: %v", m)
	}
}

func timeOf(t *testing.T, v any) time.Time {
	t.Helper()
	s, _ := v.(string)
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("not a time: %v", v)
	}
	return at
}

func reviewItem(review map[string]any, key string) map[string]any {
	for _, it := range review["items"].([]any) {
		if m := it.(map[string]any); m["key"] == key {
			return m
		}
	}
	return nil
}

func broadcastEvents(t *testing.T, bt broadcastTeam, id string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range bt.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil).body["events"].([]any) {
		ev := e.(map[string]any)
		if ev["type"] == "broadcast.updated" && ev["data"].(map[string]any)["broadcast"].(map[string]any)["id"] == id {
			out = append(out, ev["data"].(map[string]any))
		}
	}
	return out
}

func TestBroadcastPaceSpacesMail(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "p1@example.org", "p2@example.org", "p3@example.org", "p4@example.org")
	id := bt.draft(t, map[string]any{"pace": map[string]any{"per_hour": 2}})
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if it := reviewItem(review, "pace"); it["status"] != "ok" || review["duration_seconds"] != float64(3*1800) {
		t.Fatalf("review: %v", review)
	}
	bt.schedule(t, id)
	t0 := h.clock.Now()
	wait := bt.run(t)
	b := bt.get(t, id)
	if bs.count() != 1 || len(bs.to("p1@example.org")) != 1 || b["state"] != "sending" {
		t.Fatalf("first mail: %d %v", bs.count(), b)
	}
	if p := b["pace"].(map[string]any); p["per_hour"] != float64(2) || len(p) != 1 {
		t.Fatalf("pace: %v", b["pace"])
	}
	if next := timeOf(t, b["pace_next_at"]); !next.Equal(t0.Add(30 * time.Minute).Truncate(time.Microsecond)) {
		t.Fatalf("pace_next_at %v, want %v", next, t0.Add(30*time.Minute))
	}
	if b["waiting_until"] != nil || wait <= 0 || wait > 5*time.Minute {
		t.Fatalf("wait %v, waiting_until %v", wait, b["waiting_until"])
	}
	bt.run(t)
	h.clock.Advance(29 * time.Minute)
	bt.run(t)
	if bs.count() != 1 {
		t.Fatalf("a mail went out before its slot: %d", bs.count())
	}
	h.clock.Advance(time.Minute)
	bt.run(t)
	if bs.count() != 2 || len(bs.to("p2@example.org")) != 1 {
		t.Fatalf("second mail at +30 min: %d", bs.count())
	}
	sub := bt.subscription(t, "p3@example.org")
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+bt.list+"/subscriptions/"+sub["id"].(string), map[string]any{"state": "unsubscribed"})
	h.clock.Advance(29 * time.Minute)
	bt.run(t)
	if bs.count() != 2 {
		t.Fatalf("a mail went out before its slot: %d", bs.count())
	}
	h.clock.Advance(time.Minute)
	if wait := bt.run(t); wait >= 0 {
		t.Fatalf("send left work: %v", wait)
	}
	b = bt.get(t, id)
	c := counts(b)
	if bs.count() != 3 || len(bs.to("p4@example.org")) != 1 || b["state"] != "sent" || c["sent"] != float64(3) ||
		c["skipped"].(map[string]any)["unsubscribed"] != float64(1) || b["pace_next_at"] != nil {
		t.Fatalf("the unsubscribed recipient took a slot or the send did not finish: %d %v", bs.count(), b)
	}
	at := func(addr string) time.Time { return timeOf(t, deliveryOf(t, bt.owner, id, addr)["sent_at"]) }
	if at("p2@example.org").Sub(at("p1@example.org")) < 30*time.Minute || at("p4@example.org").Sub(at("p2@example.org")) < 30*time.Minute {
		t.Fatalf("spacing: %v %v %v", at("p1@example.org"), at("p2@example.org"), at("p4@example.org"))
	}
}

func TestBroadcastPaceBuildFailureTakesNoSlot(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "big@example.org", "ok@example.org")
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE contacts SET attributes = jsonb_build_object('bio', repeat('y', 100000)) WHERE workspace_id = $1 AND id = (SELECT contact_id FROM contact_emails WHERE workspace_id = $1 AND email = 'big@example.org')`, bt.ws); err != nil {
		t.Fatal(err)
	}
	var blocks []map[string]any
	for i := 0; i < 6; i++ {
		blocks = append(blocks, map[string]any{"id": fmt.Sprintf("t%d", i), "type": "text", "html": "<p>{{ contact.attributes.bio | \"none\" }}</p>"})
	}
	wide := bt.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Wide", "document": designDoc(blocks...)}).str("id")
	id := bt.draft(t, map[string]any{"design_id": wide, "pace": map[string]any{"per_hour": 2}})
	bt.schedule(t, id)
	bt.run(t)
	b := bt.get(t, id)
	c := counts(b)
	if bs.count() != 1 || len(bs.to("ok@example.org")) != 1 || b["state"] != "sent" || c["failed"] != float64(1) || c["sent"] != float64(1) {
		t.Fatalf("a mail that could not be built took the slot: %d %v", bs.count(), b)
	}
}

func TestBroadcastPaceSkippedRecipientsDoNotWait(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "w1@example.org", "w2@example.org", "w3@example.org", "w4@example.org")
	id := bt.draft(t, map[string]any{"pace": map[string]any{"per_hour": 1800}})
	bt.schedule(t, id)
	bt.run(t)
	if bs.count() != 1 {
		t.Fatalf("first run: %d", bs.count())
	}
	for _, addr := range []string{"w2@example.org", "w3@example.org"} {
		sub := bt.subscription(t, addr)
		bt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+bt.list+"/subscriptions/"+sub["id"].(string), map[string]any{"state": "unsubscribed"})
	}
	h.clock.Advance(2 * time.Second)
	waits := h.sleep.count()
	bt.run(t)
	if n := h.sleep.count() - waits; n > 0 {
		t.Fatalf("skipped recipients made the batch wait %d times", n)
	}
	c := counts(bt.get(t, id))
	if bs.count() != 2 || c["skipped"].(map[string]any)["unsubscribed"] != float64(2) {
		t.Fatalf("second run: %d %v", bs.count(), c)
	}
}

func TestBroadcastPaceSlotClaimIsExclusive(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	id := uuid.MustParse(bt.draft(t, map[string]any{"pace": map[string]any{"per_hour": 10}}))
	ws := uuid.MustParse(bt.ws)
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, `UPDATE broadcasts SET state = 'sending' WHERE workspace_id = $1 AND id = $2`, ws, id); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 20; round++ {
		if _, err := h.st.Pool.Exec(ctx, `UPDATE broadcasts SET pace_next_at = NULL WHERE workspace_id = $1 AND id = $2`, ws, id); err != nil {
			t.Fatal(err)
		}
		now := h.clock.Now()
		var wg sync.WaitGroup
		var mu sync.Mutex
		won := int64(0)
		start := make(chan struct{})
		for range 2 {
			wg.Go(func() {
				<-start
				n, err := h.st.ClaimBroadcastPaceSlot(ctx, store.ClaimBroadcastPaceSlotParams{WorkspaceID: ws, ID: id, Now: now})
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				won += n
				mu.Unlock()
			})
		}
		close(start)
		wg.Wait()
		if won != 1 {
			t.Fatalf("round %d: %d claims won the same slot", round, won)
		}
	}
	var next time.Time
	if err := h.st.Pool.QueryRow(ctx, `SELECT pace_next_at FROM broadcasts WHERE workspace_id = $1 AND id = $2`, ws, id).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if want := h.clock.Now().Add(6 * time.Minute); next.Sub(want).Abs() > time.Millisecond {
		t.Fatalf("next slot %v, want %v", next, want)
	}
	other := newBroadcastTeam(t, h, bs)
	if n, err := h.st.ClaimBroadcastPaceSlot(ctx, store.ClaimBroadcastPaceSlotParams{WorkspaceID: uuid.MustParse(other.ws), ID: id, Now: h.clock.Now().Add(time.Hour)}); err != nil || n != 0 {
		t.Fatalf("another workspace claimed the slot: %d %v", n, err)
	}
}

func TestBroadcastPaceChanges(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, nil)
	bt := newBroadcastTeam(t, h, bs)
	path := func(id string) string { return "/v1/broadcasts/" + id }
	for _, bad := range []map[string]any{{"per_hour": 0}, {"per_day": 10000001}, {"per_month": -1}} {
		bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/broadcasts", map[string]any{
			"inbox_id": bt.chatInbox, "name": "Bad pace", "pace": bad,
		})
	}
	id := bt.draft(t, map[string]any{"pace": map[string]any{"per_day": 100}})
	if p := bt.get(t, id)["pace"].(map[string]any); p["per_day"] != float64(100) {
		t.Fatalf("created pace: %v", p)
	}
	b := bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 10, "per_month": 1000}, "name": "Paced"}).body
	if p := b["pace"].(map[string]any); b["name"] != "Paced" || p["per_hour"] != float64(10) || p["per_month"] != float64(1000) || p["per_day"] != nil {
		t.Fatalf("draft pace replaced: %v", b)
	}
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 0}})
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_month": 10000001}})
	if b := bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{}}).body; b["pace"] != nil {
		t.Fatalf("pace {} kept the pace: %v", b)
	}
	if ev := broadcastEvents(t, bt, id); len(ev) != 0 {
		t.Fatalf("a draft's changes emitted events: %v", ev)
	}

	bt.subscribeMany(t, "q", 15)
	bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 1}})
	bt.schedule(t, id)
	if b := bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 1, "per_day": 24}}).body; b["state"] != "scheduled" || b["pace"].(map[string]any)["per_day"] != float64(24) {
		t.Fatalf("pace of a scheduled broadcast: %v", b)
	}
	t0 := h.clock.Now()
	bt.run(t)
	if bs.count() != 1 {
		t.Fatalf("first mail: %d", bs.count())
	}
	if next := timeOf(t, bt.get(t, id)["pace_next_at"]); !next.Equal(t0.Add(time.Hour).Truncate(time.Microsecond)) {
		t.Fatalf("pace_next_at %v", next)
	}
	b = bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 2}}).body
	if next := timeOf(t, b["pace_next_at"]); !next.Equal(t0.Add(30*time.Minute).Truncate(time.Microsecond)) || b["state"] != "sending" {
		t.Fatalf("the next slot did not move to the last mail plus the new gap: %v", b)
	}
	ev := broadcastEvents(t, bt, id)
	last := ev[len(ev)-1]
	if last["previous_state"] != nil || last["broadcast"].(map[string]any)["pace"].(map[string]any)["per_hour"] != float64(2) {
		t.Fatalf("broadcast.updated for the pace change: %v", last)
	}
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 3}, "name": "x"})
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 3}, "channel_id": nil})
	r := bt.owner.expect(http.StatusConflict, "PATCH", path(id), map[string]any{"pace": map[string]any{"per_month": 1}})
	if r.str("code") != "pace_too_slow" || r.body["days"] != float64(13*30) {
		t.Fatalf("pace_too_slow: %s", r.raw)
	}
	if p := bt.get(t, id)["pace"].(map[string]any); p["per_hour"] != float64(2) {
		t.Fatalf("a refused pace was stored: %v", p)
	}
	bt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 0}})
	b = bt.owner.expect(http.StatusOK, "PATCH", path(id), map[string]any{"pace": map[string]any{}}).body
	if b["pace"] != nil || b["pace_next_at"] != nil {
		t.Fatalf("pace {} while sending: %v", b)
	}
	if wait := bt.run(t); wait >= 0 {
		t.Fatalf("send left work after the pace was removed: %v", wait)
	}
	if b := bt.get(t, id); b["state"] != "sent" || bs.count() != 15 {
		t.Fatalf("after removing the pace: %d %v", bs.count(), b)
	}
	bt.owner.expectProblem(http.StatusConflict, "invalid_state", "PATCH", path(id), map[string]any{"pace": map[string]any{"per_hour": 2}})
	bt.owner.expectProblem(http.StatusConflict, "not_draft", "PATCH", path(id), map[string]any{"name": "x"})
}

func TestBroadcastPaceReview(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.DailyRecipients = 100 })
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribeMany(t, "r", 15)
	id := bt.draft(t, map[string]any{"pace": map[string]any{"per_month": 1}})
	review := bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	it := reviewItem(review, "pace")
	if it["status"] != "blocking" || !strings.Contains(it["detail"].(string), "about 420 days") || review["ready"] != false ||
		review["duration_seconds"] != float64(14*30*86400) {
		t.Fatalf("review above 365 days: %v", review)
	}
	r := bt.owner.expect(http.StatusConflict, "POST", "/v1/broadcasts/"+id+"/schedule", nil)
	if r.str("code") != "review_failed" || !strings.Contains(string(r.raw), `"pace"`) {
		t.Fatalf("schedule: %s", r.raw)
	}
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"pace": map[string]any{"per_month": 15}})
	review = bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	if it := reviewItem(review, "pace"); it["status"] != "ok" || it["detail"] != "about 28 days" || review["duration_seconds"] != float64(14*2*86400) {
		t.Fatalf("review under 365 days: %v", review)
	}

	bt.subscribeMany(t, "s", 235)
	bt.owner.expect(http.StatusOK, "PATCH", "/v1/broadcasts/"+id, map[string]any{"pace": map[string]any{}})
	review = bt.owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+id+"/review", nil).body
	now := h.clock.Now()
	next := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day()+1, 0, 0, 0, 0, time.UTC)
	want := math.Ceil(next.Sub(now).Seconds() + 86400)
	if review["days"] != float64(3) || review["duration_seconds"] != want || reviewItem(review, "pace")["status"] != "ok" {
		t.Fatalf("duration under the daily limit: %v, want %v", review, want)
	}
}

func TestBroadcastPaceAndDailyLimit(t *testing.T) {
	t.Parallel()
	h, bs := newBroadcastHarness(t, func(d *api.Deps) { d.Broadcasts.DailyRecipients = 2 })
	now := h.clock.Now().UTC()
	h.clock.Advance(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 10, 0, 0, time.UTC).Sub(now))
	bt := newBroadcastTeam(t, h, bs)
	bt.subscribe(t, "d1@example.org", "d2@example.org", "d3@example.org")
	id := bt.draft(t, map[string]any{"pace": map[string]any{"per_hour": 60}})
	bt.schedule(t, id)
	bt.run(t)
	h.clock.Advance(time.Minute)
	bt.run(t)
	if bs.count() != 2 {
		t.Fatalf("two mails a minute apart: %d", bs.count())
	}
	slot := h.clock.Now().Add(time.Minute).Truncate(time.Microsecond)
	h.clock.Advance(time.Minute)
	bt.run(t)
	b := bt.get(t, id)
	if bs.count() != 2 || b["waiting_until"] == nil || !timeOf(t, b["pace_next_at"]).Equal(slot) {
		t.Fatalf("daily limit with a pace: %d %v", bs.count(), b)
	}
	h.clock.Advance(24 * time.Hour)
	if wait := bt.run(t); wait >= 0 {
		t.Fatalf("send left work: %v", wait)
	}
	if b := bt.get(t, id); bs.count() != 3 || b["state"] != "sent" || b["waiting_until"] != nil {
		t.Fatalf("next day: %d %v", bs.count(), b)
	}
}

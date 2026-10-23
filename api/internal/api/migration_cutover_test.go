package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/email"
)

const testForwardingDomain = "forward.example.net"

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// deliver puts a message into a folder of the fixture mailbox as the mail server would, received
// at the given time, and waits a second so the server notices the folder changed.
func (d *dovecotRun) deliver(t *testing.T, box, folder string, raw string, received time.Time) {
	t.Helper()
	dir := filepath.Join(d.fixtures, box, "Maildir")
	if folder != "INBOX" {
		dir = filepath.Join(dir, "."+folder)
	}
	name := filepath.Join(dir, "cur", fmt.Sprintf("%d.M%dP2.cutover,S=%d:2,", received.Unix(), time.Now().UnixNano(), len(raw)))
	if err := os.WriteFile(name, []byte(raw), 0o666); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(name, 0o666)
	_ = os.Chtimes(name, received, received)
	time.Sleep(1100 * time.Millisecond)
}

func mailText(id, from, to, subject string, at time.Time, extra, body string) string {
	return "Message-ID: <" + id + ">\r\nFrom: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nDate: " + at.Format(time.RFC1123Z) +
		"\r\n" + extra + "\r\n" + body + "\r\n"
}

// syncedMigration moves the fixture mailbox user into a new e-mail channel with a greeting until
// the copy ended and the migration syncs.
func (d *dovecotRun) syncedMigration(t *testing.T, user string) (string, string, string, string, expectedMailbox) {
	t.Helper()
	want := expectedFixture(t, d.fixtures, user)
	box := copyMailbox(t, d.fixtures, user)
	address := unique("support") + "@example.com"
	channel := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+d.tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Support mail",
		"email": map[string]any{"address": address, "display_name": "Support",
			"smtp":       map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
			"auto_reply": map[string]any{"enabled": true, "text": "We got your mail and answer within a day."}},
	}).str("id")
	id, _ := d.connect(t, box, want.TeamAddresses, channel)
	d.tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/scan", nil)
	if err := d.work(t, context.Background()); err != nil {
		t.Fatal(err)
	}
	d.tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/start", nil)
	if err := d.work(t, context.Background()); err != nil {
		t.Fatal(err)
	}
	m := d.migration(id)
	if m.str("state") != "syncing" || m.str("live_from") == "" || m.str("synced_at") == "" {
		t.Fatalf("after the copy: %s", m.raw)
	}
	return id, box, channel, address, want
}

func (d *dovecotRun) liveFrom(t *testing.T, id string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339Nano, d.migration(id).str("live_from"))
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// sendProbe asks for a test message and has the job send it, returning what it sent.
func (d *dovecotRun) sendProbe(t *testing.T, id string) string {
	t.Helper()
	d.tm.owner.expect(http.StatusAccepted, "POST", "/v1/migrations/"+id+"/probe", nil)
	jobs := d.h.jobs("migration_probe", d.tm.ws)
	if len(jobs) == 0 {
		t.Fatal("no probe job")
	}
	j := jobs[len(jobs)-1]
	var a api.MigrationProbeArgs
	if err := json.Unmarshal(j.args, &a); err != nil {
		t.Fatal(err)
	}
	before := len(d.h.smtp.sent)
	if err := d.h.srv.SendMigrationProbeMail(context.Background(), a, false); err != nil {
		t.Fatal(err)
	}
	d.h.dropJob(j.id)
	d.h.smtp.mu.Lock()
	defer d.h.smtp.mu.Unlock()
	if len(d.h.smtp.sent) != before+1 {
		t.Fatal("the probe was not sent")
	}
	return string(d.h.smtp.sent[len(d.h.smtp.sent)-1].raw)
}

func TestDovecotMigrationCutover(t *testing.T) {
	logs := &logBuffer{}
	d := newDovecotRunWith(t, func(dp *api.Deps) {
		dp.Ingress.ForwardingDomain = testForwardingDomain
		dp.Log = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	h, tm := d.h, d.tm
	ctx := context.Background()
	rcv := newReceiver(t)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL, "events": []string{"conversation.created", "message.created"}})

	id, box, channel, address, want := d.syncedMigration(t, "acme")
	path := "/v1/migrations/" + id
	forward := tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+channel, nil).str("forwarding_address")
	if !regexp.MustCompile(`^[a-z0-9]{16}@`+regexp.QuoteMeta(testForwardingDomain)+`$`).MatchString(forward) || d.migration(id).str("forwarding_address") != forward {
		t.Fatalf("forwarding address %q", forward)
	}

	other := newTeam(t, h)
	for _, c := range []string{"probe", "finish", "remove-history"} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/"+c, nil)
		tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", path+"/"+c, nil)
	}
	tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/finish", nil)
	tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/remove-history", nil)

	liveFrom := d.liveFrom(t, id)
	var lastEvent int64
	_ = h.st.Pool.QueryRow(ctx, "SELECT coalesce(max(id), 0) FROM events WHERE workspace_id = $1", tm.ws).Scan(&lastEvent)
	sideJobs := func(kind string) int {
		return h.count("SELECT count(*) FROM river_job WHERE args->>'workspace_id' = $1 AND kind = $2", tm.ws, kind)
	}
	fanoutBefore, notifyBefore := sideJobs("webhook_fanout"), sideJobs("notify")

	live := mailText("live-after-cutover@example.org", "Nina Stone <nina@example.org>", address, "New after the move", liveFrom.Add(time.Minute), "", "Hello after the move.")
	d.deliver(t, box, "INBOX", live, liveFrom.Add(time.Minute))
	late := mailText("late-history@example.org", "Omar Faruk <omar@example.org>", address, "Sent before the switch", liveFrom.Add(-time.Hour), "", "This arrived during the copy.")
	d.deliver(t, box, "INBOX", late, liveFrom.Add(-time.Hour))
	h.clock.Advance(3 * time.Minute)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	reply := mailText("reply-from-gmail@example.com", "Deniz Kaya <deniz@example.com>", "Nina Stone <nina@example.org>", "Re: New after the move",
		liveFrom.Add(2*time.Minute), "In-Reply-To: <live-after-cutover@example.org>\r\nReferences: <live-after-cutover@example.org>\r\n", "We are on it.")
	d.deliver(t, box, "Sent", reply, liveFrom.Add(2*time.Minute))
	h.clock.Advance(3 * time.Minute)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}

	var liveConv string
	var liveImported, liveMigration bool
	if err := h.st.Pool.QueryRow(ctx, `SELECT e.conversation_id, e.imported_at IS NOT NULL, c.migration_id IS NOT NULL FROM message_emails e
JOIN conversations c ON c.id = e.conversation_id WHERE e.workspace_id = $1 AND e.header_message_id = 'live-after-cutover@example.org'`, tm.ws).
		Scan(&liveConv, &liveImported, &liveMigration); err != nil {
		t.Fatalf("new mail found by sync was not stored: %v", err)
	}
	if liveImported || liveMigration {
		t.Fatal("new mail found by sync was stored as history")
	}
	if n := h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND id > $2 AND type = 'message.created' AND conversation_id = $3", tm.ws, lastEvent, liveConv); n != 2 {
		t.Fatalf("%d message.created events in the new conversation, want the customer's and the teammate's", n)
	}
	if sideJobs("webhook_fanout") <= fanoutBefore || sideJobs("notify") <= notifyBefore {
		t.Fatal("new mail found by sync queued no webhook or notification")
	}
	if n := h.count("SELECT count(*) FROM messages WHERE conversation_id = $1 AND author_type = 'system'", liveConv); n != 0 {
		t.Fatalf("new mail found by sync got the greeting (%d)", n)
	}
	var replyDir string
	var replyImported bool
	if err := h.st.Pool.QueryRow(ctx, `SELECT m.direction, m.imported_at IS NOT NULL FROM message_emails e JOIN messages m ON m.id = e.message_id
WHERE e.workspace_id = $1 AND e.header_message_id = 'reply-from-gmail@example.com' AND e.conversation_id = $2`, tm.ws, liveConv).Scan(&replyDir, &replyImported); err != nil {
		t.Fatalf("the teammate's reply from the old mailbox: %v", err)
	}
	if replyDir != "out" || !replyImported || sideJobs("email_send") != 0 {
		t.Fatalf("the teammate's reply: %s imported %v, %d sends", replyDir, replyImported, sideJobs("email_send"))
	}
	var lateConv string
	if err := h.st.Pool.QueryRow(ctx, `SELECT e.conversation_id FROM message_emails e JOIN conversations c ON c.id = e.conversation_id
WHERE e.workspace_id = $1 AND e.header_message_id = 'late-history@example.org' AND e.imported_at IS NOT NULL AND c.migration_id = $2`, tm.ws, id).Scan(&lateConv); err != nil {
		t.Fatalf("mail received before the migration went live was not imported as history: %v", err)
	}
	if n := h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND conversation_id = $2", tm.ws, lateConv); n != 0 {
		t.Fatalf("history found by sync wrote %d events", n)
	}

	if r := h.ingest(forward, []byte(live), nil); r.status != http.StatusOK || r.str("status") != "duplicate" {
		t.Fatalf("the same mail at the forwarding address: %d %v", r.status, r.body)
	}
	both := mailText("both-ways@example.org", "Pia Lund <pia@example.org>", address, "Forwarded first", h.clock.Now(), "", "Which copy wins?")
	if r := h.ingest(forward, []byte(both), nil); r.status != http.StatusAccepted || r.str("status") != "stored" {
		t.Fatalf("new mail at the forwarding address: %d %v", r.status, r.body)
	}
	d.deliver(t, box, "INBOX", both, h.clock.Now())
	h.clock.Advance(3 * time.Minute)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	for _, mid := range []string{"live-after-cutover@example.org", "both-ways@example.org", "reply-from-gmail@example.com"} {
		if n := h.count("SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND header_message_id = $2", tm.ws, mid); n != 1 {
			t.Fatalf("%s stored %d times", mid, n)
		}
	}
	unknown := "zzzzzzzzzzzzzzzz@" + testForwardingDomain
	if r := h.ingest(unknown, []byte(both), nil); r.status != http.StatusNotFound {
		t.Fatalf("an unknown forwarding token: %d", r.status)
	}

	messages := func() int { return h.count("SELECT count(*) FROM messages WHERE workspace_id = $1", tm.ws) }
	probe := d.sendProbe(t, id)
	tm.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", path+"/probe", nil)
	if !strings.Contains(probe, email.MigrationProbeHeader+": "+id+".") || !strings.Contains(probe, "To: <"+address+">") && !strings.Contains(probe, "To: "+address) {
		t.Fatalf("probe:\n%s", probe)
	}
	before := messages()
	d.deliver(t, box, "INBOX", probe, h.clock.Now())
	h.clock.Advance(3 * time.Minute)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := d.migration(id)
	if m.str("probe_synced_at") == "" || m.body["verified_at"] != nil || messages() != before {
		t.Fatalf("a probe found by sync: %s", m.raw)
	}
	forged := regexp.MustCompile(`(`+email.MigrationProbeHeader+`: [0-9a-f-]+\.)[0-9a-f]+`).ReplaceAllString(probe, "${1}00")
	h.ingest(forward, []byte(forged), nil)
	if d.migration(id).body["verified_at"] != nil {
		t.Fatal("a probe with a wrong HMAC verified delivery")
	}
	if r := h.ingest(forward, []byte(probe), nil); r.status != http.StatusAccepted || r.str("status") != "dropped" {
		t.Fatalf("the probe at the forwarding address: %d %v", r.status, r.body)
	}
	m = d.migration(id)
	if m.str("verified_at") == "" || m.str("verified_route") != "forwarding" || m.str("quiet_until") == "" || messages() != before {
		t.Fatalf("after the probe: %s", m.raw)
	}

	confirmation, err := os.ReadFile("../email/testdata/forwarding/gmail_confirmation.eml")
	if err != nil {
		t.Fatal(err)
	}
	conversations := func() int { return h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1", tm.ws) }
	convBefore := conversations()
	raw := strings.ReplaceAll(string(confirmation), "k3v9xq2m8w4t7b1c@forward.example.net", forward)
	if r := h.ingest(forward, []byte(raw), nil); r.status != http.StatusAccepted || r.str("status") != "dropped" {
		t.Fatalf("Google's confirmation: %d %v", r.status, r.body)
	}
	fc, _ := d.migration(id).body["forwarding_confirmation"].(map[string]any)
	if fc["code"] != "462781234" || fc["provider"] != "google" || !strings.HasPrefix(fc["link"].(string), "https://mail-settings.google.com/mail/vf-") || conversations() != convBefore {
		t.Fatalf("forwarding confirmation: %v", fc)
	}
	spoofed := strings.Replace(raw, "dmarc=pass", "dmarc=fail", 1)
	spoofed = strings.Replace(spoofed, "462781234", "111111111", -1)
	h.ingest(forward, []byte(spoofed), nil)
	if fc, _ := d.migration(id).body["forwarding_confirmation"].(map[string]any); fc["code"] != "462781234" {
		t.Fatalf("a confirmation failing DMARC was taken: %v", fc)
	}

	finished := tm.owner.expect(http.StatusOK, "POST", path+"/finish", nil)
	if finished.str("state") != "completed" || finished.str("credentials_deleted_at") == "" || h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1", id) != 0 {
		t.Fatalf("finish: %s", finished.raw)
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/finish", nil)
	if n := h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1 AND action IN ('finished', 'credentials_deleted', 'delivery_verified', 'probe_sent')", id); n != 4 {
		t.Fatalf("%d audit entries for finishing", n)
	}
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(d.h.jobs("migration_copy", tm.ws)) + len(d.h.jobs("migration_sync", tm.ws)); n != 0 {
		t.Fatalf("%d migration jobs left after finishing", n)
	}

	var kept string
	if err := h.st.Pool.QueryRow(ctx, "SELECT id FROM conversations WHERE workspace_id = $1 AND migration_id = $2 AND id <> $3 ORDER BY id LIMIT 1", tm.ws, id, lateConv).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+kept+"/messages", map[string]any{"kind": "note", "body": "Followed up after the move."})
	m = d.migration(id)
	total, removable := num(m.body["imported_conversations"]), num(m.body["removable_conversations"])
	if total != want.Conversations+1 || removable != total-1 {
		t.Fatalf("imported %d, removable %d", total, removable)
	}
	tm.owner.expectProblem(http.StatusConflict, "confirm_count_mismatch", "POST", path+"/remove-history", map[string]any{"confirm_count": removable + 1})
	if r := tm.owner.expect(http.StatusAccepted, "POST", path+"/remove-history", map[string]any{"confirm_count": removable}); r.str("state") != "removing" {
		t.Fatalf("remove history: %s", r.raw)
	}
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m = d.migration(id)
	if m.str("state") != "removed" || num(m.body["imported_conversations"]) != 1 {
		t.Fatalf("after removing: %s", m.raw)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", tm.ws, id); n != 1 {
		t.Fatalf("%d imported conversations left, want the one written in", n)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE id = $1", liveConv); n != 1 {
		t.Fatal("removing history took the conversation of new mail")
	}
	if n := h.count(`SELECT count(*) FROM contacts ct WHERE ct.workspace_id = $1 AND ct.migration_id = $2
AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.contact_id = ct.id) AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.author_contact_id = ct.id)`, tm.ws, id); n != 0 {
		t.Fatalf("%d contacts the migration created are left without anything", n)
	}
	checkNoOrphans(t, d)

	for _, q := range []string{
		"SELECT count(*) FROM migration_errors WHERE workspace_id = $1 AND detail LIKE '%' || $2 || '%'",
		"SELECT count(*) FROM migration_audit WHERE workspace_id = $1 AND detail::text LIKE '%' || $2 || '%'",
	} {
		if n := h.count(q, tm.ws, d.pass); n != 0 {
			t.Fatalf("the mailbox password is stored: %s", q)
		}
	}
	if strings.Contains(logs.String(), d.pass) {
		t.Fatal("the mailbox password is in the log")
	}
}

func TestDovecotMigrationSyncEndsOnItsOwn(t *testing.T) {
	d := newDovecotRun(t)
	key := d.h.client()
	key.bearer = d.tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "mover"}).str("secret")
	d.tm.owner = key
	h, tm := d.h, d.tm
	ctx := context.Background()

	quiet, _, channel, address, _ := d.syncedMigration(t, "yerel")
	if d.migration(quiet).body["forwarding_address"] != nil || tm.owner.expect(http.StatusOK, "GET", "/v1/channels/"+channel, nil).body["forwarding_address"] != nil {
		t.Fatal("a forwarding address without a forwarding domain")
	}
	probe := d.sendProbe(t, quiet)
	if r := h.ingest(address, []byte(probe), nil); r.status != http.StatusAccepted || r.str("status") != "dropped" {
		t.Fatalf("the probe at the channel's address: %d %v", r.status, r.body)
	}
	if m := d.migration(quiet); m.str("verified_route") != "direct" {
		t.Fatalf("verified: %s", m.raw)
	}
	h.clock.Advance(23 * time.Hour)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	if s := d.migration(quiet).str("state"); s != "syncing" {
		t.Fatalf("finished before the quiet period ended: %s", s)
	}
	h.clock.Advance(2 * time.Hour)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := d.migration(quiet)
	if m.str("state") != "completed" || !strings.Contains(m.str("state_reason"), "quiet") || h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1", quiet) != 0 {
		t.Fatalf("after the quiet period: %s", m.raw)
	}

	limited, _, _, _, _ := d.syncedMigration(t, "acme")
	tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", "/v1/migrations/"+limited+"/finish", map[string]any{"force": false})
	h.clock.Advance(31 * 24 * time.Hour)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m = d.migration(limited)
	if m.str("state") != "completed" || !strings.Contains(m.str("state_reason"), "limit") || m.str("credentials_deleted_at") == "" ||
		h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1", limited) != 0 {
		t.Fatalf("after the sync limit: %s", m.raw)
	}

	imported := num(m.body["counts"].(map[string]any)["imported"])
	if err := h.srv.CleanMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(31 * 24 * time.Hour)
	if err := h.srv.CleanMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM migration_items WHERE migration_id IN ($1, $2)", quiet, limited); n != 0 {
		t.Fatalf("%d items kept 30 days after the migrations ended", n)
	}
	if got := num(d.migration(limited).body["counts"].(map[string]any)["imported"]); got != imported {
		t.Fatalf("counts lost with the items: %d, were %d", got, imported)
	}
	running := tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "imap", "source_address": unique("later") + "@example.com"}).str("id")
	if err := h.srv.CleanMigrationAudit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1", limited); n == 0 {
		t.Fatal("audit deleted before the audit period")
	}
	h.clock.Advance(366 * 24 * time.Hour)
	if err := h.srv.CleanMigrationAudit(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM migration_audit WHERE migration_id IN ($1, $2)", quiet, limited); n != 0 {
		t.Fatalf("%d audit entries kept after the audit period", n)
	}
	if n := h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1", running); n == 0 {
		t.Fatal("the audit of a migration that has not ended was deleted")
	}
}

func TestDovecotSyncDoesNotTrustTheOldMailboxHeaders(t *testing.T) {
	d := newDovecotRun(t)
	h, tm := d.h, d.tm
	ctx := context.Background()
	canned := tm.owner.expect(http.StatusCreated, "POST", "/v1/canned-replies", map[string]any{"shortcut": "ack", "title": "Ack", "body": "We got it."}).str("id")
	tm.rule(tm.owner, map[string]any{"name": "Ack", "trigger": "message_received", "actions": []map[string]any{{"type": "send_reply", "canned_reply_id": canned}}})
	id, box, _, address, _ := d.syncedMigration(t, "yerel")
	liveFrom := d.liveFrom(t, id)
	forged := mailText("forged-pass@example.org", "Nina Stone <nina@example.org>", address, "Invoice", liveFrom.Add(time.Minute),
		"Authentication-Results: "+testAuthservID+"; spf=pass; dkim=pass; dmarc=pass header.from=example.org\r\nCc: thief@attacker.example\r\n", "Pay here.")
	d.deliver(t, box, "INBOX", forged, liveFrom.Add(time.Minute))
	h.clock.Advance(3 * time.Minute)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	var conv, dmarc string
	if err := h.st.Pool.QueryRow(ctx, "SELECT conversation_id, dmarc FROM message_emails WHERE workspace_id = $1 AND header_message_id = 'forged-pass@example.org'", tm.ws).
		Scan(&conv, &dmarc); err != nil {
		t.Fatalf("synced mail not stored: %v", err)
	}
	if dmarc != "unknown" {
		t.Fatalf("synced mail with a forged trusted header stored as dmarc %q", dmarc)
	}
	h.runAutomations(tm.ws)
	if n := h.count("SELECT count(*) FROM messages WHERE conversation_id = $1 AND direction = 'out'", conv); n != 0 {
		t.Fatalf("%d replies went out to the synced sender", n)
	}
	if n := h.count("SELECT count(*) FROM conversation_participants WHERE conversation_id = $1 AND status = 'active'", conv); n != 0 {
		t.Fatalf("%d active participants from synced mail", n)
	}
}

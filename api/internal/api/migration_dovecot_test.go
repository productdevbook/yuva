package api_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/storage"
)

// dovecotTarget reads YUVA_TEST_IMAP_URL (imaps://:password@host:port) and
// YUVA_TEST_IMAP_FIXTURES, the directory the migrate-test Dovecot serves, written by
// internal/migrate/testdata/gen. Each test copies a mailbox there under a new user, so it can
// change it without touching what other tests read.
func dovecotTarget(t *testing.T) (host string, port int, password, fixtures string) {
	t.Helper()
	raw, dir := os.Getenv("YUVA_TEST_IMAP_URL"), os.Getenv("YUVA_TEST_IMAP_FIXTURES")
	if raw == "" || dir == "" {
		t.Skip("YUVA_TEST_IMAP_URL and YUVA_TEST_IMAP_FIXTURES are not set")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "imaps" {
		t.Fatalf("YUVA_TEST_IMAP_URL must be imaps://: %v", err)
	}
	password, _ = u.User.Password()
	h, p, _ := net.SplitHostPort(u.Host)
	port, _ = strconv.Atoi(p)
	return h, port, password, dir
}

// copyMailbox copies a fixture user's messages, without Dovecot's own files, to a new user.
func copyMailbox(t *testing.T, fixtures, from string) string {
	t.Helper()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	user := from + "-" + hex.EncodeToString(b)
	src, dst := filepath.Join(fixtures, from), filepath.Join(fixtures, user)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			if err := os.MkdirAll(target, 0o777); err != nil {
				return err
			}
			return os.Chmod(target, 0o777)
		case strings.HasPrefix(d.Name(), "dovecot"), strings.Contains(rel, "rawlog"):
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o666); err != nil {
			return err
		}
		if err := os.Chmod(target, 0o666); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(dst, "Maildir")) })
	return user
}

// imapSnapshot reads every folder's counters and every message's UID, flags and modseq over a
// plain connection of its own, so the importer's client is not involved.
func imapSnapshot(t *testing.T, addr, user, pass string) map[string][]string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Minute))
	br := bufio.NewReader(conn)
	readLine := func() string {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimRight(line, "\r\n")
		if i := strings.LastIndex(line, "{"); i >= 0 && strings.HasSuffix(line, "}") {
			if n, err := strconv.Atoi(line[i+1 : len(line)-1]); err == nil {
				lit := make([]byte, n)
				if _, err := io.ReadFull(br, lit); err != nil {
					t.Fatal(err)
				}
				rest, _ := br.ReadString('\n')
				line = line[:i] + strconv.Quote(string(lit)) + strings.TrimRight(rest, "\r\n")
			}
		}
		return line
	}
	readLine()
	n := 0
	cmd := func(c string) []string {
		n++
		tag := fmt.Sprintf("S%d", n)
		if _, err := conn.Write([]byte(tag + " " + c + "\r\n")); err != nil {
			t.Fatal(err)
		}
		var out []string
		for {
			l := readLine()
			if strings.HasPrefix(l, tag+" ") {
				if !strings.HasPrefix(l, tag+" OK") {
					t.Fatalf("%s: %s", c, l)
				}
				return out
			}
			out = append(out, l)
		}
	}
	cmd(fmt.Sprintf("LOGIN %q %q", user, pass))
	out := map[string][]string{}
	for _, l := range cmd(`LIST "" "*"`) {
		i := strings.LastIndex(l, `"/" `)
		if i < 0 || strings.Contains(l, `\Noselect`) || strings.Contains(l, `\NonExistent`) {
			continue
		}
		name := l[i+4:]
		status := cmd("STATUS " + name + " (MESSAGES UIDNEXT UIDVALIDITY HIGHESTMODSEQ)")
		cmd("EXAMINE " + name + " (CONDSTORE)")
		rows := cmd("UID FETCH 1:* (UID FLAGS MODSEQ)")
		sort.Strings(rows)
		out[name] = append(status, rows...)
	}
	cmd("LOGOUT")
	return out
}

// rawlogCommands lists the commands every session of user sent after signing in, from Dovecot's
// rawlog (a timestamp, the tag, the command), or nil when the files are not readable here:
// Dovecot writes them as its own user, and CI checks them with sudo.
func rawlogCommands(t *testing.T, fixtures, user string) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(fixtures, user, "rawlog", "*.in"))
	var out []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if errors.Is(err, fs.ErrPermission) {
			t.Logf("rawlog %s is not readable here: %v", f, err)
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			name := strings.ToUpper(fields[2])
			if name == "UID" && len(fields) > 3 {
				name += " " + strings.ToUpper(fields[3])
			}
			out = append(out, name)
		}
	}
	return out
}

type expectedThread struct {
	Subject            string    `json:"subject"`
	Contact            string    `json:"contact"`
	In                 int       `json:"in"`
	Out                int       `json:"out"`
	Authors            []string  `json:"authors"`
	Labels             []string  `json:"labels"`
	LastAt             time.Time `json:"last_at"`
	AttachmentsKept    int       `json:"attachments_kept"`
	AttachmentsSkipped int       `json:"attachments_skipped"`
	SynthesizedID      bool      `json:"synthesized_message_id"`
	DateFromServer     bool      `json:"date_from_server"`
}

type expectedMailbox struct {
	User          string `json:"user"`
	Address       string `json:"address"`
	TeamAddresses []string
	Folders       []struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Messages int    `json:"messages"`
		Read     bool   `json:"read"`
	} `json:"folders"`
	Conversations int              `json:"conversations"`
	MessagesIn    int              `json:"messages_in"`
	MessagesOut   int              `json:"messages_out"`
	Contacts      []string         `json:"contacts"`
	Labels        []string         `json:"labels"`
	Skipped       map[string]int   `json:"skipped"`
	Threads       []expectedThread `json:"threads"`
}

func expectedFixture(t *testing.T, fixtures, user string) expectedMailbox {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtures, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Mailboxes []struct {
			expectedMailbox
			TeamAddresses []string `json:"team_addresses"`
		} `json:"mailboxes"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, m := range doc.Mailboxes {
		if m.User == user {
			out := m.expectedMailbox
			out.TeamAddresses = m.TeamAddresses
			return out
		}
	}
	t.Fatalf("expected.json has no mailbox %s", user)
	return expectedMailbox{}
}

// killableStorage fails one write and cancels the copy's context when told, as if the process
// died in the middle of a batch.
type killableStorage struct {
	storage.Storage
	mu     sync.Mutex
	puts   int
	killAt int
	kill   func()
}

func (k *killableStorage) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) error {
	k.mu.Lock()
	k.puts++
	dead := k.killAt > 0 && k.puts == k.killAt
	k.mu.Unlock()
	if dead {
		k.kill()
		return context.Canceled
	}
	return k.Storage.Put(ctx, key, r, size, ct)
}

func (k *killableStorage) arm(at int, kill func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.puts, k.killAt, k.kill = 0, at, kill
}

type dovecotRun struct {
	h        *harness
	tm       team
	host     string
	port     int
	pass     string
	fixtures string
	objects  *killableStorage
}

func newDovecotRun(t *testing.T) *dovecotRun {
	return newDovecotRunWith(t, nil)
}

func newDovecotRunWith(t *testing.T, configure func(*api.Deps)) *dovecotRun {
	host, port, pass, fixtures := dovecotTarget(t)
	objects := &killableStorage{}
	h := newHarnessWith(t, func(d *api.Deps) {
		if configure != nil {
			defer configure(d)
		}
		d.Migrations = api.MigrationSettings{AllowPrivate: true, TestTimeout: 10 * time.Second}
		d.Webhooks.AllowPrivate = true
		d.Attachments = api.AttachmentSettings{MaxBytes: 25 << 20, Types: []string{
			"image/png", "image/jpeg", "image/gif", "application/pdf", "text/plain", "text/csv", "application/zip", "video/mp4",
		}}
		objects.Storage = d.Storage
		d.Storage = objects
	})
	return &dovecotRun{h: h, tm: newTeam(t, h), host: host, port: port, pass: pass, fixtures: fixtures, objects: objects}
}

func (d *dovecotRun) addr() string { return net.JoinHostPort(d.host, strconv.Itoa(d.port)) }

// connect creates a migration of user's mailbox into a new e-mail channel of the team's inbox
// and connects it, trusting Dovecot's self-signed certificate.
func (d *dovecotRun) connect(t *testing.T, user string, team []string, channel string) (string, string) {
	t.Helper()
	address := unique("support") + "@example.com"
	if channel == "" {
		channel = d.tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+d.tm.inbox+"/channels", map[string]any{
			"kind": "email", "name": "Support mail",
			"email": map[string]any{"address": address, "display_name": "Support",
				"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"}},
		}).str("id")
	} else {
		address = d.channelAddress(t, channel)
	}
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"preset": "imap", "source_address": address, "channel_id": channel, "options": map[string]any{"team_addresses": team},
	}).str("id")
	d.enterCredentials(t, id, user)
	return id, channel
}

// enterCredentials does what the person does on the connect page: enters the mailbox's password
// and trusts Dovecot's self-signed certificate.
func (d *dovecotRun) enterCredentials(t *testing.T, id, user string) {
	t.Helper()
	path := "/v1/migrations/" + id
	creds := map[string]any{"username": user, "password": d.pass, "host": d.host, "port": d.port}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	creds["certificate_sha256"] = res.str("certificate_sha256")
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != true {
		t.Fatalf("connection test: %s", res.raw)
	}
}

// connectGmail connects user's mailbox as connect does, then makes it a Google Workspace
// migration, whose preset only reaches imap.gmail.com, and tests the connection again so the
// folders are listed as Gmail's.
func (d *dovecotRun) connectGmail(t *testing.T, user string, team []string) string {
	t.Helper()
	id, _ := d.connect(t, user, team, "")
	if _, err := d.h.st.Pool.Exec(context.Background(), "UPDATE migrations SET preset = 'google_workspace' WHERE workspace_id = $1 AND id = $2", d.tm.ws, id); err != nil {
		t.Fatal(err)
	}
	if res := d.tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/test", nil); res.body["ok"] != true {
		t.Fatalf("connection test: %s", res.raw)
	}
	return id
}

func (d *dovecotRun) channelAddress(t *testing.T, channel string) string {
	var addr string
	if err := d.h.st.Pool.QueryRow(context.Background(), "SELECT address FROM email_channels WHERE channel_id = $1", channel).Scan(&addr); err != nil {
		t.Fatal(err)
	}
	return addr
}

// work runs the workspace's queued migration jobs slice by slice, as River would, until none is
// left or every one left waits, as a syncing migration's job does between passes; a slice that
// ends with an error leaves its job, as a dead process would.
func (d *dovecotRun) work(t *testing.T, ctx context.Context) error {
	t.Helper()
	waiting := map[int64]bool{}
	for range 500 {
		var jobs []riverJob
		for _, kind := range []string{"migration_scan", "migration_copy", "migration_sync", "migration_remove"} {
			jobs = append(jobs, d.h.jobs(kind, d.tm.ws)...)
		}
		jobs = slices.DeleteFunc(jobs, func(j riverJob) bool { return waiting[j.id] || d.jobState(j.id) == "running" })
		if len(jobs) == 0 {
			return nil
		}
		j := jobs[0]
		var a api.MigrationJobArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		res, err := d.h.srv.RunMigrationSlice(ctx, a, j.id, 20*time.Second)
		if err != nil {
			d.setJobState(j.id, "running")
			return err
		}
		switch {
		case res.Done:
			d.h.dropJob(j.id)
		case res.Until.After(d.h.clock.Now()):
			waiting[j.id] = true
		}
	}
	t.Fatal("the migration did not finish in 500 slices")
	return nil
}

func (d *dovecotRun) jobState(id int64) string {
	var s string
	if err := d.h.st.Pool.QueryRow(context.Background(), "SELECT state FROM river_job WHERE id = $1", id).Scan(&s); err != nil {
		d.h.t.Fatal(err)
	}
	return s
}

func (d *dovecotRun) setJobState(id int64, state string) {
	if _, err := d.h.st.Pool.Exec(context.Background(), "UPDATE river_job SET state = $2, attempted_at = now() WHERE id = $1", id, state); err != nil {
		d.h.t.Fatal(err)
	}
}

func (d *dovecotRun) migration(id string) response {
	return d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/"+id, nil)
}

func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}

func checkDryRun(t *testing.T, est map[string]any, want expectedMailbox) {
	t.Helper()
	sk := est["skipped"].(map[string]any)
	if num(est["conversations"]) != want.Conversations || num(est["messages_in"]) != want.MessagesIn || num(est["messages_out"]) != want.MessagesOut ||
		num(est["contacts_new"]) != len(want.Contacts) || num(est["contacts_existing"]) != 0 {
		t.Errorf("%s dry run totals: %v", want.User, est)
	}
	for reason, n := range want.Skipped {
		if num(sk[reason]) != n {
			t.Errorf("%s skipped %s: %v, want %d", want.User, reason, sk[reason], n)
		}
	}
	for reason, v := range sk {
		if _, ok := want.Skipped[reason]; !ok && num(v) != 0 {
			t.Errorf("%s skipped %s: %v, want 0", want.User, reason, v)
		}
	}
	var labels []string
	for _, l := range est["labels"].([]any) {
		labels = append(labels, l.(map[string]any)["name"].(string))
	}
	if !reflect.DeepEqual(labels, want.Labels) && !(len(labels) == 0 && len(want.Labels) == 0) {
		t.Errorf("%s labels %v, want %v", want.User, labels, want.Labels)
	}
}

func TestDovecotMigrationDryRunsMatchExpected(t *testing.T) {
	d := newDovecotRun(t)
	ctx := context.Background()
	suggestions := map[string][]string{
		"acme":  {"deniz@example.com Deniz Kaya 1", "sales@example.com Acme Sales 1"},
		"yerel": nil,
		"gmail": {"ece@example.com Ece Tan 2", "billing@example.com Acme Billing 1"},
	}
	for _, user := range []string{"acme", "yerel", "gmail"} {
		want := expectedFixture(t, d.fixtures, user)
		box := copyMailbox(t, d.fixtures, user)
		var id string
		if user == "gmail" {
			id = d.connectGmail(t, box, want.TeamAddresses)
		} else {
			id, _ = d.connect(t, box, want.TeamAddresses, "")
		}
		folders := d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/"+id+"/folders", nil).body["items"].([]any)
		if len(folders) != len(want.Folders) {
			t.Fatalf("%s: %d folders listed after the test, want %d", user, len(folders), len(want.Folders))
		}
		tested := map[string]map[string]any{}
		for _, f := range folders {
			tested[f.(map[string]any)["name"].(string)] = f.(map[string]any)
		}
		for _, f := range want.Folders {
			got := tested[f.Name]
			if got == nil || got["kind"] != f.Kind || num(got["messages"]) != f.Messages || (got["use"] == "import") != f.Read {
				t.Errorf("%s folder %s after the test: %v, want %+v", user, f.Name, got, f)
			}
		}
		if m := d.migration(id); m.body["team_suggestions"] != nil {
			t.Errorf("%s team suggestions before the scan: %s", user, m.raw)
		}
		d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", "/v1/migrations/"+id+"/start", nil)
		d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "GET", "/v1/migrations/"+id+"/preview", nil)
		if s := d.tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/scan", nil).str("state"); s != "scanning" {
			t.Fatalf("scan: %s", s)
		}
		if err := d.work(t, ctx); err != nil {
			t.Fatal(err)
		}
		m := d.migration(id)
		if m.str("state") != "ready" || m.body["estimate"] == nil {
			t.Fatalf("%s after the scan: %s", user, m.raw)
		}
		checkDryRun(t, m.body["estimate"].(map[string]any), want)
		var suggested []string
		for _, sg := range m.body["team_suggestions"].([]any) {
			sg := sg.(map[string]any)
			if sg["address"] == want.Address {
				continue
			}
			suggested = append(suggested, fmt.Sprintf("%s %v %d", sg["address"], sg["name"], num(sg["messages"])))
		}
		if !reflect.DeepEqual(suggested, suggestions[user]) {
			t.Errorf("%s team suggestions %q, want %q", user, suggested, suggestions[user])
		}
		byName := map[string]map[string]any{}
		for _, f := range d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/"+id+"/folders", nil).body["items"].([]any) {
			byName[f.(map[string]any)["name"].(string)] = f.(map[string]any)
		}
		for _, f := range want.Folders {
			got := byName[f.Name]
			if got == nil || got["kind"] != f.Kind || num(got["messages"]) != f.Messages || (got["use"] == "import") != f.Read {
				t.Errorf("%s folder %s: %v, want %+v", user, f.Name, got, f)
			}
		}
		preview := d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/"+id+"/preview", nil).body["items"].([]any)
		newest := want.Threads
		sort.SliceStable(newest, func(a, b int) bool { return newest[a].LastAt.After(newest[b].LastAt) })
		if len(preview) != min(5, len(newest)) {
			t.Fatalf("%s preview: %d threads", user, len(preview))
		}
		for i, p := range preview {
			pt := p.(map[string]any)
			msgs := pt["messages"].([]any)
			if pt["subject"] != newest[i].Subject || pt["contact"].(map[string]any)["address"] != newest[i].Contact || len(msgs) != newest[i].In+newest[i].Out ||
				msgs[0].(map[string]any)["text"] == "" {
				t.Errorf("%s preview %d: %v, want %+v", user, i, pt, newest[i])
			}
		}
	}
}

func TestDovecotMigrationCopiesQuietlyAndResumes(t *testing.T) {
	d := newDovecotRun(t)
	h, tm := d.h, d.tm
	ctx := context.Background()
	want := expectedFixture(t, d.fixtures, "acme")
	box := copyMailbox(t, d.fixtures, "acme")
	before := imapSnapshot(t, d.addr(), box, d.pass)
	rcv := newReceiver(t)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL, "events": []string{"conversation.created", "message.created"}})

	id, channel := d.connect(t, box, want.TeamAddresses, "")
	path := "/v1/migrations/" + id
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", path+"/scan", nil)
	other := newTeam(t, h)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", path, map[string]any{})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", path, map[string]any{})
	for _, c := range []string{"scan", "start", "pause", "resume", "cancel", "retry"} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/"+c, nil)
		tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", path+"/"+c, nil)
	}
	for _, c := range []string{"folders", "preview", "errors"} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/"+c, nil)
		tm.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", path+"/"+c, nil)
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	checkDryRun(t, d.migration(id).body["estimate"].(map[string]any), want)

	since := time.Now().AddDate(0, 0, -360).UTC().Format(time.RFC3339)
	stats := func() map[string]any {
		body := tm.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+url.QueryEscape(since), nil).body
		delete(body, "until")
		return body
	}
	usage := func() map[string]any {
		items := tm.owner.expect(http.StatusOK, "GET", "/v1/usage", nil).body["items"].([]any)
		if len(items) == 0 {
			return map[string]any{}
		}
		return items[0].(map[string]any)
	}
	statsBefore, usageBefore := stats(), usage()
	var lastEvent int64
	_ = h.st.Pool.QueryRow(ctx, "SELECT coalesce(max(id), 0) FROM events WHERE workspace_id = $1", tm.ws).Scan(&lastEvent)
	sideJobs := func() int {
		return h.count("SELECT count(*) FROM river_job WHERE args->>'workspace_id' = $1 AND kind NOT LIKE 'migration%'", tm.ws)
	}
	mailSent := func() int {
		h.smtp.mu.Lock()
		defer h.smtp.mu.Unlock()
		return len(h.smtp.sent)
	}
	deliveries := func() int { return h.count("SELECT count(*) FROM webhook_deliveries WHERE workspace_id = $1", tm.ws) }
	jobsBefore, mailBefore, deliveriesBefore := sideJobs(), mailSent(), deliveries()

	duplicates := want.Skipped["duplicate_copies"]
	skipped := want.Skipped["automatic"] + want.Skipped["internal"] + want.Skipped["outside_range"] + want.Skipped["blocked"] + want.Skipped["too_large"]
	if duplicates == 0 || skipped == 0 {
		t.Fatalf("the fixture has no duplicate or skipped message: %v", want.Skipped)
	}
	started := tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	sc := started.body["counts"].(map[string]any)
	if started.str("state") != "copying" || num(sc["total"]) != want.MessagesIn+want.MessagesOut+duplicates+skipped ||
		num(sc["duplicate"]) != duplicates || num(sc["skipped"]) != skipped {
		t.Fatalf("start: %s", started.raw)
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)

	killCtx, kill := context.WithCancel(ctx)
	d.objects.arm(9, kill)
	if err := d.work(t, killCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("the copy was not killed: %v", err)
	}
	d.objects.arm(0, nil)
	half := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND state = 'imported'", tm.ws, id)
	if half == 0 || half >= want.MessagesIn+want.MessagesOut {
		t.Fatalf("%d messages imported before the kill", half)
	}
	if n, err := h.srv.RunMigrationWatchdog(ctx); err != nil || n != 0 {
		t.Fatalf("the watchdog took over a live lease: %d %v", n, err)
	}
	h.clock.Advance(50 * time.Second)
	if n, err := h.srv.RunMigrationWatchdog(ctx); err != nil || n != 1 {
		t.Fatalf("the watchdog did not resume the migration: %d %v", n, err)
	}
	if n, err := h.srv.RunMigrationWatchdog(ctx); err != nil || n != 0 {
		t.Fatalf("the watchdog queued twice: %d %v", n, err)
	}
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := d.migration(id)
	counts := m.body["counts"].(map[string]any)
	if m.str("state") != "syncing" || num(counts["imported"]) != want.MessagesIn+want.MessagesOut || num(counts["failed"]) != 0 ||
		num(counts["duplicate"]) != duplicates || num(counts["skipped"]) != skipped || num(counts["total"]) != want.MessagesIn+want.MessagesOut+duplicates+skipped {
		t.Fatalf("after the copy: %s", m.raw)
	}
	var folderDuplicates, folderSkipped int
	for _, f := range tm.owner.expect(http.StatusOK, "GET", path+"/folders", nil).body["items"].([]any) {
		folderDuplicates += num(f.(map[string]any)["duplicate"])
		folderSkipped += num(f.(map[string]any)["skipped"])
	}
	if folderDuplicates != duplicates || folderSkipped != skipped {
		t.Fatalf("folders count %d duplicate and %d skipped, want %d and %d", folderDuplicates, folderSkipped, duplicates, skipped)
	}

	if n := h.count("SELECT count(*) FROM (SELECT header_message_id FROM message_emails WHERE workspace_id = $1 GROUP BY header_message_id HAVING count(*) > 1) d", tm.ws); n != 0 {
		t.Fatalf("%d Message-IDs stored twice", n)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", tm.ws, id); n != want.Conversations {
		t.Fatalf("%d conversations, want %d", n, want.Conversations)
	}
	checkImportedThreads(t, d, id, want)
	checkNoOrphans(t, d)

	if n := h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND id > $2 AND type NOT IN ('migration.updated', 'inbox.updated')", tm.ws, lastEvent); n != 0 {
		t.Fatalf("the import wrote %d events", n)
	}
	if n := sideJobs(); n != jobsBefore || mailSent() != mailBefore || deliveries() != deliveriesBefore || len(rcv.take()) != 0 {
		t.Fatalf("the import queued jobs (%d → %d), sent mail or webhooks", jobsBefore, n)
	}
	for _, c := range tm.owner.expect(http.StatusOK, "GET", "/v1/conversations?limit=100", nil).body["items"].([]any) {
		if c.(map[string]any)["unread"] == true {
			t.Fatalf("imported history is unread: %v", c)
		}
	}
	if st := stats(); !reflect.DeepEqual(st, statsBefore) {
		t.Fatalf("/v1/stats changed: %v, before %v", st, statsBefore)
	}
	u := usage()
	if num(u["conversations"]) != num(usageBefore["conversations"]) || num(u["messages"]) != num(usageBefore["messages"]) ||
		num(u["imported_conversations"]) != want.Conversations || num(u["imported_messages"]) != want.MessagesIn+want.MessagesOut || num(u["imported_bytes"]) == 0 {
		t.Fatalf("usage %v, before %v", u, usageBefore)
	}

	again, _ := d.connect(t, box, want.TeamAddresses, channel)
	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+again+"/scan", nil)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	est := d.migration(again).body["estimate"].(map[string]any)
	if num(est["conversations"]) != 0 || num(est["skipped"].(map[string]any)["already_in_yuva"]) != want.MessagesIn+want.MessagesOut {
		t.Fatalf("second run dry run: %v", est)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+again+"/start", nil)
	messagesBefore := h.count("SELECT count(*) FROM messages WHERE workspace_id = $1", tm.ws)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	second := d.migration(again)
	if s := second.str("state"); s != "syncing" || h.count("SELECT count(*) FROM messages WHERE workspace_id = $1", tm.ws) != messagesBefore {
		t.Fatalf("the second run added messages (%s)", s)
	}
	if c := second.body["counts"].(map[string]any); num(c["imported"]) != 0 || num(c["duplicate"]) != want.MessagesIn+want.MessagesOut+duplicates || num(c["skipped"]) != skipped {
		t.Fatalf("second run counts: %v", c)
	}

	cancelled := tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	if cancelled.str("state") != "cancelled" || cancelled.body["credentials_deleted_at"] == nil ||
		h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1", id) != 0 {
		t.Fatalf("cancel: %s", cancelled.raw)
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+again+"/cancel", nil)

	after := imapSnapshot(t, d.addr(), box, d.pass)
	if !reflect.DeepEqual(before, after) {
		for name := range before {
			if !reflect.DeepEqual(before[name], after[name]) {
				t.Errorf("%s changed:\n%v\n%v", name, before[name], after[name])
			}
		}
		t.Fatal("the source mailbox changed")
	}
	allowed := []string{"CAPABILITY", "NOOP", "LOGOUT", "STARTTLS", "LOGIN", "AUTHENTICATE", "LIST", "STATUS", "EXAMINE", "UNSELECT",
		"SEARCH", "UID SEARCH", "FETCH", "UID FETCH"}
	for _, c := range rawlogCommands(t, d.fixtures, box) {
		if !slices.Contains(allowed, c) {
			t.Errorf("the mailbox was sent %s", c)
		}
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 30})
	if err := h.srv.ApplyRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", tm.ws, id); n != want.Conversations {
		t.Fatalf("retention deleted imported history at once: %d left", n)
	}
	if n := h.count("SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND raw_key IS NOT NULL AND imported_at IS NOT NULL", tm.ws); n != want.MessagesIn+want.MessagesOut {
		t.Fatalf("retention deleted imported originals at once: %d left", n)
	}
	h.clock.Advance(29 * 24 * time.Hour)
	if err := h.srv.ApplyRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", tm.ws, id); n != want.Conversations {
		t.Fatalf("retention deleted imported history after 29 days: %d left", n)
	}
	h.clock.Advance(2 * 24 * time.Hour)
	if err := h.srv.ApplyRetention(ctx); err != nil {
		t.Fatal(err)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2 AND status = 'closed'", tm.ws, id); n != 0 {
		t.Fatalf("closed imported conversations outlived retention: %d", n)
	}
}

func TestDovecotGmailMigrationImportsAllMailOnly(t *testing.T) {
	d := newDovecotRun(t)
	h, tm := d.h, d.tm
	ctx := context.Background()
	want := expectedFixture(t, d.fixtures, "gmail")
	box := copyMailbox(t, d.fixtures, "gmail")
	before := imapSnapshot(t, d.addr(), box, d.pass)
	id := d.connectGmail(t, box, want.TeamAddresses)
	path := "/v1/migrations/" + id
	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	uses := map[string]string{}
	for _, f := range tm.owner.expect(http.StatusOK, "GET", path+"/folders", nil).body["items"].([]any) {
		uses[f.(map[string]any)["name"].(string)] = f.(map[string]any)["use"].(string)
	}
	if want := map[string]string{"INBOX": "skip", "[Gmail]/All Mail": "import", "[Gmail]/Sent Mail": "labels", "[Gmail]/Spam": "skip", "Customers": "labels"}; !reflect.DeepEqual(uses, want) {
		t.Fatalf("folder uses %v, want %v", uses, want)
	}
	labelOnly := h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2 AND reason = 'label_only'", tm.ws, id)
	if labelOnly != 5 {
		t.Fatalf("%d label-only copies, want the 4 in Sent Mail and the 1 in Customers", labelOnly)
	}
	started := tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	if started.str("state") != "copying" {
		t.Fatalf("start: %s", started.raw)
	}
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := d.migration(id)
	counts := m.body["counts"].(map[string]any)
	if m.str("state") != "syncing" || num(counts["imported"]) != want.MessagesIn+want.MessagesOut || num(counts["failed"]) != 0 {
		t.Fatalf("after the copy: %s", m.raw)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2", tm.ws, id); n != want.Conversations {
		t.Fatalf("%d conversations, want %d", n, want.Conversations)
	}
	checkImportedThreads(t, d, id, want)
	if after := imapSnapshot(t, d.addr(), box, d.pass); !reflect.DeepEqual(before, after) {
		t.Fatal("the source mailbox changed")
	}
	allowed := []string{"CAPABILITY", "NOOP", "LOGOUT", "STARTTLS", "LOGIN", "AUTHENTICATE", "LIST", "STATUS", "EXAMINE", "UNSELECT",
		"SEARCH", "UID SEARCH", "FETCH", "UID FETCH"}
	for _, c := range rawlogCommands(t, d.fixtures, box) {
		if !slices.Contains(allowed, c) {
			t.Errorf("the mailbox was sent %s", c)
		}
	}
	tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
}

func checkImportedThreads(t *testing.T, d *dovecotRun, id string, want expectedMailbox) {
	t.Helper()
	ctx := context.Background()
	rows, err := d.h.st.Pool.Query(ctx, `
SELECT c.subject, ce.email, c.status, c.last_message_at, c.imported_at IS NOT NULL,
       (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id AND m.direction = 'in'),
       (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id AND m.direction = 'out'),
       (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id AND m.imported_at IS NULL),
       (SELECT coalesce(array_agg(DISTINCT l.name ORDER BY l.name), '{}') FROM conversation_labels cl JOIN labels l ON l.id = cl.label_id WHERE cl.conversation_id = c.id),
       (SELECT coalesce(array_agg(DISTINCT coalesce(ae.email, p.email, m.imported_author->>'address') ORDER BY coalesce(ae.email, p.email, m.imported_author->>'address')), '{}')
          FROM messages m
          LEFT JOIN contact_emails ae ON ae.contact_id = m.author_contact_id AND ae.position = 0
          LEFT JOIN members mb ON mb.id = m.author_member_id LEFT JOIN people p ON p.id = mb.person_id
         WHERE m.conversation_id = c.id),
       (SELECT count(*) FROM attachments a WHERE a.conversation_id = c.id),
       (SELECT bool_or(e.header_message_id LIKE 'sha256-%') FROM message_emails e WHERE e.conversation_id = c.id)
FROM conversations c JOIN contact_emails ce ON ce.contact_id = c.contact_id AND ce.position = 0
WHERE c.workspace_id = $1 AND c.migration_id = $2`, d.tm.ws, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type got struct {
		contact, status string
		last            time.Time
		imported        bool
		in, out, live   int
		labels, authors []string
		atts            int
		synthesized     bool
	}
	bySubject := map[string]got{}
	for rows.Next() {
		var g got
		var subject string
		if err := rows.Scan(&subject, &g.contact, &g.status, &g.last, &g.imported, &g.in, &g.out, &g.live, &g.labels, &g.authors, &g.atts, &g.synthesized); err != nil {
			t.Fatal(err)
		}
		bySubject[subject] = g
	}
	for _, w := range want.Threads {
		g, ok := bySubject[w.Subject]
		authors := slices.Clone(w.Authors)
		slices.Sort(authors)
		labels := w.Labels
		if labels == nil {
			labels = []string{}
		}
		switch {
		case !ok:
			t.Errorf("no conversation %q", w.Subject)
		case g.contact != w.Contact || g.in != w.In || g.out != w.Out || g.live != 0 || !g.imported || g.status != "closed":
			t.Errorf("%q: %+v, want %+v", w.Subject, g, w)
		case !g.last.Equal(w.LastAt):
			t.Errorf("%q last message at %s, want %s", w.Subject, g.last, w.LastAt)
		case !reflect.DeepEqual(g.labels, labels) || !reflect.DeepEqual(g.authors, authors):
			t.Errorf("%q labels %v authors %v, want %v %v", w.Subject, g.labels, g.authors, labels, authors)
		case g.atts != w.AttachmentsKept || g.synthesized != w.SynthesizedID:
			t.Errorf("%q attachments %d synthesized %v, want %+v", w.Subject, g.atts, g.synthesized, w)
		}
	}
}

// checkNoOrphans compares the workspace's stored files with the keys its rows name.
func checkNoOrphans(t *testing.T, d *dovecotRun) {
	t.Helper()
	d.h.runStorageDeletes(d.tm.ws)
	var files []string
	root := filepath.Join(d.h.storage.Dir, d.tm.ws)
	_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err == nil && !e.IsDir() {
			rel, _ := filepath.Rel(d.h.storage.Dir, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	keys := d.h.storedKeys(d.tm.ws)
	slices.Sort(files)
	slices.Sort(keys)
	if !slices.Equal(files, keys) {
		t.Fatalf("stored files %v, rows name %v", files, keys)
	}
}

func TestDovecotMigrationFolderRecreatedDuringCopy(t *testing.T) {
	d := newDovecotRun(t)
	h, tm := d.h, d.tm
	ctx := context.Background()
	want := expectedFixture(t, d.fixtures, "acme")
	box := copyMailbox(t, d.fixtures, "acme")

	id, channel := d.connect(t, box, want.TeamAddresses, "")
	live := "Message-ID: <order-1001-1@example.org>\r\nFrom: =?utf-8?q?Ay=C5=9Fe_Y=C4=B1lmaz?= <ayse@example.org>\r\nTo: " +
		d.channelAddress(t, channel) + "\r\nSubject: Order 1001 never arrived\r\nDate: Mon, 03 Mar 2025 09:00:00 +0000\r\n\r\nHello, my order has not arrived.\r\n"
	if r := h.ingest(d.channelAddress(t, channel), []byte(live), nil); r.status != http.StatusAccepted {
		t.Fatalf("live mail: %d", r.status)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/scan", nil)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	if n := num(d.migration(id).body["estimate"].(map[string]any)["skipped"].(map[string]any)["already_in_yuva"]); n != 1 {
		t.Fatalf("already in Yuva: %d", n)
	}

	inbox := filepath.Join(d.fixtures, box, "Maildir")
	entries, _ := os.ReadDir(inbox)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "dovecot") && !e.IsDir() {
			if err := os.Remove(filepath.Join(inbox, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	at := time.Date(2025, 4, 1, 10, 0, 0, 0, time.UTC)
	fresh := "Message-ID: <new-after-recreate@example.org>\r\nFrom: Nina Stone <nina@example.org>\r\nTo: support@example.com\r\n" +
		"Subject: Arrived while you moved\r\nDate: " + at.Format(time.RFC1123Z) + "\r\n\r\nHello from the new folder.\r\n"
	name := filepath.Join(inbox, "cur", fmt.Sprintf("%d.Mnew.fixture,S=%d:2,", at.Unix(), len(fresh)))
	if err := os.WriteFile(name, []byte(fresh), 0o666); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(name, 0o666)
	_ = os.Chtimes(name, at, at)
	time.Sleep(1100 * time.Millisecond)

	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/start", nil)
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := d.migration(id)
	counts := m.body["counts"].(map[string]any)
	alreadyInYuva, added := 1, 1
	if m.str("state") != "syncing" || num(counts["imported"]) != want.MessagesIn+want.MessagesOut-alreadyInYuva+added || num(counts["failed"]) != 0 {
		t.Fatalf("after a recreated folder: %s", m.raw)
	}
	if n := h.count("SELECT count(*) FROM (SELECT header_message_id FROM message_emails WHERE workspace_id = $1 GROUP BY header_message_id HAVING count(*) > 1) d", tm.ws); n != 0 {
		t.Fatalf("%d Message-IDs stored twice", n)
	}
	if n := h.count("SELECT count(*) FROM message_emails WHERE workspace_id = $1 AND header_message_id = 'new-after-recreate@example.org' AND imported_at IS NOT NULL", tm.ws); n != 1 {
		t.Fatal("the message added to the recreated folder was not imported")
	}
	errs := tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/"+id+"/errors", nil).body["items"].([]any)
	if !slices.ContainsFunc(errs, func(e any) bool { return e.(map[string]any)["code"] == "folder_changed" }) {
		t.Fatalf("errors: %v", errs)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/cancel", nil)
}

func TestDovecotMoveToYuvaCreatesTheEmailChannel(t *testing.T) {
	d := newDovecotRun(t)
	h, tm := d.h, d.tm
	ctx := context.Background()
	want := expectedFixture(t, d.fixtures, "acme")
	box := copyMailbox(t, d.fixtures, "acme")
	address := unique("support") + "@example.com"
	emailChannels := func() int {
		return h.count("SELECT count(*) FROM channels WHERE workspace_id = $1 AND inbox_id = $2 AND kind = 'email'", tm.ws, tm.inbox)
	}
	if n := emailChannels(); n != 0 {
		t.Fatalf("the inbox starts with %d e-mail channels", n)
	}
	resource := testOrigin + "/mcp"
	args := map[string]any{"address": address, "provider": "imap", "inbox_id": tm.inbox, "options": map[string]any{"team_addresses": want.TeamAddresses}}

	narrow := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", resource, []string{"inboxes:read", "migrations:manage"}))
	if _, errText := callTool(t, narrow, "create_migration", args); !strings.Contains(errText, "inboxes:manage") {
		t.Fatalf("created a channel without inboxes:manage: %q", errText)
	}
	other := newTeam(t, h)
	ocs := h.mcpSession(h.oauthToken(other.owner, other.ws, "Claude Code", resource, []string{"inboxes:read", "inboxes:manage", "migrations:manage"}))
	if _, errText := callTool(t, ocs, "create_migration", args); !strings.Contains(errText, "not_found") && !strings.Contains(errText, "no such inbox") {
		t.Fatalf("another workspace's inbox: %q", errText)
	}
	if n := emailChannels(); n != 0 {
		t.Fatalf("a refused create_migration left %d e-mail channels", n)
	}

	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", resource, []string{"inboxes:read", "inboxes:manage", "migrations:manage"}))
	if text := promptText(t, cs, "move_to_yuva", map[string]string{"address": address}); !strings.Contains(text, "without channel_id") || !strings.Contains(text, "channel_setup") {
		t.Fatalf("move_to_yuva does not lead to the channel:\n%s", text)
	}
	created, errText := callTool(t, cs, "create_migration", args)
	if errText != "" {
		t.Fatal(errText)
	}
	id, channel := created["id"].(string), created["channel_id"].(string)
	if created["channel_created"] != true || created["inbox_id"] != tm.inbox || !strings.Contains(created["channel_setup"].(string), "SMTP") ||
		created["channel_settings_url"] != testOrigin+"/settings/inboxes/"+tm.inbox+"/channels/"+channel+"?workspace="+tm.ws {
		t.Fatalf("create_migration: %v", created)
	}
	if got := d.channelAddress(t, channel); got != address || emailChannels() != 1 ||
		h.count("SELECT count(*) FROM email_channels WHERE channel_id = $1 AND smtp_host = '' AND smtp_password IS NULL", channel) != 1 {
		t.Fatalf("the created channel: %s, %d e-mail channels", got, emailChannels())
	}
	again, errText := callTool(t, cs, "create_migration", args)
	if errText != "" || again["channel_id"] != channel || again["channel_created"] != nil || emailChannels() != 1 {
		t.Fatalf("a second create_migration: %v %s", again, errText)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/migrations/"+again["id"].(string), nil)

	d.enterCredentials(t, id, box)
	step := func(tool string, in map[string]any) map[string]any {
		t.Helper()
		out, errText := callTool(t, cs, tool, in)
		if errText != "" {
			t.Fatalf("%s: %s", tool, errText)
		}
		if m, ok := out["migration"].(map[string]any); ok {
			return m
		}
		return out
	}
	if m := step("test_migration_connection", map[string]any{"migration_id": id}); m["state"] != "connected" {
		t.Fatalf("after the test: %v", m)
	}
	step("scan_migration", map[string]any{"migration_id": id})
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	if m := step("get_migration", map[string]any{"migration_id": id}); m["state"] != "ready" {
		t.Fatalf("after the scan: %v", m)
	}
	if m := step("start_migration", map[string]any{"migration_id": id}); m["state"] != "copying" {
		t.Fatalf("start_migration: %v", m)
	}
	if err := d.work(t, ctx); err != nil {
		t.Fatal(err)
	}
	m := step("get_migration", map[string]any{"migration_id": id})
	counts := m["counts"].(map[string]any)
	if m["state"] != "syncing" || num(counts["imported"]) != want.MessagesIn+want.MessagesOut || num(counts["duplicate"]) != want.Skipped["duplicate_copies"] ||
		num(counts["skipped"]) != want.Skipped["automatic"]+want.Skipped["internal"] || num(m["percent"]) != 100 {
		t.Fatalf("after the copy: %v", m)
	}
	if n := h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2 AND channel_id = $3", tm.ws, id, channel); n != want.Conversations {
		t.Fatalf("%d conversations in the created channel, want %d", n, want.Conversations)
	}
	if m := step("finish_migration", map[string]any{"migration_id": id, "force": true}); m["state"] != "completed" || m["credentials_deleted_at"] == nil {
		t.Fatalf("finish_migration: %v", m)
	}
}

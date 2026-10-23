package api_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
)

// fakeIMAP answers just enough IMAP over TLS for the connection test, and records every command
// it is sent.
type fakeIMAP struct {
	t           *testing.T
	ln          net.Listener
	fingerprint string
	mu          sync.Mutex
	commands    []string
}

func newFakeIMAP(t *testing.T) *fakeIMAP {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "imap.example.com"}, DNSNames: []string{"imap.example.com"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIMAP{t: t, ln: ln, fingerprint: hex.EncodeToString(sum[:])}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeIMAP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeIMAP) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.commands)
}

func (f *fakeIMAP) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	br := bufio.NewReader(conn)
	say := func(lines ...string) { _, _ = conn.Write([]byte(strings.Join(lines, "\r\n") + "\r\n")) }
	say("* OK [CAPABILITY IMAP4rev1 SPECIAL-USE] fake ready")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])
		f.mu.Lock()
		f.commands = append(f.commands, cmd)
		f.mu.Unlock()
		switch cmd {
		case "CAPABILITY":
			say("* CAPABILITY IMAP4rev1 SPECIAL-USE", tag+" OK done")
		case "LOGIN":
			switch strings.Trim(fields[len(fields)-1], `"`) {
			case "right-password":
				say(tag + " OK [CAPABILITY IMAP4rev1 SPECIAL-USE] signed in")
			case "account-password":
				say(tag + " NO [ALERT] Application-specific password required: https://support.google.com/accounts/answer/185833 (Failure)")
			default:
				say(tag + " NO [AUTHENTICATIONFAILED] Authentication failed.")
			}
		case "LIST":
			say(`* LIST (\HasNoChildren) "/" INBOX`, `* LIST (\HasNoChildren \Sent) "/" Sent`, `* LIST (\HasNoChildren \Junk) "/" Spam`,
				`* LIST (\HasNoChildren) "/" Customers`, tag+" OK done")
		case "EXAMINE":
			say("* 3 EXISTS", "* OK [UIDVALIDITY 7] ok", "* OK [UIDNEXT 4] ok", tag+" OK [READ-ONLY] done")
		case "STATUS":
			n := map[string]int{"INBOX": 3, "Sent": 2, "Customers": 1, "Spam": 9}[strings.Trim(fields[2], `"`)]
			say("* STATUS "+fields[2]+" (MESSAGES "+strconv.Itoa(n)+" UIDNEXT 9 UIDVALIDITY 7)", tag+" OK done")
		case "LOGOUT":
			say("* BYE bye", tag+" OK done")
			return
		default:
			say(tag + " BAD unknown")
		}
	}
}

type migrationTeam struct {
	team
	fake *fakeIMAP
}

func newMigrationHarness(t *testing.T) *harness {
	return newHarnessWith(t, func(d *api.Deps) {
		d.Migrations = api.MigrationSettings{AllowPrivate: true, TestTimeout: 5 * time.Second}
	})
}

func newMigrationTeam(t *testing.T, h *harness) migrationTeam {
	t.Helper()
	return migrationTeam{team: newTeam(t, h), fake: newFakeIMAP(t)}
}

func (mt migrationTeam) create(body map[string]any) string {
	return mt.owner.expect(http.StatusCreated, "POST", "/v1/migrations", body).str("id")
}

func TestMigrationAccess(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	r := mt.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "gmail", "source_address": "Support@Example.com"})
	id := r.str("id")
	opts := r.body["options"].(map[string]any)
	if r.str("state") != "draft" || r.str("source") != "imap" || r.str("source_address") != "support@example.com" ||
		opts["skip_automatic"] != true || opts["close_after_days"] != float64(14) || opts["keep_originals"] != true || opts["quiet_period_hours"] != float64(24) {
		t.Fatalf("created: %s", r.raw)
	}
	if got := r.body["actions"].([]any); len(got) != 3 || got[0] != "update" || got[1] != "set_credentials" || got[2] != "delete" {
		t.Fatalf("actions: %v", got)
	}
	if h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'migration.updated'", mt.ws) != 1 {
		t.Fatal("no migration.updated on create")
	}
	if h.count("SELECT count(*) FROM migration_audit WHERE workspace_id = $1 AND migration_id = $2 AND action = 'created'", mt.ws, id) != 1 {
		t.Fatal("no audit row")
	}
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/migrations", map[string]any{"source": "zendesk", "preset": "imap", "source_address": "a@example.com"})
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/migrations", map[string]any{"preset": "hotmail", "source_address": "a@example.com"})

	for _, c := range [][2]string{{"GET", "/v1/migrations"}, {"GET", "/v1/migrations/presets"}, {"GET", "/v1/migrations/" + id}, {"PATCH", "/v1/migrations/" + id},
		{"DELETE", "/v1/migrations/" + id}, {"PUT", "/v1/migrations/" + id + "/credentials"}, {"POST", "/v1/migrations/" + id + "/test"}, {"POST", "/v1/migrations/detect"}, {"POST", "/v1/migrations"}} {
		mt.agent.expectProblem(http.StatusForbidden, "forbidden", c[0], c[1], map[string]any{"preset": "gmail", "source_address": "a@gmail.com", "address": "a@gmail.com", "username": "a", "password": "b", "options": map[string]any{}})
	}
	other := newTeam(t, h)
	for _, c := range [][2]string{{"GET", "/v1/migrations/" + id}, {"PATCH", "/v1/migrations/" + id}, {"DELETE", "/v1/migrations/" + id},
		{"PUT", "/v1/migrations/" + id + "/credentials"}, {"POST", "/v1/migrations/" + id + "/test"}} {
		other.owner.expectProblem(http.StatusNotFound, "not_found", c[0], c[1], map[string]any{"username": "a", "password": "b", "options": map[string]any{}})
	}
	if items := other.owner.expect(http.StatusOK, "GET", "/v1/migrations", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("other workspace lists %d migrations", len(items))
	}

	full := h.client()
	full.bearer = mt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "full"}).str("secret")
	if items := full.expect(http.StatusOK, "GET", "/v1/migrations", nil).body["items"].([]any); len(items) != 1 {
		t.Fatalf("key lists %d migrations", len(items))
	}
	narrow := h.client()
	narrow.bearer = mt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "narrow", "scopes": []string{"conversations:read"}}).str("secret")
	narrow.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/migrations/"+id, nil)
	limited := mt.owner.do("POST", "/v1/api-keys", map[string]any{"name": "limited", "scopes": []string{"migrations:manage"}, "inbox_ids": []string{mt.inbox}})
	if limited.status < 400 || !strings.Contains(string(limited.raw), "migrations:manage") {
		t.Fatalf("an inbox-limited key got migrations:manage: %d %s", limited.status, limited.raw)
	}
	agentToken := h.client()
	agentToken.bearer = h.oauthToken(mt.agent, mt.ws, "Claude Code", testOrigin, []string{"conversations:read"})
	agentToken.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/migrations", nil)

	presets := mt.owner.expect(http.StatusOK, "GET", "/v1/migrations/presets", nil).body["items"].([]any)
	byID := map[string]map[string]any{}
	for _, p := range presets {
		byID[p.(map[string]any)["id"].(string)] = p.(map[string]any)
	}
	if len(byID) != 17 || byID["front"]["auth"].([]any)[0] != "token" || byID["zendesk"]["source"] != "zendesk" || byID["helpscout"]["auth"].([]any)[0] != "oauth" || byID["takeout"]["source"] != "file" || byID["file"]["source"] != "file" || byID["gmail"]["available"] != true || byID["microsoft"]["available"] != true || byID["microsoft"]["team_client"] != true || byID["icloud"]["username"] != "local_part" ||
		byID["gmail"]["budget"].(map[string]any)["bytes_per_day"] != float64(1500000000) {
		t.Fatalf("presets: %v", byID)
	}
	det := mt.owner.expect(http.StatusOK, "POST", "/v1/migrations/detect", map[string]any{"address": "someone@yandex.com.tr"})
	if det.str("preset") != "yandex" || det.str("found_by") != "domain" || det.body["server"].(map[string]any)["host"] != "imap.yandex.com" {
		t.Fatalf("detect: %s", det.raw)
	}
	det = mt.owner.expect(http.StatusOK, "POST", "/v1/migrations/detect", map[string]any{"address": "someone@hotmail.com"})
	if det.str("preset") != "microsoft" || det.body["available"] != true || det.body["note"] != nil {
		t.Fatalf("detect microsoft: %s", det.raw)
	}

	mt.owner.expect(http.StatusNoContent, "DELETE", "/v1/migrations/"+id, nil)
	mt.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/migrations/"+id, nil)
	if h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1 AND action = 'deleted'", id) != 1 {
		t.Fatal("deletion not audited")
	}
}

func TestMigrationTargetAndOptions(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	address := unique("support") + "@example.com"
	ch := mt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+mt.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Support mail",
		"email": map[string]any{"address": address, "display_name": "Support",
			"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"}},
	}).str("id")
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/migrations", map[string]any{"preset": "imap", "source_address": "other@example.com", "channel_id": ch})
	r := mt.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "imap", "source_address": address, "channel_id": ch})
	id := r.str("id")
	if r.str("inbox_id") != mt.inbox || r.str("channel_id") != ch {
		t.Fatalf("target: %s", r.raw)
	}
	path := "/v1/migrations/" + id
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"options": map[string]any{"close_after_days": 4000}})
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"options": map[string]any{"people": []map[string]any{{"address": "x@example.com", "member_id": "0199c4a8-0000-7000-8000-000000000000"}}}})
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"options": map[string]any{"labels": []map[string]any{{"folder": "Customers", "action": "existing"}}}})
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"source_address": "other@example.com"})
	upd := mt.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"options": map[string]any{
		"folders": []string{"INBOX", "Customers", "INBOX"}, "since": "2024-01-01T00:00:00Z", "close_after_days": 0,
		"team_addresses": []string{"Sales@Example.com", "sales@example.com"},
		"people":         []map[string]any{{"address": "Owner@Example.com", "member_id": mt.agentID}},
		"labels":         []map[string]any{{"folder": "Customers", "action": "create"}},
	}})
	o := upd.body["options"].(map[string]any)
	if len(o["folders"].([]any)) != 2 || o["close_after_days"] != float64(0) || len(o["team_addresses"].([]any)) != 1 || o["team_addresses"].([]any)[0] != "sales@example.com" ||
		o["people"].([]any)[0].(map[string]any)["address"] != "owner@example.com" || o["since"] != "2024-01-01T00:00:00Z" {
		t.Fatalf("options: %s", upd.raw)
	}
	upd = mt.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"channel_id": nil, "options": map[string]any{"folders": nil}})
	if upd.body["channel_id"] != nil || upd.str("inbox_id") != mt.inbox || upd.body["options"].(map[string]any)["folders"] != nil {
		t.Fatalf("cleared: %s", upd.raw)
	}
	if h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'migration.updated'", mt.ws) != 1 {
		t.Fatal("an update without a state change wrote an event")
	}
}

func TestMigrationConnectionTest(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	id := mt.create(map[string]any{"preset": "imap", "source_address": "support@example.com"})
	path := "/v1/migrations/" + id
	mt.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/test", nil)
	mt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"username": "support@example.com", "password": "x"})
	creds := map[string]any{"username": "support@example.com", "password": "right-password", "host": "127.0.0.1", "port": mt.fake.port()}
	mt.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)

	got := mt.owner.expect(http.StatusOK, "GET", path, nil)
	if bytes.Contains(got.raw, []byte("right-password")) {
		t.Fatal("the password came back")
	}
	cr := got.body["credentials"].(map[string]any)
	if cr["kind"] != "password" || cr["server"].(map[string]any)["host"] != "127.0.0.1" || cr["server"].(map[string]any)["tls"] != "tls" {
		t.Fatalf("credentials: %s", got.raw)
	}
	if h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1 AND position('right-password'::bytea in sealed) = 0", id) != 1 {
		t.Fatal("the password is stored in the clear")
	}

	res := mt.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	steps := res.body["steps"].([]any)
	last := steps[len(steps)-1].(map[string]any)
	if res.body["ok"] != false || last["step"] != "tls" || last["code"] != "tls_failed" || res.str("certificate_sha256") != mt.fake.fingerprint {
		t.Fatalf("self-signed: %s", res.raw)
	}
	creds["certificate_sha256"] = strings.ToUpper(mt.fake.fingerprint)
	mt.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	res = mt.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if res.body["ok"] != true || res.body["folders"] != float64(4) || res.body["messages"] != float64(6) ||
		res.body["migration"].(map[string]any)["state"] != "connected" {
		t.Fatalf("pinned: %s", res.raw)
	}
	for _, c := range mt.fake.seen() {
		if !slices.Contains([]string{"CAPABILITY", "LOGIN", "LIST", "EXAMINE", "STATUS", "LOGOUT"}, c) {
			t.Errorf("the server was sent %s", c)
		}
	}

	creds["password"] = "account-password"
	mt.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	if s := mt.owner.expect(http.StatusOK, "GET", path, nil).str("state"); s != "draft" {
		t.Fatalf("new credentials left the migration %s", s)
	}
	res = mt.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	steps = res.body["steps"].([]any)
	if last := steps[len(steps)-1].(map[string]any); last["code"] != "app_password_required" || !strings.Contains(last["detail"].(string), "Application-specific password required") {
		t.Fatalf("app password: %s", res.raw)
	}

	creds["password"] = "wrong-password"
	mt.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	for range 3 {
		res = mt.owner.expect(http.StatusOK, "POST", path+"/test", nil)
		steps = res.body["steps"].([]any)
		if last := steps[len(steps)-1].(map[string]any); last["code"] != "auth_failed" || strings.Contains(string(res.raw), "wrong-password") {
			t.Fatalf("wrong password: %s", res.raw)
		}
	}
	locked := mt.owner.expect(http.StatusTooManyRequests, "POST", path+"/test", nil)
	if locked.str("code") != "rate_limited" || locked.header.Get("Retry-After") == "" {
		t.Fatalf("not locked after three failed sign-ins: %s", locked.raw)
	}
	h.clock.Advance(16 * time.Minute)
	creds["password"] = "right-password"
	mt.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", creds)
	for range 4 {
		mt.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	}
	mt.owner.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", path+"/test", nil)
	if n := h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1 AND action = 'tested'", id); n != 10 {
		t.Fatalf("%d tests recorded", n)
	}
	if h.count("SELECT count(*) FROM migration_audit WHERE migration_id = $1 AND (detail::text LIKE '%password%' AND detail::text NOT LIKE '%app_password%')", id) != 0 {
		t.Fatal("a password reached the audit record")
	}
	h.clock.Advance(time.Hour)

	private := mt.create(map[string]any{"preset": "imap", "source_address": "support@example.com"})
	mt.owner.expect(http.StatusNoContent, "PUT", "/v1/migrations/"+private+"/credentials", map[string]any{"username": "a", "password": "b", "host": "127.0.0.1", "port": 1})
	closed := mt.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+private+"/test", nil)
	if s := closed.body["steps"].([]any); s[len(s)-1].(map[string]any)["code"] != "connect_failed" {
		t.Fatalf("closed port: %s", closed.raw)
	}

	mt.owner.expect(http.StatusNoContent, "DELETE", path, nil)
	if h.count("SELECT count(*) FROM migration_credentials WHERE migration_id = $1", id) != 0 {
		t.Fatal("credentials outlived their migration")
	}
}

func TestMigrationRefusesPrivateServers(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	id := tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"preset": "imap", "source_address": "a@example.com"}).str("id")
	for _, host := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254"} {
		tm.owner.expect(http.StatusNoContent, "PUT", "/v1/migrations/"+id+"/credentials", map[string]any{"username": "a", "password": "b", "host": host})
		r := tm.owner.expect(http.StatusOK, "POST", "/v1/migrations/"+id+"/test", nil)
		if s := r.body["steps"].([]any); len(s) != 1 || s[0].(map[string]any)["code"] != "address_not_allowed" {
			t.Fatalf("%s: %s", host, r.raw)
		}
	}
}

func TestImportedHistoryIsQuiet(t *testing.T) {
	h := newMigrationHarness(t)
	mt := newMigrationTeam(t, h)
	id := mt.create(map[string]any{"preset": "gmail", "source_address": "support@example.com"})
	conv := mt.conversation(mt.owner)
	key := h.client()
	key.bearer = mt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"}).str("secret")
	key.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": "Old question"})
	unread := func() bool {
		items := mt.owner.expect(http.StatusOK, "GET", "/v1/conversations", nil).body["items"].([]any)
		for _, it := range items {
			if c := it.(map[string]any); c["id"] == conv {
				return c["unread"] == true
			}
		}
		t.Fatal("conversation not listed")
		return false
	}
	if !unread() {
		t.Fatal("a live message from the contact is not unread")
	}
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, "UPDATE messages SET imported_at = now(), imported_author = NULL WHERE conversation_id = $1", conv); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx, "UPDATE conversations SET migration_id = $2, imported_at = now() WHERE id = $1", conv, id); err != nil {
		t.Fatal(err)
	}
	if unread() {
		t.Fatal("imported history is unread")
	}
	c := mt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	imp, _ := c.body["imported"].(map[string]any)
	if imp == nil || imp["migration_id"] != id || imp["source"] != "imap" || imp["preset"] != "gmail" || imp["at"] == nil {
		t.Fatalf("imported: %s", c.raw)
	}
	for _, m := range messages(mt.owner, conv) {
		if m["kind"] == "message" && m["imported_at"] == nil {
			t.Fatalf("message without imported_at: %v", m)
		}
	}
	mt.owner.expect(http.StatusNoContent, "DELETE", "/v1/migrations/"+id, nil)
	c = mt.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if imp := c.body["imported"].(map[string]any); imp["migration_id"] != nil || imp["preset"] != nil || imp["at"] == nil {
		t.Fatalf("after deleting the migration: %s", c.raw)
	}
}

package api_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

const chatwootURL = "https://" + helpdesktest.ChatwootHost

// A Chatwoot migration connects with an administrator's token, the install's address and the
// account number; an agent of that account, another account and a request from another workspace
// are refused, and the token only ever reaches the install, with GETs.
func TestChatwootMigration(t *testing.T) {
	d := newDeskRun(t)
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "chatwoot", "preset": "chatwoot", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id

	var preset map[string]any
	for _, p := range d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/presets", nil).body["items"].([]any) {
		if p.(map[string]any)["id"] == "chatwoot" {
			preset = p.(map[string]any)
		}
	}
	if preset["auth"].([]any)[0] != "token" || len(preset["token_fields"].([]any)) != 3 || preset["token_fields"].([]any)[2] != "account" ||
		preset["budget"].(map[string]any)["requests_per_minute"] != float64(60) {
		t.Fatalf("preset: %v", preset)
	}

	token := helpdesktest.ChatwootToken
	body := func(account string) map[string]any {
		return map[string]any{"kind": "token", "token": token, "base_url": chatwootURL, "account": account}
	}
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "base_url": chatwootURL})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", body("one"))
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "base_url": "http://" + helpdesktest.ChatwootHost, "account": "1"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "base_url": chatwootURL, "account": "1", "subdomain": "acme"})
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/sign-in", map[string]any{"client_id": "x", "client_secret": "y"})
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", body("1"))
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/test", nil)
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", body("1"))

	step := func(res response) map[string]any {
		for _, s := range res.body["steps"].([]any) {
			if s.(map[string]any)["ok"] != true {
				return s.(map[string]any)
			}
		}
		return nil
	}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", body("2"))
	res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if f := step(res); res.body["ok"] != false || f == nil || f["code"] != "not_admin" {
		t.Fatalf("an agent of the account: %s", res.raw)
	}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", body("3"))
	res = d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if f := step(res); res.body["ok"] != false || f == nil || f["code"] != "auth_failed" || !strings.Contains(f["detail"].(string), "1 Acme Support") {
		t.Fatalf("an account the user is not in: %s", res.raw)
	}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": "wrong", "base_url": chatwootURL, "account": "1"})
	res = d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if f := step(res); res.body["ok"] != false || f == nil || f["code"] != "auth_failed" {
		t.Fatalf("a wrong token: %s", res.raw)
	}

	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", body("1"))
	got := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	if cred := got.body["credentials"].(map[string]any); cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != helpdesktest.ChatwootHost || strings.Contains(string(got.raw), token) {
		t.Fatalf("credentials: %s", got.raw)
	}
	res = d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if res.body["ok"] != true || d.state(id) != "connected" {
		t.Fatalf("test: %s, state %s", res.raw, d.state(id))
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/folders", nil)
	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })
	if n := d.h.count("SELECT count(*) FROM migration_items WHERE workspace_id = $1 AND migration_id = $2::uuid", d.tm.ws, id); n != 27 {
		t.Errorf("%d conversations scanned, want 27", n)
	}
	d.tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	d.work(t, func() bool { return d.state(id) == "syncing" })
	if n := d.h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND migration_id = $2::uuid", d.tm.ws, id); n != 27 {
		t.Fatalf("%d conversations, want 27", n)
	}
	export := d.str(t, "SELECT id::text FROM conversations WHERE workspace_id = $1 AND subject = 'Export is empty'", d.tm.ws)
	if got := d.str(t, "SELECT count(*) || ' ' || count(*) FILTER (WHERE kind = 'note') || ' ' || min(m.created_at)::text FROM messages m WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, export); !strings.HasPrefix(got, "22 1 2025-10-09") {
		t.Errorf("Export is empty: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(filename, ',' ORDER BY filename) FROM attachments WHERE workspace_id = $1", d.tm.ws); got != "export-log.txt,screenshot.png" {
		t.Errorf("attachments: %s", got)
	}
	for subject, want := range map[string]string{"Invoice question": "open", "Refund": "closed"} {
		if got := d.str(t, "SELECT status FROM conversations WHERE workspace_id = $1 AND subject = $2", d.tm.ws, subject); got != want {
			t.Errorf("%s is %s, want %s", subject, got, want)
		}
	}
	if got := d.str(t, "SELECT c.attributes->>'chatwoot_external_id' FROM contacts c JOIN contact_emails e ON e.workspace_id = c.workspace_id AND e.contact_id = c.id WHERE c.workspace_id = $1 AND e.email = 'ayse@example.org'", d.tm.ws); got != "crm-101" {
		t.Errorf("requester attributes: %s", got)
	}
	if n := d.h.count("SELECT count(*) FROM migration_credentials WHERE workspace_id = $1 AND position(convert_to($2, 'UTF8') IN sealed) > 0", d.tm.ws, token); n != 0 {
		t.Error("the token is stored in the clear")
	}

	for _, r := range d.desk.Requests() {
		if (r.Host == helpdesktest.OutsideHost || strings.HasPrefix(r.Path, "/rails/")) && r.Method == http.MethodGet && r.Auth == "" && r.Token == "" {
			continue
		}
		if r.Method != http.MethodGet || r.Host != helpdesktest.ChatwootHost || r.Auth != "" || (r.Token != token && r.Token != "wrong") {
			t.Errorf("%s %s%s with Authorization %q and token %q", r.Method, r.Host, r.Path, r.Auth, r.Token)
		}
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	if n := d.h.count("SELECT count(*) FROM migration_credentials WHERE workspace_id = $1", d.tm.ws); n != 0 {
		t.Errorf("%d credentials left after cancel", n)
	}
}

type privateResolver struct{}

func (privateResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("10.0.0.5")}, nil
}

func (privateResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return nil, errors.New("no MX")
}

func (privateResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errors.New("no SRV")
}

// A base URL whose host resolves to a private address is not dialled, and nothing is sent.
func TestChatwootRefusesPrivateAddress(t *testing.T) {
	var desk *helpdesktest.Server
	h := newHarnessWith(t, func(dp *api.Deps) {
		desk = helpdesktest.New(t, dp.Now)
		dp.Migrations = api.MigrationSettings{TestTimeout: 5 * time.Second, RootCAs: desk.Pool, Resolver: privateResolver{}}
	})
	d := &deskRun{h: h, tm: newTeam(t, h), desk: desk}
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "chatwoot", "preset": "chatwoot", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{
		"kind": "token", "token": helpdesktest.ChatwootToken, "base_url": chatwootURL, "account": "1",
	})
	res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if res.body["ok"] != false || d.state(id) != "draft" {
		t.Fatalf("test: %s, state %s", res.raw, d.state(id))
	}
	if n := len(desk.Requests()); n != 0 {
		t.Errorf("%d requests reached the install", n)
	}
}

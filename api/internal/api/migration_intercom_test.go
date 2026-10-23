package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

func TestIntercomMigration(t *testing.T) {
	d := newDeskRun(t)
	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "intercom", "preset": "intercom", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id
	token := helpdesktest.IntercomToken

	presets := d.tm.owner.expect(http.StatusOK, "GET", "/v1/migrations/presets", nil).body["items"].([]any)
	i := slices.IndexFunc(presets, func(p any) bool { return p.(map[string]any)["id"] == "intercom" })
	if i < 0 || !slices.Equal(anyStrings(presets[i].(map[string]any)["token_fields"]), []string{"token", "region"}) {
		t.Fatalf("preset: %v", presets)
	}

	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "region": "mars"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "region": "eu", "secret": "x"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"username": "a", "password": "b", "region": "eu"})
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "region": "eu"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", path+"/test", nil)
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "region": "eu"})

	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": "wrong-token", "region": "eu"})
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != false {
		t.Fatalf("a wrong token passed the test: %s", res.raw)
	}
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "region": "eu"})
	got := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	cred := got.body["credentials"].(map[string]any)
	if cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != "api.eu.intercom.io" || strings.Contains(string(got.raw), token) {
		t.Fatalf("credentials: %s", got.raw)
	}
	if res := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil); res.body["ok"] != true {
		t.Fatalf("test: %s", res.raw)
	}
	folders := d.tm.owner.expect(http.StatusOK, "GET", path+"/folders", nil).body["items"].([]any)
	if len(folders) != 3 {
		t.Fatalf("folders: %v", folders)
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })
	d.tm.owner.expect(http.StatusOK, "POST", path+"/start", nil)
	d.work(t, func() bool { return d.state(id) == "syncing" })

	charged := d.str(t, "SELECT id::text FROM conversations WHERE workspace_id = $1 AND subject = 'Charged twice'", d.tm.ws)
	if got := d.str(t, "SELECT status || ' ' || coalesce(assignee_id::text, '-') FROM conversations WHERE workspace_id = $1 AND id = $2", d.tm.ws, charged); got != "open -" {
		t.Errorf("Charged twice: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(kind || ':' || coalesce(direction, '-'), ' ' ORDER BY created_at, id) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND kind <> 'event'", d.tm.ws, charged); got != "message:in message:out note:- message:in message:out message:in" {
		t.Errorf("Charged twice thread: %s", got)
	}
	if got := d.str(t, "SELECT count(DISTINCT author_contact_id) FROM messages WHERE workspace_id = $1 AND conversation_id = $2 AND direction = 'in'", d.tm.ws, charged); got != "2" {
		t.Errorf("the colleague's message has its own contact: %s senders", got)
	}
	if got := d.str(t, "SELECT string_agg(filename, ',' ORDER BY filename) FROM attachments WHERE workspace_id = $1 AND conversation_id = $2", d.tm.ws, charged); got != "receipt.png" {
		t.Errorf("attachments: %s", got)
	}
	if n := d.h.count("SELECT count(*) FROM contacts WHERE workspace_id = $1 AND name = 'Visitor 77'", d.tm.ws); n != 1 {
		t.Errorf("the lead without an e-mail address: %d contacts", n)
	}
	if got := d.str(t, "SELECT v.status FROM conversations v JOIN contacts c ON c.workspace_id = v.workspace_id AND c.id = v.contact_id WHERE v.workspace_id = $1 AND c.name = 'Visitor 77'", d.tm.ws); got != "pending" {
		t.Errorf("the snoozed conversation: %s", got)
	}
	if got := d.str(t, "SELECT string_agg(code, ',') FROM migration_errors WHERE workspace_id = $1", d.tm.ws); got != "truncated" {
		t.Errorf("errors: %q", got)
	}

	for _, r := range d.desk.Requests() {
		if r.Method != http.MethodGet || r.Host == helpdesktest.OutsideHost || r.Path == "/steal" {
			t.Errorf("%s %s%s was sent", r.Method, r.Host, r.Path)
		}
		if r.Host == "api.eu.intercom.io" && r.Auth != "Bearer "+token && r.Auth != "Bearer wrong-token" {
			t.Errorf("%s carried %q", r.Path, r.Auth)
		}
		if r.Host == helpdesktest.IntercomCDN && r.Auth != "" {
			t.Errorf("the CDN received the token")
		}
	}
	for _, q := range []string{
		"SELECT count(*) FROM migration_credentials WHERE workspace_id = $1 AND position(convert_to($2, 'UTF8') IN sealed) > 0",
		"SELECT count(*) FROM migration_audit WHERE workspace_id = $1 AND detail::text LIKE '%' || $2 || '%'",
		"SELECT count(*) FROM migrations WHERE workspace_id = $1 AND (throttle::text || source_state::text) LIKE '%' || $2 || '%'",
	} {
		if n := d.h.count(q, d.tm.ws, token); n != 0 {
			t.Errorf("the token is stored in the clear: %s", q)
		}
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, e := range v.([]any) {
		out = append(out, e.(string))
	}
	return out
}

package api_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

// A help desk whose registry entry connects with a token takes it through PUT .../credentials:
// sealed, never answered, logged or put in a job's arguments, sent only to its API host, and
// deleted when the migration is cancelled. Help Scout stands in for such a desk here.
func TestTokenDeskConnects(t *testing.T) {
	d := newDeskRun(t)
	desk, _ := helpdesk.Lookup("helpscout")
	desk.Auth, desk.OAuth = helpdesk.AuthToken, nil
	t.Cleanup(helpdesk.Replace(desk))

	tokens, err := helpdesk.HelpScoutExchange(context.Background(), helpdesk.Config{Dial: d.desk.Dial, RootCAs: d.desk.Pool},
		helpdesk.App{ClientID: "hs-client", ClientSecret: "hs-secret"}, helpdesktest.HelpScoutCode)
	if err != nil {
		t.Fatal(err)
	}
	token := tokens.Access

	address, channel := d.emailChannel(t, d.tm.inbox, "support")
	id := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "helpscout", "preset": "helpscout", "source_address": address, "channel_id": channel,
	}).str("id")
	path := "/v1/migrations/" + id
	actions := func() []string {
		var out []string
		for _, a := range d.tm.owner.expect(http.StatusOK, "GET", path, nil).body["actions"].([]any) {
			out = append(out, a.(string))
		}
		return out
	}
	if got := actions(); !slices.Contains(got, "set_credentials") || slices.Contains(got, "sign_in") {
		t.Fatalf("actions: %v", got)
	}

	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/sign-in", map[string]any{"client_id": "hs-client", "client_secret": "hs-secret"})
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "PUT", path+"/credentials", map[string]any{"username": "a", "password": "b"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "subdomain": "acme"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token, "password": "x"})
	d.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path+"/credentials", map[string]any{"username": "a", "password": "b", "token": token})
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "POST", path+"/test", nil)
	other := newTeam(t, d.h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	d.tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})

	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	got := d.tm.owner.expect(http.StatusOK, "GET", path, nil)
	cred := got.body["credentials"].(map[string]any)
	if cred["kind"] != "token" || cred["server"].(map[string]any)["host"] != helpdesk.HelpScoutAPIHost || cred["scopes"] != nil {
		t.Fatalf("credentials: %v", cred)
	}
	if strings.Contains(string(got.raw), token) {
		t.Fatal("the token was answered")
	}
	test := d.tm.owner.expect(http.StatusOK, "POST", path+"/test", nil)
	if test.body["ok"] != true || d.state(id) != "connected" {
		t.Fatalf("test: %s, state %s", test.raw, d.state(id))
	}
	d.tm.owner.expect(http.StatusOK, "POST", path+"/scan", nil)
	d.work(t, func() bool { return d.state(id) == "ready" })

	for _, q := range []string{
		"SELECT count(*) FROM migration_credentials WHERE workspace_id = $1 AND position(convert_to($2, 'UTF8') IN sealed) > 0",
		"SELECT count(*) FROM migration_audit WHERE workspace_id = $1 AND detail::text LIKE '%' || $2 || '%'",
		"SELECT count(*) FROM river_job WHERE args->>'workspace_id' = $1::text AND args::text LIKE '%' || $2 || '%'",
		"SELECT count(*) FROM migrations WHERE workspace_id = $1 AND (throttle::text || source_state::text) LIKE '%' || $2 || '%'",
	} {
		if n := d.h.count(q, d.tm.ws, token); n != 0 {
			t.Errorf("the token is stored in the clear: %s", q)
		}
	}
	for _, r := range d.desk.Requests() {
		if r.Path == "/v2/oauth2/token" {
			continue
		}
		if r.Method != http.MethodGet || r.Host != helpdesk.HelpScoutAPIHost || r.Auth != "Bearer "+token {
			t.Errorf("%s %s%s with %q", r.Method, r.Host, r.Path, r.Auth)
		}
	}

	if _, err := d.h.st.Pool.Exec(context.Background(), "UPDATE migrations SET state = 'needs_attention' WHERE workspace_id = $1 AND id = $2", d.tm.ws, id); err != nil {
		t.Fatal(err)
	}
	me, err := helpdesktestFixture("helpscout/users_me.json")
	if err != nil {
		t.Fatal(err)
	}
	d.desk.Override("helpscout/users_me.json", []byte(strings.Replace(me, `"id": 501`, `"id": 777`, 1)))
	d.tm.owner.expectProblem(http.StatusConflict, "migration_state", "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	d.desk.Override("helpscout/users_me.json", []byte(me))
	d.tm.owner.expect(http.StatusNoContent, "PUT", path+"/credentials", map[string]any{"kind": "token", "token": token})
	if got := d.str(t, "SELECT state FROM migrations WHERE workspace_id = $1 AND id = $2", d.tm.ws, id); got != "needs_attention" {
		t.Fatalf("state after the reconnect: %s", got)
	}

	d.tm.owner.expect(http.StatusOK, "POST", path+"/cancel", nil)
	if n := d.h.count("SELECT count(*) FROM migration_credentials WHERE workspace_id = $1", d.tm.ws); n != 0 {
		t.Errorf("%d credentials left after cancel", n)
	}
}

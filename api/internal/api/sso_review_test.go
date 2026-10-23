package api_test

import (
	"context"
	"net/http"
	"testing"
	"uuid"
)

// ssoSessionFor signs the person in with a code and turns that session into one made through ws's
// single sign-on, as the callback would make it.
func ssoSessionFor(t *testing.T, h *harness, email string, ws uuid.UUID) *client {
	t.Helper()
	ids := func() map[string]bool {
		out := map[string]bool{}
		rows, err := h.st.Pool.Query(context.Background(), "SELECT s.id::text FROM sessions s JOIN people p ON p.id = s.person_id WHERE p.email = $1", email)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			out[id] = true
		}
		return out
	}
	before := ids()
	c := h.client()
	c.signIn(email)
	for id := range ids() {
		if before[id] {
			continue
		}
		if _, err := h.st.Pool.Exec(context.Background(),
			"UPDATE sessions SET method = 'sso', sso_workspace_id = $2, sso_settings_id = $3 WHERE id = $1", id, ws, uuid.New()); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestSSOSessionLeavesPersonWideSettingsAlone(t *testing.T) {
	h, fake := pushHarness(t)
	email := unique("ada") + "@example.com"
	w := h.bootstrap(email, unique("w"))
	x := h.bootstrap(unique("x-owner")+"@example.com", unique("x"))
	if _, err := h.st.Pool.Exec(context.Background(), "INSERT INTO members (id, workspace_id, person_id, role) VALUES ($1, $2, $3, 'admin')",
		uuid.New(), x.WorkspaceID, w.PersonID); err != nil {
		t.Fatal(err)
	}
	code := h.client()
	code.signIn(email)
	device := fake.subscribe(code)
	sso := ssoSessionFor(t, h, email, w.WorkspaceID)

	me := sso.expect(http.StatusOK, "GET", "/v1/me", nil)
	if ms := me.body["memberships"].([]any); len(ms) != 1 || ms[0].(map[string]any)["workspace"].(map[string]any)["id"] != w.WorkspaceID.String() {
		t.Fatalf("memberships: %s", me.raw)
	}
	if n := len(code.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)); n != 2 {
		t.Fatalf("a code session sees %d memberships", n)
	}
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "PATCH", "/v1/me", map[string]any{"name": "Someone else"})
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "PATCH", "/v1/me", map[string]any{"availability": "away"})
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "PATCH", "/v1/account", map[string]any{"name": "Someone else"})
	sso.expect(http.StatusOK, "PATCH", "/v1/account", map[string]any{"email_notifications": false})
	sessions := sso.expect(http.StatusOK, "GET", "/v1/me/sessions", nil).body["items"].([]any)
	if len(sessions) != 1 || sessions[0].(map[string]any)["current"] != true {
		t.Fatalf("sessions: %v", sessions)
	}
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "DELETE", "/v1/me/sessions", nil)

	if items := sso.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("an SSO session lists the person's devices: %v", items)
	}
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "DELETE", "/v1/me/push-subscriptions/"+device.id, nil)
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/me/push-subscriptions/"+device.id+"/test", nil)
	sso.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/me/push-subscriptions",
		map[string]any{"endpoint": fake.srv.URL + device.path, "keys": device.keys()})
	own := fake.subscribe(sso)
	if items := sso.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != own.id {
		t.Fatalf("own subscriptions: %v", items)
	}
	sso.expect(http.StatusNoContent, "DELETE", "/v1/me/push-subscriptions/"+own.id, nil)

	got := code.expect(http.StatusOK, "GET", "/v1/me", nil)
	if got.body["person"].(map[string]any)["availability"] != "auto" || len(code.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any)) != 1 {
		t.Fatalf("the person after the SSO session's tries: %s", got.raw)
	}
	if n := len(code.expect(http.StatusOK, "GET", "/v1/me/sessions", nil).body["items"].([]any)); n != 2 {
		t.Fatalf("%d sessions left", n)
	}
}

func TestSSOReplacedProviderEndsItsSessions(t *testing.T) {
	e := newSSOEnv(t)
	h := e.h
	e.setUp()
	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"jit": true})
	agentEmail := "agent@" + e.domain
	agent := h.client()
	_, res := e.signIn(agent, agentEmail)
	wantLocation(t, res, "/conversations")
	ownerSSO := h.client()
	_, res = e.signIn(ownerSSO, "owner@"+e.domain)
	wantLocation(t, res, "/conversations")
	agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	ownerSSO.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "allowed_groups": []string{"support"}})
	agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	ownerSSO.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": "another-client", "client_secret": "s"})
	agent.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/inboxes", nil)
	ownerSSO.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	updated := e.auditActions(t)["sso.provider_updated"]
	if len(updated) == 0 || updated[0]["details"].(map[string]any)["sessions_ended"] != float64(1) {
		t.Fatalf("audit: %v", updated)
	}

	ownerSSO.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "client_secret": idpClientSecret})
	_, res = e.signIn(agent, agentEmail)
	wantLocation(t, res, "/conversations")
	agent.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	e.owner.expect(http.StatusNoContent, "DELETE", "/v1/sso/oidc", nil)
	agent.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/inboxes", nil)
}

func TestDeletedWorkspaceReleasesItsSSODomains(t *testing.T) {
	e := newSSOEnv(t)
	h := e.h
	e.setUp()
	rival := unique("rival") + "@example.com"
	h.bootstrap(rival, unique("ws"))
	rc := h.client()
	rc.signIn(rival)
	rc.expectProblem(http.StatusConflict, "domain_taken", "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	name := e.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).str("name")
	e.owner.expect(http.StatusAccepted, "DELETE", "/v1/workspace", map[string]any{"name": name})
	claim := rc.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	e.dns.set(claim.str("txt_name"), claim.str("txt_value"))
	if v := rc.expect(http.StatusOK, "POST", "/v1/sso/domains/"+claim.str("id")+"/verify", nil); v.str("state") != "verified" {
		t.Fatalf("verify after the other workspace was deleted: %s", v.raw)
	}
}

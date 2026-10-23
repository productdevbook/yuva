package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

func pendingInvites(t *testing.T, c *client, email string) int {
	t.Helper()
	n := 0
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/invites", nil).body["items"].([]any) {
		if it.(map[string]any)["email"] == email {
			n++
		}
	}
	return n
}

// TestSSOSessionTakeoverAcrossWorkspacesFails walks the #129 finding M2: the owner of workspace W
// claims a domain, points W at an identity provider they run and mints a sign-in for someone of
// that domain who owns workspace X. Every step that reached beyond W is refused.
func TestSSOSessionTakeoverAcrossWorkspacesFails(t *testing.T) {
	e := newSSOEnv(t)
	h := e.h
	ceo := "ceo@" + e.domain
	x := h.bootstrap(ceo, unique("victim"))
	victim := h.client()
	victim.signIn(ceo)
	victim.workspace = x.WorkspaceID.String()
	in := victim.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Chat", "slug": "chat", "mode": "live", "expected_reply_minutes": 30})
	origin := "https://" + unique("site") + ".example"
	ch := victim.expect(http.StatusCreated, "POST", "/v1/inboxes/"+in.body["inbox"].(map[string]any)["id"].(string)+"/channels", map[string]any{
		"kind": "chat", "name": "Website", "chat": map[string]any{"allowed_origins": []string{origin}, "allow_anonymous": true},
	})
	chatKey := ch.body["chat"].(map[string]any)["public_key"].(string)

	yOwner := unique("third") + "@example.com"
	h.bootstrap(yOwner, unique("ws"))
	third := h.client()
	third.signIn(yOwner)
	third.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": ceo, "role": "admin"})

	e.setUp()
	attacker := h.client()
	_, res := e.signIn(attacker, ceo)
	wantLocation(t, res, "/sign-in?sso_error=not_member")
	attacker.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	if pendingInvites(t, third, ceo) != 1 {
		t.Fatal("an SSO sign-in that was refused took another workspace's invite")
	}

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"jit": true})
	before := len(h.mail.to(ceo))
	_, res = e.signIn(attacker, ceo)
	wantLocation(t, res, "/conversations")
	if pendingInvites(t, third, ceo) != 1 {
		t.Fatal("an SSO sign-in accepted another workspace's invite")
	}
	me := attacker.expect(http.StatusOK, "GET", "/v1/me", nil)
	if ms := me.body["memberships"].([]any); len(ms) != 1 || ms[0].(map[string]any)["workspace"].(map[string]any)["id"] != e.ws.WorkspaceID.String() {
		t.Fatalf("memberships: %s", me.raw)
	}
	if n := h.count("SELECT count(*) FROM sessions s JOIN people p ON p.id = s.person_id WHERE p.email = $1 AND s.method = 'sso' AND s.user_agent IS NOT NULL", ceo); n != 1 {
		t.Fatalf("%d SSO sessions record their device", n)
	}
	mails := h.mail.to(ceo)[before:]
	if len(mails) != 1 || !strings.Contains(mails[0].Subject, "through single sign-on") || !strings.Contains(mails[0].Text, ceo) {
		t.Fatalf("first sign-in mail: %+v", mails)
	}

	attacker.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	attacker.workspace = e.ws.WorkspaceID.String()
	attacker.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	attacker.workspace = x.WorkspaceID.String()
	scoped := attacker.expect(http.StatusForbidden, "GET", "/v1/inboxes", nil)
	if scoped.str("code") != "sso_session_scope" || scoped.str("sso_workspace_id") != e.ws.WorkspaceID.String() {
		t.Fatalf("session in another workspace: %s", scoped.raw)
	}
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/invites", map[string]any{"email": "spy@example.com", "role": "owner"})
	attacker.workspace = ""
	attacker.expect(http.StatusOK, "GET", "/v1/account", nil)

	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/me/passkeys/options", nil)
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/me/passkeys",
		map[string]any{"ceremony_id": "x", "credential": map[string]any{}})
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "DELETE", "/v1/me/passkeys/"+uuid.New().String(), nil)
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/workspaces", map[string]any{"name": "Mine now"})
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "DELETE", "/v1/me", map[string]any{"email": ceo})
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/account/reader-tokens", map[string]any{"channel_key": chatKey, "origin": origin})
	attacker.expectProblem(http.StatusForbidden, "sso_session_scope", "POST", "/v1/oauth/requests/"+uuid.New().String()+"/approve",
		map[string]any{"workspace_id": x.WorkspaceID.String(), "scopes": []string{"conversations:read"}})
	victim.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	if n := h.count("SELECT count(*) FROM passkeys p JOIN people pe ON pe.id = p.person_id WHERE pe.email = $1", ceo); n != 0 {
		t.Fatalf("%d passkeys", n)
	}

	_, res = e.signIn(h.client(), ceo)
	wantLocation(t, res, "/conversations")
	if n := len(h.mail.to(ceo)) - before; n != 1 {
		t.Fatalf("a second sign-in mailed again: %d mails", n)
	}

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	ownerByCode := h.client()
	ownerByCode.signIn("owner@" + e.domain)
	ownerByCode.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	agentByCode := h.client()
	agentByCode.signIn(ceo)
	agentByCode.workspace = e.ws.WorkspaceID.String()
	agentByCode.expectProblem(http.StatusForbidden, "sso_required", "GET", "/v1/inboxes", nil)
	agentByCode.workspace = x.WorkspaceID.String()
	agentByCode.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
}

func TestSSOPasskeyChangesAreMailed(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	owner := h.client()
	owner.signIn(email)
	auth := newSoftAuthenticator(t)
	before := len(h.mail.to(email))
	begin := owner.expect(http.StatusOK, "POST", "/v1/me/passkeys/options", nil)
	created := owner.expect(http.StatusCreated, "POST", "/v1/me/passkeys",
		map[string]any{"ceremony_id": begin.str("ceremony_id"), "name": "Laptop", "credential": auth.create(begin.body["options"].(map[string]any))})
	owner.expect(http.StatusNoContent, "DELETE", "/v1/me/passkeys/"+created.str("id"), nil)
	owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/passkeys/"+created.str("id"), nil)
	mails := h.mail.to(email)[before:]
	if len(mails) != 2 || mails[0].Subject != "A passkey was added to your Yuva account" || !strings.Contains(mails[0].Text, `"Laptop"`) ||
		mails[1].Subject != "A passkey was removed from your Yuva account" {
		t.Fatalf("passkey mails: %+v", mails)
	}
}

func TestSSODomainLapsesWithoutItsRecord(t *testing.T) {
	e := newSSOEnv(t)
	h := e.h
	ctx := context.Background()
	e.setUp()
	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	d := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil).body["domains"].([]any)[0].(map[string]any)
	ws := e.ws.WorkspaceID
	id := uuid.MustParse(d["id"].(string))
	ownerEmail := "owner@" + e.domain

	due := func() bool {
		rows, err := h.st.ListSsoDomainsToRecheck(ctx, store.ListSsoDomainsToRecheckParams{Due: h.clock.Now().Add(-24 * time.Hour), Lim: 10000})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.ID == id {
				return true
			}
		}
		return false
	}
	if due() {
		t.Fatal("a domain verified now is due for a check")
	}
	h.clock.Advance(25 * time.Hour)
	if !due() {
		t.Fatal("a domain checked a day ago is not due")
	}
	got, err := h.srv.RecheckSSODomain(ctx, ws, id)
	if err != nil || got.MissingSince != nil || got.VerifiedAt == nil {
		t.Fatalf("recheck with the record: %+v %v", got, err)
	}

	e.dns.mu.Lock()
	delete(e.dns.txt, d["txt_name"].(string))
	e.dns.mu.Unlock()
	h.clock.Advance(25 * time.Hour)
	first, err := h.srv.RecheckSSODomain(ctx, ws, id)
	if err != nil || first.MissingSince == nil || first.VerifiedAt == nil {
		t.Fatalf("first miss: %+v %v", first, err)
	}
	shown := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil).body["domains"].([]any)[0].(map[string]any)
	if shown["state"] != "verified" || !strings.Contains(shown["problem"].(string), "no TXT record") {
		t.Fatalf("domain while missing: %v", shown)
	}
	h.clock.Advance(6 * 24 * time.Hour)
	if got, err := h.srv.RecheckSSODomain(ctx, ws, id); err != nil || got.VerifiedAt == nil {
		t.Fatalf("lapsed after six days: %+v %v", got, err)
	}
	rival := unique("rival") + "@example.com"
	h.bootstrap(rival, unique("ws"))
	rc := h.client()
	rc.signIn(rival)
	rc.expectProblem(http.StatusConflict, "domain_taken", "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})

	before := len(h.mail.to(ownerEmail))
	h.clock.Advance(24*time.Hour + time.Minute)
	lapsed, err := h.srv.RecheckSSODomain(ctx, ws, id)
	if err != nil || lapsed.VerifiedAt != nil {
		t.Fatalf("after seven days: %+v %v", lapsed, err)
	}
	settings := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if settings.body["sso_required"] != false || settings.body["domains"].([]any)[0].(map[string]any)["state"] != "pending" {
		t.Fatalf("settings after the lapse: %s", settings.raw)
	}
	audit := e.auditActions(t)["sso_domain.lapsed"]
	if len(audit) != 1 || audit[0]["actor"].(map[string]any)["kind"] != "system" ||
		audit[0]["details"].(map[string]any)["sso_required_off"] != true || audit[0]["details"].(map[string]any)["domain"] != e.domain {
		t.Fatalf("audit: %v", audit)
	}
	mails := h.mail.to(ownerEmail)[before:]
	if len(mails) != 1 || !strings.Contains(mails[0].Subject, "no longer verified") || !strings.Contains(mails[0].Text, "no longer required") {
		t.Fatalf("owner mail: %+v", mails)
	}
	if got := h.client().expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": "someone@" + e.domain}); got.body["sso"] != false {
		t.Fatalf("a lapsed claim still starts SSO: %s", got.raw)
	}
	claim := rc.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	e.dns.set(claim.str("txt_name"), claim.str("txt_value"))
	if v := rc.expect(http.StatusOK, "POST", "/v1/sso/domains/"+claim.str("id")+"/verify", nil); v.str("state") != "verified" {
		t.Fatalf("rival verify: %s", v.raw)
	}
	if got, err := h.srv.RecheckSSODomain(ctx, ws, id); err != nil || got.VerifiedAt != nil {
		t.Fatalf("recheck of a lapsed claim: %+v %v", got, err)
	}
}

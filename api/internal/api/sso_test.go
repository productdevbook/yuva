package api_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/productdevbook/yuva/api/internal/api"
)

const (
	idpClientID     = "yuva-test-client"
	idpClientSecret = "yuva-test-secret"
)

type idpGrant struct {
	nonce     string
	challenge string
	email     string
}

// fakeIdP is an OpenID Connect provider with its own keys; tune changes the claims or the signer of
// the next id_token it issues.
type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	kid    string
	clock  func() time.Time
	mu     sync.Mutex
	grants map[string]idpGrant
	tune   func(claims map[string]any) *rsa.PrivateKey
}

func newFakeIdP(t *testing.T, clock func() time.Time) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{t: t, key: key, kid: "k1", clock: clock, grants: map[string]idpGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		u := idp.srv.URL
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": u, "authorization_endpoint": u + "/authorize", "token_endpoint": u + "/token", "jwks_uri": u + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		idp.mu.Lock()
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &idp.key.PublicKey, KeyID: idp.kid, Algorithm: "RS256", Use: "sig"}}}
		idp.mu.Unlock()
		_ = json.NewEncoder(w).Encode(set)
	})
	mux.HandleFunc("POST /token", idp.token)
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (idp *fakeIdP) rotate() {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		idp.t.Fatal(err)
	}
	idp.mu.Lock()
	idp.key, idp.kid = key, idp.kid+"r"
	idp.mu.Unlock()
}

func (idp *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	idp.mu.Lock()
	g, found := idp.grants[r.PostForm.Get("code")]
	delete(idp.grants, r.PostForm.Get("code"))
	key, kid, tune := idp.key, idp.kid, idp.tune
	idp.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if id != idpClientID || secret != idpClientSecret || !found || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge ||
		r.PostForm.Get("grant_type") != "authorization_code" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	now := idp.clock()
	claims := map[string]any{
		"iss": idp.srv.URL, "aud": idpClientID, "sub": "sub-" + g.email, "email": g.email, "email_verified": true,
		"name": "Ada Lovelace", "nonce": g.nonce, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	if tune != nil {
		if k := tune(claims); k != nil {
			key = k
		}
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", kid).WithType("JWT"))
	if err != nil {
		idp.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	jws, err := signer.Sign(payload)
	if err != nil {
		idp.t.Fatal(err)
	}
	raw, _ := jws.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
}

type ssoDNS struct {
	mu  sync.Mutex
	txt map[string][]string
}

func (d *ssoDNS) LookupCNAME(context.Context, string) (string, error) { return "", notFound("") }
func (d *ssoDNS) LookupHost(context.Context, string) ([]string, error) {
	return nil, notFound("")
}
func (d *ssoDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.txt[name]; ok {
		return v, nil
	}
	return nil, notFound(name)
}

func (d *ssoDNS) set(name, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.txt[name] = []string{value}
}

type ssoEnv struct {
	h      *harness
	idp    *fakeIdP
	dns    *ssoDNS
	owner  *client
	ws     api.BootstrapResult
	domain string
}

func newSSOEnv(t *testing.T) *ssoEnv {
	dns := &ssoDNS{txt: map[string][]string{}}
	var idp *fakeIdP
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		d.Status.Resolver = dns
	})
	idp = newFakeIdP(t, h.clock.Now)
	domain := unique("acme") + ".test"
	ownerEmail := "owner@" + domain
	ws := h.bootstrap(ownerEmail, unique("ws"))
	owner := h.client()
	owner.signIn(ownerEmail)
	return &ssoEnv{h: h, idp: idp, dns: dns, owner: owner, ws: ws, domain: domain}
}

// setUp configures the provider and claims the domain.
func (e *ssoEnv) setUp() {
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "client_secret": idpClientSecret})
	d := e.owner.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	e.dns.set(d.str("txt_name"), d.str("txt_value"))
	v := e.owner.expect(http.StatusOK, "POST", "/v1/sso/domains/"+d.str("id")+"/verify", nil)
	if v.str("state") != "verified" {
		e.h.t.Fatalf("verify: %s", v.raw)
	}
}

// signIn runs the browser's round trip and answers the callback's redirect.
func (e *ssoEnv) signIn(c *client, email string) (string, *http.Response) {
	t := e.h.t
	t.Helper()
	start := c.expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": email, "return_to": "/conversations"})
	if start.body["sso"] != true {
		t.Fatalf("start: %s", start.raw)
	}
	auth, err := url.Parse(start.str("authorization_url"))
	if err != nil {
		t.Fatal(err)
	}
	q := auth.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("login_hint") != email || q.Get("client_id") != idpClientID ||
		q.Get("redirect_uri") != testOrigin+"/auth/sso/callback" || !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("authorization url: %s", auth)
	}
	code := unique("code")
	e.idp.mu.Lock()
	e.idp.grants[code] = idpGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), email: email}
	e.idp.mu.Unlock()
	callback := "/auth/sso/callback?" + url.Values{"code": {code}, "state": {q.Get("state")}}.Encode()
	return callback, e.callback(c, callback)
}

func (e *ssoEnv) callback(c *client, path string) *http.Response {
	raw := &http.Client{Jar: c.http.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("GET", e.h.url+path, nil)
	req.Header.Set("X-Forwarded-For", c.ip)
	res, err := raw.Do(req)
	if err != nil {
		e.h.t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func wantLocation(t *testing.T, res *http.Response, want string) {
	t.Helper()
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != testOrigin+want {
		t.Fatalf("callback: %d %s, want %s", res.StatusCode, res.Header.Get("Location"), want)
	}
}

func (e *ssoEnv) auditActions(t *testing.T) map[string][]map[string]any {
	t.Helper()
	out := map[string][]map[string]any{}
	r := e.owner.expect(http.StatusOK, "GET", "/v1/audit-log?limit=100", nil)
	for _, it := range r.body["items"].([]any) {
		m := it.(map[string]any)
		out[m["action"].(string)] = append(out[m["action"].(string)], m)
	}
	return out
}

func TestSSOSignInWithJIT(t *testing.T) {
	e := newSSOEnv(t)
	agent := h2client(e)
	agent.expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": "ada@" + e.domain})
	e.setUp()

	unclaimed := agent.expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": "ada@" + unique("elsewhere") + ".test"})
	if unclaimed.body["sso"] != false || unclaimed.body["authorization_url"] != nil {
		t.Fatalf("unclaimed: %s", unclaimed.raw)
	}

	email := "ada@" + e.domain
	_, res := e.signIn(agent, email)
	wantLocation(t, res, "/sign-in?sso_error=not_member")

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"jit": true})
	callback, res := e.signIn(agent, email)
	wantLocation(t, res, "/conversations")
	me := agent.expect(http.StatusOK, "GET", "/v1/me", nil)
	ms := me.body["memberships"].([]any)
	if len(ms) != 1 || ms[0].(map[string]any)["role"] != "agent" || me.body["person"].(map[string]any)["name"] != "Ada Lovelace" {
		t.Fatalf("me after sso: %s", me.raw)
	}
	wantLocation(t, e.callback(agent, callback), "/sign-in?sso_error=expired")
	wantLocation(t, e.callback(h2client(e), callback), "/sign-in?sso_error=expired")

	e.idp.rotate()
	_, res = e.signIn(h2client(e), email)
	wantLocation(t, res, "/conversations")

	audit := e.auditActions(t)
	for _, a := range []string{"sso.provider_created", "sso_domain.added", "sso_domain.verified", "sso.settings_changed", "sso.signed_in", "member.joined"} {
		if len(audit[a]) == 0 {
			t.Fatalf("audit has no %s: %v", a, audit)
		}
	}
	refused := audit["sso.sign_in_refused"]
	if len(refused) != 1 || refused[0]["details"].(map[string]any)["reason"] != "not_member" || refused[0]["actor"].(map[string]any)["kind"] != "system" {
		t.Fatalf("refusals: %v", refused)
	}
	joined := audit["member.joined"][0]["details"].(map[string]any)
	if joined["role"] != "agent" || joined["via"] != "sso" {
		t.Fatalf("joined: %v", joined)
	}
	signed := audit["sso.signed_in"]
	if signed[len(signed)-1]["details"].(map[string]any)["joined"] != "jit" || signed[0]["actor"].(map[string]any)["kind"] != "member" {
		t.Fatalf("signed in: %v", signed)
	}
}

func h2client(e *ssoEnv) *client { return e.h.client() }

func TestSSORefusesBadTokens(t *testing.T) {
	e := newSSOEnv(t)
	e.setUp()
	email := "owner@" + e.domain
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		tune func(map[string]any) *rsa.PrivateKey
		code string
	}{
		{"wrong audience", func(c map[string]any) *rsa.PrivateKey { c["aud"] = "someone-else"; return nil }, "invalid_token"},
		{"wrong issuer", func(c map[string]any) *rsa.PrivateKey { c["iss"] = "https://evil.example"; return nil }, "invalid_token"},
		{"expired", func(c map[string]any) *rsa.PrivateKey {
			c["exp"] = e.h.clock.Now().Add(-time.Minute).Unix()
			return nil
		}, "invalid_token"},
		{"bad signature", func(map[string]any) *rsa.PrivateKey { return other }, "invalid_token"},
		{"nonce of another sign-in", func(c map[string]any) *rsa.PrivateKey { c["nonce"] = "replayed-nonce"; return nil }, "invalid_token"},
		{"unverified e-mail", func(c map[string]any) *rsa.PrivateKey { c["email_verified"] = false; return nil }, "email_unverified"},
		{"no email_verified", func(c map[string]any) *rsa.PrivateKey { delete(c, "email_verified"); return nil }, "email_unverified"},
		{"unclaimed domain", func(c map[string]any) *rsa.PrivateKey { c["email"] = "owner@" + unique("other") + ".test"; return nil }, "domain_not_claimed"},
	}
	for _, tc := range cases {
		e.idp.mu.Lock()
		e.idp.tune = tc.tune
		e.idp.mu.Unlock()
		c := e.h.client()
		_, res := e.signIn(c, email)
		wantLocation(t, res, "/sign-in?sso_error="+tc.code)
		c.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	}
	e.idp.mu.Lock()
	e.idp.tune = func(c map[string]any) *rsa.PrivateKey { c["groups"] = []string{"staff"}; return nil }
	e.idp.mu.Unlock()
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "allowed_groups": []string{"support"}})
	_, res := e.signIn(e.h.client(), email)
	wantLocation(t, res, "/sign-in?sso_error=group_not_allowed")
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "allowed_groups": []string{"support", "staff"}})
	_, res = e.signIn(e.h.client(), email)
	wantLocation(t, res, "/conversations")

	noCookie := e.h.client()
	start := noCookie.expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": email})
	u, _ := url.Parse(start.str("authorization_url"))
	wantLocation(t, e.callback(e.h.client(), "/auth/sso/callback?code=x&state="+u.Query().Get("state")), "/sign-in?sso_error=expired")
	wantLocation(t, e.callback(noCookie, "/auth/sso/callback?error=access_denied&state="+u.Query().Get("state")), "/sign-in?sso_error=denied")

	audit := e.auditActions(t)
	if n := len(audit["sso.sign_in_refused"]); n != len(cases)+1 {
		t.Fatalf("refusals: %d, want %d", n, len(cases)+1)
	}
}

func TestSSORequiredGatesWorkspaceAccess(t *testing.T) {
	e := newSSOEnv(t)
	e.owner.expectProblem(http.StatusConflict, "sso_not_ready", "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	agentEmail := "agent@" + e.domain
	e.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})
	otherOwner := unique("other") + "@example.com"
	other := e.h.bootstrap(otherOwner, unique("ws"))
	otherClient := e.h.client()
	otherClient.signIn(otherOwner)
	otherClient.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})

	codeSession := e.h.client()
	codeSession.signIn(agentEmail)
	codeSession.workspace = e.ws.WorkspaceID.String()
	codeSession.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	e.setUp()
	e.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/sso", map[string]any{"jit_role": "owner"})
	r := e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	if r.body["sso_required"] != true || r.body["jit"] != false || r.body["jit_role"] != "agent" {
		t.Fatalf("settings: %s", r.raw)
	}

	codeSession.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	if a := e.auditActions(t)["sso.settings_changed"]; len(a) == 0 || a[0]["details"].(map[string]any)["sessions_revoked"] != float64(1) {
		t.Fatalf("audit of the revoked sessions: %v", a)
	}

	fresh := e.h.client()
	fresh.signIn(agentEmail)
	fresh.workspace = e.ws.WorkspaceID.String()
	refused := fresh.expect(http.StatusForbidden, "GET", "/v1/inboxes", nil)
	if refused.str("code") != "sso_required" || refused.str("sso_email") != agentEmail {
		t.Fatalf("code session: %s", refused.raw)
	}
	fresh.expect(http.StatusOK, "GET", "/v1/me", nil)
	fresh.expect(http.StatusOK, "GET", "/v1/account", nil)
	fresh.workspace = other.WorkspaceID.String()
	fresh.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	ownerByCode := e.h.client()
	ownerByCode.signIn("owner@" + e.domain)
	ownerByCode.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)

	start := e.h.client().expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": agentEmail})
	if start.body["required"] != true {
		t.Fatalf("start: %s", start.raw)
	}
	viaSSO := e.h.client()
	_, res := e.signIn(viaSSO, agentEmail)
	wantLocation(t, res, "/conversations")
	viaSSO.workspace = e.ws.WorkspaceID.String()
	viaSSO.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	viaSSO.workspace = other.WorkspaceID.String()
	viaSSO.expectProblem(http.StatusForbidden, "sso_session_scope", "GET", "/v1/inboxes", nil)

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": false})
	fresh.workspace = e.ws.WorkspaceID.String()
	fresh.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	e.owner.expect(http.StatusNoContent, "DELETE", "/v1/sso/oidc", nil)
	got := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if got.body["sso_required"] != false || got.body["oidc"] != nil || len(got.body["domains"].([]any)) != 1 {
		t.Fatalf("after provider removed: %s", got.raw)
	}
	viaSSO.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	fresh.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	again := e.h.client()
	again.signIn(agentEmail)
	again.workspace = e.ws.WorkspaceID.String()
	again.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
}

func TestSSOSettingsAccessAndDomains(t *testing.T) {
	e := newSSOEnv(t)
	e.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID})
	e.owner.expectProblem(http.StatusBadRequest, "sso_discovery_failed", "PUT", "/v1/sso/oidc",
		map[string]any{"issuer": e.idp.srv.URL + "/nope", "client_id": idpClientID, "client_secret": idpClientSecret})
	e.owner.expectProblem(http.StatusBadRequest, "sso_discovery_failed", "PUT", "/v1/sso/oidc",
		map[string]any{"issuer": "http://169.254.169.254", "client_id": idpClientID, "client_secret": idpClientSecret})
	e.setUp()
	got := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if got.body["redirect_uri"] != testOrigin+"/auth/sso/callback" || strings.Contains(string(got.raw), idpClientSecret) {
		t.Fatalf("settings: %s", got.raw)
	}
	e.owner.expectProblem(http.StatusConflict, "domain_exists", "POST", "/v1/sso/domains", map[string]any{"domain": strings.ToUpper(e.domain)})
	e.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/sso/domains", map[string]any{"domain": "not a domain"})

	otherEmail := unique("rival") + "@example.com"
	otherWS := e.h.bootstrap(otherEmail, unique("ws"))
	rival := e.h.client()
	rival.signIn(otherEmail)
	rival.expectProblem(http.StatusConflict, "domain_taken", "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	rival.workspace = e.ws.WorkspaceID.String()
	rival.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/sso", nil)
	rival.workspace = otherWS.WorkspaceID.String()
	rivalSSO := rival.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if len(rivalSSO.body["domains"].([]any)) != 0 {
		t.Fatalf("rival sees: %s", rivalSSO.raw)
	}

	pendingDomain := unique("shared") + ".test"
	mine := e.owner.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": pendingDomain})
	theirs := rival.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": pendingDomain})
	failed := e.owner.expect(http.StatusOK, "POST", "/v1/sso/domains/"+mine.str("id")+"/verify", nil)
	if failed.str("state") != "pending" || !strings.Contains(failed.str("problem"), "no TXT record") {
		t.Fatalf("verify without record: %s", failed.raw)
	}
	e.dns.set(theirs.str("txt_name"), theirs.str("txt_value"))
	rival.expect(http.StatusOK, "POST", "/v1/sso/domains/"+theirs.str("id")+"/verify", nil)
	e.dns.set(mine.str("txt_name"), mine.str("txt_value"))
	e.owner.expectProblem(http.StatusConflict, "domain_taken", "POST", "/v1/sso/domains/"+mine.str("id")+"/verify", nil)
	rival.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/sso/domains/"+mine.str("id"), nil)
	e.owner.expect(http.StatusNoContent, "DELETE", "/v1/sso/domains/"+mine.str("id"), nil)

	agentEmail := unique("agent") + "@example.com"
	e.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "admin"})
	admin := e.h.client()
	admin.signIn(agentEmail)
	admin.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/sso", nil)
	admin.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/sso", map[string]any{"jit": true})
	key := e.h.client()
	key.bearer = e.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend", "scopes": []string{"workspace:manage"}}).str("secret")
	key.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/sso", nil)
	e.h.client().expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/sso", nil)
}

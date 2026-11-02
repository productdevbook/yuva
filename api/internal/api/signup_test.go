package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/signupguard"
)

func openSignup(perPerson, perIP int) func(*api.Deps) {
	return func(d *api.Deps) {
		d.Auth.Signup = api.SignupSettings{Open: true, WorkspacesPerPerson: perPerson, WorkspacesPerIP: perIP}
	}
}

func TestSignupInviteOnlyByDefault(t *testing.T) {
	h := newHarness(t)
	c := h.client()
	if v := c.expect(http.StatusOK, "GET", "/v1/version", nil); v.str("signup") != "invite" {
		t.Fatalf("version: %s", v.raw)
	}
	stranger := unique("stranger") + "@example.com"
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": stranger})
	if n := len(h.mail.to(stranger)); n != 0 {
		t.Fatalf("%d mails sent to an unknown address", n)
	}

	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c.signIn(email)
	c.expectProblem(http.StatusForbidden, "signup_closed", "POST", "/v1/workspaces", map[string]any{"name": "Acme"})
}

func TestSignupOpen(t *testing.T) {
	h := newHarnessWith(t, openSignup(2, 10))
	other := unique("other") + "@example.com"
	theirs := h.bootstrap(other, unique("ws-other"))

	c := h.client()
	if v := c.expect(http.StatusOK, "GET", "/v1/version", nil); v.str("signup") != "open" {
		t.Fatalf("version: %s", v.raw)
	}
	c.expectProblem(http.StatusUnauthorized, "unauthenticated", "POST", "/v1/workspaces", map[string]any{"name": "Acme"})

	email := unique("new") + "@example.com"
	c.signIn(email)
	me := c.expect(http.StatusOK, "GET", "/v1/me", nil)
	if ms := me.body["memberships"].([]any); len(ms) != 0 {
		t.Fatalf("a new person has memberships: %s", me.raw)
	}

	c.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/workspaces", map[string]any{"name": "   "})
	c.workspace = theirs.WorkspaceID.String()
	ws := c.expect(http.StatusCreated, "POST", "/v1/workspaces", map[string]any{"name": "  Acme  "})
	if ws.str("name") != "Acme" || ws.str("id") == theirs.WorkspaceID.String() {
		t.Fatalf("workspace: %s", ws.raw)
	}

	me = c.expect(http.StatusOK, "GET", "/v1/me", nil)
	ms := me.body["memberships"].([]any)
	if len(ms) != 1 || ms[0].(map[string]any)["workspace"].(map[string]any)["id"] != ws.str("id") || ms[0].(map[string]any)["role"] != "owner" {
		t.Fatalf("memberships: %s", me.raw)
	}
	c.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/members", nil)

	c.workspace = ws.str("id")
	if got := c.expect(http.StatusOK, "GET", "/v1/workspace", nil); got.str("id") != ws.str("id") {
		t.Fatalf("workspace: %s", got.raw)
	}
	members := c.expect(http.StatusOK, "GET", "/v1/members", nil)
	if items := members.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["email"] != email {
		t.Fatalf("members: %s", members.raw)
	}
	if inboxes := c.expect(http.StatusOK, "GET", "/v1/inboxes", nil); len(inboxes.body["items"].([]any)) != 0 {
		t.Fatalf("inboxes: %s", inboxes.raw)
	}
	c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/members/"+theirs.MemberID.String(), nil)

	c.expect(http.StatusCreated, "POST", "/v1/workspaces", map[string]any{"name": "Second"})
	c.expectProblem(http.StatusForbidden, "workspace_limit", "POST", "/v1/workspaces", map[string]any{"name": "Third"})
}

func TestSignupWorkspacesPerIP(t *testing.T) {
	h := newHarnessWith(t, openSignup(5, 2))
	ip := h.client().ip
	for i := range 3 {
		c := h.client()
		c.ip = ip
		c.signIn(unique("new") + "@example.com")
		if i < 2 {
			c.expect(http.StatusCreated, "POST", "/v1/workspaces", map[string]any{"name": "Acme"})
		} else {
			c.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/workspaces", map[string]any{"name": "Acme"})
		}
	}
}

func TestSignupKeepsThePageLocale(t *testing.T) {
	h := newHarnessWith(t, openSignup(3, 10))
	c := h.client()
	c.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/auth/code", map[string]any{"email": unique("x") + "@example.com", "locale": "xx"})

	signUp := func(c *client, email string, reader bool) response {
		t.Helper()
		c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email, "locale": "tr", "reader": reader})
		msgs := h.mail.wait(t, email, 1)
		if subject := msgs[len(msgs)-1].Subject; !strings.HasPrefix(subject, "Yuva giriş kodunuz") {
			t.Fatalf("code mail to %s: %q", email, subject)
		}
		return c.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": h.mail.code(t, email)})
	}
	for _, reader := range []bool{false, true} {
		me := signUp(h.client(), unique("new")+"@example.com", reader)
		if l := me.body["person"].(map[string]any)["locale"]; l != "tr" {
			t.Fatalf("reader %v: new person's locale %v: %s", reader, l, me.raw)
		}
	}

	existing := unique("owner") + "@example.com"
	h.bootstrap(existing, unique("ws"))
	c = h.client()
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": existing, "locale": "tr"})
	if subject := h.mail.wait(t, existing, 1)[0].Subject; !strings.HasPrefix(subject, "Your Yuva sign-in code") {
		t.Fatalf("code mail to an existing person: %q", subject)
	}
	me := c.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": existing, "code": h.mail.code(t, existing)})
	if l := me.body["person"].(map[string]any)["locale"]; l != "en" {
		t.Fatalf("existing person's locale %v", l)
	}
}

func TestOperatorDigest(t *testing.T) {
	ops := []string{unique("op") + "@example.com", unique("op2") + "@example.com"}
	h := newHarnessWith(t, func(d *api.Deps) {
		openSignup(3, 10)(d)
		d.OperatorEmails = ops
	})
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, "DELETE FROM operator_notices"); err != nil {
		t.Fatal(err)
	}

	existing := unique("owner") + "@example.com"
	h.bootstrap(existing, unique("ws"))
	h.client().signIn(existing)

	first := unique("new") + "@example.com"
	c := h.client()
	c.signIn(first)
	c.expect(http.StatusCreated, "POST", "/v1/workspaces", map[string]any{"name": "Acme\nCorp"})
	if err := h.srv.SendOperatorDigest(ctx); err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		msgs := h.mail.wait(t, op, 1)
		m := msgs[len(msgs)-1]
		if !strings.Contains(m.Subject, "1 new sign-up and 1 new workspace") {
			t.Fatalf("subject %q", m.Subject)
		}
		if !strings.Contains(m.Text, "Sign-up: "+first+", from "+c.ip) || !strings.Contains(m.Text, `Workspace "Acme Corp" created by `+first) || strings.Contains(m.Text, existing) {
			t.Fatalf("text:\n%s", m.Text)
		}
		if m.HTML == "" {
			t.Fatal("no html part")
		}
	}

	reader := unique("reader") + "@example.com"
	rc := h.client()
	rc.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": reader, "reader": true})
	rc.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": reader, "code": h.mail.code(t, reader)})
	h.clock.Advance(4 * time.Minute)
	if err := h.srv.SendOperatorDigest(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(h.mail.to(ops[0])); n != 1 {
		t.Fatalf("%d digests within five minutes, want %d", n, 1)
	}
	h.clock.Advance(time.Minute)
	if err := h.srv.SendOperatorDigest(ctx); err != nil {
		t.Fatal(err)
	}
	m := h.mail.wait(t, ops[0], 2)[1]
	if m.Subject != "New reader sign-up: "+reader || !strings.Contains(m.Text, "Reader: "+reader) || strings.Contains(m.Text, first) {
		t.Fatalf("second digest %q:\n%s", m.Subject, m.Text)
	}
	h.clock.Advance(10 * time.Minute)
	if err := h.srv.SendOperatorDigest(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(h.mail.to(ops[0])); n != 2 {
		t.Fatalf("a digest with nothing new: %d mails", n)
	}
}

func TestOperatorDigestOff(t *testing.T) {
	h := newHarnessWith(t, openSignup(3, 10))
	email := unique("new") + "@example.com"
	c := h.client()
	c.signIn(email)
	c.expect(http.StatusCreated, "POST", "/v1/workspaces", map[string]any{"name": "Acme"})
	var n int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM operator_notices WHERE email = $1", email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d notices recorded with no operator address", n)
	}
}

func TestSignupRefusesBlockedDomain(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) {
		openSignup(2, 10)(d)
		d.Auth.Signup.Guard = signupguard.New([]string{"blocked.example"}, nil)
	})
	c := h.client()
	for _, domain := range []string{"mailinator.com", "sub.blocked.example"} {
		addr := unique("fake") + "@" + domain
		c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": addr})
		if n := len(h.mail.to(addr)); n != 0 {
			t.Fatalf("%d mails sent to %s", n, addr)
		}
		if n := h.count("SELECT count(*) FROM login_codes WHERE email = $1", addr); n != 0 {
			t.Fatalf("%d login_codes rows for %s", n, addr)
		}
	}
	ok := unique("real") + "@example.com"
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": ok})
	if n := len(h.mail.to(ok)); n != 1 {
		t.Fatalf("%d mails sent to %s", n, ok)
	}
}

package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"regexp"
	"sync"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

const testOrigin = "http://localhost:8080"

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type outbox struct {
	mu    sync.Mutex
	sent  []mail.Message
	codes map[string]int
}

func (o *outbox) Send(_ context.Context, m mail.Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, m)
	return nil
}

func (o *outbox) to(addr string) []mail.Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []mail.Message
	for _, m := range o.sent {
		if m.To == addr {
			out = append(out, m)
		}
	}
	return out
}

var codePattern = regexp.MustCompile(`\b\d{6}\b`)

// wait returns the mails to addr once there are at least n; mail is sent after the response.
func (o *outbox) wait(t *testing.T, addr string, n int) []mail.Message {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		msgs := o.to(addr)
		if len(msgs) >= n {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d mails to %s, want %d", len(msgs), addr, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// code returns the code of the next sign-in mail to addr that no earlier call returned.
func (o *outbox) code(t *testing.T, addr string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		o.mu.Lock()
		n := 0
		for _, m := range o.sent {
			if m.To != addr {
				continue
			}
			if c := codePattern.FindString(m.Text); c != "" {
				n++
				if n > o.codes[addr] {
					if o.codes == nil {
						o.codes = map[string]int{}
					}
					o.codes[addr] = n
					o.mu.Unlock()
					return c
				}
			}
		}
		o.mu.Unlock()
		if time.Now().After(deadline) {
			t.Fatalf("no new sign-in code mailed to %s", addr)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type harness struct {
	t       *testing.T
	st      *store.Store
	url     string
	clock   *clock
	mail    *outbox
	secrets *secret.Key
	storage *storage.Local
	hub     *realtime.Hub
	srv     *api.Server
	smtp    *smtpCapture
	web     *fakeWeb
}

const (
	testIngressSecret = "test-ingress-secret"
	testSESTopic      = "arn:aws:sns:eu-west-1:123456789012:yuva-ses"
)

const testAttachmentMaxBytes = 1024

var migrateOnce sync.Once

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, nil)
}

func newHarnessWith(t *testing.T, configure func(*api.Deps)) *harness {
	t.Helper()
	dsn := os.Getenv("YUVA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("YUVA_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	var migrateErr error
	migrateOnce.Do(func() {
		if migrateErr = st.MigrateUp(ctx, slog.New(slog.DiscardHandler)); migrateErr == nil {
			migrateErr = jobs.Migrate(ctx, st.Pool, slog.New(slog.DiscardHandler))
		}
	})
	if migrateErr != nil {
		t.Fatal(migrateErr)
	}
	wa, err := webauthn.New(&webauthn.Config{RPID: "localhost", RPDisplayName: "Yuva", RPOrigins: []string{testOrigin}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := secret.ParseKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hub := realtime.NewHub(256)
	listenCtx, stopListening := context.WithCancel(ctx)
	t.Cleanup(stopListening)
	go realtime.Listen(listenCtx, dsn, st.Queries, hub, slog.New(slog.DiscardHandler))
	h := &harness{t: t, st: st, clock: &clock{now: time.Now()}, mail: &outbox{}, secrets: key, storage: objects, hub: hub,
		smtp: &smtpCapture{}, web: newFakeWeb()}
	deps := api.Deps{
		Log:      slog.New(slog.DiscardHandler),
		Store:    st,
		Version:  "test",
		Mailer:   h.mail,
		WebAuthn: wa,
		Auth:     api.AuthSettings{PublicURL: testOrigin, ClientIPHeader: "X-Forwarded-For", CodeReplyDelay: time.Millisecond},
		Now:      h.clock.Now,
		Secrets:  key,
		Storage:  objects,
		Attachments: api.AttachmentSettings{
			MaxBytes: testAttachmentMaxBytes,
			Types:    []string{"text/plain", "image/*"},
		},
		Hub:         hub,
		Ingress:     api.IngressSettings{Secret: testIngressSecret, SESTopicARNs: []string{testSESTopic}},
		EmailSender: h.smtp,
		HTTPClient:  &http.Client{Transport: h.web},
	}
	if configure != nil {
		configure(&deps)
	}
	srv := api.New(deps)
	h.srv = srv
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	h.url = ts.URL
	return h
}

func unique(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func (h *harness) bootstrap(email, workspace string) api.BootstrapResult {
	h.t.Helper()
	res, err := api.Bootstrap(context.Background(), h.st, api.BootstrapInput{
		Email: email, Workspace: workspace, AllowExisting: true,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

type client struct {
	h         *harness
	http      *http.Client
	ip        string
	bearer    string
	workspace string
	origin    string
}

func (h *harness) client() *client {
	jar, _ := cookiejar.New(nil)
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return &client{h: h, http: &http.Client{Jar: jar, Transport: panelOrigin{}}, ip: fmt.Sprintf("10.%d.%d.%d", b[0], b[1], b[2])}
}

// panelOrigin sends the Origin a browser on the panel sends with every non-GET request.
type panelOrigin struct{}

func (panelOrigin) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet && r.Header.Get("Origin") == "" && r.Header.Get("Authorization") == "" && strings.HasPrefix(r.URL.Path, "/v1/") {
		r = r.Clone(r.Context())
		r.Header.Set("Origin", testOrigin)
	}
	return http.DefaultTransport.RoundTrip(r)
}

type response struct {
	status int
	body   map[string]any
	raw    []byte
	header http.Header
}

func (r response) str(key string) string {
	s, _ := r.body[key].(string)
	return s
}

func (c *client) do(method, path string, body any) response {
	c.h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.h.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.h.url+path, rd)
	if err != nil {
		c.h.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.workspace != "" {
		req.Header.Set("Yuva-Workspace", c.workspace)
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := response{status: res.StatusCode, raw: raw, header: res.Header}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.body)
	}
	return out
}

func (c *client) expect(want int, method, path string, body any) response {
	c.h.t.Helper()
	r := c.do(method, path, body)
	if r.status != want {
		c.h.t.Fatalf("%s %s: status %d, want %d: %s", method, path, r.status, want, r.raw)
	}
	return r
}

func (c *client) expectProblem(status int, code, method, path string, body any) {
	c.h.t.Helper()
	r := c.expect(status, method, path, body)
	if got := r.str("code"); got != code {
		c.h.t.Fatalf("%s %s: problem code %q, want %q: %s", method, path, got, code, r.raw)
	}
}

func (c *client) signIn(email string) {
	c.h.t.Helper()
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	code := c.h.mail.code(c.h.t, email)
	c.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})
}

func TestSignInWithCode(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	ws := h.bootstrap(email, unique("ws"))
	c := h.client()

	c.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	code := h.mail.code(t, email)
	r := c.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})
	setCookie := r.header.Get("Set-Cookie")
	for _, want := range []string{"yuva_session=", "HttpOnly", "SameSite=Lax"} {
		if !bytes.Contains([]byte(setCookie), []byte(want)) {
			t.Fatalf("Set-Cookie %q lacks %q", setCookie, want)
		}
	}

	me := c.expect(http.StatusOK, "GET", "/v1/me", nil)
	memberships := me.body["memberships"].([]any)
	if len(memberships) != 1 || memberships[0].(map[string]any)["member_id"] != ws.MemberID.String() {
		t.Fatalf("memberships: %s", me.raw)
	}

	c.expectProblem(http.StatusBadRequest, "invalid_code", "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})

	c.expect(http.StatusNoContent, "POST", "/v1/auth/sign-out", nil)
	c.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
}

func TestSignInCodeExpires(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c := h.client()
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	code := h.mail.code(t, email)
	h.clock.Advance(10*time.Minute + time.Second)
	c.expectProblem(http.StatusBadRequest, "code_expired", "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})
}

func TestSignInCodeAttemptLimit(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c := h.client()
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	code := h.mail.code(t, email)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 1; i <= 5; i++ {
		want := "invalid_code"
		if i == 5 {
			want = "code_attempts_exceeded"
		}
		c.expectProblem(http.StatusBadRequest, want, "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": wrong})
	}
	c.expectProblem(http.StatusBadRequest, "code_attempts_exceeded", "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})

	h.clock.Advance(time.Second)
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	c.expect(http.StatusOK, "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": h.mail.code(t, email)})
}

func TestSignInCodeRateLimitAndUnknownAddress(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c := h.client()
	for range 5 {
		c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	}
	c.expectProblem(http.StatusTooManyRequests, "rate_limited", "POST", "/v1/auth/code", map[string]any{"email": email})

	stranger := unique("stranger") + "@example.com"
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": stranger})
	if n := len(h.mail.to(stranger)); n != 0 {
		t.Fatalf("%d mails sent to an unknown address", n)
	}
	c.expectProblem(http.StatusBadRequest, "invalid_code", "POST", "/v1/auth/code/verify", map[string]any{"email": stranger, "code": "123456"})
}

func TestSignInDailyFailureCap(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c := h.client()
	for i := range 4 {
		c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
		code := h.mail.code(t, email)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}
		for range 5 {
			r := c.do("POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": wrong})
			if r.status != http.StatusBadRequest {
				t.Fatalf("round %d: %d %s", i, r.status, r.raw)
			}
		}
		h.clock.Advance(4 * time.Minute)
	}
	notices := h.mail.wait(t, email, 5)
	if last := notices[len(notices)-1]; !strings.Contains(last.Subject, "paused") {
		t.Fatalf("no notice after the cap: %q", last.Subject)
	}
	c.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": email})
	code := h.mail.code(t, email)
	c.expectProblem(http.StatusTooManyRequests, "sign_in_paused", "POST", "/v1/auth/code/verify", map[string]any{"email": email, "code": code})
	if n := len(h.mail.to(email)); n != 6 {
		t.Fatalf("%d mails, want 4 codes, 1 notice and 1 code", n)
	}
	h.clock.Advance(24 * time.Hour)
	c.signIn(email)
}

func TestSignInCodeTakesTheSameTime(t *testing.T) {
	const delay = 300 * time.Millisecond
	h := newHarnessWith(t, func(d *api.Deps) { d.Auth.CodeReplyDelay = delay })
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	for _, addr := range []string{email, unique("stranger") + "@example.com"} {
		start := time.Now()
		h.client().expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": addr})
		if took := time.Since(start); took < delay || took > delay+250*time.Millisecond {
			t.Fatalf("%s took %v", addr, took)
		}
	}
	h.mail.code(t, email)
}

func TestCrossWorkspaceRefused(t *testing.T) {
	h := newHarness(t)
	emailA := unique("owner-a") + "@example.com"
	emailB := unique("owner-b") + "@example.com"
	a := h.bootstrap(emailA, unique("ws-a"))
	b := h.bootstrap(emailB, unique("ws-b"))

	ownerA := h.client()
	ownerA.signIn(emailA)
	created := ownerA.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"})
	secret := created.str("secret")

	keyA := h.client()
	keyA.bearer = secret
	keyA.expect(http.StatusOK, "GET", "/v1/members/"+a.MemberID.String(), nil)
	keyA.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/members/"+b.MemberID.String(), nil)
	keyA.workspace = b.WorkspaceID.String()
	keyA.expectProblem(http.StatusForbidden, "workspace_mismatch", "GET", "/v1/members", nil)

	ownerA.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/members/"+b.MemberID.String(), map[string]any{"role": "agent"})
	ownerA.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/members/"+b.MemberID.String(), nil)
	ownerA.workspace = b.WorkspaceID.String()
	ownerA.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/members", nil)
	ownerA.expectProblem(http.StatusForbidden, "not_a_member", "DELETE", "/v1/members/"+b.MemberID.String(), nil)

	ownerB := h.client()
	ownerB.signIn(emailB)
	members := ownerB.expect(http.StatusOK, "GET", "/v1/members", nil)
	if items := members.body["items"].([]any); len(items) != 1 {
		t.Fatalf("workspace B members: %s", members.raw)
	}
}

func TestLastOwnerRule(t *testing.T) {
	h := newHarness(t)
	ownerEmail := unique("owner") + "@example.com"
	ws := h.bootstrap(ownerEmail, unique("ws"))
	owner := h.client()
	owner.signIn(ownerEmail)
	self := "/v1/members/" + ws.MemberID.String()

	owner.expectProblem(http.StatusConflict, "last_owner", "PATCH", self, map[string]any{"role": "admin"})
	owner.expectProblem(http.StatusConflict, "last_owner", "DELETE", self, nil)

	adminEmail := unique("admin") + "@example.com"
	owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": adminEmail, "role": "admin"})
	admin := h.client()
	admin.signIn(adminEmail)
	admin.expectProblem(http.StatusForbidden, "forbidden", "PATCH", self, map[string]any{"role": "agent"})
	admin.expectProblem(http.StatusForbidden, "forbidden", "DELETE", self, nil)
	admin.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/invites", map[string]any{"email": unique("x") + "@example.com", "role": "owner"})

	me := admin.expect(http.StatusOK, "GET", "/v1/me", nil)
	adminMember := me.body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	owner.expect(http.StatusOK, "PATCH", "/v1/members/"+adminMember, map[string]any{"role": "owner"})
	owner.expect(http.StatusOK, "PATCH", self, map[string]any{"role": "agent"})
	admin.expectProblem(http.StatusConflict, "last_owner", "DELETE", "/v1/members/"+adminMember, nil)
	admin.expect(http.StatusNoContent, "DELETE", self, nil)
	owner.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/members", nil)
}

func TestInvitesAndAPIKeys(t *testing.T) {
	h := newHarness(t)
	ownerEmail := unique("owner") + "@example.com"
	h.bootstrap(ownerEmail, unique("ws"))
	owner := h.client()
	owner.signIn(ownerEmail)

	agentEmail := unique("agent") + "@example.com"
	owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent", "locale": "tr"})
	if msgs := h.mail.to(agentEmail); len(msgs) != 1 || !bytes.Contains([]byte(msgs[0].Subject), []byte("davet")) {
		t.Fatalf("invite mail: %+v", msgs)
	}
	invites := owner.expect(http.StatusOK, "GET", "/v1/invites", nil)
	if items := invites.body["items"].([]any); len(items) != 1 {
		t.Fatalf("invites: %s", invites.raw)
	}
	agent := h.client()
	agent.signIn(agentEmail)
	if msgs := h.mail.to(agentEmail); !bytes.Contains([]byte(msgs[len(msgs)-1].Subject), []byte("giriş kodu")) {
		t.Fatalf("sign-in mail not in Turkish: %q", msgs[len(msgs)-1].Subject)
	}
	owner.expectProblem(http.StatusConflict, "already_member", "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})
	members := owner.expect(http.StatusOK, "GET", "/v1/members", nil)
	if items := members.body["items"].([]any); len(items) != 2 {
		t.Fatalf("members: %s", members.raw)
	}
	agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/api-keys", map[string]any{"name": "nope"})

	created := owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"})
	key := created.body["api_key"].(map[string]any)
	keyClient := h.client()
	keyClient.bearer = created.str("secret")
	keyClient.expect(http.StatusOK, "GET", "/v1/workspace", nil)
	keyClient.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me", nil)
	keyClient.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/api-keys", map[string]any{"name": "escalate"})

	list := owner.expect(http.StatusOK, "GET", "/v1/api-keys", nil)
	listed := list.body["items"].([]any)[0].(map[string]any)
	if listed["last_used_at"] == nil || listed["prefix"] != key["prefix"] {
		t.Fatalf("api key listing: %s", list.raw)
	}
	if bytes.Contains(list.raw, []byte(created.str("secret"))) {
		t.Fatal("listing contains the secret")
	}

	owner.expect(http.StatusNoContent, "DELETE", "/v1/api-keys/"+key["id"].(string), nil)
	keyClient.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/workspace", nil)
}

func TestCookieWritesNeedPanelOrigin(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	owner := h.client()
	owner.signIn(email)
	send := func(c *client, headers map[string]string) int {
		t.Helper()
		req, _ := http.NewRequest("POST", h.url+"/v1/contacts", strings.NewReader(fmt.Sprintf(`{"name":%q}`, unique("contact"))))
		req.Header.Set("X-Forwarded-For", c.ip)
		if c.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+c.bearer)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, err := (&http.Client{Jar: c.http.Jar}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	for name, tc := range map[string]struct {
		headers map[string]string
		want    int
	}{
		"sibling origin":        {map[string]string{"Origin": "https://www.sibling.example", "Content-Type": "text/plain"}, http.StatusForbidden},
		"sibling json":          {map[string]string{"Origin": "https://www.sibling.example", "Content-Type": "application/json"}, http.StatusForbidden},
		"no origin":             {map[string]string{"Content-Type": "application/json"}, http.StatusForbidden},
		"cross-site fetch":      {map[string]string{"Sec-Fetch-Site": "same-site", "Content-Type": "application/json"}, http.StatusForbidden},
		"panel origin, text":    {map[string]string{"Origin": testOrigin, "Content-Type": "text/plain"}, http.StatusUnsupportedMediaType},
		"panel origin, no type": {map[string]string{"Origin": testOrigin}, http.StatusUnsupportedMediaType},
		"panel origin":          {map[string]string{"Origin": testOrigin, "Content-Type": "application/json"}, http.StatusCreated},
		"same-origin fetch":     {map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "application/json; charset=utf-8"}, http.StatusCreated},
	} {
		if got := send(owner, tc.headers); got != tc.want {
			t.Errorf("%s: %d, want %d", name, got, tc.want)
		}
	}
	key := h.client()
	key.bearer = owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "backend"}).str("secret")
	if got := send(key, map[string]string{"Content-Type": "text/plain"}); got != http.StatusCreated {
		t.Fatalf("API key request refused: %d", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHarness(t)
	res, err := http.Get(h.url + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	csp := res.Header.Get("Content-Security-Policy")
	for _, want := range []string{"frame-ancestors 'none'", "object-src 'none'", "script-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("panel CSP %q lacks %q", csp, want)
		}
	}
	if res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Referrer-Policy") != "same-origin" {
		t.Errorf("panel headers: %v", res.Header)
	}
	if r := h.client().do("GET", "/v1/me", nil); r.header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("API headers: %v", r.header)
	}
}

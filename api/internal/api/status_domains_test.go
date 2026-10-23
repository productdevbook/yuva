package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/cfsaas/cfsaastest"
)

const testStatusCNAME = "pages.yuva.example.net"

type fakeDNS struct {
	mu    sync.Mutex
	hook  func()
	cname map[string]string
	txt   map[string][]string
	hosts map[string][]string
	fail  bool
}

func newFakeDNS() *fakeDNS {
	return &fakeDNS{cname: map[string]string{}, txt: map[string][]string{}, hosts: map[string][]string{testStatusCNAME: {"192.0.2.10"}}}
}

func notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}

func (d *fakeDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	d.mu.Lock()
	hook := d.hook
	d.hook = nil
	d.mu.Unlock()
	if hook != nil {
		hook()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return "", &net.DNSError{Err: "server misbehaving", Name: host, IsTemporary: true}
	}
	if c, ok := d.cname[host]; ok {
		return c + ".", nil
	}
	if _, ok := d.hosts[host]; ok {
		return host + ".", nil
	}
	return "", notFound(host)
}

func (d *fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail {
		return nil, &net.DNSError{Err: "server misbehaving", Name: name, IsTemporary: true}
	}
	if t, ok := d.txt[name]; ok {
		return t, nil
	}
	return nil, notFound(name)
}

func (d *fakeDNS) LookupHost(_ context.Context, host string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.cname[host]; ok {
		host = c
	}
	if a, ok := d.hosts[host]; ok {
		return a, nil
	}
	return nil, notFound(host)
}

func (d *fakeDNS) set(domain, cname string, txt ...string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if cname == "" {
		delete(d.cname, domain)
	} else {
		d.cname[domain] = cname
	}
	if len(txt) == 0 {
		delete(d.txt, "_yuva-status."+domain)
	} else {
		d.txt["_yuva-status."+domain] = txt
	}
}

func domainHarness(t *testing.T, dns *fakeDNS, tune func(*api.StatusSettings)) *harness {
	return newHarnessWith(t, func(d *api.Deps) {
		d.Status = api.StatusSettings{CNAME: testStatusCNAME, Resolver: dns}
		if tune != nil {
			tune(&d.Status)
		}
	})
}

func onHost(t *testing.T, h *harness, host, path string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("GET", h.url+path, nil)
	req.Host = host
	res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

func customDomain(t *testing.T, r response) map[string]any {
	t.Helper()
	d, ok := r.body["custom_domain"].(map[string]any)
	if !ok {
		t.Fatalf("no custom_domain in %s", r.raw)
	}
	return d
}

func (st statusTeam) domain(t *testing.T) map[string]any {
	t.Helper()
	return customDomain(t, st.owner.expect(http.StatusOK, "GET", st.path(""), nil))
}

func (st statusTeam) check(t *testing.T, h *harness) map[string]any {
	t.Helper()
	if err := h.srv.RunStatusDomainCheck(context.Background(), uuid.MustParse(st.ws), uuid.MustParse(st.page)); err != nil {
		t.Fatal(err)
	}
	return st.domain(t)
}

func TestStatusDomainValidation(t *testing.T) {
	h := domainHarness(t, newFakeDNS(), func(s *api.StatusSettings) { s.URL = "https://status.yuva.example.net"; s.DomainLimit = 1 })
	st := newStatusTeam(t, h)
	for _, bad := range []string{"localhost", "192.0.2.1", "exa mple.com", "-x.example.com", "x..example.com", "example.123",
		"status.yuva.example.net", "a.status.yuva.example.net", "yuva.example.net", "example.net", testStatusCNAME, "localhost:8080",
		strings.Repeat("a", 64) + ".example.com"} {
		st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", st.path(""), map[string]any{"custom_domain": bad})
	}
	st.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", st.path(""), map[string]any{"custom_domain": "status.example.com"})

	name := unique("d") + ".example.org"
	r := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": " " + strings.ToUpper(name) + ". "})
	d := customDomain(t, r)
	if d["domain"] != name || d["state"] != "pending" || d["cname_target"] != testStatusCNAME || d["txt_name"] != "_yuva-status."+name ||
		!strings.HasPrefix(d["txt_value"].(string), "yuva-verify-") || d["certificates"] != "ask" {
		t.Fatalf("custom domain %v", d)
	}
	if n := len(h.jobs("status_domain_check", st.ws)); n != 1 {
		t.Fatalf("%d check jobs queued, want 1", n)
	}

	other := newStatusTeam(t, h)
	other.owner.expect(http.StatusOK, "PATCH", other.path(""), map[string]any{"custom_domain": name})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/status-pages/"+st.page, map[string]any{"custom_domain": unique("x") + ".example.org"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/status-pages/"+st.page, nil)

	second := st.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Second", "slug": unique("in")}).body["inbox"].(map[string]any)["id"].(string)
	pg2 := st.owner.expect(http.StatusCreated, "POST", "/v1/status-pages", map[string]any{"inbox_id": second, "name": "Two", "slug": unique("st")}).str("id")
	st.owner.expectProblem(http.StatusForbidden, "status_domain_limit", "PATCH", "/v1/status-pages/"+pg2, map[string]any{"custom_domain": unique("y") + ".example.org"})

	if _, ok := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": nil}).body["custom_domain"]; ok {
		t.Fatal("custom_domain still set after null")
	}
	st.owner.expect(http.StatusOK, "PATCH", "/v1/status-pages/"+pg2, map[string]any{"custom_domain": unique("y") + ".example.org"})
	other.owner.expect(http.StatusOK, "PATCH", other.path(""), map[string]any{"custom_domain": name})
}

func TestStatusDomainOff(t *testing.T) {
	h := domainHarness(t, newFakeDNS(), func(s *api.StatusSettings) { s.Domains = "off" })
	st := newStatusTeam(t, h)
	st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", st.path(""), map[string]any{"custom_domain": unique("d") + ".example.org"})
	if r := h.client().do("GET", "/status-domains/check?domain=status.example.org", nil); r.status != http.StatusNotFound {
		t.Fatalf("check with domains off: %d", r.status)
	}
}

func TestStatusDomainLifecycle(t *testing.T) {
	dns := newFakeDNS()
	h := domainHarness(t, dns, nil)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	name := unique("d") + ".example.org"
	token := customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string)
	ask := func() int { return h.client().do("GET", "/status-domains/check?domain="+name, nil).status }

	if d := st.check(t, h); d["state"] != "pending" || !strings.Contains(d["problem"].(string), "no CNAME") || ask() != http.StatusNotFound {
		t.Fatalf("without records: %v", d)
	}
	if _, body, _ := onHost(t, h, name, "/"); strings.Contains(body, `id="follow"`) {
		t.Fatal("a pending domain serves the page")
	}
	dns.set(name, testStatusCNAME, "something-else")
	if d := st.check(t, h); d["state"] != "failed" || !strings.Contains(d["problem"].(string), "verification value") {
		t.Fatalf("with another TXT value: %v", d)
	}
	dns.set(name, testStatusCNAME, token)
	d := st.check(t, h)
	if d["state"] != "verified" || d["verified_at"] == nil || ask() != http.StatusOK {
		t.Fatalf("with both records: %v", d)
	}
	next, _ := time.Parse(time.RFC3339Nano, d["next_check_at"].(string))
	if want := h.clock.Now().Add(24 * time.Hour); next.Sub(want).Abs() > time.Minute {
		t.Fatalf("next check %v, want a day later", next)
	}
	if r := h.client().do("GET", "/status-domains/check?domain="+strings.ToUpper(name)+".", nil); r.status != http.StatusOK || r.str("domain") != name {
		t.Fatalf("check of the spelled-differently domain: %d %s", r.status, r.raw)
	}

	base := "http://" + name
	if u := st.owner.expect(http.StatusOK, "GET", st.path(""), nil).str("url"); u != base {
		t.Fatalf("page url %s", u)
	}
	inc := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{"title": "API down", "message": "Looking", "publish": true}).str("id")
	for path, want := range map[string]string{
		"/": base + "/history", "/history": base + "/incidents/" + inc, "/incidents/" + inc: "API down", "/feed.atom": base + "/incidents/" + inc,
		"/badge.svg": "<svg",
	} {
		code, body, _ := onHost(t, h, name+":443", path)
		if code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("%s on the domain: %d, want %q in %s", path, code, want, body)
		}
	}
	for _, path := range []string{"/v1/me", "/status/" + st.slug, "/client/v1/status/" + st.slug, "/incidents/a/b", "/login"} {
		if code, _, _ := onHost(t, h, name, path); code != http.StatusNotFound {
			t.Fatalf("%s on the domain: %d, want 404", path, code)
		}
	}
	js := publicStatus(t, h, "/client/v1/status/"+st.slug)
	if js.str("url") != base || js.body["open"].([]any)[0].(map[string]any)["url"] != base+"/incidents/"+inc {
		t.Fatalf("JSON names %s", js.raw)
	}
	if code, body, _ := onHost(t, h, "127.0.0.1", "/status/"+st.slug); code != http.StatusOK || !strings.Contains(body, base+"/history") {
		t.Fatalf("page at the status URL: %d", code)
	}

	dns.set(name, "")
	d = st.check(t, h)
	if d["state"] != "verified" || d["missing_since"] == nil || !strings.Contains(d["problem"].(string), "no TXT record") {
		t.Fatalf("records gone: %v", d)
	}
	h.clock.Advance(24 * time.Hour)
	st.check(t, h)
	dns.set(name, testStatusCNAME, token)
	h.clock.Advance(24 * time.Hour)
	if d = st.check(t, h); d["state"] != "verified" || d["missing_since"] != nil {
		t.Fatalf("records back: %v", d)
	}
	dns.fail = true
	h.clock.Advance(24 * time.Hour)
	if d = st.check(t, h); d["state"] != "verified" || d["missing_since"] != nil || !strings.Contains(d["problem"].(string), "failed") {
		t.Fatalf("a failing resolver changes the state: %v", d)
	}
	dns.fail = false

	dns.set(name, "")
	for i := range 3 {
		h.clock.Advance(24 * time.Hour)
		if d = st.check(t, h); d["state"] != "verified" {
			t.Fatalf("turned %s after %d days", d["state"], i)
		}
	}
	h.clock.Advance(24 * time.Hour)
	if d = st.check(t, h); d["state"] != "off" || d["next_check_at"] != nil || ask() != http.StatusNotFound {
		t.Fatalf("after 3 days: %v", d)
	}
	if _, body, _ := onHost(t, h, name, "/"); strings.Contains(body, `id="follow"`) {
		t.Fatal("an off domain serves the page")
	}
	if u := st.owner.expect(http.StatusOK, "GET", st.path(""), nil).str("url"); strings.Contains(u, name) {
		t.Fatalf("page url %s after off", u)
	}

	dns.set(name, testStatusCNAME, token)
	if d := customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name})); d["state"] != "pending" || d["txt_value"] != token {
		t.Fatalf("set again after off: %v", d)
	}
	if d := st.check(t, h); d["state"] != "verified" {
		t.Fatalf("set again: %v", d)
	}
}

func TestStatusDomainSubscribeOnDomain(t *testing.T) {
	dns := newFakeDNS()
	h := domainHarness(t, dns, nil)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	name := unique("d") + ".example.org"
	token := customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string)
	dns.set(name, testStatusCNAME, token)
	st.check(t, h)
	if code, _, _ := onHost(t, h, name, "/subscribe"); code != http.StatusNotFound {
		t.Fatalf("subscribe without e-mail subscribers: %d", code)
	}
}

func TestStatusDomainCloudflare(t *testing.T) {
	dns := newFakeDNS()
	cf := cfsaastest.New(t, "zone1", "token1")
	h := domainHarness(t, dns, func(s *api.StatusSettings) { s.Domains = "cloudflare"; s.Cloudflare = cf.Client() })
	st := newStatusTeam(t, h)
	name := unique("d") + ".example.org"
	d := customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))
	if d["certificates"] != "cloudflare" || d["cloudflare"].(map[string]any)["id"] != nil {
		t.Fatalf("before verification %v", d)
	}
	if r := h.client().do("GET", "/status-domains/check?domain="+name, nil); r.status != http.StatusNotFound {
		t.Fatalf("ask endpoint in cloudflare mode: %d", r.status)
	}
	st.check(t, h)
	if cf.Has(name) {
		t.Fatal("an unverified domain is a custom hostname")
	}
	dns.set(name, testStatusCNAME, d["txt_value"].(string))
	d = st.check(t, h)
	c := d["cloudflare"].(map[string]any)
	if d["state"] != "verified" || !cf.Has(name) || cf.SSLMethod(name) != "http" || c["status"] != "pending" || c["ssl_status"] != "pending_validation" {
		t.Fatalf("after verification %v", d)
	}
	next, _ := time.Parse(time.RFC3339Nano, d["next_check_at"].(string))
	if next.Sub(h.clock.Now()) > 11*time.Minute {
		t.Fatalf("an inactive hostname is checked again at %v", next)
	}
	cf.Activate(name)
	h.clock.Advance(10 * time.Minute)
	d = st.check(t, h)
	if c = d["cloudflare"].(map[string]any); c["status"] != "active" || c["ssl_status"] != "active" {
		t.Fatalf("after activation %v", d)
	}
	id := c["id"].(string)

	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": nil})
	jobs := h.jobs("status_domain_release", st.ws)
	if len(jobs) != 1 {
		t.Fatalf("%d release jobs", len(jobs))
	}
	var args api.StatusDomainReleaseArgs
	if err := json.Unmarshal(jobs[0].args, &args); err != nil || args.HostnameID != id {
		t.Fatalf("release args %s %v", jobs[0].args, err)
	}
	if err := h.srv.ReleaseStatusDomain(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if cf.Has(name) {
		t.Fatal("a removed domain is still a custom hostname")
	}

	dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
	st.check(t, h)
	if !cf.Has(name) {
		t.Fatal("not created again")
	}
	st.owner.expect(http.StatusNoContent, "DELETE", st.path(""), nil)
	if n := len(h.jobs("status_domain_release", st.ws)); n != 2 {
		t.Fatalf("%d release jobs after deleting the page", n)
	}
}

func TestStatusDomainClaims(t *testing.T) {
	dns := newFakeDNS()
	h := domainHarness(t, dns, nil)
	a, b := newStatusTeam(t, h), newStatusTeam(t, h)
	name := unique("d") + ".example.org"
	ta := customDomain(t, a.owner.expect(http.StatusOK, "PATCH", a.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string)
	tb := customDomain(t, b.owner.expect(http.StatusOK, "PATCH", b.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string)
	dns.set(name, testStatusCNAME, ta, tb)
	if d := a.check(t, h); d["state"] != "verified" {
		t.Fatalf("first claim: %v", d)
	}
	if d := b.domain(t); d["state"] != "failed" || d["problem"] != "taken" || d["next_check_at"] != nil {
		t.Fatalf("other claim after the first verified: %v", d)
	}
	if d := b.check(t, h); d["state"] != "failed" || d["problem"] != "taken" {
		t.Fatalf("a taken claim is checked again: %v", d)
	}
	b.owner.expectProblem(http.StatusConflict, "domain_taken", "PATCH", b.path(""), map[string]any{"custom_domain": name})
	third := newStatusTeam(t, h)
	third.owner.expectProblem(http.StatusConflict, "domain_taken", "PATCH", third.path(""), map[string]any{"custom_domain": name})

	dns.set(name, "")
	a.check(t, h)
	h.clock.Advance(72 * time.Hour)
	if d := a.check(t, h); d["state"] != "off" {
		t.Fatalf("records gone 3 days: %v", d)
	}
	d := customDomain(t, b.owner.expect(http.StatusOK, "PATCH", b.path(""), map[string]any{"custom_domain": name}))
	if d["state"] != "pending" || d["txt_value"] != tb || d["next_check_at"] == nil || d["problem"] != nil {
		t.Fatalf("claim sent again after the holder turned off: %v", d)
	}
	dns.set(name, testStatusCNAME, tb)
	if d := b.check(t, h); d["state"] != "verified" {
		t.Fatalf("second claim: %v", d)
	}
	a.owner.expectProblem(http.StatusConflict, "domain_taken", "PATCH", a.path(""), map[string]any{"custom_domain": name})

	late := newStatusTeam(t, h)
	other := unique("e") + ".example.org"
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE status_pages SET custom_domain = $3, domain_token = 'yuva-verify-late',
		domain_state = 'pending', domain_claimed_at = now(), domain_check_at = now() WHERE workspace_id = $1 AND id = $2`, late.ws, late.page, name); err != nil {
		t.Fatal(err)
	}
	dns.set(name, testStatusCNAME, tb, "yuva-verify-late")
	if d := late.check(t, h); d["state"] != "failed" || d["problem"] != "taken" || d["next_check_at"] != nil {
		t.Fatalf("a claim whose records appear after another page verified: %v", d)
	}
	if d := b.check(t, h); d["state"] != "verified" {
		t.Fatalf("holder after a late claim: %v", d)
	}

	late.owner.expect(http.StatusOK, "PATCH", late.path(""), map[string]any{"custom_domain": other})
	h.clock.Advance(6 * 24 * time.Hour)
	if d := late.check(t, h); d["state"] != "pending" {
		t.Fatalf("6 days: %v", d)
	}
	h.clock.Advance(24 * time.Hour)
	if d := late.check(t, h); d["state"] != "failed" || d["problem"] != "expired" || d["next_check_at"] != nil {
		t.Fatalf("7 days: %v", d)
	}
	if d := customDomain(t, late.owner.expect(http.StatusOK, "PATCH", late.path(""), map[string]any{"custom_domain": other})); d["state"] != "pending" || d["next_check_at"] == nil {
		t.Fatalf("expired claim sent again: %v", d)
	}
	h.clock.Advance(24 * time.Hour)
	if d := late.check(t, h); d["state"] != "pending" {
		t.Fatalf("a day after sending again: %v", d)
	}
}

func TestStatusDomainCloudflareAfterCommit(t *testing.T) {
	dns := newFakeDNS()
	cf := cfsaastest.New(t, "zone1", "token1")
	h := domainHarness(t, dns, func(s *api.StatusSettings) { s.Domains = "cloudflare"; s.Cloudflare = cf.Client() })
	ctx := context.Background()
	verify := func(st statusTeam, name string) string {
		t.Helper()
		dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
		d := st.check(t, h)
		if d["state"] != "verified" || !cf.Has(name) {
			t.Fatalf("verify %s: %v", name, d)
		}
		return d["cloudflare"].(map[string]any)["id"].(string)
	}

	st := newStatusTeam(t, h)
	name := unique("d") + ".example.org"
	verify(st, name)
	h.clock.Advance(24 * time.Hour)
	dns.hook = func() {
		st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": nil})
		jobs := h.jobs("status_domain_release", st.ws)
		var args api.StatusDomainReleaseArgs
		if err := json.Unmarshal(jobs[len(jobs)-1].args, &args); err != nil {
			t.Error(err)
		}
		if err := h.srv.ReleaseStatusDomain(ctx, args); err != nil {
			t.Error(err)
		}
	}
	if err := h.srv.RunStatusDomainCheck(ctx, uuid.MustParse(st.ws), uuid.MustParse(st.page)); err != nil {
		t.Fatal(err)
	}
	if cf.Has(name) {
		t.Fatal("a domain removed during its check got a custom hostname")
	}

	st = newStatusTeam(t, h)
	name = unique("d") + ".example.org"
	id := verify(st, name)
	dns.set(name, "")
	h.clock.Advance(24 * time.Hour)
	st.check(t, h)
	h.clock.Advance(72 * time.Hour)
	if err := cf.Client().Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if d := st.check(t, h); d["state"] != "off" || cf.Has(name) {
		t.Fatalf("a domain turned off got a custom hostname: %v %v", d, cf.Has(name))
	}
}

func TestStatusDomainCacheByPage(t *testing.T) {
	dns := newFakeDNS()
	h := domainHarness(t, dns, nil)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	name := unique("d") + ".example.org"
	dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
	if code, _, _ := onHost(t, h, name, "/"); code != http.StatusOK {
		t.Fatalf("pending domain: %d", code)
	}
	st.check(t, h)
	served := func() bool {
		code, body, _ := onHost(t, h, name, "/")
		return code == http.StatusOK && strings.Contains(body, `id="follow"`)
	}
	if !served() {
		t.Fatal("verified domain right after its check")
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"slug": unique("st")})
	if code, _, hdr := onHost(t, h, name, "/history"); code != http.StatusOK {
		t.Fatalf("after a slug rename: %d %s", code, hdr.Get("Location"))
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": false})
	if served() {
		t.Fatal("served after unpublishing")
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	if !served() {
		t.Fatal("not served after publishing again")
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": nil})
	if served() {
		t.Fatal("served after the domain was removed")
	}
	for i := range 50 {
		onHost(t, h, unique("x")+".example.com", "/")
		onHost(t, h, "not a host "+string(rune('a'+i%26)), "/")
	}
}

func TestStatusDomainURLWaitsForCloudflare(t *testing.T) {
	dns := newFakeDNS()
	cf := cfsaastest.New(t, "zone1", "token1")
	h := domainHarness(t, dns, func(s *api.StatusSettings) { s.Domains = "cloudflare"; s.Cloudflare = cf.Client() })
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	name := unique("d") + ".example.org"
	dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
	if d := st.check(t, h); d["state"] != "verified" {
		t.Fatalf("not verified: %v", d)
	}
	if u := st.owner.expect(http.StatusOK, "GET", st.path(""), nil).str("url"); strings.Contains(u, name) {
		t.Fatalf("url %s before the certificate is active", u)
	}
	if u := publicStatus(t, h, "/client/v1/status/"+st.slug).str("url"); strings.Contains(u, name) {
		t.Fatalf("JSON url %s before the certificate is active", u)
	}
	cf.Activate(name)
	h.clock.Advance(10 * time.Minute)
	st.check(t, h)
	if u := st.owner.expect(http.StatusOK, "GET", st.path(""), nil).str("url"); u != "http://"+name {
		t.Fatalf("url %s once the certificate is active", u)
	}
}

func TestStatusDomainAssetsSameOrigin(t *testing.T) {
	dns := newFakeDNS()
	cf := cfsaastest.New(t, "zone1", "token1")
	h := domainHarness(t, dns, func(s *api.StatusSettings) { s.Domains = "cloudflare"; s.Cloudflare = cf.Client() })
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	name := unique("d") + ".example.org"
	dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
	if d := st.check(t, h); d["state"] != "verified" {
		t.Fatalf("not verified: %v", d)
	}
	fonts := regexp.MustCompile(`url\("([^"]+\.woff2)"\)`)
	_, def, _ := onHost(t, h, "127.0.0.1", "/status/"+st.slug)

	code, body, hdr := onHost(t, h, name, "/")
	if code != http.StatusOK {
		t.Fatalf("page on the domain: %d", code)
	}
	if csp := hdr.Get("Content-Security-Policy"); !strings.Contains(csp, "font-src 'self'") || strings.Contains(csp, "font-src 'self' ") {
		t.Fatalf("CSP %q", csp)
	}
	found := fonts.FindAllStringSubmatch(body, -1)
	if len(found) != 2 {
		t.Fatalf("font URLs %v in %s", found, body)
	}
	for _, m := range found {
		u, err := url.Parse(m[1])
		if err != nil || (u.Host != "" && u.Host != name) || !strings.HasPrefix(u.Path, "/fonts/") {
			t.Fatalf("font URL %s on %s", m[1], name)
		}
		code, _, fh := onHost(t, h, name, u.Path)
		if code != http.StatusOK || fh.Get("Content-Type") != "font/woff2" {
			t.Fatalf("font %s on the domain: %d %s", u.Path, code, fh.Get("Content-Type"))
		}
	}
	if !strings.Contains(body, `href="http://`+name+`/feed.atom"`) || !strings.Contains(body, `src="http://`+name+`/badge.svg"`) {
		t.Fatalf("feed and badge not on %s: %s", name, body)
	}
	if strings.Count(body, "/status/"+st.slug) != strings.Count(body, "/client/v1/status/"+st.slug) {
		t.Fatalf("page on the domain links the status URL: %s", body)
	}

	for _, m := range fonts.FindAllStringSubmatch(def, -1) {
		if !strings.Contains(m[1], "/status/"+st.slug+"/fonts/") {
			t.Fatalf("font URL %s at the status URL", m[1])
		}
	}
}

func TestStatusDomainAsk(t *testing.T) {
	dns := newFakeDNS()
	h := domainHarness(t, dns, nil)
	st := newStatusTeam(t, h)
	name := unique("d") + ".example.org"
	dns.set(name, testStatusCNAME, customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))["txt_value"].(string))
	st.check(t, h)
	c := h.client()
	ask := func(domain string) int { return c.do("GET", "/status-domains/check?domain="+domain, nil).status }
	if code := ask(name); code != http.StatusNotFound {
		t.Fatalf("unpublished page: %d", code)
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	if code := ask(name); code != http.StatusOK {
		t.Fatalf("published page: %d", code)
	}
	for _, bad := range []string{"", "localhost", "192.0.2.1", "a..b.example"} {
		if code := ask(bad); code != http.StatusNotFound {
			t.Fatalf("%q: %d", bad, code)
		}
	}
	proxy := func(domain string) int {
		res, err := http.Get(h.url + "/status-domains/check?domain=" + domain)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	for i := range 300 {
		if code := proxy(unique("random") + ".example.net"); code != http.StatusNotFound {
			t.Fatalf("the proxy's ask for random name %d: %d", i+1, code)
		}
	}
	limited := false
	for range 130 {
		if code := ask(name); code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("no per-address limit for a caller other than the proxy")
	}
	for range 50 {
		if code := proxy(name); code != http.StatusOK {
			t.Fatalf("the proxy's ask after others were limited: %d", code)
		}
	}
	other := h.client()
	if code := other.do("GET", "/status-domains/check?domain="+name, nil).status; code != http.StatusOK {
		t.Fatalf("another address: %d", code)
	}
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE workspaces SET deleted_at = now() WHERE id = $1", st.ws); err != nil {
		t.Fatal(err)
	}
	h.srv.ForgetStatusDomains()
	if code := proxy(name); code != http.StatusNotFound {
		t.Fatalf("deleted workspace: %d", code)
	}
}

func TestStatusDomainOffRemoves(t *testing.T) {
	cf := cfsaastest.New(t, "zone1", "token1")
	h := domainHarness(t, newFakeDNS(), func(s *api.StatusSettings) { s.Domains = "off"; s.Cloudflare = cf.Client() })
	ctx := context.Background()
	st := newStatusTeam(t, h)
	name := unique("d") + ".example.org"
	host, err := cf.Client().Create(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx, `UPDATE status_pages SET custom_domain = $3, domain_token = 'yuva-verify-x', domain_state = 'verified',
		domain_claimed_at = now(), cf_hostname_id = $4 WHERE workspace_id = $1 AND id = $2`, st.ws, st.page, name, host.ID); err != nil {
		t.Fatal(err)
	}
	if r := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name, "name": "Renamed"}); r.body["custom_domain"] != nil {
		t.Fatalf("custom_domain shown with domains off: %s", r.raw)
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": strings.ToUpper(name) + "."})
	st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", st.path(""), map[string]any{"custom_domain": unique("o") + ".example.org"})
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": ""})
	jobs := h.jobs("status_domain_release", st.ws)
	if len(jobs) != 1 {
		t.Fatalf("%d release jobs", len(jobs))
	}
	var args api.StatusDomainReleaseArgs
	if err := json.Unmarshal(jobs[0].args, &args); err != nil || args.HostnameID != host.ID {
		t.Fatalf("release args %s %v", jobs[0].args, err)
	}
	if err := h.srv.ReleaseStatusDomain(ctx, args); err != nil {
		t.Fatal(err)
	}
	if cf.Has(name) {
		t.Fatal("hostname kept after removal with domains off")
	}
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": nil})
}

func TestStatusDomainLimitConcurrent(t *testing.T) {
	h := domainHarness(t, newFakeDNS(), func(s *api.StatusSettings) { s.DomainLimit = 1 })
	st := newStatusTeam(t, h)
	pages := []string{st.page}
	for range 5 {
		inbox := st.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "More", "slug": unique("in")}).body["inbox"].(map[string]any)["id"].(string)
		pages = append(pages, st.owner.expect(http.StatusCreated, "POST", "/v1/status-pages", map[string]any{"inbox_id": inbox, "name": "More", "slug": unique("st")}).str("id"))
	}
	codes := make([]int, len(pages))
	var wg sync.WaitGroup
	for i, pg := range pages {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = st.owner.do("PATCH", "/v1/status-pages/"+pg, map[string]any{"custom_domain": unique("c") + ".example.org"}).status
		}()
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusForbidden:
		default:
			t.Fatalf("codes %v", codes)
		}
	}
	if ok != 1 {
		t.Fatalf("%d pages got a domain with a limit of 1: %v", ok, codes)
	}
}

func TestStatusPageUpdatedEvent(t *testing.T) {
	cf := cfsaastest.New(t, "zone1", "token1")
	dns := newFakeDNS()
	h := domainHarness(t, dns, func(s *api.StatusSettings) { s.Domains = "cloudflare"; s.Cloudflare = cf.Client() })
	st := newStatusTeam(t, h)
	other := newStatusTeam(t, h)
	agent := st.agent.dial("")
	agent.ready()
	foreign := other.owner.dial("")
	foreign.ready()

	name := unique("d") + ".example.org"
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"name": "Acme 2"})
	st.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+st.inbox+"/members/"+st.agentID, nil)
	for _, m := range agent.until(func(m wsMessage) bool { return m.Type == "inbox_access.changed" }) {
		if m.Type == "status.page.updated" {
			t.Fatalf("member without inbox access got %+v", m)
		}
	}
	d := customDomain(t, st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"custom_domain": name}))
	if m := agent.nextOf("status.page.updated"); !strings.Contains(string(m.Data), st.page) || m.InboxID != st.inbox {
		t.Fatalf("patch event %+v", m)
	}
	dns.set(name, testStatusCNAME, d["txt_value"].(string))
	st.check(t, h)
	if m := agent.nextOf("status.page.updated"); !strings.Contains(string(m.Data), st.page) {
		t.Fatalf("verified event %+v", m)
	}
	cf.Activate(name)
	h.clock.Advance(10 * time.Minute)
	st.check(t, h)
	if m := agent.nextOf("status.page.updated"); !strings.Contains(string(m.Data), st.page) {
		t.Fatalf("cloudflare event %+v", m)
	}

	other.owner.expect(http.StatusOK, "PATCH", other.path(""), map[string]any{"name": "Other"})
	if m := foreign.nextOf("status.page.updated"); !strings.Contains(string(m.Data), other.page) {
		t.Fatalf("foreign workspace got %+v", m)
	}
}

func TestStatusOwnDomainsField(t *testing.T) {
	for mode, want := range map[string]string{"": "ask", "ask": "ask", "cloudflare": "cloudflare", "off": ""} {
		h := domainHarness(t, newFakeDNS(), func(s *api.StatusSettings) { s.Domains = mode })
		st := newStatusTeam(t, h)
		own, ok := st.owner.expect(http.StatusOK, "GET", st.path(""), nil).body["own_domains"].(map[string]any)
		if want == "" && ok || want != "" && (!ok || own["mode"] != want) {
			t.Fatalf("mode %q: own_domains %v", mode, own)
		}
		list := st.owner.expect(http.StatusOK, "GET", "/v1/status-pages", nil).body["items"].([]any)[0].(map[string]any)
		if _, ok := list["own_domains"]; ok != (want != "") {
			t.Fatalf("mode %q: list own_domains %v", mode, list)
		}
	}
}

package api_test

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/xml"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/beevik/etree"
	"github.com/crewjam/saml"
	dsig "github.com/russellhaering/goxmldsig"

	"github.com/productdevbook/yuva/api/internal/api"
)

const (
	samlNSA = "urn:oasis:names:tc:SAML:2.0:assertion"
	samlNSP = "urn:oasis:names:tc:SAML:2.0:protocol"
)

type samlKey struct {
	key  *rsa.PrivateKey
	cert []byte
}

func newSAMLKey(t *testing.T) samlKey {
	return newSAMLKeyValid(t, time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
}

func newSAMLKeyValid(t *testing.T, notBefore, notAfter time.Time) samlKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "Fake IdP"},
		NotBefore: notBefore, NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return samlKey{key: key, cert: der}
}

// fakeSAMLIdP serves its metadata and signs responses with its own key, the way an identity
// provider would after the person signed in.
type fakeSAMLIdP struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	key     samlKey
	expired []samlKey
	down    bool
	fetches int
}

func newFakeSAMLIdP(t *testing.T) *fakeSAMLIdP {
	idp := &fakeSAMLIdP{t: t, key: newSAMLKey(t)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metadata", func(w http.ResponseWriter, _ *http.Request) {
		idp.mu.Lock()
		idp.fetches++
		down := idp.down
		idp.mu.Unlock()
		if down {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		_, _ = io.WriteString(w, idp.metadata())
	})
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (idp *fakeSAMLIdP) entityID() string { return idp.srv.URL + "/entity" }

func (idp *fakeSAMLIdP) metadata() string {
	idp.mu.Lock()
	keys := []samlKey{idp.key}
	if idp.key.cert == nil {
		keys = nil
	}
	keys = append(keys, idp.expired...)
	idp.mu.Unlock()
	var descriptors strings.Builder
	for _, k := range keys {
		descriptors.WriteString(`<md:KeyDescriptor use="signing"><ds:KeyInfo><ds:X509Data><ds:X509Certificate>` +
			base64.StdEncoding.EncodeToString(k.cert) + `</ds:X509Certificate></ds:X509Data></ds:KeyInfo></md:KeyDescriptor>`)
	}
	return `<?xml version="1.0"?>
<md:EntityDescriptor xmlns:md="urn:oasis:names:tc:SAML:2.0:metadata" xmlns:ds="http://www.w3.org/2000/09/xmldsig#" entityID="` + idp.entityID() + `">
  <md:IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    ` + descriptors.String() + `
    <md:NameIDFormat>urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress</md:NameIDFormat>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="` + idp.srv.URL + `/sso-post"/>
    <md:SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="` + idp.srv.URL + `/sso?tenant=1"/>
  </md:IDPSSODescriptor>
</md:EntityDescriptor>`
}

func (idp *fakeSAMLIdP) rotate() {
	k := newSAMLKey(idp.t)
	idp.mu.Lock()
	idp.key = k
	idp.mu.Unlock()
}

type samlAnswer struct {
	inResponseTo, email, audience, recipient, destination, assertionID string
	nameID                                                             string
	notOnOrAfter                                                       time.Time
	signAssertion, signResponse                                        bool
	signer                                                             *samlKey
	attrs                                                              map[string]string
	edit                                                               func(resp, assertion *etree.Element)
}

func (idp *fakeSAMLIdP) sign(el *etree.Element, k samlKey) *etree.Element {
	ctx := dsig.NewDefaultSigningContext(dsig.TLSCertKeyStore(tls.Certificate{Certificate: [][]byte{k.cert}, PrivateKey: k.key}))
	ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	ctx.IdAttribute = "ID"
	signed, err := ctx.SignEnveloped(el)
	if err != nil {
		idp.t.Fatal(err)
	}
	return signed
}

func samlTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func (idp *fakeSAMLIdP) assertion(a samlAnswer, id, email string) *etree.Element {
	now := time.Now()
	el := etree.NewElement("saml:Assertion")
	el.CreateAttr("xmlns:saml", samlNSA)
	el.CreateAttr("ID", id)
	el.CreateAttr("Version", "2.0")
	el.CreateAttr("IssueInstant", samlTime(now))
	el.CreateElement("saml:Issuer").SetText(idp.entityID())
	subj := el.CreateElement("saml:Subject")
	nid := subj.CreateElement("saml:NameID")
	nid.CreateAttr("Format", string(saml.EmailAddressNameIDFormat))
	nid.SetText(email)
	sc := subj.CreateElement("saml:SubjectConfirmation")
	sc.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	scd := sc.CreateElement("saml:SubjectConfirmationData")
	scd.CreateAttr("InResponseTo", a.inResponseTo)
	scd.CreateAttr("Recipient", a.recipient)
	scd.CreateAttr("NotOnOrAfter", samlTime(a.notOnOrAfter))
	cond := el.CreateElement("saml:Conditions")
	cond.CreateAttr("NotBefore", samlTime(now.Add(-time.Minute)))
	cond.CreateAttr("NotOnOrAfter", samlTime(a.notOnOrAfter))
	cond.CreateElement("saml:AudienceRestriction").CreateElement("saml:Audience").SetText(a.audience)
	authn := el.CreateElement("saml:AuthnStatement")
	authn.CreateAttr("AuthnInstant", samlTime(now))
	if len(a.attrs) > 0 {
		st := el.CreateElement("saml:AttributeStatement")
		for k, v := range a.attrs {
			at := st.CreateElement("saml:Attribute")
			at.CreateAttr("Name", k)
			at.CreateElement("saml:AttributeValue").SetText(v)
		}
	}
	return el
}

func (idp *fakeSAMLIdP) answer(a samlAnswer) string {
	idp.mu.Lock()
	k := idp.key
	idp.mu.Unlock()
	if a.signer != nil {
		k = *a.signer
	}
	resp := etree.NewElement("samlp:Response")
	resp.CreateAttr("xmlns:samlp", samlNSP)
	resp.CreateAttr("xmlns:saml", samlNSA)
	resp.CreateAttr("ID", "_r"+strings.ReplaceAll(unique("resp"), "-", ""))
	resp.CreateAttr("Version", "2.0")
	resp.CreateAttr("IssueInstant", samlTime(time.Now()))
	resp.CreateAttr("Destination", a.destination)
	if a.inResponseTo != "" {
		resp.CreateAttr("InResponseTo", a.inResponseTo)
	}
	resp.CreateElement("saml:Issuer").SetText(idp.entityID())
	resp.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", "urn:oasis:names:tc:SAML:2.0:status:Success")
	email := a.email
	if a.nameID != "" {
		email = a.nameID
	}
	as := idp.assertion(a, a.assertionID, email)
	if a.signAssertion {
		as = idp.sign(as, k)
	}
	resp.AddChild(as)
	if a.edit != nil {
		a.edit(resp, as)
	}
	if a.signResponse {
		resp = idp.sign(resp, k)
	}
	doc := etree.NewDocument()
	doc.SetRoot(resp)
	out, err := doc.WriteToString()
	if err != nil {
		idp.t.Fatal(err)
	}
	return out
}

type samlEnv struct {
	*ssoEnv
	saml *fakeSAMLIdP
	acs  string
	aud  string
}

func newSAMLEnv(t *testing.T) *samlEnv {
	e := newSSOEnv(t)
	return &samlEnv{ssoEnv: e, saml: newFakeSAMLIdP(t), acs: testOrigin + "/auth/saml/acs", aud: testOrigin + "/auth/saml/metadata/" + e.ws.WorkspaceID.String()}
}

func (e *samlEnv) setUp() {
	got := e.owner.expect(http.StatusOK, "PUT", "/v1/sso/saml", map[string]any{"metadata_url": e.saml.srv.URL + "/metadata"})
	if got.str("protocol") != "saml" {
		e.h.t.Fatalf("saml settings: %s", got.raw)
	}
	d := e.owner.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	e.dns.set(d.str("txt_name"), d.str("txt_value"))
	e.owner.expect(http.StatusOK, "POST", "/v1/sso/domains/"+d.str("id")+"/verify", nil)
}

// start begins a sign-in in the browser c and returns the AuthnRequest's id.
func (e *samlEnv) start(c *client, email string) string {
	t := e.h.t
	t.Helper()
	start := c.expect(http.StatusOK, "POST", "/v1/auth/sso", map[string]any{"email": email, "return_to": "/conversations"})
	if start.body["sso"] != true {
		t.Fatalf("start: %s", start.raw)
	}
	cookie := start.header.Get("Set-Cookie")
	for _, want := range []string{"yuva_saml=", "Path=/auth/saml/acs", "HttpOnly", "Secure", "SameSite=None", "Max-Age=600"} {
		if !strings.Contains(cookie, want) {
			t.Fatalf("saml cookie %q lacks %s", cookie, want)
		}
	}
	u, err := url.Parse(start.str("authorization_url"))
	if err != nil || !strings.HasPrefix(u.String(), e.saml.srv.URL+"/sso?tenant=1&SAMLRequest=") {
		t.Fatalf("authorization url: %s", start.str("authorization_url"))
	}
	raw, err := base64.StdEncoding.DecodeString(u.Query().Get("SAMLRequest"))
	if err != nil {
		t.Fatal(err)
	}
	inflated, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var req saml.AuthnRequest
	if err := xml.Unmarshal(inflated, &req); err != nil {
		t.Fatal(err)
	}
	if req.AssertionConsumerServiceURL != e.acs || req.Issuer.Value != e.aud || req.ID == "" ||
		req.NameIDPolicy == nil || *req.NameIDPolicy.Format != string(saml.EmailAddressNameIDFormat) {
		t.Fatalf("AuthnRequest: %s", inflated)
	}
	return req.ID
}

func (e *samlEnv) good(id, email string) samlAnswer {
	return samlAnswer{inResponseTo: id, email: email, audience: e.aud, recipient: e.acs, destination: e.acs,
		assertionID: "_a" + strings.ReplaceAll(unique("as"), "-", ""), notOnOrAfter: time.Now().Add(5 * time.Minute), signAssertion: true}
}

func (e *samlEnv) post(c *client, doc string) *http.Response {
	raw := &http.Client{Jar: c.http.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := url.Values{"SAMLResponse": {base64.StdEncoding.EncodeToString([]byte(doc))}, "RelayState": {""}}
	req, _ := http.NewRequest("POST", e.h.url+"/auth/saml/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "null")
	req.Header.Set("X-Forwarded-For", c.ip)
	res, err := raw.Do(req)
	if err != nil {
		e.h.t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func (e *samlEnv) signIn(c *client, email string, tune func(*samlAnswer)) *http.Response {
	id := e.start(c, email)
	a := e.good(id, email)
	if tune != nil {
		tune(&a)
	}
	return e.post(c, e.saml.answer(a))
}

func TestSAMLSignIn(t *testing.T) {
	e := newSAMLEnv(t)
	e.setUp()
	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"jit": true})
	got := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	sp := got.body["saml_sp"].(map[string]any)
	idp := got.body["saml"].(map[string]any)
	if sp["entity_id"] != e.aud || sp["acs_url"] != e.acs || idp["entity_id"] != e.saml.entityID() || idp["sso_url"] != e.saml.srv.URL+"/sso?tenant=1" ||
		len(idp["certificates"].([]any)) != 1 || got.body["oidc"] != nil {
		t.Fatalf("settings: %s", got.raw)
	}

	agent := e.h.client()
	email := "ada@" + e.domain
	wantLocation(t, e.signIn(agent, email, nil), "/conversations")
	me := agent.expect(http.StatusOK, "GET", "/v1/me", nil)
	if ms := me.body["memberships"].([]any); len(ms) != 1 || ms[0].(map[string]any)["role"] != "agent" {
		t.Fatalf("me after saml: %s", me.raw)
	}

	signedResponse := e.h.client()
	wantLocation(t, e.signIn(signedResponse, email, func(a *samlAnswer) { a.signAssertion, a.signResponse = false, true }), "/conversations")

	byAttribute := e.h.client()
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/saml", map[string]any{"email_attribute": "mail"})
	wantLocation(t, e.signIn(byAttribute, email, func(a *samlAnswer) {
		a.nameID, a.attrs = "AdaL", map[string]string{"mail": email}
	}), "/conversations")
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/sign-in?sso_error=email_unverified")
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/saml", map[string]any{"metadata_xml": e.saml.metadata()})

	e.saml.rotate()
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/sign-in?sso_error=invalid_token")
	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/saml", map[string]any{"metadata_url": e.saml.srv.URL + "/metadata"})
	e.saml.rotate()
	e.h.clock.Advance(61 * time.Minute)
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/conversations")

	audit := e.auditActions(t)
	for _, a := range []string{"sso.provider_created", "sso.provider_updated", "sso.signed_in", "member.joined"} {
		if len(audit[a]) == 0 {
			t.Fatalf("audit has no %s: %v", a, audit)
		}
	}
	refresh := audit["sso.provider_updated"][0]
	if d := refresh["details"].(map[string]any); d["via"] != "metadata_refresh" || refresh["actor"].(map[string]any)["kind"] != "system" {
		t.Fatalf("refresh audit: %v", refresh)
	}
	if d := audit["sso.signed_in"][0]["details"].(map[string]any); d["protocol"] != "saml" {
		t.Fatalf("signed in: %v", d)
	}

	res, err := http.Get(e.h.url + "/auth/saml/metadata/" + e.ws.WorkspaceID.String())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `entityID="`+e.aud+`"`) || !strings.Contains(string(body), `Location="`+e.acs+`"`) ||
		strings.Contains(string(body), "HTTP-Artifact") {
		t.Fatalf("sp metadata: %d %s", res.StatusCode, body)
	}
	if res, err = http.Get(e.h.url + "/auth/saml/metadata/00000000-0000-4000-8000-000000000000"); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("sp metadata of no workspace: %d", res.StatusCode)
	}
}

func TestSAMLRefusesBadResponses(t *testing.T) {
	e := newSAMLEnv(t)
	e.setUp()
	email := "owner@" + e.domain
	other := newSAMLKey(t)
	cases := []struct {
		name string
		tune func(*samlAnswer)
		code string
	}{
		{"unsigned", func(a *samlAnswer) { a.signAssertion = false }, "invalid_token"},
		{"signed by another key", func(a *samlAnswer) { a.signer = &other }, "invalid_token"},
		{"wrong audience", func(a *samlAnswer) { a.audience = "https://sp.example/other" }, "invalid_token"},
		{"wrong recipient", func(a *samlAnswer) { a.recipient = "https://sp.example/acs" }, "invalid_token"},
		{"wrong destination", func(a *samlAnswer) { a.destination = "https://sp.example/acs" }, "invalid_token"},
		{"expired", func(a *samlAnswer) { a.notOnOrAfter = time.Now().Add(-10 * time.Minute) }, "invalid_token"},
		{"wrapping: a second, unsigned assertion", func(a *samlAnswer) {
			a.edit = func(resp, _ *etree.Element) {
				evil := e.saml.assertion(*a, "_evil", "mallory@"+e.domain)
				resp.InsertChildAt(2, evil)
			}
		}, "invalid_token"},
		{"wrapping: the signed assertion hidden inside an unsigned one", func(a *samlAnswer) {
			a.edit = func(resp, signed *etree.Element) {
				evil := e.saml.assertion(*a, "_evil", "mallory@"+e.domain)
				resp.RemoveChild(signed)
				evil.CreateElement("saml:Advice").AddChild(signed)
				resp.AddChild(evil)
			}
		}, "invalid_token"},
		{"unclaimed domain", func(a *samlAnswer) { a.email = "owner@" + unique("other") + ".test" }, "domain_not_claimed"},
	}
	for _, tc := range cases {
		c := e.h.client()
		wantLocation(t, e.signIn(c, email, tc.tune), "/sign-in?sso_error="+tc.code)
		c.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	}

	swapped := e.h.client()
	id := e.start(swapped, email)
	a := e.good(id, email)
	a.signAssertion, a.signResponse = false, true
	doc := e.saml.answer(a)
	doc = strings.Replace(doc, ">"+email+"<", ">mallory@"+e.domain+"<", 1)
	wantLocation(t, e.post(swapped, doc), "/sign-in?sso_error=invalid_token")

	replay := e.h.client()
	id = e.start(replay, email)
	first := e.good(id, email)
	good := e.saml.answer(first)
	wantLocation(t, e.post(replay, good), "/conversations")
	wantLocation(t, e.post(replay, good), "/sign-in?sso_error=expired")
	again := e.h.client()
	second := e.good(e.start(again, email), email)
	second.assertionID = first.assertionID
	wantLocation(t, e.post(again, e.saml.answer(second)), "/sign-in?sso_error=invalid_token")

	unsolicited := e.h.client()
	e.start(unsolicited, email)
	idpInitiated := e.good("", email)
	wantLocation(t, e.post(unsolicited, e.saml.answer(idpInitiated)), "/sign-in?sso_error=expired")

	victim, attacker := e.h.client(), e.h.client()
	victimID := e.start(victim, email)
	e.start(attacker, email)
	wantLocation(t, e.post(attacker, e.saml.answer(e.good(victimID, email))), "/sign-in?sso_error=expired")
	wantLocation(t, e.post(e.h.client(), e.saml.answer(e.good(victimID, email))), "/sign-in?sso_error=expired")
	attacker.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)

	xxe := e.h.client()
	xid := e.start(xxe, email)
	body := e.saml.answer(e.good(xid, email))
	body = `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + strings.Replace(body, ">"+email+"<", ">&x;<", 1)
	wantLocation(t, e.post(xxe, body), "/sign-in?sso_error=expired")
	xxe.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)

	refused := e.auditActions(t)["sso.sign_in_refused"]
	if len(refused) != len(cases)+2 {
		t.Fatalf("refusals: %d, want %d", len(refused), len(cases)+2)
	}
	for _, r := range refused {
		if r["details"].(map[string]any)["protocol"] != "saml" {
			t.Fatalf("refusal: %v", r)
		}
	}
}

func TestSAMLSettingsAndGate(t *testing.T) {
	e := newSAMLEnv(t)
	e.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sso/saml", map[string]any{})
	e.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/sso/saml",
		map[string]any{"metadata_url": e.saml.srv.URL + "/metadata", "metadata_xml": e.saml.metadata()})
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_url": e.saml.srv.URL + "/nope"})
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_url": "http://169.254.169.254/metadata"})
	xxe := `<?xml version="1.0"?><!DOCTYPE md [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + strings.TrimPrefix(e.saml.metadata(), `<?xml version="1.0"?>`)
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_xml": xxe})
	noRedirect := strings.Replace(e.saml.metadata(), "HTTP-Redirect", "HTTP-Artifact", 1)
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_xml": noRedirect})

	agentEmail := "agent@" + e.domain
	e.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})
	codeSession := e.h.client()
	codeSession.signIn(agentEmail)
	codeSession.workspace = e.ws.WorkspaceID.String()

	e.owner.expect(http.StatusOK, "PUT", "/v1/sso/oidc", map[string]any{"issuer": e.idp.srv.URL, "client_id": idpClientID, "client_secret": idpClientSecret})
	e.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/sso/saml", nil)
	e.setUp()
	got := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if got.str("protocol") != "saml" || got.body["oidc"] != nil || got.body["saml"] == nil {
		t.Fatalf("after switching: %s", got.raw)
	}
	e.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/sso/oidc", nil)

	rivalEmail := unique("rival") + "@example.com"
	e.h.bootstrap(rivalEmail, unique("ws"))
	rival := e.h.client()
	rival.signIn(rivalEmail)
	rival.workspace = e.ws.WorkspaceID.String()
	rival.expectProblem(http.StatusForbidden, "not_a_member", "PUT", "/v1/sso/saml", map[string]any{"metadata_xml": e.saml.metadata()})
	rival.expectProblem(http.StatusForbidden, "not_a_member", "DELETE", "/v1/sso/saml", nil)

	e.owner.expect(http.StatusOK, "PATCH", "/v1/sso", map[string]any{"sso_required": true})
	codeSession.expectProblem(http.StatusForbidden, "sso_required", "GET", "/v1/inboxes", nil)
	viaSAML := e.h.client()
	wantLocation(t, e.signIn(viaSAML, agentEmail, nil), "/conversations")
	viaSAML.workspace = e.ws.WorkspaceID.String()
	viaSAML.expect(http.StatusOK, "GET", "/v1/inboxes", nil)

	e.owner.expect(http.StatusNoContent, "DELETE", "/v1/sso/saml", nil)
	got = e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil)
	if got.body["sso_required"] != false || got.body["saml"] != nil || got.body["protocol"] != nil {
		t.Fatalf("after removal: %s", got.raw)
	}
	codeSession.expect(http.StatusOK, "GET", "/v1/inboxes", nil)
	if a := e.auditActions(t)["sso.provider_created"]; len(a) != 2 || a[0]["details"].(map[string]any)["replaced"] != "oidc" {
		t.Fatalf("provider_created: %v", a)
	}
}

func TestSAMLExpiredCertificatesAndRefreshBackoff(t *testing.T) {
	e := newSAMLEnv(t)
	old := newSAMLKeyValid(t, time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
	valid := e.saml.key
	e.saml.mu.Lock()
	e.saml.key, e.saml.expired = samlKey{}, []samlKey{old}
	e.saml.mu.Unlock()
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_xml": e.saml.metadata()})
	e.owner.expectProblem(http.StatusBadRequest, "sso_metadata_failed", "PUT", "/v1/sso/saml", map[string]any{"metadata_url": e.saml.srv.URL + "/metadata"})

	e.saml.mu.Lock()
	e.saml.key = valid
	e.saml.mu.Unlock()
	got := e.owner.expect(http.StatusOK, "PUT", "/v1/sso/saml", map[string]any{"metadata_url": e.saml.srv.URL + "/metadata"})
	warnings, _ := got.body["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "expired") || len(got.body["saml"].(map[string]any)["certificates"].([]any)) != 1 {
		t.Fatalf("expired certificate left out: %s", got.raw)
	}
	if again := e.owner.expect(http.StatusOK, "GET", "/v1/sso", nil); again.body["warnings"] != nil {
		t.Fatalf("warnings outside the PUT: %s", again.raw)
	}
	d := e.owner.expect(http.StatusCreated, "POST", "/v1/sso/domains", map[string]any{"domain": e.domain})
	e.dns.set(d.str("txt_name"), d.str("txt_value"))
	e.owner.expect(http.StatusOK, "POST", "/v1/sso/domains/"+d.str("id")+"/verify", nil)
	email := "owner@" + e.domain

	fetches := func() int {
		e.saml.mu.Lock()
		defer e.saml.mu.Unlock()
		return e.saml.fetches
	}
	e.saml.mu.Lock()
	e.saml.down = true
	e.saml.mu.Unlock()
	before := fetches()
	e.h.clock.Advance(61 * time.Minute)
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/conversations")
	if fetches() != before+1 {
		t.Fatalf("refresh after an hour: %d fetches, want %d", fetches(), before+1)
	}
	e.h.clock.Advance(14 * time.Minute)
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/conversations")
	if fetches() != before+1 {
		t.Fatalf("retried within 15 minutes of a failure: %d fetches", fetches())
	}
	e.h.clock.Advance(2 * time.Minute)
	e.saml.rotate()
	e.saml.mu.Lock()
	e.saml.down = false
	e.saml.mu.Unlock()
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/conversations")
	if fetches() != before+2 {
		t.Fatalf("retry after 15 minutes: %d fetches, want %d", fetches(), before+2)
	}
	e.h.clock.Advance(16 * time.Minute)
	wantLocation(t, e.signIn(e.h.client(), email, nil), "/conversations")
	if fetches() != before+2 {
		t.Fatalf("fetched again within an hour of a success: %d fetches", fetches())
	}
}

func TestSAMLRefusedOnPlainHTTP(t *testing.T) {
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		d.Auth.PublicURL = "http://yuva.example"
	})
	idp := newFakeSAMLIdP(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	owner := h.client()
	owner.signIn(email)
	owner.expectProblem(http.StatusBadRequest, "saml_requires_https", "PUT", "/v1/sso/saml", map[string]any{"metadata_url": idp.srv.URL + "/metadata"})
	if got := owner.expect(http.StatusOK, "GET", "/v1/sso", nil); got.body["protocol"] != nil {
		t.Fatalf("saved anyway: %s", got.raw)
	}
}

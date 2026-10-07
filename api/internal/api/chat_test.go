package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/coder/websocket"
)

type chatTeam struct {
	team
	chatInbox string
	secret    string
	channel   string
	key       string
	origin    string
}

func newChatTeam(t *testing.T, h *harness, mode string, anonymous bool) chatTeam {
	t.Helper()
	tm := newTeam(t, h)
	in := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{
		"name": "Chat", "slug": "chat", "mode": mode, "expected_reply_minutes": 30,
	})
	inbox := in.body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+inbox+"/members/"+tm.agentID, nil)
	origin := "https://" + unique("site") + ".example"
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
		"kind": "chat", "name": "Website",
		"chat": map[string]any{"allowed_origins": []string{origin + "/"}, "allow_anonymous": anonymous, "greeting": "Hi there"},
	})
	chat := ch.body["chat"].(map[string]any)
	if got := chat["allowed_origins"].([]any)[0]; got != origin {
		t.Fatalf("allowed origin stored as %v, want %s", got, origin)
	}
	return chatTeam{team: tm, chatInbox: inbox, secret: in.str("identity_secret"), channel: ch.str("id"), key: chat["public_key"].(string), origin: origin}
}

func jwtPart(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

func signToken(secret string, header, claims map[string]any) string {
	s := jwtPart(header) + "." + jwtPart(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(s))
	return s + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (ct chatTeam) token(h *harness, claims map[string]any) string {
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = h.clock.Now().Add(5 * time.Minute).Unix()
	}
	return signToken(ct.secret, map[string]any{"alg": "HS256", "typ": "JWT"}, claims)
}

type contactSession struct {
	*client
	token     string
	visitor   string
	contactID string
	body      map[string]any
}

func (ct chatTeam) session(h *harness, req map[string]any) contactSession {
	h.t.Helper()
	c := h.client()
	c.origin = ct.origin
	req["channel_key"] = ct.key
	r := c.expect(http.StatusCreated, "POST", "/client/v1/session", req)
	c.bearer = r.str("token")
	return contactSession{client: c, token: r.str("token"), visitor: r.str("visitor_id"), contactID: r.body["contact"].(map[string]any)["id"].(string), body: r.body}
}

func (ct chatTeam) sessionStatus(h *harness, origin string, req map[string]any) response {
	c := h.client()
	c.origin = origin
	req["channel_key"] = ct.key
	return c.do("POST", "/client/v1/session", req)
}

func (cs contactSession) start(body string) string {
	r := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations", map[string]any{"body": body, "client_id": unique("c")})
	return r.body["conversation"].(map[string]any)["id"].(string)
}

func TestIdentityTokenValidation(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	now := h.clock.Now()
	hs := map[string]any{"alg": "HS256", "typ": "JWT"}
	valid := func() map[string]any {
		return map[string]any{"sub": "user-1", "exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix()}
	}
	with := func(k string, v any) map[string]any {
		c := valid()
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}
	good := signToken(ct.secret, hs, valid())
	parts := strings.Split(good, ".")
	cases := map[string]string{
		"alg none":        jwtPart(map[string]any{"alg": "none"}) + "." + jwtPart(valid()) + ".",
		"alg HS512":       signToken(ct.secret, map[string]any{"alg": "HS512"}, valid()),
		"alg RS256":       signToken(ct.secret, map[string]any{"alg": "RS256"}, valid()),
		"wrong secret":    signToken("yuva_is_not-the-secret", hs, valid()),
		"missing exp":     signToken(ct.secret, hs, with("exp", nil)),
		"expired":         signToken(ct.secret, hs, with("exp", now.Add(-time.Minute).Unix())),
		"exp too far":     signToken(ct.secret, hs, with("exp", now.Add(11*time.Minute).Unix())),
		"exp not number":  signToken(ct.secret, hs, with("exp", "soon")),
		"iat in future":   signToken(ct.secret, hs, with("iat", now.Add(5*time.Minute).Unix())),
		"missing sub":     signToken(ct.secret, hs, with("sub", nil)),
		"bad email":       signToken(ct.secret, hs, with("email", "not an address")),
		"attrs not obj":   signToken(ct.secret, hs, with("attrs", []string{"x"})),
		"tampered claims": parts[0] + "." + jwtPart(with("sub", "someone-else")) + "." + parts[2],
		"two parts":       parts[0] + "." + parts[1],
		"garbage":         "not-a-token",
	}
	for name, tok := range cases {
		r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": tok})
		if r.status != http.StatusUnauthorized || r.str("code") != "invalid_identity_token" {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": good})
	if r.status != http.StatusCreated || !r.body["contact"].(map[string]any)["identified"].(bool) {
		t.Fatalf("valid token: %d %s", r.status, r.raw)
	}
	if _, ok := r.body["visitor_id"]; ok {
		t.Fatalf("identified session has a visitor id: %s", r.raw)
	}
	rotated := ct.owner.expect(http.StatusOK, "POST", "/v1/inboxes/"+ct.chatInbox+"/identity-secret", nil).str("identity_secret")
	if r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": good}); r.status != http.StatusUnauthorized {
		t.Fatalf("token signed with the old secret: %d %s", r.status, r.raw)
	}
	ct.secret = rotated
	ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "user-1"})})
}

func TestClientOriginsAndAnonymous(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	other := newChatTeam(t, h, "live", true)

	preflight := func(origin string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", h.url+"/client/v1/session", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "content-type,authorization")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	if res := preflight(ct.origin); res.StatusCode != http.StatusNoContent || res.Header.Get("Access-Control-Allow-Origin") != ct.origin ||
		!strings.Contains(res.Header.Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Fatalf("preflight from an allowed origin: %d %v", res.StatusCode, res.Header)
	}
	if res := preflight("https://evil.example"); res.StatusCode != http.StatusForbidden || !readableRefusal(res.Header, "https://evil.example") {
		t.Fatalf("preflight from an unknown origin: %d %v", res.StatusCode, res.Header)
	}

	r := ct.sessionStatus(h, ct.origin, map[string]any{})
	if r.status != http.StatusForbidden || r.str("code") != "anonymous_not_allowed" {
		t.Fatalf("anonymous on a closed channel: %d %s", r.status, r.raw)
	}
	r = ct.sessionStatus(h, other.origin, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u"})})
	if r.status != http.StatusForbidden || r.str("code") != "origin_not_allowed" {
		t.Fatalf("session from another channel's origin: %d %s", r.status, r.raw)
	}
	r = ct.sessionStatus(h, "https://evil.example", map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u"})})
	if r.status != http.StatusForbidden || r.str("code") != "origin_not_allowed" || !readableRefusal(r.header, "https://evil.example") {
		t.Fatalf("session from an unknown origin: %d %v %s", r.status, r.header, r.raw)
	}
	r = ct.sessionStatus(h, "", map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u"})})
	if r.status != http.StatusForbidden || r.str("code") != "origin_not_allowed" {
		t.Fatalf("chat session without Origin: %d %s", r.status, r.raw)
	}
	nf := h.client()
	nf.origin = ct.origin
	nf.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/session", map[string]any{"channel_key": "yuva_pk_unknown"})

	cs := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u"})})
	if got := cs.do("GET", "/client/v1/conversations", nil).header.Get("Access-Control-Allow-Origin"); got != ct.origin {
		t.Fatalf("Access-Control-Allow-Origin %q", got)
	}
	conv := cs.start("hello")
	for _, origin := range []string{other.origin, "https://evil.example"} {
		cs.origin = origin
		for _, call := range []struct {
			method, path string
			body         any
		}{
			{"GET", "/client/v1/channels/" + ct.key, nil},
			{"POST", "/client/v1/session", map[string]any{"channel_key": ct.key}},
			{"GET", "/client/v1/session", nil},
			{"GET", "/client/v1/conversations", nil},
			{"POST", "/client/v1/conversations", map[string]any{"body": "x", "client_id": unique("c")}},
			{"GET", "/client/v1/conversations/" + conv, nil},
			{"GET", "/client/v1/conversations/" + conv + "/messages", nil},
			{"POST", "/client/v1/conversations/" + conv + "/messages", map[string]any{"body": "x", "client_id": unique("c")}},
			{"POST", "/client/v1/conversations/" + conv + "/read", map[string]any{}},
			{"PUT", "/client/v1/contact/email", map[string]any{"email": "success@simulator.amazonses.com"}},
			{"DELETE", "/client/v1/session", nil},
		} {
			r := cs.do(call.method, call.path, call.body)
			if r.status != http.StatusForbidden || r.str("code") != "origin_not_allowed" {
				t.Fatalf("%s %s from %s: %d %s", call.method, call.path, origin, r.status, r.raw)
			}
			if acao := r.header.Get("Access-Control-Allow-Origin"); acao != "" && acao != origin {
				t.Fatalf("%s %s from %s: Access-Control-Allow-Origin %q", call.method, call.path, origin, acao)
			}
			if r.header.Get("Access-Control-Allow-Credentials") != "" || !slices.Contains(r.header.Values("Vary"), "Origin") {
				t.Fatalf("%s %s from %s: %v", call.method, call.path, origin, r.header)
			}
		}
	}
	cs.origin = ct.origin
	cs.expect(http.StatusOK, "GET", "/client/v1/session", nil)
	_, status, err := dialContact(h, cs.token, other.origin, "")
	if err == nil || status != http.StatusForbidden {
		t.Fatalf("socket from another origin: %d %v", status, err)
	}
	unauth := h.client()
	unauth.origin = ct.origin
	unauth.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/client/v1/conversations", nil)
	unauth.bearer = "yuva_cs_nope"
	unauth.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/client/v1/conversations", nil)

	anon := other.session(h, map[string]any{})
	if anon.visitor == "" || anon.body["contact"].(map[string]any)["identified"].(bool) {
		t.Fatalf("anonymous session: %v", anon.body)
	}
	again := other.session(h, map[string]any{"visitor_id": anon.visitor})
	if again.contactID != anon.contactID || again.visitor != anon.visitor {
		t.Fatalf("visitor id did not resume the visitor: %s vs %s", again.contactID, anon.contactID)
	}
	fresh := other.session(h, map[string]any{"visitor_id": "yuva_v_made-up"})
	if fresh.contactID == anon.contactID || fresh.visitor == "yuva_v_made-up" {
		t.Fatal("an unknown visitor id resumed a visitor")
	}
	if ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "x"}), "visitor_id": anon.visitor}).contactID == anon.contactID {
		t.Fatal("a visitor id of another inbox was used")
	}

	cs.expect(http.StatusNoContent, "DELETE", "/client/v1/session", nil)
	cs.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/client/v1/session", nil)

	other.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+anon.contactID, map[string]any{"blocked": true})
	anon.origin = other.origin
	anon.expectProblem(http.StatusForbidden, "contact_blocked", "GET", "/client/v1/conversations", nil)
	r = other.sessionStatus(h, other.origin, map[string]any{"visitor_id": anon.visitor})
	if r.status != http.StatusForbidden || r.str("code") != "contact_blocked" {
		t.Fatalf("blocked visitor: %d %s", r.status, r.raw)
	}
}

func TestContactMergeRules(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)

	anon := ct.session(h, map[string]any{})
	conv := anon.start("hello from a visitor")
	promoted := ct.session(h, map[string]any{
		"identity_token": ct.token(h, map[string]any{"sub": "u-1", "name": "Ada Lovelace", "email": "ada@example.com", "email_verified": true, "locale": "tr", "attrs": map[string]any{"plan": "pro"}}),
		"visitor_id":     anon.visitor,
	})
	if promoted.contactID != anon.contactID {
		t.Fatalf("a new visitor was not identified in place: %s vs %s", promoted.contactID, anon.contactID)
	}
	c := ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+promoted.contactID, nil)
	if c.str("name") != "Ada Lovelace" || c.str("locale") != "tr" || c.body["attributes"].(map[string]any)["plan"] != "pro" ||
		c.body["emails"].([]any)[0] != "ada@example.com" || c.body["external_ids"].([]any)[0].(map[string]any)["external_id"] != "u-1" {
		t.Fatalf("identified contact: %s", c.raw)
	}
	if again := ct.session(h, map[string]any{"visitor_id": anon.visitor}); again.contactID == promoted.contactID {
		t.Fatal("the visitor id still opens the identified contact anonymously")
	}

	visitor := ct.session(h, map[string]any{})
	conv2 := visitor.start("second visitor")
	merged := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-1"}), "visitor_id": visitor.visitor})
	if merged.contactID != promoted.contactID {
		t.Fatalf("sub did not find the contact: %s", merged.contactID)
	}
	ids := map[string]bool{}
	for _, it := range merged.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any) {
		ids[it.(map[string]any)["id"].(string)] = true
	}
	if !ids[conv] || !ids[conv2] {
		t.Fatalf("conversations after the merge: %v", ids)
	}
	ct.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+visitor.contactID, nil)
	visitor.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/client/v1/conversations", nil)
	if got := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv2, nil).str("contact_id"); got != promoted.contactID {
		t.Fatalf("merged conversation belongs to %s", got)
	}
	for _, m := range messages(ct.owner, conv2) {
		if a := m["author"].(map[string]any); a["type"] == "contact" && a["contact_id"] != promoted.contactID {
			t.Fatalf("message author not moved: %v", a)
		}
	}

	byEmail := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-3", "email": "AYSE@example.com", "email_verified": true})})
	if byEmail.contactID != ct.contact {
		t.Fatalf("token e-mail did not find the existing contact: %s vs %s", byEmail.contactID, ct.contact)
	}

	typed := ct.session(h, map[string]any{})
	r := typed.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": "ayse@example.com"})
	if r.str("typed_email") != "ayse@example.com" || r.body["email"] != nil || r.str("id") == ct.contact {
		t.Fatalf("typed e-mail: %s", r.raw)
	}
	if typed2 := ct.session(h, map[string]any{"visitor_id": typed.visitor}); typed2.contactID != typed.contactID {
		t.Fatal("typed e-mail changed the visitor's contact")
	}
	c = ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+typed.contactID, nil)
	if len(c.body["emails"].([]any)) != 0 {
		t.Fatalf("a typed e-mail became a contact address: %s", c.raw)
	}
}

func TestIdentityTokenJTI(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	once := ct.token(h, map[string]any{"sub": "u-1", "jti": "token-1"})
	ct.session(h, map[string]any{"identity_token": once})
	if r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": once}); r.status != http.StatusUnauthorized || r.str("code") != "invalid_identity_token" {
		t.Fatalf("reused jti: %d %s", r.status, r.raw)
	}
	plain := ct.token(h, map[string]any{"sub": "u-1"})
	ct.session(h, map[string]any{"identity_token": plain})
	ct.session(h, map[string]any{"identity_token": plain})
	if r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-1", "jti": 7})}); r.status != http.StatusUnauthorized {
		t.Fatalf("jti as a number: %d %s", r.status, r.raw)
	}
	h.clock.Advance(6 * time.Minute)
	ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-1", "jti": "token-1"})})
}

func TestIdentityTokenEmailLinking(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	addr := unique("first") + "@example.com"
	first := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-1", "name": "First", "email": addr, "email_verified": true, "attrs": map[string]any{"plan": "pro"}})})
	contact := func(id string) response {
		return ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+id, nil)
	}
	if got := contact(first.contactID); len(got.body["emails"].([]any)) != 1 {
		t.Fatalf("verified e-mail not stored: %s", got.raw)
	}

	second := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-2", "name": "Second", "email": addr, "email_verified": true, "attrs": map[string]any{"plan": "free"}})})
	if second.contactID == first.contactID {
		t.Fatal("a second external id was linked to a contact that already has one in this inbox")
	}
	got := contact(first.contactID)
	if got.str("name") != "First" || got.body["attributes"].(map[string]any)["plan"] != "pro" || len(got.body["external_ids"].([]any)) != 1 {
		t.Fatalf("the first contact changed: %s", got.raw)
	}
	if again := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-1"})}); again.contactID != first.contactID {
		t.Fatal("u-1 no longer finds its contact")
	}

	if unverified := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-3", "email": "AYSE@example.com"})}); unverified.contactID == ct.contact {
		t.Fatal("an unverified e-mail linked an existing contact")
	}
	plain := unique("plain") + "@example.com"
	stored := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-4", "email": plain, "email_verified": false})})
	got = contact(stored.contactID)
	if len(got.body["emails"].([]any)) != 0 || stored.body["contact"].(map[string]any)["typed_email"] != plain {
		t.Fatalf("an unverified e-mail: %s %v", got.raw, stored.body["contact"])
	}
	if again := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-5", "email": plain, "email_verified": true})}); again.contactID == stored.contactID {
		t.Fatal("an address stored from an unverified claim linked another identity")
	}
	if r := ct.sessionStatus(h, ct.origin, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-6", "email": plain, "email_verified": "yes"})}); r.status != http.StatusUnauthorized {
		t.Fatalf("email_verified as a string: %d %s", r.status, r.raw)
	}

	other := ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Other app", "slug": unique("other")}).body["inbox"].(map[string]any)["id"].(string)
	shared := unique("shared") + "@example.com"
	elsewhere := ct.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{
		"name": "Kept", "emails": []string{shared}, "attributes": map[string]any{"plan": "team"},
		"external_ids": []map[string]any{{"inbox_id": other, "external_id": "x-1"}},
	}).str("id")
	linked := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "u-7", "name": "Changed", "email": shared, "email_verified": true, "attrs": map[string]any{"plan": "free", "seats": 3}})})
	if linked.contactID != elsewhere {
		t.Fatal("a verified e-mail did not link a contact known only in another inbox")
	}
	got = contact(elsewhere)
	attrs := got.body["attributes"].(map[string]any)
	if got.str("name") != "Kept" || attrs["plan"] != "team" || attrs["seats"] != float64(3) || len(got.body["external_ids"].([]any)) != 2 {
		t.Fatalf("contact of another inbox after linking: %s", got.raw)
	}
}

func TestContactSeesOnlyOwnPublicMessages(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	outsider := newChatTeam(t, h, "live", true)
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Deniz Kaya"})

	cs := ct.session(h, map[string]any{})
	conv := cs.start("I need help")
	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "secret note about the customer"})
	reply := ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "How can I help?"})
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})

	var noteAtt, replyAtt string
	for kind, dst := range map[string]*string{"note": &noteAtt, "message": &replyAtt} {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("kind", kind)
		_ = mw.WriteField("body", kind+" with file")
		fw, _ := mw.CreateFormFile("files", kind+".txt")
		_, _ = fw.Write([]byte("hello " + kind))
		_ = mw.Close()
		req, _ := http.NewRequest("POST", h.url+"/v1/conversations/"+conv+"/messages", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		res, err := ct.owner.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		res.Body.Close()
		*dst = m["attachments"].([]any)[0].(map[string]any)["id"].(string)
	}

	r := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages?order=desc", nil)
	raw := string(r.raw)
	for _, leak := range []string{"secret note", "note with file", ct.agentID, "@example.com", "member_id", "status_changed", "delivery"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("contact sees %q: %s", leak, raw)
		}
	}
	items := r.body["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("want the visitor's message and two replies, got %s", raw)
	}
	var sawReply bool
	for _, it := range items {
		m := it.(map[string]any)
		if m["id"] == reply.str("id") {
			sawReply = true
			a := m["author"].(map[string]any)
			if a["type"] != "member" || a["name"] != "Deniz Kaya" || a["initials"] != "DK" || m["direction"] != "out" {
				t.Fatalf("reply author: %v", m)
			}
		}
	}
	if !sawReply {
		t.Fatalf("reply missing: %s", raw)
	}
	list := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil)
	item := list.body["items"].([]any)[0].(map[string]any)
	if item["status"] != "pending" || item["unread"] != true || item["last_message"].(map[string]any)["text"] != "message with file" {
		t.Fatalf("conversation list: %s", list.raw)
	}
	read := cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/read", nil)
	if read.body["unread"] != false {
		t.Fatalf("read: %s", read.raw)
	}

	res := cs.do("GET", "/client/v1/attachments/"+replyAtt, nil)
	if res.status != http.StatusOK || string(res.raw) != "hello message" {
		t.Fatalf("reply attachment: %d %s", res.status, res.raw)
	}
	cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/attachments/"+noteAtt, nil)

	again := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "still there?", "client_id": "m-1"})
	if dup := cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "still there?", "client_id": "m-1"}); dup.str("id") != again.str("id") {
		t.Fatal("client_id was not idempotent")
	}
	if st := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("status"); st != "open" {
		t.Fatalf("a contact message did not reopen the conversation: %s", st)
	}

	others := []string{ct.conversation(ct.owner)}
	stranger := ct.session(h, map[string]any{})
	others = append(others, stranger.start("someone else"))
	foreign := outsider.session(h, map[string]any{})
	others = append(others, foreign.start("another workspace"))
	for _, id := range others {
		cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/conversations/"+id, nil)
		cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/conversations/"+id+"/messages", nil)
		cs.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/conversations/"+id+"/messages", map[string]any{"body": "x"})
		cs.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/conversations/"+id+"/read", nil)
		cs.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/conversations/"+id+"/typing", nil)
	}
	for _, it := range cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any) {
		if id := it.(map[string]any)["id"]; id != conv {
			t.Fatalf("contact lists %v", id)
		}
	}
	cs.workspace = ct.ws
	cs.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/conversations/"+conv, nil)
}

func dialContact(h *harness, token, origin, query string) (*wsClient, int, error) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !h.hub.Listening() {
		if time.Now().After(deadline) {
			h.t.Fatal("realtime listener did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	u := "ws" + strings.TrimPrefix(h.url, "http") + "/client/v1/realtime" + query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {origin}}, Subprotocols: []string{"yuva", "yuva.token." + token},
	})
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	if err != nil {
		return nil, status, err
	}
	if conn.Subprotocol() != "yuva" {
		h.t.Fatalf("subprotocol %q", conn.Subprotocol())
	}
	w := &wsClient{t: h.t, conn: conn, msgs: make(chan wsMessage, 1000), err: make(chan error, 1)}
	go func() {
		for {
			_, b, err := conn.Read(context.Background())
			if err != nil {
				w.err <- err
				return
			}
			var m wsMessage
			if err := json.Unmarshal(b, &m); err != nil {
				w.err <- err
				return
			}
			w.msgs <- m
		}
	}()
	h.t.Cleanup(func() { _ = conn.CloseNow() })
	return w, status, nil
}

func (w *wsClient) nextNot(skip ...string) wsMessage {
	w.t.Helper()
	for {
		m := w.next()
		if !strings.Contains(" "+strings.Join(skip, " ")+" ", " "+m.Type+" ") {
			return m
		}
	}
}

func (w *wsClient) quiet(d time.Duration, skip ...string) {
	w.t.Helper()
	deadline := time.After(d)
	for {
		select {
		case m := <-w.msgs:
			if !strings.Contains(" "+strings.Join(skip, " ")+" ", " "+m.Type+" ") {
				w.t.Fatalf("unexpected %s: %s", m.Type, m.Data)
			}
		case <-deadline:
			return
		}
	}
}

func TestClientRealtimeFiltering(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	conv := cs.start("hi")
	other := ct.session(h, map[string]any{})
	otherConv := other.start("not yours")

	if _, status, err := dialContact(h, "yuva_cs_nope", ct.origin, ""); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("socket with a bad token: %d %v", status, err)
	}
	members := ct.agent.dial("")
	members.ready()
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.ready()
	if p := ws.next(); p.Type != "presence" || !strings.Contains(string(p.Data), `"available":true`) {
		t.Fatalf("first presence: %s %s", p.Type, p.Data)
	}

	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+otherConv+"/messages", map[string]any{"kind": "message", "body": "for the other visitor"})
	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "internal"})
	labels := ct.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "vip", "color": "#ff0000"})
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"labels": []string{labels.str("id")}})
	m := ws.nextNot("presence")
	if m.Type != "read" || m.ConversationID != conv {
		t.Fatalf("the member's note moves the read receipt: %s %s", m.Type, m.Data)
	}
	h.clock.Advance(time.Second)
	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Hello!"})
	m = ws.nextNot("presence")
	helloID := m.ID
	if m.Type != "message.created" || m.ConversationID != conv || m.ID == 0 || !strings.Contains(string(m.Data), `"Hello!"`) ||
		strings.Contains(string(m.Data), "member_id") {
		t.Fatalf("first frame after the reply: %s %s", m.Type, m.Data)
	}
	if m = ws.nextNot("presence"); m.Type != "read" {
		t.Fatalf("the member's reply moves the read receipt: %s %s", m.Type, m.Data)
	}
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	m = ws.nextNot("presence")
	if m.Type != "conversation.updated" || !strings.Contains(string(m.Data), `"status":"closed"`) || strings.Contains(string(m.Data), "labels") {
		t.Fatalf("status frame: %s %s", m.Type, m.Data)
	}

	ct.agent.expect(http.StatusNoContent, "POST", "/v1/conversations/"+conv+"/typing", map[string]any{"typing": true})
	m = ws.nextNot("presence")
	if m.Type != "typing" || m.ID != 0 || !strings.Contains(string(m.Data), `"type":"member"`) || strings.Contains(string(m.Data), ct.agentID) {
		t.Fatalf("member typing on the contact socket: %s %s", m.Type, m.Data)
	}
	cs.expect(http.StatusNoContent, "POST", "/client/v1/conversations/"+conv+"/typing", nil)
	for {
		m = members.next()
		if m.Type == "typing" {
			break
		}
	}
	if m.ConversationID != conv || !strings.Contains(string(m.Data), `"type":"contact"`) || !strings.Contains(string(m.Data), cs.contactID) {
		t.Fatalf("contact typing on /v1/realtime: %s", m.Data)
	}

	h.clock.Advance(time.Second)
	cs.expect(http.StatusCreated, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "thanks"})
	ws.until(messageIn(conv))
	ct.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	m = ws.nextNot("presence")
	if m.Type != "read" || !strings.Contains(string(m.Data), `"read_at"`) {
		t.Fatalf("read frame: %s %s", m.Type, m.Data)
	}

	conv2 := cs.start("another one")
	m = ws.nextNot("presence")
	if m.Type != "conversation.created" || m.ConversationID != conv2 {
		t.Fatalf("own new conversation: %s %s", m.Type, m.Data)
	}
	if m = ws.nextNot("presence"); m.Type != "message.created" || m.ConversationID != conv2 {
		t.Fatalf("own new message: %s %s", m.Type, m.Data)
	}
	ws.quiet(300*time.Millisecond, "presence")

	ws.close()
	resumed, _, err := dialContact(h, cs.token, ct.origin, fmt.Sprintf("?last_event_id=%d", helloID))
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, m := range resumed.until(func(m wsMessage) bool { return m.Type == "ready" }) {
		types = append(types, m.Type)
	}
	if got := strings.Join(types, " "); got != "read conversation.updated conversation.updated message.created read conversation.created message.created ready" {
		t.Fatalf("replay: %s", got)
	}
	if _, status, err := dialContact(h, cs.token, ct.origin, "?last_event_id=1"); err != nil || status != http.StatusSwitchingProtocols {
		t.Fatalf("resume from an unknown id: %d %v", status, err)
	}
}

func TestPresenceRules(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	presence := func() map[string]any {
		r := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil)
		p, _ := r.body["inbox"].(map[string]any)["presence"].(map[string]any)
		return p
	}
	if p := presence(); p["available"] != false {
		t.Fatalf("nobody connected: %v", p)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"name": "Ali Veli"})
	agentWS := ct.agent.dial("")
	agentWS.ready()
	if p := presence(); p["available"] != true || p["members"].([]any)[0].(map[string]any)["initials"] != "AV" {
		t.Fatalf("agent connected: %v", p)
	}
	if me := ct.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"}); me.body["person"].(map[string]any)["availability"] != "away" {
		t.Fatalf("availability: %s", me.raw)
	}
	if p := presence(); p["available"] != false {
		t.Fatalf("agent away: %v", p)
	}
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "auto"})
	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+ct.chatInbox+"/members/"+ct.agentID, nil)
	if p := presence(); p["available"] != false {
		t.Fatalf("agent without access: %v", p)
	}
	ct.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+ct.chatInbox+"/members/"+ct.agentID, nil)
	if p := presence(); p["available"] != true {
		t.Fatalf("access back: %v", p)
	}

	other := map[string]string{"mon": "tue", "tue": "wed", "wed": "thu", "thu": "fri", "fri": "sat", "sat": "sun", "sun": "mon"}
	today := strings.ToLower(h.clock.Now().UTC().Weekday().String()[:3])
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{
		"timezone": "UTC", "business_hours": map[string]any{"enabled": true, "intervals": []any{map[string]any{"day": other[today], "start": "00:00", "end": "24:00"}}},
	})
	r := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil)
	if in := r.body["inbox"].(map[string]any); in["open_now"] != false || in["presence"].(map[string]any)["available"] != false {
		t.Fatalf("outside business hours: %s", r.raw)
	}
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{
		"business_hours": map[string]any{"enabled": true, "intervals": []any{map[string]any{"day": today, "start": "00:00", "end": "24:00"}}},
	})
	if p := presence(); p["available"] != true {
		t.Fatalf("within business hours: %v", p)
	}

	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.ready()
	if p := ws.next(); p.Type != "presence" || !strings.Contains(string(p.Data), `"available":true`) {
		t.Fatalf("presence frame: %s %s", p.Type, p.Data)
	}
	agentWS.close()
	if p := ws.next(); p.Type != "presence" || !strings.Contains(string(p.Data), `"available":false`) {
		t.Fatalf("presence after the agent left: %s %s", p.Type, p.Data)
	}

	ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"mode": "async"})
	r = cs.expect(http.StatusOK, "GET", "/client/v1/session", nil)
	in := r.body["inbox"].(map[string]any)
	if _, ok := in["presence"]; ok || in["expected_reply_minutes"] != float64(30) || in["mode"] != "async" {
		t.Fatalf("async inbox: %s", r.raw)
	}
	if in["chat"].(map[string]any)["greeting"] != "Hi there" {
		t.Fatalf("chat settings: %s", r.raw)
	}
}

func decodeHeader(t *testing.T, v string) string {
	t.Helper()
	out, err := new(mime.WordDecoder).DecodeHeader(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestChatEmailContinuity(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	addr := unique("support") + "@example.com"
	ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
		"kind": "email", "name": "Support mail",
		"email": map[string]any{"address": addr, "display_name": "Acme", "smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "tls": "starttls"}},
	})
	ws := uuid.MustParse(ct.ws)
	check := func(conv string) *time.Time {
		t.Helper()
		next, err := h.srv.CheckContinuity(context.Background(), ws, uuid.MustParse(conv))
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	sendBatch := func() {
		t.Helper()
		rows, err := h.st.Pool.Query(context.Background(),
			"SELECT args FROM river_job WHERE kind = 'email_send' AND args->>'workspace_id' = $1 AND state = 'available' ORDER BY id", ct.ws)
		if err != nil {
			t.Fatal(err)
		}
		var jobs []struct {
			MessageID uuid.UUID   `json:"message_id"`
			Batch     []uuid.UUID `json:"batch"`
		}
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var j struct {
				MessageID uuid.UUID   `json:"message_id"`
				Batch     []uuid.UUID `json:"batch"`
			}
			_ = json.Unmarshal(raw, &j)
			jobs = append(jobs, j)
		}
		rows.Close()
		for _, j := range jobs {
			if err := h.srv.SendEmails(context.Background(), ws, j.Batch, false); err != nil {
				t.Fatal(err)
			}
		}
		_, _ = h.st.Pool.Exec(context.Background(), "DELETE FROM river_job WHERE args->>'workspace_id' = $1", ct.ws)
	}
	reply := func(conv, body string) string {
		return ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": body}).str("id")
	}

	cs := ct.session(h, map[string]any{})
	conv := cs.start("anyone   there?\nI have a question about my order number 4711 and the delivery date of it")
	typed := unique("visitor") + "@example.com"
	cs.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": typed})
	start := h.clock.Now()
	reply(conv, "First answer")
	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "do not mail this note"})
	h.clock.Advance(time.Minute)
	second := reply(conv, "Second answer")

	near := func(a *time.Time, b time.Time) bool { return a != nil && a.Sub(b).Abs() < time.Millisecond }
	if next := check(conv); !near(next, start.Add(5*time.Minute)) {
		t.Fatalf("before the delay: next %v, want %v", next, start.Add(5*time.Minute))
	}
	h.clock.Advance(4*time.Minute + time.Second)
	if next := check(conv); next != nil {
		t.Fatalf("due: next %v", next)
	}
	before := h.smtp.count()
	sendBatch()
	if h.smtp.count() != before+1 {
		t.Fatalf("want one e-mail for both replies, got %d", h.smtp.count()-before)
	}
	sent, parsed := h.smtp.last(t)
	body := string(sent.raw)
	if got, want := decodeHeader(t, parsed.Header.Get("Subject")), "Chat — anyone there? I have a question about my order number 4711 a…"; got != want {
		t.Fatalf("subject %q, want %q", got, want)
	}
	if sent.to[0] != typed || !strings.Contains(body, "First answer") || !strings.Contains(body, "Second answer") || strings.Contains(body, "note") {
		t.Fatalf("continuity e-mail to %v:\n%s", sent.to, body)
	}
	for _, m := range messages(ct.owner, conv) {
		if m["id"] == second && m["delivery"].(map[string]any)["state"] != "sent" {
			t.Fatalf("delivery of the reply: %v", m["delivery"])
		}
	}
	if next := check(conv); next != nil {
		t.Fatalf("nothing pending, next %v", next)
	}

	reply(conv, "Third answer")
	h.clock.Advance(5*time.Minute + time.Second)
	_, _ = h.st.Pool.Exec(context.Background(), "UPDATE conversations SET continuity_sent_at = $2 WHERE id = $1", conv, h.clock.Now().Add(-time.Minute))
	if next := check(conv); !near(next, h.clock.Now().Add(4*time.Minute)) {
		t.Fatalf("one batch per period: next %v", next)
	}
	h.clock.Advance(4 * time.Minute)
	cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/read", nil)
	if next := check(conv); next != nil {
		t.Fatalf("read replies are not mailed: next %v", next)
	}

	reply(conv, "Fourth answer")
	live, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	live.ready()
	h.clock.Advance(5*time.Minute + time.Second)
	_, _ = h.st.Pool.Exec(context.Background(), "UPDATE realtime_connections SET seen_at = $2 WHERE contact_id = $1", cs.contactID, h.clock.Now())
	if next := check(conv); next == nil || h.smtp.count() != before+1 {
		t.Fatalf("connected contact: next %v, sent %d", next, h.smtp.count()-before)
	}
	live.close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		_ = h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM realtime_connections WHERE contact_id = $1", cs.contactID).Scan(&n)
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("contact connection not closed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if next := check(conv); next == nil || !next.After(h.clock.Now()) {
		t.Fatalf("just disconnected: next %v", next)
	}
	h.clock.Advance(5*time.Minute + time.Second)
	if next := check(conv); next != nil {
		t.Fatalf("gone long enough: next %v", next)
	}
	sendBatch()
	if h.smtp.count() != before+2 {
		t.Fatalf("second batch: %d", h.smtp.count()-before)
	}
	_, parsed = h.smtp.last(t)
	msgID := parsed.Header.Get("Message-ID")

	contactEvents := func() []string {
		rows, err := h.st.Pool.Query(context.Background(), "SELECT payload::text FROM events WHERE workspace_id = $1 AND type = 'contact.updated' AND payload->>'id' = $2 ORDER BY id", ct.ws, cs.contactID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				t.Fatal(err)
			}
			out = append(out, p)
		}
		return out
	}
	replyMail := func(body string) []byte {
		return []byte("From: Visitor <" + typed + ">\r\nTo: " + addr + "\r\nSubject: Re: Chat\r\nMessage-ID: <" + unique("r") + "@example.com>\r\n" +
			"In-Reply-To: " + msgID + "\r\nReferences: " + msgID + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body + "\r\n")
	}
	unverified := func(id string) any {
		t.Helper()
		for _, m := range messages(ct.owner, conv) {
			if m["id"] == id {
				return m["email"].(map[string]any)["unverified_sender"]
			}
		}
		t.Fatalf("message %s not in the conversation", id)
		return nil
	}
	owners := func() int {
		var n int
		if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = $2", ct.ws, typed).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	early := h.ingest(addr, replyMail("Is anyone reading this?"), nil)
	if early.status != http.StatusAccepted || early.str("conversation_id") != conv {
		t.Fatalf("a reply from the unconfirmed typed address did not continue the chat: %d %v", early.status, early.body)
	}
	if unverified(early.str("message_id")) != true || owners() != 0 {
		t.Fatalf("unconfirmed reply: unverified %v, %d contacts own the address", unverified(early.str("message_id")), owners())
	}
	if len(ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+cs.contactID, nil).body["emails"].([]any)) != 0 {
		t.Fatal("a reply linked the unconfirmed address")
	}

	link := confirmLink(t, h.mail.wait(t, typed, 1)[0].Text)
	confirmTypedEmail(t, h, link)
	if unverified(early.str("message_id")) != false {
		t.Fatal("the earlier reply is still from an unverified address after confirmation")
	}
	if evs := contactEvents(); !strings.Contains(evs[len(evs)-1], `"emails": ["`+typed+`"]`) {
		t.Fatalf("confirming the typed address: contact.updated events %v", evs)
	}
	res := h.ingest(addr, replyMail("Thanks, that helped."), nil)
	if res.str("conversation_id") != conv || unverified(res.str("message_id")) != false {
		t.Fatalf("the e-mail answer did not continue the chat: %v", res.body)
	}
	c := ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+cs.contactID, nil)
	if len(c.body["emails"].([]any)) != 1 || c.body["emails"].([]any)[0] != typed {
		t.Fatalf("confirmed typed address is the contact's: %s", c.raw)
	}
	found := false
	for _, it := range cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages", nil).body["items"].([]any) {
		found = found || it.(map[string]any)["body"] == "Thanks, that helped."
	}
	if !found {
		t.Fatal("the e-mail answer is not in the widget thread")
	}
}

var confirmURL = regexp.MustCompile(`https?://\S+/email/confirm\?token=\S+`)

func confirmLink(t *testing.T, text string) string {
	t.Helper()
	link := confirmURL.FindString(text)
	if link == "" {
		t.Fatalf("no confirmation link in %q", text)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("token")
}

func confirmStatus(t *testing.T, h *harness, method, token string) (int, string) {
	t.Helper()
	var (
		res *http.Response
		err error
	)
	if method == "GET" {
		res, err = http.Get(h.url + "/email/confirm?token=" + url.QueryEscape(token))
	} else {
		res, err = http.PostForm(h.url+"/email/confirm", url.Values{"token": {token}})
	}
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func confirmTypedEmail(t *testing.T, h *harness, token string) {
	t.Helper()
	if status, page := confirmStatus(t, h, "POST", token); status != http.StatusOK || !strings.Contains(page, "Address confirmed") {
		t.Fatalf("confirming: %d %s", status, page)
	}
}

func TestTypedEmailConfirmation(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	emails := func(contact string) []any {
		return ct.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+contact, nil).body["emails"].([]any)
	}
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	typed := unique("visitor") + "@example.com"
	cs.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": typed})
	msg := h.mail.wait(t, typed, 1)[0]
	if !strings.Contains(msg.Subject, "Chat") || strings.Contains(msg.Text, "hello") {
		t.Fatalf("confirmation mail: %q %q", msg.Subject, msg.Text)
	}
	token := confirmLink(t, msg.Text)

	status, page := confirmStatus(t, h, "GET", token)
	if status != http.StatusOK || !strings.Contains(page, `method="post"`) {
		t.Fatalf("opening the link: %d %s", status, page)
	}
	if len(emails(cs.contactID)) != 0 {
		t.Fatal("opening the link confirmed the address")
	}
	if status, _ := confirmStatus(t, h, "POST", token+"x"); status != http.StatusNotFound {
		t.Fatalf("wrong token: %d", status)
	}
	confirmTypedEmail(t, h, token)
	if got := emails(cs.contactID); len(got) != 1 || got[0] != typed {
		t.Fatalf("confirmed address: %v", got)
	}
	if status, _ := confirmStatus(t, h, "POST", token); status != http.StatusNotFound {
		t.Fatalf("second use: %d", status)
	}

	other := ct.session(h, map[string]any{})
	other.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": typed})
	time.Sleep(50 * time.Millisecond)
	if n := len(h.mail.to(typed)); n != 1 {
		t.Fatalf("%d confirmation mails for an address a contact owns", n)
	}

	late := ct.session(h, map[string]any{})
	lateAddr := unique("late") + "@example.com"
	for range 4 {
		late.expect(http.StatusOK, "PUT", "/client/v1/contact/email", map[string]any{"email": lateAddr})
	}
	msgs := h.mail.wait(t, lateAddr, 3)
	time.Sleep(50 * time.Millisecond)
	if n := len(h.mail.to(lateAddr)); n != 3 {
		t.Fatalf("%d confirmation mails in an hour, want 3", n)
	}
	h.clock.Advance(25 * time.Hour)
	if status, _ := confirmStatus(t, h, "POST", confirmLink(t, msgs[2].Text)); status != http.StatusNotFound {
		t.Fatalf("expired link: %d", status)
	}
	if len(emails(late.contactID)) != 0 {
		t.Fatal("an expired link confirmed the address")
	}
}

func readableRefusal(h http.Header, origin string) bool {
	return h.Get("Access-Control-Allow-Origin") == origin && slices.Contains(h.Values("Vary"), "Origin") &&
		h.Get("Access-Control-Allow-Credentials") == "" && h.Get("Access-Control-Allow-Methods") == "" &&
		h.Get("Access-Control-Allow-Headers") == ""
}

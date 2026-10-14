package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jhillyerd/enmime/v2"

	"github.com/productdevbook/yuva/api/internal/api"
)

type listTeam struct {
	chatTeam
	emailChannel string
	list         string
}

func newListTeam(t *testing.T, h *harness) listTeam {
	t.Helper()
	ct := newChatTeam(t, h, "live", true)
	ch := ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.chatInbox+"/channels", map[string]any{
		"kind": "email", "name": "News mail",
		"email": map[string]any{
			"address": unique("news") + "@example.com", "display_name": "Acme News",
			"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	l := ct.owner.expect(http.StatusCreated, "POST", "/v1/lists", map[string]any{
		"inbox_id": ct.chatInbox, "name": "Product news", "description": "One mail a month.", "channel_id": ch.str("id"),
	})
	if l.body["sender_ready"] != true || l.body["double_opt_in"] != true {
		t.Fatalf("new list: %s", l.raw)
	}
	return listTeam{chatTeam: ct, emailChannel: ch.str("id"), list: l.str("id")}
}

// runListMail sends the queued list mails as the job queue would and returns what went out.
func (h *harness) runListMail(ws string) []*enmime.Envelope {
	h.t.Helper()
	before := h.smtp.count()
	for _, j := range h.jobs("list_mail", ws) {
		var a api.ListMailArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.SendListMail(context.Background(), a, false); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	h.smtp.mu.Lock()
	defer h.smtp.mu.Unlock()
	var out []*enmime.Envelope
	for _, m := range h.smtp.sent[before:] {
		env, err := enmime.ReadEnvelope(bytes.NewReader(m.raw))
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, env)
	}
	return out
}

var listLink = regexp.MustCompile(`https?://[^\s"<>]+/(s|m)/[A-Za-z0-9_-]+`)

func linkIn(t *testing.T, env *enmime.Envelope, kind string) string {
	t.Helper()
	for _, l := range listLink.FindAllString(env.Text, -1) {
		if strings.Contains(l, "/"+kind+"/") {
			u, _ := url.Parse(l)
			return u.Path
		}
	}
	t.Fatalf("no /%s/ link in %q", kind, env.Text)
	return ""
}

func (lt listTeam) form(h *harness) (*client, string) {
	h.t.Helper()
	c := h.client()
	c.origin = lt.origin
	r := c.expect(http.StatusOK, "GET", "/client/v1/channels/"+lt.key+"/lists", nil)
	if r.header.Get("Cache-Control") != "no-store" || len(r.body["lists"].([]any)) != 1 {
		h.t.Fatalf("form lists: %s", r.raw)
	}
	return c, r.str("form_token")
}

func (lt listTeam) subscribeBody(email, token string) map[string]any {
	return map[string]any{
		"email": email, "list_ids": []string{lt.list}, "page_url": lt.origin + "/news", "form_token": token,
		"consent_text": "I want product news from Chat.",
	}
}

func pageAt(t *testing.T, h *harness, method, path string, form url.Values) (int, string) {
	t.Helper()
	var (
		res *http.Response
		err error
	)
	if method == "GET" {
		res, err = http.Get(h.url + path)
	} else {
		res, err = http.PostForm(h.url+path, form)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (lt listTeam) subscription(t *testing.T, email string) map[string]any {
	t.Helper()
	for _, it := range items(lt.owner.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/subscriptions?q="+url.QueryEscape(email), nil)) {
		if it["email"] == email {
			return lt.owner.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/subscriptions/"+it["id"].(string), nil).body
		}
	}
	return nil
}

func eventKinds(sub map[string]any) []string {
	var out []string
	for _, e := range sub["events"].([]any) {
		out = append(out, e.(map[string]any)["kind"].(string))
	}
	return out
}

func TestSubscribeFormConfirmAndOneClick(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	c, token := lt.form(h)
	addr := unique("reader") + "@example.org"
	c.expectProblem(http.StatusBadRequest, "form_token", "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(addr, token))
	h.clock.Advance(3 * time.Second)
	r := c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(addr, token))
	if r.body["confirmation"] != true {
		t.Fatalf("answer %s", r.raw)
	}
	mails := h.runListMail(lt.ws)
	if len(mails) != 1 || mails[0].GetHeader("Auto-Submitted") != "auto-generated" || mails[0].GetHeader("List-Unsubscribe") != "" {
		t.Fatalf("confirmation mails: %d", len(mails))
	}
	if strings.Contains(mails[0].Text, lt.origin) {
		t.Fatal("the confirmation repeats the page the visitor typed")
	}
	if len(h.mail.to(addr)) != 0 {
		t.Fatal("list mail went through the server mailer")
	}
	confirm := linkIn(t, mails[0], "s")

	for range 2 {
		if status, page := pageAt(t, h, "GET", confirm, nil); status != http.StatusOK || !strings.Contains(page, "Confirm subscription") {
			t.Fatalf("GET /s/: %d %s", status, page)
		}
	}
	if sub := lt.subscription(t, addr); sub["state"] != "pending" {
		t.Fatalf("opening /s/ changed the subscription: %v", sub["state"])
	}
	if status, page := pageAt(t, h, "POST", confirm, url.Values{"action": {"confirm"}}); status != http.StatusOK || !strings.Contains(page, "You are subscribed") {
		t.Fatalf("POST /s/: %d %s", status, page)
	}
	sub := lt.subscription(t, addr)
	if sub["state"] != "subscribed" || sub["consent_at"] == nil || sub["confirmed_at"] == nil {
		t.Fatalf("confirmed: %v", sub)
	}
	if n := contactEmailOwners(t, h, lt.ws, addr); n != 1 {
		t.Fatalf("confirming did not give the form's contact its address: %d owners", n)
	}
	if got := strings.Join(eventKinds(sub), ","); got != "requested,confirmation_sent,confirmed" {
		t.Fatalf("consent events %s", got)
	}
	requested := sub["events"].([]any)[0].(map[string]any)
	if requested["ip"] != c.ip || requested["consent_text"] != "I want product news from Chat." || requested["page_url"] != lt.origin+"/news" {
		t.Fatalf("requested event %v", requested)
	}
	if confirmed := sub["events"].([]any)[2].(map[string]any); confirmed["ip"] == nil {
		t.Fatalf("confirmed event without IP: %v", confirmed)
	}

	welcome := h.runListMail(lt.ws)
	if len(welcome) != 1 || welcome[0].GetHeader("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
		t.Fatalf("welcome mails: %d", len(welcome))
	}
	manage := linkIn(t, welcome[0], "m")
	if got := welcome[0].GetHeader("List-Unsubscribe"); got != "<"+testOrigin+manage+">" {
		t.Fatalf("List-Unsubscribe %q, want %s", got, manage)
	}
	if status, page := pageAt(t, h, "GET", manage, nil); status != http.StatusOK || !strings.Contains(page, "Unsubscribe from") {
		t.Fatalf("GET /m/: %d %s", status, page)
	}
	if sub := lt.subscription(t, addr); sub["state"] != "subscribed" {
		t.Fatalf("opening /m/ changed the subscription: %v", sub["state"])
	}
	status, body := pageAt(t, h, "POST", manage, url.Values{"List-Unsubscribe": {"One-Click"}})
	if status != http.StatusOK || body != "" {
		t.Fatalf("one-click: %d %q", status, body)
	}
	sub = lt.subscription(t, addr)
	if sub["state"] != "unsubscribed" || sub["unsubscribe_reason"] != "one_click" {
		t.Fatalf("after one-click: %v", sub)
	}

	other := newListTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/lists/"+lt.list, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/lists/"+lt.list+"/subscriptions/"+sub["id"].(string), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/lists/"+lt.list+"/consent-export", nil)
	oc, otherToken := other.form(h)
	h.clock.Advance(3 * time.Second)
	foreignList := other.subscribeBody(addr, otherToken)
	foreignList["list_ids"] = []string{lt.list}
	oc.expectProblem(http.StatusNotFound, "list_not_found", "POST", "/client/v1/channels/"+other.key+"/subscriptions", foreignList)
	foreign := h.client()
	foreign.origin = "https://elsewhere.example"
	foreign.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", "/client/v1/channels/"+lt.key+"/lists", nil)
}

func TestSubscribeAnswersEveryAddressAlike(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	c, token := lt.form(h)
	h.clock.Advance(3 * time.Second)
	post := func(addr string) string {
		t.Helper()
		return string(c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(addr, token)).raw)
	}
	subscribed := unique("sub") + "@example.org"
	lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": subscribed, "consent": map[string]any{"confirmed": true}})
	unsubscribed := unique("unsub") + "@example.org"
	u := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": unsubscribed, "consent": map[string]any{"confirmed": true}})
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+lt.list+"/subscriptions/"+u.str("id"), map[string]any{"state": "unsubscribed"})
	suppressed := unique("bounced") + "@example.org"
	if _, err := h.st.Pool.Exec(context.Background(), "INSERT INTO email_suppressions (workspace_id, email, reason, created_at) VALUES ($1, $2, 'bounce', now())", lt.ws, suppressed); err != nil {
		t.Fatal(err)
	}
	fresh := unique("new") + "@example.org"
	answers := map[string]string{"new": post(fresh), "pending": post(fresh), "subscribed": post(subscribed), "unsubscribed": post(unsubscribed), "suppressed": post(suppressed)}
	for k, v := range answers {
		if v != answers["new"] {
			t.Fatalf("%s address answered %s, a new one %s", k, v, answers["new"])
		}
	}
	honeypot := lt.subscribeBody(unique("bot")+"@example.org", token)
	honeypot["website"] = "https://spam.example"
	if got := string(c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", honeypot).raw); got != answers["new"] {
		t.Fatalf("honeypot answered %s", got)
	}
	if lt.subscription(t, honeypot["email"].(string)) != nil {
		t.Fatal("the honeypot subscribed someone")
	}
	if sub := lt.subscription(t, unsubscribed); sub["state"] != "pending" {
		t.Fatalf("an unsubscribed address asking again: %v", sub["state"])
	}
	if lt.subscription(t, suppressed) != nil {
		t.Fatal("a suppressed address was subscribed")
	}
	c.expectProblem(http.StatusBadRequest, "invalid_email", "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody("Name <x@example.org>", token))
	page := lt.subscribeBody(fresh, token)
	page["page_url"] = "https://elsewhere.example/news"
	c.expectProblem(http.StatusBadRequest, "page_origin", "POST", "/client/v1/channels/"+lt.key+"/subscriptions", page)
}

func TestMailingListAccessAndScopes(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	other := lt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Hidden", "slug": "hidden"})
	hidden := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists", map[string]any{"inbox_id": other.body["inbox"].(map[string]any)["id"], "name": "Hidden list"})
	lt.owner.expectProblem(http.StatusConflict, "name_taken", "POST", "/v1/lists", map[string]any{"inbox_id": lt.chatInbox, "name": "Product news"})
	lt.owner.expectProblem(http.StatusConflict, "email_channel_required", "PATCH", "/v1/lists/"+lt.list, map[string]any{"channel_id": lt.channel})

	lt.agent.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list, nil)
	lt.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/lists/"+hidden.str("id"), nil)
	if got := items(lt.agent.expect(http.StatusOK, "GET", "/v1/lists", nil)); len(got) != 1 {
		t.Fatalf("agent sees %d lists", len(got))
	}
	lt.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": "a@example.org", "consent": map[string]any{"confirmed": true}})
	lt.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/lists/"+lt.list, map[string]any{"name": "Mine"})

	reader := h.client()
	reader.bearer = lt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "reader", "scopes": []string{"lists:read"}}).str("secret")
	reader.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/subscriptions", nil)
	r := reader.do("POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": "a@example.org", "consent": map[string]any{"confirmed": true}})
	if r.status != http.StatusForbidden || r.str("code") != "insufficient_scope" || r.str("scope") != "lists:write" {
		t.Fatalf("lists:read key adding: %d %s", r.status, r.raw)
	}
	old := h.client()
	old.bearer = lt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "old", "scopes": []string{"contacts:read", "conversations:read"}}).str("secret")
	old.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/lists", nil)
	lt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "b", "scopes": []string{"broadcasts:send"}})
	writer := h.client()
	writer.bearer = lt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "writer", "scopes": []string{"lists:write", "lists:read"}}).str("secret")
	added := writer.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{
		"email": "Host.User@Example.org", "name": "Host User", "external_id": "u-1",
		"consent": map[string]any{"confirmed": true, "text": "Signed up at checkout", "reference": "order-7"},
	})
	if added.str("state") != "subscribed" || added.str("source") != "api" || added.str("email") != "host.user@example.org" {
		t.Fatalf("key added %s", added.raw)
	}
	writer.expect(http.StatusOK, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": "host.user@example.org", "consent": map[string]any{"confirmed": true}})
	writer.expectProblem(http.StatusConflict, "confirmation_required", "PATCH", "/v1/lists/"+lt.list+"/subscriptions/"+added.str("id"), map[string]any{"state": "subscribed"})
	writer.expect(http.StatusOK, "PATCH", "/v1/lists/"+lt.list+"/subscriptions/"+added.str("id"), map[string]any{"state": "unsubscribed"})
	writer.expectProblem(http.StatusConflict, "unsubscribed", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": "host.user@example.org", "consent": map[string]any{"confirmed": true}})

	foreign := newTeam(t, h)
	fk := h.client()
	fk.bearer = foreign.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "f"}).str("secret")
	for _, path := range []string{"/v1/lists/" + lt.list, "/v1/lists/" + lt.list + "/subscriptions", "/v1/lists/" + lt.list + "/subscriptions/" + added.str("id"), "/v1/lists/" + lt.list + "/growth"} {
		fk.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
		foreign.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path, nil)
	}
	fk.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": "x@example.org", "consent": map[string]any{"confirmed": true}})

	export := lt.owner.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/consent-export?format=csv", nil)
	if !strings.Contains(string(export.raw), `"host.user@example.org"`) || !strings.HasPrefix(string(export.raw), "list_id,") {
		t.Fatalf("csv export %s", export.raw)
	}
	growth := lt.owner.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/growth", nil)
	if days := growth.body["days"].([]any); len(days) != 30 || days[29].(map[string]any)["new"].(float64) < 1 {
		t.Fatalf("growth %s", growth.raw)
	}
}

func TestOptOutsOutliveTheContact(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	addr := unique("objector") + "@example.org"
	s := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": addr, "consent": map[string]any{"confirmed": true}})
	welcome := h.runListMail(lt.ws)
	if len(welcome) != 1 {
		t.Fatalf("%d welcome mails", len(welcome))
	}
	manage := linkIn(t, welcome[0], "m")
	if status, page := pageAt(t, h, "POST", manage, url.Values{"action": {"all"}}); status != http.StatusOK || !strings.Contains(page, "no more list mail") {
		t.Fatalf("unsubscribe all: %d %s", status, page)
	}
	lt.owner.expectProblem(http.StatusConflict, "opted_out", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": addr, "consent": map[string]any{"confirmed": false}})

	contact := s.body["contact"].(map[string]any)["id"].(string)
	lt.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+contact, nil)
	if status, _ := pageAt(t, h, "GET", manage, nil); status != http.StatusNotFound {
		t.Fatalf("/m/ after deleting the contact: %d", status)
	}
	lt.owner.expectProblem(http.StatusConflict, "opted_out", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": addr, "consent": map[string]any{"confirmed": true}})

	other := unique("leaver") + "@example.org"
	o := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": other, "consent": map[string]any{"confirmed": true}})
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+lt.list+"/subscriptions/"+o.str("id"), map[string]any{"state": "unsubscribed"})
	lt.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+o.body["contact"].(map[string]any)["id"].(string), nil)
	lt.owner.expectProblem(http.StatusConflict, "opted_out", "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": other, "consent": map[string]any{"confirmed": true}})
	var hashes int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM email_optouts WHERE workspace_id = $1", lt.ws).Scan(&hashes); err != nil {
		t.Fatal(err)
	}
	var plain int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM email_optouts WHERE workspace_id = $1 AND position($2::bytea in email_hash) > 0", lt.ws, []byte(addr)).Scan(&plain); err != nil {
		t.Fatal(err)
	}
	if hashes != 2 || plain != 0 {
		t.Fatalf("opt-outs: %d rows, %d holding the address", hashes, plain)
	}
}

func TestIdentitySubscriptions(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	anon := lt.session(h, map[string]any{})
	anon.expectProblem(http.StatusForbidden, "identity_required", "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Yes"})

	verified := unique("member") + "@example.org"
	cs := lt.session(h, map[string]any{"identity_token": lt.token(h, map[string]any{"sub": unique("u"), "email": verified, "email_verified": true})})
	listed := cs.expect(http.StatusOK, "GET", "/client/v1/subscriptions", nil)
	if it := listed.body["items"].([]any); len(it) != 1 || it[0].(map[string]any)["state"] != nil {
		t.Fatalf("before: %s", listed.raw)
	}
	if r := cs.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Yes, product news"}); r.str("state") != "subscribed" {
		t.Fatalf("verified address: %s", r.raw)
	}
	cs.expect(http.StatusNoContent, "DELETE", "/client/v1/subscriptions/"+lt.list, nil)
	if sub := lt.subscription(t, verified); sub["state"] != "unsubscribed" || sub["unsubscribe_reason"] != "client" {
		t.Fatalf("after DELETE: %v", sub)
	}
	if r := cs.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Again"}); r.str("state") != "pending" {
		t.Fatalf("subscribing again after leaving: %s", r.raw)
	}

	optedOut := unique("objector") + "@example.org"
	lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{"email": optedOut, "consent": map[string]any{"confirmed": true}})
	var manage string
	for _, env := range h.runListMail(lt.ws) {
		if env.GetHeader("To") == "<"+optedOut+">" || env.GetHeader("To") == optedOut {
			manage = linkIn(t, env, "m")
		}
	}
	pageAt(t, h, "POST", manage, url.Values{"action": {"all"}})
	oc := lt.session(h, map[string]any{"identity_token": lt.token(h, map[string]any{"sub": unique("u"), "email": optedOut, "email_verified": true})})
	if r := oc.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Yes"}); r.str("state") != "pending" {
		t.Fatalf("opted-out address through an identity token: %s", r.raw)
	}
}

func contactEmailOwners(t *testing.T, h *harness, ws, addr string) int {
	t.Helper()
	var n int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = $2", ws, addr).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestConfirmingAListDoesNotGiveAnIdentifiedContactTheAddress(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	victim := unique("victim") + "@example.org"
	cs := lt.session(h, map[string]any{"identity_token": lt.token(h, map[string]any{"sub": unique("u"), "email": victim})})
	if r := cs.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Yes"}); r.str("state") != "pending" {
		t.Fatalf("an unverified address: %s", r.raw)
	}
	var confirm string
	for _, env := range h.runListMail(lt.ws) {
		if strings.Contains(env.GetHeader("To"), victim) {
			confirm = linkIn(t, env, "s")
		}
	}
	if confirm == "" {
		t.Fatal("no confirmation mail to the address")
	}
	if status, page := pageAt(t, h, "POST", confirm, url.Values{"action": {"confirm"}}); status != http.StatusOK || !strings.Contains(page, "You are subscribed") {
		t.Fatalf("POST /s/: %d %s", status, page)
	}
	if sub := lt.subscription(t, victim); sub["state"] != "subscribed" {
		t.Fatalf("confirmed: %v", sub)
	}
	if n := contactEmailOwners(t, h, lt.ws, victim); n != 0 {
		t.Fatalf("a list confirmation gave the identified contact an address it never proved: %d owners", n)
	}
}

func TestSubscriptionWebhooks(t *testing.T) {
	h := privateHarness(t)
	lt := newListTeam(t, h)
	rec := newReceiver(t)
	lt.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rec.srv.URL, "events": []string{"subscription.created", "subscription.updated"}, "inbox_id": lt.chatInbox,
	})
	s := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists/"+lt.list+"/subscriptions", map[string]any{
		"email": unique("hook") + "@example.org", "external_id": "u-9", "consent": map[string]any{"confirmed": true},
	})
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/lists/"+lt.list+"/subscriptions/"+s.str("id"), map[string]any{"state": "unsubscribed"})
	h.runWebhooks(lt.ws)
	hooks := rec.take()
	if len(hooks) != 2 || hooks[0].typ != "subscription.created" || hooks[1].typ != "subscription.updated" {
		t.Fatalf("webhooks %v", hooks)
	}
	updated := hooks[1].data
	if updated["previous_state"] != "subscribed" || updated["subscription"].(map[string]any)["unsubscribe_reason"] != "member" ||
		updated["list"].(map[string]any)["id"] != lt.list || len(updated["contact"].(map[string]any)["external_ids"].([]any)) != 1 {
		t.Fatalf("subscription.updated data %v", updated)
	}
}

func TestOnlyThePersonSubscribesAgain(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	subs := "/v1/lists/" + lt.list + "/subscriptions"
	key := h.client()
	key.bearer = lt.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "host", "scopes": []string{"lists:write", "lists:read"}}).str("secret")
	add := func(addr string, confirmed bool) map[string]any {
		return map[string]any{"email": addr, "consent": map[string]any{"confirmed": confirmed}}
	}
	state := func(addr string) any {
		t.Helper()
		return lt.subscription(t, addr)["state"]
	}
	refused := func(addr string) {
		t.Helper()
		for _, c := range []*client{lt.owner, key} {
			c.expectProblem(http.StatusConflict, "unsubscribed", "POST", subs, add(addr, true))
		}
	}

	addr := unique("leaver") + "@example.org"
	id := lt.owner.expect(http.StatusCreated, "POST", subs, add(addr, true)).str("id")
	lt.owner.expect(http.StatusOK, "PATCH", subs+"/"+id, map[string]any{"state": "unsubscribed"})
	h.runListMail(lt.ws)

	refused(addr)
	for _, c := range []*client{lt.owner, key} {
		c.expectProblem(http.StatusConflict, "unsubscribed", "POST", subs, add(addr, false))
		c.expectProblem(http.StatusConflict, "confirmation_required", "PATCH", subs+"/"+id, map[string]any{"state": "subscribed"})
		c.expectProblem(http.StatusConflict, "confirmation_required", "PATCH", subs+"/"+id, map[string]any{"state": "pending"})
		c.expectProblem(http.StatusConflict, "not_pending", "POST", subs+"/"+id+"/confirmation", nil)
	}
	if got := state(addr); got != "unsubscribed" {
		t.Fatalf("after members and keys tried: %v", got)
	}
	if mails := h.runListMail(lt.ws); len(mails) != 0 {
		t.Fatalf("a member or key mailed an unsubscribed address %d times", len(mails))
	}

	c, token := lt.form(h)
	h.clock.Advance(3 * time.Second)
	c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(addr, token))
	if got := state(addr); got != "pending" {
		t.Fatalf("after asking on the form: %v", got)
	}
	refused(addr)
	if r := lt.owner.expect(http.StatusOK, "POST", subs, add(addr, false)); r.str("state") != "pending" {
		t.Fatalf("member adding a pending address: %s", r.raw)
	}
	mails := h.runListMail(lt.ws)
	if len(mails) != 1 {
		t.Fatalf("confirmation mails after the form: %d", len(mails))
	}
	if got := state(addr); got != "pending" {
		t.Fatalf("after members and keys tried again: %v", got)
	}
	if status, page := pageAt(t, h, "POST", linkIn(t, mails[0], "s"), url.Values{"action": {"confirm"}}); status != http.StatusOK || !strings.Contains(page, "You are subscribed") {
		t.Fatalf("POST /s/: %d %s", status, page)
	}
	if got := state(addr); got != "subscribed" {
		t.Fatalf("after the person confirmed: %v", got)
	}
	if r := key.expect(http.StatusOK, "POST", subs, add(addr, true)); r.str("state") != "subscribed" {
		t.Fatalf("key adding a confirmed address: %s", r.raw)
	}

	fresh := unique("fresh") + "@example.org"
	if r := lt.owner.expect(http.StatusCreated, "POST", subs, add(fresh, false)); r.str("state") != "pending" {
		t.Fatalf("member asking a new address to confirm: %s", r.raw)
	}
	if r := key.expect(http.StatusOK, "POST", subs, add(fresh, true)); r.str("state") != "subscribed" {
		t.Fatalf("key confirming a pending address that never left: %s", r.raw)
	}

	erased := unique("erased") + "@example.org"
	eid := lt.owner.expect(http.StatusCreated, "POST", subs, add(erased, true)).str("id")
	lt.owner.expect(http.StatusOK, "PATCH", subs+"/"+eid, map[string]any{"state": "unsubscribed"})
	h.clock.Advance(3 * time.Second)
	c.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(erased, token))
	lt.owner.expect(http.StatusNoContent, "DELETE", subs+"/"+eid, nil)
	lt.owner.expectProblem(http.StatusConflict, "opted_out", "POST", subs, add(erased, true))

	verified := unique("app") + "@example.org"
	cs := lt.session(h, map[string]any{"identity_token": lt.token(h, map[string]any{"sub": unique("u"), "email": verified, "email_verified": true})})
	cs.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Yes"})
	cs.expect(http.StatusNoContent, "DELETE", "/client/v1/subscriptions/"+lt.list, nil)
	refused(verified)
	if r := cs.expect(http.StatusOK, "PUT", "/client/v1/subscriptions/"+lt.list, map[string]any{"consent_text": "Again"}); r.str("state") != "pending" {
		t.Fatalf("identity asking again: %s", r.raw)
	}
	refused(verified)
	if got := state(verified); got != "pending" {
		t.Fatalf("after the app asked again: %v", got)
	}
}

func TestSubscriptionConsentRequired(t *testing.T) {
	h := newHarness(t)
	lt := newListTeam(t, h)
	subs := "/v1/lists/" + lt.list + "/subscriptions"
	addr := unique("silent") + "@example.org"
	for body, field := range map[string]string{
		`{"email":"` + addr + `"}`:                        "consent is required",
		`{"email":"` + addr + `","consent":null}`:         "consent is required",
		`{"email":"` + addr + `","consent":{}}`:           "consent.confirmed is required",
		`{"email":"` + addr + `","consent":{"text":"x"}}`: "consent.confirmed is required",
	} {
		r := lt.owner.do("POST", subs, json.RawMessage(body))
		if r.status != http.StatusBadRequest || r.str("code") != "validation_failed" || !strings.Contains(r.str("detail"), field) {
			t.Fatalf("%s: %d %s", body, r.status, r.raw)
		}
	}
	if lt.subscription(t, addr) != nil {
		t.Fatal("a subscription without consent was created")
	}
	if mails := h.runListMail(lt.ws); len(mails) != 0 {
		t.Fatalf("%d mails sent without consent", len(mails))
	}
}

package api_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/push"
)

var b64u = base64.RawURLEncoding

const testVAPIDSubject = "mailto:push-test@example.com"

// browser stands in for a browser's push subscription: its key pair and auth secret.
type browser struct {
	priv *ecdh.PrivateKey
	auth []byte
	path string
	id   string
}

func newBrowser(t *testing.T) *browser {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return &browser{priv: priv, auth: auth, path: "/push/" + unique("sub")}
}

func (b *browser) keys() map[string]any {
	return map[string]any{"p256dh": b64u.EncodeToString(b.priv.PublicKey().Bytes()), "auth": b64u.EncodeToString(b.auth)}
}

// decrypt opens an aes128gcm body per RFC 8291 with the browser's keys.
func (b *browser) decrypt(body []byte) ([]byte, error) {
	if len(body) < 21 {
		return nil, fmt.Errorf("body too short")
	}
	salt, rs, idLen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	if rs < 18 || len(body) < 21+idLen {
		return nil, fmt.Errorf("bad header")
	}
	asPub, ciphertext := body[21:21+idLen], body[21+idLen:]
	sender, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		return nil, err
	}
	shared, err := b.priv.ECDH(sender)
	if err != nil {
		return nil, err
	}
	info := "WebPush: info\x00" + string(b.priv.PublicKey().Bytes()) + string(asPub)
	ikm, err := hkdf.Key(sha256.New, shared, b.auth, info, 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	plain = bytes.TrimRight(plain, "\x00")
	if len(plain) == 0 || plain[len(plain)-1] != 0x02 {
		return nil, fmt.Errorf("no final record delimiter")
	}
	return plain[:len(plain)-1], nil
}

type pushDelivery struct {
	path    string
	header  http.Header
	payload map[string]any
}

// fakePushService implements the push service side of RFC 8030: it checks the VAPID signature,
// decrypts the message with the subscription's keys and answers with a set status.
type fakePushService struct {
	t        *testing.T
	mu       sync.Mutex
	srv      *httptest.Server
	vapid    string
	browsers map[string]*browser
	status   map[string]int
	got      []pushDelivery
}

func newFakePushService(t *testing.T, vapidPublic string) *fakePushService {
	f := &fakePushService{t: t, vapid: vapidPublic, browsers: map[string]*browser{}, status: map[string]int{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePushService) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.browsers[r.URL.Path]
	if b == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := f.checkVAPID(r); err != nil {
		f.t.Errorf("vapid: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.Header.Get("Content-Encoding") != "aes128gcm" {
		f.t.Errorf("content encoding %q", r.Header.Get("Content-Encoding"))
	}
	plain, err := b.decrypt(body)
	if err != nil {
		f.t.Errorf("decrypt: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		f.t.Errorf("payload %q: %v", plain, err)
	}
	f.got = append(f.got, pushDelivery{path: r.URL.Path, header: r.Header.Clone(), payload: payload})
	if s := f.status[r.URL.Path]; s != 0 {
		w.WriteHeader(s)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (f *fakePushService) checkVAPID(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	rest, ok := strings.CutPrefix(auth, "vapid t=")
	if !ok {
		return fmt.Errorf("authorization %q", auth)
	}
	token, key, ok := strings.Cut(rest, ", k=")
	if !ok || key != f.vapid {
		return fmt.Errorf("key %q, want %q", key, f.vapid)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("token %q", token)
	}
	pubBytes, err := b64u.DecodeString(key)
	if err != nil {
		return err
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pubBytes)
	if err != nil {
		return err
	}
	sig, err := b64u.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return fmt.Errorf("signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return fmt.Errorf("signature does not verify")
	}
	claimsJSON, err := b64u.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var claims struct {
		Aud string `json:"aud"`
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return err
	}
	if claims.Aud != "https://"+r.Host || claims.Sub != testVAPIDSubject || claims.Exp < time.Now().Unix() {
		return fmt.Errorf("claims %s", claimsJSON)
	}
	return nil
}

func (f *fakePushService) add(b *browser) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.browsers[b.path] = b
}

func (f *fakePushService) setStatus(b *browser, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[b.path] = status
}

func (f *fakePushService) take() []pushDelivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.got
	f.got = nil
	return out
}

func (f *fakePushService) to(got []pushDelivery, b *browser) []pushDelivery {
	var out []pushDelivery
	for _, d := range got {
		if d.path == b.path {
			out = append(out, d)
		}
	}
	return out
}

func testVAPIDKeys(t *testing.T) push.Keys {
	t.Helper()
	public, private, err := push.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}
	return push.Keys{PublicKey: public, PrivateKey: private, Subject: testVAPIDSubject}
}

func pushHarness(t *testing.T) (*harness, *fakePushService) {
	t.Helper()
	keys := testVAPIDKeys(t)
	fake := newFakePushService(t, keys.PublicKey)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.AllowPrivate = true
		d.Push = api.PushSettings{Keys: keys, HTTPClient: fake.srv.Client()}
	})
	return h, fake
}

func (f *fakePushService) subscribe(c *client) *browser {
	c.h.t.Helper()
	b := newBrowser(c.h.t)
	f.add(b)
	r := c.expect(http.StatusCreated, "POST", "/v1/me/push-subscriptions", map[string]any{
		"endpoint": f.srv.URL + b.path, "keys": b.keys(), "user_agent": "Test browser",
	})
	b.id = r.str("id")
	return b
}

// runNotifications runs the queued notification fan-outs and one attempt of each queued push, as
// the job queue would.
func (h *harness) runNotifications(ws string) {
	h.t.Helper()
	ctx := context.Background()
	for _, j := range h.jobs("notify", ws) {
		var a api.NotifyArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.Notify(ctx, a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	h.runPushes(ws)
}

func (h *harness) runPushes(ws string) (retries int) {
	h.t.Helper()
	for _, j := range h.jobs("push_send", ws) {
		var a api.PushArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		retry, err := h.srv.SendPush(context.Background(), a)
		if err != nil {
			h.t.Fatal(err)
		}
		if retry {
			retries++
		}
		h.dropJob(j.id)
	}
	return retries
}

type notifyTeam struct {
	team
	ownerID string
	key     *client
	live    string
	async   string
}

func newNotifyTeam(t *testing.T, h *harness) notifyTeam {
	t.Helper()
	tm := newTeam(t, h)
	me := tm.owner.expect(http.StatusOK, "GET", "/v1/me", nil)
	ownerID := me.body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
	live := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Live", "slug": "live", "mode": "live"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+live+"/members/"+tm.agentID, nil)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	return notifyTeam{team: tm, ownerID: ownerID, key: tm.apiKey(h), live: live, async: tm.inbox}
}

func (nt notifyTeam) open(h *harness, inbox, text string) string {
	h.t.Helper()
	conv := nt.key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox, "contact_id": nt.contact}).str("id")
	nt.write(h, conv, text)
	return conv
}

func (nt notifyTeam) write(h *harness, conv, text string) {
	h.t.Helper()
	post(nt.key, conv, map[string]any{"kind": "message", "direction": "in", "body": text})
}

func events(ds []pushDelivery) []string {
	var out []string
	for _, d := range ds {
		e, _ := d.payload["event"].(string)
		out = append(out, e)
	}
	return out
}

func expectPushes(t *testing.T, got []pushDelivery, b *browser, want ...string) []pushDelivery {
	t.Helper()
	var mine []pushDelivery
	for _, d := range got {
		if d.path == b.path {
			mine = append(mine, d)
		}
	}
	if fmt.Sprint(events(mine)) != fmt.Sprint(want) {
		t.Fatalf("pushes %v, want %v", events(mine), want)
	}
	return mine
}

func TestNotificationRuleMatrix(t *testing.T) {
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	owner, agent := fake.subscribe(nt.owner), fake.subscribe(nt.agent)

	liveConv := nt.open(h, nt.live, "Hello,\n  my order   is late")
	h.runNotifications(nt.ws)
	got := fake.take()
	d := expectPushes(t, got, owner, "new_live_conversation")[0]
	expectPushes(t, got, agent, "new_live_conversation")
	p := d.payload
	if p["title"] != "Live · Ayşe Yılmaz" || p["body"] != "Hello, my order is late" || p["tag"] != "conversation-"+liveConv ||
		p["url"] != "/conversations/"+liveConv+"?workspace_id="+nt.ws || p["conversation_id"] != liveConv ||
		p["workspace_id"] != nt.ws || p["inbox_id"] != nt.live {
		t.Fatalf("payload %v", p)
	}
	if d.header.Get("Urgency") != "high" || d.header.Get("TTL") != "14400" || d.header.Get("Topic") != strings.ReplaceAll(liveConv, "-", "") {
		t.Fatalf("headers %v", d.header)
	}

	asyncConv := nt.open(h, nt.async, strings.Repeat("long text ", 40))
	h.runNotifications(nt.ws)
	got = fake.take()
	d = expectPushes(t, got, owner, "new_async_conversation")[0]
	expectPushes(t, got, agent)
	if body := d.payload["body"].(string); len([]rune(body)) > 141 || !strings.HasSuffix(body, "…") || !strings.HasPrefix(body, "long text long") {
		t.Fatalf("preview %q", body)
	}
	if d.header.Get("Urgency") != "normal" || d.header.Get("TTL") != "86400" {
		t.Fatalf("headers %v", d.header)
	}

	nt.write(h, liveConv, "anyone there?")
	h.runNotifications(nt.ws)
	got = fake.take()
	expectPushes(t, got, owner, "message_in_unassigned_conversation")
	expectPushes(t, got, agent)

	nt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+liveConv, map[string]any{"assignee_id": nt.agentID})
	h.runNotifications(nt.ws)
	got = fake.take()
	expectPushes(t, got, owner)
	d = expectPushes(t, got, agent, "assigned_to_me")[0]
	if !strings.HasSuffix(d.payload["body"].(string), "assigned this conversation to you") {
		t.Fatalf("assignment body %v", d.payload)
	}

	nt.write(h, liveConv, "thanks")
	h.runNotifications(nt.ws)
	got = fake.take()
	expectPushes(t, got, owner)
	expectPushes(t, got, agent, "message_in_my_conversation")

	nt.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{
		"events": map[string]any{"message_in_unassigned_conversation": map[string]any{"push": true, "email": false}},
	})
	nt.owner.expect(http.StatusOK, "PUT", "/v1/me/notifications/inboxes/"+nt.async, map[string]any{
		"events": map[string]any{"new_async_conversation": map[string]any{"push": false, "email": false}},
	})
	nt.write(h, asyncConv, "more detail")
	nt.open(h, nt.async, "second async")
	h.runNotifications(nt.ws)
	got = fake.take()
	expectPushes(t, got, owner, "message_in_unassigned_conversation")
	expectPushes(t, got, agent, "message_in_unassigned_conversation")

	nt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+asyncConv, map[string]any{"spam": true})
	nt.write(h, asyncConv, "spam again")
	h.runNotifications(nt.ws)
	if got := fake.take(); len(got) != 0 {
		t.Fatalf("spam conversation notified: %v", events(got))
	}

	emails := h.jobs("notification_email", nt.ws)
	if len(emails) != 1 {
		t.Fatalf("e-mail checks %d, want one for the agent's conversation", len(emails))
	}
	var ea api.NotificationEmailArgs
	_ = json.Unmarshal(emails[0].args, &ea)
	if ea.MemberID.String() != nt.agentID || ea.ConversationID.String() != liveConv {
		t.Fatalf("e-mail check %+v", ea)
	}
}

func TestNotificationSettingsAPI(t *testing.T) {
	h, _ := pushHarness(t)
	nt := newNotifyTeam(t, h)
	other := newNotifyTeam(t, h)

	r := nt.agent.expect(http.StatusOK, "GET", "/v1/me/notifications", nil)
	defaults := r.body["defaults"].(map[string]any)
	if defaults["new_async_conversation"].(map[string]any)["push"] != false || defaults["assigned_to_me"].(map[string]any)["email"] != true ||
		r.body["email_delay_minutes"] != float64(15) || len(r.body["inboxes"].([]any)) != 0 {
		t.Fatalf("agent settings %s", r.raw)
	}
	r = nt.owner.expect(http.StatusOK, "GET", "/v1/me/notifications", nil)
	if r.body["defaults"].(map[string]any)["new_async_conversation"].(map[string]any)["push"] != true {
		t.Fatalf("owner defaults %s", r.raw)
	}
	nt.agent.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/me/notifications", map[string]any{"email_delay_minutes": 2})
	r = nt.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{
		"email_delay_minutes": 30, "events": map[string]any{"new_live_conversation": map[string]any{"push": false, "email": true}},
	})
	ev := r.body["events"].(map[string]any)
	if r.body["email_delay_minutes"] != float64(30) || ev["new_live_conversation"].(map[string]any)["email"] != true ||
		ev["assigned_to_me"].(map[string]any)["push"] != true {
		t.Fatalf("updated %s", r.raw)
	}
	r = nt.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{"events": map[string]any{"assigned_to_me": map[string]any{"push": false, "email": false}}})
	if r.body["events"].(map[string]any)["new_live_conversation"].(map[string]any)["email"] != true {
		t.Fatalf("an event not sent changed: %s", r.raw)
	}

	hidden := nt.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Hidden", "slug": "hidden"}).body["inbox"].(map[string]any)["id"].(string)
	body := map[string]any{"events": map[string]any{"new_async_conversation": map[string]any{"push": true, "email": false}}}
	nt.agent.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/me/notifications/inboxes/"+hidden, body)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/me/notifications/inboxes/"+nt.live, body)
	nt.agent.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/me/notifications/inboxes/"+nt.live, map[string]any{"events": map[string]any{}})
	o := nt.agent.expect(http.StatusOK, "PUT", "/v1/me/notifications/inboxes/"+nt.live, body)
	if o.str("inbox_id") != nt.live || o.body["events"].(map[string]any)["new_live_conversation"] != nil {
		t.Fatalf("override %s", o.raw)
	}
	r = nt.agent.expect(http.StatusOK, "GET", "/v1/me/notifications", nil)
	if len(r.body["inboxes"].([]any)) != 1 {
		t.Fatalf("overrides %s", r.raw)
	}
	nt.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+nt.live+"/members/"+nt.agentID, nil)
	r = nt.agent.expect(http.StatusOK, "GET", "/v1/me/notifications", nil)
	if len(r.body["inboxes"].([]any)) != 0 {
		t.Fatalf("an override of an inbox the agent lost: %s", r.raw)
	}
	nt.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/notifications/inboxes/"+nt.live, nil)
	nt.key.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/notifications", nil)
	if r := other.owner.expect(http.StatusOK, "GET", "/v1/me/notifications", nil); r.body["email_delay_minutes"] != float64(15) {
		t.Fatalf("another workspace's settings changed: %s", r.raw)
	}
}

func TestPushSubscriptionsAPI(t *testing.T) {
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	other := newNotifyTeam(t, h)

	r := nt.owner.expect(http.StatusOK, "GET", "/v1/push/vapid-public-key", nil)
	if r.str("public_key") != fake.vapid {
		t.Fatalf("vapid key %s", r.raw)
	}
	b := fake.subscribe(nt.owner)
	again := nt.owner.expect(http.StatusCreated, "POST", "/v1/me/push-subscriptions", map[string]any{
		"endpoint": fake.srv.URL + b.path, "keys": b.keys(), "user_agent": "Renamed",
	})
	if again.str("id") != b.id || again.str("user_agent") != "Renamed" || again.body["current"] != true {
		t.Fatalf("re-registering did not update: %s", again.raw)
	}
	list := nt.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["endpoint"] != fake.srv.URL+b.path {
		t.Fatalf("list %v", list)
	}
	for _, bad := range []map[string]any{
		{"endpoint": fake.srv.URL + "/x", "keys": map[string]any{"p256dh": "AAAA", "auth": b.keys()["auth"]}},
		{"endpoint": fake.srv.URL + "/x", "keys": map[string]any{"p256dh": b.keys()["p256dh"], "auth": "AAAA"}},
		{"endpoint": strings.Replace(fake.srv.URL, "https:", "http:", 1) + "/x", "keys": b.keys()},
		{"endpoint": "https://169.254.169.254/push", "keys": b.keys()},
	} {
		nt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/me/push-subscriptions", bad)
	}

	test := nt.owner.expect(http.StatusAccepted, "POST", "/v1/me/push-subscriptions/"+b.id+"/test", nil)
	if test.str("event") != "test" || test.str("title") != "Yuva" {
		t.Fatalf("test payload %s", test.raw)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/me/push-subscriptions/"+b.id+"/test", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/push-subscriptions/"+b.id, nil)
	rows, err := h.st.Pool.Query(context.Background(), "SELECT args FROM river_job WHERE kind = 'push_send' AND args->>'subscription_id' = $1", b.id)
	if err != nil {
		t.Fatal(err)
	}
	var tests []api.PushArgs
	for rows.Next() {
		var raw []byte
		_ = rows.Scan(&raw)
		var a api.PushArgs
		_ = json.Unmarshal(raw, &a)
		tests = append(tests, a)
	}
	rows.Close()
	if len(tests) != 1 {
		t.Fatalf("test pushes queued: %d", len(tests))
	}
	if retry, err := h.srv.SendPush(context.Background(), tests[0]); err != nil || retry {
		t.Fatalf("test push: %v %v", retry, err)
	}
	_, _ = h.st.Pool.Exec(context.Background(), "DELETE FROM river_job WHERE kind = 'push_send' AND args->>'subscription_id' = $1", b.id)
	if got := fake.take(); len(got) != 1 || got[0].payload["body"] != "Notifications work on this device." {
		t.Fatalf("test push delivered %v", got)
	}
	list = nt.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any)
	if list[0].(map[string]any)["last_success_at"] == nil {
		t.Fatalf("no last_success_at: %v", list)
	}

	email := nt.ownerEmail(h)
	nt.owner.expect(http.StatusNoContent, "POST", "/v1/auth/sign-out", nil)
	nt.owner.signIn(email)
	if list := nt.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(list) != 0 {
		t.Fatalf("sign-out kept the subscription: %v", list)
	}

	b2 := fake.subscribe(nt.owner)
	nt.owner.expect(http.StatusNoContent, "DELETE", "/v1/me/push-subscriptions/"+b2.id, nil)
	nt.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/me/push-subscriptions/"+b2.id, nil)

	off := newHarness(t)
	tm := newTeam(t, off)
	tm.owner.expectProblem(http.StatusNotFound, "push_disabled", "GET", "/v1/push/vapid-public-key", nil)
	tm.owner.expectProblem(http.StatusNotFound, "push_disabled", "POST", "/v1/me/push-subscriptions", map[string]any{"endpoint": fake.srv.URL + "/x", "keys": b.keys()})
}

func (nt notifyTeam) ownerEmail(h *harness) string {
	h.t.Helper()
	return nt.owner.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
}

func (w *wsClient) send(v any) {
	w.t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.conn.Write(ctx, websocket.MessageText, b); err != nil {
		w.t.Fatal(err)
	}
}

func (h *harness) waitViewing(memberID string, conv any) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got *string
		err := h.st.Pool.QueryRow(context.Background(),
			"SELECT viewing_conversation_id::text FROM realtime_connections WHERE member_id = $1 ORDER BY seen_at DESC LIMIT 1", memberID).Scan(&got)
		if err == nil && ((conv == nil && got == nil) || (got != nil && conv == *got)) {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("viewing is %v, want %v (%v)", got, conv, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNotificationsQuietWhileViewing(t *testing.T) {
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	owner, agent := fake.subscribe(nt.owner), fake.subscribe(nt.agent)
	conv := nt.open(h, nt.live, "first")
	h.runNotifications(nt.ws)
	fake.take()

	ws := nt.agent.dial("")
	ws.ready()
	ws.send(map[string]any{"type": "unknown"})
	ws.send(map[string]any{"type": "viewing", "conversation_id": conv})
	h.waitViewing(nt.agentID, conv)
	nt.agent.expect(http.StatusOK, "PATCH", "/v1/me/notifications", map[string]any{
		"events": map[string]any{"message_in_unassigned_conversation": map[string]any{"push": true, "email": true}},
	})
	nt.write(h, conv, "while the agent looks")
	h.runNotifications(nt.ws)
	got := fake.take()
	expectPushes(t, got, agent)
	expectPushes(t, got, owner, "message_in_unassigned_conversation")
	if n := len(h.jobs("notification_email", nt.ws)); n != 0 {
		t.Fatalf("e-mail scheduled while viewing: %d", n)
	}

	ws.send(map[string]any{"type": "viewing", "conversation_id": nil})
	h.waitViewing(nt.agentID, nil)
	nt.write(h, conv, "after the agent left")
	h.runNotifications(nt.ws)
	expectPushes(t, fake.take(), agent, "message_in_unassigned_conversation")

	ws.send(map[string]any{"type": "viewing", "conversation_id": conv})
	h.waitViewing(nt.agentID, conv)
	h.clock.Advance(2 * time.Minute)
	nt.write(h, conv, "the open tab went stale")
	h.runNotifications(nt.ws)
	expectPushes(t, fake.take(), agent, "message_in_unassigned_conversation")
}

func TestNotificationsNoSelfNotifyAndAway(t *testing.T) {
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	owner, agent := fake.subscribe(nt.owner), fake.subscribe(nt.agent)
	conv := nt.open(h, nt.live, "hi")
	h.runNotifications(nt.ws)
	fake.take()

	nt.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nt.agentID})
	nt.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "on it"})
	nt.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "hello"})
	h.runNotifications(nt.ws)
	if got := fake.take(); len(got) != 0 {
		t.Fatalf("self-assignment or a member's own message notified: %v", events(got))
	}
	nt.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nt.ownerID})
	h.runNotifications(nt.ws)
	got := fake.take()
	expectPushes(t, got, agent)
	expectPushes(t, got, owner, "assigned_to_me")

	nt.owner.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	nt.write(h, conv, "assigned to the away owner")
	other := nt.open(h, nt.live, "a new chat while the owner is away")
	h.runNotifications(nt.ws)
	got = fake.take()
	expectPushes(t, got, owner, "message_in_my_conversation")
	expectPushes(t, got, agent, "new_live_conversation")
	nt.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+other, map[string]any{"assignee_id": nt.ownerID})
	h.runNotifications(nt.ws)
	expectPushes(t, fake.take(), owner, "assigned_to_me")
}

func TestPushGoneAndRetry(t *testing.T) {
	h, fake := pushHarness(t)
	nt := newNotifyTeam(t, h)
	gone, missing, busy, ok := fake.subscribe(nt.owner), fake.subscribe(nt.owner), fake.subscribe(nt.owner), fake.subscribe(nt.owner)
	fake.setStatus(gone, http.StatusGone)
	fake.setStatus(busy, http.StatusServiceUnavailable)
	fake.mu.Lock()
	delete(fake.browsers, missing.path)
	fake.mu.Unlock()

	nt.open(h, nt.live, "hello")
	h.runNotifications(nt.ws)
	got := fake.take()
	if len(fake.to(got, ok)) != 1 || len(fake.to(got, busy)) != 1 {
		t.Fatalf("deliveries %v", got)
	}
	list := nt.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any)
	ids := map[string]map[string]any{}
	for _, it := range list {
		ids[it.(map[string]any)["id"].(string)] = it.(map[string]any)
	}
	if _, ok := ids[gone.id]; ok {
		t.Fatal("a 410 subscription was kept")
	}
	if _, ok := ids[missing.id]; ok {
		t.Fatal("a 404 subscription was kept")
	}
	if e, _ := ids[busy.id]["last_error"].(string); !strings.Contains(e, "503") {
		t.Fatalf("503 not recorded: %v", ids[busy.id])
	}
	if ids[ok.id]["last_success_at"] == nil {
		t.Fatalf("success not recorded: %v", ids[ok.id])
	}

	nt.open(h, nt.live, "x")
	for _, j := range h.jobs("notify", nt.ws) {
		var a api.NotifyArgs
		_ = json.Unmarshal(j.args, &a)
		if err := h.srv.Notify(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	if retries := h.runPushes(nt.ws); retries != 1 {
		t.Fatalf("retries %d, want 1 for the 503 subscription", retries)
	}
}

func TestPushEndpointSSRFRefused(t *testing.T) {
	keys := testVAPIDKeys(t)
	fake := newFakePushService(t, keys.PublicKey)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.Webhooks.Resolver = staticResolver{"push.example.com": "127.0.0.1", "fcm.example.com": "10.1.2.3"}
		d.Push = api.PushSettings{Keys: keys}
	})
	nt := newNotifyTeam(t, h)
	b := newBrowser(t)
	fake.add(b)
	port := fake.srv.URL[strings.LastIndex(fake.srv.URL, ":"):]
	for _, u := range []string{
		fake.srv.URL + b.path, "https://localhost" + port + b.path, "https://[::1]/push", "https://10.0.0.8/push",
		"https://169.254.169.254/push", "http://push.example.com/push", "ftp://push.example.com/push",
	} {
		nt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/me/push-subscriptions", map[string]any{"endpoint": u, "keys": b.keys()})
	}
	var ids []string
	for _, host := range []string{"push.example.com", "fcm.example.com"} {
		r := nt.owner.expect(http.StatusCreated, "POST", "/v1/me/push-subscriptions", map[string]any{
			"endpoint": "https://" + host + port + b.path, "keys": b.keys(),
		})
		ids = append(ids, r.str("id"))
	}
	nt.open(h, nt.live, "hello")
	h.runNotifications(nt.ws)
	if got := fake.take(); len(got) != 0 {
		t.Fatal("a push reached a private address")
	}
	if list := nt.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(list) != 0 {
		t.Fatalf("subscriptions on private addresses were kept: %v", list)
	}
}

func (h *harness) emailChecks(ws string) []api.NotificationEmailArgs {
	h.t.Helper()
	var out []api.NotificationEmailArgs
	for _, j := range h.jobs("notification_email", ws) {
		var a api.NotificationEmailArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, a)
		h.dropJob(j.id)
	}
	return out
}

func TestNotificationEmailDigest(t *testing.T) {
	h, _ := pushHarness(t)
	nt := newNotifyTeam(t, h)
	agentEmail := nt.agent.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
	nt.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"locale": "tr"})
	conv := nt.open(h, nt.live, "first question")
	h.runNotifications(nt.ws)
	if n := len(h.emailChecks(nt.ws)); n != 0 {
		t.Fatalf("e-mail checks for a new conversation by default: %d", n)
	}

	nt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nt.agentID})
	nt.write(h, conv, "are you there?")
	nt.write(h, conv, "hello??")
	h.runNotifications(nt.ws)
	checks := h.emailChecks(nt.ws)
	if len(checks) != 1 {
		t.Fatalf("e-mail checks %d, want one per member and conversation", len(checks))
	}
	check := checks[0]
	ctx := context.Background()
	before := len(h.mail.to(agentEmail))
	sent := func() []string {
		var out []string
		for _, m := range h.mail.to(agentEmail)[before:] {
			out = append(out, m.Subject+"\n"+m.Text)
		}
		return out
	}

	h.clock.Advance(10 * time.Minute)
	next, err := h.srv.SendNotificationEmail(ctx, check)
	if err != nil || next == nil || len(sent()) != 0 {
		t.Fatalf("sent before the delay: next %v err %v mails %d", next, err, len(sent()))
	}
	h.clock.Advance(next.Sub(h.clock.Now()))
	if next, err = h.srv.SendNotificationEmail(ctx, check); err != nil || next != nil {
		t.Fatalf("next %v err %v", next, err)
	}
	mails := sent()
	if len(mails) != 1 {
		t.Fatalf("mails %d", len(mails))
	}
	m := mails[0]
	for _, want := range []string{
		"Live: Ayşe Yılmaz yanıt bekliyor", "size Live gelen kutusunda bir konuşma atadı", "> are you there?", "> hello??",
		testOrigin + "/conversations/" + conv + "?workspace_id=" + nt.ws,
		testOrigin + "/settings/notifications?workspace_id=" + nt.ws, "15 dakika",
	} {
		if !strings.Contains(m, want) {
			t.Fatalf("mail lacks %q:\n%s", want, m)
		}
	}
	if !strings.Contains(m, "> first question") {
		t.Fatalf("mail lacks the first unread message:\n%s", m)
	}

	h.clock.Advance(5 * time.Minute)
	nt.write(h, conv, "still waiting")
	h.runNotifications(nt.ws)
	checks = h.emailChecks(nt.ws)
	if len(checks) != 1 {
		t.Fatalf("checks %d", len(checks))
	}
	h.clock.Advance(20 * time.Minute)
	next, err = h.srv.SendNotificationEmail(ctx, checks[0])
	if err != nil || next == nil || len(sent()) != 1 {
		t.Fatalf("a second e-mail within the hour: next %v err %v mails %d", next, err, len(sent()))
	}
	h.clock.Advance(next.Sub(h.clock.Now()))
	if _, err := h.srv.SendNotificationEmail(ctx, checks[0]); err != nil {
		t.Fatal(err)
	}
	mails = sent()
	if len(mails) != 2 || !strings.Contains(mails[1], "> still waiting") || strings.Contains(mails[1], "hello??") {
		t.Fatalf("second mail %v", mails)
	}

	nt.write(h, conv, "one more")
	h.runNotifications(nt.ws)
	checks = h.emailChecks(nt.ws)
	nt.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	h.clock.Advance(2 * time.Hour)
	if next, err := h.srv.SendNotificationEmail(ctx, checks[0]); err != nil || next != nil || len(sent()) != 2 {
		t.Fatalf("e-mailed a read conversation: next %v err %v mails %d", next, err, len(sent()))
	}
}

func TestNotificationEmailSkipsAnsweredMessages(t *testing.T) {
	h, _ := pushHarness(t)
	nt := newNotifyTeam(t, h)
	agentEmail := nt.agent.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
	conv := nt.open(h, nt.live, "first question")
	nt.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": nt.agentID})
	h.runNotifications(nt.ws)
	h.emailChecks(nt.ws)

	h.clock.Advance(time.Second)
	nt.write(h, conv, "answered already")
	h.runNotifications(nt.ws)
	checks := h.emailChecks(nt.ws)
	if len(checks) != 1 {
		t.Fatalf("checks %d", len(checks))
	}
	h.clock.Advance(time.Second)
	nt.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "here is the answer"})
	h.clock.Advance(time.Second)
	nt.write(h, conv, "a new question")
	h.runNotifications(nt.ws)
	h.emailChecks(nt.ws)

	before := len(h.mail.to(agentEmail))
	h.clock.Advance(time.Hour)
	if _, err := h.srv.SendNotificationEmail(context.Background(), checks[0]); err != nil {
		t.Fatal(err)
	}
	mails := h.mail.to(agentEmail)[before:]
	if len(mails) != 1 {
		t.Fatalf("mails %d", len(mails))
	}
	m := mails[0].Text
	if !strings.Contains(m, "> a new question") {
		t.Fatalf("mail lacks the message after the reply:\n%s", m)
	}
	for _, old := range []string{"first question", "answered already", "here is the answer"} {
		if strings.Contains(m, old) {
			t.Fatalf("mail quotes %q from before the member's reply:\n%s", old, m)
		}
	}
}

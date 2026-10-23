package api_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

type sentPush struct {
	service string
	token   string
	topic   string
	body    map[string]any
}

type fakeAppPush struct {
	t       *testing.T
	srv     *httptest.Server
	apnsKey *ecdsa.PublicKey
	fcmKey  *rsa.PublicKey
	mu      sync.Mutex
	sent    []sentPush
	tokens  int
	apnsOK  int
}

func jwtParts(t *testing.T, tok string) (signed string, claims map[string]any, sig []byte) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Errorf("not a JWT: %q", tok)
		return "", nil, nil
	}
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(raw, &claims)
	sig, _ = base64.RawURLEncoding.DecodeString(parts[2])
	return parts[0] + "." + parts[1], claims, sig
}

func newFakeAppPush(t *testing.T, apnsKey *ecdsa.PublicKey, fcmKey *rsa.PublicKey) *fakeAppPush {
	f := &fakeAppPush{t: t, apnsKey: apnsKey, fcmKey: fcmKey}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /3/device/{token}", func(w http.ResponseWriter, r *http.Request) {
		signed, claims, sig := jwtParts(t, strings.TrimPrefix(r.Header.Get("Authorization"), "bearer "))
		digest := sha256.Sum256([]byte(signed))
		if len(sig) != 64 || !ecdsa.Verify(f.apnsKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) || claims["iss"] != "TEAM123456" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"reason":"InvalidProviderToken"}`))
			return
		}
		tok := r.PathValue("token")
		switch tok {
		case "gone":
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
			return
		case "bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"reason":"BadDeviceToken"}`))
			return
		case "busy":
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.mu.Lock()
		f.apnsOK++
		f.mu.Unlock()
		f.record("apns", tok, r)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		signed, claims, sig := jwtParts(t, r.PostForm.Get("assertion"))
		digest := sha256.Sum256([]byte(signed))
		if rsa.VerifyPKCS1v15(f.fcmKey, crypto.SHA256, digest[:], sig) != nil || claims["iss"] != "push@yuva-test.iam.gserviceaccount.com" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		f.mu.Lock()
		f.tokens++
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"access_token":"fcm-access","expires_in":3600,"token_type":"Bearer"}`))
	})
	mux.HandleFunc("POST /v1/projects/yuva-test/messages:send", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fcm-access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var m struct {
			Message struct {
				Token string `json:"token"`
			} `json:"message"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &m)
		if m.Message.Token == "mismatch" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"SENDER_ID_MISMATCH"}]}}`))
			return
		}
		if m.Message.Token == "unregistered" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`))
			return
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.sent = append(f.sent, sentPush{service: "fcm", token: m.Message.Token, body: body})
		f.mu.Unlock()
	})
	f.srv = httptest.NewUnstartedServer(mux)
	f.srv.EnableHTTP2 = true
	f.srv.StartTLS()
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAppPush) record(service, token string, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentPush{service: service, token: token, topic: r.Header.Get("apns-topic"), body: body})
}

func (f *fakeAppPush) take() []sentPush {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

func testAppPushKeys(t *testing.T) (*ecdsa.PrivateKey, string, *rsa.PrivateKey, string) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(ec)
	p8 := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rder, _ := x509.MarshalPKCS8PrivateKey(rk)
	sa, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "yuva-test", "private_key_id": "k1",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rder})),
		"client_email": "push@yuva-test.iam.gserviceaccount.com", "token_uri": "https://attacker.example/token",
	})
	return ec, p8, rk, string(sa)
}

func (h *harness) runAppPushChecks(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("app_push_check", ws) {
		var a api.AppPushArgs
		_ = json.Unmarshal(j.args, &a)
		if _, err := h.srv.CheckAppPush(context.Background(), a.WorkspaceID, a.ConversationID); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (h *harness) runAppPushSends(ws string) (retries int) {
	h.t.Helper()
	for _, j := range h.jobs("app_push_send", ws) {
		var a api.AppPushSendArgs
		_ = json.Unmarshal(j.args, &a)
		retry, err := h.srv.SendAppPush(context.Background(), a)
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

func TestAppPush(t *testing.T) {
	ec, p8, rk, sa := testAppPushKeys(t)
	fake := newFakeAppPush(t, &ec.PublicKey, &rk.PublicKey)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.AppPush = api.AppPushSettings{APNsProduction: fake.srv.URL, APNsSandbox: fake.srv.URL, FCM: fake.srv.URL, TokenURL: fake.srv.URL + "/token", HTTPClient: fake.srv.Client()}
	})
	at := newAppTeam(t, h, false)
	path := "/v1/channels/" + at.channel

	ch := at.owner.expect(http.StatusOK, "GET", path, nil)
	push := ch.body["app"].(map[string]any)["push"].(map[string]any)
	if push["enabled"] != false || push["apns"].(map[string]any)["set"] != false || push["fcm"].(map[string]any)["set"] != false {
		t.Fatalf("default push: %s", ch.raw)
	}
	at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{"enabled": true}}})
	at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"apns": map[string]any{"key_id": "ABCDEFGHIJ", "team_id": "TEAM123456", "bundle_id": "com.example.app", "private_key": "not a key"},
	}}})
	at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"fcm": map[string]any{"service_account": `{"type":"authorized_user"}`},
	}}})
	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"allow_anonymous": true}})
	upd := at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"enabled": true, "show_content": true,
		"apns": map[string]any{"key_id": "ABCDEFGHIJ", "team_id": "TEAM123456", "bundle_id": "com.example.app", "environment": "sandbox", "private_key": p8},
		"fcm":  map[string]any{"service_account": sa},
	}}})
	if upd.body["app"].(map[string]any)["allow_anonymous"] != true {
		t.Fatalf("a push change reset allow_anonymous: %s", upd.raw)
	}
	if strings.Contains(string(upd.raw), "PRIVATE KEY") || strings.Contains(string(upd.raw), "private_key") {
		t.Fatalf("credentials answered back: %s", upd.raw)
	}
	push = upd.body["app"].(map[string]any)["push"].(map[string]any)
	apns, fcm := push["apns"].(map[string]any), push["fcm"].(map[string]any)
	if push["enabled"] != true || apns["set"] != true || apns["environment"] != "sandbox" || apns["bundle_id"] != "com.example.app" ||
		fcm["set"] != true || fcm["project_id"] != "yuva-test" || fcm["client_email"] != "push@yuva-test.iam.gserviceaccount.com" {
		t.Fatalf("push settings: %s", upd.raw)
	}
	kept := at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"fcm":  map[string]any{"project_id": "yuva-test"},
		"apns": map[string]any{"key_id": "ABCDEFGHIJ", "team_id": "TEAM123456", "bundle_id": "com.example.app", "environment": "sandbox"},
	}}})
	if p := kept.body["app"].(map[string]any)["push"].(map[string]any); p["fcm"].(map[string]any)["set"] != true || p["apns"].(map[string]any)["set"] != true {
		t.Fatalf("credentials lost when the key and the file were absent: %s", kept.raw)
	}
	list := at.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+at.appInbox+"/channels", nil)
	if p := list.body["items"].([]any)[0].(map[string]any)["app"].(map[string]any)["push"].(map[string]any); p["enabled"] != true {
		t.Fatalf("listed push: %s", list.raw)
	}

	cs := at.session(h, "push-user-1")
	register := func(c contactSession, platform, token, app string) string {
		t.Helper()
		return c.expect(http.StatusOK, "POST", "/client/v1/push-devices", map[string]any{"platform": platform, "token": token, "app_id": app}).str("id")
	}
	ios := register(cs, "ios", "ios-token-1", "com.example.app")
	if again := register(cs, "ios", "ios-token-1", "com.example.app"); again != ios {
		t.Fatalf("same token, new device %s != %s", again, ios)
	}
	register(cs, "android", "fcm-token-1", "com.example.app")
	register(cs, "ios", "gone", "com.example.app")
	register(cs, "android", "unregistered", "com.example.app")
	register(cs, "ios", "debug-build", "com.example.app.debug")
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/push-devices", map[string]any{"platform": "web", "token": "x", "app_id": "a"})
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/push-devices", map[string]any{"platform": "ios", "token": "a b", "app_id": "a"})

	ct := newChatTeam(t, h, "live", true)
	web := ct.session(h, map[string]any{})
	web.expectProblem(http.StatusForbidden, "forbidden", "POST", "/client/v1/push-devices", map[string]any{"platform": "ios", "token": "t", "app_id": "a"})
	other := newAppTeam(t, h, false)
	stranger := other.session(h, "push-user-1")
	stranger.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/client/v1/push-devices/"+ios, nil)

	conv := cs.start("Where is my order?")
	reply := func(body string) {
		t.Helper()
		at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": body})
	}
	reply("It ships today.")
	h.runAppPushChecks(at.ws)
	if retries := h.runAppPushSends(at.ws); retries != 0 {
		t.Fatalf("retries %d", retries)
	}
	sent := fake.take()
	byToken := map[string]sentPush{}
	for _, s := range sent {
		byToken[s.token] = s
	}
	if len(sent) != 2 {
		t.Fatalf("pushes %+v", sent)
	}
	ap := byToken["ios-token-1"]
	aps := ap.body["aps"].(map[string]any)
	alert := aps["alert"].(map[string]any)
	if ap.topic != "com.example.app" || ap.body["yuva_conversation_id"] != conv || alert["title"] != "App" || alert["body"] != "It ships today." || aps["badge"] != float64(1) {
		t.Fatalf("APNs push: %+v", ap)
	}
	fp := byToken["fcm-token-1"].body["message"].(map[string]any)
	if fp["data"].(map[string]any)["yuva_conversation_id"] != conv || fp["notification"].(map[string]any)["body"] != "It ships today." {
		t.Fatalf("FCM push: %+v", fp)
	}
	var left int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM push_devices WHERE workspace_id = $1 AND token IN ('gone', 'unregistered')", at.ws).Scan(&left); err != nil || left != 0 {
		t.Fatalf("invalid tokens kept: %d %v", left, err)
	}

	h.clock.Advance(time.Second)
	reply("And one more thing.")
	h.runAppPushChecks(at.ws)
	if n := len(h.jobs("app_push_send", at.ws)); n != 0 {
		t.Fatalf("pushed inside the coalescing window: %d", n)
	}
	h.clock.Advance(30 * time.Second)
	if _, err := h.srv.CheckAppPush(context.Background(), uuid.MustParse(at.ws), uuid.MustParse(conv)); err != nil {
		t.Fatal(err)
	}
	h.runAppPushSends(at.ws)
	if sent := fake.take(); len(sent) != 2 || sent[0].token == "debug-build" {
		t.Fatalf("after the window: %+v", sent)
	}

	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{"show_content": false}}})
	h.clock.Advance(31 * time.Second)
	reply("Secret tracking number 12345")
	h.runAppPushChecks(at.ws)
	h.runAppPushSends(at.ws)
	for _, s := range fake.take() {
		if strings.Contains(string(mustMarshal(s.body)), "12345") {
			t.Fatalf("content shown with show_content off: %+v", s.body)
		}
	}

	h.clock.Advance(31 * time.Second)
	ws, status, err := dialContact(h, cs.token, "", "")
	if err != nil {
		t.Fatalf("dial: %d %v", status, err)
	}
	reply("You are online")
	h.runAppPushChecks(at.ws)
	if n := len(h.jobs("app_push_send", at.ws)); n != 0 {
		t.Fatalf("pushed to a connected contact: %d", n)
	}
	ws.close()

	busy := register(cs, "ios", "busy", "com.example.app")
	if _, err := h.st.Pool.Exec(context.Background(), "DELETE FROM push_devices WHERE workspace_id = $1 AND id <> $2", at.ws, busy); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(31 * time.Second)
	reply("Retry me")
	h.runAppPushChecks(at.ws)
	if retries := h.runAppPushSends(at.ws); retries != 1 {
		t.Fatalf("retries %d, want 1", retries)
	}

	cs.expect(http.StatusNoContent, "DELETE", "/client/v1/push-devices/"+busy, nil)
	cs.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/client/v1/push-devices/"+busy, nil)

	moved := register(cs, "ios", "shared-token", "com.example.app")
	cs2 := at.session(h, "push-user-2")
	if id := register(cs2, "ios", "shared-token", "com.example.app"); id != moved {
		t.Fatalf("token did not move: %s != %s", id, moved)
	}
	cs.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/client/v1/push-devices/"+moved, nil)
	if _, err := h.st.Pool.Exec(context.Background(), "DELETE FROM push_devices WHERE workspace_id = $1", at.ws); err != nil {
		t.Fatal(err)
	}
	queued := register(cs, "ios", "queued-token", "com.example.app")
	h.clock.Advance(31 * time.Second)
	reply("Queued before the move")
	h.runAppPushChecks(at.ws)
	if n := len(h.jobs("app_push_send", at.ws)); n != 1 {
		t.Fatalf("send jobs %d, want 1", n)
	}
	if id := register(cs2, "ios", "queued-token", "com.example.app"); id != queued {
		t.Fatalf("token did not move: %s != %s", id, queued)
	}
	h.runAppPushSends(at.ws)
	if sent := fake.take(); len(sent) != 0 {
		t.Fatalf("a push for the old contact reached the moved device: %+v", sent)
	}

	cs3 := at.session(h, "push-user-3")
	old := cs3.start("Old question")
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+old+"/messages", map[string]any{"kind": "message", "body": "Old unread answer"})
	h.clock.Advance(time.Minute)
	register(cs3, "android", "fcm-token-3", "com.example.app")
	if _, err := h.srv.CheckAppPush(context.Background(), uuid.MustParse(at.ws), uuid.MustParse(old)); err != nil {
		t.Fatal(err)
	}
	if n := len(h.jobs("app_push_send", at.ws)); n != 0 {
		t.Fatalf("a reply older than the device was pushed: %d", n)
	}
	h.clock.Advance(time.Second)
	at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+old+"/messages", map[string]any{"kind": "message", "body": "New answer"})
	h.runAppPushChecks(at.ws)
	h.runAppPushSends(at.ws)
	if sent := fake.take(); len(sent) != 1 || sent[0].body["message"].(map[string]any)["data"].(map[string]any)["yuva_conversation_id"] != old {
		t.Fatalf("new reply after registering: %+v", sent)
	}

	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{"enabled": false}}})
	h.clock.Advance(31 * time.Second)
	reply("Push is off")
	if n := len(h.jobs("app_push_check", at.ws)); n != 0 {
		t.Fatalf("push check queued with push off: %d", n)
	}
	removed := at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"apns": map[string]any{"private_key": ""}, "fcm": map[string]any{"service_account": ""},
	}}})
	if p := removed.body["app"].(map[string]any)["push"].(map[string]any); p["apns"].(map[string]any)["set"] != false || p["fcm"].(map[string]any)["set"] != false {
		t.Fatalf("credentials not removed: %s", removed.raw)
	}
}

func TestAppPushChannelErrorsKeepTokens(t *testing.T) {
	ec, p8, rk, sa := testAppPushKeys(t)
	fake := newFakeAppPush(t, &ec.PublicKey, &rk.PublicKey)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.AppPush = api.AppPushSettings{APNsProduction: fake.srv.URL, APNsSandbox: fake.srv.URL, FCM: fake.srv.URL, TokenURL: fake.srv.URL + "/token", HTTPClient: fake.srv.Client()}
	})
	at := newAppTeam(t, h, false)
	path := "/v1/channels/" + at.channel
	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"app": map[string]any{"push": map[string]any{
		"enabled": true,
		"apns":    map[string]any{"key_id": "ABCDEFGHIJ", "team_id": "TEAM123456", "bundle_id": "com.example.app", "private_key": p8},
		"fcm":     map[string]any{"service_account": sa},
	}}})
	cs := at.session(h, "errors-user")
	register := func(platform, token string) {
		t.Helper()
		cs.expect(http.StatusOK, "POST", "/client/v1/push-devices", map[string]any{"platform": platform, "token": token, "app_id": "com.example.app"})
	}
	tokens := func() map[string]bool {
		t.Helper()
		rows, err := h.st.Pool.Query(context.Background(), "SELECT token FROM push_devices WHERE workspace_id = $1", at.ws)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var tok string
			_ = rows.Scan(&tok)
			out[tok] = true
		}
		return out
	}
	lastError := func() string {
		t.Helper()
		p := at.owner.expect(http.StatusOK, "GET", path, nil).body["app"].(map[string]any)["push"].(map[string]any)
		e, _ := p["last_error"].(string)
		return e
	}
	register("ios", "bad")
	register("android", "mismatch")
	conv := cs.start("Hello")
	reply := func(body string) {
		t.Helper()
		h.clock.Advance(31 * time.Second)
		at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": body})
		h.runAppPushChecks(at.ws)
		h.runAppPushSends(at.ws)
	}

	reply("first")
	if got := tokens(); !got["bad"] || !got["mismatch"] {
		t.Fatalf("tokens deleted on channel errors: %v", got)
	}
	if e := lastError(); !strings.Contains(e, "BadDeviceToken") && !strings.Contains(e, "SENDER_ID_MISMATCH") {
		t.Fatalf("last_error %q", e)
	}

	register("ios", "good")
	if _, err := h.st.Pool.Exec(context.Background(), "DELETE FROM push_devices WHERE workspace_id = $1 AND token = 'bad'", at.ws); err != nil {
		t.Fatal(err)
	}
	reply("verifies the key")
	if fake.apnsOK != 1 {
		t.Fatalf("APNs successes %d", fake.apnsOK)
	}
	register("ios", "bad")
	reply("after a success")
	got := tokens()
	if got["bad"] || !got["good"] || !got["mismatch"] {
		t.Fatalf("after APNs worked in this environment: %v", got)
	}
	if e := lastError(); !strings.Contains(e, "SENDER_ID_MISMATCH") {
		t.Fatalf("last_error %q, want the FCM project mismatch", e)
	}
}

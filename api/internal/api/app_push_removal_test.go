package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
)

func newPushRemovalTeam(t *testing.T) (*harness, appTeam, *fakeAppPush) {
	ec, p8, rk, sa := testAppPushKeys(t)
	fake := newFakeAppPush(t, &ec.PublicKey, &rk.PublicKey)
	h := newHarnessWith(t, func(d *api.Deps) {
		d.AppPush = api.AppPushSettings{APNsProduction: fake.srv.URL, APNsSandbox: fake.srv.URL, FCM: fake.srv.URL, TokenURL: fake.srv.URL + "/token", HTTPClient: fake.srv.Client()}
	})
	at := newAppTeam(t, h, false)
	at.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+at.channel, map[string]any{"app": map[string]any{"push": map[string]any{
		"enabled": true, "show_content": true,
		"apns": map[string]any{"key_id": "ABCDEFGHIJ", "team_id": "TEAM123456", "bundle_id": "com.example.app", "environment": "sandbox", "private_key": p8},
		"fcm":  map[string]any{"service_account": sa},
	}}})
	return h, at, fake
}

func TestAppPushSendFollowsRemoval(t *testing.T) {
	h, at, fake := newPushRemovalTeam(t)
	ctx := context.Background()
	round := func(user string, act func(conv, msg, contact string), wantSent bool) {
		t.Helper()
		cs := at.session(h, user)
		cs.expect(http.StatusOK, "POST", "/client/v1/push-devices", map[string]any{"platform": "ios", "token": "tok-" + user, "app_id": "com.example.app"})
		conv := cs.start("Question " + user)
		h.clock.Advance(time.Second)
		msg := at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Secret " + user}).str("id")
		var contact string
		if err := h.st.Pool.QueryRow(ctx, "SELECT contact_id::text FROM conversations WHERE workspace_id = $1 AND id = $2", at.ws, conv).Scan(&contact); err != nil {
			t.Fatal(err)
		}
		h.runAppPushChecks(at.ws)
		jobs := h.jobs("app_push_send", at.ws)
		if len(jobs) != 1 {
			t.Fatalf("%s: send jobs %d", user, len(jobs))
		}
		if strings.Contains(string(jobs[0].args), "Secret") || strings.Contains(string(jobs[0].args), "Question") {
			t.Fatalf("%s: job carries text: %s", user, jobs[0].args)
		}
		act(conv, msg, contact)
		h.runAppPushSends(at.ws)
		sent := fake.take()
		if wantSent != (len(sent) == 1) {
			t.Fatalf("%s: pushes %+v, want sent=%v", user, sent, wantSent)
		}
		if wantSent && !strings.Contains(string(mustMarshal(sent[0].body)), "Secret "+user) {
			t.Fatalf("%s: preview missing: %+v", user, sent[0].body)
		}
	}
	round("plain", func(string, string, string) {}, true)
	round("redact", func(_, msg, _ string) {
		at.owner.expect(http.StatusOK, "POST", "/v1/messages/"+msg+"/redact", nil)
	}, false)
	round("trash", func(conv, _, _ string) {
		at.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	}, false)
	round("contact", func(_, _, contact string) {
		at.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+contact, nil)
	}, false)
}

func TestAppPushSendIgnoresStoredText(t *testing.T) {
	h, at, fake := newPushRemovalTeam(t)
	cs := at.session(h, "legacy")
	cs.expect(http.StatusOK, "POST", "/client/v1/push-devices", map[string]any{"platform": "ios", "token": "tok-legacy", "app_id": "com.example.app"})
	conv := cs.start("Hello")
	h.clock.Advance(time.Second)
	msg := at.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Current text"}).str("id")
	h.runAppPushChecks(at.ws)
	jobs := h.jobs("app_push_send", at.ws)
	if len(jobs) != 1 {
		t.Fatalf("send jobs %d", len(jobs))
	}
	var args map[string]any
	_ = json.Unmarshal(jobs[0].args, &args)
	delete(args, "message_id")
	args["title"], args["body"] = "Old title", "Old text from before the change"
	var legacy api.AppPushSendArgs
	if err := json.Unmarshal([]byte(mustMarshal(args)), &legacy); err != nil {
		t.Fatal(err)
	}
	h.dropJob(jobs[0].id)
	if _, err := h.srv.SendAppPush(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	sent := fake.take()
	if len(sent) != 1 || !strings.Contains(string(mustMarshal(sent[0].body)), "Current text") || strings.Contains(string(mustMarshal(sent[0].body)), "Old") {
		t.Fatalf("legacy job: %+v", sent)
	}
	at.owner.expect(http.StatusOK, "POST", "/v1/messages/"+msg+"/redact", nil)
	if _, err := h.srv.SendAppPush(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	if sent := fake.take(); len(sent) != 0 {
		t.Fatalf("legacy job pushed a redacted reply: %+v", sent)
	}
}

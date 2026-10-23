package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

const (
	testSlackSecret = "slack-signing-secret"
	testSlackToken  = "xoxb-test-token"
	testSlackApp    = "A0APP"
	testSlackBot    = "U0BOT"
)

type slackCall struct {
	method string
	form   url.Values
}

type fakeSlack struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []slackCall
	ts    int
	team  string
	users map[string]string
}

func newFakeSlack(t *testing.T) *fakeSlack {
	f := &fakeSlack{team: strings.ToUpper(strings.ReplaceAll(unique("T"), "-", "")), users: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		method := strings.TrimPrefix(r.URL.Path, "/api/")
		f.mu.Lock()
		f.calls = append(f.calls, slackCall{method: method, form: r.PostForm})
		team := f.team
		f.mu.Unlock()
		answer := func(v map[string]any) {
			v["ok"] = true
			_ = json.NewEncoder(w).Encode(v)
		}
		if method == "oauth.v2.access" {
			id, secret, _ := r.BasicAuth()
			if id != "slack-client" || secret != "slack-client-secret" || r.PostForm.Get("code") == "bad" {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_code"})
				return
			}
			scope := strings.Join([]string{"chat:write", "channels:read", "groups:read", "channels:history", "groups:history", "im:write", "users:read", "users:read.email"}, ",")
			if r.PostForm.Get("code") == "narrow" {
				scope = "chat:write"
			}
			answer(map[string]any{"access_token": testSlackToken, "token_type": "bot", "scope": scope, "bot_user_id": testSlackBot, "app_id": testSlackApp,
				"team": map[string]any{"id": team, "name": "Acme"}})
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testSlackToken {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_auth"})
			return
		}
		switch method {
		case "auth.revoke":
			answer(map[string]any{"revoked": true})
		case "users.conversations":
			answer(map[string]any{"channels": []map[string]any{
				{"id": "C0SUPPORT", "name": "support", "is_private": false},
				{"id": "G0MENTIONS", "name": "mentions", "is_private": true},
			}})
		case "users.lookupByEmail":
			id, ok := f.users[r.PostForm.Get("email")]
			if !ok {
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "users_not_found"})
				return
			}
			answer(map[string]any{"user": map[string]any{"id": id, "name": strings.ToLower(id), "profile": map[string]any{"display_name": "user " + id}}})
		case "conversations.open":
			answer(map[string]any{"channel": map[string]any{"id": "D" + r.PostForm.Get("users")}})
		case "chat.postMessage":
			f.mu.Lock()
			f.ts++
			ts := fmt.Sprintf("1700000000.%06d", f.ts)
			f.mu.Unlock()
			answer(map[string]any{"ts": ts, "channel": r.PostForm.Get("channel")})
		case "chat.postEphemeral":
			answer(map[string]any{"message_ts": "1"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "unknown_method"})
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSlack) take(method string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []url.Values
	rest := f.calls[:0]
	for _, c := range f.calls {
		if c.method == method {
			out = append(out, c.form)
		} else {
			rest = append(rest, c)
		}
	}
	f.calls = rest
	return out
}

func slackHarness(t *testing.T, fake *fakeSlack) *harness {
	return newHarnessWith(t, func(d *api.Deps) {
		d.Slack = api.SlackSettings{ClientID: "slack-client", ClientSecret: "slack-client-secret", SigningSecret: testSlackSecret, APIBase: fake.srv.URL + "/api/"}
	})
}

// connectSlack runs the install as c and returns the panel redirect's result.
func connectSlack(t *testing.T, h *harness, starter, finisher *client, code string) string {
	t.Helper()
	start := starter.expect(http.StatusOK, "POST", "/v1/slack/connect", nil)
	u, err := url.Parse(start.str("authorize_url"))
	if err != nil || u.Host != "slack.com" || u.Query().Get("redirect_uri") != testOrigin+"/slack/oauth/callback" || !strings.Contains(u.Query().Get("scope"), "chat:write") {
		t.Fatalf("authorize url: %s", start.raw)
	}
	q := url.Values{"code": {code}, "state": {u.Query().Get("state")}}
	res, err := noRedirect(finisher).Get(h.url + "/slack/oauth/callback?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loc, _ := url.Parse(res.Header.Get("Location"))
	if res.StatusCode != http.StatusSeeOther || loc.Path != "/settings/slack" {
		t.Fatalf("callback: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	return loc.Query().Get("result")
}

func slackSign(body []byte, ts string) string {
	m := hmac.New(sha256.New, []byte(testSlackSecret))
	m.Write([]byte("v0:" + ts + ":"))
	m.Write(body)
	return "v0=" + hex.EncodeToString(m.Sum(nil))
}

func sendSlackEvent(t *testing.T, h *harness, body []byte, ts, sig string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", h.url+"/ingress/slack", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

var slackEventSeq int

// slackReply delivers a correctly signed thread reply and runs the job it queues.
func slackReply(t *testing.T, h *harness, team, ws, channel, thread, user, text string) {
	t.Helper()
	slackEventSeq++
	body, _ := json.Marshal(map[string]any{
		"type": "event_callback", "team_id": team, "api_app_id": testSlackApp, "event_id": fmt.Sprintf("Ev%d", slackEventSeq),
		"event": map[string]any{"type": "message", "channel": channel, "user": user, "text": text, "ts": fmt.Sprintf("1700000999.%06d", slackEventSeq), "thread_ts": thread},
	})
	ts := strconv.FormatInt(h.clock.Now().Unix(), 10)
	if got := sendSlackEvent(t, h, body, ts, slackSign(body, ts)); got != http.StatusOK {
		t.Fatalf("event: %d", got)
	}
	h.runSlackEvents(ws)
}

// slackSend delivers a correctly signed event_callback and runs the job it queues.
func slackSend(t *testing.T, h *harness, team, ws string, eventTime int64, event map[string]any) {
	t.Helper()
	slackEventSeq++
	body, _ := json.Marshal(map[string]any{
		"type": "event_callback", "team_id": team, "api_app_id": testSlackApp, "event_id": fmt.Sprintf("Ev%d", slackEventSeq),
		"event_time": eventTime, "event": event,
	})
	ts := strconv.FormatInt(h.clock.Now().Unix(), 10)
	if got := sendSlackEvent(t, h, body, ts, slackSign(body, ts)); got != http.StatusOK {
		t.Fatalf("event: %d", got)
	}
	h.runSlackEvents(ws)
}

func (h *harness) runSlackEvents(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("slack_event", ws) {
		var a api.SlackEventArgs
		_ = json.Unmarshal(j.args, &a)
		if err := h.srv.HandleSlackEvent(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (h *harness) runSlackPosts(ws string) (snoozed int) {
	h.t.Helper()
	h.runNotifyOnly(ws)
	for _, j := range h.jobs("slack_post", ws) {
		var a api.SlackPostArgs
		_ = json.Unmarshal(j.args, &a)
		wait, err := h.srv.PostToSlack(context.Background(), a)
		if err != nil {
			h.t.Fatal(err)
		}
		if wait > 0 {
			snoozed++
		}
		h.dropJob(j.id)
	}
	return snoozed
}

func TestSlackOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	r := tm.owner.expect(http.StatusOK, "GET", "/v1/slack", nil)
	if r.body["available"] != false || r.body["connected"] != false {
		t.Fatalf("slack off: %s", r.raw)
	}
	tm.owner.expectProblem(http.StatusConflict, "slack_not_configured", "POST", "/v1/slack/connect", nil)
	tm.owner.expectProblem(http.StatusConflict, "slack_not_configured", "GET", "/v1/slack/channels", nil)
	tm.owner.expectProblem(http.StatusConflict, "slack_not_configured", "POST", "/v1/me/slack/link", nil)
	if got := sendSlackEvent(t, h, []byte(`{}`), "1", "v0=00"); got != http.StatusNotFound {
		t.Fatalf("ingress with Slack off: %d", got)
	}
}

func TestSlack(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/slack/connect", nil)
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/slack/connect", nil)
	tm.owner.expectProblem(http.StatusConflict, "slack_not_connected", "GET", "/v1/slack/channels", nil)
	if got := connectSlack(t, h, tm.owner, tm.agent, "good"); got != "session_mismatch" {
		t.Fatalf("another session finished the install: %s", got)
	}
	if got := connectSlack(t, h, tm.owner, tm.owner, "narrow"); got != "scope_missing" {
		t.Fatalf("narrow grant: %s", got)
	}
	if len(fake.take("auth.revoke")) != 1 {
		t.Fatal("a token with too few scopes was kept")
	}
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	st := tm.owner.expect(http.StatusOK, "GET", "/v1/slack", nil)
	if st.body["connected"] != true || st.str("team_id") != fake.team || st.str("reply_command") != "!reply" || strings.Contains(string(st.raw), testSlackToken) {
		t.Fatalf("connection: %s", st.raw)
	}
	var sealed []byte
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT bot_token FROM slack_installations WHERE workspace_id = $1", tm.ws).Scan(&sealed); err != nil || bytes.Contains(sealed, []byte(testSlackToken)) {
		t.Fatalf("token stored in the clear: %v", err)
	}
	if got := connectSlack(t, h, other.owner, other.owner, "good"); got != "team_taken" {
		t.Fatalf("second workspace took the same Slack team: %s", got)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/inboxes/"+tm.inbox+"/slack", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"include_content": true})

	chs := tm.owner.expect(http.StatusOK, "GET", "/v1/slack/channels", nil).body["items"].([]any)
	if len(chs) != 2 || chs[0].(map[string]any)["name"] != "mentions" {
		t.Fatalf("channels: %v", chs)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"new_conversations_channel_id": "C0ELSEWHERE"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"include_content": true})
	set := tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"new_conversations_channel_id": "C0SUPPORT", "mentions_channel_id": "G0MENTIONS"})
	if set.str("new_conversations_channel_id") != "C0SUPPORT" || set.body["include_content"] != false {
		t.Fatalf("inbox slack: %s", set.raw)
	}

	conv := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "<!channel> refund"}).str("id")
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "secret order 4711"})
	h.runSlackPosts(tm.ws)
	posts := fake.take("chat.postMessage")
	if len(posts) != 1 || posts[0].Get("channel") != "C0SUPPORT" {
		t.Fatalf("new conversation posts: %v", posts)
	}
	text := posts[0].Get("text")
	if strings.Contains(text, "4711") || strings.Contains(text, "<!channel>") || !strings.Contains(text, "&lt;!channel&gt; refund") ||
		!strings.Contains(text, "Ayşe Yılmaz") || !strings.Contains(text, testOrigin+"/conversations/"+conv) {
		t.Fatalf("post text: %q", text)
	}
	thread := "1700000000.000001"

	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0STRANGER", "let me take it")
	if eph := fake.take("chat.postEphemeral"); len(eph) != 1 || eph[0].Get("user") != "U0STRANGER" || !strings.Contains(eph[0].Get("text"), "not linked") {
		t.Fatalf("unlinked reply: %v", eph)
	}
	if n := len(messages(tm.owner, conv)); n != 1 {
		t.Fatalf("an unlinked Slack user wrote into the conversation: %d messages", n)
	}

	fake.users["owner-slack@example.com"] = "U0OWNER"
	fake.users["agent-slack@example.com"] = "U0AGENT"
	key.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/slack", nil)
	tm.owner.expectProblem(http.StatusNotFound, "slack_user_not_found", "POST", "/v1/me/slack/link", map[string]any{"email": "nobody@example.com"})
	tm.owner.expect(http.StatusAccepted, "POST", "/v1/me/slack/link", map[string]any{"email": "owner-slack@example.com"})
	dms := fake.take("chat.postMessage")
	code := regexp.MustCompile(`\*([0-9]{6})\*`).FindStringSubmatch(dms[0].Get("text"))
	if len(dms) != 1 || dms[0].Get("channel") != "DU0OWNER" || code == nil {
		t.Fatalf("code DM: %v", dms)
	}
	wrong := "000000"
	if code[1] == wrong {
		wrong = "111111"
	}
	tm.owner.expectProblem(http.StatusBadRequest, "invalid_code", "POST", "/v1/me/slack/link/confirm", map[string]any{"code": wrong})
	tm.agent.expectProblem(http.StatusBadRequest, "code_expired", "POST", "/v1/me/slack/link/confirm", map[string]any{"code": code[1]})
	linked := tm.owner.expect(http.StatusOK, "POST", "/v1/me/slack/link/confirm", map[string]any{"code": code[1]})
	if linked.body["linked"] != true || linked.str("slack_user_id") != "U0OWNER" {
		t.Fatalf("link: %s", linked.raw)
	}
	tm.agent.expectProblem(http.StatusConflict, "slack_user_taken", "POST", "/v1/me/slack/link", map[string]any{"email": "owner-slack@example.com"})

	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0OWNER", "checked <https://example.com/o/1|the order>, looks fine")
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0OWNER", "!reply Your refund is on its way.")
	msgs := messages(tm.owner, conv)
	if len(msgs) != 3 {
		t.Fatalf("messages after Slack replies: %v", msgs)
	}
	note, reply := msgs[1], msgs[2]
	if note["kind"] != "note" || note["body"] != "checked the order (https://example.com/o/1), looks fine" || note["author"].(map[string]any)["via"] != "Slack" ||
		note["author"].(map[string]any)["type"] != "member" {
		t.Fatalf("note from Slack: %v", note)
	}
	if reply["kind"] != "message" || reply["direction"] != "out" || reply["body"] != "Your refund is on its way." || reply["author"].(map[string]any)["via"] != "Slack" {
		t.Fatalf("reply from Slack: %v", reply)
	}
	if eph := fake.take("chat.postEphemeral"); len(eph) != 1 || eph[0].Get("text") != "Sent to the customer." {
		t.Fatalf("reply confirmation: %v", eph)
	}

	tm.agent.expect(http.StatusAccepted, "POST", "/v1/me/slack/link", map[string]any{"email": "agent-slack@example.com"})
	code = regexp.MustCompile(`\*([0-9]{6})\*`).FindStringSubmatch(fake.take("chat.postMessage")[0].Get("text"))
	tm.agent.expect(http.StatusOK, "POST", "/v1/me/slack/link/confirm", map[string]any{"code": code[1]})
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0AGENT", "!reply I will look")
	if eph := fake.take("chat.postEphemeral"); len(eph) != 1 || !strings.Contains(eph[0].Get("text"), "cannot see this conversation") {
		t.Fatalf("agent without access: %v", eph)
	}
	if n := len(messages(tm.owner, conv)); n != 3 {
		t.Fatalf("an agent without access wrote from Slack: %d messages", n)
	}

	ts := strconv.FormatInt(h.clock.Now().Unix(), 10)
	forged, _ := json.Marshal(map[string]any{"type": "event_callback", "team_id": fake.team, "api_app_id": testSlackApp, "event_id": "EvForged",
		"event": map[string]any{"type": "message", "channel": "C0SUPPORT", "user": "U0OWNER", "text": "forged", "ts": "1700000999.9", "thread_ts": thread}})
	if got := sendSlackEvent(t, h, forged, ts, "v0="+strings.Repeat("ab", 32)); got != http.StatusUnauthorized {
		t.Fatalf("forged signature: %d", got)
	}
	old := strconv.FormatInt(h.clock.Now().Add(-10*time.Minute).Unix(), 10)
	if got := sendSlackEvent(t, h, forged, old, slackSign(forged, old)); got != http.StatusUnauthorized {
		t.Fatalf("replayed outside the window: %d", got)
	}
	for range 2 {
		if got := sendSlackEvent(t, h, forged, ts, slackSign(forged, ts)); got != http.StatusOK {
			t.Fatalf("signed event: %d", got)
		}
	}
	if n := len(h.jobs("slack_event", tm.ws)); n != 1 {
		t.Fatalf("the same event id queued %d times", n)
	}
	h.runSlackEvents(tm.ws)
	if n := len(messages(tm.owner, conv)); n != 4 {
		t.Fatalf("signed event once: %d messages", n)
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": true})
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0OWNER", "!reply buy now")
	if eph := fake.take("chat.postEphemeral"); len(eph) != 1 || !strings.Contains(eph[0].Get("text"), "spam") {
		t.Fatalf("reply to a spam conversation: %v", eph)
	}
	if n := len(messages(tm.owner, conv)); n != 4 {
		t.Fatalf("a reply went out on a spam conversation: %d messages", n)
	}
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0OWNER", "looks like spam to me")
	if msgs := messages(tm.owner, conv); len(msgs) != 5 || msgs[4]["kind"] != "note" {
		t.Fatalf("a note on a spam conversation: %v", msgs)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"spam": false})

	stale, _ := json.Marshal(map[string]any{"type": "message", "channel": "C0SUPPORT", "user": "U0OWNER", "text": "from a stale queue", "ts": "1700000999.777", "thread_ts": thread})
	if err := h.srv.HandleSlackEvent(context.Background(), api.SlackEventArgs{WorkspaceID: uuid.MustParse(tm.ws), TeamID: "T0OTHERTEAM", EventID: "EvStale", Event: stale}); err != nil {
		t.Fatal(err)
	}
	if n := len(messages(tm.owner, conv)); n != 5 {
		t.Fatalf("an event for another Slack team wrote a note: %d messages", n)
	}
	uninstalled := map[string]any{"type": "app_uninstalled"}
	slackSend(t, h, fake.team, tm.ws, h.clock.Now().Add(-time.Hour).Unix(), uninstalled)
	other1, _ := json.Marshal(uninstalled)
	if err := h.srv.HandleSlackEvent(context.Background(), api.SlackEventArgs{WorkspaceID: uuid.MustParse(tm.ws), TeamID: "T0OTHERTEAM", EventID: "EvGone", EventTime: h.clock.Now().Unix(), Event: other1}); err != nil {
		t.Fatal(err)
	}
	if tm.owner.expect(http.StatusOK, "GET", "/v1/slack", nil).body["connected"] != true {
		t.Fatal("an uninstall older than the installation, or for another team, disconnected it")
	}

	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	post(tm.owner, conv, map[string]any{"kind": "note", "body": "private remark", "mentions": []string{tm.agentID}})
	h.runSlackPosts(tm.ws)
	posts = fake.take("chat.postMessage")
	if len(posts) != 1 || posts[0].Get("channel") != "G0MENTIONS" || !strings.Contains(posts[0].Get("text"), "<@U0AGENT>") || strings.Contains(posts[0].Get("text"), "private remark") {
		t.Fatalf("mention post: %v", posts)
	}

	tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"new_conversations_channel_id": "C0SUPPORT", "include_content": true})
	conv2 := tm.conversation(tm.owner)
	post(key, conv2, map[string]any{"kind": "message", "direction": "in", "body": "where is <@U0OWNER> order 99"})
	h.runSlackPosts(tm.ws)
	posts = fake.take("chat.postMessage")
	if len(posts) != 1 || !strings.Contains(posts[0].Get("text"), "> where is &lt;@U0OWNER&gt; order 99") {
		t.Fatalf("post with content: %v", posts)
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/slack", map[string]any{"reply_command": "!send"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/slack", map[string]any{"reply_command": "/reply"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/slack", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/me/slack", nil)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/slack", nil)
	if len(fake.take("auth.revoke")) != 1 {
		t.Fatal("disconnect did not revoke the token")
	}
	if tm.owner.expect(http.StatusOK, "GET", "/v1/slack", nil).body["connected"] != false {
		t.Fatal("still connected")
	}
	tm.owner.expectProblem(http.StatusConflict, "slack_not_connected", "DELETE", "/v1/slack", nil)
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("reconnect: %s", got)
	}
	h.clock.Advance(time.Minute)
	slackSend(t, h, fake.team, tm.ws, h.clock.Now().Unix(), map[string]any{"type": "app_uninstalled"})
	if tm.owner.expect(http.StatusOK, "GET", "/v1/slack", nil).body["connected"] != false {
		t.Fatal("app_uninstalled kept the connection")
	}
	audit := tm.owner.expect(http.StatusOK, "GET", "/v1/audit-log", nil)
	for _, a := range []string{"workspace.slack_connected", "workspace.slack_disconnected", "member.slack_linked", "member.slack_unlinked"} {
		if !strings.Contains(string(audit.raw), `"`+a+`"`) {
			t.Fatalf("audit log lacks %s: %s", a, audit.raw)
		}
	}
}

package api_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func contactCount(t *testing.T, ct chatTeam) int {
	t.Helper()
	return len(ct.owner.expect(http.StatusOK, "GET", "/v1/contacts?limit=100", nil).body["items"].([]any))
}

func TestClientChannelSettings(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	async := newChatTeam(t, h, "async", true)

	c := h.client()
	c.origin = ct.origin
	before := contactCount(t, ct)
	r := c.expect(http.StatusOK, "GET", "/client/v1/channels/"+ct.key, nil)
	if got := r.header.Get("Access-Control-Allow-Origin"); got != ct.origin {
		t.Fatalf("Access-Control-Allow-Origin %q", got)
	}
	chat, _ := r.body["chat"].(map[string]any)
	presence, _ := r.body["presence"].(map[string]any)
	if r.str("name") != "Chat" || r.str("mode") != "live" || r.body["open_now"] != true || chat["greeting"] != "Hi there" ||
		chat["allow_anonymous"] != false || presence == nil || presence["available"] != false {
		t.Fatalf("channel settings: %s", r.raw)
	}
	for _, leak := range []string{"token", "visitor_id", "contact", "identity_secret", "allowed_origins"} {
		if strings.Contains(string(r.raw), `"`+leak+`"`) {
			t.Fatalf("channel settings carry %q: %s", leak, r.raw)
		}
	}
	if after := contactCount(t, ct); after != before {
		t.Fatalf("reading the channel created contacts: %d -> %d", before, after)
	}

	a := h.client()
	a.origin = async.origin
	r = a.expect(http.StatusOK, "GET", "/client/v1/channels/"+async.key, nil)
	if _, ok := r.body["presence"]; ok || r.str("mode") != "async" || r.body["expected_reply_minutes"] != float64(30) {
		t.Fatalf("async channel settings: %s", r.raw)
	}

	c.origin = async.origin
	c.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", "/client/v1/channels/"+ct.key, nil)
	c.origin = "https://evil.example"
	if r := c.do("GET", "/client/v1/channels/"+ct.key, nil); r.status != http.StatusForbidden || r.header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unknown origin: %d %s", r.status, r.raw)
	}
	c.origin = ct.origin
	c.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/channels/yuva_pk_unknown", nil)
	c.origin = ""
	c.expect(http.StatusOK, "GET", "/client/v1/channels/"+ct.key, nil)

	c.origin = ct.origin
	limited := false
	for range 200 {
		if r := c.do("GET", "/client/v1/channels/"+ct.key, nil); r.status == http.StatusTooManyRequests {
			if r.str("code") != "rate_limited" {
				t.Fatalf("rate limit: %s", r.raw)
			}
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("channel settings are not rate limited per IP")
	}
}

func TestClientConversationMemberRead(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	conv := cs.start("hello")

	item := func() map[string]any {
		return cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any)[0].(map[string]any)
	}
	if v, ok := item()["last_read_by_member_at"]; ok {
		t.Fatalf("unread conversation has a member read time: %v", v)
	}
	first := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages", nil).body["items"].([]any)[0].(map[string]any)
	h.clock.Advance(time.Second)
	ct.agent.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", map[string]any{"message_id": first["id"]})
	h.clock.Advance(time.Second)
	second := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "anyone?", "client_id": unique("m")})
	if got := item()["last_read_by_member_at"]; got != first["created_at"] {
		t.Fatalf("member read time %v, want %v", got, first["created_at"])
	}
	ct.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/read", nil)
	one := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv, nil)
	if got := one.body["last_read_by_member_at"]; got != second.str("created_at") {
		t.Fatalf("latest member read time %v, want %v", got, second.str("created_at"))
	}
	if strings.Contains(string(one.raw), ct.agentID) || strings.Contains(string(one.raw), "member_id") {
		t.Fatalf("read time names a member: %s", one.raw)
	}

	ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"mode": "async"})
	if v, ok := item()["last_read_by_member_at"]; ok {
		t.Fatalf("async inbox shows a member read time: %v", v)
	}
}

func TestWidgetScripts(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"/yuva.js", "/yuva-chat.js"} {
		req, _ := http.NewRequest("GET", h.url+name, nil)
		req.Header.Set("Origin", "https://any.example")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		etag := res.Header.Get("ETag")
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") ||
			res.Header.Get("Access-Control-Allow-Origin") != "*" || res.Header.Get("Cache-Control") != "public, max-age=300" || etag == "" {
			t.Fatalf("%s: %d %v", name, res.StatusCode, res.Header)
		}
		req, _ = http.NewRequest("GET", h.url+name, nil)
		req.Header.Set("If-None-Match", etag)
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotModified {
			t.Fatalf("%s with a matching ETag: %d", name, res.StatusCode)
		}
	}
}

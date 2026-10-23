package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSessionsListAndSignOutEverywhere(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	laptop, phone, tablet := h.client(), h.client(), h.client()
	for _, c := range []*client{laptop, phone, tablet} {
		c.signIn(email)
	}
	stranger := h.client()
	strangerEmail := unique("other") + "@example.com"
	h.bootstrap(strangerEmail, unique("ws"))
	stranger.signIn(strangerEmail)

	list := laptop.expect(http.StatusOK, "GET", "/v1/me/sessions", nil)
	items := list.body["items"].([]any)
	current := 0
	for _, it := range items {
		s := it.(map[string]any)
		if s["current"] == true {
			current++
		}
		if s["method"] != "code" || s["user_agent"] == nil || s["last_seen_at"] == nil {
			t.Fatalf("session %v", s)
		}
	}
	if len(items) != 3 || current != 1 {
		t.Fatalf("sessions: %s", list.raw)
	}

	if got := stranger.expect(http.StatusOK, "DELETE", "/v1/me/sessions", nil); got.body["revoked"] != float64(0) {
		t.Fatalf("stranger revoked %s", got.raw)
	}
	phone.expect(http.StatusOK, "GET", "/v1/me", nil)

	key := laptop.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "ci", "scopes": []string{"conversations:read"}})
	bearer := h.client()
	bearer.bearer = key.str("secret")
	bearer.expectProblem(http.StatusForbidden, "member_session_required", "GET", "/v1/me/sessions", nil)
	bearer.expectProblem(http.StatusForbidden, "member_session_required", "DELETE", "/v1/me/sessions", nil)
	h.client().expectProblem(http.StatusUnauthorized, "unauthenticated", "DELETE", "/v1/me/sessions", nil)

	if got := laptop.expect(http.StatusOK, "DELETE", "/v1/me/sessions", nil); got.body["revoked"] != float64(2) {
		t.Fatalf("revoked %s", got.raw)
	}
	phone.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	tablet.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	if left := laptop.expect(http.StatusOK, "GET", "/v1/me/sessions", nil).body["items"].([]any); len(left) != 1 {
		t.Fatalf("left %v", left)
	}
	stranger.expect(http.StatusOK, "GET", "/v1/me", nil)
}

func TestRealtimePresenceOnlyFromMembersAndMemberApps(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	connections := func() int {
		t.Helper()
		var n int
		if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM realtime_connections WHERE workspace_id = $1 AND member_id = $2", tm.ws, tm.agentID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	scopes := []string{"conversations:read"}

	assistant := h.client()
	assistant.bearer = h.oauthToken(tm.agent, tm.ws, "Some MCP client", testOrigin, scopes)
	h.firstParty.Clear()
	ws := assistant.dial("")
	ws.ready()
	if n := connections(); n != 0 {
		t.Fatalf("an assistant's realtime connection made the member present: %d", n)
	}
	ws.close()

	app := h.client()
	app.bearer = h.oauthToken(tm.agent, tm.ws, "Member App", testOrigin, scopes)
	ws = app.dial("")
	ws.ready()
	if n := connections(); n != 1 {
		t.Fatalf("member app connections: %d", n)
	}
	ws.close()

	tm.agent.dial("").ready()
	if n := connections(); n < 1 {
		t.Fatalf("panel connections: %d", n)
	}
}

func TestUserAgentsThatAreNotUTF8(t *testing.T) {
	h := newHarness(t)
	email := unique("owner") + "@example.com"
	h.bootstrap(email, unique("ws"))
	c := h.client()
	c.userAgent = "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0 \xff"
	c.signIn(email)
	items := c.expect(http.StatusOK, "GET", "/v1/me/sessions", nil).body["items"].([]any)
	if s := items[0].(map[string]any); s["user_agent"] != "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0 \uFFFD" || s["device"] != "Firefox on Linux" {
		t.Fatalf("session %v", s)
	}

	lt := newListTeam(t, h)
	reader, token := lt.form(h)
	reader.userAgent = "Reader \xff"
	h.clock.Advance(3 * time.Second)
	reader.expect(http.StatusAccepted, "POST", "/client/v1/channels/"+lt.key+"/subscriptions", lt.subscribeBody(unique("reader")+"@example.org", token))
	if n := h.count("SELECT count(*) FROM consent_events WHERE workspace_id = $1 AND user_agent = $2", lt.ws, "Reader \uFFFD"); n != 1 {
		t.Fatalf("consent events with the cleaned user agent: %d", n)
	}
}

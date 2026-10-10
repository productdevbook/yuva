package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func memberIn(t *testing.T, r response, id string) map[string]any {
	t.Helper()
	for _, it := range r.body["items"].([]any) {
		if m := it.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("member %s not listed: %s", id, r.raw)
	return nil
}

func presenceOf(t *testing.T, w *wsClient, member string) map[string]any {
	t.Helper()
	for {
		m := w.nextOf("member.presence")
		var d map[string]any
		if err := json.Unmarshal(m.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d["member_id"] == member {
			return d
		}
	}
}

func TestTeammatePresence(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)

	if m := memberIn(t, tm.owner.expect(http.StatusOK, "GET", "/v1/members", nil), tm.agentID); m["online"] != false || m["availability"] != "auto" {
		t.Fatalf("agent before connecting: %v", m)
	}
	owner := tm.owner.dial("")
	owner.ready()
	stranger := other.owner.dial("")
	stranger.ready()

	agent := tm.agent.dial("")
	agent.ready()
	if d := presenceOf(t, owner, tm.agentID); d["online"] != true || d["availability"] != "auto" {
		t.Fatalf("agent connected: %v", d)
	}
	if m := memberIn(t, tm.owner.expect(http.StatusOK, "GET", "/v1/members", nil), tm.agentID); m["online"] != true {
		t.Fatalf("agent while connected: %v", m)
	}
	if m := tm.owner.expect(http.StatusOK, "GET", "/v1/members/"+tm.agentID, nil); m.body["online"] != true {
		t.Fatalf("get member: %s", m.raw)
	}

	tm.agent.expect(http.StatusOK, "PATCH", "/v1/me", map[string]any{"availability": "away"})
	if d := presenceOf(t, owner, tm.agentID); d["availability"] != "away" || d["online"] != true {
		t.Fatalf("agent away: %v", d)
	}
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if m := memberIn(t, tm.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+tm.inbox+"/members", nil), tm.agentID); m["online"] != true || m["availability"] != "away" {
		t.Fatalf("inbox members: %v", m)
	}

	_ = agent.conn.CloseNow()
	if d := presenceOf(t, owner, tm.agentID); d["online"] != false {
		t.Fatalf("agent disconnected: %v", d)
	}

	if _, err := h.st.Pool.Exec(context.Background(),
		"INSERT INTO realtime_connections (id, workspace_id, member_id, seen_at) VALUES (gen_random_uuid(), $1, $2, $3)",
		tm.ws, tm.agentID, h.clock.Now().Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.AnnounceLapsedPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := presenceOf(t, owner, tm.agentID); d["online"] != false {
		t.Fatalf("agent whose server died: %v", d)
	}
	for {
		select {
		case m := <-stranger.msgs:
			if m.WorkspaceID != other.ws {
				t.Fatalf("another workspace's member got %s of %s", m.Type, m.WorkspaceID)
			}
			continue
		case <-time.After(300 * time.Millisecond):
		}
		break
	}
}

func (tm team) ownerMemberID(t *testing.T) string {
	t.Helper()
	me := tm.owner.expect(http.StatusOK, "GET", "/v1/me", nil)
	return me.body["memberships"].([]any)[0].(map[string]any)["member_id"].(string)
}

func viewingOf(t *testing.T, w *wsClient) map[string]any {
	t.Helper()
	m := w.nextOf("viewing")
	var d map[string]any
	if err := json.Unmarshal(m.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d["conversation_id"] != m.ConversationID || m.InboxID == "" {
		t.Fatalf("viewing event %+v", m)
	}
	return d
}

func sendFrame(t *testing.T, w *wsClient, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := w.conn.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func TestWhoIsViewing(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	secret := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": hidden, "contact_id": tm.contact}).str("id")
	ownerID := tm.ownerMemberID(t)

	owner := tm.owner.dial("")
	owner.ready()
	agent := tm.agent.dial("")
	agent.ready()
	stranger := other.owner.dial("")
	stranger.ready()

	sendFrame(t, agent, map[string]any{"type": "viewing", "conversation_id": conv})
	if d := viewingOf(t, owner); d["member_id"] != tm.agentID || d["conversation_id"] != conv || d["viewing"] != true {
		t.Fatalf("agent opened: %v", d)
	}
	sendFrame(t, owner, map[string]any{"type": "viewing", "conversation_id": conv})
	if d := viewingOf(t, owner); d["member_id"] != tm.agentID || d["viewing"] != true {
		t.Fatalf("already there for the owner: %v", d)
	}
	if d := viewingOf(t, agent); d["member_id"] != ownerID || d["viewing"] != true {
		t.Fatalf("owner opened, for the agent: %v", d)
	}

	sendFrame(t, owner, map[string]any{"type": "viewing", "conversation_id": secret})
	if d := viewingOf(t, agent); d["member_id"] != ownerID || d["conversation_id"] != conv || d["viewing"] != false {
		t.Fatalf("owner moved on, for the agent: %v", d)
	}
	sendFrame(t, agent, map[string]any{"type": "viewing", "conversation_id": secret})
	sendFrame(t, agent, map[string]any{"type": "viewing", "conversation_id": nil})
	if d := viewingOf(t, owner); d["member_id"] != tm.agentID || d["conversation_id"] != conv || d["viewing"] != false {
		t.Fatalf("agent left: %v", d)
	}

	h.clock.Advance(time.Second)
	sendFrame(t, agent, map[string]any{"type": "viewing", "conversation_id": secret})
	var stored *string
	for deadline := time.Now().Add(5 * time.Second); ; {
		var seen time.Time
		err := h.st.Pool.QueryRow(context.Background(),
			"SELECT seen_at, viewing_conversation_id::text FROM realtime_connections WHERE member_id = $1", tm.agentID).Scan(&seen, &stored)
		if err != nil {
			t.Fatal(err)
		}
		if seen.Equal(h.clock.Now()) || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if stored != nil {
		t.Fatalf("stored a conversation the agent cannot see: %s", *stored)
	}
	sendFrame(t, owner, map[string]any{"type": "viewing", "conversation_id": secret})
	sendFrame(t, agent, map[string]any{"type": "viewing", "conversation_id": conv})
	if d := viewingOf(t, owner); d["member_id"] != tm.agentID || d["conversation_id"] != conv || d["viewing"] != true {
		t.Fatalf("agent back: %v", d)
	}
	_ = agent.conn.CloseNow()
	if d := viewingOf(t, owner); d["member_id"] != tm.agentID || d["viewing"] != false {
		t.Fatalf("agent disconnected: %v", d)
	}
	for {
		select {
		case m := <-stranger.msgs:
			if m.WorkspaceID != other.ws || m.Type == "viewing" {
				t.Fatalf("another workspace's member got %s of %s", m.Type, m.WorkspaceID)
			}
			continue
		case <-time.After(300 * time.Millisecond):
		}
		break
	}
}

func TestStats(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	ownerID := tm.ownerMemberID(t)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	since := h.clock.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)

	open := func(inbox string) string {
		conv := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox, "contact_id": tm.contact}).str("id")
		post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "help"})
		return conv
	}
	a := open(tm.inbox)
	h.clock.Advance(599 * time.Second)
	post(tm.agent, a, map[string]any{"kind": "message", "body": "on it"})
	post(tm.owner, a, map[string]any{"kind": "message", "body": "and me"})
	post(tm.owner, a, map[string]any{"kind": "note", "body": "a note is no reply"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+a, map[string]any{"status": "closed"})
	b := open(hidden)
	h.clock.Advance(1199 * time.Second)
	post(tm.owner, b, map[string]any{"kind": "message", "body": "late"})
	draft := post(key, b, map[string]any{"kind": "message", "body": "a bot's draft a member sends", "draft": true})
	tm.owner.expect(http.StatusOK, "POST", "/v1/messages/"+draft+"/send", nil)

	type stats struct {
		MedianFirstReplySeconds *int64 `json:"median_first_reply_seconds"`
		Members                 []struct {
			MemberID        string `json:"member_id"`
			Replies, Closed int64
		}
	}
	get := func(c *client, query string) (stats, map[string]any) {
		t.Helper()
		r := c.expect(http.StatusOK, "GET", "/v1/stats?since="+url.QueryEscape(since)+query, nil)
		var out stats
		if err := json.Unmarshal(r.raw, &out); err != nil {
			t.Fatal(err)
		}
		return out, r.body
	}
	o, raw := get(tm.owner, "")
	if raw["replies"] != float64(4) || raw["closed"] != float64(1) || raw["first_replies"] != float64(2) || *o.MedianFirstReplySeconds != 900 || len(o.Members) != 2 {
		t.Fatalf("owner stats %s", mustMarshal(raw))
	}
	for _, m := range o.Members {
		if (m.MemberID == ownerID && (m.Replies != 3 || m.Closed != 1)) || (m.MemberID == tm.agentID && (m.Replies != 1 || m.Closed != 0)) {
			t.Fatalf("per member %+v", o.Members)
		}
	}
	ag, raw := get(tm.agent, "")
	if raw["replies"] != float64(2) || raw["first_replies"] != float64(1) || *ag.MedianFirstReplySeconds != 600 {
		t.Fatalf("agent stats %s", mustMarshal(raw))
	}
	_, raw = get(tm.owner, "&inbox_id="+hidden)
	if raw["replies"] != float64(2) || raw["closed"] != float64(0) || raw["median_first_reply_seconds"] != float64(1200) {
		t.Fatalf("one inbox %s", mustMarshal(raw))
	}
	if _, raw = get(key, ""); raw["replies"] != float64(4) {
		t.Fatalf("key stats %s", mustMarshal(raw))
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/stats?inbox_id="+hidden, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/stats?inbox_id="+tm.inbox, nil)
	if _, raw = get(other.owner, ""); raw["replies"] != float64(0) || raw["median_first_reply_seconds"] != nil {
		t.Fatalf("another workspace %s", mustMarshal(raw))
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/stats?timezone=Mars/Base", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/stats?since="+url.QueryEscape(h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339)), nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/stats?since=2000-01-01T00:00:00Z", nil)

	r := tm.owner.expect(http.StatusOK, "GET", "/v1/stats?timezone=Asia/Tokyo", nil)
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	got, _ := time.Parse(time.RFC3339, r.str("since"))
	now := h.clock.Now().In(tokyo)
	if want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tokyo); !got.Equal(want) {
		t.Fatalf("default since %s, want %s", got, want)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+tm.agentID, nil)
	if o, raw = get(tm.owner, ""); raw["replies"] != float64(4) || len(o.Members) != 1 || o.Members[0].MemberID != ownerID {
		t.Fatalf("stats after a member was removed %s", mustMarshal(raw))
	}
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contactPresenceOf(t *testing.T, w *wsClient, contact string) map[string]any {
	t.Helper()
	for {
		m := w.nextOf("contact.presence")
		var d map[string]any
		if err := json.Unmarshal(m.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d["contact_id"] == contact {
			return d
		}
	}
}

func TestContactPresenceEvents(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	outsider := newTeam(t, h)
	cs := ct.session(h, map[string]any{})
	cs.start("hello")
	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+ct.chatInbox+"/members/"+ct.agentID, nil)

	owner := ct.owner.dial("")
	owner.ready()
	agent := ct.agent.dial("")
	agent.ready()
	stranger := outsider.owner.dial("")
	stranger.ready()

	visitor, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	visitor.ready()
	if d := contactPresenceOf(t, owner, cs.contactID); d["online"] != true || d["last_seen_at"] == nil {
		t.Fatalf("contact connected: %v", d)
	}
	_ = visitor.conn.CloseNow()
	if d := contactPresenceOf(t, owner, cs.contactID); d["online"] != false {
		t.Fatalf("contact disconnected: %v", d)
	}

	if _, err := h.st.Pool.Exec(context.Background(),
		"INSERT INTO realtime_connections (id, workspace_id, contact_id, seen_at) VALUES (gen_random_uuid(), $1, $2, $3)",
		ct.ws, cs.contactID, h.clock.Now().Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.AnnounceLapsedPresence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := contactPresenceOf(t, owner, cs.contactID); d["online"] != false {
		t.Fatalf("contact whose server died: %v", d)
	}

	for _, w := range []*wsClient{agent, stranger} {
	drain:
		for {
			select {
			case m := <-w.msgs:
				if m.Type == "contact.presence" {
					t.Fatalf("a member who cannot see the contact got %s", m.Data)
				}
			case <-time.After(300 * time.Millisecond):
				break drain
			}
		}
	}
}

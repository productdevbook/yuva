package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
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

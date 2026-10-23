package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (h *harness) runSlackScrubs(ws string) (snoozed int) {
	h.t.Helper()
	for _, j := range h.jobs("slack_scrub", ws) {
		var a api.SlackScrubArgs
		_ = json.Unmarshal(j.args, &a)
		wait, err := h.srv.ScrubSlackPost(context.Background(), a)
		if err != nil {
			h.t.Fatal(err)
		}
		if wait > 0 {
			snoozed++
			continue
		}
		h.dropJob(j.id)
	}
	return snoozed
}

func TestSlackPostsFollowRedactionTrashAndDeletion(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	key := tm.apiKey(h)
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{
		"new_conversations_channel_id": "C0SUPPORT", "mentions_channel_id": "G0MENTIONS", "include_content": true,
	})
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)

	newPost := func(subject, body string) (conv, msg, ts string) {
		t.Helper()
		conv = tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": subject}).str("id")
		msg = key.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": body}).str("id")
		h.runSlackPosts(tm.ws)
		posts := fake.take("chat.postMessage")
		if len(posts) != 1 || !strings.Contains(posts[0].Get("text"), body) {
			t.Fatalf("post: %v", posts)
		}
		var channel string
		if err := h.st.Pool.QueryRow(context.Background(), "SELECT channel_id, ts FROM slack_posts WHERE workspace_id = $1 AND conversation_id = $2", tm.ws, conv).Scan(&channel, &ts); err != nil {
			t.Fatalf("post not recorded: %v", err)
		}
		return conv, msg, ts
	}
	updates := func() []map[string]string {
		t.Helper()
		var out []map[string]string
		for _, f := range fake.take("chat.update") {
			out = append(out, map[string]string{"channel": f.Get("channel"), "ts": f.Get("ts"), "text": f.Get("text")})
		}
		return out
	}

	conv, msg, ts := newPost("Refund for Ayşe", "order 4711 secret")
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/messages/"+msg+"/redact", nil)
	if len(h.jobs("slack_scrub", tm.ws)) != 0 {
		t.Fatal("a refused redaction queued a rewrite")
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/messages/"+msg+"/redact", nil)
	h.runSlackScrubs(tm.ws)
	got := updates()
	if len(got) != 1 || got[0]["channel"] != "C0SUPPORT" || got[0]["ts"] != ts ||
		!strings.Contains(got[0]["text"], "Message removed") || !strings.Contains(got[0]["text"], "/conversations/"+conv) ||
		strings.Contains(got[0]["text"], "4711") || strings.Contains(got[0]["text"], "Refund") {
		t.Fatalf("redaction update: %v", got)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/messages/"+msg+"/redact", nil)
	if len(h.jobs("slack_scrub", tm.ws)) != 0 {
		t.Fatal("a second redaction rewrote the post again")
	}

	conv, _, ts = newPost("Trash me", "trash secret")
	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	fake.mu.Lock()
	fake.updateRetryAfter = 60
	fake.mu.Unlock()
	if n := h.runSlackScrubs(tm.ws); n != 1 {
		t.Fatalf("rate limit snoozed %d", n)
	}
	fake.mu.Lock()
	fake.updateRetryAfter = 0
	fake.mu.Unlock()
	updates()
	h.runSlackScrubs(tm.ws)
	got = updates()
	if len(got) != 1 || got[0]["ts"] != ts || !strings.Contains(got[0]["text"], "in the trash") || !strings.Contains(got[0]["text"], "/conversations/"+conv) ||
		strings.Contains(got[0]["text"], "secret") || strings.Contains(got[0]["text"], "Trash me") || strings.Contains(got[0]["text"], "Ayşe") {
		t.Fatalf("trash update: %v", got)
	}

	conv, _, ts = newPost("Erase me", "erased secret")
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+tm.contact, nil)
	h.runSlackScrubs(tm.ws)
	got = updates()
	if len(got) != 1 || got[0]["ts"] != ts || !strings.Contains(got[0]["text"], "was deleted") || strings.Contains(got[0]["text"], "/conversations/") ||
		strings.Contains(got[0]["text"], "secret") || strings.Contains(got[0]["text"], "Erase me") {
		t.Fatalf("contact deletion update: %v", got)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/slack", nil)
	fake.take("auth.revoke")
	var left int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM slack_posts WHERE workspace_id = $1", tm.ws).Scan(&left); err != nil || left != 0 {
		t.Fatalf("posts kept after disconnect: %d %v", left, err)
	}
	_ = conv
}

func TestSlackScrubSkipsDisconnectedSlack(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	a := api.SlackScrubArgs{WorkspaceID: uuid.MustParse(tm.ws), ChannelID: "C0SUPPORT", TS: "1700000000.000001", Reason: "deleted"}
	if wait, err := h.srv.ScrubSlackPost(context.Background(), a); err != nil || wait != 0 {
		t.Fatalf("disconnected: %v %v", wait, err)
	}
	if n := len(fake.take("chat.update")); n != 0 {
		t.Fatalf("updates for a disconnected workspace: %d", n)
	}
}

func TestSlackPostsFollowInboxAndWorkspaceDeletion(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	key := tm.apiKey(h)
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"new_conversations_channel_id": "C0SUPPORT", "include_content": true})
	post := func(inbox, subject string) {
		t.Helper()
		conv := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox, "contact_id": tm.contact, "subject": subject}).str("id")
		key.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "direction": "in", "body": "secret " + subject})
		h.runSlackPosts(tm.ws)
		if n := len(fake.take("chat.postMessage")); n != 1 {
			t.Fatalf("posts %d", n)
		}
	}
	post(tm.inbox, "first")
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox, nil)
	h.runSlackScrubs(tm.ws)
	if got := fake.take("chat.update"); len(got) != 1 || !strings.Contains(got[0].Get("text"), "was deleted") {
		t.Fatalf("inbox deletion updates: %v", got)
	}

	other := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Second", "slug": "second"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+other+"/slack", map[string]any{"new_conversations_channel_id": "C0SUPPORT", "include_content": true})
	post(other, "second")
	name := tm.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).str("name")
	tm.owner.expect(http.StatusAccepted, "DELETE", "/v1/workspace", map[string]any{"name": name})
	if p, err := h.srv.PurgeWorkspace(context.Background(), uuid.MustParse(tm.ws), time.Minute); err != nil || !p.Done {
		t.Fatalf("purge %v %v", p, err)
	}
	if got := fake.take("chat.update"); len(got) != 1 || !strings.Contains(got[0].Get("text"), "was deleted") {
		t.Fatalf("workspace deletion updates: %v", got)
	}
}

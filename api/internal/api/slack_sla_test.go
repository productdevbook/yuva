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

func (h *harness) runSlackSLAPosts(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("slack_sla_post", ws) {
		var a api.SlackSLAArgs
		_ = json.Unmarshal(j.args, &a)
		if _, err := h.srv.PostSLAToSlack(context.Background(), a); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func TestSlackSLAPosts(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	ct := newChatTeam(t, h, "live", true)
	other := newChatTeam(t, h, "live", true)
	ct.policy(map[string]any{"name": "Chat", "first_reply_minutes": 30, "warn_minutes": 5})
	other.policy(map[string]any{"name": "Chat", "first_reply_minutes": 30, "warn_minutes": 5})
	if got := connectSlack(t, h, ct.owner, ct.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	path := "/v1/inboxes/" + ct.chatInbox + "/slack"

	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path, map[string]any{"sla_channel_id": "C0ELSEWHERE"})
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PUT", path, map[string]any{"sla_channel_id": "general"})
	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "PUT", path, map[string]any{"sla_channel_id": "G0MENTIONS"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PUT", path, map[string]any{"sla_channel_id": "G0MENTIONS"})
	other.owner.expectProblem(http.StatusConflict, "slack_not_connected", "PUT", "/v1/inboxes/"+other.chatInbox+"/slack", map[string]any{"sla_channel_id": "G0MENTIONS"})
	set := ct.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_channel_id": "G0MENTIONS"})
	if set.str("sla_channel_id") != "G0MENTIONS" || set.body["sla_warnings"] != false || set.body["include_content"] != false {
		t.Fatalf("settings: %s", set.raw)
	}

	otherConv := other.session(h, map[string]any{}).start("Other workspace")
	conv := ct.session(h, map[string]any{}).start("private order 4711")
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"assignee_id": ct.agentID})
	h.clock.Advance(26 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.runSLAChecks(other.ws)
	if n := len(h.jobs("slack_sla_post", ct.ws)); n != 0 {
		t.Fatalf("a warning queued %d posts while sla_warnings is off", n)
	}
	h.clock.Advance(7 * time.Minute)
	h.runSLAChecks(ct.ws)
	h.runSLAChecks(other.ws)
	h.runSLAChecks(ct.ws)
	if n := len(h.jobs("slack_sla_post", ct.ws)); n != 1 {
		t.Fatalf("a breach queued %d posts", n)
	}
	if n := len(h.jobs("slack_sla_post", other.ws)); n != 0 {
		t.Fatalf("another workspace's breach queued %d posts", n)
	}
	h.runSlackSLAPosts(ct.ws)
	h.runSlackSLAPosts(other.ws)
	posts := fake.take("chat.postMessage")
	if len(posts) != 1 || posts[0].Get("channel") != "G0MENTIONS" {
		t.Fatalf("posts: %v", posts)
	}
	text := posts[0].Get("text")
	if !strings.Contains(text, "SLA breached") || !strings.Contains(text, "first reply, 3m late") || !strings.Contains(text, "Assigned to") ||
		!strings.Contains(text, testOrigin+"/conversations/"+conv) || strings.Contains(text, "4711") {
		t.Fatalf("breach text: %q", text)
	}
	if strings.Contains(text, otherConv) {
		t.Fatalf("another workspace's conversation in the post: %q", text)
	}
	var thread string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT thread_ts FROM slack_threads WHERE workspace_id = $1 AND conversation_id = $2 AND channel_id = 'G0MENTIONS'", ct.ws, conv).Scan(&thread); err != nil {
		t.Fatalf("the breach post is not a reply target: %v", err)
	}
	slackReply(t, h, fake.team, ct.ws, "G0MENTIONS", thread, "U0STRANGER", "mine")
	if eph := fake.take("chat.postEphemeral"); len(eph) != 1 {
		t.Fatalf("a reply in the breach thread was not handled like other threads: %v", eph)
	}

	ct.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_channel_id": "G0MENTIONS", "sla_warnings": true, "include_content": true})
	warned := ct.session(h, map[string]any{}).start("second order 9911")
	h.clock.Advance(26 * time.Minute)
	h.runSLAChecks(ct.ws)
	if n := len(h.jobs("slack_sla_post", ct.ws)); n != 1 {
		t.Fatalf("a warning queued %d posts with sla_warnings on", n)
	}
	h.runSlackSLAPosts(ct.ws)
	posts = fake.take("chat.postMessage")
	if len(posts) != 1 || !strings.Contains(posts[0].Get("text"), "SLA warning") || !strings.Contains(posts[0].Get("text"), "9911") || !strings.Contains(posts[0].Get("text"), warned) {
		t.Fatalf("warning posts: %v", posts)
	}

	ct.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_channel_id": nil})
	h.clock.Advance(31 * time.Minute)
	h.runSLAChecks(ct.ws)
	if n := len(h.jobs("slack_sla_post", ct.ws)); n != 0 {
		t.Fatalf("a cleared channel still queued %d posts", n)
	}
}

func TestSlackSLASettingsSurvivePartialPut(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	path := "/v1/inboxes/" + tm.inbox + "/slack"
	tm.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_channel_id": "G0MENTIONS", "sla_warnings": true})
	kept := tm.owner.expect(http.StatusOK, "PUT", path, map[string]any{"new_conversations_channel_id": "C0SUPPORT"})
	if kept.str("sla_channel_id") != "G0MENTIONS" || kept.body["sla_warnings"] != true || kept.str("new_conversations_channel_id") != "C0SUPPORT" {
		t.Fatalf("a PUT without the SLA fields changed them: %s", kept.raw)
	}
	off := tm.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_warnings": false})
	if off.str("sla_channel_id") != "G0MENTIONS" || off.body["sla_warnings"] != false {
		t.Fatalf("explicit false: %s", off.raw)
	}
	cleared := tm.owner.expect(http.StatusOK, "PUT", path, map[string]any{"sla_channel_id": nil})
	if cleared.body["sla_channel_id"] != nil {
		t.Fatalf("explicit null: %s", cleared.raw)
	}
}

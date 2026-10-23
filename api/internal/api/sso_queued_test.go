package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

// requireSSOFor gives the member an address in a domain the workspace verified and turns
// sso_required on, as the settings endpoints would; it returns the settings row's id.
func requireSSOFor(t *testing.T, h *harness, ws, memberID string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	domain := unique("corp") + ".test"
	settings := uuid.New()
	if _, err := h.st.Pool.Exec(ctx, `UPDATE people SET email = 'member@' || $3::text
		WHERE id = (SELECT person_id FROM members WHERE workspace_id = $1 AND id = $2)`, ws, memberID, domain); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO sso_settings (workspace_id, id, protocol, oidc_issuer, oidc_client_id, oidc_client_secret, sso_required, created_at, updated_at)
		VALUES ($1, $2, 'oidc', 'https://idp.example', 'client', '\x00', true, now(), now())`, ws, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO sso_domains (id, workspace_id, domain, token, verified_at, created_at)
		VALUES ($1, $2, $3, 'yuva-verify-test', now(), now())`, uuid.New(), ws, domain); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestSlackReplyNeedsSSOForCoveredMember(t *testing.T) {
	t.Parallel()
	fake := newFakeSlack(t)
	h := slackHarness(t, fake)
	tm := newTeam(t, h)
	if got := connectSlack(t, h, tm.owner, tm.owner, "good"); got != "connected" {
		t.Fatalf("install: %s", got)
	}
	tm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+tm.inbox+"/slack", map[string]any{"new_conversations_channel_id": "C0SUPPORT"})
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact, "subject": "Refund"}).str("id")
	post(tm.apiKey(h), conv, map[string]any{"kind": "message", "direction": "in", "body": "where is my refund"})
	h.runSlackPosts(tm.ws)
	if posts := fake.take("chat.postMessage"); len(posts) != 1 {
		t.Fatalf("posts: %v", posts)
	}
	thread := "1700000000.000001"
	fake.users["agent-sso@example.com"] = "U0AGENT"
	tm.agent.expect(http.StatusAccepted, "POST", "/v1/me/slack/link", map[string]any{"email": "agent-sso@example.com"})
	code := regexp.MustCompile(`\*([0-9]{6})\*`).FindStringSubmatch(fake.take("chat.postMessage")[0].Get("text"))
	tm.agent.expect(http.StatusOK, "POST", "/v1/me/slack/link/confirm", map[string]any{"code": code[1]})

	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0AGENT", "a note before the rule")
	if n := len(messages(tm.owner, conv)); n != 2 {
		t.Fatalf("note before sso_required: %d messages", n)
	}
	fake.take("chat.postEphemeral")

	requireSSOFor(t, h, tm.ws, tm.agentID)
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0AGENT", "!reply sent without SSO")
	slackReply(t, h, fake.team, tm.ws, "C0SUPPORT", thread, "U0AGENT", "a note without SSO")
	eph := fake.take("chat.postEphemeral")
	if len(eph) != 2 || eph[0].Get("user") != "U0AGENT" || !strings.Contains(eph[0].Get("text"), "single sign-on") || !strings.Contains(eph[1].Get("text"), "single sign-on") {
		t.Fatalf("ephemeral: %v", eph)
	}
	if n := len(messages(tm.owner, conv)); n != 2 {
		t.Fatalf("a covered member wrote from Slack without SSO: %d messages", n)
	}
}

func TestQueuedLinkedReplyNeedsSSOForCoveredMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	rt := newRoadmapTeam(t, h, true)
	idea := rt.idea(rt.owner, "Dark mode")
	conv := rt.feedback(h)
	rt.owner.expect(http.StatusNoContent, "PUT", "/v1/discussions/"+idea+"/conversations/"+conv, nil)

	withoutSSO := rt.agent.expect(http.StatusAccepted, "POST", "/v1/discussions/"+idea+"/linked-replies", map[string]any{"body": "Queued before the rule"})
	viaSSO := rt.agent.expect(http.StatusAccepted, "POST", "/v1/discussions/"+idea+"/linked-replies", map[string]any{"body": "Queued through the provider"})
	settings := requireSSOFor(t, h, rt.ws, rt.agentID)
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE linked_replies SET sso_settings_id = $1 WHERE workspace_id = $2 AND id = $3",
		settings, rt.ws, viaSSO.str("id")); err != nil {
		t.Fatal(err)
	}
	for _, j := range h.jobs("linked_reply", rt.ws) {
		var a api.LinkedReplyArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		if err := h.srv.RunLinkedReply(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		h.dropJob(j.id)
	}
	refused := rt.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+idea+"/linked-replies/"+withoutSSO.str("id"), nil)
	if refused.body["counts"].(map[string]any)["failed"] != float64(1) || !strings.Contains(string(refused.raw), `"reason":"sso_required"`) {
		t.Fatalf("reply queued without SSO: %s", refused.raw)
	}
	sent := rt.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+idea+"/linked-replies/"+viaSSO.str("id"), nil)
	if sent.body["counts"].(map[string]any)["sent"] != float64(1) {
		t.Fatalf("reply queued through the provider: %s", sent.raw)
	}
	var bodies []string
	for _, m := range messages(rt.owner, conv) {
		if m["kind"] == "message" && m["direction"] == "out" {
			bodies = append(bodies, m["body"].(string))
		}
	}
	if len(bodies) != 1 || bodies[0] != "Queued through the provider" {
		t.Fatalf("replies that reached the feedback: %v", bodies)
	}
}

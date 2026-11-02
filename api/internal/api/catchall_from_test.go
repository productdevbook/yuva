package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

type catchAllTeam struct {
	team
	domain, channel string
}

func newCatchAllTeam(t *testing.T, h *harness) catchAllTeam {
	t.Helper()
	tm := newTeam(t, h)
	domain := unique("shop") + ".example"
	smtp := map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"}
	ch := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+tm.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Shop", "email": map[string]any{"address": "*@" + domain, "display_name": "Shop", "smtp": smtp},
	})
	billing := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Billing", "slug": "billing"}).body["inbox"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+billing+"/channels", map[string]any{
		"kind": "email", "name": "Billing", "email": map[string]any{"address": "billing@" + domain, "smtp": smtp},
	})
	return catchAllTeam{team: tm, domain: domain, channel: ch.str("id")}
}

func (ct catchAllTeam) start(c *client, status int, from string) response {
	body := map[string]any{"inbox_id": ct.inbox, "contact_id": ct.contact, "channel_id": ct.channel, "subject": "Order"}
	if from != "" {
		body["from_address"] = from
	}
	return c.expect(status, "POST", "/v1/conversations", body)
}

func sentFrom(t *testing.T, h *harness, ws, message string) string {
	t.Helper()
	h.sendQueued(t, ws, message)
	sent, parsed := h.smtp.last(t)
	if from := parsed.Header.Get("From"); !strings.Contains(from, "<"+sent.from+">") || parsed.Header.Get("Reply-To") != "Shop <"+sent.from+">" {
		t.Fatalf("envelope %s, From %q, Reply-To %q", sent.from, from, parsed.Header.Get("Reply-To"))
	}
	return sent.from
}

func (ct catchAllTeam) send(c *client, draft string) string {
	return c.expect(http.StatusOK, "POST", "/v1/messages/"+draft+"/send", nil).str("id")
}

func TestCatchAllFromAddress(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newCatchAllTeam(t, h)
	ct.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+ct.inbox+"/members/"+ct.agentID, nil)
	agentAddr := "ann@" + ct.domain
	id := ct.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": agentAddr}).str("id")
	ct.owner.expect(http.StatusOK, "POST", "/v1/members/"+ct.agentID+"/addresses/"+id+"/approve", nil)

	ct.start(ct.owner, http.StatusCreated, "")
	for _, from := range []string{"hello@elsewhere.example", "billing@" + ct.domain, "*@" + ct.domain, agentAddr} {
		ct.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations", map[string]any{
			"inbox_id": ct.inbox, "contact_id": ct.contact, "channel_id": ct.channel, "from_address": from,
		})
	}
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations", map[string]any{
		"inbox_id": ct.inbox, "contact_id": ct.contact, "from_address": "hello@" + ct.domain,
	})

	conv := ct.start(ct.owner, http.StatusCreated, "Hello@"+strings.ToUpper(ct.domain)).str("id")
	hello := "hello@" + ct.domain
	if got := sentFrom(t, h, ct.ws, ct.send(ct.owner, post(ct.owner, conv, map[string]any{"kind": "message", "body": "First", "draft": true}))); got != hello {
		t.Fatalf("first mail from %s", got)
	}
	if got := sentFrom(t, h, ct.ws, post(ct.owner, conv, map[string]any{"kind": "message", "body": "Second", "from_address": "orders@" + ct.domain})); got != "orders@"+ct.domain {
		t.Fatalf("a reply with its own from_address went from %s", got)
	}
	if got := sentFrom(t, h, ct.ws, post(ct.owner, conv, map[string]any{"kind": "message", "body": "Third"})); got != hello {
		t.Fatalf("a from_address on one reply changed the conversation: next from %s", got)
	}
	ct.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "from_address": "orders@elsewhere.example"})
	ct.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "from_address": agentAddr})

	setDeliverability(t, h, ct.ws, ct.channel, "received", checkResult(ct.domain, true, "pass"))
	mine := post(ct.agent, conv, map[string]any{"kind": "message", "body": "From Ann", "from_address": agentAddr})
	h.sendQueued(t, ct.ws, mine)
	sent, parsed := h.smtp.last(t)
	if sent.from != hello || !strings.Contains(parsed.Header.Get("From"), agentAddr) || !strings.Contains(parsed.Header.Get("Reply-To"), hello) {
		t.Fatalf("member's own address: envelope %s, From %q, Reply-To %q", sent.from, parsed.Header.Get("From"), parsed.Header.Get("Reply-To"))
	}
	var memberAddress *string
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT member_address_id::text FROM message_emails WHERE workspace_id = $1 AND message_id = $2", ct.ws, mine).Scan(&memberAddress); err != nil || memberAddress == nil || *memberAddress != id {
		t.Fatalf("member address of the mail: %v %v", memberAddress, err)
	}

	key := ct.apiKey(h)
	keyConv := ct.start(key, http.StatusCreated, "team@"+ct.domain).str("id")
	for _, from := range []string{"team@elsewhere.example", agentAddr} {
		key.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/conversations/"+keyConv+"/messages", map[string]any{
			"kind": "message", "body": "x", "draft": true, "from_address": from,
		})
	}
	keyDraft := post(key, keyConv, map[string]any{"kind": "message", "body": "From the key", "draft": true, "from_address": "care@" + ct.domain})
	if got := sentFrom(t, h, ct.ws, ct.send(ct.owner, keyDraft)); got != "care@"+ct.domain {
		t.Fatalf("a key's draft went from %s", got)
	}
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	if got := sentFrom(t, h, ct.ws, post(key, keyConv, map[string]any{"kind": "message", "body": "Key reply", "from_address": "desk@" + ct.domain})); got != "desk@"+ct.domain {
		t.Fatalf("a key's reply went from %s", got)
	}
	if got := sentFrom(t, h, ct.ws, post(key, keyConv, map[string]any{"kind": "message", "body": "Key again"})); got != "team@"+ct.domain {
		t.Fatalf("the key's next reply went from %s", got)
	}

	other := newCatchAllTeam(t, h)
	other.owner.expect(http.StatusNotFound, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "from_address": "hello@" + ct.domain})
}

func TestCatchAllDraftsKeepTheirFrom(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newCatchAllTeam(t, h)

	conv := ct.start(ct.owner, http.StatusCreated, "").str("id")
	ct.owner.expectProblem(http.StatusConflict, "email_no_sender", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "draft": true})
	ct.owner.expectProblem(http.StatusConflict, "email_no_sender", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x"})
	post(ct.owner, conv, map[string]any{"kind": "note", "body": "a note needs no sender"})

	a := post(ct.owner, conv, map[string]any{"kind": "message", "body": "A", "draft": true, "from_address": "a@" + ct.domain})
	b := post(ct.owner, conv, map[string]any{"kind": "message", "body": "B", "draft": true, "from_address": "b@" + ct.domain})
	if r := ct.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+b, map[string]any{"from_address": "bee@" + ct.domain}); r.str("draft_from_address") != "bee@"+ct.domain {
		t.Fatalf("draft_from_address after PATCH: %s", r.raw)
	}
	ct.owner.expectProblem(http.StatusConflict, "from_not_allowed", "PATCH", "/v1/messages/"+b, map[string]any{"from_address": "bee@elsewhere.example"})
	if r := ct.owner.expect(http.StatusOK, "POST", "/v1/messages/"+b+"/send", nil); r.body["draft_from_address"] != nil {
		t.Fatalf("a sent message shows draft_from_address: %s", r.raw)
	} else if got := sentFrom(t, h, ct.ws, r.str("id")); got != "bee@"+ct.domain {
		t.Fatalf("draft B went from %s", got)
	}
	if got := sentFrom(t, h, ct.ws, ct.send(ct.owner, a)); got != "a@"+ct.domain {
		t.Fatalf("draft A went from %s", got)
	}
	var address *string
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT email_address FROM conversations WHERE workspace_id = $1 AND id = $2", ct.ws, conv).Scan(&address); err != nil || address != nil {
		t.Fatalf("the drafts changed the conversation's address: %v %v", address, err)
	}
	ct.owner.expectProblem(http.StatusConflict, "email_no_sender", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x"})

	c := post(ct.owner, conv, map[string]any{"kind": "message", "body": "C", "draft": true, "from_address": "c@" + ct.domain})
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+c, map[string]any{"from_address": nil})
	ct.owner.expectProblem(http.StatusConflict, "email_no_sender", "POST", "/v1/messages/"+c+"/send", nil)
	if got := sentFrom(t, h, ct.ws, ct.owner.expect(http.StatusOK, "POST", "/v1/messages/"+c+"/send", map[string]any{"from_address": "late@" + ct.domain}).str("id")); got != "late@"+ct.domain {
		t.Fatalf("draft C went from %s", got)
	}

	ct.owner.expectProblem(http.StatusConflict, "email_no_sender", "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": ct.inbox, "channel_id": ct.channel, "to": []string{"buyer@example.net"}, "subject": "Hi", "body": "Hi", "draft": true,
	})
	composed := ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": ct.inbox, "channel_id": ct.channel, "to": []string{"buyer@example.net"}, "subject": "Hi", "body": "Hi", "draft": true,
		"from_address": "news@" + ct.domain,
	})
	newsConv := composed.body["conversation"].(map[string]any)["id"].(string)
	if got := sentFrom(t, h, ct.ws, ct.send(ct.owner, composed.body["message"].(map[string]any)["id"].(string))); got != "news@"+ct.domain {
		t.Fatalf("composed draft went from %s", got)
	}
	if got := sentFrom(t, h, ct.ws, post(ct.owner, newsConv, map[string]any{"kind": "message", "body": "Follow-up"})); got != "news@"+ct.domain {
		t.Fatalf("the reply after a composed mail went from %s", got)
	}

	et := newEmailTeam(t, h, false)
	plain := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel}).str("id")
	d := post(et.owner, plain, map[string]any{"kind": "message", "body": "x", "draft": true})
	et.owner.expectProblem(http.StatusConflict, "from_not_allowed", "PATCH", "/v1/messages/"+d, map[string]any{"from_address": "x@example.com"})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations", map[string]any{"inbox_id": et.inbox, "contact_id": et.contact, "channel_id": et.channel, "from_address": "x@example.com"})
}

func TestMCPCatchAllFromAddress(t *testing.T) {
	h := newHarness(t)
	ct := newCatchAllTeam(t, h)
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"bots_may_send": true})
	scopes := []string{"conversations:read", "conversations:write", "messages:write", "contacts:read", "contacts:write", "drafts:send"}
	cs := h.mcpSession(h.oauthToken(ct.owner, ct.ws, "Claude Code", testOrigin+"/mcp", scopes))
	start := map[string]any{"inbox_id": ct.inbox, "contact_id": ct.contact, "channel_id": ct.channel, "subject": "Welcome", "body": "Hello"}
	if _, errText := callTool(t, cs, "start_conversation", start); !strings.Contains(errText, "email_no_sender") {
		t.Fatalf("start_conversation without a sender: %q", errText)
	}
	var n int
	if err := h.st.Pool.QueryRow(t.Context(), "SELECT count(*) FROM conversations WHERE workspace_id = $1", ct.ws).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a refused start_conversation left %d conversations: %v", n, err)
	}
	start["from_address"] = "welcome@" + ct.domain
	out, errText := callTool(t, cs, "start_conversation", start)
	if errText != "" {
		t.Fatal(errText)
	}
	conv := out["conversation"].(map[string]any)["id"].(string)
	if _, errText := callTool(t, cs, "discard_draft", map[string]any{"message_id": out["message"].(map[string]any)["id"]}); errText != "" {
		t.Fatal(errText)
	}
	draft, errText := callTool(t, cs, "draft_reply", map[string]any{"conversation_id": conv, "body": "More", "from_address": "first@" + ct.domain})
	if errText != "" {
		t.Fatal(errText)
	}
	if _, errText := callTool(t, cs, "update_draft", map[string]any{"message_id": draft["id"], "from_address": "x@elsewhere.example"}); !strings.Contains(errText, "from_not_allowed") {
		t.Fatalf("update_draft from another domain: %q", errText)
	}
	if _, errText := callTool(t, cs, "update_draft", map[string]any{"message_id": draft["id"], "from_address": "more@" + ct.domain}); errText != "" {
		t.Fatal(errText)
	}
	sent, errText := callTool(t, cs, "send_draft", map[string]any{"message_id": draft["id"]})
	if errText != "" {
		t.Fatal(errText)
	}
	if got := sentFrom(t, h, ct.ws, sent["id"].(string)); got != "more@"+ct.domain {
		t.Fatalf("send_draft went from %s", got)
	}
	if _, errText := callTool(t, cs, "send_reply", map[string]any{"conversation_id": conv, "body": "x", "from_address": "x@elsewhere.example"}); !strings.Contains(errText, "from_not_allowed") {
		t.Fatalf("send_reply from another domain: %q", errText)
	}
	reply, errText := callTool(t, cs, "send_reply", map[string]any{"conversation_id": conv, "body": "y", "from_address": "reply@" + ct.domain})
	if errText != "" {
		t.Fatal(errText)
	}
	if got := sentFrom(t, h, ct.ws, reply["id"].(string)); got != "reply@"+ct.domain {
		t.Fatalf("send_reply went from %s", got)
	}
	mail, errText := callTool(t, cs, "compose_mail", map[string]any{"inbox_id": ct.inbox, "channel_id": ct.channel, "to": []string{"buyer@example.net"}, "subject": "News", "body": "News", "from_address": "news@" + ct.domain})
	if errText != "" {
		t.Fatal(errText)
	}
	if got := sentFrom(t, h, ct.ws, mail["message"].(map[string]any)["id"].(string)); got != "news@"+ct.domain {
		t.Fatalf("compose_mail went from %s", got)
	}
}

func TestCatchAllDraftFromIsWhatSends(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newCatchAllTeam(t, h)
	ct.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+ct.inbox+"/members/"+ct.agentID, nil)
	ctx := context.Background()

	pending := "pat@" + ct.domain
	ct.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": pending})
	conv := ct.start(ct.owner, http.StatusCreated, "hello@"+ct.domain).str("id")
	if r := ct.agent.expect(http.StatusConflict, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "from_address": pending}); !strings.Contains(string(r.raw), "is not one of your verified addresses") {
		t.Fatalf("the agent's own unverified address fell through to the domain rule: %s", r.raw)
	}

	h.sendQueued(t, ct.ws, post(ct.owner, conv, map[string]any{"kind": "message", "body": "First"}))
	fwd := ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/forward", map[string]any{"to": []string{"success@simulator.amazonses.com"}, "draft": true}).str("id")
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+fwd, map[string]any{"from_address": "a@" + ct.domain})
	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+fwd, nil)

	scheduled := post(ct.owner, conv, map[string]any{"kind": "message", "body": "Later", "draft": true, "from_address": "early@" + ct.domain})
	ct.owner.expect(http.StatusOK, "POST", "/v1/messages/"+scheduled+"/send", map[string]any{"send_at": h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	if r := ct.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+scheduled, map[string]any{"from_address": "Late@" + ct.domain}); r.str("draft_from_address") != "late@"+ct.domain || r.body["scheduled_at"] == nil {
		t.Fatalf("PATCH of a scheduled draft's from_address: %s", r.raw)
	}
	job := h.scheduledJob(t, ct.ws, scheduled)
	h.clock.Advance(time.Hour)
	if err := h.srv.SendScheduled(ctx, job, false); err != nil {
		t.Fatal(err)
	}
	if got := sentFrom(t, h, ct.ws, scheduled); got != "late@"+ct.domain {
		t.Fatalf("the scheduled draft went from %s", got)
	}

	bare := ct.start(ct.owner, http.StatusCreated, "").str("id")
	agentAddr := "ann@" + ct.domain
	id := ct.agent.expect(http.StatusAccepted, "POST", "/v1/me/addresses", map[string]any{"address": agentAddr}).str("id")
	ct.owner.expect(http.StatusOK, "POST", "/v1/members/"+ct.agentID+"/addresses/"+id+"/approve", nil)
	setDeliverability(t, h, ct.ws, ct.channel, "received", checkResult(ct.domain, true, "pass"))
	own := ct.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+bare+"/messages", map[string]any{"kind": "message", "body": "Mine", "draft": true, "from_address": agentAddr})
	if own.str("draft_from_address") != agentAddr {
		t.Fatalf("a draft keeping the member's own address: %s", own.raw)
	}
	h.sendQueued(t, ct.ws, ct.send(ct.agent, own.str("id")))
	if sent, parsed := h.smtp.last(t); sent.from != agentAddr || !strings.Contains(parsed.Header.Get("From"), agentAddr) {
		t.Fatalf("member's own address on a conversation without sender: envelope %s, From %q", sent.from, parsed.Header.Get("From"))
	}

	stale := post(ct.owner, conv, map[string]any{"kind": "message", "body": "Stale", "draft": true, "from_address": "old@" + ct.domain})
	help := "help@" + ct.domain
	ct.owner.expect(http.StatusOK, "PATCH", "/v1/channels/"+ct.channel, map[string]any{"email": map[string]any{"address": help,
		"smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"}}})
	ct.owner.expectProblem(http.StatusConflict, "from_not_allowed", "POST", "/v1/messages/"+stale+"/send", map[string]any{"from_address": "old@" + ct.domain})
	h.sendQueued(t, ct.ws, ct.send(ct.owner, stale))
	if sent, _ := h.smtp.last(t); sent.from != help {
		t.Fatalf("a stale draft from_address was used: %s", sent.from)
	}
}

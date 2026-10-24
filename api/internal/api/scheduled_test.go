package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func draftIDs(c *client, query string) []string {
	c.h.t.Helper()
	var out []string
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/drafts"+query, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any)["draft"].(map[string]any)["id"].(string))
	}
	return out
}

func conversationIDs(c *client, query string) []string {
	c.h.t.Helper()
	var out []string
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/conversations"+query, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func eventTypesFor(c *client, id string) []string {
	c.h.t.Helper()
	var out []string
	for _, e := range c.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil).body["events"].([]any) {
		ev := e.(map[string]any)
		if d, ok := ev["data"].(map[string]any); ok && d["id"] == id {
			out = append(out, ev["type"].(string))
		}
	}
	return out
}

func (h *harness) scheduledJob(t *testing.T, ws, message string) api.ScheduledSendArgs {
	t.Helper()
	var found *api.ScheduledSendArgs
	for _, j := range h.jobs("scheduled_send", ws) {
		var a api.ScheduledSendArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			t.Fatal(err)
		}
		if a.MessageID.String() == message {
			found = &a
		}
	}
	if found == nil {
		t.Fatalf("no scheduled_send job for %s", message)
	}
	return *found
}

func TestMemberDraftsArePrivate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	key := tm.apiKey(h)
	conv := tm.conversation(tm.owner)
	mine := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "owner's draft", "draft": true}).str("id")
	bot := key.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "bot draft", "draft": true}).str("id")

	if got := draftIDs(tm.owner, "?conversation_id="+conv); len(got) != 2 || !has(got, mine) || !has(got, bot) {
		t.Fatalf("owner drafts %v", got)
	}
	if got := draftIDs(tm.owner, "?mine=true"); len(got) != 1 || got[0] != mine {
		t.Fatalf("mine=true %v", got)
	}
	for name, c := range map[string]*client{"agent": tm.agent, "key": key} {
		if got := draftIDs(c, "?conversation_id="+conv); len(got) != 1 || got[0] != bot {
			t.Fatalf("%s sees drafts %v", name, got)
		}
		for _, m := range messages(c, conv) {
			if m["id"] == mine {
				t.Fatalf("%s sees the owner's draft in the message list", name)
			}
		}
		c.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/messages/"+mine, map[string]any{"body": "taken"})
		if r := c.do("POST", "/v1/messages/"+mine+"/send", map[string]any{}); r.status != http.StatusNotFound && (c != key || r.status != http.StatusForbidden) {
			t.Fatalf("%s sends the owner's draft: %d %s", name, r.status, r.raw)
		}
		c.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/messages/"+mine, nil)
		if ev := eventTypesFor(c, mine); len(ev) != 0 {
			t.Fatalf("%s got events of the owner's draft: %v", name, ev)
		}
	}
	if ev := eventTypesFor(tm.owner, mine); !has(ev, "draft.created") {
		t.Fatalf("owner events of own draft: %v", ev)
	}
	if ev := eventTypesFor(tm.agent, bot); !has(ev, "draft.created") {
		t.Fatalf("agent events of the bot draft: %v", ev)
	}
	tm.agent.expect(http.StatusOK, "PATCH", "/v1/messages/"+bot, map[string]any{"body": "bot draft, edited"})

	cs := h.mcpSession(h.oauthToken(tm.agent, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"inboxes:read", "conversations:read", "contacts:read"}))
	drafts, errText := callTool(t, cs, "list_drafts", map[string]any{"inbox_id": tm.inbox})
	if errText != "" || drafts["total"] != 1.0 {
		t.Fatalf("agent list_drafts: %v %s", drafts, errText)
	}

	tm.owner.expect(http.StatusOK, "POST", "/v1/messages/"+mine+"/send", map[string]any{})
	found := false
	for _, m := range messages(tm.agent, conv) {
		found = found || m["id"] == mine
	}
	if !found {
		t.Fatal("the sent message is not visible to the agent")
	}
}

func TestComposedMemberDraftHidesItsConversation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	subject := unique("Hidden plan")
	countOf := func(c *client) float64 {
		n, _ := c.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["all"].(float64)
		return n
	}
	before := countOf(et.agent)
	r := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": subject, "body": "Draft body", "draft": true,
	})
	conv := r.body["conversation"].(map[string]any)["id"].(string)
	msg := r.body["message"].(map[string]any)["id"].(string)

	et.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	if has(conversationIDs(et.agent, ""), conv) || has(conversationIDs(et.agent, "?q="+url.QueryEscape(subject)), conv) {
		t.Fatal("the agent lists the composed draft's conversation")
	}
	if n := countOf(et.agent); n != before {
		t.Fatalf("agent count %v, was %v", n, before)
	}
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending", "assignee_id": et.agentID})
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "@agent look", "mentions": []string{et.agentID}})
	if ev := eventTypesFor(et.agent, conv); len(ev) != 0 {
		t.Fatalf("agent got events of the hidden conversation: %v", ev)
	}
	if ev := eventTypesFor(et.owner, conv); !has(ev, "conversation.updated") {
		t.Fatalf("author events of the hidden conversation: %v", ev)
	}
	var public int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM events WHERE workspace_id = $1 AND conversation_id = $2 AND only_member IS NULL",
		uuid.MustParse(et.ws), uuid.MustParse(conv)).Scan(&public); err != nil || public != 0 {
		t.Fatalf("%d events of the hidden conversation reach others: %v", public, err)
	}
	for _, kind := range []string{"notify", "automation_run", "webhook_fanout"} {
		for _, j := range h.jobs(kind, et.ws) {
			if strings.Contains(string(j.args), conv) {
				t.Fatalf("a %s job for the hidden conversation: %s", kind, j.args)
			}
		}
	}
	if !has(conversationIDs(et.owner, "?q="+url.QueryEscape(subject)), conv) {
		t.Fatal("the author does not find the conversation")
	}
	et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)

	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+msg+"/send", map[string]any{})
	et.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if !has(conversationIDs(et.agent, "?q="+url.QueryEscape(subject)), conv) {
		t.Fatal("the sent conversation is not found by the agent")
	}
	if ev := eventTypesFor(et.agent, conv); !has(ev, "conversation.created") {
		t.Fatalf("agent events after the send: %v", ev)
	}
}

func TestScheduledSend(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{"signature": "Kind regards from Acme"})
	ctx := context.Background()
	now := h.clock.Now()
	at := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	compose := map[string]any{"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "Scheduled hello", "body": "See you soon"}
	with := func(m map[string]any, k string, v any) map[string]any {
		out := map[string]any{k: v}
		for kk, vv := range m {
			out[kk] = vv
		}
		return out
	}

	et.owner.expectProblem(http.StatusBadRequest, "invalid_send_at", "POST", "/v1/conversations/mail", with(compose, "send_at", at(30*time.Second)))
	et.owner.expectProblem(http.StatusBadRequest, "invalid_send_at", "POST", "/v1/conversations/mail", with(compose, "send_at", at(367*24*time.Hour)))
	key := et.apiKey(h)
	key.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/mail", with(compose, "send_at", at(time.Hour)))
	if r := et.owner.do("POST", "/v1/conversations/mail", with(with(compose, "send_at", at(time.Hour)), "draft", false)); r.status != http.StatusBadRequest {
		t.Fatalf("draft:false with send_at: %d %s", r.status, r.raw)
	}

	r := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", with(compose, "send_at", at(time.Hour)))
	conv := r.body["conversation"].(map[string]any)["id"].(string)
	msg := r.body["message"].(map[string]any)
	id := msg["id"].(string)
	if msg["draft"] != true || msg["scheduled_at"] == nil {
		t.Fatalf("scheduled message %v", msg)
	}
	if got := draftIDs(et.owner, "?scheduled=true"); len(got) != 1 || got[0] != id {
		t.Fatalf("scheduled=true %v", got)
	}
	if got := draftIDs(et.owner, "?scheduled=false"); has(got, id) {
		t.Fatalf("scheduled=false %v", got)
	}
	if got := conversationIDs(et.owner, "?scheduled=mine"); len(got) != 1 || got[0] != conv {
		t.Fatalf("scheduled=mine %v", got)
	}
	key.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/conversations?scheduled=mine", nil)
	job := h.scheduledJob(t, et.ws, id)

	h.clock.Advance(time.Hour)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { errs[i] = h.srv.SendScheduled(ctx, job, false) })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if jobs := h.jobs("email_send", et.ws); len(jobs) != 1 {
		t.Fatalf("racing workers queued %d sends", len(jobs))
	}
	h.sendQueued(t, et.ws, id)
	if n := h.smtp.count(); n != 1 {
		t.Fatalf("%d mails sent", n)
	}
	sent, parsed := h.smtp.last(t)
	if parsed.Header.Get("Subject") != "Scheduled hello" || !strings.Contains(string(sent.raw), "Kind regards from Acme") || sent.from != et.address {
		t.Fatalf("sent %s %v", sent.from, parsed.Header)
	}
	var m map[string]any
	for _, x := range messages(et.owner, conv) {
		if x["id"] == id {
			m = x
		}
	}
	if m == nil || m["draft"] != false || m["scheduled_at"] != nil {
		t.Fatalf("after the send %v", m)
	}
	if err := h.srv.SendScheduled(ctx, job, false); err != nil || h.smtp.count() != 1 || len(h.jobs("email_send", et.ws)) != 1 {
		t.Fatalf("a repeated job sent again: %v", err)
	}
	if ev := eventTypesFor(et.owner, id); !has(ev, "message.created") {
		t.Fatalf("events %v", ev)
	}
}

func TestScheduledSendMovedCancelledDiscarded(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	ctx := context.Background()
	now := h.clock.Now()
	at := func(d time.Duration) string { return now.Add(d).UTC().Format(time.RFC3339) }
	conv := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "Thread", "body": "First",
	}).body["conversation"].(map[string]any)["id"].(string)
	sends := len(h.jobs("email_send", et.ws))
	schedule := func(d time.Duration) string {
		return et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
			"kind": "message", "body": "Later", "send_at": at(d), "cc": []string{"success@simulator.amazonses.com"},
		}).str("id")
	}

	moved := schedule(time.Hour)
	old := h.scheduledJob(t, et.ws, moved)
	r := et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+moved, map[string]any{"send_at": at(2 * time.Hour)})
	if got, _ := time.Parse(time.RFC3339, r.str("scheduled_at")); !got.Equal(now.Add(2 * time.Hour).Truncate(time.Second)) {
		t.Fatalf("moved to %s", r.str("scheduled_at"))
	}
	et.owner.expectProblem(http.StatusBadRequest, "invalid_send_at", "PATCH", "/v1/messages/"+moved, map[string]any{"send_at": at(0)})

	cancelled := schedule(time.Hour)
	cjob := h.scheduledJob(t, et.ws, cancelled)
	r = et.owner.expect(http.StatusOK, "DELETE", "/v1/messages/"+cancelled+"/schedule", nil)
	if r.body["scheduled_at"] != nil || r.body["draft"] != true {
		t.Fatalf("cancelled %v", r.body)
	}

	discarded := schedule(time.Hour)
	djob := h.scheduledJob(t, et.ws, discarded)
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+discarded, nil)

	h.clock.Advance(time.Hour)
	for _, j := range []api.ScheduledSendArgs{old, cjob, djob} {
		if err := h.srv.SendScheduled(ctx, j, false); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(h.jobs("email_send", et.ws)); n != sends {
		t.Fatalf("%d sends queued after move, cancel and discard, want %d", n, sends)
	}
	h.clock.Advance(time.Hour)
	if err := h.srv.SendScheduled(ctx, h.scheduledJob(t, et.ws, moved), false); err != nil {
		t.Fatal(err)
	}
	if n := len(h.jobs("email_send", et.ws)); n != sends+1 {
		t.Fatalf("the moved message did not go at its new time: %d", n)
	}
}

func (h *harness) sendError(t *testing.T, ws, id string) (bool, *time.Time, string) {
	t.Helper()
	var (
		draft   bool
		sched   *time.Time
		sendErr []byte
	)
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT draft, scheduled_at, send_error FROM messages WHERE workspace_id = $1 AND id = $2",
		uuid.MustParse(ws), uuid.MustParse(id)).Scan(&draft, &sched, &sendErr); err != nil {
		t.Fatal(err)
	}
	return draft, sched, string(sendErr)
}

func (h *harness) conversationExists(t *testing.T, ws, id string) bool {
	t.Helper()
	var n int
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM conversations WHERE workspace_id = $1 AND id = $2",
		uuid.MustParse(ws), uuid.MustParse(id)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestScheduledSendOfRemovedMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	ctx := context.Background()
	conv := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "Shared thread", "body": "Hi",
	}).body["conversation"].(map[string]any)["id"].(string)
	sends := len(h.jobs("email_send", et.ws))
	id := et.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Later", "send_at": h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}).str("id")
	hidden := et.agent.upload("/v1/conversations/mail", map[string]string{
		"inbox_id": et.inbox, "to": "success@simulator.amazonses.com", "subject": "Agent's unsent mail", "body": "Draft", "draft": "true",
	}, []file{{"plan.txt", "text/plain", []byte("secret plan")}})
	if hidden.status != http.StatusCreated {
		t.Fatalf("composed draft: %d %s", hidden.status, hidden.raw)
	}
	hiddenConv := hidden.body["conversation"].(map[string]any)["id"].(string)
	job := h.scheduledJob(t, et.ws, id)
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+et.agentID, nil)
	if h.conversationExists(t, et.ws, hiddenConv) {
		t.Fatal("the removed member's hidden draft conversation is still there")
	}
	if len(h.jobs("storage_delete", et.ws)) == 0 {
		t.Fatal("the hidden draft's stored files are not deleted")
	}
	h.clock.Advance(time.Hour)
	if err := h.srv.SendScheduled(ctx, job, false); err != nil {
		t.Fatal(err)
	}
	if draft, sched, sendErr := h.sendError(t, et.ws, id); !draft || sched != nil || !strings.Contains(sendErr, "member_inactive") {
		t.Fatalf("draft %v scheduled %v send_error %s", draft, sched, sendErr)
	}
	if n := len(h.jobs("email_send", et.ws)); n != sends {
		t.Fatalf("mail went out for a removed member: %d", n)
	}
}

func TestScheduledSendIntoTrash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	ctx := context.Background()
	conv := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "Soon trashed", "body": "Hi",
	}).body["conversation"].(map[string]any)["id"].(string)
	sends := len(h.jobs("email_send", et.ws))
	id := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Later", "send_at": h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}).str("id")
	job := h.scheduledJob(t, et.ws, id)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	h.clock.Advance(time.Hour)
	if err := h.srv.SendScheduled(ctx, job, false); err != nil {
		t.Fatal(err)
	}
	if draft, sched, sendErr := h.sendError(t, et.ws, id); !draft || sched != nil || !strings.Contains(sendErr, "conversation_trashed") {
		t.Fatalf("draft %v scheduled %v send_error %s", draft, sched, sendErr)
	}
	if n := len(h.jobs("email_send", et.ws)); n != sends {
		t.Fatalf("mail went out into the trash: %d", n)
	}
	if len(h.jobs("scheduled_failed_notify", et.ws)) != 1 {
		t.Fatal("the author is not told")
	}
}

func TestDiscardingComposedDraftDeletesItsConversation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	count := func() float64 {
		n, _ := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["all"].(float64)
		return n
	}
	before := count()
	r := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": "Never mind", "body": "Draft", "draft": true,
	})
	conv := r.body["conversation"].(map[string]any)["id"].(string)
	msg := r.body["message"].(map[string]any)["id"].(string)
	if count() != before+1 {
		t.Fatal("the author does not count their composed draft")
	}
	et.owner.expect(http.StatusNoContent, "DELETE", "/v1/messages/"+msg, nil)
	if h.conversationExists(t, et.ws, conv) {
		t.Fatal("the discarded draft's conversation is still there")
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	if n := count(); n != before {
		t.Fatalf("count %v after the discard, was %v", n, before)
	}
	if ev := eventTypesFor(et.owner, conv); !has(ev, "conversation.deleted") {
		t.Fatalf("author events after the discard: %v", ev)
	}
}

func TestPrivateDraftAttachmentsAndClientIDs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	up := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{"kind": "message", "body": "draft with file", "draft": "true", "client_id": "same-1"},
		[]file{{"a.txt", "text/plain", []byte("hello")}})
	if up.status != http.StatusCreated {
		t.Fatalf("upload: %d %s", up.status, up.raw)
	}
	draft := up.str("id")
	att := up.body["attachments"].([]any)[0].(map[string]any)["id"].(string)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att, nil)
	tm.owner.expect(http.StatusOK, "GET", "/v1/attachments/"+att, nil)

	r := tm.agent.do("POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "agent note", "client_id": "same-1"})
	if r.status != http.StatusCreated || r.str("id") == draft || strings.Contains(string(r.raw), "draft with file") {
		t.Fatalf("another member's client_id answered with the private draft: %d %s", r.status, r.raw)
	}
	if again := tm.owner.do("POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "draft with file", "draft": true, "client_id": "same-1"}); again.status >= 300 || again.str("id") != draft {
		t.Fatalf("the author's retry: %d %s", again.status, again.raw)
	}
}

func TestScheduledCopiesAndCursor(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	tm := newTeam(t, h)
	at := func(d time.Duration) string { return h.clock.Now().Add(d).UTC().Format(time.RFC3339) }

	chat := tm.conversation(tm.owner)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+chat+"/messages", map[string]any{
		"kind": "message", "body": "x", "send_at": at(time.Hour), "cc": []string{"success@simulator.amazonses.com"},
	})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+chat+"/messages", map[string]any{
		"kind": "message", "body": "x", "send_at": at(time.Hour), "from_address": "someone@example.com",
	})
	var convs []string
	for range 3 {
		convs = append(convs, et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
			"inbox_id": et.inbox, "to": []string{"success@simulator.amazonses.com"}, "subject": unique("Thread"), "body": "Hi",
		}).body["conversation"].(map[string]any)["id"].(string))
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+convs[0]+"/messages", map[string]any{
		"kind": "message", "body": "x", "send_at": at(time.Hour), "cc": []string{"not an address"},
	})
	many := make([]string, 51)
	for i := range many {
		many[i] = "success+" + strconv.Itoa(i) + "@simulator.amazonses.com"
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+convs[0]+"/messages", map[string]any{
		"kind": "message", "body": "x", "send_at": at(time.Hour), "bcc": many,
	})
	if got := draftIDs(et.owner, "?scheduled=true"); len(got) != 0 {
		t.Fatalf("a refused schedule left drafts: %v", got)
	}
	for i, d := range []time.Duration{3 * time.Hour, time.Hour, 2 * time.Hour} {
		et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+convs[i]+"/messages", map[string]any{"kind": "message", "body": "Later", "send_at": at(d)})
	}
	r := page(et.owner, "/v1/conversations?scheduled=mine&limit=2")
	var got []string
	for _, it := range pageItems(r) {
		got = append(got, it["id"].(string))
	}
	next := r.str("next_cursor")
	if len(got) != 2 || got[0] != convs[1] || got[1] != convs[2] || next == "" {
		t.Fatalf("first page %v next %q", got, next)
	}
	r = page(et.owner, "/v1/conversations?scheduled=mine&limit=2&cursor="+url.QueryEscape(next))
	if items := pageItems(r); len(items) != 1 || items[0]["id"] != convs[0] {
		t.Fatalf("second page %v", items)
	}
}

func TestHiddenConversationOnClientAPI(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	conv := cs.start("hi")
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE conversations SET draft_of = $3 WHERE workspace_id = $1 AND id = $2",
		uuid.MustParse(ct.ws), uuid.MustParse(conv), uuid.MustParse(ct.agentID)); err != nil {
		t.Fatal(err)
	}
	for _, it := range cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any) {
		if it.(map[string]any)["id"] == conv {
			t.Fatal("the contact lists a hidden conversation")
		}
	}
	cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/conversations/"+conv, nil)
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.ready()
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "pending"})
	ct.agent.expect(http.StatusNoContent, "POST", "/v1/conversations/"+conv+"/typing", map[string]any{"typing": true})
	ws.quiet(500*time.Millisecond, "presence")
}

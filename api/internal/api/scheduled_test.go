package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
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
	if first, _ := time.Parse(time.RFC3339, pageItems(r)[0]["scheduled_at"].(string)); !first.Equal(h.clock.Now().Add(time.Hour).Truncate(time.Second)) {
		t.Fatalf("scheduled_at %v", first)
	}
	if _, ok := itemByID(listItems(et.owner, ""), convs[0])["scheduled_at"]; ok {
		t.Fatal("scheduled_at without scheduled=mine")
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

func TestComposedDraftRecipients(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	a, b, c, d := "success+a@simulator.amazonses.com", "success+b@simulator.amazonses.com", "success+c@simulator.amazonses.com", "success+d@simulator.amazonses.com"
	recipients := func(m map[string]any) map[string]any {
		r, _ := m["recipients"].(map[string]any)
		return r
	}
	list := func(r map[string]any, k string) []string {
		var out []string
		for _, v := range r[k].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	r := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{a}, "cc": []string{b}, "bcc": []string{c}, "subject": "First subject", "body": "Draft", "draft": true,
	})
	conv := r.body["conversation"].(map[string]any)["id"].(string)
	msg := r.body["message"].(map[string]any)
	id := msg["id"].(string)
	if rc := recipients(msg); rc == nil || !slices.Equal(list(rc, "to"), []string{a}) || !slices.Equal(list(rc, "cc"), []string{b}) || !slices.Equal(list(rc, "bcc"), []string{c}) {
		t.Fatalf("compose answer recipients %v", msg["recipients"])
	}
	reply := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+et.conversation(et.owner)+"/messages", map[string]any{"kind": "message", "body": "reply draft", "draft": true}).str("id")
	for _, it := range page(et.owner, "/v1/drafts").body["items"].([]any) {
		item := it.(map[string]any)
		dr := item["draft"].(map[string]any)
		switch dr["id"] {
		case id:
			if item["composed"] != true || recipients(dr) == nil {
				t.Fatalf("composed draft item %v", item)
			}
		case reply:
			if item["composed"] != false || recipients(dr) != nil {
				t.Fatalf("reply draft item %v", item)
			}
		}
	}
	for _, m := range messages(et.owner, conv) {
		if m["id"] == id && recipients(m) == nil {
			t.Fatal("the message list has no recipients for the composed draft")
		}
	}
	raw := et.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=500", nil).raw
	if strings.Contains(string(raw), "recipients") || strings.Contains(string(raw), c) {
		t.Fatal("an event carries the draft's recipients")
	}

	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+reply, map[string]any{"subject": "Nope"})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+id, map[string]any{"cc": []string{"not an address"}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+id, map[string]any{"to": []string{}})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+id, map[string]any{"subject": " "})
	many := make([]string, 51)
	for i := range many {
		many[i] = "success+" + strconv.Itoa(i) + "@simulator.amazonses.com"
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/messages/"+id, map[string]any{"bcc": many})
	et.agent.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/messages/"+id, map[string]any{"subject": "Taken"})

	p := et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+id, map[string]any{"to": []string{strings.ToUpper(d), a}, "bcc": []string{}, "subject": "  New subject "})
	if rc := recipients(p.body); !slices.Equal(list(rc, "to"), []string{d, a}) || !slices.Equal(list(rc, "cc"), []string{b}) || len(list(rc, "bcc")) != 0 {
		t.Fatalf("after the edit %v", p.body["recipients"])
	}
	cv := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if cv.str("subject") != "New subject" {
		t.Fatalf("conversation subject %q", cv.str("subject"))
	}
	if cv.str("contact_id") == r.body["conversation"].(map[string]any)["contact_id"] {
		t.Fatal("the conversation's contact did not follow the first To")
	}

	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+id+"/send", map[string]any{})
	h.sendQueued(t, et.ws, id)
	_, parsed := h.smtp.last(t)
	if parsed.Header.Get("Subject") != "New subject" || !strings.Contains(parsed.Header.Get("To"), d) || !strings.Contains(parsed.Header.Get("Cc"), b) {
		t.Fatalf("sent %v", parsed.Header)
	}
	et.owner.expectProblem(http.StatusConflict, "not_a_draft", "PATCH", "/v1/messages/"+id, map[string]any{"to": []string{a}})
	for _, m := range messages(et.owner, conv) {
		if m["id"] == id && recipients(m) != nil {
			t.Fatal("a sent message still shows draft recipients")
		}
	}
}

func addrs(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "success+" + prefix + strconv.Itoa(i) + "@simulator.amazonses.com"
	}
	return out
}

func recipientList(m map[string]any, k string) []string {
	r, _ := m["recipients"].(map[string]any)
	var out []string
	if r == nil {
		return nil
	}
	for _, v := range r[k].([]any) {
		out = append(out, v.(string))
	}
	return out
}

func TestScheduledComposedDraftKeepsCopiesInItsLists(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	ctx := context.Background()
	a, b, c, d := "success+a@simulator.amazonses.com", "success+b@simulator.amazonses.com", "success+c@simulator.amazonses.com", "success+d@simulator.amazonses.com"
	id := et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{a}, "cc": []string{b}, "subject": "Later mail", "body": "Draft", "draft": true,
	}).body["message"].(map[string]any)["id"].(string)
	at := h.clock.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	r := et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+id+"/send", map[string]any{"send_at": at, "cc": []string{strings.ToUpper(c), a}, "bcc": []string{d}})
	if !slices.Equal(recipientList(r.body, "cc"), []string{c}) || !slices.Equal(recipientList(r.body, "bcc"), []string{d}) || !slices.Equal(recipientList(r.body, "to"), []string{a}) {
		t.Fatalf("schedule answer recipients %v", r.body["recipients"])
	}
	var override int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE workspace_id = $1 AND id = $2 AND (scheduled_cc IS NOT NULL OR scheduled_bcc IS NOT NULL)",
		uuid.MustParse(et.ws), uuid.MustParse(id)).Scan(&override); err != nil || override != 0 {
		t.Fatalf("a composed draft keeps a copy override: %d %v", override, err)
	}
	u := et.owner.expect(http.StatusOK, "DELETE", "/v1/messages/"+id+"/schedule", nil)
	if !slices.Equal(recipientList(u.body, "cc"), []string{c}) {
		t.Fatalf("unschedule answer recipients %v", u.body["recipients"])
	}
	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+id+"/send", map[string]any{"send_at": at})
	if p := et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+id, map[string]any{"cc": []string{b}}); !slices.Equal(recipientList(p.body, "cc"), []string{b}) {
		t.Fatalf("after the edit %v", p.body["recipients"])
	}
	h.clock.Advance(time.Hour)
	if err := h.srv.SendScheduled(ctx, h.scheduledJob(t, et.ws, id), false); err != nil {
		t.Fatal(err)
	}
	h.sendQueued(t, et.ws, id)
	sent, parsed := h.smtp.last(t)
	if got := parsed.Header.Get("Cc"); !strings.Contains(got, b) || strings.Contains(got, c) {
		t.Fatalf("sent Cc %q", got)
	}
	if !slices.Contains(sent.to, d) {
		t.Fatalf("the Bcc kept with the schedule was dropped: %v", sent.to)
	}
}

func TestComposedDraftToEditsLeaveNoContacts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	et.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+et.inbox+"/members/"+et.agentID, nil)
	ctx := context.Background()
	known := "success+known@simulator.amazonses.com"
	et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{"inbox_id": et.inbox, "to": []string{known}, "subject": "Earlier", "body": "Hi"})
	contactIDs := func() []string {
		var out []string
		for _, it := range page(et.owner, "/v1/contacts?limit=100").body["items"].([]any) {
			out = append(out, it.(map[string]any)["id"].(string))
		}
		return out
	}
	before := contactIDs()
	r := et.agent.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success+one@simulator.amazonses.com"}, "subject": "Private", "body": "Draft", "draft": true,
	})
	conv := r.body["conversation"].(map[string]any)["id"].(string)
	id := r.body["message"].(map[string]any)["id"].(string)
	for _, to := range []string{"success+two@simulator.amazonses.com", known, "success+three@simulator.amazonses.com"} {
		et.agent.expect(http.StatusOK, "PATCH", "/v1/messages/"+id, map[string]any{"to": []string{to}})
	}
	contact := et.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
	et.agent.expect(http.StatusOK, "PATCH", "/v1/messages/"+id, map[string]any{"cc": []string{"success+copy@simulator.amazonses.com"}})
	if got := et.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id"); got != contact {
		t.Fatalf("a cc edit changed the contact: %s, was %s", got, contact)
	}
	after := contactIDs()
	if len(after) != len(before)+1 || !has(after, contact) {
		t.Fatalf("contacts after three To edits: %d, before %d", len(after), len(before))
	}
	for _, id := range before {
		if !has(after, id) {
			t.Fatalf("an earlier contact %s was deleted", id)
		}
	}
	var n int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM contact_emails WHERE workspace_id = $1 AND email = ANY($2)",
		uuid.MustParse(et.ws), []string{"success+one@simulator.amazonses.com", "success+two@simulator.amazonses.com"}).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d contacts of replaced To addresses are left: %v", n, err)
	}
}

func TestKeyComposedDraftRecipients(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	key := et.apiKey(h)
	other := et.apiKey(h)
	hidden := "success+hidden@simulator.amazonses.com"
	id := key.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
		"inbox_id": et.inbox, "to": []string{"success+to@simulator.amazonses.com"}, "bcc": []string{hidden}, "subject": "From the bot", "body": "Draft", "draft": true,
	}).body["message"].(map[string]any)["id"].(string)
	key.expectProblem(http.StatusConflict, "not_a_private_draft", "PATCH", "/v1/messages/"+id, map[string]any{"subject": "Changed"})
	et.owner.expectProblem(http.StatusConflict, "not_a_private_draft", "PATCH", "/v1/messages/"+id, map[string]any{"to": []string{"success+x@simulator.amazonses.com"}})
	key.expect(http.StatusOK, "PATCH", "/v1/messages/"+id, map[string]any{"body": "Edited"})
	bccFor := func(c *client) []string {
		for _, it := range page(c, "/v1/drafts").body["items"].([]any) {
			item := it.(map[string]any)
			if dr := item["draft"].(map[string]any); dr["id"] == id {
				if item["composed"] != true || dr["recipients"] == nil {
					t.Fatalf("key draft item %v", item)
				}
				return recipientList(dr, "bcc")
			}
		}
		t.Fatal("the key's draft is not listed")
		return nil
	}
	if got := bccFor(key); !slices.Equal(got, []string{hidden}) {
		t.Fatalf("the authoring key sees bcc %v", got)
	}
	if got := bccFor(et.owner); !slices.Equal(got, []string{hidden}) {
		t.Fatalf("a member session sees bcc %v", got)
	}
	if got := bccFor(other); len(got) != 0 {
		t.Fatalf("another key sees bcc %v", got)
	}
}

func TestComposedDraftRecipientBudget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	compose := func(to []string) string {
		return et.owner.expect(http.StatusCreated, "POST", "/v1/conversations/mail", map[string]any{
			"inbox_id": et.inbox, "to": to, "subject": unique("Many"), "body": "Draft", "draft": true,
		}).body["message"].(map[string]any)["id"].(string)
	}
	grown := compose(addrs("a", 1))
	et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+grown, map[string]any{"to": addrs("b", 50), "cc": addrs("c", 50), "bcc": addrs("d", 50)})
	full := compose(addrs("e", 50))
	et.owner.expect(http.StatusOK, "PATCH", "/v1/messages/"+full, map[string]any{"cc": addrs("f", 50), "bcc": addrs("g", 50)})
	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+grown+"/send", map[string]any{})
	et.owner.expect(http.StatusOK, "POST", "/v1/messages/"+full+"/send", map[string]any{})
	last := compose(addrs("h", 1))
	if r := et.owner.do("POST", "/v1/messages/"+last+"/send", map[string]any{}); r.status != http.StatusTooManyRequests {
		t.Fatalf("a send past the hourly recipient budget: %d %s", r.status, r.raw)
	}
	if got := draftIDs(et.owner, ""); !has(got, last) {
		t.Fatal("the refused send did not stay a draft")
	}
}

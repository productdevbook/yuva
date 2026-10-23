package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func (tm team) scopedKey(h *harness, scopes ...string) *client {
	k := h.client()
	k.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "trash", "scopes": scopes}).str("secret")
	return k
}

func listedIDs(c *client, query string) []string {
	c.h.t.Helper()
	var out []string
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/conversations?limit=100"+query, nil).body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func has(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func TestTrashWhoMayAndWhereItShows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	ownerID := tm.ownerMemberID(t)
	conv := tm.conversation(tm.owner)
	kept := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "zanzibar refund"})
	since := url.QueryEscape(h.clock.Now().Add(-time.Hour).Format(time.RFC3339))
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"})
	closedBefore := tm.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+since, nil).body["closed"]
	if closedBefore != float64(1) {
		t.Fatalf("stats before: %v", closedBefore)
	}
	homeBefore := homeBlock(homeOf(tm.owner), "inbox")["open"]

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/conversations/"+conv, nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/"+conv+"/restore", nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/conversations?trashed=true", nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv}, "trashed": true})
	plainKey := tm.apiKey(h)
	plainKey.expectProblem(http.StatusForbidden, "insufficient_scope", "DELETE", "/v1/conversations/"+conv, nil)
	plainKey.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/conversations?trashed=true", nil)

	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/conversations/"+conv, nil)
	other.scopedKey(h, "conversations:read", "conversations:delete").expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/restore", nil)

	eventsOf := func(id string) []string {
		var out []string
		for _, e := range tm.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=200", nil).body["events"].([]any) {
			if ev := e.(map[string]any); ev["conversation_id"] == id {
				out = append(out, ev["type"].(string))
			}
		}
		return out
	}
	if evs := eventsOf(conv); len(evs) < 3 {
		t.Fatalf("events before trash: %v", evs)
	}
	key := tm.scopedKey(h, "conversations:read", "conversations:write", "conversations:delete")
	trashed := key.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	if evs := eventsOf(conv); fmt.Sprint(evs) != "[conversation.trashed]" {
		t.Fatalf("events after trash: %v", evs)
	}
	at, err := time.Parse(time.RFC3339Nano, trashed.str("trashed_at"))
	if err != nil {
		t.Fatalf("trashed: %s", trashed.raw)
	}
	deletes, _ := time.Parse(time.RFC3339Nano, trashed.str("deletes_at"))
	by := trashed.body["trashed_by"].(map[string]any)
	if !deletes.Equal(at.Add(30*24*time.Hour)) || by["api_key_id"] == nil || by["member_id"] != nil {
		t.Fatalf("trashed: %s", trashed.raw)
	}
	again := tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	if again.str("trashed_at") != trashed.str("trashed_at") {
		t.Fatalf("second trash changed it: %s", again.raw)
	}

	for _, q := range []string{"", "&status=open", "&q=zanzibar", "&contact_id=" + tm.contact, "&inbox_id=" + tm.inbox} {
		if ids := listedIDs(tm.owner, q); has(ids, conv) || !has(ids, kept) && q != "&q=zanzibar" {
			t.Fatalf("list %q: %v", q, ids)
		}
	}
	if ids := listedIDs(tm.owner, "&trashed=true"); len(ids) != 1 || ids[0] != conv {
		t.Fatalf("trash view: %v", ids)
	}
	if ids := listedIDs(key, "&trashed=true&q=zanzibar"); len(ids) != 1 {
		t.Fatalf("trash search: %v", ids)
	}
	if ids := listedIDs(other.owner, "&trashed=true"); len(ids) != 0 {
		t.Fatalf("other workspace's trash: %v", ids)
	}
	if counts := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil); counts.body["all"] != float64(1) {
		t.Fatalf("counts: %s", counts.raw)
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+since, nil).body["closed"]; got != float64(0) {
		t.Fatalf("stats after trash: %v", got)
	}
	if got := homeBlock(homeOf(tm.owner), "inbox")["open"]; got != homeBefore.(float64)-1 {
		t.Fatalf("home open %v, before %v", got, homeBefore)
	}
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil); got.str("trashed_at") == "" {
		t.Fatalf("owner get: %s", got.raw)
	}
	tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv+"/messages", nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv+"/messages", nil)
	plainKey.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "x"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/read", nil)

	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read", "conversations:delete"}))
	found, text := callTool(t, cs, "search_conversations", map[string]any{"query": "zanzibar"})
	if text != "" || len(found["items"].([]any)) != 0 {
		t.Fatalf("mcp search: %v %s", found, text)
	}
	if _, text := callTool(t, cs, "get_conversation", map[string]any{"conversation_id": conv}); !strings.Contains(text, "no such conversation") {
		t.Fatalf("mcp get: %s", text)
	}

	restored := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/restore", nil)
	if _, ok := restored.body["trashed_at"]; ok || restored.body["trashed_by"] != nil || restored.body["deletes_at"] != nil || restored.str("status") != "open" {
		t.Fatalf("restored: %s", restored.raw)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/restore", nil)
	if ids := listedIDs(tm.agent, "&q=zanzibar"); len(ids) != 1 || ids[0] != conv {
		t.Fatalf("after restore: %v", ids)
	}
	byOwner := tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+kept, nil).body["trashed_by"].(map[string]any)
	if byOwner["member_id"] != ownerID || byOwner["api_key_id"] != nil {
		t.Fatalf("trashed by owner: %v", byOwner)
	}

	log := auditLog(t, tm.owner, "&action=conversation.trashed,conversation.restored")
	if len(log) != 3 {
		t.Fatalf("audit: %v", log)
	}
	if e := log[2]; e["action"] != "conversation.trashed" || actorOf(e)["kind"] != "api_key" || targetOf(e)["type"] != "conversation" || targetOf(e)["id"] != conv || detailsOf(e)["inbox_id"] != tm.inbox {
		t.Fatalf("trash entry %v", e)
	}
	if e := log[1]; e["action"] != "conversation.restored" || actorOf(e)["kind"] != "member" || actorOf(e)["id"] != ownerID {
		t.Fatalf("restore entry %v", e)
	}
	if len(auditLog(t, other.owner, "&action=conversation.trashed")) != 0 {
		t.Fatal("other workspace sees the entries")
	}
}

func TestBulkTrash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	a, b := tm.conversation(tm.owner), tm.conversation(tm.owner)
	other := newTeam(t, h)
	foreign := other.conversation(other.owner)

	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a}, "trashed": true, "status": "closed"})
	tm.scopedKey(h, "conversations:read", "conversations:write").expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/bulk",
		map[string]any{"conversation_ids": []string{a}, "trashed": true})
	deleter := tm.scopedKey(h, "conversations:read", "conversations:delete")
	deleter.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a}, "status": "closed"})
	r := deleter.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a, foreign, b}, "trashed": true})
	updated, failed := r.body["updated"].([]any), r.body["failed"].([]any)
	if len(updated) != 2 || len(failed) != 1 || failed[0].(map[string]any)["id"] != foreign || failed[0].(map[string]any)["code"] != "not_found" {
		t.Fatalf("bulk trash: %s", r.raw)
	}
	for _, u := range updated {
		if u.(map[string]any)["trashed_at"] == nil {
			t.Fatalf("bulk trash: %s", r.raw)
		}
	}
	other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+foreign, nil)
	st := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a}, "status": "closed"})
	if f := st.body["failed"].([]any); len(f) != 1 || f[0].(map[string]any)["code"] != "not_found" {
		t.Fatalf("status change in the trash: %s", st.raw)
	}
	back := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a, b}, "trashed": false})
	if len(back.body["updated"].([]any)) != 2 || len(listedIDs(tm.owner, "&trashed=true")) != 0 || len(listedIDs(tm.owner, "")) != 2 {
		t.Fatalf("bulk restore: %s", back.raw)
	}
}

func TestTrashedConversationLeavesContacts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	cs := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{"sub": "user-9"})})
	started := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations", map[string]any{"body": "first question", "subject": "Billing", "client_id": "first-1"})
	conv := started.body["conversation"].(map[string]any)["id"].(string)
	firstMessage := started.body["message"].(map[string]any)["id"].(string)
	ws, _, err := dialContact(h, cs.token, ct.origin, "")
	if err != nil {
		t.Fatal(err)
	}
	ws.nextOf("ready")
	ct.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "an answer"})
	before := len(messages(ct.owner, conv))
	ct.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	if m := ws.nextOf("conversation.removed"); m.ConversationID != conv || !strings.Contains(string(m.Data), conv) {
		t.Fatalf("removed frame: %+v", m)
	}
	ws.close()

	if items := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("client list: %v", items)
	}
	cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/conversations/"+conv, nil)
	cs.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/conversations/"+conv+"/messages", nil)
	cs.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/conversations/"+conv+"/read", nil)

	original := cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "first question", "client_id": "first-1"})
	if original.str("id") != firstMessage || original.str("conversation_id") != conv {
		t.Fatalf("retry of a message in the trash: %s", original.raw)
	}
	cs.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/conversations", map[string]any{"body": "first question", "client_id": "first-1"})
	if items := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any); len(items) != 0 {
		t.Fatalf("client list after retries: %v", items)
	}
	sent := cs.expect(http.StatusCreated, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "hello again", "client_id": "retry-1"})
	fresh := sent.str("conversation_id")
	if fresh == "" || fresh == conv {
		t.Fatalf("message into the trash: %s", sent.raw)
	}
	if retry := cs.expect(http.StatusOK, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "hello again", "client_id": "retry-1"}); retry.str("id") != sent.str("id") {
		t.Fatalf("retry: %s", retry.raw)
	}
	items := cs.expect(http.StatusOK, "GET", "/client/v1/conversations", nil).body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != fresh {
		t.Fatalf("client list after: %v", items)
	}
	if m := lastOf(messages(ct.owner, fresh), "message"); m["body"] != "hello again" {
		t.Fatalf("new conversation: %v", m)
	}
	if got := ct.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+fresh, nil); got.str("subject") != "Billing" || got.str("kind") != "conversation" {
		t.Fatalf("new conversation keeps the context: %s", got.raw)
	}
	if msgs := messages(ct.owner, conv); len(msgs) != before {
		t.Fatalf("trashed conversation changed: %v", msgs)
	}

	et := newEmailTeam(t, h, false)
	first := newMessageID()
	in := h.ingest(et.address, buildMail(mailOpts{from: "mail@example.net", to: et.address, subject: "Invoice", messageID: first, body: "Hi"}), nil)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+in.str("conversation_id"), nil)
	reply := h.ingest(et.address, buildMail(mailOpts{from: "mail@example.net", to: et.address, subject: "Re: Invoice", messageID: newMessageID(), body: "More",
		headers: map[string]string{"In-Reply-To": "<" + first + ">", "References": "<" + first + ">"}}), nil)
	if reply.status != http.StatusAccepted || reply.str("conversation_id") == "" || reply.str("conversation_id") == in.str("conversation_id") {
		t.Fatalf("reply to a trashed thread: %d %v (first %s)", reply.status, reply.body, in.str("conversation_id"))
	}
	if got := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+reply.str("conversation_id"), nil); got.body["related_conversation_id"] != nil {
		t.Fatalf("new conversation points to the trashed one: %s", got.raw)
	}
}

func TestTrashDeletedAfterThirtyDays(t *testing.T) {
	h := privateHarness(t)
	et := newEmailTeam(t, h, false)
	rcv := newReceiver(t)
	et.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "events": []string{"conversation.trashed", "conversation.restored", "conversation.deleted"},
	})
	in := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	conv := in.str("conversation_id")
	keys := h.storedKeys(et.ws)
	if len(keys) < 2 {
		t.Fatalf("stored files: %v", keys)
	}
	other := newTeam(t, h)
	otherTrashed, otherKept := other.conversation(other.owner), other.conversation(other.owner)
	key := et.scopedKey(h, "conversations:read", "conversations:delete", "audit:read")
	otherKey := other.scopedKey(h, "conversations:read", "audit:read")
	plainKey := et.apiKey(h)
	att := lastOf(messages(et.owner, conv), "message")["attachments"].([]any)[0].(map[string]any)["id"].(string)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 1})
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})

	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	et.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/restore", nil)
	h.clock.Advance(time.Hour)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)
	other.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+otherTrashed, nil)
	h.runWebhooks(et.ws)
	hooks := rcv.take()
	if len(hooks) != 3 || hooks[0].typ != "conversation.trashed" || hooks[1].typ != "conversation.restored" || hooks[2].typ != "conversation.trashed" {
		t.Fatalf("hooks: %v", hooks)
	}
	if c := hooks[0].data["conversation"].(map[string]any); c["id"] != conv || c["trashed_at"] == nil || hooks[0].data["contact"] == nil {
		t.Fatalf("conversation.trashed: %s", hooks[0].body)
	}

	key.expect(http.StatusOK, "GET", "/v1/attachments/"+att, nil)
	key.expect(http.StatusOK, "GET", "/v1/messages/"+in.str("message_id")+"/raw", nil)
	plainKey.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att, nil)
	plainKey.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+in.str("message_id")+"/raw", nil)

	h.clock.Advance(2 * 24 * time.Hour)
	if err := h.srv.ApplyRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	key.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	h.clock.Advance(27*24*time.Hour + 22*time.Hour)
	if err := h.srv.PurgeTrash(context.Background(), time.Minute); err != nil {
		t.Fatal(err)
	}
	key.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	h.clock.Advance(2 * time.Hour)
	key.expectProblem(http.StatusConflict, "trash_expired", "POST", "/v1/conversations/"+conv+"/restore", nil)
	if err := h.srv.PurgeTrash(context.Background(), time.Minute); err != nil {
		t.Fatal(err)
	}
	key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	key.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/restore", nil)
	for _, k := range keys {
		if h.stored(k) {
			t.Fatalf("file %s still stored", k)
		}
	}
	if n := h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", et.ws, conv); n != 0 {
		t.Fatalf("%d messages left", n)
	}
	otherKey.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+otherTrashed, nil)
	otherKey.expect(http.StatusOK, "GET", "/v1/conversations/"+otherKept, nil)

	deleted := oneAudit(t, auditLog(t, key, "&actor_kind=system"), "conversation.deleted")
	if a := actorOf(deleted); a["kind"] != "system" || a["id"] != nil || a["name"] != nil || targetOf(deleted)["id"] != conv || detailsOf(deleted)["inbox_id"] != et.inbox {
		t.Fatalf("deleted entry %v", deleted)
	}
	if n := len(auditLog(t, otherKey, "&action=conversation.deleted")); n != 1 {
		t.Fatalf("other workspace: %d entries", n)
	}
	h.runWebhooks(et.ws)
	hooks = rcv.take()
	if len(hooks) != 1 || hooks[0].typ != "conversation.deleted" {
		t.Fatalf("after purge: %v", hooks)
	}
	if c := hooks[0].data["conversation"].(map[string]any); c["id"] != conv || c["inbox_id"] != et.inbox || c["contact_id"] == nil || len(c) != 3 {
		t.Fatalf("conversation.deleted: %s", hooks[0].body)
	}
	var found bool
	for _, e := range key.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=100", nil).body["events"].([]any) {
		if ev := e.(map[string]any); ev["type"] == "conversation.deleted" && ev["conversation_id"] == conv {
			found = true
		}
	}
	if !found {
		t.Fatal("no conversation.deleted event")
	}
}

package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/storage"
)

func (h *harness) storedKeys(ws string) []string {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(),
		"SELECT storage_key FROM attachments WHERE workspace_id = $1 UNION ALL SELECT raw_key FROM message_emails WHERE workspace_id = $1 AND raw_key IS NOT NULL", ws)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			h.t.Fatal(err)
		}
		keys = append(keys, k)
	}
	return keys
}

func (h *harness) stored(key string) bool {
	h.t.Helper()
	f, err := h.storage.Open(context.Background(), key)
	if errors.Is(err, storage.ErrNotFound) {
		return false
	}
	if err != nil {
		h.t.Fatal(err)
	}
	f.Close()
	return true
}

func (h *harness) count(query string, args ...any) int {
	h.t.Helper()
	var n int
	if err := h.st.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func personEmail(c *client) string {
	return c.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["email"].(string)
}

func memberships(c *client) []any {
	return c.expect(http.StatusOK, "GET", "/v1/me", nil).body["memberships"].([]any)
}

func TestDeleteWorkspace(t *testing.T) {
	h, push := pushHarness(t)
	ctx := context.Background()
	ct := newChatTeam(t, h, "live", true)
	name := ct.owner.expect(http.StatusOK, "GET", "/v1/workspace", nil).str("name")
	address := unique("support") + "@example.com"
	ct.owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+ct.inbox+"/channels", map[string]any{
		"kind": "email", "name": "Support mail", "email": map[string]any{"address": address},
	})
	if r := h.ingest(address, []byte(fmt.Sprintf(attachmentMail, address, newMessageID())), nil); r.status != http.StatusAccepted {
		t.Fatalf("ingest: %d %v", r.status, r.body)
	}

	other := newEmailTeam(t, h, false)
	kept := h.ingest(other.address, []byte(fmt.Sprintf(attachmentMail, other.address, newMessageID())), nil)
	if kept.status != http.StatusAccepted {
		t.Fatalf("ingest other: %d %v", kept.status, kept.body)
	}
	otherConv := kept.str("conversation_id")
	otherAttachment := lastOf(messages(other.owner, otherConv), "message")["attachments"].([]any)[0].(map[string]any)["id"].(string)

	agentEmail := personEmail(ct.agent)
	ownerEmail := personEmail(ct.owner)
	other.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": agentEmail, "role": "agent"})
	ct.agent.signIn(agentEmail)
	ct.agent.workspace = ct.ws
	push.subscribe(ct.owner)
	push.subscribe(ct.agent)

	key := ct.apiKey(h)
	rcv := newReceiver(t)
	key.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{
		"url": rcv.srv.URL, "events": []string{"conversation.created", "message.created"},
	})
	visitor := ct.session(h, map[string]any{})
	visitor.start("Hello before the end")

	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/workspace", map[string]any{"name": name})
	key.expectProblem(http.StatusForbidden, "member_session_required", "DELETE", "/v1/workspace", map[string]any{"name": name})
	ct.owner.expectProblem(http.StatusBadRequest, "confirmation_mismatch", "DELETE", "/v1/workspace", map[string]any{"name": strings.ToUpper(name)})
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "DELETE", "/v1/workspace", nil)

	keys := h.storedKeys(ct.ws)
	if len(keys) < 2 {
		t.Fatalf("stored files before deletion: %v", keys)
	}
	for _, k := range keys {
		if !h.stored(k) {
			t.Fatalf("%s is not stored", k)
		}
	}

	ct.owner.expect(http.StatusAccepted, "DELETE", "/v1/workspace", map[string]any{"name": name})
	if j := h.jobs("workspace_delete", ct.ws); len(j) != 1 {
		t.Fatalf("deletion jobs queued: %d", len(j))
	}

	ct.owner.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/workspace", nil)
	ct.owner.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/conversations", nil)
	if m := memberships(ct.owner); len(m) != 0 {
		t.Fatalf("owner memberships after deletion: %v", m)
	}
	ct.agent.expectProblem(http.StatusForbidden, "not_a_member", "GET", "/v1/conversations", nil)
	if m := memberships(ct.agent); len(m) != 1 || m[0].(map[string]any)["workspace"].(map[string]any)["id"] != other.ws {
		t.Fatalf("agent memberships after deletion: %v", m)
	}
	key.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/conversations", nil)
	visitor.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", "/client/v1/conversations", nil)
	visitor.origin = ""
	visitor.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/client/v1/conversations", nil)
	if r := ct.sessionStatus(h, ct.origin, map[string]any{}); r.status == http.StatusCreated {
		t.Fatalf("new widget session after deletion: %s", r.raw)
	}
	if r := h.ingest(address, buildMail(mailOpts{from: "late@example.net", to: address, subject: "Late", messageID: newMessageID(), body: "Hi"}), nil); r.status != http.StatusNotFound {
		t.Fatalf("mail after deletion: %d %v", r.status, r.body)
	}
	if n := h.count("SELECT count(*) FROM webhook_endpoints WHERE workspace_id = $1", ct.ws); n != 0 {
		t.Fatalf("%d webhook endpoints left", n)
	}
	h.runWebhooks(ct.ws)
	if got := rcv.take(); len(got) != 0 {
		t.Fatalf("webhooks sent after deletion: %d", len(got))
	}
	if subs := ct.owner.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(subs) != 0 {
		t.Fatalf("owner push subscriptions: %v", subs)
	}
	if subs := ct.agent.expect(http.StatusOK, "GET", "/v1/me/push-subscriptions", nil).body["items"].([]any); len(subs) != 1 {
		t.Fatalf("agent push subscriptions: %v", subs)
	}
	ct.owner.expectProblem(http.StatusForbidden, "not_a_member", "DELETE", "/v1/workspace", map[string]any{"name": name})

	again := h.client()
	again.signIn(ownerEmail)
	if m := memberships(again); len(m) != 0 {
		t.Fatalf("owner signs in to %v", m)
	}

	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO conversations (id, workspace_id, inbox_id, contact_id, last_activity_at, created_at, updated_at)
		SELECT gen_random_uuid(), $1, $2, $3, now(), now(), now() FROM generate_series(1, 450)`, ct.ws, ct.inbox, ct.contact); err != nil {
		t.Fatal(err)
	}
	wsID := uuid.MustParse(ct.ws)
	first, err := h.srv.PurgeWorkspace(ctx, wsID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Done || first.Conversations != 200 || first.ConversationsLeft == 0 {
		t.Fatalf("first run: %+v", first)
	}
	if h.count("SELECT count(*) FROM workspaces WHERE id = $1", ct.ws) != 1 {
		t.Fatal("workspace row gone before its data")
	}
	for range 10 {
		p, err := h.srv.PurgeWorkspace(ctx, wsID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if p.Done {
			break
		}
	}
	for _, table := range []string{"workspaces", "conversations", "messages", "attachments", "message_emails", "contacts", "members", "inboxes", "channels", "api_keys", "events"} {
		col := "workspace_id"
		if table == "workspaces" {
			col = "id"
		}
		if n := h.count("SELECT count(*) FROM "+table+" WHERE "+col+" = $1", ct.ws); n != 0 {
			t.Fatalf("%d rows left in %s", n, table)
		}
	}
	for _, k := range keys {
		if h.stored(k) {
			t.Fatalf("%s is still stored", k)
		}
	}
	if n := h.count("SELECT count(*) FROM people WHERE email = ANY($1)", []string{ownerEmail, agentEmail}); n != 2 {
		t.Fatalf("%d people left, want 2", n)
	}

	other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+otherConv, nil)
	other.owner.expect(http.StatusOK, "GET", "/v1/attachments/"+otherAttachment, nil)
	other.owner.expect(http.StatusOK, "GET", "/v1/messages/"+kept.str("message_id")+"/raw", nil)
	for _, k := range h.storedKeys(other.ws) {
		if !h.stored(k) {
			t.Fatalf("other workspace lost %s", k)
		}
	}
	ct.agent.workspace = ""
	ct.agent.expect(http.StatusOK, "GET", "/v1/conversations", nil)

	if p, err := h.srv.PurgeWorkspace(ctx, uuid.MustParse(other.ws), 0); err != nil || !p.Done || p.Conversations != 0 {
		t.Fatalf("purge of a live workspace: %+v %v", p, err)
	}
	other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+otherConv, nil)
}

func TestDeleteAccount(t *testing.T) {
	h, push := pushHarness(t)
	tm := newTeam(t, h)
	ownerEmail := personEmail(tm.owner)
	agentEmail := personEmail(tm.agent)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	post(tm.agent, conv, map[string]any{"kind": "message", "body": "I will look into it"})

	tm.owner.expectProblem(http.StatusBadRequest, "confirmation_mismatch", "DELETE", "/v1/me", map[string]any{"email": agentEmail})
	tm.owner.expectProblem(http.StatusConflict, "last_owner", "DELETE", "/v1/me", map[string]any{"email": ownerEmail})
	key := tm.apiKey(h)
	key.expectProblem(http.StatusForbidden, "member_session_required", "DELETE", "/v1/me", map[string]any{"email": ownerEmail})

	second := h.client()
	second.signIn(agentEmail)
	push.subscribe(tm.agent)
	tm.agent.expect(http.StatusAccepted, "POST", "/v1/auth/code", map[string]any{"email": agentEmail})
	personID := tm.agent.expect(http.StatusOK, "GET", "/v1/me", nil).body["person"].(map[string]any)["id"].(string)
	perPerson := []string{
		"SELECT count(*) FROM sessions WHERE person_id = $1",
		"SELECT count(*) FROM push_subscriptions WHERE person_id = $1",
		"SELECT count(*) FROM members WHERE person_id = $1",
	}
	for _, q := range perPerson {
		if n := h.count(q, personID); n == 0 {
			t.Fatalf("%s: nothing to delete", q)
		}
	}
	if n := h.count("SELECT count(*) FROM login_codes WHERE email = $1", agentEmail); n == 0 {
		t.Fatal("no login codes to delete")
	}

	r := tm.agent.expect(http.StatusNoContent, "DELETE", "/v1/me", map[string]any{"email": strings.ToUpper(agentEmail)})
	if c := r.header.Get("Set-Cookie"); !strings.Contains(c, "yuva_session=") || !strings.Contains(c, "Max-Age=0") {
		t.Fatalf("Set-Cookie %q does not clear the session", c)
	}
	second.expectProblem(http.StatusUnauthorized, "unauthenticated", "GET", "/v1/me", nil)
	for _, q := range append(perPerson, "SELECT count(*) FROM people WHERE id = $1", "SELECT count(*) FROM passkeys WHERE person_id = $1") {
		if n := h.count(q, personID); n != 0 {
			t.Fatalf("%s: %d left", q, n)
		}
	}
	if n := h.count("SELECT count(*) FROM login_codes WHERE email = $1", agentEmail); n != 0 {
		t.Fatalf("%d login codes left", n)
	}
	m := lastOf(messages(tm.owner, conv), "message")
	author := m["author"].(map[string]any)
	if m["body"] != "I will look into it" || author["type"] != "member" || author["member_id"] != nil {
		t.Fatalf("message after the author's deletion: %v", m)
	}

	lone := unique("lone") + "@example.com"
	tm.owner.expect(http.StatusCreated, "POST", "/v1/invites", map[string]any{"email": lone, "role": "owner"})
	loner := h.client()
	loner.signIn(lone)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/me", map[string]any{"email": ownerEmail})
	loner.expectProblem(http.StatusConflict, "last_owner", "DELETE", "/v1/me", map[string]any{"email": lone})
	loner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
}

func TestDeletedPersonWithoutWorkspace(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	agentEmail := personEmail(tm.agent)
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+tm.agentID, nil)
	c := h.client()
	c.signIn(agentEmail)
	if m := memberships(c); len(m) != 0 {
		t.Fatalf("memberships: %v", m)
	}
	c.expect(http.StatusNoContent, "DELETE", "/v1/me", map[string]any{"email": agentEmail})
	if n := h.count("SELECT count(*) FROM people WHERE email = $1", agentEmail); n != 0 {
		t.Fatalf("person left: %d", n)
	}
}

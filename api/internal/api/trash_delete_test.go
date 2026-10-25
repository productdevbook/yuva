package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (h *harness) trashBySQL(ws, conv string) {
	h.t.Helper()
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE conversations SET trashed_at = $3 WHERE workspace_id = $1 AND id = $2", ws, conv, h.clock.Now()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) runTrashEmpties(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("trash_empty", ws) {
		var a api.TrashEmptyArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		if err := h.srv.EmptyTrashRun(context.Background(), a, time.Minute); err != nil {
			h.t.Fatal(err)
		}
		h.dropJob(j.id)
	}
}

func (h *harness) conversationRows(ws string, ids ...string) int {
	h.t.Helper()
	return h.count("SELECT count(*) FROM conversations WHERE workspace_id = $1 AND id = ANY($2::uuid[])", ws, ids)
}

type personalMailbox struct {
	owner   *client
	ownerID string
	inbox   string
	address string
}

func (tm team) personalMailbox(h *harness) personalMailbox {
	h.t.Helper()
	owner, ownerID := tm.newAgent()
	inbox := owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private mail", "slug": unique("private"), "owner_member_id": ownerID}).body["inbox"].(map[string]any)["id"].(string)
	address := unique("me") + "@example.com"
	owner.expect(http.StatusCreated, "POST", "/v1/inboxes/"+inbox+"/channels", map[string]any{
		"kind": "email", "name": "My mail", "email": map[string]any{
			"address": address, "smtp": map[string]any{"host": "smtp.example.com", "port": 2525, "username": "mailer", "password": "smtp-secret", "tls": "starttls"},
		},
	})
	return personalMailbox{owner: owner, ownerID: ownerID, inbox: inbox, address: address}
}

func (pm personalMailbox) trashedConversation(h *harness, ws string) string {
	h.t.Helper()
	r := h.ingest(pm.address, []byte(fmt.Sprintf(attachmentMail, pm.address, newMessageID())), nil)
	if r.status != http.StatusAccepted {
		h.t.Fatalf("ingest: %d %v", r.status, r.body)
	}
	conv := r.str("conversation_id")
	h.trashBySQL(ws, conv)
	return conv
}

func failures(r response) map[string]string {
	out := map[string]string{}
	for _, f := range r.body["failed"].([]any) {
		m := f.(map[string]any)
		out[m["id"].(string)] = fmt.Sprintf("%v %v", m["status"], m["code"])
	}
	return out
}

func TestDeleteConversationsForever(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	in := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	conv := in.str("conversation_id")
	keys := h.storedKeys(et.ws)
	live := et.conversation(et.owner)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+conv, nil)

	pm := et.personalMailbox(h)
	mine := pm.trashedConversation(h, et.ws)
	delegate, delegateID := et.newAgent()
	pm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+pm.inbox+"/delegates/"+delegateID, map[string]any{"access": "send"})

	other := newTeam(t, h)
	foreign := other.conversation(other.owner)
	other.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+foreign, nil)

	et.apiKey(h).expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/delete", map[string]any{"ids": []string{conv}})
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = conv
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/delete", map[string]any{"ids": ids})
	if r := delegate.expect(http.StatusOK, "POST", "/v1/conversations/delete", map[string]any{"ids": []string{mine}}); r.body["deleted"] != float64(0) || failures(r)[mine] != "403 forbidden" {
		t.Fatalf("delegate: %s", r.raw)
	}
	if r := et.agent.expect(http.StatusOK, "POST", "/v1/conversations/delete", map[string]any{"ids": []string{conv}}); r.body["deleted"] != float64(0) || failures(r)[conv] == "" {
		t.Fatalf("agent: %s", r.raw)
	}

	r := et.owner.expect(http.StatusOK, "POST", "/v1/conversations/delete", map[string]any{"ids": []string{foreign, conv, live, conv, mine}})
	f := failures(r)
	if r.body["deleted"] != float64(1) || len(f) != 3 || f[live] != "409 not_trashed" || f[mine] != "404 not_found" || f[foreign] != "404 not_found" {
		t.Fatalf("owner delete: %s", r.raw)
	}
	if got := r.body["failed"].([]any)[0].(map[string]any)["id"]; got != foreign {
		t.Fatalf("failures not in request order: %s", r.raw)
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
	if n := h.count("SELECT count(*) FROM messages WHERE workspace_id = $1 AND conversation_id = $2", et.ws, conv); n != 0 {
		t.Fatalf("%d messages left", n)
	}
	h.checkDeletesQueued(et.ws, keys)
	if h.conversationRows(et.ws, live, mine) != 2 || h.conversationRows(other.ws, foreign) != 1 {
		t.Fatal("deleted more than asked")
	}
	var found bool
	for _, e := range et.owner.expect(http.StatusOK, "GET", "/v1/events?after=0&limit=200", nil).body["events"].([]any) {
		if ev := e.(map[string]any); ev["type"] == "conversation.deleted" && ev["conversation_id"] == conv {
			found = true
		}
	}
	if !found {
		t.Fatal("no conversation.deleted event")
	}

	if r := pm.owner.expect(http.StatusOK, "POST", "/v1/conversations/delete", map[string]any{"ids": []string{mine}}); r.body["deleted"] != float64(1) || len(failures(r)) != 0 {
		t.Fatalf("mailbox owner: %s", r.raw)
	}
	if h.conversationRows(et.ws, mine) != 0 {
		t.Fatal("mailbox owner's conversation still there")
	}

	entries := auditOf(auditLog(t, et.owner, "&action=conversations.deleted"), "conversations.deleted")
	if len(entries) != 2 {
		t.Fatalf("audit: %v", entries)
	}
	for _, e := range entries {
		d := detailsOf(e)
		if d["count"] != float64(1) || len(d) != 1 || targetOf(e)["type"] != "conversations" || targetOf(e)["id"] != et.ws {
			t.Fatalf("audit entry: %v", e)
		}
	}
	if n := len(auditLog(t, other.owner, "&action=conversations.deleted")); n != 0 {
		t.Fatalf("other workspace: %d entries", n)
	}
	key := et.scopedKey(h, "conversations:read", "conversations:delete")
	again := et.conversation(et.owner)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+again, nil)
	if r := key.expect(http.StatusOK, "POST", "/v1/conversations/delete", map[string]any{"ids": []string{again}}); r.body["deleted"] != float64(1) {
		t.Fatalf("key: %s", r.raw)
	}
}

func TestEmptyTrash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	var trashed []string
	for range 3 {
		c := et.conversation(et.owner)
		et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+c, nil)
		trashed = append(trashed, c)
	}
	pm := et.personalMailbox(h)
	mine := pm.trashedConversation(h, et.ws)
	other := newTeam(t, h)
	foreign := other.conversation(other.owner)
	other.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+foreign, nil)

	et.apiKey(h).expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 3})
	r := et.owner.expect(http.StatusConflict, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 4})
	if r.str("code") != "count_changed" || r.body["count"] != float64(3) {
		t.Fatalf("mismatch: %s", r.raw)
	}
	if len(h.jobs("trash_empty", et.ws)) != 0 {
		t.Fatal("job queued on a mismatch")
	}
	if r := et.owner.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 3}); r.body["count"] != float64(3) {
		t.Fatalf("empty: %s", r.raw)
	}
	h.clock.Advance(time.Second)
	later := et.conversation(et.owner)
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+later, nil)
	h.runTrashEmpties(et.ws)
	if h.conversationRows(et.ws, trashed...) != 0 {
		t.Fatal("trash not emptied")
	}
	if h.conversationRows(et.ws, later, mine) != 2 || h.conversationRows(other.ws, foreign) != 1 {
		t.Fatal("emptied past the bound, another's mailbox or another workspace")
	}
	if r := et.owner.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 1}); r.body["count"] != float64(1) {
		t.Fatalf("second empty: %s", r.raw)
	}
	h.runTrashEmpties(et.ws)
	if r := pm.owner.expect(http.StatusConflict, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 0}); r.body["count"] != float64(1) {
		t.Fatalf("mailbox owner mismatch: %s", r.raw)
	}
	pm.owner.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 1})
	h.runTrashEmpties(et.ws)
	if h.conversationRows(et.ws, later, mine) != 0 {
		t.Fatal("mailbox owner's trash not emptied")
	}
	if r := et.owner.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 0}); r.body["count"] != float64(0) || len(h.jobs("trash_empty", et.ws)) != 0 {
		t.Fatalf("empty trash: %s", r.raw)
	}

	entries := auditOf(auditLog(t, et.owner, "&action=trash.emptied"), "trash.emptied")
	if len(entries) != 3 {
		t.Fatalf("audit: %v", entries)
	}
	counts := map[string]float64{}
	for _, e := range entries {
		d := detailsOf(e)
		if len(d) != 1 || targetOf(e)["type"] != "trash" || actorOf(e)["kind"] != "member" {
			t.Fatalf("audit entry: %v", e)
		}
		counts[actorOf(e)["id"].(string)] += d["count"].(float64)
	}
	if counts[et.ownerMemberID(t)] != 4 || counts[pm.ownerID] != 1 {
		t.Fatalf("audit counts: %v", counts)
	}
}

func TestBulkSpam(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	a, b := tm.conversation(tm.owner), tm.conversation(tm.owner)
	r := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a, b}, "spam": true})
	if len(r.body["updated"].([]any)) != 2 {
		t.Fatalf("spam: %s", r.raw)
	}
	if ids := listedIDs(tm.owner, "&spam=true"); len(ids) != 2 {
		t.Fatalf("spam view: %v", ids)
	}
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a}, "spam": false})
	if ids := listedIDs(tm.owner, "&spam=true"); len(ids) != 1 || ids[0] != b {
		t.Fatalf("spam view after not spam: %v", ids)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{a}, "spam": true, "trashed": true})
}

func TestPersonalMailboxTrash(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	shared := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil).str("conversation_id")
	h.trashBySQL(et.ws, shared)
	pm := et.personalMailbox(h)
	mine := pm.trashedConversation(h, et.ws)
	delegate, delegateID := et.newAgent()
	pm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+pm.inbox+"/delegates/"+delegateID, map[string]any{"access": "send"})
	pm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+pm.inbox+"/delegates/"+et.ownerMemberID(t), map[string]any{"access": "send"})
	trashCount := func(c *client) any {
		return c.expect(http.StatusOK, "GET", "/v1/conversations/counts", nil).body["trash"]
	}

	if ids := listedIDs(pm.owner, "&trashed=true"); fmt.Sprint(ids) != fmt.Sprint([]string{mine}) {
		t.Fatalf("owner's trash: %v", ids)
	}
	if n := trashCount(pm.owner); n != float64(1) {
		t.Fatalf("owner's trash count: %v", n)
	}
	if ids := listedIDs(pm.owner, ""); has(ids, mine) {
		t.Fatalf("trashed in the open list: %v", ids)
	}
	pm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+mine, nil)
	att := lastOf(messages(pm.owner, mine), "message")["attachments"].([]any)[0].(map[string]any)["id"].(string)
	pm.owner.expect(http.StatusOK, "GET", "/v1/attachments/"+att, nil)
	pm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+shared, nil)
	pm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+shared+"/restore", nil)

	for name, c := range map[string]*client{"agent delegate": delegate, "owner delegate": et.owner} {
		if ids := listedIDs(c, "&trashed=true"); has(ids, mine) {
			t.Fatalf("%s sees the trash: %v", name, ids)
		}
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+mine, nil)
		c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att, nil)
		c.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/conversations/"+mine+"/restore", nil)
	}
	if n := trashCount(delegate); n != float64(0) {
		t.Fatalf("delegate's trash count: %v", n)
	}
	if ids := listedIDs(et.owner, "&trashed=true"); fmt.Sprint(ids) != fmt.Sprint([]string{shared}) {
		t.Fatalf("workspace owner's trash: %v", ids)
	}
	if n := trashCount(et.owner); n != float64(1) {
		t.Fatalf("workspace owner's trash count: %v", n)
	}

	other := newTeam(t, h)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+mine+"/restore", nil)
	if ids := listedIDs(other.owner, "&trashed=true"); len(ids) != 0 {
		t.Fatalf("other workspace: %v", ids)
	}

	restored := pm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+mine+"/restore", nil)
	if restored.body["trashed_at"] != nil {
		t.Fatalf("restore: %s", restored.raw)
	}
	if ids := listedIDs(pm.owner, "&trashed=true"); len(ids) != 0 || trashCount(pm.owner) != float64(0) {
		t.Fatalf("after restore: %v", ids)
	}
}

func TestEmptyTrashDeletesOnlyTheConfirmedSet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "admin"})
	admin := tm.agent
	trash := func() string {
		c := tm.conversation(tm.owner)
		tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+c, nil)
		return c
	}
	a, b, c, e := trash(), trash(), trash(), trash()
	run := func() {
		t.Helper()
		jobs := h.jobs("trash_empty", tm.ws)
		if len(jobs) != 1 {
			t.Fatalf("jobs: %v", jobs)
		}
		var args api.TrashEmptyArgs
		if err := json.Unmarshal(jobs[0].args, &args); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := h.srv.EmptyTrashRun(context.Background(), args, time.Minute); err != nil {
				t.Fatal(err)
			}
		}
		h.dropJob(jobs[0].id)
	}
	emptied := func() []float64 {
		var out []float64
		for _, e := range auditOf(auditLog(t, tm.owner, "&action=trash.emptied"), "trash.emptied") {
			if actorOf(e)["id"] == tm.agentID {
				out = append(out, detailsOf(e)["count"].(float64))
			}
		}
		return out
	}

	admin.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 4})
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+b+"/restore", nil)
	tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+e+"/restore", nil)
	h.clock.Advance(time.Second)
	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+e, nil)
	d := trash()
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE trash_empties SET deleted = 5 WHERE workspace_id = $1", tm.ws); err != nil {
		t.Fatal(err)
	}
	run()
	if h.conversationRows(tm.ws, a, c) != 0 || h.conversationRows(tm.ws, b, d, e) != 3 {
		t.Fatal("deleted other than the confirmed, still trashed set")
	}
	if got := emptied(); fmt.Sprint(got) != "[7]" {
		t.Fatalf("trash.emptied after a retried run: %v", got)
	}

	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+b, nil)
	admin.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 3})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+tm.agentID, map[string]any{"role": "agent"})
	run()
	if h.conversationRows(tm.ws, b, d, e) != 3 {
		t.Fatal("deleted after the caller lost the right to")
	}
	if got := emptied(); len(got) != 2 || got[0]+got[1] != 7 {
		t.Fatalf("trash.emptied after a demotion: %v", got)
	}

	sets := func() int {
		t.Helper()
		var n int
		if err := h.st.Pool.QueryRow(context.Background(), "SELECT count(*) FROM trash_empties WHERE workspace_id = $1", tm.ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	tm.owner.expect(http.StatusAccepted, "POST", "/v1/conversations/trash/empty", map[string]any{"confirm_count": 3})
	for _, j := range h.jobs("trash_empty", tm.ws) {
		h.dropJob(j.id)
	}
	h.clock.Advance(6 * 24 * time.Hour)
	if err := h.srv.CleanWorkspaceTrashEmpties(context.Background(), uuid.MustParse(tm.ws)); err != nil || sets() != 1 {
		t.Fatalf("set of a discarded job cleaned too early: %v", err)
	}
	h.clock.Advance(2 * 24 * time.Hour)
	if err := h.srv.CleanWorkspaceTrashEmpties(context.Background(), uuid.MustParse(tm.ws)); err != nil || sets() != 0 {
		t.Fatalf("set of a discarded job kept: %v", err)
	}
	if h.conversationRows(tm.ws, b, d, e) != 3 {
		t.Fatal("cleaning a set deleted conversations")
	}
}

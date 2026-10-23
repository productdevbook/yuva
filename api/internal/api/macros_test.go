package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"
	"uuid"
)

func macroIDs(c *client) []string {
	c.h.t.Helper()
	var out []string
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/macros", nil).body["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func TestMacroPermissions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "billing"}).str("id")

	shared := tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Refund", "shortcut": "refund", "body": "We refunded you.",
		"actions": map[string]any{"add_labels": []string{label}, "status": "closed", "priority": "high"},
	})
	if shared.str("visibility") != "shared" || shared.body["owner_member_id"] != nil || shared.body["actions"].(map[string]any)["status"] != "closed" {
		t.Fatalf("shared macro: %s", shared.raw)
	}
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "x", "shortcut": "x", "body": "x"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/macros/"+shared.str("id"), map[string]any{"name": "mine"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/macros/"+shared.str("id"), nil)

	personal := tm.agent.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "personal", "name": "Mine", "shortcut": "refund", "actions": map[string]any{"assignee_id": tm.agentID},
	})
	if personal.str("visibility") != "personal" || personal.str("owner_member_id") != tm.agentID || personal.body["body"] != nil {
		t.Fatalf("personal macro: %s", personal.raw)
	}
	tm.agent.expectProblem(http.StatusConflict, "shortcut_taken", "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "Again", "shortcut": "refund", "body": "x"})
	tm.owner.expectProblem(http.StatusConflict, "shortcut_taken", "POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "Again", "shortcut": "refund", "body": "x"})
	tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "Owner's", "shortcut": "refund", "body": "x"})

	for _, bad := range []map[string]any{
		{"visibility": "personal", "name": "x", "shortcut": "y"},
		{"visibility": "personal", "name": "x", "shortcut": "Bad Slug", "body": "x"},
		{"visibility": "personal", "name": "x", "shortcut": "y", "actions": map[string]any{"status": "snoozed"}},
		{"visibility": "personal", "name": "x", "shortcut": "y", "actions": map[string]any{"status": "closed", "snooze_minutes": 10}},
		{"visibility": "personal", "name": "x", "shortcut": "y", "actions": map[string]any{"add_labels": []string{label}, "remove_labels": []string{label}}},
		{"visibility": "personal", "name": "x", "shortcut": "y", "actions": map[string]any{"assignee_id": other.agentID}},
		{"visibility": "personal", "name": "x", "shortcut": "y", "actions": map[string]any{"add_labels": []string{other.inbox}}},
		{"visibility": "team", "name": "x", "shortcut": "y", "body": "x"},
	} {
		tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/macros", bad)
	}

	if ids := macroIDs(tm.agent); len(ids) != 2 || ids[0] != personal.str("id") || ids[1] != shared.str("id") {
		t.Fatalf("agent lists %v", ids)
	}
	if ids := macroIDs(tm.owner); len(ids) != 2 || slices.Contains(ids, personal.str("id")) {
		t.Fatalf("owner lists %v", ids)
	}
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/macros/"+personal.str("id"), nil)
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/macros/"+personal.str("id"), map[string]any{"name": "x"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/macros/"+personal.str("id"), nil)
	tm.agent.expect(http.StatusOK, "GET", "/v1/macros/"+shared.str("id"), nil)

	upd := tm.agent.expect(http.StatusOK, "PATCH", "/v1/macros/"+personal.str("id"), map[string]any{"body": "Hello", "actions": map[string]any{"snooze_minutes": 60}})
	if a := upd.body["actions"].(map[string]any); upd.str("body") != "Hello" || a["status"] != "snoozed" || a["assignee_id"] != nil {
		t.Fatalf("updated macro: %s", upd.raw)
	}

	key := tm.scopedKey(h, "conversations:read", "canned_replies:write")
	if ids := macroIDs(key); len(ids) != 1 || ids[0] != shared.str("id") {
		t.Fatalf("key lists %v", ids)
	}
	key.expectProblem(http.StatusForbidden, "member_session_required", "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "x", "shortcut": "k", "body": "x"})
	key.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/macros/"+personal.str("id"), nil)
	keyShared := key.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "Key", "shortcut": "key", "body": "x"}).str("id")
	reader := tm.scopedKey(h, "conversations:read")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "PATCH", "/v1/macros/"+keyShared, map[string]any{"name": "y"})
	key.expect(http.StatusNoContent, "DELETE", "/v1/macros/"+keyShared, nil)

	grant := h.client()
	grant.bearer = h.oauthToken(tm.agent, tm.ws, "Agent App", testOrigin, []string{"conversations:read"})
	grant.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "x", "shortcut": "g", "body": "x"})
	grant.bearer = h.oauthToken(tm.agent, tm.ws, "Agent App", testOrigin, []string{"conversations:read", "conversations:write"})
	grant.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "x", "shortcut": "g", "body": "x"})
	grant.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "x", "shortcut": "g", "body": "x"})

	for _, c := range []*client{other.owner, other.agent, other.apiKey(h)} {
		for _, id := range []string{shared.str("id"), personal.str("id")} {
			c.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/macros/"+id, nil)
			c.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/macros/"+id, map[string]any{"name": "x"})
			c.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/macros/"+id, nil)
		}
		if ids := macroIDs(c); len(ids) != 0 {
			t.Fatalf("other workspace lists %v", ids)
		}
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/labels/"+label, nil)
	if a := tm.owner.expect(http.StatusOK, "GET", "/v1/macros/"+shared.str("id"), nil).body["actions"].(map[string]any); a["add_labels"] != nil || a["status"] != "closed" {
		t.Fatalf("deleted label kept in macro: %v", a)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/macros/"+shared.str("id"), nil)
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/macros/"+shared.str("id"), nil)
}

func conversationOf(c *client, conv string) map[string]any {
	return c.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).body
}

func TestMacroAppliedWithMessage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	vip := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "vip"}).str("id")
	old := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "old"}).str("id")
	macro := tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Escalate", "shortcut": "escalate", "body": "Escalating.",
		"actions": map[string]any{"add_labels": []string{vip}, "remove_labels": []string{old}, "assignee_id": tm.agentID, "priority": "urgent", "snooze_minutes": 90},
	}).str("id")
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"labels": []string{old}})

	h.clock.Advance(time.Second)
	msg := tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Escalating.", "macro_id": macro, "client_id": "m-1",
	})
	c := conversationOf(tm.owner, conv)
	until, _ := time.Parse(time.RFC3339Nano, c["snooze_until"].(string))
	sent, _ := time.Parse(time.RFC3339Nano, msg.str("created_at"))
	if c["status"] != "snoozed" || c["priority"] != "urgent" || c["assignee_id"] != tm.agentID ||
		!slices.Equal(c["labels"].([]any), []any{vip}) || !until.Equal(sent.Add(90*time.Minute)) {
		t.Fatalf("macro not applied: %v", c)
	}
	var kinds []string
	var applied map[string]any
	for _, m := range messages(tm.owner, conv) {
		ev, _ := m["event"].(map[string]any)
		if ev == nil {
			kinds = append(kinds, m["kind"].(string))
			continue
		}
		kinds = append(kinds, ev["type"].(string))
		if ev["type"] == "macro_applied" {
			applied = ev["macro"].(map[string]any)
		}
	}
	if applied["id"] != macro || applied["name"] != "Escalate" || kinds[0] != "labels_changed" || kinds[1] != "message" || kinds[2] != "macro_applied" ||
		!slices.Contains(kinds, "assigned") || !slices.Contains(kinds, "status_changed") {
		t.Fatalf("timeline %v, macro %v", kinds, applied)
	}

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open", "labels": []string{}})
	again := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Escalating.", "macro_id": macro, "client_id": "m-1",
	})
	if again.str("id") != msg.str("id") || conversationOf(tm.owner, conv)["status"] != "open" {
		t.Fatal("a repeated client_id applied the macro again")
	}

	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "message", "body": "Done.", "macro_id": macro, "then": map[string]any{"status": "closed", "labels": []string{old}},
	})
	c = conversationOf(tm.owner, conv)
	if c["status"] != "closed" || c["snooze_until"] != nil || c["priority"] != "urgent" || !slices.Equal(c["labels"].([]any), []any{old}) {
		t.Fatalf("then did not take the macro's place: %v", c)
	}

	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{
		"kind": "note", "body": "closing", "then": map[string]any{"status": "open"},
	})
	if conversationOf(tm.owner, conv)["status"] != "open" {
		t.Fatal("then on a note not applied")
	}
	up := tm.owner.upload("/v1/conversations/"+conv+"/messages", map[string]string{
		"kind": "message", "body": "file", "macro_id": macro, "then": `{"priority":"low"}`,
	}, []file{{"a.txt", "text/plain", []byte("hello")}})
	if up.status != http.StatusCreated {
		t.Fatalf("multipart with a macro: %d %s", up.status, up.raw)
	}
	if c := conversationOf(tm.owner, conv); c["priority"] != "low" || c["status"] != "snoozed" {
		t.Fatalf("multipart macro not applied: %v", c)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "x", "draft": true, "macro_id": macro})

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open", "priority": "normal"})
	stranger := other.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "x", "shortcut": "x", "body": "x", "actions": map[string]any{"status": "closed"}}).str("id")
	personal := tm.agent.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "Close", "shortcut": "close", "actions": map[string]any{"status": "closed"}}).str("id")
	count := len(messages(tm.owner, conv))
	for _, id := range []string{stranger, personal} {
		tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "x", "macro_id": id})
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+other.conversation(other.owner)+"/messages", map[string]any{"kind": "message", "body": "x", "macro_id": macro})

	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "x", "macro_id": personal, "then": map[string]any{"assignee_id": other.agentID}})
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "x", "macro_id": personal, "then": map[string]any{"status": "snoozed"}})
	tm.agent.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "note", "body": "x", "mentions": []string{other.agentID}, "macro_id": personal})
	if n := len(messages(tm.owner, conv)); n != count {
		t.Fatalf("a refused send left %d messages, want %d", n, count)
	}
	if c := conversationOf(tm.owner, conv); c["status"] != "open" || c["priority"] != "normal" {
		t.Fatalf("a refused send applied actions: %v", c)
	}

	reader := tm.scopedKey(h, "conversations:read", "notes:write")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "note", "body": "x", "macro_id": macro})
	writer := tm.scopedKey(h, "conversations:read", "conversations:write", "notes:write")
	writer.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "x", "macro_id": macro})
	writer.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "note", "body": "x", "macro_id": personal})
	tm.agent.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "bye", "macro_id": personal})
	if conversationOf(tm.owner, conv)["status"] != "closed" {
		t.Fatal("personal macro not applied by its owner")
	}
}

func TestListMacrosTool(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Close", "shortcut": "close", "body": "Thanks!", "actions": map[string]any{"status": "closed", "assignee_id": nil},
	})
	tm.agent.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{"visibility": "personal", "name": "Mine", "shortcut": "mine", "body": "x"})
	cs := h.mcpSession(h.oauthToken(tm.owner, tm.ws, "Claude Code", testOrigin+"/mcp", []string{"conversations:read"}))
	out, errText := callTool(t, cs, "list_macros", map[string]any{})
	if errText != "" {
		t.Fatal(errText)
	}
	b, _ := json.Marshal(out["items"])
	var items []struct {
		Name    string         `json:"name"`
		Body    string         `json:"body"`
		Actions map[string]any `json:"actions"`
	}
	_ = json.Unmarshal(b, &items)
	if len(items) != 1 || items[0].Body != "Thanks!" || items[0].Actions["status"] != "closed" || items[0].Actions["unassign"] != true {
		t.Fatalf("list_macros: %s", b)
	}
}

func TestMacroAssigneeGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	macro := tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Hand over", "shortcut": "hand-over", "body": "Passing this on.",
		"actions": map[string]any{"assignee_id": tm.agentID, "status": "pending"},
	}).str("id")
	ghost := tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Ghost", "shortcut": "ghost", "body": "x", "actions": map[string]any{"priority": "high"},
	}).str("id")
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE macros SET actions = jsonb_set(actions, '{assignee_id}', to_jsonb($2::text)) WHERE id = $1`,
		uuid.MustParse(ghost), uuid.NewV7().String()); err != nil {
		t.Fatal(err)
	}
	if r := tm.owner.expect(http.StatusOK, "PATCH", "/v1/macros/"+ghost, map[string]any{"name": "Renamed"}); r.str("name") != "Renamed" {
		t.Fatalf("rename: %s", r.raw)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusCreated, "POST", "/v1/conversations/"+conv+"/messages", map[string]any{"kind": "message", "body": "Passing this on.", "macro_id": macro})
	if c := conversationOf(tm.owner, conv); c["status"] != "pending" || c["assignee_id"] != nil {
		t.Fatalf("macro with a skipped assignee: %v", c)
	}
	var skipped any
	for _, m := range messages(tm.owner, conv) {
		if ev, _ := m["event"].(map[string]any); ev != nil && ev["type"] == "macro_applied" {
			skipped = ev["skipped"]
		}
	}
	if !slices.Equal(asStrings(skipped), []string{"assignee"}) {
		t.Fatalf("macro_applied skipped %v", skipped)
	}
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/conversations/"+conv+"/messages",
		map[string]any{"kind": "message", "body": "x", "macro_id": macro, "then": map[string]any{"assignee_id": tm.agentID}})

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/members/"+tm.agentID, nil)
	if a := tm.owner.expect(http.StatusOK, "GET", "/v1/macros/"+macro, nil).body["actions"].(map[string]any); a["assignee_id"] != nil || a["status"] != "pending" {
		t.Fatalf("removed member kept in macro: %v", a)
	}
}

func asStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// waitBlocked waits until a statement matching like waits on a lock in the test database.
func waitBlocked(t *testing.T, h *harness, like string) {
	t.Helper()
	for range 200 {
		var n int
		if err := h.st.Pool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`, like).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no statement like %q waited on a lock", like)
}

func TestMacroLabelDeletedWhileSaving(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	ws := uuid.MustParse(tm.ws)

	gone := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "gone"}).str("id")
	tx, err := h.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM labels WHERE workspace_id = $1 AND id = $2", ws, uuid.MustParse(gone)); err != nil {
		t.Fatal(err)
	}
	saved := make(chan response, 1)
	go func() {
		saved <- tm.owner.do("POST", "/v1/macros", map[string]any{"visibility": "shared", "name": "Late", "shortcut": "late", "actions": map[string]any{"add_labels": []string{gone}}})
	}()
	waitBlocked(t, h, "%FOR SHARE%")
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if r := <-saved; r.status != http.StatusBadRequest || r.str("code") != "validation_failed" {
		t.Fatalf("save after the label was deleted: %d %s", r.status, r.raw)
	}

	kept := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "kept"}).str("id")
	tx, err = h.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT id FROM labels WHERE workspace_id = $1 AND id = $2 FOR SHARE", ws, uuid.MustParse(kept)); err != nil {
		t.Fatal(err)
	}
	macro := uuid.NewV7()
	if _, err := tx.Exec(ctx, `INSERT INTO macros (id, workspace_id, name, shortcut, actions, created_at, updated_at)
		VALUES ($1, $2, 'Early', 'early', jsonb_build_object('add_labels', jsonb_build_array($3::text)), now(), now())`, macro, ws, kept); err != nil {
		t.Fatal(err)
	}
	deleted := make(chan response, 1)
	go func() { deleted <- tm.owner.do("DELETE", "/v1/labels/"+kept, nil) }()
	waitBlocked(t, h, "%DELETE FROM labels%")
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if r := <-deleted; r.status != http.StatusNoContent {
		t.Fatalf("label delete: %d %s", r.status, r.raw)
	}
	if a := tm.owner.expect(http.StatusOK, "GET", "/v1/macros/"+macro.String(), nil).body["actions"].(map[string]any); a["add_labels"] != nil {
		t.Fatalf("label deleted after the save stayed in the macro: %v", a)
	}
}

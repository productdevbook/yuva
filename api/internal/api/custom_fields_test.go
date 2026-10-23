package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
)

func (tm team) newField(body map[string]any) string {
	return tm.owner.expect(http.StatusCreated, "POST", "/v1/custom-fields", body).str("id")
}

func expectField(t *testing.T, r response, status int, code, field string) {
	t.Helper()
	if r.status != status || r.str("code") != code || r.str("field") != field {
		t.Fatalf("want %d %s naming %q: %d %s", status, code, field, r.status, r.raw)
	}
}

func convFields(r response) map[string]any {
	f, _ := r.body["fields"].(map[string]any)
	return f
}

func (h *harness) runFieldClears(ws string) {
	h.t.Helper()
	for _, j := range h.jobs("custom_field_clear", ws) {
		var a api.CustomFieldClearArgs
		if err := json.Unmarshal(j.args, &a); err != nil {
			h.t.Fatal(err)
		}
		done, err := h.srv.ClearCustomFieldValues(context.Background(), a, time.Minute)
		if err != nil || !done {
			h.t.Fatalf("clear: done %v, %v", done, err)
		}
		h.dropJob(j.id)
	}
}

func TestCustomFieldDefinitions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)

	id := tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": " Plan ", "type": "select", "options": []string{"free", "pro"}, "required_on_close": true, "position": 2})
	got := tm.owner.expect(http.StatusOK, "GET", "/v1/custom-fields/"+id, nil)
	if got.str("name") != "Plan" || fmt.Sprint(got.body["options"]) != "[free pro]" || got.body["required_on_close"] != true || got.body["position"] != 2.0 {
		t.Fatalf("field %s", got.raw)
	}
	tm.newField(map[string]any{"object": "contact", "key": "plan", "name": "Plan", "type": "text"})

	for name, body := range map[string]map[string]any{
		"bad key":               {"object": "conversation", "key": "Plan", "name": "x", "type": "text"},
		"long key":              {"object": "conversation", "key": "a" + strings.Repeat("b", 40), "name": "x", "type": "text"},
		"bad type":              {"object": "conversation", "key": "x", "name": "x", "type": "color"},
		"bad object":            {"object": "message", "key": "x", "name": "x", "type": "text"},
		"no name":               {"object": "conversation", "key": "x", "name": " ", "type": "text"},
		"select no options":     {"object": "conversation", "key": "x", "name": "x", "type": "select"},
		"select dup options":    {"object": "conversation", "key": "x", "name": "x", "type": "select", "options": []string{"a", "a"}},
		"options on text":       {"object": "conversation", "key": "x", "name": "x", "type": "text", "options": []string{"a"}},
		"required on contact":   {"object": "contact", "key": "x", "name": "x", "type": "text", "required_on_close": true},
		"position out of range": {"object": "conversation", "key": "x", "name": "x", "type": "text", "position": 10001},
	} {
		if r := tm.owner.do("POST", "/v1/custom-fields", body); r.status != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, r.status, r.raw)
		}
	}
	tm.owner.expectProblem(http.StatusConflict, "field_taken", "POST", "/v1/custom-fields", map[string]any{"object": "conversation", "key": "plan", "name": "x", "type": "text"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/custom-fields/"+id, map[string]any{"options": []string{}})

	for i := range 49 {
		tm.newField(map[string]any{"object": "conversation", "key": fmt.Sprintf("f%d", i), "name": "x", "type": "text"})
	}
	tm.owner.expectProblem(http.StatusForbidden, "field_limit", "POST", "/v1/custom-fields", map[string]any{"object": "conversation", "key": "one_more", "name": "x", "type": "text"})
	tm.newField(map[string]any{"object": "contact", "key": "one_more", "name": "x", "type": "text"})

	list := tm.owner.expect(http.StatusOK, "GET", "/v1/custom-fields?object=contact", nil)
	if n := len(list.body["items"].([]any)); n != 2 {
		t.Fatalf("contact fields %d", n)
	}
	if n := len(tm.owner.expect(http.StatusOK, "GET", "/v1/custom-fields", nil).body["items"].([]any)); n != 52 {
		t.Fatalf("all fields %d", n)
	}

	tm.agent.expect(http.StatusOK, "GET", "/v1/custom-fields", nil)
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/custom-fields", map[string]any{"object": "contact", "key": "x", "name": "x", "type": "text"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "DELETE", "/v1/custom-fields/"+id, nil)

	plain := tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "default"})
	if slices.Contains(plain.body["api_key"].(map[string]any)["scopes"].([]any), any("fields:manage")) {
		t.Fatalf("fields:manage implied: %s", plain.raw)
	}
	k := h.client()
	k.bearer = plain.str("secret")
	k.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/custom-fields", map[string]any{"object": "contact", "key": "x", "name": "x", "type": "text"})
	k.expect(http.StatusOK, "GET", "/v1/custom-fields", nil)
	tm.scopedKey(h, "contacts:read").expect(http.StatusOK, "GET", "/v1/custom-fields", nil)
	tm.scopedKey(h, "lists:read").expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/custom-fields", nil)
	manager := tm.scopedKey(h, "fields:manage")
	manager.expect(http.StatusOK, "PATCH", "/v1/custom-fields/"+id, map[string]any{"name": "Tier"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/api-keys", map[string]any{"name": "limited", "scopes": []string{"fields:manage"}, "inbox_ids": []string{tm.inbox}})

	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/custom-fields/"+id, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", "/v1/custom-fields/"+id, map[string]any{"name": "Mine"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", "/v1/custom-fields/"+id, nil)
	if n := len(other.owner.expect(http.StatusOK, "GET", "/v1/custom-fields", nil).body["items"].([]any)); n != 0 {
		t.Fatalf("other workspace sees %d fields", n)
	}
	other.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})
}

func TestConversationFieldValues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "order_id", "name": "Order", "type": "text"})
	tm.newField(map[string]any{"object": "conversation", "key": "amount", "name": "Amount", "type": "number"})
	tm.newField(map[string]any{"object": "conversation", "key": "due", "name": "Due", "type": "date"})
	tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "select", "options": []string{"free", "pro"}})
	tm.newField(map[string]any{"object": "conversation", "key": "vip", "name": "VIP", "type": "checkbox"})
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv

	if f := convFields(tm.owner.expect(http.StatusOK, "GET", path, nil)); f == nil || len(f) != 0 {
		t.Fatalf("new conversation fields %v", f)
	}
	for _, bad := range []map[string]any{
		{"order_id": ""}, {"order_id": 12}, {"order_id": strings.Repeat("x", 1001)},
		{"amount": "12"}, {"amount": true},
		{"due": "2026-13-01"}, {"due": "1 May"}, {"due": 20260501},
		{"plan": "enterprise"}, {"plan": 1},
		{"vip": "true"}, {"vip": 1},
		{"unknown": "x"},
	} {
		var key string
		for k := range bad {
			key = k
		}
		expectField(t, tm.owner.do("PATCH", path, map[string]any{"fields": bad}), http.StatusBadRequest, "field_invalid", key)
	}

	after := lastEvent(tm.owner)
	r := tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{
		"order_id": " A-1 ", "amount": 12.5, "due": "2026-05-01", "plan": "pro", "vip": false,
	}})
	want := map[string]any{"order_id": "A-1", "amount": 12.5, "due": "2026-05-01", "plan": "pro", "vip": false}
	if fmt.Sprint(convFields(r)) != fmt.Sprint(want) {
		t.Fatalf("fields %v", convFields(r))
	}
	var updated map[string]any
	for _, e := range feed(tm.owner, after) {
		if e["type"] == "conversation.updated" {
			updated = e["data"].(map[string]any)["fields"].(map[string]any)
		}
	}
	if fmt.Sprint(updated) != fmt.Sprint(want) {
		t.Fatalf("conversation.updated fields %v", updated)
	}

	r = tm.agent.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"vip": nil, "amount": 3}})
	if f := convFields(r); f["vip"] != nil || f["amount"] != 3.0 || f["order_id"] != "A-1" {
		t.Fatalf("after clearing vip %v", f)
	}
	after = lastEvent(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"amount": 3}})
	if len(feed(tm.owner, after)) != 0 {
		t.Fatal("an unchanged value sent an event")
	}

	reader := tm.scopedKey(h, "conversations:read")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "PATCH", path, map[string]any{"fields": map[string]any{"amount": 4}})
	writer := tm.scopedKey(h, "conversations:write", "conversations:read")
	writer.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"amount": 4}})

	second := tm.conversation(tm.owner)
	res := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv, second}, "fields": map[string]any{"plan": "free"}})
	if n := len(res.body["updated"].([]any)); n != 2 {
		t.Fatalf("bulk %s", res.raw)
	}
	expectField(t, tm.owner.do("POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv}, "fields": map[string]any{"plan": "gold"}}), http.StatusBadRequest, "field_invalid", "plan")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv}, "fields": map[string]any{"plan": "free"}})

	other := newTeam(t, h)
	expectField(t, other.owner.do("PATCH", "/v1/conversations/"+other.conversation(other.owner), map[string]any{"fields": map[string]any{"plan": "free"}}), http.StatusBadRequest, "field_invalid", "plan")
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", path, map[string]any{"fields": map[string]any{"plan": "free"}})
}

func TestConversationFieldFilter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "select", "options": []string{"free", "pro"}})
	tm.newField(map[string]any{"object": "conversation", "key": "seats", "name": "Seats", "type": "number"})
	tm.newField(map[string]any{"object": "conversation", "key": "vip", "name": "VIP", "type": "checkbox"})
	a, b, c := tm.conversation(tm.owner), tm.conversation(tm.owner), tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+a, map[string]any{"fields": map[string]any{"plan": "pro", "seats": 5, "vip": true}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+b, map[string]any{"fields": map[string]any{"plan": "pro", "seats": 2}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+c, map[string]any{"fields": map[string]any{"plan": "free"}})

	for q, want := range map[string][]string{
		"&field.plan=pro":                 {a, b},
		"&field.plan=free":                {c},
		"&field.seats=5":                  {a},
		"&field.seats=5.0":                {a},
		"&field.vip=true":                 {a},
		"&field.plan=pro&field.seats=2":   {b},
		"&field.plan=pro&field.vip=false": nil,
		"&field.plan=pro&status=closed":   nil,
	} {
		got := listedIDs(tm.owner, q)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("%s lists %v, want %v", q, got, want)
		}
	}
	for q, key := range map[string]string{
		"&field.nope=1":                   "nope",
		"&field.seats=many":               "seats",
		"&field.vip=yes":                  "vip",
		"&field.plan=pro&field.plan=free": "plan",
	} {
		expectField(t, tm.owner.do("GET", "/v1/conversations?limit=100"+q, nil), http.StatusBadRequest, "field_invalid", key)
	}

	other := newTeam(t, h)
	other.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})
	other.conversation(other.owner)
	if got := listedIDs(other.owner, "&field.plan=pro"); len(got) != 0 {
		t.Fatalf("other workspace lists %v", got)
	}
}

func TestRequiredOnClose(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "reason", "name": "Reason", "type": "select", "options": []string{"bug", "billing"}, "required_on_close": true, "position": 1})
	tm.newField(map[string]any{"object": "conversation", "key": "resolution", "name": "Resolution", "type": "text", "required_on_close": true, "position": 2})
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv

	expectField(t, tm.owner.do("PATCH", path, map[string]any{"status": "closed"}), http.StatusConflict, "field_required", "reason")
	expectField(t, tm.owner.do("PATCH", path, map[string]any{"status": "closed", "fields": map[string]any{"reason": "bug"}}), http.StatusConflict, "field_required", "resolution")
	if st := tm.owner.expect(http.StatusOK, "GET", path, nil).str("status"); st != "open" {
		t.Fatalf("refused close left status %s", st)
	}
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"status": "pending"})

	ready := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+ready, map[string]any{"fields": map[string]any{"reason": "billing", "resolution": "Refunded"}})
	res := tm.owner.expect(http.StatusOK, "POST", "/v1/conversations/bulk", map[string]any{"conversation_ids": []string{conv, ready}, "status": "closed"})
	failed := res.body["failed"].([]any)
	if len(res.body["updated"].([]any)) != 1 || len(failed) != 1 {
		t.Fatalf("bulk close %s", res.raw)
	}
	if f := failed[0].(map[string]any); f["id"] != conv || f["status"] != 409.0 || f["code"] != "field_required" || f["field"] != "reason" {
		t.Fatalf("bulk failure %v", f)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+ready, map[string]any{"fields": map[string]any{"resolution": nil}})

	macro := tm.owner.expect(http.StatusCreated, "POST", "/v1/macros", map[string]any{
		"visibility": "shared", "name": "Close", "shortcut": "close", "body": "Done.", "actions": map[string]any{"status": "closed"},
	}).str("id")
	expectField(t, tm.owner.do("POST", path+"/messages", map[string]any{"kind": "message", "body": "Done.", "macro_id": macro}), http.StatusConflict, "field_required", "reason")
	expectField(t, tm.owner.do("POST", path+"/messages", map[string]any{"kind": "message", "body": "Done.", "then": map[string]any{"status": "closed"}}), http.StatusConflict, "field_required", "reason")
	msgs := tm.owner.expect(http.StatusOK, "GET", path+"/messages", nil).body["items"].([]any)
	for _, m := range msgs {
		if m.(map[string]any)["body"] == "Done." {
			t.Fatal("a refused close kept its message")
		}
	}
	r := tm.owner.expect(http.StatusCreated, "POST", path+"/messages", map[string]any{"kind": "message", "body": "Done.", "macro_id": macro,
		"then": map[string]any{"fields": map[string]any{"reason": "bug", "resolution": "Fixed"}}})
	_ = r
	if got := tm.owner.expect(http.StatusOK, "GET", path, nil); got.str("status") != "closed" || convFields(got)["resolution"] != "Fixed" {
		t.Fatalf("macro with then fields: %s", got.raw)
	}

	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"resolution": nil}})
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"subject": "Still closed"})

	spam := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+spam, map[string]any{"spam": true, "status": "closed"})

	key := tm.scopedKey(h, "conversations:read", "conversations:write")
	cs := h.mcpSession(key.bearer)
	open := tm.conversation(tm.owner)
	if _, errText := callTool(t, cs, "set_status", map[string]any{"conversation_id": open, "status": "closed"}); !strings.Contains(errText, "reason") {
		t.Fatalf("MCP close without the field: %q", errText)
	}
	out, errText := callTool(t, cs, "bulk_update", map[string]any{"conversation_ids": []string{open}, "status": "closed"})
	if errText != "" || out["failed"].([]any)[0].(map[string]any)["field"] != "reason" {
		t.Fatalf("MCP bulk close: %v %q", out, errText)
	}
	if _, errText := callTool(t, cs, "set_conversation_fields", map[string]any{"conversation_id": open, "fields": map[string]any{"reason": "bug", "resolution": "ok"}}); errText != "" {
		t.Fatal(errText)
	}
	if _, errText := callTool(t, cs, "set_status", map[string]any{"conversation_id": open, "status": "closed"}); errText != "" {
		t.Fatal(errText)
	}
}

func TestCustomFieldDeletionClearsValues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	plan := tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "select", "options": []string{"free", "pro", "team"}})
	tm.newField(map[string]any{"object": "conversation", "key": "note", "name": "Note", "type": "text"})
	contactField := tm.newField(map[string]any{"object": "contact", "key": "tier", "name": "Tier", "type": "text"})
	a, b := tm.conversation(tm.owner), tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+a, map[string]any{"fields": map[string]any{"plan": "pro", "note": "x"}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+b, map[string]any{"fields": map[string]any{"plan": "free"}})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+tm.contact, map[string]any{"attributes": map[string]any{"tier": "gold"}})

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/custom-fields/"+plan, map[string]any{"options": []string{"free", "team"}})
	if jobs := h.jobs("custom_field_clear", tm.ws); len(jobs) != 1 {
		t.Fatalf("option removal queued %d jobs", len(jobs))
	}
	after := lastEvent(tm.owner)
	h.runFieldClears(tm.ws)
	if f := convFields(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+a, nil)); f["plan"] != nil || f["note"] != "x" {
		t.Fatalf("after option removal a has %v", f)
	}
	if f := convFields(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+b, nil)); f["plan"] != "free" {
		t.Fatalf("after option removal b has %v", f)
	}
	var events []string
	for _, e := range feed(tm.owner, after) {
		events = append(events, fmt.Sprint(e["type"], " ", e["conversation_id"]))
	}
	if !slices.Equal(events, []string{"conversation.updated " + a}) {
		t.Fatalf("events %v", events)
	}

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+plan, nil)
	tm.owner.expectProblem(http.StatusConflict, "field_clearing", "POST", "/v1/custom-fields", map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})
	expectField(t, tm.owner.do("PATCH", "/v1/conversations/"+b, map[string]any{"fields": map[string]any{"plan": "free"}}), http.StatusBadRequest, "field_invalid", "plan")
	h.runFieldClears(tm.ws)
	if f := convFields(tm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+b, nil)); len(f) != 0 {
		t.Fatalf("after deletion b has %v", f)
	}
	tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})

	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+contactField, nil)
	if jobs := h.jobs("custom_field_clear", tm.ws); len(jobs) != 0 {
		t.Fatalf("contact field deletion queued %d jobs", len(jobs))
	}
	attrs := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+tm.contact, nil).body["attributes"].(map[string]any)
	if attrs["tier"] != "gold" {
		t.Fatalf("contact attributes %v", attrs)
	}
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+tm.contact, map[string]any{"attributes": map[string]any{"tier": 3}})
}

func TestContactFieldValidation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", false)
	tm := ct.team
	tm.newField(map[string]any{"object": "contact", "key": "seats", "name": "Seats", "type": "number"})
	tm.newField(map[string]any{"object": "contact", "key": "renews", "name": "Renews", "type": "date"})

	expectField(t, tm.owner.do("POST", "/v1/contacts", map[string]any{"name": "B", "attributes": map[string]any{"seats": "ten"}}), http.StatusBadRequest, "field_invalid", "seats")
	expectField(t, tm.owner.do("PATCH", "/v1/contacts/"+tm.contact, map[string]any{"attributes": map[string]any{"renews": "soon"}}), http.StatusBadRequest, "field_invalid", "renews")
	r := tm.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+tm.contact, map[string]any{"attributes": map[string]any{"seats": 10, "renews": nil, "color": map[string]any{"any": "thing"}}})
	if a := r.body["attributes"].(map[string]any); a["seats"] != 10.0 || a["color"] == nil {
		t.Fatalf("attributes %v", a)
	}

	cs := h.mcpSession(tm.scopedKey(h, "contacts:read", "contacts:write").bearer)
	if _, errText := callTool(t, cs, "update_contact", map[string]any{"contact_id": tm.contact, "attributes": map[string]any{"seats": "x"}}); !strings.Contains(errText, "seats") {
		t.Fatalf("MCP update_contact: %q", errText)
	}

	user := ct.session(h, map[string]any{"identity_token": ct.token(h, map[string]any{
		"sub": "u-" + unique("f"), "name": "Ada", "attrs": map[string]any{"seats": "many", "renews": "2026-12-01", "plan": "pro"},
	})})
	attrs := tm.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+user.contactID, nil).body["attributes"].(map[string]any)
	if _, ok := attrs["seats"]; ok || attrs["renews"] != "2026-12-01" || attrs["plan"] != "pro" {
		t.Fatalf("identity attributes %v", attrs)
	}

	other := newTeam(t, h)
	other.owner.expect(http.StatusOK, "PATCH", "/v1/contacts/"+other.contact, map[string]any{"attributes": map[string]any{"seats": "ten"}})
}

func TestCustomFieldsMCP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "select", "options": []string{"free", "pro"}})
	conv, rest := tm.conversation(tm.owner), tm.conversation(tm.owner)

	reader := h.mcpSession(tm.scopedKey(h, "conversations:read").bearer)
	tools := toolNames(t, reader)
	if !slices.Contains(tools, "list_custom_fields") || slices.Contains(tools, "set_conversation_fields") {
		t.Fatalf("reader tools %v", tools)
	}
	out, errText := callTool(t, reader, "list_custom_fields", map[string]any{"object": "conversation"})
	if errText != "" || len(out["items"].([]any)) != 1 || out["items"].([]any)[0].(map[string]any)["key"] != "plan" {
		t.Fatalf("list_custom_fields %v %q", out, errText)
	}

	cs := h.mcpSession(tm.scopedKey(h, "conversations:read", "conversations:write").bearer)
	if _, errText := callTool(t, cs, "set_conversation_fields", map[string]any{"conversation_id": conv, "fields": map[string]any{"plan": "gold"}}); !strings.Contains(errText, "plan") {
		t.Fatalf("invalid value: %q", errText)
	}
	out, errText = callTool(t, cs, "set_conversation_fields", map[string]any{"conversation_id": conv, "fields": map[string]any{"plan": "pro"}})
	if errText != "" || out["fields"].(map[string]any)["plan"] != "pro" {
		t.Fatalf("set_conversation_fields %v %q", out, errText)
	}
	out, errText = callTool(t, cs, "search_conversations", map[string]any{"fields": map[string]any{"plan": "pro"}})
	items, _ := out["items"].([]any)
	if errText != "" || len(items) != 1 || items[0].(map[string]any)["id"] != conv {
		t.Fatalf("search by field %v %q (other %s)", out, errText, rest)
	}
	got, errText := callTool(t, cs, "get_conversation", map[string]any{"conversation_id": conv})
	if errText != "" || got["conversation"].(map[string]any)["fields"].(map[string]any)["plan"] != "pro" {
		t.Fatalf("get_conversation %v %q", got, errText)
	}
}

func TestCustomFieldClearsRepeat(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv
	for round := range 3 {
		id := tm.newField(map[string]any{"object": "conversation", "key": "plan", "name": "Plan", "type": "text"})
		tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"plan": fmt.Sprint("r", round)}})
		tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+id, nil)
		if jobs := h.jobs("custom_field_clear", tm.ws); len(jobs) != 1 {
			t.Fatalf("round %d queued %d clears", round, len(jobs))
		}
		h.runFieldClears(tm.ws)
		if f := convFields(tm.owner.expect(http.StatusOK, "GET", path, nil)); len(f) != 0 {
			t.Fatalf("round %d left %v", round, f)
		}
	}

	sel := tm.newField(map[string]any{"object": "conversation", "key": "tier", "name": "Tier", "type": "select", "options": []string{"a", "b"}})
	for round := range 3 {
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/custom-fields/"+sel, map[string]any{"options": []string{"a", "b"}})
		tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"tier": "b"}})
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/custom-fields/"+sel, map[string]any{"options": []string{"a"}})
		h.runFieldClears(tm.ws)
		if f := convFields(tm.owner.expect(http.StatusOK, "GET", path, nil)); f["tier"] != nil {
			t.Fatalf("option round %d left %v", round, f)
		}
	}

	id := tm.newField(map[string]any{"object": "conversation", "key": "lost", "name": "Lost", "type": "text"})
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"fields": map[string]any{"lost": "x"}})
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+id, nil)
	for _, j := range h.jobs("custom_field_clear", tm.ws) {
		h.dropJob(j.id)
	}
	tm.owner.expectProblem(http.StatusConflict, "field_clearing", "POST", "/v1/custom-fields", map[string]any{"object": "conversation", "key": "lost", "name": "Lost", "type": "text"})
	if jobs := h.jobs("custom_field_clear", tm.ws); len(jobs) != 1 {
		t.Fatalf("a refused create queued %d clears", len(jobs))
	}
	h.runFieldClears(tm.ws)
	tm.newField(map[string]any{"object": "conversation", "key": "lost", "name": "Lost", "type": "text"})
}

func TestCustomFieldClearWalksBatches(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	id := tm.newField(map[string]any{"object": "conversation", "key": "bulk", "name": "Bulk", "type": "text"})
	conv := tm.conversation(tm.owner)
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"fields": map[string]any{"bulk": "x"}})
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO conversations
		SELECT (jsonb_populate_record(c, jsonb_build_object('id', gen_random_uuid(), 'email_token', null))).*
		FROM conversations c, generate_series(1, 1200) WHERE c.id = $1`, conv); err != nil {
		t.Fatal(err)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", "/v1/custom-fields/"+id, nil)
	h.runFieldClears(tm.ws)
	var left int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM conversations WHERE workspace_id = $1 AND fields ? 'bulk'", tm.ws).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d conversations keep the value", left)
	}
}

func TestRequiredOnCloseWhenSpamIsLifted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	tm.newField(map[string]any{"object": "conversation", "key": "reason", "name": "Reason", "type": "text", "required_on_close": true})
	conv := tm.conversation(tm.owner)
	path := "/v1/conversations/" + conv
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"spam": true, "status": "closed"})
	expectField(t, tm.owner.do("PATCH", path, map[string]any{"spam": false}), http.StatusConflict, "field_required", "reason")
	expectField(t, tm.owner.do("POST", path+"/messages", map[string]any{"kind": "note", "body": "not spam", "then": map[string]any{"spam": false}}), http.StatusConflict, "field_required", "reason")
	if got := tm.owner.expect(http.StatusOK, "GET", path, nil); got.body["spam"] != true || got.str("status") != "closed" {
		t.Fatalf("refused unspam changed %s", got.raw)
	}
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"spam": false, "status": "open"})
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"spam": true, "status": "closed"})
	tm.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"spam": false, "fields": map[string]any{"reason": "ok"}})
}

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestWorkspaceRetention(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)

	open := h.ingest(et.address, buildMail(mailOpts{
		from: "keep@example.net", to: et.address, subject: "Still open", messageID: newMessageID(), body: "Hello",
	}), nil)
	if open.status != http.StatusAccepted {
		t.Fatalf("ingest open: %d %v", open.status, open.body)
	}
	closed := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
	if closed.status != http.StatusAccepted {
		t.Fatalf("ingest closed: %d %v", closed.status, closed.body)
	}
	closedConv := closed.str("conversation_id")
	att := lastOf(messages(et.owner, closedConv), "message")["attachments"].([]any)[0].(map[string]any)["id"].(string)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+closedConv, map[string]any{"status": "closed"})

	other := newTeam(t, h)
	otherConv := other.conversation(other.owner)
	other.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+otherConv, map[string]any{"status": "closed"})

	et.agent.expectProblem(http.StatusForbidden, "forbidden", "PATCH", "/v1/workspace", map[string]any{"retention_days": 30})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/workspace", map[string]any{"retention_days": 0})
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", "/v1/workspace", map[string]any{})
	set := et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 1})
	if set.body["retention_days"] != float64(1) {
		t.Fatalf("set: %s", set.raw)
	}
	if ws := et.agent.expect(http.StatusOK, "GET", "/v1/workspace", nil); ws.body["retention_days"] != float64(1) {
		t.Fatalf("get: %s", ws.raw)
	}

	h.clock.Advance(23 * time.Hour)
	if err := h.srv.ApplyRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+closedConv, nil)
	et.owner.expect(http.StatusOK, "GET", "/v1/messages/"+open.str("message_id")+"/raw", nil)

	h.clock.Advance(2 * time.Hour)
	if err := h.srv.ApplyRetention(context.Background()); err != nil {
		t.Fatal(err)
	}
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+closedConv, nil)
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/attachments/"+att, nil)
	et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+open.str("conversation_id"), nil)
	et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/messages/"+open.str("message_id")+"/raw", nil)
	m := lastOf(messages(et.owner, open.str("conversation_id")), "message")
	if m["body"] != "Hello" || m["email"].(map[string]any)["raw"] != false {
		t.Fatalf("open conversation message after retention: %v", m)
	}
	other.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+otherConv, nil)

	cleared := et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": nil})
	if _, ok := cleared.body["retention_days"]; ok {
		t.Fatalf("cleared: %s", cleared.raw)
	}
}

package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestDeleteContactsInBulk(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	shared := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil).str("conversation_id")
	pm := et.personalMailbox(h)
	private := h.ingest(pm.address, []byte(fmt.Sprintf(attachmentMail, pm.address, newMessageID())), nil).str("conversation_id")
	customer := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+shared, nil).str("contact_id")
	if got := pm.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+private, nil).str("contact_id"); got != customer {
		t.Fatalf("mailbox contact %s, shared %s", got, customer)
	}
	empty := et.owner.expect(http.StatusCreated, "POST", "/v1/contacts", map[string]any{"name": "Nobody", "emails": []string{unique("nobody") + "@example.com"}}).str("id")
	other := newTeam(t, h)

	preview := func(c *client, id string) string {
		r := c.expect(http.StatusOK, "GET", "/v1/contacts/"+id+"/deletion-preview", nil)
		return fmt.Sprintf("%v %v", r.body["conversation_count"], r.body["has_hidden_conversations"])
	}
	if got := preview(et.owner, customer); got != "1 true" {
		t.Fatalf("owner preview: %s", got)
	}
	if got := preview(et.scopedKey(h, "contacts:read", "conversations:read"), customer); got != "1 true" {
		t.Fatalf("key preview: %s", got)
	}
	if got := preview(et.owner, empty); got != "0 false" {
		t.Fatalf("empty preview: %s", got)
	}
	et.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+shared, nil)
	if got := preview(et.owner, customer); got != "1 true" {
		t.Fatalf("preview with the conversation in the trash: %s", got)
	}
	pm.owner.expectProblem(http.StatusForbidden, "forbidden", "GET", "/v1/contacts/"+customer+"/deletion-preview", nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/contacts/"+customer+"/deletion-preview", nil)
	pm.owner.expect(http.StatusOK, "PUT", "/v1/inboxes/"+pm.inbox+"/delegates/"+et.ownerMemberID(t), map[string]any{"access": "send"})
	if got := preview(et.owner, customer); got != "2 false" {
		t.Fatalf("delegate's preview: %s", got)
	}
	h.trashBySQL(et.ws, private)
	if got := preview(et.owner, customer); got != "1 true" {
		t.Fatalf("delegate's preview with the mailbox's conversation in the trash: %s", got)
	}

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = empty
	}
	et.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/contacts/delete", map[string]any{"ids": ids, "confirm_count": 1})
	r := et.owner.expect(http.StatusConflict, "POST", "/v1/contacts/delete", map[string]any{"ids": []string{empty, empty, customer}, "confirm_count": 3})
	if r.str("code") != "count_changed" || r.body["count"] != float64(2) {
		t.Fatalf("mismatch: %s", r.raw)
	}
	et.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/contacts/delete", map[string]any{"ids": []string{empty}, "confirm_count": 1})
	et.scopedKey(h, "contacts:read").expectProblem(http.StatusForbidden, "insufficient_scope", "POST", "/v1/contacts/delete", map[string]any{"ids": []string{empty}, "confirm_count": 1})
	if n := h.count("SELECT count(*) FROM contacts WHERE workspace_id = $1 AND id = ANY($2::uuid[])", et.ws, []string{empty, customer}); n != 2 {
		t.Fatalf("%d contacts left after refusals", n)
	}

	r = et.owner.expect(http.StatusOK, "POST", "/v1/contacts/delete", map[string]any{"ids": []string{customer, other.contact, empty, customer}, "confirm_count": 3})
	f := failures(r)
	if r.body["deleted"] != float64(2) || len(f) != 1 || f[other.contact] != "404 not_found" {
		t.Fatalf("delete: %s", r.raw)
	}
	if h.conversationRows(et.ws, shared, private) != 0 {
		t.Fatal("the contact's conversations are still there")
	}
	if n := h.count("SELECT count(*) FROM contacts WHERE workspace_id = $1 AND id = ANY($2::uuid[])", et.ws, []string{empty, customer}); n != 0 {
		t.Fatalf("%d contacts left", n)
	}
	other.owner.expect(http.StatusOK, "GET", "/v1/contacts/"+other.contact, nil)

	entries := auditOf(auditLog(t, et.owner, "&action=contact.deleted"), "contact.deleted")
	targets := map[string]bool{}
	for _, e := range entries {
		if len(detailsOf(e)) != 0 || targetOf(e)["type"] != "contact" {
			t.Fatalf("audit entry: %v", e)
		}
		targets[targetOf(e)["id"].(string)] = true
	}
	if len(entries) != 2 || !targets[customer] || !targets[empty] {
		t.Fatalf("audit: %v", entries)
	}
}

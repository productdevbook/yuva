package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func (h *harness) checkDeletesQueued(ws string, keys []string) {
	h.t.Helper()
	if len(keys) == 0 {
		h.t.Fatal("no stored files to delete")
	}
	for _, k := range keys {
		if !h.stored(k) {
			h.t.Fatalf("%s deleted before its job ran", k)
		}
	}
	if len(h.jobs("storage_delete", ws)) == 0 {
		h.t.Fatal("no storage_delete job queued")
	}
	h.runStorageDeletes(ws)
	for _, k := range keys {
		if h.stored(k) {
			h.t.Fatalf("%s still stored after its job ran", k)
		}
	}
}

func TestDeletionQueuesStorageDeletes(t *testing.T) {
	h := newHarness(t)
	ingest := func(et emailTeam) (string, []string) {
		t.Helper()
		in := h.ingest(et.address, []byte(fmt.Sprintf(attachmentMail, et.address, newMessageID())), nil)
		if in.status != http.StatusAccepted {
			t.Fatalf("ingest: %d %v", in.status, in.body)
		}
		keys := h.storedKeys(et.ws)
		if len(keys) < 2 {
			t.Fatalf("stored files: %v", keys)
		}
		return in.str("conversation_id"), keys
	}

	t.Run("contact", func(t *testing.T) {
		et := newEmailTeam(t, h, false)
		conv, keys := ingest(et)
		contact := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil).str("contact_id")
		et.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+contact, nil)
		h.checkDeletesQueued(et.ws, keys)
	})

	t.Run("inbox", func(t *testing.T) {
		et := newEmailTeam(t, h, false)
		_, keys := ingest(et)
		et.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+et.inbox, nil)
		h.checkDeletesQueued(et.ws, keys)
	})

	t.Run("retention of raw mail", func(t *testing.T) {
		et := newEmailTeam(t, h, false)
		conv, _ := ingest(et)
		raw := h.storedRawKeys(et.ws)
		et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 1})
		h.clock.Advance(26 * time.Hour)
		if err := h.srv.ApplyRetention(context.Background()); err != nil {
			t.Fatal(err)
		}
		et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
		h.checkDeletesQueued(et.ws, raw)
	})

	t.Run("retention of conversations", func(t *testing.T) {
		et := newEmailTeam(t, h, false)
		conv, keys := ingest(et)
		et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
		et.owner.expect(http.StatusOK, "PATCH", "/v1/workspace", map[string]any{"retention_days": 1})
		h.clock.Advance(26 * time.Hour)
		if err := h.srv.ApplyRetention(context.Background()); err != nil {
			t.Fatal(err)
		}
		et.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/conversations/"+conv, nil)
		h.checkDeletesQueued(et.ws, keys)
	})
}

func (h *harness) storedRawKeys(ws string) []string {
	h.t.Helper()
	rows, err := h.st.Pool.Query(context.Background(), "SELECT raw_key FROM message_emails WHERE workspace_id = $1 AND raw_key IS NOT NULL", ws)
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

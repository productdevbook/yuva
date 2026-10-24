package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

func listedAttachments(c *client, inbox string) map[string]any {
	c.h.t.Helper()
	out := map[string]any{}
	for _, it := range c.expect(http.StatusOK, "GET", "/v1/conversations?inbox_id="+inbox+"&limit=50", nil).body["items"].([]any) {
		m := it.(map[string]any)
		out[m["id"].(string)] = m["has_attachments"]
	}
	return out
}

func TestConversationListHasAttachments(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := newEmailTeam(t, h, false)
	b := newEmailTeam(t, h, false)

	withFile := h.ingest(a.address, []byte(fmt.Sprintf(attachmentMail, a.address, newMessageID())), nil)
	plain := h.ingest(a.address, buildMail(mailOpts{from: "plain@example.net", to: a.address, subject: "No files", messageID: newMessageID(), body: "Just text."}), nil)
	other := h.ingest(b.address, []byte(fmt.Sprintf(attachmentMail, b.address, newMessageID())), nil)

	got := listedAttachments(a.owner, a.inbox)
	if got[withFile.str("conversation_id")] != true || got[plain.str("conversation_id")] != false || len(got) != 2 {
		t.Fatalf("workspace a: %v", got)
	}
	if got := listedAttachments(b.owner, b.inbox); got[other.str("conversation_id")] != true || len(got) != 1 {
		t.Fatalf("workspace b: %v", got)
	}

	a.owner.expect(http.StatusOK, "POST", "/v1/messages/"+withFile.str("message_id")+"/redact", nil)
	if got := listedAttachments(a.owner, a.inbox); got[withFile.str("conversation_id")] != false {
		t.Fatalf("after redaction: %v", got)
	}
	if got := listedAttachments(b.owner, b.inbox); got[other.str("conversation_id")] != true {
		t.Fatalf("workspace b after a's redaction: %v", got)
	}
}

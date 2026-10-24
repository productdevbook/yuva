package api_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"
	"uuid"
)

func TestSearchOperatorsDatesNeedOneMessage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	r := h.ingest(et.address, buildMail(mailOpts{from: "dates@dates-test.example", to: et.address, subject: "Two dates", messageID: newMessageID(), body: "first"}), nil)
	conv := r.str("conversation_id")
	ws := uuid.MustParse(et.ws)
	ctx := context.Background()
	if _, err := h.st.Pool.Exec(ctx, "UPDATE messages SET created_at = $3 WHERE workspace_id = $1 AND conversation_id = $2", ws, uuid.MustParse(conv), time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO messages (id, workspace_id, conversation_id, kind, direction, author_type, body, created_at)
		SELECT gen_random_uuid(), workspace_id, conversation_id, 'message', 'in', 'contact', 'again', $3 FROM messages WHERE workspace_id = $1 AND conversation_id = $2 LIMIT 1`,
		ws, uuid.MustParse(conv), time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string][]string{
		"after:2026-04-01 before:2026-03-15":                      nil,
		"after:2026-03-01 before:2026-03-15":                      {conv},
		"after:2026-04-01 before:2026-06-01":                      {conv},
		"-after:2026-04-01 -before:2026-03-15":                    nil,
		"-after:2026-06-01 -before:2026-03-01 subject:two":        {conv},
		"after:2026-03-01 after:2026-04-01 before:2026-06-01":     {conv},
		"after:2026-03-01 after:2026-04-01 before:2026-03-15":     nil,
		"after:2026-03-01 before:2026-03-15 before:2026-06-01":    {conv},
		"after:2026-03-11 before:2026-04-30 subject:two":          nil,
		"-after:2026-03-11 -before:2026-03-11 subject:two":        nil,
		"after:2026-03-10 before:2026-03-11 subject:two -from:me": {conv},
	} {
		got := itemIDs(et.owner.expect(http.StatusOK, "GET", "/v1/conversations?q="+url.QueryEscape(q), nil))
		if !slices.Equal(got, want) {
			t.Fatalf("q=%q: got %v, want %v", q, got, want)
		}
	}
}

func TestSearchOperatorsSkipRedactedMessages(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	sender := "gone@redact-test.example"
	r := h.ingest(et.address, buildMail(mailOpts{from: sender, to: et.address, subject: "Redact me", messageID: newMessageID(), body: "secret"}), nil)
	conv := r.str("conversation_id")
	for _, q := range []string{"from:" + sender, "to:" + et.address} {
		if got := itemIDs(et.owner.expect(http.StatusOK, "GET", "/v1/conversations?q="+url.QueryEscape(q), nil)); !slices.Equal(got, []string{conv}) {
			t.Fatalf("q=%q before the redaction: %v", q, got)
		}
	}
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE messages SET redacted_at = now() WHERE workspace_id = $1 AND conversation_id = $2", uuid.MustParse(et.ws), uuid.MustParse(conv)); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"from:" + sender, "to:" + et.address, "cc:" + sender} {
		if got := itemIDs(et.owner.expect(http.StatusOK, "GET", "/v1/conversations?q="+url.QueryEscape(q), nil)); len(got) != 0 {
			t.Fatalf("q=%q after the redaction: %v", q, got)
		}
	}
}

func TestSavedViewWithSearchOperators(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	sender := "viewer@views-test.example"
	h.ingest(et.address, buildMail(mailOpts{from: sender, to: et.address, subject: "For a view", messageID: newMessageID(), body: "hello"}), nil)
	view := et.newView(et.owner, map[string]any{"visibility": "personal", "name": "From a friend", "filters": map[string]any{"q": "from:" + sender + " -to:nobody@nowhere.example"}})
	if view.body["count"] != float64(1) {
		t.Fatalf("view count: %s", view.raw)
	}
	other := et.newView(et.owner, map[string]any{"visibility": "personal", "name": "Nobody", "filters": map[string]any{"q": "from:someone-else@views-test.example"}})
	if other.body["count"] != float64(0) {
		t.Fatalf("other view count: %s", other.raw)
	}
	if r := et.owner.do("POST", "/v1/views", map[string]any{"visibility": "personal", "name": "Bad zone", "filters": map[string]any{"timezone": "Mars/Base"}}); r.status != http.StatusBadRequest || r.body["field"] != "timezone" {
		t.Fatalf("bad timezone in a view: %d %s", r.status, r.raw)
	}
}

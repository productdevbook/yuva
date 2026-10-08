package api_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestClientRatings(t *testing.T) {
	h := privateHarness(t)
	ct := newChatTeam(t, h, "live", true)
	rcv := newReceiver(t)
	ct.owner.expect(http.StatusCreated, "POST", "/v1/webhooks", map[string]any{"url": rcv.srv.URL + "/hook", "events": []string{"conversation.rated", "message.created"}})
	other := newChatTeam(t, h, "live", true)
	cs := ct.session(h, map[string]any{})
	conv := cs.start("my order is late")
	rate := "/client/v1/conversations/" + conv + "/rating"
	since := h.clock.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)

	if cs.body["inbox"].(map[string]any)["ask_for_rating"] != false {
		t.Fatalf("session inbox %v", cs.body["inbox"])
	}
	post(ct.agent, conv, map[string]any{"kind": "message", "body": "on its way"})
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if r := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv, nil); r.body["can_rate"] != false {
		t.Fatalf("an inbox that does not ask: %s", r.raw)
	}
	cs.expectProblem(http.StatusConflict, "rating_unavailable", "POST", rate, map[string]any{"rating": "good"})

	if r := ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"ask_for_rating": true}); r.body["ask_for_rating"] != true {
		t.Fatalf("inbox %s", r.raw)
	}
	if r := ct.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+ct.chatInbox, map[string]any{"name": "Chat!"}); r.body["ask_for_rating"] != true {
		t.Fatalf("another change turned ratings off: %s", r.raw)
	}
	if r := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil); r.body["inbox"].(map[string]any)["ask_for_rating"] != true {
		t.Fatalf("session %s", r.raw)
	}
	if r := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv, nil); r.body["can_rate"] != true || r.body["rating"] != nil {
		t.Fatalf("closed conversation %s", r.raw)
	}
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", rate, map[string]any{"rating": "meh"})
	other.session(h, map[string]any{}).expectProblem(http.StatusNotFound, "not_found", "POST", rate, map[string]any{"rating": "bad"})
	stranger := ct.session(h, map[string]any{})
	stranger.expectProblem(http.StatusNotFound, "not_found", "POST", rate, map[string]any{"rating": "bad"})

	r := cs.expect(http.StatusCreated, "POST", rate, map[string]any{"rating": "good", "comment": "  quick and kind  "})
	if r.body["can_rate"] != false || r.body["rating"] != "good" {
		t.Fatalf("rated %s", r.raw)
	}
	cs.expectProblem(http.StatusConflict, "already_rated", "POST", rate, map[string]any{"rating": "bad"})
	rcv.take()
	h.runWebhooks(ct.ws)
	var rated []receivedHook
	for _, got := range rcv.take() {
		if got.typ == "conversation.rated" {
			rated = append(rated, got)
		}
		if got.typ == "message.created" && got.data["message"].(map[string]any)["kind"] == "event" {
			t.Fatalf("an event message became message.created: %s", got.body)
		}
	}
	if len(rated) != 1 || rated[0].data["conversation"].(map[string]any)["rating"].(map[string]any)["rating"] != "good" || rated[0].data["contact"] == nil {
		t.Fatalf("conversation.rated hooks %v", rated)
	}

	c := ct.agent.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	rating := c.body["rating"].(map[string]any)
	if rating["rating"] != "good" || rating["comment"] != "quick and kind" || c.str("closed_at") == "" {
		t.Fatalf("panel conversation %s", c.raw)
	}
	thread := messages(ct.agent, conv)
	last := thread[len(thread)-1]
	if ev := last["event"].(map[string]any); ev["type"] != "rated" || ev["rating"] != "good" || last["body"] != "quick and kind" ||
		last["author"].(map[string]any)["type"] != "contact" {
		t.Fatalf("rated event %v", last)
	}
	for _, m := range cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv+"/messages", nil).body["items"].([]any) {
		if m.(map[string]any)["body"] == "quick and kind" {
			t.Fatalf("the rating event reached the contact's thread: %v", m)
		}
	}
	st := ct.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+url.QueryEscape(since), nil).body["ratings"].(map[string]any)
	if st["good"] != float64(1) || st["bad"] != float64(0) || st["inboxes"].([]any)[0].(map[string]any)["inbox_id"] != ct.chatInbox {
		t.Fatalf("rating stats %v", st)
	}
	if st := other.owner.expect(http.StatusOK, "GET", "/v1/stats?since="+url.QueryEscape(since), nil).body["ratings"].(map[string]any); st["good"] != float64(0) {
		t.Fatalf("another workspace's rating stats %v", st)
	}

	cs.expect(http.StatusCreated, "POST", "/client/v1/conversations/"+conv+"/messages", map[string]any{"body": "one more thing"})
	if r := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv, nil); r.body["can_rate"] != false || r.body["rating"] != nil {
		t.Fatalf("reopened %s", r.raw)
	}
	h.clock.Advance(time.Second)
	ct.agent.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if r := cs.expect(http.StatusOK, "GET", "/client/v1/conversations/"+conv, nil); r.body["can_rate"] != true {
		t.Fatalf("closed again %s", r.raw)
	}
	if _, err := h.st.Pool.Exec(context.Background(), "UPDATE conversations SET closed_at = closed_at - interval '31 days', rated_at = rated_at - interval '32 days' WHERE id = $1", conv); err != nil {
		t.Fatal(err)
	}
	cs.expectProblem(http.StatusConflict, "rating_unavailable", "POST", rate, map[string]any{"rating": "bad"})
}

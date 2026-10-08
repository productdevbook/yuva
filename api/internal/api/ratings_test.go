package api_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	"uuid"
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

func ratingPage(t *testing.T, method, u string, form url.Values) (int, string) {
	t.Helper()
	var res *http.Response
	var err error
	if method == "POST" {
		res, err = http.PostForm(u, form)
	} else {
		res, err = http.Get(u)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("no CSP on %s", u)
	}
	return res.StatusCode, string(b)
}

func TestEmailRatingRequest(t *testing.T) {
	h := newHarness(t)
	et := newEmailTeam(t, h, false)
	ctx := context.Background()
	et.owner.expect(http.StatusOK, "PATCH", "/v1/inboxes/"+et.inbox, map[string]any{
		"ask_for_rating": true, "default_locale": "de", "branding": map[string]any{"color": "#aa3300"},
	})
	open := func(subject string) string {
		return h.ingest(et.address, buildMail(mailOpts{from: "rater@example.net", to: et.address, subject: subject, messageID: newMessageID(), body: "help"}), nil).str("conversation_id")
	}
	requests := func(conv string) []map[string]any {
		var out []map[string]any
		for _, m := range messages(et.owner, conv) {
			if m["author"].(map[string]any)["type"] == "system" && m["kind"] == "message" {
				out = append(out, m)
			}
		}
		return out
	}

	unanswered := open("No answer")
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+unanswered, map[string]any{"status": "closed"})
	if err := h.srv.RequestRating(ctx, uuid.MustParse(et.ws), uuid.MustParse(unanswered)); err != nil {
		t.Fatal(err)
	}
	if n := len(requests(unanswered)); n != 0 {
		t.Fatalf("asked to rate an unanswered conversation: %d", n)
	}

	conv := open("Invoice")
	reply := post(et.owner, conv, map[string]any{"kind": "message", "body": "Here it is."})
	h.sendQueued(t, et.ws, reply)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if n := len(h.jobs("rating_request", et.ws)); n != 2 {
		t.Fatalf("rating request jobs %d", n)
	}
	for range 2 {
		if err := h.srv.RequestRating(ctx, uuid.MustParse(et.ws), uuid.MustParse(conv)); err != nil {
			t.Fatal(err)
		}
	}
	reqs := requests(conv)
	if len(reqs) != 1 {
		t.Fatalf("rating requests %v", reqs)
	}
	body := reqs[0]["body"].(string)
	if !strings.Contains(body, "Wie waren wir?") && !strings.Contains(body, "Ein Klick genügt") {
		t.Fatalf("request text %q", body)
	}
	h.sendQueued(t, et.ws, reqs[0]["id"].(string))
	_, parsed := h.smtp.last(t)
	if parsed.Header.Get("Auto-Submitted") == "" || parsed.Header.Get("In-Reply-To") == "" {
		t.Fatalf("request headers %v", parsed.Header)
	}
	start := strings.Index(body, testOrigin+"/r/")
	if start < 0 {
		t.Fatalf("no link in %q", body)
	}
	link := strings.Fields(body[start:])[0]
	page := strings.TrimSuffix(strings.TrimPrefix(link, testOrigin), "?rating=good")
	page = strings.TrimSuffix(page, "?rating=bad")
	target := h.url + page

	status, html := ratingPage(t, "GET", target+"?rating=bad", nil)
	if status != http.StatusOK || !strings.Contains(html, `lang="de"`) || !strings.Contains(html, "#aa3300") ||
		!strings.Contains(html, `value="bad" checked`) || !strings.Contains(html, "<form") {
		t.Fatalf("form page %d %s", status, html)
	}
	if c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil); c.body["rating"] != nil {
		t.Fatalf("opening the link rated: %s", c.raw)
	}
	token := []byte(strings.TrimPrefix(page, "/r/"))
	if token[20] == 'A' {
		token[20] = 'B'
	} else {
		token[20] = 'A'
	}
	if status, _ := ratingPage(t, "GET", h.url+"/r/"+string(token), nil); status != http.StatusNotFound {
		t.Fatalf("tampered link %d", status)
	}
	if status, _ := ratingPage(t, "POST", target, url.Values{"rating": {"meh"}}); status != http.StatusBadRequest {
		t.Fatalf("bad rating %d", status)
	}
	status, html = ratingPage(t, "POST", target, url.Values{"rating": {"bad"}, "comment": {"slow answer"}})
	if status != http.StatusOK || !strings.Contains(html, "Vielen Dank") {
		t.Fatalf("rated %d %s", status, html)
	}
	c := et.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+conv, nil)
	if r := c.body["rating"].(map[string]any); r["rating"] != "bad" || r["comment"] != "slow answer" {
		t.Fatalf("rating %s", c.raw)
	}
	if status, html = ratingPage(t, "POST", target, url.Values{"rating": {"good"}}); status != http.StatusOK || !strings.Contains(html, "bereits bewertet") {
		t.Fatalf("second rating %d %s", status, html)
	}

	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "open"})
	h.clock.Advance(time.Second)
	et.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	if status, _ := ratingPage(t, "GET", target, nil); status != http.StatusNotFound {
		t.Fatalf("a link of an earlier close %d", status)
	}
}

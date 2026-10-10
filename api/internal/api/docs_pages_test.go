package api_test

import (
	"net/http"
	"net/url"
	"testing"
)

func TestDocumentationPages(t *testing.T) {
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	other := newChatTeam(t, h, "live", true)
	page := ct.origin + "/guide/install"
	ratings := "/client/v1/channels/" + ct.key + "/page-ratings"
	answers := "/client/v1/channels/" + ct.key + "/page-answers?page=" + url.QueryEscape(page+"?utm=x#top")

	site := h.client()
	site.origin = ct.origin
	site.expect(http.StatusNoContent, "POST", ratings, map[string]any{"page": page + "?ref=nav#step-2", "title": "Install", "rating": "up"})
	site.expect(http.StatusNoContent, "POST", ratings, map[string]any{"page": page, "rating": "down"})
	site.expect(http.StatusNoContent, "POST", ratings, map[string]any{"page": page, "rating": "down", "previous": "up"})
	site.expectProblem(http.StatusBadRequest, "page_origin", "POST", ratings, map[string]any{"page": other.origin + "/guide", "rating": "up"})
	site.expectProblem(http.StatusBadRequest, "validation_failed", "POST", ratings, map[string]any{"page": "/guide", "rating": "up"})
	site.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/channels/nope/page-ratings", map[string]any{"page": page, "rating": "up"})
	stranger := h.client()
	stranger.origin = other.origin
	stranger.expectProblem(http.StatusForbidden, "origin_not_allowed", "POST", ratings, map[string]any{"page": page, "rating": "up"})
	stranger.expectProblem(http.StatusForbidden, "origin_not_allowed", "GET", answers, nil)
	site.expectProblem(http.StatusBadRequest, "page_origin", "GET", "/client/v1/channels/"+ct.key+"/page-answers?page="+url.QueryEscape(other.origin+"/guide"), nil)

	cs := ct.session(h, map[string]any{})
	fb := cs.expect(http.StatusCreated, "POST", "/client/v1/feedback", map[string]any{
		"body": "step 2 fails", "page_url": page + "#step-2", "page_title": "Install", "rating": "down",
		"allow_email": true, "email": "success@simulator.amazonses.com", "client_id": unique("c"),
	}).body["conversation"].(map[string]any)
	if f := fb["feedback"].(map[string]any); fb["kind"] != "feedback" || f["page_url"] != page || f["rating"] != "down" || f["category"] != "other" {
		t.Fatalf("page feedback %v", fb)
	}
	cs.expectProblem(http.StatusBadRequest, "page_origin", "POST", "/client/v1/feedback", map[string]any{"body": "x", "page_url": other.origin + "/guide"})

	cs.expectProblem(http.StatusBadRequest, "page_origin", "POST", "/client/v1/questions", map[string]any{"body": "how?", "page_url": other.origin + "/guide"})
	qc := cs.expect(http.StatusCreated, "POST", "/client/v1/questions", map[string]any{
		"body": "Does it run on ARM?", "page_url": page + "?a=1", "page_title": "Install", "client_id": unique("c"),
	}).body["conversation"].(map[string]any)
	if q := qc["question"].(map[string]any); qc["kind"] != "question" || q["page_url"] != page {
		t.Fatalf("question %v", qc)
	}
	conv := qc["id"].(string)
	publish := "/v1/conversations/" + conv + "/publish"
	edited := map[string]any{"question": "Does it run on ARM?", "answer": "Yes, arm64 builds are published."}

	ct.agent.expectProblem(http.StatusConflict, "not_publishable", "POST", publish, edited)
	ct.agent.expectProblem(http.StatusConflict, "not_publishable", "POST", "/v1/conversations/"+fb["id"].(string)+"/publish", edited)
	post(ct.agent, conv, map[string]any{"kind": "message", "body": "Yes, there are arm64 builds."})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", publish, edited)
	a := ct.agent.expect(http.StatusCreated, "POST", publish, edited)
	if a.str("page") != page || a.str("title") != "Install" || a.str("conversation_id") != conv {
		t.Fatalf("published %s", a.raw)
	}
	ct.agent.expectProblem(http.StatusConflict, "already_published", "POST", publish, edited)
	answer := "/v1/page-answers/" + a.str("id")

	r := site.expect(http.StatusOK, "GET", answers, nil)
	items := r.body["items"].([]any)
	if len(items) != 1 || r.header.Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("client answers %s %v", r.raw, r.header)
	}
	for k := range items[0].(map[string]any) {
		if k != "id" && k != "question" && k != "answer" && k != "published_at" && k != "updated_at" {
			t.Fatalf("client answer exposes %s: %s", k, r.raw)
		}
	}

	pages := ct.owner.expect(http.StatusOK, "GET", "/v1/docs/pages?days=7", nil).body["items"].([]any)
	if len(pages) != 1 {
		t.Fatalf("docs pages %v", pages)
	}
	if p := pages[0].(map[string]any); p["page"] != page || p["title"] != "Install" || p["up"] != 0.0 || p["down"] != 2.0 ||
		p["open_feedback"] != 1.0 || p["open_questions"] != 1.0 || p["published_answers"] != 1.0 {
		t.Fatalf("docs page %v", p)
	}
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/docs/pages?days=8", nil)
	if got := ct.agent.expect(http.StatusOK, "GET", "/v1/conversations?kind=question&page="+url.QueryEscape(page+"#x"), nil).body["items"].([]any); len(got) != 1 {
		t.Fatalf("questions of the page %v", got)
	}
	if got := other.owner.expect(http.StatusOK, "GET", "/v1/docs/pages", nil).body["items"].([]any); len(got) != 0 {
		t.Fatalf("another workspace sees %v", got)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", answer, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", answer, map[string]any{"answer": "no"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", answer, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/docs/page?inbox_id="+ct.chatInbox+"&page="+url.QueryEscape(page), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/docs/summary?inbox_id="+ct.chatInbox, nil)
	sum := ct.agent.expect(http.StatusOK, "GET", "/v1/docs/summary?days=7", nil).body
	if tot := sum["totals"].(map[string]any); len(sum["days"].([]any)) != 7 || tot["down"] != 2.0 || tot["feedback"] != 1.0 || tot["questions"] != 1.0 || tot["published_answers"] != 1.0 || tot["rated_pages"] != 1.0 {
		t.Fatalf("summary %v", sum)
	}
	if tot := other.owner.expect(http.StatusOK, "GET", "/v1/docs/summary", nil).body["totals"].(map[string]any); tot["down"] != 0.0 || tot["feedback"] != 0.0 {
		t.Fatalf("another workspace's summary %v", tot)
	}

	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/inboxes/"+ct.chatInbox+"/members/"+ct.agentID, nil)
	ct.agent.expectProblem(http.StatusNotFound, "not_found", "GET", answer, nil)
	ct.agent.expectProblem(http.StatusNotFound, "not_found", "PATCH", answer, map[string]any{"answer": "no"})
	ct.agent.expectProblem(http.StatusNotFound, "not_found", "DELETE", answer, nil)
	if got := ct.agent.expect(http.StatusOK, "GET", "/v1/docs/pages", nil).body["items"].([]any); len(got) != 0 {
		t.Fatalf("an agent without the inbox sees %v", got)
	}
	if tot := ct.agent.expect(http.StatusOK, "GET", "/v1/docs/summary", nil).body["totals"].(map[string]any); tot["down"] != 0.0 || tot["questions"] != 0.0 {
		t.Fatalf("an agent without the inbox counts %v", tot)
	}
	if got := ct.agent.expect(http.StatusOK, "GET", "/v1/page-answers", nil).body["items"].([]any); len(got) != 0 {
		t.Fatalf("an agent without the inbox lists %v", got)
	}

	ct.owner.expect(http.StatusNoContent, "DELETE", "/v1/contacts/"+cs.contactID, nil)
	kept := ct.owner.expect(http.StatusOK, "PATCH", answer, map[string]any{"answer": "Yes: arm64 and amd64."})
	if kept.str("answer") != "Yes: arm64 and amd64." || kept.body["conversation_id"] != nil {
		t.Fatalf("answer after the contact was deleted %s", kept.raw)
	}
	ct.owner.expect(http.StatusNoContent, "DELETE", answer, nil)
	if got := site.expect(http.StatusOK, "GET", answers, nil).body["items"].([]any); len(got) != 0 {
		t.Fatalf("unpublished answer still listed %v", got)
	}
}

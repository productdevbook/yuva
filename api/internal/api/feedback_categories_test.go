package api_test

import (
	"net/http"
	"testing"
)

func TestInboxFeedbackCategories(t *testing.T) {
	h := newHarness(t)
	at := newAppTeam(t, h, false)
	other := newAppTeam(t, h, false)
	path := "/v1/inboxes/" + at.appInbox

	cats := at.owner.expect(http.StatusOK, "GET", path, nil).body["feedback_categories"].([]any)
	if len(cats) != 4 || cats[0].(map[string]any)["key"] != "bug" || cats[3].(map[string]any)["key"] != "other" {
		t.Fatalf("default categories %v", cats)
	}

	cs := at.session(h, "user-1")
	cs.language = "tr-TR,tr;q=0.9"
	in := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil).body["inbox"].(map[string]any)
	items := in["feedback_category_items"].([]any)
	if len(items) != 4 || items[0].(map[string]any)["name"] != "Hata" || items[0].(map[string]any)["icon"] != "bug" || len(in["feedback_categories"].([]any)) != 4 {
		t.Fatalf("client categories %v", in)
	}

	ux := map[string]any{"category": "ux", "body": "x"}
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/feedback", ux)

	custom := []any{
		map[string]any{"key": "bug", "names": map[string]any{"en": "Bug"}, "icon": "bug"},
		map[string]any{"key": "ux", "names": map[string]any{"en": "Design", "tr": "Tasarım"}, "icon": "palette"},
		map[string]any{"key": "other", "names": map[string]any{"en": "Other"}, "icon": "message-circle"},
	}
	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"feedback_categories": custom})
	uxID := cs.expect(http.StatusCreated, "POST", "/client/v1/feedback", ux).body["conversation"].(map[string]any)["id"].(string)
	cs.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/client/v1/feedback", map[string]any{"category": "idea", "body": "x"})
	key := at.apiKey(h)
	key.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/feedback", map[string]any{
		"inbox_id": at.appInbox, "category": "praise", "body": "x", "contact": map[string]any{"external_id": "a"},
	})
	key.expect(http.StatusCreated, "POST", "/v1/feedback", map[string]any{
		"inbox_id": at.appInbox, "category": "ux", "body": "x", "contact": map[string]any{"external_id": "a"},
	})

	counts := at.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts?kind=feedback", nil).body["feedback_categories"].([]any)
	if len(counts) != 1 || counts[0].(map[string]any)["category"] != "ux" || counts[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("counts %v", counts)
	}
	if n := len(at.owner.expect(http.StatusOK, "GET", "/v1/conversations?category=ux", nil).body["items"].([]any)); n != 2 {
		t.Fatalf("filter by custom key found %d", n)
	}

	for _, it := range cs.expect(http.StatusOK, "GET", "/client/v1/conversations?kind=feedback", nil).body["items"].([]any) {
		fb := it.(map[string]any)["feedback"].(map[string]any)
		if fb["category"] != "other" || fb["category_key"] != "ux" || fb["category_name"] != "Tasarım" {
			t.Fatalf("client feedback %v", fb)
		}
	}
	legacy := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil).body["inbox"].(map[string]any)
	if keys := legacy["feedback_categories"].([]any); len(keys) != 2 || keys[0] != "bug" || keys[1] != "other" || len(legacy["feedback_category_items"].([]any)) != 3 {
		t.Fatalf("legacy keys %v", legacy)
	}
	at.owner.expect(http.StatusOK, "PATCH", path, map[string]any{"feedback_categories": []any{custom[0], custom[2]}})
	if fb := at.owner.expect(http.StatusOK, "GET", "/v1/conversations/"+uxID, nil).body["feedback"].(map[string]any); fb["category"] != "other" {
		t.Fatalf("removed category kept on feedback %v", fb)
	}
	counts = at.owner.expect(http.StatusOK, "GET", "/v1/conversations/counts?kind=feedback", nil).body["feedback_categories"].([]any)
	if len(counts) != 1 || counts[0].(map[string]any)["category"] != "other" || counts[0].(map[string]any)["count"] != float64(2) {
		t.Fatalf("counts after removal %v", counts)
	}

	for name, list := range map[string][]any{
		"no other":  {custom[0]},
		"duplicate": {custom[0], custom[0], custom[2]},
		"bad key":   {map[string]any{"key": "Bad Key", "names": map[string]any{"en": "x"}, "icon": "bug"}, custom[2]},
		"no name":   {map[string]any{"key": "ux", "names": map[string]any{}, "icon": "bug"}, custom[2]},
		"bad icon":  {map[string]any{"key": "ux", "names": map[string]any{"en": "x"}, "icon": "Not An Icon"}, custom[2]},
		"empty":     {},
	} {
		t.Run(name, func(t *testing.T) {
			at.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", path, map[string]any{"feedback_categories": list})
		})
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", path, map[string]any{"feedback_categories": custom})
	if got := other.owner.expect(http.StatusOK, "GET", "/v1/inboxes/"+other.appInbox, nil).body["feedback_categories"].([]any); len(got) != 4 {
		t.Fatalf("other workspace changed: %v", got)
	}
}

func TestAcceptLanguageWeights(t *testing.T) {
	h := newHarness(t)
	at := newAppTeam(t, h, false)
	cs := at.session(h, "user-1")
	cs.language = "de;q=0.4, tr;q=0.9, en;q=0.1"
	items := cs.expect(http.StatusOK, "GET", "/client/v1/session", nil).body["inbox"].(map[string]any)["feedback_category_items"].([]any)
	if items[0].(map[string]any)["name"] != "Hata" {
		t.Fatalf("items %v", items)
	}
}

package api_test

import (
	"context"
	"image/color"
	"net/http"
	"testing"
	"uuid"
)

func bulkResult(t *testing.T, r response, succeeded int, failed map[string]string) {
	t.Helper()
	if got := len(r.body["succeeded"].([]any)); got != succeeded {
		t.Fatalf("succeeded %d, want %d: %s", got, succeeded, r.raw)
	}
	fs := r.body["failed"].([]any)
	if len(fs) != len(failed) {
		t.Fatalf("failed %d, want %d: %s", len(fs), len(failed), r.raw)
	}
	for _, f := range fs {
		m := f.(map[string]any)
		if failed[m["id"].(string)] != m["code"] {
			t.Fatalf("failed %v, want %v: %s", m, failed, r.raw)
		}
	}
}

func manyIDs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = uuid.New().String()
	}
	return out
}

func TestBulkHelpArticles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ht := newHelpTeam(t, h)
	other := newHelpTeam(t, h)
	foreign, _ := other.article(other.owner, "en", "Theirs", "x")
	inbox := ht.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Docs", "slug": unique("docs"), "mode": "live", "expected_reply_minutes": 30}).body["inbox"].(map[string]any)["id"].(string)
	second := ht.owner.expect(http.StatusCreated, "POST", "/v1/help-centers", map[string]any{"inbox_id": inbox, "name": "Docs", "slug": unique("docs"), "default_locale": "en", "locales": []string{"en"}}).str("id")
	elsewhere := ht.owner.expect(http.StatusCreated, "POST", "/v1/help-centers/"+second+"/articles", map[string]any{"locale": "en", "title": "Elsewhere", "body": "x"}).str("id")

	var ids []string
	for range 20 {
		id, _ := ht.article(ht.owner, "en", "Article "+unique("a"), "Body")
		ids = append(ids, id)
	}
	bulk := ht.path("/articles/bulk")
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": append(ids, ids[0]), "action": "publish"}), 20, nil)
	if n := len(ht.events("help.article.published")); n != 20 {
		t.Fatalf("published events: %d", n)
	}
	state := func(id string) string {
		return ht.owner.expect(http.StatusOK, "GET", ht.path("/articles/"+id), nil).body["translations"].([]any)[0].(map[string]any)["state"].(string)
	}
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": append([]string{foreign, elsewhere}, ids...), "action": "unpublish", "locale": "en"}),
		20, map[string]string{foreign: "not_found", elsewhere: "not_found"})
	if n := len(ht.events("help.article.unpublished")); n != 20 || state(ids[7]) != "draft" {
		t.Fatalf("unpublished events: %d, state %s", n, state(ids[7]))
	}
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids, "action": "publish"}), 20, nil)
	if state(ids[19]) != "published" {
		t.Fatalf("undo left %s", state(ids[19]))
	}
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids[:1], "action": "publish", "locale": "tr"}), 0, map[string]string{ids[0]: "not_found"})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", bulk, map[string]any{"ids": ids[:1], "action": "publish"})

	col := ht.owner.expect(http.StatusCreated, "POST", ht.path("/collections"), map[string]any{"translations": map[string]any{"en": map[string]any{"name": "Billing"}}}).str("id")
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids[:2], "action": "move", "collection_id": col}), 2, nil)
	if got := ht.owner.expect(http.StatusOK, "GET", ht.path("/articles/"+ids[1]), nil).body["collection_id"]; got != col {
		t.Fatalf("moved into %v", got)
	}
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids[:2], "action": "move", "collection_id": nil}), 2, nil)
	if got := ht.owner.expect(http.StatusOK, "GET", ht.path("/articles/"+ids[1]), nil).body["collection_id"]; got != nil {
		t.Fatalf("taken out to %v", got)
	}

	for _, body := range []map[string]any{
		{"ids": manyIDs(101), "action": "publish"},
		{"ids": []string{}, "action": "publish"},
		{"ids": ids[:1], "action": "archive"},
		{"ids": ids[:1], "action": "move"},
		{"ids": ids[:1], "action": "move", "collection_id": uuid.New().String()},
		{"ids": ids[:1], "action": "publish", "collection_id": col},
		{"ids": ids[:1], "action": "delete", "locale": "en"},
	} {
		ht.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", bulk, body)
	}
	ht.agent.expectProblem(http.StatusNotFound, "not_found", "POST", bulk, map[string]any{"ids": ids[:1], "action": "unpublish"})
	ht.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+ht.inbox+"/members/"+ht.agentID, nil)
	ht.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", bulk, map[string]any{"ids": ids[:1], "action": "unpublish"})
	writer := ht.key("writer", map[string]any{"scopes": []string{"help:read", "help:write"}})
	writer.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", bulk, map[string]any{"ids": ids[:1], "action": "unpublish"})
	if state(ids[0]) != "published" {
		t.Fatal("a refused request changed an article")
	}
	bulkResult(t, writer.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids[:1], "action": "delete"}), 0, map[string]string{ids[0]: "insufficient_scope"})

	draft, _ := ht.article(ht.owner, "en", "Draft", "x")
	bulkResult(t, ht.agent.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{draft, ids[0]}, "action": "delete"}), 1, map[string]string{ids[0]: "forbidden"})
	bulkResult(t, ht.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": ids[:3], "action": "delete"}), 3, nil)
	if n := len(ht.events("help.article.deleted")); n != 4 {
		t.Fatalf("deleted events: %d", n)
	}
	ht.owner.expectProblem(http.StatusNotFound, "not_found", "GET", ht.path("/articles/"+ids[2]), nil)
}

func TestBulkChangelogEntries(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChangelogTeam(t, h)
	other := newChangelogTeam(t, h)
	foreign := other.entry(other.owner, "en", "Theirs", "x")
	a, b := ct.entry(ct.owner, "en", "One", "x"), ct.entry(ct.owner, "en", "Two", "x")
	turkish := ct.entry(ct.owner, "tr", "Yeni", "x")
	bulk := ct.path("/entries/bulk")

	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", bulk, map[string]any{"ids": []string{a}, "action": "publish"})
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", bulk, map[string]any{"ids": manyIDs(101), "action": "delete"})
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", bulk, map[string]any{"ids": []string{a}, "action": "unpublish"})
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, b, turkish, foreign}, "action": "publish"}),
		2, map[string]string{turkish: "no_default_translation", foreign: "not_found"})
	if n := len(ct.events("changelog.entry.published")); n != 2 || ct.get(ct.owner, b).str("state") != "published" {
		t.Fatalf("published events: %d", n)
	}
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, b}, "action": "publish"}), 2, nil)
	if n := len(ct.events("changelog.entry.published")); n != 2 {
		t.Fatalf("published again: %d events", n)
	}
	bulkResult(t, ct.agent.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, turkish}, "action": "delete"}), 1, map[string]string{a: "forbidden"})
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, b}, "action": "delete"}), 2, nil)
	if n := len(ct.events("changelog.entry.deleted")); n != 2 {
		t.Fatalf("deleted events: %d", n)
	}
	other.get(other.owner, foreign)
}

func TestBulkSubscriptions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	lt := newListTeam(t, h)
	other := newListTeam(t, h)
	sub := func(c *client, list, email string) string {
		return c.expect(http.StatusCreated, "POST", "/v1/lists/"+list+"/subscriptions", map[string]any{"email": email, "consent": map[string]any{"confirmed": true}}).str("id")
	}
	a, b := sub(lt.owner, lt.list, unique("a")+"@example.com"), sub(lt.owner, lt.list, unique("b")+"@example.com")
	foreign := sub(other.owner, other.list, unique("c")+"@example.com")
	second := lt.owner.expect(http.StatusCreated, "POST", "/v1/lists", map[string]any{"inbox_id": lt.chatInbox, "name": "Second"}).str("id")
	elsewhere := sub(lt.owner, second, unique("d")+"@example.com")
	bulk := "/v1/lists/" + lt.list + "/subscriptions/bulk"

	lt.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", bulk, map[string]any{"ids": []string{a}, "action": "unsubscribe"})
	lt.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", bulk, map[string]any{"ids": []string{a}, "action": "resubscribe"})
	bulkResult(t, lt.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, foreign, elsewhere}, "action": "unsubscribe"}),
		1, map[string]string{foreign: "not_found", elsewhere: "not_found"})
	if got := lt.owner.expect(http.StatusOK, "GET", "/v1/lists/"+lt.list+"/subscriptions/"+a, nil); got.str("state") != "unsubscribed" || got.str("unsubscribe_reason") != "member" {
		t.Fatalf("unsubscribed: %s", got.raw)
	}
	bulkResult(t, lt.owner.expect(http.StatusOK, "POST", bulk, map[string]any{"ids": []string{a, b}, "action": "delete"}), 2, nil)
	lt.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/lists/"+lt.list+"/subscriptions/"+b, nil)

	owner, ownerID := lt.newAgent()
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+ownerID, map[string]any{"role": "admin"})
	box := owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Mine", "slug": unique("mine"), "owner_member_id": ownerID}).body["inbox"].(map[string]any)["id"].(string)
	mine := owner.expect(http.StatusCreated, "POST", "/v1/lists", map[string]any{"inbox_id": box, "name": "Mine"}).str("id")
	minesub := sub(owner, mine, unique("e")+"@example.com")
	lt.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/lists/"+mine+"/subscriptions/bulk", map[string]any{"ids": []string{minesub}, "action": "delete"})
	admin, adminID := lt.newAgent()
	lt.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+adminID, map[string]any{"role": "admin"})
	admin.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/lists/"+mine+"/subscriptions/bulk", map[string]any{"ids": []string{minesub}, "action": "delete"})
	bulkResult(t, owner.expect(http.StatusOK, "POST", "/v1/lists/"+mine+"/subscriptions/bulk", map[string]any{"ids": []string{minesub}, "action": "unsubscribe"}), 1, nil)
}

func TestBulkDesignsImagesBroadcasts(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	owner, ownerID := tm.newAgent()
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+ownerID, map[string]any{"role": "admin"})
	box := owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Mine", "slug": unique("mine"), "owner_member_id": ownerID}).body["inbox"].(map[string]any)["id"].(string)

	design := tm.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Ours"}).str("id")
	mine := owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Mine", "inbox_id": box}).str("id")
	theirs := other.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "Theirs"}).str("id")
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/email-designs/bulk", map[string]any{"ids": []string{design}, "action": "delete"})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/email-designs/bulk", map[string]any{"ids": manyIDs(101), "action": "delete"})
	admin, adminID := tm.newAgent()
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/members/"+adminID, map[string]any{"role": "admin"})
	bulkResult(t, admin.expect(http.StatusOK, "POST", "/v1/email-designs/bulk", map[string]any{"ids": []string{design, mine, theirs}, "action": "delete"}),
		1, map[string]string{mine: "not_found", theirs: "not_found"})
	if got := tm.owner.expect(http.StatusOK, "GET", "/v1/email-designs/"+design, nil); got.body["archived_at"] == nil {
		t.Fatalf("archived: %s", got.raw)
	}
	bulkResult(t, owner.expect(http.StatusOK, "POST", "/v1/email-designs/bulk", map[string]any{"ids": []string{mine}, "action": "delete"}), 1, nil)

	img := tm.owner.uploadImage("a.png", "image/png", testPNG(t, 40, 40, color.RGBA{1, 2, 3, 255})).str("id")
	theirImg := other.owner.uploadImage("b.png", "image/png", testPNG(t, 40, 40, color.RGBA{3, 2, 1, 255})).str("id")
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/email-images/bulk", map[string]any{"ids": []string{img}, "action": "purge"})
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/email-images/bulk", map[string]any{"ids": []string{img}, "action": "delete"})
	used := tm.owner.uploadImage("c.png", "image/png", testPNG(t, 40, 40, color.RGBA{2, 2, 2, 255})).str("id")
	user := tm.owner.expect(http.StatusCreated, "POST", "/v1/email-designs", map[string]any{"name": "With image", "document": designDoc(map[string]any{"id": "hero", "type": "image", "image_id": used, "alt": "Banner"})}).str("id")
	r := tm.owner.expect(http.StatusOK, "POST", "/v1/email-images/bulk", map[string]any{"ids": []string{img, theirImg, used}, "action": "delete"})
	bulkResult(t, r, 1, map[string]string{theirImg: "not_found", used: "image_in_use"})
	inUse := r.body["failed"].([]any)[1].(map[string]any)
	designs, _ := inUse["used_by"].(map[string]any)["designs"].([]any)
	if inUse["status"] != float64(http.StatusConflict) || len(designs) != 1 || designs[0].(map[string]any)["id"] != user {
		t.Fatalf("used_by: %s", r.raw)
	}
	if r := tm.owner.expect(http.StatusOK, "GET", "/v1/email-images", nil); len(r.body["items"].([]any)) != 1 {
		t.Fatalf("images left: %s", r.raw)
	}

	draft := tm.owner.expect(http.StatusCreated, "POST", "/v1/broadcasts", map[string]any{"inbox_id": tm.inbox, "name": "Draft"}).str("id")
	active := tm.owner.expect(http.StatusCreated, "POST", "/v1/broadcasts", map[string]any{"inbox_id": tm.inbox, "name": "Scheduled"}).str("id")
	if _, err := h.st.Pool.Exec(context.Background(), `UPDATE broadcasts SET state = 'scheduled' WHERE id = $1`, active); err != nil {
		t.Fatal(err)
	}
	personal := owner.expect(http.StatusCreated, "POST", "/v1/broadcasts", map[string]any{"inbox_id": box, "name": "Mine"}).str("id")
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/broadcasts/bulk", map[string]any{"ids": []string{draft}, "action": "delete"})
	bulkResult(t, tm.owner.expect(http.StatusOK, "POST", "/v1/broadcasts/bulk", map[string]any{"ids": []string{draft, active, personal}, "action": "delete"}),
		1, map[string]string{active: "broadcast_active", personal: "not_found"})
	tm.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/broadcasts/"+draft, nil)
	owner.expect(http.StatusOK, "GET", "/v1/broadcasts/"+personal, nil)
}

func TestBulkDiscussions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChatTeam(t, h, "live", true)
	other := newChatTeam(t, h, "live", true)
	ada := newReader(h, ct, "Ada")
	thread := func(title string) string {
		return ada.site.expect(http.StatusCreated, "POST", "/client/v1/discussions", map[string]any{"page_url": ct.origin + "/guide", "title": title, "body": "x"}).str("id")
	}
	a, b := thread("First question"), thread("Second question")
	ours := ct.owner.expect(http.StatusCreated, "POST", "/v1/discussions", map[string]any{"channel_id": ct.channel, "page_url": ct.origin + "/guide", "title": "Our thread", "body": "x"}).str("id")
	foreign := other.owner.expect(http.StatusCreated, "POST", "/v1/discussions", map[string]any{"channel_id": other.channel, "page_url": other.origin + "/guide", "title": "Theirs", "body": "x"}).str("id")
	state := func(id string) string {
		return ct.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+id, nil).body["discussion"].(map[string]any)["state"].(string)
	}

	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{a}, "action": "hide"})
	ct.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{a}, "action": "lock"})
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{a, foreign}, "action": "hide"}), 1, map[string]string{foreign: "not_found"})
	if state(a) != "hidden" {
		t.Fatalf("hidden: %s", state(a))
	}
	ct.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{a}, "action": "unhide"})
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{a, foreign}, "action": "unhide"}), 1, map[string]string{foreign: "not_found"})
	if state(a) != "published" {
		t.Fatalf("unhidden: %s", state(a))
	}
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{b, ours}, "action": "spam"}), 1, map[string]string{ours: "validation_failed"})
	if state(b) != "hidden" {
		t.Fatalf("spam: %s", state(b))
	}
	bulkResult(t, ct.owner.expect(http.StatusOK, "POST", "/v1/discussions/bulk", map[string]any{"ids": []string{ours, a}, "action": "delete"}), 1, map[string]string{a: "forbidden"})
	ct.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/discussions/"+ours, nil)
	other.owner.expect(http.StatusOK, "GET", "/v1/discussions/"+foreign, nil)
}

func TestBulkKeyScopes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ct := newChangelogTeam(t, h)
	readOnly := ct.apiKey("read-only", map[string]any{"scopes": []string{"lists:read", "broadcasts:read", "changelog:read"}})
	writer := ct.apiKey("writer", map[string]any{"scopes": []string{"changelog:read", "changelog:write"}})
	entry := ct.entry(ct.owner, "en", "One", "x")
	ids := []string{entry}
	for _, c := range []struct {
		key    *client
		path   string
		action string
	}{
		{readOnly, "/v1/lists/" + uuid.New().String() + "/subscriptions/bulk", "unsubscribe"},
		{readOnly, "/v1/email-designs/bulk", "delete"},
		{readOnly, "/v1/email-images/bulk", "delete"},
		{readOnly, "/v1/broadcasts/bulk", "delete"},
		{readOnly, ct.path("/entries/bulk"), "delete"},
		{writer, ct.path("/entries/bulk"), "publish"},
	} {
		c.key.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", c.path, map[string]any{"ids": ids, "action": c.action})
	}
	if ct.get(ct.owner, entry).str("state") != "draft" {
		t.Fatal("a refused request changed the entry")
	}
}

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/migrate/helpdesk/helpdesktest"
)

type importRun struct {
	*deskRun
	center string
}

func newImportRun(t *testing.T) importRun {
	t.Helper()
	d := newDeskRun(t)
	c := d.tm.owner.expect(http.StatusCreated, "POST", "/v1/help-centers", map[string]any{
		"inbox_id": d.tm.inbox, "name": "Acme help", "slug": unique("acme"), "default_locale": "en", "locales": []string{"en", "tr"},
	})
	return importRun{deskRun: d, center: c.str("id")}
}

func (r importRun) path(p string) string { return "/v1/help-centers/" + r.center + p }

func (r importRun) zendeskMigration(t *testing.T) string {
	t.Helper()
	address, channel := r.emailChannel(t, r.tm.inbox, "support")
	id := r.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "zendesk", "preset": "zendesk", "source_address": address, "channel_id": channel,
	}).str("id")
	q := r.signIn(t, id, map[string]any{"subdomain": helpdesktest.ZendeskSubdomain, "client_id": "zd-client", "client_secret": "zd-secret"})
	if got := r.callback(t, r.tm.owner, url.Values{"code": {helpdesktest.ZendeskCode}, "state": {q.Get("state")}}); got != "connected" {
		t.Fatalf("sign-in: %s", got)
	}
	return id
}

// run works the import's jobs as River would until it ends, and returns its state.
func (r importRun) run(t *testing.T, id string) map[string]any {
	t.Helper()
	for range 200 {
		jobs := r.h.jobs("help_import", r.tm.ws)
		if len(jobs) == 0 {
			break
		}
		var a api.HelpImportArgs
		if err := json.Unmarshal(jobs[0].args, &a); err != nil {
			t.Fatal(err)
		}
		wait, done, err := r.h.srv.RunHelpImport(context.Background(), a, 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			r.h.dropJob(jobs[0].id)
		} else if wait > 0 {
			r.h.clock.Advance(wait)
		}
	}
	return r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports/"+id), nil).body
}

func (r importRun) start(t *testing.T, body map[string]any) string {
	t.Helper()
	return r.tm.owner.expect(http.StatusAccepted, "POST", r.path("/imports"), body).str("id")
}

func (r importRun) body(t *testing.T, locale, title string) string {
	t.Helper()
	var s string
	err := r.h.st.Pool.QueryRow(context.Background(), `SELECT t.body FROM help_article_translations t WHERE t.workspace_id = $1 AND t.locale = $2 AND t.title = $3`,
		r.tm.ws, locale, title).Scan(&s)
	if err != nil {
		t.Fatalf("%s %s: %v", locale, title, err)
	}
	return s
}

func importCounts(imp map[string]any) map[string]float64 {
	out := map[string]float64{}
	for k, v := range imp["counts"].(map[string]any) {
		out[k] = v.(float64)
	}
	return out
}

func TestZendeskGuideImport(t *testing.T) {
	r := newImportRun(t)
	mig := r.zendeskMigration(t)
	ws := r.tm.ws

	r.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", r.path("/imports"), map[string]any{"source": "zendesk"})
	r.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": mig, "docs_api_key": "k"})
	r.tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", r.path("/imports"), map[string]any{"source": "helpscout"})
	r.tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": "00000000-0000-7000-8000-000000000001"})
	hs := r.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"source": "helpscout", "preset": "helpscout", "source_address": "a@acme.example.com"}).str("id")
	r.tm.owner.expectProblem(http.StatusUnprocessableEntity, "source_mismatch", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": hs})

	other := newTeam(t, r.h)
	otherMig := other.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{"source": "zendesk", "preset": "zendesk", "source_address": "a@acme.example.com"}).str("id")
	r.tm.owner.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": otherMig})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": otherMig})
	r.tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": mig})
	r.tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+r.tm.inbox+"/members/"+r.tm.agentID, nil)
	r.tm.agent.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": mig})
	hw := r.tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "hw", "scopes": []string{"help:write"}})
	wk := r.h.client()
	wk.bearer = hw.str("secret")
	wk.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": mig})
	wk.expectProblem(http.StatusNotFound, "not_found", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": "00000000-0000-7000-8000-000000000001"})
	r.tm.owner.expectProblem(http.StatusUnprocessableEntity, "source_not_connected", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": r.unconnected(t)})

	id := r.start(t, map[string]any{"source": "zendesk", "migration_id": mig})
	r.tm.owner.expectProblem(http.StatusConflict, "import_running", "POST", r.path("/imports"), map[string]any{"source": "zendesk", "migration_id": mig})
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", r.path("/imports/"+id), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", r.path("/imports"), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "DELETE", r.path("/imports/"+id), nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", r.path("/imports/"+id+"/redirects"), nil)

	imp := r.run(t, id)
	c := importCounts(imp)
	if imp["state"] != "done" || imp["phase"] != "links" || c["collections"] != 5 || c["translations_created"] != 5 || c["images_stored"] != 2 || c["images_failed"] != 1 ||
		c["links_rewritten"] != 3 || c["failed"] != 0 || c["translations_unchanged"] != 0 || len(imp["skipped_locales"].([]any)) != 0 {
		t.Fatalf("import: %s", imp)
	}
	if n := r.h.count("SELECT count(*) FROM help_articles WHERE workspace_id = $1", ws); n != 4 {
		t.Fatalf("%d articles", n)
	}
	if n := r.h.count("SELECT count(*) FROM help_article_translations WHERE workspace_id = $1 AND published_at IS NOT NULL", ws); n != 0 {
		t.Fatalf("%d published", n)
	}
	if n := r.h.count("SELECT count(*) FROM help_collections WHERE workspace_id = $1 AND parent_id IS NOT NULL", ws); n != 3 {
		t.Fatalf("%d sub-collections", n)
	}
	if n := r.h.count("SELECT count(*) FROM help_collection_translations WHERE workspace_id = $1 AND name IN ('Başlarken', 'Hesaplar', 'Accounts / Passwords')", ws); n != 3 {
		t.Fatalf("collection names: %d", n)
	}
	reset := r.body(t, "en", "Reset your password")
	for _, want := range []string{"## Before you start", "**sign-in e-mail**", "![The reset form](" + testOrigin + "/i/", "| Plan | Reset time |", "(" + testOrigin + "/help/"} {
		if !strings.Contains(reset, want) {
			t.Errorf("reset body lacks %q:\n%s", want, reset)
		}
	}
	if strings.Contains(reset, "track()") || strings.Contains(reset, "zendesk.com/hc/en-us/articles") {
		t.Errorf("reset body:\n%s", reset)
	}
	change := r.body(t, "en", "Change your email")
	if !strings.Contains(change, testOrigin+"/help/") || !strings.Contains(change, "https://example.org/policy") || !strings.Contains(change, "/i/") {
		t.Errorf("change body:\n%s", change)
	}
	if tr := r.body(t, "tr", "Parolanızı sıfırlayın"); !strings.Contains(tr, testOrigin+"/help/") || strings.Contains(tr, "/hc/tr/") {
		t.Errorf("turkish body:\n%s", tr)
	}
	if got := r.body(t, "en", "Invoices"); !strings.Contains(got, "[Sample](https://acme.zendesk.com/hc/article_attachments/missing.png)") {
		t.Errorf("an image that could not be fetched stays a link:\n%s", got)
	}
	r.body(t, "en", "Hi article")
	if n := r.h.count("SELECT count(*) FROM events WHERE workspace_id = $1 AND type LIKE 'help.%'", ws); n != 0 {
		t.Errorf("%d help events", n)
	}

	red := r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports/"+id+"/redirects?limit=3"), nil)
	if len(red.body["items"].([]any)) != 3 || red.str("next_cursor") == "" {
		t.Fatalf("redirect page: %s", red.raw)
	}
	csv := r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports/"+id+"/redirects.csv"), nil)
	text := string(csv.raw)
	if !strings.HasPrefix(text, `"kind","locale","old_url","new_url","published"`) || !strings.Contains(text, "https://acme.zendesk.com/hc/en-us/articles/5001-Reset-your-password") ||
		!strings.Contains(text, "https://acme.zendesk.com/hc/tr/categories/100-Baslarken") || strings.Count(text, "\n") != 13 || !strings.Contains(csv.header.Get("Content-Type"), "text/csv") {
		t.Fatalf("csv:\n%s", text)
	}
	if imp["redirects"].(float64) != 12 {
		t.Errorf("redirects: %v", imp["redirects"])
	}

	r.h.clock.Advance(61 * time.Minute)
	again := r.run(t, r.start(t, map[string]any{"source": "zendesk", "migration_id": mig}))
	c = importCounts(again)
	if again["state"] != "done" || c["translations_unchanged"] != 5 || c["translations_created"] != 0 || c["translations_updated"] != 0 {
		t.Fatalf("second run: %s", again)
	}
	if n := r.h.count("SELECT count(*) FROM help_articles WHERE workspace_id = $1", ws); n != 4 {
		t.Fatalf("%d articles after a second run", n)
	}

	art := r.h.st.Pool.QueryRow(context.Background(), "SELECT article_id::text FROM help_article_translations WHERE workspace_id = $1 AND title = 'Change your email'", ws)
	var changeID string
	if err := art.Scan(&changeID); err != nil {
		t.Fatal(err)
	}
	tr := r.tm.owner.expect(http.StatusOK, "GET", r.path("/articles/"+changeID), nil).body["translations"].([]any)[0].(map[string]any)
	r.tm.owner.expect(http.StatusOK, "PUT", r.path("/articles/"+changeID+"/translations/en"), map[string]any{"title": "Change your email", "body": "Edited in Yuva", "base_version": tr["version"]})
	raw, err := os.ReadFile("../migrate/helpdesk/helpdesktest/testdata/zendesk/help_articles_en-us_1.json")
	if err != nil {
		t.Fatal(err)
	}
	r.desk.Override("zendesk/help_articles_en-us_1.json", []byte(strings.ReplaceAll(string(raw), "Open <em>Profile</em>", "Open <em>Account</em>")))
	kept := importCounts(r.run(t, r.start(t, map[string]any{"source": "zendesk", "migration_id": mig})))
	if kept["translations_kept"] != 1 || kept["translations_updated"] != 0 || r.body(t, "en", "Change your email") != "Edited in Yuva" {
		t.Fatalf("an edited draft was not kept: %v", kept)
	}
	over := importCounts(r.run(t, r.start(t, map[string]any{"source": "zendesk", "migration_id": mig, "overwrite_edited": true})))
	if over["translations_updated"] != 1 || !strings.Contains(r.body(t, "en", "Change your email"), "Open *Account*") {
		t.Fatalf("overwrite: %v", over)
	}

	r.h.clock.Advance(61 * time.Minute)
	cancelled := r.start(t, map[string]any{"source": "zendesk", "migration_id": mig})
	r.tm.owner.expect(http.StatusNoContent, "DELETE", r.path("/imports/"+cancelled), nil)
	if got := r.run(t, cancelled); got["state"] != "cancelled" {
		t.Fatalf("cancel: %s", got)
	}
	list := r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports"), nil).body["items"].([]any)
	if len(list) != 5 {
		t.Fatalf("%d imports listed", len(list))
	}
	r.checkRequests(t, "acme.zendesk.com", "/oauth/tokens", helpdesktest.ZendeskLimit/2+2, 0)
}

func (r importRun) unconnected(t *testing.T) string {
	t.Helper()
	address, channel := r.emailChannel(t, r.tm.inbox, "second")
	return r.tm.owner.expect(http.StatusCreated, "POST", "/v1/migrations", map[string]any{
		"source": "zendesk", "preset": "zendesk", "source_address": address, "channel_id": channel,
	}).str("id")
}

func TestHelpScoutDocsImport(t *testing.T) {
	r := newImportRun(t)
	ws := r.tm.ws
	r.tm.owner.expectProblem(http.StatusBadRequest, "locale_not_offered", "POST", r.path("/imports"), map[string]any{"source": "helpscout", "docs_api_key": "k", "locale": "de"})
	key := helpdesktest.DocsKey
	id := r.start(t, map[string]any{"source": "helpscout", "docs_api_key": key})
	raw := r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports/"+id), nil).raw
	if strings.Contains(string(raw), key) {
		t.Fatal("the Docs key is in the answer")
	}
	imp := r.run(t, id)
	c := importCounts(imp)
	if imp["state"] != "done" || c["collections"] != 3 || c["translations_created"] != 4 || c["images_stored"] != 1 || c["links_rewritten"] != 1 {
		t.Fatalf("import: %s", imp)
	}
	if n := r.h.count("SELECT count(*) FROM help_imports WHERE workspace_id = $1 AND sealed IS NOT NULL", ws); n != 0 {
		t.Error("the sealed key outlived the import")
	}
	invite := r.body(t, "en", "Invite a teammate")
	if !strings.Contains(invite, "**Settings**") || !strings.Contains(invite, testOrigin+"/help/") || !strings.Contains(invite, "![Invite](") {
		t.Errorf("invite body:\n%s", invite)
	}
	if n := r.h.count("SELECT count(*) FROM help_article_translations WHERE workspace_id = $1 AND keywords = 'team, invite'", ws); n != 1 {
		t.Error("keywords")
	}
	r.tm.owner.expect(http.StatusOK, "GET", r.path("/imports/"+id+"/redirects"), nil)

	bad := r.run(t, r.start(t, map[string]any{"source": "helpscout", "docs_api_key": "wrong-key"}))
	if bad["state"] != "failed" || !strings.Contains(bad["error"].(string), "Docs API key") {
		t.Fatalf("a wrong key: %s", bad)
	}
	for _, rq := range r.desk.Requests() {
		if rq.Method != http.MethodGet {
			t.Errorf("%s %s%s was sent", rq.Method, rq.Host, rq.Path)
		}
		if rq.Host == helpdesktest.OutsideHost && rq.Auth != "" {
			t.Error("the key went to an image host")
		}
	}
}

func TestHelpImportScopes(t *testing.T) {
	r := newImportRun(t)
	reader := r.tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "r", "scopes": []string{"help:read"}})
	k := r.h.client()
	k.bearer = reader.str("secret")
	k.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", r.path("/imports"), map[string]any{"source": "helpscout", "docs_api_key": "x"})
	k.expect(http.StatusOK, "GET", r.path("/imports"), nil)
	writer := r.tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "w", "scopes": []string{"help:write"}})
	w := r.h.client()
	w.bearer = writer.str("secret")
	w.expect(http.StatusAccepted, "POST", r.path("/imports"), map[string]any{"source": "helpscout", "docs_api_key": helpdesktest.DocsKey})
}

func TestHelpImportImageFailuresDoNotFailTheImport(t *testing.T) {
	r := newImportRun(t)
	mig := r.zendeskMigration(t)
	raw, err := os.ReadFile("../migrate/helpdesk/helpdesktest/testdata/zendesk/help_articles_en-us_2.json")
	if err != nil {
		t.Fatal(err)
	}
	extra := `{"id": 5005, "title": "Pictures", "locale": "en-us", "source_locale": "en-us", "section_id": 202, "draft": false, "position": 2, "label_names": [], "updated_at": "2026-09-05T10:00:00Z",
	"html_url": "https://acme.zendesk.com/hc/en-us/articles/5005-Pictures",
	"body": "<p><img src=\"http://files.example.org/plain.png\"><img src=\"https://files.example.org/private/y.png\"></p>"},`
	r.desk.Override("zendesk/help_articles_en-us_2.json", []byte(strings.Replace(string(raw), `"articles": [`, `"articles": [`+extra, 1)))
	imp := r.run(t, r.start(t, map[string]any{"source": "zendesk", "migration_id": mig}))
	c := importCounts(imp)
	if imp["state"] != "done" || c["images_failed"] != 3 || c["translations_created"] != 6 {
		t.Fatalf("import: %s", imp)
	}
	if b := r.body(t, "en", "Pictures"); !strings.Contains(b, "(http://files.example.org/plain.png)") {
		t.Errorf("body:\n%s", b)
	}
}

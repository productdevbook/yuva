package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
)

func publicComponent(t *testing.T, js response, id string) map[string]any {
	t.Helper()
	for _, c := range js.body["components"].([]any) {
		if m := c.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("no component %s in %s", id, js.raw)
	return nil
}

func TestStatusResponseTimesOptIn(t *testing.T) {
	e := newCheckEnv(t, true, canary(t), nil)
	h, st := e.h, e.st
	e.resolver["app.example.com"] = "127.0.0.1"
	m := e.create(t, map[string]any{"kind": "http", "http": map[string]any{"url": "http://app.example.com:9/secret-health-path"}})
	st.owner.expect(http.StatusCreated, "POST", st.path("/monitors"), map[string]any{"kind": "tcp", "name": "Site", "component_id": st.web,
		"tcp": map[string]any{"host": "app.example.com", "port": 443}})
	unmonitored := st.owner.expect(http.StatusCreated, "POST", st.path("/components"), map[string]any{"name": "Database"}).str("id")
	pg := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	if pg.body["show_response_times"] != false {
		t.Fatalf("show_response_times is on by default: %s", pg.raw)
	}
	ctx := context.Background()
	now := h.clock.Now()
	for i := range 20 {
		if _, err := h.st.Pool.Exec(ctx, `INSERT INTO status_checks (id, workspace_id, monitor_id, at, ok, source, duration_ms, status_code)
			VALUES ($1, $2, $3, $4, true, 'http', $5, 200)`, uuid.New(), st.ws, m.str("id"), now.Add(-time.Duration(i)*20*time.Minute), 100+10*i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.st.Pool.Exec(ctx, `INSERT INTO status_checks (id, workspace_id, monitor_id, at, ok, source, message, error, duration_ms, status_code)
		VALUES ($1, $2, $3, $4, false, 'http', 'leaked-message', 'leaked-error', 9999, 599)`, uuid.New(), st.ws, m.str("id"), now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	js := publicStatus(t, h, "/client/v1/status/"+st.slug)
	if _, ok := publicComponent(t, js, st.api)["response_times"]; ok {
		t.Fatalf("response times while the page does not show them: %s", js.raw)
	}
	page := string(publicStatus(t, h, "/status/"+st.slug).raw)
	if strings.Contains(page, "Response time") || strings.Contains(page, `class="rt-chart`) {
		t.Fatal("the page shows response times while the setting is off")
	}

	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"show_response_times": true})
	js = publicStatus(t, h, "/client/v1/status/"+st.slug)
	rt, ok := publicComponent(t, js, st.api)["response_times"].(map[string]any)
	if !ok {
		t.Fatalf("no response times for the monitored component: %s", js.raw)
	}
	points := rt["points"].([]any)
	if len(points) != 96 || rt["step_seconds"] != 900.0 || rt["median_ms"] != 195.0 || rt["p95_ms"] == nil {
		t.Fatalf("response times %v", rt)
	}
	measured := 0
	for _, p := range points {
		if p != nil {
			measured++
		}
	}
	if measured == 0 || measured > 20 {
		t.Fatalf("%d measured steps of 20 checks", measured)
	}
	if _, ok := publicComponent(t, js, unmonitored)["response_times"]; ok {
		t.Fatalf("response times for a component without a monitor: %s", js.raw)
	}
	page = string(publicStatus(t, h, "/status/"+st.slug).raw)
	if !strings.Contains(page, "Response time · 24 h") || !strings.Contains(page, "195 ms") || !strings.Contains(page, `shape-rendering="crispEdges"`) ||
		!strings.Contains(page, `<label for="rt1">Website</label>`) {
		t.Fatalf("the page lacks the response times: %s", page)
	}
	inc := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"title": "API slow", "message": "Looking.", "publish": true, "components": []map[string]any{{"component_id": st.api, "state": "degraded"}},
	})
	incident := string(publicStatus(t, h, "/status/"+st.slug+"/incidents/"+inc.str("id")).raw)
	if !strings.Contains(incident, "Response time during the incident") || !strings.Contains(incident, `class="band" d="M`) {
		t.Fatalf("the incident page lacks the response times: %s", incident)
	}
	for _, body := range []string{string(js.raw), page, incident} {
		for _, secret := range []string{"leaked-message", "leaked-error", "9999", "599", "app.example.com", "secret-health-path"} {
			if strings.Contains(body, secret) {
				t.Fatalf("a public answer shows %q", secret)
			}
		}
	}

	stranger := newTeam(t, h)
	stranger.owner.expectProblem(http.StatusNotFound, "not_found", "PATCH", st.path(""), map[string]any{"show_response_times": false})
}

func TestStatusPageLanguageParam(t *testing.T) {
	h := newHarness(t)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true, "locale": "en"})

	en := string(publicStatus(t, h, "/status/"+st.slug).raw)
	if !strings.Contains(en, "All systems operational") || !strings.Contains(en, `<html lang="en" dir="ltr">`) {
		t.Fatalf("page in its own language: %s", en)
	}
	de := string(publicStatus(t, h, "/status/"+st.slug+"?lang=de").raw)
	if !strings.Contains(de, "Alle Systeme betriebsbereit") || !strings.Contains(de, `<html lang="de"`) || !strings.Contains(de, "/history?lang=de") ||
		!strings.Contains(de, `<option value="de" lang="de" selected>Deutsch</option>`) {
		t.Fatalf("?lang=de: %s", de)
	}
	if tr := string(publicStatus(t, h, "/status/"+st.slug+"/history?lang=tr-TR").raw); !strings.Contains(tr, "Tüm olaylar ve bakımlar.") {
		t.Fatalf("?lang=tr-TR on the history: %s", tr)
	}
	if unknown := string(publicStatus(t, h, "/status/"+st.slug+"?lang=xx").raw); !strings.Contains(unknown, "All systems operational") || strings.Contains(unknown, "lang=xx") {
		t.Fatalf("an unknown language does not fall back: %s", unknown)
	}
	if again := string(publicStatus(t, h, "/status/"+st.slug).raw); !strings.Contains(again, "All systems operational") {
		t.Fatal("a picked language leaked into the page's own cached answer")
	}
}

func TestStatusHistoryFilter(t *testing.T) {
	h := newHarness(t)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	inc := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"title": "Chat delayed", "message": "Looking.", "publish": true, "components": []map[string]any{{"component_id": st.api, "state": "degraded"}},
	})
	start := h.clock.Now().Add(time.Hour)
	m := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"kind": "maintenance", "title": "Database upgrade", "message": "Planned.", "publish": true,
		"scheduled_start": start.Format(time.RFC3339), "scheduled_end": start.Add(time.Hour).Format(time.RFC3339),
		"components": []map[string]any{{"component_id": st.web}},
	})
	h.clock.Advance(time.Hour + time.Second)
	if err := h.srv.RunStatusMaintenance(context.Background(), api.StatusMaintenanceArgs{WorkspaceID: uuid.MustParse(st.ws), IncidentID: uuid.MustParse(m.str("id"))}); err != nil {
		t.Fatal(err)
	}
	st.owner.expect(http.StatusCreated, "POST", st.path("/incidents/"+inc.str("id")+"/updates"), map[string]any{"state": "resolved", "body": "Fixed."})

	for _, c := range []struct {
		query        string
		inc, maint   bool
		selectedText string
	}{
		{"", true, true, ">All<"},
		{"?filter=incidents", true, false, ">Incidents<"},
		{"?filter=maintenance", false, true, ">Maintenance<"},
		{"?filter=nothing", true, true, ">All<"},
	} {
		r := publicStatus(t, h, "/status/"+st.slug+"/history"+c.query)
		body := string(r.raw)
		if r.status != http.StatusOK || strings.Contains(body, "Chat delayed") != c.inc || strings.Contains(body, "Database upgrade") != c.maint ||
			!strings.Contains(body, `aria-current="page"`+c.selectedText) {
			t.Fatalf("history%s: %d %s", c.query, r.status, body)
		}
	}
	if r := publicStatus(t, h, "/status/"+st.slug+"/incidents/"+inc.str("id")); r.status != http.StatusOK || !strings.Contains(string(r.raw), "Fixed.") ||
		!strings.Contains(string(r.raw), "Degraded performance for 1 h, then Operational") {
		t.Fatalf("incident page %d %s", r.status, r.raw)
	}
}

func TestStatusFontsServedLocally(t *testing.T) {
	h := newHarness(t)
	st := newStatusTeam(t, h)
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true})
	page := publicStatus(t, h, "/status/"+st.slug)
	body := string(page.raw)
	i := strings.Index(body, "/fonts/Geist-Variable.")
	if i < 0 || strings.Contains(body, "fonts.googleapis") || !strings.Contains(page.header.Get("Content-Security-Policy"), "font-src 'self'") {
		t.Fatalf("fonts: %s", body)
	}
	name := body[i : i+strings.Index(body[i:], `"`)]
	f := publicStatus(t, h, "/status/"+st.slug+name)
	if f.status != http.StatusOK || f.header.Get("Content-Type") != "font/woff2" || !strings.Contains(f.header.Get("Cache-Control"), "immutable") || len(f.raw) < 10000 {
		t.Fatalf("font %s: %d %v", name, f.status, f.header)
	}
	if r := publicStatus(t, h, "/status/"+st.slug+"/fonts/other.woff2"); r.status != http.StatusNotFound {
		t.Fatalf("unknown font %d", r.status)
	}
}

func TestStatusPageShowNameAndFollow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	pg := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true, "logo_url": "https://example.com/logo.svg"})
	if pg.body["show_name"] != true || pg.body["show_follow"] != true {
		t.Fatalf("show_name and show_follow are not on by default: %s", pg.raw)
	}
	page := string(publicStatus(t, h, "/status/"+st.slug).raw)
	for _, want := range []string{`alt=""`, `<span class="name">Acme</span>`, `href="#follow"`, `id="follow"`, `>feed.atom</a>`, `>badge.svg</a>`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the default page lacks %s: %s", want, page)
		}
	}

	pg = st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"show_name": false, "show_follow": false})
	if pg.body["show_name"] != false || pg.body["show_follow"] != false {
		t.Fatalf("settings not saved: %s", pg.raw)
	}
	page = string(publicStatus(t, h, "/status/"+st.slug).raw)
	if !strings.Contains(page, `alt="Acme"`) || !strings.Contains(page, `rel="alternate"`) {
		t.Fatalf("the logo lost its name or the feed lost its discovery link: %s", page)
	}
	for _, gone := range []string{`class="name"`, `href="#follow"`, `id="follow"`, `>feed.atom</a>`, `>badge.svg</a>`} {
		if strings.Contains(page, gone) {
			t.Fatalf("the page still shows %s: %s", gone, page)
		}
	}
	for _, p := range []string{"/feed.atom", "/badge.svg"} {
		if r := publicStatus(t, h, "/status/"+st.slug+p); r.status != http.StatusOK {
			t.Fatalf("%s answers %d while the page hides its link", p, r.status)
		}
	}
	inc := st.owner.expect(http.StatusCreated, "POST", st.path("/incidents"), map[string]any{
		"title": "API slow", "message": "Looking.", "publish": true, "components": []map[string]any{{"component_id": st.api, "state": "degraded"}},
	})
	if incident := string(publicStatus(t, h, "/status/"+st.slug+"/incidents/"+inc.str("id")).raw); strings.Contains(incident, "Stay informed") {
		t.Fatalf("the incident page offers a feed while the page hides it and has no e-mail: %s", incident)
	}

	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"logo_url": nil})
	if page = string(publicStatus(t, h, "/status/"+st.slug).raw); !strings.Contains(page, `<span class="name">Acme</span>`) {
		t.Fatalf("a page without a logo hides its name: %s", page)
	}
}

func TestStatusPageDarkLogo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	st.owner.expectProblem(http.StatusBadRequest, "validation_failed", "PATCH", st.path(""), map[string]any{"logo_dark_url": "http://example.com/dark.svg"})
	pg := st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{
		"published": true, "logo_url": "https://example.com/logo.svg", "logo_dark_url": "https://example.com/dark.svg",
	})
	if pg.body["logo_dark_url"] != "https://example.com/dark.svg" {
		t.Fatalf("logo_dark_url not saved: %s", pg.raw)
	}
	page := string(publicStatus(t, h, "/status/"+st.slug).raw)
	if !strings.Contains(page, `<picture><source media="(prefers-color-scheme: dark)" srcset="https://example.com/dark.svg"><img src="https://example.com/logo.svg"`) {
		t.Fatalf("the header lacks the dark logo: %s", page)
	}
	if out := publicStatus(t, h, "/client/v1/status/"+st.slug); !strings.Contains(string(out.raw), `"logo_dark_url":"https://example.com/dark.svg"`) {
		t.Fatalf("the public JSON lacks logo_dark_url: %s", out.raw)
	}

	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"logo_url": nil})
	if page = string(publicStatus(t, h, "/status/"+st.slug).raw); strings.Contains(page, "dark.svg") {
		t.Fatalf("a page without a logo shows the dark logo: %s", page)
	}
	pg = st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"logo_url": "https://example.com/logo.svg", "logo_dark_url": nil})
	if _, ok := pg.body["logo_dark_url"]; ok {
		t.Fatalf("logo_dark_url not cleared: %s", pg.raw)
	}
	if page = string(publicStatus(t, h, "/status/"+st.slug).raw); strings.Contains(page, "<picture>") {
		t.Fatalf("a page without a dark logo still wraps the logo: %s", page)
	}
}

func TestStatusPushResponseTimes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	st := newStatusTeam(t, h)
	_, measured := st.monitor(t, map[string]any{"kind": "push", "name": "Prober", "component_id": st.api})
	_, bare := st.monitor(t, map[string]any{"kind": "push", "name": "Alerts", "component_id": st.web})
	st.owner.expect(http.StatusOK, "PATCH", st.path(""), map[string]any{"published": true, "show_response_times": true})

	for _, d := range []int{100, 200, 300} {
		ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+measured, map[string]any{"state": "operational", "duration_ms": d})
	}
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+measured, map[string]any{"state": "major_outage", "duration_ms": 90000})
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+measured, map[string]any{"state": "operational"})
	ingress(t, h, http.StatusOK, "POST", "/ingress/status/"+bare, map[string]any{"state": "operational"})
	for _, bad := range []any{12.5, -1, 120001, "100", true} {
		ingress(t, h, http.StatusBadRequest, "POST", "/ingress/status/"+measured, map[string]any{"state": "operational", "duration_ms": bad})
	}

	js := publicStatus(t, h, "/client/v1/status/"+st.slug)
	rt, ok := publicComponent(t, js, st.api)["response_times"].(map[string]any)
	if !ok || rt["median_ms"] != 200.0 || rt["p95_ms"] != 290.0 {
		t.Fatalf("response times of the measured push monitor: %s", js.raw)
	}
	if _, ok := publicComponent(t, js, st.web)["response_times"]; ok {
		t.Fatalf("response times for a push monitor that never sent a duration: %s", js.raw)
	}
	page := string(publicStatus(t, h, "/status/"+st.slug).raw)
	if !strings.Contains(page, "200 ms") || strings.Contains(page, `<label for="rt2">Website</label>`) {
		t.Fatalf("the page's response times: %s", page)
	}
}

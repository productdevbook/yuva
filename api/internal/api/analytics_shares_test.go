package api_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/storage"
)

func shareQuery(dims ...string) map[string]any {
	q := map[string]any{"metrics": []string{"visitors", "pageviews"}, "range": map[string]any{"period": "today"}}
	if len(dims) > 0 {
		q["dimensions"] = dims
	}
	return q
}

func TestAnalyticsSharePasswordViewsRevokeAndExpiry(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	h.clock.Set(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	id, key := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}, "timezone": "UTC", "inbox_id": tm.inbox})
	otherID, otherKey := createProperty(tm.owner, map[string]any{"name": "Other site", "domains": []string{"example.com"}, "timezone": "UTC"})
	send(h, key, "203.0.113.10", uaMac, view("/pricing"))
	send(h, key, "198.51.100.20", uaPixel, view("/docs"))
	for i := range 5 {
		send(h, otherKey, fmt.Sprintf("192.0.2.%d", i+1), uaWindows, view("/elsewhere"))
	}
	h.flushAnalytics()
	foreignProp, _ := createProperty(other.owner, map[string]any{"name": "Foreign", "domains": []string{"example.com"}})
	foreignGoal := other.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+foreignProp+"/goals",
		map[string]any{"name": "Docs", "kind": "pageview", "path_pattern": "/docs"}).str("id")

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", "/v1/analytics/properties/"+id+"/shares", map[string]any{"name": "x", "views": []string{"overview"}})
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/analytics/properties/"+id+"/shares",
		map[string]any{"name": "Short", "views": []string{"overview"}, "password": "short"})
	created := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+id+"/shares",
		map[string]any{"name": "Investors", "views": []string{"overview", "pages"}, "password": "correct horse"})
	token := created.str("token")
	share := created.body["share"].(map[string]any)
	shareID := share["id"].(string)
	if !strings.HasPrefix(token, "yuva_sh_") || created.str("url") != testOrigin+"/share/"+token || share["has_password"] != true ||
		!strings.HasPrefix(token, share["token_prefix"].(string)) {
		t.Fatalf("created %s", created.raw)
	}
	var stored []byte
	var pwHash string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT token_hash, password_hash FROM analytics_shares WHERE id = $1", shareID).Scan(&stored, &pwHash); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(token)) || !strings.HasPrefix(pwHash, "$argon2id$") || strings.Contains(pwHash, "correct horse") {
		t.Fatalf("token or password stored readable: %q", pwHash)
	}
	list := tm.agent.expect(http.StatusOK, "GET", "/v1/analytics/properties/"+id+"/shares", nil).body["items"].([]any)
	if len(list) != 1 || strings.Contains(string(mustJSONTest(list)), token) {
		t.Fatalf("list %v", list)
	}

	base := "/client/v1/analytics-shares/" + token
	viewer := h.client()
	if r := viewer.expect(http.StatusOK, "GET", base, nil); r.body["password_required"] != true || len(r.body) != 1 {
		t.Fatalf("a locked link tells more than that it is locked: %s", r.raw)
	}
	viewer.expectProblem(http.StatusUnauthorized, "share_password_required", "POST", base+"/query", shareQuery())
	viewer.expectProblem(http.StatusUnauthorized, "wrong_password", "POST", base+"/access", map[string]any{"password": "wrong horse"})
	access := viewer.expect(http.StatusOK, "POST", base+"/access", map[string]any{"password": "correct horse"}).str("access_token")
	viewer.bearer = access
	page := viewer.expect(http.StatusOK, "GET", base, nil)
	if page.body["password_required"] != false || page.body["name"] != "Investors" || page.body["property"].(map[string]any)["name"] != "Site" ||
		fmt.Sprint(page.body["views"]) != "[overview pages]" || page.body["goals"] != nil {
		t.Fatalf("page %s", page.raw)
	}
	r := viewer.expect(http.StatusOK, "POST", base+"/query", shareQuery())
	if totals(r)["visitors"].(float64) != 2 || totals(r)["pageviews"].(float64) != 2 {
		t.Fatalf("a share counts other properties: %s", r.raw)
	}
	r = viewer.expect(http.StatusOK, "POST", base+"/query", shareQuery("page"))
	if pages := rowsBy(r); len(pages) != 2 || pages["/pricing"] == nil || pages["/elsewhere"] != nil {
		t.Fatalf("pages %s", r.raw)
	}
	viewer.expect(http.StatusOK, "POST", base+"/query", shareQuery("time:hour"))
	for _, q := range []map[string]any{
		shareQuery("country"),
		shareQuery("time:day", "browser"),
		{"metrics": []string{"visitors"}, "range": map[string]any{"period": "today"}, "filters": []map[string]any{{"dimension": "country", "op": "is", "values": []string{"TR"}}}},
		{"metrics": []string{"events"}, "range": map[string]any{"period": "today"}},
		{"metrics": []string{"visitors"}, "dimensions": []string{"page"}, "range": map[string]any{"period": "today"}, "order_by": []map[string]any{{"metric": "events"}}},
		{"metrics": []string{"conversions"}, "range": map[string]any{"period": "today"}, "goal_id": foreignGoal},
	} {
		viewer.expectProblem(http.StatusForbidden, "view_not_shared", "POST", base+"/query", q)
	}
	viewer.expectProblem(http.StatusForbidden, "view_not_shared", "GET", base+"/realtime", nil)
	viewer.expectProblem(http.StatusBadRequest, "validation_failed", "POST", base+"/query",
		map[string]any{"metrics": []string{"visitors"}, "range": map[string]any{"period": "today"}, "segment_id": uuid.NewV7().String()})

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/analytics/properties/"+id+"/shares/"+shareID, map[string]any{"views": []string{"overview", "goals"}})
	goals := viewer.expect(http.StatusOK, "GET", base, nil).body["goals"].([]any)
	if len(goals) != 0 {
		t.Fatalf("goals of another workspace %v", goals)
	}
	viewer.expectProblem(http.StatusNotFound, "not_found", "POST", base+"/query",
		map[string]any{"metrics": []string{"conversions"}, "range": map[string]any{"period": "today"}, "goal_id": foreignGoal})

	tm.owner.expect(http.StatusOK, "PATCH", "/v1/analytics/properties/"+id+"/shares/"+shareID, map[string]any{"password": "battery staple"})
	viewer.expectProblem(http.StatusUnauthorized, "share_password_required", "POST", base+"/query", shareQuery())
	viewer.bearer = viewer.expect(http.StatusOK, "POST", base+"/access", map[string]any{"password": "battery staple"}).str("access_token")
	viewer.expect(http.StatusOK, "POST", base+"/query", shareQuery())
	tampered := access[:len(access)-2] + "AA"
	stranger := h.client()
	stranger.bearer = tampered
	stranger.expectProblem(http.StatusUnauthorized, "share_password_required", "POST", base+"/query", shareQuery())

	for _, c := range []struct{ method, path string }{
		{"GET", "/v1/analytics/properties/" + id + "/shares"},
		{"PATCH", "/v1/analytics/properties/" + id + "/shares/" + shareID},
		{"POST", "/v1/analytics/properties/" + id + "/shares/" + shareID + "/revoke"},
		{"DELETE", "/v1/analytics/properties/" + id + "/shares/" + shareID},
	} {
		var body any
		if c.method == "PATCH" {
			body = map[string]any{"name": "Mine"}
		}
		other.owner.expectProblem(http.StatusNotFound, "not_found", c.method, c.path, body)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "POST", "/v1/analytics/properties/"+foreignProp+"/shares/"+shareID+"/revoke", nil)

	revoked := tm.owner.expect(http.StatusOK, "POST", "/v1/analytics/properties/"+id+"/shares/"+shareID+"/revoke", nil)
	if revoked.body["active"] != false || revoked.body["revoked_at"] == nil {
		t.Fatalf("revoked %s", revoked.raw)
	}
	viewer.expectProblem(http.StatusNotFound, "not_found", "GET", base, nil)
	viewer.expectProblem(http.StatusNotFound, "not_found", "POST", base+"/query", shareQuery())
	tm.owner.expectProblem(http.StatusConflict, "share_revoked", "PATCH", "/v1/analytics/properties/"+id+"/shares/"+shareID, map[string]any{"name": "Again"})

	open := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+otherID+"/shares", map[string]any{
		"name": "Weekly", "views": []string{"overview", "realtime"}, "expires_at": h.clock.Now().Add(time.Hour).Format(time.RFC3339),
	}).str("token")
	anyone := h.client()
	if r := anyone.expect(http.StatusOK, "GET", "/client/v1/analytics-shares/"+open, nil); r.body["password_required"] != false {
		t.Fatalf("open link %s", r.raw)
	}
	if r := anyone.expect(http.StatusOK, "POST", "/client/v1/analytics-shares/"+open+"/query", shareQuery()); totals(r)["pageviews"].(float64) != 5 {
		t.Fatalf("open link totals %s", r.raw)
	}
	anyone.expect(http.StatusOK, "GET", "/client/v1/analytics-shares/"+open+"/realtime", nil)
	anyone.expectProblem(http.StatusConflict, "no_password", "POST", "/client/v1/analytics-shares/"+open+"/access", map[string]any{"password": "anything"})
	h.clock.Advance(2 * time.Hour)
	anyone.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/analytics-shares/"+open, nil)
	anyone.expectProblem(http.StatusNotFound, "not_found", "POST", "/client/v1/analytics-shares/"+open+"/query", shareQuery())
	anyone.expectProblem(http.StatusNotFound, "not_found", "GET", "/client/v1/analytics-shares/yuva_sh_unknown", nil)
}

func TestAnalyticsShareUnlockIsLimitedPerAddress(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	id, _ := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	token := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+id+"/shares",
		map[string]any{"name": "Locked", "views": []string{"overview"}, "password": "correct horse"}).str("token")
	guesser := h.client()
	for range 10 {
		guesser.expectProblem(http.StatusUnauthorized, "wrong_password", "POST", "/client/v1/analytics-shares/"+token+"/access", map[string]any{"password": "guess"})
	}
	r := guesser.expect(http.StatusTooManyRequests, "POST", "/client/v1/analytics-shares/"+token+"/access", map[string]any{"password": "correct horse"})
	if r.header.Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	h.client().expect(http.StatusOK, "POST", "/client/v1/analytics-shares/"+token+"/access", map[string]any{"password": "correct horse"})
}

func sharePageHeaders(t *testing.T, h *harness, token string) http.Header {
	t.Helper()
	res, err := http.Get(h.url + "/share/" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("<html")) {
		t.Fatalf("share page %d %s", res.StatusCode, body)
	}
	return res.Header
}

func TestAnalyticsShareEmbedsOnListedOriginsOnly(t *testing.T) {
	h := newHarnessWith(t, nil)
	tm := newTeam(t, h)
	id, _ := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}})
	for _, bad := range []string{"https://dash.example.com/path", "javascript:alert(1)", "https://dash.example.com; script-src *", "ftp://example.com", "https://a b.com"} {
		tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", "/v1/analytics/properties/"+id+"/shares",
			map[string]any{"name": "Bad", "views": []string{"overview"}, "embed_origins": []string{bad}})
	}
	r := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+id+"/shares", map[string]any{
		"name": "Embedded", "views": []string{"overview"}, "embed_origins": []string{"https://Dash.Example.com/", "https://*.example.org:8443"},
	})
	if fmt.Sprint(r.body["share"].(map[string]any)["embed_origins"]) != "[https://dash.example.com https://*.example.org:8443]" {
		t.Fatalf("origins %s", r.raw)
	}
	embedded := r.str("token")
	plain := tm.owner.expect(http.StatusCreated, "POST", "/v1/analytics/properties/"+id+"/shares", map[string]any{"name": "Plain", "views": []string{"overview"}}).str("token")

	hd := sharePageHeaders(t, h, embedded)
	csp := hd.Get("Content-Security-Policy")
	if !strings.HasSuffix(csp, "frame-ancestors https://dash.example.com https://*.example.org:8443") || hd.Get("X-Frame-Options") != "" {
		t.Fatalf("listed origins are not frame ancestors: %q, X-Frame-Options %q", csp, hd.Get("X-Frame-Options"))
	}
	if strings.Contains(csp, "https://other.example.net") || strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("csp %q", csp)
	}
	for _, token := range []string{plain, "yuva_sh_unknown"} {
		hd = sharePageHeaders(t, h, token)
		if !strings.HasSuffix(hd.Get("Content-Security-Policy"), "frame-ancestors 'none'") || hd.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("a link without origins can be framed: %v", hd)
		}
	}
	shareID := r.body["share"].(map[string]any)["id"].(string)
	tm.owner.expect(http.StatusOK, "POST", "/v1/analytics/properties/"+id+"/shares/"+shareID+"/revoke", nil)
	if hd = sharePageHeaders(t, h, embedded); !strings.HasSuffix(hd.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatalf("a revoked link can still be framed: %v", hd)
	}
}

func mustJSONTest(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func download(c *client, path string) (int, string, []byte) {
	c.h.t.Helper()
	req, _ := http.NewRequest(http.MethodGet, c.h.url+path, nil)
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.h.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Content-Type"), body
}

func runExport(t *testing.T, h *harness, ws, id string) {
	t.Helper()
	if err := h.srv.RunAnalyticsExport(context.Background(), uuid.MustParse(ws), uuid.MustParse(id)); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyticsRawExportIsNDJSONWithoutSaltsAndExpires(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	id, key := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}, "timezone": "UTC", "inbox_id": tm.inbox})
	days := []int{0, 14, 29}
	today := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for _, back := range days {
		h.clock.Set(today.AddDate(0, 0, -back))
		send(h, key, "203.0.113.10", uaMac, view("/pricing"))
		send(h, key, "203.0.113.10", uaMac, event("Signup", "/signup", map[string]any{"plan": "pro"}, map[string]any{"a": "29.90", "c": "EUR"}))
		send(h, key, "198.51.100.20", uaPixel, view("/docs"))
		h.flushAnalytics()
	}
	h.clock.Set(today)
	path := "/v1/analytics/properties/" + id + "/exports"
	rawReq := map[string]any{"kind": "raw", "range": map[string]any{"from": "2026-09-13", "to": "2026-10-12"}}

	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "POST", path, rawReq)
	reader := h.client()
	reader.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "reader", "scopes": []string{"analytics:read"}}).str("secret")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "POST", path, rawReq)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "POST", path, map[string]any{"kind": "raw", "format": "csv", "range": map[string]any{"period": "7d"}})
	tm.owner.expectProblem(http.StatusUnprocessableEntity, "range_too_large", "POST", path, map[string]any{"kind": "raw", "range": map[string]any{"from": "2026-09-01", "to": "2026-10-12"}})
	tm.owner.expectProblem(http.StatusUnprocessableEntity, "needs_raw_window", "POST", path, map[string]any{"kind": "raw", "range": map[string]any{"from": "2026-06-01", "to": "2026-06-10"}})

	ex := tm.owner.expect(http.StatusAccepted, "POST", path, rawReq)
	exID := ex.str("id")
	if ex.str("status") != "queued" || ex.str("format") != "ndjson" || ex.str("from") != "2026-09-13" {
		t.Fatalf("export %s", ex.raw)
	}
	if n := h.count("SELECT count(*) FROM river_job WHERE kind = 'analytics_export' AND args->>'export_id' = $1", exID); n != 1 {
		t.Fatalf("%d export jobs", n)
	}
	tm.owner.expectProblem(http.StatusConflict, "export_not_ready", "GET", path+"/"+exID+"/download", nil)
	runExport(t, h, tm.ws, exID)
	done := tm.owner.expect(http.StatusOK, "GET", path+"/"+exID, nil)
	if done.str("status") != "done" || done.body["rows"].(float64) != 9 || done.body["progress"].(float64) != 100 ||
		done.str("file_name") != "analytics-raw-2026-09-13-2026-10-12.ndjson" {
		t.Fatalf("finished export %s", done.raw)
	}
	tm.agent.expectProblem(http.StatusForbidden, "forbidden", "GET", path+"/"+exID, nil)
	if items := tm.agent.expect(http.StatusOK, "GET", path, nil); len(items.body["items"].([]any)) != 0 {
		t.Fatalf("an agent lists raw exports: %s", items.raw)
	}
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", path+"/"+exID, nil)
	if status, _, _ := download(other.owner, path+"/"+exID+"/download?workspace_id="+other.ws); status != http.StatusNotFound {
		t.Fatalf("another workspace downloads with status %d", status)
	}

	status, ctype, body := download(tm.owner, path+"/"+exID+"/download?workspace_id="+tm.ws)
	if status != http.StatusOK || ctype != "application/x-ndjson" {
		t.Fatalf("download %d %s", status, ctype)
	}
	var salts [][]byte
	rows, err := h.st.Pool.Query(context.Background(), "SELECT salt FROM analytics_salts WHERE property_id = $1", id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s []byte
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		salts = append(salts, s)
	}
	rows.Close()
	var visitors []string
	rows, err = h.st.Pool.Query(context.Background(), "SELECT visitor::text FROM analytics_visits WHERE property_id = $1 UNION SELECT visit::text FROM analytics_visits WHERE property_id = $1", id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		visitors = append(visitors, v)
	}
	rows.Close()
	if len(salts) == 0 || len(visitors) == 0 {
		t.Fatal("nothing to compare against")
	}
	perDay := map[string]int{}
	visitsSeen := map[float64]bool{}
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		var line map[string]any
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("not NDJSON: %q", sc.Text())
		}
		for k := range line {
			if strings.Contains(k, "visitor") || strings.Contains(k, "salt") || strings.Contains(k, "contact") || k == "ip" || k == "referrer_path" {
				t.Fatalf("raw export carries %q: %s", k, sc.Text())
			}
		}
		perDay[line["day"].(string)]++
		visitsSeen[line["visit"].(float64)] = true
		if line["type"] == "event" && (line["name"] != "Signup" || line["revenue_minor"].(float64) != 2990 || line["currency"] != "EUR" ||
			line["props"].(map[string]any)["plan"] != "pro") {
			t.Fatalf("event line %s", sc.Text())
		}
		if line["country"] == nil || line["browser"] == nil || line["channel"] == nil {
			t.Fatalf("line without its visit: %s", sc.Text())
		}
	}
	if len(perDay) != 3 || perDay["2026-09-13"] != 3 || perDay["2026-10-12"] != 3 || len(visitsSeen) != 6 || !visitsSeen[1] || !visitsSeen[6] {
		t.Fatalf("days %v visits %v\n%s", perDay, visitsSeen, body)
	}
	for _, s := range salts {
		for _, enc := range []string{string(s), base64.StdEncoding.EncodeToString(s), base64.RawURLEncoding.EncodeToString(s), fmt.Sprintf("%x", s)} {
			if bytes.Contains(body, []byte(enc)) {
				t.Fatal("the export carries a salt")
			}
		}
	}
	for _, v := range visitors {
		if bytes.Contains(body, []byte(v)) {
			t.Fatalf("the export carries a stored visitor or visit id %s", v)
		}
	}

	var objectKey string
	if err := h.st.Pool.QueryRow(context.Background(), "SELECT storage_key FROM analytics_exports WHERE id = $1", exID).Scan(&objectKey); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(7*24*time.Hour + time.Minute)
	if status, _, _ := download(tm.owner, path+"/"+exID+"/download?workspace_id="+tm.ws); status != http.StatusNotFound {
		t.Fatalf("an expired export downloads with %d", status)
	}
	if n, err := h.srv.CleanAnalyticsExports(context.Background()); err != nil || n < 1 {
		t.Fatalf("cleaned %d, %v", n, err)
	}
	if _, err := h.storage.Open(context.Background(), objectKey); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("the file outlives its export: %v", err)
	}
	if n := h.count("SELECT count(*) FROM analytics_exports WHERE id = $1", exID); n != 0 {
		t.Fatal("the export row outlives its expiry")
	}
}

func TestAnalyticsCSVExportEscapesFormulas(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	h.clock.Set(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC))
	id, key := createProperty(tm.owner, map[string]any{"name": "Site", "domains": []string{"example.com"}, "timezone": "UTC"})
	campaigns := []string{"=HYPERLINK(\"https://evil.example\")", "+1+1", "-2", "@SUM(A1)", "plain"}
	for i, c := range campaigns {
		send(h, key, "203.0."+strconv.Itoa(100+i)+".10", uaMac, view("/?utm_campaign="+url.QueryEscape(c)))
	}
	h.flushAnalytics()
	path := "/v1/analytics/properties/" + id + "/exports"
	daily := tm.owner.expect(http.StatusAccepted, "POST", path, map[string]any{"kind": "daily", "range": map[string]any{"period": "today"}, "dimension": "utm_campaign"}).str("id")
	viewExport := tm.agent.do("POST", path, map[string]any{"kind": "view", "query": map[string]any{
		"metrics": []string{"visitors"}, "dimensions": []string{"utm_campaign"}, "range": map[string]any{"period": "today"}}})
	if viewExport.status != http.StatusNotFound {
		t.Fatalf("an agent exports a property it cannot see: %s", viewExport.raw)
	}
	view := tm.owner.expect(http.StatusAccepted, "POST", path, map[string]any{"kind": "view", "query": map[string]any{
		"metrics": []string{"visitors"}, "dimensions": []string{"utm_campaign"}, "range": map[string]any{"period": "today"}}}).str("id")
	ndjson := tm.owner.expect(http.StatusAccepted, "POST", path, map[string]any{"kind": "daily", "format": "ndjson", "range": map[string]any{"period": "today"}}).str("id")
	tm.owner.expect(http.StatusTooManyRequests, "POST", path, map[string]any{"kind": "daily", "range": map[string]any{"period": "today"}})
	for _, ex := range []string{daily, view, ndjson} {
		runExport(t, h, tm.ws, ex)
	}
	for _, ex := range []string{daily, view} {
		status, ctype, body := download(tm.owner, path+"/"+ex+"/download?workspace_id="+tm.ws)
		if status != http.StatusOK || ctype != "text/csv" {
			t.Fatalf("download %d %s %s", status, ctype, body)
		}
		for _, want := range []string{`"'=HYPERLINK(""https://evil.example"")"`, `"'+1+1"`, `"'-2"`, `"'@SUM(A1)"`, `"plain"`} {
			if !bytes.Contains(body, []byte(want)) {
				t.Fatalf("%s: no %s in\n%s", ex, want, body)
			}
		}
		if bytes.Contains(body, []byte(`,"=`)) || bytes.Contains(body, []byte(`,"+`)) {
			t.Fatalf("a formula is not escaped:\n%s", body)
		}
	}
	_, ctype, body := download(tm.owner, path+"/"+ndjson+"/download?workspace_id="+tm.ws)
	var line map[string]any
	if ctype != "application/x-ndjson" || json.Unmarshal(bytes.TrimSpace(body), &line) != nil || line["time:day"] != "2026-10-12" || line["visitors"].(float64) != 5 {
		t.Fatalf("daily NDJSON %s %s", ctype, body)
	}
	tm.owner.expect(http.StatusNoContent, "DELETE", path+"/"+daily, nil)
	if status, _, _ := download(tm.owner, path+"/"+daily+"/download?workspace_id="+tm.ws); status != http.StatusNotFound {
		t.Fatalf("a deleted export downloads with %d", status)
	}
}

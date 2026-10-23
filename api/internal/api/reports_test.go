package api_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"uuid"
)

type seededConv struct {
	id                     string
	hidden, labeled        bool
	byAgent                bool
	asked, replied, closed time.Time
	reopened               *time.Time
	rating                 string
	ratedAt                time.Time
	slaOutcome             string
}

func (c seededConv) firstReply() int64 { return int64(c.replied.Sub(c.asked) / time.Second) }
func (c seededConv) resolution() int64 { return int64(c.closed.Sub(c.asked) / time.Second) }

// seedReport makes n conversations through the API, then moves their messages, ratings and SLA
// outcomes to spread instants over the last six days.
func seedReport(t *testing.T, h *harness, tm team, hidden, label string, n int) []seededConv {
	t.Helper()
	key := tm.apiKey(h)
	base := h.clock.Now().Add(-6 * 24 * time.Hour).Truncate(time.Second)
	step := 110 * time.Hour / time.Duration(n)
	var out []seededConv
	ctx := context.Background()
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	for i := range n {
		c := seededConv{hidden: i%3 == 2, labeled: i%2 == 0}
		c.byAgent = i%2 == 1 && !c.hidden
		inbox := tm.inbox
		if c.hidden {
			inbox = hidden
		}
		c.id = key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": inbox, "contact_id": tm.contact}).str("id")
		post(key, c.id, map[string]any{"kind": "message", "direction": "in", "body": "help"})
		replier := tm.owner
		if c.byAgent {
			replier = tm.agent
		}
		post(replier, c.id, map[string]any{"kind": "message", "body": "on it"})
		if c.labeled {
			tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+c.id, map[string]any{"labels": []string{label}})
		}
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+c.id, map[string]any{"status": "closed"})
		c.asked = base.Add(time.Duration(i) * step)
		c.replied = c.asked.Add(time.Duration(45*(i+1)*(i+1)) * time.Second)
		c.closed = c.replied.Add(time.Duration(600*(i%7+1)) * time.Second)
		if i%5 == 0 {
			tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+c.id, map[string]any{"status": "open"})
			r := c.closed.Add(time.Hour)
			c.reopened = &r
		}
		exec := func(sql string, args ...any) {
			t.Helper()
			if _, err := h.st.Pool.Exec(ctx, sql, args...); err != nil {
				t.Fatal(err)
			}
		}
		last := c.closed
		if c.reopened != nil {
			last = *c.reopened
		}
		exec(`UPDATE messages SET created_at = CASE
			WHEN kind = 'message' AND direction = 'in' THEN $2::timestamptz
			WHEN kind = 'message' THEN $3::timestamptz
			WHEN event->>'status' = 'closed' THEN $4::timestamptz
			ELSE $5::timestamptz END WHERE conversation_id = $1`, c.id, c.asked, c.replied, c.closed, last)
		exec(`UPDATE conversations SET created_at = $2, last_activity_at = $3 WHERE id = $1`, c.id, c.asked, last)
		if i%3 != 2 {
			c.rating = map[bool]string{true: "good", false: "bad"}[i%2 == 0]
			c.ratedAt = c.closed.Add(time.Minute)
			exec(`UPDATE conversations SET rating = $2, rated_at = $3 WHERE id = $1`, c.id, c.rating, c.ratedAt)
		}
		if i%4 < 2 {
			c.slaOutcome = map[bool]string{true: "met", false: "breached"}[i%4 == 0]
			exec(`INSERT INTO sla_outcomes (id, workspace_id, conversation_id, inbox_id, target, started_at, outcome, at, assignee_id)
				VALUES ($1, $2, $3, $4, 'first_reply', $5, $6, $7, NULL)`, uuid.New(), tm.ws, c.id, inbox, c.asked, c.slaOutcome, c.replied)
		}
		out = append(out, c)
	}
	return out
}

func localDay(t time.Time, loc *time.Location) string { return t.In(loc).Format(time.DateOnly) }

// exactPercentile interpolates between the sorted values on either side of p.
func exactPercentile(v []int64, p float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	pos := p * float64(len(s)-1)
	lo := int(math.Floor(pos))
	if lo+1 >= len(s) {
		return float64(s[lo])
	}
	return float64(s[lo]) + (float64(s[lo+1])-float64(s[lo]))*(pos-float64(lo))
}

func within(t *testing.T, what string, got any, want float64) {
	t.Helper()
	g, ok := got.(float64)
	if !ok || math.Abs(g-want) > math.Max(1, want*0.03) {
		t.Fatalf("%s is %v, want %.1f within 3%%", what, got, want)
	}
}

func rnum(v map[string]any, key string) int64 {
	f, _ := v[key].(float64)
	return int64(f)
}

func reportOf(t *testing.T, c *client, query string) map[string]any {
	t.Helper()
	return c.expect(http.StatusOK, "GET", "/v1/reports?"+query, nil).body
}

func TestReportsMatchTheirConversations(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	other := newTeam(t, h)
	ctx := context.Background()
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Billing"}).str("id")
	ownerID := tm.ownerMemberID(t)
	convs := seedReport(t, h, tm, hidden, label, 24)
	seedReport(t, h, other, other.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "B", "slug": "b"}).body["inbox"].(map[string]any)["id"].(string),
		other.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "X"}).str("id"), 3)

	ws := uuid.MustParse(tm.ws)
	if more, err := h.srv.RollupReports(ctx, ws); err != nil || more {
		t.Fatalf("rollup: %v %v", more, err)
	}
	var rows int
	if err := h.st.Pool.QueryRow(ctx, "SELECT count(*) FROM report_rollups WHERE workspace_id = $1", ws).Scan(&rows); err != nil || rows == 0 {
		t.Fatalf("no rollup rows: %d %v", rows, err)
	}

	now := h.clock.Now()
	for _, tz := range []string{"UTC", "Asia/Kolkata", "America/New_York", "Pacific/Chatham"} {
		loc, _ := time.LoadLocation(tz)
		from := localDay(convs[0].asked.Add(-24*time.Hour), loc)
		to := localDay(now, loc)
		for _, viewer := range []struct {
			name  string
			c     *client
			sees  func(seededConv) bool
			query string
		}{
			{"owner", tm.owner, func(seededConv) bool { return true }, ""},
			{"agent", tm.agent, func(c seededConv) bool { return !c.hidden }, ""},
			{"label", tm.owner, func(c seededConv) bool { return c.labeled }, "&label_id=" + label},
		} {
			type day struct {
				opened, closed, reopened, replies, good, bad, met, breached int64
				firsts, resolutions                                         []int64
			}
			want := map[string]*day{}
			at := func(ts time.Time) *day {
				k := localDay(ts, loc)
				if want[k] == nil {
					want[k] = &day{}
				}
				return want[k]
			}
			var firsts []int64
			for _, c := range convs {
				if !viewer.sees(c) {
					continue
				}
				at(c.asked).opened++
				at(c.replied).replies++
				at(c.replied).firsts = append(at(c.replied).firsts, c.firstReply())
				firsts = append(firsts, c.firstReply())
				at(c.closed).closed++
				at(c.closed).resolutions = append(at(c.closed).resolutions, c.resolution())
				if c.reopened != nil {
					at(*c.reopened).reopened++
				}
				switch c.rating {
				case "good":
					at(c.ratedAt).good++
				case "bad":
					at(c.ratedAt).bad++
				}
				switch c.slaOutcome {
				case "met":
					at(c.replied).met++
				case "breached":
					at(c.replied).breached++
				}
			}
			r := reportOf(t, viewer.c, "from="+from+"&to="+to+"&timezone="+url.QueryEscape(tz)+viewer.query)
			series := r["series"].([]any)
			for _, it := range series {
				p := it.(map[string]any)
				v := p["values"].(map[string]any)
				w := want[p["start"].(string)]
				if w == nil {
					w = &day{}
				}
				sla := v["sla"].(map[string]any)["first_reply"].(map[string]any)
				ratings := v["ratings"].(map[string]any)
				got := []int64{rnum(v, "opened"), rnum(v, "closed"), rnum(v, "reopened"), rnum(v, "replies"), rnum(v, "first_replies"), rnum(v, "resolutions"),
					rnum(ratings, "good"), rnum(ratings, "bad"), rnum(sla, "met"), rnum(sla, "breached")}
				exp := []int64{w.opened, w.closed, w.reopened, w.replies, int64(len(w.firsts)), int64(len(w.resolutions)), w.good, w.bad, w.met, w.breached}
				if !slices.Equal(got, exp) {
					t.Fatalf("%s %s %s: got %v, want %v (%s)", tz, viewer.name, p["start"], got, exp, mustMarshal(v))
				}
				if len(w.firsts) > 0 {
					within(t, tz+" "+p["start"].(string)+" median first reply", v["first_reply_median_seconds"], exactPercentile(w.firsts, 0.5))
				}
				if len(w.resolutions) > 0 {
					within(t, tz+" "+p["start"].(string)+" p90 resolution", v["resolution_p90_seconds"], exactPercentile(w.resolutions, 0.9))
				}
			}
			totals := r["totals"].(map[string]any)
			within(t, tz+" "+viewer.name+" median first reply", totals["first_reply_median_seconds"], exactPercentile(firsts, 0.5))
			within(t, tz+" "+viewer.name+" p90 first reply", totals["first_reply_p90_seconds"], exactPercentile(firsts, 0.9))
			if rnum(totals, "first_replies") != int64(len(firsts)) {
				t.Fatalf("%s %s totals %s", tz, viewer.name, mustMarshal(totals))
			}
		}
	}

	byMember := reportOf(t, tm.owner, "from="+localDay(convs[0].asked, time.UTC)+"&to="+localDay(now, time.UTC)+"&group_by=member&metrics=replies,first_reply")
	wantReplies := map[string]int64{}
	for _, c := range convs {
		if c.byAgent {
			wantReplies[tm.agentID]++
		} else {
			wantReplies[ownerID]++
		}
	}
	gotReplies := map[string]int64{}
	for _, g := range byMember["groups"].([]any) {
		gm := g.(map[string]any)
		key, _ := gm["key"].(string)
		gotReplies[key] = rnum(gm["totals"].(map[string]any), "replies")
		if _, ok := gm["totals"].(map[string]any)["opened"]; ok {
			t.Fatalf("opened was not asked for: %s", mustMarshal(gm))
		}
	}
	if gotReplies[tm.agentID] != wantReplies[tm.agentID] || gotReplies[ownerID] != wantReplies[ownerID] {
		t.Fatalf("replies per member %v, want %v", gotReplies, wantReplies)
	}

	byLabel := reportOf(t, tm.owner, "from="+localDay(convs[0].asked, time.UTC)+"&to="+localDay(now, time.UTC)+"&group_by=label&granularity=week")
	groups := byLabel["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["key"] != label || rnum(groups[0].(map[string]any)["totals"].(map[string]any), "opened") != 12 ||
		rnum(byLabel["totals"].(map[string]any), "opened") != 24 {
		t.Fatalf("by label %s", mustMarshal(byLabel))
	}
	var weekly int64
	for _, p := range byLabel["series"].([]any) {
		weekly += rnum(p.(map[string]any)["values"].(map[string]any), "opened")
	}
	if weekly != 24 {
		t.Fatalf("weeks add up to %d", weekly)
	}

	agentInboxes := reportOf(t, tm.agent, "from="+localDay(convs[0].asked, time.UTC)+"&to="+localDay(now, time.UTC)+"&group_by=inbox")
	if g := agentInboxes["groups"].([]any); len(g) != 1 || g[0].(map[string]any)["key"] != tm.inbox {
		t.Fatalf("agent's inboxes %s", mustMarshal(agentInboxes))
	}
	tm.agent.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/reports?from=2026-01-01&to=2026-01-02&inbox_id="+hidden, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/reports?from=2026-01-01&to=2026-01-02&inbox_id="+tm.inbox, nil)
	other.owner.expectProblem(http.StatusNotFound, "not_found", "GET", "/v1/reports?from=2026-01-01&to=2026-01-02&label_id="+label, nil)
	if more, err := h.srv.RollupReports(ctx, uuid.MustParse(other.ws)); err != nil || more {
		t.Fatalf("rollup: %v %v", more, err)
	}
	if o := reportOf(t, other.owner, "from="+localDay(convs[0].asked, time.UTC)+"&to="+localDay(now, time.UTC)); rnum(o["totals"].(map[string]any), "opened") != 3 {
		t.Fatalf("another workspace sees %s", mustMarshal(o["totals"]))
	}

	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/reports?from=2026-02-01&to=2026-01-01", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/reports?from=2025-01-01&to=2026-02-01", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/reports?from=2025-01-01&to=2025-02-01&timezone=Mars/Base", nil)
	tm.owner.expectProblem(http.StatusBadRequest, "validation_failed", "GET", "/v1/reports?from=2025-01-01&to=2025-02-01&metrics=visits", nil)
	tm.owner.expect(http.StatusOK, "GET", "/v1/reports?from=2025-01-01&to=2026-01-31", nil)

	reader := h.client()
	reader.bearer = tm.owner.expect(http.StatusCreated, "POST", "/v1/api-keys", map[string]any{"name": "r", "scopes": []string{"conversations:read"}}).str("secret")
	reader.expectProblem(http.StatusForbidden, "insufficient_scope", "GET", "/v1/reports?from=2026-01-01&to=2026-01-02", nil)
	key := tm.apiKey(h)
	key.expect(http.StatusOK, "GET", "/v1/reports?from=2026-01-01&to=2026-01-02", nil)
	out, errText := callTool(t, h.mcpSession(key.bearer), "get_report", map[string]any{
		"from": localDay(convs[0].asked, time.UTC), "to": localDay(now, time.UTC), "metrics": []string{"opened", "ratings"}, "group_by": "inbox",
	})
	if errText != "" || rnum(out["totals"].(map[string]any), "opened") != 24 || len(out["groups"].([]any)) != 2 {
		t.Fatalf("mcp get_report %v %s", out, errText)
	}
}

func TestReportTodayAgreesWithStats(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	key := tm.apiKey(h)
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Billing"}).str("id")
	seedReport(t, h, tm, hidden, label, 30)
	ws := uuid.MustParse(tm.ws)
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		conv := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact}).str("id")
		post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "help"})
		h.clock.Advance(time.Duration(i+1) * 7 * time.Minute)
		post(tm.agent, conv, map[string]any{"kind": "message", "body": "on it"})
		tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	}
	compare := func(stage string) {
		t.Helper()
		for _, tz := range []string{"UTC", "Asia/Kolkata", "America/Los_Angeles"} {
			loc, _ := time.LoadLocation(tz)
			today := localDay(h.clock.Now(), loc)
			for _, c := range []*client{tm.owner, tm.agent} {
				s := c.expect(http.StatusOK, "GET", "/v1/stats?timezone="+url.QueryEscape(tz), nil).body
				r := reportOf(t, c, "from="+today+"&to="+today+"&timezone="+url.QueryEscape(tz))
				tot := r["totals"].(map[string]any)
				ratings, sla := tot["ratings"].(map[string]any), tot["sla"].(map[string]any)
				var met, breached int64
				for _, k := range []string{"first_reply", "next_reply", "resolution"} {
					met += rnum(sla[k].(map[string]any), "met")
					breached += rnum(sla[k].(map[string]any), "breached")
				}
				sr, ss := s["ratings"].(map[string]any), s["sla"].(map[string]any)
				got := []int64{rnum(tot, "replies"), rnum(tot, "closed"), rnum(tot, "first_replies"), rnum(ratings, "good"), rnum(ratings, "bad"), met, breached}
				want := []int64{rnum(s, "replies"), rnum(s, "closed"), rnum(s, "first_replies"), rnum(sr, "good"), rnum(sr, "bad"), rnum(ss, "met"), rnum(ss, "breached")}
				if !slices.Equal(got, want) || got[0] == 0 {
					t.Fatalf("%s %s: report %v, stats %v", stage, tz, got, want)
				}
				if m, ok := s["median_first_reply_seconds"].(float64); ok {
					within(t, stage+" "+tz+" median", tot["first_reply_median_seconds"], m)
				}
				if r["built_until"] == nil {
					t.Fatalf("no built_until: %s", mustMarshal(r))
				}
			}
		}
	}
	compare("live tail")
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	compare("rolled up")

	res := tm.owner.expect(http.StatusOK, "GET", "/v1/reports/export?from="+localDay(h.clock.Now().Add(-7*24*time.Hour), time.UTC)+"&to="+localDay(h.clock.Now(), time.UTC)+
		"&group_by=member&metrics=replies,first_reply", nil)
	if ct := res.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") || !strings.Contains(res.header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("export headers %v", res.header)
	}
	records, err := csv.NewReader(strings.NewReader(string(res.raw))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(records[0], ","); got != "period,group_id,group_name,replies,first_replies,first_reply_median_seconds,first_reply_p90_seconds" {
		t.Fatalf("csv header %s", got)
	}
	if len(records) != 1+2*8 {
		t.Fatalf("csv has %d rows: %s", len(records), res.raw)
	}
	var replies int64
	names := map[string]bool{}
	for _, rec := range records[1:] {
		var n int64
		_, _ = fmt.Sscan(rec[3], &n)
		replies += n
		names[rec[2]] = true
	}
	if replies != 33 || len(names) != 2 {
		t.Fatalf("csv replies %d names %v", replies, names)
	}
}

func TestReportNightlyRebuildDropsTrashed(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	hidden := tm.owner.expect(http.StatusCreated, "POST", "/v1/inboxes", map[string]any{"name": "Private", "slug": "private"}).body["inbox"].(map[string]any)["id"].(string)
	label := tm.owner.expect(http.StatusCreated, "POST", "/v1/labels", map[string]any{"name": "Billing"}).str("id")
	convs := seedReport(t, h, tm, hidden, label, 6)
	ws := uuid.MustParse(tm.ws)
	opened := func() int64 {
		r := reportOf(t, tm.owner, "from="+localDay(convs[0].asked, time.UTC)+"&to="+localDay(h.clock.Now(), time.UTC)+"&metrics=opened")
		return rnum(r["totals"].(map[string]any), "opened")
	}
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	tm.owner.expect(http.StatusOK, "DELETE", "/v1/conversations/"+convs[1].id, nil)
	if got := opened(); got != 6 {
		t.Fatalf("before the nightly rebuild %d", got)
	}
	h.clock.Advance(24 * time.Hour)
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	if got := opened(); got != 5 {
		t.Fatalf("after the nightly rebuild %d", got)
	}
}

func todayCounts(t *testing.T, c *client, now time.Time) (report, stats []int64) {
	t.Helper()
	day := localDay(now, time.UTC)
	tot := reportOf(t, c, "from="+day+"&to="+day+"&metrics=replies,closed,first_reply")["totals"].(map[string]any)
	s := c.expect(http.StatusOK, "GET", "/v1/stats", nil).body
	return []int64{rnum(tot, "replies"), rnum(tot, "closed"), rnum(tot, "first_replies")},
		[]int64{rnum(s, "replies"), rnum(s, "closed"), rnum(s, "first_replies")}
}

func TestReportJobBetweenStateAndRowsCountsOnce(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	key := tm.apiKey(h)
	ws := uuid.MustParse(tm.ws)
	tm.owner.expect(http.StatusNoContent, "PUT", "/v1/inboxes/"+tm.inbox+"/members/"+tm.agentID, nil)
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	var first time.Time
	if err := h.st.Pool.QueryRow(ctx, "SELECT built_until FROM report_rollup_state WHERE workspace_id = $1", ws).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if !first.Equal(first.Truncate(15 * time.Minute)) {
		t.Fatalf("built_until %s is not on a quarter hour", first)
	}
	conv := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact}).str("id")
	post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "help"})
	post(tm.agent, conv, map[string]any{"kind": "message", "body": "on it"})
	tm.owner.expect(http.StatusOK, "PATCH", "/v1/conversations/"+conv, map[string]any{"status": "closed"})
	h.clock.Advance(20 * time.Minute)
	if _, err := h.srv.RollupReports(ctx, ws); err != nil {
		t.Fatal(err)
	}
	// A request that read the state before this run committed still holds the first built_until.
	if _, err := h.st.Pool.Exec(ctx, "UPDATE report_rollup_state SET built_until = $2 WHERE workspace_id = $1", ws, first); err != nil {
		t.Fatal(err)
	}
	if got, want := todayCounts(t, tm.owner, h.clock.Now()); !slices.Equal(got, want) || want[0] != 1 {
		t.Fatalf("report %v, stats %v", got, want)
	}
}

func TestReportBackfillStartsAtRepliesInImportedThreads(t *testing.T) {
	h := newHarness(t)
	tm := newTeam(t, h)
	ctx := context.Background()
	key := tm.apiKey(h)
	conv := key.expect(http.StatusCreated, "POST", "/v1/conversations", map[string]any{"inbox_id": tm.inbox, "contact_id": tm.contact}).str("id")
	asked := post(key, conv, map[string]any{"kind": "message", "direction": "in", "body": "help"})
	answered := post(tm.owner, conv, map[string]any{"kind": "message", "body": "on it"})
	now := h.clock.Now()
	day := 24 * time.Hour
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE conversations SET imported_at = $2, created_at = $3, last_activity_at = $4 WHERE id = $1", []any{conv, now, now.Add(-60 * day), now.Add(-50 * day)}},
		{"UPDATE messages SET imported_at = $2, created_at = $3 WHERE id = $1", []any{asked, now, now.Add(-60 * day)}},
		{"UPDATE messages SET created_at = $2 WHERE id = $1", []any{answered, now.Add(-50 * day)}},
		{"DELETE FROM messages WHERE conversation_id = $1 AND id NOT IN ($2, $3)", []any{conv, asked, answered}},
	} {
		if _, err := h.st.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.srv.RollupReports(ctx, uuid.MustParse(tm.ws)); err != nil {
		t.Fatal(err)
	}
	r := reportOf(t, tm.owner, "from="+localDay(now.Add(-61*day), time.UTC)+"&to="+localDay(now, time.UTC)+"&metrics=replies,opened")
	if tot := r["totals"].(map[string]any); rnum(tot, "replies") != 1 || rnum(tot, "opened") != 0 {
		t.Fatalf("a reply in an imported thread 50 days ago: %s", mustMarshal(tot))
	}
}

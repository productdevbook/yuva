package api

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/oapi-codegen/nullable"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	statusResponseSteps     = 96
	statusResponseStep      = 15 * time.Minute
	statusPageDays          = 60
	statusFilterIncidents   = "incidents"
	statusFilterMaintenance = "maintenance"
	yuvaSite                = "https://useyuva.com"
)

type responseSeries struct {
	from        time.Time
	step        time.Duration
	points      []int32
	median, p95 *int32
}

func (r *responseSeries) public() *oas.PublicStatusResponseTimes {
	out := &oas.PublicStatusResponseTimes{From: r.from.UTC(), StepSeconds: int32(r.step / time.Second), MedianMs: r.median, P95Ms: r.p95,
		Points: make([]nullable.Nullable[int32], len(r.points))}
	for i, p := range r.points {
		if p < 0 {
			out.Points[i] = nullable.NewNullNullable[int32]()
		} else {
			out.Points[i] = nullable.NewNullableWithValue(p)
		}
	}
	return out
}

// responseSeriesFor aggregates the measured successful http, tcp and push checks of the components into n steps from
// from; a step without a measured success is -1.
func responseSeriesFor(ctx context.Context, q *store.Queries, pg store.StatusPage, ids []uuid.UUID, from time.Time, step time.Duration, n int) (map[uuid.UUID]*responseSeries, error) {
	out := map[uuid.UUID]*responseSeries{}
	if len(ids) == 0 {
		return out, nil
	}
	until := from.Add(time.Duration(n) * step)
	for _, id := range ids {
		r := &responseSeries{from: from, step: step, points: make([]int32, n)}
		for i := range r.points {
			r.points[i] = -1
		}
		out[id] = r
	}
	buckets, err := q.StatusResponseBuckets(ctx, store.StatusResponseBucketsParams{Since: from, StepSeconds: int32(step / time.Second),
		WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ComponentIds: ids, Until: until})
	if err != nil {
		return nil, err
	}
	for _, b := range buckets {
		if r := out[b.ComponentID]; r != nil && b.Step >= 0 && int(b.Step) < n {
			r.points[b.Step] = b.P50
		}
	}
	totals, err := q.StatusResponseTotals(ctx, store.StatusResponseTotalsParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ComponentIds: ids, Since: from, Until: until})
	if err != nil {
		return nil, err
	}
	for _, t := range totals {
		if r := out[t.ComponentID]; r != nil {
			r.median, r.p95 = &t.P50, &t.P95
		}
	}
	return out, nil
}

func formatMs(v int32) string {
	if v < 1000 {
		return strconv.Itoa(int(v)) + " ms"
	}
	return strings.TrimSuffix(strconv.FormatFloat(float64(v)/1000, 'f', 1, 64), ".0") + " s"
}

type timeView struct{ ISO, Text string }

type updateView struct {
	State, Class string
	Body         template.HTML
	At           timeView
	Edited       bool
}

type incidentView struct {
	URL, Title, State, StateClass, Impact, ImpactClass, Affects string
	Maintenance, Closed                                         bool
	Start, End                                                  *timeView
	Updates                                                     []updateView
	When                                                        timeView
	Day, Kind, KindClass, Meta                                  string
	Kicker, Text                                                template.HTML
}

type componentView struct {
	Name, Description, State, Class, Uptime string
	Bars                                    template.HTML
}

type groupView struct {
	Name       string
	Components []componentView
}

type cellView struct {
	Label, Value string
	Time         *timeView
}

type legendView struct{ Class, Label string }

type langView struct {
	Code, Name string
	On         bool
}

type pickView struct{ ID, Name string }

type keepView struct{ Name, Value string }

type filterView struct {
	URL, Label string
	On         bool
}

type monthView struct {
	Index, Name, Uptime string
	Strip               template.HTML
	Items               []incidentView
}

type chartView struct {
	ID, Name, Stat, StatLabel, Sub, Label string
	Grid                                  *cellGrid
	Axis                                  []template.HTML
	Empty                                 bool
}

type responseView struct {
	W            statusWords
	Title, Num   string
	Charts       []chartView
	Tabs, Select bool
}

type affectedView struct{ Name, Text string }

type statusPageView struct {
	W                                         statusWords
	Dir, View, Name, Description, Logo        string
	LogoDark, Site, Support                   string
	Accent                                    template.CSS
	FontSans, FontMono                        string
	Home, Self, HistoryURL, FeedURL, BadgeURL string
	JSONURL, SubscribeURL, FeedPath, JSONPath string
	EmbedCode                                 string
	State, StateClass, Headline, OldestDay    string
	Open, Maint, Recent                       []incidentView
	Groups                                    []groupView
	ShowBars, ShowName, ShowFollow            bool
	Legend                                    []legendView
	Cells                                     []cellView
	Num                                       map[string]string
	Langs                                     []langView
	Keep                                      []keepView
	Picks                                     []pickView
	Response                                  *responseView
	Months                                    []monthView
	Filters                                   []filterView
	NoHistory                                 string
	Incident                                  *incidentView
	Affected                                  []affectedView
	Updated                                   timeView
	TimesIn                                   template.HTML
	Mark                                      template.HTML
	YuvaSite                                  string
	Script                                    template.JS
}

func (snap *statusSnap) at(t time.Time) timeView {
	return timeView{ISO: t.UTC().Format(time.RFC3339), Text: snap.words.moment(t.In(snap.loc))}
}

// link is a page address carrying the visitor's language when they picked one.
func (snap *statusSnap) link(path string, query ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(query); i += 2 {
		if query[i+1] != "" {
			q.Set(query[i], query[i+1])
		}
	}
	if snap.lang != "" {
		q.Set("lang", snap.lang)
	}
	if len(q) == 0 {
		return snap.url(path)
	}
	return snap.url(path) + "?" + q.Encode()
}

func timeHTML(t timeView, attr string) template.HTML {
	return template.HTML(`<time datetime="` + template.HTMLEscapeString(t.ISO) + `" ` + attr + `>` + template.HTMLEscapeString(t.Text) + `</time>`)
}

// fillHTML puts html where the format has %s and escapes the rest of the format.
func fillHTML(format string, html template.HTML) template.HTML {
	before, after, ok := strings.Cut(format, "%s")
	if !ok {
		return template.HTML(template.HTMLEscapeString(format))
	}
	return template.HTML(template.HTMLEscapeString(before)) + html + template.HTML(template.HTMLEscapeString(after))
}

func impactClass(impact string) string {
	switch impact {
	case "minor":
		return stDegraded
	case "major":
		return stPartialOutage
	case "critical":
		return stMajorOutage
	}
	return "none"
}

func updateClass(state string) string {
	switch state {
	case incResolved, incCompleted:
		return stOperational
	case incMonitoring, incScheduled, incInProgress:
		return stMaintenance
	}
	return stDegraded
}

func incidentStart(inc store.StatusIncident) time.Time {
	switch {
	case inc.Kind == kindMaintenance && inc.StartedAt != nil:
		return *inc.StartedAt
	case inc.ScheduledStart != nil:
		return *inc.ScheduledStart
	case inc.StartedAt != nil:
		return *inc.StartedAt
	}
	return *inc.PublishedAt
}

func incidentEnd(inc store.StatusIncident) *time.Time {
	if inc.ResolvedAt != nil {
		return inc.ResolvedAt
	}
	return nil
}

func (snap *statusSnap) incidentView(inc store.StatusIncident, parts incidentParts) incidentView {
	w := snap.words
	v := incidentView{URL: snap.link("/incidents/" + inc.ID.String()), Title: inc.Title, State: w.IncidentStates[inc.State], StateClass: updateClass(inc.State),
		Impact: fillS(w.ImpactOf, w.Impacts[inc.Impact]), ImpactClass: impactClass(inc.Impact), Maintenance: inc.Kind == kindMaintenance,
		Closed: incidentClosed(inc.State), When: snap.at(*inc.PublishedAt), Kind: w.IncidentKind, KindClass: "i-" + impactClass(inc.Impact)}
	if v.Maintenance {
		v.Kind, v.KindClass = w.Maintenance, "k-"+stMaintenance
	}
	if inc.ScheduledStart != nil {
		a, b := snap.at(*inc.ScheduledStart), snap.at(*inc.ScheduledEnd)
		v.Start, v.End = &a, &b
		v.When = a
	}
	v.Day = w.shortDay(incidentStart(inc).In(snap.loc))
	var names []string
	for _, c := range parts.components[inc.ID] {
		if i := slices.IndexFunc(snap.comps, func(x store.StatusComponent) bool { return x.ID == c.ComponentID }); i >= 0 {
			names = append(names, snap.comps[i].Name)
		}
	}
	v.Affects = strings.Join(names, ", ")
	for _, u := range parts.updates[inc.ID] {
		v.Updates = append(v.Updates, updateView{State: w.IncidentStates[u.State], Class: updateClass(u.State), Body: richStatusText(u.Body), At: snap.at(u.CreatedAt), Edited: u.EditedAt != nil})
	}
	if len(v.Updates) > 0 {
		v.Text = v.Updates[0].Body
	}
	meta := v.State
	if v.Closed {
		if end := incidentEnd(inc); end != nil && end.After(incidentStart(inc)) {
			meta = w.duration(end.Sub(incidentStart(inc)))
		}
	}
	if v.Affects != "" {
		meta += " · " + v.Affects
	}
	v.Meta = meta
	sep := template.HTML(" · ")
	head := template.HTML(template.HTMLEscapeString(v.Kind + " · " + v.State))
	switch {
	case v.Maintenance && inc.State == incScheduled && v.Start != nil:
		v.Kicker = head + sep + timeHTML(*v.Start, "data-local") + " – " + timeHTML(*v.End, "data-local")
	case v.Maintenance && v.End != nil:
		v.Kicker = head + sep + fillHTML(w.Until, timeHTML(*v.End, "data-local"))
	default:
		v.Kicker = head + sep + fillHTML(w.Since, timeHTML(snap.at(incidentStart(inc)), "data-local"))
	}
	return v
}

// pageDays joins the components' days into one per day: the worst state, and uptime over all of them.
func (snap *statusSnap) pageDays() []uptimeDay {
	var out []uptimeDay
	for _, c := range snap.comps {
		days := snap.days[c.ID]
		if out == nil {
			out = make([]uptimeDay, len(days))
			for i, d := range days {
				out[i] = uptimeDay{date: d.date, state: stOperational}
			}
		}
		for i, d := range days {
			if i >= len(out) || !d.known {
				continue
			}
			out[i].known = true
			out[i].state = worseState(out[i].state, d.state)
			out[i].counts += d.counts
			out[i].down += d.down
		}
	}
	return out
}

func lastDays(days []uptimeDay, n int) []uptimeDay {
	if len(days) > n {
		return days[len(days)-n:]
	}
	return days
}

func uptimeText(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.2f%%", *v)
}

func (snap *statusSnap) dayTitle(d uptimeDay) (string, string) {
	w := snap.words
	if !d.known {
		return "none", w.day(d.date) + ": " + w.NoData
	}
	title := w.day(d.date) + ": " + w.States[d.state]
	if u := d.uptime(); u != nil {
		title += " (" + uptimeText(u) + ")"
	}
	return d.state, title
}

func (snap *statusSnap) dayBars(days []uptimeDay, label string) template.HTML {
	g := newCellGrid(len(days), 1)
	for i, d := range days {
		cls, title := snap.dayTitle(d)
		g.addTitled("d-"+cls, i, 0, title)
	}
	return g.svg("bars", label)
}

func (snap *statusSnap) monthStrip(month time.Time, byDate map[string]uptimeDay, label string) template.HTML {
	n := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, snap.loc).Day()
	g := newCellGrid(31, 1)
	for i := range n {
		day := time.Date(month.Year(), month.Month(), i+1, 0, 0, 0, 0, snap.loc)
		d, ok := byDate[day.Format(time.DateOnly)]
		if !ok {
			d = uptimeDay{date: day}
		}
		cls, title := snap.dayTitle(d)
		g.addTitled("d-"+cls, i, 0, title)
	}
	return g.svg("strip", label)
}

// stepHold is how many empty steps between two measured ones the line bridges: a step can be
// shorter than the monitor's interval and then holds no check without anything being wrong.
func stepHold(interval, step time.Duration) int {
	return int(math.Ceil(1.5 * float64(interval) / float64(step)))
}

const (
	chartRows = 10
	chartCols = statusResponseSteps
)

// chartHot is how far above the median a value is drawn in the warn colour.
const chartHot = 2

// stepCells draws the points as a grid of squares, one column per point: a solid cell at the point's
// row, solid cells filling the step to the previous row, a faint wash below and empty cells above. A
// run of at most hold missing points between two measured ones keeps the last value; longer runs
// leave the column empty. Rows run from zero to three times the median, or to the peak when that is
// higher, so a flat line sits low and a spike climbs. Empty cells in columns band..bandEnd are tinted.
func stepCells(points []int32, hold, band, bandEnd int) (grid *cellGrid, empty bool) {
	points = slices.Clone(points)
	last := -1
	for i, p := range points {
		if p < 0 {
			continue
		}
		if last >= 0 && i-last-1 <= hold {
			for k := last + 1; k < i; k++ {
				points[k] = points[last]
			}
		}
		last = i
	}
	var measured []int32
	for _, p := range points {
		if p >= 0 {
			measured = append(measured, p)
		}
	}
	if len(measured) == 0 {
		return nil, true
	}
	slices.Sort(measured)
	median := float64(measured[len(measured)/2])
	top := max(float64(measured[len(measured)-1]), chartHot*1.5*median, 1)
	row := func(p int32) int {
		return chartRows - 3 - int(math.Round(float64(p)/top*(chartRows-3)))
	}
	hot := func(p int32) bool { return median > 0 && float64(p) >= chartHot*median }
	grid = newCellGrid(len(points), chartRows)
	for c, p := range points {
		prevRow, r := 0, 0
		if p >= 0 {
			r = row(p)
			prevRow = r
			if c > 0 && points[c-1] >= 0 {
				prevRow = row(points[c-1])
			}
		}
		lo, hi := min(r, prevRow), max(r, prevRow)
		for y := range chartRows {
			var k string
			switch {
			case p < 0 || y < lo:
				k = "off"
				if c >= band && c < bandEnd {
					k = "band"
				}
			case y <= hi:
				k = "on"
			default:
				k = "on w"
			}
			if p >= 0 && hot(p) && k != "off" && k != "band" {
				k = "hot" + k[2:]
			}
			grid.add(k, c, y)
		}
	}
	return grid, false
}

func (snap *statusSnap) componentName(id uuid.UUID) string {
	for _, c := range snap.comps {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

func newResponseView(w statusWords, title, num string, charts []chartView) *responseView {
	if len(charts) == 0 {
		return nil
	}
	return &responseView{W: w, Title: title, Num: num, Charts: charts, Tabs: len(charts) > 1 && len(charts) <= 3, Select: len(charts) > 3}
}

func (snap *statusSnap) pageResponse(num string) *responseView {
	w := snap.words
	var charts []chartView
	for i, id := range snap.monitored {
		r := snap.response[id]
		name := snap.componentName(id)
		c := chartView{ID: strconv.Itoa(i), Name: name, StatLabel: name + " · " + w.Median, Stat: "–", Label: name + ": " + w.ResponseTime}
		c.Grid, c.Empty = stepCells(r.points, stepHold(snap.intervals[id], r.step), 0, 0)
		if r.median != nil {
			c.Stat, c.Sub = formatMs(*r.median), fillS(w.P95, formatMs(*r.p95))
		}
		for _, h := range []int{24, 18, 12, 6} {
			c.Axis = append(c.Axis, template.HTML(template.HTMLEscapeString(fill1(w.HoursBack, h))))
		}
		c.Axis = append(c.Axis, template.HTML(template.HTMLEscapeString(w.Now)))
		charts = append(charts, c)
	}
	return newResponseView(w, w.ResponseTime, num, charts)
}

func sectionNumbers(names ...string) map[string]string {
	out := map[string]string{}
	for _, n := range names {
		if n != "" {
			out[n] = fmt.Sprintf("%02d", len(out)+1)
		}
	}
	return out
}

func (s *Server) statusView(snap *statusSnap, view string) statusPageView {
	pg, w := snap.page, snap.words
	state := string(snap.out.State)
	v := statusPageView{W: w, Dir: w.direction(), View: view, Name: pg.Name, Description: pg.Description, Home: snap.link(""),
		HistoryURL: snap.link("/history"), FeedURL: snap.url("/feed.atom"), BadgeURL: snap.url("/badge.svg"),
		JSONURL: s.auth.PublicURL + clientStatusPrefix + pg.Slug, State: w.States[state], StateClass: state,
		Headline: w.Headlines[state], Updated: snap.at(snap.now), Script: template.JS(statusScript),
		FontSans: snap.url(publicFontPath("Geist")), FontMono: snap.url(publicFontPath("GeistMono")),
		Mark: yuvaMark, YuvaSite: yuvaSite, ShowFollow: pg.ShowFollow}
	v.FeedPath, v.JSONPath = urlPath(v.FeedURL), clientStatusPrefix+pg.Slug
	v.EmbedCode = `<iframe src="` + snap.url("") + `" title="` + pg.Name + `" width="100%" height="640" style="border:0"></iframe>`
	v.TimesIn = fillHTML(w.TimesIn, template.HTML(`<span data-zone>`+template.HTMLEscapeString(pg.Timezone)+`</span>`))
	if pg.AccentColor != nil {
		v.Accent = template.CSS(*pg.AccentColor)
	}
	if s.statusEmailOpen(pg) {
		v.SubscribeURL = snap.url("/subscribe")
		if len(snap.comps) > 1 {
			for _, c := range snap.comps {
				v.Picks = append(v.Picks, pickView{ID: c.ID.String(), Name: c.Name})
			}
		}
	}
	if pg.LogoUrl != nil {
		v.Logo = *pg.LogoUrl
		if pg.LogoDarkUrl != nil {
			v.LogoDark = *pg.LogoDarkUrl
		}
	}
	v.ShowName = pg.ShowName || v.Logo == ""
	if pg.SiteUrl != nil {
		v.Site = *pg.SiteUrl
	}
	if pg.SupportUrl != nil {
		v.Support = *pg.SupportUrl
	}
	codes := make([]string, 0, len(statusLanguages))
	for code := range statusLanguages {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		v.Langs = append(v.Langs, langView{Code: code, Name: statusLanguages[code].Name, On: code == w.Lang})
	}
	switch view {
	case "history":
		v.Self = snap.url("/history")
		if snap.filter != "" {
			v.Keep = []keepView{{Name: "filter", Value: snap.filter}}
		}
	case "incident":
	default:
		v.Self = snap.url("")
	}
	recentFrom := snap.now.AddDate(0, 0, -statusRecentDays)
	for _, inc := range snap.incidents {
		switch {
		case !incidentClosed(inc.State) && inc.Kind == kindMaintenance:
			v.Maint = append(v.Maint, snap.incidentView(inc, snap.parts))
		case !incidentClosed(inc.State):
			v.Open = append(v.Open, snap.incidentView(inc, snap.parts))
		case inc.ResolvedAt != nil && inc.ResolvedAt.After(recentFrom):
			v.Recent = append(v.Recent, snap.incidentView(inc, snap.parts))
		}
	}
	v.ShowBars = pg.ShowUptime
	days := min(int(pg.HistoryDays), statusPageDays)
	v.OldestDay = fill1(w.DaysAgo, days-1)
	for _, st := range componentStates {
		v.Legend = append(v.Legend, legendView{Class: st, Label: w.States[st]})
	}
	group := func(id *uuid.UUID) []componentView {
		var out []componentView
		for _, c := range snap.comps {
			if (id == nil) != (c.GroupID == nil) || (id != nil && *id != *c.GroupID) {
				continue
			}
			cv := componentView{Name: c.Name, Description: c.Description, State: w.States[c.State], Class: c.State}
			if pg.ShowUptime {
				recent := lastDays(snap.days[c.ID], days)
				if u := uptimeOver(recent); u != nil {
					cv.Uptime = fillS(w.Uptime, uptimeText(u))
				}
				label := c.Name + ": " + fill1(w.DaysAgo, days-1) + " – " + w.Today
				if cv.Uptime != "" {
					label += ", " + cv.Uptime
				}
				cv.Bars = snap.dayBars(recent, label)
			}
			out = append(out, cv)
		}
		return out
	}
	if ungrouped := group(nil); len(ungrouped) > 0 {
		v.Groups = append(v.Groups, groupView{Components: ungrouped})
	}
	for _, g := range snap.groups {
		id := g.ID
		v.Groups = append(v.Groups, groupView{Name: g.Name, Components: group(&id)})
	}
	affected := 0
	for _, c := range snap.comps {
		if c.State != stOperational {
			affected++
		}
	}
	affectedText := w.AffectedNone
	switch {
	case affected == 1:
		affectedText = w.AffectedOne
	case affected > 1:
		affectedText = fill1(w.AffectedMany, affected)
	}
	all := snap.pageDays()
	updated := snap.at(snap.now)
	switch view {
	case "history":
		v.Months, v.Cells = snap.historyMonths(all)
		v.NoHistory = fill1(w.NoHistory, int(pg.HistoryDays))
		for _, f := range []struct{ key, label string }{{"", w.FilterAll}, {statusFilterIncidents, w.FilterIncidents}, {statusFilterMaintenance, w.Maintenance}} {
			v.Filters = append(v.Filters, filterView{URL: snap.link("/history", "filter", f.key), Label: f.label, On: f.key == snap.filter})
		}
	case "incident":
	default:
		if pg.ShowUptime {
			if u := uptimeOver(lastDays(all, days)); u != nil {
				v.Cells = append(v.Cells, cellView{Label: fill1(w.UptimeDays, days), Value: uptimeText(u)})
			}
		}
		v.Cells = append(v.Cells, cellView{Label: w.AffectedNow, Value: affectedText},
			cellView{Label: w.Updated, Value: w.JustNow, Time: &updated})
		response := ""
		if len(snap.monitored) > 0 {
			response = "response"
		}
		subscribe := ""
		if v.SubscribeURL != "" {
			subscribe = "subscribe"
		}
		follow := ""
		if v.ShowFollow {
			follow = "follow"
		}
		v.Num = sectionNumbers("components", response, "recent", subscribe, follow)
		if response != "" {
			v.Response = snap.pageResponse(v.Num["response"])
		}
	}
	return v
}

func urlPath(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		return u.Path
	}
	return raw
}

// historyMonths lists the months of the page's history days, newest first, with the incidents the
// filter keeps, and the cells above them.
func (snap *statusSnap) historyMonths(all []uptimeDay) ([]monthView, []cellView) {
	w, pg := snap.words, snap.page
	byDate := map[string]uptimeDay{}
	for _, d := range all {
		byDate[d.date.Format(time.DateOnly)] = d
	}
	local := snap.now.In(snap.loc)
	first := local.AddDate(0, 0, -int(pg.HistoryDays)+1)
	var months []monthView
	incidents, maintenance := 0, 0
	at := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, snap.loc)
	stop := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, snap.loc)
	for i := 0; !at.Before(stop); i++ {
		m := monthView{Index: fmt.Sprintf("%02d", i+1), Name: w.longMonth(at)}
		if pg.ShowUptime {
			var in []uptimeDay
			for _, d := range all {
				if d.date.Year() == at.Year() && d.date.Month() == at.Month() {
					in = append(in, d)
				}
			}
			m.Uptime = uptimeText(uptimeOver(in))
			m.Strip = snap.monthStrip(at, byDate, m.Name)
		}
		for _, inc := range snap.incidents {
			t := incidentStart(inc).In(snap.loc)
			if t.Year() != at.Year() || t.Month() != at.Month() {
				continue
			}
			if inc.Kind == kindMaintenance {
				maintenance++
				if snap.filter == statusFilterIncidents {
					continue
				}
			} else {
				incidents++
				if snap.filter == statusFilterMaintenance {
					continue
				}
			}
			m.Items = append(m.Items, snap.incidentView(inc, snap.parts))
		}
		months = append(months, m)
		at = at.AddDate(0, -1, 0)
	}
	var cells []cellView
	if pg.ShowUptime {
		if u := uptimeOver(all); u != nil {
			cells = append(cells, cellView{Label: fill1(w.UptimeDays, int(pg.HistoryDays)), Value: uptimeText(u)})
		}
	}
	cells = append(cells, cellView{Label: w.FilterIncidents, Value: strconv.Itoa(incidents)}, cellView{Label: w.Maintenance, Value: strconv.Itoa(maintenance)})
	return months, cells
}

// incidentDetail adds what only the incident page shows: its times, how its components fared and,
// when the page shows them, response times around it.
func (s *Server) incidentDetail(ctx context.Context, snap *statusSnap, v *statusPageView, inc store.StatusIncident, parts incidentParts) error {
	w := snap.words
	v.Self = snap.url("/incidents/" + inc.ID.String())
	start := incidentStart(inc)
	end := snap.now
	if e := incidentEnd(inc); e != nil {
		end = *e
	}
	startView := snap.at(start)
	v.Cells = []cellView{{Label: w.Started, Value: startView.Text, Time: &startView}}
	switch {
	case inc.State == incScheduled:
		if inc.ScheduledEnd != nil {
			ev := snap.at(*inc.ScheduledEnd)
			v.Cells = append(v.Cells, cellView{Label: w.Ended, Value: ev.Text, Time: &ev}, cellView{Label: w.Duration, Value: w.duration(inc.ScheduledEnd.Sub(start))})
		}
	case incidentEnd(inc) != nil:
		ev := snap.at(end)
		v.Cells = append(v.Cells, cellView{Label: w.Ended, Value: ev.Text, Time: &ev}, cellView{Label: w.Duration, Value: w.duration(end.Sub(start))})
	default:
		v.Cells = append(v.Cells, cellView{Label: w.Ended, Value: w.Ongoing}, cellView{Label: w.Duration, Value: w.duration(end.Sub(start))})
	}
	var ids []uuid.UUID
	for _, c := range parts.components[inc.ID] {
		if snap.visible[c.ComponentID] {
			ids = append(ids, c.ComponentID)
		}
	}
	if len(ids) > 0 && inc.State != incScheduled {
		spans, err := s.st.StatusComponentSpans(ctx, store.StatusComponentSpansParams{WorkspaceID: snap.page.WorkspaceID, ComponentIds: ids, Since: start})
		if err != nil {
			return err
		}
		for _, id := range ids {
			worst, off := stOperational, time.Duration(0)
			for _, sp := range spans {
				if sp.ComponentID != id {
					continue
				}
				spEnd := snap.now
				if sp.EndedAt != nil {
					spEnd = *sp.EndedAt
				}
				a, b := maxTime(sp.StartedAt, start), minTime(spEnd, end)
				if !b.After(a) || sp.State == stOperational {
					continue
				}
				worst = worseState(worst, sp.State)
				off += b.Sub(a)
			}
			after := ""
			for _, c := range snap.comps {
				if c.ID == id {
					after = c.State
				}
			}
			closed := incidentEnd(inc) != nil
			if closed {
				for _, sp := range spans {
					if sp.ComponentID == id && !sp.StartedAt.After(end) && (sp.EndedAt == nil || sp.EndedAt.After(end)) {
						after = sp.State
					}
				}
			}
			text := w.States[after]
			switch {
			case worst != stOperational && closed:
				text = fillS(fillS(fillS(w.AffectedThen, w.States[worst]), w.duration(off)), w.States[after])
			case worst != stOperational:
				text = fillS(fillS(w.AffectedSoFar, w.States[worst]), w.duration(off))
			}
			v.Affected = append(v.Affected, affectedView{Name: snap.componentName(id), Text: text})
		}
	} else {
		for _, id := range ids {
			v.Affected = append(v.Affected, affectedView{Name: snap.componentName(id)})
		}
	}
	stay := ""
	if v.SubscribeURL != "" || v.ShowFollow {
		stay = "stay"
	}
	response := ""
	if snap.page.ShowResponseTimes && inc.State != incScheduled {
		var monitored []uuid.UUID
		for _, id := range ids {
			if slices.Contains(snap.monitored, id) {
				monitored = append(monitored, id)
			}
		}
		pad := max(30*time.Minute, end.Sub(start)/6)
		from := start.Add(-pad)
		step := max(((end.Add(pad).Sub(from)+statusResponseSteps*time.Second-1)/(statusResponseSteps*time.Second))*time.Second, 30*time.Second)
		series, err := responseSeriesFor(ctx, s.st.Queries, snap.page, monitored, from, step, statusResponseSteps)
		if err != nil {
			return err
		}
		before, err := s.st.StatusResponseTotals(ctx, store.StatusResponseTotalsParams{WorkspaceID: snap.page.WorkspaceID, PageID: snap.page.ID,
			ComponentIds: monitored, Since: start.Add(-24 * time.Hour), Until: start})
		if err != nil {
			return err
		}
		hm := func(t time.Time, mark string) template.HTML {
			h := timeHTML(timeView{ISO: t.UTC().Format(time.RFC3339), Text: t.In(snap.loc).Format("15:04")}, "data-hm")
			if mark != "" {
				h += template.HTML(" " + template.HTMLEscapeString(mark))
			}
			return h
		}
		var charts []chartView
		for i, id := range monitored {
			r := series[id]
			name := snap.componentName(id)
			c := chartView{ID: strconv.Itoa(i), Name: name, StatLabel: name + " · " + w.Peak, Stat: "–", Label: name + ": " + w.ResponseDuring}
			bandFrom := max(int(start.Sub(from)/step), 0)
			bandTo := min(int((end.Sub(from)+step-1)/step), statusResponseSteps)
			c.Grid, c.Empty = stepCells(r.points, stepHold(snap.intervals[id], step), bandFrom, bandTo)
			if c.Empty {
				continue
			}
			peak := int32(-1)
			for j, p := range r.points {
				t := from.Add(time.Duration(j) * step)
				if !t.Add(step).Before(start) && !t.After(end) {
					peak = max(peak, p)
				}
			}
			if peak >= 0 {
				c.Stat = formatMs(peak)
			}
			for _, b := range before {
				if b.ComponentID == id {
					c.Sub = fillS(w.Usually, formatMs(b.P50))
				}
			}
			c.Axis = []template.HTML{hm(from, ""), hm(start, w.StartMark)}
			if incidentEnd(inc) != nil {
				c.Axis = append(c.Axis, hm(end, w.EndMark), hm(minTime(end.Add(pad), snap.now), ""))
			} else {
				c.Axis = append(c.Axis, template.HTML(template.HTMLEscapeString(w.Now)))
			}
			charts = append(charts, c)
		}
		if len(charts) > 0 {
			response = "response"
		}
		v.Num = sectionNumbers(response, "updates", "affected", stay)
		v.Response = newResponseView(w, w.ResponseDuring, v.Num["response"], charts)
		return nil
	}
	v.Num = sectionNumbers(response, "updates", "affected", stay)
	return nil
}

func renderStatus(v statusPageView) ([]byte, error) {
	var b bytes.Buffer
	if err := statusTemplate.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

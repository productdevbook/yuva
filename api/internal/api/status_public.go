package api

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	statusPathPrefix   = "/status/"
	clientStatusPrefix = "/client/v1/status/"
	statusCacheTTL     = 30 * time.Second
	statusMaxCached    = 5000
	statusRecentDays   = 14
	statusPublicMax    = 500
	statusFeedEntries  = 50
	statusCacheControl = "public, max-age=30, stale-if-error=86400"
)

var limitStatusPerIP = limitPublicPerIP

type statusCacheEntry = publicEntry

type statusCache = publicCache

func newStatusCache() *statusCache { return newPublicCache(statusCacheTTL, statusMaxCached) }

type statusHostKey struct{}

// statusOn is the context value of a request that came to the status host or to an own domain.
type statusOn struct {
	domain          bool
	host            string
	workspace, page uuid.UUID
}

// statusHost serves the status pages at /{slug} on YUVA_STATUS_URL's host and nothing else there,
// and a status page, help center, roadmap board or changelog at / on its verified own domain and
// nothing else there.
func (s *Server) statusHost(next http.Handler) http.Handler {
	var statusHost string
	if u, err := url.Parse(s.status.URL); s.status.URL != "" && err == nil {
		statusHost = u.Host
	}
	var publicHost string
	if u, err := url.Parse(s.auth.PublicURL); err == nil {
		publicHost = u.Host
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case statusHost != "" && strings.EqualFold(r.Host, statusHost):
			if s.serveStatusRoot(w, r) {
				return
			}
			if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, statusPathPrefix) || strings.Count(r.URL.Path, "/") > 3 {
				writeProblem(w, errNotFound)
				return
			}
			r2 := r.Clone(context.WithValue(r.Context(), statusHostKey{}, statusOn{}))
			r2.URL.Path = strings.TrimSuffix(statusPathPrefix, "/") + r.URL.Path
			r2.URL.RawPath = ""
			next.ServeHTTP(w, r2)
		case s.statusDomainsOn() && !strings.EqualFold(r.Host, publicHost):
			host := strings.ToLower(r.Host)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.TrimSuffix(host, ".")
			hit, ok := s.domainPage(r.Context(), host)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if hit.kind != kindStatus {
				s.serveOwnDomain(w, r, next, host, hit)
				return
			}
			path, ok := statusDomainPath(hit.slug, r.URL.Path)
			if !ok {
				writeProblem(w, errNotFound)
				return
			}
			r2 := r.Clone(context.WithValue(r.Context(), statusHostKey{}, statusOn{domain: true, host: host, workspace: hit.workspace, page: hit.page}))
			r2.URL.Path = path
			r2.URL.RawPath = ""
			next.ServeHTTP(w, r2)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) routeStatus(mux *http.ServeMux) {
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}", s.serveStatus("page"))
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/history", s.serveStatus("history"))
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/incidents/{id}", s.serveStatus("incident"))
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/feed.atom", s.serveStatus("feed"))
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/badge.svg", s.serveStatus("badge"))
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/fonts/{name}", servePublicFont)
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/subscribe", s.serveStatusSubscribe)
	mux.HandleFunc("POST "+statusPathPrefix+"{slug}/subscribe", s.serveStatusSubscribe)
	mux.HandleFunc("GET "+statusPathPrefix+"{slug}/subscribe/sent", s.serveStatusSubscribe)
}

func (s *Server) publicStatusPage(w http.ResponseWriter, r *http.Request) (store.StatusPage, bool) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	redirect := http.StatusFound
	if r.Method != http.MethodGet {
		redirect = http.StatusTemporaryRedirect
	}
	if s.status.URL != "" && ctx.Value(statusHostKey{}) == nil {
		target := s.statusURL(slug, strings.TrimPrefix(r.URL.Path, statusPathPrefix+slug))
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, redirect)
		return store.StatusPage{}, false
	}
	if err := s.rateLimit(ctx, rateCheck{"status:ip:" + rateIP(s.clientIP(r)), limitStatusPerIP}); err != nil {
		s.writeError(w, r, err)
		return store.StatusPage{}, false
	}
	var pg store.StatusPage
	var err error
	moved := false
	if on, _ := ctx.Value(statusHostKey{}).(statusOn); on.domain {
		pg, err = s.st.GetStatusPage(ctx, store.GetStatusPageParams{WorkspaceID: on.workspace, ID: on.page})
		moved = err == nil && (pg.CustomDomain == nil || *pg.CustomDomain != on.host || *pg.DomainState != domainVerified)
	} else if pg, err = s.st.FindStatusPageBySlug(ctx, slug); store.IsNotFound(err) {
		pg, err = s.st.FindStatusPageByFormerSlug(ctx, slug)
		if err == nil && pg.Published {
			target := s.pageURL(pg, strings.TrimPrefix(r.URL.Path, statusPathPrefix+slug))
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			if redirect == http.StatusFound {
				redirect = http.StatusMovedPermanently
			} else {
				redirect = http.StatusPermanentRedirect
			}
			http.Redirect(w, r, target, redirect)
			return pg, false
		}
	}
	if moved || store.IsNotFound(err) || (err == nil && !pg.Published) {
		s.statusGone(w)
		return pg, false
	}
	if err != nil {
		s.log.ErrorContext(ctx, "status page", slog.Any("error", err))
		writeProblem(w, errInternal)
		return pg, false
	}
	return pg, true
}

func (s *Server) serveStatus(view string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		pg, ok := s.publicStatusPage(w, r)
		if !ok {
			return
		}
		req := statusRequestFor(r, pg, view)
		if view == "incident" {
			var err error
			if req.incident, err = uuid.Parse(r.PathValue("id")); err != nil {
				s.statusGone(w)
				return
			}
		}
		e, err := s.statusAnswer(ctx, pg, req)
		if err == errStatusIncidentGone {
			s.statusGone(w)
			return
		}
		if err != nil {
			s.log.ErrorContext(ctx, "status page", slog.Any("error", err))
			writeProblem(w, errInternal)
			return
		}
		writeStatusAnswer(w, e)
	}
}

func (s *Server) statusGone(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(statusWordsFor("en").Gone + "\n"))
}

var (
	statusScript = `var L=document.documentElement.lang;` +
		`document.querySelectorAll("time[data-local]").forEach(function(t){var d=new Date(t.getAttribute("datetime"));if(!isNaN(d))t.textContent=d.toLocaleString(L,{dateStyle:"medium",timeStyle:"short"})});` +
		`document.querySelectorAll("time[data-hm]").forEach(function(t){var d=new Date(t.getAttribute("datetime"));if(!isNaN(d))t.textContent=d.toLocaleTimeString(L,{hour:"2-digit",minute:"2-digit"})});` +
		`try{var z=Intl.DateTimeFormat().resolvedOptions().timeZone;if(z)document.querySelectorAll("[data-zone]").forEach(function(e){e.textContent=z})}catch(e){}` +
		`document.querySelectorAll("select[data-rt]").forEach(function(s){var c=document.querySelectorAll("[data-rt-chart]");function f(){c.forEach(function(e){e.hidden=e.getAttribute("data-rt-chart")!==s.value})}s.closest("label").hidden=false;s.addEventListener("change",f);f()});` +
		`document.querySelectorAll("select[data-lang]").forEach(function(s){var b=s.form.querySelector("button");if(b)b.hidden=true;s.addEventListener("change",function(){s.form.submit()})});` +
		`setTimeout(function(){location.reload()},60000)`
	statusPageCSP = publicCSP{img: "'self' https: data:", script: "", scriptHash: publicScriptHash(statusScript), frameAncestors: "*", formAction: "'self'"}.String()
)

func writeStatusAnswer(w http.ResponseWriter, e statusCacheEntry) {
	h := w.Header()
	h.Set("Content-Type", e.ctype)
	h.Set("Cache-Control", statusCacheControl)
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	switch {
	case strings.HasPrefix(e.ctype, "text/html"):
		h.Set("Content-Security-Policy", statusPageCSP)
	case strings.HasPrefix(e.ctype, "application/json"):
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	default:
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(e.body)
}

// statusRequest is what a public answer depends on besides the page: the view, the incident, the
// language a visitor picked with ?lang= ("" for the page's own), the history filter and the own
// domain the request came in on ("" elsewhere).
type statusRequest struct {
	view     string
	incident uuid.UUID
	lang     string
	filter   string
	host     string
}

func statusRequestFor(r *http.Request, pg store.StatusPage, view string) statusRequest {
	req := statusRequest{view: view}
	if on, _ := r.Context().Value(statusHostKey{}).(statusOn); on.domain {
		req.host = on.host
	}
	q := r.URL.Query()
	if code := q.Get("lang"); code != "" {
		if w, ok := statusLanguage(code); ok && w.Lang != statusWordsFor(pg.Locale).Lang {
			req.lang = w.Lang
		}
	}
	if f := q.Get("filter"); view == "history" && (f == statusFilterIncidents || f == statusFilterMaintenance) {
		req.filter = f
	}
	return req
}

func (s *Server) statusAnswer(ctx context.Context, pg store.StatusPage, req statusRequest) (statusCacheEntry, error) {
	now := s.now()
	view := req.view
	key := pg.ID.String() + "|" + view + "|" + req.incident.String() + "|" + statusWordsFor(pg.Locale).Lang + "|" + req.lang + "|" + req.filter + "|" + req.host
	if e, ok := s.statusCache.get(key, pg.ChangedAt, now); ok {
		return e, nil
	}
	var inc store.StatusIncident
	if view == "incident" {
		var err error
		inc, err = s.st.GetStatusIncident(ctx, store.GetStatusIncidentParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: req.incident})
		if store.IsNotFound(err) || (err == nil && inc.PublishedAt == nil) {
			return statusCacheEntry{}, errStatusIncidentGone
		}
		if err != nil {
			return statusCacheEntry{}, err
		}
	}
	snap, err := s.statusSnapshot(ctx, s.st.Queries, pg, now)
	if err != nil {
		return statusCacheEntry{}, err
	}
	if req.host != "" {
		snap.url = func(path string) string { return s.domainURL(req.host, path) }
		snap.out = snap.json()
	}
	if req.lang != "" {
		snap.words, snap.lang = statusWordsFor(req.lang), req.lang
	}
	snap.filter = req.filter
	e := statusCacheEntry{changed: pg.ChangedAt, at: now}
	switch view {
	case "json":
		e.ctype = "application/json"
		e.body, err = json.Marshal(snap.out)
	case "feed":
		e.ctype = "application/atom+xml; charset=utf-8"
		e.body, err = s.statusFeed(snap)
	case "badge":
		e.ctype = "image/svg+xml"
		e.body = statusBadge(snap)
	case "incident":
		var parts incidentParts
		if parts, err = loadIncidentParts(ctx, s.st.Queries, pg.WorkspaceID, []store.StatusIncident{inc}); err != nil {
			return e, err
		}
		v := s.statusView(snap, "incident")
		iv := snap.incidentView(inc, parts)
		v.Incident = &iv
		if err = s.incidentDetail(ctx, snap, &v, inc, parts); err != nil {
			return e, err
		}
		e.ctype = "text/html; charset=utf-8"
		e.body, err = renderStatus(v)
	default:
		e.ctype = "text/html; charset=utf-8"
		e.body, err = renderStatus(s.statusView(snap, view))
	}
	if err != nil {
		return e, err
	}
	s.statusCache.put(key, e)
	return e, nil
}

type statusSnap struct {
	page      store.StatusPage
	words     statusWords
	loc       *time.Location
	now       time.Time
	out       oas.PublicStatusPage
	groups    []store.StatusComponentGroup
	comps     []store.StatusComponent
	days      map[uuid.UUID][]uptimeDay
	uptime    map[uuid.UUID]*float64
	incidents []store.StatusIncident
	parts     incidentParts
	visible   map[uuid.UUID]bool
	url       func(path string) string
	response  map[uuid.UUID]*responseSeries
	monitored []uuid.UUID
	intervals map[uuid.UUID]time.Duration
	lang      string
	filter    string
}

func (s *Server) statusSnapshot(ctx context.Context, q *store.Queries, pg store.StatusPage, now time.Time) (*statusSnap, error) {
	ws := pg.WorkspaceID
	snap := &statusSnap{page: pg, words: statusWordsFor(pg.Locale), loc: propertyLocation(pg.Timezone), now: now,
		days: map[uuid.UUID][]uptimeDay{}, uptime: map[uuid.UUID]*float64{}, visible: map[uuid.UUID]bool{},
		url: func(path string) string { return s.pageURL(pg, path) }}
	groups, err := q.ListStatusGroups(ctx, store.ListStatusGroupsParams{WorkspaceID: ws, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	all, err := q.ListStatusComponents(ctx, store.ListStatusComponentsParams{WorkspaceID: ws, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, c := range all {
		if !c.Hidden {
			snap.comps = append(snap.comps, c)
			snap.visible[c.ID] = true
			ids = append(ids, c.ID)
		}
	}
	for _, g := range groups {
		if slices.ContainsFunc(snap.comps, func(c store.StatusComponent) bool { return c.GroupID != nil && *c.GroupID == g.ID }) {
			snap.groups = append(snap.groups, g)
		}
	}
	window := now.AddDate(0, 0, -int(pg.HistoryDays)-1)
	if pg.ShowUptime && len(ids) > 0 {
		spans, err := q.StatusComponentSpans(ctx, store.StatusComponentSpansParams{WorkspaceID: ws, ComponentIds: ids, Since: window})
		if err != nil {
			return nil, err
		}
		by := map[uuid.UUID][]stateSpan{}
		for _, sp := range spans {
			var end time.Time
			if sp.EndedAt != nil {
				end = *sp.EndedAt
			}
			by[sp.ComponentID] = append(by[sp.ComponentID], stateSpan{state: sp.State, start: sp.StartedAt, end: end})
		}
		for _, id := range ids {
			snap.days[id] = uptimeDays(by[id], int(pg.HistoryDays), snap.loc, now)
			snap.uptime[id] = uptimeOver(snap.days[id])
		}
	}
	if pg.ShowResponseTimes && len(ids) > 0 {
		from := now.Truncate(statusResponseStep).Add(statusResponseStep - statusResponseSteps*statusResponseStep)
		rows, err := q.StatusResponseComponents(ctx, store.StatusResponseComponentsParams{WorkspaceID: ws, PageID: pg.ID, ComponentIds: ids, Since: from})
		if err != nil {
			return nil, err
		}
		snap.intervals = map[uuid.UUID]time.Duration{}
		for _, r := range rows {
			snap.intervals[r.ComponentID] = time.Duration(r.IntervalSeconds) * time.Second
		}
		for _, id := range ids {
			if _, ok := snap.intervals[id]; ok {
				snap.monitored = append(snap.monitored, id)
			}
		}
		if snap.response, err = responseSeriesFor(ctx, q, pg, snap.monitored, from, statusResponseStep, statusResponseSteps); err != nil {
			return nil, err
		}
	}
	if snap.incidents, err = q.ListPublicStatusIncidents(ctx, store.ListPublicStatusIncidentsParams{WorkspaceID: ws, PageID: pg.ID, Since: window, Lim: statusPublicMax}); err != nil {
		return nil, err
	}
	if snap.parts, err = loadIncidentParts(ctx, q, ws, snap.incidents); err != nil {
		return nil, err
	}
	snap.out = snap.json()
	return snap, nil
}

func (snap *statusSnap) publicIncident(inc store.StatusIncident, parts incidentParts) oas.PublicStatusIncident {
	out := oas.PublicStatusIncident{Id: inc.ID, Kind: oas.StatusIncidentKind(inc.Kind), Title: inc.Title, Impact: oas.StatusIncidentImpact(inc.Impact),
		State: oas.StatusIncidentState(inc.State), Components: []uuid.UUID{}, Updates: []oas.PublicStatusUpdate{},
		Url: snap.url("/incidents/" + inc.ID.String()), PublishedAt: *inc.PublishedAt, ScheduledStart: inc.ScheduledStart,
		ScheduledEnd: inc.ScheduledEnd, ResolvedAt: inc.ResolvedAt, UpdatedAt: inc.UpdatedAt}
	for _, c := range parts.components[inc.ID] {
		if snap.visible[c.ComponentID] {
			out.Components = append(out.Components, c.ComponentID)
		}
	}
	for _, u := range parts.updates[inc.ID] {
		out.Updates = append(out.Updates, oas.PublicStatusUpdate{State: oas.StatusIncidentState(u.State), Body: u.Body, CreatedAt: u.CreatedAt, EditedAt: u.EditedAt})
	}
	return out
}

func (snap *statusSnap) json() oas.PublicStatusPage {
	pg := snap.page
	out := oas.PublicStatusPage{Name: pg.Name, Description: pg.Description, Url: snap.url(""), LogoUrl: pg.LogoUrl, AccentColor: pg.AccentColor,
		LogoDarkUrl: pg.LogoDarkUrl, SiteUrl: pg.SiteUrl, SupportUrl: pg.SupportUrl, Locale: pg.Locale, Timezone: pg.Timezone, State: oas.StatusComponentStateValue(pageState(snap.comps)),
		Groups: []oas.PublicStatusGroup{}, Components: []oas.PublicStatusComponent{}, Open: []oas.PublicStatusIncident{}, Recent: []oas.PublicStatusIncident{},
		UpdatedAt: pg.ChangedAt}
	for _, g := range snap.groups {
		out.Groups = append(out.Groups, oas.PublicStatusGroup{Id: g.ID, Name: g.Name})
	}
	for _, c := range snap.comps {
		pc := oas.PublicStatusComponent{Id: c.ID, Name: c.Name, Description: c.Description, GroupId: c.GroupID, State: oas.StatusComponentStateValue(c.State)}
		if pg.ShowUptime {
			pc.Uptime = snap.uptime[c.ID]
			days := []oas.PublicStatusDay{}
			for _, d := range snap.days[c.ID] {
				st := d.state
				if !d.known {
					continue
				}
				days = append(days, oas.PublicStatusDay{Date: openapi_types.Date{Time: d.date}, Uptime: d.uptime(), State: oas.StatusComponentStateValue(st)})
			}
			pc.Days = &days
		}
		if r := snap.response[c.ID]; r != nil {
			pc.ResponseTimes = r.public()
		}
		out.Components = append(out.Components, pc)
	}
	recentFrom := snap.now.AddDate(0, 0, -statusRecentDays)
	for _, inc := range snap.incidents {
		switch {
		case !incidentClosed(inc.State):
			out.Open = append(out.Open, snap.publicIncident(inc, snap.parts))
		case inc.ResolvedAt != nil && inc.ResolvedAt.After(recentFrom):
			out.Recent = append(out.Recent, snap.publicIncident(inc, snap.parts))
		}
	}
	slices.SortStableFunc(out.Open, func(a, b oas.PublicStatusIncident) int {
		if (a.State == oas.StatusScheduled) != (b.State == oas.StatusScheduled) {
			if a.State == oas.StatusScheduled {
				return 1
			}
			return -1
		}
		return 0
	})
	return out
}

func (s *Server) GetClientStatusPage(ctx context.Context, req oas.GetClientStatusPageRequestObject) (oas.GetClientStatusPageResponseObject, error) {
	r := requestFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"status:ip:" + rateIP(s.clientIP(r)), limitStatusPerIP}); err != nil {
		return nil, err
	}
	pg, err := s.st.FindStatusPageBySlug(ctx, req.Slug)
	if store.IsNotFound(err) {
		pg, err = s.st.FindStatusPageByFormerSlug(ctx, req.Slug)
		if err == nil && pg.Published {
			target := s.pageURL(pg, "")
			return oas.GetClientStatusPage301Response{Headers: oas.GetClientStatusPage301ResponseHeaders{Location: &target}}, nil
		}
	}
	if store.IsNotFound(err) || (err == nil && !pg.Published) {
		return nil, errStatusPageGone
	}
	if err != nil {
		return nil, err
	}
	e, err := s.statusAnswer(ctx, pg, statusRequest{view: "json"})
	if err != nil {
		return nil, err
	}
	return statusAnswerResponse(e), nil
}

type statusAnswerResponse statusCacheEntry

func (a statusAnswerResponse) VisitGetClientStatusPageResponse(w http.ResponseWriter) error {
	writeStatusAnswer(w, statusCacheEntry(a))
	return nil
}

type statusPreviewResponse []byte

func (b statusPreviewResponse) VisitPreviewStatusPageResponse(w http.ResponseWriter) error {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", strings.NewReplacer("frame-ancestors *", "frame-ancestors 'self'", "font-src 'self'", "font-src 'self' https:").Replace(statusPageCSP))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(b)
	return err
}

func (s *Server) PreviewStatusPage(ctx context.Context, req oas.PreviewStatusPageRequestObject) (oas.PreviewStatusPageResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	snap, err := s.statusSnapshot(ctx, s.st.Queries, pg, s.now())
	if err != nil {
		return nil, err
	}
	body, err := renderStatus(s.statusView(snap, "page"))
	if err != nil {
		return nil, err
	}
	return statusPreviewResponse(body), nil
}

var statusColors = map[string]string{stOperational: "#16a34a", stDegraded: "#ca8a04", stPartialOutage: "#ea580c", stMajorOutage: "#dc2626", stMaintenance: "#2563eb", "": "#9ca3af"}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomText struct {
	Type string `xml:"type,attr,omitempty"`
	Body string `xml:",chardata"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

type atomEntry struct {
	ID         string         `xml:"id"`
	Title      string         `xml:"title"`
	Updated    string         `xml:"updated"`
	Published  string         `xml:"published"`
	Link       atomLink       `xml:"link"`
	Categories []atomCategory `xml:"category"`
	Content    atomText       `xml:"content"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

func (s *Server) statusFeed(snap *statusSnap) ([]byte, error) {
	w := snap.words
	feed := atomFeed{ID: snap.url(""), Title: snap.page.Name, Updated: snap.page.ChangedAt.UTC().Format(time.RFC3339),
		Links: []atomLink{{Href: snap.url(""), Rel: "alternate"}, {Href: snap.url("/feed.atom"), Rel: "self"}}}
	incs := slices.Clone(snap.incidents)
	slices.SortStableFunc(incs, func(a, b store.StatusIncident) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	for i, inc := range incs {
		if i == statusFeedEntries {
			break
		}
		var content strings.Builder
		for _, u := range snap.parts.updates[inc.ID] {
			content.WriteString("<p><b>" + template.HTMLEscapeString(w.IncidentStates[u.State]) + "</b> " +
				template.HTMLEscapeString(w.moment(u.CreatedAt.In(snap.loc))))
			if u.Body != "" {
				content.WriteString("<br>" + string(richStatusText(u.Body)))
			}
			content.WriteString("</p>")
		}
		url := snap.url("/incidents/" + inc.ID.String())
		feed.Entries = append(feed.Entries, atomEntry{ID: "urn:uuid:" + inc.ID.String(), Title: inc.Title + " – " + w.IncidentStates[inc.State],
			Updated: inc.UpdatedAt.UTC().Format(time.RFC3339), Published: inc.PublishedAt.UTC().Format(time.RFC3339),
			Link: atomLink{Href: url}, Content: atomText{Type: "html", Body: content.String()}})
	}
	return marshalAtom(feed)
}

func marshalAtom(feed atomFeed) ([]byte, error) {
	out, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), out...), nil
}

func statusBadge(snap *statusSnap) []byte {
	state := string(snap.out.State)
	label, value := "status", snap.words.States[state]
	lw, vw := 6*len([]rune(label))+12, 7*len([]rune(value))+12
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`, lw+vw,
		template.HTMLEscapeString(label), template.HTMLEscapeString(value))
	fmt.Fprintf(&b, `<rect width="%d" height="20" rx="3" fill="#555"/><rect x="%d" width="%d" height="20" rx="3" fill="%s"/><rect x="%d" width="4" height="20" fill="%s"/>`,
		lw+vw, lw, vw, statusColors[state], lw, statusColors[state])
	fmt.Fprintf(&b, `<g fill="#fff" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11"><text x="%d" y="14">%s</text><text x="%d" y="14">%s</text></g></svg>`,
		6, template.HTMLEscapeString(label), lw+6, template.HTMLEscapeString(value))
	return b.Bytes()
}

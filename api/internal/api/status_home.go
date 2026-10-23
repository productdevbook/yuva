package api

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	statusHomeUpdated = "2026-10-19"
	statusHomeCSP     = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; font-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	statusHomeSlugs   = 5
)

//go:embed status_home_og.png
var statusHomeOG []byte

type homeAlt struct{ Hreflang, Href string }

type homeLangLink struct {
	Code, Name, Href string
	On               bool
}

type homeFeature struct{ N, Title, Text string }

type homeStep struct {
	Time, Title, Text, Class string
}

type homeConv struct{ Who, Text, Via string }

type homeReach struct{ N, Label string }

type homeCardRow struct {
	Name, Uptime, Class, State string
	Bars                       template.HTML
}

type homeCard struct {
	Host, URL, Kicker, Title, Class, Oldest string
	Rows                                    []homeCardRow
	HasBars                                 bool
}

type homeView struct {
	W                                       homeWords
	HomePath                                string
	Lang, Dir, Canonical, Origin            string
	Alternates                              []homeAlt
	Locales                                 []homeLangLink
	LocaleName, OGLocale                    string
	OGAlternates                            []string
	FontSans, FontMono, OGImage, Favicon    string
	CreateURL, SignInURL, DocsURL, GuideURL string
	GitHubURL, SiteURL, Snippet             string
	Card                                    *homeCard
	Facts                                   []homeFact
	Features                                []homeFeature
	Steps                                   []homeStep
	Reach                                   []homeReach
	Convs                                   []homeConv
	Band                                    template.HTML
	Mark                                    template.HTML
	LD                                      template.HTML
	Style                                   template.CSS
	Chart                                   template.HTML
	Globe                                   template.HTML
}

func (s *Server) statusOrigin() string { return strings.TrimRight(s.status.URL, "/") }

func homeURL(origin string, l homeLocale) string {
	if l.Code == "en" {
		return origin + "/"
	}
	return origin + l.path()
}

func (s *Server) homeAlternates(origin string) []homeAlt {
	out := make([]homeAlt, 0, len(homeLocales)+1)
	for _, l := range homeLocales {
		out = append(out, homeAlt{l.Code, homeURL(origin, l)})
	}
	return append(out, homeAlt{"x-default", origin + "/"})
}

func sampleStrip(days int, marks map[int]string) template.HTML {
	g := newCellGrid(days/2, 2)
	for i := range days {
		st := marks[i]
		if st == "" {
			st = stOperational
		}
		g.add("d-"+st, i%(days/2), i/(days/2))
	}
	return g.svg("strip", "")
}

func sampleChart(label string) template.HTML {
	points := make([]int32, chartCols)
	for i := range points {
		points[i] = 120
	}
	for _, seg := range []struct {
		from, to int
		ms       int32
	}{
		{17, 23, 300}, {24, 27, 650}, {28, 32, 1100}, {33, 36, 1500}, {37, 42, 1400},
		{43, 50, 900}, {51, 56, 500}, {57, 60, 250},
	} {
		for i := seg.from; i <= seg.to; i++ {
			points[i] = seg.ms
		}
	}
	g, _ := stepCells(points, 0, 30, 66)
	return g.svg("chart", label)
}

func (s *Server) homeCard(ctx context.Context, words statusWords) (*homeCard, error) {
	if s.status.HomePage == "" {
		return nil, nil
	}
	pg, err := s.st.FindStatusPageBySlug(ctx, s.status.HomePage)
	if err != nil && !store.IsNotFound(err) {
		return nil, err
	}
	if err != nil || !pg.Published {
		return nil, nil
	}
	snap, err := s.statusSnapshot(ctx, s.st.Queries, pg, s.now())
	if err != nil {
		return nil, err
	}
	snap.words = words
	state := string(snap.out.State)
	pageURL := s.pageURL(pg, "")
	card := &homeCard{URL: pageURL, Host: strings.TrimPrefix(strings.TrimPrefix(pageURL, "https://"), "http://"),
		Kicker: words.Headlines[state], Title: pg.Name, Class: state}
	days := min(int(pg.HistoryDays), statusPageDays)
	card.Oldest = fill1(words.DaysAgo, days-1)
	for i, c := range snap.comps {
		if i == statusHomeSlugs {
			break
		}
		row := homeCardRow{Name: c.Name, Class: c.State, State: words.States[c.State]}
		if pg.ShowUptime {
			recent := lastDays(snap.days[c.ID], days)
			row.Uptime = uptimeText(uptimeOver(recent))
			row.Bars = snap.dayBars(recent, c.Name+": "+fill1(words.DaysAgo, days-1)+" – "+words.Today+", "+row.Uptime)
		}
		card.HasBars = card.HasBars || row.Bars != ""
		card.Rows = append(card.Rows, row)
	}
	return card, nil
}

func (s *Server) homeView(ctx context.Context, l homeLocale) (homeView, error) {
	origin := s.statusOrigin()
	w := homeWordsFor(l.Code)
	panel := strings.TrimRight(s.auth.PublicURL, "/")
	v := homeView{W: w, Lang: l.Code, Dir: l.dir(), Origin: origin, Canonical: homeURL(origin, l), Alternates: s.homeAlternates(origin),
		LocaleName: l.Name, OGLocale: l.OG, FontSans: publicFontPath("Geist"), FontMono: publicFontPath("GeistMono"),
		OGImage: origin + "/og.png", Favicon: "/favicon.svg", CreateURL: panel + "/status", SignInURL: panel + "/sign-in",
		DocsURL: docsBase, GuideURL: docsBase + "status-pages/", GitHubURL: "https://github.com/productdevbook/yuva", SiteURL: yuvaSite,
		Style: template.CSS(statusStyle + homeStyle), Mark: yuvaMark, Globe: homeGlobe,
		Snippet: `<script src="` + panel + `/yuva-status.js" defer></script>` + "\n" + `<yuva-status page="northwind" theme="auto"></yuva-status>`,
		Facts:   w.Facts, Chart: sampleChart(w.ResponseAlt),
		Band: sampleStrip(90, map[int]string{14: stDegraded, 37: stMaintenance, 71: stDegraded})}
	v.HomePath = l.path()
	if l.Code == "en" {
		v.HomePath = "/"
	}
	for _, o := range homeLocales {
		v.Locales = append(v.Locales, homeLangLink{Code: o.Code, Name: o.Name, Href: o.path(), On: o.Code == l.Code})
		if o.Code == "en" {
			v.Locales[len(v.Locales)-1].Href = "/"
		}
		if o.Code != l.Code {
			v.OGAlternates = append(v.OGAlternates, o.OG)
		}
	}
	for i, f := range w.Features {
		v.Features = append(v.Features, homeFeature{N: fmt.Sprintf("%02d", i+1), Title: f.Title, Text: f.Text})
	}
	for i, st := range []struct{ time, class string }{{"20:05", stDegraded}, {"20:05", stDegraded}, {"20:07", stMaintenance}, {"20:52", stOperational}} {
		v.Steps = append(v.Steps, homeStep{Time: st.time, Title: w.Steps[i].Title, Text: w.Steps[i].Text, Class: st.class})
	}
	for i, n := range []string{"1,240", "312", "14"} {
		v.Reach = append(v.Reach, homeReach{N: n, Label: w.Reach[i]})
	}
	for i, c := range []struct{ who, via string }{{"Maya Chen", w.ViaChat}, {"Jonas Weber", w.ViaMail}, {"Priya Nair", w.ViaApp}} {
		v.Convs = append(v.Convs, homeConv{Who: c.who, Text: w.Convs[i], Via: c.via})
	}
	var err error
	if v.Card, err = s.homeCard(ctx, statusWordsFor(l.Code)); err != nil {
		return v, err
	}
	ld, err := homeLD(v)
	if err != nil {
		return v, err
	}
	v.LD = ld
	return v, nil
}

func homeLD(v homeView) (template.HTML, error) {
	org := map[string]any{"@type": "Organization", "name": "Yuva", "url": yuvaSite, "logo": v.Origin + v.Favicon}
	faq := make([]map[string]any, 0, len(v.W.FAQ))
	for _, q := range v.W.FAQ {
		faq = append(faq, map[string]any{"@type": "Question", "name": q.Q, "acceptedAnswer": map[string]any{"@type": "Answer", "text": q.A}})
	}
	var b strings.Builder
	for _, block := range []map[string]any{
		org,
		{"@type": "WebSite", "name": v.W.SiteName, "url": v.Canonical, "inLanguage": v.Lang, "publisher": map[string]any{"@type": "Organization", "name": "Yuva", "url": yuvaSite}},
		{"@type": "SoftwareApplication", "name": v.W.SiteName, "url": v.Canonical, "description": v.W.Description, "inLanguage": v.Lang,
			"applicationCategory": "BusinessApplication", "operatingSystem": "Web", "image": v.OGImage,
			"publisher": map[string]any{"@type": "Organization", "name": "Yuva", "url": yuvaSite}},
		{"@type": "FAQPage", "url": v.Canonical, "inLanguage": v.Lang, "mainEntity": faq},
	} {
		block["@context"] = "https://schema.org"
		raw, err := json.Marshal(block)
		if err != nil {
			return "", err
		}
		b.WriteString(`<script type="application/ld+json">` + string(raw) + "</script>\n")
	}
	return template.HTML(b.String()), nil
}

func (s *Server) renderHome(ctx context.Context, l homeLocale) ([]byte, error) {
	v, err := s.homeView(ctx, l)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := homeTemplate.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (s *Server) homeAnswer(ctx context.Context, l homeLocale) (statusCacheEntry, error) {
	now := s.now()
	key := "home|" + l.Code
	if e, ok := s.statusCache.get(key, time.Time{}, now); ok {
		return e, nil
	}
	body, err := s.renderHome(ctx, l)
	if err != nil {
		return statusCacheEntry{}, err
	}
	e := statusCacheEntry{at: now, ctype: "text/html; charset=utf-8", body: body}
	s.statusCache.put(key, e)
	return e, nil
}

func writeHomeAnswer(w http.ResponseWriter, r *http.Request, e statusCacheEntry, csp string) {
	writePublicAnswer(w, r, e, statusCacheControl, csp)
}

func (s *Server) statusHomeOff(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=30")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="robots" content="noindex"><title>Not found</title></head>` +
		`<body style="font:16px/1.5 system-ui,sans-serif;margin:15vh auto;max-width:28rem;padding:0 1rem"><h1 style="font-size:1.25rem">Not found</h1><p>There is nothing here.</p></body></html>` + "\n"))
}

func homeLocaleAt(path string) (homeLocale, bool) {
	code, ok := strings.CutSuffix(strings.TrimPrefix(path, "/"), "/")
	if !ok || code == "" || strings.Contains(code, "/") {
		return homeLocale{}, false
	}
	for _, l := range homeLocales {
		if strings.ToLower(l.Code) == code {
			return l, true
		}
	}
	return homeLocale{}, false
}

// serveStatusRoot answers what the bare status origin serves besides pages: the home page in every
// language, robots.txt, the sitemap, the icon, the share image and the home page's fonts. False
// leaves the request to the page routes.
func (s *Server) serveStatusRoot(w http.ResponseWriter, r *http.Request) bool {
	p := r.URL.Path
	if p == "/" && s.status.HomeOff {
		s.statusHomeOff(w)
		return true
	}
	if s.status.HomeOff || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return false
	}
	origin := s.statusOrigin()
	static := func(ctype string, body []byte) bool {
		writeHomeAnswer(w, r, statusCacheEntry{ctype: ctype, body: body}, "default-src 'none'; style-src 'unsafe-inline'")
		return true
	}
	switch p {
	case "/robots.txt":
		return static("text/plain; charset=utf-8", []byte("User-agent: *\nAllow: /\n\nSitemap: "+origin+"/sitemap.xml\n"))
	case "/sitemap.xml":
		return static("application/xml; charset=utf-8", homeSitemap(origin, s.homeAlternates(origin)))
	case "/favicon.svg":
		return static("image/svg+xml", homeFavicon)
	case "/og.png":
		return static("image/png", statusHomeOG)
	}
	if name, ok := strings.CutPrefix(p, "/fonts/"); ok && name != "" && !strings.Contains(name, "/") {
		r.SetPathValue("name", name)
		servePublicFont(w, r)
		return true
	}
	if p == "/" {
		s.serveHome(w, r, homeLocales[0])
		return true
	}
	if l, ok := homeLocaleAt(p); ok {
		s.serveHome(w, r, l)
		return true
	}
	if l, ok := homeLocaleAt(p + "/"); ok && len(l.Code) == 2 {
		http.Redirect(w, r, l.path(), http.StatusMovedPermanently)
		return true
	}
	return false
}

func (s *Server) serveHome(w http.ResponseWriter, r *http.Request, l homeLocale) {
	ctx := r.Context()
	if code := r.URL.Query().Get("lang"); code != "" {
		if to, ok := homeLocaleFor(code); ok {
			target := to.path()
			if to.Code == "en" {
				target = "/"
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
	}
	if err := s.rateLimit(ctx, rateCheck{"status:ip:" + rateIP(s.clientIP(r)), limitStatusPerIP}); err != nil {
		s.writeError(w, r, err)
		return
	}
	e, err := s.homeAnswer(ctx, l)
	if err != nil {
		s.log.ErrorContext(ctx, "status home", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	writeHomeAnswer(w, r, e, statusHomeCSP)
}

func homeSitemap(origin string, alts []homeAlt) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	esc := func(s string) string {
		var e bytes.Buffer
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	for _, l := range homeLocales {
		b.WriteString("<url><loc>" + esc(homeURL(origin, l)) + "</loc><lastmod>" + statusHomeUpdated + "</lastmod>")
		for _, a := range alts {
			b.WriteString(`<xhtml:link rel="alternate" hreflang="` + a.Hreflang + `" href="` + esc(a.Href) + `"/>`)
		}
		b.WriteString("</url>\n")
	}
	b.WriteString("</urlset>\n")
	return b.Bytes()
}

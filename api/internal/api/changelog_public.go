package api

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	changelogCacheTTL     = 5 * time.Minute
	changelogCacheMax     = 5000
	changelogCacheControl = "public, max-age=60, stale-while-revalidate=300, stale-if-error=86400"
	changelogPageSize     = 20
	changelogFeedEntries  = 50
	changelogPageType     = "text/html; charset=utf-8"
	changelogFeedType     = "application/atom+xml; charset=utf-8"
)

var errChangelogPageGone = problem(http.StatusNotFound, "not_found", "no such changelog page")

type changelogRedirect string

func (r changelogRedirect) Error() string { return string(r) }

type changelogRoute struct {
	view   string
	locale string
	number int32
	slug   string
}

func parseChangelogRoute(rest string) (changelogRoute, bool) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return changelogRoute{view: "root"}, true
	}
	if rest == "sitemap.xml" {
		return changelogRoute{view: "sitemap"}, true
	}
	parts := strings.Split(rest, "/")
	rt := changelogRoute{locale: parts[0]}
	switch {
	case len(parts) == 1:
		rt.view = "home"
	case len(parts) == 2 && parts[1] == "feed.atom":
		rt.view = "feed"
	case len(parts) == 3 && parts[1] == "entries":
		m := helpNumberSlugPattern.FindStringSubmatch(parts[2])
		if m == nil {
			return rt, false
		}
		n, _ := strconv.Atoi(m[1])
		rt.view, rt.number, rt.slug = "entry", int32(n), m[2]
	default:
		return rt, false
	}
	return rt, true
}

func (s *Server) routeChangelog(mux *http.ServeMux) {
	mux.HandleFunc("GET "+changelogPathPrefix+"{slug}/fonts/{name}", servePublicFont)
	mux.HandleFunc("GET "+changelogPathPrefix+"{slug}", s.serveChangelog)
	mux.HandleFunc("GET "+changelogPathPrefix+"{slug}/{rest...}", s.serveChangelog)
}

func changelogGone(w http.ResponseWriter, locale string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(changelogWordsFor(locale).Gone + "\n"))
}

// publicChangelog finds the changelog a public URL names. An unpublished changelog, an unknown slug and
// a former slug of an unpublished changelog all answer the same plain 404.
func (s *Server) publicChangelog(w http.ResponseWriter, r *http.Request) (store.Changelog, bool) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if s.changelog.Off {
		changelogGone(w, "en")
		return store.Changelog{}, false
	}
	if err := s.rateLimit(ctx, rateCheck{"changelog:ip:" + rateIP(s.clientIP(r)), limitPublicPerIP}); err != nil {
		s.writeError(w, r, err)
		return store.Changelog{}, false
	}
	var c store.Changelog
	var err error
	moved := false
	if on, ok := ownDomainOn(ctx, kindChangelog); ok {
		c, err = s.st.GetChangelog(ctx, store.GetChangelogParams{WorkspaceID: on.workspace, ID: on.page})
		moved = err == nil && !stillOn(c.CustomDomain, c.DomainState, on.host)
	} else if c, err = s.st.FindChangelogBySlug(ctx, slug); store.IsNotFound(err) {
		c, err = s.st.FindChangelogByFormerSlug(ctx, slug)
		if err == nil && c.Published {
			redirectPublic(w, r, s.changelogURL(c, strings.TrimPrefix(r.URL.Path, changelogPathPrefix+slug)), true)
			return c, false
		}
	}
	if moved || store.IsNotFound(err) || (err == nil && !c.Published) {
		changelogGone(w, "en")
		return c, false
	}
	if err != nil {
		s.log.ErrorContext(ctx, "changelog", slog.Any("error", err))
		writeProblem(w, errInternal)
		return c, false
	}
	return c, true
}

func (s *Server) changelogPageCSP(in store.Inbox) string {
	imgs := []string{"'self'"}
	for _, o := range []string{originOf(s.auth.PublicURL), originOf(inboxLogo(in))} {
		if o != "" && !slices.Contains(imgs, o) {
			imgs = append(imgs, o)
		}
	}
	return publicCSP{img: strings.Join(imgs, " "), scriptHash: changelogScriptHash, frameAncestors: "'none'", formAction: "'none'", connect: "'none'"}.String()
}

func (s *Server) serveChangelog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, ok := s.publicChangelog(w, r)
	if !ok {
		return
	}
	rt, ok := parseChangelogRoute(r.PathValue("rest"))
	if ok && rt.view == "root" {
		redirectPublic(w, r, s.changelogURL(c, "/"+c.DefaultLocale+"/"), false)
		return
	}
	if ok && rt.locale != "" && !slices.Contains(c.Locales, rt.locale) {
		ok = false
	}
	if !ok {
		changelogGone(w, c.DefaultLocale)
		return
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: c.WorkspaceID, ID: c.InboxID})
	if err != nil {
		s.log.ErrorContext(ctx, "changelog inbox", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	label := r.URL.Query().Get("label")
	if !oas.ChangelogLabel(label).Valid() {
		label = ""
	}
	before := r.URL.Query().Get("before")
	key := strings.Join([]string{c.ID.String(), rt.view, rt.locale, label, before, strconv.Itoa(int(rt.number)), rt.slug}, "|")
	now := s.now()
	e, hit := s.changelogCache.get(key, c.ChangedAt, now)
	csp := s.changelogPageCSP(in)
	if !hit {
		body, ctype, err := s.changelogAnswer(ctx, c, in, rt, label, before)
		if err == errChangelogPageGone {
			changelogGone(w, rt.locale)
			return
		}
		switch err := err.(type) {
		case nil:
		case changelogRedirect:
			http.Redirect(w, r, string(err), http.StatusMovedPermanently)
			return
		case *apiError:
			writeProblem(w, err)
			return
		default:
			s.log.ErrorContext(ctx, "changelog page", slog.Any("error", err))
			writeProblem(w, errInternal)
			return
		}
		e = publicEntry{changed: c.ChangedAt, at: now, ctype: ctype, body: body}
		s.changelogCache.put(key, e)
	}
	if rt.view == "sitemap" {
		csp = "default-src 'none'; style-src 'unsafe-inline'"
	}
	writePublicAnswer(w, r, e, changelogCacheControl, csp)
}

type changelogChip struct{ Code, Name string }

type changelogItem struct {
	Title, URL, Image string
	ImageW, ImageH    int32
	Body              template.HTML
	At                timeView
	Labels            []changelogChip
	Caption           string
}

type changelogFilter struct {
	Name, Href string
	On         bool
}

type changelogPageView struct {
	W                           changelogWords
	SW                          statusWords
	Lang, Dir, View             string
	Title, OGTitle, Description string
	Canonical, OGType, OGLocale string
	OGImage                     string
	NoIndex, Single             bool
	Alternates                  []helpAlt
	JSONLD                      template.HTML
	Name, Logo                  string
	Accent                      template.CSS
	FontSans, FontMono          string
	Home, FeedURL, H1, Lead     string
	Crumbs                      []helpCrumb
	Filters                     []changelogFilter
	Items                       []changelogItem
	Older                       string
	Langs                       []helpLangLink
	Mark                        template.HTML
	YuvaSite                    string
	Script                      template.JS
}

// changelogOptions say what a page shows beyond what the public sees.
type changelogOptions struct {
	preview, draft bool
	entry          uuid.UUID
}

func (s *Server) changelogAnswer(ctx context.Context, c store.Changelog, in store.Inbox, rt changelogRoute, label, before string) ([]byte, string, error) {
	switch rt.view {
	case "sitemap":
		b, err := s.changelogSitemap(ctx, c)
		return b, "application/xml; charset=utf-8", err
	case "feed":
		b, err := s.changelogFeed(ctx, c, rt.locale)
		return b, changelogFeedType, err
	}
	b, err := s.changelogPage(ctx, c, in, rt, label, before, changelogOptions{})
	return b, changelogPageType, err
}

func (s *Server) changelogItemOf(c store.Changelog, locale string, r store.ListPublicChangelogPageRow) changelogItem {
	return s.changelogItemFrom(c, locale, r.Number, r.Labels, r.PublishedAt, r.Title, r.Slug, r.Html, r.ImageToken, r.ImageWidth, r.ImageHeight)
}

func (s *Server) changelogItemFrom(c store.Changelog, locale string, number int32, labels []string, at time.Time, title, slug, body string, token *string, w, h *int32) changelogItem {
	sw := statusWordsFor(locale)
	cw := changelogWordsFor(locale)
	it := changelogItem{
		Title: title, URL: s.changelogEntryURL(c, number, locale, slug), Body: template.HTML(body), At: timeView{ISO: at.UTC().Format(time.RFC3339), Text: sw.day(at.UTC())},
		Caption: cw.Labels,
	}
	for _, l := range labels {
		it.Labels = append(it.Labels, changelogChip{Code: l, Name: cw.label(l)})
	}
	if token != nil && w != nil && h != nil {
		it.Image, it.ImageW, it.ImageH = s.imageURL(*token), *w, *h
	}
	return it
}

func (s *Server) changelogBaseView(c store.Changelog, in store.Inbox, locale, view string) *changelogPageView {
	sw := statusWordsFor(locale)
	v := &changelogPageView{
		W: changelogWordsFor(locale), SW: sw, Lang: locale, Dir: sw.direction(), View: view, Name: c.Name, OGLocale: strings.ReplaceAll(locale, "-", "_"),
		OGType: "website", FontSans: s.changelogURL(c, publicFontPath("Geist")), FontMono: s.changelogURL(c, publicFontPath("GeistMono")),
		Home: s.changelogURL(c, "/"+locale+"/"), FeedURL: s.changelogURL(c, "/"+locale+"/feed.atom"), Mark: yuvaMark, YuvaSite: yuvaSite,
		Script: template.JS(changelogScript), Logo: inboxLogo(in),
	}
	var br oas.InboxBranding
	if json.Unmarshal(in.Branding, &br) == nil && br.Color != nil && colorPattern.MatchString(*br.Color) {
		v.Accent = template.CSS(*br.Color)
	}
	return v
}

func (s *Server) changelogPage(ctx context.Context, c store.Changelog, in store.Inbox, rt changelogRoute, label, before string, opt changelogOptions) ([]byte, error) {
	q := s.st.Queries
	locale := rt.locale
	v := s.changelogBaseView(c, in, locale, rt.view)
	v.NoIndex = opt.preview
	locales, err := q.ListPublicChangelogLocales(ctx, store.ListPublicChangelogLocalesParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if err != nil {
		return nil, err
	}
	switch rt.view {
	case "home":
		if err := s.changelogHomeView(ctx, c, v, locale, label, before, opt); err != nil {
			return nil, err
		}
		for _, l := range c.Locales {
			if slices.Contains(locales, l) {
				v.Alternates = append(v.Alternates, helpAlt{Hreflang: l, Href: s.changelogURL(c, "/"+l+"/")})
				v.Langs = append(v.Langs, helpLangLink{Code: l, Label: helpLanguageName(l), Href: s.changelogURL(c, "/"+l+"/"), On: l == locale})
			}
		}
	default:
		alts, err := s.changelogEntryView(ctx, c, v, rt, opt)
		if err != nil {
			return nil, err
		}
		for _, l := range c.Locales {
			if href, ok := alts[l]; ok {
				v.Alternates = append(v.Alternates, helpAlt{Hreflang: l, Href: href})
				v.Langs = append(v.Langs, helpLangLink{Code: l, Label: helpLanguageName(l), Href: href, On: l == locale})
			}
		}
	}
	var buf bytes.Buffer
	if err := changelogPageTemplate.Execute(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Server) changelogHomeView(ctx context.Context, c store.Changelog, v *changelogPageView, locale, label, before string, opt changelogOptions) error {
	var lab *string
	if label != "" {
		lab = &label
	}
	if opt.preview {
		items, err := s.changelogPreviewItems(ctx, c, locale, lab, opt.draft)
		if err != nil {
			return err
		}
		v.Items = items
	} else if err := s.changelogPublicItems(ctx, c, v, locale, lab, before); err != nil {
		return err
	}
	v.H1, v.Lead = c.Name, c.Description
	v.Title, v.OGTitle = c.Name+" – "+v.W.Title, c.Name
	v.Description = helpDescription(c.Description, c.Name+" – "+v.W.Title)
	v.Canonical = v.Home
	if label != "" {
		v.Canonical += "?label=" + label
	}
	filter := func(name, l string) changelogFilter {
		href := v.Home
		if l != "" {
			href += "?label=" + l
		}
		return changelogFilter{Name: name, Href: href, On: l == label}
	}
	v.Filters = []changelogFilter{filter(v.W.All, ""), filter(v.W.New, "new"), filter(v.W.Improved, "improved"), filter(v.W.Fixed, "fixed")}
	if len(v.Items) > 0 {
		v.OGImage = v.Items[0].Image
	}
	return nil
}

func (s *Server) changelogPublicItems(ctx context.Context, c store.Changelog, v *changelogPageView, locale string, label *string, before string) error {
	var at *time.Time
	var cursorID *uuid.UUID
	if before != "" {
		b := before
		var err error
		if at, cursorID, err = decodeCursor(&b); err != nil {
			return errChangelogPageGone
		}
	}
	rows, err := s.st.ListPublicChangelogPage(ctx, store.ListPublicChangelogPageParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Locale: locale, Label: label, CursorAt: at, CursorID: cursorID, Lim: changelogPageSize + 1,
	})
	if err != nil {
		return err
	}
	if len(rows) > changelogPageSize {
		rows = rows[:changelogPageSize]
		last := rows[len(rows)-1]
		q := "?before=" + encodeCursor(last.PublishedAt, last.ID)
		if label != nil {
			q += "&label=" + *label
		}
		v.Older = v.Home + q
	}
	for _, r := range rows {
		v.Items = append(v.Items, s.changelogItemOf(c, locale, r))
	}
	return nil
}

func (s *Server) changelogEntryView(ctx context.Context, c store.Changelog, v *changelogPageView, rt changelogRoute, opt changelogOptions) (map[string]string, error) {
	if opt.preview {
		return s.changelogPreviewEntry(ctx, c, v, rt, opt)
	}
	q := s.st.Queries
	r, err := q.GetPublicChangelogEntryByNumber(ctx, store.GetPublicChangelogEntryByNumberParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Locale: rt.locale, Number: rt.number,
	})
	if store.IsNotFound(err) {
		return nil, errChangelogPageGone
	}
	if err != nil {
		return nil, err
	}
	if rt.slug != r.Slug {
		return nil, changelogRedirect(s.changelogEntryURL(c, r.Number, rt.locale, r.Slug))
	}
	it := s.changelogItemFrom(c, rt.locale, r.Number, r.Labels, r.PublishedAt, r.Title, r.Slug, r.Html, r.ImageToken, r.ImageWidth, r.ImageHeight)
	locs, err := q.ListPublicChangelogEntryLocales(ctx, store.ListPublicChangelogEntryLocalesParams{WorkspaceID: c.WorkspaceID, EntryID: r.ID})
	if err != nil {
		return nil, err
	}
	alts := map[string]string{}
	for _, l := range locs {
		if slices.Contains(c.Locales, l.Locale) {
			alts[l.Locale] = s.changelogEntryURL(c, r.Number, l.Locale, l.Slug)
		}
	}
	v.Single, v.Items = true, []changelogItem{it}
	v.H1, v.OGType, v.Canonical, v.OGImage = r.Title, "article", it.URL, it.Image
	v.Title, v.OGTitle = r.Title+" – "+c.Name, r.Title
	v.Description = helpDescription(r.Excerpt, r.Title+" – "+c.Name)
	v.Crumbs = []helpCrumb{{Name: v.W.Title, URL: v.Home}, {Name: r.Title}}
	post := map[string]any{
		"@context": "https://schema.org", "@type": "BlogPosting", "headline": r.Title, "description": v.Description,
		"datePublished": r.PublishedAt.UTC().Format(time.RFC3339), "dateModified": r.TranslationPublishedAt.UTC().Format(time.RFC3339),
		"inLanguage": rt.locale, "mainEntityOfPage": it.URL, "publisher": map[string]any{"@type": "Organization", "name": c.Name},
	}
	if it.Image != "" {
		post["image"] = it.Image
	}
	v.JSONLD = jsonLD(post, breadcrumbLD(v.Crumbs[:1], r.Title, it.URL))
	return alts, nil
}

func (s *Server) changelogFeed(ctx context.Context, c store.Changelog, locale string) ([]byte, error) {
	rows, err := s.st.ListPublicChangelogPage(ctx, store.ListPublicChangelogPageParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Locale: locale, Lim: changelogFeedEntries,
	})
	if err != nil {
		return nil, err
	}
	feed := atomFeed{
		ID: s.changelogURL(c, "/"+locale+"/"), Title: c.Name, Updated: c.ChangedAt.UTC().Format(time.RFC3339),
		Links: []atomLink{{Href: s.changelogURL(c, "/"+locale+"/"), Rel: "alternate"}, {Href: s.changelogURL(c, "/"+locale+"/feed.atom"), Rel: "self"}},
	}
	for _, r := range rows {
		e := atomEntry{
			ID: "urn:uuid:" + r.ID.String(), Title: r.Title, Updated: r.TranslationPublishedAt.UTC().Format(time.RFC3339),
			Published: r.PublishedAt.UTC().Format(time.RFC3339), Link: atomLink{Href: s.changelogEntryURL(c, r.Number, locale, r.Slug)},
			Content: atomText{Type: "html", Body: r.Html},
		}
		for _, l := range r.Labels {
			e.Categories = append(e.Categories, atomCategory{Term: l})
		}
		feed.Entries = append(feed.Entries, e)
	}
	return marshalAtom(feed)
}

func (s *Server) changelogSitemap(ctx context.Context, c store.Changelog) ([]byte, error) {
	q := s.st.Queries
	locales, err := q.ListPublicChangelogLocales(ctx, store.ListPublicChangelogLocalesParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if err != nil {
		return nil, err
	}
	rows, err := q.ListChangelogSitemapEntries(ctx, store.ListChangelogSitemapEntriesParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	entry := func(loc string, mod time.Time, alts map[string]string) {
		b.WriteString("<url><loc>" + xmlEscape(loc) + "</loc><lastmod>" + mod.UTC().Format("2006-01-02") + "</lastmod>")
		for _, l := range c.Locales {
			if href, ok := alts[l]; ok {
				b.WriteString(`<xhtml:link rel="alternate" hreflang="` + xmlEscape(l) + `" href="` + xmlEscape(href) + `"/>`)
			}
		}
		b.WriteString("</url>\n")
	}
	homes := map[string]string{}
	for _, l := range locales {
		homes[l] = s.changelogURL(c, "/"+l+"/")
	}
	for _, l := range c.Locales {
		if h, ok := homes[l]; ok {
			entry(h, c.ChangedAt, homes)
		}
	}
	alts := map[int32]map[string]string{}
	for _, r := range rows {
		if alts[r.Number] == nil {
			alts[r.Number] = map[string]string{}
		}
		alts[r.Number][r.Locale] = s.changelogEntryURL(c, r.Number, r.Locale, r.Slug)
	}
	for _, r := range rows {
		if slices.Contains(c.Locales, r.Locale) {
			entry(alts[r.Number][r.Locale], r.PublishedAt, alts[r.Number])
		}
	}
	b.WriteString("</urlset>\n")
	return b.Bytes(), nil
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	helpPopularShown  = 5
	helpOtherShown    = 50
	helpSearchShown   = 10
	helpPageCacheTTL  = 5 * time.Minute
	helpPageCacheMax  = 5000
	helpCacheControl  = "public, max-age=60, stale-while-revalidate=300, stale-if-error=86400"
	helpTOCMinEntries = 2
)

var helpScript = `var L=document.documentElement.lang;` +
	`document.querySelectorAll("time[data-local]").forEach(function(t){var d=new Date(t.getAttribute("datetime"));if(!isNaN(d))t.textContent=d.toLocaleDateString(L,{dateStyle:"medium"})});` +
	`document.querySelectorAll("form[data-helpful]").forEach(function(f){f.addEventListener("submit",function(e){e.preventDefault();var b=e.submitter,d=new URLSearchParams();` +
	`f.querySelectorAll("input[type=hidden]").forEach(function(i){d.set(i.name,i.value)});if(b&&b.name)d.set(b.name,b.value);` +
	`fetch(f.action,{method:"POST",body:d,headers:{Accept:"application/json"}}).then(function(r){if(!r.ok)throw 0;f.parentNode.textContent=f.getAttribute("data-thanks")}).catch(function(){f.submit()})})})`

var helpScriptHash = publicScriptHash(helpScript)

var helpHeading = regexp.MustCompile(`(?s)<h([23]) id="([^"]+)"[^>]*>(.*?)</h[23]>`)
var helpTag = regexp.MustCompile(`<[^>]*>`)

type helpAlt struct{ Hreflang, Href string }

type helpLangLink struct {
	Code, Label, Href string
	On                bool
}

type helpCrumb struct{ Name, URL string }

type helpSubView struct{ Name, URL, Count string }

type helpCollView struct {
	Name, Description, URL, Count string
	Subs                          []helpSubView
}

type helpItemView struct {
	Title, URL, Excerpt, Collection string
	TitleHTML, ExcerptHTML          template.HTML
}

type helpTOCEntry struct {
	ID, Text string
	Sub      bool
}

type helpfulView struct {
	Action, State string
}

type helpChatView struct{ Script, Server, Key, Article string }

type helpPageView struct {
	W                           helpWords
	SW                          statusWords
	Lang, Dir, View             string
	Title, OGTitle, Description string
	Canonical, OGType, OGLocale string
	NoIndex                     bool
	Alternates                  []helpAlt
	JSONLD                      template.HTML
	Name, Logo, Site            string
	Accent                      template.CSS
	FontSans, FontMono          string
	Home, SearchURL, Query      string
	H1, Lead                    string
	Crumbs                      []helpCrumb
	Collections                 []helpCollView
	Items, Popular, Related     []helpItemView
	Body                        template.HTML
	TOC                         []helpTOCEntry
	Updated                     timeView
	Helpful                     *helpfulView
	Partial                     bool
	Langs                       []helpLangLink
	Chat                        *helpChatView
	Mark                        template.HTML
	YuvaSite                    string
	Script                      template.JS
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	o, ok := normalizeOrigin(u.Scheme + "://" + u.Host)
	if !ok {
		return ""
	}
	return o
}

func (s *Server) helpPageCSP(c store.HelpCenter, chat bool) string {
	imgs := []string{"'self'"}
	add := func(o string) {
		if o != "" && !slices.Contains(imgs, o) {
			imgs = append(imgs, o)
		}
	}
	add(originOf(s.linkBase()))
	if c.LogoUrl != nil {
		add(originOf(*c.LogoUrl))
	}
	csp := publicCSP{scriptHash: helpScriptHash, frameAncestors: "'none'", formAction: "'self'", connect: "'self'"}
	if chat {
		pub := originOf(s.auth.PublicURL)
		ws := strings.Replace(pub, "http", "ws", 1)
		csp.script = pub
		csp.connect = "'self' " + pub + " " + ws
		imgs = append(imgs, "https:", "data:")
	}
	csp.img = strings.Join(imgs, " ")
	return csp.String()
}

func (s *Server) helpTime(w statusWords, t time.Time) timeView {
	return timeView{ISO: t.UTC().Format(time.RFC3339), Text: w.day(t.UTC())}
}

func helpLanguageName(code string) string {
	if w, ok := statusLanguage(code); ok {
		return w.Name
	}
	return code
}

func (s *Server) helpBaseView(ctx context.Context, q *store.Queries, c store.HelpCenter, locale, view string) (*helpPageView, error) {
	sw := statusWordsFor(locale)
	v := &helpPageView{
		W: helpWordsFor(locale), SW: sw, Lang: locale, Dir: sw.direction(), View: view, Name: c.Name, OGLocale: strings.ReplaceAll(locale, "-", "_"),
		OGType: "website", FontSans: s.helpURL(c, publicFontPath("Geist")), FontMono: s.helpURL(c, publicFontPath("GeistMono")),
		Home: s.helpHomeURL(c, locale), SearchURL: s.helpSearchURL(c, locale), Mark: yuvaMark, YuvaSite: yuvaSite, Script: template.JS(helpScript),
	}
	if c.AccentColor != nil {
		v.Accent = template.CSS(*c.AccentColor)
	}
	if c.LogoUrl != nil {
		v.Logo = *c.LogoUrl
	}
	if c.SiteUrl != nil {
		v.Site = *c.SiteUrl
	}
	if c.ChatChannelID != nil && !s.noWidget {
		ch, err := q.GetChatChannel(ctx, store.GetChatChannelParams{WorkspaceID: c.WorkspaceID, ChannelID: *c.ChatChannelID})
		if err != nil && !store.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			v.Chat = &helpChatView{Script: s.auth.PublicURL + "/yuva.js", Server: s.auth.PublicURL, Key: ch.PublicKey}
		}
	}
	return v, nil
}

func (v *helpPageView) setAlternates(c store.HelpCenter, urls map[string]string) {
	for _, l := range c.Locales {
		if href, ok := urls[l]; ok {
			v.Alternates = append(v.Alternates, helpAlt{Hreflang: l, Href: href})
			v.Langs = append(v.Langs, helpLangLink{Code: l, Label: helpLanguageName(l), Href: href, On: l == v.Lang})
		}
	}
	if href, ok := urls[c.DefaultLocale]; ok {
		v.Alternates = append(v.Alternates, helpAlt{Hreflang: "x-default", Href: href})
	}
}

func (s *Server) helpItems(c store.HelpCenter, arts []helpArt, names map[uuid.UUID]*helpColl) []helpItemView {
	out := make([]helpItemView, len(arts))
	for i, a := range arts {
		out[i] = helpItemView{Title: a.Title, URL: s.helpArtURL(c, a), Excerpt: a.Excerpt}
		if a.CollectionID != nil && names != nil {
			if col := names[*a.CollectionID]; col != nil {
				out[i].Collection = col.Name
			}
		}
	}
	return out
}

func (s *Server) helpCollViews(c store.HelpCenter, locale string, cols []*helpColl, w helpWords) []helpCollView {
	out := make([]helpCollView, len(cols))
	for i, col := range cols {
		out[i] = helpCollView{Name: col.Name, Description: col.Description, URL: s.helpCollectionURL(c, locale, col), Count: w.articles(col.Total)}
		for _, sub := range col.Subs {
			out[i].Subs = append(out[i].Subs, helpSubView{Name: sub.Name, URL: s.helpCollectionURL(c, locale, sub), Count: w.articles(sub.Total)})
		}
	}
	return out
}

func helpDescription(text, fallback string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return fallback
	}
	if r := []rune(text); len(r) > 200 {
		return strings.TrimSpace(string(r[:200])) + "…"
	}
	return text
}

func jsonLD(v ...any) template.HTML {
	var b strings.Builder
	for _, item := range v {
		raw, err := json.Marshal(item)
		if err != nil {
			continue
		}
		b.WriteString(`<script type="application/ld+json">` + string(raw) + "</script>\n")
	}
	return template.HTML(b.String())
}

func breadcrumbLD(crumbs []helpCrumb, last, lastURL string) map[string]any {
	items := make([]map[string]any, 0, len(crumbs)+1)
	for _, cr := range crumbs {
		if cr.URL != "" {
			items = append(items, map[string]any{"@type": "ListItem", "position": len(items) + 1, "name": cr.Name, "item": cr.URL})
		}
	}
	items = append(items, map[string]any{"@type": "ListItem", "position": len(items) + 1, "name": last, "item": lastURL})
	return map[string]any{"@context": "https://schema.org", "@type": "BreadcrumbList", "itemListElement": items}
}

func (s *Server) helpHomeView(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string) (*helpPageView, error) {
	v, err := s.helpBaseView(ctx, q, c, locale, "home")
	if err != nil {
		return nil, err
	}
	cols, index, err := s.helpCollections(ctx, q, c, locale)
	if err != nil {
		return nil, err
	}
	popular, err := s.helpPopular(ctx, q, c, locale, helpPopularShown)
	if err != nil {
		return nil, err
	}
	all, err := s.helpPublishedArticles(ctx, q, c, locale, nil)
	if err != nil {
		return nil, err
	}
	var loose []helpArt
	for _, a := range all {
		if (a.CollectionID == nil || index[*a.CollectionID] == nil) && len(loose) < helpOtherShown {
			loose = append(loose, a)
		}
	}
	locales, err := q.ListHelpPublishedLocales(ctx, store.ListHelpPublishedLocalesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
	if err != nil {
		return nil, err
	}
	urls := map[string]string{}
	for _, l := range locales {
		urls[l] = s.helpHomeURL(c, l)
	}
	urls[locale] = v.Home
	v.setAlternates(c, urls)
	v.H1, v.Lead = c.Name, c.Description
	v.Title, v.OGTitle = c.Name+" – "+v.W.Help, c.Name
	v.Description = helpDescription(c.Description, c.Name+" – "+v.W.Help)
	v.Canonical = v.Home
	v.Collections = s.helpCollViews(c, locale, cols, v.W)
	v.Popular = s.helpItems(c, popular, index)
	v.Items = s.helpItems(c, loose, index)
	return v, nil
}

func (s *Server) helpCollectionView(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string, col *helpColl, index map[uuid.UUID]*helpColl) (*helpPageView, error) {
	v, err := s.helpBaseView(ctx, q, c, locale, "collection")
	if err != nil {
		return nil, err
	}
	arts, err := s.helpPublishedArticles(ctx, q, c, locale, &col.ID)
	if err != nil {
		return nil, err
	}
	trs, err := q.ListHelpCollectionTranslations(ctx, store.ListHelpCollectionTranslationsParams{WorkspaceID: c.WorkspaceID, CollectionIds: []uuid.UUID{col.ID}})
	if err != nil {
		return nil, err
	}
	urls := map[string]string{locale: s.helpCollectionURL(c, locale, col)}
	for _, t := range trs {
		if t.Locale == locale || !slices.Contains(c.Locales, t.Locale) {
			continue
		}
		_, other, err := s.helpCollections(ctx, q, c, t.Locale)
		if err != nil {
			return nil, err
		}
		if oc := other[col.ID]; oc != nil {
			urls[t.Locale] = s.helpCollectionURL(c, t.Locale, oc)
		}
	}
	v.setAlternates(c, urls)
	v.Canonical = urls[locale]
	v.Crumbs = []helpCrumb{{Name: v.W.Home, URL: v.Home}}
	if col.ParentID != nil {
		if parent := index[*col.ParentID]; parent != nil {
			v.Crumbs = append(v.Crumbs, helpCrumb{Name: parent.Name, URL: s.helpCollectionURL(c, locale, parent)})
		}
	}
	v.Crumbs = append(v.Crumbs, helpCrumb{Name: col.Name})
	v.H1, v.Lead = col.Name, col.Description
	v.Title, v.OGTitle = col.Name+" – "+c.Name, col.Name
	v.Description = helpDescription(col.Description, col.Name+" – "+c.Name)
	v.Collections = s.helpCollViews(c, locale, col.Subs, v.W)
	v.Items = s.helpItems(c, arts, nil)
	v.JSONLD = jsonLD(breadcrumbLD(v.Crumbs[:len(v.Crumbs)-1], col.Name, v.Canonical))
	return v, nil
}

func helpTOC(body string) []helpTOCEntry {
	var out []helpTOCEntry
	for _, m := range helpHeading.FindAllStringSubmatch(body, -1) {
		text := strings.Join(strings.Fields(html.UnescapeString(helpTag.ReplaceAllString(m[3], ""))), " ")
		if text != "" {
			out = append(out, helpTOCEntry{ID: m[2], Text: text, Sub: m[1] == "3"})
		}
	}
	if len(out) < helpTOCMinEntries {
		return nil
	}
	return out
}

func (s *Server) helpArticleView(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string, a store.GetPublicHelpArticleByNumberRow, index map[uuid.UUID]*helpColl, thanks string) (*helpPageView, error) {
	v, err := s.helpBaseView(ctx, q, c, locale, "article")
	if err != nil {
		return nil, err
	}
	locs, err := q.ListHelpArticleLocales(ctx, store.ListHelpArticleLocalesParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID})
	if err != nil {
		return nil, err
	}
	urls := map[string]string{}
	for _, l := range locs {
		if slices.Contains(c.Locales, l.Locale) {
			urls[l.Locale] = s.helpArticleURL(c, a.Number, l.Locale, l.Slug)
		}
	}
	canonical := s.helpArticleURL(c, a.Number, locale, a.Slug)
	urls[locale] = canonical
	v.setAlternates(c, urls)
	v.Canonical = canonical
	v.OGType = "article"
	v.Crumbs = []helpCrumb{{Name: v.W.Home, URL: v.Home}}
	if a.CollectionID != nil {
		if col := index[*a.CollectionID]; col != nil {
			if col.ParentID != nil {
				if parent := index[*col.ParentID]; parent != nil {
					v.Crumbs = append(v.Crumbs, helpCrumb{Name: parent.Name, URL: s.helpCollectionURL(c, locale, parent)})
				}
			}
			v.Crumbs = append(v.Crumbs, helpCrumb{Name: col.Name, URL: s.helpCollectionURL(c, locale, col)})
		}
	}
	v.H1 = a.Title
	v.Crumbs = append(v.Crumbs, helpCrumb{Name: a.Title})
	v.Title, v.OGTitle = a.Title+" – "+c.Name, a.Title
	v.Description = helpDescription(a.Excerpt, a.Title+" – "+c.Name)
	v.Body = template.HTML(a.Html)
	v.TOC = helpTOC(a.Html)
	v.Updated = s.helpTime(v.SW, a.PublishedAt)
	v.Helpful = &helpfulView{Action: canonical + "/helpful", State: thanks}
	related, err := s.helpRelated(ctx, q, c, locale, a.ID, a.CollectionID)
	if err != nil {
		return nil, err
	}
	v.Related = s.helpItems(c, related, nil)
	if v.Chat != nil {
		v.Chat.Article = a.ID.String()
	}
	article := map[string]any{
		"@context": "https://schema.org", "@type": "Article", "headline": a.Title, "description": v.Description, "inLanguage": locale,
		"datePublished": a.PublishedAt.UTC().Format(time.RFC3339), "dateModified": a.PublishedAt.UTC().Format(time.RFC3339),
		"mainEntityOfPage": canonical, "publisher": map[string]any{"@type": "Organization", "name": c.Name},
	}
	v.JSONLD = jsonLD(article, breadcrumbLD(v.Crumbs[:len(v.Crumbs)-1], a.Title, canonical))
	return v, nil
}

func markText(text string, hs []oas.HelpHighlight, field oas.HelpHighlightField) template.HTML {
	units := utf16.Encode([]rune(text))
	var b strings.Builder
	pos := 0
	for _, h := range hs {
		if h.Field != field || int(h.Start) < pos || int(h.End) > len(units) || h.End <= h.Start {
			continue
		}
		b.WriteString(html.EscapeString(string(utf16.Decode(units[pos:h.Start]))))
		b.WriteString("<mark>" + html.EscapeString(string(utf16.Decode(units[h.Start:h.End]))) + "</mark>")
		pos = int(h.End)
	}
	b.WriteString(html.EscapeString(string(utf16.Decode(units[pos:]))))
	return template.HTML(b.String())
}

func (s *Server) helpSearchView(ctx context.Context, q *store.Queries, c store.HelpCenter, locale, query string, words []string, res oas.HelpSearchResults) (*helpPageView, error) {
	v, err := s.helpBaseView(ctx, q, c, locale, "search")
	if err != nil {
		return nil, err
	}
	v.NoIndex = true
	v.Query = query
	v.Canonical = v.SearchURL
	v.Crumbs = []helpCrumb{{Name: v.W.Home, URL: v.Home}, {Name: v.W.SearchGo}}
	v.H1 = v.W.SearchGo
	v.Title, v.OGTitle = v.W.SearchGo+" – "+c.Name, v.W.SearchGo
	v.Description = helpDescription("", c.Name+" – "+v.W.Help)
	if len(words) > 0 {
		v.H1 = strings.Replace(v.W.Results, "%s", query, 1)
	}
	v.Partial = res.Partial
	names := map[string]map[uuid.UUID]*helpColl{}
	for _, r := range res.Items {
		item := helpItemView{Title: r.Title, Excerpt: r.Excerpt, TitleHTML: markText(r.Title, r.Highlights, oas.HelpHighlightTitle), ExcerptHTML: markText(r.Excerpt, r.Highlights, oas.HelpHighlightExcerpt)}
		if r.Url != nil {
			item.URL = *r.Url
		}
		if r.CollectionId != nil {
			if _, ok := names[r.Locale]; !ok {
				_, names[r.Locale], err = s.helpCollections(ctx, q, c, r.Locale)
				if err != nil {
					return nil, err
				}
			}
			if col := names[r.Locale][*r.CollectionId]; col != nil {
				item.Collection = col.Name
			}
		}
		v.Items = append(v.Items, item)
	}
	locales, err := q.ListHelpPublishedLocales(ctx, store.ListHelpPublishedLocalesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
	if err != nil {
		return nil, err
	}
	for _, l := range c.Locales {
		if l == locale || slices.Contains(locales, l) {
			href := s.helpSearchURL(c, l)
			if query != "" {
				href += "?q=" + url.QueryEscape(query)
			}
			v.Langs = append(v.Langs, helpLangLink{Code: l, Label: helpLanguageName(l), Href: href, On: l == locale})
		}
	}
	return v, nil
}

func renderHelpPage(v *helpPageView) ([]byte, error) {
	var b bytes.Buffer
	if err := helpTemplate.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (s *Server) helpSitemap(ctx context.Context, q *store.Queries, c store.HelpCenter) ([]byte, error) {
	locales, err := q.ListHelpPublishedLocales(ctx, store.ListHelpPublishedLocalesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
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
		homes[l] = s.helpHomeURL(c, l)
	}
	for _, l := range c.Locales {
		if h, ok := homes[l]; ok {
			entry(h, c.ChangedAt, homes)
		}
	}
	colURLs := map[uuid.UUID]map[string]string{}
	var order []uuid.UUID
	for _, l := range locales {
		_, index, err := s.helpCollections(ctx, q, c, l)
		if err != nil {
			return nil, err
		}
		for id, col := range index {
			if colURLs[id] == nil {
				colURLs[id] = map[string]string{}
				order = append(order, id)
			}
			colURLs[id][l] = s.helpCollectionURL(c, l, col)
		}
	}
	slices.SortFunc(order, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, id := range order {
		for _, l := range c.Locales {
			if href, ok := colURLs[id][l]; ok {
				entry(href, c.ChangedAt, colURLs[id])
			}
		}
	}
	rows, err := q.ListHelpSitemapArticles(ctx, store.ListHelpSitemapArticlesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
	if err != nil {
		return nil, err
	}
	alts := map[int32]map[string]string{}
	for _, r := range rows {
		if alts[r.Number] == nil {
			alts[r.Number] = map[string]string{}
		}
		alts[r.Number][r.Locale] = s.helpArticleURL(c, r.Number, r.Locale, r.Slug)
	}
	for _, r := range rows {
		if slices.Contains(c.Locales, r.Locale) {
			entry(alts[r.Number][r.Locale], r.PublishedAt, alts[r.Number])
		}
	}
	b.WriteString("</urlset>\n")
	return b.Bytes(), nil
}

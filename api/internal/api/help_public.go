package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	limitHelpViewPerHour  = limit{1, time.Hour}
	limitHelpSearchPerIP  = limit{30, time.Minute}
	limitHelpSearchCenter = limit{1200, time.Minute}
	limitHelpClickPerIP   = limit{60, time.Minute}

	helpNumberSlugPattern = regexp.MustCompile(`^([0-9]{1,9})(?:-(.*))?$`)
)

type helpHostKey struct{}

type helpRoute struct {
	view    string
	locale  string
	number  int32
	slug    string
	helpful bool
}

func parseHelpRoute(rest string) (helpRoute, bool) {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return helpRoute{view: "home"}, true
	}
	if rest == "sitemap.xml" {
		return helpRoute{view: "sitemap"}, true
	}
	parts := strings.Split(rest, "/")
	rt := helpRoute{locale: parts[0]}
	switch {
	case len(parts) == 1:
		rt.view = "home"
	case len(parts) == 2 && parts[1] == "search":
		rt.view = "search"
	case (len(parts) == 3 || len(parts) == 4) && (parts[1] == "collections" || parts[1] == "articles"):
		m := helpNumberSlugPattern.FindStringSubmatch(parts[2])
		if m == nil {
			return rt, false
		}
		n, _ := strconv.Atoi(m[1])
		rt.view, rt.number, rt.slug = strings.TrimSuffix(parts[1], "s"), int32(n), m[2]
		if len(parts) == 4 {
			if parts[1] != "articles" || parts[3] != "helpful" {
				return rt, false
			}
			rt.helpful = true
		}
	default:
		return rt, false
	}
	return rt, true
}

func (s *Server) routeHelp(mux *http.ServeMux) {
	mux.HandleFunc("GET "+helpPathPrefix+"{slug}/fonts/{name}", servePublicFont)
	mux.HandleFunc("GET "+helpPathPrefix+"{slug}", s.serveHelp)
	mux.HandleFunc("GET "+helpPathPrefix+"{slug}/{rest...}", s.serveHelp)
	mux.HandleFunc("POST "+helpPathPrefix+"{slug}/{rest...}", s.serveHelpPost)
}

func helpGone(w http.ResponseWriter, locale string) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "public, max-age=30")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(helpWordsFor(locale).Gone + "\n"))
}

// helpHost serves the help centers at /{slug} on YUVA_HELP_URL's host and nothing else there.
func (s *Server) helpHost(next http.Handler) http.Handler {
	if s.help.Off || s.help.URL == "" {
		return next
	}
	u, err := url.Parse(s.help.URL)
	if err != nil || u.Host == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, u.Host) {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/" || strings.HasPrefix(p, helpPathPrefix) {
			helpGone(w, "en")
			return
		}
		r2 := r.Clone(context.WithValue(r.Context(), helpHostKey{}, true))
		r2.URL.Path = strings.TrimSuffix(helpPathPrefix, "/") + p
		r2.URL.RawPath = ""
		next.ServeHTTP(w, r2)
	})
}

func (s *Server) publicHelpCenter(w http.ResponseWriter, r *http.Request) (store.HelpCenter, bool) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if s.help.Off {
		helpGone(w, "en")
		return store.HelpCenter{}, false
	}
	rest := strings.TrimPrefix(r.URL.Path, helpPathPrefix+slug)
	redirect := func(c store.HelpCenter, permanent bool) {
		redirectPublic(w, r, s.helpURL(c, rest), permanent)
	}
	on, onDomain := ownDomainOn(ctx, kindHelp)
	if s.help.URL != "" && ctx.Value(helpHostKey{}) == nil && !onDomain {
		redirect(store.HelpCenter{Slug: slug}, false)
		return store.HelpCenter{}, false
	}
	if err := s.rateLimit(ctx, rateCheck{"help:ip:" + rateIP(s.clientIP(r)), limitPublicPerIP}); err != nil {
		s.writeError(w, r, err)
		return store.HelpCenter{}, false
	}
	var c store.HelpCenter
	var err error
	moved := false
	if onDomain {
		c, err = s.st.GetHelpCenter(ctx, store.GetHelpCenterParams{WorkspaceID: on.workspace, ID: on.page})
		moved = err == nil && !stillOn(c.CustomDomain, c.DomainState, on.host)
	} else if c, err = s.st.FindHelpCenterBySlug(ctx, slug); store.IsNotFound(err) {
		c, err = s.st.FindHelpCenterByFormerSlug(ctx, slug)
		if err == nil && c.Published {
			redirect(c, true)
			return c, false
		}
	}
	if moved || store.IsNotFound(err) || (err == nil && !c.Published) {
		helpGone(w, "en")
		return c, false
	}
	if err != nil {
		s.log.ErrorContext(ctx, "help center", slog.Any("error", err))
		writeProblem(w, errInternal)
		return c, false
	}
	return c, true
}

type helpRedirect string

func (h helpRedirect) Error() string { return string(h) }

func (s *Server) helpCached(c store.HelpCenter, key, ctype string, build func() ([]byte, error)) (publicEntry, error) {
	now := s.now()
	if e, ok := s.helpCache.get(key, c.ChangedAt, now); ok {
		return e, nil
	}
	body, err := build()
	if err != nil {
		return publicEntry{}, err
	}
	e := publicEntry{changed: c.ChangedAt, at: now, ctype: ctype, body: body}
	s.helpCache.put(key, e)
	return e, nil
}

const helpPageType = "text/html; charset=utf-8"

// A view counts once per address, article and hour.
func (s *Server) countHelpView(ctx context.Context, r *http.Request, c store.HelpCenter, article uuid.UUID, locale string) {
	ok, err := s.limits.allow(ctx, "help-view:"+rateIP(s.clientIP(r))+":"+article.String(), limitHelpViewPerHour, s.now())
	if err != nil {
		s.log.ErrorContext(ctx, "help view rate limit", slog.Any("error", err))
	}
	if ok {
		s.countHelpArticle(c, article, locale, helpArticleCount{views: 1})
	}
}

func (s *Server) serveHelp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, ok := s.publicHelpCenter(w, r)
	if !ok {
		return
	}
	rt, ok := parseHelpRoute(r.PathValue("rest"))
	locale := c.DefaultLocale
	if ok && rt.locale != "" {
		locale = rt.locale
		if !slices.Contains(c.Locales, locale) {
			ok = false
		}
	}
	if !ok {
		helpGone(w, c.DefaultLocale)
		return
	}
	if rt.helpful {
		helpGone(w, locale)
		return
	}
	e, csp, err := s.helpAnswer(ctx, r, c, rt, locale)
	if err != nil {
		switch err := err.(type) {
		case helpRedirect:
			target := string(err)
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		case *apiError:
			writeProblem(w, err)
		default:
			if err == errHelpPageGone {
				helpGone(w, locale)
				return
			}
			s.log.ErrorContext(ctx, "help page", slog.Any("error", err))
			writeProblem(w, errInternal)
		}
		return
	}
	writePublicAnswer(w, r, e, helpCacheControl, csp)
}

var errHelpPageGone = problem(http.StatusNotFound, "not_found", "no such help page")

func (s *Server) helpAnswer(ctx context.Context, r *http.Request, c store.HelpCenter, rt helpRoute, locale string) (publicEntry, string, error) {
	q := s.st.Queries
	chat := c.ChatChannelID != nil && !s.noWidget
	csp := s.helpPageCSP(c, chat)
	prefix := c.ID.String() + "|" + rt.view + "|" + locale + "|"
	render := func(v *helpPageView, err error) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return renderHelpPage(v)
	}
	switch rt.view {
	case "sitemap":
		e, err := s.helpCached(c, prefix, "application/xml; charset=utf-8", func() ([]byte, error) { return s.helpSitemap(ctx, q, c) })
		return e, "default-src 'none'; style-src 'unsafe-inline'", err
	case "home":
		e, err := s.helpCached(c, prefix, helpPageType, func() ([]byte, error) { return render(s.helpHomeView(ctx, q, c, locale)) })
		return e, csp, err
	case "collection":
		_, index, err := s.helpCollections(ctx, q, c, locale)
		if err != nil {
			return publicEntry{}, "", err
		}
		var col *helpColl
		for _, candidate := range index {
			if candidate.Number == rt.number {
				col = candidate
			}
		}
		if col == nil {
			return publicEntry{}, "", errHelpPageGone
		}
		if rt.slug != col.Slug {
			return publicEntry{}, "", helpRedirect(s.helpCollectionURL(c, locale, col))
		}
		e, err := s.helpCached(c, prefix+strconv.Itoa(int(rt.number)), helpPageType, func() ([]byte, error) {
			return render(s.helpCollectionView(ctx, q, c, locale, col, index))
		})
		return e, csp, err
	case "article":
		a, err := q.GetPublicHelpArticleByNumber(ctx, store.GetPublicHelpArticleByNumberParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, Number: rt.number})
		if store.IsNotFound(err) {
			return publicEntry{}, "", errHelpPageGone
		}
		if err != nil {
			return publicEntry{}, "", err
		}
		if rt.slug != a.Slug {
			return publicEntry{}, "", helpRedirect(s.helpArticleURL(c, a.Number, locale, a.Slug))
		}
		if r.Method == http.MethodGet {
			s.countHelpView(ctx, r, c, a.ID, locale)
		}
		thanks := r.URL.Query().Get("thanks")
		if thanks != "up" && thanks != "down" {
			thanks = ""
		}
		e, err := s.helpCached(c, prefix+strconv.Itoa(int(rt.number))+"|"+a.Slug+"|"+thanks, helpPageType, func() ([]byte, error) {
			_, index, err := s.helpCollections(ctx, q, c, locale)
			if err != nil {
				return nil, err
			}
			return render(s.helpArticleView(ctx, q, c, locale, a, index, thanks))
		})
		return e, csp, err
	case "search":
		return s.helpSearchAnswer(ctx, r, c, locale, csp)
	}
	return publicEntry{}, "", errHelpPageGone
}

func (s *Server) helpSearchAnswer(ctx context.Context, r *http.Request, c store.HelpCenter, locale, csp string) (publicEntry, string, error) {
	q := s.st.Queries
	raw := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(raw) > maxHelpQueryLen {
		raw = string([]rune(raw)[:maxHelpQueryLen])
	}
	words := helpQueryWords(raw)
	prefix := c.ID.String() + "|search|" + locale + "|" + strings.Join(words, " ")
	if len(words) == 0 {
		e, err := s.helpCached(c, prefix, helpPageType, func() ([]byte, error) {
			v, err := s.helpSearchView(ctx, q, c, locale, "", nil, oas.HelpSearchResults{Items: []oas.HelpSearchResult{}})
			if err != nil {
				return nil, err
			}
			return renderHelpPage(v)
		})
		return e, csp, err
	}
	ip := rateIP(s.clientIP(r))
	if err := s.rateLimit(ctx,
		rateCheck{"help-search:ip:" + ip + ":" + c.ID.String(), limitHelpSearchPerIP},
		rateCheck{"help-search:center:" + c.ID.String(), limitHelpSearchCenter},
	); err != nil {
		return publicEntry{}, "", err
	}
	now := s.now()
	for i, found := range []bool{true, false} {
		if e, ok := s.helpCache.get(prefix+"|"+strconv.Itoa(i), c.ChangedAt, now); ok {
			s.countHelpSearch(c, locale, raw, found)
			return e, csp, nil
		}
	}
	res, err := s.searchHelp(ctx, c, locale, words, false, helpSearchShown)
	if err != nil {
		return publicEntry{}, "", err
	}
	v, err := s.helpSearchView(ctx, q, c, locale, raw, words, res)
	if err != nil {
		return publicEntry{}, "", err
	}
	body, err := renderHelpPage(v)
	if err != nil {
		return publicEntry{}, "", err
	}
	found := len(res.Items) > 0
	s.countHelpSearch(c, locale, raw, found)
	suffix := "|1"
	if !found {
		suffix = "|0"
	}
	e := publicEntry{changed: c.ChangedAt, at: now, ctype: helpPageType, body: body}
	s.helpCache.put(prefix+suffix, e)
	return e, csp, nil
}

func (s *Server) serveHelpPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, ok := s.publicHelpCenter(w, r)
	if !ok {
		return
	}
	rt, ok := parseHelpRoute(r.PathValue("rest"))
	if !ok || !rt.helpful || !slices.Contains(c.Locales, rt.locale) {
		helpGone(w, c.DefaultLocale)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		writeProblem(w, errValidation("the form is not valid"))
		return
	}
	rating, previous := oas.PageRating(r.PostForm.Get("rating")), oas.PageRating(r.PostForm.Get("previous"))
	if !rating.Valid() || (previous != "" && !previous.Valid()) {
		writeProblem(w, errValidation("rating and previous must be up or down"))
		return
	}
	ip := rateIP(s.clientIP(r))
	if err := s.rateLimit(ctx,
		rateCheck{"help-reaction:ip:" + ip, limitSessionPerIP},
		rateCheck{"help-reaction:center:" + c.ID.String(), limitSessionPerChannel},
	); err != nil {
		s.writeError(w, r, err)
		return
	}
	a, err := s.st.GetPublicHelpArticleByNumber(ctx, store.GetPublicHelpArticleByNumberParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: rt.locale, Number: rt.number})
	if store.IsNotFound(err) {
		helpGone(w, rt.locale)
		return
	}
	if err != nil {
		s.log.ErrorContext(ctx, "help reaction", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	s.countHelpReaction(c, a.ID, rt.locale, rating, previous)
	w.Header().Set("Cache-Control", "no-store")
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, s.helpArticleURL(c, a.Number, rt.locale, a.Slug)+"?thanks="+string(rating)+"#helpful", http.StatusSeeOther)
}

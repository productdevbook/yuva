package api

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/store"
)

type helpColl struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Number      int32
	Icon        *string
	Name        string
	Description string
	Slug        string
	Own, Total  int32
	Subs        []*helpColl
}

type helpArt struct {
	ID           uuid.UUID
	Number       int32
	CollectionID *uuid.UUID
	Locale       string
	Title        string
	Slug         string
	Excerpt      string
}

// helpServedLocale is the center's locale a visitor's language maps to: exactly, then by base
// language, else the center's default.
func helpServedLocale(c store.HelpCenter, requested string) string {
	return servedLocale(c.Locales, c.DefaultLocale, requested)
}

func servedLocale(locales []string, def, requested string) string {
	want := strings.ToLower(strings.ReplaceAll(requested, "_", "-"))
	if want == "" {
		return def
	}
	for _, l := range locales {
		if strings.ToLower(l) == want {
			return l
		}
	}
	base, _, _ := strings.Cut(want, "-")
	for _, l := range locales {
		if b, _, _ := strings.Cut(strings.ToLower(l), "-"); b == base {
			return l
		}
	}
	return def
}

func (s *Server) helpHomeURL(c store.HelpCenter, locale string) string {
	return s.helpURL(c, "/"+locale+"/")
}

func (s *Server) helpCollectionURL(c store.HelpCenter, locale string, col *helpColl) string {
	return s.helpURL(c, "/"+locale+"/collections/"+strconv.Itoa(int(col.Number))+"-"+col.Slug)
}

func (s *Server) helpSearchURL(c store.HelpCenter, locale string) string {
	return s.helpURL(c, "/"+locale+"/search")
}

func (s *Server) helpArtURL(c store.HelpCenter, a helpArt) string {
	return s.helpArticleURL(c, a.Number, a.Locale, a.Slug)
}

// helpCollections lists the collections a visitor sees in a locale: those with a translation there
// and a published article of their own, or a sub-collection that has one. Top-level collections
// come first by position, each with its visible sub-collections.
func (s *Server) helpCollections(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string) ([]*helpColl, map[uuid.UUID]*helpColl, error) {
	rows, err := q.ListPublicHelpCollections(ctx, store.ListPublicHelpCollectionsParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale})
	if err != nil {
		return nil, nil, err
	}
	all := make([]*helpColl, len(rows))
	byID := map[uuid.UUID]*helpColl{}
	for i, r := range rows {
		all[i] = &helpColl{ID: r.ID, ParentID: r.ParentID, Number: r.Number, Icon: r.Icon, Name: r.Name, Description: r.Description, Slug: r.Slug, Own: r.ArticleCount}
		byID[r.ID] = all[i]
	}
	var tops []*helpColl
	for _, col := range all {
		if col.ParentID == nil {
			tops = append(tops, col)
			continue
		}
		if parent := byID[*col.ParentID]; parent != nil && col.Own > 0 {
			parent.Subs = append(parent.Subs, col)
		}
	}
	visible := tops[:0:0]
	index := map[uuid.UUID]*helpColl{}
	for _, col := range tops {
		col.Total = col.Own
		for _, sub := range col.Subs {
			col.Total += sub.Own
			sub.Total = sub.Own
			index[sub.ID] = sub
		}
		if col.Total > 0 {
			visible = append(visible, col)
			index[col.ID] = col
		}
	}
	return visible, index, nil
}

func helpArtFrom(id uuid.UUID, number int32, collection *uuid.UUID, locale, title, slug, excerpt string) helpArt {
	return helpArt{ID: id, Number: number, CollectionID: collection, Locale: locale, Title: title, Slug: slug, Excerpt: excerpt}
}

func (s *Server) helpPublishedArticles(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string, collection *uuid.UUID) ([]helpArt, error) {
	rows, err := q.ListPublicHelpArticles(ctx, store.ListPublicHelpArticlesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, CollectionID: collection})
	if err != nil {
		return nil, err
	}
	out := make([]helpArt, len(rows))
	for i, r := range rows {
		out[i] = helpArtFrom(r.ID, r.Number, r.CollectionID, r.Locale, r.Title, r.Slug, r.Excerpt)
	}
	return out, nil
}

func (s *Server) helpPopular(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string, lim int32) ([]helpArt, error) {
	rows, err := q.ListPopularHelpArticles(ctx, store.ListPopularHelpArticlesParams{
		WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, Since: utcDay(s.now()).AddDate(0, 0, -helpViewsWindow+1), Lim: lim,
	})
	if err != nil {
		return nil, err
	}
	out := make([]helpArt, len(rows))
	for i, r := range rows {
		out[i] = helpArtFrom(r.ID, r.Number, r.CollectionID, r.Locale, r.Title, r.Slug, r.Excerpt)
	}
	return out, nil
}

func (s *Server) helpRelated(ctx context.Context, q *store.Queries, c store.HelpCenter, locale string, id uuid.UUID, collection *uuid.UUID) ([]helpArt, error) {
	if collection == nil {
		return nil, nil
	}
	rows, err := q.ListRelatedHelpArticles(ctx, store.ListRelatedHelpArticlesParams{
		WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, ID: id, CollectionID: collection, Lim: 3,
	})
	if err != nil {
		return nil, err
	}
	out := make([]helpArt, len(rows))
	for i, r := range rows {
		out[i] = helpArtFrom(r.ID, r.Number, r.CollectionID, r.Locale, r.Title, r.Slug, r.Excerpt)
	}
	return out, nil
}

// helpPathMatches matches a page path against an article's patterns: exactly, or by prefix when the
// pattern ends in *.
func helpPathMatches(patterns []string, path string) bool {
	return slices.ContainsFunc(patterns, func(p string) bool {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			return strings.HasPrefix(path, prefix)
		}
		return p == path || strings.TrimSuffix(p, "/") == strings.TrimSuffix(path, "/")
	})
}

func (s *Server) helpSuggested(ctx context.Context, q *store.Queries, c store.HelpCenter, locale, page string, lim int) ([]helpArt, error) {
	u, err := url.Parse(strings.TrimSpace(page))
	if page == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, nil
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	rows, err := q.ListHelpArticlesWithPaths(ctx, store.ListHelpArticlesWithPathsParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale})
	if err != nil {
		return nil, err
	}
	var out []helpArt
	for _, r := range rows {
		if len(out) < lim && helpPathMatches(r.Paths, path) {
			out = append(out, helpArtFrom(r.ID, r.Number, r.CollectionID, r.Locale, r.Title, r.Slug, r.Excerpt))
		}
	}
	return out, nil
}

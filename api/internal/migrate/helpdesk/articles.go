package helpdesk

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HelpLocales are the languages of a help center as the source writes them, and the one it shows
// first. A source without languages (Help Scout Docs) lists none.
type HelpLocales struct {
	All     []string
	Default string
}

// HelpCollection is a category or section of a Zendesk Guide, or a collection or category of Help
// Scout Docs; ParentID is empty at the top.
type HelpCollection struct {
	ID          string
	ParentID    string
	Name        string
	Description string
	URL         string
	Position    int
}

// HelpArticle is one article in one language. CollectionID is the deepest collection it is in.
// LinkKey is what links between articles name it by.
type HelpArticle struct {
	ID           string
	LinkKey      string
	CollectionID string
	Title        string
	HTML         string
	Keywords     []string
	URL          string
	Position     int
	Draft        bool
	UpdatedAt    time.Time
}

// ArticleCursor is where a listing of articles stands.
type ArticleCursor struct {
	After      string `json:"after,omitempty"`
	Collection int    `json:"collection,omitempty"`
	Page       int    `json:"page,omitempty"`
}

type ArticlePage struct {
	Articles []HelpArticle
	Next     ArticleCursor
	End      bool
}

// ArticleSource is a help center Yuva reads articles from, with GET only.
type ArticleSource interface {
	HelpLocales(ctx context.Context) (HelpLocales, error)
	HelpCollections(ctx context.Context, locale string) ([]HelpCollection, error)
	HelpArticles(ctx context.Context, locale string, c ArticleCursor) (ArticlePage, error)
	// HelpImage fetches an image the articles show: with the source's token from its own hosts,
	// without it from anywhere else.
	HelpImage(ctx context.Context, rawURL string) ([]byte, error)
	// HelpBase resolves the relative addresses of the articles' HTML.
	HelpBase() *url.URL
	// HelpLink tells which article a link in an article's HTML names, in which language ("" when
	// the link does not say).
	HelpLink(href string) (key, locale string, ok bool)
}

// FlattenHelpCollections limits a tree to the two levels a help center has: a collection nested
// deeper becomes a sub-collection of its top-level ancestor, named "Parent / Child".
func FlattenHelpCollections(in []HelpCollection) []HelpCollection {
	byID := make(map[string]HelpCollection, len(in))
	for _, c := range in {
		byID[c.ID] = c
	}
	out := make([]HelpCollection, 0, len(in))
	for _, c := range in {
		var chain []HelpCollection
		for cur := c; cur.ParentID != "" && len(chain) < 20; {
			p, ok := byID[cur.ParentID]
			if !ok {
				break
			}
			chain = append(chain, p)
			cur = p
		}
		if len(chain) > 1 {
			names := make([]string, 0, len(chain))
			for i := len(chain) - 2; i >= 0; i-- {
				names = append(names, chain[i].Name)
			}
			c.Name = strings.Join(append(names, c.Name), " / ")
			c.ParentID = chain[len(chain)-1].ID
		}
		out = append(out, c)
	}
	return out
}

var zendeskArticleLink = regexp.MustCompile(`^/hc/(?:([a-zA-Z]{2,3}(?:-[a-zA-Z0-9]+)*)/)?articles/(\d+)`)

func (z *Zendesk) HelpBase() *url.URL { return &url.URL{Scheme: "https", Host: z.host} }

func (z *Zendesk) helpURL(path string, q url.Values) string {
	return z.url("/api/v2/help_center"+path, q)
}

func (z *Zendesk) HelpLocales(ctx context.Context) (HelpLocales, error) {
	var out struct {
		Locales []string `json:"locales"`
		Default string   `json:"default_locale"`
	}
	if err := z.c.getJSON(ctx, ClassAPI, z.helpURL("/locales", nil), &out); err != nil {
		return HelpLocales{}, err
	}
	return HelpLocales{All: out.Locales, Default: out.Default}, nil
}

type zdHelpPaging struct {
	Meta struct {
		HasMore     bool   `json:"has_more"`
		AfterCursor string `json:"after_cursor"`
	} `json:"meta"`
}

func sameLocale(item, want string) bool { return item == "" || strings.EqualFold(item, want) }

// HelpCollections lists the categories and then the sections that exist in locale; a section
// names its category, or its parent section when sections are nested.
func (z *Zendesk) HelpCollections(ctx context.Context, locale string) ([]HelpCollection, error) {
	var out []HelpCollection
	q := url.Values{"page[size]": {strconv.Itoa(zendeskPage)}}
	for {
		var page struct {
			zdHelpPaging
			Categories []struct {
				ID          int64  `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				HTMLURL     string `json:"html_url"`
				Position    int    `json:"position"`
				Locale      string `json:"locale"`
			} `json:"categories"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.helpURL("/"+url.PathEscape(locale)+"/categories", q), &page); err != nil {
			return nil, err
		}
		for _, c := range page.Categories {
			if sameLocale(c.Locale, locale) {
				out = append(out, HelpCollection{ID: "c" + idString(c.ID), Name: c.Name, Description: c.Description, URL: c.HTMLURL, Position: c.Position})
			}
		}
		if !page.Meta.HasMore || page.Meta.AfterCursor == "" {
			break
		}
		q.Set("page[after]", page.Meta.AfterCursor)
	}
	q = url.Values{"page[size]": {strconv.Itoa(zendeskPage)}}
	for {
		var page struct {
			zdHelpPaging
			Sections []struct {
				ID          int64  `json:"id"`
				CategoryID  int64  `json:"category_id"`
				ParentID    *int64 `json:"parent_section_id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				HTMLURL     string `json:"html_url"`
				Position    int    `json:"position"`
				Locale      string `json:"locale"`
			} `json:"sections"`
		}
		if err := z.c.getJSON(ctx, ClassAPI, z.helpURL("/"+url.PathEscape(locale)+"/sections", q), &page); err != nil {
			return nil, err
		}
		for _, s := range page.Sections {
			if !sameLocale(s.Locale, locale) {
				continue
			}
			parent := "c" + idString(s.CategoryID)
			if p := valueOr(s.ParentID); p != 0 {
				parent = "s" + idString(p)
			}
			out = append(out, HelpCollection{ID: "s" + idString(s.ID), ParentID: parent, Name: s.Name, Description: s.Description, URL: s.HTMLURL, Position: s.Position})
		}
		if !page.Meta.HasMore || page.Meta.AfterCursor == "" {
			return out, nil
		}
		q.Set("page[after]", page.Meta.AfterCursor)
	}
}

// HelpArticles lists a page of the articles that exist in locale; an article the account shows
// in another language instead is left out.
func (z *Zendesk) HelpArticles(ctx context.Context, locale string, c ArticleCursor) (ArticlePage, error) {
	q := url.Values{"page[size]": {strconv.Itoa(zendeskPage)}}
	if c.After != "" {
		q.Set("page[after]", c.After)
	}
	var page struct {
		zdHelpPaging
		Articles []struct {
			ID        int64     `json:"id"`
			Title     string    `json:"title"`
			Body      string    `json:"body"`
			HTMLURL   string    `json:"html_url"`
			SectionID int64     `json:"section_id"`
			Locale    string    `json:"locale"`
			Draft     bool      `json:"draft"`
			Position  int       `json:"position"`
			Labels    []string  `json:"label_names"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"articles"`
	}
	if err := z.c.getJSON(ctx, ClassAPI, z.helpURL("/"+url.PathEscape(locale)+"/articles", q), &page); err != nil {
		return ArticlePage{}, err
	}
	out := ArticlePage{Next: c, End: !page.Meta.HasMore || page.Meta.AfterCursor == ""}
	out.Next.After = page.Meta.AfterCursor
	for _, a := range page.Articles {
		if !sameLocale(a.Locale, locale) {
			continue
		}
		out.Articles = append(out.Articles, HelpArticle{
			ID: idString(a.ID), LinkKey: idString(a.ID), CollectionID: "s" + idString(a.SectionID), Title: a.Title, HTML: a.Body, Keywords: a.Labels,
			URL: a.HTMLURL, Position: a.Position, Draft: a.Draft, UpdatedAt: a.UpdatedAt.UTC(),
		})
	}
	return out, nil
}

func (z *Zendesk) HelpImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, &RefusedError{Method: "GET", Host: rawURL}
	}
	return z.c.get(ctx, z.c.files, ClassAPI, u.String(), "image/*,*/*;q=0.5")
}

func (z *Zendesk) HelpLink(href string) (string, string, bool) {
	u, err := url.Parse(href)
	if err != nil || (u.Host != "" && !strings.EqualFold(u.Hostname(), z.host)) {
		return "", "", false
	}
	m := zendeskArticleLink.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return m[2], m[1], true
}

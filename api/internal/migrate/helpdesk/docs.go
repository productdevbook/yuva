package helpdesk

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	HelpScoutDocsHost = "docsapi.helpscout.net"
	docsPageSize      = 25
	docsListPage      = 100
)

// helpScoutDocsDesk is Help Scout Docs: its API takes a Docs API key as the user name of Basic
// auth, not the Mailbox sign-in, and answers 2,000 requests in 10 minutes. It is not one of the
// help desks a migration moves tickets from, so it is not in the registry.
var helpScoutDocsDesk = Desk{
	Source: "helpscout_docs", Name: "Help Scout Docs", Auth: AuthToken, Account: AccountNone,
	APIHosts:      func(Account) Hosts { return Hosts{Exact: []string{HelpScoutDocsHost}} },
	FilesAnywhere: true, FilesNoToken: true,
	Send:         Send{Scheme: SendBasic, Password: "X"},
	Limits:       []Limit{{Window: 10 * time.Minute, Default: 1000}},
	RetryHeaders: []string{"Retry-After"},
}

// HelpScoutDocs reads the Docs sites of one Help Scout account with a Docs API key. The key goes
// only to docsapi.helpscout.net; images come from wherever the articles show them, without it.
type HelpScoutDocs struct {
	c           *client
	site        string
	collections []docsCollection
	listed      bool
}

type docsCollection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PublicURL   string `json:"publicUrl"`
	Order       int    `json:"order"`
}

func NewHelpScoutDocs(cfg Config, key string, site string, budget *Budget) *HelpScoutDocs {
	return &HelpScoutDocs{site: site, c: helpScoutDocsDesk.client(cfg, Account{}, Credentials{Token: func() string { return key }}, budget)}
}

func (h *HelpScoutDocs) url(path string, q url.Values) string {
	return withQuery("https://"+HelpScoutDocsHost+path, q)
}

type docsList[T any] struct {
	Page  int `json:"page"`
	Pages int `json:"pages"`
	Items []T `json:"items"`
}

func (l docsList[T]) more() bool { return l.Page < l.Pages }

func (h *HelpScoutDocs) HelpBase() *url.URL { return nil }

// HelpLocales: Docs sites have no languages.
func (h *HelpScoutDocs) HelpLocales(ctx context.Context) (HelpLocales, error) {
	var out struct {
		Sites docsList[struct {
			ID string `json:"id"`
		}] `json:"sites"`
	}
	if err := h.c.getJSON(ctx, ClassAPI, h.url("/v1/sites", url.Values{"page": {"1"}}), &out); err != nil {
		return HelpLocales{}, err
	}
	return HelpLocales{}, nil
}

func (h *HelpScoutDocs) listCollections(ctx context.Context) ([]docsCollection, error) {
	if h.listed {
		return h.collections, nil
	}
	var all []docsCollection
	for n := 1; ; n++ {
		q := url.Values{"page": {strconv.Itoa(n)}, "pageSize": {strconv.Itoa(docsListPage)}, "visibility": {"all"}}
		if h.site != "" {
			q.Set("siteId", h.site)
		}
		var out struct {
			Collections docsList[docsCollection] `json:"collections"`
		}
		if err := h.c.getJSON(ctx, ClassAPI, h.url("/v1/collections", q), &out); err != nil {
			return nil, err
		}
		all = append(all, out.Collections.Items...)
		if !out.Collections.more() {
			break
		}
	}
	h.collections, h.listed = all, true
	return all, nil
}

// HelpCollections are the collections with their categories as sub-collections.
func (h *HelpScoutDocs) HelpCollections(ctx context.Context, _ string) ([]HelpCollection, error) {
	cols, err := h.listCollections(ctx)
	if err != nil {
		return nil, err
	}
	var out []HelpCollection
	for _, c := range cols {
		out = append(out, HelpCollection{ID: c.ID, Name: c.Name, Description: c.Description, URL: c.PublicURL, Position: c.Order})
		for n := 1; ; n++ {
			var page struct {
				Categories docsList[struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Description string `json:"description"`
					PublicURL   string `json:"publicUrl"`
					Order       int    `json:"order"`
				}] `json:"categories"`
			}
			q := url.Values{"page": {strconv.Itoa(n)}, "pageSize": {strconv.Itoa(docsListPage)}}
			if err := h.c.getJSON(ctx, ClassAPI, h.url("/v1/collections/"+url.PathEscape(c.ID)+"/categories", q), &page); err != nil {
				return nil, err
			}
			for _, cat := range page.Categories.Items {
				out = append(out, HelpCollection{ID: cat.ID, ParentID: c.ID, Name: cat.Name, Description: cat.Description, URL: cat.PublicURL, Position: cat.Order})
			}
			if !page.Categories.more() {
				break
			}
		}
	}
	return out, nil
}

// HelpArticles reads a page of the article list of one collection and then each article, which is
// the only answer that holds its text.
func (h *HelpScoutDocs) HelpArticles(ctx context.Context, _ string, c ArticleCursor) (ArticlePage, error) {
	cols, err := h.listCollections(ctx)
	if err != nil {
		return ArticlePage{}, err
	}
	if c.Collection >= len(cols) {
		return ArticlePage{End: true, Next: c}, nil
	}
	n := max(c.Page, 1)
	col := cols[c.Collection]
	var list struct {
		Articles docsList[struct {
			ID string `json:"id"`
		}] `json:"articles"`
	}
	q := url.Values{"page": {strconv.Itoa(n)}, "pageSize": {strconv.Itoa(docsPageSize)}, "status": {"all"}}
	if err := h.c.getJSON(ctx, ClassAPI, h.url("/v1/collections/"+url.PathEscape(col.ID)+"/articles", q), &list); err != nil {
		return ArticlePage{}, err
	}
	out := ArticlePage{}
	for i, s := range list.Articles.Items {
		var one struct {
			Article struct {
				ID           string    `json:"id"`
				Number       int       `json:"number"`
				CollectionID string    `json:"collectionId"`
				Status       string    `json:"status"`
				Name         string    `json:"name"`
				Text         string    `json:"text"`
				PublicURL    string    `json:"publicUrl"`
				Categories   []string  `json:"categories"`
				Keywords     []string  `json:"keywords"`
				UpdatedAt    time.Time `json:"updatedAt"`
			} `json:"article"`
		}
		if err := h.c.getJSON(ctx, ClassAPI, h.url("/v1/articles/"+url.PathEscape(s.ID), nil), &one); err != nil {
			return ArticlePage{}, err
		}
		a := one.Article
		parent := col.ID
		if len(a.Categories) > 0 {
			parent = a.Categories[0]
		}
		out.Articles = append(out.Articles, HelpArticle{
			ID: a.ID, LinkKey: strconv.Itoa(a.Number), CollectionID: parent, Title: a.Name, HTML: a.Text, Keywords: a.Keywords, URL: a.PublicURL,
			Position: (n-1)*docsPageSize + i, Draft: a.Status != "published", UpdatedAt: a.UpdatedAt.UTC(),
		})
	}
	out.Next = ArticleCursor{Collection: c.Collection, Page: n + 1}
	if !list.Articles.more() {
		out.Next = ArticleCursor{Collection: c.Collection + 1, Page: 1}
	}
	out.End = out.Next.Collection >= len(cols)
	return out, nil
}

func (h *HelpScoutDocs) HelpImage(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, &RefusedError{Method: "GET", Host: rawURL}
	}
	return h.c.get(ctx, h.c.files, ClassAPI, u.String(), "image/*,*/*;q=0.5")
}

var docsArticleLink = regexp.MustCompile(`^/article/(\d+)`)

func (h *HelpScoutDocs) HelpLink(href string) (string, string, bool) {
	u, err := url.Parse(href)
	if err != nil || (u.Host != "" && !strings.HasSuffix(strings.ToLower(u.Hostname()), ".helpscoutdocs.com")) {
		return "", "", false
	}
	m := docsArticleLink.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return m[1], "", true
}

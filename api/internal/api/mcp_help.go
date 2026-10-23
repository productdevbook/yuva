package api

import (
	"context"
	"encoding/json"
	"strings"
	"uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
)

const (
	mcpHelpPrefix    = mcpURIPrefix + "help-center/"
	helpDraftedNote  = "Saved as a draft. A member reviews and publishes it in Yuva; this connection cannot publish."
	helpResourceTop  = 100
	helpSearchMCPMax = 20
)

type mcpHelpCenter struct {
	ID                uuid.UUID `json:"id"`
	InboxID           uuid.UUID `json:"inbox_id"`
	Name              string    `json:"name"`
	URL               string    `json:"url" jsonschema:"the public home page"`
	DefaultLocale     string    `json:"default_locale"`
	Locales           []string  `json:"locales" jsonschema:"the languages articles can be written in"`
	Published         bool      `json:"published" jsonschema:"false: nobody outside the team sees the center yet"`
	Collections       int32     `json:"collections"`
	Articles          int32     `json:"articles"`
	PublishedArticles int32     `json:"published_articles"`
}

func helpCenterOutMCP(c oas.HelpCenter) mcpHelpCenter {
	return mcpHelpCenter{
		ID: c.Id, InboxID: c.InboxId, Name: c.Name, URL: c.Url, DefaultLocale: c.DefaultLocale, Locales: c.Locales, Published: c.Published,
		Collections: c.Counts.Collections, Articles: c.Counts.Articles, PublishedArticles: c.Counts.PublishedArticles,
	}
}

type helpCenterIn struct {
	CenterID uuid.UUID `json:"center_id" jsonschema:"a help center from list_help_centers"`
}

type searchHelpIn struct {
	CenterID uuid.UUID `json:"center_id" jsonschema:"a help center from list_help_centers"`
	Query    string    `json:"query" jsonschema:"one to eight words; the last may be the start of a word"`
	Locale   string    `json:"locale,omitempty" jsonschema:"the center's default locale when empty"`
	Drafts   bool      `json:"drafts,omitempty" jsonschema:"true searches the drafts members are writing too, not only what is published"`
	Limit    int32     `json:"limit,omitempty" jsonschema:"1 to 20, default 10"`
}

type mcpHelpHit struct {
	ArticleID    uuid.UUID  `json:"article_id"`
	CollectionID *uuid.UUID `json:"collection_id,omitempty"`
	Number       int32      `json:"number"`
	Locale       string     `json:"locale"`
	Title        string     `json:"title"`
	Excerpt      string     `json:"excerpt"`
	State        string     `json:"state" jsonschema:"draft, published, or changed (published with a newer draft)"`
	URL          string     `json:"url,omitempty" jsonschema:"the public page, when published"`
}

type mcpHelpSearch struct {
	Items   []mcpHelpHit `json:"items"`
	Partial bool         `json:"partial" jsonschema:"no article had every word; these have some of them"`
}

type getHelpArticleIn struct {
	CenterID  uuid.UUID `json:"center_id"`
	ArticleID uuid.UUID `json:"article_id" jsonschema:"an article from search_help_articles"`
}

type importHelpIn struct {
	CenterID        uuid.UUID `json:"center_id" jsonschema:"a help center from list_help_centers"`
	MigrationID     uuid.UUID `json:"migration_id" jsonschema:"a Zendesk migration of the workspace that is signed in"`
	OverwriteEdited bool      `json:"overwrite_edited,omitempty" jsonschema:"replace drafts a member edited since the last import; off keeps them"`
}

type getHelpImportIn struct {
	CenterID uuid.UUID `json:"center_id"`
	ImportID uuid.UUID `json:"import_id" jsonschema:"an import from import_help_articles"`
}

type mcpHelpImport struct {
	ID             uuid.UUID            `json:"id"`
	State          string               `json:"state"`
	Phase          string               `json:"phase"`
	Counts         oas.HelpImportCounts `json:"counts"`
	SkippedLocales []string             `json:"skipped_locales"`
	Redirects      int32                `json:"redirects" jsonschema:"old URL to new URL rows; a member downloads them in Yuva"`
	Error          string               `json:"error,omitempty"`
	Note           string               `json:"note" jsonschema:"what happens next"`
}

func helpImportOutMCP(h oas.HelpImport) mcpHelpImport {
	return mcpHelpImport{ID: h.Id, State: string(h.State), Phase: string(h.Phase), Counts: h.Counts, SkippedLocales: h.SkippedLocales, Redirects: h.Redirects,
		Error: deref(h.Error), Note: "Everything is imported as drafts. A member reviews and publishes them in Yuva and sets up the redirects from the old URLs."}
}

type mcpHelpTranslation struct {
	Locale        string `json:"locale"`
	State         string `json:"state" jsonschema:"draft, published, or changed (published with a newer draft)"`
	Outdated      bool   `json:"outdated,omitempty" jsonschema:"the default language was published after this translation was saved"`
	Version       int32  `json:"version" jsonschema:"send it as base_version to update_help_article_draft"`
	Title         string `json:"title"`
	Keywords      string `json:"keywords,omitempty"`
	Body          string `json:"body" jsonschema:"the draft, in Markdown"`
	PublishedBody string `json:"published_body,omitempty" jsonschema:"what the public sees, in Markdown, when published"`
	URL           string `json:"url,omitempty"`
}

type mcpHelpArticle struct {
	ID           uuid.UUID            `json:"id"`
	CenterID     uuid.UUID            `json:"center_id"`
	Number       int32                `json:"number"`
	CollectionID *uuid.UUID           `json:"collection_id,omitempty"`
	Paths        []string             `json:"paths,omitempty" jsonschema:"page paths where the widget suggests it"`
	Translations []mcpHelpTranslation `json:"translations"`
}

func helpArticleOutMCP(a oas.HelpArticle) mcpHelpArticle {
	out := mcpHelpArticle{ID: a.Id, CenterID: a.CenterId, Number: a.Number, CollectionID: a.CollectionId, Paths: a.Paths, Translations: []mcpHelpTranslation{}}
	for _, t := range a.Translations {
		out.Translations = append(out.Translations, helpTranslationOutMCP(t))
	}
	return out
}

func helpTranslationOutMCP(t oas.HelpArticleTranslation) mcpHelpTranslation {
	out := mcpHelpTranslation{
		Locale: t.Locale, State: string(t.State), Outdated: t.Outdated, Version: t.Version, Title: t.Title, Keywords: t.Keywords, Body: t.Body, URL: deref(t.Url),
	}
	if t.Published != nil {
		out.PublishedBody = t.Published.Body
	}
	return out
}

type draftHelpIn struct {
	CenterID     uuid.UUID  `json:"center_id"`
	ArticleID    *uuid.UUID `json:"article_id,omitempty" jsonschema:"add a translation to this article; omit to start a new article"`
	Locale       string     `json:"locale,omitempty" jsonschema:"the language of this text; the center's default locale when empty"`
	Title        string     `json:"title" jsonschema:"3 to 200 characters"`
	Body         string     `json:"body" jsonschema:"Markdown written for customers, up to 100000 characters"`
	Keywords     string     `json:"keywords,omitempty" jsonschema:"extra words the article should be found by, such as synonyms"`
	CollectionID *uuid.UUID `json:"collection_id,omitempty" jsonschema:"a new article only"`
}

type updateHelpDraftIn struct {
	CenterID    uuid.UUID `json:"center_id"`
	ArticleID   uuid.UUID `json:"article_id"`
	Locale      string    `json:"locale"`
	BaseVersion int32     `json:"base_version" jsonschema:"the version from get_help_article; a draft saved by someone else since is refused"`
	Title       *string   `json:"title,omitempty" jsonschema:"unchanged when absent"`
	Body        *string   `json:"body,omitempty" jsonschema:"unchanged when absent"`
	Keywords    *string   `json:"keywords,omitempty" jsonschema:"unchanged when absent"`
}

type mcpHelpDraft struct {
	ArticleID uuid.UUID `json:"article_id"`
	CenterID  uuid.UUID `json:"center_id"`
	Number    int32     `json:"number"`
	Locale    string    `json:"locale"`
	Version   int32     `json:"version"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	Note      string    `json:"note"`
}

type helpGapsIn struct {
	CenterID uuid.UUID `json:"center_id"`
	Days     int32     `json:"days,omitempty" jsonschema:"7, 30 or 90; default 30"`
}

type mcpHelpDownArticle struct {
	ArticleID   uuid.UUID `json:"article_id"`
	Number      int32     `json:"number"`
	Title       string    `json:"title"`
	Views       int64     `json:"views"`
	HelpfulUp   int64     `json:"helpful_up"`
	HelpfulDown int64     `json:"helpful_down"`
}

type mcpHelpQuery struct {
	Query    string `json:"query" jsonschema:"normalized; (scrubbed) when it looked like an address or a number"`
	Locale   string `json:"locale"`
	Searches int64  `json:"searches"`
}

type mcpHelpGapContent struct {
	NoResultQueries []mcpHelpQuery `json:"no_result_queries" jsonschema:"what visitors searched for and found nothing, most frequent first"`
}

type mcpHelpGaps struct {
	Days             int32                `json:"days"`
	Searches         int64                `json:"searches"`
	NoResultSearches int64                `json:"no_result_searches"`
	MostDown         []mcpHelpDownArticle `json:"most_down" jsonschema:"articles that got the most no answers to was this helpful"`
	CustomerContent  mcpHelpGapContent    `json:"customer_content"`
}

type mcpHelpCenterResource struct {
	Center      mcpHelpCenter            `json:"center"`
	Collections []mcpHelpCollection      `json:"collections"`
	Articles    []mcpHelpArticleListItem `json:"articles"`
}

type mcpHelpCollection struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parent_id,omitempty"`
	Number   int32      `json:"number"`
	Names    []string   `json:"names" jsonschema:"one per language: locale and name"`
	Articles int32      `json:"articles"`
}

type mcpHelpArticleListItem struct {
	ID           uuid.UUID  `json:"id"`
	Number       int32      `json:"number"`
	CollectionID *uuid.UUID `json:"collection_id,omitempty"`
	Titles       []string   `json:"titles" jsonschema:"one per language: locale, state and title"`
}

func (c *mcpCall) helpCenter(ctx context.Context, id uuid.UUID) (oas.HelpCenter, error) {
	res, err := c.s.GetHelpCenter(ctx, oas.GetHelpCenterRequestObject{CenterId: id})
	if err != nil {
		return oas.HelpCenter{}, err
	}
	return oas.HelpCenter(res.(oas.GetHelpCenter200JSONResponse)), nil
}

func (c *mcpCall) helpDraft(a oas.HelpArticle, locale string) (mcpHelpDraft, error) {
	for _, t := range a.Translations {
		if t.Locale == locale {
			return mcpHelpDraft{ArticleID: a.Id, CenterID: a.CenterId, Number: a.Number, Locale: t.Locale, Version: t.Version, State: string(t.State), Title: t.Title, Note: helpDraftedNote}, nil
		}
	}
	return mcpHelpDraft{}, errInternal
}

type createHelpCenterIn struct {
	InboxID       uuid.UUID `json:"inbox_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug" jsonschema:"a-z, 0-9 and inner hyphens, 3 to 48 characters, unique across the server; part of the center's address"`
	Description   string    `json:"description,omitempty" jsonschema:"shown on the center's home page"`
	DefaultLocale string    `json:"default_locale,omitempty" jsonschema:"a BCP 47 tag; the inbox's language when empty"`
	Locales       []string  `json:"locales,omitempty" jsonschema:"the languages articles can be written in; the default one is added"`
	SiteURL       string    `json:"site_url,omitempty" jsonschema:"the product's http(s) address"`
}

type createHelpCollectionIn struct {
	CenterID    uuid.UUID  `json:"center_id" jsonschema:"a help center from list_help_centers"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Locale      string     `json:"locale,omitempty" jsonschema:"the language of name and description; the center's default when empty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty" jsonschema:"a top-level collection of the same center, to make this one a sub-collection"`
}

var mcpHelpTools = []mcpTool{
	defineTool("list_help_centers", "List help centers",
		"Lists the help centers of the inboxes you can see with their languages, whether they are published and how many articles they hold. Maps to GET /v1/help-centers.",
		readOnly, nil, mcpTool{ops: []string{"ListHelpCenters"}},
		func(ctx context.Context, c *mcpCall, _ noInput) (mcpPage[mcpHelpCenter], error) {
			res, err := c.s.ListHelpCenters(ctx, oas.ListHelpCentersRequestObject{})
			if err != nil {
				return mcpPage[mcpHelpCenter]{}, err
			}
			out := mcpPage[mcpHelpCenter]{Items: []mcpHelpCenter{}}
			for _, h := range res.(oas.ListHelpCenters200JSONResponse).Items {
				out.Items = append(out.Items, helpCenterOutMCP(h))
			}
			return out, nil
		}),
	defineTool("search_help_articles", "Search help articles",
		"Searches the articles of a help center in one language, best match first: published articles, or with drafts also the ones members are still writing. Look here before drafting a new article. Maps to GET /v1/help-centers/{id}/search.",
		readOnly, nil, mcpTool{ops: []string{"SearchHelpArticles"}},
		func(ctx context.Context, c *mcpCall, in searchHelpIn) (mcpHelpSearch, error) {
			prm := oas.SearchHelpArticlesParams{Q: in.Query}
			if in.Locale != "" {
				prm.Locale = &in.Locale
			}
			if in.Drafts {
				prm.Drafts = &in.Drafts
			}
			if in.Limit != 0 {
				prm.Limit = &in.Limit
			}
			res, err := c.s.SearchHelpArticles(ctx, oas.SearchHelpArticlesRequestObject{CenterId: in.CenterID, Params: prm})
			if err != nil {
				return mcpHelpSearch{}, err
			}
			r := oas.HelpSearchResults(res.(oas.SearchHelpArticles200JSONResponse))
			out := mcpHelpSearch{Items: []mcpHelpHit{}, Partial: r.Partial}
			for _, h := range r.Items {
				out.Items = append(out.Items, mcpHelpHit{
					ArticleID: h.ArticleId, CollectionID: h.CollectionId, Number: h.Number, Locale: h.Locale, Title: h.Title, Excerpt: h.Excerpt,
					State: string(h.State), URL: deref(h.Url),
				})
			}
			return out, nil
		}),
	defineTool("get_help_article", "Get a help article",
		"A help article with every translation: its draft, its version, and what is published. Maps to GET /v1/help-centers/{id}/articles/{id}.",
		readOnly, nil, mcpTool{ops: []string{"GetHelpArticle"}},
		func(ctx context.Context, c *mcpCall, in getHelpArticleIn) (mcpHelpArticle, error) {
			res, err := c.s.GetHelpArticle(ctx, oas.GetHelpArticleRequestObject{CenterId: in.CenterID, ArticleId: in.ArticleID})
			if err != nil {
				return mcpHelpArticle{}, err
			}
			return helpArticleOutMCP(oas.HelpArticle(res.(oas.GetHelpArticle200JSONResponse))), nil
		}),
	defineTool("draft_help_article", "Draft a help article",
		"Writes a help article as a draft: a new article, or with article_id a new translation of an existing one (in a language it has no translation in yet). Nothing becomes public: a member reviews and publishes it in Yuva. Search first with search_help_articles so the article is not written twice. Maps to POST /v1/help-centers/{id}/articles or PUT .../translations/{locale}.",
		additive, nil, mcpTool{ops: []string{"CreateHelpArticle", "PutHelpArticleTranslation"}},
		func(ctx context.Context, c *mcpCall, in draftHelpIn) (mcpHelpDraft, error) {
			locale := in.Locale
			if locale == "" {
				h, err := c.helpCenter(ctx, in.CenterID)
				if err != nil {
					return mcpHelpDraft{}, err
				}
				locale = h.DefaultLocale
			}
			if in.ArticleID == nil {
				create := oas.HelpArticleCreate{Locale: locale, Title: in.Title, Body: &in.Body, CollectionId: in.CollectionID}
				if in.Keywords != "" {
					create.Keywords = &in.Keywords
				}
				res, err := c.s.CreateHelpArticle(ctx, oas.CreateHelpArticleRequestObject{CenterId: in.CenterID, Body: &create})
				if err != nil {
					return mcpHelpDraft{}, err
				}
				return c.helpDraft(oas.HelpArticle(res.(oas.CreateHelpArticle201JSONResponse)), locale)
			}
			write := oas.HelpArticleTranslationWrite{Title: in.Title, Body: &in.Body}
			if in.Keywords != "" {
				write.Keywords = &in.Keywords
			}
			res, err := c.s.PutHelpArticleTranslation(ctx, oas.PutHelpArticleTranslationRequestObject{CenterId: in.CenterID, ArticleId: *in.ArticleID, Locale: locale, Body: &write})
			if err != nil {
				return mcpHelpDraft{}, err
			}
			return c.helpTranslationDraft(ctx, in.CenterID, *in.ArticleID, res)
		}),
	defineTool("update_help_article_draft", "Update a help article draft",
		"Changes the draft of one translation of a help article; fields you leave out stay as they are. base_version must be the version from get_help_article, so a draft that someone saved since is not overwritten (409 version_conflict). The published text does not change: a member publishes the new draft in Yuva. Maps to PUT /v1/help-centers/{id}/articles/{id}/translations/{locale}.",
		idempotent, nil, mcpTool{ops: []string{"GetHelpArticle", "PutHelpArticleTranslation"}},
		func(ctx context.Context, c *mcpCall, in updateHelpDraftIn) (mcpHelpDraft, error) {
			res, err := c.s.GetHelpArticle(ctx, oas.GetHelpArticleRequestObject{CenterId: in.CenterID, ArticleId: in.ArticleID})
			if err != nil {
				return mcpHelpDraft{}, err
			}
			cur := helpArticleOutMCP(oas.HelpArticle(res.(oas.GetHelpArticle200JSONResponse)))
			var t *mcpHelpTranslation
			for i := range cur.Translations {
				if cur.Translations[i].Locale == in.Locale {
					t = &cur.Translations[i]
				}
			}
			if t == nil {
				return mcpHelpDraft{}, errHelpTranslationGone
			}
			write := oas.HelpArticleTranslationWrite{BaseVersion: in.BaseVersion, Title: t.Title, Body: &t.Body, Keywords: &t.Keywords}
			if in.Title != nil {
				write.Title = *in.Title
			}
			if in.Body != nil {
				write.Body = in.Body
			}
			if in.Keywords != nil {
				write.Keywords = in.Keywords
			}
			put, err := c.s.PutHelpArticleTranslation(ctx, oas.PutHelpArticleTranslationRequestObject{CenterId: in.CenterID, ArticleId: in.ArticleID, Locale: in.Locale, Body: &write})
			if err != nil {
				return mcpHelpDraft{}, err
			}
			return c.helpTranslationDraft(ctx, in.CenterID, in.ArticleID, put)
		}),
	defineTool("import_help_articles", "Import a Zendesk Guide as drafts",
		"Starts reading the Guide of a signed-in Zendesk migration (read-only; needs owner or admin rights and migrations:manage, since it uses the migration's sign-in) into a help center: categories and sections become collections, articles come with every translation the center offers, images are stored, links between articles are rewritten. Everything stays a draft; a member publishes in Yuva. Idempotent: run again to update. Follow it with get_help_import. A Help Scout Docs import needs a Docs API key and is started in the panel. Maps to POST /v1/help-centers/{id}/imports.",
		additive, nil, mcpTool{ops: []string{"StartHelpImport"}},
		func(ctx context.Context, c *mcpCall, in importHelpIn) (mcpHelpImport, error) {
			res, err := c.s.StartHelpImport(ctx, oas.StartHelpImportRequestObject{CenterId: in.CenterID, Body: &oas.HelpImportCreate{
				Source: oas.Zendesk, MigrationId: &in.MigrationID, OverwriteEdited: &in.OverwriteEdited,
			}})
			if err != nil {
				return mcpHelpImport{}, err
			}
			return helpImportOutMCP(oas.HelpImport(res.(oas.StartHelpImport202JSONResponse))), nil
		}),
	defineTool("get_help_import", "Follow a help article import",
		"State, phase and counts of an article import. Maps to GET /v1/help-centers/{id}/imports/{id}.",
		readOnly, nil, mcpTool{ops: []string{"GetHelpImport"}},
		func(ctx context.Context, c *mcpCall, in getHelpImportIn) (mcpHelpImport, error) {
			res, err := c.s.GetHelpImport(ctx, oas.GetHelpImportRequestObject{CenterId: in.CenterID, ImportId: in.ImportID})
			if err != nil {
				return mcpHelpImport{}, err
			}
			return helpImportOutMCP(oas.HelpImport(res.(oas.GetHelpImport200JSONResponse))), nil
		}),
	defineTool("help_search_gaps", "Find gaps in a help center",
		"What visitors searched for and found nothing, most frequent first, and the articles that got the most no answers to was this helpful, over the last 7, 30 or 90 days. Use it to decide which article to write or improve next. Maps to GET /v1/help-centers/{id}/report.",
		readOnly, nil, mcpTool{ops: []string{"GetHelpReport"}},
		func(ctx context.Context, c *mcpCall, in helpGapsIn) (mcpHelpGaps, error) {
			prm := oas.GetHelpReportParams{}
			if in.Days != 0 {
				prm.Days = &in.Days
			}
			res, err := c.s.GetHelpReport(ctx, oas.GetHelpReportRequestObject{CenterId: in.CenterID, Params: prm})
			if err != nil {
				return mcpHelpGaps{}, err
			}
			r := oas.HelpReport(res.(oas.GetHelpReport200JSONResponse))
			out := mcpHelpGaps{Days: r.Days, Searches: r.Searches, NoResultSearches: r.NoResultSearches, MostDown: []mcpHelpDownArticle{},
				CustomerContent: mcpHelpGapContent{NoResultQueries: []mcpHelpQuery{}}}
			for _, a := range r.MostDown {
				out.MostDown = append(out.MostDown, mcpHelpDownArticle{ArticleID: a.ArticleId, Number: a.Number, Title: a.Title, Views: a.Views, HelpfulUp: a.HelpfulUp, HelpfulDown: a.HelpfulDown})
			}
			for _, q := range r.NoResultQueries {
				out.CustomerContent.NoResultQueries = append(out.CustomerContent.NoResultQueries, mcpHelpQuery{Query: q.Query, Locale: q.Locale, Searches: q.Searches})
			}
			return out, nil
		}),
	defineTool("create_help_center", "Create a help center",
		"Creates a help center for an inbox's product. It stays unpublished until a member publishes it in Yuva. Then add collections with create_help_collection and drafts with draft_help_article. Owners, admins and keys with help:publish; not OAuth connections. Maps to POST /v1/help-centers.",
		additive, nil, publicSetupTool("CreateHelpCenter"),
		func(ctx context.Context, c *mcpCall, in createHelpCenterIn) (mcpHelpCenter, error) {
			body := oas.HelpCenterCreate{InboxId: in.InboxID, Name: in.Name, Slug: in.Slug, Description: optString[string](in.Description),
				DefaultLocale: optString[string](in.DefaultLocale), SiteUrl: optString[string](in.SiteURL)}
			if in.Locales != nil {
				body.Locales = &in.Locales
			}
			res, err := c.s.CreateHelpCenter(ctx, oas.CreateHelpCenterRequestObject{Body: &body})
			if err != nil {
				return mcpHelpCenter{}, err
			}
			return helpCenterOutMCP(oas.HelpCenter(res.(oas.CreateHelpCenter201JSONResponse))), nil
		}),
	defineTool("create_help_collection", "Create a help collection",
		"Adds a collection (a group of articles) to a help center, named in one language. Maps to POST /v1/help-centers/{id}/collections.",
		additive, nil, mcpTool{ops: []string{"CreateHelpCollection"}},
		func(ctx context.Context, c *mcpCall, in createHelpCollectionIn) (mcpHelpCollection, error) {
			locale := in.Locale
			if locale == "" && keyMayCall(c.p, "GetHelpCenter") != nil {
				return mcpHelpCollection{}, errValidation("locale is required: this connection cannot read the help center's default locale")
			}
			if locale == "" {
				res, err := c.s.GetHelpCenter(ctx, oas.GetHelpCenterRequestObject{CenterId: in.CenterID})
				if err != nil {
					return mcpHelpCollection{}, err
				}
				locale = oas.HelpCenter(res.(oas.GetHelpCenter200JSONResponse)).DefaultLocale
			}
			res, err := c.s.CreateHelpCollection(ctx, oas.CreateHelpCollectionRequestObject{CenterId: in.CenterID, Body: &oas.HelpCollectionCreate{
				ParentId:     in.ParentID,
				Translations: map[string]oas.HelpCollectionTranslationInput{locale: {Name: in.Name, Description: optString[string](in.Description)}},
			}})
			if err != nil {
				return mcpHelpCollection{}, err
			}
			col := oas.HelpCollection(res.(oas.CreateHelpCollection201JSONResponse))
			out := mcpHelpCollection{ID: col.Id, ParentID: col.ParentId, Number: col.Number, Names: []string{}, Articles: col.ArticleCount}
			for l, t := range col.Translations {
				out.Names = append(out.Names, l+": "+t.Name)
			}
			return out, nil
		}),
}

func (c *mcpCall) helpTranslationDraft(ctx context.Context, center, article uuid.UUID, put oas.PutHelpArticleTranslationResponseObject) (mcpHelpDraft, error) {
	var t oas.HelpArticleTranslation
	switch r := put.(type) {
	case oas.PutHelpArticleTranslation200JSONResponse:
		t = oas.HelpArticleTranslation(r)
	case oas.PutHelpArticleTranslation201JSONResponse:
		t = oas.HelpArticleTranslation(r)
	default:
		return mcpHelpDraft{}, errInternal
	}
	res, err := c.s.GetHelpArticle(ctx, oas.GetHelpArticleRequestObject{CenterId: center, ArticleId: article})
	if err != nil {
		return mcpHelpDraft{}, err
	}
	return c.helpDraft(oas.HelpArticle(res.(oas.GetHelpArticle200JSONResponse)), t.Locale)
}

func mayReadHelpResource(p principal) bool {
	return keyMayCall(p, "GetHelpCenter") == nil && keyMayCall(p, "ListHelpCollections") == nil && keyMayCall(p, "ListHelpArticles") == nil
}

func (c *mcpCall) helpCenterResource(ctx context.Context, id uuid.UUID) (mcpHelpCenterResource, error) {
	h, err := c.helpCenter(ctx, id)
	if err != nil {
		return mcpHelpCenterResource{}, err
	}
	out := mcpHelpCenterResource{Center: helpCenterOutMCP(h), Collections: []mcpHelpCollection{}, Articles: []mcpHelpArticleListItem{}}
	cols, err := c.s.ListHelpCollections(ctx, oas.ListHelpCollectionsRequestObject{CenterId: id})
	if err != nil {
		return out, err
	}
	for _, col := range cols.(oas.ListHelpCollections200JSONResponse).Items {
		item := mcpHelpCollection{ID: col.Id, ParentID: col.ParentId, Number: col.Number, Names: []string{}, Articles: col.ArticleCount}
		for _, l := range h.Locales {
			if t, ok := col.Translations[l]; ok {
				item.Names = append(item.Names, l+": "+t.Name)
			}
		}
		out.Collections = append(out.Collections, item)
	}
	lim := int32(helpResourceTop)
	arts, err := c.s.ListHelpArticles(ctx, oas.ListHelpArticlesRequestObject{CenterId: id, Params: oas.ListHelpArticlesParams{Limit: &lim}})
	if err != nil {
		return out, err
	}
	for _, a := range arts.(oas.ListHelpArticles200JSONResponse).Items {
		item := mcpHelpArticleListItem{ID: a.Id, Number: a.Number, CollectionID: a.CollectionId, Titles: []string{}}
		for _, t := range a.Translations {
			item.Titles = append(item.Titles, t.Locale+" ("+string(t.State)+"): "+t.Title)
		}
		out.Articles = append(out.Articles, item)
	}
	return out, nil
}

func helpEventCenter(e realtime.Event) (uuid.UUID, bool) {
	switch e.Type {
	case helpEventPublished, helpEventUnpublish, helpEventDeleted, helpSignalUpdated:
		var d struct {
			CenterID uuid.UUID `json:"center_id"`
		}
		return d.CenterID, json.Unmarshal(e.Data, &d) == nil
	}
	return uuid.UUID{}, false
}

func addHelpPrompts(srv *mcp.Server, c *mcpCall) {
	if !toolsListed(c.p, "search_help_articles", "draft_help_article") {
		return
	}
	srv.AddPrompt(&mcp.Prompt{
		Name: "write_help_article", Title: "Write a help article",
		Description: "Write a help article as a draft, from a conversation that needed one or from a topic: search first so nothing is written twice. Nothing is published.",
		Arguments: []*mcp.PromptArgument{
			{Name: "center_id", Description: "The help center to write in; list_help_centers shows them.", Required: true},
			{Name: "conversation_id", Description: "A conversation whose answer the article should hold."},
			{Name: "topic", Description: "What the article is about, when there is no conversation."},
			{Name: "locale", Description: "The language to write in; the center's default when empty."},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		a := req.Params.Arguments
		source := "the topic " + strings.TrimSpace(a["topic"])
		if id := a["conversation_id"]; id != "" {
			source = "the conversation " + id + " (read it with get_conversation, and write what any customer in the same situation needs, never their name or details)"
		}
		language := "the center's default language"
		if a["locale"] != "" {
			language = a["locale"]
		}
		return promptResult("Write a help article for "+source,
			"Write a help article for the help center "+a["center_id"]+" about "+source+".",
			"1. Call search_help_articles with drafts true and the key words of the question. If an article already answers it, tell me which one instead of writing a new one; to translate it, use draft_help_article with its article_id.\n"+
				"2. Write the article in Markdown for customers, in "+language+": a title of a few words that names the question, a short answer first, then numbered steps or short sections with headings. Say nothing you could not check in the conversation or the existing articles.\n"+
				"3. Store it with draft_help_article. Do not try to publish it: a member reviews and publishes it in Yuva.",
			"Then show me the title and the article's number."), nil
	})
	if !toolsListed(c.p, "help_search_gaps") {
		return
	}
	srv.AddPrompt(&mcp.Prompt{
		Name: "help_gaps_review", Title: "Review help center gaps",
		Description: "Turn the searches that found nothing and the articles that got the most no answers into a list of articles to write or improve, and draft the first few.",
		Arguments: []*mcp.PromptArgument{
			{Name: "center_id", Description: "The help center to review.", Required: true},
			{Name: "days", Description: "7, 30 or 90; 30 when empty."},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		a := req.Params.Arguments
		return promptResult("Review the gaps of help center "+a["center_id"],
			"Review the help center "+a["center_id"]+" for gaps.",
			"1. Call help_search_gaps for the center"+map[bool]string{true: " with days " + a["days"], false: ""}[a["days"] != ""]+".\n"+
				"2. Group the searches that found nothing by what the visitor wanted. For each group call search_help_articles with drafts true: an article may exist under other words (suggest adding them as keywords) or be a draft nobody published yet.\n"+
				"3. For the articles with the most no answers, call get_help_article and say what is likely missing or unclear.",
			"Answer with a table: the gap, how often it was searched, and what to do (write, add keywords, improve, publish the draft). Then draft the top three new articles with draft_help_article; do not publish anything."), nil
	})
}

package api

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	helpReportTop      = 20
	helpReportMostDown = 10
	helpPreviewCSP     = "default-src 'none'; img-src 'self' https: data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
)

var limitHelpRender = limit{60, time.Minute}

func (s *Server) GetHelpReport(ctx context.Context, req oas.GetHelpReportRequestObject) (oas.GetHelpReportResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	since, err := docsSince(s, req.Params.Days)
	if err != nil {
		return nil, err
	}
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	scope := store.HelpReportArticlesParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Since: since}
	rows, err := s.st.HelpReportArticles(ctx, scope)
	if err != nil {
		return nil, err
	}
	totals, err := s.st.HelpReportSearchTotals(ctx, store.HelpReportSearchTotalsParams(scope))
	if err != nil {
		return nil, err
	}
	queries, err := s.st.HelpReportNoResultQueries(ctx, store.HelpReportNoResultQueriesParams(scope))
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ArticleID
	}
	ts, err := helpSummaries(ctx, s.st.Queries, c.WorkspaceID, ids)
	if err != nil {
		return nil, err
	}
	sortHelpTranslations(c, ts)
	title := func(id uuid.UUID) string {
		for _, t := range ts {
			if t.ArticleID == id {
				if t.PublishedTitle != nil {
					return *t.PublishedTitle
				}
				return t.Title
			}
		}
		return ""
	}
	days := int32(defaultDocsDays)
	if req.Params.Days != nil {
		days = *req.Params.Days
	}
	out := oas.HelpReport{
		Days: days, Searches: totals.Searches, NoResultSearches: totals.NoResults, Clicks: totals.Clicks,
		TopArticles: []oas.HelpReportArticle{}, MostDown: []oas.HelpReportArticle{}, NoResultQueries: []oas.HelpReportQuery{},
	}
	articles := make([]oas.HelpReportArticle, len(rows))
	for i, r := range rows {
		out.Views += r.Views
		articles[i] = oas.HelpReportArticle{
			ArticleId: r.ArticleID, Number: r.Number, Title: title(r.ArticleID), Views: r.Views, HelpfulUp: r.HelpfulUp,
			HelpfulDown: r.HelpfulDown, Conversations: r.Conversations, Sent: r.Sent,
		}
	}
	slices.SortStableFunc(articles, func(a, b oas.HelpReportArticle) int {
		if a.Views != b.Views {
			return int(b.Views - a.Views)
		}
		return int(a.Number - b.Number)
	})
	for _, a := range articles {
		if a.Views > 0 && len(out.TopArticles) < helpReportTop {
			out.TopArticles = append(out.TopArticles, a)
		}
	}
	slices.SortStableFunc(articles, func(a, b oas.HelpReportArticle) int {
		if a.HelpfulDown != b.HelpfulDown {
			return int(b.HelpfulDown - a.HelpfulDown)
		}
		return int(a.Number - b.Number)
	})
	for _, a := range articles {
		if a.HelpfulDown > 0 && len(out.MostDown) < helpReportMostDown {
			out.MostDown = append(out.MostDown, a)
		}
	}
	for _, q := range queries {
		out.NoResultQueries = append(out.NoResultQueries, oas.HelpReportQuery{Query: q.Query, Locale: q.Locale, Searches: q.Searches})
	}
	return oas.GetHelpReport200JSONResponse(out), nil
}

func (s *Server) RenderHelpMarkdown(ctx context.Context, req oas.RenderHelpMarkdownRequestObject) (oas.RenderHelpMarkdownResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"help-render:" + p.viewerID().String(), limitHelpRender}); err != nil {
		return nil, err
	}
	if len([]rune(req.Body.Body)) > maxHelpBody {
		return nil, errValidation("body must be at most 100000 characters")
	}
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	images, err := s.helpImages(ctx, s.st.Queries, c.WorkspaceID, req.Body.Body)
	if err != nil {
		return nil, err
	}
	return oas.RenderHelpMarkdown200JSONResponse{Html: renderHelp(req.Body.Body, images)}, nil
}

var helpAltEscaper = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`, "\n", " ", "\r", " ")

func (s *Server) UploadHelpImage(ctx context.Context, req oas.UploadHelpImageRequestObject) (oas.UploadHelpImageResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"email-image-upload:" + p.workspaceID.String(), limitImageUpload}); err != nil {
		return nil, err
	}
	if _, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false); err != nil {
		return nil, err
	}
	data, filename, err := readImageUpload(req.Body)
	if err != nil {
		return nil, err
	}
	var row store.EmailImage
	var created bool
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		var err error
		row, created, err = s.storeEmailImage(ctx, q, p, data, filename)
		return err
	})
	if err != nil {
		return nil, err
	}
	url := s.imageURL(row.Token)
	alt := strings.TrimSpace(helpAltEscaper.Replace(strings.TrimSuffix(row.Filename, extOf(row.Filename))))
	out := oas.HelpImage{Id: row.ID, Url: url, Width: row.Width, Height: row.Height, Markdown: "![" + alt + "](" + url + ")"}
	if created {
		return oas.UploadHelpImage201JSONResponse(out), nil
	}
	return oas.UploadHelpImage200JSONResponse(out), nil
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[i:]
	}
	return ""
}

type helpPreviewResponse []byte

func (b helpPreviewResponse) VisitPreviewHelpCenterResponse(w http.ResponseWriter) error {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", helpPreviewCSP)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(b)
	return err
}

type helpPreviewLink struct {
	Title, URL string
}

type helpPreviewPage struct {
	Lang, Center, Title, Description string
	Links                            []helpPreviewLink
	Body                             template.HTML
}

var helpPreviewTemplate = template.Must(template.New("preview").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · {{.Center}}</title>
<style>body{font:16px/1.6 system-ui,sans-serif;max-width:46rem;margin:2rem auto;padding:0 1rem;color:#1f2328}a{color:#0969da}img{max-width:100%;height:auto}pre{overflow:auto;background:#f6f8fa;padding:.75rem}table{border-collapse:collapse}td,th{border:1px solid #d0d7de;padding:.25rem .5rem}</style>
</head><body><p>{{.Center}}</p><h1>{{.Title}}</h1>{{if .Description}}<p>{{.Description}}</p>{{end}}
{{if .Links}}<ul>{{range .Links}}<li><a href="{{.URL}}">{{.Title}}</a></li>{{end}}</ul>{{end}}
{{.Body}}</body></html>
`))

func (s *Server) PreviewHelpCenter(ctx context.Context, req oas.PreviewHelpCenterRequestObject) (oas.PreviewHelpCenterResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prm := req.Params
	view := oas.PreviewHelpCenterParamsView("home")
	if prm.View != nil {
		if !prm.View.Valid() {
			return nil, errValidation("view must be home, collection or article")
		}
		view = *prm.View
	}
	if view != "home" && prm.Id == nil {
		return nil, errValidation("id is required for view=" + string(view))
	}
	if err := s.rateLimit(ctx, rateCheck{"help-render:" + p.viewerID().String(), limitHelpRender}); err != nil {
		return nil, err
	}
	q := s.st.Queries
	c, err := visibleHelpCenter(ctx, q, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	locale := c.DefaultLocale
	if prm.Locale != nil {
		if err := offersLocale(c, *prm.Locale); err != nil {
			return nil, err
		}
		locale = *prm.Locale
	}
	page := helpPreviewPage{Lang: locale, Center: c.Name}
	switch view {
	case "article":
		a, err := helpArticle(ctx, q, c, *prm.Id, false)
		if err != nil {
			return nil, err
		}
		t, err := q.GetHelpTranslation(ctx, store.GetHelpTranslationParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: locale})
		if store.IsNotFound(err) {
			return nil, errHelpTranslationGone
		}
		if err != nil {
			return nil, err
		}
		if boolOr(prm.Draft, false) {
			images, err := s.helpImages(ctx, q, c.WorkspaceID, t.Body)
			if err != nil {
				return nil, err
			}
			page.Title, page.Body = t.Title, template.HTML(renderHelp(t.Body, images))
		} else {
			if t.PublishedAt == nil {
				return nil, problem(http.StatusNotFound, "not_found", "the translation is not published; preview its draft with draft=true")
			}
			page.Title, page.Body = deref(t.PublishedTitle), template.HTML(deref(t.PublishedHtml))
		}
	default:
		var only *uuid.UUID
		page.Title, page.Description = c.Name, c.Description
		if view == "collection" {
			col, err := helpCollection(ctx, q, c, *prm.Id)
			if err != nil {
				return nil, err
			}
			only = &col.ID
			ts, err := q.ListHelpCollectionTranslations(ctx, store.ListHelpCollectionTranslationsParams{WorkspaceID: c.WorkspaceID, CollectionIds: []uuid.UUID{col.ID}})
			if err != nil {
				return nil, err
			}
			for _, t := range ts {
				if t.Locale == locale {
					page.Title, page.Description = t.Name, t.Description
				}
			}
		}
		rows, err := q.ListHelpArticles(ctx, store.ListHelpArticlesParams{
			WorkspaceID: c.WorkspaceID, CenterID: c.ID, CollectionID: only, State: new("published"), Locale: locale, Lim: maxPageSize,
		})
		if err != nil {
			return nil, err
		}
		changed, err := q.ListHelpArticles(ctx, store.ListHelpArticlesParams{
			WorkspaceID: c.WorkspaceID, CenterID: c.ID, CollectionID: only, State: new("changed"), Locale: locale, Lim: maxPageSize,
		})
		if err != nil {
			return nil, err
		}
		rows = append(rows, changed...)
		slices.SortStableFunc(rows, func(a, b store.HelpArticle) int {
			if a.Position != b.Position {
				return int(a.Position - b.Position)
			}
			return int(a.Number - b.Number)
		})
		ids := make([]uuid.UUID, len(rows))
		for i, a := range rows {
			ids[i] = a.ID
		}
		ts, err := helpSummaries(ctx, q, c.WorkspaceID, ids)
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			for _, t := range ts {
				if t.ArticleID == a.ID && t.Locale == locale && t.PublishedAt != nil {
					page.Links = append(page.Links, helpPreviewLink{Title: deref(t.PublishedTitle), URL: deref(s.helpPublicURL(c, a.Number, t))})
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := helpPreviewTemplate.Execute(&buf, page); err != nil {
		return nil, err
	}
	return helpPreviewResponse(buf.Bytes()), nil
}

package api

import (
	"context"
	"net/http"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

type changelogPreviewResponse []byte

func (b changelogPreviewResponse) VisitPreviewChangelogResponse(w http.ResponseWriter) error {
	h := w.Header()
	h.Set("Content-Type", changelogPageType)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", publicCSP{img: "'self' https:", scriptHash: changelogScriptHash, frameAncestors: "'self'", formAction: "'none'"}.String())
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(b)
	return err
}

// changelogPreviewItem is an entry as the page shows it: its published copy, or with draft its draft.
func (s *Server) changelogPreviewItem(ctx context.Context, q *store.Queries, c store.Changelog, e store.ChangelogEntry, locale string, draft bool) (changelogItem, bool, error) {
	t, err := q.GetChangelogTranslation(ctx, store.GetChangelogTranslationParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: locale})
	if store.IsNotFound(err) {
		return changelogItem{}, false, nil
	}
	if err != nil {
		return changelogItem{}, false, err
	}
	title, slug, body := t.Title, t.Slug, ""
	if draft {
		images, err := s.helpImages(ctx, q, c.WorkspaceID, t.Body)
		if err != nil {
			return changelogItem{}, false, err
		}
		body = renderHelp(t.Body, images)
	} else {
		if e.State != changelogStatePublished || t.PublishedAt == nil {
			return changelogItem{}, false, nil
		}
		title, slug, body = deref(t.PublishedTitle), deref(t.PublishedSlug), deref(t.PublishedHtml)
	}
	at := e.CreatedAt
	if e.PublishedAt != nil {
		at = *e.PublishedAt
	}
	var token *string
	var w, h *int32
	if img, err := s.entryImage(ctx, q, e); err != nil {
		return changelogItem{}, false, err
	} else if img != nil {
		token, w, h = &img.Token, &img.Width, &img.Height
	}
	return s.changelogItemFrom(c, locale, e.Number, e.Labels, at, title, slug, body, token, w, h), true, nil
}

func (s *Server) changelogPreviewItems(ctx context.Context, c store.Changelog, locale string, label *string, draft bool) ([]changelogItem, error) {
	rows, err := s.st.ListChangelogEntries(ctx, store.ListChangelogEntriesParams{
		WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Label: label, Locale: &locale, Lim: changelogPageSize,
	})
	if err != nil {
		return nil, err
	}
	var out []changelogItem
	for _, e := range rows {
		it, ok, err := s.changelogPreviewItem(ctx, s.st.Queries, c, e, locale, draft)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *Server) changelogPreviewEntry(ctx context.Context, c store.Changelog, v *changelogPageView, rt changelogRoute, opt changelogOptions) (map[string]string, error) {
	q := s.st.Queries
	e, err := q.GetChangelogEntry(ctx, store.GetChangelogEntryParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, ID: opt.entry})
	if store.IsNotFound(err) {
		return nil, errChangelogPageGone
	}
	if err != nil {
		return nil, err
	}
	it, ok, err := s.changelogPreviewItem(ctx, q, c, e, rt.locale, opt.draft)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errChangelogPageGone
	}
	v.Single, v.Items = true, []changelogItem{it}
	v.H1, v.OGType, v.Canonical, v.OGImage = it.Title, "article", it.URL, it.Image
	v.Title, v.OGTitle = it.Title+" – "+c.Name, it.Title
	v.Description = helpDescription(changelogExcerpt(string(it.Body)), it.Title+" – "+c.Name)
	v.Crumbs = []helpCrumb{{Name: v.W.Title, URL: v.Home}, {Name: it.Title}}
	return map[string]string{}, nil
}

func (s *Server) PreviewChangelog(ctx context.Context, req oas.PreviewChangelogRequestObject) (oas.PreviewChangelogResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prm := req.Params
	view, opt := "home", changelogOptions{preview: true, draft: boolOr(prm.Draft, false)}
	switch {
	case prm.View == nil || *prm.View == oas.PreviewChangelogParamsViewHome:
	case *prm.View == oas.PreviewChangelogParamsViewEntry:
		if prm.Id == nil {
			return nil, errValidation("id is required for view=entry")
		}
		view, opt.entry = "entry", *prm.Id
	default:
		return nil, errValidation("view must be home or entry")
	}
	if err := s.rateLimit(ctx, rateCheck{"changelog-render:" + p.viewerID().String(), limitChangelogRender}); err != nil {
		return nil, err
	}
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: c.WorkspaceID, ID: c.InboxID})
	if err != nil {
		return nil, err
	}
	locale := c.DefaultLocale
	if prm.Locale != nil {
		if err := offersChangelogLocale(c, *prm.Locale); err != nil {
			return nil, err
		}
		locale = *prm.Locale
	}
	rt := changelogRoute{view: view, locale: locale}
	body, err := s.changelogPage(ctx, c, in, rt, "", "", opt)
	if err == errChangelogPageGone {
		return nil, problem(http.StatusNotFound, "not_found", "no such entry in this locale, or it has no published copy")
	}
	if err != nil {
		return nil, err
	}
	return changelogPreviewResponse(body), nil
}

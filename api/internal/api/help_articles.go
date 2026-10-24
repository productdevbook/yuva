package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxHelpPaths    = 20
	maxHelpPathLen  = 200
	maxHelpBody     = 100000
	maxHelpKeywords = 500
)

func helpArticle(ctx context.Context, q *store.Queries, c store.HelpCenter, id uuid.UUID, lock bool) (store.HelpArticle, error) {
	get := q.GetHelpArticle
	if lock {
		get = func(ctx context.Context, arg store.GetHelpArticleParams) (store.HelpArticle, error) {
			return q.LockHelpArticle(ctx, store.LockHelpArticleParams(arg))
		}
	}
	a, err := get(ctx, store.GetHelpArticleParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: id})
	if store.IsNotFound(err) {
		return a, errHelpArticleGone
	}
	return a, err
}

func helpState(t store.HelpArticleTranslation) oas.HelpTranslationState {
	switch {
	case t.PublishedAt == nil:
		return oas.HelpStateDraft
	case t.HasUnpublishedChanges:
		return oas.HelpStateChanged
	}
	return oas.HelpStatePublished
}

func helpOutdated(c store.HelpCenter, t store.HelpArticleTranslation, all []store.HelpArticleTranslation) bool {
	if t.Locale == c.DefaultLocale {
		return false
	}
	for _, d := range all {
		if d.ArticleID == t.ArticleID && d.Locale == c.DefaultLocale {
			return d.PublishedAt != nil && d.PublishedAt.After(t.UpdatedAt)
		}
	}
	return false
}

func (s *Server) helpPublicURL(c store.HelpCenter, number int32, t store.HelpArticleTranslation) *string {
	if t.PublishedAt == nil || t.PublishedSlug == nil {
		return nil
	}
	return new(s.helpArticleURL(c, number, t.Locale, *t.PublishedSlug))
}

func (s *Server) helpTranslationBody(c store.HelpCenter, number int32, t store.HelpArticleTranslation, all []store.HelpArticleTranslation) oas.HelpArticleTranslation {
	out := oas.HelpArticleTranslation{
		ArticleId: t.ArticleID, Locale: t.Locale, Title: t.Title, Body: t.Body, Keywords: t.Keywords, Slug: t.Slug, Version: t.Version,
		State: helpState(t), Outdated: helpOutdated(c, t, all), Url: s.helpPublicURL(c, number, t),
		UpdatedByMemberId: t.UpdatedByMemberID, UpdatedByApiKeyId: t.UpdatedByApiKeyID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if t.PublishedAt != nil {
		out.Published = &oas.HelpPublishedCopy{
			Title: deref(t.PublishedTitle), Body: deref(t.PublishedBody), Keywords: deref(t.PublishedKeywords), Slug: deref(t.PublishedSlug),
			Html: deref(t.PublishedHtml), Excerpt: deref(t.PublishedExcerpt), PublishedAt: *t.PublishedAt,
			PublishedByMemberId: t.PublishedByMemberID, PublishedByApiKeyId: t.PublishedByApiKeyID,
		}
	}
	return out
}

func sortHelpTranslations(c store.HelpCenter, ts []store.HelpArticleTranslation) {
	rank := func(l string) int {
		if i := slices.Index(c.Locales, l); i >= 0 {
			return i
		}
		return len(c.Locales)
	}
	slices.SortStableFunc(ts, func(a, b store.HelpArticleTranslation) int {
		if a.ArticleID != b.ArticleID {
			return strings.Compare(a.ArticleID.String(), b.ArticleID.String())
		}
		if d := rank(a.Locale) - rank(b.Locale); d != 0 {
			return d
		}
		return strings.Compare(a.Locale, b.Locale)
	})
}

func (s *Server) helpArticleOut(ctx context.Context, q *store.Queries, c store.HelpCenter, a store.HelpArticle) (oas.HelpArticle, error) {
	ts, err := q.ListHelpTranslations(ctx, store.ListHelpTranslationsParams{WorkspaceID: c.WorkspaceID, ArticleIds: []uuid.UUID{a.ID}})
	if err != nil {
		return oas.HelpArticle{}, err
	}
	sortHelpTranslations(c, ts)
	out := oas.HelpArticle{
		Id: a.ID, CenterId: a.CenterID, CollectionId: a.CollectionID, Number: a.Number, Position: a.Position, Paths: a.Paths,
		CreatedByMemberId: a.CreatedByMemberID, CreatedByApiKeyId: a.CreatedByApiKeyID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		Translations: make([]oas.HelpArticleTranslation, len(ts)),
	}
	for i, t := range ts {
		out.Translations[i] = s.helpTranslationBody(c, a.Number, t, ts)
	}
	return out, nil
}

func (s *Server) helpTranslationOut(ctx context.Context, q *store.Queries, c store.HelpCenter, a store.HelpArticle, t store.HelpArticleTranslation) (oas.HelpArticleTranslation, error) {
	all := []store.HelpArticleTranslation{t}
	if t.Locale != c.DefaultLocale {
		ts, err := helpSummaries(ctx, q, c.WorkspaceID, []uuid.UUID{a.ID})
		if err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		all = append(all, ts...)
	}
	return s.helpTranslationBody(c, a.Number, t, all), nil
}

func helpSummaries(ctx context.Context, q *store.Queries, ws uuid.UUID, ids []uuid.UUID) ([]store.HelpArticleTranslation, error) {
	rows, err := q.ListHelpTranslationSummaries(ctx, store.ListHelpTranslationSummariesParams{WorkspaceID: ws, ArticleIds: ids})
	if err != nil {
		return nil, err
	}
	out := make([]store.HelpArticleTranslation, len(rows))
	for i, r := range rows {
		out[i] = store.HelpArticleTranslation{
			WorkspaceID: ws, ArticleID: r.ArticleID, Locale: r.Locale, Title: r.Title, Slug: r.Slug, Version: r.Version,
			PublishedTitle: r.PublishedTitle, PublishedSlug: r.PublishedSlug, PublishedAt: r.PublishedAt,
			HasUnpublishedChanges: r.HasUnpublishedChanges, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}

type helpDraft struct {
	title, body, keywords, slug string
}

func helpDraftFields(title string, body, keywords, slug *string) (helpDraft, error) {
	var d helpDraft
	var err error
	if d.title, err = trimmed(strings.Join(strings.Fields(title), " "), 3, 200, "title"); err != nil {
		return d, err
	}
	if body != nil {
		d.body = *body
	}
	if len([]rune(d.body)) > maxHelpBody {
		return d, errValidation("body must be at most 100000 characters")
	}
	if keywords != nil {
		if d.keywords, err = trimmed(*keywords, 0, maxHelpKeywords, "keywords"); err != nil {
			return d, err
		}
	}
	if slug != nil && *slug != "" {
		if err := validSlug(*slug, "slug"); err != nil {
			return d, err
		}
		d.slug = *slug
	} else {
		d.slug = helpSlug(d.title, "article")
	}
	return d, nil
}

func validHelpPaths(paths []string) error {
	if len(paths) > maxHelpPaths {
		return errValidation("paths holds at most 20 patterns")
	}
	for _, v := range paths {
		if len(v) > maxHelpPathLen || !strings.HasPrefix(v, "/") || strings.ContainsAny(v, " \t\r\n?#") {
			return errValidation("paths must start with / and be at most 200 characters without spaces, ? or #")
		}
		if i := strings.IndexByte(v, '*'); i >= 0 && i != len(v)-1 {
			return errValidation("a * in paths may only end a pattern")
		}
	}
	return nil
}

func helpCollectionOf(ctx context.Context, q *store.Queries, c store.HelpCenter, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	_, err := q.GetHelpCollection(ctx, store.GetHelpCollectionParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: *id})
	if store.IsNotFound(err) {
		return errValidation("collection_id must be a collection of the center")
	}
	return err
}

func (s *Server) addHelpRevision(ctx context.Context, q *store.Queries, p principal, t store.HelpArticleTranslation, kind oas.HelpRevisionKind, title, body, keywords string) error {
	member, key := helpCreatedBy(p)
	if kind == oas.HelpRevisionSave {
		last, err := q.LatestHelpSaveRevisionAt(ctx, store.LatestHelpSaveRevisionAtParams{
			WorkspaceID: t.WorkspaceID, ArticleID: t.ArticleID, Locale: t.Locale, MemberID: member, ApiKeyID: key,
		})
		if err != nil {
			return err
		}
		if s.now().Sub(last) < helpSaveRevisionGap {
			return nil
		}
	}
	images, err := s.helpImages(ctx, q, t.WorkspaceID, body)
	if err != nil {
		return err
	}
	if err := q.InsertHelpRevision(ctx, store.InsertHelpRevisionParams{
		ID: newID(), WorkspaceID: t.WorkspaceID, ArticleID: t.ArticleID, Locale: t.Locale, Kind: string(kind), Title: title, Body: body,
		Keywords: keywords, ImageIds: images.ids(), MemberID: member, ApiKeyID: key, Via: p.viaClient(), Now: s.now(),
	}); err != nil {
		return err
	}
	return q.PruneHelpRevisions(ctx, store.PruneHelpRevisionsParams{WorkspaceID: t.WorkspaceID, ArticleID: t.ArticleID, Locale: t.Locale, Keep: helpRevisionsKept})
}

func (s *Server) helpArticleSignal(ctx context.Context, p principal, c store.HelpCenter, t store.HelpArticleTranslation) {
	data := map[string]any{"center_id": c.ID, "article_id": t.ArticleID, "locale": t.Locale, "version": t.Version}
	if m := p.memberRef(); m != nil {
		data["member_id"] = *m
	}
	s.signal(ctx, realtime.Event{Type: helpSignalUpdated, WorkspaceID: c.WorkspaceID, InboxID: &c.InboxID, Data: mustJSON(data)})
}

func helpEventData(c store.HelpCenter, a store.HelpArticle, locale *string) oas.HelpArticleEventData {
	return oas.HelpArticleEventData{CenterId: c.ID, ArticleId: a.ID, Number: a.Number, Locale: locale}
}

func (s *Server) helpConflict(ctx context.Context, q *store.Queries, c store.HelpCenter, a store.HelpArticle, cur *store.HelpArticleTranslation) error {
	e := problem(http.StatusConflict, "version_conflict", "the translation was saved by someone else since")
	if cur != nil {
		body, err := s.helpTranslationOut(ctx, q, c, a, *cur)
		if err != nil {
			return err
		}
		e.Detail += " version " + strconv.Itoa(int(cur.Version))
		e.Extra = map[string]any{"translation": body}
	}
	return e
}

func encodeHelpCursor(position, number int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(int(position)) + "." + strconv.Itoa(int(number))))
}

func decodeHelpCursor(v *string) (*int32, *int32, error) {
	if v == nil || *v == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*v)
	if err != nil {
		return nil, nil, errInvalidCursor
	}
	a, b, ok := strings.Cut(string(raw), ".")
	pos, err1 := strconv.ParseInt(a, 10, 32)
	num, err2 := strconv.ParseInt(b, 10, 32)
	if !ok || err1 != nil || err2 != nil {
		return nil, nil, errInvalidCursor
	}
	return new(int32(pos)), new(int32(num)), nil
}

func (s *Server) ListHelpArticles(ctx context.Context, req oas.ListHelpArticlesRequestObject) (oas.ListHelpArticlesResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	pos, num, err := decodeHelpCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
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
	var state *string
	if prm.State != nil {
		if !prm.State.Valid() {
			return nil, errValidation("state must be draft, published, changed or missing")
		}
		state = new(string(*prm.State))
	}
	var query *string
	if prm.Q != nil {
		v := strings.TrimSpace(*prm.Q)
		if len([]rune(v)) > 200 {
			return nil, errValidation("q must be at most 200 characters")
		}
		if v != "" {
			query = &v
		}
	}
	rows, err := s.st.ListHelpArticles(ctx, store.ListHelpArticlesParams{
		WorkspaceID: p.workspaceID, CenterID: c.ID, CollectionID: prm.CollectionId, State: state, Locale: locale, Q: query,
		CursorPosition: pos, CursorNumber: num, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		last := rows[len(rows)-1]
		next = new(encodeHelpCursor(last.Position, last.Number))
	}
	ids := make([]uuid.UUID, len(rows))
	for i, a := range rows {
		ids[i] = a.ID
	}
	ts, err := helpSummaries(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	sortHelpTranslations(c, ts)
	out := oas.ListHelpArticles200JSONResponse{Items: make([]oas.HelpArticleSummary, len(rows)), NextCursor: next}
	for i, a := range rows {
		item := oas.HelpArticleSummary{
			Id: a.ID, CenterId: a.CenterID, CollectionId: a.CollectionID, Number: a.Number, Position: a.Position, Paths: a.Paths,
			CreatedByMemberId: a.CreatedByMemberID, CreatedByApiKeyId: a.CreatedByApiKeyID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
			Translations: []oas.HelpArticleTranslationSummary{},
		}
		for _, t := range ts {
			if t.ArticleID != a.ID {
				continue
			}
			item.Translations = append(item.Translations, oas.HelpArticleTranslationSummary{
				Locale: t.Locale, Title: t.Title, Slug: t.Slug, Version: t.Version, State: helpState(t), Outdated: helpOutdated(c, t, ts),
				PublishedAt: t.PublishedAt, Url: s.helpPublicURL(c, a.Number, t), UpdatedAt: t.UpdatedAt,
			})
		}
		out.Items[i] = item
	}
	return out, nil
}

func (s *Server) CreateHelpArticle(ctx context.Context, req oas.CreateHelpArticleRequestObject) (oas.CreateHelpArticleResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	d, err := helpDraftFields(b.Title, b.Body, b.Keywords, b.Slug)
	if err != nil {
		return nil, err
	}
	var paths []string
	if b.Paths != nil {
		paths = *b.Paths
	}
	if err := validHelpPaths(paths); err != nil {
		return nil, err
	}
	if paths == nil {
		paths = []string{}
	}
	var out oas.HelpArticle
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		if err := offersLocale(c, b.Locale); err != nil {
			return err
		}
		if err := helpCollectionOf(ctx, q, c, b.CollectionId); err != nil {
			return err
		}
		if max := s.help.MaxArticles; max > 0 {
			if err := q.LockHelpArticleQuota(ctx, p.workspaceID); err != nil {
				return err
			}
			n, err := q.CountWorkspaceHelpArticles(ctx, p.workspaceID)
			if err != nil {
				return err
			}
			if int(n) >= max {
				return errHostedLimit(oas.LimitHelpMaxArticles, "this workspace has as many help articles as the server allows ("+strconv.Itoa(max)+", YUVA_HELP_MAX_ARTICLES)")
			}
		}
		number, err := q.TakeHelpArticleNumber(ctx, store.TakeHelpArticleNumberParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		if err != nil {
			return err
		}
		images, err := s.helpImages(ctx, q, p.workspaceID, d.body)
		if err != nil {
			return err
		}
		member, key := helpCreatedBy(p)
		now := s.now()
		a, err := q.CreateHelpArticle(ctx, store.CreateHelpArticleParams{
			ID: newID(), WorkspaceID: c.WorkspaceID, CenterID: c.ID, CollectionID: b.CollectionId, Number: number,
			Position: int32OrZero(b.Position), Paths: paths, MemberID: member, ApiKeyID: key, Now: now,
		})
		if err != nil {
			return err
		}
		t, err := q.CreateHelpTranslation(ctx, store.CreateHelpTranslationParams{
			WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: b.Locale, Title: d.title, Body: d.body, Keywords: d.keywords, Slug: d.slug,
			ImageIds: images.ids(), SearchConfig: helpSearchConfig(b.Locale), MemberID: member, ApiKeyID: key, Now: now,
		})
		if err != nil {
			return err
		}
		if err := s.addHelpRevision(ctx, q, p, t, oas.HelpRevisionSave, t.Title, t.Body, t.Keywords); err != nil {
			return err
		}
		out, err = s.helpArticleOut(ctx, q, c, a)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateHelpArticle201JSONResponse(out), nil
}

func (s *Server) GetHelpArticle(ctx context.Context, req oas.GetHelpArticleRequestObject) (oas.GetHelpArticleResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	a, err := helpArticle(ctx, s.st.Queries, c, req.ArticleId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.helpArticleOut(ctx, s.st.Queries, c, a)
	if err != nil {
		return nil, err
	}
	return oas.GetHelpArticle200JSONResponse(out), nil
}

func (s *Server) UpdateHelpArticle(ctx context.Context, req oas.UpdateHelpArticleRequestObject) (oas.UpdateHelpArticleResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if b.Paths != nil {
		if err := validHelpPaths(*b.Paths); err != nil {
			return nil, err
		}
	}
	var out oas.HelpArticle
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		var err error
		out, err = s.updateHelpArticle(ctx, q, p, req.CenterId, req.ArticleId, *b)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateHelpArticle200JSONResponse(out), nil
}

func (s *Server) updateHelpArticle(ctx context.Context, q *store.Queries, p principal, centerID, articleID uuid.UUID, b oas.HelpArticleUpdate) (oas.HelpArticle, error) {
	c, err := visibleHelpCenter(ctx, q, p, centerID, true)
	if err != nil {
		return oas.HelpArticle{}, err
	}
	cur, err := helpArticle(ctx, q, c, articleID, true)
	if err != nil {
		return oas.HelpArticle{}, err
	}
	col := nullableID(b.CollectionId, cur.CollectionID)
	if err := helpCollectionOf(ctx, q, c, col); err != nil {
		return oas.HelpArticle{}, err
	}
	position, paths := cur.Position, cur.Paths
	if b.Position != nil {
		position = *b.Position
	}
	if b.Paths != nil {
		paths = *b.Paths
	}
	if paths == nil {
		paths = []string{}
	}
	a, err := q.UpdateHelpArticle(ctx, store.UpdateHelpArticleParams{
		WorkspaceID: c.WorkspaceID, ID: cur.ID, CollectionID: col, Position: position, Paths: paths, Now: s.now(),
	})
	if err != nil {
		return oas.HelpArticle{}, err
	}
	if err := s.touchHelpCenter(ctx, q, c); err != nil {
		return oas.HelpArticle{}, err
	}
	return s.helpArticleOut(ctx, q, c, a)
}

func (s *Server) DeleteHelpArticle(ctx context.Context, req oas.DeleteHelpArticleRequestObject) (oas.DeleteHelpArticleResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		return s.deleteHelpArticle(ctx, q, ev, p, req.CenterId, req.ArticleId)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteHelpArticle204Response{}, nil
}

func (s *Server) deleteHelpArticle(ctx context.Context, q *store.Queries, ev *eventBatch, p principal, centerID, articleID uuid.UUID) error {
	c, err := visibleHelpCenter(ctx, q, p, centerID, true)
	if err != nil {
		return err
	}
	a, err := helpArticle(ctx, q, c, articleID, true)
	if err != nil {
		return err
	}
	ts, err := helpSummaries(ctx, q, c.WorkspaceID, []uuid.UUID{a.ID})
	if err != nil {
		return err
	}
	published := slices.ContainsFunc(ts, func(t store.HelpArticleTranslation) bool { return t.PublishedAt != nil })
	if published {
		if err := mayPublishHelp(p, c); err != nil {
			return err
		}
	}
	if _, err := q.DeleteHelpArticle(ctx, store.DeleteHelpArticleParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: a.ID}); err != nil {
		return err
	}
	if published {
		if err := s.touchHelpCenter(ctx, q, c); err != nil {
			return err
		}
	}
	ev.add(helpEventDeleted, &c.InboxID, nil, helpEventData(c, a, nil))
	return nil
}

func (s *Server) saveHelpDraft(ctx context.Context, q *store.Queries, p principal, t store.HelpArticleTranslation, d helpDraft) (store.HelpArticleTranslation, error) {
	images, err := s.helpImages(ctx, q, t.WorkspaceID, d.body, deref(t.PublishedBody))
	if err != nil {
		return t, err
	}
	member, key := helpCreatedBy(p)
	return q.SaveHelpTranslation(ctx, store.SaveHelpTranslationParams{
		WorkspaceID: t.WorkspaceID, ArticleID: t.ArticleID, Locale: t.Locale, Title: d.title, Body: d.body, Keywords: d.keywords,
		Slug: d.slug, ImageIds: images.ids(), MemberID: member, ApiKeyID: key, Now: s.now(),
	})
}

func lockedHelpTranslation(ctx context.Context, q *store.Queries, a store.HelpArticle, locale string) (store.HelpArticleTranslation, error) {
	t, err := q.LockHelpTranslation(ctx, store.LockHelpTranslationParams{WorkspaceID: a.WorkspaceID, ArticleID: a.ID, Locale: locale})
	if store.IsNotFound(err) {
		return t, errHelpTranslationGone
	}
	return t, err
}

func (s *Server) PutHelpArticleTranslation(ctx context.Context, req oas.PutHelpArticleTranslationRequestObject) (oas.PutHelpArticleTranslationResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	d, err := helpDraftFields(b.Title, b.Body, b.Keywords, b.Slug)
	if err != nil {
		return nil, err
	}
	var out oas.HelpArticleTranslation
	var saved store.HelpArticleTranslation
	var center store.HelpCenter
	created := false
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		center = c
		if err := offersLocale(c, req.Locale); err != nil {
			return err
		}
		a, err := helpArticle(ctx, q, c, req.ArticleId, true)
		if err != nil {
			return err
		}
		cur, err := lockedHelpTranslation(ctx, q, a, req.Locale)
		missing := err == errHelpTranslationGone
		if err != nil && !missing {
			return err
		}
		switch {
		case missing && b.BaseVersion != 0:
			return s.helpConflict(ctx, q, c, a, nil)
		case !missing && b.BaseVersion != cur.Version:
			return s.helpConflict(ctx, q, c, a, &cur)
		}
		if missing {
			var images helpImageSet
			if images, err = s.helpImages(ctx, q, p.workspaceID, d.body); err != nil {
				return err
			}
			member, key := helpCreatedBy(p)
			saved, err = q.CreateHelpTranslation(ctx, store.CreateHelpTranslationParams{
				WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: req.Locale, Title: d.title, Body: d.body, Keywords: d.keywords,
				Slug: d.slug, ImageIds: images.ids(), SearchConfig: helpSearchConfig(req.Locale), MemberID: member, ApiKeyID: key, Now: s.now(),
			})
			created = true
		} else {
			saved, err = s.saveHelpDraft(ctx, q, p, cur, d)
		}
		if err != nil {
			return err
		}
		if err := s.addHelpRevision(ctx, q, p, saved, oas.HelpRevisionSave, saved.Title, saved.Body, saved.Keywords); err != nil {
			return err
		}
		if err := q.TouchHelpArticle(ctx, store.TouchHelpArticleParams{WorkspaceID: c.WorkspaceID, ID: a.ID, Now: s.now()}); err != nil {
			return err
		}
		out, err = s.helpTranslationOut(ctx, q, c, a, saved)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.helpArticleSignal(ctx, p, center, saved)
	if created {
		return oas.PutHelpArticleTranslation201JSONResponse(out), nil
	}
	return oas.PutHelpArticleTranslation200JSONResponse(out), nil
}

func (s *Server) DeleteHelpArticleTranslation(ctx context.Context, req oas.DeleteHelpArticleTranslationRequestObject) (oas.DeleteHelpArticleTranslationResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		a, err := helpArticle(ctx, q, c, req.ArticleId, true)
		if err != nil {
			return err
		}
		t, err := lockedHelpTranslation(ctx, q, a, req.Locale)
		if err != nil {
			return err
		}
		if t.PublishedAt != nil {
			if err := mayPublishHelp(p, c); err != nil {
				return err
			}
		}
		n, err := q.CountHelpTranslations(ctx, store.CountHelpTranslationsParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID})
		if err != nil {
			return err
		}
		if n <= 1 {
			return errHelpLastTranslation
		}
		if _, err := q.DeleteHelpTranslation(ctx, store.DeleteHelpTranslationParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: t.Locale}); err != nil {
			return err
		}
		if t.PublishedAt != nil {
			if err := s.touchHelpCenter(ctx, q, c); err != nil {
				return err
			}
		}
		ev.add(helpEventDeleted, &c.InboxID, nil, helpEventData(c, a, &t.Locale))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteHelpArticleTranslation204Response{}, nil
}

func (s *Server) PublishHelpArticleTranslation(ctx context.Context, req oas.PublishHelpArticleTranslationRequestObject) (oas.PublishHelpArticleTranslationResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var base *int32
	if req.Body != nil {
		base = req.Body.BaseVersion
	}
	var out oas.HelpArticleTranslation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		var err error
		out, err = s.publishHelpTranslation(ctx, q, ev, p, req.CenterId, req.ArticleId, req.Locale, base)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.PublishHelpArticleTranslation200JSONResponse(out), nil
}

func (s *Server) publishHelpTranslation(ctx context.Context, q *store.Queries, ev *eventBatch, p principal, centerID, articleID uuid.UUID, locale string, base *int32) (oas.HelpArticleTranslation, error) {
	c, err := visibleHelpCenter(ctx, q, p, centerID, true)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	if err := mayPublishHelp(p, c); err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	a, err := helpArticle(ctx, q, c, articleID, true)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	t, err := lockedHelpTranslation(ctx, q, a, locale)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	if base != nil && *base != t.Version {
		return oas.HelpArticleTranslation{}, s.helpConflict(ctx, q, c, a, &t)
	}
	if t.PublishedAt == nil || t.HasUnpublishedChanges {
		images, err := s.helpImages(ctx, q, p.workspaceID, t.Body)
		if err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		html := renderHelp(t.Body, images)
		member, key := helpCreatedBy(p)
		if t, err = q.PublishHelpTranslation(ctx, store.PublishHelpTranslationParams{
			WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: t.Locale, Html: &html, Excerpt: new(htmlExcerpt(html)),
			Now: new(s.now()), MemberID: member, ApiKeyID: key,
		}); err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		if err := s.addHelpRevision(ctx, q, p, t, oas.HelpRevisionPublish, t.Title, t.Body, t.Keywords); err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		if err := s.touchHelpCenter(ctx, q, c); err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		data := helpEventData(c, a, &t.Locale)
		data.Title, data.Url = t.PublishedTitle, s.helpPublicURL(c, a.Number, t)
		ev.add(helpEventPublished, &c.InboxID, nil, data)
	}
	return s.helpTranslationOut(ctx, q, c, a, t)
}

func (s *Server) UnpublishHelpArticleTranslation(ctx context.Context, req oas.UnpublishHelpArticleTranslationRequestObject) (oas.UnpublishHelpArticleTranslationResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out oas.HelpArticleTranslation
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		var err error
		out, err = s.unpublishHelpTranslation(ctx, q, ev, p, req.CenterId, req.ArticleId, req.Locale)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UnpublishHelpArticleTranslation200JSONResponse(out), nil
}

func (s *Server) unpublishHelpTranslation(ctx context.Context, q *store.Queries, ev *eventBatch, p principal, centerID, articleID uuid.UUID, locale string) (oas.HelpArticleTranslation, error) {
	c, err := visibleHelpCenter(ctx, q, p, centerID, true)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	if err := mayPublishHelp(p, c); err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	a, err := helpArticle(ctx, q, c, articleID, true)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	t, err := lockedHelpTranslation(ctx, q, a, locale)
	if err != nil {
		return oas.HelpArticleTranslation{}, err
	}
	if t.PublishedAt != nil {
		title := t.PublishedTitle
		if t, err = q.UnpublishHelpTranslation(ctx, store.UnpublishHelpTranslationParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: t.Locale}); err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		if err := s.touchHelpCenter(ctx, q, c); err != nil {
			return oas.HelpArticleTranslation{}, err
		}
		data := helpEventData(c, a, &t.Locale)
		data.Title = title
		ev.add(helpEventUnpublish, &c.InboxID, nil, data)
	}
	return s.helpTranslationOut(ctx, q, c, a, t)
}

func helpRevisionBody(r store.HelpArticleRevision) oas.HelpArticleRevision {
	return oas.HelpArticleRevision{
		Id: r.ID, Locale: r.Locale, Kind: oas.HelpRevisionKind(r.Kind), Title: r.Title, Body: r.Body, Keywords: r.Keywords,
		AuthorMemberId: r.AuthorMemberID, AuthorApiKeyId: r.AuthorApiKeyID, Via: r.Via, CreatedAt: r.CreatedAt,
	}
}

func (s *Server) ListHelpArticleRevisions(ctx context.Context, req oas.ListHelpArticleRevisionsRequestObject) (oas.ListHelpArticleRevisionsResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	a, err := helpArticle(ctx, s.st.Queries, c, req.ArticleId, false)
	if err != nil {
		return nil, err
	}
	if _, err := s.st.GetHelpTranslation(ctx, store.GetHelpTranslationParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: req.Locale}); err != nil {
		if store.IsNotFound(err) {
			return nil, errHelpTranslationGone
		}
		return nil, err
	}
	rows, err := s.st.ListHelpRevisions(ctx, store.ListHelpRevisionsParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: req.Locale})
	if err != nil {
		return nil, err
	}
	out := oas.ListHelpArticleRevisions200JSONResponse{Items: make([]oas.HelpArticleRevision, len(rows))}
	for i, r := range rows {
		out.Items[i] = helpRevisionBody(r)
	}
	return out, nil
}

func (s *Server) RestoreHelpArticleRevision(ctx context.Context, req oas.RestoreHelpArticleRevisionRequestObject) (oas.RestoreHelpArticleRevisionResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out oas.HelpArticleTranslation
	var saved store.HelpArticleTranslation
	var center store.HelpCenter
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		center = c
		a, err := helpArticle(ctx, q, c, req.ArticleId, true)
		if err != nil {
			return err
		}
		t, err := lockedHelpTranslation(ctx, q, a, req.Locale)
		if err != nil {
			return err
		}
		r, err := q.GetHelpRevision(ctx, store.GetHelpRevisionParams{WorkspaceID: c.WorkspaceID, ArticleID: a.ID, Locale: t.Locale, ID: req.RevisionId})
		if store.IsNotFound(err) {
			return errHelpRevisionGone
		}
		if err != nil {
			return err
		}
		if req.Body.BaseVersion != t.Version {
			return s.helpConflict(ctx, q, c, a, &t)
		}
		saved, err = s.saveHelpDraft(ctx, q, p, t, helpDraft{title: r.Title, body: r.Body, keywords: r.Keywords, slug: t.Slug})
		if err != nil {
			return err
		}
		if err := s.addHelpRevision(ctx, q, p, saved, oas.HelpRevisionRestore, saved.Title, saved.Body, saved.Keywords); err != nil {
			return err
		}
		if err := q.TouchHelpArticle(ctx, store.TouchHelpArticleParams{WorkspaceID: c.WorkspaceID, ID: a.ID, Now: s.now()}); err != nil {
			return err
		}
		out, err = s.helpTranslationOut(ctx, q, c, a, saved)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.helpArticleSignal(ctx, p, center, saved)
	return oas.RestoreHelpArticleRevision200JSONResponse(out), nil
}

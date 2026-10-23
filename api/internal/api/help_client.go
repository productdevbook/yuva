package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	helpClientSuggested   = 5
	helpClientPopular     = 5
	helpClientSearchLimit = 5
	helpClientSearchMax   = 10
)

type helpJSON struct{ body any }

func (h helpJSON) write(w http.ResponseWriter) error {
	raw, err := json.Marshal(h.body)
	if err != nil {
		return err
	}
	hd := w.Header()
	hd.Set("Content-Type", "application/json")
	hd.Set("Cache-Control", publicMaxAge)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(append(raw, '\n'))
	return err
}

type (
	clientHelpAnswer       struct{ helpJSON }
	clientHelpSearchAnswer struct{ helpJSON }
	clientHelpCollAnswer   struct{ helpJSON }
	clientHelpArtAnswer    struct{ helpJSON }
	clientHelpNoContent    struct{}
)

func (a clientHelpAnswer) VisitGetClientHelpResponse(w http.ResponseWriter) error { return a.write(w) }
func (a clientHelpSearchAnswer) VisitSearchClientHelpResponse(w http.ResponseWriter) error {
	return a.write(w)
}
func (a clientHelpCollAnswer) VisitGetClientHelpCollectionResponse(w http.ResponseWriter) error {
	return a.write(w)
}
func (a clientHelpArtAnswer) VisitGetClientHelpArticleResponse(w http.ResponseWriter) error {
	return a.write(w)
}

func (clientHelpNoContent) VisitCreateClientHelpReactionResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (clientHelpNoContent) VisitCreateClientHelpSearchClickResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) helpClientCenter(ctx context.Context, key string) (store.HelpCenter, store.FindChatChannelByKeyRow, error) {
	var none store.HelpCenter
	if err := s.helpOn(); err != nil {
		return none, store.FindChatChannelByKeyRow{}, err
	}
	if err := s.rateLimit(ctx, rateCheck{"help-client:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitChannelPerIP}); err != nil {
		return none, store.FindChatChannelByKeyRow{}, err
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 200 {
		return none, store.FindChatChannelByKeyRow{}, errUnknownChannelKey
	}
	ch, err := s.st.FindChatChannelByKey(ctx, key)
	if store.IsNotFound(err) {
		return none, ch, errUnknownChannelKey
	}
	if err != nil {
		return none, ch, err
	}
	if !s.originAllowedFor(ctx, ch.WorkspaceID, originFrom(ctx), ch.Kind, ch.AllowedOrigins) {
		return none, ch, errOriginRefused
	}
	c, err := s.st.GetInboxHelpCenter(ctx, store.GetInboxHelpCenterParams{WorkspaceID: ch.WorkspaceID, InboxID: ch.InboxID})
	if store.IsNotFound(err) || (err == nil && !c.Published) {
		return none, ch, errHelpNoCenter
	}
	return c, ch, err
}

func (s *Server) clientHelpInbox(ctx context.Context, q *store.Queries, in store.Inbox) (*oas.ClientInboxHelp, error) {
	if s.help.Off {
		return nil, nil
	}
	c, err := q.GetInboxHelpCenter(ctx, store.GetInboxHelpCenterParams{WorkspaceID: in.WorkspaceID, InboxID: in.ID})
	if store.IsNotFound(err) || (err == nil && !c.Published) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &oas.ClientInboxHelp{Url: s.helpURL(c, "/"), Locales: c.Locales, DefaultLocale: c.DefaultLocale}, nil
}

func (s *Server) clientHelpCollRef(index map[uuid.UUID]*helpColl, id *uuid.UUID) *oas.ClientHelpCollectionRef {
	if id == nil || index[*id] == nil {
		return nil
	}
	return &oas.ClientHelpCollectionRef{Id: *id, Name: index[*id].Name}
}

func (s *Server) clientHelpSummaries(c store.HelpCenter, arts []helpArt, index map[uuid.UUID]*helpColl) []oas.ClientHelpArticleSummary {
	out := make([]oas.ClientHelpArticleSummary, len(arts))
	for i, a := range arts {
		out[i] = oas.ClientHelpArticleSummary{
			Id: a.ID, Number: a.Number, Locale: a.Locale, Title: a.Title, Excerpt: a.Excerpt, Url: s.helpArtURL(c, a),
			Collection: s.clientHelpCollRef(index, a.CollectionID),
		}
	}
	return out
}

func (s *Server) clientHelpCollection(c store.HelpCenter, locale string, col *helpColl) oas.ClientHelpCollection {
	out := oas.ClientHelpCollection{
		Id: col.ID, Number: col.Number, ParentId: col.ParentID, Name: col.Name, Description: col.Description,
		ArticleCount: col.Total, Url: s.helpCollectionURL(c, locale, col),
	}
	if col.Icon != nil {
		out.Icon = new(oas.HelpIcon(*col.Icon))
	}
	return out
}

func (s *Server) GetClientHelp(ctx context.Context, req oas.GetClientHelpRequestObject) (oas.GetClientHelpResponseObject, error) {
	c, _, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	q := s.st.Queries
	locale := helpServedLocale(c, deref(req.Params.Locale))
	cols, index, err := s.helpCollections(ctx, q, c, locale)
	if err != nil {
		return nil, err
	}
	out := oas.ClientHelp{
		Center: oas.ClientHelpCenter{
			Name: c.Name, Description: c.Description, Locale: locale, Locales: c.Locales, DefaultLocale: c.DefaultLocale,
			Url: s.helpHomeURL(c, locale), LogoUrl: c.LogoUrl,
		},
		Collections: []oas.ClientHelpCollection{},
	}
	if c.AccentColor != nil {
		out.Center.AccentColor = new(oas.StatusColor(*c.AccentColor))
	}
	for _, col := range cols {
		out.Collections = append(out.Collections, s.clientHelpCollection(c, locale, col))
		for _, sub := range col.Subs {
			out.Collections = append(out.Collections, s.clientHelpCollection(c, locale, sub))
		}
	}
	suggested, err := s.helpSuggested(ctx, q, c, locale, deref(req.Params.Page), helpClientSuggested)
	if err != nil {
		return nil, err
	}
	popular, err := s.helpPopular(ctx, q, c, locale, helpClientPopular)
	if err != nil {
		return nil, err
	}
	out.Suggested, out.Popular = s.clientHelpSummaries(c, suggested, index), s.clientHelpSummaries(c, popular, index)
	return clientHelpAnswer{helpJSON{out}}, nil
}

func (s *Server) SearchClientHelp(ctx context.Context, req oas.SearchClientHelpRequestObject) (oas.SearchClientHelpResponseObject, error) {
	prm := req.Params
	if prm.Limit != nil && (*prm.Limit < 1 || *prm.Limit > helpClientSearchMax) {
		return nil, errValidation("limit must be 1 to 10")
	}
	if utf8.RuneCountInString(prm.Q) > maxHelpQueryLen {
		return nil, errValidation("q must be at most 200 characters")
	}
	c, ch, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	words := helpQueryWords(prm.Q)
	if len(words) == 0 {
		return nil, errHelpQueryEmpty
	}
	ip := s.clientIP(requestFrom(ctx))
	if err := s.rateLimit(ctx, append([]rateCheck{{"help-search:ip:" + rateIP(ip) + ":" + ch.ChannelID.String(), limitHelpSearchPerIP}},
		sharedChecks(ip, "help-search:center:"+c.ID.String(), budgetHelpSearch)...)...); err != nil {
		return nil, err
	}
	lim := int32(helpClientSearchLimit)
	if prm.Limit != nil {
		lim = *prm.Limit
	}
	locale := helpServedLocale(c, deref(prm.Locale))
	res, err := s.searchHelp(ctx, c, locale, words, false, lim)
	if err != nil {
		return nil, err
	}
	if boolOr(prm.Final, false) {
		s.countHelpSearch(c, locale, prm.Q, len(res.Items) > 0)
	}
	out := oas.ClientHelpSearchResults{Items: make([]oas.ClientHelpSearchResult, 0, len(res.Items)), Partial: res.Partial}
	indexes := map[string]map[uuid.UUID]*helpColl{}
	for _, r := range res.Items {
		if _, ok := indexes[r.Locale]; !ok {
			if _, indexes[r.Locale], err = s.helpCollections(ctx, s.st.Queries, c, r.Locale); err != nil {
				return nil, err
			}
		}
		out.Items = append(out.Items, oas.ClientHelpSearchResult{
			Id: r.ArticleId, Number: r.Number, Locale: r.Locale, Title: r.Title, Excerpt: r.Excerpt, Url: deref(r.Url),
			Collection: s.clientHelpCollRef(indexes[r.Locale], r.CollectionId), Highlights: r.Highlights,
		})
	}
	return clientHelpSearchAnswer{helpJSON{out}}, nil
}

func (s *Server) GetClientHelpCollection(ctx context.Context, req oas.GetClientHelpCollectionRequestObject) (oas.GetClientHelpCollectionResponseObject, error) {
	c, _, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	q := s.st.Queries
	locale := helpServedLocale(c, deref(req.Params.Locale))
	_, index, err := s.helpCollections(ctx, q, c, locale)
	if err != nil {
		return nil, err
	}
	col := index[req.CollectionId]
	if col == nil {
		return nil, errHelpCollectionGone
	}
	arts, err := s.helpPublishedArticles(ctx, q, c, locale, &col.ID)
	if err != nil {
		return nil, err
	}
	out := oas.ClientHelpCollectionDetail{
		Collection: s.clientHelpCollection(c, locale, col), Collections: []oas.ClientHelpCollection{},
		Articles: s.clientHelpSummaries(c, arts, index),
	}
	for _, sub := range col.Subs {
		out.Collections = append(out.Collections, s.clientHelpCollection(c, locale, sub))
	}
	return clientHelpCollAnswer{helpJSON{out}}, nil
}

func (s *Server) GetClientHelpArticle(ctx context.Context, req oas.GetClientHelpArticleRequestObject) (oas.GetClientHelpArticleResponseObject, error) {
	c, _, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	q := s.st.Queries
	locale := helpServedLocale(c, deref(req.Params.Locale))
	fallback := false
	a, err := q.GetPublicHelpArticle(ctx, store.GetPublicHelpArticleParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, ID: req.ArticleId})
	if store.IsNotFound(err) && locale != c.DefaultLocale {
		locale, fallback = c.DefaultLocale, true
		a, err = q.GetPublicHelpArticle(ctx, store.GetPublicHelpArticleParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, ID: req.ArticleId})
	}
	if store.IsNotFound(err) {
		return nil, errHelpArticleGone
	}
	if err != nil {
		return nil, err
	}
	r := requestFrom(ctx)
	s.countHelpView(ctx, r, c, a.ID, locale)
	_, index, err := s.helpCollections(ctx, q, c, locale)
	if err != nil {
		return nil, err
	}
	related, err := s.helpRelated(ctx, q, c, locale, a.ID, a.CollectionID)
	if err != nil {
		return nil, err
	}
	out := oas.ClientHelpArticle{
		Id: a.ID, Number: a.Number, Locale: locale, Fallback: fallback, Title: a.Title, BodyHtml: a.Html, UpdatedAt: a.PublishedAt,
		Url: s.helpArticleURL(c, a.Number, locale, a.Slug), Collection: s.clientHelpCollRef(index, a.CollectionID),
		Related: s.clientHelpSummaries(c, related, index),
	}
	return clientHelpArtAnswer{helpJSON{out}}, nil
}

func (s *Server) CreateClientHelpReaction(ctx context.Context, req oas.CreateClientHelpReactionRequestObject) (oas.CreateClientHelpReactionResponseObject, error) {
	b := req.Body
	if b == nil || !b.Rating.Valid() || (b.Previous != nil && !b.Previous.Valid()) {
		return nil, errValidation("rating and previous must be up or down")
	}
	if utf8.RuneCountInString(deref(b.Search)) > maxHelpQueryLen {
		return nil, errValidation("search must be at most 200 characters")
	}
	if err := s.rateLimit(ctx, rateCheck{"help-reaction:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitSessionPerIP}); err != nil {
		return nil, err
	}
	c, ch, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, sharedChecks(s.clientIP(requestFrom(ctx)), "help-reaction:channel:"+ch.ChannelID.String(), budgetSessions)...); err != nil {
		return nil, err
	}
	if err := offersLocale(c, b.Locale); err != nil {
		return nil, err
	}
	if err := s.helpPublishedIn(ctx, c, req.ArticleId, b.Locale); err != nil {
		return nil, err
	}
	var previous oas.PageRating
	if b.Previous != nil {
		previous = *b.Previous
	}
	s.countHelpReaction(c, req.ArticleId, b.Locale, b.Rating, previous)
	return clientHelpNoContent{}, nil
}

func (s *Server) helpPublishedIn(ctx context.Context, c store.HelpCenter, article uuid.UUID, locale string) error {
	_, err := s.st.GetPublicHelpArticle(ctx, store.GetPublicHelpArticleParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, Locale: locale, ID: article})
	if store.IsNotFound(err) {
		return errHelpArticleGone
	}
	return err
}

func (s *Server) CreateClientHelpSearchClick(ctx context.Context, req oas.CreateClientHelpSearchClickRequestObject) (oas.CreateClientHelpSearchClickResponseObject, error) {
	b := req.Body
	if b == nil || utf8.RuneCountInString(b.Q) > maxHelpQueryLen || len(helpQueryWords(b.Q)) == 0 {
		return nil, errHelpQueryEmpty
	}
	if err := s.rateLimit(ctx, rateCheck{"help-click:ip:" + rateIP(s.clientIP(requestFrom(ctx))), limitHelpClickPerIP}); err != nil {
		return nil, err
	}
	c, _, err := s.helpClientCenter(ctx, req.ChannelKey)
	if err != nil {
		return nil, err
	}
	if err := offersLocale(c, b.Locale); err != nil {
		return nil, err
	}
	if err := s.helpPublishedIn(ctx, c, b.ArticleId, b.Locale); err != nil {
		return nil, err
	}
	s.countHelpSearchClick(c, b.Locale, b.Q)
	return clientHelpNoContent{}, nil
}

func (s *Server) helpArticleLocales(ctx context.Context, q *store.Queries, ws, inboxID, article uuid.UUID) ([]store.GetHelpArticleLocalesForConversationRow, error) {
	return q.GetHelpArticleLocalesForConversation(ctx, store.GetHelpArticleLocalesForConversationParams{WorkspaceID: ws, ID: article, InboxID: inboxID})
}

func helpConversationLocale(rows []store.GetHelpArticleLocalesForConversationRow) string {
	for _, r := range rows {
		if r.Locale == r.DefaultLocale {
			return r.Locale
		}
	}
	locales := make([]string, len(rows))
	for i, r := range rows {
		locales[i] = r.Locale
	}
	slices.Sort(locales)
	return locales[0]
}

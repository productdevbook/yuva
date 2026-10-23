package api

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxHelpLocales      = 20
	maxHelpCollections  = 200
	helpRevisionsKept   = 50
	helpSaveRevisionGap = 10 * time.Minute
	helpEventPublished  = string(oas.HelpArticlePublishedEvent)
	helpEventUnpublish  = string(oas.HelpArticleUnpublishedEvent)
	helpEventDeleted    = string(oas.HelpArticleDeletedEvent)
	helpSignalUpdated   = string(oas.HelpArticleUpdated)
	helpPathPrefix      = "/help/"
	helpFormerSlugsKept = 10
	helpFormerSlugDays  = 90
)

var (
	limitHelpSlug = limit{10, 24 * time.Hour}

	errHelpOff              = problem(http.StatusNotFound, "help_off", "help centers are off on this server")
	errHelpNoCenter         = problem(http.StatusNotFound, "help_off", "this inbox has no published help center")
	errHelpCenterGone       = problem(http.StatusNotFound, "not_found", "no such help center")
	errHelpCollectionGone   = problem(http.StatusNotFound, "not_found", "no such collection")
	errHelpArticleGone      = problem(http.StatusNotFound, "not_found", "no such article")
	errHelpTranslationGone  = problem(http.StatusNotFound, "not_found", "no such translation")
	errHelpRevisionGone     = problem(http.StatusNotFound, "not_found", "no such revision")
	errHelpCenterExists     = problem(http.StatusConflict, "help_center_exists", "this inbox already has a help center")
	errHelpSlugTaken        = problem(http.StatusConflict, "slug_taken", "another help center has this slug")
	errHelpLastTranslation  = problem(http.StatusConflict, "last_translation", "an article keeps at least one translation; delete the article instead")
	errHelpQueryEmpty       = problem(http.StatusBadRequest, "query_empty", "the query has no words to search for")
	errHelpSearchTimeout    = problem(http.StatusServiceUnavailable, "search_timeout", "the search took too long; try fewer or longer words")
	errHelpLocaleNotOffered = func(locale string) *apiError {
		return problem(http.StatusBadRequest, "locale_not_offered", "the center does not offer the locale "+locale)
	}
)

// HelpSettings come from YUVA_HELP_URL (empty: <public URL>/help), YUVA_HELP=off and YUVA_HELP_MAX_ARTICLES (0: no limit).
type HelpSettings struct {
	URL         string
	Off         bool
	MaxArticles int
}

func (s *Server) helpOn() error {
	if s.help.Off {
		return errHelpOff
	}
	return nil
}

func (s *Server) helpBase() string {
	if s.help.URL != "" {
		return s.help.URL
	}
	return s.auth.PublicURL + helpPathPrefix[:len(helpPathPrefix)-1]
}

func (s *Server) helpURL(c store.HelpCenter, path string) string {
	if s.domainLive(c.CustomDomain, c.DomainState, c.CfStatus, c.CfSslStatus) {
		return s.domainURL(*c.CustomDomain, path)
	}
	return s.helpBase() + "/" + c.Slug + path
}

func (s *Server) helpArticleURL(c store.HelpCenter, number int32, locale, slug string) string {
	return s.helpURL(c, "/"+locale+"/articles/"+strconv.Itoa(int(number))+"-"+slug)
}

func helpUniqueConflict(err error) error {
	switch store.UniqueViolationConstraint(err) {
	case "help_centers_workspace_id_inbox_id_key":
		return errHelpCenterExists
	case "help_centers_slug_key", "help_center_slugs_pkey":
		return errHelpSlugTaken
	}
	return err
}

func visibleHelpCenter(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.HelpCenter, error) {
	get := q.GetHelpCenter
	if lock {
		get = func(ctx context.Context, arg store.GetHelpCenterParams) (store.HelpCenter, error) {
			return q.LockHelpCenter(ctx, store.LockHelpCenterParams(arg))
		}
	}
	c, err := get(ctx, store.GetHelpCenterParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return c, errHelpCenterGone
	}
	if err != nil {
		return c, err
	}
	ok, err := canSeeInbox(ctx, q, p, c.InboxID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, errHelpCenterGone
	}
	return c, nil
}

// OAuth tokens never change what the public sees, whatever the center allows.
func mayPublishHelp(p principal, c store.HelpCenter) error {
	if p.isGrant() {
		return errMemberSessionRequired
	}
	if p.isKey() {
		return requireScope(p, oas.HelpPublish)
	}
	if p.role == roleOwner || p.role == roleAdmin || c.AgentsMayPublish {
		return nil
	}
	return errForbidden
}

func requireCenterManager(p principal) error {
	if p.isGrant() {
		return errMemberSessionRequired
	}
	return requireManagerOrKey(p)
}

func helpCreatedBy(p principal) (member, key *uuid.UUID) {
	if p.isKey() {
		return nil, new(p.keyID)
	}
	return p.memberRef(), nil
}

func validHelpLocale(v, field string) error {
	if len(v) > 35 || !languagePattern.MatchString(v) {
		return errValidation(field + " must be a language tag such as en or tr")
	}
	return nil
}

func offersLocale(c store.HelpCenter, locale string) error {
	if err := validHelpLocale(locale, "locale"); err != nil {
		return err
	}
	if !slices.Contains(c.Locales, locale) {
		return errHelpLocaleNotOffered(locale)
	}
	return nil
}

type helpCenterFields struct {
	name, description, slug, defaultLocale string
	locales                                []string
	logo, color, site                      *string
	chat, property                         *uuid.UUID
	agentsPublish, published               bool
}

func (f *helpCenterFields) check(ctx context.Context, q *store.Queries, ws, inboxID uuid.UUID) error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	if f.description, err = trimmed(f.description, 0, 1000, "description"); err != nil {
		return err
	}
	if err := validPublicSlug(f.slug); err != nil {
		return err
	}
	if err := validHelpLocale(f.defaultLocale, "default_locale"); err != nil {
		return err
	}
	if len(f.locales) < 1 || len(f.locales) > maxHelpLocales {
		return errValidation("locales must hold 1 to 20 language tags")
	}
	for i, l := range f.locales {
		if err := validHelpLocale(l, "locales"); err != nil {
			return err
		}
		if slices.Contains(f.locales[:i], l) {
			return errValidation("locales names " + l + " twice")
		}
	}
	if !slices.Contains(f.locales, f.defaultLocale) {
		return errValidation("locales must hold default_locale")
	}
	if f.color != nil && !colorPattern.MatchString(*f.color) {
		return errValidation("accent_color must be #rrggbb")
	}
	if f.logo, err = statusLink(f.logo, "logo_url", "https"); err != nil {
		return err
	}
	if f.site, err = statusLink(f.site, "site_url", "https", "http"); err != nil {
		return err
	}
	if f.chat != nil {
		ch, err := q.GetChannel(ctx, store.GetChannelParams{WorkspaceID: ws, ID: *f.chat})
		if store.IsNotFound(err) || (err == nil && (ch.InboxID != inboxID || ch.Kind != "chat")) {
			return errValidation("chat_channel_id must be a chat channel of the center's inbox")
		}
		if err != nil {
			return err
		}
	}
	if f.property != nil {
		_, err := q.GetAnalyticsProperty(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: ws, ID: *f.property})
		if store.IsNotFound(err) {
			return errValidation("analytics_property_id must be an analytics property of the workspace")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) helpCenterOut(ctx context.Context, q *store.Queries, p principal, c store.HelpCenter) (oas.HelpCenter, error) {
	n, err := q.CountHelpCenter(ctx, store.CountHelpCenterParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
	if err != nil {
		return oas.HelpCenter{}, err
	}
	out := oas.HelpCenter{
		Id: c.ID, InboxId: c.InboxID, Name: c.Name, Description: c.Description, Slug: oas.HelpCenterSlug(c.Slug),
		Url: s.helpURL(c, "/"), DefaultLocale: c.DefaultLocale, Locales: c.Locales, LogoUrl: c.LogoUrl, SiteUrl: c.SiteUrl,
		ChatChannelId: c.ChatChannelID, AnalyticsPropertyId: c.AnalyticsPropertyID, AgentsMayPublish: c.AgentsMayPublish,
		Published: c.Published, CanPublish: mayPublishHelp(p, c) == nil, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		Counts:       oas.HelpCenterCounts{Collections: n.Collections, Articles: n.Articles, PublishedArticles: n.PublishedArticles},
		CustomDomain: s.customDomainBody(helpOwn(c)), OwnDomains: s.ownDomainsBody(),
	}
	if c.AccentColor != nil {
		out.AccentColor = new(oas.StatusColor(*c.AccentColor))
	}
	return out, nil
}

func (s *Server) ListHelpCenters(ctx context.Context, req oas.ListHelpCentersRequestObject) (oas.ListHelpCentersResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	rows, err := s.st.ListHelpCenters(ctx, store.ListHelpCentersParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListHelpCenters200JSONResponse{Items: make([]oas.HelpCenter, len(rows))}
	for i, c := range rows {
		if out.Items[i], err = s.helpCenterOut(ctx, s.st.Queries, p, c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) CreateHelpCenter(ctx context.Context, req oas.CreateHelpCenterRequestObject) (oas.CreateHelpCenterResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireCenterManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.HelpCenter
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		in, err := visibleInbox(ctx, q, p, b.InboxId)
		if err != nil {
			return err
		}
		f := helpCenterFields{
			name: b.Name, slug: string(b.Slug), defaultLocale: in.DefaultLocale, logo: b.LogoUrl, site: b.SiteUrl,
			chat: b.ChatChannelId, property: b.AnalyticsPropertyId, agentsPublish: boolOr(b.AgentsMayPublish, false),
			published: boolOr(b.Published, false),
		}
		if b.Description != nil {
			f.description = *b.Description
		}
		if b.DefaultLocale != nil {
			f.defaultLocale = *b.DefaultLocale
		}
		f.locales = []string{f.defaultLocale}
		if b.Locales != nil {
			f.locales = *b.Locales
		}
		if b.AccentColor != nil {
			f.color = new(string(*b.AccentColor))
		}
		if err := f.check(ctx, q, p.workspaceID, in.ID); err != nil {
			return err
		}
		if _, err := q.GetInboxHelpCenter(ctx, store.GetInboxHelpCenterParams{WorkspaceID: p.workspaceID, InboxID: in.ID}); err == nil {
			return errHelpCenterExists
		} else if !store.IsNotFound(err) {
			return err
		}
		if taken, err := q.HelpSlugTaken(ctx, store.HelpSlugTakenParams{Slug: f.slug, ID: uuid.Nil()}); err != nil || taken {
			if err == nil {
				err = errHelpSlugTaken
			}
			return err
		}
		c, err := q.CreateHelpCenter(ctx, store.CreateHelpCenterParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: in.ID, Name: f.name, Description: f.description, Slug: f.slug,
			DefaultLocale: f.defaultLocale, Locales: f.locales, LogoUrl: f.logo, AccentColor: f.color, SiteUrl: f.site,
			ChatChannelID: f.chat, AnalyticsPropertyID: f.property, AgentsMayPublish: f.agentsPublish, Published: f.published, Now: s.now(),
		})
		if err != nil {
			return err
		}
		out, err = s.helpCenterOut(ctx, q, p, c)
		return err
	})
	if err != nil {
		return nil, helpUniqueConflict(err)
	}
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	return oas.CreateHelpCenter201JSONResponse(out), nil
}

func (s *Server) GetHelpCenter(ctx context.Context, req oas.GetHelpCenterRequestObject) (oas.GetHelpCenterResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.helpCenterOut(ctx, s.st.Queries, p, c)
	if err != nil {
		return nil, err
	}
	return oas.GetHelpCenter200JSONResponse(out), nil
}

func nullableID(v interface {
	IsSpecified() bool
	IsNull() bool
	Get() (uuid.UUID, error)
}, cur *uuid.UUID) *uuid.UUID {
	if !v.IsSpecified() {
		return cur
	}
	if v.IsNull() {
		return nil
	}
	id, _ := v.Get()
	return &id
}

func (s *Server) UpdateHelpCenter(ctx context.Context, req oas.UpdateHelpCenterRequestObject) (oas.UpdateHelpCenterResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireCenterManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.HelpCenter
	var hosts []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		cur, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		if cur.CustomDomain != nil {
			hosts = append(hosts, *cur.CustomDomain)
		}
		f := helpCenterFields{
			name: cur.Name, description: cur.Description, slug: cur.Slug, defaultLocale: cur.DefaultLocale, locales: cur.Locales,
			logo: nullableLink(b.LogoUrl, cur.LogoUrl), color: nullableLink(b.AccentColor, cur.AccentColor),
			site: nullableLink(b.SiteUrl, cur.SiteUrl), chat: nullableID(b.ChatChannelId, cur.ChatChannelID),
			property: nullableID(b.AnalyticsPropertyId, cur.AnalyticsPropertyID), agentsPublish: boolOr(b.AgentsMayPublish, cur.AgentsMayPublish),
			published: boolOr(b.Published, cur.Published),
		}
		if b.Name != nil {
			f.name = *b.Name
		}
		if b.Description != nil {
			f.description = *b.Description
		}
		if b.Slug != nil {
			f.slug = string(*b.Slug)
		}
		if b.DefaultLocale != nil {
			f.defaultLocale = *b.DefaultLocale
		}
		if b.Locales != nil {
			f.locales = *b.Locales
		}
		if err := f.check(ctx, q, p.workspaceID, cur.InboxID); err != nil {
			return err
		}
		used, err := q.HelpLocalesInUse(ctx, store.HelpLocalesInUseParams{WorkspaceID: p.workspaceID, CenterID: cur.ID, Locales: f.locales})
		if err != nil {
			return err
		}
		if len(used) > 0 {
			e := problem(http.StatusConflict, "locale_in_use", "articles or collections have translations in "+strings.Join(used, ", "))
			e.Extra = map[string]any{"locales": used}
			return e
		}
		now := s.now()
		if f.slug != cur.Slug {
			if err := s.rateLimit(ctx, rateCheck{"help-slug:" + cur.ID.String(), limitHelpSlug}); err != nil {
				return err
			}
			if taken, err := q.HelpSlugTaken(ctx, store.HelpSlugTakenParams{Slug: f.slug, ID: cur.ID}); err != nil || taken {
				if err == nil {
					err = errHelpSlugTaken
				}
				return err
			}
			if err := q.ReleaseHelpFormerSlug(ctx, store.ReleaseHelpFormerSlugParams{WorkspaceID: cur.WorkspaceID, CenterID: cur.ID, Slug: f.slug}); err != nil {
				return err
			}
			if err := q.AddHelpFormerSlug(ctx, store.AddHelpFormerSlugParams{Slug: cur.Slug, WorkspaceID: cur.WorkspaceID, CenterID: cur.ID, Now: now}); err != nil {
				return err
			}
			if err := q.PruneHelpFormerSlugs(ctx, store.PruneHelpFormerSlugsParams{WorkspaceID: cur.WorkspaceID, CenterID: cur.ID, Keep: helpFormerSlugsKept}); err != nil {
				return err
			}
		}
		c, err := q.UpdateHelpCenter(ctx, store.UpdateHelpCenterParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Description: f.description, Slug: f.slug,
			DefaultLocale: f.defaultLocale, Locales: f.locales, LogoUrl: f.logo, AccentColor: f.color, SiteUrl: f.site,
			ChatChannelID: f.chat, AnalyticsPropertyID: f.property, AgentsMayPublish: f.agentsPublish, Published: f.published, Now: now,
		})
		if err != nil {
			return err
		}
		if sent, raw := customDomainField(b.CustomDomain); sent {
			d, err := s.setOwnDomain(ctx, q, ev, helpOwn(c), raw)
			if err != nil {
				return err
			}
			c = d.row.(store.HelpCenter)
			if c.CustomDomain != nil {
				hosts = append(hosts, *c.CustomDomain)
			}
		}
		out, err = s.helpCenterOut(ctx, q, p, c)
		return err
	})
	if err != nil {
		return nil, helpUniqueConflict(err)
	}
	s.statusDomainCache.forget(req.CenterId, hosts...)
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	s.helpCenterSignal(ctx, p.workspaceID, out.InboxId, out.Id)
	return oas.UpdateHelpCenter200JSONResponse(out), nil
}

func (s *Server) DeleteHelpCenter(ctx context.Context, req oas.DeleteHelpCenterRequestObject) (oas.DeleteHelpCenterResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireCenterManager(p); err != nil {
		return nil, err
	}
	var inbox uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		inbox = c.InboxID
		releaseOwnCloudflare(ev, helpOwn(c))
		_, err = q.DeleteHelpCenter(ctx, store.DeleteHelpCenterParams{WorkspaceID: p.workspaceID, ID: c.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.statusDomainCache.forget(req.CenterId)
	s.statusHint(ctx, p.workspaceID, inbox)
	return oas.DeleteHelpCenter204Response{}, nil
}

func (s *Server) touchHelpCenter(ctx context.Context, q *store.Queries, c store.HelpCenter) error {
	return q.TouchHelpCenter(ctx, store.TouchHelpCenterParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: s.now()})
}

func helpCollectionBody(c store.HelpCollection, articles int32, ts []store.HelpCollectionTranslation) oas.HelpCollection {
	out := oas.HelpCollection{
		Id: c.ID, CenterId: c.CenterID, ParentId: c.ParentID, Number: c.Number, Position: c.Position, ArticleCount: articles,
		Translations: map[string]oas.HelpCollectionTranslation{}, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
	if c.Icon != nil {
		out.Icon = new(oas.HelpIcon(*c.Icon))
	}
	for _, t := range ts {
		if t.CollectionID == c.ID {
			out.Translations[t.Locale] = oas.HelpCollectionTranslation{Name: t.Name, Description: t.Description, Slug: t.Slug}
		}
	}
	return out
}

func (s *Server) helpCollectionOut(ctx context.Context, q *store.Queries, c store.HelpCollection) (oas.HelpCollection, error) {
	ts, err := q.ListHelpCollectionTranslations(ctx, store.ListHelpCollectionTranslationsParams{WorkspaceID: c.WorkspaceID, CollectionIds: []uuid.UUID{c.ID}})
	if err != nil {
		return oas.HelpCollection{}, err
	}
	n, err := q.CountHelpCollectionArticles(ctx, store.CountHelpCollectionArticlesParams{WorkspaceID: c.WorkspaceID, CollectionID: &c.ID})
	if err != nil {
		return oas.HelpCollection{}, err
	}
	return helpCollectionBody(c, n, ts), nil
}

func helpCollection(ctx context.Context, q *store.Queries, c store.HelpCenter, id uuid.UUID) (store.HelpCollection, error) {
	col, err := q.GetHelpCollection(ctx, store.GetHelpCollectionParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: id})
	if store.IsNotFound(err) {
		return col, errHelpCollectionGone
	}
	return col, err
}

func validHelpIcon(v string) error {
	if !oas.HelpIcon(v).Valid() {
		return errValidation("icon must be one of the HelpIcon names")
	}
	return nil
}

func helpParent(ctx context.Context, q *store.Queries, c store.HelpCenter, parent *uuid.UUID, self uuid.UUID) error {
	if parent == nil {
		return nil
	}
	if *parent == self {
		return errValidation("a collection cannot be its own parent")
	}
	pc, err := q.GetHelpCollection(ctx, store.GetHelpCollectionParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: *parent})
	if store.IsNotFound(err) {
		return errValidation("parent_id must be a collection of the center")
	}
	if err != nil {
		return err
	}
	if pc.ParentID != nil {
		return errValidation("a sub-collection cannot have sub-collections")
	}
	return nil
}

func (s *Server) writeHelpCollectionTranslations(ctx context.Context, q *store.Queries, c store.HelpCenter, colID uuid.UUID, ts map[string]oas.HelpCollectionTranslationInput) error {
	for locale, t := range ts {
		if err := offersLocale(c, locale); err != nil {
			return err
		}
		name, err := trimmed(t.Name, 1, 100, "translations."+locale+".name")
		if err != nil {
			return err
		}
		desc := ""
		if t.Description != nil {
			if desc, err = trimmed(*t.Description, 0, 500, "translations."+locale+".description"); err != nil {
				return err
			}
		}
		if err := q.UpsertHelpCollectionTranslation(ctx, store.UpsertHelpCollectionTranslationParams{
			WorkspaceID: c.WorkspaceID, CollectionID: colID, Locale: locale, Name: name, Description: desc, Slug: helpSlug(name, "collection"), Now: s.now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) ListHelpCollections(ctx context.Context, req oas.ListHelpCollectionsRequestObject) (oas.ListHelpCollectionsResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleHelpCenter(ctx, s.st.Queries, p, req.CenterId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListHelpCollections(ctx, store.ListHelpCollectionsParams{WorkspaceID: p.workspaceID, CenterID: c.ID})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	ts, err := s.st.ListHelpCollectionTranslations(ctx, store.ListHelpCollectionTranslationsParams{WorkspaceID: p.workspaceID, CollectionIds: ids})
	if err != nil {
		return nil, err
	}
	out := oas.ListHelpCollections200JSONResponse{Items: make([]oas.HelpCollection, len(rows))}
	for i, r := range rows {
		col := store.HelpCollection{
			ID: r.ID, WorkspaceID: r.WorkspaceID, CenterID: r.CenterID, ParentID: r.ParentID, Number: r.Number, Icon: r.Icon,
			Position: r.Position, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		out.Items[i] = helpCollectionBody(col, r.ArticleCount, ts)
	}
	return out, nil
}

func (s *Server) CreateHelpCollection(ctx context.Context, req oas.CreateHelpCollectionRequestObject) (oas.CreateHelpCollectionResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if len(b.Translations) == 0 {
		return nil, errValidation("translations must name the collection in at least one locale")
	}
	if b.Icon != nil {
		if err := validHelpIcon(string(*b.Icon)); err != nil {
			return nil, err
		}
	}
	var out oas.HelpCollection
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		n, err := q.CountHelpCollections(ctx, store.CountHelpCollectionsParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID})
		if err != nil {
			return err
		}
		if n >= maxHelpCollections {
			return errValidation("a center has at most 200 collections")
		}
		id := newID()
		if err := helpParent(ctx, q, c, b.ParentId, id); err != nil {
			return err
		}
		number, err := q.TakeHelpCollectionNumber(ctx, store.TakeHelpCollectionNumberParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		if err != nil {
			return err
		}
		var icon *string
		if b.Icon != nil {
			icon = new(string(*b.Icon))
		}
		col, err := q.CreateHelpCollection(ctx, store.CreateHelpCollectionParams{
			ID: id, WorkspaceID: c.WorkspaceID, CenterID: c.ID, ParentID: b.ParentId, Number: number, Icon: icon,
			Position: int32OrZero(b.Position), Now: s.now(),
		})
		if err != nil {
			return err
		}
		if err := s.writeHelpCollectionTranslations(ctx, q, c, col.ID, b.Translations); err != nil {
			return err
		}
		if err := s.touchHelpCenter(ctx, q, c); err != nil {
			return err
		}
		out, err = s.helpCollectionOut(ctx, q, col)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateHelpCollection201JSONResponse(out), nil
}

func int32OrZero(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func (s *Server) UpdateHelpCollection(ctx context.Context, req oas.UpdateHelpCollectionRequestObject) (oas.UpdateHelpCollectionResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	var out oas.HelpCollection
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		cur, err := helpCollection(ctx, q, c, req.CollectionId)
		if err != nil {
			return err
		}
		parent := nullableID(b.ParentId, cur.ParentID)
		if b.ParentId.IsSpecified() && parent != nil {
			if err := helpParent(ctx, q, c, parent, cur.ID); err != nil {
				return err
			}
			kids, err := q.HelpCollectionHasChildren(ctx, store.HelpCollectionHasChildrenParams{WorkspaceID: c.WorkspaceID, ID: &cur.ID})
			if err != nil {
				return err
			}
			if kids {
				return errValidation("a collection with sub-collections cannot get a parent")
			}
		}
		icon := nullableLink(b.Icon, cur.Icon)
		if icon != nil {
			if err := validHelpIcon(*icon); err != nil {
				return err
			}
		}
		position := cur.Position
		if b.Position != nil {
			position = *b.Position
		}
		col, err := q.UpdateHelpCollection(ctx, store.UpdateHelpCollectionParams{
			WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: cur.ID, ParentID: parent, Icon: icon, Position: position, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if b.Translations != nil {
			if err := s.writeHelpCollectionTranslations(ctx, q, c, col.ID, *b.Translations); err != nil {
				return err
			}
		}
		if b.RemoveLocales != nil && len(*b.RemoveLocales) > 0 {
			if err := q.DeleteHelpCollectionTranslations(ctx, store.DeleteHelpCollectionTranslationsParams{
				WorkspaceID: c.WorkspaceID, CollectionID: col.ID, Locales: *b.RemoveLocales,
			}); err != nil {
				return err
			}
			n, err := q.CountHelpCollectionTranslations(ctx, store.CountHelpCollectionTranslationsParams{WorkspaceID: c.WorkspaceID, CollectionID: col.ID})
			if err != nil {
				return err
			}
			if n == 0 {
				return errValidation("a collection keeps at least one translation")
			}
		}
		if err := s.touchHelpCenter(ctx, q, c); err != nil {
			return err
		}
		out, err = s.helpCollectionOut(ctx, q, col)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateHelpCollection200JSONResponse(out), nil
}

func (s *Server) DeleteHelpCollection(ctx context.Context, req oas.DeleteHelpCollectionRequestObject) (oas.DeleteHelpCollectionResponseObject, error) {
	if err := s.helpOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleHelpCenter(ctx, q, p, req.CenterId, true)
		if err != nil {
			return err
		}
		n, err := q.DeleteHelpCollection(ctx, store.DeleteHelpCollectionParams{WorkspaceID: c.WorkspaceID, CenterID: c.ID, ID: req.CollectionId})
		if err != nil {
			return err
		}
		if n == 0 {
			return errHelpCollectionGone
		}
		return s.touchHelpCenter(ctx, q, c)
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteHelpCollection204Response{}, nil
}

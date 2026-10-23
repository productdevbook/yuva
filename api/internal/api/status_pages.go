package api

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/cfsaas"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxStatusComponents   = 100
	defaultStatusHistory  = 90
	minStatusHistory      = 7
	statusEventComponent  = string(oas.StatusComponentEventTypeStatusComponentUpdated)
	statusEventCreated    = string(oas.StatusIncidentCreatedEvent)
	statusEventUpdated    = string(oas.StatusIncidentUpdatedEvent)
	statusEventResolved   = string(oas.StatusIncidentResolvedEvent)
	statusSourceMember    = string(oas.StatusSourceMember)
	statusSourceAPIKey    = string(oas.StatusSourceApiKey)
	statusSourceIncident  = string(oas.StatusSourceIncident)
	statusSourceMonitor   = string(oas.StatusSourceMonitor)
	statusComponentsLimit = "a page has at most 100 components"
)

var (
	errStatusPageGone      = problem(http.StatusNotFound, "not_found", "no such status page")
	errStatusGroupGone     = problem(http.StatusNotFound, "not_found", "no such component group")
	errStatusComponentGone = problem(http.StatusNotFound, "not_found", "no such component")
	errStatusIncidentGone  = problem(http.StatusNotFound, "not_found", "no such incident")
	errStatusUpdateGone    = problem(http.StatusNotFound, "not_found", "no such update")
	errStatusSlugTaken     = problem(http.StatusConflict, "slug_taken", "another status page has this slug")
	errStatusPageExists    = problem(http.StatusConflict, "status_page_exists", "this inbox already has a status page")
	errIncidentClosed      = problem(http.StatusConflict, "incident_closed", "the incident is resolved or the maintenance completed")
	errIncidentPublished   = problem(http.StatusConflict, "incident_published", "only a draft can be deleted")

	statusSlugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	reservedSlugs     = []string{"admin", "api", "app", "assets", "badge", "client", "docs", "feed", "help", "history", "incidents", "login",
		"mail", "new", "oauth", "panel", "settings", "signin", "static", "status", "support", "v1", "www", "yuva"}
)

// StatusSettings: URL is YUVA_STATUS_URL, the origin public pages are named from; empty means
// <public URL>/status.
// Monitors is YUVA_STATUS_MONITORS, the most monitors a workspace may have; 0 means no limit.
// NoticesOff is YUVA_STATUS_NOTICES=off; NoticesDaily and MaxSubscribers are
// YUVA_STATUS_NOTICES_DAILY and YUVA_STATUS_MAX_SUBSCRIBERS, 0 meaning no limit.
// HomeOff is YUVA_STATUS_HOME=off; HomePage is YUVA_STATUS_HOME_PAGE, the slug of the page shown as the
// home page's live card.
type StatusSettings struct {
	URL            string
	HomeOff        bool
	HomePage       string
	Monitors       int
	NoticesOff     bool
	NoticesDaily   int64
	MaxSubscribers int
	// MinInterval is the shortest interval of http and tcp monitors in seconds, at least 60.
	MinInterval int
	// CanaryURLs are checked when a round has failures; nil checks the built-in two.
	CanaryURLs  []string
	CanariesOff bool
	// AllowPrivate lets http and tcp monitors reach private and loopback addresses (a self-hosted
	// server with YUVA_WEBHOOK_ALLOW_PRIVATE); link-local and metadata addresses stay refused.
	AllowPrivate bool
	// RootCAs replaces the system roots for https checks; nil uses the system's.
	RootCAs *x509.CertPool

	Domains     string
	CNAME       string
	DomainLimit int
	Cloudflare  *cfsaas.Client
	Resolver    StatusResolver
}

func (s *Server) statusBase() string {
	if s.status.URL != "" {
		return s.status.URL
	}
	return s.auth.PublicURL + statusPathPrefix[:len(statusPathPrefix)-1]
}

func (s *Server) statusURL(slug, path string) string {
	return s.statusBase() + "/" + slug + path
}

func statusUniqueConflict(err error) error {
	switch store.UniqueViolationConstraint(err) {
	case "status_pages_workspace_id_inbox_id_key":
		return errStatusPageExists
	case "status_pages_slug_key", "status_page_slugs_pkey":
		return errStatusSlugTaken
	case "status_pages_custom_domain_verified_key":
		return errStatusDomainTaken
	}
	return err
}

func validPublicSlug(slug string) error {
	if len(slug) < 3 || len(slug) > 48 || !statusSlugPattern.MatchString(slug) {
		return errValidation("slug must be 3 to 48 lowercase letters, digits and single dashes")
	}
	if slices.Contains(reservedSlugs, slug) {
		return errValidation("slug " + slug + " is reserved")
	}
	return nil
}

func statusPageVisible(ctx context.Context, q *store.Queries, p principal, pg store.StatusPage) (bool, error) {
	return canSeeInbox(ctx, q, p, pg.InboxID)
}

func visibleStatusPage(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.StatusPage, error) {
	get := q.GetStatusPage
	if lock {
		get = func(ctx context.Context, arg store.GetStatusPageParams) (store.StatusPage, error) {
			return q.LockStatusPage(ctx, store.LockStatusPageParams(arg))
		}
	}
	pg, err := get(ctx, store.GetStatusPageParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return pg, errStatusPageGone
	}
	if err != nil {
		return pg, err
	}
	ok, err := statusPageVisible(ctx, q, p, pg)
	if err != nil {
		return pg, err
	}
	if !ok {
		return pg, errStatusPageGone
	}
	return pg, nil
}

func statusSource(p principal) (string, *uuid.UUID) {
	if p.isKey() {
		id := p.keyID
		return statusSourceAPIKey, &id
	}
	id := p.memberID
	return statusSourceMember, &id
}

func statusPageRef(pg store.StatusPage) oas.StatusPageRef {
	return oas.StatusPageRef{Id: pg.ID, Name: pg.Name, Slug: pg.Slug}
}

func (s *Server) statusPageBody(pg store.StatusPage, state string, email oas.StatusEmailSubscribers) oas.StatusPage {
	out := oas.StatusPage{
		Id: pg.ID, InboxId: pg.InboxID, Name: pg.Name, Description: pg.Description, Slug: oas.StatusPageSlug(pg.Slug),
		Url: s.pageURL(pg, ""), LogoUrl: pg.LogoUrl, LogoDarkUrl: pg.LogoDarkUrl, SiteUrl: pg.SiteUrl, SupportUrl: pg.SupportUrl, Locale: pg.Locale,
		Timezone: pg.Timezone, HistoryDays: pg.HistoryDays, ShowUptime: pg.ShowUptime, ShowResponseTimes: pg.ShowResponseTimes,
		ShowName: pg.ShowName, ShowFollow: pg.ShowFollow, Published: pg.Published, State: oas.StatusComponentStateValue(state), CreatedAt: pg.CreatedAt, UpdatedAt: pg.UpdatedAt,
		EmailSubscribers: email, CustomDomain: s.customDomainBody(pg), OwnDomains: s.ownDomainsBody(),
	}
	if pg.AccentColor != nil {
		c := oas.StatusColor(*pg.AccentColor)
		out.AccentColor = &c
	}
	return out
}

func pageState(comps []store.StatusComponent) string {
	var states []string
	for _, c := range comps {
		if !c.Hidden {
			states = append(states, c.State)
		}
	}
	return worstState(states)
}

func (s *Server) statusPageOut(ctx context.Context, q *store.Queries, pg store.StatusPage) (oas.StatusPage, error) {
	comps, err := q.ListStatusComponents(ctx, store.ListStatusComponentsParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID})
	if err != nil {
		return oas.StatusPage{}, err
	}
	email, err := s.statusEmailBody(ctx, q, pg)
	if err != nil {
		return oas.StatusPage{}, err
	}
	return s.statusPageBody(pg, pageState(comps), email), nil
}

func statusLink(raw *string, field string, schemes ...string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return nil, nil
	}
	u, err := url.Parse(v)
	if err != nil || len(v) > 2000 || !slices.Contains(schemes, u.Scheme) || (u.Scheme != "mailto" && u.Host == "") || (u.Scheme == "mailto" && u.Opaque == "") {
		return nil, errValidation(field + " must be an absolute " + strings.Join(schemes, " or ") + " URL of at most 2000 characters")
	}
	return &v, nil
}

type statusPageFields struct {
	name, description, slug, locale, timezone          string
	logo, logoDark, color, site, support               *string
	history                                            int32
	uptime, responses, showName, showFollow, published bool
}

func (f *statusPageFields) check() error {
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
	if len(f.locale) > 35 || !languagePattern.MatchString(f.locale) {
		return errValidation("locale must be a language tag such as en or tr")
	}
	if err := validTimezone(f.timezone); err != nil {
		return err
	}
	if f.history < minStatusHistory || f.history > defaultStatusHistory {
		return errValidation("history_days must be 7 to 90")
	}
	if f.color != nil && !colorPattern.MatchString(*f.color) {
		return errValidation("accent_color must be #rrggbb")
	}
	if f.logo, err = statusLink(f.logo, "logo_url", "https"); err != nil {
		return err
	}
	if f.logoDark, err = statusLink(f.logoDark, "logo_dark_url", "https"); err != nil {
		return err
	}
	if f.site, err = statusLink(f.site, "site_url", "https", "http"); err != nil {
		return err
	}
	f.support, err = statusLink(f.support, "support_url", "https", "http", "mailto")
	return err
}

func (s *Server) ListStatusPages(ctx context.Context, req oas.ListStatusPagesRequestObject) (oas.ListStatusPagesResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListStatusPages(ctx, store.ListStatusPagesParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListStatusPages200JSONResponse{Items: make([]oas.StatusPage, len(rows))}
	for i, pg := range rows {
		if out.Items[i], err = s.statusPageOut(ctx, s.st.Queries, pg); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) CreateStatusPage(ctx context.Context, req oas.CreateStatusPageRequestObject) (oas.CreateStatusPageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	in, err := visibleInbox(ctx, s.st.Queries, p, b.InboxId)
	if err != nil {
		return nil, err
	}
	f := statusPageFields{
		name: b.Name, slug: string(b.Slug), locale: in.DefaultLocale, timezone: in.Timezone, history: defaultStatusHistory, uptime: true,
		showName: true, showFollow: true,
		logo: b.LogoUrl, logoDark: b.LogoDarkUrl, site: b.SiteUrl, support: b.SupportUrl,
	}
	if b.Description != nil {
		f.description = *b.Description
	}
	if b.AccentColor != nil {
		c := string(*b.AccentColor)
		f.color = &c
	}
	if b.Locale != nil {
		f.locale = string(*b.Locale)
	}
	if b.Timezone != nil {
		f.timezone = *b.Timezone
	}
	if b.HistoryDays != nil {
		f.history = *b.HistoryDays
	}
	if b.ShowUptime != nil {
		f.uptime = *b.ShowUptime
	}
	if b.ShowResponseTimes != nil {
		f.responses = *b.ShowResponseTimes
	}
	if b.ShowName != nil {
		f.showName = *b.ShowName
	}
	if b.ShowFollow != nil {
		f.showFollow = *b.ShowFollow
	}
	if b.Published != nil {
		f.published = *b.Published
	}
	if err := f.check(); err != nil {
		return nil, err
	}
	var pg store.StatusPage
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		if _, err := q.GetInboxStatusPage(ctx, store.GetInboxStatusPageParams{WorkspaceID: p.workspaceID, InboxID: in.ID}); err == nil {
			return errStatusPageExists
		} else if !store.IsNotFound(err) {
			return err
		}
		if taken, err := q.StatusSlugTaken(ctx, store.StatusSlugTakenParams{Slug: f.slug, ID: uuid.Nil()}); err != nil || taken {
			if err == nil {
				err = errStatusSlugTaken
			}
			return err
		}
		pg, err = q.CreateStatusPage(ctx, store.CreateStatusPageParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: in.ID, Name: f.name, Description: f.description, Slug: f.slug,
			LogoUrl: f.logo, LogoDarkUrl: f.logoDark, AccentColor: f.color, SiteUrl: f.site, SupportUrl: f.support, Locale: f.locale, Timezone: f.timezone,
			HistoryDays: f.history, ShowUptime: f.uptime, ShowResponseTimes: f.responses, ShowName: f.showName, ShowFollow: f.showFollow,
			Published: f.published, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, statusUniqueConflict(err)
	}
	if pg.Published {
		s.statusHint(ctx, p.workspaceID, pg.InboxID)
	}
	return oas.CreateStatusPage201JSONResponse(s.statusPageBody(pg, stOperational, noStatusEmail())), nil
}

func (s *Server) GetStatusPage(ctx context.Context, req oas.GetStatusPageRequestObject) (oas.GetStatusPageResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.statusPageOut(ctx, s.st.Queries, pg)
	if err != nil {
		return nil, err
	}
	return oas.GetStatusPage200JSONResponse(out), nil
}

func nullableLink(v interface {
	IsSpecified() bool
	IsNull() bool
	Get() (string, error)
}, cur *string) *string {
	if !v.IsSpecified() {
		return cur
	}
	if v.IsNull() {
		return nil
	}
	s, _ := v.Get()
	return &s
}

func (s *Server) UpdateStatusPage(ctx context.Context, req oas.UpdateStatusPageRequestObject) (oas.UpdateStatusPageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.StatusPage
	var hosts []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		cur, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		if cur.CustomDomain != nil {
			hosts = append(hosts, *cur.CustomDomain)
		}
		f := statusPageFields{
			name: cur.Name, description: cur.Description, slug: cur.Slug, locale: cur.Locale, timezone: cur.Timezone,
			logo: nullableLink(b.LogoUrl, cur.LogoUrl), color: nullableLink(b.AccentColor, cur.AccentColor),
			site: nullableLink(b.SiteUrl, cur.SiteUrl), support: nullableLink(b.SupportUrl, cur.SupportUrl),
			history: cur.HistoryDays, uptime: cur.ShowUptime, responses: cur.ShowResponseTimes, published: cur.Published,
			showName: cur.ShowName, showFollow: cur.ShowFollow, logoDark: nullableLink(b.LogoDarkUrl, cur.LogoDarkUrl),
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
		if b.Locale != nil {
			f.locale = string(*b.Locale)
		}
		if b.Timezone != nil {
			f.timezone = *b.Timezone
		}
		if b.HistoryDays != nil {
			f.history = *b.HistoryDays
		}
		if b.ShowUptime != nil {
			f.uptime = *b.ShowUptime
		}
		if b.ShowResponseTimes != nil {
			f.responses = *b.ShowResponseTimes
		}
		if b.ShowName != nil {
			f.showName = *b.ShowName
		}
		if b.ShowFollow != nil {
			f.showFollow = *b.ShowFollow
		}
		if b.Published != nil {
			f.published = *b.Published
		}
		if err := f.check(); err != nil {
			return err
		}
		if f.slug != cur.Slug {
			if taken, err := q.StatusSlugTaken(ctx, store.StatusSlugTakenParams{Slug: f.slug, ID: cur.ID}); err != nil || taken {
				if err == nil {
					err = errStatusSlugTaken
				}
				return err
			}
		}
		now := s.now()
		if f.slug != cur.Slug {
			if err := q.ReleaseStatusFormerSlug(ctx, store.ReleaseStatusFormerSlugParams{WorkspaceID: cur.WorkspaceID, PageID: cur.ID, Slug: f.slug}); err != nil {
				return err
			}
			if err := q.AddStatusFormerSlug(ctx, store.AddStatusFormerSlugParams{Slug: cur.Slug, WorkspaceID: cur.WorkspaceID, PageID: cur.ID, Now: now}); err != nil {
				return err
			}
		}
		pg, err := q.UpdateStatusPage(ctx, store.UpdateStatusPageParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Description: f.description, Slug: f.slug, LogoUrl: f.logo,
			LogoDarkUrl: f.logoDark, AccentColor: f.color, SiteUrl: f.site, SupportUrl: f.support, Locale: f.locale, Timezone: f.timezone,
			HistoryDays: f.history, ShowUptime: f.uptime, ShowResponseTimes: f.responses, ShowName: f.showName, ShowFollow: f.showFollow,
			Published: f.published, Now: now,
		})
		if err != nil {
			return err
		}
		if b.EmailSubscribers != nil {
			if pg, err = s.setStatusEmail(ctx, q, pg, *b.EmailSubscribers); err != nil {
				return err
			}
		}
		if b.CustomDomain.IsSpecified() {
			var raw *string
			if !b.CustomDomain.IsNull() {
				v := b.CustomDomain.MustGet()
				raw = &v
			}
			if pg, err = s.setStatusDomain(ctx, q, ev, pg, raw); err != nil {
				return err
			}
			if pg.CustomDomain != nil {
				hosts = append(hosts, *pg.CustomDomain)
			}
		}
		out, err = s.statusPageOut(ctx, q, pg)
		return err
	})
	if err != nil {
		return nil, statusUniqueConflict(err)
	}
	s.statusDomainCache.forget(req.PageId, hosts...)
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	s.statusPageSignal(ctx, p.workspaceID, out.InboxId, out.Id)
	return oas.UpdateStatusPage200JSONResponse(out), nil
}

func (s *Server) DeleteStatusPage(ctx context.Context, req oas.DeleteStatusPageRequestObject) (oas.DeleteStatusPageResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		cur, err := q.LockStatusPage(ctx, store.LockStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID})
		if err != nil {
			return err
		}
		releaseCloudflare(ev, cur)
		if _, err := q.DeleteStatusPage(ctx, store.DeleteStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID}); err != nil {
			return err
		}
		if pg.MailingListID != nil {
			_, err := q.DeleteMailingList(ctx, store.DeleteMailingListParams{WorkspaceID: p.workspaceID, ID: *pg.MailingListID})
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.statusCache.forget(pg.ID)
	s.statusDomainCache.forget(pg.ID)
	s.statusHint(ctx, p.workspaceID, pg.InboxID)
	return oas.DeleteStatusPage204Response{}, nil
}

func groupBody(g store.StatusComponentGroup) oas.StatusComponentGroup {
	return oas.StatusComponentGroup{Id: g.ID, PageId: g.PageID, Name: g.Name, Position: g.Position, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
}

func statusPosition(v *int32, cur int32) (int32, error) {
	if v == nil {
		return cur, nil
	}
	if *v < 0 || *v > 10000 {
		return 0, errValidation("position must be 0 to 10000")
	}
	return *v, nil
}

func (s *Server) ListStatusComponentGroups(ctx context.Context, req oas.ListStatusComponentGroupsRequestObject) (oas.ListStatusComponentGroupsResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListStatusGroups(ctx, store.ListStatusGroupsParams{WorkspaceID: p.workspaceID, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	out := oas.ListStatusComponentGroups200JSONResponse{Items: make([]oas.StatusComponentGroup, len(rows))}
	for i, g := range rows {
		out.Items[i] = groupBody(g)
	}
	return out, nil
}

func (s *Server) CreateStatusComponentGroup(ctx context.Context, req oas.CreateStatusComponentGroupRequestObject) (oas.CreateStatusComponentGroupResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	name, err := trimmed(req.Body.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	pos, err := statusPosition(req.Body.Position, 0)
	if err != nil {
		return nil, err
	}
	var g store.StatusComponentGroup
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		now := s.now()
		if g, err = q.CreateStatusGroup(ctx, store.CreateStatusGroupParams{ID: newID(), WorkspaceID: p.workspaceID, PageID: pg.ID, Name: name, Position: pos, Now: now}); err != nil {
			return err
		}
		return q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: now})
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateStatusComponentGroup201JSONResponse(groupBody(g)), nil
}

func (s *Server) UpdateStatusComponentGroup(ctx context.Context, req oas.UpdateStatusComponentGroupRequestObject) (oas.UpdateStatusComponentGroupResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var g store.StatusComponentGroup
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		cur, err := q.GetStatusGroup(ctx, store.GetStatusGroupParams{WorkspaceID: p.workspaceID, PageID: pg.ID, ID: req.GroupId})
		if store.IsNotFound(err) {
			return errStatusGroupGone
		}
		if err != nil {
			return err
		}
		name := cur.Name
		if req.Body.Name != nil {
			if name, err = trimmed(*req.Body.Name, 1, 100, "name"); err != nil {
				return err
			}
		}
		pos, err := statusPosition(req.Body.Position, cur.Position)
		if err != nil {
			return err
		}
		now := s.now()
		if g, err = q.UpdateStatusGroup(ctx, store.UpdateStatusGroupParams{WorkspaceID: p.workspaceID, PageID: pg.ID, ID: cur.ID, Name: name, Position: pos, Now: now}); err != nil {
			return err
		}
		return q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: now})
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateStatusComponentGroup200JSONResponse(groupBody(g)), nil
}

func (s *Server) DeleteStatusComponentGroup(ctx context.Context, req oas.DeleteStatusComponentGroupRequestObject) (oas.DeleteStatusComponentGroupResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		n, err := q.DeleteStatusGroup(ctx, store.DeleteStatusGroupParams{WorkspaceID: p.workspaceID, PageID: pg.ID, ID: req.GroupId})
		if err != nil {
			return err
		}
		if n == 0 {
			return errStatusGroupGone
		}
		return q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: s.now()})
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteStatusComponentGroup204Response{}, nil
}

func componentBodies(ctx context.Context, q *store.Queries, ws uuid.UUID, comps []store.StatusComponent) ([]oas.StatusComponent, error) {
	ids := make([]uuid.UUID, len(comps))
	for i, c := range comps {
		ids[i] = c.ID
	}
	open, err := q.OpenStatusComponentStates(ctx, store.OpenStatusComponentStatesParams{WorkspaceID: ws, ComponentIds: ids})
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]store.StatusComponentState{}
	for _, o := range open {
		byID[o.ComponentID] = o
	}
	out := make([]oas.StatusComponent, len(comps))
	for i, c := range comps {
		o, ok := byID[c.ID]
		if !ok {
			o = store.StatusComponentState{Source: statusSourceMember, StartedAt: c.CreatedAt}
		}
		out[i] = oas.StatusComponent{
			Id: c.ID, PageId: c.PageID, GroupId: c.GroupID, Name: c.Name, Description: c.Description, Position: c.Position, Hidden: c.Hidden,
			State: oas.StatusComponentStateValue(c.State), StateSource: oas.StatusStateSource(o.Source), StateSince: o.StartedAt,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		}
	}
	return out, nil
}

func componentBody(ctx context.Context, q *store.Queries, c store.StatusComponent) (oas.StatusComponent, error) {
	out, err := componentBodies(ctx, q, c.WorkspaceID, []store.StatusComponent{c})
	if err != nil {
		return oas.StatusComponent{}, err
	}
	return out[0], nil
}

func (s *Server) componentEvent(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, c store.StatusComponent, previous string) error {
	body, err := componentBody(ctx, q, c)
	if err != nil {
		return err
	}
	data := oas.StatusComponentEventData{Component: body, Page: statusPageRef(pg)}
	if previous != "" {
		st := oas.StatusComponentStateValue(previous)
		data.PreviousState = &st
	}
	inbox := pg.InboxID
	ev.add(statusEventComponent, &inbox, nil, data)
	return nil
}

func (s *Server) setComponentState(ctx context.Context, q *store.Queries, ev *eventBatch, pg store.StatusPage, c store.StatusComponent,
	state, source string, sourceID *uuid.UUID, now time.Time) (store.StatusComponent, error) {
	if c.State == state {
		err := q.HandOverStatusComponentState(ctx, store.HandOverStatusComponentStateParams{
			WorkspaceID: c.WorkspaceID, ComponentID: c.ID, Source: source, SourceID: sourceID,
		})
		return c, err
	}
	if err := q.EndStatusComponentState(ctx, store.EndStatusComponentStateParams{WorkspaceID: c.WorkspaceID, ComponentID: c.ID, Now: now}); err != nil {
		return c, err
	}
	if _, err := q.StartStatusComponentState(ctx, store.StartStatusComponentStateParams{
		ID: newID(), WorkspaceID: c.WorkspaceID, ComponentID: c.ID, State: state, Source: source, SourceID: sourceID, Now: now,
	}); err != nil {
		return c, err
	}
	previous := c.State
	c, err := q.SetStatusComponentState(ctx, store.SetStatusComponentStateParams{WorkspaceID: c.WorkspaceID, ID: c.ID, State: state, Now: now})
	if err != nil {
		return c, err
	}
	if err := q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: c.WorkspaceID, ID: pg.ID, Now: now}); err != nil {
		return c, err
	}
	return c, s.componentEvent(ctx, q, ev, pg, c, previous)
}

func (s *Server) ListStatusComponents(ctx context.Context, req oas.ListStatusComponentsRequestObject) (oas.ListStatusComponentsResponseObject, error) {
	p := principalFrom(ctx)
	pg, err := visibleStatusPage(ctx, s.st.Queries, p, req.PageId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListStatusComponents(ctx, store.ListStatusComponentsParams{WorkspaceID: p.workspaceID, PageID: pg.ID})
	if err != nil {
		return nil, err
	}
	items, err := componentBodies(ctx, s.st.Queries, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	return oas.ListStatusComponents200JSONResponse{Items: items}, nil
}

func statusGroupOf(ctx context.Context, q *store.Queries, pg store.StatusPage, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	_, err := q.GetStatusGroup(ctx, store.GetStatusGroupParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: *id})
	if store.IsNotFound(err) {
		return errValidation("group_id names no group of this page")
	}
	return err
}

func (s *Server) CreateStatusComponent(ctx context.Context, req oas.CreateStatusComponentRequestObject) (oas.CreateStatusComponentResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	name, err := trimmed(b.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	desc := ""
	if b.Description != nil {
		if desc, err = trimmed(*b.Description, 0, 500, "description"); err != nil {
			return nil, err
		}
	}
	pos, err := statusPosition(b.Position, 0)
	if err != nil {
		return nil, err
	}
	state := stOperational
	if b.State != nil {
		if state = string(*b.State); !validComponentState(state) {
			return nil, errValidation("state must be operational, degraded, partial_outage, major_outage or maintenance")
		}
	}
	var out oas.StatusComponent
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		if err := statusGroupOf(ctx, q, pg, b.GroupId); err != nil {
			return err
		}
		n, err := q.CountStatusComponents(ctx, store.CountStatusComponentsParams{WorkspaceID: p.workspaceID, PageID: pg.ID})
		if err != nil {
			return err
		}
		if n >= maxStatusComponents {
			return errValidation(statusComponentsLimit)
		}
		now := s.now()
		c, err := q.CreateStatusComponent(ctx, store.CreateStatusComponentParams{
			ID: newID(), WorkspaceID: p.workspaceID, PageID: pg.ID, GroupID: b.GroupId, Name: name, Description: desc, Position: pos,
			Hidden: b.Hidden != nil && *b.Hidden, State: state, Now: now,
		})
		if err != nil {
			return err
		}
		source, sourceID := statusSource(p)
		if _, err := q.StartStatusComponentState(ctx, store.StartStatusComponentStateParams{
			ID: newID(), WorkspaceID: p.workspaceID, ComponentID: c.ID, State: state, Source: source, SourceID: sourceID, Now: now,
		}); err != nil {
			return err
		}
		if err := q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: now}); err != nil {
			return err
		}
		if err := s.componentEvent(ctx, q, ev, pg, c, ""); err != nil {
			return err
		}
		out, err = componentBody(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateStatusComponent201JSONResponse(out), nil
}

func lockedStatusComponent(ctx context.Context, q *store.Queries, pg store.StatusPage, id uuid.UUID) (store.StatusComponent, error) {
	c, err := q.LockStatusComponent(ctx, store.LockStatusComponentParams{WorkspaceID: pg.WorkspaceID, PageID: pg.ID, ID: id})
	if store.IsNotFound(err) {
		return c, errStatusComponentGone
	}
	return c, err
}

func (s *Server) UpdateStatusComponent(ctx context.Context, req oas.UpdateStatusComponentRequestObject) (oas.UpdateStatusComponentResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.StatusComponent
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		c, err := lockedStatusComponent(ctx, q, pg, req.ComponentId)
		if err != nil {
			return err
		}
		arg := store.UpdateStatusComponentParams{WorkspaceID: p.workspaceID, ID: c.ID, GroupID: c.GroupID, Name: c.Name, Description: c.Description,
			Position: c.Position, Hidden: c.Hidden, Now: s.now()}
		if b.Name != nil {
			if arg.Name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
				return err
			}
		}
		if b.Description != nil {
			if arg.Description, err = trimmed(*b.Description, 0, 500, "description"); err != nil {
				return err
			}
		}
		if b.GroupId.IsSpecified() {
			arg.GroupID = nil
			if !b.GroupId.IsNull() {
				id := b.GroupId.MustGet()
				arg.GroupID = &id
			}
			if err := statusGroupOf(ctx, q, pg, arg.GroupID); err != nil {
				return err
			}
		}
		if arg.Position, err = statusPosition(b.Position, c.Position); err != nil {
			return err
		}
		if b.Hidden != nil {
			arg.Hidden = *b.Hidden
		}
		if c, err = q.UpdateStatusComponent(ctx, arg); err != nil {
			return err
		}
		if err := q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: arg.Now}); err != nil {
			return err
		}
		if err := s.componentEvent(ctx, q, ev, pg, c, ""); err != nil {
			return err
		}
		out, err = componentBody(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateStatusComponent200JSONResponse(out), nil
}

func (s *Server) DeleteStatusComponent(ctx context.Context, req oas.DeleteStatusComponentRequestObject) (oas.DeleteStatusComponentResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var inbox uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		inbox = pg.InboxID
		n, err := q.DeleteStatusComponent(ctx, store.DeleteStatusComponentParams{WorkspaceID: p.workspaceID, PageID: pg.ID, ID: req.ComponentId})
		if err != nil {
			return err
		}
		if n == 0 {
			return errStatusComponentGone
		}
		return q.TouchStatusPage(ctx, store.TouchStatusPageParams{WorkspaceID: p.workspaceID, ID: pg.ID, Now: s.now()})
	})
	if err != nil {
		return nil, err
	}
	s.statusHint(ctx, p.workspaceID, inbox)
	return oas.DeleteStatusComponent204Response{}, nil
}

func (s *Server) SetStatusComponentState(ctx context.Context, req oas.SetStatusComponentStateRequestObject) (oas.SetStatusComponentStateResponseObject, error) {
	p := principalFrom(ctx)
	state := string(req.Body.State)
	if !validComponentState(state) {
		return nil, errValidation("state must be operational, degraded, partial_outage, major_outage or maintenance")
	}
	var out oas.StatusComponent
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		pg, err := visibleStatusPage(ctx, q, p, req.PageId, true)
		if err != nil {
			return err
		}
		c, err := lockedStatusComponent(ctx, q, pg, req.ComponentId)
		if err != nil {
			return err
		}
		source, sourceID := statusSource(p)
		if c, err = s.setComponentState(ctx, q, ev, pg, c, state, source, sourceID, s.now()); err != nil {
			return err
		}
		out, err = componentBody(ctx, q, c)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.SetStatusComponentState200JSONResponse(out), nil
}

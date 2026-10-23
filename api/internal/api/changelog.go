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
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	changelogPathPrefix      = "/changelog/"
	changelogFormerSlugsKept = 10
	changelogFormerSlugDays  = 90
	changelogMaxScheduled    = 50
	changelogMaxAhead        = 365 * 24 * time.Hour
	maxChangelogBody         = 50000
	changelogExcerptRunes    = 300

	changelogStateDraft     = string(oas.ChangelogStateDraft)
	changelogStateScheduled = string(oas.ChangelogStateScheduled)
	changelogStatePublished = string(oas.ChangelogStatePublished)

	changelogEventPublished   = string(oas.ChangelogEntryPublishedEvent)
	changelogEventUnpublished = string(oas.ChangelogEntryUnpublishedEvent)
	changelogEventDeleted     = string(oas.ChangelogEntryDeletedEvent)
	changelogSignalUpdated    = string(oas.ChangelogEntryUpdated)
)

var (
	limitChangelogSlug   = limit{10, 24 * time.Hour}
	limitChangelogRender = limit{60, time.Minute}

	errWhatsNewOff            = problem(http.StatusNotFound, "whats_new_off", "What's new is off on this server")
	errWhatsNewNoChangelog    = problem(http.StatusNotFound, "whats_new_off", "this inbox has no published changelog")
	errChangelogGone          = problem(http.StatusNotFound, "not_found", "no such changelog")
	errChangelogEntryGone     = problem(http.StatusNotFound, "not_found", "no such entry")
	errChangelogTranslation   = problem(http.StatusNotFound, "not_found", "no such translation")
	errChangelogExists        = problem(http.StatusConflict, "changelog_exists", "this inbox already has a changelog")
	errChangelogSlugTaken     = problem(http.StatusConflict, "slug_taken", "another changelog has this slug")
	errChangelogLastTr        = problem(http.StatusConflict, "last_translation", "an entry keeps at least one translation; delete the entry instead")
	errChangelogDefaultTr     = problem(http.StatusConflict, "default_translation", "a published or scheduled entry keeps its translation in the default locale")
	errChangelogNoDefault     = problem(http.StatusConflict, "no_default_translation", "the entry needs a translation in the changelog's default locale")
	errChangelogNotPublished  = problem(http.StatusConflict, "not_published", "the entry is not published")
	errChangelogPublished     = problem(http.StatusConflict, "already_published", "the entry is published; unpublish it first")
	errChangelogTooManySched  = problem(http.StatusConflict, "too_many_scheduled", "a changelog holds at most 50 scheduled entries")
	errChangelogScheduleTime  = errValidation("publish_at must be in the future and at most 365 days ahead")
	errChangelogLabels        = errValidation("labels must hold 1 to 3 of new, improved and fixed, each once")
	errChangelogImageNotFound = errValidation("image_id must be an e-mail image of the workspace")
)

// ChangelogSettings come from YUVA_WHATS_NEW=off and YUVA_CHANGELOG_MAX_ENTRIES (0: no limit).
type ChangelogSettings struct {
	Off        bool
	MaxEntries int
}

func (s *Server) whatsNewOn() error {
	if s.changelog.Off {
		return errWhatsNewOff
	}
	return nil
}

func (s *Server) changelogURL(c store.Changelog, path string) string {
	if s.domainLive(c.CustomDomain, c.DomainState, c.CfStatus, c.CfSslStatus) {
		return s.domainURL(*c.CustomDomain, path)
	}
	return s.auth.PublicURL + changelogPathPrefix + c.Slug + path
}

func (s *Server) changelogEntryURL(c store.Changelog, number int32, locale, slug string) string {
	return s.changelogURL(c, "/"+locale+"/entries/"+strconv.Itoa(int(number))+"-"+slug)
}

func changelogUniqueConflict(err error) error {
	switch store.UniqueViolationConstraint(err) {
	case "changelogs_workspace_id_inbox_id_key":
		return errChangelogExists
	case "changelogs_slug_key", "changelog_slugs_pkey":
		return errChangelogSlugTaken
	}
	return err
}

func visibleChangelog(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.Changelog, error) {
	get := q.GetChangelog
	if lock {
		get = func(ctx context.Context, arg store.GetChangelogParams) (store.Changelog, error) {
			return q.LockChangelog(ctx, store.LockChangelogParams(arg))
		}
	}
	c, err := get(ctx, store.GetChangelogParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return c, errChangelogGone
	}
	if err != nil {
		return c, err
	}
	ok, err := canSeeInbox(ctx, q, p, c.InboxID)
	if err != nil {
		return c, err
	}
	if !ok {
		return c, errChangelogGone
	}
	return c, nil
}

// OAuth tokens, MCP included, never change what the public sees, whatever the changelog allows.
func mayPublishChangelog(p principal, c store.Changelog) error {
	if p.isGrant() {
		return errMemberSessionRequired
	}
	if p.isKey() {
		return requireScope(p, oas.ChangelogPublish)
	}
	if p.role == roleOwner || p.role == roleAdmin || c.AgentsMayPublish {
		return nil
	}
	return errForbidden
}

func requireChangelogManager(p principal) error {
	if p.isGrant() {
		return errMemberSessionRequired
	}
	return requireManagerOrKey(p)
}

func offersChangelogLocale(c store.Changelog, locale string) error {
	if err := validHelpLocale(locale, "locale"); err != nil {
		return err
	}
	if !slices.Contains(c.Locales, locale) {
		return problem(http.StatusBadRequest, "locale_not_offered", "the changelog does not offer the locale "+locale)
	}
	return nil
}

type changelogFields struct {
	name, description, slug, defaultLocale string
	locales                                []string
	property                               *uuid.UUID
	agentsPublish, published               bool
}

func (f *changelogFields) check(ctx context.Context, q *store.Queries, ws uuid.UUID) error {
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

func (s *Server) changelogOut(ctx context.Context, q *store.Queries, p principal, c store.Changelog) (oas.Changelog, error) {
	n, err := q.CountChangelog(ctx, store.CountChangelogParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID})
	if err != nil {
		return oas.Changelog{}, err
	}
	return oas.Changelog{
		Id: c.ID, InboxId: c.InboxID, Name: c.Name, Description: c.Description, Slug: oas.ChangelogSlug(c.Slug),
		Url: s.changelogURL(c, ""), DefaultLocale: c.DefaultLocale, Locales: c.Locales, AnalyticsPropertyId: c.AnalyticsPropertyID,
		AgentsMayPublish: c.AgentsMayPublish, Published: c.Published, CanPublish: mayPublishChangelog(p, c) == nil,
		Counts:    oas.ChangelogCounts{Entries: n.Entries, Published: n.Published, Scheduled: n.Scheduled},
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, CustomDomain: s.customDomainBody(changelogOwn(c)), OwnDomains: s.ownDomainsBody(),
	}, nil
}

func (s *Server) ListChangelogs(ctx context.Context, req oas.ListChangelogsRequestObject) (oas.ListChangelogsResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	rows, err := s.st.ListChangelogs(ctx, store.ListChangelogsParams{
		WorkspaceID: p.workspaceID, InboxID: req.Params.InboxId, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(),
	})
	if err != nil {
		return nil, err
	}
	out := oas.ListChangelogs200JSONResponse{Items: make([]oas.Changelog, len(rows))}
	for i, c := range rows {
		if out.Items[i], err = s.changelogOut(ctx, s.st.Queries, p, c); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Server) CreateChangelog(ctx context.Context, req oas.CreateChangelogRequestObject) (oas.CreateChangelogResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireChangelogManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.Changelog
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		in, err := visibleInbox(ctx, q, p, b.InboxId)
		if err != nil {
			return err
		}
		f := changelogFields{
			name: b.Name, slug: string(b.Slug), defaultLocale: in.DefaultLocale, property: b.AnalyticsPropertyId,
			agentsPublish: boolOr(b.AgentsMayPublish, false), published: boolOr(b.Published, false),
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
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		if _, err := q.GetInboxChangelog(ctx, store.GetInboxChangelogParams{WorkspaceID: p.workspaceID, InboxID: in.ID}); err == nil {
			return errChangelogExists
		} else if !store.IsNotFound(err) {
			return err
		}
		if taken, err := q.ChangelogSlugTaken(ctx, store.ChangelogSlugTakenParams{Slug: f.slug, ID: uuid.Nil()}); err != nil || taken {
			if err == nil {
				err = errChangelogSlugTaken
			}
			return err
		}
		c, err := q.CreateChangelog(ctx, store.CreateChangelogParams{
			ID: newID(), WorkspaceID: p.workspaceID, InboxID: in.ID, Name: f.name, Description: f.description, Slug: f.slug,
			DefaultLocale: f.defaultLocale, Locales: f.locales, AnalyticsPropertyID: f.property, AgentsMayPublish: f.agentsPublish,
			Published: f.published, Now: s.now(),
		})
		if err != nil {
			return err
		}
		out, err = s.changelogOut(ctx, q, p, c)
		return err
	})
	if err != nil {
		return nil, changelogUniqueConflict(err)
	}
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	return oas.CreateChangelog201JSONResponse(out), nil
}

func (s *Server) GetChangelog(ctx context.Context, req oas.GetChangelogRequestObject) (oas.GetChangelogResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.changelogOut(ctx, s.st.Queries, p, c)
	if err != nil {
		return nil, err
	}
	return oas.GetChangelog200JSONResponse(out), nil
}

func (s *Server) UpdateChangelog(ctx context.Context, req oas.UpdateChangelogRequestObject) (oas.UpdateChangelogResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireChangelogManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out oas.Changelog
	var hosts []string
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		cur, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		if cur.CustomDomain != nil {
			hosts = append(hosts, *cur.CustomDomain)
		}
		f := changelogFields{
			name: cur.Name, description: cur.Description, slug: cur.Slug, defaultLocale: cur.DefaultLocale, locales: cur.Locales,
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
		if err := f.check(ctx, q, p.workspaceID); err != nil {
			return err
		}
		used, err := q.ChangelogLocalesInUse(ctx, store.ChangelogLocalesInUseParams{WorkspaceID: p.workspaceID, ChangelogID: cur.ID, Locales: f.locales})
		if err != nil {
			return err
		}
		if len(used) > 0 {
			e := problem(http.StatusConflict, "locale_in_use", "entries have translations in "+strings.Join(used, ", "))
			e.Extra = map[string]any{"locales": used}
			return e
		}
		if f.defaultLocale != cur.DefaultLocale {
			missing, err := q.CountChangelogEntriesMissingLocale(ctx, store.CountChangelogEntriesMissingLocaleParams{
				WorkspaceID: p.workspaceID, ChangelogID: cur.ID, Locale: f.defaultLocale,
			})
			if err != nil {
				return err
			}
			if missing > 0 {
				return problem(http.StatusConflict, "default_locale_missing", "published or scheduled entries have no translation in "+f.defaultLocale)
			}
		}
		now := s.now()
		if f.slug != cur.Slug {
			if err := s.rateLimitTx(ctx, q, rateCheck{"changelog-slug:" + cur.ID.String(), limitChangelogSlug}); err != nil {
				return err
			}
			if taken, err := q.ChangelogSlugTaken(ctx, store.ChangelogSlugTakenParams{Slug: f.slug, ID: cur.ID}); err != nil || taken {
				if err == nil {
					err = errChangelogSlugTaken
				}
				return err
			}
			if err := q.ReleaseChangelogFormerSlug(ctx, store.ReleaseChangelogFormerSlugParams{WorkspaceID: cur.WorkspaceID, ChangelogID: cur.ID, Slug: f.slug}); err != nil {
				return err
			}
			if err := q.AddChangelogFormerSlug(ctx, store.AddChangelogFormerSlugParams{Slug: cur.Slug, WorkspaceID: cur.WorkspaceID, ChangelogID: cur.ID, Now: now}); err != nil {
				return err
			}
			if err := q.PruneChangelogFormerSlugs(ctx, store.PruneChangelogFormerSlugsParams{WorkspaceID: cur.WorkspaceID, ChangelogID: cur.ID, Keep: changelogFormerSlugsKept}); err != nil {
				return err
			}
		}
		c, err := q.UpdateChangelog(ctx, store.UpdateChangelogParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Description: f.description, Slug: f.slug, DefaultLocale: f.defaultLocale,
			Locales: f.locales, AnalyticsPropertyID: f.property, AgentsMayPublish: f.agentsPublish, Published: f.published, Now: now,
		})
		if err != nil {
			return err
		}
		if sent, raw := customDomainField(b.CustomDomain); sent {
			d, err := s.setOwnDomain(ctx, q, ev, changelogOwn(c), raw)
			if err != nil {
				return err
			}
			c = d.row.(store.Changelog)
			if c.CustomDomain != nil {
				hosts = append(hosts, *c.CustomDomain)
			}
		}
		out, err = s.changelogOut(ctx, q, p, c)
		return err
	})
	if err != nil {
		return nil, changelogUniqueConflict(err)
	}
	s.statusDomainCache.forget(req.ChangelogId, hosts...)
	s.statusHint(ctx, p.workspaceID, out.InboxId)
	s.changelogUpdatedSignal(ctx, p.workspaceID, out.InboxId, out.Id)
	return oas.UpdateChangelog200JSONResponse(out), nil
}

func (s *Server) DeleteChangelog(ctx context.Context, req oas.DeleteChangelogRequestObject) (oas.DeleteChangelogResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireChangelogManager(p); err != nil {
		return nil, err
	}
	var inbox uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		inbox = c.InboxID
		releaseOwnCloudflare(ev, changelogOwn(c))
		_, err = q.DeleteChangelog(ctx, store.DeleteChangelogParams{WorkspaceID: p.workspaceID, ID: c.ID})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.statusDomainCache.forget(req.ChangelogId)
	s.statusHint(ctx, p.workspaceID, inbox)
	return oas.DeleteChangelog204Response{}, nil
}

func (s *Server) touchChangelog(ctx context.Context, q *store.Queries, c store.Changelog) error {
	return q.TouchChangelog(ctx, store.TouchChangelogParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Now: s.now()})
}

func (s *Server) changelogSignal(ctx context.Context, p principal, c store.Changelog, entry uuid.UUID, t *store.ChangelogEntryTranslation) {
	data := map[string]any{"changelog_id": c.ID, "entry_id": entry}
	if t != nil {
		data["locale"], data["version"] = t.Locale, t.Version
	}
	if m := p.memberRef(); m != nil {
		data["member_id"] = *m
	}
	s.signal(ctx, realtime.Event{Type: changelogSignalUpdated, WorkspaceID: c.WorkspaceID, InboxID: &c.InboxID, Data: mustJSON(data)})
}

func (s *Server) CleanChangelog(ctx context.Context) error {
	ids, err := s.st.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if _, err := s.st.DeleteOldChangelogFormerSlugs(ctx, store.DeleteOldChangelogFormerSlugsParams{WorkspaceID: ws, Before: s.now().AddDate(0, 0, -changelogFormerSlugDays)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) RenderMarkdown(ctx context.Context, req oas.RenderMarkdownRequestObject) (oas.RenderMarkdownResponseObject, error) {
	p := principalFrom(ctx)
	if p.scoped() && requireScope(p, oas.HelpWrite) != nil && requireScope(p, oas.ChangelogWrite) != nil {
		return nil, errInsufficientScope(oas.ChangelogWrite)
	}
	if err := s.rateLimit(ctx, rateCheck{"markdown-render:" + p.viewerID().String(), limitChangelogRender}); err != nil {
		return nil, err
	}
	if len([]rune(req.Body.Body)) > maxHelpBody {
		return nil, errValidation("body must be at most 100000 characters")
	}
	images, err := s.helpImages(ctx, s.st.Queries, p.workspaceID, req.Body.Body)
	if err != nil {
		return nil, err
	}
	return oas.RenderMarkdown200JSONResponse{Html: renderHelp(req.Body.Body, images)}, nil
}

func (s *Server) UploadChangelogImage(ctx context.Context, req oas.UploadChangelogImageRequestObject) (oas.UploadChangelogImageResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"email-image-upload:" + p.workspaceID.String(), limitImageUpload}); err != nil {
		return nil, err
	}
	if _, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false); err != nil {
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
		return oas.UploadChangelogImage201JSONResponse(out), nil
	}
	return oas.UploadChangelogImage200JSONResponse(out), nil
}

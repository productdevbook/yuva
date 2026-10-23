package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

func changelogEntry(ctx context.Context, q *store.Queries, c store.Changelog, id uuid.UUID, lock bool) (store.ChangelogEntry, error) {
	get := q.GetChangelogEntry
	if lock {
		get = func(ctx context.Context, arg store.GetChangelogEntryParams) (store.ChangelogEntry, error) {
			return q.LockChangelogEntry(ctx, store.LockChangelogEntryParams(arg))
		}
	}
	e, err := get(ctx, store.GetChangelogEntryParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, ID: id})
	if store.IsNotFound(err) {
		return e, errChangelogEntryGone
	}
	return e, err
}

func lockedChangelogTranslation(ctx context.Context, q *store.Queries, e store.ChangelogEntry, locale string) (store.ChangelogEntryTranslation, error) {
	t, err := q.LockChangelogTranslation(ctx, store.LockChangelogTranslationParams{WorkspaceID: e.WorkspaceID, EntryID: e.ID, Locale: locale})
	if store.IsNotFound(err) {
		return t, errChangelogTranslation
	}
	return t, err
}

func changelogTranslationState(t store.ChangelogEntryTranslation) oas.ChangelogTranslationState {
	switch {
	case t.PublishedAt == nil:
		return oas.ChangelogTranslationDraft
	case t.HasUnpublishedChanges:
		return oas.ChangelogTranslationChanged
	}
	return oas.ChangelogTranslationPublished
}

func changelogOutdated(c store.Changelog, t store.ChangelogEntryTranslation, all []store.ChangelogEntryTranslation) bool {
	if t.Locale == c.DefaultLocale {
		return false
	}
	for _, d := range all {
		if d.EntryID == t.EntryID && d.Locale == c.DefaultLocale {
			return d.PublishedAt != nil && d.PublishedAt.After(t.UpdatedAt)
		}
	}
	return false
}

func (s *Server) changelogPublicURL(c store.Changelog, e store.ChangelogEntry, t store.ChangelogEntryTranslation) *string {
	if e.State != changelogStatePublished || t.PublishedAt == nil || t.PublishedSlug == nil {
		return nil
	}
	return new(s.changelogEntryURL(c, e.Number, t.Locale, *t.PublishedSlug))
}

func (s *Server) changelogTranslationBody(c store.Changelog, e store.ChangelogEntry, t store.ChangelogEntryTranslation, all []store.ChangelogEntryTranslation) oas.ChangelogEntryTranslation {
	out := oas.ChangelogEntryTranslation{
		EntryId: t.EntryID, Locale: t.Locale, Title: t.Title, Body: t.Body, Slug: t.Slug, Version: t.Version,
		State: changelogTranslationState(t), Outdated: changelogOutdated(c, t, all), Url: s.changelogPublicURL(c, e, t),
		UpdatedByMemberId: t.UpdatedByMemberID, UpdatedByApiKeyId: t.UpdatedByApiKeyID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if t.PublishedAt != nil {
		out.Published = &oas.ChangelogPublishedCopy{
			Title: deref(t.PublishedTitle), Body: deref(t.PublishedBody), Slug: deref(t.PublishedSlug), Html: deref(t.PublishedHtml),
			Excerpt: deref(t.PublishedExcerpt), PublishedAt: *t.PublishedAt,
			PublishedByMemberId: t.PublishedByMemberID, PublishedByApiKeyId: t.PublishedByApiKeyID,
		}
	}
	return out
}

func sortChangelogTranslations(c store.Changelog, ts []store.ChangelogEntryTranslation) {
	rank := func(l string) int {
		if i := slices.Index(c.Locales, l); i >= 0 {
			return i
		}
		return len(c.Locales)
	}
	slices.SortStableFunc(ts, func(a, b store.ChangelogEntryTranslation) int {
		if a.EntryID != b.EntryID {
			return strings.Compare(a.EntryID.String(), b.EntryID.String())
		}
		if d := rank(a.Locale) - rank(b.Locale); d != 0 {
			return d
		}
		return strings.Compare(a.Locale, b.Locale)
	})
}

func changelogSummaries(ctx context.Context, q *store.Queries, ws uuid.UUID, ids []uuid.UUID) ([]store.ChangelogEntryTranslation, error) {
	rows, err := q.ListChangelogTranslationSummaries(ctx, store.ListChangelogTranslationSummariesParams{WorkspaceID: ws, EntryIds: ids})
	if err != nil {
		return nil, err
	}
	out := make([]store.ChangelogEntryTranslation, len(rows))
	for i, r := range rows {
		out[i] = store.ChangelogEntryTranslation{
			WorkspaceID: ws, EntryID: r.EntryID, Locale: r.Locale, Title: r.Title, Slug: r.Slug, Version: r.Version,
			PublishedTitle: r.PublishedTitle, PublishedSlug: r.PublishedSlug, PublishedAt: r.PublishedAt,
			HasUnpublishedChanges: r.HasUnpublishedChanges, UpdatedAt: r.UpdatedAt,
		}
	}
	return out, nil
}

func changelogLabels(labels []string) []oas.ChangelogLabel {
	out := make([]oas.ChangelogLabel, len(labels))
	for i, l := range labels {
		out[i] = oas.ChangelogLabel(l)
	}
	return out
}

func checkChangelogLabels(in []oas.ChangelogLabel) ([]string, error) {
	if len(in) < 1 || len(in) > 3 {
		return nil, errChangelogLabels
	}
	out := make([]string, 0, len(in))
	for _, l := range in {
		if !l.Valid() || slices.Contains(out, string(l)) {
			return nil, errChangelogLabels
		}
		out = append(out, string(l))
	}
	return out, nil
}

func (s *Server) changelogImage(ctx context.Context, q *store.Queries, ws uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	if _, err := q.GetEmailImage(ctx, store.GetEmailImageParams{WorkspaceID: ws, ID: *id}); err != nil {
		if store.IsNotFound(err) {
			return errChangelogImageNotFound
		}
		return err
	}
	return nil
}

func scheduleFailure(e store.ChangelogEntry) *oas.ChangelogScheduleFailure {
	if e.ScheduleFailed == nil || e.ScheduleFailedAt == nil {
		return nil
	}
	return &oas.ChangelogScheduleFailure{Reason: oas.ChangelogScheduleFailureReason(*e.ScheduleFailed), At: *e.ScheduleFailedAt}
}

func (s *Server) changelogEntryHead(e store.ChangelogEntry, image *store.EmailImage) oas.ChangelogEntry {
	out := oas.ChangelogEntry{
		Id: e.ID, ChangelogId: e.ChangelogID, Number: e.Number, Labels: changelogLabels(e.Labels), State: oas.ChangelogEntryState(e.State),
		PublishAt: e.PublishAt, PublishedAt: e.PublishedAt, ScheduledByMemberId: e.ScheduledByMemberID, ScheduledByApiKeyId: e.ScheduledByApiKeyID,
		ScheduleFailed: scheduleFailure(e), CreatedByMemberId: e.CreatedByMemberID, CreatedByApiKeyId: e.CreatedByApiKeyID,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
	if image != nil {
		out.Image = new(s.emailImageBody(*image))
	}
	return out
}

func (s *Server) entryImage(ctx context.Context, q *store.Queries, e store.ChangelogEntry) (*store.EmailImage, error) {
	if e.ImageID == nil {
		return nil, nil
	}
	img, err := q.GetEmailImage(ctx, store.GetEmailImageParams{WorkspaceID: e.WorkspaceID, ID: *e.ImageID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &img, nil
}

func (s *Server) changelogEntryOut(ctx context.Context, q *store.Queries, c store.Changelog, e store.ChangelogEntry) (oas.ChangelogEntry, error) {
	ts, err := q.ListChangelogTranslations(ctx, store.ListChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryIds: []uuid.UUID{e.ID}})
	if err != nil {
		return oas.ChangelogEntry{}, err
	}
	sortChangelogTranslations(c, ts)
	img, err := s.entryImage(ctx, q, e)
	if err != nil {
		return oas.ChangelogEntry{}, err
	}
	out := s.changelogEntryHead(e, img)
	out.Translations = make([]oas.ChangelogEntryTranslation, len(ts))
	for i, t := range ts {
		out.Translations[i] = s.changelogTranslationBody(c, e, t, ts)
	}
	return out, nil
}

func (s *Server) changelogTranslationOut(ctx context.Context, q *store.Queries, c store.Changelog, e store.ChangelogEntry, t store.ChangelogEntryTranslation) (oas.ChangelogEntryTranslation, error) {
	all := []store.ChangelogEntryTranslation{t}
	if t.Locale != c.DefaultLocale {
		ts, err := changelogSummaries(ctx, q, c.WorkspaceID, []uuid.UUID{e.ID})
		if err != nil {
			return oas.ChangelogEntryTranslation{}, err
		}
		all = append(all, ts...)
	}
	return s.changelogTranslationBody(c, e, t, all), nil
}

func (s *Server) changelogConflict(ctx context.Context, q *store.Queries, c store.Changelog, e store.ChangelogEntry, cur *store.ChangelogEntryTranslation) error {
	err := problem(http.StatusConflict, "version_conflict", "the translation was saved by someone else since")
	if cur != nil {
		body, berr := s.changelogTranslationOut(ctx, q, c, e, *cur)
		if berr != nil {
			return berr
		}
		err.Detail += " version " + strconv.Itoa(int(cur.Version))
		err.Extra = map[string]any{"translation": body}
	}
	return err
}

var errChangelogScheduledLocked = problem(http.StatusForbidden, "entry_scheduled", "a scheduled entry goes public as it is when its time comes; only those who may publish change it")

func mayChangeScheduled(p principal, c store.Changelog, e store.ChangelogEntry) error {
	if e.State != changelogStateScheduled {
		return nil
	}
	if err := mayPublishChangelog(p, c); err != nil {
		if err == errMemberSessionRequired {
			return err
		}
		return errChangelogScheduledLocked
	}
	return nil
}

type changelogDraft struct{ title, body, slug string }

func changelogDraftFields(title string, body *string, slug *string) (changelogDraft, error) {
	var d changelogDraft
	var err error
	if d.title, err = trimmed(strings.Join(strings.Fields(title), " "), 3, 200, "title"); err != nil {
		return d, err
	}
	if body != nil {
		d.body = *body
	}
	if len([]rune(d.body)) > maxChangelogBody {
		return d, errValidation("body must be at most 50000 characters")
	}
	if slug != nil && *slug != "" {
		if err := validSlug(*slug, "slug"); err != nil {
			return d, err
		}
		d.slug = *slug
	} else {
		d.slug = helpSlug(d.title, "update")
	}
	return d, nil
}

func encodeNumberCursor(n int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(int(n))))
}

func decodeNumberCursor(v *string) (*int32, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*v)
	if err != nil {
		return nil, errInvalidCursor
	}
	n, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil || n < 1 {
		return nil, errInvalidCursor
	}
	return new(int32(n)), nil
}

func (s *Server) ListChangelogEntries(ctx context.Context, req oas.ListChangelogEntriesRequestObject) (oas.ListChangelogEntriesResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prm := req.Params
	lim, err := pageSize(prm.Limit)
	if err != nil {
		return nil, err
	}
	cursor, err := decodeNumberCursor(prm.Cursor)
	if err != nil {
		return nil, err
	}
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	var state, label, locale *string
	if prm.State != nil {
		if !slices.Contains([]string{"draft", "scheduled", "published", "changed"}, string(*prm.State)) {
			return nil, errValidation("state must be draft, scheduled, published or changed")
		}
		state = new(string(*prm.State))
	}
	if prm.Label != nil {
		if !prm.Label.Valid() {
			return nil, errValidation("label must be new, improved or fixed")
		}
		label = new(string(*prm.Label))
	}
	if prm.Locale != nil {
		if err := validHelpLocale(*prm.Locale, "locale"); err != nil {
			return nil, err
		}
		locale = prm.Locale
	}
	rows, err := s.st.ListChangelogEntries(ctx, store.ListChangelogEntriesParams{
		WorkspaceID: p.workspaceID, ChangelogID: c.ID, State: state, Label: label, Locale: locale, CursorNumber: cursor, Lim: lim + 1,
	})
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > int(lim) {
		rows = rows[:lim]
		next = new(encodeNumberCursor(rows[len(rows)-1].Number))
	}
	ids := make([]uuid.UUID, len(rows))
	var imageIDs []uuid.UUID
	for i, e := range rows {
		ids[i] = e.ID
		if e.ImageID != nil {
			imageIDs = append(imageIDs, *e.ImageID)
		}
	}
	ts, err := changelogSummaries(ctx, s.st.Queries, p.workspaceID, ids)
	if err != nil {
		return nil, err
	}
	sortChangelogTranslations(c, ts)
	images := map[uuid.UUID]store.EmailImage{}
	if len(imageIDs) > 0 {
		imgs, err := s.st.ListEmailImagesByIDs(ctx, store.ListEmailImagesByIDsParams{WorkspaceID: p.workspaceID, Ids: imageIDs})
		if err != nil {
			return nil, err
		}
		for _, img := range imgs {
			images[img.ID] = img
		}
	}
	out := oas.ListChangelogEntries200JSONResponse{Items: make([]oas.ChangelogEntrySummary, len(rows)), NextCursor: next}
	for i, e := range rows {
		var img *store.EmailImage
		if e.ImageID != nil {
			if v, ok := images[*e.ImageID]; ok {
				img = &v
			}
		}
		head := s.changelogEntryHead(e, img)
		item := oas.ChangelogEntrySummary{
			Id: head.Id, ChangelogId: head.ChangelogId, Number: head.Number, Labels: head.Labels, Image: head.Image, State: head.State,
			PublishAt: head.PublishAt, PublishedAt: head.PublishedAt, ScheduledByMemberId: head.ScheduledByMemberId,
			ScheduledByApiKeyId: head.ScheduledByApiKeyId, ScheduleFailed: head.ScheduleFailed, CreatedByMemberId: head.CreatedByMemberId,
			CreatedByApiKeyId: head.CreatedByApiKeyId, CreatedAt: head.CreatedAt, UpdatedAt: head.UpdatedAt,
			Translations: []oas.ChangelogEntryTranslationSummary{},
		}
		for _, t := range ts {
			if t.EntryID != e.ID {
				continue
			}
			item.Translations = append(item.Translations, oas.ChangelogEntryTranslationSummary{
				Locale: t.Locale, Title: t.Title, Slug: t.Slug, Version: t.Version, State: changelogTranslationState(t),
				Outdated: changelogOutdated(c, t, ts), PublishedAt: t.PublishedAt, Url: s.changelogPublicURL(c, e, t), UpdatedAt: t.UpdatedAt,
			})
		}
		out.Items[i] = item
	}
	return out, nil
}

func (s *Server) CreateChangelogEntry(ctx context.Context, req oas.CreateChangelogEntryRequestObject) (oas.CreateChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	d, err := changelogDraftFields(b.Title, b.Body, (*string)(b.Slug))
	if err != nil {
		return nil, err
	}
	labels, err := checkChangelogLabels(b.Labels)
	if err != nil {
		return nil, err
	}
	var out oas.ChangelogEntry
	var center store.Changelog
	var created store.ChangelogEntry
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := offersChangelogLocale(c, b.Locale); err != nil {
			return err
		}
		if err := s.changelogImage(ctx, q, p.workspaceID, b.ImageId); err != nil {
			return err
		}
		if max := s.changelog.MaxEntries; max > 0 {
			if err := q.LockChangelogEntryQuota(ctx, p.workspaceID); err != nil {
				return err
			}
			n, err := q.CountWorkspaceChangelogEntries(ctx, p.workspaceID)
			if err != nil {
				return err
			}
			if int(n) >= max {
				return errHostedLimit(oas.LimitChangelogMaxEntries, "this workspace has as many What's new entries as the server allows ("+strconv.Itoa(max)+", YUVA_CHANGELOG_MAX_ENTRIES)")
			}
		}
		number, err := q.TakeChangelogEntryNumber(ctx, store.TakeChangelogEntryNumberParams{WorkspaceID: c.WorkspaceID, ID: c.ID})
		if err != nil {
			return err
		}
		images, err := s.helpImages(ctx, q, p.workspaceID, d.body)
		if err != nil {
			return err
		}
		member, key := helpCreatedBy(p)
		now := s.now()
		e, err := q.CreateChangelogEntry(ctx, store.CreateChangelogEntryParams{
			ID: newID(), WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, Number: number, Labels: labels, ImageID: b.ImageId,
			MemberID: member, ApiKeyID: key, Now: now,
		})
		if err != nil {
			return err
		}
		created = e
		if _, err := q.CreateChangelogTranslation(ctx, store.CreateChangelogTranslationParams{
			WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: b.Locale, Title: d.title, Body: d.body, Slug: d.slug,
			ImageIds: images.ids(), MemberID: member, ApiKeyID: key, Now: now,
		}); err != nil {
			return err
		}
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, created.ID, nil)
	return oas.CreateChangelogEntry201JSONResponse(out), nil
}

func (s *Server) GetChangelogEntry(ctx context.Context, req oas.GetChangelogEntryRequestObject) (oas.GetChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	c, err := visibleChangelog(ctx, s.st.Queries, p, req.ChangelogId, false)
	if err != nil {
		return nil, err
	}
	e, err := changelogEntry(ctx, s.st.Queries, c, req.EntryId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.changelogEntryOut(ctx, s.st.Queries, c, e)
	if err != nil {
		return nil, err
	}
	return oas.GetChangelogEntry200JSONResponse(out), nil
}

func (s *Server) UpdateChangelogEntry(ctx context.Context, req oas.UpdateChangelogEntryRequestObject) (oas.UpdateChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	var out oas.ChangelogEntry
	var center store.Changelog
	var changed bool
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		cur, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		labels := cur.Labels
		if b.Labels != nil {
			if labels, err = checkChangelogLabels(*b.Labels); err != nil {
				return err
			}
		}
		image := cur.ImageID
		if b.ImageId.IsSpecified() {
			image = nullableID(b.ImageId, cur.ImageID)
			if err := s.changelogImage(ctx, q, p.workspaceID, image); err != nil {
				return err
			}
		}
		if cur.State == changelogStatePublished {
			if err := mayPublishChangelog(p, c); err != nil {
				return err
			}
		}
		if err := mayChangeScheduled(p, c, cur); err != nil {
			return err
		}
		e, err := q.UpdateChangelogEntry(ctx, store.UpdateChangelogEntryParams{WorkspaceID: c.WorkspaceID, ID: cur.ID, Labels: labels, ImageID: image, Now: s.now()})
		if err != nil {
			return err
		}
		if e.State == changelogStatePublished {
			changed = true
			if err := s.touchChangelog(ctx, q, c); err != nil {
				return err
			}
		}
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	if changed {
		s.statusHint(ctx, p.workspaceID, center.InboxID)
	}
	return oas.UpdateChangelogEntry200JSONResponse(out), nil
}

func (s *Server) DeleteChangelogEntry(ctx context.Context, req oas.DeleteChangelogEntryRequestObject) (oas.DeleteChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var center store.Changelog
	var wasPublished bool
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if e.State != changelogStateDraft {
			if err := mayPublishChangelog(p, c); err != nil {
				return err
			}
		}
		ts, err := q.ListChangelogTranslations(ctx, store.ListChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryIds: []uuid.UUID{e.ID}})
		if err != nil {
			return err
		}
		if _, err := q.DeleteChangelogEntry(ctx, store.DeleteChangelogEntryParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, ID: e.ID}); err != nil {
			return err
		}
		if e.State == changelogStatePublished {
			wasPublished = true
			if err := s.touchChangelog(ctx, q, c); err != nil {
				return err
			}
			ev.add(changelogEventDeleted, &c.InboxID, nil, s.changelogEventData(c, e, ts))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	if wasPublished {
		s.statusHint(ctx, p.workspaceID, center.InboxID)
	}
	return oas.DeleteChangelogEntry204Response{}, nil
}

func (s *Server) saveChangelogDraft(ctx context.Context, q *store.Queries, p principal, t store.ChangelogEntryTranslation, d changelogDraft) (store.ChangelogEntryTranslation, error) {
	images, err := s.helpImages(ctx, q, t.WorkspaceID, d.body, deref(t.PublishedBody))
	if err != nil {
		return t, err
	}
	member, key := helpCreatedBy(p)
	return q.SaveChangelogTranslation(ctx, store.SaveChangelogTranslationParams{
		WorkspaceID: t.WorkspaceID, EntryID: t.EntryID, Locale: t.Locale, Title: d.title, Body: d.body, Slug: d.slug,
		ImageIds: images.ids(), MemberID: member, ApiKeyID: key, Now: s.now(),
	})
}

func (s *Server) PutChangelogEntryTranslation(ctx context.Context, req oas.PutChangelogEntryTranslationRequestObject) (oas.PutChangelogEntryTranslationResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	d, err := changelogDraftFields(b.Title, b.Body, (*string)(b.Slug))
	if err != nil {
		return nil, err
	}
	var out oas.ChangelogEntryTranslation
	var saved store.ChangelogEntryTranslation
	var center store.Changelog
	created := false
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := offersChangelogLocale(c, req.Locale); err != nil {
			return err
		}
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if err := mayChangeScheduled(p, c, e); err != nil {
			return err
		}
		cur, err := lockedChangelogTranslation(ctx, q, e, req.Locale)
		missing := err == errChangelogTranslation
		if err != nil && !missing {
			return err
		}
		switch {
		case missing && b.BaseVersion != 0:
			return s.changelogConflict(ctx, q, c, e, nil)
		case !missing && b.BaseVersion != cur.Version:
			return s.changelogConflict(ctx, q, c, e, &cur)
		}
		if missing {
			images, err := s.helpImages(ctx, q, p.workspaceID, d.body)
			if err != nil {
				return err
			}
			member, key := helpCreatedBy(p)
			saved, err = q.CreateChangelogTranslation(ctx, store.CreateChangelogTranslationParams{
				WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: req.Locale, Title: d.title, Body: d.body, Slug: d.slug,
				ImageIds: images.ids(), MemberID: member, ApiKeyID: key, Now: s.now(),
			})
			if err != nil {
				return err
			}
			created = true
		} else if saved, err = s.saveChangelogDraft(ctx, q, p, cur, d); err != nil {
			return err
		}
		if err := q.TouchChangelogEntry(ctx, store.TouchChangelogEntryParams{WorkspaceID: c.WorkspaceID, ID: e.ID, Now: s.now()}); err != nil {
			return err
		}
		out, err = s.changelogTranslationOut(ctx, q, c, e, saved)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, &saved)
	if created {
		return oas.PutChangelogEntryTranslation201JSONResponse(out), nil
	}
	return oas.PutChangelogEntryTranslation200JSONResponse(out), nil
}

func (s *Server) DeleteChangelogEntryTranslation(ctx context.Context, req oas.DeleteChangelogEntryTranslationRequestObject) (oas.DeleteChangelogEntryTranslationResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var center store.Changelog
	var wasPublic bool
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if err := mayChangeScheduled(p, c, e); err != nil {
			return err
		}
		t, err := lockedChangelogTranslation(ctx, q, e, req.Locale)
		if err != nil {
			return err
		}
		if t.PublishedAt != nil {
			if err := mayPublishChangelog(p, c); err != nil {
				return err
			}
		}
		n, err := q.CountChangelogTranslations(ctx, store.CountChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID})
		if err != nil {
			return err
		}
		if n <= 1 {
			return errChangelogLastTr
		}
		if t.Locale == c.DefaultLocale && e.State != changelogStateDraft {
			return errChangelogDefaultTr
		}
		if _, err := q.DeleteChangelogTranslation(ctx, store.DeleteChangelogTranslationParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: t.Locale}); err != nil {
			return err
		}
		if t.PublishedAt != nil {
			wasPublic = true
			return s.touchChangelog(ctx, q, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	if wasPublic {
		s.statusHint(ctx, p.workspaceID, center.InboxID)
	}
	return oas.DeleteChangelogEntryTranslation204Response{}, nil
}

func (s *Server) changelogEventData(c store.Changelog, e store.ChangelogEntry, ts []store.ChangelogEntryTranslation) oas.ChangelogEntryEventData {
	ref := oas.ChangelogEntryRef{
		Id: e.ID, Number: e.Number, Labels: changelogLabels(e.Labels), PublishedAt: e.PublishedAt,
		Translations: []oas.ChangelogEntryRefTranslation{},
	}
	sorted := slices.Clone(ts)
	sortChangelogTranslations(c, sorted)
	for _, t := range sorted {
		if t.PublishedAt == nil {
			continue
		}
		tr := oas.ChangelogEntryRefTranslation{Locale: t.Locale, Title: deref(t.PublishedTitle)}
		if c.Published && t.PublishedSlug != nil {
			tr.Url = new(s.changelogEntryURL(c, e.Number, t.Locale, *t.PublishedSlug))
		}
		ref.Translations = append(ref.Translations, tr)
	}
	return oas.ChangelogEntryEventData{
		Changelog: oas.ChangelogRef{Id: c.ID, Name: c.Name, Url: s.changelogURL(c, "")},
		Entry:     ref,
	}
}

var spaceBeforePunctuation = regexp.MustCompile(`\s+([.,;:!?])`)

// htmlExcerpt leaves a space where an inline tag ended; a sentence does not read with one before its full stop.
func changelogExcerpt(renderedHTML string) string {
	return spaceBeforePunctuation.ReplaceAllString(htmlExcerpt(renderedHTML), "$1")
}

type changelogActor struct{ member, key *uuid.UUID }

// freezeChangelogEntry freezes the drafts of a published entry (those of one locale when given) as their
// published copies, moves the changelog's cache key and stores the event.
func (s *Server) freezeChangelogEntry(ctx context.Context, q *store.Queries, ev *eventBatch, c store.Changelog, e store.ChangelogEntry, actor changelogActor, only string) error {
	ts, err := q.ListChangelogTranslations(ctx, store.ListChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryIds: []uuid.UUID{e.ID}})
	if err != nil {
		return err
	}
	now := s.now()
	for i, t := range ts {
		if (only != "" && t.Locale != only) || (t.PublishedAt != nil && !t.HasUnpublishedChanges) {
			continue
		}
		images, err := s.helpImages(ctx, q, c.WorkspaceID, t.Body)
		if err != nil {
			return err
		}
		html := renderHelp(t.Body, images)
		if ts[i], err = q.PublishChangelogTranslation(ctx, store.PublishChangelogTranslationParams{
			WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: t.Locale, Html: &html, Excerpt: new(changelogExcerpt(html)),
			Now: new(now), MemberID: actor.member, ApiKeyID: actor.key,
		}); err != nil {
			return err
		}
	}
	if err := s.touchChangelog(ctx, q, c); err != nil {
		return err
	}
	ev.add(changelogEventPublished, &c.InboxID, nil, s.changelogEventData(c, e, ts))
	return nil
}

func (s *Server) hasDefaultChangelogTranslation(ctx context.Context, q *store.Queries, c store.Changelog, e store.ChangelogEntry) (bool, error) {
	_, err := q.GetChangelogTranslation(ctx, store.GetChangelogTranslationParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID, Locale: c.DefaultLocale})
	if store.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (s *Server) PublishChangelogEntry(ctx context.Context, req oas.PublishChangelogEntryRequestObject) (oas.PublishChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var locale string
	var base *int32
	if req.Body != nil {
		if req.Body.Locale != nil {
			locale = *req.Body.Locale
			if err := validHelpLocale(locale, "locale"); err != nil {
				return nil, err
			}
		}
		base = req.Body.BaseVersion
	}
	var out oas.ChangelogEntry
	var center store.Changelog
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := mayPublishChangelog(p, c); err != nil {
			return err
		}
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		member, key := helpCreatedBy(p)
		actor := changelogActor{member: member, key: key}
		if locale != "" {
			if e.State != changelogStatePublished {
				return errChangelogNotPublished
			}
			t, err := lockedChangelogTranslation(ctx, q, e, locale)
			if err != nil {
				return err
			}
			if base != nil && *base != t.Version {
				return s.changelogConflict(ctx, q, c, e, &t)
			}
			if t.PublishedAt == nil || t.HasUnpublishedChanges {
				if err := s.freezeChangelogEntry(ctx, q, ev, c, e, actor, locale); err != nil {
					return err
				}
			}
		} else {
			ok, err := s.hasDefaultChangelogTranslation(ctx, q, c, e)
			if err != nil {
				return err
			}
			if !ok {
				return errChangelogNoDefault
			}
			if e, err = q.PublishChangelogEntry(ctx, store.PublishChangelogEntryParams{WorkspaceID: c.WorkspaceID, ID: e.ID, Now: s.now()}); err != nil {
				return err
			}
			if err := s.freezeChangelogEntry(ctx, q, ev, c, e, actor, ""); err != nil {
				return err
			}
		}
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	s.statusHint(ctx, p.workspaceID, center.InboxID)
	return oas.PublishChangelogEntry200JSONResponse(out), nil
}

func (s *Server) ScheduleChangelogEntry(ctx context.Context, req oas.ScheduleChangelogEntryRequestObject) (oas.ScheduleChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	at := req.Body.PublishAt.UTC()
	if now := s.now(); !at.After(now) || at.After(now.Add(changelogMaxAhead)) {
		return nil, errChangelogScheduleTime
	}
	var out oas.ChangelogEntry
	var center store.Changelog
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := mayPublishChangelog(p, c); err != nil {
			return err
		}
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if e.State == changelogStatePublished {
			return errChangelogPublished
		}
		ok, err := s.hasDefaultChangelogTranslation(ctx, q, c, e)
		if err != nil {
			return err
		}
		if !ok {
			return errChangelogNoDefault
		}
		n, err := q.CountScheduledChangelogEntries(ctx, store.CountScheduledChangelogEntriesParams{WorkspaceID: c.WorkspaceID, ChangelogID: c.ID, ExceptID: e.ID})
		if err != nil {
			return err
		}
		if n >= changelogMaxScheduled {
			return errChangelogTooManySched
		}
		member, key := helpCreatedBy(p)
		if e, err = q.ScheduleChangelogEntry(ctx, store.ScheduleChangelogEntryParams{
			WorkspaceID: c.WorkspaceID, ID: e.ID, PublishAt: &at, MemberID: member, ApiKeyID: key, Now: s.now(),
		}); err != nil {
			return err
		}
		ev.job(ChangelogPublishArgs{WorkspaceID: c.WorkspaceID, EntryID: e.ID}, changelogJobOpts(at))
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	return oas.ScheduleChangelogEntry200JSONResponse(out), nil
}

func (s *Server) UnscheduleChangelogEntry(ctx context.Context, req oas.UnscheduleChangelogEntryRequestObject) (oas.UnscheduleChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out oas.ChangelogEntry
	var center store.Changelog
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, _ *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := mayPublishChangelog(p, c); err != nil {
			return err
		}
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if e.State == changelogStateScheduled {
			if e, err = q.UnscheduleChangelogEntry(ctx, store.UnscheduleChangelogEntryParams{WorkspaceID: c.WorkspaceID, ID: e.ID, Now: s.now()}); err != nil {
				return err
			}
		}
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	return oas.UnscheduleChangelogEntry200JSONResponse(out), nil
}

func (s *Server) UnpublishChangelogEntry(ctx context.Context, req oas.UnpublishChangelogEntryRequestObject) (oas.UnpublishChangelogEntryResponseObject, error) {
	if err := s.whatsNewOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out oas.ChangelogEntry
	var center store.Changelog
	var changed bool
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, ev *eventBatch) error {
		c, err := visibleChangelog(ctx, q, p, req.ChangelogId, true)
		if err != nil {
			return err
		}
		center = c
		if err := mayPublishChangelog(p, c); err != nil {
			return err
		}
		e, err := changelogEntry(ctx, q, c, req.EntryId, true)
		if err != nil {
			return err
		}
		if e.State == changelogStatePublished {
			changed = true
			ts, err := q.ListChangelogTranslations(ctx, store.ListChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryIds: []uuid.UUID{e.ID}})
			if err != nil {
				return err
			}
			if err := q.UnpublishChangelogTranslations(ctx, store.UnpublishChangelogTranslationsParams{WorkspaceID: c.WorkspaceID, EntryID: e.ID}); err != nil {
				return err
			}
			if e, err = q.UnpublishChangelogEntry(ctx, store.UnpublishChangelogEntryParams{WorkspaceID: c.WorkspaceID, ID: e.ID, Now: s.now()}); err != nil {
				return err
			}
			if err := s.touchChangelog(ctx, q, c); err != nil {
				return err
			}
			ev.add(changelogEventUnpublished, &c.InboxID, nil, s.changelogEventData(c, e, ts))
		}
		out, err = s.changelogEntryOut(ctx, q, c, e)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.changelogSignal(ctx, p, center, req.EntryId, nil)
	if changed {
		s.statusHint(ctx, p.workspaceID, center.InboxID)
	}
	return oas.UnpublishChangelogEntry200JSONResponse(out), nil
}

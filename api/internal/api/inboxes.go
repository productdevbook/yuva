package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

var (
	slugPattern     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	languagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	colorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	clockPattern    = regexp.MustCompile(`^(([01][0-9]|2[0-3]):[0-5][0-9]|24:00)$`)
	errSlugTaken    = problem(http.StatusConflict, "slug_taken", "another inbox uses this slug")
	weekdays        = map[oas.Weekday]bool{oas.Mon: true, oas.Tue: true, oas.Wed: true, oas.Thu: true, oas.Fri: true, oas.Sat: true, oas.Sun: true}
)

const identitySecretPrefix = "yuva_is_"

func inboxBody(in store.Inbox) oas.Inbox {
	out := oas.Inbox{
		Id: in.ID, Name: in.Name, Slug: in.Slug, DefaultLocale: in.DefaultLocale, Timezone: in.Timezone,
		Mode: oas.InboxMode(in.Mode), ExpectedReplyMinutes: in.ExpectedReplyMinutes, AskForRating: in.AskForRating,
		AssignmentMode: oas.AssignmentMode(in.AssignmentMode), AssignmentCap: in.AssignmentCap,
		Sender: inboxSenderBody(in), CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt,
	}
	out.FeedbackCategories = inboxCategories(in)
	_ = json.Unmarshal(in.Branding, &out.Branding)
	_ = json.Unmarshal(in.BusinessHours, &out.BusinessHours)
	if out.BusinessHours.Intervals == nil {
		out.BusinessHours.Intervals = []oas.BusinessHoursInterval{}
	}
	return out
}

func inboxImportPercent(state string, c oas.MigrationCounts) int32 {
	switch {
	case state == migrationSyncing:
		return 100
	case state == migrationScanning || c.Total <= 0:
		return 0
	}
	return int32(min(100, done(c)*100/c.Total))
}

func withImports(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, items []oas.Inbox) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(items))
	for i, in := range items {
		ids[i] = in.Id
	}
	rows, err := q.ListInboxImports(ctx, store.ListInboxImportsParams{WorkspaceID: workspaceID, InboxIds: ids})
	if err != nil {
		return err
	}
	for _, r := range rows {
		for i := range items {
			if items[i].Id == r.InboxID {
				items[i].Import = &oas.InboxImport{
					Address: r.SourceAddress, State: oas.InboxImportState(r.State),
					Percent: inboxImportPercent(r.State, migrationCounts(r.Counts)),
				}
			}
		}
	}
	return nil
}

func memberInbox(ctx context.Context, q *store.Queries, in store.Inbox) (oas.Inbox, error) {
	out := []oas.Inbox{inboxBody(in)}
	err := withImports(ctx, q, in.WorkspaceID, out)
	return out[0], err
}

func validSlug(s string, field string) error {
	if len(s) > 64 || !slugPattern.MatchString(s) {
		return errValidation(field + " must be lowercase letters, digits and single dashes, at most 64 characters")
	}
	return nil
}

func validBranding(b oas.InboxBranding) error {
	if b.Color != nil && !colorPattern.MatchString(*b.Color) {
		return errValidation("branding.color must be #rrggbb")
	}
	if b.LogoUrl != nil {
		u, err := url.Parse(*b.LogoUrl)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(*b.LogoUrl) > 2000 {
			return errValidation("branding.logo_url must be an absolute http(s) URL")
		}
	}
	if b.Greeting != nil && len([]rune(*b.Greeting)) > 500 {
		return errValidation("branding.greeting must be at most 500 characters")
	}
	return nil
}

func validBusinessHours(h oas.BusinessHours) error {
	if len(h.Intervals) > 50 {
		return errValidation("business_hours has more than 50 intervals")
	}
	for _, iv := range h.Intervals {
		if !weekdays[iv.Day] {
			return errValidation("business_hours day must be mon to sun")
		}
		if !clockPattern.MatchString(iv.Start) || iv.Start == "24:00" || !clockPattern.MatchString(iv.End) || iv.End <= iv.Start {
			return errValidation("business_hours intervals need HH:MM start and a later end")
		}
	}
	return nil
}

func validTimezone(tz string) error {
	if tz == "" || tz == "Local" {
		return errValidation("timezone must be an IANA time zone name")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return errValidation("timezone must be an IANA time zone name")
	}
	return nil
}

type inboxFields struct {
	name, slug, locale, timezone, mode string
	branding                           oas.InboxBranding
	hours                              oas.BusinessHours
	replyMinutes                       *int32
	assignment                         assignmentSettings
}

func (f *inboxFields) validate() error {
	var err error
	if f.name, err = trimmed(f.name, 1, 200, "name"); err != nil {
		return err
	}
	if err := validSlug(f.slug, "slug"); err != nil {
		return err
	}
	if len(f.locale) > 35 || !languagePattern.MatchString(f.locale) {
		return errValidation("default_locale must be a language tag such as en or tr")
	}
	if err := validTimezone(f.timezone); err != nil {
		return err
	}
	if f.mode != string(oas.Live) && f.mode != string(oas.Async) {
		return errValidation("mode must be live or async")
	}
	if f.replyMinutes != nil && (*f.replyMinutes < 1 || *f.replyMinutes > 43200) {
		return errValidation("expected_reply_minutes must be 1 to 43200")
	}
	if err := validBranding(f.branding); err != nil {
		return err
	}
	if err := f.assignment.validate(); err != nil {
		return err
	}
	if f.hours.Intervals == nil {
		f.hours.Intervals = []oas.BusinessHoursInterval{}
	}
	return validBusinessHours(f.hours)
}

func (s *Server) sealIdentitySecret(workspaceID, inboxID uuid.UUID) (string, []byte) {
	plain := identitySecretPrefix + randomToken(32)
	return plain, s.secrets.Seal([]byte(plain), append(workspaceID[:], inboxID[:]...))
}

func (s *Server) ListInboxes(ctx context.Context, _ oas.ListInboxesRequestObject) (oas.ListInboxesResponseObject, error) {
	p := principalFrom(ctx)
	rows, err := s.st.ListInboxes(ctx, store.ListInboxesParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	out := oas.ListInboxes200JSONResponse{Items: make([]oas.Inbox, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, inboxBody(r))
	}
	if err := withImports(ctx, s.st.Queries, p.workspaceID, out.Items); err != nil {
		return nil, err
	}
	if err := withSESTenants(ctx, s.st.Queries, p, out.Items); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) CreateInbox(ctx context.Context, req oas.CreateInboxRequestObject) (oas.CreateInboxResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	b := req.Body
	f := inboxFields{
		name: b.Name, slug: b.Slug, locale: "en", timezone: "UTC", mode: string(oas.Async),
		hours: oas.BusinessHours{Intervals: []oas.BusinessHoursInterval{}}, replyMinutes: b.ExpectedReplyMinutes,
		assignment: assignmentSettings{mode: oas.AssignmentOff, cap: b.AssignmentCap},
	}
	if b.AssignmentMode != nil {
		f.assignment.mode = *b.AssignmentMode
	}
	if b.DefaultLocale != nil {
		f.locale = *b.DefaultLocale
	}
	if b.Timezone != nil {
		f.timezone = *b.Timezone
	}
	if b.Mode != nil {
		f.mode = string(*b.Mode)
	}
	if b.Branding != nil {
		f.branding = *b.Branding
	}
	if b.BusinessHours != nil {
		f.hours = *b.BusinessHours
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	id := newID()
	plain, sealed := s.sealIdentitySecret(p.workspaceID, id)
	var in store.Inbox
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		var err error
		in, err = q.CreateInbox(ctx, store.CreateInboxParams{
			ID: id, WorkspaceID: p.workspaceID, Name: f.name, Slug: f.slug, Branding: mustJSON(f.branding),
			DefaultLocale: f.locale, Timezone: f.timezone, Mode: f.mode, ExpectedReplyMinutes: f.replyMinutes,
			BusinessHours: mustJSON(f.hours), IdentitySecret: sealed, AskForRating: b.AskForRating,
			AssignmentMode: string(f.assignment.mode), AssignmentCap: f.assignment.cap, Now: s.now(),
		})
		if store.IsUniqueViolation(err) {
			return errSlugTaken
		}
		if err != nil {
			return err
		}
		events.add(realtime.InboxCreated, &in.ID, nil, inboxBody(in))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateInbox201JSONResponse{Inbox: inboxBody(in), IdentitySecret: plain}, nil
}

func (s *Server) GetInbox(ctx context.Context, req oas.GetInboxRequestObject) (oas.GetInboxResponseObject, error) {
	in, err := visibleInbox(ctx, s.st.Queries, principalFrom(ctx), req.InboxId)
	if err != nil {
		return nil, err
	}
	out, err := memberInbox(ctx, s.st.Queries, in)
	if err != nil {
		return nil, err
	}
	items := []oas.Inbox{out}
	if err := withSESTenants(ctx, s.st.Queries, principalFrom(ctx), items); err != nil {
		return nil, err
	}
	return oas.GetInbox200JSONResponse(items[0]), nil
}

func (s *Server) UpdateInbox(ctx context.Context, req oas.UpdateInboxRequestObject) (oas.UpdateInboxResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	if ok, err := canSeeInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	} else if !ok {
		return nil, errInboxGone
	}
	b := req.Body
	var out oas.Inbox
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		cur, err := q.LockInbox(ctx, store.LockInboxParams{WorkspaceID: p.workspaceID, ID: req.InboxId})
		if store.IsNotFound(err) {
			return errInboxGone
		}
		if err != nil {
			return err
		}
		f := inboxFields{
			name: cur.Name, slug: cur.Slug, locale: cur.DefaultLocale, timezone: cur.Timezone, mode: cur.Mode,
			replyMinutes: cur.ExpectedReplyMinutes,
			assignment:   assignmentSettings{mode: oas.AssignmentMode(cur.AssignmentMode), cap: cur.AssignmentCap},
		}
		f.assignment.apply(b.AssignmentMode, b.AssignmentCap)
		_ = json.Unmarshal(cur.Branding, &f.branding)
		_ = json.Unmarshal(cur.BusinessHours, &f.hours)
		if b.Name != nil {
			f.name = *b.Name
		}
		if b.Slug != nil {
			f.slug = *b.Slug
		}
		if b.DefaultLocale != nil {
			f.locale = *b.DefaultLocale
		}
		if b.Timezone != nil {
			f.timezone = *b.Timezone
		}
		if b.Mode != nil {
			f.mode = string(*b.Mode)
		}
		if b.Branding != nil {
			f.branding = *b.Branding
		}
		if b.BusinessHours != nil {
			f.hours = *b.BusinessHours
		}
		ask := cur.AskForRating
		if b.AskForRating != nil {
			ask = *b.AskForRating
		}
		if b.ExpectedReplyMinutes.IsSpecified() {
			f.replyMinutes = nil
			if !b.ExpectedReplyMinutes.IsNull() {
				v := b.ExpectedReplyMinutes.MustGet()
				f.replyMinutes = &v
			}
		}
		if err := f.validate(); err != nil {
			return err
		}
		in, err := q.UpdateInbox(ctx, store.UpdateInboxParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, Name: f.name, Slug: f.slug, Branding: mustJSON(f.branding),
			DefaultLocale: f.locale, Timezone: f.timezone, Mode: f.mode, ExpectedReplyMinutes: f.replyMinutes,
			BusinessHours: mustJSON(f.hours), AskForRating: ask,
			AssignmentMode: string(f.assignment.mode), AssignmentCap: f.assignment.cap, Now: s.now(),
		})
		if store.IsUniqueViolation(err) {
			return errSlugTaken
		}
		if err != nil {
			return err
		}
		if b.FeedbackCategories != nil {
			if in, err = s.setFeedbackCategories(ctx, q, in, *b.FeedbackCategories); err != nil {
				return err
			}
		}
		if b.Sender != nil {
			if in, err = s.setInboxSender(ctx, q, in, *b.Sender); err != nil {
				return err
			}
		}
		if out, err = memberInbox(ctx, q, in); err != nil {
			return err
		}
		events.add(realtime.InboxUpdated, &out.Id, nil, out)
		if f.assignment.widened(cur.AssignmentMode, cur.AssignmentCap) {
			events.jobOnce(AssignWaitingArgs{WorkspaceID: p.workspaceID}, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := []oas.Inbox{out}
	if err := withSESTenants(ctx, s.st.Queries, p, items); err != nil {
		return nil, err
	}
	return oas.UpdateInbox200JSONResponse(items[0]), nil
}

func (s *Server) DeleteInbox(ctx context.Context, req oas.DeleteInboxRequestObject) (oas.DeleteInboxResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	var keys []string
	var pages []uuid.UUID
	err := s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		in, err := q.LockInbox(ctx, store.LockInboxParams{WorkspaceID: p.workspaceID, ID: req.InboxId})
		if store.IsNotFound(err) {
			return errInboxGone
		}
		if err != nil {
			return err
		}
		if keys, err = q.ListInboxStorageKeys(ctx, store.ListInboxStorageKeysParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId}); err != nil {
			return err
		}
		raw, err := q.ListInboxRawKeys(ctx, store.ListInboxRawKeysParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
		if err != nil {
			return err
		}
		keys = append(keys, raw...)
		if t, err := inboxTenant(ctx, q, p.workspaceID, req.InboxId); err != nil {
			return err
		} else if t != nil {
			events.job(SESTenantDeleteArgs{Name: t.Name, Region: t.Region}, nil)
		}
		owned, err := q.ListInboxOwnDomains(ctx, store.ListInboxOwnDomainsParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
		if err != nil {
			return err
		}
		for _, o := range owned {
			releaseCloudflareHostname(events, p.workspaceID, o.CfHostnameID, o.CustomDomain)
			pages = append(pages, o.ID)
		}
		driven, err := slaConversationsOf(ctx, q, p.workspaceID, func(sp store.SlaPolicy) bool { return slices.Contains(sp.InboxIds, req.InboxId) })
		if err != nil {
			return err
		}
		if err := lockSLAConversations(ctx, q, p.workspaceID, driven); err != nil {
			return err
		}
		convs, err := q.ListInboxConversationIDs(ctx, store.ListInboxConversationIDsParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
		if err != nil {
			return err
		}
		if err := forgetConversations(ctx, q, p.workspaceID, convs); err != nil {
			return err
		}
		if err := s.scrubSlackInbox(ctx, q, events, p.workspaceID, req.InboxId); err != nil {
			return err
		}
		if _, err = q.DeleteInbox(ctx, store.DeleteInboxParams{WorkspaceID: p.workspaceID, ID: req.InboxId}); err != nil {
			return err
		}
		if err := q.DeleteAssignmentTurns(ctx, store.DeleteAssignmentTurnsParams{WorkspaceID: p.workspaceID, PoolID: req.InboxId}); err != nil {
			return err
		}
		if err := q.StripInboxFromSLAPolicies(ctx, store.StripInboxFromSLAPoliciesParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId, Now: s.now()}); err != nil {
			return err
		}
		if err := s.syncSLAConversations(ctx, q, events, p.workspaceID, driven); err != nil {
			return err
		}
		if err := s.audit(ctx, q, p, oas.AuditInboxDeleted, oas.AuditTargetInbox, in.ID, auditDetails{"name": in.Name}); err != nil {
			return err
		}
		id := req.InboxId
		events.add(realtime.InboxDeleted, &id, nil, oas.InboxRef{Id: id})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, page := range pages {
		s.statusDomainCache.forget(page)
	}
	s.deleteObjects(ctx, keys)
	return oas.DeleteInbox204Response{}, nil
}

func (s *Server) RotateInboxIdentitySecret(ctx context.Context, req oas.RotateInboxIdentitySecretRequestObject) (oas.RotateInboxIdentitySecretResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	plain, sealed := s.sealIdentitySecret(p.workspaceID, req.InboxId)
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		n, err := q.SetInboxIdentitySecret(ctx, store.SetInboxIdentitySecretParams{
			WorkspaceID: p.workspaceID, ID: req.InboxId, IdentitySecret: sealed, Now: s.now(),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return errInboxGone
		}
		return s.audit(ctx, q, p, oas.AuditInboxIdentitySecretRotated, oas.AuditTargetInbox, req.InboxId, nil)
	})
	if err != nil {
		return nil, err
	}
	return oas.RotateInboxIdentitySecret200JSONResponse{IdentitySecret: plain}, nil
}

func (s *Server) ListInboxMembers(ctx context.Context, req oas.ListInboxMembersRequestObject) (oas.ListInboxMembersResponseObject, error) {
	p := principalFrom(ctx)
	if _, err := visibleInbox(ctx, s.st.Queries, p, req.InboxId); err != nil {
		return nil, err
	}
	rows, err := s.st.ListInboxMembers(ctx, store.ListInboxMembersParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId})
	if err != nil {
		return nil, err
	}
	members := make([]oas.Member, len(rows))
	for i, r := range rows {
		members[i] = memberBody(store.GetMemberRow{ID: r.ID, WorkspaceID: r.WorkspaceID, PersonID: r.PersonID, Role: r.Role, CreatedAt: r.CreatedAt, Email: r.Email, Name: r.Name})
	}
	if err := s.withPresence(ctx, s.st.Queries, p.workspaceID, members); err != nil {
		return nil, err
	}
	out := oas.ListInboxMembers200JSONResponse{Items: make([]oas.InboxMember, len(rows))}
	for i, m := range members {
		via := oas.InboxAccessTeam
		if rows[i].Direct {
			via = oas.InboxAccessDirect
		}
		out.Items[i] = oas.InboxMember{
			Id: m.Id, PersonId: m.PersonId, Email: m.Email, Name: m.Name, Role: m.Role, Availability: m.Availability, Online: m.Online,
			CreatedAt: m.CreatedAt, Via: via, TeamIds: rows[i].TeamIds,
		}
	}
	return out, nil
}

func (s *Server) inboxAndMember(ctx context.Context, p principal, inboxID, memberID uuid.UUID) (store.Inbox, error) {
	in, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: p.workspaceID, ID: inboxID})
	if store.IsNotFound(err) {
		return in, errInboxGone
	} else if err != nil {
		return in, err
	}
	if _, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: memberID}); store.IsNotFound(err) {
		return in, errMemberGone
	} else if err != nil {
		return in, err
	}
	return in, nil
}

func (s *Server) GrantInboxAccess(ctx context.Context, req oas.GrantInboxAccessRequestObject) (oas.GrantInboxAccessResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	in, err := s.inboxAndMember(ctx, p, req.InboxId, req.MemberId)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		changed, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{req.MemberId}, func() (bool, error) {
			n, err := q.GrantInboxAccess(ctx, store.GrantInboxAccessParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId, MemberID: req.MemberId, Now: s.now()})
			return n > 0, err
		})
		if err != nil || !changed {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberInboxAccessGranted, oas.AuditTargetMember, req.MemberId, auditDetails{"inbox_id": in.ID, "inbox_name": in.Name})
	})
	if store.IsForeignKeyViolation(err) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	return oas.GrantInboxAccess204Response{}, nil
}

func (s *Server) RevokeInboxAccess(ctx context.Context, req oas.RevokeInboxAccessRequestObject) (oas.RevokeInboxAccessResponseObject, error) {
	p := principalFrom(ctx)
	if err := requireManagerOrKey(p); err != nil {
		return nil, err
	}
	in, err := s.inboxAndMember(ctx, p, req.InboxId, req.MemberId)
	if err != nil {
		return nil, err
	}
	err = s.inTx(ctx, p.workspaceID, func(q *store.Queries, events *eventBatch) error {
		if err := q.LockInboxAccess(ctx, p.workspaceID); err != nil {
			return err
		}
		changed, err := changeAccess(ctx, q, events, p.workspaceID, []uuid.UUID{req.MemberId}, func() (bool, error) {
			n, err := q.RevokeInboxAccess(ctx, store.RevokeInboxAccessParams{WorkspaceID: p.workspaceID, InboxID: req.InboxId, MemberID: req.MemberId})
			return n > 0, err
		})
		if err != nil || !changed {
			return err
		}
		return s.audit(ctx, q, p, oas.AuditMemberInboxAccessRevoked, oas.AuditTargetMember, req.MemberId, auditDetails{"inbox_id": in.ID, "inbox_name": in.Name})
	})
	if err != nil {
		return nil, err
	}
	return oas.RevokeInboxAccess204Response{}, nil
}

func accessChanged(events *eventBatch, inboxID, memberID uuid.UUID, granted bool) {
	events.add(realtime.InboxAccessChanged, &inboxID, nil, oas.InboxAccessChange{InboxId: inboxID, MemberId: memberID, Granted: granted})
}

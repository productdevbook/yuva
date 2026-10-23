package api

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	// consentEvent carries a change of consent; it is never stored or counted.
	consentEvent = "yuva_consent"
	// persistentMonths is how long a persistent id may stand for a visitor: the device keeps it at
	// most that long, and the cohorts job forgets it that long after it was first seen.
	persistentMonths        = 13
	defaultContactVisits    = 20
	maxContactVisits        = 100
	retentionDayPeriods     = 30
	retentionPeriods        = 12
	defaultRetentionCohorts = 12
)

var limitAnalyticsConsent = limit{20, time.Minute}

// offers says whether a property offers a consent feature: never on a children's property, and
// identify only next to persistent and with a linked inbox, whose identity secret signs the tokens.
func (c *cachedProperty) offers(feature oas.AnalyticsConsentFeature) bool {
	p := c.prop
	if p.Audience != string(oas.AnalyticsAudienceGeneral) || !slices.Contains(p.ConsentFeatures, string(feature)) {
		return false
	}
	if feature == oas.AnalyticsConsentIdentify {
		return p.InboxID != nil && slices.Contains(p.ConsentFeatures, string(oas.AnalyticsConsentPersistent))
	}
	return true
}

// visitorKey is the property's own key for persistent ids, opened once per cached property.
func (s *Server) visitorKey(cp *cachedProperty) ([]byte, error) {
	cp.keyOnce.Do(func() {
		cp.key, cp.keyErr = s.secrets.Open(cp.prop.VisitorKey, analyticsSecretContext(cp.prop.WorkspaceID, cp.prop.ID))
	})
	return cp.key, cp.keyErr
}

// consentedVisitor makes the event's visitor the hash of its persistent id when the visitor
// consented and the property offers persistent; otherwise the item is left as default mode has it.
func (s *Server) consentedVisitor(ctx context.Context, cp *cachedProperty, c *collectConsent, it *analyticsItem) {
	if c == nil || c.A != 1 || !analytics.ValidPersistentID(c.P) || !cp.offers(oas.AnalyticsConsentPersistent) {
		return
	}
	key, err := s.visitorKey(cp)
	if err != nil {
		s.log.ErrorContext(ctx, "analytics visitor key", slog.Any("error", err))
		return
	}
	v := analytics.PersistentHash(key, cp.prop.ID, c.P)
	it.visitor, it.persistent = func([]byte) int64 { return v }, true
}

// consentAllowed limits identifications and withdrawals per shortened address and property.
func (s *Server) consentAllowed(ctx context.Context, r *http.Request, cp *cachedProperty) bool {
	addr, err := netip.ParseAddr(s.visitorIP(r))
	if err != nil {
		return false
	}
	short := analytics.ShortenAddr(addr.Unmap())
	return s.limits.allow(ctx, nil, "analytics:consent:"+cp.prop.ID.String()+":"+short.String(), limitAnalyticsConsent)
}

// takeWithdrawals applies the withdrawals of a request that is otherwise not collected (Sec-GPC or
// DNT): a visitor who turns those on still gets their persistent id unlinked.
func (s *Server) takeWithdrawals(w http.ResponseWriter, r *http.Request, cp *cachedProperty) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAnalyticsBody))
	if err != nil {
		return
	}
	evs, _, err := parseAnalyticsBatch(body)
	if err != nil {
		return
	}
	var changes []collectEvent
	for _, e := range evs {
		if e.N == consentEvent && e.C != nil && e.C.A == 0 {
			changes = append(changes, e)
		}
	}
	if len(changes) > 0 {
		if err := s.takeConsentChanges(r.Context(), r, cp, changes, s.now()); err != nil {
			s.log.ErrorContext(r.Context(), "analytics consent", slog.Any("error", err))
		}
	}
}

// takeConsentChanges applies identifications (c.a 1 with c.p and c.j, on a property that offers
// identify) and withdrawals (a yuva_consent event with c.a 0 and c.p). r is nil for server events,
// which an API key sends and no address limits. Anything else is ignored.
func (s *Server) takeConsentChanges(ctx context.Context, r *http.Request, cp *cachedProperty, changes []collectEvent, now time.Time) *apiError {
	seen := map[string]bool{}
	for _, e := range changes {
		c := e.C
		if c == nil || !analytics.ValidPersistentID(c.P) {
			continue
		}
		withdraw := e.N == consentEvent && c.A == 0
		identify := c.A == 1 && c.J != "" && cp.offers(oas.AnalyticsConsentIdentify)
		if !withdraw && !identify {
			continue
		}
		k := strconv.FormatBool(withdraw) + c.P
		if seen[k] {
			continue
		}
		seen[k] = true
		if r != nil && !s.consentAllowed(ctx, r, cp) {
			continue
		}
		key, err := s.visitorKey(cp)
		if err != nil {
			s.log.ErrorContext(ctx, "analytics visitor key", slog.Any("error", err))
			return errInternal
		}
		v := analytics.PersistentHash(key, cp.prop.ID, c.P)
		if withdraw {
			err = s.withdrawVisitor(ctx, cp.prop, v)
		} else {
			err = s.identifyVisitor(ctx, cp.prop, v, c.J, now, analytics.LocalDay(now, cp.loc))
		}
		if err != nil {
			s.log.ErrorContext(ctx, "analytics consent", slog.Any("error", err))
			return errInternal
		}
	}
	return nil
}

// identifyVisitor links a consented persistent id to the contact of a valid identity token of the
// property's inbox, found or made as /client/v1/session does, and its raw visits with it. A token
// that fails any check, or whose jti was used, links nothing.
func (s *Server) identifyVisitor(ctx context.Context, prop store.AnalyticsProperty, visitor int64, token string, now, day time.Time) error {
	ws := prop.WorkspaceID
	if prop.InboxID == nil {
		return nil
	}
	inbox, err := s.st.GetInbox(ctx, store.GetInboxParams{WorkspaceID: ws, ID: *prop.InboxID})
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	secret, err := s.secrets.Open(inbox.IdentitySecret, identitySecretContext(ws, inbox.ID))
	if err != nil {
		return err
	}
	claims, err := verifyIdentityToken(strings.TrimSpace(token), secret, now)
	if err != nil {
		return nil
	}
	for attempt := 0; ; attempt++ {
		err = s.inTx(ctx, ws, func(q *store.Queries, events *eventBatch) error {
			if claims.jti != "" {
				if err := s.useTokenID(ctx, q, inbox, claims); err != nil {
					return err
				}
			}
			contact, err := s.identifyContact(ctx, q, events, inbox, claims, "")
			if err != nil {
				return err
			}
			if err := q.LinkAnalyticsVisitor(ctx, store.LinkAnalyticsVisitorParams{
				WorkspaceID: ws, PropertyID: prop.ID, Visitor: visitor, Day: day, ContactID: &contact.ID, Now: &now,
			}); err != nil {
				return err
			}
			_, err = q.SetAnalyticsVisitorContact(ctx, store.SetAnalyticsVisitorContactParams{WorkspaceID: ws, PropertyID: prop.ID, Visitor: visitor, ContactID: &contact.ID})
			return err
		})
		if err == nil || attempt == 1 || !store.IsUniqueViolation(err) {
			break
		}
	}
	if errors.Is(err, errTokenReused) {
		return nil
	}
	return err
}

// withdrawVisitor forgets a persistent id's cohort days and contact, so its visits leave the
// contact's page and retention.
func (s *Server) withdrawVisitor(ctx context.Context, prop store.AnalyticsProperty, visitor int64) error {
	return s.st.InTx(ctx, func(q *store.Queries) error {
		if _, err := q.DeleteAnalyticsPersistentVisitor(ctx, store.DeleteAnalyticsPersistentVisitorParams{
			WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID, Visitor: visitor}); err != nil {
			return err
		}
		_, err := q.SetAnalyticsVisitorContact(ctx, store.SetAnalyticsVisitorContactParams{WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID, Visitor: visitor})
		return err
	})
}

// dropAnalyticsVisits deletes the events and page rows of visits whose rows were just deleted, and
// has their days rolled up again.
func dropAnalyticsVisits(ctx context.Context, q *store.Queries, ws uuid.UUID, props []uuid.UUID, days []time.Time, visits []int64) error {
	if len(visits) == 0 {
		return nil
	}
	if err := q.DeleteAnalyticsVisitEvents(ctx, store.DeleteAnalyticsVisitEventsParams{WorkspaceID: ws, PropertyIds: props, Days: days, Visits: visits}); err != nil {
		return err
	}
	if err := q.DeleteAnalyticsVisitPages(ctx, store.DeleteAnalyticsVisitPagesParams{WorkspaceID: ws, PropertyIds: props, Days: days, Visits: visits}); err != nil {
		return err
	}
	return q.MarkAnalyticsVisitDaysChanged(ctx, store.MarkAnalyticsVisitDaysChangedParams{WorkspaceID: ws, PropertyIds: props, Days: days})
}

// deleteContactAnalytics deletes the visits linked to a contact that is about to be deleted; its
// persistent ids go with the contact row.
func deleteContactAnalytics(ctx context.Context, q *store.Queries, ws, contactID uuid.UUID) error {
	gone, err := q.DeleteContactAnalyticsVisits(ctx, store.DeleteContactAnalyticsVisitsParams{WorkspaceID: ws, ContactID: &contactID})
	if err != nil {
		return err
	}
	props, days, visits := make([]uuid.UUID, len(gone)), make([]time.Time, len(gone)), make([]int64, len(gone))
	for i, g := range gone {
		props[i], days[i], visits[i] = g.PropertyID, g.Day, g.Visit
	}
	return dropAnalyticsVisits(ctx, q, ws, props, days, visits)
}

// moveContactAnalytics moves the visits and persistent ids linked to a contact merged into another.
func moveContactAnalytics(ctx context.Context, q *store.Queries, ws, from, to uuid.UUID) error {
	if err := q.MoveContactAnalyticsVisits(ctx, store.MoveContactAnalyticsVisitsParams{WorkspaceID: ws, FromContact: &from, ToContact: &to}); err != nil {
		return err
	}
	return q.MoveContactAnalyticsVisitors(ctx, store.MoveContactAnalyticsVisitorsParams{WorkspaceID: ws, FromContact: &from, ToContact: &to})
}

func (s *Server) DeleteAnalyticsVisitor(ctx context.Context, req oas.DeleteAnalyticsVisitorRequestObject) (oas.DeleteAnalyticsVisitorResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var id uuid.UUID
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		prop, err := visibleProperty(ctx, q, p, req.PropertyId, false)
		if err != nil {
			return err
		}
		if err := requireAnalyticsManager(p); err != nil {
			return err
		}
		if !analytics.ValidPersistentID(req.PersistentId) {
			return errValidation("persistentId is 16 to 64 characters of A-Z, a-z, 0-9, - and _")
		}
		id = prop.ID
		key, err := s.secrets.Open(prop.VisitorKey, analyticsSecretContext(p.workspaceID, prop.ID))
		if err != nil {
			return err
		}
		v := analytics.PersistentHash(key, prop.ID, req.PersistentId)
		gone, err := q.DeleteAnalyticsVisitorVisits(ctx, store.DeleteAnalyticsVisitorVisitsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, Visitor: v})
		if err != nil {
			return err
		}
		props, days, visits := make([]uuid.UUID, len(gone)), make([]time.Time, len(gone)), make([]int64, len(gone))
		for i, g := range gone {
			props[i], days[i], visits[i] = g.PropertyID, g.Day, g.Visit
		}
		if err := dropAnalyticsVisits(ctx, q, p.workspaceID, props, days, visits); err != nil {
			return err
		}
		_, err = q.DeleteAnalyticsPersistentVisitor(ctx, store.DeleteAnalyticsPersistentVisitorParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, Visitor: v})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.analytics.answers.forget(id)
	return oas.DeleteAnalyticsVisitor204Response{}, nil
}

// bucketStart is the start of the day, week (from Monday) or month that holds d.
func bucketStart(grain string, d time.Time) time.Time {
	switch grain {
	case "week":
		return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
	case "month":
		return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return d
}

// periodsBetween counts the grain's periods from the one holding a to the one holding b.
func periodsBetween(grain string, a, b time.Time) int {
	switch grain {
	case "week":
		return int(bucketStart(grain, b).Sub(bucketStart(grain, a)).Hours()/24) / 7
	case "month":
		return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	}
	return int(b.Sub(a).Hours() / 24)
}

func (s *Server) GetAnalyticsRetention(ctx context.Context, req oas.GetAnalyticsRetentionRequestObject) (oas.GetAnalyticsRetentionResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	grain, periods := "week", retentionPeriods
	if g := req.Params.Grain; g != nil {
		if !g.Valid() {
			return nil, errValidation("grain must be day, week or month")
		}
		grain = string(*g)
	}
	if grain == "day" {
		periods = retentionDayPeriods
	}
	today := analytics.LocalDay(s.now(), propertyLocation(prop.Timezone))
	to := today
	if req.Params.To != nil {
		to = req.Params.To.Time
	}
	var from time.Time
	switch {
	case req.Params.From != nil:
		from = req.Params.From.Time
	case grain == "day":
		from = to.AddDate(0, 0, -retentionDayPeriods)
	case grain == "week":
		from = bucketStart(grain, to).AddDate(0, 0, -7*defaultRetentionCohorts)
	default:
		from = bucketStart(grain, to).AddDate(0, -defaultRetentionCohorts, 0)
	}
	if from, to, err = checkedRange(from, to); err != nil {
		return nil, err
	}
	from = bucketStart(grain, from)
	rows, err := s.st.ListAnalyticsCohorts(ctx, store.ListAnalyticsCohortsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, Grain: grain, FromDay: from, ToDay: to})
	if err != nil {
		return nil, err
	}
	out := oas.AnalyticsRetention{Grain: oas.AnalyticsRetentionGrain(grain), From: openapi_types.Date{Time: from}, To: openapi_types.Date{Time: to},
		BuiltAt: prop.CohortsBuiltAt, Cohorts: []oas.AnalyticsCohort{}}
	for _, r := range rows {
		c := r.Cohort.UTC()
		if n := len(out.Cohorts); n == 0 || !out.Cohorts[n-1].Cohort.Time.Equal(c) {
			length := min(periodsBetween(grain, c, today), periods) + 1
			out.Cohorts = append(out.Cohorts, oas.AnalyticsCohort{Cohort: openapi_types.Date{Time: c}, Periods: make([]int64, max(length, 1))})
		}
		last := &out.Cohorts[len(out.Cohorts)-1]
		if int(r.Period) < len(last.Periods) {
			last.Periods[r.Period] = r.Visitors
		}
		if r.Period == 0 {
			last.Visitors = r.Visitors
		}
	}
	return oas.GetAnalyticsRetention200JSONResponse(out), nil
}

func encodeVisitCursor(c analytics.VisitCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.Started.UnixMicro(), 10) + "." + strconv.FormatInt(c.Visit, 10)))
}

func decodeVisitCursor(s string) (*analytics.VisitCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	at, visit, ok := strings.Cut(string(raw), ".")
	if !ok {
		return nil, errors.New("bad cursor")
	}
	us, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(visit, 10, 64)
	if err != nil {
		return nil, err
	}
	return &analytics.VisitCursor{Started: time.UnixMicro(us).UTC(), Visit: v}, nil
}

func (s *Server) ListContactVisits(ctx context.Context, req oas.ListContactVisitsRequestObject) (oas.ListContactVisitsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	r, err := s.st.GetContact(ctx, store.GetContactParams{WorkspaceID: p.workspaceID, ID: req.ContactId})
	if store.IsNotFound(err) {
		return nil, errContactGone
	}
	if err != nil {
		return nil, err
	}
	if err := visibleContact(ctx, s.st.Queries, p, r.ID); err != nil {
		return nil, err
	}
	limit := defaultContactVisits
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > maxContactVisits {
		return nil, errValidation("limit must be 1 to 100")
	}
	var after *analytics.VisitCursor
	if req.Params.Cursor != nil && *req.Params.Cursor != "" {
		if after, err = decodeVisitCursor(*req.Params.Cursor); err != nil {
			return nil, errValidation("cursor is not one this list returned")
		}
	}
	var inboxes []uuid.UUID
	if !p.seesAllInboxes() {
		if inboxes, err = s.st.ListViewerInboxIDs(ctx, store.ListViewerInboxIDsParams{WorkspaceID: p.workspaceID, ViewerID: p.viewerID()}); err != nil {
			return nil, err
		}
		inboxes = nonNil(inboxes)
	}
	list, err := s.analytics.store.ContactVisits(ctx, p.workspaceID, r.ID, inboxes, after, limit+1)
	if err != nil {
		return nil, err
	}
	out := oas.AnalyticsContactVisitList{Items: []oas.AnalyticsContactVisit{}}
	if len(list) > limit {
		list = list[:limit]
		next := encodeVisitCursor(analytics.VisitCursor{Started: list[limit-1].Started, Visit: list[limit-1].Visit})
		out.NextCursor = &next
	}
	for _, v := range list {
		item := oas.AnalyticsContactVisit{
			PropertyId: v.PropertyID, PropertyName: v.PropertyName, StartedAt: v.Started, EndedAt: v.Ended, Hostname: v.Hostname,
			EntryPage: v.Entry, ExitPage: v.Exit, Pageviews: int(v.Views), Events: int(v.Events), Channel: v.Channel, Source: v.Source,
			UtmCampaign: v.Campaign, Country: v.Country, City: v.City, Device: v.Device, Browser: v.Browser, Os: v.OS, Platform: v.Platform,
			AppVersion: v.AppVersion, VisitorType: v.VisitorType, Pages: make([]oas.AnalyticsVisitPage, len(v.Pages)),
		}
		for i, pg := range v.Pages {
			item.Pages[i] = oas.AnalyticsVisitPage{Path: pg.Path, Views: int(pg.Views)}
		}
		out.Items = append(out.Items, item)
	}
	return oas.ListContactVisits200JSONResponse(out), nil
}

// BuildAnalyticsCohorts forgets the persistent ids first seen more than 13 months ago and rebuilds
// the retention cohorts of every property that has consented visitors or had them.
func (s *Server) BuildAnalyticsCohorts(ctx context.Context) error {
	if s.analytics == nil {
		return nil
	}
	ids, err := s.st.ListAnalyticsWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	before := analytics.LocalDay(now, time.UTC).AddDate(0, -persistentMonths, 0)
	for _, ws := range ids {
		for {
			n, err := s.st.DeleteExpiredAnalyticsVisitors(ctx, store.DeleteExpiredAnalyticsVisitorsParams{WorkspaceID: ws, Before: before})
			if err != nil {
				return err
			}
			if n < 1000 {
				break
			}
		}
		props, err := s.st.ListAnalyticsCohortProperties(ctx, ws)
		if err != nil {
			return err
		}
		for _, id := range props {
			if err := s.st.InTx(ctx, func(q *store.Queries) error {
				if err := q.DeleteAnalyticsCohorts(ctx, store.DeleteAnalyticsCohortsParams{WorkspaceID: ws, PropertyID: id}); err != nil {
					return err
				}
				if _, err := q.BuildAnalyticsCohorts(ctx, store.BuildAnalyticsCohortsParams{WorkspaceID: ws, PropertyID: id}); err != nil {
					return err
				}
				return q.MarkAnalyticsCohortsBuilt(ctx, store.MarkAnalyticsCohortsBuiltParams{WorkspaceID: ws, ID: id, Now: &now})
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"golang.org/x/net/idna"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	analyticsKeyPrefix      = "yuva_ak_"
	analyticsKeyOverlap     = 24 * time.Hour
	defaultRetentionMonths  = 25
	maxAnalyticsDeleteRange = 400
)

var (
	errPropertyGone        = problem(http.StatusNotFound, "not_found", "no such analytics property")
	errPropertyLimit       = problem(http.StatusForbidden, "property_limit", "this workspace has as many analytics properties as the server allows (YUVA_ANALYTICS_PROPERTIES_PER_WORKSPACE)")
	errPropertyConfirm     = problem(http.StatusBadRequest, "confirmation_mismatch", "type the property's name exactly to delete it")
	analyticsDomainPattern = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	currencyPattern        = regexp.MustCompile(`^[A-Z]{3}$`)
)

func (s *Server) analyticsOn() error {
	if s.analytics == nil {
		return errNotFound
	}
	return nil
}

// propertyVisible: agents and keys limited to inboxes see the properties of their inboxes; a
// property without an inbox only those who see every inbox.
func propertyVisible(ctx context.Context, q *store.Queries, p principal, prop store.AnalyticsProperty) (bool, error) {
	if prop.InboxID == nil {
		return p.seesAllInboxes(), nil
	}
	return inboxObjectsVisible(ctx, q, p, *prop.InboxID)
}

func visibleProperty(ctx context.Context, q *store.Queries, p principal, id uuid.UUID, lock bool) (store.AnalyticsProperty, error) {
	get := q.GetAnalyticsProperty
	if lock {
		get = func(ctx context.Context, arg store.GetAnalyticsPropertyParams) (store.AnalyticsProperty, error) {
			return q.LockAnalyticsProperty(ctx, store.LockAnalyticsPropertyParams(arg))
		}
	}
	prop, err := get(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: p.workspaceID, ID: id})
	if store.IsNotFound(err) {
		return prop, errPropertyGone
	}
	if err != nil {
		return prop, err
	}
	ok, err := propertyVisible(ctx, q, p, prop)
	if err != nil {
		return prop, err
	}
	if !ok {
		return prop, errPropertyGone
	}
	return prop, nil
}

func requireAnalyticsManager(p principal) error {
	if p.isKey() && p.limited {
		return errForbidden
	}
	return requireManagerOrKey(p)
}

func pathRules(raw []byte) []analytics.PathRule {
	var out []analytics.PathRule
	_ = json.Unmarshal(raw, &out)
	return out
}

func (s *Server) propertyBody(prop store.AnalyticsProperty, visitorsToday int64) oas.AnalyticsProperty {
	out := oas.AnalyticsProperty{
		Id: prop.ID, InboxId: prop.InboxID, Name: prop.Name, PublicKey: prop.PublicKey, Timezone: prop.Timezone, Currency: prop.Currency,
		Collection: oas.AnalyticsCollection(prop.Collection), Audience: oas.AnalyticsAudience(prop.Audience),
		Domains: prop.Domains, ExcludedPaths: prop.ExcludedPaths, TrackedProps: prop.TrackedProps, RetentionMonths: prop.RetentionMonths,
		VisitorsToday: visitorsToday, FirstEventAt: prop.FirstEventAt, LastEventAt: prop.LastEventAt, CreatedAt: prop.CreatedAt, UpdatedAt: prop.UpdatedAt,
		Platforms: make([]oas.AnalyticsPlatform, len(prop.Platforms)), ConsentFeatures: make([]oas.AnalyticsConsentFeature, len(prop.ConsentFeatures)),
		ExcludedIps: make([]string, len(prop.ExcludedIps)), PathRules: []oas.AnalyticsPathRule{},
	}
	if prop.PreviousKeyUntil != nil && prop.PreviousKeyUntil.After(s.now()) {
		out.PreviousKeyExpiresAt = prop.PreviousKeyUntil
	}
	for i, v := range prop.Platforms {
		out.Platforms[i] = oas.AnalyticsPlatform(v)
	}
	for i, v := range prop.ConsentFeatures {
		out.ConsentFeatures[i] = oas.AnalyticsConsentFeature(v)
	}
	for i, v := range prop.ExcludedIps {
		if v.IsSingleIP() {
			out.ExcludedIps[i] = v.Addr().String()
		} else {
			out.ExcludedIps[i] = v.String()
		}
	}
	for _, r := range pathRules(prop.PathRules) {
		out.PathRules = append(out.PathRules, oas.AnalyticsPathRule{Pattern: r.Pattern, Replacement: r.Replacement})
	}
	return out
}

func (s *Server) propertyBodies(ctx context.Context, ws uuid.UUID, props []store.AnalyticsProperty) ([]oas.AnalyticsProperty, error) {
	days := map[uuid.UUID]time.Time{}
	for _, p := range props {
		days[p.ID] = analytics.LocalDay(s.now(), propertyLocation(p.Timezone))
	}
	counts, err := s.analytics.store.VisitorsToday(ctx, ws, days)
	if err != nil {
		return nil, err
	}
	out := make([]oas.AnalyticsProperty, len(props))
	for i, p := range props {
		out[i] = s.propertyBody(p, counts[p.ID])
	}
	return out, nil
}

func propertyLocation(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// propertyFields are a property's settings while they are checked.
type propertyFields struct {
	inboxID         *uuid.UUID
	name            string
	domains         []string
	platforms       []string
	timezone        string
	currency        string
	collection      string
	consentFeatures []string
	audience        string
	excludedPaths   []string
	excludedIPs     []netip.Prefix
	pathRules       []analytics.PathRule
	trackedProps    []string
	retention       *int32
}

func fieldsOf(prop store.AnalyticsProperty) propertyFields {
	return propertyFields{
		inboxID: prop.InboxID, name: prop.Name, domains: prop.Domains, platforms: prop.Platforms, timezone: prop.Timezone, currency: prop.Currency,
		collection: prop.Collection, consentFeatures: prop.ConsentFeatures, audience: prop.Audience, excludedPaths: prop.ExcludedPaths,
		excludedIPs: prop.ExcludedIps, pathRules: pathRules(prop.PathRules), trackedProps: prop.TrackedProps, retention: prop.RetentionMonths,
	}
}

func normalizeAnalyticsDomain(v string) (string, bool) {
	v = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(v)), ".")
	if v == "[::1]" || v == "::1" {
		return "[::1]", true
	}
	host, wild := strings.CutPrefix(v, "*.")
	if a, err := idna.Lookup.ToASCII(host); err == nil {
		host = a
	}
	if wild {
		host = "*." + host
	}
	return host, len(host) <= 253 && analyticsDomainPattern.MatchString(host)
}

func validPathPattern(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "/") || len(v) > 300 {
		return "", errValidation("path patterns start with / and have at most 300 characters")
	}
	return v, nil
}

func uniqueStrings(in []string) []string {
	out := []string{}
	for _, v := range in {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func (f *propertyFields) validate() error {
	var err error
	if f.name, err = trimmed(f.name, 1, 100, "name"); err != nil {
		return err
	}
	if len(f.domains) < 1 || len(f.domains) > 50 {
		return errValidation("domains must list 1 to 50 host names")
	}
	for i, d := range f.domains {
		n, ok := normalizeAnalyticsDomain(d)
		if !ok {
			return errValidation(strconv.Quote(d) + " is not a host name such as www.example.com or *.example.com")
		}
		f.domains[i] = n
	}
	f.domains = uniqueStrings(f.domains)
	if len(f.platforms) == 0 {
		return errValidation("platforms must name at least one platform")
	}
	for _, v := range f.platforms {
		if !oas.AnalyticsPlatform(v).Valid() {
			return errValidation("unknown platform " + strconv.Quote(v))
		}
	}
	f.platforms = uniqueStrings(f.platforms)
	if _, err := time.LoadLocation(f.timezone); err != nil || f.timezone == "" || f.timezone == "Local" {
		return errValidation("timezone must be an IANA time zone such as Europe/Istanbul")
	}
	f.currency = strings.ToUpper(strings.TrimSpace(f.currency))
	if !currencyPattern.MatchString(f.currency) {
		return errValidation("currency must be an ISO 4217 code such as EUR")
	}
	if !oas.AnalyticsCollection(f.collection).Valid() {
		return errValidation("collection must be anonymous or consent")
	}
	if !oas.AnalyticsAudience(f.audience).Valid() {
		return errValidation("audience must be general or children")
	}
	for _, v := range f.consentFeatures {
		if !oas.AnalyticsConsentFeature(v).Valid() {
			return errValidation("unknown consent feature " + strconv.Quote(v))
		}
	}
	f.consentFeatures = uniqueStrings(f.consentFeatures)
	if len(f.excludedPaths) > 50 {
		return errValidation("excluded_paths may list at most 50 patterns")
	}
	for i, v := range f.excludedPaths {
		if f.excludedPaths[i], err = validPathPattern(v); err != nil {
			return err
		}
	}
	f.excludedPaths = uniqueStrings(f.excludedPaths)
	if len(f.excludedIPs) > 50 {
		return errValidation("excluded_ips may list at most 50 addresses or ranges")
	}
	if len(f.pathRules) > 50 {
		return errValidation("path_rules may list at most 50 rules")
	}
	for i, r := range f.pathRules {
		if f.pathRules[i].Pattern, err = validPathPattern(r.Pattern); err != nil {
			return err
		}
		if f.pathRules[i].Replacement, err = trimmed(r.Replacement, 1, 300, "path_rules.replacement"); err != nil {
			return err
		}
	}
	if len(f.trackedProps) > 10 {
		return errValidation("tracked_props may list at most 10 keys")
	}
	for i, v := range f.trackedProps {
		if f.trackedProps[i], err = trimmed(v, 1, 300, "tracked_props"); err != nil {
			return err
		}
		if strings.HasPrefix(f.trackedProps[i], "$") || strings.HasPrefix(f.trackedProps[i], "yuva_") {
			return errValidation("prop keys starting with $ or yuva_ are reserved")
		}
	}
	f.trackedProps = uniqueStrings(f.trackedProps)
	if f.audience == string(oas.AnalyticsAudienceChildren) && (len(f.consentFeatures) > 0 || len(f.trackedProps) > 0) {
		return errValidation("a children's property cannot offer consent features or tracked props")
	}
	if slices.Contains(f.consentFeatures, string(oas.AnalyticsConsentIdentify)) {
		if !slices.Contains(f.consentFeatures, string(oas.AnalyticsConsentPersistent)) {
			return errValidation("identify needs persistent: a visitor is identified through their persistent id")
		}
		if f.inboxID == nil {
			return errValidation("identify needs a linked inbox, whose identity secret signs the tokens")
		}
	}
	if f.retention != nil && (*f.retention < 1 || *f.retention > 120) {
		return errValidation("retention_months must be 1 to 120, or null to keep rollups forever")
	}
	return nil
}

func parseExcludedIPs(in []string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		p, err := netip.ParsePrefix(v)
		if err != nil {
			a, aerr := netip.ParseAddr(v)
			if aerr != nil {
				return nil, errValidation(strconv.Quote(v) + " is not an IP address or CIDR range")
			}
			a = a.Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		}
		p = p.Masked()
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Server) applyPropertyBody(ctx context.Context, q *store.Queries, p principal, f *propertyFields, b oas.AnalyticsPropertyUpdate) error {
	if b.InboxId.IsSpecified() {
		f.inboxID = nil
		if !b.InboxId.IsNull() {
			in, err := ownedInbox(ctx, q, p, b.InboxId.MustGet())
			if err != nil {
				return err
			}
			f.inboxID = &in.ID
		}
	}
	if b.Name != nil {
		f.name = *b.Name
	}
	if b.Domains != nil {
		f.domains = slices.Clone(*b.Domains)
	}
	if b.Platforms != nil {
		f.platforms = nil
		for _, v := range *b.Platforms {
			f.platforms = append(f.platforms, string(v))
		}
	}
	if b.Timezone != nil {
		f.timezone = strings.TrimSpace(*b.Timezone)
	}
	if b.Currency != nil {
		f.currency = *b.Currency
	}
	if b.Collection != nil {
		f.collection = string(*b.Collection)
	}
	if b.Audience != nil {
		f.audience = string(*b.Audience)
	}
	if b.ConsentFeatures != nil {
		f.consentFeatures = []string{}
		for _, v := range *b.ConsentFeatures {
			f.consentFeatures = append(f.consentFeatures, string(v))
		}
	}
	if b.ExcludedPaths != nil {
		f.excludedPaths = slices.Clone(*b.ExcludedPaths)
	}
	if b.ExcludedIps != nil {
		ips, err := parseExcludedIPs(*b.ExcludedIps)
		if err != nil {
			return err
		}
		f.excludedIPs = ips
	}
	if b.PathRules != nil {
		f.pathRules = []analytics.PathRule{}
		for _, r := range *b.PathRules {
			f.pathRules = append(f.pathRules, analytics.PathRule{Pattern: r.Pattern, Replacement: r.Replacement})
		}
	}
	if b.TrackedProps != nil {
		f.trackedProps = slices.Clone(*b.TrackedProps)
	}
	if b.RetentionMonths.IsSpecified() {
		f.retention = nil
		if !b.RetentionMonths.IsNull() {
			f.retention = new(b.RetentionMonths.MustGet())
		}
	}
	return f.validate()
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func (s *Server) ListAnalyticsProperties(ctx context.Context, _ oas.ListAnalyticsPropertiesRequestObject) (oas.ListAnalyticsPropertiesResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	rows, err := s.st.ListAnalyticsProperties(ctx, store.ListAnalyticsPropertiesParams{WorkspaceID: p.workspaceID, AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID()})
	if err != nil {
		return nil, err
	}
	items, err := s.propertyBodies(ctx, p.workspaceID, rows)
	if err != nil {
		return nil, err
	}
	usage, err := s.analyticsUsage(ctx, p.workspaceID)
	if err != nil {
		return nil, err
	}
	return oas.ListAnalyticsProperties200JSONResponse{Items: items, Usage: usage}, nil
}

func (s *Server) CreateAnalyticsProperty(ctx context.Context, req oas.CreateAnalyticsPropertyRequestObject) (oas.CreateAnalyticsPropertyResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireAnalyticsManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	f := propertyFields{
		name: b.Name, timezone: "UTC", currency: "EUR", collection: string(oas.AnalyticsCollectionAnonymous), audience: string(oas.AnalyticsAudienceGeneral),
		platforms: []string{string(oas.AnalyticsPlatformWeb)}, consentFeatures: []string{}, excludedPaths: []string{}, excludedIPs: []netip.Prefix{},
		pathRules: []analytics.PathRule{}, trackedProps: []string{}, retention: new(int32(defaultRetentionMonths)),
	}
	var out store.AnalyticsProperty
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		if b.InboxId != nil {
			in, err := ownedInbox(ctx, q, p, *b.InboxId)
			if err != nil {
				return err
			}
			f.inboxID, f.timezone = &in.ID, in.Timezone
		}
		upd := oas.AnalyticsPropertyUpdate{
			Domains: &b.Domains, Platforms: b.Platforms, Timezone: b.Timezone, Currency: b.Currency, Collection: b.Collection,
			Audience: b.Audience, ConsentFeatures: b.ConsentFeatures, ExcludedPaths: b.ExcludedPaths, ExcludedIps: b.ExcludedIps,
			PathRules: b.PathRules, TrackedProps: b.TrackedProps, RetentionMonths: b.RetentionMonths,
		}
		if err := s.applyPropertyBody(ctx, q, p, &f, upd); err != nil {
			return err
		}
		if _, err := q.LockWorkspace(ctx, p.workspaceID); err != nil {
			return err
		}
		n, err := q.CountAnalyticsProperties(ctx, p.workspaceID)
		if err != nil {
			return err
		}
		if n >= int64(s.analytics.settings.PropertiesPerWorkspace) {
			return errPropertyLimit
		}
		id := newID()
		var createdBy *uuid.UUID
		if !p.isKey() && p.memberID != uuid.Nil() {
			createdBy = &p.memberID
		}
		out, err = q.CreateAnalyticsProperty(ctx, store.CreateAnalyticsPropertyParams{
			ID: id, WorkspaceID: p.workspaceID, InboxID: f.inboxID, Name: f.name, PublicKey: newAnalyticsKey(), Domains: f.domains,
			Platforms: f.platforms, Timezone: f.timezone, Currency: f.currency, Collection: f.collection, ConsentFeatures: nonNil(f.consentFeatures),
			Audience: f.audience, ExcludedPaths: nonNil(f.excludedPaths), ExcludedIps: nonNil(f.excludedIPs), PathRules: mustJSON(nonNil(f.pathRules)),
			TrackedProps: nonNil(f.trackedProps), RetentionMonths: f.retention, VisitorKey: s.secrets.Seal(analytics.NewSalt(), analyticsSecretContext(p.workspaceID, id)),
			CreatedBy: createdBy, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateAnalyticsProperty201JSONResponse(s.propertyBody(out, 0)), nil
}

func analyticsSecretContext(workspaceID, propertyID uuid.UUID) []byte {
	return append(append([]byte("analytics-visitor-key:"), workspaceID[:]...), propertyID[:]...)
}

func newAnalyticsKey() string { return analyticsKeyPrefix + randomToken(18) }

func (s *Server) GetAnalyticsProperty(ctx context.Context, req oas.GetAnalyticsPropertyRequestObject) (oas.GetAnalyticsPropertyResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.propertyBodies(ctx, p.workspaceID, []store.AnalyticsProperty{prop})
	if err != nil {
		return nil, err
	}
	return oas.GetAnalyticsProperty200JSONResponse(out[0]), nil
}

func (s *Server) UpdateAnalyticsProperty(ctx context.Context, req oas.UpdateAnalyticsPropertyRequestObject) (oas.UpdateAnalyticsPropertyResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out store.AnalyticsProperty
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleProperty(ctx, q, p, req.PropertyId, true)
		if err != nil {
			return err
		}
		if err := requireAnalyticsManager(p); err != nil {
			return err
		}
		f := fieldsOf(cur)
		if err := s.applyPropertyBody(ctx, q, p, &f, *req.Body); err != nil {
			return err
		}
		if !slices.Equal(slices.Sorted(slices.Values(cur.TrackedProps)), slices.Sorted(slices.Values(f.trackedProps))) {
			if _, err := q.BumpAnalyticsGoalsVersion(ctx, store.BumpAnalyticsGoalsVersionParams{WorkspaceID: p.workspaceID, ID: cur.ID}); err != nil {
				return err
			}
		}
		out, err = q.UpdateAnalyticsProperty(ctx, store.UpdateAnalyticsPropertyParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, InboxID: f.inboxID, Name: f.name, Domains: f.domains, Platforms: f.platforms,
			Timezone: f.timezone, Currency: f.currency, Collection: f.collection, ConsentFeatures: nonNil(f.consentFeatures), Audience: f.audience,
			ExcludedPaths: nonNil(f.excludedPaths), ExcludedIps: nonNil(f.excludedIPs), PathRules: mustJSON(nonNil(f.pathRules)),
			TrackedProps: nonNil(f.trackedProps), RetentionMonths: f.retention, Now: s.now(),
		})
		if err != nil {
			return err
		}
		return notifyAnalyticsChanged(ctx, q, p.workspaceID, &out.ID)
	})
	if err != nil {
		return nil, err
	}
	s.analytics.props.forget(out.ID)
	body, err := s.propertyBodies(ctx, p.workspaceID, []store.AnalyticsProperty{out})
	if err != nil {
		return nil, err
	}
	return oas.UpdateAnalyticsProperty200JSONResponse(body[0]), nil
}

func (s *Server) RotateAnalyticsKey(ctx context.Context, req oas.RotateAnalyticsKeyRequestObject) (oas.RotateAnalyticsKeyResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var out store.AnalyticsProperty
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		cur, err := visibleProperty(ctx, q, p, req.PropertyId, true)
		if err != nil {
			return err
		}
		if err := requireAnalyticsManager(p); err != nil {
			return err
		}
		now := s.now()
		out, err = q.RotateAnalyticsKey(ctx, store.RotateAnalyticsKeyParams{
			WorkspaceID: p.workspaceID, ID: cur.ID, PublicKey: newAnalyticsKey(), PreviousUntil: new(now.Add(analyticsKeyOverlap)), Now: now,
		})
		if err != nil {
			return err
		}
		today := analytics.LocalDay(now, propertyLocation(out.Timezone))
		if _, err := q.CreateAnalyticsAnnotation(ctx, store.CreateAnalyticsAnnotationParams{ID: newID(), WorkspaceID: p.workspaceID, PropertyID: out.ID,
			Kind: annotationKeyRotated, StartsOn: today, EndsOn: today, Now: now}); err != nil {
			return err
		}
		return notifyAnalyticsChanged(ctx, q, p.workspaceID, &out.ID)
	})
	if err != nil {
		return nil, err
	}
	s.analytics.props.forget(out.ID)
	body, err := s.propertyBodies(ctx, p.workspaceID, []store.AnalyticsProperty{out})
	if err != nil {
		return nil, err
	}
	return oas.RotateAnalyticsKey200JSONResponse(body[0]), nil
}

func (s *Server) DeleteAnalyticsProperty(ctx context.Context, req oas.DeleteAnalyticsPropertyRequestObject) (oas.DeleteAnalyticsPropertyResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	var id uuid.UUID
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		cur, err := visibleProperty(ctx, q, p, req.PropertyId, true)
		if err != nil {
			return err
		}
		if err := requireAnalyticsManager(p); err != nil {
			return err
		}
		if req.Body.Name != cur.Name {
			return errPropertyConfirm
		}
		if _, err := q.MarkAnalyticsPropertyDeleted(ctx, store.MarkAnalyticsPropertyDeletedParams{WorkspaceID: p.workspaceID, ID: cur.ID, Now: new(s.now())}); err != nil {
			return err
		}
		id = cur.ID
		if err := notifyAnalyticsChanged(ctx, q, p.workspaceID, &id); err != nil {
			return err
		}
		_, err = s.jobs.InsertTx(ctx, tx, AnalyticsDeleteArgs{WorkspaceID: p.workspaceID, PropertyID: cur.ID, Property: true},
			&river.InsertOpts{ScheduledAt: s.now().Add(analyticsDeleteDelay)})
		return err
	})
	if err != nil {
		return nil, err
	}
	s.analytics.forget(id)
	return oas.DeleteAnalyticsProperty202Response{}, nil
}

func (s *Server) DeleteAnalyticsData(ctx context.Context, req oas.DeleteAnalyticsDataRequestObject) (oas.DeleteAnalyticsDataResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	args := AnalyticsDeleteArgs{WorkspaceID: p.workspaceID}
	switch {
	case b.All != nil && *b.All:
		if b.From != nil || b.To != nil {
			return nil, errValidation("send either from and to, or all: true")
		}
		args.All = true
	case b.From != nil && b.To != nil:
		from, to := b.From.Time, b.To.Time
		if to.Before(from) {
			return nil, errValidation("to must not be before from")
		}
		if to.Sub(from) > maxAnalyticsDeleteRange*24*time.Hour {
			return nil, errValidation("from and to may be at most 400 days apart")
		}
		args.From, args.To = from.Format(time.DateOnly), to.Format(time.DateOnly)
	default:
		return nil, errValidation("send from and to, or all: true")
	}
	err := s.st.InTxRaw(ctx, func(tx pgx.Tx, q *store.Queries) error {
		cur, err := visibleProperty(ctx, q, p, req.PropertyId, true)
		if err != nil {
			return err
		}
		if err := requireAnalyticsManager(p); err != nil {
			return err
		}
		args.PropertyID = cur.ID
		_, err = s.jobs.InsertTx(ctx, tx, args, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.DeleteAnalyticsData202Response{}, nil
}

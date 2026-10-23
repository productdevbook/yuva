package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	maxEventAge     = 72 * time.Hour
	minProcessKey   = 16
	maxProcessKey   = 128
	maxVisitorKey   = 200
	maxForwardedUA  = 1000
	maxForwardedHdr = 500
	maxAppVersion   = 50
	screenEvent     = "screen"
)

var (
	errPlatformNotAllowed = problem(http.StatusForbidden, "platform_not_allowed", "the app's platform is not one of the property's platforms")
	errAnalyticsNeedsKey  = problem(http.StatusForbidden, "forbidden", "only an API key can send server-side analytics events")
	appPlatforms          = map[string]analytics.Platform{"ios": analytics.PlatformIOS, "android": analytics.PlatformAndroid}
	appSystems            = map[string]string{"ios": "iOS", "android": "Android"}
	appDevices            = map[string]int16{"mobile": analytics.DeviceMobile, "tablet": analytics.DeviceTablet, "desktop": analytics.DeviceDesktop}
)

func (c *cachedProperty) apps() bool {
	return slices.Contains(c.prop.Platforms, string(oas.AnalyticsPlatformIos)) || slices.Contains(c.prop.Platforms, string(oas.AnalyticsPlatformAndroid))
}

type appInfo struct {
	Platform  string `json:"platform"`
	Version   string `json:"version"`
	OSVersion string `json:"os_version"`
	Device    string `json:"device"`
	Width     int    `json:"width"`
	Language  string `json:"language"`
}

// eventTime is when an event happened: at, or now when at is zero or ahead of now. late is true for
// an event older than 72 hours.
func eventTime(at, now time.Time) (time.Time, bool) {
	if at.IsZero() || at.After(now) {
		return now, false
	}
	return at, now.Sub(at) > maxEventAge
}

func majorVersion(v string) int16 {
	end := strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(v)
	}
	n, err := strconv.Atoi(v[:end])
	if err != nil || n <= 0 || n >= 1<<15 {
		return 0
	}
	return int16(n)
}

// viewOrCustom checks an event that is not an engagement: a page or screen view carries no
// revenue, anything else is a custom or built-in event.
func viewOrCustom(e collectEvent, pg analytics.Page, screen, children bool) (*customEvent, error) {
	if e.N == "pageview" || (screen && e.N == screenEvent) {
		if len(e.Rev) > 0 && string(e.Rev) != "null" {
			return nil, errors.New("only custom and built-in events carry revenue")
		}
		return nil, nil
	}
	ce, err := parseCustomEvent(e, pg, children)
	if err != nil {
		return nil, err
	}
	return &ce, nil
}

// collectApp takes an app's batch, sent without an Origin: the process key stands for the visitor,
// screens are app:// URLs and each event carries its own time. Invalid and late events are dropped
// on their own.
func (s *Server) collectApp(w http.ResponseWriter, r *http.Request, cp *cachedProperty, body []byte, now time.Time, drop func(reason string, n int)) {
	prop := cp.prop
	var head struct {
		K string          `json:"k"`
		A *appInfo        `json:"a"`
		E json.RawMessage `json:"e"`
	}
	refuse := func(err *apiError) {
		drop(analytics.ReasonInvalid, 1)
		writeProblem(w, err)
	}
	if json.Unmarshal(body, &head) != nil || head.E == nil {
		refuse(errValidation(`apps send {"k": ..., "a": ..., "e": [...]}`))
		return
	}
	if n := len(head.K); n < minProcessKey || n > maxProcessKey {
		refuse(errValidation("k must be a process key of 16 to 128 characters"))
		return
	}
	if head.A == nil {
		refuse(errValidation("a is required"))
		return
	}
	app := *head.A
	platform, ok := appPlatforms[app.Platform]
	if !ok {
		refuse(errValidation("a.platform must be ios or android"))
		return
	}
	if !slices.Contains(prop.Platforms, app.Platform) {
		drop(analytics.ReasonOrigin, 1)
		writeProblem(w, errPlatformNotAllowed)
		return
	}
	evs, _, err := parseAnalyticsBatch(body)
	if err != nil {
		refuse(errValidation(err.Error()))
		return
	}
	children := prop.Audience == string(oas.AnalyticsAudienceChildren)
	consent := prop.Collection == string(oas.AnalyticsCollectionConsent)
	items := make([]analyticsItem, 0, len(evs))
	var changes []collectEvent
	for _, e := range evs {
		pg, ok := analytics.ParseScreen(e.U)
		if !ok {
			drop(analytics.ReasonInvalid, 1)
			continue
		}
		if e.N == consentEvent {
			changes = append(changes, e)
			continue
		}
		var at time.Time
		if e.T > 0 {
			at = time.UnixMilli(e.T)
		}
		at, late := eventTime(at, now)
		if late {
			drop(analytics.ReasonLate, 1)
			continue
		}
		var ce *customEvent
		if e.N != "engagement" {
			if ce, err = viewOrCustom(e, pg, true, children); err != nil {
				drop(analytics.ReasonInvalid, 1)
				continue
			}
		}
		if e.C != nil && e.C.J != "" {
			changes = append(changes, e)
		}
		if consent && (e.C == nil || e.C.A != 1) {
			continue
		}
		items = append(items, analyticsItem{e: e, page: pg, custom: ce, at: at})
	}
	if len(changes) > 0 {
		if err := s.takeConsentChanges(r.Context(), r, cp, changes, now); err != nil {
			writeProblem(w, err)
			return
		}
	}
	if len(items) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	slices.SortStableFunc(items, func(a, b analyticsItem) int { return a.at.Compare(b.at) })
	short, ok := s.admitAnalytics(w, r, cp, now, len(items), drop)
	if !ok {
		return
	}
	language := ""
	if !children {
		if language = analytics.Language(app.Language); language == "" {
			language = analytics.Language(r.Header.Get("Accept-Language"))
		}
	}
	agent := analytics.Agent{OS: appSystems[app.Platform], OSMajor: majorVersion(app.OSVersion), Device: appDevices[app.Device]}
	version := analytics.Clean(app.Version, maxAppVersion)
	key := head.K
	visitor := func(salt []byte) int64 { return analytics.KeyVisitorHash(salt, prop.ID, analytics.ScopeProcess, key) }
	describe := func(pg analytics.Page, refHost, refPath string) *analytics.VisitInfo {
		info := s.visitInfo(r, prop, short, agent, pg, refHost, refPath, app.Width, language)
		info.Platform, info.AppVersion = platform, version
		return info
	}
	for i := range items {
		items[i].visitor, items[i].describe = visitor, describe
		s.consentedVisitor(r.Context(), cp, items[i].e.C, &items[i])
	}
	if err := s.takeAnalytics(r.Context(), cp, now, items, drop); err != nil {
		writeProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// serverEvent is a server-side event in the collection endpoint's shape.
func serverEvent(se oas.AnalyticsServerEvent) (collectEvent, error) {
	e := collectEvent{N: se.Name, U: se.Url, R: derefString(se.Referrer), I: se.Interactive}
	if se.Props != nil {
		raw, err := json.Marshal(se.Props)
		if err != nil {
			return e, errors.New("props must be an object of strings, numbers and booleans")
		}
		e.P = raw
	}
	if se.Revenue != nil {
		raw, err := json.Marshal(se.Revenue)
		if err != nil {
			return e, errors.New("revenue does not match AnalyticsEventRevenue")
		}
		e.Rev = raw
	}
	return e, nil
}

// serverVisitor is how a server-side event names its visitor: the forwarded address and user agent
// hashed as the collection endpoint hashes a browser's request, or the backend's own key.
type serverVisitor struct {
	short   analytics.ShortIP
	hasIP   bool
	excl    bool
	ua      string
	agent   analytics.Agent
	visitor func(salt []byte) int64
}

func readServerVisitor(se oas.AnalyticsServerEvent, propertyID uuid.UUID, excluded []netip.Prefix) (serverVisitor, error) {
	var out serverVisitor
	ua, platformHint, mobileHint, key := derefString(se.UserAgent), derefString(se.SecChUaPlatform), derefString(se.SecChUaMobile), derefString(se.VisitorKey)
	switch {
	case len(ua) > maxForwardedUA:
		return out, errors.New("user_agent has at most 1,000 characters")
	case len(platformHint) > 100 || len(mobileHint) > 10 || len(derefString(se.AcceptLanguage)) > maxForwardedHdr:
		return out, errors.New("sec_ch_ua_platform, sec_ch_ua_mobile or accept_language is too long")
	case se.VisitorKey != nil && (strings.TrimSpace(key) == "" || len(key) > maxVisitorKey):
		return out, errors.New("visitor_key must be 1 to 200 characters")
	case se.VisitorKey == nil && (derefString(se.Ip) == "" || strings.TrimSpace(ua) == ""):
		return out, errors.New("give ip and user_agent, or visitor_key")
	}
	if raw := derefString(se.Ip); raw != "" {
		addr, err := netip.ParseAddr(strings.TrimSpace(raw))
		if err != nil {
			return out, errors.New("ip is not an IP address")
		}
		addr = addr.Unmap()
		out.excl = slices.ContainsFunc(excluded, func(p netip.Prefix) bool { return p.Contains(addr) })
		out.short, out.hasIP = analytics.ShortenAddr(addr), true
	}
	if ua != "" {
		out.agent = analytics.ParseAgent(ua, platformHint, mobileHint)
	}
	if se.VisitorKey != nil {
		out.visitor = func(salt []byte) int64 {
			return analytics.KeyVisitorHash(salt, propertyID, analytics.ScopeVisitor, key)
		}
		return out, nil
	}
	short, hashed := out.short, ua+"\x00"+platformHint+"\x00"+mobileHint
	out.visitor = func(salt []byte) int64 { return analytics.VisitorHash(salt, propertyID, short, hashed) }
	return out, nil
}

func (s *Server) SendAnalyticsEvents(ctx context.Context, req oas.SendAnalyticsEventsRequestObject) (oas.SendAnalyticsEventsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if !p.isKey() {
		return nil, errAnalyticsNeedsKey
	}
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	if req.Body == nil || len(req.Body.Events) == 0 || len(req.Body.Events) > maxAnalyticsBatch {
		return nil, errValidation("events must hold 1 to 100 events")
	}
	a, now := s.analytics, s.now()
	cp := &cachedProperty{prop: prop, loc: propertyLocation(prop.Timezone), rules: pathRules(prop.PathRules)}
	day := analytics.LocalDay(now, cp.loc)
	drop := func(reason string, n int) { a.buffer.Drop(prop.WorkspaceID, prop.ID, day, reason, int64(n)) }
	out := oas.AnalyticsEventsResult{Errors: []oas.AnalyticsEventError{}}
	children := prop.Audience == string(oas.AnalyticsAudienceChildren)
	consent := prop.Collection == string(oas.AnalyticsCollectionConsent)
	items := make([]analyticsItem, 0, len(req.Body.Events))
	var changes []collectEvent
	for i, se := range req.Body.Events {
		item, reason, err := s.serverItem(ctx, cp, se, now, children, consent)
		if err != nil {
			drop(analytics.ReasonInvalid, 1)
			out.Errors = append(out.Errors, oas.AnalyticsEventError{Index: i, Detail: err.Error()})
			continue
		}
		out.Accepted++
		if reason != notCollected && se.IdentityToken != nil && se.PersistentId != nil && se.Consent != nil && *se.Consent {
			changes = append(changes, collectEvent{N: consentEvent, C: &collectConsent{A: 1, P: *se.PersistentId, J: *se.IdentityToken}})
		}
		switch reason {
		case "":
			items = append(items, item)
		case notCollected:
		default:
			drop(reason, 1)
		}
	}
	if items, err = s.rateServerItems(ctx, prop, items, now, drop); err != nil {
		return nil, err
	}
	if len(changes) > 0 {
		if err := s.takeConsentChanges(ctx, nil, cp, changes, now); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(items, func(a, b analyticsItem) int { return a.at.Compare(b.at) })
	if len(items) > 0 {
		ok, err := s.takeUsage(ctx, prop.WorkspaceID, len(items), now)
		if err != nil {
			return nil, err
		}
		if !ok {
			drop(analytics.ReasonOverLimit, len(items))
			return oas.SendAnalyticsEvents202JSONResponse(out), nil
		}
		if err := s.takeAnalytics(ctx, cp, now, items, drop); err != nil {
			return nil, err
		}
	}
	return oas.SendAnalyticsEvents202JSONResponse(out), nil
}

// rateServerItems counts a batch's events against their addresses' limits and then the property's,
// one statement each, and drops in order the events past them.
func (s *Server) rateServerItems(ctx context.Context, prop store.AnalyticsProperty, items []analyticsItem, now time.Time, drop func(reason string, n int)) ([]analyticsItem, error) {
	var takes []rateTake
	at := map[string]int{}
	for _, it := range items {
		if it.address == "" {
			continue
		}
		i, ok := at[it.address]
		if !ok {
			i, at[it.address] = len(takes), len(takes)
			takes = append(takes, rateTake{it.address, limitAnalyticsPerAddress, 0})
		}
		takes[i].n++
	}
	if len(takes) > 0 {
		g, err := s.limits.take(ctx, now, takes...)
		if err != nil {
			return nil, err
		}
		kept := make([]analyticsItem, 0, len(items))
		for _, it := range items {
			if it.address != "" {
				i := at[it.address]
				if g[i].n == 0 {
					drop(analytics.ReasonRate, 1)
					continue
				}
				g[i].n--
			}
			kept = append(kept, it)
		}
		items = kept
	}
	if len(items) == 0 {
		return items, nil
	}
	g, err := s.limits.take(ctx, now, rateTake{rateKeyAnalyticsProperty(prop.ID), limit{s.analytics.settings.PropertyEventsPerMinute, time.Minute}, len(items)})
	if err != nil {
		return nil, err
	}
	if n := g[0].n; n < len(items) {
		drop(analytics.ReasonRate, len(items)-n)
		items = items[:n]
	}
	return items, nil
}

// notCollected marks a valid event that is not collected and not counted: DNT, GPC, no consent.
const notCollected = "-"

// serverItem checks one server-side event; reason names why a valid event is dropped before it
// reaches its visit (late, excluded, bot, or notCollected).
func (s *Server) serverItem(ctx context.Context, cp *cachedProperty, se oas.AnalyticsServerEvent, now time.Time, children, consent bool) (analyticsItem, string, error) {
	prop := cp.prop
	e, err := serverEvent(se)
	if err != nil {
		return analyticsItem{}, "", err
	}
	screen := strings.HasPrefix(strings.TrimSpace(se.Url), "app://")
	var pg analytics.Page
	switch {
	case screen:
		if pg, screen = analytics.ParseScreen(se.Url); !screen {
			return analyticsItem{}, "", errors.New("url must be app://<app id>/<screen> of at most 2,000 characters")
		}
		if !cp.apps() {
			return analyticsItem{}, "", errors.New("app screens need a property with the ios or android platform")
		}
	default:
		var ok bool
		if pg, ok = analytics.ParsePage(se.Url, false); !ok {
			return analyticsItem{}, "", errors.New("url must be an http(s) URL of at most 2,000 characters")
		}
		if !analytics.DomainAllowed(prop.Domains, matchHost(pg.Host)) {
			return analyticsItem{}, "", errors.New("the url's host is not one of the property's domains")
		}
	}
	if e.N == "engagement" {
		return analyticsItem{}, "", errors.New("engagement is sent by browsers only")
	}
	ce, err := viewOrCustom(e, pg, screen, children)
	if err != nil {
		return analyticsItem{}, "", err
	}
	v, err := readServerVisitor(se, prop.ID, prop.ExcludedIps)
	if err != nil {
		return analyticsItem{}, "", err
	}
	var at time.Time
	if se.Timestamp != nil {
		at = *se.Timestamp
	}
	at, late := eventTime(at, now)
	item := analyticsItem{e: e, page: pg, custom: ce, at: at, visitor: v.visitor}
	if se.PersistentId != nil && se.Consent != nil && *se.Consent {
		s.consentedVisitor(ctx, cp, &collectConsent{A: 1, P: *se.PersistentId}, &item)
	}
	switch {
	case (se.Dnt != nil && *se.Dnt) || (se.Gpc != nil && *se.Gpc) || (consent && (se.Consent == nil || !*se.Consent)):
		return item, notCollected, nil
	case late:
		return item, analytics.ReasonLate, nil
	case v.excl:
		return item, analytics.ReasonExcluded, nil
	case v.agent.Bot:
		return item, analytics.ReasonBot, nil
	}
	if v.hasIP {
		item.address = rateKeyAnalyticsAddress(prop.ID, v.short)
	}
	language := ""
	if !children {
		language = analytics.Language(derefString(se.AcceptLanguage))
	}
	short := v.short
	if !v.hasIP {
		short = analytics.ShortIP{}
	}
	agent := v.agent
	item.describe = func(pg analytics.Page, refHost, refPath string) *analytics.VisitInfo {
		info := s.visitInfo(nil, prop, short, agent, pg, refHost, refPath, 0, language)
		info.Platform = analytics.PlatformServer
		return info
	}
	return item, "", nil
}

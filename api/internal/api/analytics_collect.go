package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	analyticsPrefix          = "/client/v1/analytics/"
	maxAnalyticsBody         = 64 << 10
	maxAnalyticsBatch        = 100
	maxEngagedMS             = 30 * 60 * 1000
	propertyCacheTTL         = time.Minute
	maxCachedKeys            = 10_000
	usageCacheTTL            = 30 * time.Second
	analyticsBufferRetry     = 5
	defaultAnalyticsBuffer   = 50_000
	defaultVisitCache        = 200_000
	defaultSaltGrace         = 2 * time.Hour
	defaultPropertyPerMinute = 60_000
	defaultPropertiesPerWS   = 50
	defaultRawDays           = 90
	defaultRollupTop         = 1000
	noticeAnalyticsLimit     = "analytics_limit"
	analyticsPropertyKey     = ctxKey(200)
)

var (
	limitAnalyticsPerAddress = limit{120, time.Minute}
	errPageNotAllowed        = problem(http.StatusForbidden, "page_not_allowed", "the page's host is not one of the property's domains")
	errPropertyOrigin        = problem(http.StatusForbidden, "origin_not_allowed", "this origin is not one of the property's domains")
	errAnalyticsBodyTooLarge = problem(http.StatusRequestEntityTooLarge, "payload_too_large", "send at most 64 KiB")
	errAnalyticsBusy         = &apiError{Status: http.StatusServiceUnavailable, Code: "unavailable", Detail: "the server takes no more events right now; try again shortly", RetryAfter: analyticsBufferRetry}
)

// AnalyticsSettings: zero values mean the defaults; Off removes analytics from the server.
type AnalyticsSettings struct {
	Off                     bool
	Buffer                  int
	VisitCache              int
	SaltGrace               time.Duration
	MonthlyEvents           int64
	PropertyEventsPerMinute int
	PropertiesPerWorkspace  int
	// RawDays is how long raw visits and events are kept; RollupTop how many values of a dimension a
	// day's rollup keeps before the rest is (other).
	RawDays   int
	RollupTop int
	// GeoIP is dbip, file or off (the default when empty); GeoIPDir holds the DB-IP downloads and
	// GeoIPFile is the operator's MMDB file.
	GeoIP     string
	GeoIPDir  string
	GeoIPFile string
	// GeoIPURL replaces DB-IP's download address, for tests.
	GeoIPURL func(month time.Time) string
}

func analyticsSettings(a AnalyticsSettings) AnalyticsSettings {
	if a.Buffer <= 0 {
		a.Buffer = defaultAnalyticsBuffer
	}
	if a.VisitCache <= 0 {
		a.VisitCache = defaultVisitCache
	}
	if a.SaltGrace <= 0 {
		a.SaltGrace = defaultSaltGrace
	}
	if a.PropertyEventsPerMinute <= 0 {
		a.PropertyEventsPerMinute = defaultPropertyPerMinute
	}
	if a.PropertiesPerWorkspace <= 0 {
		a.PropertiesPerWorkspace = defaultPropertiesPerWS
	}
	if a.RawDays <= 0 {
		a.RawDays = defaultRawDays
	}
	if a.RollupTop <= 0 {
		a.RollupTop = defaultRollupTop
	}
	if a.GeoIP == "" {
		a.GeoIP = analytics.GeoOff
	}
	return a
}

type analyticsState struct {
	settings    AnalyticsSettings
	geo         *analytics.GeoIP
	geoUpdating atomic.Bool
	answers     answerCache
	slots       querySlots
	store       analytics.Store
	buffer      *analytics.Buffer
	flusher     *analytics.Flusher
	resolver    *analytics.Resolver
	props       *propertyCache
	usage       *usageCache
}

func (s *Server) newAnalytics(d Deps) *analyticsState {
	set := analyticsSettings(d.Analytics)
	st := &analytics.PGStore{Pool: d.Store.Pool}
	a := &analyticsState{
		settings: set, store: st, buffer: analytics.NewBuffer(set.Buffer), props: newPropertyCache(),
		usage: &usageCache{m: map[uuid.UUID]*usageEntry{}}, slots: querySlots{limits: s.limits},
		resolver: &analytics.Resolver{Store: st, Salts: analytics.NewSalts(st), Visits: analytics.NewVisits(set.VisitCache), Grace: set.SaltGrace},
	}
	if set.GeoIP != analytics.GeoOff {
		a.geo = &analytics.GeoIP{Mode: set.GeoIP, Dir: set.GeoIPDir, File: set.GeoIPFile, URL: set.GeoIPURL, Now: s.now}
	}
	a.flusher = &analytics.Flusher{Store: st, Buffer: a.buffer, Log: s.log, Now: s.now, AfterFlush: s.afterAnalyticsFlush}
	if d.Hub != nil {
		d.Hub.OnInvalidate(a.invalidated)
	}
	return a
}

func (a *analyticsState) forget(propertyID uuid.UUID) {
	a.props.forget(propertyID)
	a.answers.forget(propertyID)
	a.resolver.Salts.Forget(propertyID)
	a.resolver.Visits.Forget(propertyID)
}

const analyticsInvalidation = "analytics:"

// notifyAnalyticsChanged tells every process, once the transaction commits, to drop what it cached
// of a property, or of every property of the workspace when propertyID is nil, so a deleted
// property's key stops answering everywhere at once.
func notifyAnalyticsChanged(ctx context.Context, q *store.Queries, ws uuid.UUID, propertyID *uuid.UUID) error {
	target := "*"
	if propertyID != nil {
		target = propertyID.String()
	}
	return q.NotifySignal(ctx, store.NotifySignalParams{Channel: realtime.InvalidateChannel, Payload: analyticsInvalidation + ws.String() + ":" + target})
}

func (a *analyticsState) invalidated(payload string) {
	switch payload {
	case realtime.InvalidateAll:
		a.props.forgetAll()
		return
	case analyticsGeoIPNotice:
		if a.geo != nil && a.geoUpdating.CompareAndSwap(false, true) {
			go func() {
				defer a.geoUpdating.Store(false)
				ctx, cancel := context.WithTimeout(context.Background(), geoipUpdateTimeout)
				defer cancel()
				_, _ = a.geo.Update(ctx)
			}()
		}
		return
	}
	rest, ok := strings.CutPrefix(payload, analyticsInvalidation)
	if !ok {
		return
	}
	ws, target, _ := strings.Cut(rest, ":")
	if target == "*" {
		if id, err := uuid.Parse(ws); err == nil {
			a.props.forgetWorkspace(id)
		}
		return
	}
	if id, err := uuid.Parse(target); err == nil {
		a.forget(id)
	}
}

// RunAnalytics writes collected events until ctx ends and then drains what is left; it returns at
// once when analytics is off.
func (s *Server) RunAnalytics(ctx context.Context) {
	if s.analytics == nil {
		return
	}
	if err := s.analytics.flusher.Prepare(ctx); err != nil {
		s.log.ErrorContext(ctx, "analytics partitions", slog.Any("error", err))
	}
	if g := s.analytics.geo; g != nil {
		_ = g.Load()
		if s.analytics.geoUpdating.CompareAndSwap(false, true) {
			go func() {
				defer s.analytics.geoUpdating.Store(false)
				_, _ = s.UpdateGeoIP(context.WithoutCancel(ctx))
			}()
		}
	}
	s.analytics.flusher.Run(ctx)
}

// FlushAnalytics writes the collected events now.
func (s *Server) FlushAnalytics(ctx context.Context) error {
	if s.analytics == nil {
		return nil
	}
	return s.analytics.flusher.Flush(ctx)
}

type cachedProperty struct {
	prop  store.AnalyticsProperty
	loc   *time.Location
	rules []analytics.PathRule
	at    time.Time
	// key is the property's visitor key, opened the first time a consented visitor needs it.
	keyOnce sync.Once
	key     []byte
	keyErr  error
}

// propertyCache finds properties by public key for a minute. An unknown key is remembered too, apart
// from the known ones, so a flood of made-up keys neither reaches the database on every request nor
// pushes a real property out; a key that cannot be one is never looked up.
type propertyCache struct {
	mu     sync.Mutex
	m      map[string]*cachedProperty
	misses map[string]time.Time
	gen    uint64
}

func newPropertyCache() *propertyCache {
	return &propertyCache{m: map[string]*cachedProperty{}, misses: map[string]time.Time{}}
}

func fresh(at, now time.Time) bool {
	return now.Sub(at) < propertyCacheTTL && now.After(at.Add(-time.Second))
}

func analyticsKeyShape(key string) bool {
	rest, ok := strings.CutPrefix(key, analyticsKeyPrefix)
	if !ok || len(rest) != 24 {
		return false
	}
	for _, r := range rest {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (c *propertyCache) get(ctx context.Context, st *store.Store, key string, now time.Time) (*cachedProperty, error) {
	if !analyticsKeyShape(key) {
		return nil, nil
	}
	c.mu.Lock()
	e, ok := c.m[key]
	missed, miss := c.misses[key]
	gen := c.gen
	c.mu.Unlock()
	if ok && fresh(e.at, now) {
		return e, nil
	}
	if miss && fresh(missed, now) {
		return nil, nil
	}
	e = &cachedProperty{at: now}
	prop, err := st.FindAnalyticsPropertyByKey(ctx, store.FindAnalyticsPropertyByKeyParams{Key: key, Now: new(now)})
	switch {
	case store.IsNotFound(err):
	case err != nil:
		return nil, err
	default:
		e.prop, e.loc, e.rules = prop, propertyLocation(prop.Timezone), pathRules(prop.PathRules)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return e.found()
	}
	if e.prop.ID == uuid.Nil() {
		if len(c.misses) >= maxCachedKeys {
			for k, at := range c.misses {
				if !fresh(at, now) {
					delete(c.misses, k)
				}
			}
			if len(c.misses) >= maxCachedKeys {
				c.misses = map[string]time.Time{}
			}
		}
		c.misses[key] = now
		return nil, nil
	}
	delete(c.misses, key)
	if len(c.m) >= maxCachedKeys {
		for k, old := range c.m {
			if !fresh(old.at, now) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxCachedKeys {
			c.m = map[string]*cachedProperty{}
		}
	}
	c.m[key] = e
	return e, nil
}

func (e *cachedProperty) found() (*cachedProperty, error) {
	if e.prop.ID == uuid.Nil() {
		return nil, nil
	}
	return e, nil
}

func (c *propertyCache) forgetAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.m, c.misses = map[string]*cachedProperty{}, map[string]time.Time{}
}

func (c *propertyCache) forgetWorkspace(ws uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for k, e := range c.m {
		if e.prop.WorkspaceID == ws {
			delete(c.m, k)
		}
	}
}

func (c *propertyCache) forget(propertyID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for k, e := range c.m {
		if e.prop.ID == propertyID {
			delete(c.m, k)
		}
	}
}

type usageEntry struct {
	month   time.Time
	limit   int64
	events  int64
	reached bool
	loaded  time.Time
}

// usageCache knows each workspace's events this month and its cap, read again every 30 seconds
// and moved by this process's own events in between.
type usageCache struct {
	mu sync.Mutex
	m  map[uuid.UUID]*usageEntry
}

func (s *Server) workspaceAnalyticsLimit(override *int64) int64 {
	if override != nil {
		return *override
	}
	return s.analytics.settings.MonthlyEvents
}

func (s *Server) loadUsage(ctx context.Context, ws uuid.UUID, now time.Time) (*usageEntry, error) {
	month := monthOf(now)
	c := s.analytics.usage
	c.mu.Lock()
	e, ok := c.m[ws]
	c.mu.Unlock()
	if ok && e.month.Equal(month) && now.Sub(e.loaded) < usageCacheTTL && !now.Before(e.loaded) {
		return e, nil
	}
	u, err := s.st.GetAnalyticsUsage(ctx, store.GetAnalyticsUsageParams{WorkspaceID: ws, Month: month})
	if err != nil {
		return nil, err
	}
	e = &usageEntry{month: month, limit: s.workspaceAnalyticsLimit(u.MonthlyLimit), events: u.Events, reached: u.LimitReachedAt != nil, loaded: now}
	c.mu.Lock()
	c.m[ws] = e
	c.mu.Unlock()
	return e, nil
}

// takeUsage counts n events against the workspace's monthly cap; false means the cap is reached.
func (s *Server) takeUsage(ctx context.Context, ws uuid.UUID, n int, now time.Time) (bool, error) {
	e, err := s.loadUsage(ctx, ws, now)
	if err != nil {
		return false, err
	}
	c := s.analytics.usage
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.limit > 0 && e.events+int64(n) > e.limit {
		return false, nil
	}
	e.events += int64(n)
	return true, nil
}

func (s *Server) analyticsUsage(ctx context.Context, ws uuid.UUID) (oas.AnalyticsUsage, error) {
	month := monthOf(s.now())
	u, err := s.st.GetAnalyticsUsage(ctx, store.GetAnalyticsUsageParams{WorkspaceID: ws, Month: month})
	if err != nil {
		return oas.AnalyticsUsage{}, err
	}
	out := oas.AnalyticsUsage{Month: month.Format("2006-01"), Events: u.Events}
	if l := s.workspaceAnalyticsLimit(u.MonthlyLimit); l > 0 {
		out.Limit = &l
		if u.Events >= l {
			out.LimitReachedAt = u.LimitReachedAt
		}
	}
	return out, nil
}

// afterAnalyticsFlush tells owners and the operator, once a month, that a workspace reached its cap.
func (s *Server) afterAnalyticsFlush(ctx context.Context, res analytics.IngestResult) {
	now := s.now()
	for ws, total := range res.Usage {
		e, err := s.loadUsage(ctx, ws, now)
		if err != nil {
			s.log.ErrorContext(ctx, "analytics usage", slog.Any("error", err))
			continue
		}
		c := s.analytics.usage
		c.mu.Lock()
		if e.month.Equal(res.Month) && total > e.events {
			e.events = total
		}
		limit, reached := e.limit, e.reached
		c.mu.Unlock()
		if limit <= 0 || total < limit || reached {
			continue
		}
		if err := s.noteAnalyticsLimit(ctx, ws, res.Month, total, limit); err != nil {
			s.log.ErrorContext(ctx, "analytics limit notice", slog.Any("error", err))
		}
	}
}

func (s *Server) noteAnalyticsLimit(ctx context.Context, ws uuid.UUID, month time.Time, total, limit int64) error {
	var w store.Workspace
	var owners []store.ListOwnerAddressesRow
	first := false
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		n, err := q.MarkAnalyticsLimitReached(ctx, store.MarkAnalyticsLimitReachedParams{WorkspaceID: ws, Month: month, Now: new(s.now())})
		if err != nil || n == 0 {
			return err
		}
		first = true
		if w, err = q.GetWorkspace(ctx, ws); err != nil {
			return err
		}
		if owners, err = q.ListOwnerAddresses(ctx, ws); err != nil {
			return err
		}
		if len(s.operator) == 0 {
			return nil
		}
		return q.CreateOperatorNotice(ctx, store.CreateOperatorNoticeParams{
			ID: newID(), Kind: noticeAnalyticsLimit, WorkspaceName: &w.Name, CreatedAt: s.now(),
			Detail: new(formatCount(total) + " analytics events in " + month.Format("January 2006") + "; the limit is " + formatCount(limit)),
		})
	})
	c := s.analytics.usage
	c.mu.Lock()
	if e, ok := c.m[ws]; ok {
		e.reached = true
	}
	c.mu.Unlock()
	if err != nil || !first {
		return err
	}
	for _, o := range owners {
		msg, err := mail.Render("analytics_limit", o.Locale, map[string]any{
			"Workspace": w.Name, "Limit": formatCount(limit), "Month": month.Format("2006-01"), "URL": s.auth.PublicURL + "/analytics",
		})
		if err != nil {
			return err
		}
		msg.To = o.Email
		s.sendMail(ctx, msg)
	}
	return nil
}

func formatCount(n int64) string {
	raw := []byte(strings.TrimPrefix(strconv.FormatInt(n, 10), "-"))
	var out []byte
	for i, c := range raw {
		if i > 0 && (len(raw)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if n < 0 {
		return "-" + string(out)
	}
	return string(out)
}

// analyticsCORS is clientCORS for the collection endpoint: the key in the path names the property,
// and a browser's Origin host must be one of its domains.
func (s *Server) analyticsCORS(w http.ResponseWriter, r *http.Request, next http.Handler) {
	a := s.analytics
	if a == nil {
		writeProblem(w, errNotFound)
		return
	}
	raw := r.Header.Get("Origin")
	if raw != "" {
		w.Header().Add("Vary", "Origin")
	}
	key := strings.TrimPrefix(r.URL.Path, analyticsPrefix)
	if key == "" || strings.Contains(key, "/") || len(key) > 200 {
		writeProblem(w, errNotFound)
		return
	}
	cp, err := a.props.get(r.Context(), s.st, key, s.now())
	if err != nil {
		s.log.ErrorContext(r.Context(), "analytics property", slog.Any("error", err))
		writeProblem(w, errInternal)
		return
	}
	if cp == nil {
		writeProblem(w, errNotFound)
		return
	}
	if raw == "" && r.Method == http.MethodPost && cp.apps() {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), analyticsPropertyKey, cp)))
		return
	}
	host := ""
	if origin, ok := normalizeOrigin(raw); ok {
		if u, err := url.Parse(origin); err == nil {
			host = matchHost(u.Hostname())
		}
	}
	if host == "" || !slices.Contains(cp.prop.Platforms, string(oas.AnalyticsPlatformWeb)) || !analytics.DomainAllowed(cp.prop.Domains, host) {
		if r.Method == http.MethodPost {
			a.buffer.Drop(cp.prop.WorkspaceID, cp.prop.ID, analytics.LocalDay(s.now(), cp.loc), analytics.ReasonOrigin, 1)
		}
		writeProblem(w, errPropertyOrigin)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", raw)
	w.Header().Set("Access-Control-Expose-Headers", "Retry-After")
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), analyticsPropertyKey, cp)))
}

func matchHost(h string) string {
	h = strings.ToLower(h)
	if strings.Contains(h, ":") {
		return "[" + h + "]"
	}
	return h
}

type collectEvent struct {
	N   string          `json:"n"`
	U   string          `json:"u"`
	R   string          `json:"r"`
	W   int             `json:"w"`
	H   bool            `json:"h"`
	MS  int64           `json:"ms"`
	SD  int             `json:"sd"`
	T   int64           `json:"t"`
	P   json.RawMessage `json:"p"`
	Rev json.RawMessage `json:"$"`
	I   *bool           `json:"i"`
	C   *collectConsent `json:"c"`
}

// collectConsent is an event's AnalyticsConsent.
type collectConsent struct {
	A int    `json:"a"`
	P string `json:"p"`
	J string `json:"j"`
}

// customEvent is a custom or built-in event once its name, props and revenue are checked.
type customEvent struct {
	name        string
	props       map[string]string
	revenue     *analytics.Revenue
	interactive bool
	scrubbed    bool
}

// parseCustomEvent checks an event that is neither a page view nor an engagement. Built-in
// outbound and download events need an http(s) url, an outbound one on another host than the page;
// a children's property keeps no props but that url.
func parseCustomEvent(e collectEvent, pg analytics.Page, children bool) (customEvent, error) {
	name, err := analytics.EventName(e.N)
	if err != nil {
		return customEvent{}, err
	}
	props, err := analytics.ParseProps(e.P)
	if err != nil {
		return customEvent{}, err
	}
	rev, err := analytics.ParseRevenue(e.Rev)
	if err != nil {
		return customEvent{}, err
	}
	out := customEvent{name: name, props: props.Values, revenue: rev, interactive: e.I == nil || *e.I, scrubbed: props.Scrubbed > 0}
	if children {
		out.props = nil
	}
	switch name {
	case analytics.EventOutbound, analytics.EventFile:
		link, ok := analytics.LinkURL(props.Values[analytics.PropURL])
		if !ok {
			return customEvent{}, errors.New(name + " needs a url prop with an http(s) URL")
		}
		if name == analytics.EventOutbound && strings.EqualFold(link.Hostname(), pg.Host) {
			return customEvent{}, errors.New("outbound_click's url must be on another host than the page")
		}
		if out.props == nil {
			out.props = map[string]string{}
		}
		out.props[analytics.PropURL] = link.String()
	case analytics.EventNotFound:
		out.interactive = false
	}
	return out, nil
}

func readAnalyticsBody(w http.ResponseWriter, r *http.Request, drop func(reason string, n int)) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAnalyticsBody))
	if err == nil {
		return body, true
	}
	drop(analytics.ReasonInvalid, 1)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeProblem(w, errAnalyticsBodyTooLarge)
	} else {
		writeProblem(w, errValidation("the body could not be read"))
	}
	return nil, false
}

// parseAnalyticsBatch reads one event, or a batch of them (batch true).
func parseAnalyticsBatch(body []byte) ([]collectEvent, bool, error) {
	var probe struct {
		E json.RawMessage `json:"e"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, false, errors.New("the body is not a JSON object")
	}
	if probe.E == nil {
		var e collectEvent
		if err := json.Unmarshal(body, &e); err != nil {
			return nil, false, errors.New("the event does not match AnalyticsEvent")
		}
		return []collectEvent{e}, false, nil
	}
	var evs []collectEvent
	if err := json.Unmarshal(probe.E, &evs); err != nil {
		return nil, true, errors.New("e must be a list of events")
	}
	if len(evs) < 1 || len(evs) > maxAnalyticsBatch {
		return nil, true, errors.New("e must hold 1 to 100 events")
	}
	return evs, true, nil
}

// serveAnalyticsCollect takes events for the property analyticsCORS found. The client's address is
// shortened right after it is checked against the property's excluded addresses; only the short
// form reaches rate limits and hashing, and nothing of it is stored or logged.
func (s *Server) serveAnalyticsCollect(w http.ResponseWriter, r *http.Request) {
	a := s.analytics
	cp, _ := r.Context().Value(analyticsPropertyKey).(*cachedProperty)
	if a == nil || cp == nil {
		writeProblem(w, errNotFound)
		return
	}
	if r.Header.Get("Sec-GPC") == "1" || r.Header.Get("DNT") == "1" {
		s.takeWithdrawals(w, r, cp)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	ctx, now, prop := r.Context(), s.now(), cp.prop
	day := analytics.LocalDay(now, cp.loc)
	drop := func(reason string, n int) { a.buffer.Drop(prop.WorkspaceID, prop.ID, day, reason, int64(n)) }
	body, ok := readAnalyticsBody(w, r, drop)
	if !ok {
		return
	}
	if r.Header.Get("Origin") == "" {
		s.collectApp(w, r, cp, body, now, drop)
		return
	}
	evs, batch, err := parseAnalyticsBatch(body)
	if err != nil {
		drop(analytics.ReasonInvalid, 1)
		writeProblem(w, errValidation(err.Error()))
		return
	}
	pages := make([]analytics.Page, len(evs))
	customs := make([]*customEvent, len(evs))
	valid := make([]bool, len(evs))
	var changes []collectEvent
	children := prop.Audience == string(oas.AnalyticsAudienceChildren)
	// In a batch an invalid event is dropped on its own, so a mistake in one custom event does not
	// cost the page views sent with it; a single event answers why it was refused.
	refuse := func(reason string) bool {
		drop(analytics.ReasonInvalid, 1)
		if !batch {
			writeProblem(w, errValidation(reason))
			return true
		}
		return false
	}
	for i, e := range evs {
		pg, ok := analytics.ParsePage(e.U, e.H)
		if !ok {
			if refuse("u must be an http(s) URL of at most 2,000 characters") {
				return
			}
			continue
		}
		if !analytics.DomainAllowed(prop.Domains, matchHost(pg.Host)) {
			drop(analytics.ReasonOrigin, len(evs))
			writeProblem(w, errPageNotAllowed)
			return
		}
		pages[i] = pg
		if e.N == consentEvent {
			changes = append(changes, e)
			continue
		}
		if e.N == "pageview" || e.N == "engagement" {
			if len(e.Rev) > 0 && string(e.Rev) != "null" {
				if refuse("only custom and built-in events carry revenue") {
					return
				}
				continue
			}
			valid[i] = true
			continue
		}
		ce, err := parseCustomEvent(e, pg, children)
		if err != nil {
			if refuse(err.Error()) {
				return
			}
			continue
		}
		customs[i], valid[i] = &ce, true
	}
	var keep []int
	for i, e := range evs {
		if valid[i] && (prop.Collection != string(oas.AnalyticsCollectionConsent) || (e.C != nil && e.C.A == 1)) {
			keep = append(keep, i)
		}
		if valid[i] && e.C != nil && e.C.J != "" {
			changes = append(changes, e)
		}
	}
	if len(changes) > 0 {
		if err := s.takeConsentChanges(ctx, r, cp, changes, now); err != nil {
			writeProblem(w, err)
			return
		}
	}
	if len(keep) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	agent := analytics.ParseAgent(r.UserAgent(), r.Header.Get("Sec-CH-UA-Platform"), r.Header.Get("Sec-CH-UA-Mobile"))
	if agent.Bot || (r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Accept-Language") == "") {
		drop(analytics.ReasonBot, len(keep))
		w.WriteHeader(http.StatusAccepted)
		return
	}
	short, ok := s.admitAnalytics(w, r, cp, now, len(keep), drop)
	if !ok {
		return
	}
	ua := r.UserAgent() + "\x00" + r.Header.Get("Sec-CH-UA-Platform") + "\x00" + r.Header.Get("Sec-CH-UA-Mobile")
	language := ""
	if prop.Audience != string(oas.AnalyticsAudienceChildren) {
		language = analytics.Language(r.Header.Get("Accept-Language"))
	}
	visitor := func(salt []byte) int64 { return analytics.VisitorHash(salt, prop.ID, short, ua) }
	items := make([]analyticsItem, 0, len(keep))
	for _, i := range keep {
		width := evs[i].W
		it := analyticsItem{e: evs[i], page: pages[i], custom: customs[i], at: now, visitor: visitor,
			describe: func(pg analytics.Page, refHost, refPath string) *analytics.VisitInfo {
				return s.visitInfo(r, prop, short, agent, pg, refHost, refPath, width, language)
			}}
		s.consentedVisitor(ctx, cp, evs[i].C, &it)
		items = append(items, it)
	}
	if err := s.takeAnalytics(ctx, cp, now, items, drop); err != nil {
		writeProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func rateKeyAnalyticsAddress(prop uuid.UUID, short analytics.ShortIP) string {
	return "analytics:address:" + prop.String() + ":" + short.String()
}

func rateKeyAnalyticsProperty(prop uuid.UUID) string { return "analytics:property:" + prop.String() }

// admitAnalytics checks n events from the request's address against the property's excluded
// addresses and limits and counts them against the workspace's monthly cap. The address is
// shortened right after the exclusion check. Unless ok, the answer is written.
func (s *Server) admitAnalytics(w http.ResponseWriter, r *http.Request, cp *cachedProperty, now time.Time, n int, drop func(reason string, n int)) (analytics.ShortIP, bool) {
	a, prop := s.analytics, cp.prop
	addr, err := netip.ParseAddr(s.visitorIP(r))
	if err != nil {
		drop(analytics.ReasonInvalid, n)
		w.WriteHeader(http.StatusAccepted)
		return analytics.ShortIP{}, false
	}
	addr = addr.Unmap()
	excluded := slices.ContainsFunc(prop.ExcludedIps, func(p netip.Prefix) bool { return p.Contains(addr) })
	short := analytics.ShortenAddr(addr)
	addr = netip.Addr{}
	if excluded {
		drop(analytics.ReasonExcluded, n)
		w.WriteHeader(http.StatusAccepted)
		return analytics.ShortIP{}, false
	}
	g := s.limits.take(r.Context(), nil, rateTake{rateKeyAnalyticsAddress(prop.ID, short), limitAnalyticsPerAddress, n})
	if g[0].n < n {
		drop(analytics.ReasonRate, n-g[0].n)
		writeProblem(w, rateLimited(g[0].wait))
		return analytics.ShortIP{}, false
	}
	if g = s.limits.take(r.Context(), nil, rateTake{rateKeyAnalyticsProperty(prop.ID), limit{a.settings.PropertyEventsPerMinute, time.Minute}, n}); g[0].n < n {
		drop(analytics.ReasonRate, n-g[0].n)
		w.WriteHeader(http.StatusAccepted)
		return analytics.ShortIP{}, false
	}
	ok, err := s.takeUsage(r.Context(), prop.WorkspaceID, n, now)
	if err != nil {
		s.log.ErrorContext(r.Context(), "analytics usage", slog.Any("error", err))
		writeProblem(w, errInternal)
		return analytics.ShortIP{}, false
	}
	if !ok {
		drop(analytics.ReasonOverLimit, n)
		w.WriteHeader(http.StatusAccepted)
		return analytics.ShortIP{}, false
	}
	return short, true
}

// analyticsItem is a checked event on its way into the buffer: when it happened, how its visitor is
// hashed under a salt and how a visit it starts is described.
type analyticsItem struct {
	e        collectEvent
	page     analytics.Page
	custom   *customEvent
	at       time.Time
	visitor  func(salt []byte) int64
	describe func(pg analytics.Page, refHost, refPath string) *analytics.VisitInfo
	// persistent: visitor is a consented persistent id's hash, the same on every day.
	persistent bool
	// address is the rate limit key of a server event's address, empty without one.
	address string
}

// takeAnalytics puts checked events into the buffer: excluded paths and spam referrers are dropped,
// path rules applied, each event joins or starts its visitor's visit, and only a visit's first
// event describes it. errAnalyticsBusy means the buffer is full.
func (s *Server) takeAnalytics(ctx context.Context, cp *cachedProperty, now time.Time, items []analyticsItem, drop func(reason string, n int)) *apiError {
	a, prop := s.analytics, cp.prop
	var own string
	if u, err := url.Parse(s.auth.PublicURL); err == nil {
		own = u.Hostname()
	}
	out := make([]analytics.Event, 0, len(items))
	scrubbed := 0
	for _, it := range items {
		e, pg := it.e, it.page
		if slices.ContainsFunc(prop.ExcludedPaths, func(p string) bool { return analytics.MatchPath(p, pg.Path) }) {
			drop(analytics.ReasonExcluded, 1)
			continue
		}
		path := analytics.ApplyRules(cp.rules, pg.Path)
		engagement := e.N == "engagement"
		var refHost, refPath string
		if !engagement {
			refHost, refPath = analytics.ParseReferrer(e.R, pg.Host, own)
			if refHost != "" && analytics.SpamReferrer(refHost) {
				drop(analytics.ReasonBot, 1)
				continue
			}
		}
		v, ok, err := a.resolver.ResolveAt(ctx, prop.WorkspaceID, prop.ID, cp.loc, now, it.at, it.visitor, engagement)
		if err != nil {
			s.log.ErrorContext(ctx, "analytics visit", slog.Any("error", err))
			return errInternal
		}
		if !ok {
			continue
		}
		if v.Bot {
			if v.NewBot {
				a.buffer.MarkBot(analytics.BotVisitor{WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID, Day: v.Day, Visitor: v.Visitor})
			}
			drop(analytics.ReasonBot, 1)
			continue
		}
		ce := it.custom
		if pg.Scrubbed || (ce != nil && ce.scrubbed) {
			scrubbed++
		}
		ev := analytics.Event{
			WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID, Day: v.Day, TS: it.at, Visit: v.Visit, Visitor: v.Visitor,
			Kind: analytics.KindPageview, Name: e.N, Hostname: pg.Host, Path: path, Interactive: true, Persistent: it.persistent,
		}
		if ce != nil {
			ev.Kind, ev.Name, ev.Props, ev.Revenue, ev.Interactive = analytics.KindCustom, ce.name, ce.props, ce.revenue, ce.interactive
		}
		if engagement {
			ev.Kind, ev.Interactive = analytics.KindEngagement, false
			ev.EngagedMS = new(int32(min(max(e.MS, 0), maxEngagedMS)))
			ev.Scroll = new(int16(min(max(e.SD, 0), 100)))
		} else if v.NewVisit {
			ev.Info = it.describe(pg, refHost, refPath)
		}
		out = append(out, ev)
	}
	if len(out) > 0 && !a.buffer.Add(out) {
		drop(analytics.ReasonBuffer, len(out))
		return errAnalyticsBusy
	}
	if scrubbed > 0 {
		a.buffer.Count(prop.WorkspaceID, prop.ID, analytics.LocalDay(now, cp.loc), analytics.ReasonScrubbed, int64(scrubbed))
	}
	return nil
}

var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// visitorIP is the connecting address. Only when that address is one of the trusted proxies does the
// visitor's address in front of it count: the rightmost X-Forwarded-For hop that is not a trusted
// proxy, if it is a public address. With no trusted proxies configured nobody is believed.
func (s *Server) visitorIP(r *http.Request) string {
	peer := peerAddr(r)
	if !s.trusted(peer) {
		return peer
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for h := range strings.SplitSeq(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hops = append(hops, h)
			}
		}
	}
	pick := ""
	for i := len(hops) - 1; i >= 0; i-- {
		if pick = hops[i]; !s.trusted(pick) {
			break
		}
	}
	if a, ok := publicAddr(pick); ok && !s.trusted(pick) {
		return a
	}
	if a, ok := publicAddr(s.clientIP(r)); ok {
		return a
	}
	return peer
}

func publicAddr(v string) (string, bool) {
	a, err := netip.ParseAddr(v)
	if err != nil {
		return "", false
	}
	a = a.Unmap()
	if a.Zone() != "" || !a.IsGlobalUnicast() || a.IsPrivate() || sharedAddressSpace.Contains(a) {
		return "", false
	}
	return a.String(), true
}

// visitInfo describes a new visit: its source and channel, its location from the shortened address
// (the country only for a children's property), its browser and device.
func (s *Server) visitInfo(r *http.Request, prop store.AnalyticsProperty, short analytics.ShortIP, agent analytics.Agent, pg analytics.Page,
	refHost, refPath string, width int, language string) *analytics.VisitInfo {
	source, channel := analytics.Classify(refHost, refPath, pg.Acquisition, refHost != "" && analytics.DomainAllowed(prop.Domains, matchHost(refHost)))
	info := &analytics.VisitInfo{
		Hostname: pg.Host, ReferrerHost: refHost, ReferrerPath: refPath, Acquisition: pg.Acquisition, Source: analytics.Clean(source, 300),
		Channel: channel, Language: language, Platform: analytics.PlatformWeb,
		Device: agent.Device, Browser: agent.Browser, BrowserMajor: agent.BrowserMajor, OS: agent.OS, OSMajor: agent.OSMajor,
	}
	var loc analytics.Location
	if short != (analytics.ShortIP{}) {
		loc = s.analytics.geo.Lookup(short)
	}
	if loc.Country == "" {
		loc = analytics.Location{Country: s.requestCountry(r)}
	}
	info.Country = loc.Country
	if prop.Audience != string(oas.AnalyticsAudienceChildren) {
		info.Region, info.City = loc.Region, loc.City
		info.Screen = analytics.ScreenClass(width)
	}
	return info
}

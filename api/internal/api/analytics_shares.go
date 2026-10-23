package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
	"github.com/productdevbook/yuva/api/internal/ui"
)

const (
	shareTokenPrefix     = "yuva_sh_"
	shareAccessPrefix    = "yuva_sa_"
	sharePagePrefix      = "/share/"
	shareClientPrefix    = "/client/v1/analytics-shares/"
	maxSharesPerProperty = 50
	shareAccessTTL       = 12 * time.Hour
	shareTouchEvery      = time.Minute
	maxEmbedOrigins      = 20

	// OWASP's argon2id settings of equal strength trade memory for passes; the 7 MiB one keeps
	// concurrent unlocks cheap in memory on a host without swap.
	argonTime    = 5
	argonMemory  = 7 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	argonMaxTime = 10
	argonMaxMem  = 64 * 1024
	shareHashers = 4
)

var (
	limitSharePerIP       = limit{120, time.Minute}
	limitUnlockPerIP      = limit{30, time.Minute}
	limitUnlockPerShareIP = limit{10, time.Minute}
	budgetUnlockFailures  = budget{limit{60, time.Hour}, limit{1000, time.Hour}}
)

// shareHashSlots bounds how many password hashes run at once, and so the memory they take.
var shareHashSlots = make(chan struct{}, shareHashers)

var (
	errShareGone             = problem(http.StatusNotFound, "not_found", "no such share link")
	errShareLimit            = problem(http.StatusForbidden, "share_limit", "this property has 50 share links; delete one first")
	errShareRevoked          = problem(http.StatusConflict, "share_revoked", "a revoked share link cannot be changed")
	errSharePasswordRequired = problem(http.StatusUnauthorized, "share_password_required", "this share link needs its password")
	errShareWrongPassword    = problem(http.StatusUnauthorized, "wrong_password", "the password is not right")
	errShareNoPassword       = problem(http.StatusConflict, "no_password", "this share link has no password")
	errViewNotShared         = problem(http.StatusForbidden, "view_not_shared", "this share link does not show that")
)

// shareViewDims are the dimensions each view adds to what a share link may query; totals and time
// grains come with every view.
var shareViewDims = map[oas.AnalyticsShareView][]string{
	oas.AnalyticsSharePages:     {"page", "entry_page", "exit_page", "hostname"},
	oas.AnalyticsShareSources:   {"channel", "source", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"},
	oas.AnalyticsShareLocations: {"country", "region", "city"},
	oas.AnalyticsShareDevices:   {"device", "browser", "browser_version", "os", "os_version", "screen_size", "language", "platform", "app_version"},
	oas.AnalyticsShareEvents:    {"event", "prop"},
}

var goalMetrics = []oas.AnalyticsMetric{oas.AnalyticsMetricConversions, oas.AnalyticsMetricConversionRate, oas.AnalyticsMetricCompletions}

func hashSharePassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func checkSharePassword(encoded, pw string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m > argonMaxMem || t > argonMaxTime || p == 0 || p > 4 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// normalizeEmbedOrigin keeps an origin as frame-ancestors takes it: scheme, host (a leading *. for
// subdomains) and port, nothing that could end the directive.
func normalizeEmbedOrigin(v string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || u.Opaque != "" {
		return "", false
	}
	host, port := u.Hostname(), u.Port()
	host, ok := normalizeAnalyticsDomain(host)
	if !ok || host == "[::1]" {
		return "", false
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", false
		}
		host += ":" + strconv.Itoa(n)
	}
	return u.Scheme + "://" + host, true
}

func embedOrigins(in []oas.AnalyticsEmbedOrigin) ([]string, error) {
	if len(in) > maxEmbedOrigins {
		return nil, errValidation("embed_origins may list at most 20 origins")
	}
	out := []string{}
	for _, v := range in {
		o, ok := normalizeEmbedOrigin(v)
		if !ok {
			return nil, errValidation(strconv.Quote(v) + " is not an origin such as https://www.example.com or https://*.example.com")
		}
		if !slices.Contains(out, o) {
			out = append(out, o)
		}
	}
	return out, nil
}

func shareViews(in []oas.AnalyticsShareView) ([]string, error) {
	if len(in) == 0 {
		return nil, errValidation("views must name at least one view")
	}
	out := []string{}
	for _, v := range in {
		if !v.Valid() {
			return nil, errValidation("unknown view " + strconv.Quote(string(v)))
		}
		if !slices.Contains(out, string(v)) {
			out = append(out, string(v))
		}
	}
	return out, nil
}

func sharePassword(pw string) (string, error) {
	if n := len([]rune(pw)); n < 8 || n > 200 {
		return "", errValidation("password must be 8 to 200 characters")
	}
	return hashSharePassword(pw), nil
}

func (s *Server) shareExpiry(t *time.Time) error {
	if t != nil && !t.After(s.now()) {
		return errValidation("expires_at must be in the future")
	}
	return nil
}

func (s *Server) shareBody(sh store.AnalyticsShare) oas.AnalyticsShare {
	out := oas.AnalyticsShare{
		Id: sh.ID, PropertyId: sh.PropertyID, Name: sh.Name, HasPassword: sh.PasswordHash != nil, TokenPrefix: sh.TokenPrefix,
		ExpiresAt: sh.ExpiresAt, RevokedAt: sh.RevokedAt, LastViewedAt: sh.LastViewedAt, CreatedBy: sh.CreatedBy,
		CreatedAt: sh.CreatedAt, UpdatedAt: sh.UpdatedAt, Views: make([]oas.AnalyticsShareView, len(sh.Views)), EmbedOrigins: sh.EmbedOrigins,
		Active: sh.RevokedAt == nil && (sh.ExpiresAt == nil || sh.ExpiresAt.After(s.now())),
	}
	for i, v := range sh.Views {
		out.Views[i] = oas.AnalyticsShareView(v)
	}
	return out
}

func (s *Server) ListAnalyticsShares(ctx context.Context, req oas.ListAnalyticsSharesRequestObject) (oas.ListAnalyticsSharesResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListAnalyticsShares(ctx, store.ListAnalyticsSharesParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID})
	if err != nil {
		return nil, err
	}
	out := oas.ListAnalyticsShares200JSONResponse{Items: make([]oas.AnalyticsShare, len(rows))}
	for i, sh := range rows {
		out.Items[i] = s.shareBody(sh)
	}
	return out, nil
}

func (s *Server) CreateAnalyticsShare(ctx context.Context, req oas.CreateAnalyticsShareRequestObject) (oas.CreateAnalyticsShareResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireAnalyticsManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	name, err := trimmed(b.Name, 1, 100, "name")
	if err != nil {
		return nil, err
	}
	views, err := shareViews(b.Views)
	if err != nil {
		return nil, err
	}
	origins := []string{}
	if b.EmbedOrigins != nil {
		if origins, err = embedOrigins(*b.EmbedOrigins); err != nil {
			return nil, err
		}
	}
	if err := s.shareExpiry(b.ExpiresAt); err != nil {
		return nil, err
	}
	var pwHash *string
	if b.Password != nil {
		h, err := sharePassword(*b.Password)
		if err != nil {
			return nil, err
		}
		pwHash = &h
	}
	token := shareTokenPrefix + randomToken(32)
	var out store.AnalyticsShare
	err = s.st.InTx(ctx, func(q *store.Queries) error {
		prop, err := visibleProperty(ctx, q, p, req.PropertyId, true)
		if err != nil {
			return err
		}
		n, err := q.CountAnalyticsShares(ctx, store.CountAnalyticsSharesParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID})
		if err != nil {
			return err
		}
		if n >= maxSharesPerProperty {
			return errShareLimit
		}
		out, err = q.CreateAnalyticsShare(ctx, store.CreateAnalyticsShareParams{
			ID: newID(), WorkspaceID: p.workspaceID, PropertyID: prop.ID, Name: name, TokenHash: hashSecret(token),
			TokenPrefix: token[:len(shareTokenPrefix)+4], Views: views, PasswordHash: pwHash, EmbedOrigins: origins,
			ExpiresAt: b.ExpiresAt, CreatedBy: p.memberRef(), Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.CreateAnalyticsShare201JSONResponse{Share: s.shareBody(out), Token: token,
		Url: strings.TrimSuffix(s.auth.PublicURL, "/") + sharePagePrefix + token}, nil
}

func (s *Server) UpdateAnalyticsShare(ctx context.Context, req oas.UpdateAnalyticsShareRequestObject) (oas.UpdateAnalyticsShareResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireAnalyticsManager(p); err != nil {
		return nil, err
	}
	b := req.Body
	var out store.AnalyticsShare
	err := s.st.InTx(ctx, func(q *store.Queries) error {
		prop, err := visibleProperty(ctx, q, p, req.PropertyId, false)
		if err != nil {
			return err
		}
		cur, err := q.GetAnalyticsShare(ctx, store.GetAnalyticsShareParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, ID: req.ShareId})
		if store.IsNotFound(err) {
			return errShareGone
		}
		if err != nil {
			return err
		}
		if cur.RevokedAt != nil {
			return errShareRevoked
		}
		if b.Name != nil {
			if cur.Name, err = trimmed(*b.Name, 1, 100, "name"); err != nil {
				return err
			}
		}
		if b.Views != nil {
			if cur.Views, err = shareViews(*b.Views); err != nil {
				return err
			}
		}
		if b.EmbedOrigins != nil {
			if cur.EmbedOrigins, err = embedOrigins(*b.EmbedOrigins); err != nil {
				return err
			}
		}
		if b.Password.IsSpecified() {
			cur.PasswordHash = nil
			if !b.Password.IsNull() {
				h, err := sharePassword(b.Password.MustGet())
				if err != nil {
					return err
				}
				cur.PasswordHash = &h
			}
		}
		if b.ExpiresAt.IsSpecified() {
			cur.ExpiresAt = nil
			if !b.ExpiresAt.IsNull() {
				t := b.ExpiresAt.MustGet()
				if err := s.shareExpiry(&t); err != nil {
					return err
				}
				cur.ExpiresAt = &t
			}
		}
		out, err = q.UpdateAnalyticsShare(ctx, store.UpdateAnalyticsShareParams{
			WorkspaceID: p.workspaceID, PropertyID: prop.ID, ID: cur.ID, Name: cur.Name, Views: cur.Views,
			PasswordHash: cur.PasswordHash, EmbedOrigins: cur.EmbedOrigins, ExpiresAt: cur.ExpiresAt, Now: s.now(),
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateAnalyticsShare200JSONResponse(s.shareBody(out)), nil
}

func (s *Server) RevokeAnalyticsShare(ctx context.Context, req oas.RevokeAnalyticsShareRequestObject) (oas.RevokeAnalyticsShareResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireAnalyticsManager(p); err != nil {
		return nil, err
	}
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.st.RevokeAnalyticsShare(ctx, store.RevokeAnalyticsShareParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, ID: req.ShareId, Now: s.now()})
	if store.IsNotFound(err) {
		return nil, errShareGone
	}
	if err != nil {
		return nil, err
	}
	return oas.RevokeAnalyticsShare200JSONResponse(s.shareBody(out)), nil
}

func (s *Server) DeleteAnalyticsShare(ctx context.Context, req oas.DeleteAnalyticsShareRequestObject) (oas.DeleteAnalyticsShareResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	if err := requireAnalyticsManager(p); err != nil {
		return nil, err
	}
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	n, err := s.st.DeleteAnalyticsShare(ctx, store.DeleteAnalyticsShareParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, ID: req.ShareId})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errShareGone
	}
	return oas.DeleteAnalyticsShare204Response{}, nil
}

// lookupShare finds a live share link by its token and its property, scoped by the workspace the
// lookup returned.
func (s *Server) lookupShare(ctx context.Context, token string) (store.AnalyticsShare, store.AnalyticsProperty, error) {
	var prop store.AnalyticsProperty
	if !strings.HasPrefix(token, shareTokenPrefix) || len(token) > 200 {
		return store.AnalyticsShare{}, prop, errShareGone
	}
	sh, err := s.st.GetAnalyticsShareByTokenHash(ctx, store.GetAnalyticsShareByTokenHashParams{TokenHash: hashSecret(token), Now: s.now()})
	if store.IsNotFound(err) {
		return sh, prop, errShareGone
	}
	if err != nil {
		return sh, prop, err
	}
	prop, err = s.st.GetAnalyticsProperty(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: sh.WorkspaceID, ID: sh.PropertyID})
	if store.IsNotFound(err) {
		return sh, prop, errShareGone
	}
	return sh, prop, err
}

// openShare is lookupShare for the share endpoints: limited per address, and with a password only
// for a request carrying the link's access token.
func (s *Server) openShare(ctx context.Context, token string, needAccess bool) (store.AnalyticsShare, store.AnalyticsProperty, bool, error) {
	if err := s.analyticsOn(); err != nil {
		return store.AnalyticsShare{}, store.AnalyticsProperty{}, false, err
	}
	r := requestFrom(ctx)
	if err := s.rateLimit(ctx, rateCheck{"share:ip:" + rateIP(s.clientIP(r)), limitSharePerIP}); err != nil {
		return store.AnalyticsShare{}, store.AnalyticsProperty{}, false, err
	}
	sh, prop, err := s.lookupShare(ctx, token)
	if err != nil {
		return sh, prop, false, err
	}
	unlocked := sh.PasswordHash == nil
	if !unlocked {
		if access, ok := bearerToken(r); ok {
			unlocked = s.checkShareAccess(sh, access)
		}
	}
	if needAccess && !unlocked {
		return sh, prop, false, errSharePasswordRequired
	}
	return sh, prop, unlocked, nil
}

const shareAccessInfo = "yuva analytics share access v1"

func shareAccessMAC(key []byte, sh store.AnalyticsShare, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	if sh.PasswordHash != nil {
		mac.Write([]byte(*sh.PasswordHash))
	}
	return mac.Sum(nil)
}

// shareAccessToken is signed over the link and its password hash, so a new password or none ends
// every token given out before.
func (s *Server) shareAccessToken(sh store.AnalyticsShare) (string, time.Time) {
	exp := s.now().Add(shareAccessTTL).Truncate(time.Second)
	if sh.ExpiresAt != nil && sh.ExpiresAt.Before(exp) {
		exp = *sh.ExpiresAt
	}
	payload := binary.BigEndian.AppendUint64(slices.Clone(sh.ID[:]), uint64(exp.Unix()))
	return shareAccessPrefix + base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(shareAccessMAC(s.secrets.Derive(shareAccessInfo), sh, payload)), exp
}

func (s *Server) checkShareAccess(sh store.AnalyticsShare, token string) bool {
	rest, ok := strings.CutPrefix(token, shareAccessPrefix)
	if !ok {
		return false
	}
	p64, m64, ok := strings.Cut(rest, ".")
	if !ok {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(p64)
	if err != nil || len(payload) != 24 {
		return false
	}
	mac, err := base64.RawURLEncoding.DecodeString(m64)
	if err != nil || !s.anyDerived(shareAccessInfo, func(k []byte) bool { return hmac.Equal(mac, shareAccessMAC(k, sh, payload)) }) {
		return false
	}
	if !slices.Equal(payload[:16], sh.ID[:]) {
		return false
	}
	return s.now().Before(time.Unix(int64(binary.BigEndian.Uint64(payload[16:])), 0))
}

func shareHas(sh store.AnalyticsShare, v oas.AnalyticsShareView) bool {
	return slices.Contains(sh.Views, string(v))
}

func (s *Server) GetSharedAnalytics(ctx context.Context, req oas.GetSharedAnalyticsRequestObject) (oas.GetSharedAnalyticsResponseObject, error) {
	sh, prop, unlocked, err := s.openShare(ctx, req.Token, false)
	if err != nil {
		return nil, err
	}
	if !unlocked {
		return oas.GetSharedAnalytics200JSONResponse{PasswordRequired: true}, nil
	}
	now := s.now()
	if err := s.st.TouchAnalyticsShare(ctx, store.TouchAnalyticsShareParams{WorkspaceID: sh.WorkspaceID, ID: sh.ID, Now: now, StaleBefore: now.Add(-shareTouchEvery)}); err != nil {
		return nil, err
	}
	out := oas.AnalyticsSharedPage{
		PasswordRequired: false, Name: &sh.Name, ExpiresAt: sh.ExpiresAt,
		Property: &oas.AnalyticsSharedProperty{Name: prop.Name, Timezone: prop.Timezone, Currency: prop.Currency, Platforms: make([]oas.AnalyticsPlatform, len(prop.Platforms))},
		RawWindowFrom: &openapi_types.Date{Time: s.rawFrom()},
	}
	for i, v := range prop.Platforms {
		out.Property.Platforms[i] = oas.AnalyticsPlatform(v)
	}
	views := make([]oas.AnalyticsShareView, len(sh.Views))
	for i, v := range sh.Views {
		views[i] = oas.AnalyticsShareView(v)
	}
	out.Views = &views
	if shareHas(sh, oas.AnalyticsShareGoals) {
		goals, err := s.st.ListAnalyticsGoals(ctx, store.ListAnalyticsGoalsParams{WorkspaceID: sh.WorkspaceID, PropertyID: prop.ID})
		if err != nil {
			return nil, err
		}
		gs := make([]oas.AnalyticsGoal, len(goals))
		for i, g := range goals {
			gs[i] = goalBody(g)
		}
		out.Goals = &gs
	}
	if shareHas(sh, oas.AnalyticsShareLocations) {
		out.Geoip = new(s.geoIPBody())
	}
	return oas.GetSharedAnalytics200JSONResponse(out), nil
}

func (s *Server) UnlockSharedAnalytics(ctx context.Context, req oas.UnlockSharedAnalyticsRequestObject) (oas.UnlockSharedAnalyticsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	addr := s.clientIP(requestFrom(ctx))
	ip := rateIP(addr)
	if err := s.rateLimit(ctx, rateCheck{"share-unlock:ip:" + ip, limitUnlockPerIP}); err != nil {
		return nil, err
	}
	sh, _, err := s.lookupShare(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	if sh.PasswordHash == nil {
		return nil, errShareNoPassword
	}
	if err := s.rateLimit(ctx, rateCheck{"share-unlock:" + sh.ID.String() + ":" + ip, limitUnlockPerShareIP}); err != nil {
		return nil, err
	}
	// Only wrong passwords count against the link and the caller's network, so viewers who know
	// the password spend nothing; once a budget is used up every unlock waits, since a right
	// password cannot be told from a guess without hashing it.
	failures := sharedChecks(addr, "share-unlock:failed:"+sh.ID.String(), budgetUnlockFailures)
	if full, wait, err := s.limits.full(ctx, failures...); err != nil {
		return nil, err
	} else if full {
		return nil, rateLimited(wait)
	}
	select {
	case shareHashSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ok := checkSharePassword(*sh.PasswordHash, req.Body.Password)
	<-shareHashSlots
	if !ok {
		_ = s.rateLimit(ctx, failures...)
		return nil, errShareWrongPassword
	}
	token, exp := s.shareAccessToken(sh)
	return oas.UnlockSharedAnalytics200JSONResponse{AccessToken: token, ExpiresAt: exp}, nil
}

// shareAllows checks a query against the link's views: totals and time grains always, other
// dimensions and filters only with the view they belong to, goals with goals, events with events.
func shareAllows(sh store.AnalyticsShare, b oas.AnalyticsQuery) error {
	if b.SegmentId != nil {
		return errValidation("a share link cannot use saved segments; send filters instead")
	}
	if b.Dataset != nil && *b.Dataset == oas.AnalyticsDatasetSupport {
		return errViewNotShared
	}
	allowed := func(dim string) bool {
		if strings.HasPrefix(dim, "time:") {
			return true
		}
		for v, dims := range shareViewDims {
			if shareHas(sh, v) && slices.Contains(dims, dim) {
				return true
			}
		}
		return false
	}
	if b.Dimensions != nil {
		for _, d := range *b.Dimensions {
			if !allowed(string(d)) {
				return errViewNotShared
			}
		}
	}
	if b.Filters != nil {
		for _, f := range *b.Filters {
			if !allowed(string(f.Dimension)) {
				return errViewNotShared
			}
		}
	}
	metricAllowed := func(m oas.AnalyticsMetric) bool {
		switch {
		case slices.Contains(goalMetrics, m):
			return shareHas(sh, oas.AnalyticsShareGoals)
		case m == oas.AnalyticsMetricEvents:
			return shareHas(sh, oas.AnalyticsShareEvents)
		case m == oas.AnalyticsMetricRevenue:
			return shareHas(sh, oas.AnalyticsShareGoals) || shareHas(sh, oas.AnalyticsShareEvents)
		}
		return true
	}
	for _, m := range b.Metrics {
		if !metricAllowed(m) {
			return errViewNotShared
		}
	}
	if b.OrderBy != nil {
		for _, o := range *b.OrderBy {
			if !metricAllowed(o.Metric) {
				return errViewNotShared
			}
		}
	}
	if b.GoalId != nil && !shareHas(sh, oas.AnalyticsShareGoals) {
		return errViewNotShared
	}
	return nil
}

func (s *Server) QuerySharedAnalytics(ctx context.Context, req oas.QuerySharedAnalyticsRequestObject) (oas.QuerySharedAnalyticsResponseObject, error) {
	sh, prop, _, err := s.openShare(ctx, req.Token, true)
	if err != nil {
		return nil, err
	}
	if err := shareAllows(sh, *req.Body); err != nil {
		return nil, err
	}
	ar, err := s.analyticsRequest(ctx, prop, *req.Body)
	if err != nil {
		return nil, err
	}
	out, err := s.analyticsResult(ctx, prop, ar, false)
	if err != nil {
		return nil, err
	}
	return oas.QuerySharedAnalytics200JSONResponse(out), nil
}

func (s *Server) GetSharedAnalyticsRealtime(ctx context.Context, req oas.GetSharedAnalyticsRealtimeRequestObject) (oas.GetSharedAnalyticsRealtimeResponseObject, error) {
	sh, prop, _, err := s.openShare(ctx, req.Token, true)
	if err != nil {
		return nil, err
	}
	if !shareHas(sh, oas.AnalyticsShareRealtime) {
		return nil, errViewNotShared
	}
	out, err := s.analyticsRealtime(ctx, prop)
	if err != nil {
		return nil, err
	}
	return oas.GetSharedAnalyticsRealtime200JSONResponse(out), nil
}

// serveSharePage serves the panel's page for a share link, framable by the link's embed origins
// only; an unknown, revoked or expired link gets the page too, which then says so.
func (s *Server) serveSharePage(w http.ResponseWriter, r *http.Request) {
	if err := s.rateLimit(r.Context(), rateCheck{"share:ip:" + rateIP(s.clientIP(r)), limitSharePerIP}); err != nil {
		s.writeError(w, r, err)
		return
	}
	var ancestors []string
	if s.analytics != nil {
		sh, _, err := s.lookupShare(r.Context(), r.PathValue("token"))
		if err == nil {
			ancestors = sh.EmbedOrigins
		} else if !isAPIError(err) {
			s.log.ErrorContext(r.Context(), "share page", "error", err)
		}
	}
	ui.SharePage(w, r, ancestors)
}

func isAPIError(err error) bool {
	_, ok := err.(*apiError)
	return ok
}

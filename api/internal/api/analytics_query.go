package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	analyticsQueriesPerWorkspace = 4
	analyticsQueryWait           = 2 * time.Second
	analyticsQueryLease          = 2*analytics.QueryTimeout + 30*time.Second
	analyticsFinishedTTL         = 10 * time.Minute
	analyticsOpenTTL             = 30 * time.Second
	analyticsRealtimeTTL         = 10 * time.Second
	analyticsCacheEntries        = 2000
	maxAnalyticsRange            = 400
	defaultAnalyticsLimit        = 100
	defaultHealthDays            = 30
)

var errAnalyticsBusyQueries = &apiError{Status: http.StatusTooManyRequests, Code: "rate_limited",
	Detail: "this workspace already runs four analytics queries; try again in a moment", RetryAfter: 2}

// querySlots lets each workspace run a few analytics queries at once across all replicas. A slot is
// leased for longer than a query and its comparison may run, so one a crashed process held frees
// itself.
type querySlots struct {
	limits *rateLimiter
}

func (q querySlots) acquire(ctx context.Context, ws uuid.UUID) (func(), error) {
	release, ok, err := q.limits.acquireSlot(ctx, "analytics-query:"+ws.String(), analyticsQueriesPerWorkspace, analyticsQueryLease, analyticsQueryWait)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errAnalyticsBusyQueries
	}
	return release, nil
}

type cachedAnswer struct {
	value   any
	expires time.Time
}

// answerCache keeps query and realtime answers in this process for their time to live.
type answerCache struct {
	mu sync.Mutex
	m  map[string]cachedAnswer
}

func (c *answerCache) get(key string, now time.Time) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || !now.Before(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *answerCache) put(key string, v any, now time.Time, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) >= analyticsCacheEntries {
		c.m = map[string]cachedAnswer{}
	}
	c.m[key] = cachedAnswer{value: v, expires: now.Add(ttl)}
}

func (c *answerCache) forget(propertyID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := propertyID.String() + "|"
	for k := range c.m {
		if strings.HasPrefix(k, prefix) {
			delete(c.m, k)
		}
	}
}

// analyticsRange turns a range into the property's local days.
func analyticsRange(r oas.AnalyticsRange, today time.Time) (time.Time, time.Time, error) {
	if r.Period != nil {
		if r.From != nil || r.To != nil {
			return time.Time{}, time.Time{}, errValidation("send either range.period, or range.from and range.to")
		}
		y, m, _ := today.Date()
		month := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
		switch *r.Period {
		case oas.AnalyticsToday:
			return today, today, nil
		case oas.AnalyticsYesterday:
			return today.AddDate(0, 0, -1), today.AddDate(0, 0, -1), nil
		case oas.AnalyticsLast7Days:
			return today.AddDate(0, 0, -6), today, nil
		case oas.AnalyticsLast28Days:
			return today.AddDate(0, 0, -27), today, nil
		case oas.AnalyticsLast30Days:
			return today.AddDate(0, 0, -29), today, nil
		case oas.AnalyticsLast90Days:
			return today.AddDate(0, 0, -89), today, nil
		case oas.AnalyticsLast6Months:
			return today.AddDate(0, -6, 1), today, nil
		case oas.AnalyticsLast12Months:
			return today.AddDate(-1, 0, 1), today, nil
		case oas.AnalyticsThisMonth:
			return month, today, nil
		case oas.AnalyticsLastMonth:
			return month.AddDate(0, -1, 0), month.AddDate(0, 0, -1), nil
		case oas.AnalyticsThisYear:
			return time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), today, nil
		}
		return time.Time{}, time.Time{}, errValidation("unknown range.period")
	}
	if r.From == nil || r.To == nil {
		return time.Time{}, time.Time{}, errValidation("send range.period, or range.from and range.to")
	}
	return checkedRange(r.From.Time, r.To.Time)
}

func checkedRange(from, to time.Time) (time.Time, time.Time, error) {
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if to.Before(from) {
		return from, to, errValidation("the range ends before it starts")
	}
	if to.Sub(from) >= maxAnalyticsRange*24*time.Hour {
		return from, to, problem(http.StatusUnprocessableEntity, "range_too_large", "a range may span at most 400 days")
	}
	return from, to, nil
}

func comparisonRange(c oas.AnalyticsCompare, from, to time.Time) (time.Time, time.Time, error) {
	switch c.With {
	case oas.AnalyticsComparePrevious:
		n := int(to.Sub(from).Hours()/24) + 1
		return from.AddDate(0, 0, -n), from.AddDate(0, 0, -1), nil
	case oas.AnalyticsCompareYear:
		return from.AddDate(-1, 0, 0), to.AddDate(-1, 0, 0), nil
	case oas.AnalyticsCompareRange:
		if c.From == nil || c.To == nil {
			return from, to, errValidation("compare.with range needs compare.from and compare.to")
		}
		return checkedRange(c.From.Time, c.To.Time)
	}
	return from, to, errValidation("compare.with must be previous, year or range")
}

func analyticsQueryError(err error) error {
	var raw *analytics.NeedsRawError
	if errors.As(err, &raw) {
		e := problem(http.StatusUnprocessableEntity, "needs_raw_window", raw.Error()+"; narrow the range or drop the filter")
		e.Extra = map[string]any{"raw_window_from": raw.From.Format(time.DateOnly)}
		return e
	}
	if errors.Is(err, analytics.ErrBadQuery) {
		return errValidation(strings.TrimPrefix(err.Error(), analytics.ErrBadQuery.Error()+": "))
	}
	return err
}

func metricsBody(s analytics.Sums, names []string) oas.AnalyticsMetrics {
	var m oas.AnalyticsMetrics
	for _, n := range names {
		v := s.Metric(n)
		switch n {
		case "visitors":
			m.Visitors = &v
		case "visits":
			m.Visits = &v
		case "pageviews":
			m.Pageviews = &v
		case "views_per_visit":
			m.ViewsPerVisit = &v
		case "bounce_rate":
			m.BounceRate = &v
		case "visit_duration":
			m.VisitDuration = &v
		case "time_on_page":
			m.TimeOnPage = &v
		case "scroll_depth":
			m.ScrollDepth = &v
		case "events":
			m.Events = &v
		case "conversions":
			m.Conversions = &v
		case "conversion_rate":
			m.ConversionRate = &v
		case "completions":
			m.Completions = &v
		case "revenue":
			m.Revenue = revenueBody(s.Revenue)
		case "conversations":
			m.Conversations = &v
		case "feedback":
			m.Feedback = &v
		case "ratings_up":
			m.RatingsUp = &v
		case "ratings_down":
			m.RatingsDown = &v
		case "helpful_rate":
			m.HelpfulRate = &v
		case "discussions":
			m.Discussions = &v
		case "support_rate":
			m.SupportRate = &v
		}
	}
	return m
}

// revenueBody lists the revenue per currency, the largest total first.
func revenueBody(r map[string]analytics.Money) *[]oas.AnalyticsRevenue {
	out := []oas.AnalyticsRevenue{}
	for c, m := range r {
		avg := int64(0)
		if m.Events > 0 {
			avg = int64(math.Round(float64(m.Minor) / float64(m.Events)))
		}
		out = append(out, oas.AnalyticsRevenue{Currency: c, Total: m.Minor, Average: avg, Events: m.Events, Digits: analytics.CurrencyDigits(c)})
	}
	slices.SortFunc(out, func(a, b oas.AnalyticsRevenue) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), strings.Compare(a.Currency, b.Currency))
	})
	return &out
}

// rowBody splits places into their name and the country (and region) they are in.
func rowBody(r analytics.Row, dims, metrics []string) oas.AnalyticsRow {
	out := oas.AnalyticsRow{Dimensions: slices.Clone(r.Dims), Metrics: metricsBody(r.Sums, metrics)}
	for i, d := range dims {
		if i >= len(out.Dimensions) || (d != "region" && d != "city") {
			continue
		}
		parts := strings.Split(out.Dimensions[i], "\t")
		out.Dimensions[i] = parts[len(parts)-1]
		if len(parts) > 1 {
			out.Country = &parts[0]
		}
		if len(parts) > 2 {
			out.Region = &parts[1]
		}
	}
	return out
}

type analyticsAnswer struct {
	result     analytics.Result
	comparison *analytics.Result
}

func timeOnly(dims []string) bool {
	return len(dims) == 1 && strings.HasPrefix(dims[0], "time:")
}

func timeIndex(dims []string) int {
	return slices.IndexFunc(dims, func(d string) bool { return strings.HasPrefix(d, "time:") })
}

func sortAnalyticsRows(rows []analytics.Row, dims []string, order []analytics.Order) {
	if timeOnly(dims) {
		return
	}
	ti := timeIndex(dims)
	analytics.SortRows(rows, order)
	if ti >= 0 {
		slices.SortStableFunc(rows, func(a, b analytics.Row) int { return strings.Compare(a.Dims[ti], b.Dims[ti]) })
	}
}

// bucketOrdinals numbers a result's time buckets in order, so a comparison range's rows line up
// with the range's by position.
func bucketOrdinals(rows []analytics.Row, ti int) map[string]int {
	var buckets []string
	for _, r := range rows {
		buckets = append(buckets, r.Dims[ti])
	}
	slices.Sort(buckets)
	buckets = slices.Compact(buckets)
	out := map[string]int{}
	for i, b := range buckets {
		out[b] = i
	}
	return out
}

func rowKey(r analytics.Row, ti int, ordinals map[string]int) string {
	parts := slices.Clone(r.Dims)
	if ti >= 0 {
		parts[ti] = strconv.Itoa(ordinals[parts[ti]])
	}
	return strings.Join(parts, "\x1f")
}

func (s *Server) QueryAnalytics(ctx context.Context, req oas.QueryAnalyticsRequestObject) (oas.QueryAnalyticsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
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
	if req.Params.Format != nil && *req.Params.Format == oas.AnalyticsFormatCSV {
		body := analyticsCSV(ar.q.Dimensions, ar.q.Metrics, out.Rows)
		name := `attachment; filename="analytics-` + ar.q.From.Format(time.DateOnly) + `-` + ar.q.To.Format(time.DateOnly) + `.csv"`
		return oas.QueryAnalytics200TextcsvResponse{Body: bytes.NewReader(body), ContentLength: int64(len(body)),
			Headers: oas.QueryAnalytics200ResponseHeaders{ContentDisposition: &name}}, nil
	}
	return oas.QueryAnalytics200JSONResponse{Body: out}, nil
}

// analyticsRequest is a query body checked and turned into the property's query, its comparison
// and its page.
type analyticsRequest struct {
	q      analytics.Query
	cmp    *analytics.Query
	order  []analytics.Order
	limit  int
	offset int
	// supportOrder ranks pages with fewer than supportMinViews page views after the others.
	supportOrder bool
	insights     bool
}

func (s *Server) analyticsRequest(ctx context.Context, prop store.AnalyticsProperty, b oas.AnalyticsQuery) (analyticsRequest, error) {
	var ar analyticsRequest
	ws := prop.WorkspaceID
	loc := propertyLocation(prop.Timezone)
	today := analytics.LocalDay(s.now(), loc)
	from, to, err := analyticsRange(b.Range, today)
	if err != nil {
		return ar, err
	}
	q := analytics.Query{WorkspaceID: ws, PropertyID: prop.ID, Timezone: prop.Timezone, From: from, To: to, RawFrom: s.rawFrom()}
	support := b.Dataset != nil && *b.Dataset == oas.AnalyticsDatasetSupport
	if support {
		if err := checkSupportQuery(prop, b); err != nil {
			return ar, err
		}
		q.Dataset = analytics.DatasetSupport
	}
	if b.Prop != nil {
		if q.Prop, err = trimmed(*b.Prop, 1, 300, "prop"); err != nil {
			return ar, err
		}
	}
	if b.GoalId != nil {
		g, err := s.st.GetAnalyticsGoal(ctx, store.GetAnalyticsGoalParams{WorkspaceID: ws, PropertyID: prop.ID, ID: *b.GoalId})
		if store.IsNotFound(err) {
			return ar, errGoalGone
		}
		if err != nil {
			return ar, err
		}
		q.Goal = goalDef(g)
	}
	if len(b.Metrics) == 0 {
		return ar, errValidation("metrics must name at least one metric")
	}
	for _, m := range b.Metrics {
		if slices.Contains(q.Metrics, string(m)) {
			return ar, errValidation("metric " + string(m) + " is listed twice")
		}
		if err := checkDatasetMetric(support, string(m)); err != nil {
			return ar, err
		}
		q.Metrics = append(q.Metrics, string(m))
	}
	if b.Dimensions != nil {
		for _, d := range *b.Dimensions {
			q.Dimensions = append(q.Dimensions, string(d))
		}
	}
	if b.Filters != nil {
		if q.Filters, err = segmentFilters(*b.Filters); err != nil {
			return ar, err
		}
	}
	if b.SegmentId != nil {
		sg, err := s.st.GetAnalyticsSegment(ctx, store.GetAnalyticsSegmentParams{WorkspaceID: ws, PropertyID: prop.ID, ID: *b.SegmentId})
		if store.IsNotFound(err) {
			return ar, errSegmentGone
		}
		if err != nil {
			return ar, err
		}
		var saved []oas.AnalyticsFilter
		if err := json.Unmarshal(sg.Filters, &saved); err != nil {
			return ar, err
		}
		fs, err := segmentFilters(saved)
		if err != nil {
			return ar, err
		}
		q.Filters = append(q.Filters, fs...)
	}
	first := "visitors"
	if i := slices.IndexFunc(q.Metrics, func(m string) bool { return m != "revenue" }); i >= 0 && !slices.Contains(q.Metrics, first) {
		first = q.Metrics[i]
	}
	order := []analytics.Order{{Metric: first, Desc: true}}
	if support {
		order, ar.supportOrder = []analytics.Order{{Metric: "support_rate", Desc: true}, {Metric: "conversations", Desc: true}}, true
	}
	if b.OrderBy != nil && len(*b.OrderBy) > 0 {
		order, ar.supportOrder = nil, false
		for _, o := range *b.OrderBy {
			if o.Metric == oas.AnalyticsMetricRevenue {
				return ar, errValidation("rows cannot be ordered by revenue, which is per currency")
			}
			if err := checkDatasetMetric(support, string(o.Metric)); err != nil {
				return ar, err
			}
			order = append(order, analytics.Order{Metric: string(o.Metric), Desc: o.Dir == nil || *o.Dir == oas.AnalyticsDesc})
		}
	}
	for _, o := range order {
		q.SortBy = append(q.SortBy, o.Metric)
	}
	limit := defaultAnalyticsLimit
	if b.Limit != nil {
		limit = *b.Limit
	}
	if limit < 1 || limit > 1000 {
		return ar, errValidation("limit must be 1 to 1000")
	}
	offset := 0
	if b.Cursor != nil && *b.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(*b.Cursor)
		if err == nil {
			offset, err = strconv.Atoi(string(raw))
		}
		if err != nil || offset < 0 {
			return ar, errValidation("cursor is not one this query returned")
		}
	}
	if b.Compare != nil {
		cf, ct, err := comparisonRange(*b.Compare, from, to)
		if err != nil {
			return ar, err
		}
		c := q
		c.From, c.To = cf, ct
		ar.cmp = &c
	}
	ar.q, ar.order, ar.limit, ar.offset = q, order, limit, offset
	ar.insights = b.Insights != nil && *b.Insights
	return ar, nil
}

// analyticsResult answers a checked query; all leaves out the comparison and returns every row
// instead of one page.
func (s *Server) analyticsResult(ctx context.Context, prop store.AnalyticsProperty, ar analyticsRequest, all bool) (oas.AnalyticsResult, error) {
	q, cmp, order, limit, offset := ar.q, ar.cmp, ar.order, ar.limit, ar.offset
	if all {
		cmp = nil
	}
	ans, err := s.runAnalyticsQuery(ctx, prop, q, cmp)
	if err != nil {
		return oas.AnalyticsResult{}, err
	}
	res := ans.result
	rows := slices.Clone(res.Rows)
	if len(q.Dimensions) == 0 {
		rows = nil
	}
	sortAnalyticsRows(rows, q.Dimensions, order)
	if ar.supportOrder {
		sortSupportRows(rows, q.Dimensions)
	}
	var insights []oas.AnalyticsInsight
	if ar.insights && !all {
		insights = analyticsInsights(q, ans, order)
	}
	next := ""
	if !timeOnly(q.Dimensions) && !all {
		if offset > len(rows) {
			offset = len(rows)
		}
		end := min(offset+limit, len(rows))
		if end < len(rows) {
			next = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(end)))
		}
		rows = rows[offset:end]
	}
	out := oas.AnalyticsResult{
		Rows: make([]oas.AnalyticsRow, len(rows)), Totals: metricsBody(res.Totals, q.Metrics),
		Meta: oas.AnalyticsResultMeta{
			From: openapi_types.Date{Time: q.From}, To: openapi_types.Date{Time: q.To}, Timezone: prop.Timezone,
			ServedFrom: oas.AnalyticsResultMetaServedFrom(res.ServedFrom), RawWindowFrom: openapi_types.Date{Time: q.RawFrom},
		},
	}
	if next != "" {
		out.NextCursor = &next
	}
	if insights != nil {
		out.Insights = &insights
	}
	for i, r := range rows {
		out.Rows[i] = rowBody(r, q.Dimensions, q.Metrics)
	}
	if ans.comparison != nil {
		c := ans.comparison
		ti := timeIndex(q.Dimensions)
		var cur, prev map[string]int
		if ti >= 0 {
			cur, prev = bucketOrdinals(res.Rows, ti), bucketOrdinals(c.Rows, ti)
		}
		byKey := map[string]analytics.Row{}
		for _, r := range c.Rows {
			byKey[rowKey(r, ti, prev)] = r
		}
		comp := oas.AnalyticsComparison{From: openapi_types.Date{Time: cmp.From}, To: openapi_types.Date{Time: cmp.To},
			Totals: metricsBody(c.Totals, q.Metrics), Rows: make([]oas.AnalyticsRow, len(rows))}
		for i, r := range rows {
			m, ok := byKey[rowKey(r, ti, cur)]
			if !ok {
				m = analytics.Row{Dims: r.Dims}
			}
			comp.Rows[i] = rowBody(m, q.Dimensions, q.Metrics)
		}
		out.Comparison = &comp
	}
	return out, nil
}

// runAnalyticsQuery answers a query and its comparison from the cache or the store, at most four
// at a time per workspace. A cached answer is keyed by the property's rollup version, so a day
// rolled up again is never answered from before.
func (s *Server) runAnalyticsQuery(ctx context.Context, prop store.AnalyticsProperty, q analytics.Query, cmp *analytics.Query) (analyticsAnswer, error) {
	a := s.analytics
	now := s.now()
	key, _ := json.Marshal(struct {
		Q   analytics.Query
		C   *analytics.Query
		Day time.Time
	}{q, cmp, analytics.LocalDay(now, propertyLocation(prop.Timezone))})
	cacheKey := prop.ID.String() + "|" + strconv.Itoa(int(prop.RollupVersion)) + "|" + string(key)
	if v, ok := a.answers.get(cacheKey, now); ok {
		return v.(analyticsAnswer), nil
	}
	release, err := a.slots.acquire(ctx, prop.WorkspaceID)
	if err != nil {
		return analyticsAnswer{}, err
	}
	defer release()
	run := a.store.Query
	if q.Dataset == analytics.DatasetSupport {
		run = func(ctx context.Context, q analytics.Query) (analytics.Result, error) {
			return s.supportQuery(ctx, prop, q)
		}
	}
	var ans analyticsAnswer
	if ans.result, err = run(ctx, q); err != nil {
		return ans, analyticsQueryError(err)
	}
	ttl := analyticsFinishedTTL
	if ans.result.ServedFrom != analytics.ServedRollups {
		ttl = analyticsOpenTTL
	}
	if cmp != nil {
		c, err := run(ctx, *cmp)
		if err != nil {
			return ans, analyticsQueryError(err)
		}
		ans.comparison = &c
		if c.ServedFrom != analytics.ServedRollups {
			ttl = analyticsOpenTTL
		}
	}
	a.answers.put(cacheKey, ans, now, ttl)
	return ans, nil
}

func analyticsCSV(dims, metrics []string, rows []oas.AnalyticsRow) []byte {
	var buf bytes.Buffer
	_ = writeAnalyticsCSV(&buf, dims, metrics, rows)
	return buf.Bytes()
}

func writeAnalyticsCSV(w io.Writer, dims, metrics []string, rows []oas.AnalyticsRow) error {
	header := slices.Clone(dims)
	places := slices.ContainsFunc(dims, func(d string) bool { return d == "region" || d == "city" })
	if places {
		header = append(header, "country", "region")
	}
	header = append(header, metrics...)
	write := func(cells []string) error {
		for i, c := range cells {
			cells[i] = csvCell(c)
		}
		_, err := io.WriteString(w, strings.Join(cells, ",")+"\r\n")
		return err
	}
	if err := write(header); err != nil {
		return err
	}
	for _, r := range rows {
		cells := slices.Clone(r.Dimensions)
		if places {
			cells = append(cells, derefString(r.Country), derefString(r.Region))
		}
		raw, _ := json.Marshal(r.Metrics)
		var m map[string]float64
		_ = json.Unmarshal(raw, &m)
		for _, name := range metrics {
			cells = append(cells, strconv.FormatFloat(m[name], 'f', -1, 64))
		}
		if err := write(cells); err != nil {
			return err
		}
	}
	return nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func topValues(in []analytics.TopValue) []oas.AnalyticsTopValue {
	out := make([]oas.AnalyticsTopValue, len(in))
	for i, v := range in {
		out[i] = oas.AnalyticsTopValue{Value: v.Value, Visitors: v.Visitors}
	}
	return out
}

func (s *Server) GetAnalyticsRealtime(ctx context.Context, req oas.GetAnalyticsRealtimeRequestObject) (oas.GetAnalyticsRealtimeResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	out, err := s.analyticsRealtime(ctx, prop)
	if err != nil {
		return nil, err
	}
	return oas.GetAnalyticsRealtime200JSONResponse(out), nil
}

func (s *Server) analyticsRealtime(ctx context.Context, prop store.AnalyticsProperty) (oas.AnalyticsRealtime, error) {
	a, now := s.analytics, s.now()
	key := prop.ID.String() + "|realtime"
	if v, ok := a.answers.get(key, now); ok {
		return v.(oas.AnalyticsRealtime), nil
	}
	release, err := a.slots.acquire(ctx, prop.WorkspaceID)
	if err != nil {
		return oas.AnalyticsRealtime{}, err
	}
	defer release()
	rt, err := a.store.Realtime(ctx, prop.WorkspaceID, prop.ID, propertyLocation(prop.Timezone), now)
	if err != nil {
		return oas.AnalyticsRealtime{}, err
	}
	out := oas.AnalyticsRealtime{Visitors: rt.Visitors, Pageviews: rt.Pageviews, Minutes: make([]oas.AnalyticsMinute, len(rt.Minutes)),
		Pages: topValues(rt.Pages), Sources: topValues(rt.Sources), Countries: topValues(rt.Countries)}
	for i, m := range rt.Minutes {
		out.Minutes[i] = oas.AnalyticsMinute{At: m.At, Visitors: m.Visitors, Pageviews: m.Pageviews}
	}
	a.answers.put(key, out, now, analyticsRealtimeTTL)
	return out, nil
}

func (s *Server) GetAnalyticsHealth(ctx context.Context, req oas.GetAnalyticsHealthRequestObject) (oas.GetAnalyticsHealthResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	n := defaultHealthDays
	if req.Params.Days != nil {
		n = *req.Params.Days
	}
	if n < 1 || n > analyticsStatsDays {
		return nil, errValidation("days must be 1 to 90")
	}
	today := analytics.LocalDay(s.now(), propertyLocation(prop.Timezone))
	from := today.AddDate(0, 0, 1-n)
	stats, err := s.st.ListAnalyticsIngestStats(ctx, store.ListAnalyticsIngestStatsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, FromDay: from})
	if err != nil {
		return nil, err
	}
	pending, err := s.st.CountPendingAnalyticsRollups(ctx, store.CountPendingAnalyticsRollupsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID})
	if err != nil {
		return nil, err
	}
	byDay := map[time.Time]*oas.AnalyticsHealthDay{}
	out := oas.AnalyticsHealth{
		FirstEventAt: prop.FirstEventAt, LastEventAt: prop.LastEventAt, RawWindowFrom: openapi_types.Date{Time: s.rawFrom()},
		PendingRollupDays: pending, Geoip: s.geoIPBody(), Days: make([]oas.AnalyticsHealthDay, 0, n),
	}
	for d := today; !d.Before(from); d = d.AddDate(0, 0, -1) {
		out.Days = append(out.Days, oas.AnalyticsHealthDay{Day: openapi_types.Date{Time: d}})
	}
	for i := range out.Days {
		byDay[out.Days[i].Day.Time] = &out.Days[i]
	}
	for _, st := range stats {
		d := byDay[st.Day.UTC()]
		if d == nil {
			continue
		}
		addIngestCount(&d.Accepted, &d.Scrubbed, &d.Dropped, st.Reason, st.Events)
		addIngestCount(&out.Totals.Accepted, &out.Totals.Scrubbed, &out.Totals.Dropped, st.Reason, st.Events)
	}
	return oas.GetAnalyticsHealth200JSONResponse(out), nil
}

func addIngestCount(accepted, scrubbed *int64, dropped *oas.AnalyticsDropCounts, reason string, n int64) {
	switch reason {
	case analytics.ReasonAccepted:
		*accepted += n
	case analytics.ReasonScrubbed:
		*scrubbed += n
	case analytics.ReasonBot:
		dropped.Bot += n
	case analytics.ReasonRate:
		dropped.Rate += n
	case analytics.ReasonOrigin:
		dropped.Origin += n
	case analytics.ReasonInvalid:
		dropped.Invalid += n
	case analytics.ReasonLate:
		dropped.Late += n
	case analytics.ReasonOverLimit:
		dropped.OverLimit += n
	case analytics.ReasonBuffer:
		dropped.Buffer += n
	case analytics.ReasonExcluded:
		dropped.Excluded += n
	}
}

func (s *Server) geoIPBody() oas.AnalyticsGeoIP {
	info := s.analytics.geo.Info()
	out := oas.AnalyticsGeoIP{Source: oas.AnalyticsGeoIPSource(info.Source)}
	if !info.BuiltAt.IsZero() {
		out.BuiltAt = &info.BuiltAt
	}
	if info.Error != "" {
		out.Error = &info.Error
	}
	if info.Source == analytics.GeoDBIP {
		out.Attribution, out.AttributionUrl = new(analytics.DBIPAttribution), new(analytics.DBIPAttributionURL)
	}
	return out
}

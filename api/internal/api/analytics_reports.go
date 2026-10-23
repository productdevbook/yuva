package api

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	reportNone    = "none"
	reportWeekly  = "weekly"
	reportMonthly = "monthly"
	// Scheduled reports go out from this local hour of the first day of the week or month.
	reportHour    = 8
	reportTopRows = 5
	reportGoals   = 5

	eventAnalyticsAlert = string(oas.AnalyticsAlertEventTypeAnalyticsAlert)
)

var limitReportsPerMember = limit{5, time.Hour}

func analyticsSubscriptionBody(propertyID uuid.UUID, sub *store.AnalyticsSubscription) oas.AnalyticsSubscription {
	out := oas.AnalyticsSubscription{PropertyId: propertyID, Report: oas.AnalyticsReportNone, Alerts: []oas.AnalyticsAlertKind{}}
	if sub == nil {
		return out
	}
	out.Report, out.LastReportAt, out.UpdatedAt = oas.AnalyticsReportFrequency(sub.Report), sub.LastReportAt, &sub.UpdatedAt
	for _, a := range sub.Alerts {
		out.Alerts = append(out.Alerts, oas.AnalyticsAlertKind(a))
	}
	return out
}

func (s *Server) subscriptionOf(ctx context.Context, p principal, propertyID uuid.UUID) (*store.AnalyticsSubscription, error) {
	sub, err := s.st.GetAnalyticsSubscription(ctx, store.GetAnalyticsSubscriptionParams{WorkspaceID: p.workspaceID, PropertyID: propertyID, MemberID: p.memberID})
	if store.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (s *Server) GetAnalyticsSubscription(ctx context.Context, req oas.GetAnalyticsSubscriptionRequestObject) (oas.GetAnalyticsSubscriptionResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	sub, err := s.subscriptionOf(ctx, p, prop.ID)
	if err != nil {
		return nil, err
	}
	return oas.GetAnalyticsSubscription200JSONResponse(analyticsSubscriptionBody(prop.ID, sub)), nil
}

func (s *Server) UpdateAnalyticsSubscription(ctx context.Context, req oas.UpdateAnalyticsSubscriptionRequestObject) (oas.UpdateAnalyticsSubscriptionResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	b := req.Body
	if b.Report == nil && b.Alerts == nil {
		return nil, errValidation("send report or alerts")
	}
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	sub, err := s.subscriptionOf(ctx, p, prop.ID)
	if err != nil {
		return nil, err
	}
	report, alerts := reportNone, []string{}
	if sub != nil {
		report, alerts = sub.Report, sub.Alerts
	}
	if b.Report != nil {
		if !b.Report.Valid() {
			return nil, errValidation("report must be none, weekly or monthly")
		}
		report = string(*b.Report)
	}
	if b.Alerts != nil {
		alerts = []string{}
		for _, a := range *b.Alerts {
			if !a.Valid() {
				return nil, errValidation("alerts lists spike, drop, goal_drop, support_load and new_referrer")
			}
			if !slices.Contains(alerts, string(a)) {
				alerts = append(alerts, string(a))
			}
		}
	}
	out, err := s.st.UpsertAnalyticsSubscription(ctx, store.UpsertAnalyticsSubscriptionParams{
		WorkspaceID: p.workspaceID, PropertyID: prop.ID, MemberID: p.memberID, Report: report, Alerts: alerts, Now: s.now(),
	})
	if err != nil {
		return nil, err
	}
	return oas.UpdateAnalyticsSubscription200JSONResponse(analyticsSubscriptionBody(prop.ID, &out)), nil
}

func (s *Server) SendAnalyticsReport(ctx context.Context, req oas.SendAnalyticsReportRequestObject) (oas.SendAnalyticsReportResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	if err := s.rateLimit(ctx, rateCheck{"analytics-report:" + p.memberID.String(), limitReportsPerMember}); err != nil {
		return nil, err
	}
	sub, err := s.subscriptionOf(ctx, p, prop.ID)
	if err != nil {
		return nil, err
	}
	freq := reportWeekly
	if sub != nil && sub.Report == reportMonthly {
		freq = reportMonthly
	}
	addr, err := s.st.GetMemberAddress(ctx, store.GetMemberAddressParams{WorkspaceID: p.workspaceID, ID: p.memberID})
	if err != nil {
		return nil, err
	}
	from, to := reportRange(freq, analytics.LocalDay(s.now(), propertyLocation(prop.Timezone)), true)
	msg, err := s.renderAnalyticsReport(ctx, prop, freq, from, to, addr.Locale)
	if err != nil {
		return nil, err
	}
	msg.To = addr.Email
	s.sendMail(ctx, msg)
	return oas.SendAnalyticsReport202Response{}, nil
}

func alertBody(a store.AnalyticsAlert) oas.AnalyticsAlert {
	return oas.AnalyticsAlert{
		Id: a.ID, PropertyId: a.PropertyID, Kind: oas.AnalyticsAlertKind(a.Kind), Day: openapi_types.Date{Time: a.Day}, HoursFrom: a.HoursFrom,
		HoursTo: a.HoursTo, Metric: a.Metric, Current: a.Current, Baseline: a.Baseline, GoalId: a.GoalID, GoalName: a.GoalName,
		Referrer: a.Referrer, Text: a.Text, AnnotationId: a.AnnotationID, CreatedAt: a.CreatedAt,
	}
}

func (s *Server) ListAnalyticsAlerts(ctx context.Context, req oas.ListAnalyticsAlertsRequestObject) (oas.ListAnalyticsAlertsResponseObject, error) {
	if err := s.analyticsOn(); err != nil {
		return nil, err
	}
	p := principalFrom(ctx)
	prop, err := visibleProperty(ctx, s.st.Queries, p, req.PropertyId, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.ListAnalyticsAlerts(ctx, store.ListAnalyticsAlertsParams{WorkspaceID: p.workspaceID, PropertyID: prop.ID, Before: req.Params.Before})
	if err != nil {
		return nil, err
	}
	out := oas.ListAnalyticsAlerts200JSONResponse{Items: make([]oas.AnalyticsAlert, len(rows))}
	for i, a := range rows {
		out.Items[i] = alertBody(a)
	}
	return out, nil
}

// reportRange is the period a report covers: the week (Monday to Sunday) or calendar month before
// today's, or with recent the seven finished days or the month before.
func reportRange(freq string, today time.Time, recent bool) (time.Time, time.Time) {
	if freq == reportMonthly {
		first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first.AddDate(0, -1, 0), first.AddDate(0, 0, -1)
	}
	if recent {
		return today.AddDate(0, 0, -7), today.AddDate(0, 0, -1)
	}
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, -7), monday.AddDate(0, 0, -1)
}

type reportMetric struct {
	Value, Change string
}

type reportRow struct {
	Name, Value, Change string
}

type reportSupport struct {
	Conversations, Feedback, Up, Down string
	TopPage, TopCount, TopRate        string
}

type reportData struct {
	Property, From, To                      string
	Monthly                                 bool
	Visitors, Visits, Pageviews, BounceRate reportMetric
	Duration                                reportMetric
	Pages, Sources, Goals                   []reportRow
	Support                                 *reportSupport
	URL, SettingsURL                        string
}

// reportNumber writes a count with the thousands separator of the language.
func reportNumber(v float64, locale string) string {
	sep := ","
	if l := mailLocale(locale); l == "tr" || l == "de" {
		sep = "."
	}
	return strings.ReplaceAll(formatCount(int64(math.Round(v))), ",", sep)
}

func mailLocale(locale string) string {
	base, _, _ := strings.Cut(strings.ReplaceAll(strings.ToLower(locale), "_", "-"), "-")
	return base
}

// reportPercent writes a percentage as the language does: 12%, %12, 12 %.
func reportPercent(v float64, locale string) string {
	n := strconv.FormatFloat(v, 'f', 0, 64)
	switch mailLocale(locale) {
	case "tr":
		return "%" + n
	case "de":
		return n + " %"
	}
	return n + "%"
}

func reportDecimal(v float64, locale string) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if l := mailLocale(locale); l == "tr" || l == "de" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func reportChange(cur, prev float64, locale string) string {
	if prev == 0 {
		return ""
	}
	pct := math.Round((cur - prev) / prev * 100)
	switch {
	case pct > 0:
		return "+" + reportPercent(pct, locale)
	case pct < 0:
		return "−" + reportPercent(-pct, locale)
	}
	return "±" + reportPercent(0, locale)
}

func reportDuration(sec float64) string {
	n := int(math.Round(sec))
	return fmt.Sprintf("%d:%02d", n/60, n%60)
}

func (s *Server) panelPropertyURL(ws, propertyID uuid.UUID, view string) string {
	path := "/analytics/" + propertyID.String()
	if view != "" {
		path += "/" + view
	}
	return s.auth.PublicURL + path + "?workspace=" + ws.String()
}

// renderAnalyticsReport builds a report mail of the days from to: plain text and HTML without
// images, and links to the panel only.
func (s *Server) renderAnalyticsReport(ctx context.Context, prop store.AnalyticsProperty, freq string, from, to time.Time, locale string) (mail.Message, error) {
	base := analytics.Query{WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID, Timezone: prop.Timezone, From: from, To: to, RawFrom: s.rawFrom()}
	n := int(to.Sub(from).Hours()/24) + 1
	prev := base
	prev.From, prev.To = from.AddDate(0, 0, -n), from.AddDate(0, 0, -1)
	if freq == reportMonthly {
		prev.From, prev.To = from.AddDate(0, -1, 0), from.AddDate(0, 0, -1)
	}
	totals := base
	totals.Metrics = []string{"visitors", "visits", "pageviews", "bounce_rate", "visit_duration"}
	cmpTotals := prev
	cmpTotals.Metrics = totals.Metrics
	ans, err := s.runAnalyticsQuery(ctx, prop, totals, &cmpTotals)
	if err != nil {
		return mail.Message{}, err
	}
	cur, old := ans.result.Totals, ans.comparison.Totals
	metric := func(name string) reportMetric {
		return reportMetric{Value: reportNumber(cur.Metric(name), locale), Change: reportChange(cur.Metric(name), old.Metric(name), locale)}
	}
	d := reportData{
		Property: prop.Name, From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Monthly: freq == reportMonthly,
		Visitors: metric("visitors"), Visits: metric("visits"), Pageviews: metric("pageviews"),
		BounceRate: reportMetric{Value: reportPercent(cur.Metric("bounce_rate"), locale)},
		Duration:   reportMetric{Value: reportDuration(cur.Metric("visit_duration"))},
		URL:        s.panelPropertyURL(prop.WorkspaceID, prop.ID, ""), SettingsURL: s.panelPropertyURL(prop.WorkspaceID, prop.ID, "settings"),
		Pages: []reportRow{}, Sources: []reportRow{}, Goals: []reportRow{},
	}
	top := func(dim, metric string) ([]reportRow, error) {
		q := base
		q.Metrics, q.Dimensions, q.SortBy = []string{metric}, []string{dim}, []string{metric}
		a, err := s.runAnalyticsQuery(ctx, prop, q, nil)
		if err != nil {
			return nil, err
		}
		rows := slices.Clone(a.result.Rows)
		analytics.SortRows(rows, []analytics.Order{{Metric: metric, Desc: true}})
		out := []reportRow{}
		for _, r := range rows {
			if len(out) == reportTopRows {
				break
			}
			if r.Dims[0] == analytics.OtherValue || r.Sums.Metric(metric) == 0 {
				continue
			}
			name := r.Dims[0]
			if name == "" {
				name = "(direct)"
			}
			out = append(out, reportRow{Name: name, Value: reportNumber(r.Sums.Metric(metric), locale)})
		}
		return out, nil
	}
	if d.Pages, err = top("page", "pageviews"); err != nil {
		return mail.Message{}, err
	}
	if d.Sources, err = top("source", "visits"); err != nil {
		return mail.Message{}, err
	}
	goals, err := s.st.ListAnalyticsGoals(ctx, store.ListAnalyticsGoalsParams{WorkspaceID: prop.WorkspaceID, PropertyID: prop.ID})
	if err != nil {
		return mail.Message{}, err
	}
	for _, g := range goals[:min(len(goals), reportGoals)] {
		q, c := base, prev
		q.Metrics, q.Goal = []string{"conversions"}, goalDef(g)
		c.Metrics, c.Goal = q.Metrics, q.Goal
		a, err := s.runAnalyticsQuery(ctx, prop, q, &c)
		if err != nil {
			return mail.Message{}, err
		}
		v := a.result.Totals.Metric("conversions")
		d.Goals = append(d.Goals, reportRow{Name: g.Name, Value: reportNumber(v, locale), Change: reportChange(v, a.comparison.Totals.Metric("conversions"), locale)})
	}
	if prop.InboxID != nil {
		q := base
		q.Dataset, q.Dimensions = analytics.DatasetSupport, []string{"page"}
		q.Metrics = []string{"conversations", "feedback", "ratings_up", "ratings_down", "pageviews", "support_rate"}
		q.SortBy = []string{"conversations"}
		a, err := s.runAnalyticsQuery(ctx, prop, q, nil)
		if err != nil {
			return mail.Message{}, err
		}
		t := a.result.Totals
		sp := &reportSupport{Conversations: reportNumber(float64(t.Conversations), locale), Feedback: reportNumber(float64(t.Feedback), locale),
			Up: reportNumber(float64(t.RatingsUp), locale), Down: reportNumber(float64(t.RatingsDown), locale)}
		rows := slices.Clone(a.result.Rows)
		analytics.SortRows(rows, []analytics.Order{{Metric: "conversations", Desc: true}})
		if len(rows) > 0 && rows[0].Sums.Conversations > 0 {
			r := rows[0]
			sp.TopPage, sp.TopCount = r.Dims[0], reportNumber(float64(r.Sums.Conversations), locale)
			sp.TopRate = reportDecimal(r.Sums.Metric("support_rate"), locale)
		}
		if t.Conversations+t.Feedback+t.RatingsUp+t.RatingsDown+t.Discussions > 0 {
			d.Support = sp
		}
	}
	return mail.Render("analytics_report", locale, d)
}

type analyticsReportsWorker struct {
	river.WorkerDefaults[jobs.AnalyticsReportsArgs]
	s *Server
}

func (w *analyticsReportsWorker) Work(ctx context.Context, _ *river.Job[jobs.AnalyticsReportsArgs]) error {
	_, err := w.s.SendDueAnalyticsReports(ctx)
	return err
}

type analyticsAlertsWorker struct {
	river.WorkerDefaults[jobs.AnalyticsAlertsArgs]
	s *Server
}

func (w *analyticsAlertsWorker) Work(ctx context.Context, _ *river.Job[jobs.AnalyticsAlertsArgs]) error {
	_, err := w.s.CheckAnalyticsAlerts(ctx)
	return err
}

// SendDueAnalyticsReports mails every report whose period ended and whose first day's report hour
// has come, once per member and period; it returns how many it sent.
func (s *Server) SendDueAnalyticsReports(ctx context.Context) (int, error) {
	if s.analytics == nil {
		return 0, nil
	}
	ids, err := s.st.ListAnalyticsReportWorkspaceIDs(ctx)
	if err != nil {
		return 0, err
	}
	now, sent := s.now(), 0
	for _, ws := range ids {
		due, err := s.st.ListAnalyticsReportsDue(ctx, ws)
		if err != nil {
			return sent, err
		}
		addrs := map[uuid.UUID]map[uuid.UUID]store.ListAnalyticsSubscriberAddressesRow{}
		for _, d := range due {
			loc := propertyLocation(d.Timezone)
			today := analytics.LocalDay(now, loc)
			from, to := reportRange(d.Report, today, false)
			start := to.AddDate(0, 0, 1)
			if now.Before(time.Date(start.Year(), start.Month(), start.Day(), reportHour, 0, 0, 0, loc)) {
				continue
			}
			if d.LastPeriod != nil && !d.LastPeriod.Before(from) {
				continue
			}
			if addrs[d.PropertyID] == nil {
				list, err := s.st.ListAnalyticsSubscriberAddresses(ctx, store.ListAnalyticsSubscriberAddressesParams{WorkspaceID: ws, PropertyID: d.PropertyID})
				if err != nil {
					return sent, err
				}
				addrs[d.PropertyID] = map[uuid.UUID]store.ListAnalyticsSubscriberAddressesRow{}
				for _, a := range list {
					addrs[d.PropertyID][a.MemberID] = a
				}
			}
			a, ok := addrs[d.PropertyID][d.MemberID]
			if !ok {
				continue
			}
			n, err := s.st.ClaimAnalyticsReport(ctx, store.ClaimAnalyticsReportParams{WorkspaceID: ws, PropertyID: d.PropertyID, MemberID: d.MemberID,
				Report: d.Report, Period: from, Now: now})
			if err != nil {
				return sent, err
			}
			if n == 0 {
				continue
			}
			prop, err := s.st.GetAnalyticsProperty(ctx, store.GetAnalyticsPropertyParams{WorkspaceID: ws, ID: d.PropertyID})
			if store.IsNotFound(err) {
				continue
			}
			if err != nil {
				return sent, err
			}
			msg, err := s.renderAnalyticsReport(ctx, prop, d.Report, from, to, a.Locale)
			if err != nil {
				s.log.WarnContext(ctx, "analytics report", slog.String("workspace_id", ws.String()), slog.Any("error", err))
				continue
			}
			msg.To = a.Email
			if err := s.mailer.Send(ctx, msg); err != nil {
				s.log.WarnContext(ctx, "analytics report", slog.String("workspace_id", ws.String()), slog.Any("error", err))
				continue
			}
			sent++
		}
	}
	return sent, nil
}

package api

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/store"
)

const reportMaxMonths = 13

var allReportMetrics = []oas.ReportMetric{
	oas.ReportMetricOpened, oas.ReportMetricClosed, oas.ReportMetricReopened, oas.ReportMetricReplies,
	oas.ReportMetricFirstReply, oas.ReportMetricResolution, oas.ReportMetricSla, oas.ReportMetricRatings,
}

var slaReportTargets = []string{"first_reply", "next_reply", "resolution"}

// storedReportMetrics are the report_rollups metrics a requested metric reads.
func storedReportMetrics(m oas.ReportMetric) []string {
	switch m {
	case oas.ReportMetricSla:
		var out []string
		for _, t := range slaReportTargets {
			out = append(out, "sla_"+t+"_met", "sla_"+t+"_breached")
		}
		return out
	case oas.ReportMetricRatings:
		return []string{"rating_good", "rating_bad"}
	}
	return []string{string(m)}
}

type reportQuery struct {
	from, to    time.Time
	tz          string
	loc         *time.Location
	grain       oas.ReportGranularity
	metrics     []oas.ReportMetric
	groupBy     *oas.ReportGroupBy
	since       time.Time
	until       time.Time
	builtUntil  *time.Time
	arg         store.ReportRowsParams
	splitTotals bool
}

func (s *Server) reportQuery(ctx context.Context, p principal, prm oas.GetReportParams) (reportQuery, error) {
	rq := reportQuery{tz: "UTC", grain: oas.ReportGranularityDay, groupBy: prm.GroupBy}
	if prm.Timezone != nil {
		rq.tz = *prm.Timezone
	}
	if err := validTimezone(rq.tz); err != nil {
		return rq, err
	}
	rq.loc, _ = time.LoadLocation(rq.tz)
	if prm.Granularity != nil {
		if !prm.Granularity.Valid() {
			return rq, errValidation("granularity must be day, week or month")
		}
		rq.grain = *prm.Granularity
	}
	if prm.GroupBy != nil && !prm.GroupBy.Valid() {
		return rq, errValidation("group_by must be inbox, channel, label, team or member")
	}
	if prm.ChannelKind != nil && !prm.ChannelKind.Valid() {
		return rq, errValidation("channel_kind must be email, chat, app or api")
	}
	names, err := enumList(prm.Metrics, "metrics must be opened, closed, reopened, replies, first_reply, resolution, sla or ratings")
	if err != nil {
		return rq, err
	}
	for _, m := range allReportMetrics {
		if names == nil || slices.Contains(names, string(m)) {
			rq.metrics = append(rq.metrics, m)
		}
	}
	rq.from, rq.to = prm.From.Time, prm.To.Time
	if rq.to.Before(rq.from) {
		return rq, errValidation("to must not be before from")
	}
	if !rq.to.Before(rq.from.AddDate(0, reportMaxMonths, 0)) {
		return rq, errValidation("a report covers at most 13 months")
	}
	if prm.InboxId != nil {
		if _, err := visibleInbox(ctx, s.st.Queries, p, *prm.InboxId); err != nil {
			return rq, err
		}
	}
	if prm.LabelId != nil {
		if _, err := s.st.GetLabel(ctx, store.GetLabelParams{WorkspaceID: p.workspaceID, ID: *prm.LabelId}); store.IsNotFound(err) {
			return rq, errLabelGone
		} else if err != nil {
			return rq, err
		}
	}
	if prm.TeamId != nil {
		if _, err := s.st.GetTeam(ctx, store.GetTeamParams{WorkspaceID: p.workspaceID, ID: *prm.TeamId}); store.IsNotFound(err) {
			return rq, errTeamGone
		} else if err != nil {
			return rq, err
		}
	}
	if prm.MemberId != nil {
		if _, err := s.st.GetMember(ctx, store.GetMemberParams{WorkspaceID: p.workspaceID, ID: *prm.MemberId}); store.IsNotFound(err) {
			return rq, errMemberGone
		} else if err != nil {
			return rq, err
		}
	}
	rq.since = time.Date(rq.from.Year(), rq.from.Month(), rq.from.Day(), 0, 0, 0, 0, rq.loc)
	rq.until = time.Date(rq.to.Year(), rq.to.Month(), rq.to.Day()+1, 0, 0, 0, 0, rq.loc)
	if now := s.now().Add(time.Microsecond); rq.until.After(now) {
		rq.until = now
	}
	var stored []string
	for _, m := range rq.metrics {
		stored = append(stored, storedReportMetrics(m)...)
	}
	group := ""
	if prm.GroupBy != nil {
		group = string(*prm.GroupBy)
	}
	rq.arg = store.ReportRowsParams{
		Grain: string(rq.grain), Timezone: rq.tz, GroupBy: group, WorkspaceID: p.workspaceID,
		ByLabel: group == string(oas.ReportGroupByLabel) || prm.LabelId != nil, Metrics: stored,
		LabelID: prm.LabelId, InboxID: prm.InboxId, TeamID: prm.TeamId, MemberID: prm.MemberId,
		AllInboxes: p.seesAllInboxes(), ViewerID: p.viewerID(), Since: rq.since, Until: rq.until, Built: rq.since,
	}
	if prm.ChannelKind != nil {
		ck := string(*prm.ChannelKind)
		rq.arg.ChannelKind = &ck
	}
	rq.splitTotals = group == string(oas.ReportGroupByLabel) && prm.LabelId == nil
	return rq, nil
}

// reportTail fills the rows after built_until from the conversations themselves, when that is
// less than a day of activity.
func (s *Server) reportTail(ctx context.Context, rq *reportQuery) error {
	st, err := s.st.GetReportRollupState(ctx, rq.arg.WorkspaceID)
	if store.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	built := st.BuiltUntil.UTC()
	rq.builtUntil = &built
	rq.arg.Built = built
	from := later(rq.since, &built)
	if s.now().Sub(built) > reportTailMax || !rq.until.After(from) {
		return nil
	}
	cells, err := s.reportFacts(ctx, s.st.Queries, rq.arg.WorkspaceID, from, rq.until)
	if err != nil {
		return err
	}
	a := &rq.arg
	for k, n := range cells {
		a.TailBucket = append(a.TailBucket, k.bucket)
		a.TailInbox = append(a.TailInbox, k.inbox)
		a.TailChannel = append(a.TailChannel, k.channel)
		a.TailTeam = append(a.TailTeam, idText(k.team))
		a.TailMember = append(a.TailMember, idText(k.member))
		a.TailLabel = append(a.TailLabel, idText(k.label))
		a.TailMetric = append(a.TailMetric, k.metric)
		a.TailBin = append(a.TailBin, k.bin)
		a.TailN = append(a.TailN, n)
	}
	return nil
}

func idText(id uuid.UUID) string {
	if id == uuid.Nil() {
		return ""
	}
	return id.String()
}

// reportAcc holds one group's or period's sums: counts per metric and histogram bins per duration.
type reportAcc struct {
	counts map[string]int64
	bins   map[string]map[int16]int64
}

func newReportAcc() *reportAcc {
	return &reportAcc{counts: map[string]int64{}, bins: map[string]map[int16]int64{}}
}

func (a *reportAcc) add(metric string, bin int16, n int64) {
	a.counts[metric] += n
	if slices.Contains(reportDurationMetrics, metric) {
		if a.bins[metric] == nil {
			a.bins[metric] = map[int16]int64{}
		}
		a.bins[metric][bin] += n
	}
}

// percentile interpolates between the samples on either side of p, each taken at its bin's
// middle, so an even count's median is the mean of the two middle samples as /v1/stats has it.
func percentile(bins map[int16]int64, p float64) *int64 {
	var total int64
	keys := make([]int16, 0, len(bins))
	for b, n := range bins {
		total += n
		keys = append(keys, b)
	}
	if total == 0 {
		return nil
	}
	slices.Sort(keys)
	at := func(i int64) float64 {
		var seen int64
		for _, b := range keys {
			seen += bins[b]
			if i < seen {
				return reportBinSeconds(b)
			}
		}
		return reportBinSeconds(keys[len(keys)-1])
	}
	pos := p * float64(total-1)
	lo := int64(math.Floor(pos))
	v := at(lo)
	if frac := pos - float64(lo); frac > 0 {
		v += (at(lo+1) - v) * frac
	}
	out := int64(math.Round(v))
	return &out
}

func (a *reportAcc) values(metrics []oas.ReportMetric) oas.ReportValues {
	var v oas.ReportValues
	count := func(m string) *int64 {
		n := a.counts[m]
		return &n
	}
	for _, m := range metrics {
		switch m {
		case oas.ReportMetricOpened:
			v.Opened = count("opened")
		case oas.ReportMetricClosed:
			v.Closed = count("closed")
		case oas.ReportMetricReopened:
			v.Reopened = count("reopened")
		case oas.ReportMetricReplies:
			v.Replies = count("replies")
		case oas.ReportMetricFirstReply:
			v.FirstReplies = count("first_reply")
			v.FirstReplyMedianSeconds = percentile(a.bins["first_reply"], 0.5)
			v.FirstReplyP90Seconds = percentile(a.bins["first_reply"], 0.9)
		case oas.ReportMetricResolution:
			v.Resolutions = count("resolution")
			v.ResolutionMedianSeconds = percentile(a.bins["resolution"], 0.5)
			v.ResolutionP90Seconds = percentile(a.bins["resolution"], 0.9)
		case oas.ReportMetricSla:
			t := func(name string) oas.ReportSlaTarget {
				return oas.ReportSlaTarget{Met: a.counts["sla_"+name+"_met"], Breached: a.counts["sla_"+name+"_breached"]}
			}
			v.Sla = &oas.ReportSla{FirstReply: t("first_reply"), NextReply: t("next_reply"), Resolution: t("resolution")}
		case oas.ReportMetricRatings:
			v.Ratings = &oas.ReportRatings{Good: a.counts["rating_good"], Bad: a.counts["rating_bad"]}
		}
	}
	return v
}

// reportPeriods lists the starts of the periods from rq.from to rq.to, the first cut at from.
func reportPeriods(rq reportQuery) []time.Time {
	var out []time.Time
	for d := rq.from; !d.After(rq.to); {
		out = append(out, d)
		switch rq.grain {
		case oas.ReportGranularityWeek:
			d = d.AddDate(0, 0, 7-(int(d.Weekday())+6)%7)
		case oas.ReportGranularityMonth:
			d = time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		default:
			d = d.AddDate(0, 0, 1)
		}
	}
	return out
}

type reportGroupSums struct {
	key     string
	total   *reportAcc
	periods []*reportAcc
}

func newReportGroupSums(key string, n int) *reportGroupSums {
	g := &reportGroupSums{key: key, total: newReportAcc(), periods: make([]*reportAcc, n)}
	for i := range g.periods {
		g.periods[i] = newReportAcc()
	}
	return g
}

func (g *reportGroupSums) series(starts []time.Time, metrics []oas.ReportMetric) []oas.ReportPeriod {
	out := make([]oas.ReportPeriod, len(starts))
	for i, st := range starts {
		out[i] = oas.ReportPeriod{Start: openapi_types.Date{Time: st}, Values: g.periods[i].values(metrics)}
	}
	return out
}

// periodIndex finds the period of a row's truncated local start; a week or month that began
// before from is the first period.
func periodIndex(starts []time.Time, period time.Time) int {
	i, _ := slices.BinarySearchFunc(starts, period, func(a, b time.Time) int { return a.Compare(b) })
	if i < len(starts) && starts[i].Equal(period) {
		return i
	}
	return max(i-1, 0)
}

func sumReportRows(rows []store.ReportRowsRow, starts []time.Time, groups map[string]*reportGroupSums) {
	for _, r := range rows {
		g := groups[r.Grp]
		if g == nil {
			g = newReportGroupSums(r.Grp, len(starts))
			groups[r.Grp] = g
		}
		g.total.add(r.Metric, r.Bin, r.N)
		g.periods[periodIndex(starts, r.Period)].add(r.Metric, r.Bin, r.N)
	}
}

type reportResult struct {
	report oas.Report
	starts []time.Time
	groups []*reportGroupSums
}

func (s *Server) runReport(ctx context.Context, p principal, prm oas.GetReportParams) (reportResult, error) {
	rq, err := s.reportQuery(ctx, p, prm)
	if err != nil {
		return reportResult{}, err
	}
	if err := s.reportTail(ctx, &rq); err != nil {
		return reportResult{}, err
	}
	rows, err := s.st.ReportRows(ctx, rq.arg)
	if err != nil {
		return reportResult{}, err
	}
	starts := reportPeriods(rq)
	byKey := map[string]*reportGroupSums{}
	sumReportRows(rows, starts, byKey)
	all := newReportGroupSums("", len(starts))
	if rq.splitTotals {
		arg := rq.arg
		arg.GroupBy, arg.ByLabel = "", false
		totals, err := s.st.ReportRows(ctx, arg)
		if err != nil {
			return reportResult{}, err
		}
		sumReportRows(totals, starts, map[string]*reportGroupSums{"": all})
	} else {
		for _, g := range byKey {
			mergeAcc(all.total, g.total)
			for i := range g.periods {
				mergeAcc(all.periods[i], g.periods[i])
			}
		}
	}
	out := reportResult{starts: starts}
	out.report = oas.Report{
		From: openapi_types.Date{Time: rq.from}, To: openapi_types.Date{Time: rq.to}, Timezone: rq.tz, Granularity: rq.grain,
		Metrics: rq.metrics, GroupBy: rq.groupBy, BuiltUntil: rq.builtUntil,
		Totals: all.total.values(rq.metrics), Series: all.series(starts, rq.metrics),
	}
	if rq.groupBy != nil {
		for _, g := range byKey {
			out.groups = append(out.groups, g)
		}
		slices.SortFunc(out.groups, func(a, b *reportGroupSums) int {
			return cmp.Or(cmp.Compare(boolInt(a.key == ""), boolInt(b.key == "")), strings.Compare(a.key, b.key))
		})
		groups := make([]oas.ReportGroup, 0, len(out.groups))
		for _, g := range out.groups {
			rg := oas.ReportGroup{Totals: g.total.values(rq.metrics), Series: g.series(starts, rq.metrics)}
			if g.key != "" {
				rg.Key = &g.key
			}
			groups = append(groups, rg)
		}
		out.report.Groups = &groups
	}
	return out, nil
}

func mergeAcc(dst, src *reportAcc) {
	for m := range src.counts {
		if bins, ok := src.bins[m]; ok {
			for b, n := range bins {
				dst.add(m, b, n)
			}
			continue
		}
		dst.add(m, 0, src.counts[m])
	}
}

func (s *Server) GetReport(ctx context.Context, req oas.GetReportRequestObject) (oas.GetReportResponseObject, error) {
	res, err := s.runReport(ctx, principalFrom(ctx), req.Params)
	if err != nil {
		return nil, err
	}
	return oas.GetReport200JSONResponse(res.report), nil
}

var reportCSVColumns = []struct {
	metric oas.ReportMetric
	name   string
	value  func(v oas.ReportValues) *int64
}{
	{oas.ReportMetricOpened, "opened", func(v oas.ReportValues) *int64 { return v.Opened }},
	{oas.ReportMetricClosed, "closed", func(v oas.ReportValues) *int64 { return v.Closed }},
	{oas.ReportMetricReopened, "reopened", func(v oas.ReportValues) *int64 { return v.Reopened }},
	{oas.ReportMetricReplies, "replies", func(v oas.ReportValues) *int64 { return v.Replies }},
	{oas.ReportMetricFirstReply, "first_replies", func(v oas.ReportValues) *int64 { return v.FirstReplies }},
	{oas.ReportMetricFirstReply, "first_reply_median_seconds", func(v oas.ReportValues) *int64 { return v.FirstReplyMedianSeconds }},
	{oas.ReportMetricFirstReply, "first_reply_p90_seconds", func(v oas.ReportValues) *int64 { return v.FirstReplyP90Seconds }},
	{oas.ReportMetricResolution, "resolutions", func(v oas.ReportValues) *int64 { return v.Resolutions }},
	{oas.ReportMetricResolution, "resolution_median_seconds", func(v oas.ReportValues) *int64 { return v.ResolutionMedianSeconds }},
	{oas.ReportMetricResolution, "resolution_p90_seconds", func(v oas.ReportValues) *int64 { return v.ResolutionP90Seconds }},
	{oas.ReportMetricSla, "sla_first_reply_met", func(v oas.ReportValues) *int64 { return &v.Sla.FirstReply.Met }},
	{oas.ReportMetricSla, "sla_first_reply_breached", func(v oas.ReportValues) *int64 { return &v.Sla.FirstReply.Breached }},
	{oas.ReportMetricSla, "sla_next_reply_met", func(v oas.ReportValues) *int64 { return &v.Sla.NextReply.Met }},
	{oas.ReportMetricSla, "sla_next_reply_breached", func(v oas.ReportValues) *int64 { return &v.Sla.NextReply.Breached }},
	{oas.ReportMetricSla, "sla_resolution_met", func(v oas.ReportValues) *int64 { return &v.Sla.Resolution.Met }},
	{oas.ReportMetricSla, "sla_resolution_breached", func(v oas.ReportValues) *int64 { return &v.Sla.Resolution.Breached }},
	{oas.ReportMetricRatings, "ratings_good", func(v oas.ReportValues) *int64 { return &v.Ratings.Good }},
	{oas.ReportMetricRatings, "ratings_bad", func(v oas.ReportValues) *int64 { return &v.Ratings.Bad }},
}

func (s *Server) ExportReport(ctx context.Context, req oas.ExportReportRequestObject) (oas.ExportReportResponseObject, error) {
	p := principalFrom(ctx)
	res, err := s.runReport(ctx, p, oas.GetReportParams(req.Params))
	if err != nil {
		return nil, err
	}
	r := res.report
	var cols []int
	for i, c := range reportCSVColumns {
		if slices.Contains(r.Metrics, c.metric) {
			cols = append(cols, i)
		}
	}
	var b bytes.Buffer
	line := func(cells []string) {
		for i, c := range cells {
			cells[i] = csvCell(c)
		}
		b.WriteString(strings.Join(cells, ",") + "\r\n")
	}
	head := []string{"period"}
	if r.GroupBy != nil {
		head = append(head, "group_id", "group_name")
	}
	for _, i := range cols {
		head = append(head, reportCSVColumns[i].name)
	}
	line(head)
	row := func(pre []string, v oas.ReportValues, start time.Time) {
		cells := append([]string{start.Format(time.DateOnly)}, pre...)
		for _, i := range cols {
			if n := reportCSVColumns[i].value(v); n != nil {
				cells = append(cells, strconv.FormatInt(*n, 10))
			} else {
				cells = append(cells, "")
			}
		}
		line(cells)
	}
	if r.Groups == nil {
		for _, per := range r.Series {
			row(nil, per.Values, per.Start.Time)
		}
	} else {
		names, err := s.reportGroupNames(ctx, p.workspaceID, *r.GroupBy, *r.Groups)
		if err != nil {
			return nil, err
		}
		for _, g := range *r.Groups {
			id, name := "", ""
			if g.Key != nil {
				id, name = *g.Key, names[*g.Key]
			}
			for _, per := range g.Series {
				row([]string{id, name}, per.Values, per.Start.Time)
			}
		}
	}
	body := b.Bytes()
	name := `attachment; filename="report-` + r.From.Format(time.DateOnly) + `-` + r.To.Format(time.DateOnly) + `.csv"`
	return oas.ExportReport200TextcsvResponse{Body: bytes.NewReader(body), ContentLength: int64(len(body)),
		Headers: oas.ExportReport200ResponseHeaders{ContentDisposition: &name}}, nil
}

func (s *Server) reportGroupNames(ctx context.Context, ws uuid.UUID, by oas.ReportGroupBy, groups []oas.ReportGroup) (map[string]string, error) {
	out := map[string]string{}
	var ids []uuid.UUID
	for _, g := range groups {
		if g.Key == nil {
			continue
		}
		if by == oas.ReportGroupByChannel {
			out[*g.Key] = *g.Key
		} else if id, err := uuid.Parse(*g.Key); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.st.ReportNames(ctx, store.ReportNamesParams{WorkspaceID: ws, Ids: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID.String()] = r.Name
	}
	return out, nil
}

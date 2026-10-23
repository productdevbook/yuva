package api

import (
	"context"
	"strings"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type reportIn struct {
	From        string     `json:"from" jsonschema:"first day, YYYY-MM-DD, in timezone"`
	To          string     `json:"to" jsonschema:"last day included, YYYY-MM-DD; at most 13 months after from"`
	Timezone    string     `json:"timezone,omitempty" jsonschema:"IANA time zone of the days, UTC by default"`
	Granularity string     `json:"granularity,omitempty" jsonschema:"day (the default), week (Monday to Sunday) or month"`
	Metrics     []string   `json:"metrics,omitempty" jsonschema:"the metrics to count, all when empty"`
	GroupBy     string     `json:"group_by,omitempty" jsonschema:"break every figure down by inbox, channel, label, team or member"`
	InboxID     *uuid.UUID `json:"inbox_id,omitempty"`
	ChannelKind string     `json:"channel_kind,omitempty"`
	LabelID     *uuid.UUID `json:"label_id,omitempty"`
	TeamID      *uuid.UUID `json:"team_id,omitempty" jsonschema:"a team from list_teams"`
	MemberID    *uuid.UUID `json:"member_id,omitempty" jsonschema:"a member from list_members"`
}

type mcpTeamReportPeriod struct {
	Start  string           `json:"start" jsonschema:"the period's first day"`
	Values oas.ReportValues `json:"values"`
}

type mcpTeamReportGroup struct {
	Key    string            `json:"key,omitempty" jsonschema:"the inbox, label, team or member id, or the channel kind; empty for figures without one"`
	Totals oas.ReportValues  `json:"totals"`
	Series []mcpTeamReportPeriod `json:"series"`
}

type mcpTeamReport struct {
	From        string            `json:"from"`
	To          string            `json:"to"`
	Timezone    string            `json:"timezone"`
	Granularity string            `json:"granularity"`
	BuiltUntil  *time.Time        `json:"built_until,omitempty" jsonschema:"the rollups reach this instant; absent while they are first built"`
	Totals      oas.ReportValues  `json:"totals"`
	Series      []mcpTeamReportPeriod `json:"series"`
	Groups      []mcpTeamReportGroup  `json:"groups,omitempty"`
}

func mcpTeamReportSeries(in []oas.ReportPeriod) []mcpTeamReportPeriod {
	out := make([]mcpTeamReportPeriod, len(in))
	for i, p := range in {
		out[i] = mcpTeamReportPeriod{Start: p.Start.String(), Values: p.Values}
	}
	return out
}

var mcpReportTools = []mcpTool{
	defineTool("get_report", "Team report",
		"Counts the team's work in the inboxes you can see per day, week or month of up to 13 months: conversations opened, closed and reopened, replies, first-reply and resolution times (median and p90 seconds), SLA targets met and breached, and ratings; optionally by inbox, channel kind, label, team or member, and narrowed to one of each. Maps to GET /v1/reports.",
		readOnly, map[string][]any{
			"granularity": {"day", "week", "month"}, "group_by": {"inbox", "channel", "label", "team", "member"},
			"metrics": {"opened", "closed", "reopened", "replies", "first_reply", "resolution", "sla", "ratings"}, "channel_kind": {"email", "chat", "app", "api"},
		},
		mcpTool{ops: []string{"GetReport"}},
		func(ctx context.Context, c *mcpCall, in reportIn) (mcpTeamReport, error) {
			from, err1 := time.Parse(time.DateOnly, in.From)
			to, err2 := time.Parse(time.DateOnly, in.To)
			if err1 != nil || err2 != nil {
				return mcpTeamReport{}, errValidation("from and to are days, YYYY-MM-DD")
			}
			params := oas.GetReportParams{
				From: openapi_types.Date{Time: from}, To: openapi_types.Date{Time: to},
				Timezone: optString[string](in.Timezone), Granularity: optString[oas.ReportGranularity](in.Granularity),
				GroupBy: optString[oas.ReportGroupBy](in.GroupBy), ChannelKind: optString[oas.ChannelKind](in.ChannelKind),
				InboxId: in.InboxID, LabelId: in.LabelID, TeamId: in.TeamID, MemberId: in.MemberID,
			}
			if len(in.Metrics) > 0 {
				m := []oas.ReportMetric{oas.ReportMetric(strings.Join(in.Metrics, ","))}
				params.Metrics = &m
			}
			res, err := c.s.GetReport(ctx, oas.GetReportRequestObject{Params: params})
			if err != nil {
				return mcpTeamReport{}, err
			}
			r := oas.Report(res.(oas.GetReport200JSONResponse))
			out := mcpTeamReport{
				From: r.From.String(), To: r.To.String(), Timezone: r.Timezone, Granularity: string(r.Granularity), BuiltUntil: r.BuiltUntil,
				Totals: r.Totals, Series: mcpTeamReportSeries(r.Series),
			}
			if r.Groups != nil {
				for _, g := range *r.Groups {
					mg := mcpTeamReportGroup{Totals: g.Totals, Series: mcpTeamReportSeries(g.Series)}
					if g.Key != nil {
						mg.Key = *g.Key
					}
					out.Groups = append(out.Groups, mg)
				}
			}
			return out, nil
		}),
}

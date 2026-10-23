# Reports

Reports count your team's work: conversations opened, closed and reopened, replies, how fast you
answer and resolve, whether you met your [SLA](sla.md) targets, and how customers rated you. Ask for any
range of up to 13 months, by day, week or month, broken down by inbox, channel, label, team or member,
and download it as CSV. The panel's Reports section has a Trends tab for this; the Overview, Ratings and Team tabs show today, the last 7 days and the last 30 days. The API and the MCP server, described below, give the same figures to your own tools.

## The Trends tab

Open Reports › Trends. Pick a period (a preset of up to 12 months, or custom dates up to 13 months), day, week or month, and break the figures down by inbox, channel, label, team or member. Filters narrow the report to one inbox, channel, label, team or member. KPI tiles show each figure with a comparison to the previous period, charts show volume and response times, and a sortable table lists the groups. A "data up to" note says how far the counts reach. **Export CSV** downloads what you see.

## 1. What is counted

| Figure | Counts |
|---|---|
| `opened` | Conversations started in the period. Spam is left out. |
| `closed` | Every close, including a conversation closed more than once. |
| `reopened` | A change from closed to any other status. |
| `replies` | Replies sent by members, including a bot's draft that a member sent. |
| `first_replies` | Conversations whose first member reply falls in the period, with the median and p90 time in seconds (`first_reply_median_seconds`, `first_reply_p90_seconds`). |
| `resolutions` | Closes that follow a message from the customer, with the median and p90 time in seconds. The clock starts at the first customer message after the previous close. |
| `sla` | Targets met and breached, for `first_reply`, `next_reply` and `resolution`. |
| `ratings` | `good` and `bad`, counted on the day the customer rated. Feedback is never rated. |

Replies that bots, [automation rules](automation.md) and automatic replies send are never counted, neither
as replies nor as a first reply. Trashed conversations and imported history are left out. A median or p90
with nothing to measure is absent.

Medians and p90 are within 2.5% of the exact value. Today's figures agree with the panel's overview.

## 2. Time zones and periods

`from` and `to` are local days in `timezone` (an IANA name such as `Europe/Istanbul`; `UTC` by default),
and `to` is included. They may be at most 13 months apart. Days after today are empty. A week runs Monday
to Sunday; a week or month cut by `from` starts there. The report lists every period, including empty
ones.

## 3. Grouping and filters

`group_by` breaks every figure down by `inbox`, `channel`, `label`, `team` or `member`. Each group has its
own totals and series. Figures with no team, member or channel go to a group without a `key`. A
conversation counts under each of its labels and not at all in a `label` grouping if it has none.

A figure belongs to the member who did it: the author of a reply, who closed or reopened, who sent the
first reply, who closed for a resolution. Opened conversations and ratings belong to the assignee, and SLA
outcomes to the assignee and team the conversation had at the outcome.

`inbox_id`, `channel_kind` (`email`, `chat`, `app` or `api`), `label_id`, `team_id` and `member_id` narrow
the report to one of each. `metrics` limits it to some of `opened`, `closed`, `reopened`, `replies`,
`first_reply`, `resolution`, `sla` and `ratings`; without it you get all of them.

Labels and the team are those a conversation had when its period was last counted.

## 4. Who sees what

A report covers the inboxes the caller can see. Owners and admins see every inbox, agents only the
inboxes they or their teams have access to, and an API key limited to inboxes only those. Asking for an
inbox the caller cannot see answers `404`.

## 5. How fresh the numbers are

Yuva keeps counts per quarter hour, so a local day is exact in every time zone. Today is counted again
every few minutes, and the last 31 days are counted again every night, which picks up label and team
changes. Anything newer than the last count is added live, so today's totals are current. On a server that
has just been upgraded, the history is counted in the background: `built_until` in the answer says how
far the counts reach, and later periods are empty until they catch up.

The counts hold numbers and ids only: no message text, no customer names or addresses. They are deleted
with the workspace. [Retention](operations.md#retention) does not delete them: periods whose conversations were
removed by retention keep their counts.

## 6. API

Use an [API key](headless.md#scopes) or an OAuth connection with the `reports:read` scope. Every member
may hold it; keys made without named scopes have it, older keys do not.

```sh
curl -G https://support.example.com/v1/reports \
  -H "Authorization: Bearer $YUVA_API_KEY" \
  -d from=2026-09-01 -d to=2026-09-30 -d timezone=Europe/Istanbul \
  -d granularity=week -d group_by=inbox -d metrics=opened,closed,first_reply
```

The answer has `totals`, a `series` with one entry per period and, with `group_by`, `groups` each with
their own `totals` and `series`, plus `built_until`. Inboxes, labels, teams and members appear as ids;
read their names from their own endpoints.

`GET /v1/reports/export` takes the same parameters and returns a CSV file: a header row, then one row per
period, or per group and period with `group_by`. Columns are `period` (the period's first day), with
`group_by` also `group_id` and `group_name`, then `opened`, `closed`, `reopened`, `replies`,
`first_replies`, `first_reply_median_seconds`, `first_reply_p90_seconds`, `resolutions`,
`resolution_median_seconds`, `resolution_p90_seconds`, `sla_first_reply_met`, `sla_first_reply_breached`,
`sla_next_reply_met`, `sla_next_reply_breached`, `sla_resolution_met`, `sla_resolution_breached`,
`ratings_good` and `ratings_bad`, limited to the metrics you chose. A time with nothing to measure is an
empty cell. Cells that start with `=`, `+`, `-`, `@`, a tab or a carriage return get a leading `'` so a
spreadsheet does not run them.

| Parameter | |
|---|---|
| `from`, `to` | Required. First and last local day. |
| `timezone` | IANA name; `UTC` by default. |
| `granularity` | `day` (default), `week` or `month`. |
| `metrics` | Repeat or comma-separate; all by default. |
| `group_by` | `inbox`, `channel`, `label`, `team` or `member`. |
| `inbox_id`, `channel_kind`, `label_id`, `team_id`, `member_id` | Filters. |

## 7. MCP

With the `reports:read` scope an [assistant](mcp.md) can call `get_report`, which takes the same
parameters (`from` and `to` as `YYYY-MM-DD`) and answers with the same figures. Ask, for example: "How
fast did we answer by inbox last month, and who closed the most?"

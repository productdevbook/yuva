import { Trans, useLingui } from "@lingui/react/macro"
import { DownloadIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"
import { useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { DailyChart } from "@/components/common/DailyChart"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Segmented } from "@/components/common/Segmented"
import { formatDuration, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { Skeleton } from "@/components/ui/skeleton"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { addDays, dayKey, daysBetween, downloadReport, useReport, type ReportQuery } from "@/features/reports/data"
import { Card, headButton, StatTile, tableHeadClass } from "@/features/settings/ui"
import type { ChannelKind, ReportGranularity, ReportGroupBy, ReportValues } from "@/lib/api"
import { useNow } from "@/hooks/use-now"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels, useMembers, useTeams } from "@/lib/workspace"

const PRESETS = ["today", "7d", "30d", "month", "lastmonth", "3m", "6m", "12m", "custom"] as const
type Preset = (typeof PRESETS)[number]
const GRANULARITIES: ReportGranularity[] = ["day", "week", "month"]
const GROUPS: ReportGroupBy[] = ["inbox", "channel", "label", "team", "member"]
const CHANNELS: ChannelKind[] = ["email", "chat", "app", "api"]

function monthsBack(today: string, n: number) {
  const [y, m, d] = today.split("-").map(Number)
  return dayKey(new Date(y!, m! - 1 - n, d! + 1))
}

function rangeOf(preset: Preset, today: string, custom: { from?: string; to?: string }) {
  const [y, m] = today.split("-").map(Number)
  const first = dayKey(new Date(y!, m! - 1, 1))
  switch (preset) {
    case "today":
      return { from: today, to: today }
    case "7d":
      return { from: addDays(today, -6), to: today }
    case "month":
      return { from: first, to: today }
    case "lastmonth":
      return { from: dayKey(new Date(y!, m! - 2, 1)), to: addDays(first, -1) }
    case "3m":
      return { from: monthsBack(today, 3), to: today }
    case "6m":
      return { from: monthsBack(today, 6), to: today }
    case "12m":
      return { from: monthsBack(today, 12), to: today }
    case "custom":
      return { from: custom.from || addDays(today, -29), to: custom.to || today }
    default:
      return { from: addDays(today, -29), to: today }
  }
}

function withinLimit(from: string, to: string) {
  const [y, m, d] = from.split("-").map(Number)
  return to >= from && to < dayKey(new Date(y!, m! - 1 + 13, d))
}

function defaultGranularity(days: number): ReportGranularity {
  return days <= 45 ? "day" : days <= 200 ? "week" : "month"
}

function slaShare(v: ReportValues | undefined) {
  if (!v?.sla) return undefined
  const { first_reply: a, next_reply: b, resolution: c } = v.sla
  const met = a.met + b.met + c.met
  const total = met + a.breached + b.breached + c.breached
  return total ? met / total : undefined
}

function ratingShare(v: ReportValues | undefined) {
  if (!v?.ratings) return undefined
  const total = v.ratings.good + v.ratings.bad
  return total ? v.ratings.good / total : undefined
}

function hasActivity(v: ReportValues | undefined) {
  if (!v) return false
  const n = [v.opened, v.closed, v.reopened, v.replies, v.first_replies, v.resolutions]
  return n.some((x) => x) || !!v.ratings?.good || !!v.ratings?.bad || slaShare(v) !== undefined
}

type Kind = "count" | "time" | "share"

function Delta({ cur, prev, kind, better }: { cur?: number; prev?: number; kind: Kind; better?: "up" | "down" }) {
  const { t, i18n } = useLingui()
  if (cur === undefined || prev === undefined || (kind !== "share" && prev === 0)) {
    return <span>{t`No earlier figure`}</span>
  }
  const change = kind === "share" ? (cur - prev) * 100 : (cur / prev - 1) * 100
  const text =
    kind === "share"
      ? new Intl.NumberFormat(i18n.locale, {
          style: "unit",
          unit: "percent",
          signDisplay: "exceptZero",
          maximumFractionDigits: 0,
        }).format(change)
      : new Intl.NumberFormat(i18n.locale, {
          style: "percent",
          signDisplay: "exceptZero",
          maximumFractionDigits: 0,
        }).format(change / 100)
  const tone =
    !better || Math.round(change) === 0
      ? "text-faint"
      : change > 0 === (better === "up")
        ? "text-success"
        : "text-destructive"
  return <span className={tone}>{t`${text} vs previous period`}</span>
}

type SortKey =
  | "name"
  | "opened"
  | "closed"
  | "replies"
  | "firstReply"
  | "firstReplyP90"
  | "resolution"
  | "sla"
  | "rating"

function useGroupName(by: ReportGroupBy | undefined) {
  const { t } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const teams = useTeams().data ?? []
  const members = useMembers().data ?? []
  return (key: string | undefined) => {
    if (!key) return t`None`
    switch (by) {
      case "inbox":
        return inboxes.find((x) => x.id === key)?.name ?? key
      case "label":
        return labels.find((x) => x.id === key)?.name ?? key
      case "team":
        return teams.find((x) => x.id === key)?.name ?? key
      case "member": {
        const m = members.find((x) => x.id === key)
        return m ? m.name || m.email : t`Deleted member`
      }
      case "channel":
        return text.channel[key as ChannelKind] ?? key
      default:
        return key
    }
  }
}

function Controls({
  preset,
  setPreset,
  custom,
  setCustom,
  granularity,
  setGranularity,
  group,
  setGroup,
  filters,
  setFilter,
  invalid,
  exporting,
  onExport,
}: {
  preset: Preset
  setPreset: (p: Preset) => void
  custom: { from: string; to: string }
  setCustom: (c: { from: string; to: string }) => void
  granularity: ReportGranularity
  setGranularity: (g: ReportGranularity) => void
  group: ReportGroupBy | ""
  setGroup: (g: ReportGroupBy | "") => void
  filters: { inbox: string; channel: string; label: string; team: string; member: string }
  setFilter: (k: "inbox" | "channel" | "label" | "team" | "member", v: string) => void
  invalid: boolean
  exporting: boolean
  onExport: () => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const teams = useTeams().data ?? []
  const members = useMembers().data ?? []
  const presetText: Record<Preset, string> = {
    today: t`Today`,
    "7d": t`Last 7 days`,
    "30d": t`Last 30 days`,
    month: t`This month`,
    lastmonth: t`Last month`,
    "3m": t`Last 3 months`,
    "6m": t`Last 6 months`,
    "12m": t`Last 12 months`,
    custom: t`Custom`,
  }
  const groupText: Record<ReportGroupBy, string> = {
    inbox: t`Product`,
    channel: t`Channel`,
    label: t`Label`,
    team: t`Team`,
    member: t`Member`,
  }
  const granularityText: Record<ReportGranularity, string> = { day: t`Days`, week: t`Weeks`, month: t`Months` }
  const field = "flex min-w-0 flex-col gap-1 text-caption text-faint"
  const select = "w-full"
  return (
    <div className="flex flex-col gap-3" data-testid="trends-controls">
      <div className="flex flex-wrap items-end gap-3">
        <label className={cn(field, "w-full sm:w-48")}>
          <Trans>Period</Trans>
          <NativeSelect
            className={select}
            value={preset}
            onChange={(e) => setPreset(e.target.value as Preset)}
            data-testid="trends-preset"
          >
            {PRESETS.map((p) => (
              <NativeSelectOption key={p} value={p}>
                {presetText[p]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        {preset === "custom" && (
          <>
            <label className={cn(field, "min-w-36 flex-1 sm:flex-none")}>
              <Trans>From</Trans>
              <input
                type="date"
                className="h-9 rounded-md border border-input bg-transparent px-2.5 text-sm text-foreground"
                value={custom.from}
                max={custom.to}
                onChange={(e) => e.target.value && setCustom({ ...custom, from: e.target.value })}
                aria-invalid={invalid}
                data-testid="trends-from"
              />
            </label>
            <label className={cn(field, "min-w-36 flex-1 sm:flex-none")}>
              <Trans>To</Trans>
              <input
                type="date"
                className="h-9 rounded-md border border-input bg-transparent px-2.5 text-sm text-foreground"
                value={custom.to}
                min={custom.from}
                onChange={(e) => e.target.value && setCustom({ ...custom, to: e.target.value })}
                aria-invalid={invalid}
                data-testid="trends-to"
              />
            </label>
          </>
        )}
        <div className="flex min-w-0 flex-col gap-1 text-caption text-faint">
          <Trans>Split by</Trans>
          <Segmented
            value={granularity}
            label={t`Split by`}
            onChange={setGranularity}
            items={GRANULARITIES.map((g) => ({ value: g, label: granularityText[g], testId: `trends-gran-${g}` }))}
          />
        </div>
        <Button
          variant="outline"
          className={cn(headButton.secondary, "ms-auto")}
          disabled={invalid || exporting}
          onClick={onExport}
          data-testid="trends-export"
        >
          <DownloadIcon className="size-4" />
          <Trans>Export CSV</Trans>
        </Button>
      </div>
      {invalid && (
        <p className="text-caption text-destructive" role="alert">
          <Trans>The end must not be before the start, and the range can be at most 13 months.</Trans>
        </p>
      )}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <label className={field}>
          <Trans>Group by</Trans>
          <NativeSelect
            className={select}
            value={group}
            onChange={(e) => setGroup(e.target.value as ReportGroupBy | "")}
            data-testid="trends-group"
          >
            <NativeSelectOption value="">{t`Nothing`}</NativeSelectOption>
            {GROUPS.map((g) => (
              <NativeSelectOption key={g} value={g}>
                {groupText[g]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        <label className={field}>
          {groupText.inbox}
          <NativeSelect
            className={select}
            value={filters.inbox}
            onChange={(e) => setFilter("inbox", e.target.value)}
            data-testid="trends-filter-inbox"
          >
            <NativeSelectOption value="">{t`All`}</NativeSelectOption>
            {inboxes.map((x) => (
              <NativeSelectOption key={x.id} value={x.id}>
                {x.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        <label className={field}>
          {groupText.channel}
          <NativeSelect
            className={select}
            value={filters.channel}
            onChange={(e) => setFilter("channel", e.target.value)}
            data-testid="trends-filter-channel"
          >
            <NativeSelectOption value="">{t`All`}</NativeSelectOption>
            {CHANNELS.map((k) => (
              <NativeSelectOption key={k} value={k}>
                {text.channel[k]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        <label className={field}>
          {groupText.label}
          <NativeSelect
            className={select}
            value={filters.label}
            onChange={(e) => setFilter("label", e.target.value)}
            data-testid="trends-filter-label"
          >
            <NativeSelectOption value="">{t`All`}</NativeSelectOption>
            {labels.map((x) => (
              <NativeSelectOption key={x.id} value={x.id}>
                {x.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        <label className={field}>
          {groupText.team}
          <NativeSelect
            className={select}
            value={filters.team}
            onChange={(e) => setFilter("team", e.target.value)}
            data-testid="trends-filter-team"
          >
            <NativeSelectOption value="">{t`All`}</NativeSelectOption>
            {teams.map((x) => (
              <NativeSelectOption key={x.id} value={x.id}>
                {x.name}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
        <label className={field}>
          {groupText.member}
          <NativeSelect
            className={select}
            value={filters.member}
            onChange={(e) => setFilter("member", e.target.value)}
            data-testid="trends-filter-member"
          >
            <NativeSelectOption value="">{t`All`}</NativeSelectOption>
            {members.map((x) => (
              <NativeSelectOption key={x.id} value={x.id}>
                {x.name || x.email}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </label>
      </div>
    </div>
  )
}

function periodTick(g: ReportGranularity, locale: string) {
  const month = new Intl.DateTimeFormat(locale, { month: "short", year: "2-digit", timeZone: "UTC" })
  return g === "month" ? (d: string) => month.format(new Date(`${d}T00:00:00Z`)) : undefined
}

function periodLabel(g: ReportGranularity, locale: string) {
  const long = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeZone: "UTC" })
  const month = new Intl.DateTimeFormat(locale, { month: "long", year: "numeric", timeZone: "UTC" })
  return (d: string) => {
    const date = new Date(`${d}T00:00:00Z`)
    return g === "month" ? month.format(date) : g === "week" ? `${long.format(date)} +7` : long.format(date)
  }
}

function VolumeChart({
  series,
  granularity,
}: {
  series: { start: string; values: ReportValues }[]
  granularity: ReportGranularity
}) {
  const { t, i18n } = useLingui()
  const config: ChartConfig = {
    opened: { label: t`Opened`, color: "var(--brand)" },
    closed: { label: t`Closed`, color: "var(--success)" },
    replies: { label: t`Replies`, color: "var(--muted-foreground)" },
  }
  const rows = series.map((p) => ({
    day: p.start,
    opened: p.values.opened ?? 0,
    closed: p.values.closed ?? 0,
    replies: p.values.replies ?? 0,
  }))
  return (
    <Card>
      <h2 className="mb-3 font-semibold">
        <Trans>Conversations and replies</Trans>
      </h2>
      <DailyChart
        rows={rows}
        config={config}
        testId="trends-volume"
        tick={periodTick(granularity, i18n.locale)}
        label={periodLabel(granularity, i18n.locale)}
      />
    </Card>
  )
}

function TimesChart({
  series,
  granularity,
}: {
  series: { start: string; values: ReportValues }[]
  granularity: ReportGranularity
}) {
  const { t, i18n } = useLingui()
  const [which, setWhich] = useState<"first" | "resolution">("first")
  const config: ChartConfig = {
    median: { label: t`Median`, color: "var(--brand)" },
    p90: { label: t`90th percentile`, color: "var(--muted-foreground)" },
  }
  const rows = series.map((p) => ({
    day: p.start,
    median: which === "first" ? p.values.first_reply_median_seconds : p.values.resolution_median_seconds,
    p90: which === "first" ? p.values.first_reply_p90_seconds : p.values.resolution_p90_seconds,
  }))
  const any = rows.some((r) => r.median !== undefined)
  const short = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const tick = periodTick(granularity, i18n.locale) ?? ((d: string) => short.format(new Date(`${d}T00:00:00Z`)))
  const label = periodLabel(granularity, i18n.locale)
  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-semibold">
          <Trans>Response times</Trans>
        </h2>
        <Segmented
          value={which}
          label={t`Time`}
          onChange={setWhich}
          items={[
            { value: "first", label: t`First reply`, testId: "trends-times-first" },
            { value: "resolution", label: t`Resolved in`, testId: "trends-times-resolution" },
          ]}
        />
      </div>
      {any ? (
        <ChartContainer config={config} className="aspect-auto h-48 w-full" data-testid="trends-times">
          <LineChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="day"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              minTickGap={24}
              tickFormatter={(d) => tick(String(d))}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width="auto"
              tickFormatter={(v) => formatDuration(Number(v), i18n.locale)}
            />
            <ChartTooltip
              cursor={false}
              content={
                <ChartTooltipContent
                  labelFormatter={(_, p) => (p?.[0] ? label(String(p[0].payload.day)) : "")}
                  formatter={(value, name) => (
                    <div className="flex w-full items-center gap-2">
                      <span
                        className={
                          name === "p90"
                            ? "h-0 w-2.5 shrink-0 border-t-2 border-dashed border-muted-foreground"
                            : "size-2.5 shrink-0 rounded-[2px] bg-brand"
                        }
                      />
                      <span className="flex-1 text-muted-foreground">{config[String(name)]?.label}</span>
                      <span className="font-mono font-medium text-foreground tabular-nums">
                        {formatDuration(Number(value), i18n.locale)}
                      </span>
                    </div>
                  )}
                />
              }
            />
            <Line
              dataKey="median"
              type="monotone"
              stroke="var(--color-median)"
              strokeWidth={2}
              dot={rows.length < 40}
              connectNulls
              isAnimationActive={false}
            />
            <Line
              dataKey="p90"
              type="monotone"
              stroke="var(--color-p90)"
              strokeDasharray="4 4"
              strokeWidth={1.5}
              dot={false}
              connectNulls
              isAnimationActive={false}
            />
          </LineChart>
        </ChartContainer>
      ) : (
        <p className="py-10 text-center text-body text-muted-foreground">
          <Trans>Nothing was measured in this period.</Trans>
        </p>
      )}
    </Card>
  )
}

function GroupTable({ by, groups }: { by: ReportGroupBy; groups: { key?: string; totals: ReportValues }[] }) {
  const { t, i18n } = useLingui()
  const name = useGroupName(by)
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "opened", desc: true })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const dur = (v: number | undefined) => (v === undefined ? "–" : formatDuration(v, i18n.locale))
  const share = (v: number | undefined) => (v === undefined ? "–" : pct.format(v))
  const rows = groups.map((g) => ({
    key: g.key ?? "",
    name: name(g.key),
    v: g.totals,
    sla: slaShare(g.totals),
    rating: ratingShare(g.totals),
  }))
  const value = (r: (typeof rows)[number], k: SortKey): number | string | undefined => {
    switch (k) {
      case "name":
        return r.name
      case "opened":
        return r.v.opened
      case "closed":
        return r.v.closed
      case "replies":
        return r.v.replies
      case "firstReply":
        return r.v.first_reply_median_seconds
      case "firstReplyP90":
        return r.v.first_reply_p90_seconds
      case "resolution":
        return r.v.resolution_median_seconds
      case "sla":
        return r.sla
      case "rating":
        return r.rating
    }
  }
  const sorted = [...rows].sort((a, b) => {
    const x = value(a, sort.key)
    const y = value(b, sort.key)
    if (x === undefined && y === undefined) return 0
    if (x === undefined) return 1
    if (y === undefined) return -1
    const c = typeof x === "string" ? x.localeCompare(String(y), i18n.locale) : Number(x) - Number(y)
    return sort.desc ? -c : c
  })
  const head = (key: SortKey, label: React.ReactNode, end = true) => (
    <th
      scope="col"
      className={cn("px-3 py-2.5 font-medium", end ? "text-end" : "text-start")}
      aria-sort={sort.key === key ? (sort.desc ? "descending" : "ascending") : "none"}
    >
      <button
        type="button"
        className={cn(
          "outline-none focus-visible:underline",
          sort.key === key ? "text-foreground" : "hover:text-foreground"
        )}
        onClick={() => setSort((s) => ({ key, desc: s.key === key ? !s.desc : key !== "name" }))}
        data-testid={`trends-sort-${key}`}
      >
        {label}
        {sort.key === key && (sort.desc ? " ↓" : " ↑")}
      </button>
    </th>
  )
  return (
    <section className="overflow-x-auto rounded-xl border bg-card" data-testid="trends-groups">
      <table className="w-full min-w-[820px] text-body">
        <caption className="sr-only">{t`Figures by group`}</caption>
        <thead className={tableHeadClass}>
          <tr>
            {head("name", <Trans>Name</Trans>, false)}
            {head("opened", <Trans>Opened</Trans>)}
            {head("closed", <Trans>Closed</Trans>)}
            {head("replies", <Trans>Replies</Trans>)}
            {head("firstReply", <Trans>First reply</Trans>)}
            {head("firstReplyP90", <Trans>First reply, 90th percentile</Trans>)}
            {head("resolution", <Trans>Resolved in</Trans>)}
            {head("sla", <Trans>SLA met</Trans>)}
            {head("rating", <Trans>Satisfied</Trans>)}
          </tr>
        </thead>
        <tbody>
          {sorted.map((r) => (
            <tr key={r.key || "none"} className="border-t" data-testid="trends-group-row">
              <th scope="row" className="max-w-56 truncate px-3 py-2.5 text-start font-medium">
                {r.name}
              </th>
              <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(r.v.opened ?? 0)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(r.v.closed ?? 0)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(r.v.replies ?? 0)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{dur(r.v.first_reply_median_seconds)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{dur(r.v.first_reply_p90_seconds)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{dur(r.v.resolution_median_seconds)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{share(r.sla)}</td>
              <td className="px-3 py-2.5 text-end tabular-nums">{share(r.rating)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}

export function TrendsReport() {
  const { t, i18n } = useLingui()
  const [params, setParams] = useSearchParams()
  const [inbox, setInbox] = useInboxFilter()
  const [exporting, setExporting] = useState(false)
  const [failure, setFailure] = useState<unknown>()
  const today = dayKey(new Date(useNow(60_000)))
  const raw = params.get("range") as Preset | null
  const preset: Preset = raw && PRESETS.includes(raw) ? raw : "30d"
  const custom = { from: params.get("from") ?? "", to: params.get("to") ?? "" }
  const range = rangeOf(preset, today, custom)
  const days = daysBetween(range.from, range.to)
  const invalid = !withinLimit(range.from, range.to)
  const rawGran = params.get("by") as ReportGranularity | null
  const granularity = rawGran && GRANULARITIES.includes(rawGran) ? rawGran : defaultGranularity(days)
  const rawGroup = params.get("group") as ReportGroupBy | null
  const group = rawGroup && GROUPS.includes(rawGroup) ? rawGroup : ""
  const filters = {
    inbox,
    channel: params.get("channel") ?? "",
    label: params.get("label") ?? "",
    team: params.get("team") ?? "",
    member: params.get("member") ?? "",
  }
  const set = (changes: Record<string, string | undefined>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        for (const [k, v] of Object.entries(changes)) {
          if (v) next.set(k, v)
          else next.delete(k)
        }
        return next
      },
      { replace: true }
    )
  const query: ReportQuery = {
    ...range,
    granularity,
    group_by: group || undefined,
    inbox_id: filters.inbox,
    channel_kind: (filters.channel as ChannelKind) || undefined,
    label_id: filters.label,
    team_id: filters.team,
    member_id: filters.member,
  }
  const prevTo = addDays(range.from, -1)
  const previous: ReportQuery = {
    ...query,
    group_by: undefined,
    from: addDays(prevTo, -(days - 1)),
    to: prevTo,
    granularity: "month",
  }
  const report = useReport(query, !invalid)
  const before = useReport(previous, !invalid)
  const cur = report.data?.totals
  const prev = before.data?.totals
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const dur = (v: number | undefined) => (v === undefined ? "–" : formatDuration(v, i18n.locale))
  const builtUntil = report.data?.built_until
  const builtText = useMemo(
    () =>
      builtUntil
        ? new Intl.DateTimeFormat(i18n.locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(builtUntil))
        : undefined,
    [builtUntil, i18n.locale]
  )
  const tiles: {
    key: string
    label: string
    value: string
    cur?: number
    prev?: number
    kind: Kind
    better?: "up" | "down"
  }[] = [
    {
      key: "opened",
      label: t`Opened`,
      value: fmt.format(cur?.opened ?? 0),
      cur: cur?.opened,
      prev: prev?.opened,
      kind: "count",
    },
    {
      key: "closed",
      label: t`Closed`,
      value: fmt.format(cur?.closed ?? 0),
      cur: cur?.closed,
      prev: prev?.closed,
      kind: "count",
      better: "up",
    },
    {
      key: "first-reply",
      label: t`First reply`,
      value: dur(cur?.first_reply_median_seconds),
      cur: cur?.first_reply_median_seconds,
      prev: prev?.first_reply_median_seconds,
      kind: "time",
      better: "down",
    },
    {
      key: "first-reply-p90",
      label: t`First reply, 90th percentile`,
      value: dur(cur?.first_reply_p90_seconds),
      cur: cur?.first_reply_p90_seconds,
      prev: prev?.first_reply_p90_seconds,
      kind: "time",
      better: "down",
    },
    {
      key: "resolution",
      label: t`Resolved in`,
      value: dur(cur?.resolution_median_seconds),
      cur: cur?.resolution_median_seconds,
      prev: prev?.resolution_median_seconds,
      kind: "time",
      better: "down",
    },
    {
      key: "sla",
      label: t`SLA met`,
      value: slaShare(cur) === undefined ? "–" : pct.format(slaShare(cur)!),
      cur: slaShare(cur),
      prev: slaShare(prev),
      kind: "share",
      better: "up",
    },
    {
      key: "rating",
      label: t`Satisfied`,
      value: ratingShare(cur) === undefined ? "–" : pct.format(ratingShare(cur)!),
      cur: ratingShare(cur),
      prev: ratingShare(prev),
      kind: "share",
      better: "up",
    },
  ]
  const onExport = async () => {
    setExporting(true)
    setFailure(undefined)
    try {
      await downloadReport(query)
    } catch (e) {
      setFailure(e)
    } finally {
      setExporting(false)
    }
  }
  const empty =
    report.data && !hasActivity(report.data.totals) && !report.data.series.some((p) => hasActivity(p.values))
  return (
    <>
      <Controls
        preset={preset}
        setPreset={(p) =>
          set(
            p === "custom"
              ? { range: p, from: range.from, to: range.to, by: params.get("by") ?? undefined }
              : { range: p === "30d" ? undefined : p, from: undefined, to: undefined }
          )
        }
        custom={{ from: range.from, to: range.to }}
        setCustom={(c) => set({ range: "custom", from: c.from, to: c.to })}
        granularity={granularity}
        setGranularity={(g) => set({ by: g })}
        group={group}
        setGroup={(g) => set({ group: g || undefined })}
        filters={filters}
        setFilter={(k, v) => (k === "inbox" ? setInbox(v) : set({ [k]: v || undefined }))}
        invalid={invalid}
        exporting={exporting}
        onExport={onExport}
      />
      <ErrorLine error={report.error ?? failure} />
      {report.isPending && !invalid ? (
        <div className="flex flex-col gap-4" data-testid="trends-loading">
          <Skeleton className="h-24 rounded-xl" />
          <Skeleton className="h-56 rounded-xl" />
        </div>
      ) : (
        report.data && (
          <div
            className={cn("flex flex-col gap-4 transition-opacity", report.isPlaceholderData && "opacity-60")}
            data-testid="trends"
          >
            <p className="text-caption text-faint" data-testid="trends-built-until">
              {builtText ? <Trans>Data up to {builtText}</Trans> : <Trans>The figures are still being prepared.</Trans>}
            </p>
            {empty ? (
              <EmptyBox title={<Trans>No activity in this period</Trans>} testId="trends-empty">
                <Trans>Nothing was opened, replied to or closed here. Try a longer period or other filters.</Trans>
              </EmptyBox>
            ) : (
              <>
                <div className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-4" data-testid="trends-tiles">
                  {tiles.map((x) => (
                    <StatTile
                      key={x.key}
                      label={x.label}
                      value={x.value}
                      hint={<Delta cur={x.cur} prev={x.prev} kind={x.kind} better={x.better} />}
                      hintClassName="whitespace-normal"
                      testId={`trends-tile-${x.key}`}
                    />
                  ))}
                </div>
                <div className="grid gap-4 xl:grid-cols-2">
                  <VolumeChart series={report.data.series} granularity={granularity} />
                  <TimesChart series={report.data.series} granularity={granularity} />
                </div>
                {group && report.data.groups && (
                  <>
                    <h2 className="font-semibold">
                      <Trans>By group</Trans>
                    </h2>
                    <GroupTable by={group} groups={report.data.groups} />
                  </>
                )}
              </>
            )}
          </div>
        )
      )}
    </>
  )
}

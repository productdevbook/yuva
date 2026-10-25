import { Trans, useLingui } from "@lingui/react/macro"
import { ChartLineIcon, DownloadIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"
import { useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Segmented } from "@/components/common/Segmented"
import { formatDuration, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select"
import { KpiTile, Legend } from "@/features/analytics/Overview"
import { Panel } from "@/features/analytics/parts"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look } from "@/features/inbox/PageFrame"
import { addDays, dayKey, daysBetween, downloadReport, useReport, type ReportQuery } from "@/features/reports/data"
import { ActionsPortal, headClass, ReportSkeleton, SlaBar, ToolbarPortal } from "@/features/reports/parts"
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

function slaCounts(v: ReportValues | undefined) {
  if (!v?.sla) return { met: 0, breached: 0 }
  const { first_reply: a, next_reply: b, resolution: c } = v.sla
  return { met: a.met + b.met + c.met, breached: a.breached + b.breached + c.breached }
}

function slaShare(v: ReportValues | undefined) {
  const { met, breached } = slaCounts(v)
  return met + breached ? met / (met + breached) : undefined
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

const pick =
  "[&>select]:h-[30px] [&>select]:rounded-lg [&>select]:bg-card [&>select]:py-0 [&>select]:text-body [&>select]:shadow-none dark:[&>select]:bg-card"
const pickOn = "[&>select]:border-brand/40 [&>select]:bg-brand-wash [&>select]:text-brand dark:[&>select]:bg-brand-wash"
const dateInput = "h-[30px] rounded-lg border border-input bg-card px-2 text-body text-foreground"

function Filter({
  label,
  value,
  onChange,
  testId,
  children,
}: {
  label: React.ReactNode
  value: string
  onChange: (v: string) => void
  testId: string
  children: React.ReactNode
}) {
  return (
    <label className="inline-flex shrink-0 items-center gap-1.5 text-small text-faint @max-[640px]:flex-col @max-[640px]:items-stretch @max-[640px]:gap-1">
      {label}
      <NativeSelect
        className={cn(pick, value && pickOn, "@max-[640px]:w-full")}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        data-testid={testId}
      >
        {children}
      </NativeSelect>
    </label>
  )
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
  const all = <NativeSelectOption value="">{t`All`}</NativeSelectOption>
  return (
    <>
      <ToolbarPortal>
        <div
          className="flex flex-wrap items-center gap-2 @max-[640px]:grid @max-[640px]:grid-cols-[minmax(0,1fr)_auto]"
          data-testid="trends-controls"
        >
          <NativeSelect
            className={cn(pick, "@max-[640px]:w-full")}
            value={preset}
            onChange={(e) => setPreset(e.target.value as Preset)}
            aria-label={t`Period`}
            data-testid="trends-preset"
          >
            {PRESETS.map((p) => (
              <NativeSelectOption key={p} value={p}>
                {presetText[p]}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          {preset === "custom" && (
            <>
              <label className="inline-flex shrink-0 items-center gap-1.5 text-small text-faint @max-[640px]:justify-between">
                <Trans>From</Trans>
                <input
                  type="date"
                  className={dateInput}
                  value={custom.from}
                  max={custom.to}
                  onChange={(e) => e.target.value && setCustom({ ...custom, from: e.target.value })}
                  aria-invalid={invalid}
                  data-testid="trends-from"
                />
              </label>
              <label className="inline-flex shrink-0 items-center gap-1.5 text-small text-faint @max-[640px]:justify-between">
                <Trans>To</Trans>
                <input
                  type="date"
                  className={dateInput}
                  value={custom.to}
                  min={custom.from}
                  onChange={(e) => e.target.value && setCustom({ ...custom, to: e.target.value })}
                  aria-invalid={invalid}
                  data-testid="trends-to"
                />
              </label>
            </>
          )}
          <Segmented
            value={granularity}
            label={t`Split by`}
            onChange={setGranularity}
            items={GRANULARITIES.map((g) => ({ value: g, label: granularityText[g], testId: `trends-gran-${g}` }))}
          />
          <span className="h-5 w-px shrink-0 bg-border @max-[640px]:hidden" aria-hidden />
          <div className="contents @max-[640px]:col-span-2 @max-[640px]:grid @max-[640px]:grid-cols-2 @max-[640px]:gap-2">
            <Filter
              label={<Trans>Group by</Trans>}
              value={group}
              onChange={(v) => setGroup(v as ReportGroupBy | "")}
              testId="trends-group"
            >
              <NativeSelectOption value="">{t`Nothing`}</NativeSelectOption>
              {GROUPS.map((g) => (
                <NativeSelectOption key={g} value={g}>
                  {groupText[g]}
                </NativeSelectOption>
              ))}
            </Filter>
            <Filter
              label={groupText.inbox}
              value={filters.inbox}
              onChange={(v) => setFilter("inbox", v)}
              testId="trends-filter-inbox"
            >
              {all}
              {inboxes.map((x) => (
                <NativeSelectOption key={x.id} value={x.id}>
                  {x.name}
                </NativeSelectOption>
              ))}
            </Filter>
            <Filter
              label={groupText.channel}
              value={filters.channel}
              onChange={(v) => setFilter("channel", v)}
              testId="trends-filter-channel"
            >
              {all}
              {CHANNELS.map((k) => (
                <NativeSelectOption key={k} value={k}>
                  {text.channel[k]}
                </NativeSelectOption>
              ))}
            </Filter>
            <Filter
              label={groupText.label}
              value={filters.label}
              onChange={(v) => setFilter("label", v)}
              testId="trends-filter-label"
            >
              {all}
              {labels.map((x) => (
                <NativeSelectOption key={x.id} value={x.id}>
                  {x.name}
                </NativeSelectOption>
              ))}
            </Filter>
            <Filter
              label={groupText.team}
              value={filters.team}
              onChange={(v) => setFilter("team", v)}
              testId="trends-filter-team"
            >
              {all}
              {teams.map((x) => (
                <NativeSelectOption key={x.id} value={x.id}>
                  {x.name}
                </NativeSelectOption>
              ))}
            </Filter>
            <Filter
              label={groupText.member}
              value={filters.member}
              onChange={(v) => setFilter("member", v)}
              testId="trends-filter-member"
            >
              {all}
              {members.map((x) => (
                <NativeSelectOption key={x.id} value={x.id}>
                  {x.name || x.email}
                </NativeSelectOption>
              ))}
            </Filter>
          </div>
        </div>
      </ToolbarPortal>
      <ActionsPortal>
        <Button
          variant="outline"
          className={cn(look.btn, "bg-card")}
          disabled={invalid || exporting}
          onClick={onExport}
          data-testid="trends-export"
        >
          <DownloadIcon className="size-4" />
          <Trans>Export CSV</Trans>
        </Button>
      </ActionsPortal>
    </>
  )
}

function periodTick(g: ReportGranularity, locale: string) {
  const month = new Intl.DateTimeFormat(locale, { month: "short", year: "2-digit", timeZone: "UTC" })
  const short = new Intl.DateTimeFormat(locale, { day: "numeric", month: "short", timeZone: "UTC" })
  const fmt = g === "month" ? month : short
  return (d: string) => fmt.format(new Date(`${d}T00:00:00Z`))
}

function periodLabel(g: ReportGranularity, locale: string) {
  const long = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeZone: "UTC" })
  const month = new Intl.DateTimeFormat(locale, { month: "long", year: "numeric", timeZone: "UTC" })
  return (d: string) => {
    const date = new Date(`${d}T00:00:00Z`)
    return g === "month" ? month.format(date) : g === "week" ? `${long.format(date)} +7` : long.format(date)
  }
}

function seriesRow(config: ChartConfig, format: (v: number) => string, dashed: string[]) {
  return (value: unknown, name: unknown) => (
    <div className="flex w-full items-center gap-2">
      <span
        className={cn("h-0 w-3 shrink-0 border-t-2", dashed.includes(String(name)) && "border-dashed")}
        style={{ borderColor: `var(--color-${String(name)})` }}
      />
      <span className="flex-1 text-muted-foreground">{config[String(name)]?.label}</span>
      <span className="font-mono font-medium text-foreground tabular-nums">{format(Number(value))}</span>
    </div>
  )
}

const chartMargin = { top: 8, right: 8, bottom: 0, left: -4 }

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
    closed: { label: t`Closed`, color: "var(--foreground)" },
    replies: { label: t`Replies`, color: "var(--muted-foreground)" },
  }
  const rows = series.map((p) => ({
    day: p.start,
    opened: p.values.opened ?? 0,
    closed: p.values.closed ?? 0,
    replies: p.values.replies ?? 0,
  }))
  const tick = periodTick(granularity, i18n.locale)
  const label = periodLabel(granularity, i18n.locale)
  const count = new Intl.NumberFormat(i18n.locale)
  const compact = new Intl.NumberFormat(i18n.locale, { notation: "compact" })
  return (
    <Panel
      title={<Trans>Conversations and replies</Trans>}
      action={
        <Legend
          items={[
            { color: "var(--brand)", label: config.opened!.label },
            { color: "var(--foreground)", label: config.closed!.label },
            { color: "var(--muted-foreground)", label: config.replies!.label, dashed: true },
          ]}
        />
      }
    >
      <ChartContainer config={config} className="aspect-auto h-56 w-full px-1" data-testid="trends-volume">
        <AreaChart data={rows} margin={chartMargin}>
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey="day"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={28}
            interval="preserveStartEnd"
            tickFormatter={(d) => tick(String(d))}
          />
          <YAxis
            tickLine={false}
            axisLine={false}
            width={40}
            allowDecimals={false}
            tickFormatter={(v) => compact.format(Number(v))}
          />
          <ChartTooltip
            cursor={{ stroke: "var(--faint)", strokeWidth: 1 }}
            content={
              <ChartTooltipContent
                indicator="line"
                labelFormatter={(_, p) => (p?.[0] ? label(String(p[0].payload.day)) : "")}
                formatter={seriesRow(config, (v) => count.format(v), ["replies"])}
              />
            }
          />
          <Area
            dataKey="opened"
            type="monotone"
            stroke="var(--color-opened)"
            strokeWidth={2}
            fill="var(--color-opened)"
            fillOpacity={0.08}
            activeDot={{ r: 4 }}
            isAnimationActive={false}
          />
          <Area
            dataKey="closed"
            type="monotone"
            stroke="var(--color-closed)"
            strokeWidth={2}
            fill="none"
            activeDot={{ r: 4 }}
            isAnimationActive={false}
          />
          <Area
            dataKey="replies"
            type="monotone"
            stroke="var(--color-replies)"
            strokeDasharray="4 4"
            strokeWidth={1.5}
            fill="none"
            activeDot={{ r: 3 }}
            isAnimationActive={false}
          />
        </AreaChart>
      </ChartContainer>
    </Panel>
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
  const tick = periodTick(granularity, i18n.locale)
  const label = periodLabel(granularity, i18n.locale)
  return (
    <Panel
      title={<Trans>Response times</Trans>}
      action={
        <>
          <Legend
            items={[
              { color: "var(--brand)", label: config.median!.label },
              { color: "var(--muted-foreground)", label: config.p90!.label, dashed: true },
            ]}
          />
          <Segmented
            value={which}
            label={t`Time`}
            onChange={setWhich}
            itemClassName="h-6 px-2 text-small"
            items={[
              { value: "first", label: t`First reply`, testId: "trends-times-first" },
              { value: "resolution", label: t`Resolved in`, testId: "trends-times-resolution" },
            ]}
          />
        </>
      }
    >
      {any ? (
        <ChartContainer config={config} className="aspect-auto h-56 w-full px-1" data-testid="trends-times">
          <LineChart data={rows} margin={chartMargin}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="day"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              minTickGap={28}
              interval="preserveStartEnd"
              tickFormatter={(d) => tick(String(d))}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={48}
              tickFormatter={(v) => formatDuration(Number(v), i18n.locale)}
            />
            <ChartTooltip
              cursor={{ stroke: "var(--faint)", strokeWidth: 1 }}
              content={
                <ChartTooltipContent
                  indicator="line"
                  labelFormatter={(_, p) => (p?.[0] ? label(String(p[0].payload.day)) : "")}
                  formatter={seriesRow(config, (v) => formatDuration(v, i18n.locale), ["p90"])}
                />
              }
            />
            <Line
              dataKey="median"
              type="monotone"
              stroke="var(--color-median)"
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
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
              activeDot={{ r: 3 }}
              connectNulls
              isAnimationActive={false}
            />
          </LineChart>
        </ChartContainer>
      ) : (
        <p className="py-14 text-center text-body text-faint">
          <Trans>Nothing was measured in this period.</Trans>
        </p>
      )}
    </Panel>
  )
}

function GroupTable({
  by,
  groups,
  totals,
}: {
  by: ReportGroupBy
  groups: { key?: string; totals: ReportValues }[]
  totals: ReportValues
}) {
  const { t, i18n } = useLingui()
  const name = useGroupName(by)
  const inboxes = useInboxes().data ?? []
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "opened", desc: true })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const dur = (v: number | undefined) => (v === undefined ? "–" : formatDuration(v, i18n.locale))
  const share = (v: number | undefined) => (v === undefined ? "–" : pct.format(v))
  const rows = groups.map((g) => ({
    key: g.key ?? "",
    name: name(g.key),
    color: by === "inbox" ? inboxes.find((x) => x.id === g.key)?.branding.color : undefined,
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
      className={cn("h-9 px-3 font-semibold", end ? "text-end" : "text-start")}
      aria-sort={sort.key === key ? (sort.desc ? "descending" : "ascending") : "none"}
    >
      <button
        type="button"
        className={cn(
          "tracking-[inherit] uppercase outline-none focus-visible:underline",
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
  const cells = (v: ReportValues, sla: number | undefined, rating: number | undefined) => {
    const { met, breached } = slaCounts(v)
    return (
      <>
        <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(v.opened ?? 0)}</td>
        <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(v.closed ?? 0)}</td>
        <td className="px-3 py-2.5 text-end tabular-nums">{fmt.format(v.replies ?? 0)}</td>
        <td className="px-3 py-2.5 text-end tabular-nums">{dur(v.first_reply_median_seconds)}</td>
        <td className="px-3 py-2.5 text-end tabular-nums">{dur(v.first_reply_p90_seconds)}</td>
        <td className="px-3 py-2.5 text-end tabular-nums">{dur(v.resolution_median_seconds)}</td>
        <td className="px-3 py-2.5 tabular-nums">
          <span className="flex items-center justify-end gap-2">
            <SlaBar met={met} breached={breached} />
            {share(sla)}
          </span>
        </td>
        <td className="px-3 py-2.5 text-end tabular-nums">{share(rating)}</td>
      </>
    )
  }
  return (
    <section className="overflow-x-auto rounded-[14px] border bg-card" data-testid="trends-groups">
      <table className="w-full min-w-[860px] text-body">
        <caption className="sr-only">{t`Figures by group`}</caption>
        <thead className={headClass}>
          <tr className="border-b">
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
            <tr key={r.key || "none"} className="border-b" data-testid="trends-group-row">
              <th scope="row" className="max-w-56 px-3 py-2.5 text-start font-medium">
                <span className="flex min-w-0 items-center gap-2">
                  {r.color && (
                    <i className="size-2 shrink-0 rounded-[3px]" style={{ backgroundColor: r.color }} aria-hidden />
                  )}
                  <span className="truncate">{r.name}</span>
                </span>
              </th>
              {cells(r.v, r.sla, r.rating)}
            </tr>
          ))}
          <tr className="bg-muted/50 font-semibold" data-testid="trends-group-total">
            <th scope="row" className="px-3 py-2.5 text-start">
              <Trans>All</Trans>
            </th>
            {cells(totals, slaShare(totals), ratingShare(totals))}
          </tr>
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
      {invalid && (
        <p className="text-caption text-destructive" role="alert">
          <Trans>The end must not be before the start, and the range can be at most 13 months.</Trans>
        </p>
      )}
      <ErrorLine error={report.error ?? failure} />
      {report.isPending && !invalid ? (
        <ReportSkeleton tiles={5} testId="trends-loading" />
      ) : (
        report.data && (
          <div
            className={cn("flex flex-col gap-4 transition-opacity", report.isPlaceholderData && "opacity-60")}
            data-testid="trends"
          >
            <p className="text-small text-faint" data-testid="trends-built-until">
              {builtText ? <Trans>Data up to {builtText}</Trans> : <Trans>The figures are still being prepared.</Trans>}
            </p>
            {empty ? (
              <EmptyBox
                icon={ChartLineIcon}
                title={<Trans>No activity in this period</Trans>}
                className="rounded-[14px] border-solid bg-card"
                testId="trends-empty"
              >
                <Trans>Nothing was opened, replied to or closed here. Try a longer period or other filters.</Trans>
              </EmptyBox>
            ) : (
              <>
                <div className="grid grid-cols-12 gap-2.5 @max-[760px]:grid-cols-2" data-testid="trends-tiles">
                  {tiles.map((x, i) => (
                    <KpiTile
                      key={x.key}
                      label={x.label}
                      wrapLabel
                      value={x.value}
                      hint={<Delta cur={x.cur} prev={x.prev} kind={x.kind} better={x.better} />}
                      testId={`trends-tile-${x.key}`}
                      className={cn(
                        i < 4 ? "col-span-3" : "col-span-4",
                        "@max-[760px]:col-span-1",
                        i === tiles.length - 1 && "@max-[760px]:col-span-2"
                      )}
                    />
                  ))}
                </div>
                <div className="grid grid-cols-2 gap-4 @max-[1000px]:grid-cols-1">
                  <VolumeChart series={report.data.series} granularity={granularity} />
                  <TimesChart series={report.data.series} granularity={granularity} />
                </div>
                {group && report.data.groups && (
                  <>
                    <h2 className="text-[14px] leading-5 font-semibold">
                      <Trans>By group</Trans>
                    </h2>
                    <GroupTable by={group} groups={report.data.groups} totals={report.data.totals} />
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

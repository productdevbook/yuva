import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownRightIcon, ArrowUpRightIcon } from "lucide-react"
import { useState } from "react"
import { Link } from "react-router"
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts"

import { Segmented } from "@/components/common"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import { look } from "@/features/inbox/PageFrame"
import { AddNoteButton, annotationMarks, AnnotationList } from "@/features/analytics/Annotations"
import { deviceCard, groupDimensions, RowLabel, type Group } from "@/features/analytics/Breakdown"
import { OverviewInsights } from "@/features/analytics/Insights"
import {
  Attribution,
  BarList,
  ConsentedOnly,
  offersPersistent,
  Panel,
  PanelProblem,
  QueryProblem,
  useCurrentProperty,
} from "@/features/analytics/parts"
import { useAnalytics, useAnnotations, useRealtime, useShare } from "@/features/analytics/queries"
import { grainFor, scopeOf, todayIso, useView, useViewSearch, type View } from "@/features/analytics/state"
import {
  formatCompact,
  formatMetric,
  lowerIsBetter,
  num,
  OTHER,
  useAnalyticsText,
  type NumMetric,
} from "@/features/analytics/text"
import type { AnalyticsFilterDimension, AnalyticsMetric } from "@/lib/api"
import { cn } from "@/lib/utils"

export function Change({ now, before, metric }: { now?: number; before?: number; metric: AnalyticsMetric }) {
  const { i18n } = useLingui()
  if (now === undefined || before === undefined) return null
  if (before === 0) return <span className="text-caption text-faint">–</span>
  const change = (now - before) / before
  const up = change >= 0
  const good = lowerIsBetter(metric) ? !up : up
  const Icon = up ? ArrowUpRightIcon : ArrowDownRightIcon
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 text-caption font-medium tabular-nums",
        change === 0 ? "text-faint" : good ? "text-success" : "text-destructive"
      )}
      data-testid="kpi-change"
    >
      <Icon className="size-3.5" />
      {new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0, signDisplay: "never" }).format(
        change
      )}
    </span>
  )
}

export function KpiTile({
  label,
  value,
  hint,
  live,
  testId,
  className,
  wrapLabel,
}: {
  label: React.ReactNode
  value: React.ReactNode
  hint?: React.ReactNode
  live?: boolean
  wrapLabel?: boolean
  testId?: string
  className?: string
}) {
  return (
    <div
      className={cn(
        "flex min-w-0 flex-1 basis-0 flex-col gap-0.5 rounded-[14px] border bg-card px-4 py-3.5 @max-[900px]:basis-[calc(50%-5px)]",
        className
      )}
      data-testid={testId}
    >
      <span className="flex min-w-0 items-center gap-1.5 text-small text-faint">
        {live && <span className="size-2 shrink-0 rounded-full bg-success" aria-hidden />}
        <span className={wrapLabel ? "line-clamp-2" : "truncate"}>{label}</span>
      </span>
      <b className="text-[22px] leading-7 font-semibold tracking-[-0.01em] whitespace-nowrap tabular-nums">{value}</b>
      <span className="min-h-4 text-caption text-faint">{hint}</span>
    </div>
  )
}

function Kpis() {
  const { t, i18n } = useLingui()
  const property = useCurrentProperty()
  const share = useShare()
  const { view } = useView()
  const live = useRealtime(property.id, !share || share.views.includes("realtime"))
  const traffic = useAnalytics(property.id, {
    metrics: ["visitors", "pageviews", "bounce_rate"],
    range: view.range,
    ...scopeOf(view),
    compare: { with: "previous" },
  })
  const support = useAnalytics(
    property.id,
    { dataset: "support", metrics: ["conversations"], range: view.range, filters: supportFilters(view) },
    !!property.inbox_id && !share
  )
  const totals = traffic.data?.totals
  const before = traffic.data?.comparison?.totals
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pending = traffic.isPending
  const value = (m: NumMetric) =>
    pending ? <Skeleton className="mt-1 h-7 w-16" /> : formatMetric(m, num(totals, m), i18n.locale)
  const visitors = num(totals, "visitors") ?? 0
  const conversations = num(support.data?.totals, "conversations")
  const share1 = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 1 })
  return (
    <>
      <QueryProblem error={traffic.error} />
      <div className="flex flex-wrap gap-2.5" data-testid="analytics-kpis">
        {(!share || share.views.includes("realtime")) && (
          <KpiTile
            live
            label={<Trans>Visitors now</Trans>}
            value={live.data ? fmt.format(live.data.visitors) : <Skeleton className="mt-1 h-7 w-10" />}
            hint={<Trans>live</Trans>}
            testId="kpi-realtime"
          />
        )}
        <KpiTile
          label={<Trans>Visitors</Trans>}
          value={value("visitors")}
          hint={<Delta now={num(totals, "visitors")} before={num(before, "visitors")} metric="visitors" />}
          testId="kpi-visitors"
        />
        <KpiTile
          label={<Trans>Page views</Trans>}
          value={value("pageviews")}
          hint={<Delta now={num(totals, "pageviews")} before={num(before, "pageviews")} metric="pageviews" />}
          testId="kpi-pageviews"
        />
        <KpiTile
          label={<Trans>Bounce</Trans>}
          value={value("bounce_rate")}
          hint={<Delta now={num(totals, "bounce_rate")} before={num(before, "bounce_rate")} metric="bounce_rate" />}
          testId="kpi-bounce_rate"
        />
        {!share && (
          <KpiTile
            label={<Trans>Started a conversation</Trans>}
            value={
              !property.inbox_id ? (
                "–"
              ) : conversations === undefined ? (
                <Skeleton className="mt-1 h-7 w-12" />
              ) : (
                fmt.format(conversations)
              )
            }
            hint={
              !property.inbox_id ? (
                <Link to={`/analytics/${property.id}/support`} className="hover:text-foreground">
                  <Trans>Link an inbox to count them</Trans>
                </Link>
              ) : conversations !== undefined && visitors > 0 ? (
                <span className="text-success">{t`${share1.format(conversations / visitors)} of visitors`}</span>
              ) : undefined
            }
            testId="kpi-conversations"
          />
        )}
      </div>
    </>
  )
}

function Delta({ now, before, metric }: { now?: number; before?: number; metric: NumMetric }) {
  const { i18n } = useLingui()
  if (now === undefined || before === undefined || (before === 0 && metric !== "bounce_rate")) return null
  const rate = metric === "bounce_rate"
  const change = rate ? now - before : (now - before) / before
  if (change === 0) return null
  const good = lowerIsBetter(metric) ? change < 0 : change > 0
  const shown = rate
    ? new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 0, signDisplay: "always" }).format(change)
    : new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0, signDisplay: "always" }).format(
        change
      )
  return (
    <span className={cn("tabular-nums", good ? "text-success" : "text-destructive")} data-testid="kpi-change">
      {rate ? <Trans>{shown} pts</Trans> : shown}
    </span>
  )
}

function supportFilters(view: View) {
  return view.filters.filter((f) => f.dimension === "page" || f.dimension === "hostname")
}

function bucketLabel(format: Intl.DateTimeFormat, v: string) {
  const d = new Date(v.includes("T") ? `${v}:00Z` : `${v}T00:00:00Z`)
  return Number.isNaN(d.getTime()) ? v : format.format(d)
}

export function TimeChart({ metric: fixed, goalId }: { metric?: NumMetric; goalId?: string }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view } = useView()
  const metric = fixed ?? view.metric
  const grain = grainFor(view.range, metric)
  const q = useAnalytics(property.id, {
    metrics: [metric],
    dimensions: [grain],
    range: view.range,
    ...scopeOf(view),
    compare: view.compare,
    goal_id: goalId,
  })
  const notes = useAnnotations(property.id, q.data?.meta.from, q.data?.meta.to)
  const share = useShare()
  const hour = grain === "time:hour"
  const tick = new Intl.DateTimeFormat(
    i18n.locale,
    hour
      ? { timeStyle: "short", timeZone: "UTC" }
      : {
          day: "numeric",
          month: "short",
          timeZone: "UTC",
        }
  )
  const long = new Intl.DateTimeFormat(i18n.locale, {
    dateStyle: "medium",
    ...(hour ? { timeStyle: "short" } : {}),
    timeZone: "UTC",
  })
  const before = q.data?.comparison?.rows ?? []
  const data = (q.data?.rows ?? []).map((r, i) => ({
    at: r.dimensions[0] ?? "",
    value: num(r.metrics, metric) ?? 0,
    previous: view.compare ? (num(before[i]?.metrics, metric) ?? 0) : undefined,
    previousAt: before[i]?.dimensions[0],
  }))
  const config: ChartConfig = {
    value: { label: text.metric[metric], color: "var(--brand)" },
    ...(view.compare ? { previous: { label: t`Compared days`, color: "var(--muted-foreground)" } } : {}),
  }
  const weekly = grain === "time:week"
  return (
    <Panel
      title={
        <>
          {text.metric[metric]}
          <span className="ms-2 text-caption font-normal text-faint">
            {weekly ? <Trans>per week</Trans> : hour ? <Trans>per hour</Trans> : <Trans>per day</Trans>}
          </span>
        </>
      }
      action={
        <>
          {view.compare && (
            <Legend
              items={[
                { color: "var(--brand)", label: text.metric[metric] },
                { color: "var(--muted-foreground)", label: t`Compared days`, dashed: true },
              ]}
            />
          )}
          {!share && <AddNoteButton propertyId={property.id} />}
        </>
      }
      testId="analytics-chart"
    >
      {q.isPending ? (
        <Skeleton className="h-56 w-full" />
      ) : q.error ? (
        <PanelProblem error={q.error} />
      ) : (
        <ChartContainer config={config} className="aspect-auto h-56 w-full">
          <AreaChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -12 }}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="at"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              minTickGap={28}
              interval="preserveStartEnd"
              tickFormatter={(v) => bucketLabel(tick, String(v))}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={48}
              allowDecimals={metric === "views_per_visit" || metric === "conversion_rate"}
              tickFormatter={(v) => formatMetric(metric, Number(v), i18n.locale)}
            />
            <ChartTooltip
              cursor={false}
              content={
                <ChartTooltipContent
                  labelFormatter={(_, p) => (p?.[0] ? bucketLabel(long, String(p[0].payload.at)) : "")}
                  formatter={(value, name, item) => (
                    <div className="flex w-full items-center gap-2">
                      <span
                        className={cn(
                          "size-2.5 shrink-0 rounded-[2px]",
                          name === "previous" ? "border border-dashed border-muted-foreground" : "bg-brand"
                        )}
                      />
                      <span className="flex-1 text-muted-foreground">
                        {name === "previous" && item.payload.previousAt
                          ? bucketLabel(long, String(item.payload.previousAt))
                          : config[String(name)]?.label}
                      </span>
                      <span className="font-mono font-medium text-foreground tabular-nums">
                        {formatMetric(metric, Number(value), i18n.locale)}
                      </span>
                    </div>
                  )}
                />
              }
            />
            {view.compare && (
              <Area
                dataKey="previous"
                type="monotone"
                stroke="var(--color-previous)"
                strokeDasharray="4 4"
                strokeWidth={1.5}
                fill="none"
                isAnimationActive={false}
              />
            )}
            <Area
              dataKey="value"
              type="monotone"
              stroke="var(--color-value)"
              strokeWidth={2}
              fill="var(--color-value)"
              fillOpacity={0.12}
              isAnimationActive={false}
            />
            {annotationMarks(
              notes.data ?? [],
              data.map((d) => d.at),
              grain
            )}
          </AreaChart>
        </ChartContainer>
      )}
      {!share && <AnnotationList propertyId={property.id} items={notes.data ?? []} />}
    </Panel>
  )
}

export function Legend({ items }: { items: { color: string; label: React.ReactNode; dashed?: boolean }[] }) {
  return (
    <ul className="flex flex-wrap items-center gap-x-3.5 gap-y-1 text-small text-muted-foreground">
      {items.map((it, i) => (
        <li key={i} className="flex items-center gap-1.5">
          <span
            className={cn("h-0 w-3.5 border-t-2", it.dashed && "border-dashed")}
            style={{ borderColor: it.color }}
            aria-hidden
          />
          {it.label}
        </li>
      ))}
    </ul>
  )
}

function VisitorsChart() {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const share = useShare()
  const { view } = useView()
  const grain = grainFor(view.range, "visitors")
  const q = useAnalytics(property.id, {
    metrics: ["visitors"],
    dimensions: [grain],
    range: view.range,
    ...scopeOf(view),
  })
  const support = useAnalytics(
    property.id,
    {
      dataset: "support",
      metrics: ["conversations"],
      dimensions: [grain],
      range: view.range,
      filters: supportFilters(view),
    },
    !!property.inbox_id && !share && grain !== "time:hour"
  )
  const hour = grain === "time:hour"
  const tick = new Intl.DateTimeFormat(
    i18n.locale,
    hour ? { timeStyle: "short", timeZone: "UTC" } : { day: "numeric", month: "short", timeZone: "UTC" }
  )
  const long = new Intl.DateTimeFormat(i18n.locale, {
    dateStyle: "medium",
    ...(hour ? { timeStyle: "short" } : {}),
    timeZone: "UTC",
  })
  const asked = new Map(
    (support.data?.rows ?? []).map((r) => [r.dimensions[0] ?? "", num(r.metrics, "conversations") ?? 0])
  )
  const rows = (q.data?.rows ?? []).map((r) => ({
    at: r.dimensions[0] ?? "",
    visitors: num(r.metrics, "visitors") ?? 0,
    conversations: asked.get(r.dimensions[0] ?? "") ?? 0,
  }))
  const showConversations = !!property.inbox_id && !share && !hour
  const config: ChartConfig = {
    visitors: { label: text.metric.visitors, color: "var(--brand)" },
    conversations: { label: t`Started a conversation`, color: "var(--foreground)" },
  }
  return (
    <section className="flex flex-col gap-2 rounded-[14px] border bg-card pb-3" data-testid="analytics-chart">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-4 pt-3.5">
        <h2 className="min-w-0 flex-1 text-[14px] leading-5 font-semibold">{text.metric.visitors}</h2>
        {showConversations && (
          <Legend
            items={[
              { color: "var(--brand)", label: text.metric.visitors },
              { color: "var(--foreground)", label: t`Started a conversation` },
            ]}
          />
        )}
      </div>
      {q.isPending ? (
        <Skeleton className="mx-4 h-[200px]" />
      ) : q.error ? (
        <PanelProblem error={q.error} />
      ) : rows.length === 0 ? (
        <p className="px-4 py-12 text-center text-body text-faint">
          <Trans>Nothing counted in this range.</Trans>
        </p>
      ) : (
        <ChartContainer
          config={config}
          className="aspect-auto h-[200px] w-full px-2"
          role="img"
          aria-label={text.metric.visitors}
        >
          <AreaChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: -4 }}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="at"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              minTickGap={32}
              interval="preserveStartEnd"
              tickFormatter={(v) =>
                !hour && String(v).slice(0, 10) === todayIso() ? t`Today` : bucketLabel(tick, String(v))
              }
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              width={44}
              allowDecimals={false}
              tickFormatter={(v) => formatCompact(Number(v), i18n.locale)}
            />
            <ChartTooltip
              cursor={{ stroke: "var(--faint)", strokeWidth: 1 }}
              content={
                <ChartTooltipContent
                  indicator="line"
                  labelFormatter={(_, p) => (p?.[0] ? bucketLabel(long, String(p[0].payload.at)) : "")}
                  formatter={(value, name) => (
                    <div className="flex w-full items-center gap-2">
                      <span
                        className="h-0 w-3 shrink-0 border-t-2"
                        style={{ borderColor: `var(--color-${String(name)})` }}
                      />
                      <span className="flex-1 text-muted-foreground">{config[String(name)]?.label}</span>
                      <span className="font-mono font-medium text-foreground tabular-nums">
                        {new Intl.NumberFormat(i18n.locale).format(Number(value))}
                      </span>
                    </div>
                  )}
                />
              }
            />
            <Area
              dataKey="visitors"
              type="monotone"
              stroke="var(--color-visitors)"
              strokeWidth={2}
              fill="var(--color-visitors)"
              fillOpacity={0.08}
              activeDot={{ r: 4 }}
              isAnimationActive={false}
            />
            {showConversations && (
              <Area
                dataKey="conversations"
                type="monotone"
                stroke="var(--color-conversations)"
                strokeWidth={2}
                fill="none"
                activeDot={{ r: 4 }}
                isAnimationActive={false}
              />
            )}
          </AreaChart>
        </ChartContainer>
      )}
    </section>
  )
}

function Box({
  group,
  title,
  dims,
  mono,
  testId,
}: {
  group: Group
  title: React.ReactNode
  dims: AnalyticsFilterDimension[]
  mono?: boolean
  testId: string
}) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const share = useShare()
  const search = useViewSearch()
  const { view, addFilter } = useView()
  const [picked, setPicked] = useState(dims[0])
  const dim = dims.includes(picked) ? picked : dims[0]
  const q = useAnalytics(property.id, {
    metrics: ["visitors"],
    dimensions: [dim],
    range: view.range,
    ...scopeOf(view),
    limit: 5,
  })
  const rows = q.data?.rows ?? []
  const max = Math.max(1, ...rows.map((r) => num(r.metrics, "visitors") ?? 0))
  const join = search ? `${search}&by=${dim}` : `?by=${dim}`
  const details = share ? `/share/${share.token}/${group}${join}` : `/analytics/${property.id}/${group}${join}`
  return (
    <section className="flex min-w-0 flex-col overflow-hidden rounded-[14px] border bg-card" data-testid={testId}>
      <div className="flex min-h-11 items-center gap-2 px-4 pt-3 pb-2">
        <h2 className="min-w-0 flex-1 truncate text-[14px] leading-5 font-semibold">
          <Link to={details} className="hover:underline">
            {title}
          </Link>
        </h2>
        {dims.length > 1 && (
          <Segmented
            value={dim}
            onChange={setPicked}
            label={t`Breakdown`}
            itemClassName="h-6 px-2 text-small"
            items={dims.map((d) => ({ value: d, label: text.dimension[d] }))}
          />
        )}
      </div>
      <div className={cn(look.over, "flex justify-between px-4 pb-1.5")}>
        <span className="truncate">{text.dimension[dim]}</span>
        <span>{text.metric.visitors}</span>
      </div>
      {q.isPending ? (
        <Skeleton className="m-4 mt-1 h-36" />
      ) : q.error ? (
        <PanelProblem error={q.error} />
      ) : rows.length === 0 ? (
        <p className="px-4 py-8 text-center text-body text-faint">
          <Trans>Nothing counted in this range.</Trans>
        </p>
      ) : (
        <ul className="pb-1">
          {rows.map((r, i) => {
            const value = r.dimensions[0] ?? ""
            const v = num(r.metrics, "visitors") ?? 0
            const click = value === OTHER || share ? undefined : () => addFilter(dim, value)
            const inner = (
              <>
                <span
                  aria-hidden
                  className="absolute inset-y-[3px] start-0 rounded-md bg-brand/10"
                  style={{ width: `${Math.max(1.5, (v / max) * 100)}%` }}
                />
                <span className={cn("relative min-w-0 flex-1 truncate text-start", mono && "font-mono text-[13px]")}>
                  <RowLabel dim={dim} row={r} value={value} text={text} />
                </span>
                <span className="relative shrink-0 font-semibold tabular-nums">{formatCompact(v, i18n.locale)}</span>
              </>
            )
            const cls = "relative mx-2 flex h-[34px] items-center gap-3 rounded-md px-2 outline-none"
            return (
              <li key={`${value}${i}`}>
                {click ? (
                  <button
                    type="button"
                    onClick={click}
                    className={cn(
                      cls,
                      "w-[calc(100%-1rem)] transition-colors hover:bg-surface focus-visible:bg-surface"
                    )}
                    data-testid="analytics-bar"
                  >
                    {inner}
                  </button>
                ) : (
                  <div className={cls} data-testid="analytics-bar">
                    {inner}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      )}
      <Link
        to={details}
        className="mx-4 mt-auto mb-3 w-fit pt-1 text-small text-muted-foreground outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Trans>Details</Trans>
        <span aria-hidden className="ms-1 inline-block rtl:-scale-x-100">
          →
        </span>
      </Link>
      {group === "locations" && <Attribution propertyId={property.id} className="border-t px-4 py-2" />}
    </section>
  )
}

function VisitorTypeCard() {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view, addFilter } = useView()
  const q = useAnalytics(property.id, {
    metrics: ["visitors"],
    dimensions: ["visitor_type"],
    range: view.range,
    ...scopeOf(view),
  })
  const rows = (q.data?.rows ?? []).filter((r) => r.dimensions[0] === "new" || r.dimensions[0] === "returning")
  return (
    <Panel
      title={<Trans>New and returning</Trans>}
      action={<ConsentedOnly />}
      testId="card-visitor-type"
      footer={
        <Link
          to={`/analytics/${property.id}/retention`}
          className="text-small text-brand underline-offset-4 hover:underline"
        >
          <Trans>Retention</Trans>
        </Link>
      }
    >
      {q.isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : q.error ? (
        <PanelProblem error={q.error} />
      ) : rows.length === 0 ? (
        <p className="px-2 py-6 text-center text-body text-faint">
          <Trans>No consented visitors in this range.</Trans>
        </p>
      ) : (
        <BarList
          head={[text.dimension.visitor_type, text.metric.visitors]}
          items={rows.map((r) => {
            const value = r.dimensions[0] ?? ""
            const v = num(r.metrics, "visitors") ?? 0
            return {
              key: value,
              label: text.visitorType[value],
              value: v,
              display: formatCompact(v, i18n.locale),
              onClick: () => addFilter("visitor_type", value),
            }
          })}
        />
      )}
    </Panel>
  )
}

export function Overview() {
  const property = useCurrentProperty()
  const text = useAnalyticsText()
  const share = useShare()
  const { view } = useView()
  const shown = (g: Group) => !share || share.views.includes(g)
  return (
    <div className="flex flex-col gap-4" data-testid="analytics-overview">
      <Kpis />
      {!share && <OverviewInsights property={property} view={view} />}
      <VisitorsChart />
      <div className="grid gap-4 @3xl:grid-cols-2">
        {shown("pages") && (
          <Box
            group="pages"
            title={text.title.top}
            dims={["page", "entry_page", "exit_page"]}
            mono
            testId="card-pages"
          />
        )}
        {shown("sources") && (
          <Box
            group="sources"
            title={<Trans>Sources</Trans>}
            dims={["channel", "source", "utm_campaign"]}
            testId="card-sources"
          />
        )}
        {shown("locations") && (
          <Box
            group="locations"
            title={<Trans>Locations</Trans>}
            dims={groupDimensions("locations", property)}
            testId="card-locations"
          />
        )}
        {shown("devices") && (
          <Box group="devices" title={<Trans>Devices</Trans>} dims={deviceCard(property)} testId="card-devices" />
        )}
        {!share && offersPersistent(property) && <VisitorTypeCard />}
      </div>
    </div>
  )
}

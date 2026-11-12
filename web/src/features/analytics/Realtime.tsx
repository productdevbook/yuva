import { Trans, useLingui } from "@lingui/react/macro"
import { Bar, BarChart, Cell, XAxis, YAxis } from "recharts"

import { ErrorLine, Stat, StatCard } from "@/components/common"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import { Attribution, BarList, Panel, useCurrentProperty } from "@/features/analytics/parts"
import { useGeoAttribution, useRealtime } from "@/features/analytics/queries"
import { flag, formatCompact, useAnalyticsText, valueText } from "@/features/analytics/text"
import type { AnalyticsDimension } from "@/lib/api"

function Top({
  title,
  dim,
  items,
  footer,
  testId,
}: {
  title: React.ReactNode
  dim: AnalyticsDimension
  items: { value: string; visitors: number }[]
  footer?: React.ReactNode
  testId: string
}) {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  return (
    <Panel title={title} footer={footer} testId={testId}>
      {items.length === 0 ? (
        <p className="px-2 py-6 text-center text-body text-faint">
          <Trans>Nobody in the last 30 minutes.</Trans>
        </p>
      ) : (
        <BarList
          head={[null, text.metric.visitors]}
          items={items.map((it) => ({
            key: it.value,
            label: (
              <>
                {dim === "country" && (
                  <span className="me-1.5" aria-hidden>
                    {flag(it.value)}
                  </span>
                )}
                {valueText(text, dim, it.value, i18n.locale)}
              </>
            ),
            ltr: dim === "page" && it.value !== "",
            value: it.visitors,
            display: formatCompact(it.visitors, i18n.locale),
          }))}
        />
      )}
    </Panel>
  )
}

export function Realtime() {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const live = useRealtime(property.id)
  const geo = useGeoAttribution(property.id)
  const r = live.data
  const fmt = new Intl.NumberFormat(i18n.locale)
  const time = new Intl.DateTimeFormat(i18n.locale, { timeStyle: "short" })
  const config: ChartConfig = { pageviews: { label: text.metric.pageviews, color: "var(--brand)" } }
  return (
    <div className="flex flex-col gap-7" data-testid="analytics-realtime">
      <ErrorLine error={live.error} />
      {!r ? (
        <Skeleton className="h-64 rounded-[10px]" />
      ) : (
        <>
          <StatCard>
            <Stat
              live
              label={<Trans>Visitors in the last 30 minutes</Trans>}
              value={<span data-testid="realtime-visitors">{fmt.format(r.visitors)}</span>}
            />
            <Stat label={text.title.lastHalfHour} value={fmt.format(r.pageviews)} />
          </StatCard>
          <Panel
            title={text.title.perMinute}
            action={
              <span className="text-caption text-faint">
                <Trans>Refreshes every 15 seconds</Trans>
              </span>
            }
            dotted
          >
            <ChartContainer config={config} className="aspect-auto h-44 w-full">
              <BarChart data={r.minutes} margin={{ top: 4, right: 4, bottom: 0, left: -20 }} barCategoryGap={2}>
                <XAxis
                  dataKey="at"
                  tickLine={false}
                  axisLine={{ stroke: "var(--input)" }}
                  tickMargin={8}
                  minTickGap={32}
                  tickFormatter={(v) => time.format(new Date(String(v)))}
                />
                <YAxis allowDecimals={false} tickLine={false} axisLine={false} width={44} />
                <ChartTooltip
                  cursor={false}
                  content={
                    <ChartTooltipContent
                      labelFormatter={(_, p) => (p?.[0] ? time.format(new Date(String(p[0].payload.at))) : "")}
                    />
                  }
                />
                <Bar dataKey="pageviews" radius={[2, 2, 0, 0]} isAnimationActive={false}>
                  {r.minutes.map((m, i) => (
                    <Cell key={m.at} fill={i === r.minutes.length - 1 ? "var(--color-pageviews)" : "var(--bar2)"} />
                  ))}
                </Bar>
              </BarChart>
            </ChartContainer>
          </Panel>
          <div className="grid gap-x-5 gap-y-7 @2xl:grid-cols-2">
            <Top title={text.title.pages} dim="page" items={r.pages} testId="realtime-pages" />
            <Top title={<Trans>Sources</Trans>} dim="source" items={r.sources} testId="realtime-sources" />
            <Top
              title={<Trans>Countries</Trans>}
              dim="country"
              items={r.countries}
              footer={geo && <Attribution propertyId={property.id} />}
              testId="realtime-countries"
            />
          </div>
          <p className="mx-1 text-caption text-faint">
            {t`Only totals are shown. There is no list of individual visitors.`}
          </p>
        </>
      )}
    </div>
  )
}

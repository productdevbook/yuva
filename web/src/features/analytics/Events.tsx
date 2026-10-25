import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, MousePointerClickIcon } from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { Notice, Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { CsvButton, QueryProblem, useCurrentProperty } from "@/features/analytics/parts"
import { downloadCsv, useAnalytics, useAnalyticsTable, useShare } from "@/features/analytics/queries"
import { useView } from "@/features/analytics/state"
import { formatMetric, formatMoney, num, OTHER, useAnalyticsText, valueText } from "@/features/analytics/text"
import type { AnalyticsDimension, AnalyticsFilter, AnalyticsQuery, AnalyticsRevenue, AnalyticsRow } from "@/lib/api"
import { cn } from "@/lib/utils"

const BUILT_IN_URL = ["outbound_click", "file_download"]

function EventTable({
  body,
  dim,
  head,
  onPick,
  pickTitle,
  csvName,
  testId,
}: {
  body: AnalyticsQuery
  dim: AnalyticsDimension
  head: React.ReactNode
  onPick?: (value: string) => void
  pickTitle?: string
  csvName: string
  testId: string
}) {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const table = useAnalyticsTable(property.id, body)
  const rows = table.data?.pages.flatMap((p) => p.rows) ?? []
  const label = (r: AnalyticsRow) => {
    const v = r.dimensions[0] ?? ""
    const shown = valueText(text, dim, v, i18n.locale)
    return (
      <>
        <span dir="auto">{shown}</span>
        {dim === "event" && shown !== v && (
          <span className="ms-2 font-mono text-caption text-faint" dir="ltr">
            {v}
          </span>
        )}
      </>
    )
  }
  return (
    <>
      <div className="flex justify-end">
        <CsvButton testId="analytics-csv" run={() => downloadCsv(property.id, body, csvName)} />
      </div>
      <QueryProblem error={table.error} />
      <div className="overflow-hidden rounded-[14px] border bg-card">
        <table className="w-full table-fixed text-body" data-testid={testId}>
          <thead>
            <tr className="border-b text-caption text-faint">
              <th className="px-4 py-2.5 text-start font-medium">{head}</th>
              <th className="w-28 px-2 py-2.5 text-end font-medium @max-2xl:hidden" title={text.metricHelp.visitors}>
                {text.metric.visitors}
              </th>
              <th className="w-24 px-2 py-2.5 text-end font-medium" title={text.metricHelp.events}>
                {text.metric.events}
              </th>
              <th className="w-40 px-4 py-2.5 text-end font-medium @max-2xl:hidden" title={text.metricHelp.revenue}>
                {text.metric.revenue}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {table.isPending ? (
              <tr>
                <td colSpan={4} className="p-4">
                  <Skeleton className="h-24 w-full" />
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-4 py-6 text-faint">
                  <Trans>Nothing counted in this range.</Trans>
                </td>
              </tr>
            ) : (
              rows.map((r, i) => {
                const v = r.dimensions[0] ?? ""
                return (
                  <tr key={`${v}${i}`} className="hover:bg-muted/50">
                    <td className="truncate px-4 py-2">
                      {onPick && v !== OTHER ? (
                        <button
                          type="button"
                          className="max-w-full truncate rounded text-start outline-none hover:text-brand focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => onPick(v)}
                          title={pickTitle}
                          data-testid="analytics-row"
                        >
                          {label(r)}
                        </button>
                      ) : (
                        label(r)
                      )}
                    </td>
                    <td className="px-2 py-2 text-end tabular-nums @max-2xl:hidden">
                      {formatMetric("visitors", num(r.metrics, "visitors"), i18n.locale)}
                    </td>
                    <td className="px-2 py-2 text-end tabular-nums">
                      {formatMetric("events", num(r.metrics, "events"), i18n.locale)}
                    </td>
                    <td
                      className={cn(
                        "px-4 py-2 text-end tabular-nums @max-2xl:hidden",
                        !r.metrics.revenue?.length && "text-faint"
                      )}
                    >
                      <RevenueAmounts revenue={r.metrics.revenue} />
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
        {table.hasNextPage && (
          <div className="flex justify-center border-t p-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void table.fetchNextPage()}
              disabled={table.isFetchingNextPage}
            >
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </div>
    </>
  )
}

export function RevenueAmounts({ revenue }: { revenue?: AnalyticsRevenue[] }) {
  const { i18n } = useLingui()
  if (!revenue?.length) return "–"
  return revenue.map((r) => (
    <span key={r.currency} className="block whitespace-nowrap">
      {formatMoney(r.total, r.currency, r.digits, i18n.locale)}
    </span>
  ))
}

export function RevenueTiles({ revenue }: { revenue?: AnalyticsRevenue[] }) {
  const { i18n } = useLingui()
  if (!revenue?.length) return null
  return (
    <>
      {revenue.map((r) => {
        const avg = formatMoney(r.average, r.currency, r.digits, i18n.locale)
        const n = new Intl.NumberFormat(i18n.locale).format(r.events)
        return (
          <div
            key={r.currency}
            className="flex min-w-0 flex-col rounded-[14px] border bg-card px-4 py-3 @max-2xl:px-3"
            data-testid="revenue-tile"
          >
            <span className="max-w-full truncate text-small text-faint">
              <Trans>Revenue</Trans> · {r.currency}
            </span>
            <b className="truncate text-page tabular-nums">{formatMoney(r.total, r.currency, r.digits, i18n.locale)}</b>
            <span className="max-w-full truncate text-caption text-faint tabular-nums">
              <Trans>
                {n} payments, {avg} on average
              </Trans>
            </span>
          </div>
        )
      })}
    </>
  )
}

export function Tile({ label, value, help }: { label: string; value: React.ReactNode; help?: string }) {
  return (
    <div className="flex min-w-0 flex-col rounded-[14px] border bg-card px-4 py-3 @max-2xl:px-3" title={help}>
      <span className="max-w-full truncate text-small text-faint">{label}</span>
      <b className="text-page tabular-nums">{value}</b>
    </div>
  )
}

function EventDetail({ name }: { name: string }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view, addFilter } = useView()
  const share = useShare()
  const [params, setParams] = useSearchParams()
  const [other, setOther] = useState("")
  const key = params.get("prop") ?? ""
  const children = property.audience === "children"
  const suggestions = [
    ...(BUILT_IN_URL.includes(name) ? ["url"] : []),
    ...(children ? [] : property.tracked_props),
  ].filter((k, i, all) => all.indexOf(k) === i)
  const only: AnalyticsFilter = { dimension: "event", op: "is", values: [name] }
  const filters = [...view.filters.filter((f) => f.dimension !== "event"), only]
  const totals = useAnalytics(property.id, {
    metrics: ["visitors", "events", "revenue"],
    dimensions: ["event"],
    range: view.range,
    filters,
    limit: 1,
  })
  const row = totals.data?.rows[0]
  const setParam = (k: string, v: string | null) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (v === null) next.delete(k)
        else next.set(k, v)
        if (k === "event") next.delete("prop")
        return next
      },
      { replace: k === "prop" }
    )
  const shown = valueText(text, "event", name, i18n.locale)
  return (
    <section className="flex flex-col gap-4" data-testid="analytics-event">
      <div className="flex flex-col items-start gap-1">
        <Button variant="ghost" size="sm" className="-ms-2" onClick={() => setParam("event", null)}>
          <ArrowLeftIcon className="rtl:rotate-180" />
          <Trans>All events</Trans>
        </Button>
        <h2 className="text-title break-words" data-testid="event-name">
          <span dir="auto">{shown}</span>
          {shown !== name && (
            <span className="ms-2 font-mono text-body text-faint" dir="ltr">
              {name}
            </span>
          )}
        </h2>
      </div>
      <QueryProblem error={totals.error} />
      <div className="grid grid-cols-3 gap-2 @max-2xl:grid-cols-2" data-testid="event-totals">
        {totals.isPending ? (
          <>
            <Skeleton className="h-[74px] rounded-[14px]" />
            <Skeleton className="h-[74px] rounded-[14px]" />
          </>
        ) : (
          <>
            <Tile
              label={text.metric.visitors}
              value={formatMetric("visitors", num(row?.metrics, "visitors") ?? 0, i18n.locale)}
            />
            <Tile
              label={text.metric.events}
              value={formatMetric("events", num(row?.metrics, "events") ?? 0, i18n.locale)}
            />
            <RevenueTiles revenue={row?.metrics.revenue} />
          </>
        )}
      </div>
      <div className="flex flex-col gap-2">
        <h3 className="text-body font-medium">
          <Trans>Break down by prop</Trans>
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          {suggestions.length > 0 && (
            <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
              <Segmented
                value={suggestions.includes(key) ? key : ""}
                onChange={(v) => setParam("prop", v)}
                label={t`Props`}
                items={suggestions.map((k) => ({ value: k, label: <span dir="ltr">{k}</span>, testId: `prop-${k}` }))}
              />
            </div>
          )}
          <form
            className="flex items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              const k = other.trim()
              if (k) setParam("prop", k)
            }}
          >
            <Input
              value={other}
              onChange={(e) => setOther(e.target.value)}
              maxLength={300}
              dir="ltr"
              placeholder={suggestions.length > 0 ? t`Other prop` : t`Prop name, like plan`}
              aria-label={t`Prop name`}
              className="h-8 w-44"
              data-testid="prop-input"
            />
            <Button type="submit" variant="outline" size="sm" disabled={!other.trim()}>
              <Trans>Show</Trans>
            </Button>
          </form>
        </div>
        {children && (
          <p className="text-small text-faint">
            <Trans>Properties for children keep no props, only the address of outbound links and downloads.</Trans>
          </p>
        )}
      </div>
      {key ? (
        <EventTable
          key={key}
          body={{
            metrics: ["visitors", "events", "revenue"],
            dimensions: ["prop"],
            prop: key,
            range: view.range,
            filters,
            order_by: [{ metric: "events", dir: "desc" }],
            limit: 100,
          }}
          dim="prop"
          head={<span dir="ltr">{key}</span>}
          onPick={share ? undefined : (v) => addFilter("prop", v, "is", key)}
          pickTitle={t`Keep only visits with this value`}
          csvName={`${property.name}-${name}-${key}`}
          testId="analytics-props"
        />
      ) : (
        <p className="rounded-[14px] border border-dashed px-4 py-6 text-center text-body text-faint">
          <Trans>Pick a prop to see the values this event carried.</Trans>
        </p>
      )}
      <p className="mx-1 text-caption text-faint">
        <Trans>
          Props of any name can be broken down for the days the detailed records are kept. Earlier days only have the
          tracked props.
        </Trans>
      </p>
    </section>
  )
}

function EventList() {
  const { t } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view } = useView()
  const share = useShare()
  const [, setParams] = useSearchParams()
  return (
    <section className="flex flex-col gap-3" data-testid="analytics-events">
      <EventTable
        body={{
          metrics: ["visitors", "events", "revenue"],
          dimensions: ["event"],
          range: view.range,
          filters: view.filters,
          order_by: [{ metric: "events", dir: "desc" }],
          limit: 100,
        }}
        dim="event"
        head={text.dimension.event}
        onPick={(v) =>
          setParams((prev) => {
            const next = new URLSearchParams(prev)
            next.set("event", v)
            return next
          })
        }
        pickTitle={t`Show its props`}
        csvName={`${property.name}-events`}
        testId="analytics-table"
      />
      {!share && (
        <Notice icon={MousePointerClickIcon}>
          <p>
            <Trans>
              Events come from track() calls on your pages and apps. Outbound links, downloads and chat opens count when
              the script has their attributes.
            </Trans>
          </p>
          <Link
            to={`/analytics/${property.id}/settings?tab=install&snippet=events`}
            className="w-fit text-small text-brand underline-offset-4 hover:underline"
            data-testid="events-install"
          >
            <Trans>How to send events</Trans>
          </Link>
        </Notice>
      )}
    </section>
  )
}

export function EventsView() {
  const [params] = useSearchParams()
  const event = params.get("event")
  return event ? <EventDetail key={event} name={event} /> : <EventList />
}

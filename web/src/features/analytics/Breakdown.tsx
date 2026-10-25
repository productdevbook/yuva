import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, ArrowUpIcon } from "lucide-react"
import { useState } from "react"
import { useSearchParams } from "react-router"

import { Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Attribution, CsvButton, QueryProblem, useCurrentProperty } from "@/features/analytics/parts"
import { downloadCsv, useAnalyticsTable, useShare } from "@/features/analytics/queries"
import { scopeOf, useView } from "@/features/analytics/state"
import {
  flag,
  formatMetric,
  hasApps,
  isPathDimension,
  num,
  OTHER,
  useAnalyticsText,
  valueText,
  type AnalyticsText,
  type NumMetric,
  type Wording,
} from "@/features/analytics/text"
import type { AnalyticsFilterDimension, AnalyticsProperty, AnalyticsQuery, AnalyticsRow } from "@/lib/api"
import { cn } from "@/lib/utils"

export type Group = "pages" | "sources" | "locations" | "devices"

export function groupDimensions(group: Group, p: AnalyticsProperty): AnalyticsFilterDimension[] {
  switch (group) {
    case "pages":
      return ["page", "entry_page", "exit_page", "hostname"]
    case "sources":
      return ["channel", "source", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"]
    case "locations":
      return p.audience === "children" ? ["country"] : ["country", "region", "city"]
    case "devices":
      if (!hasApps(p))
        return ["device", "browser", "browser_version", "os", "os_version", "screen_size", "language", "platform"]
      if (!p.platforms.includes("web")) return ["platform", "app_version", "device", "os", "os_version", "language"]
      return [
        "platform",
        "app_version",
        "device",
        "browser",
        "browser_version",
        "os",
        "os_version",
        "screen_size",
        "language",
      ]
  }
}

export function deviceCard(p: AnalyticsProperty): AnalyticsFilterDimension[] {
  if (!hasApps(p)) return ["device", "browser", "os"]
  return p.platforms.includes("web") ? ["platform", "browser", "os"] : ["platform", "app_version", "os"]
}

export function RowLabel({
  dim,
  row,
  value,
  text,
}: {
  dim: AnalyticsFilterDimension
  row: AnalyticsRow
  value: string
  text: AnalyticsText
}) {
  const { i18n } = useLingui()
  const shown = valueText(text, dim, value, i18n.locale)
  const country = dim === "country" ? value : dim === "region" || dim === "city" ? (row.country ?? "") : ""
  const place = dim === "city" && row.region ? row.region : ""
  return (
    <>
      {country && (
        <span className="me-1.5" aria-hidden>
          {flag(country)}
        </span>
      )}
      <span dir={isPathDimension(dim) && value !== OTHER && value !== "" ? "ltr" : undefined}>{shown}</span>
      {place && <span className="ms-1.5 text-faint">{place}</span>}
    </>
  )
}

function tableMetrics(dim: AnalyticsFilterDimension, wording: Wording): NumMetric[] {
  if (dim !== "page") return ["visitors", "visits", "pageviews", "bounce_rate", "visit_duration"]
  return wording === "screens"
    ? ["visitors", "pageviews", "bounce_rate"]
    : ["visitors", "pageviews", "bounce_rate", "time_on_page", "scroll_depth"]
}

export function BreakdownView({ group }: { group: Group }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view, addFilter } = useView()
  const share = useShare()
  const [params, setParams] = useSearchParams()
  const dims = groupDimensions(group, property)
  const raw = params.get("by") as AnalyticsFilterDimension | null
  const dim = raw && dims.includes(raw) ? raw : dims[0]
  const metrics = tableMetrics(dim, text.wording)
  const [order, setOrder] = useState<{ metric: NumMetric; dir: "asc" | "desc" }>({
    metric: "visitors",
    dir: "desc",
  })
  const sort = metrics.includes(order.metric) ? order : { metric: "visitors" as const, dir: "desc" as const }
  const body: AnalyticsQuery = {
    metrics,
    dimensions: [dim],
    range: view.range,
    ...scopeOf(view),
    order_by: [sort],
    limit: 100,
  }
  const table = useAnalyticsTable(property.id, body)
  const rows = table.data?.pages.flatMap((p) => p.rows) ?? []
  const titles: Record<Group, string> = {
    pages: text.title.pages,
    sources: t`Sources`,
    locations: t`Locations`,
    devices: t`Devices`,
  }
  return (
    <section className="flex flex-col gap-3" data-testid={`analytics-${group}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
          {dims.length > 1 && (
            <Segmented
              value={dim}
              onChange={(v) =>
                setParams(
                  (prev) => {
                    const next = new URLSearchParams(prev)
                    if (v === dims[0]) next.delete("by")
                    else next.set("by", v)
                    return next
                  },
                  { replace: true }
                )
              }
              label={titles[group]}
              items={dims.map((d) => ({ value: d, label: text.dimension[d], testId: `breakdown-${d}` }))}
            />
          )}
        </div>
        <CsvButton testId="analytics-csv" run={() => downloadCsv(property.id, body, `${property.name}-${dim}`)} />
      </div>
      <QueryProblem error={table.error} />
      <div className="overflow-hidden rounded-[14px] border bg-card">
        <table className="w-full table-fixed text-body" data-testid="analytics-table">
          <thead>
            <tr className="border-b text-caption text-faint">
              <th className="px-4 py-2.5 text-start font-medium">{text.dimension[dim]}</th>
              {metrics.map((m, i) => (
                <th key={m} className={cn("w-28 px-2 py-2.5 text-end font-medium", i > 0 && "@max-2xl:hidden")}>
                  <button
                    type="button"
                    className={cn(
                      "inline-flex items-center gap-1 rounded outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
                      sort.metric === m && "text-foreground"
                    )}
                    onClick={() =>
                      setOrder({ metric: m, dir: sort.metric === m && sort.dir === "desc" ? "asc" : "desc" })
                    }
                    aria-sort={sort.metric === m ? (sort.dir === "desc" ? "descending" : "ascending") : undefined}
                    title={text.metricHelp[m]}
                  >
                    {text.metric[m]}
                    {sort.metric === m &&
                      (sort.dir === "desc" ? <ArrowDownIcon className="size-3" /> : <ArrowUpIcon className="size-3" />)}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y">
            {table.isPending ? (
              <tr>
                <td colSpan={metrics.length + 1} className="p-4">
                  <Skeleton className="h-24 w-full" />
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={metrics.length + 1} className="px-4 py-6 text-faint">
                  <Trans>Nothing counted in this range.</Trans>
                </td>
              </tr>
            ) : (
              rows.map((r, i) => {
                const value = r.dimensions[0] ?? ""
                const filterable = value !== OTHER && !share
                return (
                  <tr key={`${value}${i}`} className="hover:bg-muted/50">
                    <td className="truncate px-4 py-2">
                      {filterable ? (
                        <button
                          type="button"
                          className="max-w-full truncate rounded text-start outline-none hover:text-brand focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => addFilter(dim, value)}
                          title={t`Filter by this`}
                          data-testid="analytics-row"
                        >
                          <RowLabel dim={dim} row={r} value={value} text={text} />
                        </button>
                      ) : (
                        <RowLabel dim={dim} row={r} value={value} text={text} />
                      )}
                    </td>
                    {metrics.map((m, j) => (
                      <td key={m} className={cn("px-2 py-2 text-end tabular-nums", j > 0 && "@max-2xl:hidden")}>
                        {formatMetric(m, num(r.metrics, m), i18n.locale)}
                      </td>
                    ))}
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
      {group === "locations" && <Attribution propertyId={property.id} className="mx-1" />}
    </section>
  )
}

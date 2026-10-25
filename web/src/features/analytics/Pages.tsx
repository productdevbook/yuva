import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { Skeleton } from "@/components/ui/skeleton"
import { PanelProblem, QueryProblem, useCurrentProperty, usePropertyInbox } from "@/features/analytics/parts"
import { useAnalytics } from "@/features/analytics/queries"
import { scopeOf, useView, useViewSearch } from "@/features/analytics/state"
import { formatMetric, num, OTHER, useAnalyticsText } from "@/features/analytics/text"
import { ProductTag, tableHeadClass } from "@/features/settings/ui"
import { cn } from "@/lib/utils"

const GRID = "grid grid-cols-[minmax(0,1.4fr)_160px_80px_150px_80px_110px] items-center gap-4 px-4"

type Row = {
  path: string
  visitors?: number
  bounce?: number
  conversations?: number
  helpful?: number
}

export function PagesView() {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const property = useCurrentProperty()
  const { view } = useView()
  const search = useViewSearch()
  const inbox = usePropertyInbox(property.inbox_id)
  const traffic = useAnalytics(property.id, {
    metrics: ["visitors", "bounce_rate"],
    dimensions: ["page"],
    range: view.range,
    ...scopeOf(view),
    limit: 50,
  })
  const support = useAnalytics(
    property.id,
    {
      dataset: "support",
      metrics: ["conversations", "helpful_rate"],
      dimensions: ["page"],
      range: view.range,
      filters: view.filters.filter((f) => f.dimension === "page" || f.dimension === "hostname"),
      limit: 100,
    },
    !!property.inbox_id
  )
  const rows = new Map<string, Row>()
  for (const r of traffic.data?.rows ?? []) {
    const path = r.dimensions[0] ?? ""
    if (path === OTHER) continue
    rows.set(path, { path, visitors: num(r.metrics, "visitors"), bounce: num(r.metrics, "bounce_rate") })
  }
  for (const r of support.data?.rows ?? []) {
    const path = r.dimensions[0] ?? ""
    if (path === OTHER) continue
    const row = rows.get(path) ?? { path }
    row.conversations = num(r.metrics, "conversations")
    row.helpful = num(r.metrics, "helpful_rate")
    rows.set(path, row)
  }
  const list = [...rows.values()].sort(
    (a, b) => (b.conversations ?? 0) - (a.conversations ?? 0) || (b.visitors ?? 0) - (a.visitors ?? 0)
  )
  const max = Math.max(1, ...list.map((r) => r.visitors ?? 0))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const color = inbox?.branding.color || "var(--brand)"
  const linked = !!property.inbox_id
  const page = (path: string) => {
    const p = new URLSearchParams(search)
    p.append("f", `page:is:${path}`)
    return `/analytics/${property.id}?${p.toString()}`
  }
  return (
    <div className="flex flex-col gap-4" data-testid="analytics-pages">
      <p className="text-muted-foreground">
        <Trans>
          Pages ranked by the questions they cause. A page with many visitors and many conversations needs a better
          answer on the page itself.
        </Trans>
      </p>
      <QueryProblem error={traffic.error} />
      <section className="overflow-x-auto rounded-[14px] border bg-card">
        <div className="min-w-[760px]">
          <div className={cn(GRID, tableHeadClass, "py-2.5")}>
            <span>{text.dimension.page}</span>
            <span>{text.metric.visitors}</span>
            <span>
              <Trans>Bounce</Trans>
            </span>
            <span>
              <Trans>Led to a conversation</Trans>
            </span>
            <span>
              <Trans>Helpful</Trans>
            </span>
            <span />
          </div>
          {traffic.isPending ? (
            <Skeleton className="m-4 h-40" />
          ) : traffic.error ? (
            <PanelProblem error={traffic.error} />
          ) : list.length === 0 ? (
            <p className="border-t px-4 py-10 text-center text-body text-faint">
              <Trans>Nothing counted in this range.</Trans>
            </p>
          ) : (
            <ul>
              {list.map((r) => {
                const asked = r.conversations ?? 0
                const hot = asked >= 4
                return (
                  <li key={r.path} className={cn(GRID, "border-t py-3")} data-testid="pages-row">
                    <span className="flex min-w-0 items-center gap-2">
                      {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
                      <Link
                        to={page(r.path)}
                        className="truncate font-mono text-[13px] hover:underline"
                        dir="ltr"
                        title={r.path}
                      >
                        {r.path || "/"}
                      </Link>
                    </span>
                    <span className="flex items-center gap-2">
                      <span className="flex h-1.5 flex-1 overflow-hidden rounded-full bg-border">
                        <span
                          className="rounded-full"
                          style={{ width: `${((r.visitors ?? 0) / max) * 100}%`, backgroundColor: color }}
                        />
                      </span>
                      <span className="w-11 text-end text-caption text-faint tabular-nums">
                        {fmt.format(r.visitors ?? 0)}
                      </span>
                    </span>
                    <span className="text-muted-foreground tabular-nums">
                      {r.bounce === undefined ? "—" : formatMetric("bounce_rate", r.bounce, i18n.locale)}
                    </span>
                    <span className={cn("tabular-nums", hot ? "font-semibold text-brand" : "text-muted-foreground")}>
                      {!linked ? "—" : <Plural value={asked} one="# conversation" other="# conversations" />}
                    </span>
                    <span className="text-muted-foreground tabular-nums">
                      {r.helpful === undefined || r.helpful === null
                        ? "—"
                        : formatMetric("helpful_rate", r.helpful, i18n.locale)}
                    </span>
                    <span>
                      {hot && (
                        <Link
                          to={`/analytics/${property.id}/support${search}`}
                          className="font-medium text-brand hover:underline"
                          data-testid="pages-improve"
                        >
                          <Trans>Improve page →</Trans>
                        </Link>
                      )}
                    </span>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </section>
    </div>
  )
}

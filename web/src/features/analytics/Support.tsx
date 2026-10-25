import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowDownIcon, ArrowUpIcon, FilterIcon, LifeBuoyIcon } from "lucide-react"
import { useState } from "react"
import { Link, useSearchParams } from "react-router"

import { ErrorLine, Notice, Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { RowLabel } from "@/features/analytics/Breakdown"
import { InsightList, SUPPORT_FILTERS } from "@/features/analytics/Insights"
import { Change } from "@/features/analytics/Overview"
import { CsvButton, Panel, QueryProblem, useCurrentProperty } from "@/features/analytics/parts"
import { downloadCsv, useAnalyticsTable, useUpdateProperty } from "@/features/analytics/queries"
import { useView, useViewSearch } from "@/features/analytics/state"
import { formatMetric, num, OTHER, useAnalyticsText, type NumMetric } from "@/features/analytics/text"
import { ChoiceSelect } from "@/features/settings/ui"
import { ApiError, type AnalyticsProperty, type AnalyticsQuery } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type Place = "page" | "hostname"

const PLACES: Place[] = ["page", "hostname"]

const TILES: NumMetric[] = ["conversations", "support_rate", "feedback", "helpful_rate", "discussions", "pageviews"]

const COLUMNS: NumMetric[] = [
  "conversations",
  "support_rate",
  "pageviews",
  "feedback",
  "ratings_up",
  "helpful_rate",
  "discussions",
]

const LOW_VIEWS = 100

function NoInbox({ p }: { p: AnalyticsProperty }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const inboxes = useInboxes().data ?? []
  const update = useUpdateProperty(p.id)
  const [inbox, setInbox] = useState("")
  const search = useViewSearch()
  return (
    <section
      className="flex flex-col items-center gap-2 rounded-[14px] border border-dashed px-4 py-10 text-center"
      data-testid="support-no-inbox"
    >
      <LifeBuoyIcon className="size-6 text-faint" />
      <p className="font-medium">
        <Trans>Link an inbox to see support per page</Trans>
      </p>
      <p className="max-w-md text-body text-faint">
        <Trans>
          Conversations, feedback, ratings and discussions of the linked inbox show here next to each page’s views. They
          are counted per page and day; no visitor is ever matched to a contact.
        </Trans>
      </p>
      {canManage ? (
        inboxes.length === 0 ? (
          <p className="text-body text-faint">
            <Trans>This workspace has no inbox yet.</Trans>
          </p>
        ) : (
          <form
            className="mt-2 flex w-full max-w-md flex-wrap items-center justify-center gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              if (inbox) update.mutate({ inbox_id: inbox })
            }}
          >
            <ChoiceSelect
              label={t`Inbox`}
              value={inbox}
              onChange={setInbox}
              placeholder={t`Choose an inbox`}
              options={inboxes.map((i) => [i.id, i.name] as const)}
              className="min-w-48 flex-1"
            />
            <Button type="submit" disabled={!inbox || update.isPending} data-testid="support-link-inbox">
              <Trans>Link inbox</Trans>
            </Button>
            <p className="w-full text-small text-faint">
              <Trans>Agents of the inbox can then see this property’s numbers too.</Trans>
            </p>
            <ErrorLine error={update.error} />
          </form>
        )
      ) : (
        <p className="text-body text-faint">
          <Trans>An owner or admin can link one in the property’s settings.</Trans>
        </p>
      )}
      {canManage && (
        <Link
          to={`/analytics/${p.id}/settings${search}`}
          className="text-small text-brand underline-offset-4 hover:underline"
        >
          <Trans>Open the property’s settings</Trans>
        </Link>
      )}
    </section>
  )
}

function Tiles({
  totals,
  before,
  loading,
  compare,
}: {
  totals?: Record<string, unknown>
  before?: Record<string, unknown>
  loading: boolean
  compare: boolean
}) {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const value = (m: NumMetric, o?: Record<string, unknown>) =>
    m === "support_rate" && !o?.pageviews ? undefined : (o?.[m] as number | undefined)
  return (
    <div className="grid grid-cols-3 gap-2 @max-2xl:grid-cols-2" data-testid="support-tiles">
      {TILES.map((m) => (
        <div
          key={m}
          className="flex min-w-0 flex-col items-start rounded-[14px] border bg-card px-4 py-3 @max-2xl:px-3"
          title={text.metricHelp[m]}
          data-testid={`support-${m}`}
        >
          <span className="max-w-full truncate text-small text-faint">{text.metric[m]}</span>
          {loading ? (
            <Skeleton className="mt-1 h-7 w-16" />
          ) : (
            <span className="flex flex-wrap items-baseline gap-x-2">
              <b className="text-page tabular-nums">{formatMetric(m, value(m, totals), i18n.locale)}</b>
              {compare && <Change now={value(m, totals)} before={value(m, before)} metric={m} />}
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

export function SupportView() {
  const property = useCurrentProperty()
  if (!property.inbox_id) return <NoInbox p={property} />
  return <SupportReport p={property} />
}

function SupportReport({ p }: { p: AnalyticsProperty }) {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const { view, addFilter } = useView()
  const inboxName = (useInboxes().data ?? []).find((i) => i.id === p.inbox_id)?.name
  const [params, setParams] = useSearchParams()
  const raw = params.get("by") as Place | null
  const dim: Place = raw && PLACES.includes(raw) ? raw : "page"
  const [order, setOrder] = useState<{ metric: NumMetric; dir: "asc" | "desc" } | null>(null)
  const filters = view.filters.filter((f) => SUPPORT_FILTERS.includes(f.dimension))
  const ignored = view.filters.length - filters.length
  const body: AnalyticsQuery = {
    dataset: "support",
    metrics: [...COLUMNS, "ratings_down"],
    dimensions: [dim],
    range: view.range,
    filters,
    compare: view.compare,
    order_by: order ? [order] : undefined,
    insights: true,
    limit: 100,
  }
  const table = useAnalyticsTable(p.id, body)
  if (table.error instanceof ApiError && table.error.code === "no_linked_inbox") return <NoInbox p={p} />
  const first = table.data?.pages[0]
  const rows = table.data?.pages.flatMap((x) => x.rows) ?? []
  const insights = first?.insights ?? []
  const sortable = (m: NumMetric) => m !== "ratings_up"
  const fmt = (m: NumMetric, v?: number) => formatMetric(m, v, i18n.locale)
  return (
    <section className="flex flex-col gap-4" data-testid="analytics-support">
      {ignored > 0 && (
        <Notice icon={FilterIcon} data-testid="support-filters-ignored">
          <Trans>Support numbers can only be filtered by page and host. The other filters are left out here.</Trans>
        </Notice>
      )}
      <QueryProblem error={table.error} />
      <Tiles
        totals={first?.totals as Record<string, unknown> | undefined}
        before={first?.comparison?.totals as Record<string, unknown> | undefined}
        loading={table.isPending}
        compare={!!view.compare}
      />
      {insights.length > 0 && (
        <Panel title={<Trans>What stands out</Trans>} testId="support-insights">
          <InsightList items={insights} />
        </Panel>
      )}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Segmented
          value={dim}
          onChange={(v) =>
            setParams(
              (prev) => {
                const next = new URLSearchParams(prev)
                if (v === "page") next.delete("by")
                else next.set("by", v)
                return next
              },
              { replace: true }
            )
          }
          label={t`Support`}
          items={PLACES.map((d) => ({ value: d, label: text.dimension[d], testId: `support-by-${d}` }))}
        />
        <CsvButton testId="support-csv" run={() => downloadCsv(p.id, body, `${p.name}-support-${dim}`)} />
      </div>
      <div className="overflow-hidden rounded-[14px] border bg-card">
        <table className="w-full table-fixed text-body" data-testid="support-table">
          <thead>
            <tr className="border-b text-caption text-faint">
              <th className="px-4 py-2.5 text-start font-medium">{text.dimension[dim]}</th>
              {COLUMNS.map((m, i) => {
                const label = m === "ratings_up" ? t`Ratings` : text.metric[m]
                const active = order?.metric === m
                return (
                  <th key={m} className={cn("w-24 px-2 py-2.5 text-end font-medium", i > 1 && "@max-2xl:hidden")}>
                    {sortable(m) ? (
                      <button
                        type="button"
                        className={cn(
                          "inline-flex items-center gap-1 rounded text-end outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
                          active && "text-foreground"
                        )}
                        onClick={() =>
                          setOrder(
                            active && order.dir === "asc"
                              ? null
                              : { metric: m, dir: active && order.dir === "desc" ? "asc" : "desc" }
                          )
                        }
                        aria-sort={active ? (order.dir === "desc" ? "descending" : "ascending") : undefined}
                        title={text.metricHelp[m]}
                      >
                        {label}
                        {active &&
                          (order.dir === "desc" ? (
                            <ArrowDownIcon className="size-3" />
                          ) : (
                            <ArrowUpIcon className="size-3" />
                          ))}
                      </button>
                    ) : (
                      <span title={t`Helpful and not helpful`}>{label}</span>
                    )}
                  </th>
                )
              })}
            </tr>
          </thead>
          <tbody className="divide-y">
            {table.isPending ? (
              <tr>
                <td colSpan={COLUMNS.length + 1} className="p-4">
                  <Skeleton className="h-24 w-full" />
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={COLUMNS.length + 1} className="px-4 py-6 text-faint">
                  <Trans>No conversations, feedback, ratings or discussions from these pages in this range.</Trans>
                </td>
              </tr>
            ) : (
              rows.map((r, i) => {
                const value = r.dimensions[0] ?? ""
                const low = dim === "page" && (num(r.metrics, "pageviews") ?? 0) < LOW_VIEWS
                return (
                  <tr key={`${value}${i}`} className="hover:bg-muted/50" data-testid="support-row">
                    <td className="truncate px-4 py-2">
                      {value === OTHER ? (
                        <RowLabel dim={dim} row={r} value={value} text={text} />
                      ) : (
                        <button
                          type="button"
                          className="max-w-full truncate rounded text-start outline-none hover:text-brand focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => addFilter(dim, value)}
                          title={t`Filter by this`}
                        >
                          <RowLabel dim={dim} row={r} value={value} text={text} />
                        </button>
                      )}
                    </td>
                    {COLUMNS.map((m, j) => (
                      <td
                        key={m}
                        className={cn(
                          "px-2 py-2 text-end tabular-nums",
                          j > 1 && "@max-2xl:hidden",
                          low && m === "support_rate" && "text-faint"
                        )}
                        title={low && m === "support_rate" ? t`Fewer than 100 views` : undefined}
                      >
                        {m === "ratings_up" ? (
                          <>
                            <span className="text-success">{fmt("ratings_up", num(r.metrics, "ratings_up"))}</span>
                            <span className="text-faint"> · </span>
                            <span className="text-destructive">
                              {fmt("ratings_down", num(r.metrics, "ratings_down"))}
                            </span>
                          </>
                        ) : m === "support_rate" && !num(r.metrics, "pageviews") ? (
                          "–"
                        ) : (
                          fmt(m, num(r.metrics, m))
                        )}
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
      <p className="mx-1 text-caption text-faint">
        {inboxName ? (
          <Trans>
            From the inbox {inboxName}. Ranked by conversations per 1,000 views; pages with fewer than 100 views come
            after the others. Counted per page and day, never per visitor or contact.
          </Trans>
        ) : (
          <Trans>
            Ranked by conversations per 1,000 views; pages with fewer than 100 views come after the others. Counted per
            page and day, never per visitor or contact.
          </Trans>
        )}
      </p>
    </section>
  )
}

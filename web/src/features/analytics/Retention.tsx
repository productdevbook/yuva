import { Trans, useLingui } from "@lingui/react/macro"
import { RepeatIcon } from "lucide-react"
import { Link, useSearchParams } from "react-router"

import { Segmented } from "@/components/common"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ConsentedOnly, offersPersistent, Panel, QueryProblem, useCurrentProperty } from "@/features/analytics/parts"
import { useRetention, type RetentionGrain } from "@/features/analytics/queries"
import type { AnalyticsCohort } from "@/lib/api"
import { useSession } from "@/lib/session"

const GRAINS: RetentionGrain[] = ["day", "week", "month"]

function Empty({
  title,
  children,
  action,
}: {
  title: React.ReactNode
  children: React.ReactNode
  action?: React.ReactNode
}) {
  return (
    <div
      className="flex flex-col items-center gap-2 rounded-[14px] border border-dashed px-4 py-10 text-center"
      data-testid="retention-empty"
    >
      <RepeatIcon className="size-6 text-faint" />
      <p className="font-medium">{title}</p>
      <div className="flex max-w-md flex-col gap-2 text-body text-faint">{children}</div>
      {action}
    </div>
  )
}

function cohortLabel(cohort: string, grain: RetentionGrain, locale: string) {
  const d = new Date(`${cohort}T12:00:00Z`)
  return new Intl.DateTimeFormat(
    locale,
    grain === "month" ? { month: "long", year: "numeric", timeZone: "UTC" } : { dateStyle: "medium", timeZone: "UTC" }
  ).format(d)
}

function Table({ cohorts, grain }: { cohorts: AnalyticsCohort[]; grain: RetentionGrain }) {
  const { t, i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const width = Math.max(1, ...cohorts.map((c) => c.periods.length))
  const columns = Array.from({ length: width - 1 }, (_, i) => i + 1)
  const head = (period: number) =>
    grain === "day" ? t`Day ${period}` : grain === "week" ? t`Week ${period}` : t`Month ${period}`
  return (
    <div className="-mx-3 overflow-x-auto">
      <table className="w-full border-separate border-spacing-0.5 text-body" data-testid="retention-table">
        <thead>
          <tr className="text-caption text-faint">
            <th className="sticky start-0 z-10 bg-card px-3 py-2 text-start font-medium">
              {grain === "day" ? (
                <Trans>First day</Trans>
              ) : grain === "week" ? (
                <Trans>First week</Trans>
              ) : (
                <Trans>First month</Trans>
              )}
            </th>
            <th className="px-2 py-2 text-end font-medium">
              <Trans>Visitors</Trans>
            </th>
            {columns.map((i) => (
              <th key={i} className="min-w-14 px-2 py-2 text-end font-medium whitespace-nowrap">
                {head(i)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {[...cohorts].reverse().map((c) => (
            <tr key={c.cohort} data-testid="retention-cohort">
              <th
                scope="row"
                className="sticky start-0 z-10 bg-card px-3 py-1.5 text-start font-normal whitespace-nowrap text-muted-foreground"
              >
                {cohortLabel(c.cohort, grain, i18n.locale)}
              </th>
              <td className="px-2 py-1.5 text-end tabular-nums">{n.format(c.visitors)}</td>
              {columns.map((i) => {
                const back = c.periods[i]
                if (back === undefined) return <td key={i} />
                const share = c.visitors ? back / c.visitors : 0
                return (
                  <td
                    key={i}
                    className="rounded-md px-2 py-1.5 text-end tabular-nums"
                    style={{
                      background: `color-mix(in oklab, var(--brand) ${Math.round(6 + share * 50)}%, transparent)`,
                    }}
                    title={`${n.format(back)} / ${n.format(c.visitors)}`}
                  >
                    {pct.format(share)}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function RetentionView() {
  const { t, i18n } = useLingui()
  const { canManage } = useSession()
  const property = useCurrentProperty()
  const [params, setParams] = useSearchParams()
  const raw = params.get("grain") as RetentionGrain | null
  const grain = raw && GRAINS.includes(raw) ? raw : "week"
  const on = offersPersistent(property)
  const q = useRetention(property.id, grain, on)
  if (!on) {
    return (
      <Empty
        title={<Trans>Retention needs consent features</Trans>}
        action={
          canManage &&
          property.audience !== "children" && (
            <Button
              className="mt-2"
              variant="outline"
              render={<Link to={`/analytics/${property.id}/settings?tab=tracking`} />}
              data-testid="retention-settings"
            >
              <Trans>Open consent features</Trans>
            </Button>
          )
        }
      >
        {property.audience === "children" ? (
          <p>
            <Trans>Properties for children never keep an id on the device, so they have no retention.</Trans>
          </p>
        ) : (
          <>
            <p>
              <Trans>
                Without cookies a visitor is new every day, so nobody can be followed from one day to the next. Only
                visitors who agree to a persistent id in your consent tool show up here.
              </Trans>
            </p>
            <p>
              {canManage ? (
                <Trans>
                  Turn on the persistent id in the consent features, then call yuva.consent with persistent set to true.
                </Trans>
              ) : (
                <Trans>An owner or admin can turn on the persistent id in the consent features.</Trans>
              )}
            </p>
          </>
        )}
      </Empty>
    )
  }
  const builtAt = q.data?.built_at ? formatDateTime(q.data.built_at, i18n.locale) : ""
  return (
    <section className="flex flex-col gap-3" data-testid="analytics-retention">
      <div className="-mx-1 max-w-[calc(100%+0.5rem)] overflow-x-auto px-1 py-1">
        <Segmented
          value={grain}
          onChange={(v) =>
            setParams(
              (prev) => {
                const next = new URLSearchParams(prev)
                if (v === "week") next.delete("grain")
                else next.set("grain", v)
                return next
              },
              { replace: true }
            )
          }
          label={t`Cohorts by`}
          items={[
            { value: "day", label: t`Days`, testId: "retention-day" },
            { value: "week", label: t`Weeks`, testId: "retention-week" },
            { value: "month", label: t`Months`, testId: "retention-month" },
          ]}
        />
      </div>
      <QueryProblem error={q.error} />
      {q.isPending ? (
        <Skeleton className="h-64 rounded-[14px]" />
      ) : q.data && q.data.cohorts.length === 0 ? (
        <Empty title={<Trans>No consented visitors yet</Trans>}>
          <p>
            <Trans>
              Cohorts fill once visitors agree to a persistent id and come back. The numbers are rebuilt every hour.
            </Trans>
          </p>
        </Empty>
      ) : q.data ? (
        <Panel
          title={<Trans>Visitors who came back</Trans>}
          action={<ConsentedOnly />}
          testId="retention-panel"
          footer={
            <p className="text-caption text-faint">
              {grain === "day" ? (
                <Trans>
                  Each row holds the visitors first seen that day, each column the share of them who came back that many
                  days later.
                </Trans>
              ) : grain === "week" ? (
                <Trans>
                  Each row holds the visitors first seen that week, each column the share of them who came back that
                  many weeks later.
                </Trans>
              ) : (
                <Trans>
                  Each row holds the visitors first seen that month, each column the share of them who came back that
                  many months later.
                </Trans>
              )}{" "}
              {builtAt ? <Trans>Rebuilt every hour, last at {builtAt}.</Trans> : <Trans>Rebuilt every hour.</Trans>}
            </p>
          }
        >
          <Table cohorts={q.data.cohorts} grain={grain} />
        </Panel>
      ) : null}
    </section>
  )
}

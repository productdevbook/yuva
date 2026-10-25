import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ThumbsDownIcon, ThumbsUpIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { Bar, BarChart, CartesianGrid, Cell, XAxis, YAxis } from "recharts"
import { Outlet, useLocation, useSearchParams } from "react-router"

import { ErrorLine, InboxMark } from "@/components/common"
import { ChipGroup } from "@/components/common/ChipGroup"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Segmented } from "@/components/common/Segmented"
import { formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { KpiTile } from "@/features/analytics/Overview"
import { Panel } from "@/features/analytics/parts"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import {
  createdSince,
  dayKey,
  dayStart,
  ratedSince,
  share,
  useInboxStats,
  useRecentConversations,
  useWindowStats,
} from "@/features/reports/data"
import {
  headClass,
  LoadProblem,
  ReportSkeleton,
  SlaBar,
  SlotsContext,
  Tiles,
  ToolbarPortal,
} from "@/features/reports/parts"
import { Face, headButton, ProductTag } from "@/features/settings/ui"
import { useNow } from "@/hooks/use-now"
import type { ConversationListItem, Stats } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes, useMembers } from "@/lib/workspace"

const RANGES = ["today", "7d", "30d"] as const
type Range = (typeof RANGES)[number]

function useRange(): Range {
  const [params] = useSearchParams()
  const raw = params.get("range") as Range | null
  return raw && RANGES.includes(raw) ? raw : "today"
}

function startOf(range: Range) {
  return dayStart(range === "today" ? 0 : range === "7d" ? 6 : 29)
}

function sinceOf(range: Range) {
  return range === "today" ? undefined : startOf(range).toISOString()
}

function useReportStats() {
  const [inboxId] = useInboxFilter()
  return useWindowStats(inboxId, sinceOf(useRange()))
}

function useRecent() {
  const [inboxId] = useInboxFilter()
  const since = startOf(useRange()).toISOString()
  const recent = useRecentConversations(since, inboxId)
  return { since, recent }
}

function useRangeText() {
  const { t } = useLingui()
  return { today: t`Today`, "7d": t`Last 7 days`, "30d": t`Last 30 days` } satisfies Record<Range, string>
}

function csvCell(v: string | number) {
  const s = String(v)
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
}

function useScopeName() {
  const { t } = useLingui()
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  return (inboxId && inboxes.find((i) => i.id === inboxId)?.name) || t`all products`
}

function useCsv() {
  const range = useRange()
  const [inboxId] = useInboxFilter()
  const stats = useReportStats().data
  const members = new Map((useMembers().data ?? []).map((m) => [m.id, m]))
  const inboxes = (useInboxes().data ?? []).filter((i) => !inboxId || i.id === inboxId)
  const perInbox = useInboxStats(
    inboxes.map((i) => i.id),
    sinceOf(range)
  )
  const ready = !!stats && perInbox.every((q) => q.data)
  const rows: (string | number)[][] = []
  if (stats) {
    const ratings = new Map(stats.ratings.inboxes.map((r) => [r.inbox_id, r]))
    rows.push(
      [
        "kind",
        "name",
        "replies",
        "closed",
        "first_replies",
        "median_first_reply_seconds",
        "median_resolution_seconds",
        "good",
        "bad",
      ],
      [
        "total",
        range,
        stats.replies,
        stats.closed,
        stats.first_replies,
        stats.median_first_reply_seconds ?? "",
        stats.median_resolution_seconds ?? "",
        stats.ratings.good,
        stats.ratings.bad,
      ],
      ...inboxes.map((inbox, i) => {
        const s = perInbox[i]?.data
        const r = ratings.get(inbox.id)
        return [
          "inbox",
          inbox.name,
          s?.replies ?? "",
          s?.closed ?? "",
          s?.first_replies ?? "",
          s?.median_first_reply_seconds ?? "",
          s?.median_resolution_seconds ?? "",
          r?.good ?? 0,
          r?.bad ?? 0,
        ]
      }),
      ...stats.members.map((m) => {
        const member = members.get(m.member_id)
        return ["member", member ? member.name || member.email : m.member_id, m.replies, m.closed, "", "", "", "", ""]
      })
    )
  }
  const lines = rows.map((r) => r.map(csvCell).join(","))
  const download = () => {
    const blob = new Blob([lines.join("\n") + "\n"], { type: "text/csv" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `yuva-report-${range}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }
  return { ready, lines, download, name: `yuva-report-${range}.csv` }
}

function CsvDialog({ csv, onClose }: { csv: ReturnType<typeof useCsv>; onClose: () => void }) {
  const names = useRangeText()
  const range = names[useRange()]
  const scope = useScopeName()
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl" data-testid="report-csv">
        <DialogHeader>
          <DialogTitle>
            <Trans>Download CSV</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              One row for the total, one per product and one per member, for {range} in {scope}.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div
          className="overflow-x-auto rounded-lg border bg-surface px-3 py-2.5 font-mono text-caption leading-[1.7] text-muted-foreground"
          data-testid="report-csv-preview"
        >
          {csv.lines.map((l, i) => (
            <div key={i} className="whitespace-nowrap">
              {l}
            </div>
          ))}
        </div>
        <DialogFooter className="items-center">
          <span className="min-w-0 flex-1 truncate font-mono text-caption text-faint">{csv.name}</span>
          <Button variant="outline" className={headButton.secondary} onClick={onClose}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            className={headButton.primary}
            onClick={() => {
              csv.download()
              onClose()
            }}
            data-testid="report-csv-download"
          >
            <Trans>Download</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PeriodBar() {
  const { t, i18n } = useLingui()
  const range = useRange()
  const names = useRangeText()
  const scope = useScopeName()
  const now = new Date(useNow(60_000))
  const [, setParams] = useSearchParams()
  const day = new Intl.DateTimeFormat(i18n.locale, { month: "short", day: "numeric" })
  const note =
    range === "today"
      ? new Intl.DateTimeFormat(i18n.locale, { weekday: "long", month: "short", day: "numeric" }).format(now)
      : `${day.format(startOf(range))} – ${day.format(now)}`
  return (
    <>
      <Segmented
        value={range}
        label={t`Period`}
        onChange={(v) =>
          setParams(
            (prev) => {
              const next = new URLSearchParams(prev)
              if (v === "today") next.delete("range")
              else next.set("range", v)
              return next
            },
            { replace: true }
          )
        }
        items={RANGES.map((r) => ({ value: r, label: names[r], testId: `report-range-${r}` }))}
      />
      <span className="shrink-0 text-small whitespace-nowrap text-faint" data-testid="report-range-note">
        {note} · {scope}
      </span>
    </>
  )
}

export function ReportsPane() {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const current = pathname.split("/")[2] ?? "today"
  const trends = current === "trends"
  const csv = useCsv()
  const [csvOpen, setCsvOpen] = useState(false)
  const [toolbar, setToolbar] = useState<HTMLElement | null>(null)
  const [actions, setActions] = useState<HTMLElement | null>(null)
  const slots = useMemo(() => ({ toolbar, actions }), [toolbar, actions])
  const tabs = [
    { key: "today", to: { pathname: "/reports/today", search }, label: t`Overview`, testId: "report-tab-today" },
    { key: "trends", to: { pathname: "/reports/trends", search: "" }, label: t`Trends`, testId: "report-tab-trends" },
    { key: "ratings", to: { pathname: "/reports/ratings", search }, label: t`Ratings`, testId: "report-tab-ratings" },
    { key: "members", to: { pathname: "/reports/members", search }, label: t`Team`, testId: "report-tab-members" },
  ]
  return (
    <SlotsContext.Provider value={slots}>
      <main className="@container flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="reports">
        <PageTop
          title={<Trans>Reports</Trans>}
          sub={<Trans>How fast and how well your team answers</Trans>}
          actions={
            <>
              {!trends && (
                <Button
                  className={look.btnInk}
                  disabled={!csv.ready}
                  onClick={() => setCsvOpen(true)}
                  data-testid="report-export"
                >
                  <Trans>Download CSV</Trans>
                </Button>
              )}
              <span ref={setActions} className="contents" />
            </>
          }
          tabs={<FrameTabs label={t`Reports`} value={current} items={tabs} />}
        />
        <div
          ref={setToolbar}
          className="flex flex-wrap items-center gap-2 border-b px-6 py-3 empty:hidden phone:px-4"
          data-testid="reports-toolbar"
        />
        {!trends && (
          <ToolbarPortal>
            <PeriodBar />
          </ToolbarPortal>
        )}
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto flex w-full max-w-[1240px] flex-col gap-4 p-6 pb-16 @max-xl:p-4 @max-xl:pb-16">
            <Outlet />
          </div>
        </div>
        {csvOpen && <CsvDialog csv={csv} onClose={() => setCsvOpen(false)} />}
      </main>
    </SlotsContext.Provider>
  )
}

function Report({ children, tiles }: { children: (s: Stats) => React.ReactNode; tiles?: number }) {
  const stats = useReportStats()
  return (
    <>
      {stats.isPending && <ReportSkeleton tiles={tiles} testId="report-loading" />}
      {stats.error && !stats.data ? (
        <LoadProblem error={stats.error} onRetry={() => void stats.refetch()} testId="report-error" />
      ) : (
        <ErrorLine error={stats.error} />
      )}
      {stats.data && children(stats.data)}
    </>
  )
}

function NewChart({ items, more }: { items: ConversationListItem[]; more: boolean }) {
  const { t, i18n } = useLingui()
  const range = useRange()
  const now = new Date(useNow(60_000))
  const hourly = range === "today"
  const fmt = new Intl.NumberFormat(i18n.locale)
  let slots: { key: string; label: string; n: number }[]
  if (hourly) {
    const hours = items.map((c) => new Date(c.created_at!).getHours())
    const from = Math.min(8, ...hours)
    const to = Math.max(now.getHours(), from + 5)
    const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
    slots = Array.from({ length: to - from + 1 }, (_, i) => {
      const at = new Date(now)
      at.setHours(from + i, 0, 0, 0)
      return { key: String(from + i), label: time.format(at), n: hours.filter((h) => h === from + i).length }
    })
  } else {
    const days = range === "7d" ? 7 : 30
    const label = new Intl.DateTimeFormat(i18n.locale, { day: "numeric", month: "short" })
    slots = Array.from({ length: days }, (_, i) => {
      const d = dayStart(days - 1 - i)
      const key = dayKey(d)
      return { key, label: label.format(d), n: items.filter((c) => dayKey(new Date(c.created_at!)) === key).length }
    })
  }
  const total = `${fmt.format(items.length)}${more ? "+" : ""}`
  const last = hourly ? slots.findIndex((s) => Number(s.key) === now.getHours()) : slots.length - 1
  const config: ChartConfig = { n: { label: t`New conversations`, color: "var(--brand)" } }
  return (
    <Panel
      title={hourly ? <Trans>New conversations by hour</Trans> : <Trans>New conversations by day</Trans>}
      action={
        <span className="text-caption text-faint tabular-nums">
          <Trans>{total} in total</Trans>
        </span>
      }
      testId="report-chart"
    >
      <ChartContainer
        config={config}
        className="aspect-auto h-44 w-full px-1"
        role="img"
        aria-label={t`New conversations`}
      >
        <BarChart data={slots} margin={{ top: 8, right: 8, bottom: 0, left: -4 }} barCategoryGap="22%">
          <CartesianGrid vertical={false} />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={28}
            interval="preserveStartEnd"
          />
          <YAxis tickLine={false} axisLine={false} width={36} allowDecimals={false} />
          <ChartTooltip
            cursor={{ fill: "var(--muted)", fillOpacity: 0.6 }}
            content={
              <ChartTooltipContent
                indicator="line"
                hideLabel={false}
                labelFormatter={(_, p) => p?.[0]?.payload.label}
              />
            }
          />
          <Bar dataKey="n" fill="var(--color-n)" radius={[3, 3, 0, 0]} maxBarSize={22} isAnimationActive={false}>
            {slots.map((s, i) => (
              <Cell key={s.key} fillOpacity={i === last ? 1 : 0.5} />
            ))}
          </Bar>
        </BarChart>
      </ChartContainer>
    </Panel>
  )
}

const PRODUCT_GRID =
  "grid grid-cols-[minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.3fr)] items-center gap-3 px-4"

function SlaCounts({ met, breached }: { met: number; breached: number }) {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  if (met + breached === 0) return <span className="text-muted-foreground">–</span>
  return (
    <>
      <span className="text-success">{fmt.format(met)}</span>
      <span className="text-faint"> / </span>
      <span className={breached ? "text-destructive" : "text-muted-foreground"}>{fmt.format(breached)}</span>
    </>
  )
}

function SlaCell({ met, breached, testId }: { met: number; breached: number; testId: string }) {
  return (
    <span className="flex items-center gap-2 tabular-nums" data-testid={testId}>
      <SlaBar met={met} breached={breached} />
      <span className="min-w-0">
        <SlaCounts met={met} breached={breached} />
      </span>
    </span>
  )
}

function ByProduct({ items }: { items: ConversationListItem[] }) {
  const { i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const since = sinceOf(useRange())
  const inboxes = (useInboxes().data ?? []).filter((i) => !inboxId || i.id === inboxId)
  const stats = useInboxStats(
    inboxes.map((i) => i.id),
    since
  )
  const fmt = new Intl.NumberFormat(i18n.locale)
  const rows = inboxes
    .map((inbox, i) => ({ inbox, s: stats[i]?.data, n: items.filter((c) => c.inbox_id === inbox.id).length }))
    .sort((a, b) => b.n - a.n)
  return (
    <section className="overflow-x-auto rounded-[14px] border bg-card" data-testid="report-inboxes">
      <div className="min-w-[560px]">
        <div className={cn(PRODUCT_GRID, headClass, "h-9 border-b")}>
          <span>
            <Trans>Product</Trans>
          </span>
          <span>
            <Trans>New</Trans>
          </span>
          <span>
            <Trans>First reply</Trans>
          </span>
          <span>
            <Trans>Resolved in</Trans>
          </span>
          <span>
            <Trans>SLA met / breached</Trans>
          </span>
        </div>
        {rows.map(({ inbox, s, n }) => {
          const cell = (v: number | undefined) =>
            !s ? <Skeleton className="h-4 w-10" /> : v !== undefined ? formatDuration(v, i18n.locale) : "–"
          return (
            <div
              key={inbox.id}
              className={cn(PRODUCT_GRID, "min-h-[46px] border-b text-body last:border-b-0")}
              data-testid="report-inbox"
            >
              <span className="flex min-w-0 items-center gap-2.5">
                <InboxMark name={inbox.name} color={inbox.branding.color} className="size-[22px]" />
                <span className="truncate font-medium">{inbox.name}</span>
              </span>
              <span className="text-muted-foreground tabular-nums">{fmt.format(n)}</span>
              <span className="text-muted-foreground tabular-nums">{cell(s?.median_first_reply_seconds)}</span>
              <span className="text-muted-foreground tabular-nums">{cell(s?.median_resolution_seconds)}</span>
              {s ? (
                <SlaCell met={s.sla.met} breached={s.sla.breached} testId="report-inbox-sla" />
              ) : (
                <Skeleton className="h-4 w-10" />
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}

export function TodayReport() {
  const { t, i18n } = useLingui()
  const range = useRange()
  const { since, recent } = useRecent()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const fresh = createdSince(recent.data?.items ?? [], since)
  return (
    <Report>
      {(s) => {
        const met = fmt.format(s.sla.met)
        const breached = fmt.format(s.sla.breached)
        return (
          <>
            <Tiles testId="report-tiles">
              <KpiTile
                label={t`New`}
                value={recent.data ? `${fmt.format(fresh.length)}${recent.data.complete ? "" : "+"}` : "–"}
                hint={t`conversations started`}
                testId="report-new"
              />
              {s.sla.met + s.sla.breached > 0 && (
                <KpiTile
                  label={t`SLA met`}
                  value={new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(
                    s.sla.met / (s.sla.met + s.sla.breached)
                  )}
                  hint={t`${met} met, ${breached} breached`}
                  testId="report-sla"
                />
              )}
              <KpiTile label={t`Replied`} value={fmt.format(s.replies)} hint={t`messages sent`} />
              <KpiTile label={t`Closed`} value={fmt.format(s.closed)} hint={t`conversations`} />
              <KpiTile
                label={t`First reply`}
                value={
                  s.median_first_reply_seconds !== undefined
                    ? formatDuration(s.median_first_reply_seconds, i18n.locale)
                    : "–"
                }
                hint={range === "today" ? t`median today` : t`median`}
              />
              <KpiTile
                label={t`Resolved in`}
                value={
                  s.median_resolution_seconds !== undefined
                    ? formatDuration(s.median_resolution_seconds, i18n.locale)
                    : "–"
                }
                hint={t`median, first message to closed`}
                testId="report-resolved"
              />
            </Tiles>
            {recent.isPending ? (
              <Skeleton className="h-60 rounded-[14px]" />
            ) : (
              <NewChart items={fresh} more={!recent.data?.complete} />
            )}
            <ByProduct items={fresh} />
          </>
        )
      }}
    </Report>
  )
}

function Score({ good }: { good: boolean }) {
  const { t } = useLingui()
  return good ? (
    <ThumbsUpIcon className="size-4 text-success" aria-label={t`Good`} />
  ) : (
    <ThumbsDownIcon className="size-4 text-destructive" aria-label={t`Bad`} />
  )
}

type CommentFilter = "all" | "bad" | "good"

function daysAgo(iso: string, locale: string) {
  const days = Math.round((dayStart(0).getTime() - dayStart(0, new Date(iso)).getTime()) / 86400000)
  return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(-days, "day")
}

export function RatingsReport() {
  const { t, i18n } = useLingui()
  const range = useRange()
  const [, setParams] = useSearchParams()
  const [filter, setFilter] = useState<CommentFilter>("all")
  const { since, recent } = useRecent()
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const members = new Map((useMembers().data ?? []).map((m) => [m.id, m]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const written = ratedSince(recent.data?.items ?? [], since).filter((c) => c.rating?.comment)
  const shown = written.filter((c) => filter === "all" || (filter === "good") === (c.rating!.rating === "good"))
  return (
    <Report tiles={1}>
      {(s) => {
        const rated = s.ratings.good + s.ratings.bad
        if (rated === 0) {
          return (
            <EmptyBox
              icon={ThumbsUpIcon}
              title={<Trans>No ratings in this period</Trans>}
              className="rounded-[14px] border-solid bg-card"
              testId="report-no-ratings"
              actions={
                range !== "30d" && (
                  <Button
                    variant="outline"
                    className={look.btnSm}
                    onClick={() =>
                      setParams((prev) => new URLSearchParams({ ...Object.fromEntries(prev), range: "30d" }), {
                        replace: true,
                      })
                    }
                    data-testid="report-widen"
                  >
                    <Trans>Show the last 30 days</Trans>
                  </Button>
                )
              }
            >
              <Trans>People rate a conversation with a thumb when it is closed. Try a longer period.</Trans>
            </EmptyBox>
          )
        }
        return (
          <div
            className="grid items-start gap-4 grid-cols-[340px_minmax(0,1fr)] @max-[1000px]:grid-cols-1"
            data-testid="report-ratings"
          >
            <section className="flex flex-col gap-3.5 rounded-[14px] border bg-card p-5">
              <span className="text-small text-faint">
                <Trans>Satisfied</Trans>
              </span>
              <span className="flex flex-wrap items-baseline gap-x-2.5">
                <b className="text-[40px] leading-[44px] font-semibold tracking-[-0.02em] tabular-nums">
                  {pct.format(share(s.ratings.good, s.ratings.bad) ?? 0)}
                </b>
                <span className="text-small text-muted-foreground">
                  <Trans>good</Trans> · <Plural value={rated} one="# rating" other="# ratings" />
                </span>
              </span>
              <span className="flex h-2.5 gap-0.5" role="img" aria-label={t`Satisfied`}>
                {s.ratings.good > 0 && (
                  <i
                    className="block h-full rounded-s-full bg-success"
                    style={{ flexGrow: s.ratings.good, flexBasis: 0 }}
                  />
                )}
                {s.ratings.bad > 0 && (
                  <i
                    className="block h-full min-w-1 rounded-e-full bg-destructive"
                    style={{ flexGrow: s.ratings.bad, flexBasis: 0 }}
                  />
                )}
              </span>
              <span className="flex justify-between text-small text-muted-foreground tabular-nums">
                <span className="inline-flex items-center gap-1.5">
                  <Score good />
                  {fmt.format(s.ratings.good)}
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <Score good={false} />
                  {fmt.format(s.ratings.bad)}
                </span>
              </span>
            </section>
            <Panel
              title={<Trans>What people wrote</Trans>}
              action={
                <ChipGroup
                  value={filter}
                  onChange={setFilter}
                  label={t`Show`}
                  options={[
                    ["all", t`All`],
                    ["bad", t`Bad only`],
                    ["good", t`Good only`],
                  ]}
                  className="shrink-0 flex-nowrap"
                />
              }
            >
              <div className="-mx-3 -mb-3 border-t">
                {recent.isPending ? (
                  <Skeleton className="m-4 h-24" />
                ) : shown.length === 0 ? (
                  <p className="px-4 py-10 text-center text-body text-faint">
                    <Trans>Nobody left a comment with a rating in this period.</Trans>
                  </p>
                ) : (
                  <ul data-testid="report-comments">
                    {shown.slice(0, 20).map((c) => {
                      const inbox = inboxes.get(c.inbox_id)
                      const by = c.assignee_id ? members.get(c.assignee_id) : undefined
                      const name = by ? by.name || by.email : ""
                      const who = c.contact.name || c.contact.email || ""
                      const good = c.rating!.rating === "good"
                      return (
                        <li key={c.id} className="flex items-start gap-3 border-b px-4 py-3.5 last:border-b-0">
                          <span
                            className={cn(
                              "grid size-7 shrink-0 place-items-center rounded-full",
                              good ? "bg-success/15" : "bg-destructive/15"
                            )}
                          >
                            <Score good={good} />
                          </span>
                          <span className="flex min-w-0 flex-1 flex-col gap-1 leading-[1.45]">
                            <span className="text-body break-words">“{c.rating!.comment}”</span>
                            <span className="flex min-w-0 flex-wrap items-center gap-1.5 text-caption text-faint">
                              {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
                              <span>
                                {name ? (
                                  <Trans>
                                    {who} · answered by {name}
                                  </Trans>
                                ) : (
                                  who
                                )}{" "}
                                · {daysAgo(c.rating!.rated_at, i18n.locale)}
                              </span>
                            </span>
                          </span>
                        </li>
                      )
                    })}
                  </ul>
                )}
              </div>
            </Panel>
          </div>
        )
      }}
    </Report>
  )
}

const MEMBER_GRID =
  "grid grid-cols-[minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_minmax(0,1.1fr)_minmax(0,1.3fr)] items-center gap-4 px-4"

type MemberSort = "replies" | "closed" | "rating"

export function MembersReport() {
  const { t, i18n } = useLingui()
  const { since, recent } = useRecent()
  const [sort, setSort] = useState<MemberSort>("replies")
  const members = new Map((useMembers().data ?? []).map((m) => [m.id, m]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  const rated = ratedSince(recent.data?.items ?? [], since)
  return (
    <Report tiles={1}>
      {(s) => {
        const people = s.members.map((p) => {
          const mine = rated.filter((c) => c.assignee_id === p.member_id)
          const good = mine.filter((c) => c.rating!.rating === "good").length
          return { ...p, rated: mine.length, share: mine.length ? good / mine.length : -1 }
        })
        people.sort((a, b) =>
          sort === "rating"
            ? b.share - a.share || b.replies - a.replies
            : sort === "closed"
              ? b.closed - a.closed || b.replies - a.replies
              : b.replies - a.replies || b.closed - a.closed
        )
        if (people.length === 0) {
          return (
            <div className="rounded-[14px] border bg-card">
              <p className="px-4 py-12 text-center text-body text-faint">
                <Trans>Nobody has replied or closed anything in this period.</Trans>
              </p>
            </div>
          )
        }
        const most = Math.max(1, ...people.map((p) => p.replies))
        const head = (key: MemberSort, label: React.ReactNode) => (
          <button
            type="button"
            className={cn(
              "w-fit text-start tracking-[inherit] uppercase outline-none focus-visible:underline",
              sort === key ? "text-foreground" : "hover:text-foreground"
            )}
            aria-sort={sort === key ? "descending" : undefined}
            onClick={() => setSort(key)}
            data-testid={`report-sort-${key}`}
          >
            {label}
            {sort === key && " ↓"}
          </button>
        )
        return (
          <>
            <section className="overflow-x-auto rounded-[14px] border bg-card" data-testid="report-members">
              <div className="min-w-[680px]">
                <div className={cn(MEMBER_GRID, headClass, "h-9 border-b")}>
                  <span>
                    <Trans>Member</Trans>
                  </span>
                  {head("replies", <Trans>Replies</Trans>)}
                  {head("closed", <Trans>Closed</Trans>)}
                  {head("rating", <Trans>Rating</Trans>)}
                  <span>
                    <Trans>SLA met / breached</Trans>
                  </span>
                </div>
                {people.map((p) => {
                  const m = members.get(p.member_id)
                  const name = m ? m.name || m.email : t`Deleted member`
                  const ratedText = fmt.format(p.rated)
                  const sla = s.sla.members.find((x) => x.member_id === p.member_id)
                  return (
                    <div
                      key={p.member_id}
                      className={cn(MEMBER_GRID, "min-h-[50px] border-b text-body last:border-b-0")}
                    >
                      <span className="flex min-w-0 items-center gap-2.5">
                        <Face id={p.member_id} name={name} className="size-7" />
                        <span className="truncate font-medium">{name}</span>
                      </span>
                      <span className="flex items-center gap-2">
                        <span className="flex h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-muted">
                          <span className="rounded-full bg-brand" style={{ width: `${(p.replies / most) * 100}%` }} />
                        </span>
                        <span className="w-7 shrink-0 text-muted-foreground tabular-nums">{fmt.format(p.replies)}</span>
                      </span>
                      <span className="text-muted-foreground tabular-nums">{fmt.format(p.closed)}</span>
                      <span className="text-muted-foreground tabular-nums">
                        {p.rated ? (
                          <>
                            <Trans>{pct.format(p.share)} good</Trans>
                            <span className="text-caption text-faint">
                              {" "}
                              · <Trans>{ratedText} rated</Trans>
                            </span>
                          </>
                        ) : (
                          "–"
                        )}
                      </span>
                      <SlaCell met={sla?.met ?? 0} breached={sla?.breached ?? 0} testId="report-member-sla" />
                    </div>
                  )
                })}
              </div>
            </section>
            <p className="text-caption text-faint">
              <Trans>
                Rating is the share of rated conversations a member answered that people marked good, across all
                products.
              </Trans>
            </p>
          </>
        )
      }}
    </Report>
  )
}

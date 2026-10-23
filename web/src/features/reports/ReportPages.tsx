import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ThumbsDownIcon, ThumbsUpIcon } from "lucide-react"
import { useState } from "react"
import { Outlet, useLocation, useSearchParams } from "react-router"

import { ErrorLine, InboxMark } from "@/components/common"
import { ChipGroup } from "@/components/common/ChipGroup"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Segmented } from "@/components/common/Segmented"
import { formatDuration } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
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
  Card,
  CardHead,
  EmptyRow,
  Face,
  headButton,
  Page,
  PageHeader,
  PageTabs,
  ProductTag,
  StatTile,
  tableHeadClass,
} from "@/features/settings/ui"
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
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <span className="flex min-w-48 flex-1 flex-col leading-[1.3]">
        <span className="font-semibold">{names[range]}</span>
        <span className="text-caption text-faint" data-testid="report-range-note">
          {note} · {scope}
        </span>
      </span>
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
    </div>
  )
}

export function ReportsPane() {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const current = pathname.split("/")[2] ?? "today"
  const csv = useCsv()
  const [csvOpen, setCsvOpen] = useState(false)
  const tabs = [
    { key: "today", to: { pathname: "/reports/today", search }, label: t`Overview`, testId: "report-tab-today" },
    { key: "ratings", to: { pathname: "/reports/ratings", search }, label: t`Ratings`, testId: "report-tab-ratings" },
    { key: "members", to: { pathname: "/reports/members", search }, label: t`Team`, testId: "report-tab-members" },
  ]
  return (
    <Page testId="reports">
      <PageHeader
        back={false}
        title={<Trans>Reports</Trans>}
        description={<Trans>How fast and how well your team answers</Trans>}
        action={
          <Button
            className={headButton.primary}
            disabled={!csv.ready}
            onClick={() => setCsvOpen(true)}
            data-testid="report-export"
          >
            <Trans>Download CSV</Trans>
          </Button>
        }
        tabs={<PageTabs label={t`Reports`} items={tabs} current={current} />}
      />
      <PeriodBar />
      <Outlet />
      {csvOpen && <CsvDialog csv={csv} onClose={() => setCsvOpen(false)} />}
    </Page>
  )
}

function Report({ children }: { children: (s: Stats) => React.ReactNode }) {
  const stats = useReportStats()
  return (
    <>
      {stats.isPending && <Skeleton className="h-28 rounded-xl" />}
      <ErrorLine error={stats.error} />
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
  const top = Math.max(1, ...slots.map((s) => s.n))
  const last = hourly ? slots.findIndex((s) => Number(s.key) === now.getHours()) : slots.length - 1
  const ticks = [0, Math.round((slots.length - 1) / 3), Math.round(((slots.length - 1) * 2) / 3), slots.length - 1]
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-xl border bg-card p-4" data-testid="report-chart">
      <div className="flex items-baseline gap-2">
        <h2 className="flex-1 font-semibold">
          {hourly ? <Trans>New conversations by hour</Trans> : <Trans>New conversations by day</Trans>}
        </h2>
        <span className="text-caption text-faint tabular-nums">
          <Trans>{total} in total</Trans>
        </span>
      </div>
      <div className="flex h-40 items-end gap-1.5 border-b" role="img" aria-label={t`New conversations`}>
        {slots.map((s, i) => (
          <span
            key={s.key}
            className="flex-1 rounded-t bg-brand"
            title={`${s.label} · ${fmt.format(s.n)}`}
            style={{ height: s.n ? `${(s.n / top) * 100}%` : 2, opacity: s.n ? (i === last ? 1 : 0.7) : 0.25 }}
          />
        ))}
      </div>
      <div className="flex justify-between text-caption text-faint">
        {[...new Set(ticks)].map((i) => (
          <span key={i}>{slots[i]?.label}</span>
        ))}
      </div>
    </section>
  )
}

const PRODUCT_GRID =
  "grid grid-cols-[minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-3 px-4"

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
    <section className="overflow-x-auto rounded-xl border bg-card" data-testid="report-inboxes">
      <div className="min-w-[520px]">
        <div className={cn(PRODUCT_GRID, tableHeadClass, "py-2.5")}>
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
            <div key={inbox.id} className={cn(PRODUCT_GRID, "border-t py-[11px]")} data-testid="report-inbox">
              <span className="flex min-w-0 items-center gap-2.5">
                <InboxMark name={inbox.name} color={inbox.branding.color} className="size-[22px]" />
                <span className="truncate font-medium">{inbox.name}</span>
              </span>
              <span className="text-muted-foreground tabular-nums">{fmt.format(n)}</span>
              <span className="text-muted-foreground tabular-nums">{cell(s?.median_first_reply_seconds)}</span>
              <span className="text-muted-foreground tabular-nums">{cell(s?.median_resolution_seconds)}</span>
              <span className="tabular-nums" data-testid="report-inbox-sla">
                {s ? <SlaCounts met={s.sla.met} breached={s.sla.breached} /> : <Skeleton className="h-4 w-10" />}
              </span>
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
            <div className="grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-4" data-testid="report-tiles">
              <StatTile
                label={t`New`}
                value={recent.data ? `${fmt.format(fresh.length)}${recent.data.complete ? "" : "+"}` : "–"}
                hint={t`conversations started`}
                testId="report-new"
              />
              {s.sla.met + s.sla.breached > 0 && (
                <StatTile
                  label={t`SLA met`}
                  value={new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(
                    s.sla.met / (s.sla.met + s.sla.breached)
                  )}
                  hint={t`${met} met, ${breached} breached`}
                  testId="report-sla"
                />
              )}
              <StatTile label={t`Replied`} value={fmt.format(s.replies)} hint={t`messages sent`} />
              <StatTile label={t`Closed`} value={fmt.format(s.closed)} hint={t`conversations`} />
              <StatTile
                label={t`First reply`}
                value={
                  s.median_first_reply_seconds !== undefined
                    ? formatDuration(s.median_first_reply_seconds, i18n.locale)
                    : "–"
                }
                hint={range === "today" ? t`median today` : t`median`}
              />
              <StatTile
                label={t`Resolved in`}
                value={
                  s.median_resolution_seconds !== undefined
                    ? formatDuration(s.median_resolution_seconds, i18n.locale)
                    : "–"
                }
                hint={t`median, first message to closed`}
                hintClassName="whitespace-normal"
                testId="report-resolved"
              />
            </div>
            <div className="grid grid-cols-1 gap-4">
              {recent.isPending ? (
                <Skeleton className="h-60 rounded-xl" />
              ) : (
                <NewChart items={fresh} more={!recent.data?.complete} />
              )}
              <ByProduct items={fresh} />
            </div>
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
    <Report>
      {(s) => {
        const rated = s.ratings.good + s.ratings.bad
        if (rated === 0) {
          return (
            <EmptyBox
              title={<Trans>No ratings in this period</Trans>}
              className="border-solid bg-card"
              testId="report-no-ratings"
              actions={
                range !== "30d" && (
                  <Button
                    variant="outline"
                    className={headButton.secondary}
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
        const most = Math.max(s.ratings.good, s.ratings.bad, 1)
        const rows: [boolean, number][] = [
          [true, s.ratings.good],
          [false, s.ratings.bad],
        ]
        return (
          <div className="grid items-start gap-4 lg:grid-cols-[340px_minmax(0,1fr)]" data-testid="report-ratings">
            <section className="flex flex-col gap-3.5 rounded-xl border bg-card p-5">
              <span className="flex flex-wrap items-baseline gap-x-2.5">
                <b className="text-[44px] leading-none font-semibold tracking-[-0.03em] tabular-nums">
                  {pct.format(share(s.ratings.good, s.ratings.bad) ?? 0)}
                </b>
                <span className="text-muted-foreground">
                  <Trans>good</Trans> · <Plural value={rated} one="# rating" other="# ratings" />
                </span>
              </span>
              {rows.map(([good, n]) => (
                <span key={String(good)} className="flex items-center gap-2.5">
                  <span className="w-7">
                    <Score good={good} />
                  </span>
                  <span className="flex h-2 flex-1 overflow-hidden rounded-full bg-border">
                    <span
                      className={cn("rounded-full", good ? "bg-success" : "bg-destructive")}
                      style={{ width: `${(n / most) * 100}%` }}
                    />
                  </span>
                  <span className="w-7 text-end text-caption text-faint tabular-nums">{fmt.format(n)}</span>
                </span>
              ))}
            </section>
            <Card flush>
              <CardHead
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
                <Trans>What people wrote</Trans>
              </CardHead>
              {recent.isPending ? (
                <Skeleton className="m-4 h-24" />
              ) : shown.length === 0 ? (
                <EmptyRow>
                  <Trans>Nobody left a comment with a rating in this period.</Trans>
                </EmptyRow>
              ) : (
                <ul data-testid="report-comments">
                  {shown.slice(0, 20).map((c) => {
                    const inbox = inboxes.get(c.inbox_id)
                    const by = c.assignee_id ? members.get(c.assignee_id) : undefined
                    const name = by ? by.name || by.email : ""
                    const who = c.contact.name || c.contact.email || ""
                    return (
                      <li key={c.id} className="flex items-start gap-3 border-b px-4 py-[11px] last:border-b-0">
                        <span className="w-7 shrink-0 pt-0.5">
                          <Score good={c.rating!.rating === "good"} />
                        </span>
                        <span className="flex min-w-0 flex-1 flex-col gap-0.5 leading-[1.4]">
                          <span className="break-words">“{c.rating!.comment}”</span>
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
            </Card>
          </div>
        )
      }}
    </Report>
  )
}

const MEMBER_GRID =
  "grid grid-cols-[minmax(0,1.4fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_minmax(0,1.1fr)_minmax(0,1fr)] items-center gap-4 px-4"

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
    <Report>
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
            <Card flush>
              <EmptyRow>
                <Trans>Nobody has replied or closed anything in this period.</Trans>
              </EmptyRow>
            </Card>
          )
        }
        const most = Math.max(1, ...people.map((p) => p.replies))
        const head = (key: MemberSort, label: React.ReactNode) => (
          <button
            type="button"
            className={cn(
              "w-fit text-start outline-none focus-visible:underline",
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
            <section className="overflow-x-auto rounded-xl border bg-card" data-testid="report-members">
              <div className="min-w-[680px]">
                <div className={cn(MEMBER_GRID, tableHeadClass, "py-2.5")}>
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
                    <div key={p.member_id} className={cn(MEMBER_GRID, "border-t py-3")}>
                      <span className="flex min-w-0 items-center gap-2.5">
                        <Face id={p.member_id} name={name} className="size-7" />
                        <span className="truncate font-medium">{name}</span>
                      </span>
                      <span className="flex items-center gap-2">
                        <span className="flex h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-border">
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
                      <span className="tabular-nums" data-testid="report-member-sla">
                        <SlaCounts met={sla?.met ?? 0} breached={sla?.breached ?? 0} />
                      </span>
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

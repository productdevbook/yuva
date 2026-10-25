import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronRightIcon, CircleAlertIcon, MailIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { BROADCAST_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatRelative, formatShort, useEnumText } from "@/components/common/text"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  broadcastLink,
  FINISHED,
  isActive,
  useBroadcastAction,
  useBroadcasts,
} from "@/features/broadcasts/broadcastQueries"
import { BroadcastBulkBar } from "@/features/broadcasts/BroadcastBulk"
import { BroadcastSteps, broadcastWhen } from "@/features/broadcasts/BroadcastList"
import {
  keyHint,
  progressOf,
  ProgressBar,
  StateBadge,
  StatusNotice,
  useBroadcastsStatus,
  useNumber,
  usePercent,
} from "@/features/broadcasts/BroadcastParts"
import { NewBroadcastDialog } from "@/features/broadcasts/NewBroadcastDialog"
import { listLink, useLists, useSender } from "@/features/broadcasts/queries"
import { TestSendDialog, useTestSend } from "@/features/broadcasts/TestSend"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { FrameButton, ProductTag } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useNow } from "@/hooks/use-now"
import { useRowFocus } from "@/hooks/use-row-focus"
import { useSelection } from "@/hooks/use-selection"
import type { Broadcast, MailingList } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const FILTERS = ["all", "sent", "scheduled", "drafts"] as const
type Filter = (typeof FILTERS)[number]

function matches(f: Filter, b: Broadcast) {
  if (f === "sent") return FINISHED.includes(b.state)
  if (f === "scheduled") return b.state === "scheduled"
  if (f === "drafts") return b.state === "draft"
  return true
}

const cols =
  "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 @2xl:grid-cols-[minmax(0,1fr)_92px_76px_96px_56px_64px] @4xl:grid-cols-[minmax(128px,1fr)_minmax(88px,140px)_92px_76px_96px_56px_64px]"

const DAY = 86400_000

function Tile({ label, value, sub }: { label: string; value: string; sub?: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-[14px] border bg-card px-3.5 py-3">
      <span className="truncate text-small text-faint">{label}</span>
      <b className="block text-[20px] leading-7 font-semibold tracking-[-0.01em] tabular-nums">{value}</b>
      <span className="min-h-4 truncate text-caption text-faint tabular-nums">{sub}</span>
    </div>
  )
}

function Tiles({ items }: { items: Broadcast[] }) {
  const { t } = useLingui()
  const num = useNumber()
  const percent = usePercent()
  const sum = (pick: (b: Broadcast) => number) => items.reduce((n, b) => n + pick(b), 0)
  const sent = sum((b) => b.counts.sent)
  const clicked = sum((b) => b.counts.clicked)
  const people = num(clicked)
  const share = (n: number) => (sent > 0 ? percent(n / sent) : "–")
  const unsubscribed = sum((b) => b.counts.unsubscribed)
  const bounced = sum((b) => b.counts.bounced)
  const unsubscribedShare = share(unsubscribed)
  const spam = num(sum((b) => b.counts.complained))
  const total = items.length
  return (
    <div
      className="grid grid-cols-2 gap-2.5 @3xl:grid-cols-5 [&>*:last-child]:col-span-2 @3xl:[&>*:last-child]:col-span-1"
      data-testid="broadcasts-tiles"
    >
      <Tile
        label={t`Sent`}
        value={num(sent)}
        sub={plural(total, {
          one: "Last 30 days · # broadcast",
          other: "Last 30 days · # broadcasts",
        })}
      />
      <Tile label={t`Clicked`} value={share(clicked)} sub={t`${people} people`} />
      <Tile label={t`Replied`} value={num(sum((b) => b.counts.replied))} sub={t`Replies land in the inbox`} />
      <Tile
        label={t`Unsubscribed`}
        value={num(unsubscribed)}
        sub={sent > 0 ? t`${unsubscribedShare} of sent` : undefined}
      />
      <Tile label={t`Bounced`} value={num(bounced)} sub={t`${spam} marked as spam`} />
    </div>
  )
}

function ListsCard({ lists, pending }: { lists: MailingList[]; pending: boolean }) {
  const num = useNumber()
  const inboxes = useInboxes().data ?? []
  const top = [...lists].sort((a, b) => b.counts.subscribed - a.counts.subscribed).slice(0, 6)
  if (!pending && top.length === 0) return null
  return (
    <aside className="flex flex-col overflow-hidden rounded-[14px] border bg-card" data-testid="broadcasts-lists">
      <Link
        to="/broadcasts/lists"
        className="flex items-center gap-1 px-4 py-3 text-[15px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      >
        <Trans>Lists</Trans>
        <ChevronRightIcon className="size-4 text-faint rtl:rotate-180" aria-hidden />
      </Link>
      {pending ? (
        <div className="flex flex-col gap-2 border-t p-4">
          <Skeleton className="h-5 w-full" />
          <Skeleton className="h-5 w-4/5" />
          <Skeleton className="h-5 w-3/5" />
        </div>
      ) : (
        <ul>
          {top.map((l) => {
            const inbox = inboxes.find((i) => i.id === l.inbox_id)
            return (
              <li key={l.id} className="border-t">
                <Link
                  to={listLink(l.id)}
                  className="flex items-center gap-2 px-4 py-2.5 text-body outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
                >
                  <span
                    className="size-2 shrink-0 rounded-[3px] bg-brand"
                    style={inbox?.branding.color ? { background: inbox.branding.color } : undefined}
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1 truncate">{l.name}</span>
                  <span className="shrink-0 font-semibold tabular-nums">{num(l.counts.subscribed)}</span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </aside>
  )
}

function NextSend({ b, lists }: { b: Broadcast; lists: Map<string, MailingList> }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const from = useSender(b.inbox_id, b.channel_id)
  const [testing, setTesting] = useState(false)
  const test = useTestSend(b.id)
  const now = useNow(60_000)
  const at = b.scheduled_at ?? b.updated_at
  const d = new Date(at)
  const soon = d.getTime() - now < 6 * 86400_000
  const when = soon
    ? new Intl.DateTimeFormat(i18n.locale, { weekday: "long", hour: "2-digit", minute: "2-digit" }).format(d)
    : formatDateTime(at, i18n.locale)
  const to = b.audience.list_ids
    .map((id) => lists.get(id)?.name)
    .filter(Boolean)
    .join(", ")
  const people = num(b.counts.recipients)
  const parts = [
    to ? t`Sends ${when} to ${to}` : t`Sends ${when}`,
    ...(b.counts.recipients > 0 ? [t`${people} people`] : []),
    ...(from ? [t`from ${from}`] : []),
  ]
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-[14px] border bg-card px-4 py-3.5"
      data-testid="broadcasts-next"
    >
      <time
        dateTime={at}
        className="flex h-12 w-11 shrink-0 flex-col items-center justify-center rounded-[10px] border bg-surface leading-none"
        aria-hidden
      >
        <span className="text-[10px] font-semibold tracking-[0.06em] text-brand uppercase">
          {new Intl.DateTimeFormat(i18n.locale, { month: "short" }).format(d)}
        </span>
        <span className="mt-[3px] text-[18px] font-semibold tabular-nums">
          {new Intl.DateTimeFormat(i18n.locale, { day: "numeric" }).format(d)}
        </span>
      </time>
      <span className="flex min-w-48 min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-center gap-2">
          <Link
            to={broadcastLink(b.id)}
            className="truncate text-[15px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            {b.name}
          </Link>
          {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
        </span>
        <span className="truncate text-body text-muted-foreground">{parts.join(" · ")}</span>
      </span>
      <FrameButton onClick={() => setTesting(true)} data-testid="broadcasts-next-test">
        <Trans>Send a test</Trans>
      </FrameButton>
      <FrameButton render={<Link to={broadcastLink(b.id)} />} data-testid="broadcasts-next-edit">
        <Trans>Edit</Trans>
      </FrameButton>
      {testing && <TestSendDialog b={b} state={test} disabled={false} onClose={() => setTesting(false)} />}
    </div>
  )
}

function BroadcastRow({
  b,
  lists,
  top,
  check,
}: {
  b: Broadcast
  lists: Map<string, MailingList>
  top: number
  check?: { checked: boolean; onToggle: (range: boolean) => void } | "none"
}) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const percent = usePercent()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const at = broadcastWhen(b)
  const c = b.counts
  const names = b.audience.list_ids.map((id) => lists.get(id)?.name).filter(Boolean)
  const listed = b.audience.list_ids.reduce((n, id) => n + (lists.get(id)?.counts.subscribed ?? 0), 0)
  const share = c.sent > 0 ? c.clicked / c.sent : 0
  return (
    <li
      className={cn(
        "relative flex items-center border-t first:border-t-0",
        isActive(b) &&
          "before:absolute before:inset-y-3 before:start-1 before:z-10 before:w-[3px] before:rounded-[3px] before:bg-brand"
      )}
    >
      {check === "none" && <span className="ms-4 size-5 shrink-0" aria-hidden />}
      {check && check !== "none" && (
        <RowCheck className="ms-4" checked={check.checked} name={b.name} onToggle={check.onToggle} />
      )}
      <Link
        to={broadcastLink(b.id)}
        className={cn(
          cols,
          "min-h-14 min-w-0 flex-1 px-4 py-2.5 text-[13px] outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
        )}
        data-testid="broadcasts-table-row"
      >
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-body font-semibold">{b.name}</span>
          <span className="truncate text-small text-muted-foreground">{b.subject || t`No subject yet`}</span>
          {isActive(b) && <ProgressBar b={b} className="mt-1" />}
        </span>
        <span className="hidden min-w-0 @4xl:flex">
          {names.length > 0 ? (
            <span className={cn(look.listChip, "min-w-0 shrink")} title={inbox?.name}>
              <span
                className="size-2 shrink-0 rounded-[3px] bg-brand"
                style={inbox?.branding.color ? { background: inbox.branding.color } : undefined}
                aria-hidden
              />
              <span className="min-w-0 truncate">{names.join(", ")}</span>
            </span>
          ) : (
            <span className="text-body text-faint">–</span>
          )}
        </span>
        <span className="justify-self-end @2xl:justify-self-start">
          <StateBadge state={b.state} />
        </span>
        <span className="hidden text-body tabular-nums @2xl:block">
          {c.recipients > 0 ? (
            num(c.recipients)
          ) : listed > 0 ? (
            <span className="text-muted-foreground" title={t`Subscribed to the list now`}>
              {num(listed)}
            </span>
          ) : (
            "–"
          )}
        </span>
        <span className="hidden items-center gap-2 @2xl:flex">
          <span className="relative h-1.5 w-8 flex-none overflow-hidden rounded-full bg-muted">
            {b.click_tracking && c.sent > 0 && (
              <span
                className="absolute inset-y-0 start-0 rounded-full bg-brand"
                style={{ width: `${top > 0 ? Math.round((share / top) * 85) : 0}%` }}
              />
            )}
          </span>
          <span
            className="min-w-9 text-caption text-muted-foreground tabular-nums"
            title={b.click_tracking ? undefined : t`Clicks not tracked`}
          >
            {b.click_tracking && c.sent > 0 ? percent(share) : "–"}
          </span>
        </span>
        <span className="hidden text-body tabular-nums @2xl:block">{c.sent > 0 ? num(c.replied) : "–"}</span>
        <span className="col-span-2 flex flex-wrap items-center gap-x-3 text-caption text-faint tabular-nums @2xl:hidden">
          {at && <time dateTime={at}>{formatShort(at, i18n.locale)}</time>}
          {c.recipients > 0 && <span>{num(c.recipients)}</span>}
          {b.click_tracking && c.sent > 0 && (
            <span>
              {t`Clicked`} {percent(share)}
            </span>
          )}
        </span>
        <span className="hidden text-small text-muted-foreground @2xl:block">
          {at ? (
            <time dateTime={at} title={formatRelative(at, i18n.locale)}>
              {formatShort(at, i18n.locale)}
            </time>
          ) : (
            "–"
          )}
        </span>
      </Link>
    </li>
  )
}

function SendingNow({ b }: { b: Broadcast }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const num = useNumber()
  const text = useEnumText()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const action = useBroadcastAction(b.id)
  const { done, total } = progressOf(b)
  const sent = num(done)
  const all = num(total)
  const paused = b.state === "paused"
  return (
    <div
      className="relative flex flex-col gap-3 rounded-[14px] border bg-card p-4 before:absolute before:inset-y-4 before:start-1 before:w-[3px] before:rounded-[3px] before:bg-brand"
      data-testid="broadcasts-sending"
    >
      <span className="flex flex-col">
        <h2 className="text-[15px] font-semibold">
          <Trans>Going out now</Trans>
        </h2>
        <p className="text-small text-faint">
          <Trans>Mail leaves at each broadcast’s pace, so providers accept it.</Trans>
        </p>
      </span>
      <span className="flex flex-wrap items-center gap-x-2.5 gap-y-2">
        <StateBadge state={b.state} />
        <Link
          to={broadcastLink(b.id)}
          className="min-w-0 truncate text-[15px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {b.name}
        </Link>
        {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
        <span className="flex-1" />
        {canManage && b.state !== "preparing" && (
          <FrameButton
            className="h-[30px] px-2.5"
            disabled={action.isPending}
            onClick={() => action.mutate(paused ? "resume" : "pause")}
            title={keyHint(paused ? t`Resume` : t`Pause`, BROADCAST_KEYS.pause)}
            data-testid={paused ? "broadcasts-sending-resume" : "broadcasts-sending-pause"}
          >
            {paused ? <Trans>Resume</Trans> : <Trans>Pause</Trans>}
          </FrameButton>
        )}
        <FrameButton className="h-[30px] px-2.5" render={<Link to={broadcastLink(b.id)} />}>
          <Trans>Open</Trans>
        </FrameButton>
      </span>
      <span className="flex items-center gap-3">
        <ProgressBar b={b} className="h-2 flex-1" />
        <span className="shrink-0 text-body text-muted-foreground tabular-nums">
          {b.state === "preparing" ? (
            text.broadcastState.preparing
          ) : (
            <Trans>
              {sent} of {all} done
            </Trans>
          )}
        </span>
      </span>
      <ErrorLine error={action.error} />
    </div>
  )
}

function Empty({
  filter,
  scope,
  canCreate,
  onNew,
}: {
  filter: Filter
  scope?: string
  canCreate: boolean
  onNew: () => void
}) {
  const first = filter === "all"
  return (
    <div
      className="mx-auto flex w-[min(560px,100%)] flex-col items-center gap-3 rounded-[14px] border bg-card px-6 py-10 text-center text-body text-muted-foreground"
      data-testid={first ? "broadcasts-home-empty" : "broadcasts-tab-empty"}
    >
      <span className="grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
        <MailIcon className="size-5" aria-hidden />
      </span>
      <p className="font-semibold text-foreground">
        {first ? <Trans>No broadcasts yet</Trans> : <Trans>Nothing here.</Trans>}
      </p>
      {first ? (
        <div className="max-w-xl">
          {scope && (
            <p className="mb-2">
              <Trans>No broadcasts for {scope} yet.</Trans>
            </p>
          )}
          <BroadcastSteps />
        </div>
      ) : (
        <p>
          {filter === "sent" ? (
            <Trans>No broadcasts have gone out yet.</Trans>
          ) : filter === "scheduled" ? (
            <Trans>No broadcast is scheduled.</Trans>
          ) : (
            <Trans>No drafts.</Trans>
          )}
        </p>
      )}
      {canCreate && (
        <Button className={look.btnInk} onClick={onNew} data-testid="broadcasts-empty-new">
          <Trans>New broadcast</Trans>
        </Button>
      )}
    </div>
  )
}

export function BroadcastsOverview() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const status = useBroadcastsStatus()
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const broadcasts = useBroadcasts(inboxId)
  const lists = useLists(inboxId, false)
  const archived = useLists(inboxId, true)
  const [creating, setCreating] = useState(false)
  const raw = params.get("tab") as Filter | null
  const filter: Filter = raw && FILTERS.includes(raw) ? raw : "all"
  const items = broadcasts.data?.pages.flatMap((p) => p.items) ?? []
  const [query, setQuery] = useState("")
  const byId = new Map(
    [...(lists.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l])
  )
  const now = useNow(60_000)
  const next = items
    .filter((b) => b.state === "scheduled" && b.scheduled_at)
    .sort((a, b) => a.scheduled_at!.localeCompare(b.scheduled_at!))[0]
  const sending = items.find(isActive)
  const q = query.trim().toLowerCase()
  const rows = items.filter(
    (b) => matches(filter, b) && (!q || b.name.toLowerCase().includes(q) || b.subject.toLowerCase().includes(q))
  )
  const recent = items.filter((b) => {
    const at = b.state === "sent" || b.state === "failed" ? (b.finished_at ?? b.started_at) : undefined
    return !!at && b.counts.sent > 0 && now - Date.parse(at) <= 30 * DAY
  })
  const count = (f: Filter) => items.filter((b) => matches(f, b)).length
  const selection = useSelection(
    rows.filter((b) => b.state === "draft").map((b) => b.id),
    `${filter}|${inboxId}`
  )
  const top = Math.max(0, ...rows.map((b) => (b.counts.sent > 0 ? b.counts.clicked / b.counts.sent : 0)))
  const names: Record<Filter, string> = {
    all: t`All`,
    sent: t`Sent`,
    scheduled: t`Scheduled`,
    drafts: t`Drafts`,
  }
  const tabs = FILTERS.map((f) => ({
    key: f,
    to: f === "all" ? { search: "" } : { search: `?tab=${f}` },
    label: names[f],
    count: f !== "all" && !broadcasts.hasNextPage && count(f) > 0 ? count(f) : undefined,
    testId: `broadcasts-filter-${f}`,
  }))
  const mayCreate = canManage && status?.mode !== "off"
  const table = useRowFocus<HTMLUListElement>(!creating)
  const scope = inboxId ? inboxes.find((i) => i.id === inboxId)?.name : undefined
  useHotkeys(
    {
      ...(mayCreate ? { [BROADCAST_KEYS.create]: () => setCreating(true) } : {}),
      ...Object.fromEntries(
        FILTERS.map((f, i) => [String(i + 1), () => navigate(f === "all" ? "/broadcasts" : `/broadcasts?tab=${f}`)])
      ),
    },
    !creating
  )
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="broadcasts-home">
      <PageTop
        title={<Trans>Broadcasts</Trans>}
        sub={<Trans>E-mails to the people who signed up</Trans>}
        actions={
          mayCreate && (
            <Button
              className={look.btnInk}
              onClick={() => setCreating(true)}
              title={keyHint(t`New broadcast`, BROADCAST_KEYS.create)}
              data-testid="broadcasts-home-new"
            >
              <Trans>New broadcast</Trans>
            </Button>
          )
        }
        tabs={<FrameTabs label={t`Broadcasts`} value={filter} items={tabs} />}
      />
      <div className="@container min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto grid w-full max-w-[1360px] items-start gap-4 p-6 pb-16 @max-xl:p-4 @min-[1100px]:grid-cols-[minmax(0,1fr)_300px]">
          <div className="flex min-w-0 flex-col gap-4">
            <StatusNotice />
            <BroadcastBulkBar items={items} selection={selection} />
            {broadcasts.isPending ? (
              <div className="flex flex-col gap-4" aria-busy data-testid="broadcasts-loading">
                <Skeleton className="h-24 rounded-[14px]" />
                <div className="grid grid-cols-2 gap-2.5 @3xl:grid-cols-5">
                  {[0, 1, 2, 3, 4].map((n) => (
                    <Skeleton key={n} className="h-[84px] rounded-[14px]" />
                  ))}
                </div>
                <Skeleton className="h-80 rounded-[14px]" />
              </div>
            ) : broadcasts.error && items.length === 0 ? (
              <div className="mx-auto my-12 flex max-w-sm flex-col items-center gap-3 px-4 text-center">
                <span className="grid size-8 place-items-center rounded-full bg-destructive/10 text-destructive">
                  <CircleAlertIcon className="size-4" aria-hidden />
                </span>
                <ErrorLine error={broadcasts.error} />
                <Button variant="outline" className={look.btn} onClick={() => void broadcasts.refetch()}>
                  <Trans>Try again</Trans>
                </Button>
              </div>
            ) : items.length === 0 ? (
              <Empty filter="all" scope={scope} canCreate={mayCreate} onNew={() => setCreating(true)} />
            ) : (
              <>
                <ErrorLine error={broadcasts.error} />
                {sending && filter === "all" && <SendingNow b={sending} />}
                {next && (filter === "all" || filter === "scheduled") && <NextSend b={next} lists={byId} />}
                {filter === "all" && !broadcasts.hasNextPage && recent.length > 0 && <Tiles items={recent} />}
                {rows.length === 0 && !q ? (
                  <Empty filter={filter} scope={scope} canCreate={mayCreate} onNew={() => setCreating(true)} />
                ) : (
                  <div className="overflow-hidden rounded-[14px] border bg-card" data-testid="broadcasts-table">
                    <div className="flex items-center gap-2 border-b px-4 py-3">
                      <Input
                        type="search"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder={t`Search`}
                        aria-label={t`Search`}
                        className="h-[34px] w-64 rounded-[9px] bg-card phone:w-full"
                      />
                      <span className="flex-1" />
                      {canManage && !selection.selecting && rows.some((b) => b.state === "draft") && (
                        <SelectToggle selection={selection} />
                      )}
                    </div>
                    <div className={cn(cols, look.over, "hidden h-9 border-b px-4 @2xl:grid")} aria-hidden>
                      <span>
                        <Trans>Broadcast</Trans>
                      </span>
                      <span className="hidden @4xl:block">
                        <Trans>List</Trans>
                      </span>
                      <span>
                        <Trans>Status</Trans>
                      </span>
                      <span>
                        <Trans>Recipients</Trans>
                      </span>
                      <span>
                        <Trans>Clicked</Trans>
                      </span>
                      <span>
                        <Trans>Replied</Trans>
                      </span>
                      <span>
                        <Trans>Date</Trans>
                      </span>
                    </div>
                    {rows.length === 0 ? (
                      <p className="px-4 py-8 text-center text-body text-muted-foreground">
                        <Trans>Nothing here.</Trans>
                      </p>
                    ) : (
                      <ul ref={table}>
                        {rows.map((b) => (
                          <BroadcastRow
                            key={b.id}
                            b={b}
                            lists={byId}
                            top={top}
                            check={
                              selection.selecting
                                ? b.state === "draft"
                                  ? {
                                      checked: selection.has(b.id),
                                      onToggle: (range) => selection.toggle(b.id, range),
                                    }
                                  : "none"
                                : undefined
                            }
                          />
                        ))}
                      </ul>
                    )}
                    {broadcasts.hasNextPage && (
                      <div className="flex justify-center border-t p-3">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void broadcasts.fetchNextPage()}
                          disabled={broadcasts.isFetchingNextPage}
                        >
                          <Trans>Load more</Trans>
                        </Button>
                      </div>
                    )}
                  </div>
                )}
                <p className="text-small text-faint">
                  <Trans>Yuva counts clicks and replies. It does not track opens.</Trans>
                </p>
              </>
            )}
          </div>
          <ListsCard lists={lists.data?.pages.flatMap((p) => p.items) ?? []} pending={lists.isPending} />
        </div>
      </div>
      {creating && (
        <NewBroadcastDialog
          inboxId={inboxId}
          onClose={() => setCreating(false)}
          onCreated={(b) => {
            setCreating(false)
            navigate(broadcastLink(b.id))
          }}
        />
      )}
    </main>
  )
}

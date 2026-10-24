import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { BROADCAST_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatRelative, formatShort, useEnumText } from "@/components/common/text"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { Button } from "@/components/ui/button"
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
import { useLists, useSender } from "@/features/broadcasts/queries"
import { TestSendDialog, useTestSend } from "@/features/broadcasts/TestSend"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameButton, FrameTabs, framePad, PageFrame, ProductTag } from "@/features/status/PageFrame"
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
  "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 @2xl:grid-cols-[minmax(0,1.4fr)_100px_90px_140px_64px_72px] @4xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_100px_90px_140px_64px_72px]"

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
      className="flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border bg-card px-4 py-4"
      data-testid="broadcasts-next"
    >
      <time
        dateTime={at}
        className="flex w-[54px] shrink-0 flex-col items-center rounded-lg border bg-background py-1.5 leading-[1.1]"
        aria-hidden
      >
        <span className="text-[11px] font-semibold text-brand uppercase">
          {new Intl.DateTimeFormat(i18n.locale, { month: "short" }).format(d)}
        </span>
        <span className="text-[20px] font-semibold tabular-nums">
          {new Intl.DateTimeFormat(i18n.locale, { day: "numeric" }).format(d)}
        </span>
      </time>
      <span className="flex min-w-48 min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-center gap-2">
          <Link
            to={broadcastLink(b.id)}
            className="truncate text-[16.5px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
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
    <li className="flex items-center border-t first:border-t-0">
      {check === "none" && <span className="ms-4 size-5 shrink-0" aria-hidden />}
      {check && check !== "none" && (
        <RowCheck className="ms-4" checked={check.checked} name={b.name} onToggle={check.onToggle} />
      )}
      <Link
        to={broadcastLink(b.id)}
        className={cn(
          cols,
          "min-h-[58px] min-w-0 flex-1 px-4 py-2.5 outline-none transition-colors first:border-t-0 hover:bg-surface focus-visible:bg-surface"
        )}
        data-testid="broadcasts-table-row"
      >
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-body font-semibold">{b.name}</span>
          <span className="truncate text-small text-muted-foreground">{b.subject || t`No subject yet`}</span>
          {isActive(b) && <ProgressBar b={b} className="mt-1" />}
        </span>
        <span className="hidden min-w-0 items-center gap-1.5 text-body @4xl:flex">
          {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
          <span className="truncate">{names.join(", ") || "–"}</span>
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
          <span className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
            {b.click_tracking && c.sent > 0 && (
              <span
                className="absolute inset-y-0 start-0 rounded-full bg-success"
                style={{ width: `${top > 0 ? Math.round((share / top) * 85) : 0}%` }}
              />
            )}
          </span>
          <span
            className="w-11 text-end text-caption text-muted-foreground tabular-nums"
            title={b.click_tracking ? undefined : t`Clicks not tracked`}
          >
            {b.click_tracking && c.sent > 0 ? percent(share) : "–"}
          </span>
        </span>
        <span className="hidden text-body tabular-nums @2xl:block">{c.sent > 0 ? num(c.replied) : "–"}</span>
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
    <div className="flex flex-col gap-3 rounded-xl border bg-card p-4" data-testid="broadcasts-sending">
      <span className="flex flex-wrap items-center gap-x-2.5 gap-y-2">
        <StateBadge state={b.state} />
        <Link
          to={broadcastLink(b.id)}
          className="min-w-0 truncate text-[16.5px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
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
      className="flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-10 text-center text-body text-muted-foreground"
      data-testid={first ? "broadcasts-home-empty" : "broadcasts-tab-empty"}
    >
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
        <FrameButton ink onClick={onNew} data-testid="broadcasts-empty-new">
          <Trans>New broadcast</Trans>
        </FrameButton>
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
  const byId = new Map(
    [...(lists.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l])
  )
  const next = items
    .filter((b) => b.state === "scheduled" && b.scheduled_at)
    .sort((a, b) => a.scheduled_at!.localeCompare(b.scheduled_at!))[0]
  const sending = items.find(isActive)
  const rows = items.filter((b) => matches(filter, b))
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
    <PageFrame
      testId="broadcasts-home"
      title={<Trans>Broadcasts</Trans>}
      description={<Trans>E-mails to the people who signed up</Trans>}
      actions={
        mayCreate && (
          <FrameButton
            ink
            onClick={() => setCreating(true)}
            title={keyHint(t`New broadcast`, BROADCAST_KEYS.create)}
            data-testid="broadcasts-home-new"
          >
            <Trans>New broadcast</Trans>
          </FrameButton>
        )
      }
      tabs={<FrameTabs label={t`Broadcasts`} items={tabs} current={filter} />}
      bodyClassName="@container"
    >
      <div className={framePad}>
        <StatusNotice />
        <ErrorLine error={broadcasts.error} />
        <BroadcastBulkBar items={items} selection={selection} />
        {broadcasts.isPending ? (
          <Skeleton className="h-48 rounded-xl" />
        ) : items.length === 0 ? (
          <Empty filter="all" scope={scope} canCreate={mayCreate} onNew={() => setCreating(true)} />
        ) : (
          <>
            {sending && filter === "all" && <SendingNow b={sending} />}
            {next && (filter === "all" || filter === "scheduled") && <NextSend b={next} lists={byId} />}
            {rows.length === 0 ? (
              <Empty filter={filter} scope={scope} canCreate={mayCreate} onNew={() => setCreating(true)} />
            ) : (
              <>
                {canManage && !selection.selecting && rows.some((b) => b.state === "draft") && (
                  <div className="flex justify-end">
                    <SelectToggle selection={selection} />
                  </div>
                )}
                <div className="overflow-hidden rounded-xl border bg-card" data-testid="broadcasts-table">
                  <div
                    className={cn(
                      cols,
                      "hidden border-b bg-surface px-4 py-3 text-small font-medium text-muted-foreground @2xl:grid"
                    )}
                    aria-hidden
                  >
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
                              ? { checked: selection.has(b.id), onToggle: (range) => selection.toggle(b.id, range) }
                              : "none"
                            : undefined
                        }
                      />
                    ))}
                  </ul>
                </div>
              </>
            )}
            {broadcasts.hasNextPage && (
              <Button
                variant="ghost"
                size="sm"
                className="self-center"
                onClick={() => void broadcasts.fetchNextPage()}
                disabled={broadcasts.isFetchingNextPage}
              >
                <Trans>Load more</Trans>
              </Button>
            )}
            <p className="text-small text-faint">
              <Trans>Yuva counts clicks and replies. It does not track opens.</Trans>
            </p>
          </>
        )}
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
    </PageFrame>
  )
}

import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useLayoutEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import {
  FilterMenu,
  FilterRow,
  FilterSearch,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
  RowCheckbox,
} from "@/components/common/ListBox"
import { BROADCAST_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatRelative, formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { broadcastLink, isActive, useBroadcastAction, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { BroadcastBulkBar, deletable } from "@/features/broadcasts/BroadcastBulk"
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
import {
  BroadcastsLayout,
  inView,
  rememberedList,
  rememberList,
  SenderLine,
  useViewNames,
  VIEWS,
  viewLink,
  type BroadcastView,
} from "@/features/broadcasts/BroadcastsFrame"
import { ProductFilter, useProductScope } from "@/components/common/ProductFilter"
import { NewBroadcastForm } from "@/features/broadcasts/NewBroadcastForm"
import { useLists } from "@/features/broadcasts/queries"
import { useListCursor } from "@/features/inbox/listCursor"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Broadcast, BroadcastState, MailingList } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const STATES: BroadcastState[] = ["draft", "scheduled", "preparing", "sending", "paused", "sent", "cancelled", "failed"]

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"
const cell = "px-3 py-2.5 text-body"
const hidePeople = "@max-xl/list:hidden"
const hideReplied = "@max-2xl/list:hidden"
const hideClicked = "@max-md/list:hidden"

function viewOfParams(params: URLSearchParams): BroadcastView {
  const tab = params.get("tab") as BroadcastView | null
  return tab && VIEWS.includes(tab) ? tab : "all"
}

function Row({
  b,
  lists,
  cursor,
  checked,
  selectable,
  selecting,
  check,
  onOpen,
}: {
  b: Broadcast
  lists: Map<string, MailingList>
  cursor: boolean
  checked: boolean
  selectable: boolean
  selecting: boolean
  check?: (range: boolean) => void
  onOpen: () => void
}) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const percent = usePercent()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const c = b.counts
  const names = b.audience.list_ids.map((id) => lists.get(id)?.name).filter(Boolean)
  const listed = b.audience.list_ids.reduce((n, id) => n + (lists.get(id)?.counts.subscribed ?? 0), 0)
  const at = broadcastWhen(b)
  const sub = [names.join(" + ") || b.subject || t`No subject yet`, at ? formatShort(at, i18n.locale) : ""]
    .filter(Boolean)
    .join(" · ")
  const people = c.recipients > 0 ? num(c.recipients) : listed > 0 ? num(listed) : "–"
  const clicked = b.click_tracking && c.sent > 0 ? percent(c.clicked / c.sent) : "–"
  return (
    <TableRow
      onClick={(e) => {
        if (check && (e.shiftKey || selecting)) check(e.shiftKey)
        else onOpen()
      }}
      className={cn(
        "cursor-pointer border-line-soft hover:bg-subtle",
        cursor && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]",
        checked && "bg-brand-wash hover:bg-brand-wash"
      )}
      data-row-id={b.id}
      data-testid="broadcasts-table-row"
      data-checked={checked || undefined}
    >
      {selectable && (
        <TableCell className="w-10 py-2.5 ps-3 pe-0" onClick={(e) => e.stopPropagation()}>
          {check ? (
            <RowCheckbox checked={checked} name={b.name} onToggle={check} className="mt-0" />
          ) : (
            <span className="block size-4" aria-hidden />
          )}
        </TableCell>
      )}
      <TableCell className={cn(cell, "min-w-0 overflow-hidden")}>
        <Link
          to={broadcastLink(b.id)}
          onClick={(e) => {
            e.stopPropagation()
            if (check && (e.shiftKey || selecting)) {
              e.preventDefault()
              check(e.shiftKey)
            } else onOpen()
          }}
          title={inbox?.name}
          className="flex min-w-0 flex-col rounded-sm leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="broadcast-open"
        >
          <span className="truncate font-medium text-foreground" dir="auto">
            {b.name}
          </span>
          <span
            className="truncate text-small text-muted-foreground"
            title={at ? formatRelative(at, i18n.locale) : undefined}
          >
            {sub}
          </span>
          {isActive(b) && <ProgressBar b={b} className="mt-1 max-w-60" />}
        </Link>
      </TableCell>
      <TableCell className={cn(cell, "w-28")}>
        <StateBadge state={b.state} />
      </TableCell>
      <TableCell
        className={cn(cell, hidePeople, "w-24 text-end text-fg2 tabular-nums")}
        title={c.recipients > 0 ? t`Recipients` : t`Subscribed to the list now`}
      >
        {people}
      </TableCell>
      <TableCell
        className={cn(cell, hideClicked, "w-24 text-end text-fg2 tabular-nums")}
        title={b.click_tracking ? undefined : t`Clicks not tracked`}
      >
        {clicked}
      </TableCell>
      <TableCell className={cn(cell, hideReplied, "w-24 pe-4 text-end text-fg2 tabular-nums")}>
        {c.sent > 0 ? num(c.replied) : "–"}
      </TableCell>
    </TableRow>
  )
}

function SendingNow({ b }: { b: Broadcast }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const num = useNumber()
  const text = useEnumText()
  const action = useBroadcastAction(b.id)
  const { done, total } = progressOf(b)
  const sent = num(done)
  const all = num(total)
  const paused = b.state === "paused"
  return (
    <Card className="mt-4 gap-3 px-4 py-3.5 shadow-none" data-testid="broadcasts-sending">
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-2">
        <StateBadge state={b.state} />
        <Link
          to={broadcastLink(b.id)}
          className="min-w-0 truncate text-[15px] font-semibold outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
        >
          {b.name}
        </Link>
        <span className="flex-1" />
        {canManage && b.state !== "preparing" && (
          <Button
            variant="outline"
            size="sm"
            disabled={action.isPending}
            onClick={() => action.mutate(paused ? "resume" : "pause")}
            title={keyHint(paused ? t`Resume` : t`Pause`, BROADCAST_KEYS.pause)}
            data-testid={paused ? "broadcasts-sending-resume" : "broadcasts-sending-pause"}
          >
            {paused ? <Trans>Resume</Trans> : <Trans>Pause</Trans>}
          </Button>
        )}
      </div>
      <div className="flex items-center gap-3">
        <ProgressBar b={b} className="h-2 flex-1" />
        <span className="shrink-0 text-small text-muted-foreground tabular-nums">
          {b.state === "preparing" ? (
            text.broadcastState.preparing
          ) : (
            <Trans>
              {sent} of {all} done
            </Trans>
          )}
        </span>
      </div>
      <ErrorLine error={action.error} />
    </Card>
  )
}

export function BroadcastsOverview() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const status = useBroadcastsStatus()
  const text = useEnumText()
  const names = useViewNames()
  const { chosen, inboxId, params, set } = useProductScope()
  const view = viewOfParams(params)
  const q = params.get("q") ?? ""
  const state = (params.get("state") ?? "") as BroadcastState | ""
  const broadcasts = useBroadcasts(inboxId, undefined, "once")
  const lists = useLists(inboxId, false)
  const archived = useLists(inboxId, true)
  const [creating, setCreating] = useState(false)
  const items = useMemo(() => broadcasts.data?.pages.flatMap((p) => p.items) ?? [], [broadcasts.data])
  const byId = new Map(
    [...(lists.data?.pages ?? []), ...(archived.data?.pages ?? [])].flatMap((p) => p.items).map((l) => [l.id, l])
  )
  const inViewItems = items.filter((b) => inView(view, b))
  const states = STATES.filter((s) => inViewItems.some((b) => b.state === s))
  const needle = q.trim().toLowerCase()
  const rows = inViewItems.filter(
    (b) =>
      (!state || b.state === state) &&
      (!needle || b.name.toLowerCase().includes(needle) || b.subject.toLowerCase().includes(needle))
  )
  const selection = useSelection(
    rows.filter(deletable).map((b) => b.id),
    `${view}|${inboxId}|${q}|${state}`
  )
  const sending = view === "all" || view === "sent" ? items.find(isActive) : undefined
  const latest = [...items].sort((a, b) => b.updated_at.localeCompare(a.updated_at))[0]
  const senderInbox = inboxId || latest?.inbox_id
  const listRef = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const open = (id: string) => {
    rememberList(view)
    navigate(broadcastLink(id))
  }
  const { cursorId, setCursorId } = useListCursor(rows, null, open, !creating, listRef, true, true)
  const [scope, setScope] = useState(`${view}|${inboxId}|${q}|${state}`)
  if (scope !== `${view}|${inboxId}|${q}|${state}`) {
    setScope(`${view}|${inboxId}|${q}|${state}`)
    setCursorId(null)
  }
  useLayoutEffect(() => {
    const back = rememberedList()
    if (back.y) window.scrollTo({ top: back.y })
    rememberList(back.view, 0)
  }, [])
  const mayCreate = canManage && status?.mode !== "off"
  useHotkeys(
    {
      ...(mayCreate ? { [BROADCAST_KEYS.create]: () => setCreating(true) } : {}),
      [SHORTCUTS.search]: () => search.current?.focus(),
      ...(canManage
        ? {
            [SHORTCUTS.select]: () => {
              const b = rows.find((x) => x.id === cursorId)
              if (b && deletable(b)) selection.toggle(b.id)
            },
          }
        : {}),
      ...Object.fromEntries(VIEWS.map((v, i) => [String(i + 1), () => navigate(viewLink(v))])),
    },
    !creating
  )
  const anyFilter = !!(q || chosen || state)
  const clearFilters = () => {
    set("q", "")
    set("inbox", "")
    set("state", "")
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(rows.length)}${broadcasts.hasNextPage ? "+" : ""}`
  const check = canManage
    ? (b: Broadcast) => (deletable(b) ? (range: boolean) => selection.toggle(b.id, range) : undefined)
    : undefined
  return (
    <BroadcastsLayout testId="broadcasts-home">
      <section aria-label={names[view]} className="@container/list min-w-0">
        <PageTitle
          title={names[view]}
          sub={
            <span className="inline-flex min-w-0 flex-wrap items-center gap-x-1.5" data-testid="list-lede">
              {senderInbox ? (
                <SenderLine inboxId={senderInbox} channelId={inboxId ? undefined : latest?.channel_id} />
              ) : (
                <Trans>E-mails to the people who signed up</Trans>
              )}
            </span>
          }
          actions={
            mayCreate && (
              <Button
                onClick={() => setCreating(true)}
                disabled={creating}
                title={keyHint(t`New broadcast`, BROADCAST_KEYS.create)}
                data-testid="broadcasts-home-new"
              >
                <PlusIcon />
                <Trans>New broadcast</Trans>
              </Button>
            )
          }
          testId="broadcasts-head"
        />
        {creating && (
          <NewBroadcastForm
            inboxId={inboxId}
            onClose={() => setCreating(false)}
            onCreated={(b) => {
              setCreating(false)
              navigate(broadcastLink(b.id))
            }}
          />
        )}
        <StatusNotice className="mt-4" />
        {sending && <SendingNow b={sending} />}
        <FilterRow testId="broadcasts-filters">
          <FilterSearch
            inputRef={search}
            value={q}
            onChange={(v) => set("q", v)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && rows[0]) {
                e.preventDefault()
                open(rows[0].id)
              }
            }}
            placeholder={t`Search name or subject…`}
            label={t`Search broadcasts`}
            testId="broadcasts-search"
          />
          <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="broadcasts-product" />
          {(states.length > 1 || state) && (
            <FilterMenu
              value={state}
              onChange={(v) => set("state", v)}
              options={states.map((s) => ({ value: s, label: text.broadcastState[s] }))}
              anyLabel={t`Any status`}
              label={t`Filter by status`}
              testId="broadcasts-state"
            />
          )}
          {anyFilter && (
            <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="broadcasts-clear">
              <Trans>Clear filters</Trans>
            </Button>
          )}
        </FilterRow>
        <div ref={listRef}>
          <ListBox bar={canManage && <BroadcastBulkBar items={rows} selection={selection} />} testId="broadcasts-table">
            {broadcasts.isPending ? (
              <ListBoxSkeleton />
            ) : broadcasts.error && items.length === 0 ? (
              <ListBoxEmpty
                action={
                  <Button variant="outline" size="sm" onClick={() => void broadcasts.refetch()}>
                    <Trans>Try again</Trans>
                  </Button>
                }
              >
                <ErrorLine error={broadcasts.error} />
              </ListBoxEmpty>
            ) : items.length === 0 ? (
              <ListBoxEmpty
                testId="broadcasts-home-empty"
                action={
                  mayCreate && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCreating(true)}
                      data-testid="broadcasts-empty-new"
                    >
                      <Trans>New broadcast</Trans>
                    </Button>
                  )
                }
              >
                <span className="flex max-w-xl flex-col gap-2 text-start">
                  <span className="font-semibold text-foreground">
                    <Trans>No broadcasts yet</Trans>
                  </span>
                  <BroadcastSteps />
                </span>
              </ListBoxEmpty>
            ) : rows.length === 0 ? (
              <ListBoxEmpty
                testId="broadcasts-tab-empty"
                action={
                  anyFilter && (
                    <Button variant="outline" size="sm" onClick={clearFilters}>
                      <Trans>Clear filters</Trans>
                    </Button>
                  )
                }
              >
                {needle ? (
                  <Trans>No broadcast matches “{q}”.</Trans>
                ) : view === "sent" ? (
                  <Trans>No broadcasts have gone out yet.</Trans>
                ) : view === "scheduled" ? (
                  <Trans>No broadcast is scheduled.</Trans>
                ) : view === "drafts" ? (
                  <Trans>No drafts.</Trans>
                ) : (
                  <Trans>Nothing here.</Trans>
                )}
              </ListBoxEmpty>
            ) : (
              <Table className="table-fixed text-body">
                <TableHeader>
                  <TableRow className="border-line-soft hover:bg-transparent" data-testid="broadcasts-columns">
                    {canManage && <TableHead className="w-10 ps-3 pe-0" aria-hidden />}
                    <TableHead className={cn(head, !canManage && "ps-4")}>
                      <Trans>Broadcast</Trans>
                    </TableHead>
                    <TableHead className={cn(head, "w-28")}>
                      <Trans>Status</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hidePeople, "w-24 text-end")}>
                      <Trans>People</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideClicked, "w-24 text-end")}>
                      <Trans>Clicked</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideReplied, "w-24 pe-4 text-end")}>
                      <Trans>Replied</Trans>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((b) => (
                    <Row
                      key={b.id}
                      b={b}
                      lists={byId}
                      cursor={b.id === cursorId}
                      checked={selection.has(b.id)}
                      selectable={canManage}
                      selecting={selection.selecting}
                      check={check?.(b)}
                      onOpen={() => open(b.id)}
                    />
                  ))}
                </TableBody>
              </Table>
            )}
          </ListBox>
        </div>
        <ErrorLine error={broadcasts.data ? broadcasts.error : null} className="py-2" />
        {broadcasts.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => void broadcasts.fetchNextPage()}
            disabled={broadcasts.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
        <p className="m-0 mt-3 text-small text-muted-foreground" data-testid="broadcasts-count">
          {plural(rows.length, { one: `${shown} broadcast`, other: `${shown} broadcasts` })}
          {" · "}
          <Trans>Yuva counts clicks and replies. It does not track opens.</Trans>
        </p>
        <KeyHints
          items={[
            [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
            [
              ["↵"],
              <Trans key="open" context="keyboard hint">
                open
              </Trans>,
            ],
            ...(canManage
              ? [[[SHORTCUTS.select], <Trans key="select">select</Trans>] as [string[], React.ReactNode]]
              : []),
            [[SHORTCUTS.search], <Trans key="search">search</Trans>],
          ]}
        />
      </section>
    </BroadcastsLayout>
  )
}

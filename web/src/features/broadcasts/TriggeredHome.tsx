import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useMemo, useRef, useState } from "react"
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
} from "@/components/common/ListBox"
import { SHORTCUTS, TRIGGERED_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { broadcastLink, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { keyHint, StateBadge, StatusNotice, useBroadcastsStatus, useNumber } from "@/features/broadcasts/BroadcastParts"
import { BroadcastsLayout, ProductFilter, useProductScope, useViewNames } from "@/features/broadcasts/BroadcastsFrame"
import { NewBroadcastForm } from "@/features/broadcasts/NewBroadcastForm"
import { sentenceCase, useTriggerSentence } from "@/features/broadcasts/Trigger"
import { useListCursor } from "@/features/inbox/listCursor"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Broadcast } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const FILTERS = ["active", "paused", "drafts", "stopped"] as const
type Filter = (typeof FILTERS)[number]

function matches(f: Filter | "", b: Broadcast) {
  if (f === "active") return b.state === "active" || b.state === "scheduled"
  if (f === "paused") return b.state === "paused"
  if (f === "drafts") return b.state === "draft"
  if (f === "stopped") return b.state === "cancelled" || b.state === "failed"
  return true
}

const head = "h-9 px-3 text-start text-caption font-medium text-muted-foreground"
const cell = "px-3 py-2.5 text-body"
const hideSent = "@max-xl/list:hidden"
const hideReplied = "@max-2xl/list:hidden"
const hideWaiting = "@max-md/list:hidden"

function Row({ b, cursor, onOpen }: { b: Broadcast; cursor: boolean; onOpen: () => void }) {
  const { i18n } = useLingui()
  const num = useNumber()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const sentence = sentenceCase(useTriggerSentence(b), i18n.locale)
  return (
    <TableRow
      onClick={onOpen}
      className={cn(
        "cursor-pointer border-line-soft hover:bg-subtle",
        cursor && "bg-subtle [&>td:first-child]:shadow-[inset_2px_0_0_var(--brand)]"
      )}
      data-row-id={b.id}
      data-testid="triggered-row"
    >
      <TableCell className={cn(cell, "min-w-0 overflow-hidden ps-4")}>
        <Link
          to={broadcastLink(b.id)}
          onClick={(e) => e.stopPropagation()}
          title={inbox?.name}
          className="flex min-w-0 flex-col rounded-sm leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="truncate font-medium text-foreground" dir="auto">
            {b.name}
          </span>
          <span className="truncate text-small text-muted-foreground">{sentence}</span>
        </Link>
      </TableCell>
      <TableCell className={cn(cell, "w-28")}>
        <StateBadge state={b.state} kind={b.kind} />
      </TableCell>
      <TableCell className={cn(cell, hideWaiting, "w-24 text-end text-fg2 tabular-nums")}>
        {num(b.counts.waiting ?? 0)}
      </TableCell>
      <TableCell className={cn(cell, hideSent, "w-24 text-end text-fg2 tabular-nums")}>{num(b.counts.sent)}</TableCell>
      <TableCell className={cn(cell, hideReplied, "w-24 pe-4 text-end text-fg2 tabular-nums")}>
        {b.counts.sent > 0 ? num(b.counts.replied) : "–"}
      </TableCell>
    </TableRow>
  )
}

export function TriggeredHome() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const status = useBroadcastsStatus()
  const views = useViewNames()
  const { chosen, inboxId, params, set } = useProductScope()
  const broadcasts = useBroadcasts(inboxId, undefined, "triggered")
  const [creating, setCreating] = useState(false)
  const q = params.get("q") ?? ""
  const raw = params.get("tab") as Filter | null
  const filter: Filter | "" = raw && FILTERS.includes(raw) ? raw : ""
  const items = useMemo(() => broadcasts.data?.pages.flatMap((p) => p.items) ?? [], [broadcasts.data])
  const needle = q.trim().toLowerCase()
  const rows = items.filter((b) => matches(filter, b) && (!needle || b.name.toLowerCase().includes(needle)))
  const names: Record<Filter, string> = {
    active: t`Active`,
    paused: t`Paused`,
    drafts: t`Drafts`,
    stopped: t`Stopped`,
  }
  const listRef = useRef<HTMLDivElement>(null)
  const search = useRef<HTMLInputElement>(null)
  const open = (id: string) => navigate(broadcastLink(id))
  const { cursorId, setCursorId } = useListCursor(rows, null, open, !creating, listRef, true, true)
  const [scope, setScope] = useState(`${filter}|${inboxId}|${q}`)
  if (scope !== `${filter}|${inboxId}|${q}`) {
    setScope(`${filter}|${inboxId}|${q}`)
    setCursorId(null)
  }
  const mayCreate = canManage && status?.mode !== "off"
  useHotkeys(
    {
      ...(mayCreate ? { [TRIGGERED_KEYS.create]: () => setCreating(true) } : {}),
      [SHORTCUTS.search]: () => search.current?.focus(),
    },
    !creating
  )
  const anyFilter = !!(q || chosen || filter)
  const clearFilters = () => {
    set("q", "")
    set("inbox", "")
    set("tab", "")
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(rows.length)}${broadcasts.hasNextPage ? "+" : ""}`
  return (
    <BroadcastsLayout testId="triggered-home">
      <section aria-label={views.triggered} className="@container/list min-w-0">
        <PageTitle
          title={t`Triggered e-mails`}
          sub={<Trans>One mail to each person, a set time after they sign in</Trans>}
          actions={
            mayCreate && (
              <Button
                onClick={() => setCreating(true)}
                disabled={creating}
                title={keyHint(t`New triggered e-mail`, TRIGGERED_KEYS.create)}
                data-testid="triggered-new"
              >
                <PlusIcon />
                <Trans>New triggered e-mail</Trans>
              </Button>
            )
          }
          testId="triggered-head"
        />
        {creating && (
          <NewBroadcastForm
            inboxId={inboxId}
            triggered
            onClose={() => setCreating(false)}
            onCreated={(b) => {
              setCreating(false)
              navigate(broadcastLink(b.id))
            }}
          />
        )}
        <StatusNotice className="mt-4" />
        <FilterRow testId="triggered-filters">
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
            placeholder={t`Search by name…`}
            label={t`Search triggered e-mails`}
            testId="triggered-search"
          />
          <ProductFilter value={chosen} onChange={(id) => set("inbox", id)} testId="triggered-product" />
          <FilterMenu
            value={filter}
            onChange={(v) => set("tab", v)}
            options={FILTERS.map((f) => ({ value: f, label: names[f] }))}
            anyLabel={t`Any status`}
            label={t`Filter by status`}
            testId="triggered-state"
          />
          {anyFilter && (
            <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="triggered-clear">
              <Trans>Clear filters</Trans>
            </Button>
          )}
        </FilterRow>
        <div ref={listRef}>
          <ListBox testId="triggered-table">
            {broadcasts.isPending ? (
              <ListBoxSkeleton rows={3} />
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
                testId="triggered-empty"
                action={
                  mayCreate && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setCreating(true)}
                      data-testid="triggered-empty-new"
                    >
                      <Trans>New triggered e-mail</Trans>
                    </Button>
                  )
                }
              >
                <span className="flex max-w-md flex-col gap-2">
                  <span className="font-semibold text-foreground">
                    <Trans>Send a mail a set time after people sign up, once each</Trans>
                  </span>
                  <Trans>
                    A triggered e-mail sends one design to each person a set time after they first sign in to your
                    product, confirm a subscription to a list or start to match a condition, for example a welcome mail
                    on day one and a tip on day three.
                  </Trans>
                </span>
              </ListBoxEmpty>
            ) : rows.length === 0 ? (
              <ListBoxEmpty
                testId="triggered-tab-empty"
                action={
                  <Button variant="outline" size="sm" onClick={clearFilters}>
                    <Trans>Clear filters</Trans>
                  </Button>
                }
              >
                <Trans>Nothing here.</Trans>
              </ListBoxEmpty>
            ) : (
              <Table className="table-fixed text-body">
                <TableHeader>
                  <TableRow className="border-line-soft hover:bg-transparent">
                    <TableHead className={cn(head, "ps-4")}>
                      <Trans>Triggered e-mail</Trans>
                    </TableHead>
                    <TableHead className={cn(head, "w-28")}>
                      <Trans>Status</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideWaiting, "w-24 text-end")}>
                      <Trans>Waiting</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideSent, "w-24 text-end")}>
                      <Trans>Sent</Trans>
                    </TableHead>
                    <TableHead className={cn(head, hideReplied, "w-24 pe-4 text-end")}>
                      <Trans>Replied</Trans>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((b) => (
                    <Row key={b.id} b={b} cursor={b.id === cursorId} onOpen={() => open(b.id)} />
                  ))}
                </TableBody>
              </Table>
            )}
          </ListBox>
        </div>
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
        <p className="m-0 mt-3 text-small text-muted-foreground">
          {plural(rows.length, { one: `${shown} triggered e-mail`, other: `${shown} triggered e-mails` })}
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
            [[SHORTCUTS.search], <Trans key="search">search</Trans>],
          ]}
        />
      </section>
    </BroadcastsLayout>
  )
}

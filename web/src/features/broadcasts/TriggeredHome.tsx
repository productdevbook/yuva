import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, PlusIcon, ZapIcon } from "lucide-react"
import { useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine, FlatEmpty, FlatRow, FlatWho, InfoSection, KV, PageHeading, PageTop } from "@/components/common"
import { TRIGGERED_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { broadcastLink, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import {
  keyHint,
  StateBadge,
  StateLine,
  StatusNotice,
  useBroadcastsStatus,
  useNumber,
} from "@/features/broadcasts/BroadcastParts"
import { BroadcastsLayout, TopTrail, useFlat } from "@/features/broadcasts/BroadcastsFrame"
import { NewBroadcastDialog } from "@/features/broadcasts/NewBroadcastDialog"
import { sentenceCase, useTriggerSentence } from "@/features/broadcasts/Trigger"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop as StackTop } from "@/features/inbox/PageFrame"
import { useListCursor } from "@/features/inbox/listCursor"
import { ProductTag } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useRowFocus } from "@/hooks/use-row-focus"
import type { Broadcast } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

const FILTERS = ["all", "active", "paused", "drafts", "stopped"] as const
type Filter = (typeof FILTERS)[number]

function matches(f: Filter, b: Broadcast) {
  if (f === "active") return b.state === "active" || b.state === "scheduled"
  if (f === "paused") return b.state === "paused"
  if (f === "drafts") return b.state === "draft"
  if (f === "stopped") return b.state === "cancelled" || b.state === "failed"
  return true
}

const cols =
  "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 @2xl:grid-cols-[minmax(0,1fr)_96px_72px_72px_72px]"

function Row({ b }: { b: Broadcast }) {
  const { i18n } = useLingui()
  const num = useNumber()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const sentence = sentenceCase(useTriggerSentence(b), i18n.locale)
  const waiting = num(b.counts.waiting ?? 0)
  const sent = num(b.counts.sent)
  const live = b.state === "active"
  return (
    <li
      className={cn(
        "relative flex items-center border-t first:border-t-0",
        live &&
          "before:absolute before:inset-y-3 before:start-1 before:z-10 before:w-[3px] before:rounded-[3px] before:bg-brand"
      )}
    >
      <Link
        to={broadcastLink(b.id)}
        className={cn(
          cols,
          "min-h-14 min-w-0 flex-1 px-4 py-2.5 text-[13px] outline-none transition-colors hover:bg-surface focus-visible:bg-surface"
        )}
        data-testid="triggered-row"
      >
        <span className="flex min-w-0 flex-col">
          <span className="flex min-w-0 items-center gap-2">
            <span className="truncate text-body font-semibold">{b.name}</span>
            {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
          </span>
          <span className="truncate text-small text-muted-foreground">{sentence}</span>
        </span>
        <span className="justify-self-end @2xl:justify-self-start">
          <StateBadge state={b.state} kind={b.kind} />
        </span>
        <span className="hidden text-body tabular-nums @2xl:block">{num(b.counts.waiting ?? 0)}</span>
        <span className="hidden text-body tabular-nums @2xl:block">{num(b.counts.sent)}</span>
        <span className="hidden text-body tabular-nums @2xl:block">
          {b.counts.sent > 0 ? num(b.counts.replied) : "–"}
        </span>
        <span className="col-span-2 flex flex-wrap items-center gap-x-3 text-caption text-faint tabular-nums @2xl:hidden">
          <span>
            <Trans>{waiting} waiting</Trans>
          </span>
          <span>
            <Trans>{sent} sent</Trans>
          </span>
        </span>
      </Link>
    </li>
  )
}

function Empty({
  filter,
  canCreate,
  onNew,
  flat,
}: {
  filter: Filter
  canCreate: boolean
  onNew: () => void
  flat?: boolean
}) {
  const first = filter === "all"
  return (
    <div
      className={cn(
        "flex flex-col gap-3 text-body text-muted-foreground",
        flat
          ? "mt-7 items-start border-t border-line-soft pt-4"
          : "mx-auto w-[min(560px,100%)] items-center rounded-[14px] border bg-card px-6 py-10 text-center"
      )}
      data-testid={first ? "triggered-empty" : "triggered-tab-empty"}
    >
      {!flat && (
        <span className="grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
          <ZapIcon className="size-5" aria-hidden />
        </span>
      )}
      <p className="font-semibold text-foreground">
        {first ? <Trans>Send a mail a set time after people sign up, once each</Trans> : <Trans>Nothing here.</Trans>}
      </p>
      {first && (
        <p className="max-w-md">
          <Trans>
            A triggered e-mail sends one design to each person a set time after they first sign in to your product,
            confirm a subscription to a list or start to match a condition, for example a welcome mail on day one and a
            tip on day three.
          </Trans>
        </p>
      )}
      {canCreate && first && (
        <Button className={look.btnInk} onClick={onNew} data-testid="triggered-empty-new">
          <Trans>New triggered e-mail</Trans>
        </Button>
      )}
    </div>
  )
}

function StackedTriggered() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const status = useBroadcastsStatus()
  const [inboxId] = useInboxFilter()
  const broadcasts = useBroadcasts(inboxId, undefined, "triggered")
  const [creating, setCreating] = useState(false)
  const [query, setQuery] = useState("")
  const raw = params.get("tab") as Filter | null
  const filter: Filter = raw && FILTERS.includes(raw) ? raw : "all"
  const items = broadcasts.data?.pages.flatMap((p) => p.items) ?? []
  const q = query.trim().toLowerCase()
  const rows = items.filter((b) => matches(filter, b) && (!q || b.name.toLowerCase().includes(q)))
  const names: Record<Filter, string> = {
    all: t`All`,
    active: t`Active`,
    paused: t`Paused`,
    drafts: t`Drafts`,
    stopped: t`Stopped`,
  }
  const tabs = FILTERS.map((f) => ({
    key: f,
    to: f === "all" ? { search: "" } : { search: `?tab=${f}` },
    label: names[f],
    count:
      f !== "all" && !broadcasts.hasNextPage && items.some((b) => matches(f, b))
        ? items.filter((b) => matches(f, b)).length
        : undefined,
    testId: `triggered-filter-${f}`,
  }))
  const mayCreate = canManage && status?.mode !== "off"
  const table = useRowFocus<HTMLUListElement>(!creating)
  useHotkeys(
    {
      ...(mayCreate ? { [TRIGGERED_KEYS.create]: () => setCreating(true) } : {}),
      ...Object.fromEntries(
        FILTERS.map((f, i) => [
          String(i + 1),
          () => navigate(f === "all" ? "/broadcasts/triggered" : `/broadcasts/triggered?tab=${f}`),
        ])
      ),
    },
    !creating
  )
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="triggered-home">
      <StackTop
        title={<Trans>Triggered e-mails</Trans>}
        sub={<Trans>One mail to each person, a set time after they sign in</Trans>}
        actions={
          mayCreate && (
            <Button
              className={look.btnInk}
              onClick={() => setCreating(true)}
              title={keyHint(t`New triggered e-mail`, TRIGGERED_KEYS.create)}
              data-testid="triggered-new"
            >
              <Trans>New triggered e-mail</Trans>
            </Button>
          )
        }
        tabs={<FrameTabs label={t`Triggered e-mails`} value={filter} items={tabs} />}
      />
      <div className="@container min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-[1360px] flex-col gap-4 p-6 pb-16 @max-xl:p-4">
          <StatusNotice />
          {broadcasts.isPending ? (
            <div className="flex flex-col gap-4" aria-busy data-testid="triggered-loading">
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
            <Empty filter="all" canCreate={mayCreate} onNew={() => setCreating(true)} />
          ) : (
            <>
              <ErrorLine error={broadcasts.error} />
              <div className="overflow-hidden rounded-[14px] border bg-card" data-testid="triggered-table">
                <div className="flex items-center gap-2 border-b px-4 py-3">
                  <Input
                    type="search"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t`Search`}
                    aria-label={t`Search`}
                    className="h-[34px] w-64 rounded-[9px] bg-card phone:w-full"
                  />
                </div>
                <div className={cn(cols, look.over, "hidden h-9 border-b px-4 @2xl:grid")} aria-hidden>
                  <span>
                    <Trans>Triggered e-mail</Trans>
                  </span>
                  <span>
                    <Trans>Status</Trans>
                  </span>
                  <span>
                    <Trans>Waiting</Trans>
                  </span>
                  <span>
                    <Trans>Sent</Trans>
                  </span>
                  <span>
                    <Trans>Replied</Trans>
                  </span>
                </div>
                {rows.length === 0 ? (
                  <p className="px-4 py-8 text-center text-body text-muted-foreground">
                    <Trans>Nothing here.</Trans>
                  </p>
                ) : (
                  <ul ref={table}>
                    {rows.map((b) => (
                      <Row key={b.id} b={b} />
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
              <p className="text-small text-faint">
                <Trans>Yuva counts clicks and replies. It does not track opens.</Trans>
              </p>
            </>
          )}
        </div>
      </div>
      {creating && (
        <NewBroadcastDialog
          inboxId={inboxId}
          triggered
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

function FlatTriggeredRow({ b, cursor, onOpen }: { b: Broadcast; cursor: boolean; onOpen: () => void }) {
  const { t, i18n } = useLingui()
  const num = useNumber()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === b.inbox_id)
  const sentence = sentenceCase(useTriggerSentence(b), i18n.locale)
  return (
    <FlatRow
      color={inbox?.branding.color}
      cursor={cursor}
      data-row-id={b.id}
      cols="grid-cols-[7px_minmax(0,1fr)_minmax(0,140px)_auto]"
      button={{ onClick: onOpen, title: inbox?.name, "data-testid": "triggered-row" }}
      who={
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <FlatWho strong>{b.name}</FlatWho>
          <span className="truncate text-small text-muted-foreground">{sentence}</span>
        </span>
      }
      end={
        <>
          <span className="w-16 text-end font-mono text-xs text-fg2 tabular-nums" title={t`Waiting`}>
            {num(b.counts.waiting ?? 0)}
          </span>
          <span className="w-16 text-end font-mono text-xs text-fg2 tabular-nums @max-xl/list:hidden" title={t`Sent`}>
            {num(b.counts.sent)}
          </span>
          <span
            className="w-16 text-end font-mono text-xs text-muted-foreground tabular-nums @max-2xl/list:hidden"
            title={t`Replied`}
          >
            {b.counts.sent > 0 ? num(b.counts.replied) : "–"}
          </span>
        </>
      }
    >
      <StateLine state={b.state} kind={b.kind} />
    </FlatRow>
  )
}

function FlatTriggered() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const { canManage } = useSession()
  const status = useBroadcastsStatus()
  const [inboxId] = useInboxFilter()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  const broadcasts = useBroadcasts(inboxId, undefined, "triggered")
  const [creating, setCreating] = useState(false)
  const [query, setQuery] = useState("")
  const raw = params.get("tab") as Filter | null
  const filter: Filter = raw && FILTERS.includes(raw) ? raw : "all"
  const items = useMemo(() => broadcasts.data?.pages.flatMap((p) => p.items) ?? [], [broadcasts.data])
  const q = query.trim().toLowerCase()
  const rows = items.filter((b) => matches(filter, b) && (!q || b.name.toLowerCase().includes(q)))
  const names: Record<Filter, string> = {
    all: t`All`,
    active: t`Active`,
    paused: t`Paused`,
    drafts: t`Drafts`,
    stopped: t`Stopped`,
  }
  const complete = !!broadcasts.data && !broadcasts.hasNextPage
  const sub = FILTERS.map((f) => ({
    key: f,
    label: names[f],
    to: f === "all" ? "/broadcasts/triggered" : `/broadcasts/triggered?tab=${f}`,
    current: filter === f,
    count: f !== "all" && complete ? items.filter((b) => matches(f, b)).length : undefined,
    testId: `triggered-filter-${f}`,
  }))
  const listRef = useRef<HTMLDivElement>(null)
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
      ...Object.fromEntries(
        FILTERS.map((f, i) => [
          String(i + 1),
          () => navigate(f === "all" ? "/broadcasts/triggered" : `/broadcasts/triggered?tab=${f}`),
        ])
      ),
    },
    !creating
  )
  const num = useNumber()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const count = rows.length
  const n = `${fmt.format(count)}${broadcasts.hasNextPage ? "+" : ""}`
  const sum = (pick: (b: Broadcast) => number) => items.reduce((x, b) => x + pick(b), 0)
  return (
    <BroadcastsLayout
      sub={sub}
      testId="triggered-home"
      aside={
        complete &&
        items.length > 0 && (
          <InfoSection title={t`Triggered e-mails`} testId="triggered-summary">
            <KV label={t`Active`} value={num(items.filter((b) => b.state === "active").length)} tone="brand" />
            <KV label={t`Waiting`} value={num(sum((b) => b.counts.waiting ?? 0))} />
            <KV label={t`Sent`} value={num(sum((b) => b.counts.sent))} />
            <KV label={t`Replied`} value={num(sum((b) => b.counts.replied))} />
          </InfoSection>
        )
      }
    >
      <PageTop
        actions={
          mayCreate && (
            <Button
              className="h-8 rounded-[9px] bg-foreground px-3.5 font-semibold text-background hover:bg-foreground/90"
              onClick={() => setCreating(true)}
              title={keyHint(t`New triggered e-mail`, TRIGGERED_KEYS.create)}
              data-testid="triggered-new"
            >
              <PlusIcon className="size-3.5" aria-hidden />
              <Trans>New triggered e-mail</Trans>
            </Button>
          )
        }
      >
        <TopTrail place="triggered" />
      </PageTop>
      <section aria-label={t`Triggered e-mails`} className="@container/list flex max-w-[980px] flex-col">
        <PageHeading
          title={filter === "all" ? t`Triggered e-mails` : `${t`Triggered`} · ${names[filter]}`}
          lede={
            <span data-testid="list-lede">
              {broadcasts.hasNextPage ? (
                <Trans>{n} triggered e-mails</Trans>
              ) : (
                <Plural value={count} one="# triggered e-mail" other="# triggered e-mails" />
              )}
              {" · "}
              {product ?? t`every product`}
              {". "}
              <Trans>One mail to each person, a set time after they sign in</Trans>
            </span>
          }
        />
        <StatusNotice className="mt-5" />
        {broadcasts.isPending ? (
          <div className="mt-7 flex flex-col gap-3" aria-busy data-testid="triggered-loading">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        ) : broadcasts.error && items.length === 0 ? (
          <FlatEmpty>
            <ErrorLine error={broadcasts.error} />
            <Button variant="outline" className={look.btn} onClick={() => void broadcasts.refetch()}>
              <Trans>Try again</Trans>
            </Button>
          </FlatEmpty>
        ) : items.length === 0 ? (
          <Empty filter="all" canCreate={mayCreate} onNew={() => setCreating(true)} flat />
        ) : (
          <>
            <ErrorLine error={broadcasts.error} />
            <div className="mt-5 flex">
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t`Search`}
                aria-label={t`Search`}
                className="h-[34px] w-64 rounded-[9px] bg-card"
              />
            </div>
            {rows.length === 0 ? (
              <FlatEmpty testId="triggered-tab-empty">
                <p className="m-0">
                  <Trans>Nothing here.</Trans>
                </p>
              </FlatEmpty>
            ) : (
              <div ref={listRef} className="mt-7" data-testid="triggered-table">
                <div
                  className="grid grid-cols-[7px_minmax(0,1fr)_minmax(0,140px)_auto] gap-x-3.5 px-1 pb-2 text-caption font-medium text-muted2"
                  aria-hidden
                >
                  <span />
                  <span>
                    <Trans>Triggered e-mail</Trans>
                  </span>
                  <span>
                    <Trans>Status</Trans>
                  </span>
                  <span className="flex justify-end gap-2">
                    <span className="w-16 text-end">
                      <Trans>Waiting</Trans>
                    </span>
                    <span className="w-16 text-end @max-xl/list:hidden">
                      <Trans>Sent</Trans>
                    </span>
                    <span className="w-16 text-end @max-2xl/list:hidden">
                      <Trans>Replied</Trans>
                    </span>
                  </span>
                </div>
                {rows.map((b) => (
                  <FlatTriggeredRow key={b.id} b={b} cursor={b.id === cursorId} onOpen={() => open(b.id)} />
                ))}
              </div>
            )}
            {broadcasts.hasNextPage && (
              <div className="flex border-t border-line-soft pt-4">
                <Button
                  variant="outline"
                  className={look.btn}
                  onClick={() => void broadcasts.fetchNextPage()}
                  disabled={broadcasts.isFetchingNextPage}
                >
                  <Trans>Load more</Trans>
                </Button>
              </div>
            )}
          </>
        )}
      </section>
      {creating && (
        <NewBroadcastDialog
          inboxId={inboxId}
          triggered
          onClose={() => setCreating(false)}
          onCreated={(b) => {
            setCreating(false)
            navigate(broadcastLink(b.id))
          }}
        />
      )}
    </BroadcastsLayout>
  )
}

export function TriggeredHome() {
  return useFlat() ? <FlatTriggered /> : <StackedTriggered />
}

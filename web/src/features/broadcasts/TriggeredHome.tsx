import { Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, ZapIcon } from "lucide-react"
import { useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { TRIGGERED_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { broadcastLink, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { keyHint, StateBadge, StatusNotice, useBroadcastsStatus, useNumber } from "@/features/broadcasts/BroadcastParts"
import { NewBroadcastDialog } from "@/features/broadcasts/NewBroadcastDialog"
import { sentenceCase, useTriggerSentence } from "@/features/broadcasts/Trigger"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
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

function Empty({ filter, canCreate, onNew }: { filter: Filter; canCreate: boolean; onNew: () => void }) {
  const first = filter === "all"
  return (
    <div
      className="mx-auto flex w-[min(560px,100%)] flex-col items-center gap-3 rounded-[14px] border bg-card px-6 py-10 text-center text-body text-muted-foreground"
      data-testid={first ? "triggered-empty" : "triggered-tab-empty"}
    >
      <span className="grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
        <ZapIcon className="size-5" aria-hidden />
      </span>
      <p className="font-semibold text-foreground">
        {first ? <Trans>Send a mail a set time after people sign up, once each</Trans> : <Trans>Nothing here.</Trans>}
      </p>
      {first && (
        <p className="max-w-md">
          <Trans>
            A triggered e-mail sends one design to each person a set time after they first sign in to your product, for
            example a welcome mail on day one and a tip on day three.
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

export function TriggeredHome() {
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
      <PageTop
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

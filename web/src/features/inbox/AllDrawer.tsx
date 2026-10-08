import { Trans, useLingui } from "@lingui/react/macro"
import { SearchIcon, XIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { ContactAvatar, Dot, ErrorLine } from "@/components/common"
import { formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useConversations } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { useShownId } from "@/features/inbox/WaitingPill"
import type { ConversationListItem, ConversationStatus } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

type Tab = "waiting" | "snoozed" | "pending" | "team" | "closed"
const TABS: Tab[] = ["waiting", "snoozed", "pending", "team", "closed"]

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

export function ConversationRow({ c, current, onOpen }: { c: ConversationListItem; current: boolean; onOpen: () => void }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const channel = useChannelMap().get(c.channel_id ?? "")
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  const preview = c.last_message?.text || c.subject
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn("flex w-full gap-3 rounded-xl px-2.5 py-3 text-start transition-colors hover:bg-card", current && "bg-brand-wash hover:bg-brand-wash")}
      data-testid="drawer-row"
    >
      <ContactAvatar id={c.contact.id} name={name} className="size-9 text-xs" />
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          <b className={cn("min-w-0 truncate font-medium", c.unread && "font-semibold")}>{name}</b>
          <time className="ms-auto shrink-0 text-xs text-faint" dateTime={c.last_activity_at}>
            {formatShort(c.last_activity_at, i18n.locale)}
          </time>
        </span>
        <span className="mt-px flex min-w-0 items-center gap-1.5 text-xs text-faint">
          {inbox && (
            <>
              <Dot color={inbox.branding.color} className="size-2 rounded-[3px]" />
              <span className="truncate">{inbox.name}</span>
            </>
          )}
          {channel && (
            <span className="shrink-0">
              {inbox && "· "}
              {text.channel[channel.kind]}
            </span>
          )}
        </span>
        {preview && <span className="mt-0.5 block truncate text-[13px] text-muted-foreground">{preview}</span>}
      </span>
    </button>
  )
}

export function AllDrawer({
  open,
  onOpenChange,
  query,
  onQuery,
  focusKey,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  query: string
  onQuery: (q: string) => void
  focusKey: number
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { membership } = useSession()
  const queue = useQueue()
  const shown = useShownId()
  const [tab, setTab] = useState<Tab>("waiting")
  const q = useDebounced(query.trim(), 250)
  const input = useRef<HTMLInputElement>(null)
  const inbox = queue.inboxId || undefined
  const by = (status: ConversationStatus, enabled: boolean) => ({ status, q: q || undefined, inbox_id: inbox, enabled })
  const lists = {
    open: useListOf(by("open", open && !!q)),
    snoozed: useListOf(by("snoozed", open)),
    pending: useListOf(by("pending", open)),
    closed: useListOf(by("closed", open)),
  }

  useEffect(() => {
    if (!open) return
    const id = requestAnimationFrame(() => input.current?.focus())
    return () => cancelAnimationFrame(id)
  }, [open, focusKey])

  const me = membership.member_id
  const searched = lists.open.items
  const rows: Record<Tab, { items: ConversationListItem[]; more: boolean; pending: boolean; error: unknown }> = {
    waiting: q
      ? { ...lists.open, items: searched.filter((c) => !c.assignee_id || c.assignee_id === me) }
      : { items: queue.waiting, more: false, pending: queue.open.isPending, error: queue.open.error },
    team: q
      ? { ...lists.open, items: searched.filter((c) => c.assignee_id && c.assignee_id !== me) }
      : { items: queue.team, more: false, pending: queue.open.isPending, error: queue.open.error },
    snoozed: lists.snoozed,
    pending: lists.pending,
    closed: lists.closed,
  }
  const labels: Record<Tab, string> = {
    waiting: t`Waiting`,
    snoozed: t`Snoozed`,
    pending: t`Replied`,
    team: t`With team`,
    closed: t`Done`,
  }
  const current = rows[tab]
  const fmt = new Intl.NumberFormat(i18n.locale, { notation: "compact" })
  const openRow = (c: ConversationListItem) => {
    onOpenChange(false)
    if (queue.waiting.some((x) => x.id === c.id)) queue.show(c.id)
    else navigate(`/conversations/${c.id}`)
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" showCloseButton={false} className="w-[420px] max-w-full bg-background phone:w-full" data-testid="all-drawer">
        <div className="grid gap-3 border-b px-4.5 pt-4.5 pb-3">
          <div className="flex items-center">
            <SheetTitle className="text-[17px]">
              <Trans>Conversations</Trans>
            </SheetTitle>
            <Button variant="ghost" size="icon-sm" className="ms-auto" onClick={() => onOpenChange(false)} aria-label={t`Close`}>
              <XIcon />
            </Button>
          </div>
          <label className="flex items-center gap-2 rounded-xl border bg-card px-3 py-2 text-faint focus-within:border-input">
            <SearchIcon className="size-[15px] shrink-0" />
            <input
              ref={input}
              value={query}
              onChange={(e) => onQuery(e.target.value)}
              placeholder={t`Search people or messages`}
              aria-label={t`Search conversations`}
              className="min-w-0 flex-1 bg-transparent text-base text-foreground outline-none placeholder:text-faint md:text-sm"
              data-testid="drawer-search"
            />
          </label>
          <div role="tablist" className="flex gap-0.5 overflow-x-auto rounded-[10px] border bg-card p-[3px]">
            {TABS.map((k) => {
              const r = rows[k]
              return (
                <button
                  key={k}
                  type="button"
                  role="tab"
                  aria-selected={tab === k}
                  onClick={() => setTab(k)}
                  className={cn(
                    "flex-1 rounded-[7px] px-1.5 py-1.5 text-[13px] whitespace-nowrap text-muted-foreground transition-colors",
                    tab === k && "bg-background font-medium text-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06)]",
                  )}
                  data-testid={`drawer-tab-${k}`}
                >
                  {labels[k]}
                  {!r.pending && (
                    <span className="ms-1 text-xs font-normal text-faint tabular-nums">
                      {fmt.format(r.items.length)}
                      {r.more && "+"}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {current.pending ? (
            <div className="grid gap-2 p-2">
              <Skeleton className="h-14 w-full rounded-xl" />
              <Skeleton className="h-14 w-full rounded-xl" />
            </div>
          ) : current.items.length === 0 ? (
            <p className="px-4 py-12 text-center text-sm text-faint">
              {q ? <Trans>Nothing matches your search here.</Trans> : <Trans>No conversations here.</Trans>}
            </p>
          ) : (
            current.items.map((c) => <ConversationRow key={c.id} c={c} current={c.id === shown} onOpen={() => openRow(c)} />)
          )}
          <ErrorLine error={current.error} className="px-3 py-2" />
        </div>
      </SheetContent>
    </Sheet>
  )
}

function useListOf({ enabled, ...filters }: { status: ConversationStatus; q?: string; inbox_id?: string; enabled: boolean }) {
  const list = useConversations(filters, enabled)
  return {
    items: list.data?.pages.flatMap((p) => p.items) ?? [],
    more: !!list.hasNextPage,
    pending: enabled && list.isPending,
    error: list.error,
  }
}

import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ClockIcon, UserRoundCheckIcon, XIcon } from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { ContactAvatar, Dot, MemberAvatar, toast } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDuration, formatShort, useEnumText, useErrorText } from "@/components/common/text"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import type { Flow } from "@/features/conversation/actions"
import { ListPreview } from "@/features/conversation/ListPreview"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { useListSearch } from "@/features/inbox/listSearch"
import { useBulkUpdateConversations, useConversations } from "@/features/inbox/queries"
import { useQueue, waitingSince } from "@/features/inbox/queue"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { ConversationBulkUpdate, ConversationListItem } from "@/lib/api"
import { useAllViewers } from "@/lib/presence"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useView } from "@/lib/view"
import { useChannelMap, useInboxes, useMemberMap } from "@/lib/workspace"

type Chip = "open" | "pending" | "snoozed" | "closed"
const CHIPS: Chip[] = ["open", "pending", "snoozed", "closed"]

let remembered: { chip: Chip; id: string | null } = { chip: "open", id: null }

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function useNow(ms: number) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(id)
  }, [ms])
  return now
}

function Row({
  c,
  chip,
  current,
  picked,
  now,
  onOpen,
  onPick,
}: {
  c: ConversationListItem
  chip: Chip
  current: boolean
  picked: boolean
  now: number
  onOpen: () => void
  onPick: () => void
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const channel = useChannelMap().get(c.channel_id ?? "")
  const members = useMemberMap()
  const viewers = useAllViewers().get(c.id) ?? []
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  const preview = c.last_message?.text || c.subject
  const rating = currentRating(c)
  let wait = formatShort(c.last_activity_at, i18n.locale)
  let urgent = false
  if (chip === "open") {
    const minutes = Math.max(0, (now - new Date(waitingSince(c)).getTime()) / 60_000)
    wait = formatDuration(minutes * 60, i18n.locale)
    urgent = minutes > (inbox?.expected_reply_minutes ?? 1440)
  }
  const viewer = viewers[0] ? members.get(viewers[0]) : undefined
  return (
    <div
      className={cn("flex items-center gap-2.5 rounded-xl px-2 py-2.5 transition-colors", current ? "bg-brand-wash" : "hover:bg-card")}
      data-testid="list-row"
      data-conversation-id={c.id}
      aria-current={current || undefined}
    >
      <input
        type="checkbox"
        checked={picked}
        onChange={onPick}
        aria-label={t`Select ${name}`}
        className="size-[18px] shrink-0 cursor-pointer accent-primary"
        data-testid="row-check"
      />
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-1 items-center gap-3 text-start">
        <ContactAvatar id={c.contact.id} name={name} className="size-9 text-xs" />
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-baseline gap-2">
            <b className={cn("max-w-[65%] min-w-0 shrink-0 truncate", c.unread ? "font-semibold" : "font-medium")}>{name}</b>
            <span className="inline-flex min-w-0 shrink items-center gap-[5px] truncate text-xs text-faint">
              {inbox && <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />}
              <span className="truncate">{[inbox?.name, channel && text.channel[channel.kind]].filter(Boolean).join(" · ")}</span>
              {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
            </span>
            <span className={cn("ms-auto shrink-0 text-xs whitespace-nowrap", urgent ? "text-brand" : "text-faint")} data-urgent={urgent}>
              {wait}
            </span>
          </span>
          {preview && <span className="mt-0.5 block truncate text-[13px] text-muted-foreground">{preview}</span>}
        </span>
      </button>
      {viewer && (
        <span title={t`${viewer.name || viewer.email} is looking`} data-testid="row-viewer">
          <MemberAvatar name={viewer.name || viewer.email} className="size-5 text-[8px]" />
        </span>
      )}
    </div>
  )
}

function BulkBar({ rows, picked, onClear }: { rows: ConversationListItem[]; picked: Set<string>; onClear: () => void }) {
  const { t, i18n } = useLingui()
  const errorText = useErrorText()
  const { membership } = useSession()
  const bulk = useBulkUpdateConversations()
  const chosen = rows.filter((c) => picked.has(c.id))
  const n = chosen.length
  const run = (body: Omit<ConversationBulkUpdate, "conversation_ids">, done: string, undoOf: (c: ConversationListItem) => Omit<ConversationBulkUpdate, "conversation_ids">) => {
    const before = new Map(chosen.map((c) => [c.id, c]))
    bulk.mutate(
      { conversation_ids: [...before.keys()], ...body },
      {
        onSuccess: (r) => {
          onClear()
          const changed = r.updated.map((c) => before.get(c.id)).filter((c): c is ConversationListItem => !!c)
          toast(done, () => {
            const groups = new Map<string, { body: Omit<ConversationBulkUpdate, "conversation_ids">; ids: string[] }>()
            for (const c of changed) {
              const b = undoOf(c)
              const key = JSON.stringify(b)
              groups.set(key, { body: b, ids: [...(groups.get(key)?.ids ?? []), c.id] })
            }
            for (const g of groups.values()) bulk.mutate({ conversation_ids: g.ids, ...g.body }, { onError: (e) => toast(errorText(e)) })
          })
        },
        onError: (e) => toast(errorText(e)),
      },
    )
  }
  const statusBack = (c: ConversationListItem): Omit<ConversationBulkUpdate, "conversation_ids"> =>
    c.status === "snoozed" && c.snooze_until && new Date(c.snooze_until) > new Date() ? { status: "snoozed", snooze_until: c.snooze_until } : { status: c.status }
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const inHour = new Date(Date.now() + 60 * 60 * 1000)
  const morning = new Date()
  morning.setDate(morning.getDate() + 1)
  morning.setHours(9, 0, 0, 0)
  const button = "inline-flex h-8 items-center gap-1.5 rounded-lg bg-zinc-800 px-3 text-[13px] text-white transition-colors hover:bg-zinc-700 disabled:opacity-50"
  return (
    <div className="mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-xl bg-zinc-950 py-2 ps-3.5 pe-2 text-[13px] text-white" role="toolbar" aria-label={t`Selected conversations`} data-testid="bulk-bar">
      <span className="flex-1">
        <Trans>{n} selected</Trans>
      </span>
      <button type="button" className={button} disabled={bulk.isPending} onClick={() => run({ status: "closed" }, t`${n} closed`, statusBack)} data-testid="bulk-close">
        <CheckIcon className="size-3.5" />
        <Trans context="conversation">Close</Trans>
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger className={button} disabled={bulk.isPending} data-testid="bulk-snooze">
          <ClockIcon className="size-3.5" />
          <Trans>Snooze</Trans>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => run({ status: "snoozed", snooze_until: inHour.toISOString() }, t`${n} snoozed`, statusBack)}>
            <span className="flex-1">
              <Trans>In 1 hour</Trans>
            </span>
            <span className="text-xs text-faint">{time.format(inHour)}</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => run({ status: "snoozed", snooze_until: morning.toISOString() }, t`${n} snoozed`, statusBack)}>
            <span className="flex-1">
              <Trans>Tomorrow morning</Trans>
            </span>
            <span className="text-xs text-faint">{time.format(morning)}</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <button
        type="button"
        className={button}
        disabled={bulk.isPending}
        onClick={() => run({ assignee_id: membership.member_id }, t`${n} assigned to you`, (c) => ({ assignee_id: c.assignee_id ?? null }))}
        data-testid="bulk-assign"
      >
        <UserRoundCheckIcon className="size-3.5" />
        <Trans>Assign to me</Trans>
      </button>
      <button type="button" onClick={onClear} aria-label={t`Clear the selection`} className="grid size-8 place-items-center rounded-lg text-white hover:bg-zinc-800">
        <XIcon className="size-4" />
      </button>
    </div>
  )
}

export function ListPage() {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const queue = useQueue()
  const [, setView] = useView()
  const { query } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [chip, setChipState] = useState<Chip>(remembered.chip)
  const [currentId, setCurrentId] = useState<string | null>(remembered.id)
  const [picked, setPicked] = useState<Set<string>>(() => new Set())
  const now = useNow(30_000)
  const inbox = queue.inboxId || undefined
  const searchOpen = useConversations({ status: "open", q: q || undefined, inbox_id: inbox }, !!q)
  const pending = useConversations({ status: "pending", q: q || undefined, inbox_id: inbox })
  const snoozed = useConversations({ status: "snoozed", q: q || undefined, inbox_id: inbox })
  const closed = useConversations({ status: "closed", q: q || undefined, inbox_id: inbox })
  const flat = (l: typeof pending) => l.data?.pages.flatMap((p) => p.items) ?? []

  const lists: Record<Chip, { items: ConversationListItem[]; more: boolean; pending: boolean }> = useMemo(() => {
    const open = (q ? flat(searchOpen) : (queue.open.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inbox || c.inbox_id === inbox))
      .filter((c) => !queue.leaving.has(c.id))
      .sort((a, b) => (waitingSince(a) < waitingSince(b) ? -1 : waitingSince(a) > waitingSince(b) ? 1 : 0))
    return {
      open: { items: open, more: q ? !!searchOpen.hasNextPage : !!queue.open.hasNextPage, pending: q ? searchOpen.isPending : queue.open.isPending },
      pending: { items: flat(pending).filter((c) => !queue.leaving.has(c.id)), more: !!pending.hasNextPage, pending: pending.isPending },
      snoozed: { items: flat(snoozed).filter((c) => !queue.leaving.has(c.id)), more: !!snoozed.hasNextPage, pending: snoozed.isPending },
      closed: { items: flat(closed).filter((c) => !queue.leaving.has(c.id)), more: !!closed.hasNextPage, pending: closed.isPending },
    }
  }, [q, searchOpen, queue.open, queue.leaving, inbox, pending, snoozed, closed])

  const rows = lists[chip].items
  const current = rows.find((c) => c.id === currentId) ?? rows[0]
  const shownId = phone ? null : (current?.id ?? null)
  useViewing(shownId ?? undefined)
  useEffect(() => {
    remembered = { chip, id: shownId }
  }, [chip, shownId])

  const setChip = (c: Chip) => {
    setChipState(c)
    setCurrentId(null)
    setPicked(new Set())
  }
  const listRef = useRef<HTMLDivElement>(null)
  const select = useCallback((id: string | null) => {
    setCurrentId(id)
    if (!id) return
    requestAnimationFrame(() => listRef.current?.querySelector(`[data-conversation-id="${id}"]`)?.scrollIntoView({ block: "nearest" }))
  }, [])
  const step = (d: 1 | -1) => {
    if (rows.length === 0) return
    const i = current ? rows.findIndex((c) => c.id === current.id) : -1
    select(rows[Math.min(rows.length - 1, Math.max(0, i + d))].id)
  }
  const togglePick = (id: string) =>
    setPicked((s) => {
      const n = new Set(s)
      if (n.has(id)) n.delete(id)
      else n.add(id)
      return n
    })
  const fullScreen = (id: string) => {
    setView("queue")
    queue.show(id)
  }
  const open = (id: string) => (phone ? navigate(`/conversations/${id}`) : select(id))

  const flow: Flow = useMemo(
    () => ({
      advance: (fromId: string) => {
        const i = rows.findIndex((c) => c.id === fromId)
        const rest = rows.filter((c) => c.id !== fromId)
        select((rest[Math.max(0, i)] ?? rest[rest.length - 1])?.id ?? null)
      },
      show: (id: string) => select(id),
    }),
    [rows, select],
  )

  useHotkeys({
    [SHORTCUTS.next]: () => step(1),
    [SHORTCUTS.previous]: () => step(-1),
    [SHORTCUTS.select]: () => current && togglePick(current.id),
    Enter: () => current && fullScreen(current.id),
    Escape: () => picked.size > 0 && setPicked(new Set()),
  })

  const labels: Record<Chip, string> = { open: t`Waiting`, pending: t`Replied`, snoozed: t`Snoozed`, closed: t`Done` }
  const fmt = new Intl.NumberFormat(i18n.locale, { notation: "compact" })
  const list = lists[chip]
  return (
    <main className="flex h-[calc(100svh-var(--topbar,57px))] min-h-0 phone:h-auto" data-testid="list-view">
      <section
        aria-label={t`Conversations`}
        className="flex min-w-0 flex-[1_1_420px] flex-col border-e phone:border-e-0"
      >
        <div className="flex flex-wrap items-center gap-1.5 px-4 py-3">
          {CHIPS.map((k) => {
            const l = lists[k]
            return (
              <button
                key={k}
                type="button"
                aria-pressed={chip === k}
                onClick={() => setChip(k)}
                className={cn(
                  "rounded-full border px-3 py-1.5 text-[13px] transition-colors",
                  chip === k ? "border-foreground bg-foreground text-background" : "bg-card text-muted-foreground hover:text-foreground",
                )}
                data-testid={`chip-${k}`}
              >
                {labels[k]}{" "}
                {!l.pending && (
                  <span className="opacity-70 tabular-nums">
                    {fmt.format(l.items.length)}
                    {l.more && "+"}
                  </span>
                )}
              </button>
            )
          })}
          <span className="flex-1" />
          <span className="text-xs text-faint">{chip === "open" ? <Trans>Oldest first</Trans> : <Trans>Newest first</Trans>}</span>
        </div>
        {picked.size > 0 && <BulkBar rows={rows} picked={picked} onClear={() => setPicked(new Set())} />}
        <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto px-2 pb-4 phone:overflow-visible" data-testid="list">
          {list.pending ? (
            <div className="grid gap-2 p-2">
              <Skeleton className="h-14 w-full rounded-xl" />
              <Skeleton className="h-14 w-full rounded-xl" />
            </div>
          ) : rows.length === 0 ? (
            <p className="px-4 py-12 text-center text-sm text-faint">
              {q ? <Trans>Nothing matches your search here.</Trans> : <Trans>No conversations here.</Trans>}
            </p>
          ) : (
            rows.map((c) => (
              <Row
                key={c.id}
                c={c}
                chip={chip}
                now={now}
                current={!phone && c.id === current?.id}
                picked={picked.has(c.id)}
                onOpen={() => open(c.id)}
                onPick={() => togglePick(c.id)}
              />
            ))
          )}
        </div>
      </section>
      {!phone && (
        <section aria-label={t`Preview`} className="flex min-w-0 flex-[1.4_1_480px] flex-col">
          {current ? (
            <ListPreview key={current.id} id={current.id} flow={flow} onFullScreen={() => fullScreen(current.id)} />
          ) : (
            <p className="m-auto text-sm text-faint">
              <Trans>Pick a conversation to see it here.</Trans>
            </p>
          )}
        </section>
      )}
    </main>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import {
  BookUserIcon,
  BotIcon,
  ClockIcon,
  MailIcon,
  MailOpenIcon,
  PinIcon,
  PinOffIcon,
  CommandIcon,
  EllipsisVerticalIcon,
  InboxIcon,
  KeyboardIcon,
  MessageSquarePlusIcon,
  ReplyIcon,
  SearchIcon,
  SettingsIcon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { useShell } from "@/app/shell"
import { TeamMenu } from "@/app/TeamMenu"
import { UserMenu } from "@/app/UserMenu"
import { ChannelIcon, ContactAvatar, ErrorLine, Kbd, MemberAvatar, toast } from "@/components/common"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatShort, useErrorText } from "@/components/common/text"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from "@/components/ui/context-menu"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { NewConversationDialog } from "@/features/contact/ContactDetails"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { useDraftText } from "@/features/conversation/actions"
import { useLiveIds } from "@/features/conversation/liveness"
import { Ticks } from "@/features/conversation/messages/MessageActions"
import { currentRating, RatingMark } from "@/features/conversation/rating"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { useListActions } from "@/features/inbox/memberState"
import { setListQuery, useListSearch } from "@/features/inbox/listSearch"
import { useBulkUpdateConversations, useConversations, useCounts } from "@/features/inbox/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Contact, ConversationListItem } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useSetAvailability } from "@/lib/availability"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useChannelMap, useMemberMap } from "@/lib/workspace"

export type Chip = "all" | "unread" | "mine" | "unassigned" | "snoozed" | "done"
const CHIPS: Chip[] = ["all", "unread", "mine", "unassigned", "snoozed", "done"]

const chipFilters: Record<Chip, ConversationFilters> = {
  all: {},
  unread: {},
  mine: { status: "open", assignee: "me" },
  unassigned: { status: "open", assignee: "unassigned" },
  snoozed: { status: "snoozed" },
  done: { status: "closed" },
}

let remembered: Chip = "all"
let focused = 0

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function Preview({ c, current, receipts }: { c: ConversationListItem; current: boolean; receipts: boolean }) {
  const { t } = useLingui()
  const draft = useDraftText(c.id)
  const typing = useTyping(c.id).some((a) => a.type === "contact")
  if (typing) {
    return (
      <span className="truncate text-brand" data-testid="row-typing">
        <Trans>typing…</Trans>
      </span>
    )
  }
  if (draft && !current) {
    return (
      <span className="truncate" data-testid="row-draft">
        <span className="font-medium text-brand">
          <Trans>Draft:</Trans>
        </span>{" "}
        {draft}
      </span>
    )
  }
  const m = c.last_message
  if (!m) return <span className="truncate italic">{c.subject || t`No messages yet`}</span>
  return (
    <span className="flex min-w-0 items-center gap-1">
      {m.author_type === "bot" && <BotIcon className="size-3.5 shrink-0" aria-label={t`Bot`} data-testid="row-bot" />}
      {m.author_type === "member" && !receipts && <ReplyIcon className="size-3.5 shrink-0" aria-label={t`Team reply`} />}
      {(m.author_type === "member" || m.author_type === "bot") && receipts && <Ticks at={m.created_at} readAt={c.last_read_by_contact_at} />}
      <span className="truncate">{m.text || c.subject}</span>
    </span>
  )
}

export function RowMenuItems({ c, Item, onDone }: { c: ConversationListItem; Item: typeof ContextMenuItem; onDone?: () => void }) {
  const actions = useListActions()
  return (
    <>
      <Item
        onClick={() => {
          actions.pin(c.id, !c.pinned_at)
          onDone?.()
        }}
        data-testid="row-pin"
      >
        {c.pinned_at ? <PinOffIcon /> : <PinIcon />}
        {c.pinned_at ? <Trans>Unpin</Trans> : <Trans>Pin to the top</Trans>}
      </Item>
      <Item
        onClick={() => {
          if (c.unread) actions.markRead(c.id)
          else actions.markUnread(c.id)
          onDone?.()
        }}
        data-testid="row-mark-unread"
      >
        {c.unread ? <MailOpenIcon /> : <MailIcon />}
        {c.unread ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
      </Item>
    </>
  )
}

function Row({
  c,
  current,
  cursor,
  live,
  onOpen,
}: {
  c: ConversationListItem
  current: boolean
  cursor: boolean
  live: boolean
  onOpen: () => void
}) {
  const { t, i18n } = useLingui()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const members = useMemberMap()
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  const rating = currentRating(c)
  return (
    <ContextMenu>
      <ContextMenuTrigger
        render={<button type="button" />}
        onClick={onOpen}
        className={cn(
          "group flex w-full items-center gap-3 ps-3 text-start outline-none transition-colors",
          current ? "bg-muted" : "hover:bg-muted/60",
          cursor && !current && "bg-muted/60",
          cursor && "shadow-[inset_3px_0_0_var(--brand)] rtl:shadow-[inset_-3px_0_0_var(--brand)]",
        )}
        aria-current={current || undefined}
        data-testid="list-row"
        data-conversation-id={c.id}
        data-unread={c.unread || undefined}
        data-pinned={c.pinned_at ? true : undefined}
      >
        <span className="relative shrink-0 py-2.5">
          <ContactAvatar id={c.contact.id} name={name} className="size-12" />
          {live && <span className="absolute end-0 bottom-3 size-3 rounded-full border-2 border-background bg-success" title={t`Online now`} data-testid="row-live" />}
        </span>
        <span className="flex min-w-0 flex-1 flex-col gap-0.5 border-b py-3 pe-3 group-last:border-transparent">
          <span className="flex min-w-0 items-baseline gap-2">
            <span className={cn("min-w-0 flex-1 truncate text-reading", c.unread ? "font-semibold" : "font-medium")}>{name}</span>
            {c.pinned_at && <PinIcon className="size-3.5 shrink-0 rotate-45 text-faint" aria-label={t`Pinned`} data-testid="row-pinned" />}
            <time className={cn("shrink-0 text-caption", c.unread ? "font-medium text-brand" : "text-faint")} dateTime={c.last_activity_at}>
              {formatShort(c.last_activity_at, i18n.locale)}
            </time>
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-small text-muted-foreground">
            {channel && <ChannelIcon kind={channel.kind} className="size-3.5 shrink-0 text-faint" />}
            {c.status === "snoozed" && <ClockIcon className="size-3.5 shrink-0 text-faint" aria-label={t`Snoozed`} />}
            <span className="flex min-w-0 flex-1">
              <Preview c={c} current={current} receipts={channel?.kind === "chat" || channel?.kind === "app"} />
            </span>
            {rating && <RatingMark rating={rating.rating} comment={rating.comment} />}
            {assignee && (
              <span title={t`Assigned to ${assignee.name || assignee.email}`} data-testid="row-assignee">
                <MemberAvatar name={assignee.name || assignee.email} className="size-[18px]" />
              </span>
            )}
            {c.unread && <span className="size-2.5 shrink-0 rounded-full bg-brand" aria-label={t`Unread`} data-testid="row-unread" />}
          </span>
        </span>
      </ContextMenuTrigger>
      <ContextMenuContent className="min-w-52" data-testid="row-menu">
        <RowMenuItems c={c} Item={ContextMenuItem} />
      </ContextMenuContent>
    </ContextMenu>
  )
}

function AwayBanner() {
  const { me } = useSession()
  const set = useSetAvailability()
  if (me.person.availability !== "away") return null
  return (
    <Alert role="status" className="mx-3 mb-2 flex items-center gap-2 rounded-xl border-border bg-card py-2 ps-3 pe-2 text-small text-muted-foreground" data-testid="away-banner">
      <ClockIcon className="size-3.5 shrink-0" />
      <span className="min-w-0 flex-1">
        <Trans>You are away. Live chat shows nobody available and notifications pause.</Trans>
      </span>
      <Button variant="outline" size="sm" className="shrink-0" onClick={() => set.mutate("auto")} disabled={set.isPending}>
        <Trans>I'm back</Trans>
      </Button>
    </Alert>
  )
}

function CleanupBanner({ onReview }: { onReview: (id: string) => void }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const [inboxId] = useInboxFilter()
  const counts = useCounts().data
  const spam = useConversations({ status: "open", spam: true }, !!counts?.spam)
  const bulk = useBulkUpdateConversations()
  const items = (spam.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  if (!counts?.spam || items.length === 0) return null
  const ids = items.slice(0, 100).map((c) => c.id)
  const closeAll = () =>
    bulk.mutate(
      { conversation_ids: ids, status: "closed" },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          toast(t`${done.length} closed`, () => bulk.mutate({ conversation_ids: done, status: "open" }, { onError: (e) => toast(errorText(e)) }))
        },
        onError: (e) => toast(errorText(e)),
      },
    )
  return (
    <Alert role="status" className="mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-xl border-dashed border-border bg-card py-2 ps-3 pe-2 text-small" data-testid="cleanup-banner">
      <span className="min-w-0 flex-1">
        <b className="font-medium">
          <Trans>Quick cleanup</Trans>
        </b>{" "}
        <span className="text-faint">
          <Trans>{items.length} marked as spam · no reply needed</Trans>
        </span>
      </span>
      <span className="flex gap-1">
        <Button variant="ghost" size="sm" onClick={() => onReview(items[0].id)}>
          <Trans>Review</Trans>
        </Button>
        <Button variant="outline" size="sm" onClick={closeAll} disabled={bulk.isPending} data-testid="cleanup-close-all">
          <Trans>Close all</Trans>
        </Button>
      </span>
    </Alert>
  )
}

function StartConversation({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [contact, setContact] = useState<Contact | null>(null)
  return (
    <>
      <Dialog
        open={open && !contact}
        onOpenChange={(o) => {
          if (!o) onOpenChange(false)
        }}
      >
        <DialogContent data-testid="start-conversation">
          <DialogHeader>
            <DialogTitle>
              <Trans>Who is it for?</Trans>
            </DialogTitle>
          </DialogHeader>
          {open && <ContactSearch exclude="" onPick={setContact} />}
        </DialogContent>
      </Dialog>
      {contact && (
        <NewConversationDialog
          contact={contact}
          open={open}
          onOpenChange={(o) => {
            if (!o) {
              setContact(null)
              onOpenChange(false)
            }
          }}
        />
      )}
    </>
  )
}

function ListHeader() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { membership, canManage } = useSession()
  const { openPalette, openShortcuts } = useShell()
  const [starting, setStarting] = useState(false)
  const phone = useIsPhone()
  return (
    <div className={cn("flex h-15 shrink-0 items-center gap-1 pe-1.5", phone ? "ps-2" : "ps-4")}>
      {phone && <UserMenu align="start" />}
      <span className="min-w-0 flex-1 truncate ps-1 text-title" data-testid="workspace-name">
        {membership.workspace.name}
      </span>
      {phone && <TeamMenu />}
      <Button variant="ghost" size="icon-sm" onClick={() => setStarting(true)} aria-label={t`New conversation`} title={t`New conversation`} data-testid="new-conversation">
        <MessageSquarePlusIcon />
      </Button>
      {phone && (
        <Button variant="ghost" size="icon-sm" render={<Link to="/settings" />} aria-label={t`Settings`} title={t`Settings (${mod},)`} data-testid="open-settings">
          <SettingsIcon />
        </Button>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" />} aria-label={t`Menu`} title={t`Menu`} data-testid="list-menu">
          <EllipsisVerticalIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-56">
          <DropdownMenuItem onClick={() => navigate("/contacts")} data-testid="open-contacts">
            <BookUserIcon />
            <Trans>Contacts</Trans>
            <DropdownMenuShortcut>{keyLabel(SHORTCUTS.contacts).join(" ")}</DropdownMenuShortcut>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => navigate("/settings")}>
            <SettingsIcon />
            <Trans>Settings</Trans>
            <DropdownMenuShortcut>{mod} ,</DropdownMenuShortcut>
          </DropdownMenuItem>
          {canManage && (
            <DropdownMenuItem onClick={() => navigate("/setup")}>
              <InboxIcon />
              <Trans>Add an inbox</Trans>
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={openPalette}>
            <CommandIcon />
            <Trans>Everything</Trans>
            <DropdownMenuShortcut>{mod} K</DropdownMenuShortcut>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={openShortcuts}>
            <KeyboardIcon />
            <Trans>Keyboard shortcuts</Trans>
            <DropdownMenuShortcut>?</DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <StartConversation open={starting} onOpenChange={setStarting} />
    </div>
  )
}

export function ChatList({ currentId, onOpen, active = true }: { currentId: string | null; onOpen: (id: string) => void; active?: boolean }) {
  const { t } = useLingui()
  const { query, focus } = useListSearch()
  const q = useDebounced(query.trim(), 250)
  const [inboxId] = useInboxFilter()
  const [chip, setChipState] = useState<Chip>(remembered)
  const [cursorId, setCursorId] = useState<string | null>(null)
  const search = useRef<HTMLInputElement>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)

  const filters = { ...chipFilters[chip], inbox_id: inboxId || undefined, q: q || undefined }
  const list = useConversations(filters)
  const pinnedList = useConversations({ ...filters, pinned: true })
  const loaded = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const pinnedLoaded = useMemo(() => pinnedList.data?.pages.flatMap((p) => p.items) ?? [], [pinnedList.data])
  const keep = (c: ConversationListItem) => chip !== "unread" || c.unread
  const pinnedRows = [...pinnedLoaded, ...loaded.filter((c) => c.pinned_at && !pinnedLoaded.some((x) => x.id === c.id))]
    .filter((c) => c.pinned_at && keep(c))
    .sort((a, b) => (a.last_activity_at < b.last_activity_at ? 1 : -1))
  const mainRows = loaded.filter((c) => !c.pinned_at && keep(c))
  const rows = [...pinnedRows, ...mainRows]
  const live = useLiveIds(rows.filter((c) => c.status !== "closed"))

  useEffect(() => {
    if (focus <= focused) return
    focused = focus
    search.current?.focus()
  }, [focus])

  useEffect(() => {
    const el = sentinel.current
    const root = scroller.current
    if (!el || !root || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), { root, rootMargin: "400px" })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (nearEnd && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [nearEnd, hasNextPage, isFetchingNextPage, fetchNextPage, loaded.length])

  const setChip = (c: Chip) => {
    remembered = c
    setChipState(c)
    setCursorId(null)
    scroller.current?.scrollTo({ top: 0 })
  }

  const reveal = (id: string) =>
    requestAnimationFrame(() => scroller.current?.querySelector(`[data-conversation-id="${id}"]`)?.scrollIntoView({ block: "nearest" }))
  const index = (id: string | null) => (id ? rows.findIndex((c) => c.id === id) : -1)
  const moveCursor = (d: 1 | -1) => {
    if (rows.length === 0) return
    const from = index(cursorId) >= 0 ? index(cursorId) : index(currentId)
    const next = rows[Math.min(rows.length - 1, Math.max(0, from < 0 ? (d === 1 ? 0 : rows.length - 1) : from + d))]
    setCursorId(next.id)
    reveal(next.id)
  }
  const openCursor = () => {
    const id = cursorId && index(cursorId) >= 0 ? cursorId : rows[0]?.id
    if (!id) return
    setCursorId(null)
    onOpen(id)
  }
  const step = (d: 1 | -1) => {
    if (rows.length === 0) return
    const i = index(currentId)
    const next = rows[Math.min(rows.length - 1, Math.max(0, i < 0 ? 0 : i + d))]
    setCursorId(null)
    onOpen(next.id)
    reveal(next.id)
  }
  useHotkeys({
    ArrowDown: () => moveCursor(1),
    ArrowUp: () => moveCursor(-1),
    Enter: () => cursorId && openCursor(),
    [SHORTCUTS.next]: () => step(1),
    [SHORTCUTS.previous]: () => step(-1),
  }, active)

  const labels: Record<Chip, string> = {
    all: t`All`,
    unread: t`Unread`,
    mine: t`Mine`,
    unassigned: t`Unassigned`,
    snoozed: t`Snoozed`,
    done: t`Done`,
  }
  const empty: Record<Chip, React.ReactNode> = {
    all: <Trans>No conversations yet.</Trans>,
    unread: <Trans>You have read everything.</Trans>,
    mine: <Trans>Nothing is assigned to you.</Trans>,
    unassigned: <Trans>Every open conversation has someone.</Trans>,
    snoozed: <Trans>Nothing is snoozed.</Trans>,
    done: <Trans>Nothing is done yet.</Trans>,
  }

  return (
    <nav aria-label={t`Conversations`} className="flex h-full min-h-0 flex-col bg-background" data-testid="chat-list">
      <ListHeader />
      <div className="flex shrink-0 items-center gap-1 px-3 pb-2">
        <InputGroup className="h-9 flex-1 rounded-xl border-transparent bg-muted shadow-none focus-within:border-border focus-within:bg-card dark:bg-muted">
          <InputGroupAddon>
            <SearchIcon className="size-[15px] text-faint" />
          </InputGroupAddon>
          <InputGroupInput
            ref={search}
            name="list-search"
            value={query}
            onChange={(e) => {
              setListQuery(e.target.value)
              setCursorId(null)
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                e.preventDefault()
                moveCursor(e.key === "ArrowDown" ? 1 : -1)
              } else if (e.key === "Enter") {
                e.preventDefault()
                openCursor()
              } else if (e.key === "Escape") {
                if (query) setListQuery("")
                else e.currentTarget.blur()
              }
            }}
            placeholder={t`Search`}
            aria-label={t`Search conversations`}
            data-testid="list-search"
          />
          <InputGroupAddon align="inline-end">
            {query ? (
              <InputGroupButton size="icon-xs" onClick={() => setListQuery("")} aria-label={t`Clear the search`}>
                <XIcon />
              </InputGroupButton>
            ) : (
              <Kbd className="phone:hidden">/</Kbd>
            )}
          </InputGroupAddon>
        </InputGroup>
        <InboxPicker className="max-w-36" />
      </div>
      <div className="flex shrink-0 flex-wrap gap-1.5 px-3 pb-2.5" role="toolbar" aria-label={t`Filter`} data-testid="chips">
        {CHIPS.map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={chip === k}
            onClick={() => setChip(k)}
            className={cn(
              "h-7 shrink-0 rounded-full px-3 text-small font-medium transition-colors",
              chip === k ? "bg-brand-wash text-brand" : "bg-muted text-muted-foreground hover:text-foreground",
            )}
            data-testid={`chip-${k}`}
          >
            {labels[k]}
          </button>
        ))}
      </div>
      <AwayBanner />
      <CleanupBanner onReview={onOpen} />
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 py-1.5">
                <Skeleton className="size-12 rounded-full" />
                <div className="flex flex-1 flex-col gap-2">
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-3 w-3/4" />
                </div>
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <p className="px-6 py-12 text-center text-body text-faint" data-testid="list-empty">
            {q ? <Trans>Nothing matches your search here.</Trans> : empty[chip]}
          </p>
        ) : (
          <>
            {pinnedRows.length > 0 && (
              <div className="flex items-center gap-1.5 px-4 pt-1 pb-0.5 text-caption font-medium text-faint" data-testid="pinned-heading">
                <PinIcon className="size-3 rotate-45" />
                <Trans>Pinned</Trans>
              </div>
            )}
            {pinnedRows.map((c) => (
              <Row key={c.id} c={c} current={c.id === currentId} cursor={c.id === cursorId} live={live.has(c.id)} onOpen={() => onOpen(c.id)} />
            ))}
            {pinnedRows.length > 0 && mainRows.length > 0 && <div className="mt-1 border-t" role="separator" />}
            {mainRows.map((c) => (
              <Row key={c.id} c={c} current={c.id === currentId} cursor={c.id === cursorId} live={live.has(c.id)} onOpen={() => onOpen(c.id)} />
            ))}
          </>
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="size-12 rounded-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={list.error} className="px-4 py-2" />
      </div>
    </nav>
  )
}

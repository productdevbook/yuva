import { Trans, useLingui } from "@lingui/react/macro"
import {
  CheckIcon,
  ChevronDownIcon,
  ClockIcon,
  PaperclipIcon,
  SearchIcon,
  SquareCheckBigIcon,
  StarIcon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { ContactAvatar, ErrorLine, Kbd, toast } from "@/components/common"
import { SearchError, isSearchError } from "@/components/common/SearchError"
import { MAIL_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatWhen } from "@/components/common/SendLater"
import { formatShort, useErrorText, usePreviewText } from "@/components/common/text"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { TrashStrip, useTrashTools } from "@/features/conversation/TrashBulk"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Skeleton } from "@/components/ui/skeleton"
import { useDraftText } from "@/features/conversation/actions"
import { useSnoozeTimes } from "@/features/conversation/HeaderActions"
import { useListActions } from "@/features/inbox/memberState"
import { useListCursor } from "@/features/inbox/listCursor"
import { useBulkUpdateConversations, useConversations } from "@/features/inbox/queries"
import { mailFilters, type Folder } from "@/features/mail/folders"
import { OperatorHelp, OperatorList, useOperatorSuggest } from "@/features/mail/SearchOperators"
import { useStar } from "@/features/mail/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { ConversationListItem, ConversationStatus } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function useDebounced(value: string, ms: number) {
  const [v, setV] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return v
}

function sender(c: ConversationListItem) {
  return c.contact.name || c.contact.email || ""
}

function Row({
  c,
  folder,
  current,
  cursor,
  selecting,
  checked,
  showInbox,
  scheduledAt,
  onOpen,
  onStar,
}: {
  c: ConversationListItem
  folder: Folder
  current: boolean
  cursor: boolean
  selecting: boolean
  checked: boolean
  showInbox: boolean
  scheduledAt?: string
  onOpen: (range: boolean) => void
  onStar: () => void
}) {
  const { t, i18n } = useLingui()
  const previewText = usePreviewText()
  const draft = useDraftText(c.id)
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const who = sender(c) || t`Unknown sender`
  const subject = c.subject.trim() || t`(no subject)`
  const line = c.last_message ? previewText(c.last_message) : ""
  const starred = !!c.starred
  return (
    <div
      className={cn(
        "group relative mx-2 flex w-[calc(100%-1rem)] items-start rounded-xl transition-colors",
        current ? "bg-card shadow-[0_1px_2px_rgb(0_0_0/0.05),0_0_0_1px_var(--border)]" : "hover:bg-muted/60",
        cursor && !current && "bg-muted/60",
        (current || cursor) &&
          "before:absolute before:inset-y-3 before:-start-2 before:w-[3px] before:rounded-e-[3px] before:bg-brand",
        checked && "bg-brand/7 shadow-[0_0_0_1px_color-mix(in_srgb,var(--brand)_30%,var(--border))]"
      )}
      data-testid="mail-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
      data-checked={checked || undefined}
    >
      <button
        type="button"
        onClick={(e) => onOpen(e.shiftKey)}
        className="flex min-w-0 flex-1 items-start gap-3 rounded-xl px-2.5 py-2.5 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-current={current || undefined}
        aria-pressed={selecting ? checked : undefined}
        data-testid="mail-open"
      >
        {selecting && (
          <span
            aria-hidden
            className={cn(
              "mt-2.5 grid size-[18px] shrink-0 place-items-center rounded-[5px] border-[1.5px]",
              checked ? "border-foreground bg-foreground text-background" : "border-input bg-card"
            )}
            data-testid="row-check"
          >
            {checked && <CheckIcon className="size-3" strokeWidth={3} />}
          </span>
        )}
        <ContactAvatar id={c.contact.id} name={who} className="size-[38px] font-semibold" />
        <span className="flex min-w-0 flex-1 flex-col gap-px">
          <span className="flex min-w-0 items-center gap-1.5">
            {c.unread && (
              <span className="size-2 shrink-0 rounded-full bg-brand" aria-label={t`Unread`} data-testid="row-unread" />
            )}
            <span
              className={cn(
                "min-w-0 truncate text-body",
                c.unread ? "font-semibold text-foreground" : "font-medium text-foreground/80"
              )}
              data-testid="mail-sender"
            >
              {folder === "sent" || folder === "scheduled" ? t`To: ${who}` : who}
            </span>
          </span>
          <span
            className={cn(
              "min-w-0 truncate text-body",
              c.unread ? "font-semibold text-foreground" : "text-foreground/85"
            )}
            data-testid="mail-subject"
          >
            {subject}
          </span>
          <span className="min-w-0 truncate text-small text-faint">
            {draft && !current ? (
              <>
                <span className="font-semibold text-destructive">
                  <Trans>Draft:</Trans>
                </span>{" "}
                {draft}
              </>
            ) : (
              <span className={cn(c.last_message?.redacted && "italic")} data-testid="mail-line">
                {line}
              </span>
            )}
          </span>
          {showInbox && inbox && (
            <span className="mt-1 flex min-w-0">
              <span className="max-w-full truncate rounded-full bg-muted px-2 text-[11px] leading-[18px] text-muted-foreground">
                {inbox.name}
              </span>
            </span>
          )}
        </span>
        <span className="flex shrink-0 flex-col items-end gap-1.5 pt-px">
          {scheduledAt ? (
            <time
              className="flex items-center gap-1 font-mono text-[12px] whitespace-nowrap text-warning"
              dateTime={scheduledAt}
              data-testid="mail-scheduled-at"
            >
              <ClockIcon className="size-3" />
              {formatWhen(scheduledAt, i18n.locale)}
            </time>
          ) : (
            <time
              className={cn(
                "font-mono text-[12px] whitespace-nowrap tabular-nums",
                c.unread ? "font-medium text-foreground" : "text-faint"
              )}
              dateTime={c.last_activity_at}
            >
              {formatShort(c.last_activity_at, i18n.locale)}
            </time>
          )}
          <span className="flex h-6 items-center gap-1.5 pe-7 text-faint">
            {c.status === "snoozed" && folder !== "snoozed" && (
              <ClockIcon className="size-3.5" aria-label={t`Snoozed`} />
            )}
            {c.has_attachments && (
              <PaperclipIcon className="size-3.5" aria-label={t`Has attachments`} data-testid="mail-attachment" />
            )}
          </span>
        </span>
      </button>
      <button
        type="button"
        onClick={onStar}
        className={cn(
          "absolute end-2 bottom-2 grid size-7 shrink-0 place-items-center rounded-md outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          starred ? "text-warning" : "text-faint"
        )}
        aria-pressed={starred}
        aria-label={starred ? t`Remove the star` : t`Star`}
        title={starred ? t`Remove the star (S)` : t`Star (S)`}
        data-testid="mail-star"
      >
        <StarIcon className={cn("size-4", starred && "fill-current")} />
      </button>
    </div>
  )
}

function statusGroups(prev: ConversationListItem[]) {
  const groups = new Map<string, { status: ConversationStatus; until?: string; ids: string[] }>()
  for (const c of prev) {
    const until =
      c.status === "snoozed" && c.snooze_until && Date.parse(c.snooze_until) > Date.now() ? c.snooze_until : undefined
    const status: ConversationStatus = c.status === "snoozed" && !until ? "open" : c.status
    const key = `${status}|${until ?? ""}`
    const g = groups.get(key) ?? { status, until, ids: [] }
    g.ids.push(c.id)
    groups.set(key, g)
  }
  return groups.values()
}

export function MailList({
  box,
  folder,
  currentId,
  onOpen,
  active,
  head,
}: {
  box: string
  folder: Folder
  currentId: string | null
  onOpen: (id: string) => void
  active: boolean
  head?: React.ReactNode
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const [query, setQuery] = useState("")
  const q = useDebounced(query.trim(), 250)
  const scroller = useRef<HTMLDivElement>(null)
  const sentinel = useRef<HTMLDivElement>(null)
  const [nearEnd, setNearEnd] = useState(false)
  const [selecting, setSelecting] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const anchor = useRef<string | null>(null)
  const list = useConversations(mailFilters(box, folder, q))
  const rows = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const { cursorId, setCursorId, moveCursor, openCursor } = useListCursor(rows, currentId, onOpen, active, scroller)
  const star = useStar()
  const bulk = useBulkUpdateConversations()
  const memberActions = useListActions()
  const times = useSnoozeTimes()
  const failed = (e: unknown) => toast(errorText(e))

  const [scope, setScope] = useState(`${box}|${folder}`)
  if (scope !== `${box}|${folder}`) {
    setScope(`${box}|${folder}`)
    setSelecting(false)
    setSelected(new Set())
    setCursorId(null)
    setQuery("")
  }
  useEffect(() => {
    scroller.current?.scrollTo({ top: 0 })
  }, [box, folder])

  useEffect(() => {
    const el = sentinel.current
    const root = scroller.current
    if (!el || !root || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
      root,
      rootMargin: "400px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [])
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (nearEnd && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [nearEnd, hasNextPage, isFetchingNextPage, fetchNextPage, rows.length])

  const toggle = (id: string, range = false) => {
    setSelecting(true)
    const ids = rows.map((c) => c.id)
    const from = anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    setSelected((s) => {
      const next = new Set(s)
      if (range && from >= 0 && to >= 0) {
        for (let i = Math.min(from, to); i <= Math.max(from, to); i++) next.add(ids[i])
      } else if (!next.delete(id)) next.add(id)
      return next
    })
    anchor.current = id
  }
  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
    anchor.current = null
  }
  const chosenRows = rows.filter((c) => selected.has(c.id))
  const allPicked = rows.length > 0 && chosenRows.length === rows.length
  const chosen = chosenRows.map((c) => c.id)
  const picked = chosen.length
  const none = picked === 0
  const inTrash = folder === "trash"
  const trashTools = useTrashTools()

  const restoreStatus = (prev: ConversationListItem[]) => {
    for (const g of statusGroups(prev))
      bulk.mutate({ conversation_ids: g.ids, status: g.status, snooze_until: g.until }, { onError: failed })
  }
  const setStatus = (status: "open" | "closed", done: (n: number) => string) => {
    const prev = chosenRows.filter((c) => c.status !== status).slice(0, 100)
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), status },
      {
        onSuccess: (r) => {
          const ids = new Set(r.updated.map((c) => c.id))
          if (ids.size > 0) toast(done(ids.size), () => restoreStatus(prev.filter((c) => ids.has(c.id))))
          const missed = r.failed.length
          if (missed > 0) toast(t`${missed} could not be changed`)
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const archiveChosen = () => setStatus("closed", (n) => t`${n} archived`)
  const inboxChosen = () => setStatus("open", (n) => t`${n} moved to the Inbox`)
  const snoozeChosen = (at: Date, when: string) => {
    const prev = chosenRows.slice(0, 100)
    if (prev.length === 0) return
    bulk.mutate(
      { conversation_ids: prev.map((c) => c.id), status: "snoozed", snooze_until: at.toISOString() },
      {
        onSuccess: (r) => {
          const ids = new Set(r.updated.map((c) => c.id))
          const n = ids.size
          if (n > 0) toast(t`${n} snoozed, back ${when}`, () => restoreStatus(prev.filter((c) => ids.has(c.id))))
          stopSelecting()
        },
        onError: failed,
      }
    )
  }
  const anyUnread = chosenRows.some((c) => c.unread)
  const readChosen = () => {
    const ids = chosenRows.filter((c) => c.unread === anyUnread).map((c) => c.id)
    const n = ids.length
    if (n === 0) return
    const apply = (read: boolean) => {
      for (const id of ids) {
        if (read) memberActions.markRead(id)
        else memberActions.markUnread(id)
      }
    }
    apply(anyUnread)
    toast(anyUnread ? t`${n} marked as read` : t`${n} marked as unread`, () => apply(!anyUnread))
    stopSelecting()
  }
  const allStarred = chosenRows.length > 0 && chosenRows.every((c) => c.starred)
  const starChosen = () => {
    const ids = chosenRows.filter((c) => !!c.starred === allStarred).map((c) => c.id)
    star(ids, !allStarred)
    stopSelecting()
  }
  const setTrashed = async (ids: string[], trashed: boolean) => {
    const done: string[] = []
    try {
      for (let at = 0; at < ids.length; at += 100) {
        const r = await bulk.mutateAsync({ conversation_ids: ids.slice(at, at + 100), trashed })
        done.push(...r.updated.map((c) => c.id))
      }
    } catch (e) {
      failed(e)
    }
    return done
  }
  const trashChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    confirm({
      title: <Trans>Move {ids.length} to the Trash?</Trans>,
      description: (
        <Trans>
          You can restore them from the Trash for 30 days, then they are deleted for good with their messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        void setTrashed(ids, true).then((done) => {
          if (done.length === 0) return
          toast(t`${done.length} moved to the Trash`, () => void setTrashed(done, false))
          stopSelecting()
        }),
    })
  }
  const restoreChosen = () => {
    const ids = [...chosen]
    if (ids.length === 0) return
    void setTrashed(ids, false).then((done) => {
      if (done.length === 0) return
      toast(t`${done.length} restored`, () => void setTrashed(done, true))
      stopSelecting()
    })
  }

  const deleteChosen = () =>
    trashTools.deleteForever([...chosen], (id) => chosenRows.find((c) => c.id === id)?.subject || id, stopSelecting)

  const listKeys: Record<string, () => void> = {
    [MAIL_KEYS.select]: () => {
      const id = cursorId ?? currentId
      if (id) toggle(id)
      else setSelecting(true)
    },
  }
  const selectionKeys: Record<string, () => void> =
    selecting && !none
      ? inTrash
        ? { [MAIL_KEYS.restore]: restoreChosen, [MAIL_KEYS.deleteForever]: deleteChosen }
        : {
            [MAIL_KEYS.archive]: folder === "archive" ? inboxChosen : archiveChosen,
            [MAIL_KEYS.star]: starChosen,
            [MAIL_KEYS.unread]: readChosen,
            ...(canManage ? { [MAIL_KEYS.trash]: trashChosen } : {}),
          }
      : {}
  useHotkeys({ ...listKeys, ...selectionKeys, ...(selecting ? { Escape: stopSelecting } : {}) }, active)

  const searchInput = useRef<HTMLInputElement>(null)
  const suggest = useOperatorSuggest(query, setQuery, searchInput)
  const searchBox = (
    <InputGroup className="h-8 min-w-0 flex-1 rounded-[9px] border-input bg-card shadow-none focus-within:border-faint">
      <InputGroupAddon>
        <SearchIcon className="size-[15px] text-faint" />
      </InputGroupAddon>
      <InputGroupInput
        ref={searchInput}
        name="mail-search"
        value={query}
        {...suggest.inputProps}
        onChange={(e) => {
          setQuery(e.target.value)
          suggest.typed(e)
          setCursorId(null)
        }}
        onKeyDown={(e) => {
          if (suggest.onKeyDown(e)) return
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault()
            moveCursor(e.key === "ArrowDown" ? 1 : -1)
          } else if (e.key === "Enter") {
            e.preventDefault()
            openCursor()
          } else if (e.key === "Escape") {
            if (query) setQuery("")
            else e.currentTarget.blur()
          }
        }}
        placeholder={t`Search mail`}
        aria-label={t`Search mail`}
        data-testid="mail-search"
      />
      <InputGroupAddon align="inline-end">
        <OperatorHelp />
        {query ? (
          <InputGroupButton size="icon-xs" onClick={() => setQuery("")} aria-label={t`Clear the search`}>
            <XIcon />
          </InputGroupButton>
        ) : (
          <Kbd className="phone:hidden">{SHORTCUTS.search}</Kbd>
        )}
      </InputGroupAddon>
    </InputGroup>
  )

  const bb =
    "h-[34px] shrink-0 rounded-lg px-2.5 font-normal text-background hover:bg-background/15 hover:text-background aria-expanded:bg-background/15 aria-expanded:text-background disabled:opacity-40"
  const selection = (
    <div
      className="relative mx-2.5 mb-2.5 flex shrink-0 flex-wrap items-center gap-0.5 rounded-xl bg-foreground p-1.5 text-background shadow-[0_12px_32px_rgb(0_0_0/0.18)]"
      role="toolbar"
      aria-label={t`Selection`}
      data-testid="selection-bar"
    >
      <Button
        variant="ghost"
        className={cn(bb, "w-[34px] px-0")}
        onClick={stopSelecting}
        aria-label={t`Cancel`}
        title={t`Clear the selection (Esc)`}
      >
        <XIcon />
      </Button>
      <span className="me-auto shrink-0 pe-1.5 text-small font-semibold whitespace-nowrap" aria-live="polite">
        {none ? <Trans>Pick mails</Trans> : <Trans>Selected: {picked}</Trans>}
      </span>
      <Button
        variant="ghost"
        className={cn(bb, "shrink-0")}
        disabled={rows.length === 0}
        onClick={() => setSelected(allPicked ? new Set() : new Set(rows.map((c) => c.id)))}
        aria-pressed={allPicked}
        data-testid="select-all"
      >
        <Trans>Select all</Trans>
      </Button>
      {inTrash ? (
        <>
          <Button
            variant="ghost"
            className={bb}
            disabled={none || bulk.isPending}
            onClick={restoreChosen}
            title={t`Restore (Z)`}
            data-testid="restore-selected"
          >
            <Trans>Restore</Trans>
          </Button>
          <Button
            variant="ghost"
            className={bb}
            disabled={none || trashTools.pending}
            onClick={deleteChosen}
            title={t`Delete for good (${MAIL_KEYS.deleteForever})`}
            data-testid="delete-forever-selected"
          >
            <Trans>Delete forever</Trans>
          </Button>
        </>
      ) : (
        <>
          {folder === "archive" ? (
            <Button
              variant="ghost"
              className={bb}
              disabled={none || bulk.isPending}
              onClick={inboxChosen}
              title={t`Move to the Inbox (E)`}
              data-testid="inbox-selected"
            >
              <Trans>Move to the Inbox</Trans>
            </Button>
          ) : (
            <Button
              variant="ghost"
              className={bb}
              disabled={none || bulk.isPending}
              onClick={archiveChosen}
              title={t`Archive (E)`}
              data-testid="archive-selected"
            >
              <Trans>Archive</Trans>
            </Button>
          )}
          <Button
            variant="ghost"
            className={bb}
            disabled={none}
            onClick={starChosen}
            title={t`Star or unstar (S)`}
            data-testid="star-selected"
          >
            {allStarred ? <Trans>Remove the star</Trans> : <Trans>Star</Trans>}
          </Button>
          <Button
            variant="ghost"
            className={bb}
            disabled={none}
            onClick={readChosen}
            title={t`Mark as read or unread (U)`}
            data-testid="read-selected"
          >
            {anyUnread || none ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="ghost" className={bb} disabled={none || bulk.isPending} />}
              data-testid="snooze-selected"
            >
              <Trans>Snooze</Trans>
              <ChevronDownIcon className="opacity-70" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" side="top" className="min-w-56">
              <DropdownMenuGroup>
                <DropdownMenuLabel>
                  <Trans>When should they come back?</Trans>
                </DropdownMenuLabel>
                {times.map((o) => (
                  <DropdownMenuItem key={o.label} onClick={() => snoozeChosen(o.at, o.when)}>
                    <span className="flex-1">{o.label}</span>
                    {o.detail && <span className="text-caption text-faint">{o.detail}</span>}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          {canManage && (
            <Button
              variant="ghost"
              className={bb}
              disabled={none || bulk.isPending}
              onClick={trashChosen}
              title={t`Move to the Trash (#)`}
              data-testid="trash-selected"
            >
              <Trans>Trash</Trans>
            </Button>
          )}
        </>
      )}
    </div>
  )

  const empty: Record<Folder, React.ReactNode> = {
    inbox: <Trans>Your inbox is empty.</Trans>,
    starred: <Trans>Star a mail to keep it here.</Trans>,
    snoozed: <Trans>Nothing is snoozed.</Trans>,
    sent: <Trans>You have not sent any mail yet.</Trans>,
    drafts: <Trans>You have no drafts. Mail you start writing is kept here.</Trans>,
    scheduled: <Trans>No mail is scheduled. Pick Send later to send mail at a time you choose.</Trans>,
    archive: <Trans>Nothing is archived yet.</Trans>,
    trash: <Trans>The Trash is empty.</Trans>,
  }

  return (
    <nav aria-label={t`Mail`} className="flex h-full min-h-0 flex-col bg-surface" data-testid="mail-list">
      {head}
      <div className="relative flex shrink-0 flex-wrap items-center gap-1.5 px-3 pt-1 pb-2">
        {searchBox}
        <Button
          variant="outline"
          className={cn("h-8 w-8 rounded-[9px] border-input bg-card px-0 font-normal", selecting && "bg-muted")}
          onClick={() => (selecting ? stopSelecting() : setSelecting(true))}
          aria-pressed={selecting}
          aria-label={t`Select mails`}
          title={t`Select mails (X)`}
          data-testid="select-mails"
        >
          <SquareCheckBigIcon />
        </Button>
        <OperatorList s={suggest} />
        <SearchError error={list.error} className="w-full" />
      </div>
      {inTrash && <TrashStrip onEmpty={(n) => trashTools.emptyTrash(n)} busy={trashTools.pending} />}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto pb-3" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1.5 px-4 py-2">
            {[0, 1, 2, 3, 4, 5, 6].map((i) => (
              <div key={i} className="flex items-start gap-3 py-2">
                <Skeleton className="size-9 rounded-full" />
                <div className="flex flex-1 flex-col gap-1.5">
                  <Skeleton className="h-3 w-[45%]" />
                  <Skeleton className="h-3 w-4/5" />
                  <Skeleton className="h-3 w-[95%]" />
                </div>
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <div className="flex flex-col items-start gap-1.5 px-6 py-10" data-testid="list-empty">
            <p className="text-body text-muted-foreground">
              {q ? <Trans>No mail matches your search.</Trans> : empty[folder]}
            </p>
          </div>
        ) : (
          rows.map((c) => (
            <Row
              key={c.id}
              c={c}
              folder={folder}
              current={c.id === currentId}
              cursor={c.id === cursorId}
              selecting={selecting}
              checked={selected.has(c.id)}
              showInbox={box === "all"}
              scheduledAt={folder === "scheduled" ? c.scheduled_at : undefined}
              onOpen={(range) => (selecting ? toggle(c.id, range) : onOpen(c.id))}
              onStar={() => star([c.id], !c.starred)}
            />
          ))
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex items-center gap-3 px-5 py-2">
            <Skeleton className="size-9 rounded-full" />
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={isSearchError(list.error) ? null : list.error} className="px-4 py-2" />
      </div>
      {selecting && selection}
      {confirmDialog}
      {trashTools.dialogs}
    </nav>
  )
}

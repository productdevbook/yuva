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

import { ErrorLine, Kbd, toast } from "@/components/common"
import { SearchError, isSearchError } from "@/components/common/SearchError"
import { MAIL_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatWhen } from "@/components/common/SendLater"
import { formatShort, useErrorText, usePreviewText } from "@/components/common/text"
import { useConfirm } from "@/components/common/ConfirmDialog"
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
import { look } from "@/features/inbox/PageFrame"
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
  onOpen: () => void
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
        "group relative flex w-full items-start border-b transition-colors",
        current ? "bg-surface" : "hover:bg-surface",
        cursor && !current && "bg-surface",
        (current || cursor) && "shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]"
      )}
      data-testid="mail-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
      data-checked={checked || undefined}
    >
      <button
        type="button"
        onClick={onStar}
        className={cn(
          "mt-2.5 ms-1.5 grid size-8 shrink-0 place-items-center rounded-md outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          starred ? "text-warning" : "text-faint"
        )}
        aria-pressed={starred}
        aria-label={starred ? t`Remove the star` : t`Star`}
        title={starred ? t`Remove the star (S)` : t`Star (S)`}
        data-testid="mail-star"
      >
        <StarIcon className={cn("size-4", starred && "fill-current")} />
      </button>
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-start gap-2.5 py-3 ps-1 pe-3.5 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        aria-current={current || undefined}
        aria-pressed={selecting ? checked : undefined}
        data-testid="mail-open"
      >
        {selecting && (
          <span
            aria-hidden
            className={cn(
              "mt-0.5 grid size-5 shrink-0 place-items-center rounded-[6px] border",
              checked ? "border-brand bg-brand text-white" : "border-input bg-background"
            )}
            data-testid="row-check"
          >
            {checked && <CheckIcon className="size-3.5" strokeWidth={3} />}
          </span>
        )}
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex min-w-0 items-baseline gap-2">
            <span
              className={cn("min-w-0 flex-1 truncate text-body", c.unread ? "font-semibold" : "font-medium")}
              data-testid="mail-sender"
            >
              {folder === "sent" || folder === "scheduled" ? t`To: ${who}` : who}
            </span>
            {c.has_attachments && (
              <PaperclipIcon
                className="size-3.5 shrink-0 self-center text-faint"
                aria-label={t`Has attachments`}
                data-testid="mail-attachment"
              />
            )}
            {c.status === "snoozed" && folder !== "snoozed" && (
              <ClockIcon className="size-3.5 shrink-0 self-center text-faint" aria-label={t`Snoozed`} />
            )}
            {c.unread && (
              <span
                className="size-2 shrink-0 self-center rounded-full bg-brand"
                aria-label={t`Unread`}
                data-testid="row-unread"
              />
            )}
            {scheduledAt ? (
              <time
                className="flex shrink-0 items-center gap-1 text-caption text-warning"
                dateTime={scheduledAt}
                data-testid="mail-scheduled-at"
              >
                <ClockIcon className="size-3" />
                {formatWhen(scheduledAt, i18n.locale)}
              </time>
            ) : (
              <time
                className={cn("shrink-0 text-caption", c.unread ? "font-semibold text-foreground" : "text-faint")}
                dateTime={c.last_activity_at}
              >
                {formatShort(c.last_activity_at, i18n.locale)}
              </time>
            )}
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
          <span className="flex min-w-0 items-center gap-1.5 text-small text-muted-foreground">
            {showInbox && inbox && (
              <span className="shrink-0 rounded border px-1 text-caption text-faint">{inbox.name}</span>
            )}
            {draft && !current ? (
              <span className="min-w-0 truncate">
                <span className="font-medium text-brand">
                  <Trans>Draft:</Trans>
                </span>{" "}
                {draft}
              </span>
            ) : (
              <span className={cn("min-w-0 truncate", c.last_message?.redacted && "italic")} data-testid="mail-line">
                {line}
              </span>
            )}
          </span>
        </span>
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
}: {
  box: string
  folder: Folder
  currentId: string | null
  onOpen: (id: string) => void
  active: boolean
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

  const toggle = (id: string) => {
    setSelecting(true)
    setSelected((s) => {
      const next = new Set(s)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }
  const stopSelecting = () => {
    setSelecting(false)
    setSelected(new Set())
  }
  const chosenRows = rows.filter((c) => selected.has(c.id))
  const chosen = chosenRows.map((c) => c.id)
  const picked = chosen.length
  const none = picked === 0
  const inTrash = folder === "trash"

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
        ? { [MAIL_KEYS.restore]: restoreChosen }
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
    <InputGroup className="h-[34px] min-w-0 flex-1 rounded-lg border-input bg-background shadow-none focus-within:border-faint">
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

  const selection = (
    <div
      className="flex shrink-0 flex-wrap items-center gap-1.5 px-3 pt-1 pb-2"
      role="toolbar"
      aria-label={t`Selection`}
      data-testid="selection-bar"
    >
      <span className="me-1 min-w-0 truncate text-small font-medium" aria-live="polite">
        {none ? <Trans>Pick mails</Trans> : <Trans>Selected: {picked}</Trans>}
      </span>
      {inTrash ? (
        <Button
          variant="outline"
          className={look.btnSm}
          disabled={none || bulk.isPending}
          onClick={restoreChosen}
          title={t`Restore (Z)`}
          data-testid="restore-selected"
        >
          <Trans>Restore</Trans>
        </Button>
      ) : (
        <>
          {folder === "archive" ? (
            <Button
              variant="outline"
              className={look.btnSm}
              disabled={none || bulk.isPending}
              onClick={inboxChosen}
              title={t`Move to the Inbox (E)`}
              data-testid="inbox-selected"
            >
              <Trans>Move to the Inbox</Trans>
            </Button>
          ) : (
            <Button
              variant="outline"
              className={look.btnSm}
              disabled={none || bulk.isPending}
              onClick={archiveChosen}
              title={t`Archive (E)`}
              data-testid="archive-selected"
            >
              <Trans>Archive</Trans>
            </Button>
          )}
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={none}
            onClick={starChosen}
            title={t`Star or unstar (S)`}
            data-testid="star-selected"
          >
            {allStarred ? <Trans>Remove the star</Trans> : <Trans>Star</Trans>}
          </Button>
          <Button
            variant="outline"
            className={look.btnSm}
            disabled={none}
            onClick={readChosen}
            title={t`Mark as read or unread (U)`}
            data-testid="read-selected"
          >
            {anyUnread || none ? <Trans>Mark as read</Trans> : <Trans>Mark as unread</Trans>}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={<Button variant="outline" className={look.btnSm} disabled={none || bulk.isPending} />}
              data-testid="snooze-selected"
            >
              <Trans>Snooze</Trans>
              <ChevronDownIcon className="text-faint" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="min-w-56">
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
              variant="outline"
              className={cn(look.btnSm, "text-destructive")}
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
      <Button
        variant="ghost"
        className="h-[30px] rounded-lg px-2"
        onClick={stopSelecting}
        title={t`Clear the selection (Esc)`}
      >
        <Trans>Cancel</Trans>
      </Button>
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
    <nav aria-label={t`Mail`} className="flex h-full min-h-0 flex-col bg-background" data-testid="mail-list">
      <div className="relative flex shrink-0 flex-wrap items-center gap-1.5 px-3 pt-2.5 pb-1.5">
        {searchBox}
        <Button
          variant="outline"
          className={cn(look.btn, "w-[34px] px-0", selecting && "bg-muted")}
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
      {selecting && selection}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto border-t" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex flex-col gap-2 py-2">
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-3 w-3/4" />
              </div>
            ))}
          </div>
        ) : rows.length === 0 && !hasNextPage ? (
          <div className="flex flex-col items-center gap-3 px-4 py-10 text-center" data-testid="list-empty">
            <p className="text-body text-faint">{q ? <Trans>No mail matches your search.</Trans> : empty[folder]}</p>
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
              onOpen={() => (selecting ? toggle(c.id) : onOpen(c.id))}
              onStar={() => star([c.id], !c.starred)}
            />
          ))
        )}
        <div ref={sentinel} className="h-px" />
        {isFetchingNextPage && (
          <div className="flex flex-col gap-2 px-3 py-2">
            <Skeleton className="h-4 w-1/2" />
          </div>
        )}
        <ErrorLine error={isSearchError(list.error) ? null : list.error} className="px-4 py-2" />
      </div>
      {confirmDialog}
    </nav>
  )
}

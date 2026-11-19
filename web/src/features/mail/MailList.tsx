import { Trans, useLingui } from "@lingui/react/macro"
import {
  ChevronDownIcon,
  ClockIcon,
  MailIcon,
  MailOpenIcon,
  PaperclipIcon,
  SearchIcon,
  StarIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { ErrorLine, FlatTag, FlatTime, Kbd, toast } from "@/components/common"
import {
  boxRowClass,
  BulkBar,
  bulkButton,
  FilterRow,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  RowCheckbox,
} from "@/components/common/ListBox"
import { SearchError, isSearchError } from "@/components/common/SearchError"
import { MAIL_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatWhen } from "@/components/common/SendLater"
import { formatShort, useErrorText, usePreviewText } from "@/components/common/text"
import { TrashStrip, useTrashTools } from "@/features/conversation/TrashBulk"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
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

function MailRow({
  c,
  folder,
  current,
  cursor,
  checked,
  showInbox,
  scheduledAt,
  onOpen,
  onToggle,
  onStar,
}: {
  c: ConversationListItem
  folder: Folder
  current: boolean
  cursor: boolean
  checked: boolean
  showInbox: boolean
  scheduledAt?: string
  onOpen: () => void
  onToggle: (range: boolean) => void
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
  const name = folder === "sent" || folder === "scheduled" ? t`To: ${who}` : who
  return (
    <div
      data-row
      className={boxRowClass({ current, cursor, checked })}
      data-testid="mail-row"
      data-conversation-id={c.id}
      data-unread={c.unread || undefined}
      data-checked={checked || undefined}
    >
      <RowCheckbox checked={checked} name={who} onToggle={onToggle} className="mt-[13px]" />
      <button
        type="button"
        onClick={onStar}
        className={cn(
          "order-last -ms-2 me-1.5 mt-1.5 grid size-7 shrink-0 place-items-center rounded-md outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring @2xl/list:order-none @2xl/list:ms-[-6px] @2xl/list:me-[-6px]",
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
        className="grid min-w-0 flex-1 grid-cols-[8px_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-0.5 py-2.5 pe-2 text-start text-body leading-5 outline-none [grid-template-areas:'dot_who_time'_'._main_main'] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset @2xl/list:grid-cols-[8px_minmax(0,180px)_minmax(0,1fr)_auto] @2xl/list:pe-3 @2xl/list:[grid-template-areas:'dot_who_main_time']"
        aria-current={current || undefined}
        title={inbox?.name}
        data-testid="mail-open"
      >
        <span
          className={cn("size-2 rounded-full bg-brand [grid-area:dot]", !c.unread && "invisible")}
          aria-label={c.unread ? t`Unread` : undefined}
          data-testid={c.unread ? "row-unread" : undefined}
        />
        <span
          className={cn(
            "min-w-0 truncate [grid-area:who]",
            c.unread ? "font-semibold text-foreground" : "font-medium text-foreground/85"
          )}
          data-testid="mail-sender"
        >
          {name}
        </span>
        <span className="flex min-w-0 items-center gap-2 [grid-area:main]">
          {showInbox && inbox && <FlatTag className="shrink-0">{inbox.name}</FlatTag>}
          <span className="min-w-0 truncate">
            <span className={cn("text-foreground", c.unread && "font-semibold")} data-testid="mail-subject">
              {subject}
            </span>
            <span className="text-muted-foreground">
              {" · "}
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
          </span>
        </span>
        <span className="flex items-center justify-end gap-2 [grid-area:time]">
          {c.status === "snoozed" && folder !== "snoozed" && (
            <ClockIcon className="size-3.5 shrink-0 text-muted2" aria-label={t`Snoozed`} />
          )}
          {c.has_attachments && (
            <PaperclipIcon
              className="size-3.5 shrink-0 text-muted2"
              aria-label={t`Has attachments`}
              data-testid="mail-attachment"
            />
          )}
          {scheduledAt ? (
            <FlatTime iso={scheduledAt} className="flex items-center gap-1 text-warning">
              <ClockIcon className="size-3" />
              <span data-testid="mail-scheduled-at">{formatWhen(scheduledAt, i18n.locale)}</span>
            </FlatTime>
          ) : (
            <FlatTime
              iso={c.last_activity_at}
              className={cn("@2xl/list:w-16 @2xl/list:text-end", c.unread && "font-medium text-foreground")}
            >
              {formatShort(c.last_activity_at, i18n.locale)}
            </FlatTime>
          )}
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
  hidden = false,
  head,
}: {
  box: string
  folder: Folder
  currentId: string | null
  onOpen: (id: string) => void
  active: boolean
  hidden?: boolean
  head?: React.ReactNode
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage } = useSession()
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
  const { cursorId, setCursorId, moveCursor, openCursor } = useListCursor(
    rows,
    currentId,
    onOpen,
    active,
    scroller,
    !hidden,
    !hidden
  )
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
    window.scrollTo({ top: 0 })
  }, [box, folder])

  useEffect(() => {
    const el = sentinel.current
    if (!el || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => setNearEnd(entries.some((e) => e.isIntersecting)), {
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
    const ids = rows.map((c) => c.id)
    const from = anchor.current ? ids.indexOf(anchor.current) : -1
    const to = ids.indexOf(id)
    const next = new Set(selected)
    if (range && from >= 0 && to >= 0) {
      for (let i = Math.min(from, to); i <= Math.max(from, to); i++) next.add(ids[i])
    } else if (!next.delete(id)) next.add(id)
    anchor.current = id
    setSelected(next)
    setSelecting(next.size > 0)
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
  const anyRead = chosenRows.some((c) => !c.unread)
  const markChosen = (read: boolean) => {
    const ids = chosenRows.filter((c) => c.unread === read).map((c) => c.id)
    const n = ids.length
    if (n === 0) return
    const apply = (to: boolean) => {
      for (const id of ids) {
        if (to) memberActions.markRead(id)
        else memberActions.markUnread(id)
      }
    }
    apply(read)
    toast(read ? t`${n} marked as read` : t`${n} marked as unread`, () => apply(!read))
    stopSelecting()
  }
  const readChosen = () => markChosen(anyUnread)
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
    void setTrashed(ids, true).then((done) => {
      if (done.length === 0) return
      toast(t`${done.length} moved to the Trash`, () => void setTrashed(done, false))
      stopSelecting()
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
  useHotkeys({ ...listKeys, ...selectionKeys, ...(selecting ? { Escape: stopSelecting } : {}) }, active && !hidden)

  const searchInput = useRef<HTMLInputElement>(null)
  const suggest = useOperatorSuggest(query, setQuery, searchInput)
  const searchBox = (
    <InputGroup className="h-8 min-w-48 flex-[1_1_240px] rounded-md border-input bg-card shadow-none focus-within:border-faint">
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

  const bb = bulkButton()
  const busy = bulk.isPending
  const bulkActions = inTrash ? (
    <ButtonGroup>
      <Button
        variant="outline"
        className={bb}
        disabled={none || busy}
        onClick={restoreChosen}
        title={t`Restore (Z)`}
        data-testid="restore-selected"
      >
        <Trans>Restore</Trans>
      </Button>
      <Button
        variant="outline"
        className={bulkButton(true)}
        disabled={none || trashTools.pending}
        onClick={deleteChosen}
        title={t`Delete for good (${MAIL_KEYS.deleteForever})`}
        data-testid="delete-forever-selected"
      >
        <Trans>Delete forever</Trans>
      </Button>
    </ButtonGroup>
  ) : (
    <>
      <ButtonGroup>
        {folder === "archive" ? (
          <Button
            variant="outline"
            className={bb}
            disabled={none || busy}
            onClick={inboxChosen}
            title={t`Move to the Inbox (E)`}
            data-testid="inbox-selected"
          >
            <Trans>Move to the Inbox</Trans>
          </Button>
        ) : (
          <Button
            variant="outline"
            className={bb}
            disabled={none || busy}
            onClick={archiveChosen}
            title={t`Archive (E)`}
            data-testid="archive-selected"
          >
            <Trans>Archive</Trans>
          </Button>
        )}
        <Button
          variant="outline"
          className={bb}
          disabled={!anyUnread}
          onClick={() => markChosen(true)}
          title={t`Mark as read (U)`}
          aria-label={t`Mark as read`}
          data-testid="read-selected"
        >
          <MailOpenIcon />
          <span className="hidden @lg/list:inline">
            <Trans>Mark as read</Trans>
          </span>
        </Button>
        <Button
          variant="outline"
          className={bb}
          disabled={!anyRead}
          onClick={() => markChosen(false)}
          title={t`Mark as unread (U)`}
          aria-label={t`Mark as unread`}
          data-testid="unread-selected"
        >
          <MailIcon />
          <span className="hidden @lg/list:inline">
            <Trans>Mark as unread</Trans>
          </span>
        </Button>
      </ButtonGroup>
      <ButtonGroup>
        <Button
          variant="outline"
          className={bb}
          disabled={none}
          onClick={starChosen}
          title={t`Star or unstar (S)`}
          data-testid="star-selected"
        >
          <StarIcon className={cn(allStarred && "fill-current text-warning")} />
          {allStarred ? <Trans>Remove the star</Trans> : <Trans>Star</Trans>}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<Button variant="outline" className={bb} disabled={none || busy} />}
            data-testid="snooze-selected"
          >
            <Trans>Snooze</Trans>
            <ChevronDownIcon className="text-muted-foreground" />
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
      </ButtonGroup>
      {canManage && (
        <Button
          variant="outline"
          className={bulkButton(true)}
          disabled={none || busy}
          onClick={trashChosen}
          title={t`Move to the Trash (#)`}
          data-testid="trash-selected"
        >
          <Trash2Icon />
          <Trans>Delete</Trans>
        </Button>
      )}
    </>
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

  const mailRow = (c: ConversationListItem) => (
    <MailRow
      key={c.id}
      c={c}
      folder={folder}
      current={c.id === currentId}
      cursor={c.id === cursorId}
      checked={selected.has(c.id)}
      showInbox={box === "all"}
      scheduledAt={folder === "scheduled" ? c.scheduled_at : undefined}
      onOpen={() => onOpen(c.id)}
      onToggle={(range) => toggle(c.id, range)}
      onStar={() => star([c.id], !c.starred)}
    />
  )

  return (
    <nav
      aria-label={t`Mail`}
      className={cn("@container/list flex min-w-0 flex-col", hidden && "hidden")}
      data-testid="mail-list"
    >
      {head}
      <FilterRow>
        {searchBox}
        <OperatorList s={suggest} />
        <SearchError error={list.error} className="w-full" />
      </FilterRow>
      {inTrash && (
        <div className="mt-3 [&>*]:mx-0">
          <TrashStrip onEmpty={(n) => trashTools.emptyTrash(n)} busy={trashTools.pending} />
        </div>
      )}
      <div ref={scroller} data-testid="list">
        <ListBox
          bar={
            selecting && (
              <BulkBar
                picked={picked}
                total={rows.length}
                onToggleAll={() => (allPicked ? stopSelecting() : setSelected(new Set(rows.map((c) => c.id))))}
                onCancel={stopSelecting}
                busy={busy}
              >
                {bulkActions}
              </BulkBar>
            )
          }
          testId="list-box"
        >
          {list.isPending ? (
            <ListBoxSkeleton />
          ) : rows.length === 0 && !hasNextPage ? (
            <ListBoxEmpty testId="list-empty">
              {q ? <Trans>No mail matches your search.</Trans> : empty[folder]}
            </ListBoxEmpty>
          ) : (
            rows.map(mailRow)
          )}
          {isFetchingNextPage && <ListBoxSkeleton rows={1} />}
        </ListBox>
        <div ref={sentinel} className="h-px" />
        <ErrorLine error={isSearchError(list.error) ? null : list.error} className="py-2" />
      </div>
      <KeyHints
        items={[
          [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
          [
            ["↵"],
            <Trans key="open" context="keyboard hint">
              open
            </Trans>,
          ],
          [[MAIL_KEYS.select], <Trans key="select">select</Trans>],
          [[MAIL_KEYS.archive], <Trans key="archive">archive</Trans>],
          [[MAIL_KEYS.star], <Trans key="star">star</Trans>],
          [[MAIL_KEYS.compose], <Trans key="compose">new mail</Trans>],
        ]}
      />
      {trashTools.dialogs}
    </nav>
  )
}

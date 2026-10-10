import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronDownIcon, ChevronUpIcon, PanelRightIcon, PaperclipIcon, SearchIcon, SearchXIcon, XIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import { ChannelIcon, ContactAvatar, Dot, EmptyState, MemberAvatar, toast } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ContactSheet } from "@/features/contact/ContactSheet"
import { useContact } from "@/features/contact/queries"
import { readDraft, useConversationActions, useDraftRevision, usePendingReplies } from "@/features/conversation/actions"
import { Beaten, TypingCollision } from "@/features/conversation/Collision"
import { useCommandHandlers } from "@/features/conversation/commands"
import { ReplyBox, type ReplyHandle } from "@/features/conversation/composer/ReplyBox"
import { ConversationMenu } from "@/features/conversation/ConversationMenu"
import { CategoryChip } from "@/features/conversation/Feedback"
import { usePagePublishing } from "@/features/docs/PageContext"
import { CloseButton, HandMenu, LabelsMenu, SnoozeMenu, type HeaderMenu } from "@/features/conversation/HeaderActions"
import { History } from "@/features/conversation/History"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { foldText, quoteOf } from "@/features/conversation/messages/MessageActions"
import { Thread } from "@/features/conversation/messages/Thread"
import { Status, Watchers } from "@/features/conversation/ConversationStatus"
import { useConversation, useMarkRead, useMessages, useMoveConversation, useUpdateConversation } from "@/features/conversation/queries"
import { RelatedLine } from "@/features/conversation/RelatedLine"
import { useListActions, usePinnedIds } from "@/features/inbox/memberState"
import { useConversations } from "@/features/inbox/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useMediaQuery } from "@/hooks/use-media-query"
import { ApiError, isGone, type Channel, type Contact, type Conversation, type ConversationUpdate, type Member, type Message } from "@/lib/api"
import { useViewers } from "@/lib/presence"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useChannel, useChannelMap, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

type Props = {
  id: string
  onBack?: () => void
  panel: boolean
  inlinePanel: boolean
  onPanel: (open: boolean) => void
}

function BackButton({ onBack }: { onBack?: () => void }) {
  const { t } = useLingui()
  if (!onBack) return null
  return (
    <Button variant="ghost" size="icon-sm" onClick={onBack} aria-label={t`Back to the list`} title={t`Back to the list`} data-testid="chat-back">
      <ArrowLeftIcon className="rtl:rotate-180" />
    </Button>
  )
}

export function ChatView(props: Props) {
  const errorText = useErrorText()
  const conversation = useConversation(props.id)
  const c = conversation.data
  const contact = useContact(c?.contact_id)
  const channels = useChannelMap()
  const known = c?.channel_id ? channels.get(c.channel_id) : undefined
  const fetched = useChannel(c?.channel_id && !known ? c.channel_id : undefined).data
  if (conversation.isPending) {
    return (
      <div className="flex min-h-0 flex-1 flex-col" data-testid="conversation-loading">
        <div className="flex h-15 items-center gap-3 border-b px-3">
          <BackButton onBack={props.onBack} />
          <Skeleton className="size-10 rounded-full" />
          <div className="flex flex-col gap-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-56" />
          </div>
        </div>
        <div className="flex flex-1 flex-col gap-2 bg-surface p-6">
          <Skeleton className="h-12 w-2/3 rounded-2xl" />
          <Skeleton className="h-12 w-1/2 self-end rounded-2xl" />
        </div>
      </div>
    )
  }
  if (!c || isGone(conversation.error)) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {props.onBack && (
          <div className="flex h-15 items-center border-b px-3">
            <BackButton onBack={props.onBack} />
          </div>
        )}
        <EmptyState icon={SearchXIcon} title={<Trans>This conversation is not available</Trans>}>
          <p>{errorText(conversation.error ?? new ApiError(404))}</p>
          <Button variant="outline" size="sm" className="mt-4" render={<Link to="/" />}>
            <Trans>Back to the list</Trans>
          </Button>
        </EmptyState>
      </div>
    )
  }
  return <Loaded {...props} c={c} contact={contact.data} channel={known ?? fetched} />
}

function Loaded({ c, contact, channel, onBack, panel, inlinePanel, onPanel }: Props & { c: Conversation; contact?: Contact; channel?: Channel }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const { membership } = useSession()
  const me = membership.member_id
  const members = useMemberMap()
  const labels = useLabels().data ?? []
  const inboxes = useInboxes().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const messages = useMessages(c.id)
  const history = useConversations({ contact_id: c.contact_id })
  const others = (history.data?.pages.flatMap((p) => p.items) ?? []).filter((o) => o.id !== c.id)
  const name = contact ? contact.name || contact.emails[0] || t`Unnamed contact` : "…"
  const actions = useConversationActions(c, name)
  const updater = useUpdateConversation(c.id)
  const mover = useMoveConversation(c.id)
  const navigate = useNavigate()
  const reply = useRef<ReplyHandle>(null)
  const [menu, setMenu] = useState<HeaderMenu>(null)
  const [moreOpen, setMoreOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [expandQuoted, setExpandQuoted] = useState(false)
  const [claimedFrom, setClaimedFrom] = useState<string | null>(null)
  const [beatenBy, setBeatenBy] = useState<string | null>(null)
  const fine = useMediaQuery("(pointer: fine)")
  const [sheet, setSheet] = useState(false)
  const [finding, setFinding] = useState(false)
  const [findQuery, setFindQuery] = useState("")
  const [findIndex, setFindIndex] = useState(-1)
  const [dragging, setDragging] = useState(false)
  const shownPanel = inlinePanel ? panel : sheet
  const setPanel = inlinePanel ? onPanel : setSheet
  const revision = useDraftRevision(c.id)
  const queued = usePendingReplies(c.id)

  const items = useMemo(() => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()), [messages.data])
  const focusId = useSearchParams()[0].get("m") ?? undefined
  const focusMissing = !!focusId && !!messages.data && !items.some((m) => m.id === focusId)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = messages
  useEffect(() => {
    if (focusMissing && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [focusMissing, hasNextPage, isFetchingNextPage, fetchNextPage])
  const suggestion = items.findLast((m) => m.draft)
  const shown = items.filter((m) => m !== suggestion)
  const inbound = shown.filter((m) => m.kind === "message" && m.direction !== "out")
  const lastInbound = inbound.at(-1)
  const feedbackId = c.kind === "feedback" && !messages.hasNextPage ? inbound[0]?.id : undefined
  const emailTo = channel?.kind === "email" ? (inbound.findLast((m) => m.email)?.email?.from ?? contact?.emails[0]) : undefined
  const undeliverable = !!emailTo && !!contact?.undeliverable.some((u) => u.email === emailTo)
  const q = finding ? foldText(findQuery.trim()) : ""
  const hits = q ? shown.filter((m) => (m.kind === "message" || m.kind === "note") && foldText(m.body).includes(q)) : []
  const hitIndex = hits.length === 0 ? -1 : findIndex < 0 || findIndex >= hits.length ? hits.length - 1 : findIndex
  const currentHit = hitIndex >= 0 ? hits[hitIndex].id : undefined
  const quote = (m: Message) => reply.current?.quote(quoteOf(m.body))
  const forward = (m: Message, to: Member) => {
    const label = to.name || to.email
    actions.note(`@${label}\n${quoteOf(m.body)}`, [], [to.id]).then(
      () => toast(t`Forwarded to ${label} as a note`),
      (err) => toast(errorText(err)),
    )
  }
  const ctx: ThreadContext = {
    members,
    contact,
    labels,
    inboxes,
    subject: c.subject,
    expandQuoted,
    inboxId: c.inbox_id,
    contactReadAt: c.last_read_by_contact_at,
    find: q ? { query: findQuery.trim(), current: currentHit } : undefined,
    onQuote: quote,
    onForward: forward,
  }
  const pagePublishing = usePagePublishing(c, shown, !messages.hasNextPage)
  const chipLabels = labels.filter((l) => c.labels.includes(l.id))
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined

  const memberName = (id?: string) => {
    const m = id ? members.get(id) : undefined
    return m ? m.name || m.email : t`A teammate`
  }
  const typists = useTyping(c.id)
  const contactTyping = typists.some((a) => a.type === "contact")
  const typingMate = typists.find((a) => a.type === "member" && a.member_id !== me)
  const viewers = useViewers(c.id)
  const mate = typingMate ?? (viewers[0] ? { type: "member" as const, member_id: viewers[0], name: undefined } : undefined)
  const mateName = mate ? mate.name || memberName(mate.member_id) : ""
  const showCollision = !!mate && claimedFrom !== mate.member_id && c.status === "open"

  const seen = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (!messages.data) return
    if (!seen.current) {
      seen.current = new Set(items.map((m) => m.id))
      return
    }
    for (const m of items) {
      if (seen.current.has(m.id)) continue
      seen.current.add(m.id)
      const by = m.sent_by ?? m.author
      if (m.kind !== "message" || m.direction !== "out" || m.draft || by.member_id === me) continue
      if (readDraft(c.id).body.trim()) setBeatenBy(by.type === "bot" ? by.name || t`Bot` : memberName(by.member_id))
    }
  })

  const last = shown.at(-1)
  const lastId = last?.id
  const lastMine = !!last && (last.sent_by ?? last.author).member_id === me
  const { mutate: mark } = useMarkRead(c.id)
  const marked = useRef<string | null>(null)
  useEffect(() => {
    const go = () => {
      if (!lastId || marked.current === lastId || document.visibilityState !== "visible") return
      marked.current = lastId
      mark(lastId)
    }
    go()
    document.addEventListener("visibilitychange", go)
    return () => document.removeEventListener("visibilitychange", go)
  }, [lastId, mark])

  const fail = (err: unknown) => toast(errorText(err))
  const update = (body: ConversationUpdate) => updater.mutate(body, { onError: fail })
  const move = (inboxId: string) => mover.mutate(inboxId, { onError: fail })
  const claim = () => {
    if (mate?.member_id) setClaimedFrom(mate.member_id)
    if (c.assignee_id !== me) actions.claim()
    reply.current?.focus("message")
  }
  const leaveTo = () => {
    const m = mate?.member_id ? members.get(mate.member_id) : undefined
    if (m) actions.hand(m, "")
  }
  const togglePanel = () => setPanel(!shownPanel)
  const listActions = useListActions()
  const pinned = usePinnedIds().has(c.id)
  const togglePin = () => listActions.pin(c.id, !pinned)
  const markUnread = () => {
    marked.current = lastId ?? null
    listActions.markUnread(c.id)
    navigate("/")
  }
  const openFind = () => {
    setFinding(true)
    requestAnimationFrame(() => document.querySelector<HTMLInputElement>("[data-testid=find-input]")?.select())
  }
  const closeFind = () => {
    setFinding(false)
    setFindQuery("")
    setFindIndex(-1)
  }
  const stepFind = (d: 1 | -1) => hits.length > 0 && setFindIndex((hitIndex + d + hits.length) % hits.length)
  const openMenu = (m: HeaderMenu) => (o: boolean) => setMenu(o ? m : null)

  const handlers = {
    reply: () => (showCollision ? claim() : reply.current?.focus("message")),
    note: () => reply.current?.focus("note"),
    snooze: () => setMenu("snooze"),
    hand: () => setMenu("hand"),
    labels: () => setMenu("labels"),
    close: () => (c.status === "closed" ? actions.reopen() : actions.close()),
    contact: togglePanel,
    more: () => setMoreOpen(true),
    history: () => setHistoryOpen((o) => !o),
  }
  useCommandHandlers(handlers)
  useHotkeys({
    [SHORTCUTS.reply]: handlers.reply,
    [SHORTCUTS.note]: handlers.note,
    [SHORTCUTS.attach]: () => reply.current?.attach(),
    [SHORTCUTS.snooze]: handlers.snooze,
    [SHORTCUTS.hand]: handlers.hand,
    [SHORTCUTS.labels]: handlers.labels,
    [SHORTCUTS.close]: handlers.close,
    [SHORTCUTS.leave]: () => showCollision && leaveTo(),
    [SHORTCUTS.history]: handlers.history,
    [SHORTCUTS.contact]: handlers.contact,
    [SHORTCUTS.more]: handlers.more,
    [SHORTCUTS.spam]: () => update({ spam: !c.spam }),
    [SHORTCUTS.quoted]: () => setExpandQuoted((x) => !x),
    [SHORTCUTS.discardSuggestion]: () => reply.current?.discardSuggestion(),
    [SHORTCUTS.find]: openFind,
    [SHORTCUTS.pin]: togglePin,
    [SHORTCUTS.unread]: markUnread,
    [SHORTCUTS.publish]: pagePublishing.run,
  })

  const sep = <span aria-hidden>·</span>
  return (
    <section
      aria-label={name}
      className="relative flex min-h-0 min-w-0 flex-1 flex-col"
      data-testid="conversation"
      data-conversation-id={c.id}
      onDragEnter={(e) => {
        if (e.dataTransfer.types.includes("Files")) setDragging(true)
      }}
      onDragOver={(e) => {
        if (!e.dataTransfer.types.includes("Files")) return
        e.preventDefault()
        e.dataTransfer.dropEffect = "copy"
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDragging(false)
      }}
      onDrop={(e) => {
        if (!e.dataTransfer.files.length) return
        e.preventDefault()
        setDragging(false)
        reply.current?.addFiles(Array.from(e.dataTransfer.files))
        reply.current?.focus()
      }}
    >
      {dragging && (
        <div className="pointer-events-none absolute inset-2 z-30 grid place-items-center rounded-2xl border-2 border-dashed border-brand/60 bg-background/85 text-center" data-testid="drop-overlay">
          <span className="flex flex-col items-center gap-2 text-reading font-medium text-brand">
            <PaperclipIcon className="size-7" />
            <Trans>Drop files to attach them to your reply</Trans>
          </span>
        </div>
      )}
      <header className="@container/person flex h-15 shrink-0 items-center gap-2 border-b bg-background ps-2 pe-2 phone:gap-1 phone:ps-1" data-testid="person">
        <BackButton onBack={onBack} />
        <button type="button" onClick={togglePanel} className="flex min-w-0 flex-1 items-center gap-3 rounded-lg py-1 ps-1.5 pe-2 text-start hover:bg-muted" title={t`Contact details (C)`}>
          <span className="relative shrink-0">
            <ContactAvatar id={c.contact_id} name={name} className="size-10" />
            {actions.live && <span className="absolute -end-px -bottom-px size-3 rounded-full border-2 border-background bg-success" data-testid="contact-live" />}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-reading font-medium" data-testid="person-name">
              {name}
            </span>
            <span className="flex min-w-0 items-center gap-x-1.5 truncate text-caption text-faint">
              {channel && <ChannelIcon kind={channel.kind} className="size-3 shrink-0" />}
              {inbox && (
                <span className="inline-flex min-w-0 items-center gap-1 truncate phone:hidden @max-xl/person:hidden">
                  <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />
                  <span className="truncate">{inbox.name}</span>
                </span>
              )}
              {inbox && <span aria-hidden className="phone:hidden @max-xl/person:hidden">·</span>}
              <span className="truncate">
                <Status c={c} lastInbound={lastInbound} />
              </span>
            </span>
          </span>
        </button>
        {assignee && (
          <span title={t`Assigned to ${assignee.name || assignee.email}`} className="phone:hidden @max-xl/person:hidden" data-testid="header-assignee">
            <MemberAvatar name={assignee.name || assignee.email} online={assignee.online} away={assignee.availability === "away"} className="size-7" />
          </span>
        )}
        <div className="flex shrink-0 items-center gap-0.5">
          <HandMenu c={c} actions={actions} open={menu === "hand"} onOpenChange={openMenu("hand")} />
          <SnoozeMenu name={name} actions={actions} open={menu === "snooze"} onOpenChange={openMenu("snooze")} />
          <span className="phone:hidden @max-xl/person:hidden">
            <LabelsMenu c={c} update={update} open={menu === "labels"} onOpenChange={openMenu("labels")} />
          </span>
          <CloseButton c={c} actions={actions} />
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => (finding ? closeFind() : openFind())}
            aria-pressed={finding}
            className={cn(finding && "bg-muted text-foreground")}
            aria-label={t`Search in this conversation`}
            title={t`Search in this conversation (${keyLabel(SHORTCUTS.find).join("")})`}
            data-testid="find-toggle"
          >
            <SearchIcon />
          </Button>
          {inlinePanel && (
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={togglePanel}
              aria-pressed={shownPanel}
              className={cn(shownPanel && "bg-muted text-foreground")}
              aria-label={t`Contact details (C)`}
              title={t`Contact details (C)`}
              data-testid="toggle-contact-panel"
            >
              <PanelRightIcon className="rtl:-scale-x-100" />
            </Button>
          )}
          <ConversationMenu
            c={c}
            open={moreOpen}
            onOpenChange={setMoreOpen}
            update={update}
            move={move}
            onContact={() => setPanel(true)}
            pinned={pinned}
            onPin={togglePin}
            onUnread={markUnread}
          />
        </div>
      </header>
      {(chipLabels.length > 0 || (c.kind === "feedback" && c.feedback) || pagePublishing.line || c.related_conversation_id || viewers.length > 0 || typingMate) && (
        <div className="flex shrink-0 flex-wrap items-center gap-x-1.5 gap-y-1 border-b bg-background px-4 py-1.5 text-caption text-faint" data-testid="header-details">
          {c.kind === "feedback" && c.feedback && <CategoryChip category={c.feedback.category} />}
          {c.kind === "feedback" && c.feedback && pagePublishing.line && sep}
          {pagePublishing.line}
          {pagePublishing.line && chipLabels.length > 0 && sep}
          {chipLabels.map((l) => (
            <span key={l.id} className="inline-flex items-center gap-1 rounded-full border bg-card px-2 py-px text-caption text-muted-foreground">
              <Dot color={l.color} className="size-1.5" />
              {l.name}
            </span>
          ))}
          {c.related_conversation_id && (
            <>
              {(chipLabels.length > 0 || c.feedback || pagePublishing.line) && sep}
              <RelatedLine id={c.related_conversation_id} hrefFor={(id) => `/conversations/${id}`} />
            </>
          )}
          <Watchers conversationId={c.id} />
        </div>
      )}
      {finding && (
        <div className="flex shrink-0 items-center gap-1 border-b bg-background py-1.5 ps-3 pe-2" role="search" data-testid="find-bar">
          <SearchIcon className="size-4 shrink-0 text-faint" />
          <input
            name="find"
            data-testid="find-input"
            autoFocus
            value={findQuery}
            onChange={(e) => {
              setFindQuery(e.target.value)
              setFindIndex(-1)
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault()
                stepFind(e.shiftKey ? 1 : -1)
              } else if (e.key === "Escape") {
                e.preventDefault()
                closeFind()
              }
            }}
            placeholder={t`Search in the loaded messages`}
            aria-label={t`Search in this conversation`}
            className="h-8 min-w-0 flex-1 bg-transparent px-1.5 text-body outline-none"
          />
          <span className="shrink-0 px-1.5 text-caption text-faint tabular-nums" aria-live="polite" data-testid="find-count">
            {q ? (hits.length ? t`${hitIndex + 1} of ${hits.length}` : t`No matches`) : ""}
          </span>
          <Button variant="ghost" size="icon-sm" onClick={() => stepFind(-1)} disabled={hits.length < 2} aria-label={t`Older match`} title={t`Older match (Enter)`}>
            <ChevronUpIcon />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={() => stepFind(1)} disabled={hits.length < 2} aria-label={t`Newer match`} title={t`Newer match (⇧Enter)`}>
            <ChevronDownIcon />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={closeFind} aria-label={t`Close the search`} title={t`Close the search (Esc)`}>
            <XIcon />
          </Button>
        </div>
      )}
      {c.spam && (
        <Alert role="status" variant="destructive" className="flex shrink-0 items-center gap-3 rounded-none border-x-0 border-t-0 border-b bg-destructive/6 px-4 py-2 text-small text-foreground" data-testid="spam-banner">
          <span className="flex-1">
            <Trans>Marked as spam: never answered automatically.</Trans>
          </span>
          <Button variant="link" className="text-foreground underline" onClick={() => update({ spam: false })}>
            <Trans>Not spam</Trans>
          </Button>
        </Alert>
      )}
      <Thread
        key={c.id}
        items={items}
        ctx={ctx}
        conversation={c}
        name={name}
        feedbackId={feedbackId}
        top={<History others={others} open={historyOpen} onToggle={() => setHistoryOpen((o) => !o)} name={name} />}
        older={{ has: !!messages.hasNextPage, loading: messages.isFetchingNextPage, load: () => void messages.fetchNextPage() }}
        pending={messages.isPending}
        error={messages.error}
        queued={queued}
        typing={contactTyping}
        lastId={lastId}
        lastMine={lastMine}
        focusId={focusId}
      />
      <Composer>
        {beatenBy ? <Beaten name={beatenBy} /> : showCollision && <TypingCollision name={mateName} typing={!!typingMate} onLeave={leaveTo} onClaim={claim} />}
        <ReplyBox
          key={`${c.id}:${revision}`}
          ref={reply}
          c={c}
          contactName={name}
          via={channel ? text.channel[channel.kind] : undefined}
          emailTo={emailTo}
          undeliverable={undeliverable}
          suggestion={suggestion}
          ctx={ctx}
          actions={actions}
          autoFocus={fine && revision === 0}
        />
      </Composer>
      {pagePublishing.dialogs}
      {!inlinePanel && <ContactSheet contactId={c.contact_id} conversationId={c.id} open={sheet} onOpenChange={setSheet} />}
    </section>
  )
}

function Composer({ children }: { children: React.ReactNode }) {
  return (
    <div className="shrink-0 border-t bg-background px-4 pt-2.5 pb-3 phone:px-2 phone:pb-[max(0.5rem,env(safe-area-inset-bottom))]">
      <div className="mx-auto flex max-w-[880px] flex-col gap-2">{children}</div>
    </div>
  )
}

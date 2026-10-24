import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowLeftIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  PanelRightIcon,
  PaperclipIcon,
  SearchIcon,
  SearchXIcon,
  XIcon,
} from "lucide-react"
import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useSearchParams } from "react-router"

import {
  ContactAvatar,
  Dot,
  EmptyState,
  ImportedChip,
  isVisitor,
  placeOf,
  sourceOf,
  toast,
  useContactName,
  useSourceText,
  VisitorAvatar,
} from "@/components/common"
import { inboxRoot } from "@/app/shell"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { Alert } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ContactSheet } from "@/features/contact/ContactSheet"
import { useContact } from "@/features/contact/queries"
import { readDraft, useConversationActions, useDraftRevision, usePendingReplies } from "@/features/conversation/actions"
import { Beaten, TypingCollision } from "@/features/conversation/Collision"
import { useCommandHandlers, useInsertHandler } from "@/features/conversation/commands"
import { ReplyBox, type ReplyHandle } from "@/features/conversation/composer/ReplyBox"
import { ConversationMenu, type MenuView } from "@/features/conversation/ConversationMenu"
import { TrashBanner } from "@/features/conversation/Trash"
import { CategoryChip, ProgressStrip } from "@/features/conversation/Feedback"
import { IdeaChip } from "@/features/conversation/IdeaChip"
import { ForwardDialog } from "@/features/conversation/ForwardDialog"
import { feedbackDraft } from "@/features/issues/draft"
import { IssueChips } from "@/features/issues/IssueChips"
import { IssueDialogs, IssueMenu, type IssueMode } from "@/features/issues/IssueMenu"
import { IncidentChips } from "@/features/status/IncidentChips"
import { LinkIncidentDialog } from "@/features/status/LinkIncidentDialog"
import { useConversationIncidents } from "@/features/status/linked"
import { FromConversationDialog } from "@/features/docs/discussions/FromConversation"
import { conversationPage, PageLine } from "@/features/docs/PageContext"
import {
  CloseButton,
  HandMenu,
  ProgressMenu,
  SnoozeMenu,
  TeamMenu,
  useInboxTeams,
  type HeaderMenu,
} from "@/features/conversation/HeaderActions"
import { History } from "@/features/conversation/History"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { foldText, quoteOf } from "@/features/conversation/messages/MessageActions"
import { Thread } from "@/features/conversation/messages/Thread"
import { Status, Watchers } from "@/features/conversation/ConversationStatus"
import { SlaPopover } from "@/features/conversation/SlaPopover"
import {
  useConversation,
  useMarkRead,
  useMessages,
  useParticipants,
  useRedactMessage,
  useMoveConversation,
  useTrashConversation,
  useUpdateConversation,
} from "@/features/conversation/queries"
import { BroadcastLine } from "@/features/conversation/BroadcastLine"
import { RelatedLine } from "@/features/conversation/RelatedLine"
import { look } from "@/features/inbox/PageFrame"
import { useListActions, usePinnedIds } from "@/features/inbox/memberState"
import { MailThread } from "@/features/mail/MailThread"
import { useConversations } from "@/features/inbox/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import {
  ApiError,
  isGone,
  type Channel,
  type Contact,
  type Conversation,
  type ConversationUpdate,
  type Member,
  type Message,
} from "@/lib/api"
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

const LinkIdeaDialog = lazy(() => import("@/features/roadmap/LinkIdeaDialog"))

function BackButton({ onBack }: { onBack?: () => void }) {
  const { t } = useLingui()
  if (!onBack) return null
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      onClick={onBack}
      aria-label={t`Back to the list`}
      title={t`Back to the list`}
      data-testid="chat-back"
    >
      <ArrowLeftIcon className="rtl:rotate-180" />
    </Button>
  )
}

export function ChatView(props: Props) {
  const errorText = useErrorText()
  const navigate = useNavigate()
  const { apart } = useSession()
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
          <Button variant="outline" size="sm" className="mt-4" render={<Link to={inboxRoot(apart)} />}>
            <Trans>Back to the list</Trans>
          </Button>
        </EmptyState>
      </div>
    )
  }
  if (c.inbox_purpose === "mail")
    return <MailThread id={c.id} onBack={props.onBack} onLeave={props.onBack ?? (() => navigate(inboxRoot(apart)))} />
  return <Loaded {...props} c={c} contact={contact.data} channel={known ?? fetched} />
}

function Loaded({
  c,
  contact,
  channel,
  onBack,
  panel,
  inlinePanel,
  onPanel,
}: Props & { c: Conversation; contact?: Contact; channel?: Channel }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const { membership, canManage, apart } = useSession()
  const me = membership.member_id
  const members = useMemberMap()
  const labels = useLabels().data ?? []
  const inboxes = useInboxes().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const messages = useMessages(c.id)
  const history = useConversations({ contact_id: c.contact_id })
  const others = (history.data?.pages.flatMap((p) => p.items) ?? []).filter((o) => o.id !== c.id)
  const contactName = useContactName()
  const visitor = !!contact && isVisitor(contact.name, contact.emails[0])
  const name = contact ? contactName(c, channel, contact.name, contact.emails[0]) : "…"
  const source = sourceOf(c, channel)
  const sourceText = useSourceText()
  const place = placeOf(c, channel)
  const actions = useConversationActions(c, name)
  const updater = useUpdateConversation(c.id)
  const mover = useMoveConversation(c.id)
  const trasher = useTrashConversation(c.id)
  const redactor = useRedactMessage()
  const [confirm, confirmDialog] = useConfirm()
  const trashed = !!c.trashed_at
  const navigate = useNavigate()
  const reply = useRef<ReplyHandle>(null)
  const [menu, setMenu] = useState<HeaderMenu>(null)
  const [moreOpen, setMoreOpen] = useState(false)
  const [moreView, setMoreView] = useState<MenuView>("root")
  const [historyOpen, setHistoryOpen] = useState(false)
  const [expandQuoted, setExpandQuoted] = useState(false)
  const [claimedFrom, setClaimedFrom] = useState<string | null>(null)
  const [beatenBy, setBeatenBy] = useState<string | null>(null)
  const fine = useMediaQuery("(pointer: fine)")
  const phone = useIsPhone()
  const actionsRef = useRef<HTMLDivElement>(null)
  const [sheet, setSheet] = useState(false)
  const [finding, setFinding] = useState(false)
  const [findQuery, setFindQuery] = useState("")
  const [findIndex, setFindIndex] = useState(-1)
  const [dragging, setDragging] = useState(false)
  const shownPanel = inlinePanel ? panel : sheet
  const setPanel = inlinePanel ? onPanel : setSheet
  const revision = useDraftRevision(c.id)
  const queued = usePendingReplies(c.id)

  const items = useMemo(
    () => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()),
    [messages.data]
  )
  const [params] = useSearchParams()
  const focusId = params.get("m") ?? undefined
  const composeNote = params.get("compose") === "note"
  useEffect(() => {
    if (composeNote) requestAnimationFrame(() => reply.current?.focus("note"))
  }, [composeNote])
  const focusMissing = !!focusId && !!messages.data && !items.some((m) => m.id === focusId)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = messages
  useEffect(() => {
    if (focusMissing && hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [focusMissing, hasNextPage, isFetchingNextPage, fetchNextPage])
  const suggestion = items.findLast((m) => m.draft && m.kind !== "forward")
  const shown = items.filter((m) => m !== suggestion)
  const inbound = shown.filter((m) => m.kind === "message" && m.direction !== "out")
  const lastInbound = inbound.at(-1)
  const feedbackId = c.kind === "feedback" && !messages.hasNextPage ? inbound[0]?.id : undefined
  const contactEmails = contact?.emails.map((e) => e.toLowerCase()) ?? []
  const lastContactMail = inbound.findLast(
    (m) => m.author.type === "contact" && !!m.email?.from && contactEmails.includes(m.email.from.toLowerCase())
  )
  const emailTo = channel?.kind === "email" ? (lastContactMail?.email?.from ?? contact?.emails[0]) : undefined
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
      (err) => toast(errorText(err))
    )
  }
  const canRemove = (m: Message) =>
    !trashed &&
    !m.draft &&
    !m.redacted_at &&
    (m.kind === "message" || m.kind === "note") &&
    (canManage || (m.author.type === "member" && m.author.member_id === me))
  const remove = (m: Message) => {
    const sentMail = m.kind === "message" && m.direction === "out" && !!(m.email || m.delivery)
    confirm({
      title: m.kind === "note" ? <Trans>Remove this note?</Trans> : <Trans>Remove this message?</Trans>,
      description: sentMail ? (
        <Trans>
          Its text and files are deleted for good and the thread shows that you removed it. A sent e-mail cannot be
          taken back from the recipient's mailbox.
        </Trans>
      ) : (
        <Trans>Its text and files are deleted for good and the thread shows that you removed it.</Trans>
      ),
      confirm: <Trans>Remove</Trans>,
      run: () =>
        redactor.mutate(m, {
          onSuccess: () => toast(t`Removed`),
          onError: (err) => toast(errorText(err)),
        }),
    })
  }
  const participants = useParticipants(c.id, c.channel_kind === "email")
  const ctx: ThreadContext = {
    members,
    contact,
    participants: participants.data?.items,
    labels,
    inboxes,
    subject: c.subject,
    feedback: c.kind === "feedback",
    expandQuoted,
    inboxId: c.inbox_id,
    contactReadAt: c.last_read_by_contact_at,
    find: q ? { query: findQuery.trim(), current: currentHit } : undefined,
    onQuote: quote,
    onForward: forward,
    onForwardMail: (m) => setForwarding({ target: m }),
    onRemove: remove,
    canRemove,
  }
  const pageLine = !!conversationPage(c)
  const [discussing, setDiscussing] = useState(false)
  const [forwarding, setForwarding] = useState<{ target?: Message } | null>(null)
  const [linking, setLinking] = useState(false)
  const [ideaLinking, setIdeaLinking] = useState(false)
  const [issueMode, setIssueMode] = useState<IssueMode | null>(null)
  const issues = c.issues ?? []
  const incidents = useConversationIncidents(c.id).data ?? []
  const chipLabels = labels.filter((l) => c.labels.includes(l.id))
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const inboxTeams = useInboxTeams(c.inbox_id)

  const memberName = (id?: string) => {
    const m = id ? members.get(id) : undefined
    return m ? m.name || m.email : t`A teammate`
  }
  const typists = useTyping(c.id)
  const contactTyping = typists.some((a) => a.type === "contact")
  const typingMate = typists.find((a) => a.type === "member" && a.member_id !== me)
  const viewers = useViewers(c.id)
  const mate =
    typingMate ?? (viewers[0] ? { type: "member" as const, member_id: viewers[0], name: undefined } : undefined)
  const mateName = mate ? mate.name || memberName(mate.member_id) : ""
  const showCollision = !!mate && claimedFrom !== mate.member_id && c.status === "open"

  const [seen, setSeen] = useState<{ items: Message[]; ids: Set<string> } | null>(null)
  if (messages.data && seen?.items !== items) {
    const ids = new Set(seen?.ids)
    for (const m of items) {
      if (ids.has(m.id)) continue
      ids.add(m.id)
      const by = m.sent_by ?? m.author
      if (!seen || m.kind !== "message" || m.direction !== "out" || m.draft || by.member_id === me) continue
      if (readDraft(c.id).body.trim()) setBeatenBy(by.type === "bot" ? by.name || t`Bot` : memberName(by.member_id))
    }
    setSeen({ items, ids })
  }

  const last = shown.at(-1)
  const lastId = last?.id
  const lastMine = !!last && (last.sent_by ?? last.author).member_id === me
  const { mutate: mark } = useMarkRead(c.id)
  const marked = useRef<string | null>(null)
  useEffect(() => {
    const go = () => {
      if (trashed || !lastId || marked.current === lastId || document.visibilityState !== "visible") return
      marked.current = lastId
      mark(lastId)
    }
    go()
    document.addEventListener("visibilitychange", go)
    return () => document.removeEventListener("visibilitychange", go)
  }, [lastId, mark, trashed])

  const fail = (err: unknown) => toast(errorText(err))
  const changed = (body: ConversationUpdate): { done: string; revert: ConversationUpdate } | undefined => {
    if (body.priority) {
      const level = text.priority[body.priority]
      return { done: t`Priority set to ${level}`, revert: { priority: c.priority } }
    }
    if (body.labels) return { done: t`Labels changed`, revert: { labels: c.labels } }
    if (body.spam !== undefined) return { done: body.spam ? t`Marked as spam` : t`Not spam`, revert: { spam: c.spam } }
    if (body.status === "open" && (c.status === "closed" || c.status === "pending"))
      return { done: t`Conversation reopened`, revert: { status: c.status } }
    return undefined
  }
  const update = (body: ConversationUpdate) => {
    const note = changed(body)
    updater.mutate(body, {
      onSuccess: () => note && toast(note.done, () => updater.mutate(note.revert, { onError: fail })),
      onError: fail,
    })
  }
  const move = (inboxId: string) => {
    const from = c.inbox_id
    const to = inboxes.find((i) => i.id === inboxId)?.name ?? ""
    mover.mutate(inboxId, {
      onSuccess: () => toast(t`Moved to ${to}`, () => mover.mutate(from, { onError: fail })),
      onError: fail,
    })
  }
  const restore = () =>
    trasher.mutate(false, {
      onSuccess: () => toast(t`Restored from the Trash`, () => trasher.mutate(true, { onError: fail })),
      onError: fail,
    })
  const trash = () =>
    confirm({
      title: <Trans>Move this conversation to the Trash?</Trans>,
      description: (
        <Trans>
          It disappears for your team and for the customer. You can restore it from the Trash for 30 days, then it is
          deleted for good with its messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        trasher.mutate(true, {
          onSuccess: () => toast(t`Moved to the Trash`, () => trasher.mutate(false, { onError: fail })),
          onError: fail,
        }),
    })
  const claim = () => {
    if (mate?.member_id) setClaimedFrom(mate.member_id)
    if (c.assignee_id !== me) actions.claim()
    reply.current?.focus("message")
  }
  const leaveTo = () => {
    const m = mate?.member_id ? members.get(mate.member_id) : undefined
    if (m) {
      setClaimedFrom(m.id)
      actions.hand(m, "")
    }
  }
  const togglePanel = () => setPanel(!shownPanel)
  const listActions = useListActions()
  const pinned = usePinnedIds().has(c.id)
  const togglePin = () => listActions.pin(c.id, !pinned)
  const markUnread = () => {
    marked.current = lastId ?? null
    listActions.markUnread(c.id)
    navigate(c.kind === "feedback" ? "/feedback" : inboxRoot(apart))
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
    assignTeam: () => (inboxTeams.length > 0 || c.team_id) && setMenu("team"),
    labels: () => {
      setMoreView("labels")
      setMoreOpen(true)
    },
    fields: () => {
      const wasOpen = shownPanel
      if (!wasOpen) setPanel(true)
      setTimeout(
        () => document.querySelector<HTMLElement>("[data-testid=conversation-fields] [data-field-input]")?.focus(),
        wasOpen ? 0 : 200
      )
    },
    progress: () => c.kind === "feedback" && setMenu("progress"),
    sla: () => c.sla && setMenu("sla"),
    close: () => (c.status === "closed" ? actions.reopen() : actions.close()),
    contact: togglePanel,
    more: () => setMoreOpen(true),
    history: () => setHistoryOpen((o) => !o),
    discussion: () => setDiscussing(true),
    incident: () => setLinking(true),
    forward: () => {
      const id = document.activeElement?.closest("[data-message-id]")?.getAttribute("data-message-id")
      const focused = id ? items.find((m) => m.id === id) : undefined
      setForwarding({
        target: focused?.kind === "message" && !focused.draft && !focused.redacted_at ? focused : undefined,
      })
    },
    idea: () => c.kind === "feedback" && setIdeaLinking(true),
    createIssue: () => c.kind === "feedback" && setIssueMode("create"),
    linkIssue: () => c.kind === "feedback" && setIssueMode("link"),
    article: () => reply.current?.article(),
    recipients: () => reply.current?.recipients(),
    signature: () => reply.current?.signature(),
    sendAs: () => reply.current?.sendAs(),
    trash: () => canManage && trash(),
  }
  useInsertHandler((choice) => !trashed && reply.current?.insert(choice))
  useCommandHandlers(
    trashed ? { contact: handlers.contact, history: handlers.history, restore: () => canManage && restore() } : handlers
  )
  useHotkeys(
    trashed
      ? {
          [SHORTCUTS.contact]: handlers.contact,
          [SHORTCUTS.history]: handlers.history,
          [SHORTCUTS.quoted]: () => setExpandQuoted((x) => !x),
          [SHORTCUTS.find]: openFind,
          [SHORTCUTS.restore]: () => canManage && restore(),
        }
      : {
          [SHORTCUTS.reply]: handlers.reply,
          [SHORTCUTS.note]: handlers.note,
          [SHORTCUTS.attach]: () => reply.current?.attach(),
          [SHORTCUTS.snooze]: handlers.snooze,
          [SHORTCUTS.hand]: handlers.hand,
          [SHORTCUTS.assignTeam]: handlers.assignTeam,
          [SHORTCUTS.labels]: handlers.labels,
          [SHORTCUTS.fields]: handlers.fields,
          [SHORTCUTS.progress]: handlers.progress,
          [SHORTCUTS.sla]: handlers.sla,
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
          [SHORTCUTS.discussion]: handlers.discussion,
          [SHORTCUTS.incident]: handlers.incident,
          [SHORTCUTS.forward]: handlers.forward,
          [SHORTCUTS.idea]: handlers.idea,
          [SHORTCUTS.createIssue]: handlers.createIssue,
          [SHORTCUTS.linkIssue]: handlers.linkIssue,
          [SHORTCUTS.article]: handlers.article,
          [SHORTCUTS.recipients]: handlers.recipients,
          [SHORTCUTS.signature]: handlers.signature,
          [SHORTCUTS.sendAs]: handlers.sendAs,
          [SHORTCUTS.trash]: handlers.trash,
        }
  )

  const sep = <span aria-hidden>·</span>
  const subject = c.kind === "feedback" ? "" : c.subject.trim()
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
        <div
          className="pointer-events-none absolute inset-2 z-30 grid place-items-center rounded-2xl border-2 border-dashed border-brand/60 bg-background/85 text-center"
          data-testid="drop-overlay"
        >
          <span className="flex flex-col items-center gap-2 text-reading font-medium text-brand">
            <PaperclipIcon className="size-7" />
            <Trans>Drop files to attach them to your reply</Trans>
          </span>
        </div>
      )}
      <header
        className="@container/person flex shrink-0 flex-col gap-2 border-b bg-background px-5 pt-3 pb-2.5 phone:px-3 phone:pt-2 phone:pb-2"
        data-testid="person"
      >
        <div className="flex min-w-0 items-center gap-2">
          <BackButton onBack={onBack} />
          <button
            type="button"
            onClick={togglePanel}
            className="flex min-w-0 flex-1 items-center gap-2.5 rounded-md text-start leading-[1.3] outline-none focus-visible:ring-2 focus-visible:ring-ring"
            title={t`Contact details (C)`}
          >
            {phone && (
              <span className="relative shrink-0">
                {visitor ? (
                  <VisitorAvatar className="size-9" />
                ) : (
                  <ContactAvatar id={c.contact_id} name={name} className="size-9" />
                )}
                {actions.live && (
                  <span
                    className="absolute -end-px -bottom-px size-3 rounded-full border-2 border-background bg-success"
                    data-testid="contact-live"
                  />
                )}
              </span>
            )}
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-[16px] font-semibold" data-testid={subject ? "header-subject" : undefined}>
                {subject || name}
              </span>
              <span
                className={cn(
                  "flex min-w-0 items-center gap-x-1.5 truncate text-caption text-faint",
                  phone && "flex-wrap gap-y-0.5 whitespace-normal"
                )}
              >
                {!phone && actions.live && (
                  <span className="size-2 shrink-0 rounded-full bg-success" data-testid="contact-live" aria-hidden />
                )}
                <span className="max-w-[60%] shrink-0 truncate" data-testid="person-name">
                  {name}
                </span>
                {inbox && (
                  <>
                    {sep}
                    <span className="inline-flex min-w-0 items-center gap-1 truncate">
                      <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />
                      <span className="truncate">{inbox.name}</span>
                    </span>
                  </>
                )}
                {source && (
                  <>
                    {sep}
                    <span className="shrink-0">{sourceText[source]}</span>
                  </>
                )}
                {place && (
                  <>
                    {sep}
                    <span className="min-w-0 truncate" data-testid="header-place">
                      {place}
                    </span>
                  </>
                )}
                {sep}
                <span className="truncate" data-testid="header-status">
                  <Status c={c} lastInbound={lastInbound} />
                </span>
              </span>
            </span>
          </button>
        </div>
        <div
          ref={actionsRef}
          role="toolbar"
          aria-label={t`Conversation actions`}
          className="flex flex-wrap items-center gap-1.5"
          data-testid="conversation-actions"
        >
          {!trashed && (
            <>
              <HandMenu
                c={c}
                actions={actions}
                open={menu === "hand"}
                onOpenChange={openMenu("hand")}
                label={
                  <span data-testid={assignee ? "header-assignee" : undefined}>
                    {assignee ? assignee.name || assignee.email : <Trans>Unassigned</Trans>}
                  </span>
                }
              />
              <TeamMenu c={c} open={menu === "team"} onOpenChange={openMenu("team")} />
              {!c.assignee_id && (
                <Button
                  variant="outline"
                  className={look.btnSm}
                  onClick={actions.claim}
                  title={t`Assign to me`}
                  data-testid="assign-me-button"
                >
                  <Trans>Assign to me</Trans>
                </Button>
              )}
              <SnoozeMenu name={name} actions={actions} open={menu === "snooze"} onOpenChange={openMenu("snooze")} />
              <SlaPopover c={c} open={menu === "sla"} onOpenChange={openMenu("sla")} />
              {c.kind === "feedback" && (
                <ProgressMenu c={c} update={update} open={menu === "progress"} onOpenChange={openMenu("progress")} />
              )}
              {c.kind === "feedback" && <IssueMenu onPick={setIssueMode} count={issues.length} />}
            </>
          )}
          <div className="ms-auto flex items-center gap-1.5">
            {!trashed && (
              <ConversationMenu
                c={c}
                open={moreOpen}
                onOpenChange={setMoreOpen}
                view={moreView}
                onView={setMoreView}
                update={update}
                move={move}
                pinned={pinned}
                onPin={togglePin}
                onUnread={markUnread}
                onDiscussion={handlers.discussion}
                onIncident={handlers.incident}
                onForward={() => setForwarding({})}
                onIdea={handlers.idea}
                onFind={() => (finding ? closeFind() : openFind())}
                onTrash={canManage ? trash : undefined}
              />
            )}
            <Button
              variant="outline"
              className={cn(look.btnSm, "w-[30px] px-0", shownPanel && "bg-muted")}
              onClick={togglePanel}
              aria-pressed={shownPanel}
              aria-label={t`Contact details (C)`}
              title={t`Contact details (C)`}
              data-testid="toggle-contact-panel"
            >
              <PanelRightIcon className="rtl:-scale-x-100" />
            </Button>
            {!trashed && <CloseButton c={c} actions={actions} />}
          </div>
        </div>
      </header>
      {(chipLabels.length > 0 ||
        (c.kind === "feedback" && c.feedback) ||
        pageLine ||
        c.related_conversation_id ||
        c.broadcast_id ||
        c.imported ||
        incidents.length > 0 ||
        !!c.idea ||
        issues.length > 0 ||
        viewers.length > 0 ||
        typingMate) && (
        <div
          className="flex shrink-0 flex-wrap items-center gap-x-1.5 gap-y-1 border-b bg-background px-4 py-1.5 text-caption text-faint"
          data-testid="header-details"
        >
          {c.imported && <ImportedChip at={c.imported.at} />}
          {c.broadcast_id && <BroadcastLine id={c.broadcast_id} />}
          {c.broadcast_id && (chipLabels.length > 0 || (c.kind === "feedback" && c.feedback) || pageLine) && sep}
          {c.kind === "feedback" && c.feedback && <CategoryChip category={c.feedback.category} inboxId={c.inbox_id} />}
          {c.kind === "feedback" && c.feedback && pageLine && sep}
          <PageLine c={c} />
          {pageLine && chipLabels.length > 0 && sep}
          {chipLabels.map((l) => (
            <span
              key={l.id}
              className="inline-flex items-center gap-1 rounded-full border bg-card px-2 py-px text-caption text-muted-foreground"
            >
              <Dot color={l.color} className="size-1.5" />
              {l.name}
            </span>
          ))}
          {c.related_conversation_id && (
            <>
              {(chipLabels.length > 0 || c.feedback || pageLine) && sep}
              <RelatedLine id={c.related_conversation_id} hrefFor={(id) => `/conversations/${id}`} />
            </>
          )}
          <IncidentChips conversationId={c.id} items={incidents} />
          {c.idea && <IdeaChip conversationId={c.id} idea={c.idea} />}
          <IssueChips subject={{ kind: "conversation", id: c.id }} items={issues} />
          <Watchers conversationId={c.id} />
        </div>
      )}
      {c.kind === "feedback" && !phone && <ProgressStrip progress={c.feedback?.progress} update={update} />}
      {finding && (
        <div
          className="flex shrink-0 items-center gap-1 border-b bg-background py-1.5 ps-3 pe-2"
          role="search"
          data-testid="find-bar"
        >
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
          <span
            className="shrink-0 px-1.5 text-caption text-faint tabular-nums"
            aria-live="polite"
            data-testid="find-count"
          >
            {q ? (hits.length ? t`${hitIndex + 1} of ${hits.length}` : t`No matches`) : ""}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => stepFind(-1)}
            disabled={hits.length < 2}
            aria-label={t`Older match`}
            title={t`Older match (Enter)`}
          >
            <ChevronUpIcon />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => stepFind(1)}
            disabled={hits.length < 2}
            aria-label={t`Newer match`}
            title={t`Newer match (⇧Enter)`}
          >
            <ChevronDownIcon />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={closeFind}
            aria-label={t`Close the search`}
            title={t`Close the search (Esc)`}
          >
            <XIcon />
          </Button>
        </div>
      )}
      {trashed && <TrashBanner c={c} onRestore={restore} busy={trasher.isPending} />}
      {c.spam && !trashed && (
        <Alert
          role="status"
          variant="destructive"
          className="flex shrink-0 items-center gap-3 rounded-none border-x-0 border-t-0 border-b bg-destructive/6 px-4 py-2 text-small text-foreground"
          data-testid="spam-banner"
        >
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
        older={{
          has: !!messages.hasNextPage,
          loading: messages.isFetchingNextPage,
          load: () => void messages.fetchNextPage(),
        }}
        pending={messages.isPending}
        error={messages.error}
        queued={queued}
        typing={contactTyping}
        lastId={lastId}
        lastMine={lastMine}
        focusId={focusId}
      />
      {!trashed && (
        <Composer>
          {beatenBy ? (
            <Beaten name={beatenBy} />
          ) : (
            showCollision && <TypingCollision name={mateName} typing={!!typingMate} onLeave={leaveTo} onClaim={claim} />
          )}
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
      )}
      {confirmDialog}
      {linking && (
        <LinkIncidentDialog
          conversationIds={[c.id]}
          linked={new Set(incidents.map((i) => i.id))}
          onClose={() => setLinking(false)}
        />
      )}
      {ideaLinking && (
        <Suspense fallback={null}>
          <LinkIdeaDialog c={c} onClose={() => setIdeaLinking(false)} />
        </Suspense>
      )}
      <IssueDialogs
        subject={{ kind: "conversation", id: c.id }}
        mode={issueMode}
        draft={feedbackDraft(c)}
        onClose={() => setIssueMode(null)}
      />
      {forwarding && (
        <ForwardDialog c={c} target={forwarding.target} messages={shown} onClose={() => setForwarding(null)} />
      )}
      {discussing && <FromConversationDialog c={c} messages={shown} onClose={() => setDiscussing(false)} />}
      {!inlinePanel && (
        <ContactSheet contactId={c.contact_id} conversationId={c.id} open={sheet} onOpenChange={setSheet} />
      )}
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

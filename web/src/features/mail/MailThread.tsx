import { Plural, Trans, useLingui } from "@lingui/react/macro"
import {
  ArchiveIcon,
  ArrowLeftIcon,
  ClockIcon,
  EllipsisIcon,
  FileDownIcon,
  FilterIcon,
  ForwardIcon,
  InboxIcon,
  MailIcon,
  PanelRightIcon,
  ReplyAllIcon,
  ReplyIcon,
  SearchXIcon,
  StarIcon,
  StickyNoteIcon,
  TagIcon,
  Trash2Icon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { Dot, EmptyState, PersonAvatar, toast, useContactName } from "@/components/common"
import { FoldPanel } from "@/components/common/Fold"
import { usePhoneBackTarget } from "@/components/common/PageLayout"
import { keyLabel, MAIL_KEYS } from "@/components/common/ShortcutSheet"
import { formatShort, useEnumText, useErrorText } from "@/components/common/text"
import { useConfirm } from "@/components/common/ConfirmDialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useContact } from "@/features/contact/queries"
import {
  readDraft,
  useConversationActions,
  useDraftRevision,
  usePendingReplies,
  type ConversationActions,
} from "@/features/conversation/actions"
import { ReplyBox, type ReplyHandle } from "@/features/conversation/composer/ReplyBox"
import { ForwardDialog } from "@/features/conversation/ForwardDialog"
import { useSnoozeTimes } from "@/features/conversation/HeaderActions"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { MailFold, MailRow } from "@/features/conversation/messages/MailRow"
import { EventLine } from "@/features/conversation/messages/EventLine"
import { OwnDraftCard } from "@/features/conversation/messages/OwnDraftCard"
import { isOwnDraft, isShownDraft } from "@/features/conversation/ownDraft"
import { RedactedLine } from "@/features/conversation/messages/Redacted"
import { TrashBanner } from "@/features/conversation/Trash"
import {
  useConversation,
  useDraftActions,
  useMarkRead,
  useMessages,
  useParticipants,
  useTrashConversation,
  useUpdateConversation,
} from "@/features/conversation/queries"
import { useListActions } from "@/features/inbox/memberState"
import { useFilterAccess, type Folder } from "@/features/mail/folders"
import { MailPanel, type MailFold as MailPart } from "@/features/mail/MailSide"
import { useStar } from "@/features/mail/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useMediaQuery } from "@/hooks/use-media-query"
import {
  ApiError,
  isGone,
  rawMessageUrl,
  type Contact,
  type Conversation,
  type ConversationUpdate,
  type Message,
} from "@/lib/api"
import { writesMailbox } from "@/lib/mailbox"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

type Props = {
  id: string
  onBack?: () => void
  onLeave: () => void
  openDraft?: string
  box?: string
  folder?: Folder
  panel?: boolean
  inlinePanel?: boolean
  onPanel?: (open: boolean) => void
}

function BackButton({ onBack }: { onBack?: () => void }) {
  const { t } = useLingui()
  usePhoneBackTarget(onBack ? { label: t`Back to the list`, onClick: onBack } : undefined)
  if (!onBack) return null
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="max-md:hidden"
      onClick={onBack}
      aria-label={t`Back to the list`}
      title={t`Back to the list`}
      data-testid="mail-back"
    >
      <ArrowLeftIcon className="rtl:rotate-180" />
    </Button>
  )
}

export function MailThread(props: Props) {
  const errorText = useErrorText()
  const conversation = useConversation(props.id)
  const c = conversation.data
  const contact = useContact(c?.contact_id)
  if (conversation.isPending) {
    return (
      <div className="flex min-h-0 flex-1 flex-col" data-testid="mail-loading">
        <div className="flex min-h-15 items-center gap-3 border-b border-border/60 px-6 phone:px-3">
          <BackButton onBack={props.onBack} />
          <div className="flex flex-1 flex-col gap-1.5">
            <Skeleton className="h-4 w-[55%]" />
            <Skeleton className="h-3 w-[30%]" />
          </div>
        </div>
        <div className="flex flex-1 flex-col gap-4 bg-background px-7 py-5 phone:px-4">
          <Skeleton className="h-40 w-full rounded-xl" />
        </div>
      </div>
    )
  }
  if (!c || isGone(conversation.error)) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {props.onBack && (
          <div className="flex h-15 items-center border-b border-border/60 px-3">
            <BackButton onBack={props.onBack} />
          </div>
        )}
        <EmptyState icon={SearchXIcon} title={<Trans>This mail is not available</Trans>}>
          <p>{errorText(conversation.error ?? new ApiError(404))}</p>
        </EmptyState>
      </div>
    )
  }
  return <Loaded {...props} c={c} contact={contact.data} />
}

function NoteRow({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { i18n } = useLingui()
  const author = useAuthorName(m, ctx)
  return (
    <article
      className="border-t border-line-soft bg-note px-[18px] py-3 shadow-[inset_3px_0_0_var(--color-note-border)] phone:px-3"
      data-testid="mail-note"
      data-message-id={m.id}
    >
      <span className="flex flex-wrap items-center gap-x-2 text-small text-muted-foreground">
        <PersonAvatar name={author} className="size-6 bg-muted-foreground text-[10px] text-background" />
        <b className="font-semibold text-foreground">{author}</b>
        <span>
          <Trans>Note for the team</Trans>
        </span>
        <span className="flex-1" />
        <time dateTime={m.created_at}>{formatShort(m.created_at, i18n.locale)}</time>
      </span>
      <div className="mt-1.5 ms-[34px] text-reading break-words whitespace-pre-wrap phone:ms-0">{m.body}</div>
      <div className="ms-[34px] empty:hidden phone:ms-0">
        <Attachments items={m.attachments} />
      </div>
    </article>
  )
}

function MailSnooze({
  actions,
  open,
  onOpenChange,
}: {
  actions: ConversationActions
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useLingui()
  const times = useSnoozeTimes()
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger
        render={<Button variant="outline" className="h-[30px] gap-1.5 rounded-lg border-input px-2.5 font-normal" />}
        aria-label={t`Snooze (B)`}
        title={t`Snooze (B)`}
        data-testid="mail-snooze"
      >
        <ClockIcon className="text-muted-foreground" />
        <span className="phone:hidden">
          <Trans>Snooze</Trans>
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[260px] max-w-[calc(100vw-32px)]">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>When should it come back?</Trans>
          </DropdownMenuLabel>
          {times.map((o) => (
            <DropdownMenuItem key={o.label} onClick={() => actions.snooze(o.at, o.when)}>
              <span className="flex-1">{o.label}</span>
              {o.detail && <span className="text-caption text-faint">{o.detail}</span>}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Loaded({
  c,
  contact,
  onBack,
  onLeave,
  openDraft,
  box,
  folder,
  panel,
  inlinePanel,
  onPanel,
}: Props & { c: Conversation; contact?: Contact }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage, membership, workspaceId } = useSession()
  const text = useEnumText()
  const me = membership.member_id
  const members = useMemberMap()
  const labels = useLabels().data ?? []
  const inboxes = useInboxes().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const write = writesMailbox(inbox)
  const channel = useChannelMap().get(c.channel_id ?? "")
  const messages = useMessages(c.id)
  const contactName = useContactName()
  const name = contact ? contactName(c, channel, contact.name, contact.emails[0]) : "…"
  const actions = useConversationActions(c, name)
  const trasher = useTrashConversation(c.id)
  const listActions = useListActions()
  const star = useStar()
  const [confirm, confirmDialog] = useConfirm()
  const reply = useRef<ReplyHandle>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const [snoozing, setSnoozing] = useState(false)
  const navigate = useNavigate()
  const setFiltering = (messageId: string) =>
    navigate(`/mail/${c.inbox_id}/filters?message=${encodeURIComponent(messageId)}`)
  const mayFilter = useFilterAccess(c.inbox_id)
  const [timing, setTiming] = useState<string | null>(null)
  const draftActions = useDraftActions()
  const [expandQuoted, setExpandQuoted] = useState(false)
  const fine = useMediaQuery("(pointer: fine)")
  const revision = useDraftRevision(c.id)
  const queued = usePendingReplies(c.id)
  const trashed = !!c.trashed_at
  const archived = c.status === "closed"
  const starred = !!c.starred

  const items = useMemo(
    () => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()),
    [messages.data]
  )
  const suggestion = items.findLast((m) => m.draft && m.kind !== "forward" && !isOwnDraft(m, me))
  const shown = items.filter((m) => !m.draft)
  const cards = useMemo(
    () =>
      items
        .filter((m) => isShownDraft(m, me))
        .toSorted((a, b) => (a.scheduled_at ?? "~").localeCompare(b.scheduled_at ?? "~")),
    [items, me]
  )
  const nextCard = cards[0]
  const cardCount = cards.length
  const edited = useRef(false)
  const editTarget = openDraft ? items.find((m) => m.id === openDraft) : undefined
  useEffect(() => {
    if (!editTarget || edited.current) return
    edited.current = true
    reply.current?.edit(editTarget)
  }, [editTarget])
  useEffect(() => {
    if (cardCount === 0) return
    const down = () => scroller.current?.scrollTo({ top: scroller.current.scrollHeight })
    const timers = [0, 200, 600].map((ms) => setTimeout(down, ms))
    return () => timers.forEach(clearTimeout)
  }, [cardCount])
  const mails = shown.filter((m) => m.kind === "message" || m.kind === "forward")
  const lastMail = mails.at(-1)
  const [opened, setOpened] = useState<Record<string, boolean>>({})
  const [allOpen, setAllOpen] = useState(false)
  const isOpen = (m: Message) => opened[m.id] ?? (allOpen || m.id === lastMail?.id || !!m.delivery?.error)
  const toggleOne = (m: Message) => setOpened((o) => ({ ...o, [m.id]: !isOpen(m) }))
  const [moreOpen, setMoreOpen] = useState(false)
  const [forwarding, setForwarding] = useState<{ target?: Message } | null>(null)
  const [sheet, setSheet] = useState(false)
  const [folds, setFolds] = useState<Record<MailPart, boolean>>({ details: true, thread: true, before: true })
  const setFold = (part: MailPart, open: boolean) => setFolds((f) => ({ ...f, [part]: open }))
  const shownPanel = inlinePanel ? !!panel : sheet
  const setPanel = (open: boolean) => (inlinePanel ? onPanel?.(open) : setSheet(open))
  const togglePanel = () => setPanel(!shownPanel)
  const updater = useUpdateConversation(c.id)

  const contactEmails = contact?.emails.map((e) => e.toLowerCase()) ?? []
  const inbound = shown.filter((m) => m.kind === "message" && m.direction !== "out")
  const filterable = (m: Message) => mayFilter && !trashed && !m.draft && !!m.email && m.direction === "in"
  const latestFilterable = shown.findLast((m) => m.kind === "message" && filterable(m))
  const lastContactMail = inbound.findLast(
    (m) => m.author.type === "contact" && !!m.email?.from && contactEmails.includes(m.email.from.toLowerCase())
  )
  const emailTo = channel?.kind === "email" ? (lastContactMail?.email?.from ?? contact?.emails[0]) : undefined
  const undeliverable = !!emailTo && !!contact?.undeliverable.some((u) => u.email === emailTo)

  const participants = useParticipants(c.id, c.channel_kind === "email")
  const ctx: ThreadContext = {
    members,
    contact,
    participants: participants.data?.items,
    labels,
    inboxes,
    subject: c.subject,
    expandQuoted,
    inboxId: c.inbox_id,
    contactReadAt: c.last_read_by_contact_at,
    onReply: write && !trashed ? (_m, all) => reply.current?.replyAll(all) : undefined,
    onForwardMail: write && !trashed ? (m) => setForwarding({ target: m }) : undefined,
  }

  const lastId = shown.at(-1)?.id
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
  const restore = () =>
    trasher.mutate(false, {
      onSuccess: () => toast(t`Restored from the Trash`, () => trasher.mutate(true, { onError: fail })),
      onError: fail,
    })
  const trash = () =>
    confirm({
      title: <Trans>Move this mail to the Trash?</Trans>,
      description: (
        <Trans>
          You can restore it from the Trash for 30 days, then it is deleted for good with its messages and files.
        </Trans>
      ),
      confirm: <Trans>Move to the Trash</Trans>,
      run: () =>
        trasher.mutate(true, {
          onSuccess: () => {
            toast(t`Moved to the Trash`, () => trasher.mutate(false, { onError: fail }))
            onLeave()
          },
          onError: fail,
        }),
    })
  const archive = () => {
    if (archived) void actions.reopen()
    else {
      void actions.close()
      onLeave()
    }
  }
  const markUnread = () => {
    marked.current = lastId ?? null
    listActions.markUnread(c.id)
    toast(t`Marked as unread`, () => listActions.markRead(c.id))
    onLeave()
  }
  const toggleStar = () => write && star([c.id], !starred)
  const update = (body: ConversationUpdate) => updater.mutate(body, { onError: fail })
  const editLabels = () => {
    setFold("details", true)
    setPanel(true)
  }
  const replyTo = (all: boolean) => {
    if (write) reply.current?.replyAll(all)
  }
  const editDraft = (m: Message) => reply.current?.edit(m)
  const cancelSchedule = (m: Message) =>
    draftActions.unschedule.mutate(m, {
      onSuccess: (draft) => {
        reply.current?.adopt(draft)
        toast(t`Schedule cancelled`)
      },
      onError: fail,
    })
  const cardKeys: Record<string, () => void> = nextCard
    ? nextCard.scheduled_at
      ? {
          [MAIL_KEYS.editScheduled]: () => editDraft(nextCard),
          [MAIL_KEYS.changeTime]: () => setTiming(nextCard.id),
          [MAIL_KEYS.cancelSchedule]: () => cancelSchedule(nextCard),
        }
      : {
          [MAIL_KEYS.editScheduled]: () => editDraft(nextCard),
          [MAIL_KEYS.sendAgain]: () => draftActions.send.mutate(nextCard, { onError: fail }),
        }
    : {}

  useHotkeys(
    trashed
      ? {
          [MAIL_KEYS.quoted]: () => setExpandQuoted((x) => !x),
          [MAIL_KEYS.restore]: () => write && canManage && restore(),
          [MAIL_KEYS.expand]: () => setAllOpen((x) => !x),
          [MAIL_KEYS.details]: togglePanel,
        }
      : {
          [MAIL_KEYS.reply]: () => replyTo(false),
          [MAIL_KEYS.replyAll]: () => replyTo(true),
          [MAIL_KEYS.note]: () => write && reply.current?.focus("note"),
          [MAIL_KEYS.attach]: () => write && reply.current?.attach(),
          [MAIL_KEYS.design]: () => write && reply.current?.design(),
          [MAIL_KEYS.archive]: () => write && archive(),
          [MAIL_KEYS.trash]: () => write && canManage && trash(),
          [MAIL_KEYS.star]: toggleStar,
          [MAIL_KEYS.unread]: markUnread,
          [MAIL_KEYS.snooze]: () => write && setSnoozing(true),
          [MAIL_KEYS.filter]: () => write && latestFilterable && setFiltering(latestFilterable.id),
          [MAIL_KEYS.quoted]: () => setExpandQuoted((x) => !x),
          [MAIL_KEYS.expand]: () => setAllOpen((x) => !x),
          [MAIL_KEYS.details]: togglePanel,
          [MAIL_KEYS.more]: () => write && setMoreOpen(true),
          [MAIL_KEYS.forward]: () => write && setForwarding({}),
          [MAIL_KEYS.labels]: () => write && editLabels(),
          ...(write ? cardKeys : {}),
        }
  )

  const subject = c.subject.trim() || t`(no subject)`
  const mailCount = mails.length
  const address = inbox?.mailbox?.addresses[0]
  const latestRaw = shown.findLast((m) => m.kind === "message" && !!m.email?.raw)
  const statusTone = archived
    ? "bg-seg text-fg2"
    : c.status === "snoozed"
      ? "bg-warning/10 text-warning"
      : "bg-success/10 text-success"
  const chipLabels = labels.filter((l) => c.labels.includes(l.id))
  const detailsKey = keyLabel(MAIL_KEYS.details).join("")
  const starButton = (
    <Button
      variant="ghost"
      size="icon-sm"
      disabled={!write}
      onClick={toggleStar}
      className={cn("size-[30px]", starred ? "text-warning hover:text-warning" : "text-muted-foreground")}
      aria-pressed={starred}
      aria-label={starred ? t`Remove the star` : t`Star`}
      title={starred ? t`Remove the star (S)` : t`Star (S)`}
      data-testid="mail-thread-star"
    >
      <StarIcon className={cn("size-[18px]", starred && "fill-current")} />
    </Button>
  )
  const moreMenu = write && !trashed && (
    <DropdownMenu open={moreOpen} onOpenChange={setMoreOpen}>
      <DropdownMenuTrigger
        render={<Button variant="outline" className="h-[30px] w-[30px] rounded-lg border-input px-0" />}
        aria-label={t`Mail actions`}
        title={t`Mail actions (${MAIL_KEYS.more})`}
        data-testid="mail-more"
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[260px] max-w-[calc(100vw-32px)]">
        <DropdownMenuItem onClick={markUnread} data-testid="mail-unread">
          <MailIcon />
          <span className="flex-1">
            <Trans>Mark as unread</Trans>
          </span>
          <DropdownMenuShortcut>{MAIL_KEYS.unread}</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={editLabels} data-testid="mail-labels">
          <TagIcon />
          <span className="flex-1">{t`Labels`}…</span>
          <DropdownMenuShortcut>{MAIL_KEYS.labels}</DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => setForwarding({})} data-testid="mail-forward-menu">
          <ForwardIcon className="rtl:-scale-x-100" />
          <span className="flex-1">
            <Trans>Forward</Trans>
          </span>
          <DropdownMenuShortcut>{MAIL_KEYS.forward}</DropdownMenuShortcut>
        </DropdownMenuItem>
        {latestFilterable && (
          <DropdownMenuItem onClick={() => setFiltering(latestFilterable.id)} data-testid="mail-filter">
            <FilterIcon />
            <span className="flex-1">
              <Trans>Filter messages like this</Trans>
            </span>
            <DropdownMenuShortcut>{MAIL_KEYS.filter}</DropdownMenuShortcut>
          </DropdownMenuItem>
        )}
        {latestRaw && (
          <DropdownMenuItem
            render={<a href={rawMessageUrl(latestRaw.id, workspaceId)} download />}
            data-testid="mail-raw"
          >
            <FileDownIcon />
            <span className="flex-1">
              <Trans>Download the original e-mail (.eml)</Trans>
            </span>
          </DropdownMenuItem>
        )}
        {canManage && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={trash} data-testid="mail-trash">
              <Trash2Icon />
              <span className="flex-1">
                <Trans>Move to the Trash</Trans>
              </span>
              <DropdownMenuShortcut>{MAIL_KEYS.trash}</DropdownMenuShortcut>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
  const toolbar = (
    <div
      role="toolbar"
      aria-label={t`Mail actions`}
      className="flex flex-wrap items-center gap-1.5 phone:w-full"
      data-testid="mail-actions"
    >
      {starButton}
      {!trashed && write && <MailSnooze actions={actions} open={snoozing} onOpenChange={setSnoozing} />}
      {!trashed && !write && (
        <Button
          variant="outline"
          className="h-[30px] gap-1.5 rounded-lg border-input px-2.5 font-normal"
          onClick={markUnread}
          title={t`Mark as unread (U)`}
          data-testid="mail-unread"
        >
          <MailIcon />
          <span className="phone:hidden">
            <Trans>Mark as unread</Trans>
          </span>
        </Button>
      )}
      <div className="ms-auto flex items-center gap-1.5">
        {!trashed && write && (
          <Button
            variant={archived ? "outline" : "default"}
            className="h-[30px] rounded-lg px-3 font-semibold"
            onClick={archive}
            title={archived ? t`Move to the Inbox (E)` : t`Archive (E)`}
            data-testid="mail-archive"
          >
            {archived ? <InboxIcon /> : <ArchiveIcon />}
            {archived ? <Trans>Move to the Inbox</Trans> : <Trans>Archive</Trans>}
          </Button>
        )}
        {moreMenu}
        <Button
          variant="outline"
          className={cn("h-[30px] w-[30px] rounded-lg border-input px-0", shownPanel && "bg-muted")}
          onClick={togglePanel}
          aria-pressed={shownPanel}
          aria-label={`${t`Details`} (${detailsKey})`}
          title={`${t`Details`} (${detailsKey})`}
          data-testid="mail-toggle-details"
        >
          <PanelRightIcon className="rtl:-scale-x-100" />
        </Button>
      </div>
    </div>
  )
  const middleMails = mails.slice(1, -1)
  const folding = !allOpen && mails.length >= 4 && middleMails.every((m) => !isOpen(m))
  const hidden = folding
    ? new Set(
        shown
          .slice(shown.indexOf(mails[1]), shown.indexOf(mails[mails.length - 2]) + 1)
          .filter((m) => m.kind !== "note")
          .map((m) => m.id)
      )
    : new Set<string>()
  const foldAt = folding ? shown.findIndex((m) => hidden.has(m.id)) : -1
  const messageList = shown.map((m, i) => {
    if (hidden.has(m.id))
      return i === foldAt ? (
        <MailFold
          key="mail-fold"
          count={middleMails.length}
          onOpen={() => setAllOpen(true)}
          shortcut={MAIL_KEYS.expand}
        />
      ) : null
    if (m.redacted_at)
      return (
        <div key={m.id} className="border-t border-line-soft px-[18px] py-1.5 phone:px-3">
          <RedactedLine m={m} ctx={ctx} />
        </div>
      )
    if (m.kind === "note") return <NoteRow key={m.id} m={m} ctx={ctx} />
    if (m.kind === "event")
      return (
        <div key={m.id} className="border-t border-line-soft px-[18px] py-1 phone:px-3">
          <EventLine m={m} ctx={ctx} />
        </div>
      )
    return (
      <MailRow
        key={m.id}
        m={m}
        ctx={ctx}
        name={name}
        open={isOpen(m)}
        onToggle={() => toggleOne(m)}
        onFilter={m.kind === "message" && filterable(m) ? () => setFiltering(m.id) : undefined}
        keys={{ reply: MAIL_KEYS.reply, replyAll: MAIL_KEYS.replyAll, forward: MAIL_KEYS.forward }}
      />
    )
  })
  const after = (
    <>
      {cards.map((m) => (
        <OwnDraftCard
          key={m.id}
          m={m}
          keys={
            m.id === nextCard?.id
              ? {
                  edit: MAIL_KEYS.editScheduled,
                  time: MAIL_KEYS.changeTime,
                  cancel: MAIL_KEYS.cancelSchedule,
                  again: MAIL_KEYS.sendAgain,
                }
              : undefined
          }
          timeOpen={timing === m.id}
          onTimeOpenChange={(open) => setTiming(open ? m.id : null)}
          onEdit={editDraft}
          onCancel={cancelSchedule}
        />
      ))}
      {queued.map((p) => (
        <div
          key={p.key}
          className="rounded-[14px] border border-dashed bg-card px-5 py-3 opacity-70"
          data-testid="pending-reply"
        >
          <p className="text-reading break-words whitespace-pre-wrap">{p.body}</p>
          <p className="mt-1 text-caption text-faint">
            {p.sendAt ? (
              <Trans>Scheduling…</Trans>
            ) : p.close ? (
              <Trans>Sending, then archiving…</Trans>
            ) : (
              <Trans>Sending…</Trans>
            )}
          </p>
        </div>
      ))}
    </>
  )
  const barButton = "h-[30px] gap-1.5 rounded-lg border-input px-2.5 font-normal"
  const replyBar = (
    <div
      className="flex shrink-0 flex-wrap items-center gap-2 border-t px-[18px] py-2.5 phone:px-3"
      data-testid="mail-reply-modes"
    >
      <Button
        variant="outline"
        className={barButton}
        onClick={() => replyTo(false)}
        title={t`Reply (R)`}
        data-testid="mail-reply"
      >
        <ReplyIcon />
        <Trans>Reply</Trans>
      </Button>
      <Button
        variant="outline"
        className={barButton}
        onClick={() => replyTo(true)}
        title={t`Reply all (A)`}
        data-testid="mail-reply-all"
      >
        <ReplyAllIcon />
        <Trans>Reply all</Trans>
      </Button>
      <Button
        variant="outline"
        className={barButton}
        onClick={() => setForwarding({})}
        title={t`Forward (${MAIL_KEYS.forward})`}
        data-testid="mail-forward"
      >
        <ForwardIcon className="rtl:-scale-x-100" />
        <Trans>Forward</Trans>
      </Button>
      <span className="flex-1" />
      <Button
        variant="outline"
        className={barButton}
        onClick={() => reply.current?.focus("note")}
        title={t`Write a note for the team`}
        data-testid="mail-note-button"
      >
        <StickyNoteIcon />
        <Trans>Note for the team</Trans>
      </Button>
    </div>
  )
  const panelBody = (
    <MailPanel
      c={c}
      name={name}
      mails={mails}
      update={update}
      box={box}
      folder={folder}
      folds={folds}
      onFold={setFold}
    />
  )
  const inlineShown = !!inlinePanel && !!panel
  return (
    <section
      aria-label={subject}
      className="relative flex min-h-0 min-w-0 flex-1 flex-col"
      data-testid="mail-thread"
      data-conversation-id={c.id}
    >
      <header className="flex shrink-0 flex-wrap items-start gap-x-3 gap-y-2" data-testid="mail-header">
        <BackButton onBack={onBack} />
        <div className="min-w-0 flex-[1_1_320px] phone:flex-[1_1_0]">
          <h1
            className="m-0 text-title font-semibold [overflow-wrap:anywhere] phone:text-[19px] phone:leading-[25px]"
            data-testid="mail-thread-subject"
          >
            {subject}
          </h1>
          <div
            className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1.5 text-small text-muted-foreground"
            data-testid="mail-thread-lede"
          >
            <span
              className={cn(
                "inline-flex h-[22px] items-center rounded-[5px] px-2 text-caption font-semibold whitespace-nowrap",
                statusTone
              )}
            >
              {text.statusOf(c.status, c.kind)}
            </span>
            <span className="min-w-0 truncate">{name}</span>
            {(address || inbox) && (
              <>
                <span aria-hidden>·</span>
                <span className="min-w-0 truncate">{address ?? inbox?.name}</span>
              </>
            )}
            {mailCount > 0 && (
              <>
                <span aria-hidden>·</span>
                <span>
                  <Plural value={mailCount} one="# message" other="# messages" />
                </span>
              </>
            )}
            {chipLabels.map((l) => (
              <span
                key={l.id}
                className="inline-flex h-[22px] items-center gap-1 rounded-[5px] bg-seg px-2 text-caption font-medium text-fg2 phone:hidden"
              >
                <Dot color={l.color} className="size-1.5" />
                {l.name}
              </span>
            ))}
          </div>
        </div>
        {toolbar}
      </header>
      <div
        className={cn(
          "mt-3.5 grid min-h-0 flex-1 grid-rows-[minmax(0,1fr)] gap-3.5 phone:mt-2.5",
          inlineShown ? "grid-cols-[minmax(0,1fr)_320px] max-[1320px]:grid-cols-[minmax(0,1fr)_280px]" : "grid-cols-1"
        )}
      >
        <section
          aria-label={t`Messages`}
          className="relative flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl border bg-card"
        >
          {trashed && <TrashBanner c={c} onRestore={restore} busy={trasher.isPending} />}
          {!trashed && !write && (
            <p
              role="status"
              className="shrink-0 border-b border-line-soft bg-subtle px-3.5 py-2 text-small text-fg2"
              data-testid="mail-read-only"
            >
              <Trans>You can read this mailbox but not change it. Its owner can let you send from it.</Trans>
            </p>
          )}
          <div
            ref={scroller}
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain [&>:first-child]:border-t-0"
            data-testid="mail-messages"
          >
            {messages.hasNextPage && (
              <div className="flex justify-center py-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void messages.fetchNextPage()}
                  disabled={messages.isFetchingNextPage}
                >
                  <Trans>Load older messages</Trans>
                </Button>
              </div>
            )}
            {messages.isPending && <Skeleton className="m-4 h-40 rounded-xl" />}
            {messageList}
            <div className="flex flex-col gap-2 px-[18px] pb-3 empty:hidden phone:px-3">{after}</div>
          </div>
          {!trashed && write && replyBar}
          {!trashed && write && (
            <div className="shrink-0 border-t phone:pb-[env(safe-area-inset-bottom)]">
              <ReplyBox
                key={`${c.id}:${revision}`}
                ref={reply}
                c={c}
                contactName={name}
                emailTo={emailTo}
                undeliverable={undeliverable}
                suggestion={suggestion}
                ctx={ctx}
                actions={actions}
                autoFocus={fine && revision === 0 && !!readDraft(c.id).body}
                bare
              />
            </div>
          )}
        </section>
        {inlineShown && (
          <FoldPanel label={t`Details`} testId="mail-panel">
            {panelBody}
          </FoldPanel>
        )}
      </div>
      {!inlinePanel && (
        <Sheet open={sheet} onOpenChange={setSheet}>
          <SheetContent
            className="w-[min(360px,92vw)] overflow-y-auto phone:inset-x-0 phone:top-auto phone:bottom-0 phone:max-h-[82svh] phone:w-full phone:rounded-t-2xl phone:border-s-0 phone:border-t"
            data-testid="mail-panel"
          >
            <SheetHeader className="py-3">
              <SheetTitle className="text-body font-semibold">
                <Trans>Details</Trans>
              </SheetTitle>
            </SheetHeader>
            {panelBody}
          </SheetContent>
        </Sheet>
      )}
      {forwarding && (
        <ForwardDialog c={c} target={forwarding.target} messages={shown} onClose={() => setForwarding(null)} />
      )}
      {confirmDialog}
    </section>
  )
}

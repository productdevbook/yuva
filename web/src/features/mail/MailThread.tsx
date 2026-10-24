import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArchiveIcon,
  ArrowLeftIcon,
  ChevronDownIcon,
  ClockIcon,
  FileDownIcon,
  FilterIcon,
  InboxIcon,
  MailIcon,
  PaperclipIcon,
  ReplyAllIcon,
  ReplyIcon,
  SearchXIcon,
  StarIcon,
  StickyNoteIcon,
  Trash2Icon,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { EmptyState, Kbd, Notice, toast, useContactName } from "@/components/common"
import { MAIL_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, useErrorText } from "@/components/common/text"
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
import { useSnoozeTimes } from "@/features/conversation/HeaderActions"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { DeliveryState } from "@/features/conversation/messages/DeliveryState"
import { MailBody, MailControls, useMail } from "@/features/conversation/messages/EmailBody"
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
} from "@/features/conversation/queries"
import { look } from "@/features/inbox/PageFrame"
import { useListActions } from "@/features/inbox/memberState"
import { FilterDialog } from "@/features/mail/FilterDialog"
import { useFilterAccess } from "@/features/mail/folders"
import { useStar } from "@/features/mail/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useMediaQuery } from "@/hooks/use-media-query"
import { ApiError, isGone, rawMessageUrl, type Contact, type Conversation, type Message } from "@/lib/api"
import { writesMailbox } from "@/lib/mailbox"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

type Props = { id: string; onBack?: () => void; onLeave: () => void; openDraft?: string }

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
        <div className="flex h-15 items-center gap-3 border-b px-4">
          <BackButton onBack={props.onBack} />
          <Skeleton className="h-5 w-64" />
        </div>
        <div className="flex flex-1 flex-col gap-3 bg-surface p-6">
          <Skeleton className="h-16 w-full rounded-xl" />
          <Skeleton className="h-40 w-full rounded-xl" />
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
        <EmptyState icon={SearchXIcon} title={<Trans>This mail is not available</Trans>}>
          <p>{errorText(conversation.error ?? new ApiError(404))}</p>
        </EmptyState>
      </div>
    )
  }
  return <Loaded {...props} c={c} contact={contact.data} />
}

function Address({ label, value, testId }: { label: React.ReactNode; value: string; testId: string }) {
  if (!value) return null
  return (
    <>
      <dt className="text-faint">{label}</dt>
      <dd className="min-w-0 break-words" data-testid={testId}>
        {value}
      </dd>
    </>
  )
}

function MailCard({
  m,
  ctx,
  contactName,
  open,
  onToggle,
  onFilter,
}: {
  m: Message
  ctx: ThreadContext
  contactName: string
  open: boolean
  onToggle: () => void
  onFilter?: () => void
}) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const mail = useMail(m, ctx.expandQuoted)
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  const out = m.direction === "out"
  const e = m.email
  const owner = m.on_behalf_of_member_id ? ctx.members.get(m.on_behalf_of_member_id) : undefined
  const ownerName = owner ? owner.name || owner.email : t`Deleted member`
  const name = !out
    ? contactName
    : m.on_behalf_of_member_id
      ? t`${author} on behalf of ${ownerName}`
      : m.sent_by
        ? t`${author}, sent by ${by}`
        : author
  const date = formatDateTime(m.created_at, i18n.locale)
  const snippet = m.body.replace(/\s+/g, " ").trim()
  if (!open) {
    return (
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full min-w-0 items-center gap-3 rounded-xl border bg-card px-4 py-2.5 text-start outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-ring"
        aria-expanded={false}
        data-testid="mail-message"
        data-collapsed
        data-message-id={m.id}
      >
        <span className="w-40 shrink-0 truncate text-body font-medium phone:w-28">{name}</span>
        <span className="min-w-0 flex-1 truncate text-small text-muted-foreground">{snippet}</span>
        {m.attachments.length > 0 && <PaperclipIcon className="size-3.5 shrink-0 text-faint" />}
        <time className="shrink-0 text-caption text-faint" dateTime={m.created_at}>
          {date}
        </time>
      </button>
    )
  }
  return (
    <article
      className="flex min-w-0 flex-col rounded-xl border bg-card"
      tabIndex={-1}
      data-testid="mail-message"
      data-message-id={m.id}
    >
      <header className="flex min-w-0 flex-col gap-1 border-b px-4 pt-3 pb-2.5">
        <button
          type="button"
          onClick={onToggle}
          className="flex min-w-0 items-baseline gap-2 rounded-md text-start outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-expanded
          title={t`Collapse`}
        >
          <span className="min-w-0 flex-1 truncate phone:whitespace-normal phone:break-words">
            <span className="text-body font-semibold" data-testid="mail-from-name">
              {name}
            </span>
            {e?.from && (
              <span className="ms-1.5 text-small text-faint" data-testid="mail-from">
                &lt;{e.from}&gt;
              </span>
            )}
          </span>
          <time className="shrink-0 text-caption text-faint" dateTime={m.created_at}>
            {date}
          </time>
        </button>
        {e && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-small text-muted-foreground">
            <Address label={<Trans>To</Trans>} value={e.to.join(", ")} testId="mail-to" />
            <Address label={<Trans>Cc</Trans>} value={e.cc?.join(", ") ?? ""} testId="mail-cc" />
            <Address label={<Trans>Bcc</Trans>} value={e.bcc?.join(", ") ?? ""} testId="mail-bcc" />
          </dl>
        )}
      </header>
      <div className={cn("min-w-0", mail.originalHtml || mail.waiting ? "" : "px-4 py-3")}>
        <MailBody mail={mail} />
      </div>
      {(mail.listed.length > 0 || m.delivery) && (
        <div className="flex flex-col gap-2 px-4 pb-3">
          <Attachments items={mail.listed} />
          {m.delivery && <DeliveryState d={m.delivery} />}
        </div>
      )}
      <footer className="flex flex-wrap items-center gap-1 border-t px-2 py-1">
        <MailControls mail={mail} />
        {onFilter && (
          <button
            type="button"
            onClick={onFilter}
            className="ms-auto inline-flex h-6 items-center gap-1 rounded-md px-1.5 text-small text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="mail-filter-like"
          >
            <FilterIcon className="size-3.5" />
            <Trans>Filter messages like this</Trans>
          </button>
        )}
        {e?.raw && (
          <a
            href={rawMessageUrl(m.id, workspaceId)}
            download
            className={cn(
              "inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
              !onFilter && "ms-auto"
            )}
            title={t`Download the original e-mail (.eml)`}
            data-testid="raw-download"
          >
            <FileDownIcon className="size-3.5" />
            <span className="sr-only">
              <Trans>Download the original e-mail (.eml)</Trans>
            </span>
          </a>
        )}
      </footer>
    </article>
  )
}

function NoteCard({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { i18n } = useLingui()
  const author = useAuthorName(m, ctx)
  return (
    <article
      className="flex min-w-0 flex-col gap-1 rounded-xl border border-warning/30 bg-warning/6 px-4 py-2.5"
      data-testid="mail-note"
      data-message-id={m.id}
    >
      <span className="flex items-center gap-1.5 text-caption text-faint">
        <StickyNoteIcon className="size-3.5" />
        <span className="font-medium text-foreground">{author}</span>
        <span aria-hidden>·</span>
        <Trans>Note for the team</Trans>
        <span aria-hidden>·</span>
        <time dateTime={m.created_at}>{formatDateTime(m.created_at, i18n.locale)}</time>
      </span>
      <div className="text-reading break-words whitespace-pre-wrap">{m.body}</div>
      <Attachments items={m.attachments} />
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
        render={<Button variant="outline" className={look.btnSm} />}
        title={t`Snooze (B)`}
        data-testid="mail-snooze"
      >
        <ClockIcon />
        <span className="phone:sr-only">
          <Trans>Snooze</Trans>
        </span>
        <ChevronDownIcon className="text-faint" />
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

function Loaded({ c, contact, onBack, onLeave, openDraft }: Props & { c: Conversation; contact?: Contact }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { canManage, membership } = useSession()
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
  const [filtering, setFiltering] = useState<string | null>(null)
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
  const [opened, setOpened] = useState<Set<string>>(new Set())
  const [allOpen, setAllOpen] = useState(false)
  const isOpen = (m: Message) => allOpen || m.id === lastMail?.id || opened.has(m.id)
  const toggleOne = (id: string) =>
    setOpened((s) => {
      const next = new Set(s)
      if (!next.delete(id)) next.add(id)
      return next
    })

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
        }
      : {
          [MAIL_KEYS.reply]: () => replyTo(false),
          [MAIL_KEYS.replyAll]: () => replyTo(true),
          [MAIL_KEYS.note]: () => write && reply.current?.focus("note"),
          [MAIL_KEYS.attach]: () => write && reply.current?.attach(),
          [MAIL_KEYS.archive]: () => write && archive(),
          [MAIL_KEYS.trash]: () => write && canManage && trash(),
          [MAIL_KEYS.star]: toggleStar,
          [MAIL_KEYS.unread]: markUnread,
          [MAIL_KEYS.snooze]: () => write && setSnoozing(true),
          [MAIL_KEYS.filter]: () => write && latestFilterable && setFiltering(latestFilterable.id),
          [MAIL_KEYS.quoted]: () => setExpandQuoted((x) => !x),
          [MAIL_KEYS.expand]: () => setAllOpen((x) => !x),
          ...(write ? cardKeys : {}),
        }
  )

  const subject = c.subject.trim() || t`(no subject)`
  const btn = cn(look.btnSm, "gap-1.5")
  return (
    <section
      aria-label={subject}
      className="relative flex min-h-0 min-w-0 flex-1 flex-col"
      data-testid="mail-thread"
      data-conversation-id={c.id}
    >
      <header className="flex shrink-0 flex-col gap-2 border-b bg-background px-5 pt-3 pb-2.5 phone:px-3 phone:pt-2">
        <div className="flex min-w-0 items-center gap-2">
          <BackButton onBack={onBack} />
          <h2 className="min-w-0 flex-1 truncate text-[17px] font-semibold" data-testid="mail-thread-subject">
            {subject}
          </h2>
          <button
            type="button"
            disabled={!write}
            onClick={toggleStar}
            className={cn(
              "grid size-8 shrink-0 place-items-center rounded-md outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none",
              starred ? "text-warning" : "text-faint"
            )}
            aria-pressed={starred}
            aria-label={starred ? t`Remove the star` : t`Star`}
            title={starred ? t`Remove the star (S)` : t`Star (S)`}
            data-testid="mail-thread-star"
          >
            <StarIcon className={cn("size-[18px]", starred && "fill-current")} />
          </button>
        </div>
        <div
          role="toolbar"
          aria-label={t`Mail actions`}
          className="flex flex-wrap items-center gap-1.5"
          data-testid="mail-actions"
        >
          {inbox && (
            <span className="me-1 inline-flex min-w-0 items-center gap-1 text-caption text-faint">
              <MailIcon className="size-3.5 shrink-0" />
              <span className="truncate">{inbox.name}</span>
            </span>
          )}
          {!trashed && (
            <>
              {write && (
                <Button
                  variant="outline"
                  className={btn}
                  onClick={archive}
                  title={archived ? t`Move to the Inbox (E)` : t`Archive (E)`}
                  data-testid="mail-archive"
                >
                  {archived ? <InboxIcon /> : <ArchiveIcon />}
                  <span className="phone:sr-only">
                    {archived ? <Trans>Move to the Inbox</Trans> : <Trans>Archive</Trans>}
                  </span>
                </Button>
              )}
              {write && <MailSnooze actions={actions} open={snoozing} onOpenChange={setSnoozing} />}
              <Button
                variant="outline"
                className={btn}
                onClick={markUnread}
                title={t`Mark as unread (U)`}
                data-testid="mail-unread"
              >
                <MailIcon />
                <span className="phone:sr-only">
                  <Trans>Mark as unread</Trans>
                </span>
              </Button>
              {write && canManage && (
                <Button
                  variant="outline"
                  className={cn(btn, "text-destructive")}
                  onClick={trash}
                  title={t`Move to the Trash (#)`}
                  data-testid="mail-trash"
                >
                  <Trash2Icon />
                  <span className="phone:sr-only">
                    <Trans>Trash</Trans>
                  </span>
                </Button>
              )}
            </>
          )}
        </div>
      </header>
      {trashed && <TrashBanner c={c} onRestore={restore} busy={trasher.isPending} />}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto bg-surface" data-testid="mail-messages">
        <div className="mx-auto flex max-w-[880px] flex-col gap-2 px-4 py-4 phone:px-2">
          {messages.hasNextPage && (
            <Button
              variant="ghost"
              size="sm"
              className="self-center"
              onClick={() => void messages.fetchNextPage()}
              disabled={messages.isFetchingNextPage}
            >
              <Trans>Load older messages</Trans>
            </Button>
          )}
          {messages.isPending && <Skeleton className="h-40 w-full rounded-xl" />}
          {shown.map((m) =>
            m.redacted_at ? (
              <RedactedLine key={m.id} m={m} ctx={ctx} />
            ) : m.kind === "note" ? (
              <NoteCard key={m.id} m={m} ctx={ctx} />
            ) : m.kind === "event" ? (
              <EventLine key={m.id} m={m} ctx={ctx} />
            ) : (
              <MailCard
                key={m.id}
                m={m}
                ctx={ctx}
                contactName={name}
                open={isOpen(m)}
                onToggle={() => toggleOne(m.id)}
                onFilter={m.kind === "message" && filterable(m) ? () => setFiltering(m.id) : undefined}
              />
            )
          )}
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
              className="rounded-xl border border-dashed bg-card px-4 py-3 opacity-70"
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
        </div>
      </div>
      {!trashed && !write && (
        <div className="shrink-0 border-t bg-background px-4 py-3 phone:px-2">
          <Notice className="mx-auto max-w-[880px]" data-testid="mail-read-only">
            <Trans>You can read this mailbox but not change it. Its owner can let you send from it.</Trans>
          </Notice>
        </div>
      )}
      {!trashed && write && (
        <div className="shrink-0 border-t bg-background px-4 pt-2 pb-3 phone:px-2 phone:pb-[max(0.5rem,env(safe-area-inset-bottom))]">
          <div className="mx-auto flex max-w-[880px] flex-col gap-2">
            <div className="flex items-center gap-1.5" data-testid="mail-reply-modes">
              <Button
                variant="ghost"
                size="sm"
                className="gap-1.5"
                onClick={() => replyTo(false)}
                title={t`Reply (R)`}
                data-testid="mail-reply"
              >
                <ReplyIcon />
                <Trans>Reply</Trans>
                <Kbd className="phone:hidden">{MAIL_KEYS.reply}</Kbd>
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="gap-1.5"
                onClick={() => replyTo(true)}
                title={t`Reply all (A)`}
                data-testid="mail-reply-all"
              >
                <ReplyAllIcon />
                <Trans>Reply all</Trans>
                <Kbd className="phone:hidden">{MAIL_KEYS.replyAll}</Kbd>
              </Button>
            </div>
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
            />
          </div>
        </div>
      )}
      {confirmDialog}
      {filtering && <FilterDialog messageId={filtering} onClose={() => setFiltering(null)} />}
    </section>
  )
}

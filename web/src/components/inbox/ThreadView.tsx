import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, CornerDownRightIcon, MessageCircleIcon, PanelRightIcon, SearchXIcon, ShieldAlertIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { EmptyState, ErrorLine, TypingDots } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Composer, type ComposerHandle } from "@/components/inbox/Composer"
import {
  AssigneeMenu,
  LabelsMenu,
  PriorityMenu,
  SpamButton,
  StatusMenu,
  type MenuName,
} from "@/components/inbox/ConversationControls"
import { MessageItem } from "@/components/inbox/MessageItem"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { ApiError, isGone, type ConversationUpdate } from "@/lib/api"
import {
  useChannel,
  useContact,
  useConversation,
  useInboxes,
  useLabels,
  useMarkRead,
  useMemberMap,
  useMessages,
  useUpdateConversation,
} from "@/lib/queries"
import { useSession } from "@/lib/session"
import { useTyping } from "@/lib/typing"

function RelatedLine({ id, hrefFor }: { id: string; hrefFor: (id: string) => string }) {
  const { t } = useLingui()
  const related = useConversation(id)
  if (related.isPending) return null
  const title = related.data?.subject || t`No subject`
  return (
    <p
      className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground [&_svg]:size-3.5 [&_svg]:shrink-0"
      data-testid="related-conversation"
    >
      <CornerDownRightIcon />
      <span className="min-w-0 truncate">
        {related.data ? (
          <Trans>
            Replied in the thread of{" "}
            <Link to={hrefFor(id)} className="font-medium text-foreground underline-offset-2 hover:underline">
              {title}
            </Link>
          </Trans>
        ) : (
          <Trans>Replied in the thread of another conversation</Trans>
        )}
      </span>
    </p>
  )
}

function TypingLine({ conversationId, contactName }: { conversationId: string; contactName: string }) {
  const { t, i18n } = useLingui()
  const members = useMemberMap()
  const typists = useTyping(conversationId)
  if (typists.length === 0) return null
  const names = typists.map((a) =>
    a.type === "contact"
      ? contactName
      : a.name || (a.member_id && (members.get(a.member_id)?.name || members.get(a.member_id)?.email)) || t`A teammate`,
  )
  const list = new Intl.ListFormat(i18n.locale, { type: "conjunction" }).format(names)
  return (
    <p
      className="flex shrink-0 items-center gap-2 px-4 pb-1 text-xs text-muted-foreground"
      role="status"
      data-testid="typing"
    >
      <TypingDots />
      {names.length === 1 ? <Trans>{list} is typing…</Trans> : <Trans>{list} are typing…</Trans>}
    </p>
  )
}

function dayLabel(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "full" }).format(new Date(iso))
}

export function ThreadView({
  conversationId,
  backHref,
  hrefFor,
  onToggleContact,
  contactShown,
}: {
  conversationId: string
  backHref: string
  hrefFor: (id: string) => string
  onToggleContact: () => void
  contactShown: boolean
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { membership } = useSession()
  const errorText = useErrorText()
  const text = useEnumText()
  const conversation = useConversation(conversationId)
  const messages = useMessages(conversationId)
  const contact = useContact(conversation.data?.contact_id)
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  const members = useMemberMap()
  const updater = useUpdateConversation(conversationId)
  const [openMenu, setOpenMenu] = useState<MenuName | null>(null)
  const [expandQuoted, setExpandQuoted] = useState(false)
  const composer = useRef<ComposerHandle>(null)
  const scroller = useRef<HTMLDivElement>(null)

  const update = (body: ConversationUpdate) => updater.mutate(body)
  const c = conversation.data
  const channel = useChannel(c?.channel_id)

  useHotkeys(
    {
      [SHORTCUTS.reply]: () => composer.current?.focus("message"),
      [SHORTCUTS.note]: () => composer.current?.focus("note"),
      [SHORTCUTS.attach]: () => composer.current?.attach(),
      [SHORTCUTS.assign]: () => setOpenMenu("assign"),
      [SHORTCUTS.assignMe]: () => update({ assignee_id: membership.member_id }),
      [SHORTCUTS.status]: () => setOpenMenu("status"),
      [SHORTCUTS.close]: () => update({ status: "closed" }),
      [SHORTCUTS.reopen]: () => update({ status: "open" }),
      [SHORTCUTS.priority]: () => setOpenMenu("priority"),
      [SHORTCUTS.labels]: () => setOpenMenu("labels"),
      [SHORTCUTS.spam]: () => c && update({ spam: !c.spam }),
      [SHORTCUTS.quoted]: () => setExpandQuoted((x) => !x),
      [SHORTCUTS.contact]: onToggleContact,
      [SHORTCUTS.back]: () => navigate(backHref),
    },
    !!c && !isGone(conversation.error),
  )

  const items = useMemo(
    () => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()),
    [messages.data],
  )
  const firstId = items[0]?.id
  const lastId = items[items.length - 1]?.id
  const lastMine = items[items.length - 1]?.author.member_id === membership.member_id
  const atBottom = useRef(true)
  const shown = useRef<{ first?: string; last?: string; height: number }>({ height: 0 })

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !lastId) return
    const prev = shown.current
    if (!prev.last) el.scrollTop = el.scrollHeight
    else if (firstId !== prev.first && lastId === prev.last) el.scrollTop += el.scrollHeight - prev.height
    else if (lastId !== prev.last && (atBottom.current || lastMine)) el.scrollTop = el.scrollHeight
    shown.current = { first: firstId, last: lastId, height: el.scrollHeight }
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48
  }, [firstId, lastId, lastMine, c?.id])

  const markRead = useMarkRead(conversationId)
  const markedId = useRef<string | undefined>(undefined)
  const { mutate: mark } = markRead
  const maybeMarkRead = useCallback(() => {
    if (!lastId || !scroller.current || markedId.current === lastId) return
    if (!atBottom.current || document.visibilityState !== "visible") return
    markedId.current = lastId
    mark(lastId)
  }, [lastId, mark, c?.id])
  useEffect(() => {
    maybeMarkRead()
    document.addEventListener("visibilitychange", maybeMarkRead)
    return () => document.removeEventListener("visibilitychange", maybeMarkRead)
  }, [maybeMarkRead])

  const top = useRef<HTMLDivElement>(null)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = messages
  useEffect(() => {
    const el = top.current
    if (!el || !hasNextPage || isFetchingNextPage) return
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) void fetchNextPage()
      },
      { root: scroller.current, rootMargin: "300px 0px 0px 0px" },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, firstId, c?.id])

  const onScroll = () => {
    const el = scroller.current
    if (!el) return
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48
    if (atBottom.current) maybeMarkRead()
  }

  useEffect(() => setOpenMenu(null), [conversationId])

  if (conversation.isPending) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-4">
        <Skeleton className="h-6 w-64" />
        <Skeleton className="h-24 w-full" />
      </div>
    )
  }
  if (!c || isGone(conversation.error)) {
    return (
      <EmptyState icon={SearchXIcon} title={<Trans>This conversation is not available</Trans>}>
        <p>{errorText(conversation.error ?? new ApiError(404))}</p>
        <Button variant="outline" size="sm" className="mt-3" render={<Link to={backHref} />}>
          <Trans>Back to the list</Trans>
        </Button>
      </EmptyState>
    )
  }

  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const contactName = contact.data ? contact.data.name || contact.data.emails[0] || t`Unnamed contact` : "…"
  const controls = { conversation: c, update, openMenu, setOpenMenu }
  const ctx = { members, contact: contact.data, labels, subject: c.subject, expandQuoted }
  const isEmail = channel.data?.kind === "email"
  const lastInbound = items.findLast((m) => m.kind === "message" && m.direction === "in" && m.email)
  const emailTo = isEmail ? (lastInbound?.email?.from ?? contact.data?.emails[0]) : undefined
  const undeliverable = emailTo ? contact.data?.undeliverable.some((u) => u.email === emailTo) : false
  let lastDay = ""

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col" data-testid="thread">
      <div className="flex shrink-0 flex-col gap-2 border-b px-3 py-2.5">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon-sm"
            className="md:hidden"
            render={<Link to={backHref} />}
            aria-label={t`Back to the list`}
          >
            <ArrowLeftIcon />
          </Button>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <h2 className="truncate text-base font-semibold" data-testid="thread-subject">
                {c.subject || <Trans>No subject</Trans>}
              </h2>
              {channel.data?.kind === "chat" && (
                <Badge variant="secondary" data-testid="chat-badge">
                  <MessageCircleIcon />
                  {text.channel.chat}
                </Badge>
              )}
            </div>
            <p className="truncate text-xs text-muted-foreground">
              {contactName}
              {inbox && <> · {inbox.name}</>}
            </p>
            {c.related_conversation_id && <RelatedLine id={c.related_conversation_id} hrefFor={hrefFor} />}
          </div>
          <Button
            variant={contactShown ? "secondary" : "ghost"}
            size="icon-sm"
            onClick={onToggleContact}
            aria-label={t`Show or hide the contact`}
            aria-pressed={contactShown}
          >
            <PanelRightIcon />
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <AssigneeMenu {...controls} />
          <StatusMenu {...controls} />
          <PriorityMenu {...controls} />
          <LabelsMenu {...controls} />
          <SpamButton conversation={c} update={update} />
          <ErrorLine error={updater.error} className="text-xs" />
        </div>
      </div>
      {c.spam && (
        <div
          className="flex shrink-0 items-center gap-2 border-b bg-destructive/5 px-4 py-2 text-xs"
          role="status"
          data-testid="spam-banner"
        >
          <ShieldAlertIcon className="size-4 shrink-0 text-destructive" />
          <span className="min-w-0 flex-1">
            <Trans>Marked as spam: left out of lists and counts, and never answered automatically.</Trans>
          </span>
        </div>
      )}
      <div
        ref={scroller}
        onScroll={onScroll}
        className="min-h-0 flex-1 overflow-y-auto py-3 [overflow-anchor:none]"
        data-testid="messages"
      >
        {hasNextPage && (
          <div ref={top} className="flex justify-center py-2">
            {isFetchingNextPage ? (
              <Skeleton className="h-4 w-32" />
            ) : (
              <Button variant="ghost" size="sm" onClick={() => void fetchNextPage()}>
                <Trans>Load older messages</Trans>
              </Button>
            )}
          </div>
        )}
        {messages.isPending ? (
          <div className="flex flex-col gap-3 px-4">
            <Skeleton className="h-14 w-2/3" />
            <Skeleton className="ml-auto h-14 w-1/2" />
          </div>
        ) : messages.error ? (
          <ErrorLine error={messages.error} className="px-4" />
        ) : (
          items.map((m) => {
            const day = dayLabel(m.created_at, i18n.locale)
            const sep = day !== lastDay
            lastDay = day
            return (
              <div key={m.id}>
                {sep && (
                  <div className="my-2 flex items-center gap-3 px-4 text-xs text-muted-foreground">
                    <span className="h-px flex-1 bg-border" />
                    {day}
                    <span className="h-px flex-1 bg-border" />
                  </div>
                )}
                <MessageItem m={m} ctx={ctx} />
              </div>
            )
          })
        )}
      </div>
      <TypingLine conversationId={conversationId} contactName={contactName} />
      <Composer ref={composer} conversationId={conversationId} emailTo={emailTo} undeliverable={undeliverable} />
    </div>
  )
}

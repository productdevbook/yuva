import { Trans, useLingui } from "@lingui/react/macro"
import { SearchXIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { EmptyState, toast } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { ContactPanel } from "@/features/contact/ContactPanel"
import { useContact } from "@/features/contact/queries"
import { readDraft, useQueueActions } from "@/features/conversation/actions"
import { Beaten, TypingCollision } from "@/features/conversation/Collision"
import { useCommandHandlers } from "@/features/conversation/commands"
import { ReplyBox, type ReplyHandle } from "@/features/conversation/composer/ReplyBox"
import { ConversationMenu } from "@/features/conversation/ConversationMenu"
import { Facts, useFacts } from "@/features/conversation/Facts"
import { History } from "@/features/conversation/History"
import { LaterActions, type LaterMenu } from "@/features/conversation/LaterActions"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { Talk, TypingNote } from "@/features/conversation/messages/Talk"
import { PersonHeader } from "@/features/conversation/PersonHeader"
import { useConversation, useMarkRead, useMessages, useMoveConversation, useUpdateConversation } from "@/features/conversation/queries"
import { UpNext } from "@/features/conversation/UpNext"
import { useConversations } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useMediaQuery } from "@/hooks/use-media-query"
import { ApiError, isGone, type Channel, type Contact, type Conversation, type ConversationUpdate } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useViewers } from "@/lib/presence"
import { useTyping } from "@/lib/typing"
import { useChannel, useChannelMap, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"
import { Alert } from "@/components/ui/alert"

export function QueueConversation({ id }: { id: string }) {
  const errorText = useErrorText()
  const queue = useQueue()
  const conversation = useConversation(id)
  const c = conversation.data
  const contact = useContact(c?.contact_id)
  const channels = useChannelMap()
  const known = c?.channel_id ? channels.get(c.channel_id) : undefined
  const fetched = useChannel(c?.channel_id && !known ? c.channel_id : undefined).data
  if (conversation.isPending) {
    return (
      <div className="flex flex-col gap-4" data-testid="conversation-loading">
        <div className="flex items-center gap-4">
          <Skeleton className="size-14 rounded-full" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-7 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
        </div>
        <Skeleton className="mt-6 h-16 w-2/3 rounded-2xl" />
        <Skeleton className="h-36 w-full rounded-[20px]" />
      </div>
    )
  }
  if (!c || isGone(conversation.error)) {
    const next = queue.nextAfter(id)
    return (
      <EmptyState icon={SearchXIcon} title={<Trans>This conversation is not available</Trans>} className="pt-[12vh]">
        <p>{errorText(conversation.error ?? new ApiError(404))}</p>
        <Button variant="outline" size="sm" className="mt-4" onClick={() => (next ? queue.show(next.id) : queue.advance(id))}>
          {next ? <Trans>Open the next one</Trans> : <Trans>Back to the queue</Trans>}
        </Button>
      </EmptyState>
    )
  }
  return <Loaded c={c} contact={contact.data} channel={known ?? fetched} />
}

function Loaded({ c, contact, channel }: { c: Conversation; contact?: Contact; channel?: Channel }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const { membership } = useSession()
  const me = membership.member_id
  const queue = useQueue()
  const members = useMemberMap()
  const labels = useLabels().data ?? []
  const inboxes = useInboxes().data ?? []
  const messages = useMessages(c.id)
  const history = useConversations({ contact_id: c.contact_id })
  const others = (history.data?.pages.flatMap((p) => p.items) ?? []).filter((o) => o.id !== c.id)
  const othersReady = history.isSuccess
  const name = contact ? contact.name || contact.emails[0] || t`Unnamed contact` : "…"
  const actions = useQueueActions(c, name)
  const updater = useUpdateConversation(c.id)
  const mover = useMoveConversation(c.id)
  const reply = useRef<ReplyHandle>(null)
  const [menu, setMenu] = useState<LaterMenu>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [contactOpen, setContactOpen] = useState(false)
  const [moreOpen, setMoreOpen] = useState(false)
  const [expandQuoted, setExpandQuoted] = useState(false)
  const [claimedFrom, setClaimedFrom] = useState<string | null>(null)
  const [beatenBy, setBeatenBy] = useState<string | null>(null)
  const fine = useMediaQuery("(pointer: fine)")

  const items = useMemo(() => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()), [messages.data])
  const suggestion = items.findLast((m) => m.draft)
  const shown = items.filter((m) => m !== suggestion)
  const inbound = shown.filter((m) => m.kind === "message" && m.direction !== "out")
  const lastInbound = inbound.at(-1)
  const feedbackId = c.kind === "feedback" && !messages.hasNextPage ? inbound[0]?.id : undefined
  const isEmail = channel?.kind === "email"
  const emailTo = isEmail ? (inbound.findLast((m) => m.email)?.email?.from ?? contact?.emails[0]) : undefined
  const undeliverable = !!emailTo && !!contact?.undeliverable.some((u) => u.email === emailTo)
  const ctx: ThreadContext = { members, contact, labels, inboxes, subject: c.subject, expandQuoted }
  const facts = useFacts(c, contact, othersReady ? others.length : undefined)
  const waiting = queue.waiting.some((x) => x.id === c.id)
  const next = queue.nextAfter(c.id)

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

  const [wasWaiting, setWasWaiting] = useState(waiting)
  useEffect(() => {
    if (waiting) setWasWaiting(true)
  }, [waiting])
  const movedAway = wasWaiting && !waiting && !queue.leaving.has(c.id)

  const { mutate: mark } = useMarkRead(c.id)
  const marked = useRef<string | null>(null)
  const lastId = shown.at(-1)?.id
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
  const openNext = () => next && queue.show(next.id)

  const handlers = {
    next: openNext,
    reply: () => (showCollision ? claim() : reply.current?.focus("message")),
    note: () => reply.current?.focus("note"),
    snooze: () => setMenu("snooze"),
    hand: () => setMenu("hand"),
    close: actions.close,
    contact: () => setContactOpen(true),
    more: () => setMoreOpen(true),
    history: () => setHistoryOpen((o) => !o),
  }
  useCommandHandlers(handlers)
  useHotkeys({
    [SHORTCUTS.next]: handlers.next,
    [SHORTCUTS.reply]: handlers.reply,
    [SHORTCUTS.note]: handlers.note,
    [SHORTCUTS.attach]: () => reply.current?.attach(),
    [SHORTCUTS.snooze]: handlers.snooze,
    [SHORTCUTS.hand]: handlers.hand,
    [SHORTCUTS.close]: handlers.close,
    [SHORTCUTS.leave]: () => showCollision && leaveTo(),
    [SHORTCUTS.history]: handlers.history,
    [SHORTCUTS.contact]: handlers.contact,
    [SHORTCUTS.more]: handlers.more,
    [SHORTCUTS.spam]: () => update({ spam: !c.spam }),
    [SHORTCUTS.quoted]: () => setExpandQuoted((x) => !x),
    [SHORTCUTS.discardSuggestion]: () => reply.current?.discardSuggestion(),
  })

  const touch = useRef<{ x: number; y: number } | null>(null)
  const onTouchStart = (e: React.TouchEvent) => {
    const target = e.target as HTMLElement
    touch.current = target.closest("textarea, input, [role=menu], [role=listbox], iframe") ? null : { x: e.touches[0].clientX, y: e.touches[0].clientY }
  }
  const onTouchEnd = (e: React.TouchEvent) => {
    const start = touch.current
    touch.current = null
    if (!start) return
    const dx = e.changedTouches[0].clientX - start.x
    const dy = e.changedTouches[0].clientY - start.y
    if (Math.abs(dx) < 90 || Math.abs(dy) > Math.abs(dx) / 2) return
    if (dx > 0) actions.close()
    else actions.later()
  }

  return (
    <div onTouchStart={onTouchStart} onTouchEnd={onTouchEnd} data-testid="conversation" data-conversation-id={c.id}>
      {movedAway && (
        <Alert role="status" className="mb-5 flex items-center gap-3 rounded-[14px] border-dashed border-border bg-card py-2.5 ps-3.5 pe-2.5 text-muted-foreground" data-testid="moved-away">
          <span className="flex-1">
            <Trans>This conversation is no longer waiting for you.</Trans>
          </span>
          {next && (
            <Button variant="outline" size="sm" onClick={openNext} className="border-border text-[13px] font-normal hover:border-faint">
              <Trans>Open the next one</Trans>
            </Button>
          )}
        </Alert>
      )}
      <PersonHeader
        c={c}
        name={name}
        channel={channel}
        lastInbound={lastInbound}
        onContact={() => setContactOpen(true)}
        menu={<ConversationMenu c={c} open={moreOpen} onOpenChange={setMoreOpen} update={update} move={move} onContact={() => setContactOpen(true)} />}
      />
      {c.spam && (
        <Alert role="status" variant="destructive" className="mt-4 flex items-center gap-3 rounded-xl border-transparent bg-destructive/6 px-3.5 py-2 text-[13px] text-foreground" data-testid="spam-banner">
          <span className="flex-1">
            <Trans>Marked as spam: left out of the queue and never answered automatically.</Trans>
          </span>
          <Button variant="link" className="text-[13px] text-foreground underline" onClick={() => update({ spam: false })}>
            <Trans>Not spam</Trans>
          </Button>
        </Alert>
      )}
      <Facts facts={facts} onOpen={() => setContactOpen(true)} />
      <div className="mt-7 grid gap-1.5">
        <History others={others} open={historyOpen} onToggle={() => setHistoryOpen((o) => !o)} name={name} />
        {messages.hasNextPage && (
          <Button variant="ghost" size="sm" className="justify-self-center" onClick={() => void messages.fetchNextPage()} disabled={messages.isFetchingNextPage}>
            <Trans>Load older messages</Trans>
          </Button>
        )}
        {messages.isPending ? (
          <div className="grid gap-2">
            <Skeleton className="h-12 w-2/3 rounded-[18px]" />
            <Skeleton className="h-12 w-1/2 justify-self-end rounded-[18px]" />
          </div>
        ) : messages.error ? (
          <p className="text-sm text-destructive">{errorText(messages.error)}</p>
        ) : (
          <Talk items={shown} ctx={ctx} conversation={c} name={name} feedbackId={feedbackId} />
        )}
        {contactTyping && <TypingNote name={name.split(" ")[0]} />}
      </div>
      {beatenBy ? <Beaten name={beatenBy} /> : showCollision && <TypingCollision name={mateName} typing={!!typingMate} onLeave={leaveTo} onClaim={claim} />}
      <ReplyBox
        key={c.id}
        ref={reply}
        c={c}
        contactName={name}
        via={channel ? text.channel[channel.kind] : undefined}
        emailTo={emailTo}
        undeliverable={undeliverable}
        suggestion={suggestion}
        ctx={ctx}
        actions={actions}
        autoFocus={fine}
      />
      <LaterActions c={c} name={name} actions={actions} menu={menu} setMenu={setMenu} />
      <div className="mt-4.5 hidden justify-between px-1 text-xs text-faint phone:flex" aria-hidden>
        <span>
          <Trans>← later</Trans>
        </span>
        <span>
          <Trans>close →</Trans>
        </span>
      </div>
      {next && <UpNext c={next} onOpen={openNext} />}
      <Sheet open={contactOpen} onOpenChange={setContactOpen}>
        <SheetContent side="right" className="w-[400px] max-w-full bg-background phone:w-full" data-testid="contact-sheet">
          <SheetTitle className="sr-only">
            <Trans>Contact details</Trans>
          </SheetTitle>
          <ContactPanel contactId={c.contact_id} conversationId={c.id} hrefFor={(id) => `/conversations/${id}`} />
        </SheetContent>
      </Sheet>
    </div>
  )
}

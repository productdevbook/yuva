import { Trans, useLingui } from "@lingui/react/macro"
import { Maximize2Icon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { ChannelIcon, ContactAvatar, Dot, toast } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useContact } from "@/features/contact/queries"
import { useQueueActions, type Flow } from "@/features/conversation/actions"
import { ReplyBox, type ReplyHandle } from "@/features/conversation/composer/ReplyBox"
import { LaterActions, type LaterMenu } from "@/features/conversation/LaterActions"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { Talk, TypingNote } from "@/features/conversation/messages/Talk"
import { Status, Watchers } from "@/features/conversation/PersonHeader"
import { useConversation, useMarkRead, useMessages, useUpdateConversation } from "@/features/conversation/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Channel, Contact, Conversation } from "@/lib/api"
import { useTyping } from "@/lib/typing"
import { useChannel, useChannelMap, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

export function ListPreview({ id, flow, onFullScreen }: { id: string; flow: Flow; onFullScreen: () => void }) {
  const conversation = useConversation(id)
  const c = conversation.data
  const contact = useContact(c?.contact_id)
  const channels = useChannelMap()
  const known = c?.channel_id ? channels.get(c.channel_id) : undefined
  const fetched = useChannel(c?.channel_id && !known ? c.channel_id : undefined).data
  if (!c) {
    return (
      <div className="flex flex-col gap-3 p-6" data-testid="preview-loading">
        <Skeleton className="h-11 w-60 rounded-full" />
        <Skeleton className="h-16 w-2/3 rounded-2xl" />
      </div>
    )
  }
  return <Loaded c={c} contact={contact.data} channel={known ?? fetched} flow={flow} onFullScreen={onFullScreen} />
}

function Loaded({ c, contact, channel, flow, onFullScreen }: { c: Conversation; contact?: Contact; channel?: Channel; flow: Flow; onFullScreen: () => void }) {
  const { t } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const members = useMemberMap()
  const labels = useLabels().data ?? []
  const inboxes = useInboxes().data ?? []
  const inbox = inboxes.find((i) => i.id === c.inbox_id)
  const messages = useMessages(c.id)
  const name = contact ? contact.name || contact.emails[0] || t`Unnamed contact` : "…"
  const actions = useQueueActions(c, name, flow)
  const updater = useUpdateConversation(c.id)
  const reply = useRef<ReplyHandle>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const [menu, setMenu] = useState<LaterMenu>(null)
  const items = useMemo(() => (messages.data?.pages ?? []).toReversed().flatMap((p) => p.items.toReversed()), [messages.data])
  const suggestion = items.findLast((m) => m.draft)
  const shown = items.filter((m) => m !== suggestion)
  const inbound = shown.filter((m) => m.kind === "message" && m.direction !== "out")
  const feedbackId = c.kind === "feedback" && !messages.hasNextPage ? inbound[0]?.id : undefined
  const emailTo = channel?.kind === "email" ? (inbound.findLast((m) => m.email)?.email?.from ?? contact?.emails[0]) : undefined
  const undeliverable = !!emailTo && !!contact?.undeliverable.some((u) => u.email === emailTo)
  const ctx: ThreadContext = { members, contact, labels, inboxes, subject: c.subject, expandQuoted: false }
  const contactTyping = useTyping(c.id).some((a) => a.type === "contact")

  const lastId = shown.at(-1)?.id
  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
  }, [lastId])
  const { mutate: mark } = useMarkRead(c.id)
  const marked = useRef<string | null>(null)
  useEffect(() => {
    if (!lastId || marked.current === lastId || document.visibilityState !== "visible") return
    marked.current = lastId
    mark(lastId)
  }, [lastId, mark])

  useHotkeys({
    [SHORTCUTS.reply]: () => reply.current?.focus("message"),
    [SHORTCUTS.note]: () => reply.current?.focus("note"),
    [SHORTCUTS.attach]: () => reply.current?.attach(),
    [SHORTCUTS.snooze]: () => setMenu("snooze"),
    [SHORTCUTS.hand]: () => setMenu("hand"),
    [SHORTCUTS.close]: actions.close,
    [SHORTCUTS.spam]: () => updater.mutate({ spam: !c.spam }, { onError: (e) => toast(errorText(e)) }),
  })

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="preview" data-conversation-id={c.id}>
      <div className="flex flex-wrap items-center gap-3 px-6 pt-5 pb-3">
        <span className="relative shrink-0">
          <ContactAvatar id={c.contact_id} name={name} className="size-11 text-[15px]" />
          {channel && (
            <span className="absolute -end-1 -bottom-1 grid size-5 place-items-center rounded-full border bg-card text-muted-foreground">
              <ChannelIcon kind={channel.kind} className="size-[11px]" />
            </span>
          )}
        </span>
        <div className="min-w-[180px] flex-1">
          <h1 className="truncate text-xl font-semibold tracking-[-0.02em]" data-testid="preview-name">
            {name}
          </h1>
          <div className="flex flex-wrap items-center gap-x-1.5 text-[13px] text-faint">
            {inbox && (
              <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                <Dot color={inbox.branding.color} className="size-2 rounded-[3px]" />
                {inbox.name}
              </span>
            )}
            {channel && <span>· {text.channel[channel.kind]}</span>}
            <span aria-hidden>·</span>
            <Status c={c} lastInbound={inbound.at(-1)} />
            <Watchers conversationId={c.id} />
          </div>
        </div>
        <Button variant="outline" size="sm" className="bg-card" onClick={onFullScreen} data-testid="open-full-screen">
          <Maximize2Icon />
          <Trans>Open full screen</Trans>
        </Button>
      </div>
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto px-6 pb-2">
        {messages.hasNextPage && (
          <div className="flex justify-center">
            <Button variant="ghost" size="sm" onClick={() => void messages.fetchNextPage()} disabled={messages.isFetchingNextPage}>
              <Trans>Load older messages</Trans>
            </Button>
          </div>
        )}
        {messages.isPending ? (
          <Skeleton className="h-12 w-2/3 rounded-[18px]" />
        ) : (
          <Talk items={shown} ctx={ctx} conversation={c} name={name} feedbackId={feedbackId} />
        )}
        {contactTyping && <TypingNote name={name.split(" ")[0]} />}
      </div>
      <div className="px-6 pb-4 [&>[data-testid=reply-box]]:mt-1">
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
          autoFocus={false}
        />
        <LaterActions c={c} name={name} actions={actions} menu={menu} setMenu={setMenu} />
        <p className="mt-2 text-center text-xs text-faint">
          <Trans>J / K to move · X to select · E to close · Enter for full screen</Trans>
        </p>
      </div>
    </div>
  )
}

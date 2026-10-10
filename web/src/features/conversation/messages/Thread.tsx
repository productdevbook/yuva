import { Trans, useLingui } from "@lingui/react/macro"
import { FileDownIcon, LockIcon } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"

import { BotAvatar, ContactAvatar, ErrorLine, Kbd, PersonAvatar, TypingDots } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Badge } from "@/components/ui/badge"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { Message as MessageFrame, MessageAvatar, MessageContent, MessageFooter, MessageHeader } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
  useMessageScroller,
  useMessageScrollerScrollable,
  useMessageScrollerVisibility,
} from "@/components/ui/message-scroller"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { PendingReply } from "@/features/conversation/actions"
import { FeedbackDetails } from "@/features/conversation/Feedback"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { baseSubject, timeOf, useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { DeliveryState } from "@/features/conversation/messages/DeliveryState"
import { MailBody, MailControls, mailSurface, useMail } from "@/features/conversation/messages/EmailBody"
import { EventLine } from "@/features/conversation/messages/EventLine"
import { Highlight, MessageActions, Ticks } from "@/features/conversation/messages/MessageActions"
import { useDraftActions } from "@/features/conversation/queries"
import { rawMessageUrl, type Conversation, type Feedback, type FeedbackCategory, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const GROUP_MS = 5 * 60_000
const DAY_ID = "day:"

type Side = "in" | "me" | "mate" | "bot"

function sideOf(m: Message, me: string): Side {
  if (m.direction !== "out") return "in"
  const by = m.sent_by ?? m.author
  if (by.type === "member" && by.member_id === me) return "me"
  if (m.author.type === "bot" && !m.sent_by) return "bot"
  return "mate"
}

function authorKey(m: Message) {
  const a = m.author
  return `${a.type}:${a.member_id ?? a.contact_id ?? a.api_key_id ?? ""}:${m.sent_by?.member_id ?? ""}`
}

function isMail(m: Message) {
  return m.kind === "message" && !m.draft && (!!m.email || !!m.html)
}

function groupable(m: Message | undefined) {
  return !!m && m.kind === "message" && !m.draft && !isMail(m)
}

function sameGroup(a: Message | undefined, b: Message | undefined) {
  if (!groupable(a) || !groupable(b)) return false
  if (authorKey(a!) !== authorKey(b!)) return false
  const gap = Math.abs(new Date(b!.created_at).getTime() - new Date(a!.created_at).getTime())
  return gap < GROUP_MS && new Date(a!.created_at).toDateString() === new Date(b!.created_at).toDateString()
}

function useDayLabel() {
  const { t, i18n } = useLingui()
  return (iso: string) => {
    const d = new Date(iso)
    const today = new Date()
    const yesterday = new Date()
    yesterday.setDate(today.getDate() - 1)
    if (d.toDateString() === today.toDateString()) return t`Today`
    if (d.toDateString() === yesterday.toDateString()) return t`Yesterday`
    const sameYear = d.getFullYear() === today.getFullYear()
    return new Intl.DateTimeFormat(i18n.locale, sameYear ? { weekday: "long", day: "numeric", month: "long" } : { dateStyle: "long" }).format(d)
  }
}

function Time({ iso, className }: { iso: string; className?: string }) {
  const { i18n } = useLingui()
  return (
    <time className={className} dateTime={iso} title={formatDateTime(iso, i18n.locale)}>
      {timeOf(iso, i18n.locale)}
    </time>
  )
}

const tagClass = "h-auto px-1.5 py-px text-caption"
const hitRing = "ring-2 ring-warning ring-offset-1 ring-offset-surface"
const headerClass = "flex-wrap gap-x-1.5 gap-y-0.5 px-1 text-caption font-normal text-faint"
const footerClass = "flex-wrap gap-x-1 gap-y-0.5 px-1 text-caption font-normal text-faint"

function EmailTags({ m }: { m: Message }) {
  const { t } = useLingui()
  const e = m.email
  if (!e) return null
  const cc = e.cc?.length ? e.cc.join(", ") : ""
  return (
    <>
      {cc && (
        <span className="min-w-0 truncate" title={cc} data-testid="email-cc">
          <Trans>Cc: {cc}</Trans>
        </span>
      )}
      {e.auto && (
        <Badge variant="secondary" className={cn(tagClass, "text-muted-foreground")} title={t`Automatic mail never triggers an automatic reply.`}>
          <Trans>Auto-reply</Trans>
        </Badge>
      )}
      {e.dmarc === "fail" && (
        <Badge variant="destructive" className={tagClass} title={t`The sender's domain did not authorize this mail. It may be forged.`}>
          <Trans>DMARC failed</Trans>
        </Badge>
      )}
      {e.unverified_sender && (
        <Tooltip>
          <TooltipTrigger render={<Badge tabIndex={0} className={cn(tagClass, "bg-warning/10 text-warning")} data-testid="unverified-sender" />}>
            <Trans>Unverified sender</Trans>
          </TooltipTrigger>
          <TooltipContent className="block max-w-72">
            <Trans>
              The visitor typed {e.from} in the chat and has not confirmed it yet. This reply came from that address, so
              it may not be from the visitor.
            </Trans>
          </TooltipContent>
        </Tooltip>
      )}
    </>
  )
}

function useWho(m: Message, side: Side, ctx: ThreadContext, name: string) {
  const { t } = useLingui()
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  if (side === "in") return m.author.type === "contact" ? name : author
  if (side === "me" && !m.sent_by) return t`You`
  return m.sent_by ? t`${author}, sent by ${by}` : author
}

function Face({ m, side, ctx, name }: { m: Message; side: Side; ctx: ThreadContext; name: string }) {
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  if (side === "in") return <ContactAvatar id={ctx.contact?.id ?? m.conversation_id} name={name} className="size-[26px]" />
  if (side === "bot") return <BotAvatar name={author} url={m.author.avatar_url} className="size-[26px]" />
  return <PersonAvatar name={by} className={cn("size-[26px] text-white", side === "me" ? "bg-muted-foreground text-background" : "bg-mate")} />
}

function Meta({ m, side, ctx, name }: { m: Message; side: Side; ctx: ThreadContext; name: string }) {
  const who = useWho(m, side, ctx, name)
  return (
    <MessageFooter className="gap-1 px-1 text-caption font-normal text-faint">
      <span>
        {side === "in" ? who.split(" ")[0] : who} · <Time iso={m.created_at} />
      </span>
      {side !== "in" && !m.delivery && <Ticks at={m.created_at} readAt={ctx.contactReadAt} />}
    </MessageFooter>
  )
}

const sideBubble: Record<Side, { variant: "default" | "outline"; className?: string }> = {
  in: { variant: "outline", className: "*:data-[slot=bubble-content]:bg-card" },
  me: { variant: "default" },
  mate: { variant: "default", className: "*:data-[slot=bubble-content]:bg-mate *:data-[slot=bubble-content]:text-white" },
  bot: { variant: "outline", className: "*:data-[slot=bubble-content]:bg-surface" },
}

const avatarSlot = "size-[26px] min-w-[26px] bg-transparent group-has-data-[slot=message-footer]/message:-translate-y-5"

function tail(out: boolean) {
  return out ? "rounded-ee-md" : "rounded-es-md"
}

function Delivery({ m }: { m: Message }) {
  if (!m.delivery) return null
  return (
    <div data-slot="message-delivery">
      <DeliveryState d={m.delivery} />
    </div>
  )
}

function Line({ m, ctx, name, me, last }: { m: Message; ctx: ThreadContext; name: string; me: string; last: boolean }) {
  const side = sideOf(m, me)
  const out = side !== "in"
  const align = out ? "end" : "start"
  const look = sideBubble[side]
  return (
    <MessageFrame align={align} tabIndex={-1} className={cn("outline-none pointer-coarse:flex-wrap pointer-coarse:gap-y-1", last ? "pb-2" : "pb-0.5")} data-testid="message" data-message-id={m.id}>
      <MessageAvatar className={cn(avatarSlot, !last && "invisible")}>
        <Face m={m} side={side} ctx={ctx} name={name} />
      </MessageAvatar>
      <MessageContent className="w-auto max-w-[80%] gap-1 phone:max-w-[88%]">
        {m.body && (
          <Bubble variant={look.variant} align={align} className={cn("max-w-full", look.className)}>
            <BubbleContent className={cn("rounded-2xl", last && tail(out), ctx.find?.current === m.id && hitRing)}>
              <div className="text-reading break-words whitespace-pre-wrap">
                <Highlight text={m.body} query={ctx.find?.query} />
              </div>
            </BubbleContent>
          </Bubble>
        )}
        <Attachments items={m.attachments} outgoing={out} />
        <Delivery m={m} />
        {last && <Meta m={m} side={side} ctx={ctx} name={name} />}
      </MessageContent>
      <MessageActions m={m} ctx={ctx} />
    </MessageFrame>
  )
}

const mailLook = {
  "original-dark": { variant: "outline", className: "*:data-[slot=bubble-content]:bg-card" },
  "original-light": { variant: "outline", className: "*:data-[slot=bubble-content]:bg-white *:data-[slot=bubble-content]:text-neutral-900" },
  reading: { variant: "outline", className: "*:data-[slot=bubble-content]:bg-card" },
  readingOut: { variant: "tinted", className: undefined },
} as const

function Mail({ m, ctx, name, me, subject }: { m: Message; ctx: ThreadContext; name: string; me: string; subject?: string }) {
  const { t } = useLingui()
  const { workspaceId } = useSession()
  const mail = useMail(m, ctx.expandQuoted)
  const side = sideOf(m, me)
  const out = side !== "in"
  const align = out ? "end" : "start"
  const who = useWho(m, side, ctx, name)
  const surface = mailSurface(mail)
  const framed = surface === "original-dark" || surface === "original-light"
  const look = surface === "text" ? sideBubble[side] : mailLook[surface === "reading" && out ? "readingOut" : surface]
  const wide = surface !== "text"
  const e = m.email
  return (
    <MessageFrame align={align} tabIndex={-1} className="pb-2 outline-none pointer-coarse:flex-wrap pointer-coarse:gap-y-1" data-testid="message" data-kind="email" data-message-id={m.id}>
      <MessageAvatar className={avatarSlot}>
        <Face m={m} side={side} ctx={ctx} name={name} />
      </MessageAvatar>
      <MessageContent className={cn("gap-1", wide ? "max-w-[88%] phone:max-w-full phone:flex-1" : "w-auto max-w-[80%] phone:max-w-[88%]")}>
        <MessageHeader className={headerClass}>
          <span className="min-w-0 truncate" title={e && !out ? e.from : undefined}>
            {who}
            {e && !out && e.from !== who && <span className="text-faint"> &lt;{e.from}&gt;</span>}
          </span>
          <EmailTags m={m} />
        </MessageHeader>
        <Bubble variant={look.variant} align={align} className={cn("max-w-full", wide && "w-full", look.className)}>
          <BubbleContent className={cn("rounded-2xl", tail(out), wide && "w-full", framed && "p-0", ctx.find?.current === m.id && hitRing)}>
            {subject && (
              <div className={cn("text-small font-semibold break-words", framed ? "border-b px-3 pt-2 pb-1.5" : "mb-1")} data-testid="email-subject">
                {subject}
              </div>
            )}
            <MailBody mail={mail} />
          </BubbleContent>
        </Bubble>
        <Attachments items={mail.listed} outgoing={out} />
        <Delivery m={m} />
        <MessageFooter className={footerClass}>
          <Time iso={m.created_at} className="me-1" />
          {out && !m.delivery && <Ticks at={m.created_at} readAt={ctx.contactReadAt} />}
          <MailControls mail={mail} />
          {e?.raw && (
            <a
              href={rawMessageUrl(m.id, workspaceId)}
              download
              className="inline-flex size-6 items-center justify-center rounded-md transition-colors hover:bg-muted hover:text-foreground"
              title={t`Download the original e-mail (.eml)`}
              data-testid="raw-download"
            >
              <FileDownIcon className="size-3.5" />
              <span className="sr-only">
                <Trans>Download the original e-mail (.eml)</Trans>
              </span>
            </a>
          )}
        </MessageFooter>
      </MessageContent>
      <MessageActions m={m} ctx={ctx} />
    </MessageFrame>
  )
}

const categoryTag: Record<FeedbackCategory, string> = {
  bug: "bg-destructive/10 text-destructive",
  idea: "bg-warning/12 text-warning",
  praise: "bg-success/12 text-success",
  other: "bg-muted text-muted-foreground",
}

function FeedbackMessage({ m, ctx, name, feedback }: { m: Message; ctx: ThreadContext; name: string; feedback: Feedback }) {
  const category = feedback.category
  const text = useEnumText()
  return (
    <MessageFrame tabIndex={-1} className="pb-2 outline-none" data-testid="feedback-card" data-message-id={m.id}>
      <MessageAvatar className={avatarSlot}>
        <Face m={m} side="in" ctx={ctx} name={name} />
      </MessageAvatar>
      <MessageContent className="max-w-[88%] gap-1 phone:max-w-[94%]">
        <MessageHeader className={headerClass}>
          <Badge className={cn(tagClass, "rounded-md", categoryTag[category])}>{text.category[category]}</Badge>
          {feedback.page_url ? <Trans>Feedback sent from a documentation page</Trans> : <Trans>Feedback sent from the app</Trans>}
        </MessageHeader>
        <Bubble variant="outline" className="max-w-full *:data-[slot=bubble-content]:bg-card">
          <BubbleContent className={cn("rounded-2xl rounded-es-md p-0", ctx.find?.current === m.id && hitRing)}>
            {m.body && (
              <div className="px-3 py-2 text-reading break-words whitespace-pre-wrap">
                <Highlight text={m.body} query={ctx.find?.query} />
              </div>
            )}
            <div className={cn("px-0.5 py-0.5", m.body && "border-t")}>
              <FeedbackDetails feedback={feedback} />
            </div>
          </BubbleContent>
        </Bubble>
        <Attachments items={m.attachments} />
        <Meta m={m} side="in" ctx={ctx} name={name} />
      </MessageContent>
    </MessageFrame>
  )
}

function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

function NoteBody({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { membership } = useSession()
  const people = (m.mentions ?? []).flatMap((id) => {
    const x = ctx.members.get(id)
    return x ? [{ id, name: x.name || x.email }] : []
  })
  if (people.length === 0)
    return (
      <div className="break-words whitespace-pre-wrap">
        <Highlight text={m.body} query={ctx.find?.query} />
      </div>
    )
  const sorted = [...people].sort((a, b) => b.name.length - a.name.length)
  const re = new RegExp(`@(${sorted.map((p) => escapeRe(p.name)).join("|")})`, "g")
  const parts: React.ReactNode[] = []
  let last = 0
  for (const hit of m.body.matchAll(re)) {
    const who = sorted.find((p) => p.name === hit[1])!
    parts.push(<Highlight key={`t${last}`} text={m.body.slice(last, hit.index)} query={ctx.find?.query} />)
    parts.push(
      <span
        key={hit.index}
        className={cn("rounded px-0.5 font-medium", who.id === membership.member_id ? "bg-brand-wash text-brand" : "bg-note-ink/10 text-note-ink")}
        data-testid="mention"
      >
        {hit[0]}
      </span>,
    )
    last = hit.index + hit[0].length
  }
  parts.push(<Highlight key={`t${last}`} text={m.body.slice(last)} query={ctx.find?.query} />)
  const missing = people.filter((p) => !m.body.includes(`@${p.name}`))
  const names = missing.map((p) => p.name).join(", ")
  return (
    <>
      <div className="break-words whitespace-pre-wrap">{parts}</div>
      {missing.length > 0 && (
        <div className="mt-1 text-caption text-note-ink">
          <Trans>Mentions: {names}</Trans>
        </div>
      )}
    </>
  )
}

function noteSide(m: Message, me: string): Side {
  if (m.author.type === "member" && m.author.member_id === me) return "me"
  if (m.author.type === "bot") return "bot"
  return "mate"
}

function Note({ m, ctx, name, me }: { m: Message; ctx: ThreadContext; name: string; me: string }) {
  const side = noteSide(m, me)
  const who = useWho(m, side, ctx, name)
  return (
    <MessageFrame align="end" tabIndex={-1} className="pb-2 outline-none pointer-coarse:flex-wrap pointer-coarse:gap-y-1" data-testid="note" data-message-id={m.id}>
      <MessageAvatar className={avatarSlot}>
        <Face m={m} side={side} ctx={ctx} name={name} />
      </MessageAvatar>
      <MessageContent className="w-auto max-w-[80%] gap-1 phone:max-w-[88%]">
        <MessageHeader className={cn(headerClass, "justify-end")}>{who}</MessageHeader>
        {m.body && (
          <Bubble
            variant="outline"
            align="end"
            className="max-w-full *:data-[slot=bubble-content]:border-note-border *:data-[slot=bubble-content]:bg-note"
          >
            <BubbleContent className={cn("rounded-2xl rounded-ee-md text-reading", ctx.find?.current === m.id && hitRing)}>
              <NoteBody m={m} ctx={ctx} />
            </BubbleContent>
          </Bubble>
        )}
        <Attachments items={m.attachments} outgoing />
        <MessageFooter className={cn(footerClass, "gap-1 text-note-ink")}>
          <LockIcon className="size-3" />
          <span>
            <Trans>Only the team sees this</Trans> · <Time iso={m.created_at} />
          </span>
        </MessageFooter>
      </MessageContent>
      <MessageActions m={m} ctx={ctx} />
    </MessageFrame>
  )
}

const mod = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

function Draft({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { t } = useLingui()
  const author = useAuthorName(m, ctx)
  const { send, save, discard } = useDraftActions()
  const [editing, setEditing] = useState(false)
  const [body, setBody] = useState(m.body)
  const busy = send.isPending || save.isPending || discard.isPending
  const edit = () => {
    setBody(m.body)
    setEditing(true)
  }
  const submit = () => {
    if (!body.trim()) return
    save.mutate({ m, body }, { onSuccess: () => setEditing(false) })
  }
  return (
    <MessageFrame align="end" className="pb-2" data-testid="draft" data-message-id={m.id}>
      <MessageContent className="max-w-[88%] gap-1.5 phone:max-w-[94%]">
        <MessageHeader className="px-1 font-normal text-caption text-faint">
          <Trans>Draft by {author}</Trans>
        </MessageHeader>
        {editing ? (
          <form
            className="flex w-full max-w-xl flex-col gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              submit()
            }}
          >
            <Textarea
              autoFocus
              name="draft-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  submit()
                } else if (e.key === "Escape") {
                  e.preventDefault()
                  setEditing(false)
                }
              }}
              aria-label={t`Draft text`}
              className="min-h-28 bg-card text-reading md:text-reading"
              data-testid="draft-editor"
            />
            <div className="flex items-center justify-end gap-2">
              <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)}>
                <Trans>Cancel</Trans>
              </Button>
              <span className="hidden items-center gap-1 sm:flex">
                <Kbd>{mod}</Kbd>
                <Kbd>Enter</Kbd>
              </span>
              <Button type="submit" size="sm" disabled={busy || !body.trim()}>
                <Trans>Save draft</Trans>
              </Button>
            </div>
          </form>
        ) : (
          <Bubble
            variant="outline"
            align="end"
            className="max-w-full *:data-[slot=bubble-content]:border-dashed *:data-[slot=bubble-content]:border-warning/50 *:data-[slot=bubble-content]:bg-warning/5"
          >
            <BubbleContent className={cn("rounded-2xl rounded-se-md px-[15px] py-[11px]", ctx.find?.current === m.id && hitRing)}>
              <div className="text-reading break-words whitespace-pre-wrap">{m.body}</div>
            </BubbleContent>
          </Bubble>
        )}
        <Attachments items={m.attachments} outgoing />
        {!editing && (
          <div className="flex gap-1" data-slot="draft-actions" data-testid="draft-actions">
            <Button size="sm" onClick={() => send.mutate(m)} disabled={busy} data-testid="draft-send">
              <Trans>Send</Trans>
            </Button>
            <Button variant="ghost" size="sm" onClick={edit} disabled={busy} data-testid="draft-edit">
              <Trans>Edit</Trans>
            </Button>
            <Button variant="ghost" size="sm" onClick={() => discard.mutate(m)} disabled={busy} data-testid="draft-discard">
              <Trans>Discard</Trans>
            </Button>
          </div>
        )}
        <ErrorLine error={send.error ?? save.error ?? discard.error} className="text-caption" />
      </MessageContent>
    </MessageFrame>
  )
}

function Rated({ m, ctx, name }: { m: Message; ctx: ThreadContext; name: string }) {
  const rating = m.event?.rating
  const first = name.split(" ")[0]
  if (!rating) return <EventLine m={m} ctx={ctx} />
  return (
    <Marker className="justify-center py-2" data-testid="rated">
      <MarkerContent
        className={cn(
          "max-w-[88%] rounded-2xl border px-3.5 py-2 text-center text-body text-foreground",
          rating === "good" ? "border-success/25 bg-success/6" : "border-destructive/25 bg-destructive/5",
        )}
      >
        <span className="font-medium">
          {rating === "good" ? <Trans>{first} rated the conversation 👍</Trans> : <Trans>{first} rated the conversation 👎</Trans>}
        </span>
        {m.body && <span className="text-muted-foreground"> · “{m.body}”</span>}
        <Time iso={m.created_at} className="ms-1.5 text-caption text-faint" />
      </MarkerContent>
    </Marker>
  )
}

function DayMarker({ label }: { label: string }) {
  return (
    <Marker variant="separator" className="py-2.5" data-testid="day-separator">
      <MarkerContent className="rounded-full border bg-card px-3 py-1 text-caption text-muted-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06)]">{label}</MarkerContent>
    </Marker>
  )
}

function Pending({ p }: { p: PendingReply }) {
  return (
    <MessageFrame align="end" className="pb-2" data-testid="pending-reply">
      <MessageContent className="w-auto max-w-[80%] gap-1 phone:max-w-[88%]">
        {p.body.trim() && (
          <Bubble variant="default" align="end" className="max-w-full opacity-70">
            <BubbleContent className="rounded-2xl rounded-ee-md">
              <div className="text-reading break-words whitespace-pre-wrap">{p.body}</div>
            </BubbleContent>
          </Bubble>
        )}
        {p.files.length > 0 && <div className="self-end text-caption text-faint">{p.files.map((f) => f.name).join(", ")}</div>}
        <MessageFooter className="px-1 text-caption font-normal text-faint">
          {p.close ? <Trans>Sending, then closing…</Trans> : <Trans>Sending…</Trans>}
        </MessageFooter>
      </MessageContent>
    </MessageFrame>
  )
}

function Typing({ name }: { name: string }) {
  return (
    <MessageFrame className="items-center pt-1.5" role="status" data-testid="typing">
      <Bubble variant="outline" className="*:data-[slot=bubble-content]:bg-card">
        <BubbleContent className="rounded-[14px] px-3 py-2.5">
          <TypingDots className="gap-[3px] [&>span]:size-[5px]" />
        </BubbleContent>
      </Bubble>
      <span className="text-small text-faint">
        <Trans>{name} is typing…</Trans>
      </span>
    </MessageFrame>
  )
}

function Row({ className, ...props }: React.ComponentProps<typeof MessageScrollerItem>) {
  return <MessageScrollerItem className={cn("[contain-intrinsic-size:none] [content-visibility:visible]", className)} {...props} />
}

function useScrolling(root: React.RefObject<HTMLDivElement | null>) {
  const [on, setOn] = useState(false)
  useEffect(() => {
    const el = root.current
    if (!el) return
    let input = 0
    let showing = false
    let timer = 0
    const mark = () => {
      input = Date.now()
    }
    const scroll = () => {
      if (!showing && Date.now() - input > 1000) return
      showing = true
      setOn(true)
      window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        showing = false
        setOn(false)
      }, 1200)
    }
    const inputs = ["wheel", "touchmove", "pointerdown", "keydown"] as const
    for (const name of inputs) el.addEventListener(name, mark, { passive: true })
    el.addEventListener("scroll", scroll, { passive: true })
    return () => {
      for (const name of inputs) el.removeEventListener(name, mark)
      el.removeEventListener("scroll", scroll)
      window.clearTimeout(timer)
    }
  }, [root])
  return on
}

function DayChip({ items, root }: { items: Message[]; root: React.RefObject<HTMLDivElement | null> }) {
  const dayLabel = useDayLabel()
  const { visibleMessageIds } = useMessageScrollerVisibility()
  const { start } = useMessageScrollerScrollable()
  const scrolling = useScrolling(root)
  const first = visibleMessageIds[0]
  const m = first && !first.startsWith(DAY_ID) ? items.find((x) => x.id === first) : undefined
  if (!m) return null
  return (
    <div
      className={cn("pointer-events-none absolute inset-x-0 top-2 z-10 flex justify-center transition-opacity duration-300", start && scrolling ? "opacity-100" : "opacity-0")}
      aria-hidden
      data-testid="day-chip"
      data-active={start && scrolling}
    >
      <span className="rounded-full border bg-card px-3 py-1 text-caption text-muted-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06)]">{dayLabel(m.created_at)}</span>
    </div>
  )
}

function Older({ root, loading, onLoad }: { root: React.RefObject<HTMLDivElement | null>; loading: boolean; onLoad: () => void }) {
  const self = useRef<HTMLDivElement>(null)
  const load = useRef(onLoad)
  load.current = onLoad
  useEffect(() => {
    const el = self.current
    if (!el || loading || typeof IntersectionObserver === "undefined") return
    const io = new IntersectionObserver((entries) => entries.some((e) => e.isIntersecting) && load.current(), {
      root: root.current,
      rootMargin: "240px 0px 0px 0px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [root, loading])
  return (
    <div ref={self} className="flex justify-center py-1">
      <Button variant="ghost" size="sm" onClick={onLoad} disabled={loading} data-testid="load-older">
        <Trans>Load older messages</Trans>
      </Button>
    </div>
  )
}

export type ThreadProps = {
  items: Message[]
  ctx: ThreadContext
  conversation: Conversation
  name: string
  feedbackId?: string
  top?: React.ReactNode
  older: { has: boolean; loading: boolean; load: () => void }
  pending: boolean
  error: unknown
  queued: PendingReply[]
  typing: boolean
  lastId?: string
  lastMine: boolean
  focusId?: string
}

export function Thread(props: ThreadProps) {
  return (
    <MessageScrollerProvider autoScroll defaultScrollPosition="end" scrollPreviousItemPeek={0}>
      <ThreadView {...props} />
    </MessageScrollerProvider>
  )
}

function ThreadView({ items, ctx: base, conversation, name, feedbackId, top, older, pending, error, queued, typing, lastId, lastMine, focusId }: ThreadProps) {
  const { t } = useLingui()
  const { membership } = useSession()
  const dayLabel = useDayLabel()
  const { scrollToEnd, scrollToMessage } = useMessageScroller()
  const viewport = useRef<HTMLDivElement>(null)
  const me = membership.member_id
  const current = base.find?.current
  const [flash, setFlash] = useState<string>()
  const focusLoaded = !!focusId && items.some((m) => m.id === focusId)
  useEffect(() => {
    if (!focusId || !focusLoaded) return
    scrollToMessage(focusId, { align: "center" })
    setFlash(focusId)
    const timer = window.setTimeout(() => setFlash(undefined), 2400)
    return () => window.clearTimeout(timer)
  }, [focusId, focusLoaded, scrollToMessage])
  const ctx = flash && !current ? { ...base, find: { query: base.find?.query ?? "", current: flash } } : base

  useEffect(() => {
    if (lastMine || queued.length > 0) scrollToEnd()
  }, [lastId, lastMine, queued.length, scrollToEnd])
  useEffect(() => {
    if (current) scrollToMessage(current, { align: "center", behavior: "smooth" })
  }, [current, scrollToMessage])

  const rows = useMemo(() => {
    const out: { key: string; day?: string; m?: Message; i: number }[] = []
    let lastDay = ""
    items.forEach((m, i) => {
      const day = new Date(m.created_at).toDateString()
      if (day !== lastDay) out.push({ key: `${DAY_ID}${day}`, day: m.created_at, i })
      lastDay = day
      out.push({ key: m.id, m, i })
    })
    return out
  }, [items])

  const subjects = useMemo(() => {
    const shown = new Map<string, string>()
    let previous: string | undefined
    for (const m of items) {
      const subject = m.kind === "message" && !m.draft ? m.email?.subject?.trim() : undefined
      if (!subject) continue
      if (previous === undefined || baseSubject(subject) !== previous) shown.set(m.id, subject)
      previous = baseSubject(subject)
    }
    return shown
  }, [items])

  const sending = useMemo(
    () => queued.filter((p) => !items.some((m) => (!!m.client_id && m.client_id === p.clientId) || (!!p.messageId && m.id === p.messageId && !m.draft))),
    [queued, items],
  )

  const render = (m: Message, i: number) => {
    if (m.kind === "event" && m.event?.type === "rated") return <Rated m={m} ctx={ctx} name={name} />
    if (m.kind === "event") return <EventLine m={m} ctx={ctx} />
    if (m.kind === "note") return <Note m={m} ctx={ctx} name={name} me={me} />
    if (m.draft) return <Draft m={m} ctx={ctx} />
    if (m.id === feedbackId && conversation.feedback) return <FeedbackMessage m={m} ctx={ctx} name={name} feedback={conversation.feedback} />
    if (isMail(m)) return <Mail m={m} ctx={ctx} name={name} me={me} subject={subjects.get(m.id)} />
    return <Line m={m} ctx={ctx} name={name} me={me} last={!sameGroup(m, items[i + 1])} />
  }

  return (
    <MessageScroller className="bg-surface">
      <MessageScrollerViewport ref={viewport} aria-label={t`Messages`} data-testid="thread">
        <MessageScrollerContent className="mx-auto w-full max-w-[880px] gap-0 px-5 pt-11 pb-16 phone:px-3">
          {top && <Row>{top}</Row>}
          {older.has && (
            <Row>
              <Older root={viewport} loading={older.loading} onLoad={older.load} />
            </Row>
          )}
          {pending ? (
            <Row className="grid gap-2">
              <Skeleton className="h-12 w-2/3 rounded-2xl" />
              <Skeleton className="h-12 w-1/2 justify-self-end rounded-2xl" />
            </Row>
          ) : error ? (
            <Row>
              <ErrorLine error={error} />
            </Row>
          ) : (
            rows.map((r) =>
              r.m ? (
                <Row key={r.key} messageId={r.m.id}>
                  {render(r.m, r.i)}
                </Row>
              ) : (
                <Row key={r.key} messageId={r.key}>
                  <DayMarker label={dayLabel(r.day!)} />
                </Row>
              ),
            )
          )}
          {sending.map((p) => (
            <Row key={p.key}>
              <Pending p={p} />
            </Row>
          ))}
          {typing && (
            <Row>
              <Typing name={name.split(" ")[0]} />
            </Row>
          )}
        </MessageScrollerContent>
      </MessageScrollerViewport>
      <DayChip items={items} root={viewport} />
      <MessageScrollerButton aria-label={t`Go to the latest message`} title={t`Go to the latest message`} className="z-10 shadow-md phone:inset-s-auto phone:end-2 phone:translate-x-0 rtl:phone:translate-x-0" data-testid="to-latest" />
    </MessageScroller>
  )
}

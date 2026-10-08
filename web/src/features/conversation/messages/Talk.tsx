import { Trans, useLingui } from "@lingui/react/macro"
import { FileDownIcon, LockIcon } from "lucide-react"

import { BotAvatar, ContactAvatar, PersonAvatar, TypingDots } from "@/components/common"
import { formatDateTime, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { FeedbackDetails } from "@/features/conversation/Feedback"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { timeOf, useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { DeliveryState } from "@/features/conversation/messages/DeliveryState"
import { EmailBody } from "@/features/conversation/messages/EmailBody"
import { EventLine } from "@/features/conversation/messages/EventLine"
import { useDraftActions } from "@/features/conversation/queries"
import { attachmentUrl, rawMessageUrl, type Conversation, type Feedback, type FeedbackCategory, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const GROUP_MS = 5 * 60_000

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

function isLetter(m: Message) {
  return m.kind === "message" && m.direction !== "out" && !!m.email
}

function groupable(m: Message | undefined) {
  return !!m && m.kind === "message" && !m.draft && !isLetter(m)
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

const tagClass = "inline-flex items-center rounded-full px-1.5 py-px text-[11px] font-medium"

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
        <span className={cn(tagClass, "bg-muted text-muted-foreground")} title={t`Automatic mail never triggers an automatic reply.`}>
          <Trans>Auto-reply</Trans>
        </span>
      )}
      {e.dmarc === "fail" && (
        <span className={cn(tagClass, "bg-destructive/8 text-destructive")} title={t`The sender's domain did not authorize this mail. It may be forged.`}>
          <Trans>DMARC failed</Trans>
        </span>
      )}
      {e.unverified_sender && (
        <Tooltip>
          <TooltipTrigger render={<span tabIndex={0} className={cn(tagClass, "bg-warning/10 text-warning")} data-testid="unverified-sender" />}>
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

function Letter({ m, ctx, name }: { m: Message; ctx: ThreadContext; name: string }) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const e = m.email!
  return (
    <div className="rounded-2xl border bg-card px-[18px] py-4" data-testid="message" data-message-id={m.id}>
      {e.subject && <div className="text-[15px] font-semibold break-words">{e.subject}</div>}
      <div className="mt-0.5 mb-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-faint">
        <span className="min-w-0 truncate">
          {name} &lt;{e.from}&gt;
        </span>
        <span aria-hidden>·</span>
        <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
        <EmailTags m={m} />
        {e.raw && (
          <a
            href={rawMessageUrl(m.id, workspaceId)}
            download
            className="ms-auto inline-flex transition-colors hover:text-foreground"
            title={t`Download the original e-mail (.eml)`}
            data-testid="raw-download"
          >
            <FileDownIcon className="size-3.5" />
            <span className="sr-only">
              <Trans>Download the original e-mail (.eml)</Trans>
            </span>
          </a>
        )}
      </div>
      <EmailBody m={m} outgoing={false} expandQuoted={ctx.expandQuoted} plain />
    </div>
  )
}

const categoryTag: Record<FeedbackCategory, string> = {
  bug: "bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300",
  idea: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
  praise: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300",
  other: "bg-muted text-muted-foreground",
}

function FeedbackCard({ m, feedback }: { m: Message; feedback: Feedback }) {
  const category = feedback.category
  const { i18n } = useLingui()
  const text = useEnumText()
  const { workspaceId } = useSession()
  const shots = m.attachments.filter((a) => a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml")
  const rest = m.attachments.filter((a) => !shots.includes(a))
  return (
    <div className="overflow-hidden rounded-2xl border bg-card" data-testid="feedback-card" data-message-id={m.id}>
      <div className="flex items-center gap-2 border-b px-4 py-3 text-[13px]">
        <span className={cn("rounded-md px-2 py-0.5 text-xs font-medium", categoryTag[category])}>{text.category[category]}</span>
        <Trans>Feedback sent from the app</Trans>
        <time className="ms-auto text-faint" dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
      </div>
      <div className={cn("grid items-start gap-4 px-4 py-3.5 text-[15px] leading-[1.55]", shots.length > 0 && "grid-cols-[1fr_72px] phone:grid-cols-1")}>
        <div className="break-words whitespace-pre-wrap">{m.body}</div>
        {shots.length > 0 && (
          <div className="flex flex-col gap-2 phone:flex-row">
            {shots.map((a) => (
              <a key={a.id} href={attachmentUrl(a, workspaceId)} target="_blank" rel="noreferrer" title={a.filename}>
                <img
                  src={attachmentUrl(a, workspaceId)}
                  alt={a.filename}
                  loading="lazy"
                  className="h-[120px] w-[72px] rounded-[10px] border bg-surface object-cover"
                />
              </a>
            ))}
          </div>
        )}
      </div>
      {rest.length > 0 && (
        <div className="px-4 pb-3.5">
          <Attachments items={rest} outgoing={false} />
        </div>
      )}
      <div className="border-t px-1.5 py-1.5">
        <FeedbackDetails feedback={feedback} />
      </div>
    </div>
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
  if (people.length === 0) return <div className="break-words whitespace-pre-wrap">{m.body}</div>
  const sorted = [...people].sort((a, b) => b.name.length - a.name.length)
  const re = new RegExp(`@(${sorted.map((p) => escapeRe(p.name)).join("|")})`, "g")
  const parts: React.ReactNode[] = []
  let last = 0
  for (const hit of m.body.matchAll(re)) {
    const who = sorted.find((p) => p.name === hit[1])!
    parts.push(m.body.slice(last, hit.index))
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
  parts.push(m.body.slice(last))
  const missing = people.filter((p) => !m.body.includes(`@${p.name}`))
  const names = missing.map((p) => p.name).join(", ")
  return (
    <>
      <div className="break-words whitespace-pre-wrap">{parts}</div>
      {missing.length > 0 && (
        <div className="mt-1 text-xs text-note-ink">
          <Trans>Mentions: {names}</Trans>
        </div>
      )}
    </>
  )
}

function Note({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { i18n } = useLingui()
  const author = useAuthorName(m, ctx)
  return (
    <div className="my-2.5 rounded-[14px] border border-note-border bg-note px-3.5 py-3 text-sm leading-[1.55]" data-testid="note" data-message-id={m.id}>
      <div className="mb-1 flex items-center gap-1.5 text-xs font-medium text-note-ink">
        <LockIcon className="size-3" />
        <span className="min-w-0 truncate">
          <Trans>{author} · only the team sees this</Trans>
        </span>
        <time className="ms-auto shrink-0 font-normal" dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
      </div>
      {m.body && <NoteBody m={m} ctx={ctx} />}
      {m.attachments.length > 0 && (
        <div className="mt-2">
          <Attachments items={m.attachments} outgoing={false} />
        </div>
      )}
    </div>
  )
}

function Face({ m, side, ctx, name }: { m: Message; side: Side; ctx: ThreadContext; name: string }) {
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  if (side === "in") return <ContactAvatar id={ctx.contact?.id ?? m.conversation_id} name={name} className="size-[26px] text-[10px]" />
  if (side === "bot") return <BotAvatar name={author} url={m.author.avatar_url} className="size-[26px] text-[10px]" />
  return <PersonAvatar name={by} className={cn("size-[26px] text-[10px] font-semibold text-white", side === "me" ? "bg-zinc-600" : "bg-mate")} />
}

function Meta({ m, side, ctx, name }: { m: Message; side: Side; ctx: ThreadContext; name: string }) {
  const { t, i18n } = useLingui()
  const author = useAuthorName(m, ctx)
  const by = useAuthorName({ ...m, author: m.sent_by ?? m.author }, ctx)
  const who = side === "in" ? name.split(" ")[0] : side === "me" && !m.sent_by ? t`You` : m.sent_by ? t`${author}, sent by ${by}` : author
  return (
    <div className={cn("mx-1 mt-1 text-[11px] text-faint", side !== "in" && "text-end")}>
      {who} ·{" "}
      <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
        {timeOf(m.created_at, i18n.locale)}
      </time>
    </div>
  )
}

function Line({ m, ctx, name, me, last }: { m: Message; ctx: ThreadContext; name: string; me: string; last: boolean }) {
  const side = sideOf(m, me)
  const out = side !== "in"
  const rich = !!m.html
  return (
    <div
      className={cn("flex max-w-[88%] items-end gap-2.5 phone:max-w-[94%]", out && "flex-row-reverse self-end", last ? "mb-2" : "mb-0.5")}
      data-testid="message"
      data-message-id={m.id}
    >
      <span className={cn("shrink-0", !last && "invisible")}>
        <Face m={m} side={side} ctx={ctx} name={name} />
      </span>
      <div className={cn("flex min-w-0 flex-col", out && "items-end")}>
        {rich ? (
          <EmailBody m={m} outgoing={out} expandQuoted={ctx.expandQuoted} />
        ) : (
          m.body && (
            <div
              className={cn(
                "rounded-[18px] border px-[15px] py-[11px] text-[15px] leading-[1.55] break-words whitespace-pre-wrap",
                side === "in" && "bg-card",
                side === "me" && "border-primary bg-primary text-white",
                side === "mate" && "border-transparent bg-mate text-white",
                side === "bot" && "bg-surface",
                last && (out ? "rounded-ee-md" : "rounded-es-md"),
              )}
            >
              {m.body}
            </div>
          )
        )}
        {!rich && m.attachments.length > 0 && (
          <div className="mt-1">
            <Attachments items={m.attachments} outgoing={out} />
          </div>
        )}
        {m.delivery && <div className="mt-1">{<DeliveryState d={m.delivery} />}</div>}
        {last && <Meta m={m} side={side} ctx={ctx} name={name} />}
      </div>
    </div>
  )
}

function OlderDraft({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const author = useAuthorName(m, ctx)
  const { send, discard } = useDraftActions()
  return (
    <div className="flex max-w-[88%] flex-col items-end gap-1.5 self-end" data-testid="draft" data-message-id={m.id}>
      <span className="text-[11px] text-faint">
        <Trans>Draft by {author}</Trans>
      </span>
      <div className="rounded-[18px] border border-dashed border-warning/50 bg-warning/5 px-[15px] py-[11px] text-[15px] leading-[1.55] break-words whitespace-pre-wrap">
        {m.body}
      </div>
      <div className="flex gap-1">
        <Button size="xs" onClick={() => send.mutate(m)} disabled={send.isPending || discard.isPending}>
          <Trans>Send</Trans>
        </Button>
        <Button variant="ghost" size="xs" onClick={() => discard.mutate(m)} disabled={send.isPending || discard.isPending}>
          <Trans>Discard</Trans>
        </Button>
      </div>
    </div>
  )
}

export function Talk({
  items,
  ctx,
  conversation,
  name,
  feedbackId,
}: {
  items: Message[]
  ctx: ThreadContext
  conversation: Conversation
  name: string
  feedbackId?: string
}) {
  const { membership } = useSession()
  const dayLabel = useDayLabel()
  const me = membership.member_id
  let lastDay = ""
  return (
    <div className="flex flex-col">
      {items.map((m, i) => {
        const day = new Date(m.created_at).toDateString()
        const sep = day !== lastDay ? <div className="mt-2.5 mb-2 flex items-center gap-3 text-xs text-faint before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">{dayLabel(m.created_at)}</div> : null
        lastDay = day
        let body: React.ReactNode
        if (m.kind === "event") body = <EventLine m={m} ctx={ctx} />
        else if (m.kind === "note") body = <Note m={m} ctx={ctx} />
        else if (m.draft) body = <OlderDraft m={m} ctx={ctx} />
        else if (m.id === feedbackId && conversation.feedback) body = <FeedbackCard m={m} feedback={conversation.feedback} />
        else if (isLetter(m)) body = <Letter m={m} ctx={ctx} name={name} />
        else body = <Line m={m} ctx={ctx} name={name} me={me} last={!sameGroup(m, items[i + 1])} />
        return (
          <div key={m.id} className="contents">
            {sep}
            {body}
          </div>
        )
      })}
    </div>
  )
}

export function TypingNote({ name }: { name: string }) {
  return (
    <div className="mt-1.5 flex items-center gap-2.5 text-[13px] text-faint" role="status" data-testid="typing">
      <span className="inline-flex rounded-[14px] border bg-card px-3 py-2.5">
        <TypingDots className="gap-[3px] [&>span]:size-[5px]" />
      </span>
      <Trans>{name} is typing…</Trans>
    </div>
  )
}

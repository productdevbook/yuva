import { Trans, useLingui } from "@lingui/react/macro"
import {
  AlertCircleIcon,
  BotIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  ClockIcon,
  DownloadIcon,
  FileDownIcon,
  ImageIcon,
  LockIcon,
  PaperclipIcon,
  ShieldAlertIcon,
  TagIcon,
  UserIcon,
  UserXIcon,
} from "lucide-react"
import { useEffect, useMemo, useState } from "react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { EmailHtml, placedContentIds } from "@/components/common/EmailHtml"
import { formatBytes, formatDateTime, useEnumText } from "@/components/common/text"
import { statusIcons } from "@/components/inbox/ConversationControls"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  attachmentUrl,
  rawMessageUrl,
  type Attachment,
  type Contact,
  type Label,
  type Member,
  type Message,
  type MessageDelivery,
} from "@/lib/api"
import { useMessageEmail } from "@/lib/queries"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

type Ctx = {
  members: Map<string, Member>
  contact?: Contact
  labels: Label[]
  subject: string
  expandQuoted: boolean
}

function baseSubject(s: string) {
  return s.replace(/^(\s*(re|fwd?|aw|ynt|ilt)\s*:\s*)+/i, "").trim().toLowerCase()
}

function useAuthorName(m: Message, ctx: Ctx) {
  const { t } = useLingui()
  if (m.author.type === "contact") return ctx.contact?.name || ctx.contact?.emails[0] || t`Contact`
  if (m.author.type === "system") return t`System`
  const member = m.author.member_id ? ctx.members.get(m.author.member_id) : undefined
  return member ? member.name || member.email : t`Removed member`
}

function time(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(new Date(iso))
}

function AttachmentChip({ a }: { a: Attachment }) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  if (a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml") {
    return (
      <a
        href={attachmentUrl(a, workspaceId)}
        target="_blank"
        rel="noreferrer"
        className="group flex max-w-60 flex-col overflow-hidden rounded-md border bg-background text-xs transition-colors hover:bg-muted"
        title={t`Open ${a.filename}`}
        data-testid="attachment"
      >
        <img
          src={attachmentUrl(a, workspaceId)}
          alt={a.filename}
          loading="lazy"
          className="max-h-44 w-full bg-muted object-contain"
        />
        <span className="flex items-center gap-1.5 px-2 py-1">
          <ImageIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="min-w-0 truncate font-medium">{a.filename}</span>
          <span className="ml-auto shrink-0 text-muted-foreground">{formatBytes(a.size, i18n.locale)}</span>
        </span>
      </a>
    )
  }
  return (
    <a
      href={attachmentUrl(a, workspaceId)}
      download={a.filename}
      className="flex max-w-full items-center gap-2 rounded-md border bg-background px-2 py-1.5 text-left text-xs transition-colors hover:bg-muted"
      title={t`Download ${a.filename}`}
      data-testid="attachment"
    >
      <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">{a.filename}</span>
      <span className="shrink-0 text-muted-foreground">{formatBytes(a.size, i18n.locale)}</span>
      <DownloadIcon className="size-3.5 shrink-0 text-muted-foreground" />
    </a>
  )
}

function EventLine({ m, ctx }: { m: Message; ctx: Ctx }) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const actor = useAuthorName(m, ctx)
  const e = m.event
  if (!e) return null
  const memberName = (id?: string) => {
    const x = id ? ctx.members.get(id) : undefined
    return x ? x.name || x.email : "?"
  }
  const labelNames = (ids?: string[]) =>
    (ids ?? []).map((id) => ctx.labels.find((l) => l.id === id)?.name ?? "?").join(", ")
  let icon: React.ReactNode
  let body: React.ReactNode
  switch (e.type) {
    case "assigned": {
      const assignee = memberName(e.assignee_id)
      icon = <UserIcon />
      body = <Trans>{actor} assigned this to {assignee}</Trans>
      break
    }
    case "unassigned": {
      icon = <UserXIcon />
      body = <Trans>{actor} removed the assignee</Trans>
      break
    }
    case "status_changed": {
      const Icon = statusIcons[e.status ?? "open"]
      icon = <Icon />
      const status = text.status[e.status ?? "open"]
      body =
        e.status === "closed" ? (
          <Trans>{actor} closed the conversation</Trans>
        ) : e.status === "open" ? (
          <Trans>{actor} reopened the conversation</Trans>
        ) : (
          <Trans>
            {actor} set the status to {status}
          </Trans>
        )
      break
    }
    case "labels_changed": {
      icon = <TagIcon />
      const added = labelNames(e.added_labels)
      const removed = labelNames(e.removed_labels)
      body =
        added && removed ? (
          <Trans>
            {actor} added {added} and removed {removed}
          </Trans>
        ) : added ? (
          <Trans>
            {actor} added the label {added}
          </Trans>
        ) : (
          <Trans>
            {actor} removed the label {removed}
          </Trans>
        )
      break
    }
  }
  return (
    <div
      className="flex items-center justify-center gap-1.5 px-4 py-1 text-xs text-muted-foreground [&_svg]:size-3.5"
      data-testid="event-line"
    >
      {icon}
      <span>{body}</span>
      <span aria-hidden>·</span>
      <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
        {time(m.created_at, i18n.locale)}
      </time>
    </div>
  )
}

function DeliveryState({ d }: { d: MessageDelivery }) {
  const { i18n } = useLingui()
  const at = formatDateTime(d.updated_at, i18n.locale)
  if (d.state === "queued") {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground" data-testid="delivery" data-state="queued">
        <ClockIcon className="size-3.5" />
        <Trans>Sending by e-mail…</Trans>
      </span>
    )
  }
  if (d.state === "sent") {
    return (
      <span
        className="inline-flex items-center gap-1 text-xs text-muted-foreground"
        title={at}
        data-testid="delivery"
        data-state="sent"
      >
        <CheckIcon className="size-3.5 text-success" />
        <Trans>Sent by e-mail</Trans>
      </span>
    )
  }
  return (
    <div
      role="alert"
      className="flex max-w-full flex-col gap-0.5 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1.5 text-xs"
      data-testid="delivery"
      data-state="failed"
    >
      <span className="inline-flex items-center gap-1 font-medium text-destructive">
        <AlertCircleIcon className="size-3.5 shrink-0" />
        <Trans>Not delivered</Trans>
      </span>
      {d.error && <span className="break-words text-foreground">{d.error}</span>}
      <span className="text-muted-foreground">
        <Trans>Fix the address or the channel's SMTP settings, then send the reply again.</Trans>
      </span>
    </div>
  )
}

function MetaBadge({ children, tone = "muted", title }: { children: React.ReactNode; tone?: "muted" | "danger"; title?: string }) {
  return (
    <span
      title={title}
      className={cn(
        "inline-flex items-center gap-1 rounded px-1.5 py-px text-[11px] font-medium [&_svg]:size-3",
        tone === "danger" ? "bg-destructive/10 text-destructive" : "bg-muted text-muted-foreground",
      )}
    >
      {children}
    </span>
  )
}

function Attachments({ items, outgoing }: { items: Attachment[]; outgoing: boolean }) {
  if (items.length === 0) return null
  return (
    <div className={cn("flex max-w-full flex-col gap-1", outgoing && "items-end")}>
      {items.map((a) => (
        <AttachmentChip key={a.id} a={a} />
      ))}
    </div>
  )
}

function EmailBody({ m, outgoing, expandQuoted }: { m: Message; outgoing: boolean; expandQuoted: boolean }) {
  const { t } = useLingui()
  const { workspaceId } = useSession()
  const [quotedOwn, setQuotedOwn] = useState<boolean | null>(null)
  const [images, setImages] = useState(false)
  useEffect(() => setQuotedOwn(null), [expandQuoted])
  const quotedShown = !!m.email?.quoted && (quotedOwn ?? expandQuoted)
  const detail = useMessageEmail(m.id, quotedShown)
  const full = quotedShown ? detail.data : undefined
  const html = full ? full.full_html : m.html
  const text = full ? full.full_text : m.body
  const remote = !!html && !!(full ? full.has_remote_images : m.email?.has_remote_images)
  const inline = useMemo(
    () =>
      new Map(
        m.attachments
          .filter((a) => a.content_id && a.content_type.startsWith("image/") && a.content_type !== "image/svg+xml")
          .map((a) => [a.content_id!, attachmentUrl(a, workspaceId)]),
      ),
    [m.attachments, workspaceId],
  )
  const listed = useMemo(() => {
    const placed = html && inline.size > 0 ? placedContentIds(html) : new Set<string>()
    return m.attachments.filter((a) => !(a.inline && a.content_id && inline.has(a.content_id) && placed.has(a.content_id)))
  }, [html, inline, m.attachments])
  return (
    <>
      {html ? (
        <div
          className={cn(
            "w-full overflow-hidden rounded-xl border bg-white",
            outgoing ? "rounded-tr-sm" : "rounded-tl-sm",
          )}
        >
          <EmailHtml html={html} images={images} inline={inline} />
        </div>
      ) : (
        text && (
          <div
            className={cn(
              "rounded-xl px-3 py-2 text-sm break-words whitespace-pre-wrap",
              outgoing ? "rounded-tr-sm bg-primary text-primary-foreground" : "rounded-tl-sm bg-muted",
            )}
          >
            {text}
          </div>
        )
      )}
      {quotedShown && detail.isPending && <Skeleton className="h-10 w-full" />}
      <ErrorLine error={quotedShown && detail.error} className="text-xs" />
      {(m.email?.quoted || (remote && !images)) && (
        <div className={cn("flex flex-wrap gap-1", outgoing && "justify-end")}>
          {m.email?.quoted && (
            <Button
              variant="ghost"
              size="xs"
              className="text-muted-foreground"
              onClick={() => setQuotedOwn(!quotedShown)}
              aria-expanded={quotedShown}
              data-testid="quoted-toggle"
            >
              <ChevronsUpDownIcon />
              {quotedShown ? <Trans>Hide quoted text</Trans> : <Trans>Show quoted text</Trans>}
            </Button>
          )}
          {remote && !images && (
            <Button
              variant="ghost"
              size="xs"
              className="text-muted-foreground"
              onClick={() => setImages(true)}
              title={t`Remote images can tell the sender that you opened the mail.`}
              data-testid="load-images"
            >
              <ImageIcon />
              <Trans>Load images</Trans>
            </Button>
          )}
        </div>
      )}
      <Attachments items={listed} outgoing={outgoing} />
    </>
  )
}

export function MessageItem({ m, ctx }: { m: Message; ctx: Ctx }) {
  const { t, i18n } = useLingui()
  const { workspaceId } = useSession()
  const author = useAuthorName(m, ctx)
  if (m.kind === "event") return <EventLine m={m} ctx={ctx} />
  const note = m.kind === "note"
  const outgoing = !note && m.direction === "out"
  const email = m.email
  const showFrom = !!email && !outgoing && email.from !== author
  const cc = !outgoing && email?.cc?.length ? email.cc.join(", ") : ""
  const showSubject = !!email?.subject && baseSubject(email.subject) !== baseSubject(ctx.subject)
  return (
    <div
      className={cn("flex gap-2.5 px-4 py-1.5", outgoing && "flex-row-reverse")}
      data-testid={note ? "note" : "message"}
      data-message-id={m.id}
    >
      <PersonAvatar name={author} className="mt-5 size-7" />
      <div
        className={cn(
          "flex max-w-[min(42rem,85%)] min-w-0 flex-col gap-1",
          outgoing && "items-end",
          m.html && "w-full",
        )}
      >
        <div
          className={cn(
            "flex max-w-full flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground",
            outgoing && "flex-row-reverse",
          )}
        >
          <span className="font-medium text-foreground">{author}</span>
          {showFrom && <span className="truncate">{email.from}</span>}
          {cc && (
            <span className="min-w-0 truncate" title={cc} data-testid="email-cc">
              <Trans>Cc: {cc}</Trans>
            </span>
          )}
          {note && (
            <span className="inline-flex items-center gap-1 rounded bg-amber-100 px-1.5 py-px text-[11px] font-medium text-amber-800 dark:bg-amber-900/50 dark:text-amber-200">
              <LockIcon className="size-3" />
              <Trans>Internal note</Trans>
            </span>
          )}
          {email?.auto && (
            <MetaBadge title={t`Automatic mail never triggers an automatic reply.`}>
              <BotIcon />
              <Trans>Auto-reply</Trans>
            </MetaBadge>
          )}
          {email?.dmarc === "fail" && (
            <MetaBadge tone="danger" title={t`The sender's domain did not authorize this mail. It may be forged.`}>
              <ShieldAlertIcon />
              <Trans>DMARC failed</Trans>
            </MetaBadge>
          )}
          <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
            {time(m.created_at, i18n.locale)}
          </time>
          {email?.raw && (
            <a
              href={rawMessageUrl(m.id, workspaceId)}
              download
              className="inline-flex items-center gap-0.5 hover:text-foreground"
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
        {showSubject && (
          <p className={cn("max-w-full truncate text-xs font-medium", outgoing && "text-right")} data-testid="email-subject">
            {email!.subject}
          </p>
        )}
        {email || m.html ? (
          <EmailBody m={m} outgoing={outgoing} expandQuoted={ctx.expandQuoted} />
        ) : (
          m.body && (
            <div
              className={cn(
                "rounded-xl px-3 py-2 text-sm break-words whitespace-pre-wrap",
                note
                  ? "border border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-50"
                  : outgoing
                    ? "rounded-tr-sm bg-primary text-primary-foreground"
                    : "rounded-tl-sm bg-muted",
              )}
            >
              {m.body}
            </div>
          )
        )}
        {!(email || m.html) && <Attachments items={m.attachments} outgoing={outgoing} />}
        {m.delivery && !note && <DeliveryState d={m.delivery} />}
      </div>
    </div>
  )
}

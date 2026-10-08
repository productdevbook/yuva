import { Trans, useLingui } from "@lingui/react/macro"
import { FileDownIcon, LockIcon } from "lucide-react"

import { formatDateTime } from "@/components/common/text"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { Attachments } from "@/features/conversation/messages/Attachments"
import { Bubble } from "@/features/conversation/messages/Bubble"
import { baseSubject, timeOf, useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import { DeliveryState } from "@/features/conversation/messages/DeliveryState"
import { EmailBody } from "@/features/conversation/messages/EmailBody"
import { EventLine } from "@/features/conversation/messages/EventLine"
import { rawMessageUrl, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const tagClass = "inline-flex items-center rounded-full px-1.5 py-px text-[11px] font-medium"

function Tag({ tone = "muted", title, children }: { tone?: "muted" | "danger" | "warning"; title?: string; children: React.ReactNode }) {
  return (
    <span
      title={title}
      className={cn(
        tagClass,
        tone === "danger" ? "bg-destructive/8 text-destructive" : tone === "warning" ? "bg-warning/10 text-warning" : "bg-muted text-muted-foreground",
      )}
    >
      {children}
    </span>
  )
}

function UnverifiedSender({ from }: { from: string }) {
  return (
    <Tooltip>
      <TooltipTrigger render={<span tabIndex={0} className={cn(tagClass, "bg-warning/10 text-warning")} data-testid="unverified-sender" />}>
        <Trans>Unverified sender</Trans>
      </TooltipTrigger>
      <TooltipContent className="block max-w-72">
        <Trans>
          The visitor typed {from} in the chat and has not confirmed it yet. This reply came from that address, so it
          may not be from the visitor.
        </Trans>
      </TooltipContent>
    </Tooltip>
  )
}

export function MessageItem({ m, ctx }: { m: Message; ctx: ThreadContext }) {
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
  const rich = !!(email || m.html)
  return (
    <div
      className={cn("flex flex-col gap-1.5", outgoing ? "items-end ps-10" : note ? "" : "items-start pe-10")}
      data-testid={note ? "note" : "message"}
      data-message-id={m.id}
    >
      <div className={cn("flex max-w-full flex-wrap items-center gap-x-2 gap-y-1 px-1 text-xs text-faint", outgoing && "flex-row-reverse")}>
        <span className="font-medium text-muted-foreground">{author}</span>
        {note && (
          <span className="inline-flex items-center gap-1">
            <LockIcon className="size-3" />
            <Trans>Internal note</Trans>
          </span>
        )}
        {showFrom && <span className="truncate">{email.from}</span>}
        {cc && (
          <span className="min-w-0 truncate" title={cc} data-testid="email-cc">
            <Trans>Cc: {cc}</Trans>
          </span>
        )}
        {email?.auto && (
          <Tag title={t`Automatic mail never triggers an automatic reply.`}>
            <Trans>Auto-reply</Trans>
          </Tag>
        )}
        {email?.dmarc === "fail" && (
          <Tag tone="danger" title={t`The sender's domain did not authorize this mail. It may be forged.`}>
            <Trans>DMARC failed</Trans>
          </Tag>
        )}
        {email && !outgoing && email.unverified_sender && <UnverifiedSender from={email.from} />}
        <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
        {email?.raw && (
          <a
            href={rawMessageUrl(m.id, workspaceId)}
            download
            className="inline-flex transition-colors hover:text-foreground"
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
        <p className={cn("max-w-full truncate px-1 text-xs font-medium", outgoing && "text-end")} data-testid="email-subject">
          {email!.subject}
        </p>
      )}
      {rich ? (
        <EmailBody m={m} outgoing={outgoing} expandQuoted={ctx.expandQuoted} />
      ) : (
        m.body && <Bubble tone={note ? "note" : outgoing ? "out" : "in"}>{m.body}</Bubble>
      )}
      {!rich && <Attachments items={m.attachments} outgoing={outgoing} />}
      {m.delivery && !note && <DeliveryState d={m.delivery} />}
    </div>
  )
}

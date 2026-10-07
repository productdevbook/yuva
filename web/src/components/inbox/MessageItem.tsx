import { Trans, useLingui } from "@lingui/react/macro"
import { DownloadIcon, LockIcon, PaperclipIcon, TagIcon, UserIcon, UserXIcon } from "lucide-react"

import { PersonAvatar } from "@/components/common"
import { formatBytes, formatDateTime, useEnumText } from "@/components/common/text"
import { statusIcons } from "@/components/inbox/ConversationControls"
import { attachmentUrl, type Attachment, type Contact, type Label, type Member, type Message } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

type Ctx = {
  members: Map<string, Member>
  contact?: Contact
  labels: Label[]
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

export function MessageItem({ m, ctx }: { m: Message; ctx: Ctx }) {
  const { i18n } = useLingui()
  const author = useAuthorName(m, ctx)
  if (m.kind === "event") return <EventLine m={m} ctx={ctx} />
  const note = m.kind === "note"
  const outgoing = !note && m.direction === "out"
  return (
    <div
      className={cn("flex gap-2.5 px-4 py-1.5", outgoing && "flex-row-reverse")}
      data-testid={note ? "note" : "message"}
    >
      <PersonAvatar name={author} className="mt-5 size-7" />
      <div className={cn("flex max-w-[min(42rem,85%)] min-w-0 flex-col gap-1", outgoing && "items-end")}>
        <div className={cn("flex items-center gap-2 text-xs text-muted-foreground", outgoing && "flex-row-reverse")}>
          <span className="font-medium text-foreground">{author}</span>
          {note && (
            <span className="inline-flex items-center gap-1 rounded bg-amber-100 px-1.5 py-px text-[11px] font-medium text-amber-800 dark:bg-amber-900/50 dark:text-amber-200">
              <LockIcon className="size-3" />
              <Trans>Internal note</Trans>
            </span>
          )}
          <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
            {time(m.created_at, i18n.locale)}
          </time>
        </div>
        {m.body && (
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
        )}
        {m.attachments.length > 0 && (
          <div className={cn("flex max-w-full flex-col gap-1", outgoing && "items-end")}>
            {m.attachments.map((a) => (
              <AttachmentChip key={a.id} a={a} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

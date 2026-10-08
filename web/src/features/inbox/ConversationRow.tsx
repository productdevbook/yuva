import { Trans, useLingui } from "@lingui/react/macro"
import { CornerUpLeftIcon, FlagIcon } from "lucide-react"
import { Link } from "react-router"

import { ChannelIcon, Dot, LabelChip, PersonAvatar, TypingDots } from "@/components/common"
import { formatShort, useEnumText } from "@/components/common/text"
import { Checkbox } from "@/components/ui/checkbox"
import { priorityClass, statusIcons } from "@/features/conversation/controls/shared"
import { CategoryChip } from "@/features/conversation/Feedback"
import type { ConversationListItem } from "@/lib/api"
import { useContactTyping } from "@/lib/typing"
import { cn } from "@/lib/utils"
import { useChannel, useInboxes, useLabels, useMemberMap } from "@/lib/workspace"

export function ConversationRow({
  c,
  href,
  selected,
  showStatus,
  showInbox,
  picked,
  selecting,
  onPick,
  failure,
}: {
  c: ConversationListItem
  href: string
  selected: boolean
  showStatus: boolean
  showInbox: boolean
  picked: boolean
  selecting: boolean
  onPick: (on: boolean, range: boolean) => void
  failure?: string
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const members = useMemberMap()
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const labels = (useLabels().data ?? []).filter((l) => c.labels.includes(l.id))
  const kind = useChannel(c.channel_id).data?.kind
  const typing = useContactTyping(c.id)
  const assignee = c.assignee_id ? members.get(c.assignee_id) : undefined
  const name = c.contact.name || c.contact.email || t`Unnamed contact`
  const preview = c.last_message
  const notable = c.priority === "urgent" || c.priority === "high"
  const StatusIcon = statusIcons[c.status]
  const meta = (c.kind === "feedback" && c.feedback) || showStatus || notable || labels.length > 0
  return (
    <li
      className={cn("group relative transition-colors hover:bg-surface", picked && "bg-surface", selected && "bg-muted hover:bg-muted")}
      data-testid="conversation-item"
      data-picked={picked || undefined}
      data-failed={failure ? true : undefined}
    >
      <Link
        to={href}
        data-testid="conversation-row"
        data-unread={c.unread || undefined}
        aria-current={selected ? "true" : undefined}
        className="flex gap-3 px-4 py-3.5 outline-none focus-visible:bg-surface"
      >
        <span
          className={cn(
            "grid size-9 shrink-0 place-items-center rounded-full border text-muted-foreground group-hover:invisible group-has-[[data-slot=checkbox]:focus-visible]:invisible",
            (selecting || picked) && "invisible",
          )}
          data-testid={kind === "chat" ? "chat-badge" : undefined}
        >
          <ChannelIcon kind={kind} className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 text-sm">
            <span className={cn("min-w-0 truncate", c.unread ? "font-semibold" : "font-medium")}>{name}</span>
            {showInbox && inbox && (
              <span className="flex min-w-0 shrink items-center gap-1.5 text-xs text-faint">
                <Dot color={inbox.branding.color} className="size-1.5" />
                <span className="truncate">{inbox.name}</span>
              </span>
            )}
            <span className="ms-auto flex shrink-0 items-center gap-2">
              {assignee && (
                <span title={assignee.name || assignee.email}>
                  <PersonAvatar name={assignee.name || assignee.email} className="size-5 text-[9px]" />
                </span>
              )}
              <time className={cn("text-xs", c.unread ? "text-foreground" : "text-faint")} dateTime={c.last_activity_at}>
                {formatShort(c.last_activity_at, i18n.locale)}
              </time>
            </span>
          </div>
          <div className="mt-1 flex items-center gap-2 text-sm">
            {typing ? (
              <span className="flex min-w-0 items-center gap-1.5 text-brand" data-testid="row-typing">
                <TypingDots />
                <span className="truncate">
                  <Trans>typing…</Trans>
                </span>
              </span>
            ) : (
              <p className={cn("min-w-0 flex-1 truncate", c.unread ? "text-foreground" : "text-muted-foreground")}>
                <span className={cn(c.unread && "font-medium", !c.subject && "italic")}>{c.subject || <Trans>No subject</Trans>}</span>
                {preview && (
                  <span className="text-faint" data-testid="conversation-preview">
                    {" — "}
                    {preview.author_type === "member" && (
                      <CornerUpLeftIcon className="me-1 inline size-3 align-[-1px]" aria-label={t`Reply`} />
                    )}
                    {preview.text || <Trans>Attachment</Trans>}
                  </span>
                )}
              </p>
            )}
            {c.unread && (
              <span className="ms-auto size-2 shrink-0 rounded-full bg-brand" data-testid="unread-dot">
                <span className="sr-only">
                  <Trans>Unread</Trans>
                </span>
              </span>
            )}
          </div>
          {meta && (
            <div className="mt-1.5 flex min-w-0 items-center gap-3 text-xs text-faint">
              {c.kind === "feedback" && c.feedback && <CategoryChip category={c.feedback.category} />}
              {showStatus && (
                <span className="inline-flex items-center gap-1">
                  <StatusIcon className="size-3.5" />
                  {text.status[c.status]}
                </span>
              )}
              {notable && (
                <span className={cn("inline-flex items-center gap-1", priorityClass[c.priority])}>
                  <FlagIcon className="size-3.5" />
                  {text.priority[c.priority]}
                </span>
              )}
              {labels.slice(0, 3).map((l) => (
                <LabelChip key={l.id} name={l.name} color={l.color} />
              ))}
            </div>
          )}
          {failure && (
            <p className="mt-1.5 text-xs text-destructive" role="alert" data-testid="bulk-failure">
              {failure}
            </p>
          )}
        </div>
      </Link>
      <div
        className={cn(
          "absolute start-4 top-3.5 grid size-9 place-items-center opacity-0 group-hover:opacity-100 focus-within:opacity-100",
          (selecting || picked) && "opacity-100",
        )}
        onMouseDown={(e) => e.shiftKey && e.preventDefault()}
      >
        <Checkbox
          checked={picked}
          onCheckedChange={(on, details) => onPick(on, (details.event as MouseEvent).shiftKey === true)}
          aria-label={t`Select ${name}`}
          data-testid="row-select"
        />
      </div>
    </li>
  )
}

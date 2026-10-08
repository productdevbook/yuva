import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, PanelRightIcon } from "lucide-react"
import { Link } from "react-router"

import { ChannelIcon, Dot, ErrorLine } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { AssigneeMenu } from "@/features/conversation/controls/AssigneeMenu"
import { LabelsMenu } from "@/features/conversation/controls/LabelsMenu"
import { MoreMenu } from "@/features/conversation/controls/MoreMenu"
import { PriorityMenu } from "@/features/conversation/controls/PriorityMenu"
import type { ControlProps } from "@/features/conversation/controls/shared"
import { StatusMenu } from "@/features/conversation/controls/StatusMenu"
import { CategoryChip, FeedbackDetails } from "@/features/conversation/Feedback"
import { RelatedLine } from "@/features/conversation/RelatedLine"
import type { Channel } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export function ThreadHeader({
  controls,
  channel,
  contactName,
  backHref,
  hrefFor,
  move,
  moving,
  error,
  contactShown,
  onToggleContact,
}: {
  controls: ControlProps
  channel?: Channel
  contactName: string
  backHref: string
  hrefFor: (id: string) => string
  move: (inboxId: string) => void
  moving: boolean
  error: unknown
  contactShown: boolean
  onToggleContact: () => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const c = controls.conversation
  const inbox = useInboxes().data?.find((i) => i.id === c.inbox_id)
  const sep = <span aria-hidden>·</span>
  return (
    <header className="shrink-0 border-b px-4 pt-3 pb-3.5 sm:px-6">
      <div className="flex items-start gap-2">
        <Button variant="ghost" size="icon-sm" className="-ms-2 md:hidden" render={<Link to={backHref} />} aria-label={t`Back to the list`}>
          <ArrowLeftIcon />
        </Button>
        <div className="min-w-0 flex-1 pt-1">
          <h2 className={cn("truncate text-[1.05rem] font-semibold tracking-tight", !c.subject && "text-muted-foreground")} data-testid="thread-subject">
            {c.subject || <Trans>No subject</Trans>}
          </h2>
          <p className="mt-1 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-faint">
            <span className="truncate font-medium text-muted-foreground">{contactName}</span>
            {inbox && (
              <>
                {sep}
                <span className="inline-flex items-center gap-1.5">
                  <Dot color={inbox.branding.color} className="size-1.5" />
                  {inbox.name}
                </span>
              </>
            )}
            {channel && (
              <>
                {sep}
                <span className="inline-flex items-center gap-1" data-testid={channel.kind === "chat" ? "chat-badge" : undefined}>
                  <ChannelIcon kind={channel.kind} className="size-3.5" />
                  {text.channel[channel.kind]}
                </span>
              </>
            )}
            {c.kind === "feedback" && c.feedback && (
              <>
                {sep}
                <CategoryChip category={c.feedback.category} />
              </>
            )}
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
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <AssigneeMenu {...controls} />
        <StatusMenu {...controls} />
        <PriorityMenu {...controls} />
        <LabelsMenu {...controls} />
        <MoreMenu {...controls} move={move} moving={moving} />
        <ErrorLine error={error} className="text-xs" />
      </div>
      {c.kind === "feedback" && c.feedback && (
        <div className="mt-3">
          <FeedbackDetails feedback={c.feedback} />
        </div>
      )}
    </header>
  )
}

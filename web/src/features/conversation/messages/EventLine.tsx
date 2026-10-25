import { Trans, useLingui } from "@lingui/react/macro"
import { Link } from "react-router"

import { formatDateTime, useEnumText } from "@/components/common/text"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { broadcastLink } from "@/features/broadcasts/broadcastQueries"
import { useAutomationText } from "@/features/settings/automation/text"
import { timeOf, useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import type { Message } from "@/lib/api"

export function EventLine({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const automation = useAutomationText()
  const actor = useAuthorName(m, ctx)
  const e = m.event
  if (!e) return null
  const memberName = (id?: string) => {
    const x = id ? ctx.members.get(id) : undefined
    return x ? x.name || x.email : "?"
  }
  const labelNames = (ids?: string[]) =>
    (ids ?? []).map((id) => ctx.labels.find((l) => l.id === id)?.name ?? "?").join(", ")
  let body: React.ReactNode
  switch (e.type) {
    case "assigned": {
      const assignee = memberName(e.assignee_id)
      body = e.auto ? (
        <Trans>Assigned automatically to {assignee}</Trans>
      ) : (
        <Trans>
          {actor} assigned this to {assignee}
        </Trans>
      )
      break
    }
    case "unassigned":
      body = e.auto ? <Trans>Unassigned automatically</Trans> : <Trans>{actor} removed the assignee</Trans>
      break
    case "status_changed": {
      const status = text.statusOf(e.status ?? "open", ctx.feedback ? "feedback" : undefined)
      body =
        e.status === "closed" && ctx.feedback ? (
          <Trans>{actor} marked this as resolved</Trans>
        ) : e.status === "closed" ? (
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
    case "rated":
      body =
        e.rating === "bad" ? (
          <Trans>{actor} rated the conversation 👎</Trans>
        ) : (
          <Trans>{actor} rated the conversation 👍</Trans>
        )
      break
    case "broadcast_reply": {
      const broadcast = e.broadcast
      const name = broadcast?.name
      body = broadcast ? (
        <Trans>
          Reply to{" "}
          <Link to={broadcastLink(broadcast.id)} className="font-medium text-muted-foreground hover:underline">
            {name}
          </Link>
        </Trans>
      ) : (
        <Trans>Reply to a broadcast</Trans>
      )
      break
    }
    case "feedback_progress": {
      const progress = e.feedback_progress ? text.progress[e.feedback_progress] : ""
      body = e.feedback_progress ? (
        <Trans>
          {actor} marked this {progress}
        </Trans>
      ) : (
        <Trans>{actor} cleared the progress</Trans>
      )
      break
    }
    case "macro_applied": {
      const macro = e.macro?.name ?? ""
      body = e.skipped?.includes("assignee") ? (
        <Trans>
          {actor} applied the macro {macro}, assignee skipped
        </Trans>
      ) : (
        <Trans>
          {actor} applied the macro {macro}
        </Trans>
      )
      break
    }
    case "rule_applied": {
      const rule = m.rule?.name ?? ""
      const done = (e.actions ?? [])
        .filter((a) => a.result === "applied")
        .map((a) => automation.action[a.type])
        .join(", ")
      body = done ? (
        <Trans>
          Rule {rule} ran: {done}
        </Trans>
      ) : (
        <Trans>Rule {rule} ran, nothing needed changing</Trans>
      )
      break
    }
    case "moved": {
      const from = ctx.inboxes.find((i) => i.id === e.previous_inbox_id)?.name ?? t`another inbox`
      body = <Trans>Moved from {from}</Trans>
      break
    }
  }
  return (
    <Marker className="justify-start py-1.5 ps-[46px]" data-testid="event-line">
      <MarkerContent className="max-w-full px-0 text-start text-caption text-faint">
        {body}
        <span aria-hidden> · </span>
        <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
      </MarkerContent>
    </Marker>
  )
}

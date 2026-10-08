import { Trans, useLingui } from "@lingui/react/macro"

import { formatDateTime, useEnumText } from "@/components/common/text"
import { timeOf, useAuthorName, type ThreadContext } from "@/features/conversation/messages/context"
import type { Message } from "@/lib/api"

export function EventLine({ m, ctx }: { m: Message; ctx: ThreadContext }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const actor = useAuthorName(m, ctx)
  const e = m.event
  if (!e) return null
  const memberName = (id?: string) => {
    const x = id ? ctx.members.get(id) : undefined
    return x ? x.name || x.email : "?"
  }
  const labelNames = (ids?: string[]) => (ids ?? []).map((id) => ctx.labels.find((l) => l.id === id)?.name ?? "?").join(", ")
  let body: React.ReactNode
  switch (e.type) {
    case "assigned": {
      const assignee = memberName(e.assignee_id)
      body = <Trans>{actor} assigned this to {assignee}</Trans>
      break
    }
    case "unassigned":
      body = <Trans>{actor} removed the assignee</Trans>
      break
    case "status_changed": {
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
      body = e.rating === "bad" ? <Trans>{actor} rated the conversation 👎</Trans> : <Trans>{actor} rated the conversation 👍</Trans>
      break
    case "moved": {
      const from = ctx.inboxes.find((i) => i.id === e.previous_inbox_id)?.name ?? t`another inbox`
      body = <Trans>Moved from {from}</Trans>
      break
    }
  }
  return (
    <div className="my-2 flex justify-center" data-testid="event-line">
      <span className="max-w-full rounded-full border bg-card px-2.5 py-[3px] text-center text-xs text-faint">
        {body}
        <span aria-hidden> · </span>
        <time dateTime={m.created_at} title={formatDateTime(m.created_at, i18n.locale)}>
          {timeOf(m.created_at, i18n.locale)}
        </time>
      </span>
    </div>
  )
}

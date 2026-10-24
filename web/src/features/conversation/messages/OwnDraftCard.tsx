import { Trans, useLingui } from "@lingui/react/macro"
import { ClockIcon, TriangleAlertIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, Kbd } from "@/components/common"
import { formatWhen, SendLaterMenu } from "@/components/common/SendLater"
import { Button } from "@/components/ui/button"
import { useDraftActions } from "@/features/conversation/queries"
import { look } from "@/features/inbox/PageFrame"
import type { Message } from "@/lib/api"
import { cn } from "@/lib/utils"

export type DraftKeys = { edit: string; time: string; cancel: string; again: string }

export function OwnDraftCard({
  m,
  keys,
  timeOpen,
  onTimeOpenChange,
  onEdit,
  onCancel,
}: {
  m: Message
  keys?: DraftKeys
  timeOpen?: boolean
  onTimeOpenChange?: (open: boolean) => void
  onEdit: (m: Message) => void
  onCancel: (m: Message) => void
}) {
  const { t, i18n } = useLingui()
  const [timeState, setTimeState] = useState(false)
  const { send, discard, reschedule } = useDraftActions()
  const busy = send.isPending || discard.isPending || reschedule.isPending
  const failed = !m.scheduled_at && !!m.send_error
  const when = m.scheduled_at ? formatWhen(m.scheduled_at, i18n.locale) : ""
  const btn = cn(look.btnSm, "gap-1.5")
  return (
    <article
      className={cn(
        "flex min-w-0 flex-col gap-2 rounded-xl border border-dashed bg-card px-4 py-3",
        failed ? "border-destructive/50" : "border-warning/50"
      )}
      data-testid={failed ? "draft-failed" : "draft-scheduled"}
      data-message-id={m.id}
    >
      <span
        className={cn(
          "flex items-center gap-1.5 text-caption font-semibold",
          failed ? "text-destructive" : "text-warning"
        )}
      >
        {failed ? <TriangleAlertIcon className="size-3.5" /> : <ClockIcon className="size-3.5" />}
        {failed ? <Trans>Could not be sent</Trans> : <Trans>Scheduled for {when}</Trans>}
      </span>
      {failed && m.send_error && (
        <p className="text-small text-destructive" data-testid="draft-send-error">
          {m.send_error.detail}
        </p>
      )}
      <p className="line-clamp-4 text-reading break-words whitespace-pre-wrap">{m.body}</p>
      <div className="flex flex-wrap items-center gap-1.5" role="toolbar" aria-label={t`Scheduled message`}>
        {failed && (
          <Button
            className={look.btnInkSm}
            disabled={busy}
            onClick={() => send.mutate(m)}
            data-testid="draft-send-again"
          >
            <Trans>Send again</Trans>
            {keys && <Kbd className="bg-background/15 text-background phone:hidden">{keys.again}</Kbd>}
          </Button>
        )}
        <Button variant="outline" className={btn} disabled={busy} onClick={() => onEdit(m)} data-testid="draft-edit">
          <Trans>Edit</Trans>
          {keys && <Kbd className="phone:hidden">{keys.edit}</Kbd>}
        </Button>
        {!failed && (
          <>
            <SendLaterMenu
              open={timeOpen ?? timeState}
              onOpenChange={onTimeOpenChange ?? setTimeState}
              side="bottom"
              align="start"
              title={<Trans>Change the time</Trans>}
              confirm={<Trans>Reschedule</Trans>}
              initial={m.scheduled_at ? new Date(m.scheduled_at) : undefined}
              onPick={(at) => reschedule.mutate({ m, at })}
              trigger={<Button variant="outline" className={btn} disabled={busy} data-testid="draft-change-time" />}
            >
              <Trans>Change time</Trans>
              {keys && <Kbd className="phone:hidden">{keys.time}</Kbd>}
            </SendLaterMenu>
            <Button
              variant="outline"
              className={btn}
              disabled={busy}
              onClick={() => onCancel(m)}
              data-testid="draft-cancel-schedule"
            >
              <Trans>Cancel schedule</Trans>
              {keys && <Kbd className="phone:hidden">{keys.cancel}</Kbd>}
            </Button>
          </>
        )}
        {failed && (
          <Button
            variant="ghost"
            className="h-[30px] rounded-lg px-2"
            disabled={busy}
            onClick={() => discard.mutate(m)}
            data-testid="draft-discard"
          >
            <Trans>Discard</Trans>
          </Button>
        )}
      </div>
      <ErrorLine error={send.error ?? discard.error ?? reschedule.error} className="text-caption" />
    </article>
  )
}

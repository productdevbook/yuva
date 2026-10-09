import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon } from "lucide-react"

import { formatDateTime } from "@/components/common/text"
import type { MessageDelivery } from "@/lib/api"

export function DeliveryState({ d }: { d: MessageDelivery }) {
  const { i18n } = useLingui()
  if (d.state === "queued") {
    return (
      <span className="text-caption text-faint" data-testid="delivery" data-state="queued">
        <Trans>Sending by e-mail…</Trans>
      </span>
    )
  }
  if (d.state === "sent") {
    return (
      <span
        className="inline-flex items-center gap-1 text-caption text-faint"
        title={formatDateTime(d.updated_at, i18n.locale)}
        data-testid="delivery"
        data-state="sent"
      >
        <CheckIcon className="size-3.5" />
        <Trans>Sent by e-mail</Trans>
      </span>
    )
  }
  return (
    <div
      role="alert"
      className="flex max-w-sm flex-col gap-0.5 rounded-xl bg-destructive/6 px-3 py-2 text-caption"
      data-testid="delivery"
      data-state="failed"
    >
      <span className="font-medium text-destructive">
        <Trans>Not delivered</Trans>
      </span>
      {d.error && <span className="break-words">{d.error}</span>}
      <span className="text-muted-foreground">
        <Trans>Fix the address or the channel's SMTP settings, then send the reply again.</Trans>
      </span>
    </div>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"

import { formatRelative, useEnumText } from "@/components/common/text"
import { StatusTag } from "@/features/settings/ui"
import type { WebhookAttempt, WebhookDeliveryState, WebhookEndpoint } from "@/lib/api"
import { cn } from "@/lib/utils"

export function EndpointStatus({ e }: { e: WebhookEndpoint }) {
  if (!e.enabled && e.disabled_reason) {
    return (
      <StatusTag tone="danger" data-testid="webhook-status" data-status="disabled">
        <Trans>Turned off by Yuva</Trans>
      </StatusTag>
    )
  }
  if (!e.enabled) {
    return (
      <StatusTag data-testid="webhook-status" data-status="off">
        <Trans>Off</Trans>
      </StatusTag>
    )
  }
  if (e.failing_since) {
    return (
      <StatusTag tone="warning" data-testid="webhook-status" data-status="failing">
        <Trans>Failing</Trans>
      </StatusTag>
    )
  }
  return (
    <StatusTag tone="success" data-testid="webhook-status" data-status="on">
      <Trans>On</Trans>
    </StatusTag>
  )
}

export function DeliveryStateTag({ state }: { state: WebhookDeliveryState }) {
  const text = useEnumText()
  return (
    <StatusTag tone={state === "failed" ? "danger" : state === "succeeded" ? "success" : "warning"} data-testid="delivery-state" data-state={state}>
      {text.delivery[state]}
    </StatusTag>
  )
}

export function Attempts({ n }: { n: number }) {
  return n === 1 ? <Trans>1 attempt</Trans> : <Trans>{n} attempts</Trans>
}

export function NextAttempt({ at }: { at: string }) {
  const { i18n } = useLingui()
  const when = formatRelative(at, i18n.locale)
  return (
    <span>
      <Trans>next attempt {when}</Trans>
    </span>
  )
}

export function AttemptResult({ a }: { a: WebhookAttempt }) {
  return <span className={cn("font-mono text-xs", a.success ? "text-success" : "text-destructive")}>{a.status_code ?? <Trans>no answer</Trans>}</span>
}

export function ManualTag() {
  return (
    <span className="ms-1.5 rounded-full bg-muted px-1.5 py-px text-[10px] font-medium text-muted-foreground">
      <Trans>manual</Trans>
    </span>
  )
}

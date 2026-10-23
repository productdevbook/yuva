import { Trans, useLingui } from "@lingui/react/macro"

import { SlaBadge, useSlaNow, useSlaText } from "@/components/common"
import { formatDateTime, formatDuration, formatRelative } from "@/components/common/text"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { look } from "@/features/inbox/PageFrame"
import type { Conversation, ConversationSlaTarget, SlaTargetName } from "@/lib/api"
import { targetState } from "@/lib/sla"
import { cn } from "@/lib/utils"

function useTargetText(): Record<SlaTargetName, string> {
  const { t } = useLingui()
  return { first_reply: t`First reply`, next_reply: t`Next reply`, resolution: t`Resolution` }
}

function TargetDetail({ target, state, now }: { target: ConversationSlaTarget; state: string; now: number }) {
  const { i18n } = useLingui()
  const when = (iso: string) => formatRelative(iso, i18n.locale, now)
  if (state === "met" && target.met_at) {
    const at = when(target.met_at)
    return <Trans>Met {at}</Trans>
  }
  if (state === "breached" && target.met_at) {
    const at = formatDateTime(target.met_at, i18n.locale)
    return <Trans>Breached, met late at {at}</Trans>
  }
  if (state === "breached" && target.due_at) {
    const at = when(target.due_at)
    return <Trans>Was due {at}</Trans>
  }
  if (state === "paused" && target.remaining_seconds !== undefined) {
    const left = formatDuration(target.remaining_seconds, i18n.locale)
    return <Trans>{left} left when it resumes</Trans>
  }
  if (target.due_at && (state === "on_track" || state === "due_soon")) {
    const at = when(target.due_at)
    return <Trans>Due {at}</Trans>
  }
  if (state === "stopped") return <Trans>Closed before a reply</Trans>
  return null
}

export function SlaPopover({
  c,
  open,
  onOpenChange,
}: {
  c: Conversation
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useLingui()
  const text = useSlaText()
  const names = useTargetText()
  const { now, policy } = useSlaNow(c.sla)
  const sla = c.sla
  const policyName = policy?.name
  if (!sla) return null
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={<button type="button" className={cn(look.chip, "gap-1.5")} />}
        aria-label={t`SLA targets (W)`}
        title={t`SLA targets (W)`}
        data-testid="sla-trigger"
      >
        <SlaBadge sla={sla} />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 max-w-[calc(100vw-32px)] gap-3" data-testid="sla-popover">
        <p className="font-medium">{policy ? <Trans>SLA · {policyName}</Trans> : <Trans>SLA</Trans>}</p>
        {sla.targets.length === 0 ? (
          <p className="text-muted-foreground">
            <Trans>No target has started yet.</Trans>
          </p>
        ) : (
          <ul className="flex flex-col gap-2.5">
            {sla.targets.map((target) => {
              const state = targetState(target, policy?.warn_minutes, now)
              return (
                <li key={target.target} className="flex flex-col gap-0.5" data-testid="sla-target" data-state={state}>
                  <span className="flex items-baseline justify-between gap-3">
                    <span className="font-medium">{names[target.target]}</span>
                    <span
                      className={cn(
                        "text-small font-medium",
                        state === "breached" && "text-destructive",
                        state === "due_soon" && "text-warning",
                        state === "met" && "text-success",
                        (state === "on_track" || state === "paused" || state === "stopped") && "text-muted-foreground"
                      )}
                    >
                      {text[state]}
                    </span>
                  </span>
                  <span className="text-small text-muted-foreground">
                    <TargetDetail target={target} state={state} now={now} />
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  )
}

import { useLingui } from "@lingui/react/macro"
import { AlarmClockIcon, AlarmClockOffIcon, CheckIcon, PauseIcon, TimerIcon, TriangleAlertIcon } from "lucide-react"

import { formatRelative } from "@/components/common/text"
import type { ConversationSla, SlaState, SlaTargetState } from "@/lib/api"
import { slaState, useMinute } from "@/lib/sla"
import { cn } from "@/lib/utils"
import { useSlaPolicies } from "@/lib/workspace"

type Shown = SlaState | SlaTargetState

const tone: Record<Shown, string> = {
  on_track: "text-muted-foreground",
  due_soon: "text-warning",
  breached: "text-destructive",
  met: "text-success",
  paused: "text-faint",
  stopped: "text-faint",
}

const icons: Record<Shown, React.ComponentType<{ className?: string }>> = {
  on_track: TimerIcon,
  due_soon: AlarmClockIcon,
  breached: TriangleAlertIcon,
  met: CheckIcon,
  paused: PauseIcon,
  stopped: AlarmClockOffIcon,
}

export function useSlaText(): Record<Shown, string> {
  const { t } = useLingui()
  return {
    on_track: t`On track`,
    due_soon: t`Due soon`,
    breached: t`Breached`,
    met: t`Met`,
    paused: t`Paused`,
    stopped: t`Stopped`,
  }
}

export function useSlaNow(sla: ConversationSla | undefined) {
  const now = useMinute()
  const policy = useSlaPolicies().data?.find((p) => p.id === sla?.policy_id)
  return { now, policy, state: sla ? slaState(sla, policy, now) : undefined }
}

export function SlaBadge({ sla, className, ...props }: { sla: ConversationSla } & React.ComponentProps<"span">) {
  const { i18n } = useLingui()
  const text = useSlaText()
  const { now, state } = useSlaNow(sla)
  if (!state) return null
  const Icon = icons[state]
  const timed = sla.due_at && (state === "on_track" || state === "due_soon" || state === "breached")
  return (
    <span
      className={cn("inline-flex min-w-0 shrink-0 items-center gap-1 text-caption font-medium", tone[state], className)}
      data-testid="sla-badge"
      data-state={state}
      {...props}
    >
      <Icon className="size-3.5 shrink-0" />
      <span className="truncate">
        {text[state]}
        {timed && sla.due_at && <span className="font-normal"> · {formatRelative(sla.due_at, i18n.locale, now)}</span>}
      </span>
    </span>
  )
}

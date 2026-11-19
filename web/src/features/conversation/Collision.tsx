import { Trans, useLingui } from "@lingui/react/macro"

import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { PersonAvatar } from "@/components/common"
import { firstName } from "@/features/conversation/actions"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

const line =
  "flex shrink-0 items-center gap-x-2.5 gap-y-1 border-b border-line-soft px-3.5 py-2 text-small phone:px-2.5 phone:whitespace-nowrap"
const textButton = "h-6 shrink-0 px-1.5 text-small font-semibold text-fg2 hover:text-foreground"

export function TypingCollision({
  name: full,
  typing,
  onLeave,
  onClaim,
}: {
  name: string
  typing: boolean
  onLeave: () => void
  onClaim: () => void
}) {
  const { t } = useLingui()
  const name = firstName(full)
  const replyKey = keyLabel(SHORTCUTS.reply).join("")
  const leaveKey = keyLabel(SHORTCUTS.leave).join("")
  return (
    <div
      role="status"
      className={cn(line, typing ? "bg-warning/8" : "bg-subtle")}
      data-testid="collision"
      data-typing={typing}
    >
      <PersonAvatar name={full} className="size-6 shrink-0 bg-mate text-[10px] text-white" />
      <span className="min-w-0 flex-1 truncate">
        <b className="font-semibold text-foreground">
          {typing ? <Trans>{name} is typing a reply too.</Trans> : <Trans>{name} is looking at this too.</Trans>}
        </b>{" "}
        <span className="text-fg2 phone:hidden">
          {typing ? <Trans>Do not both write.</Trans> : <Trans>Agree who replies.</Trans>}
        </span>
      </span>
      <Button variant="ghost" size="sm" className={textButton} onClick={onClaim} title={t`I'll reply (${replyKey})`}>
        <Trans>I'll reply</Trans>
      </Button>
      <Button
        variant="ghost"
        size="sm"
        className={textButton}
        onClick={onLeave}
        title={t`Leave it to ${name} (${leaveKey})`}
      >
        <Trans>Leave it to {name}</Trans>
      </Button>
    </div>
  )
}

export function Beaten({ name: full }: { name: string }) {
  const name = firstName(full)
  return (
    <div role="status" className={cn(line, "bg-subtle")} data-testid="beaten">
      <PersonAvatar name={full} className="size-6 shrink-0 bg-mate text-[10px] text-white" />
      <span className="min-w-0 flex-1">
        <b className="font-semibold text-foreground">
          <Trans>{name} replied before you.</Trans>
        </b>{" "}
        <span className="text-fg2">
          <Trans>Your draft is still here; send it as well or clear it.</Trans>
        </span>
      </span>
    </div>
  )
}

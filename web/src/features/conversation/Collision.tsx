import { Trans } from "@lingui/react/macro"

import { PersonAvatar } from "@/components/common"
import { firstName } from "@/features/conversation/actions"
import { cn } from "@/lib/utils"

const box = "relative z-[1] mt-6 -mb-3 flex items-center gap-2.5 rounded-[14px] border py-2.5 ps-3 pe-2.5 text-[13px] text-muted-foreground phone:flex-wrap"

export function TypingCollision({ name: full, typing, onLeave, onClaim }: { name: string; typing: boolean; onLeave: () => void; onClaim: () => void }) {
  const name = firstName(full)
  return (
    <div className={cn(box, typing ? "border-mate/30 bg-mate/8" : "border-mate/20 bg-card")} role="status" data-testid="collision" data-typing={typing}>
      <PersonAvatar name={full} className="size-[26px] bg-mate text-[10px] font-semibold text-white" />
      <span className="min-w-0 flex-1 phone:basis-[calc(100%-40px)]">
        {typing ? (
          <>
            <b className="font-medium text-foreground">
              <Trans>{name} is typing a reply too.</Trans>
            </b>{" "}
            <Trans>Do not both write.</Trans>
          </>
        ) : (
          <>
            <b className="font-medium text-foreground">
              <Trans>{name} is looking at this too.</Trans>
            </b>{" "}
            <Trans>Agree who replies.</Trans>
          </>
        )}
      </span>
      <button type="button" onClick={onLeave} className="h-8 rounded-full border bg-card px-3 whitespace-nowrap hover:border-faint">
        <Trans>Leave it to {name}</Trans>
      </button>
      <button type="button" onClick={onClaim} className="h-8 rounded-full border border-primary bg-primary px-3 whitespace-nowrap text-white hover:brightness-105">
        <Trans>I'll reply</Trans>
      </button>
    </div>
  )
}

export function Beaten({ name: full }: { name: string }) {
  const name = firstName(full)
  return (
    <div className={cn(box, "bg-card")} role="status" data-testid="beaten">
      <PersonAvatar name={full} className="size-[26px] bg-mate text-[10px] font-semibold text-white" />
      <span className="min-w-0 flex-1">
        <b className="font-medium text-foreground">
          <Trans>{name} replied before you.</Trans>
        </b>{" "}
        <Trans>Your draft is still here; send it as well or clear it.</Trans>
      </span>
    </div>
  )
}

import { Trans } from "@lingui/react/macro"
import { TriangleAlertIcon } from "lucide-react"
import { createContext, useContext } from "react"
import { createPortal } from "react-dom"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { look } from "@/features/inbox/PageFrame"
import { cn } from "@/lib/utils"

type Slots = { toolbar: HTMLElement | null; actions: HTMLElement | null }

export const SlotsContext = createContext<Slots>({ toolbar: null, actions: null })

export function ToolbarPortal({ children }: { children: React.ReactNode }) {
  const { toolbar } = useContext(SlotsContext)
  return toolbar ? createPortal(children, toolbar) : null
}

export function ActionsPortal({ children }: { children: React.ReactNode }) {
  const { actions } = useContext(SlotsContext)
  return actions ? createPortal(children, actions) : null
}

export const headClass = "text-[11px] leading-4 font-semibold tracking-[0.05em] text-faint uppercase"

export function Tiles({
  children,
  className,
  testId,
}: {
  children: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <div className={cn("flex flex-wrap gap-2.5", className)} data-testid={testId}>
      {children}
    </div>
  )
}

export function SlaBar({ met, breached }: { met: number; breached: number }) {
  if (met + breached === 0) return null
  return (
    <span className="flex h-2 w-[72px] shrink-0 gap-0.5" aria-hidden>
      {met > 0 && <i className="block h-full rounded-s bg-success" style={{ flexGrow: met, flexBasis: 0 }} />}
      {breached > 0 && (
        <i className="block h-full min-w-0.5 rounded-e bg-destructive" style={{ flexGrow: breached, flexBasis: 0 }} />
      )}
    </span>
  )
}

export function LoadProblem({ error, onRetry, testId }: { error: unknown; onRetry: () => void; testId?: string }) {
  return (
    <div className="mx-auto flex max-w-sm flex-col items-center gap-2.5 py-12 text-center" data-testid={testId}>
      <span className="grid size-8 place-items-center rounded-full bg-destructive text-white">
        <TriangleAlertIcon className="size-4" aria-hidden />
      </span>
      <ErrorLine error={error} />
      <Button variant="outline" className={look.btnSm} onClick={onRetry}>
        <Trans>Try again</Trans>
      </Button>
    </div>
  )
}

export function ReportSkeleton({ tiles = 5, testId }: { tiles?: number; testId?: string }) {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" data-testid={testId}>
      <Tiles>
        {Array.from({ length: tiles }, (_, i) => (
          <div
            key={i}
            className="flex min-w-0 flex-1 basis-0 flex-col gap-1.5 rounded-[14px] border bg-card px-4 py-3.5 @max-[900px]:basis-[calc(50%-5px)]"
          >
            <Skeleton className="h-3.5 w-20" />
            <Skeleton className="h-7 w-16" />
            <Skeleton className="h-3 w-24" />
          </div>
        ))}
      </Tiles>
      <Skeleton className="h-72 rounded-[14px]" />
    </div>
  )
}

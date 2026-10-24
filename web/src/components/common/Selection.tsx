import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, SquareCheckBigIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Selection } from "@/hooks/use-selection"
import { cn } from "@/lib/utils"

export function SelectToggle({ selection, className }: { selection: Selection; className?: string }) {
  const { t } = useLingui()
  const { selecting } = selection
  return (
    <Button
      variant="outline"
      className={cn(look.btn, "w-[34px] px-0", selecting && "bg-muted", className)}
      onClick={() => (selecting ? selection.stop() : selection.start())}
      aria-pressed={selecting}
      aria-label={t`Select`}
      title={t`Select`}
      data-testid="select-toggle"
    >
      <SquareCheckBigIcon />
    </Button>
  )
}

export function RowCheck({
  checked,
  name,
  onToggle,
  className,
}: {
  checked: boolean
  name: string
  onToggle: (range: boolean) => void
  className?: string
}) {
  const { t } = useLingui()
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={t`Select ${name}`}
      onClick={(e) => onToggle(e.shiftKey)}
      className={cn(
        "grid size-5 shrink-0 place-items-center rounded-[6px] border outline-none focus-visible:ring-2 focus-visible:ring-ring",
        checked ? "border-brand bg-brand text-white" : "border-input bg-background",
        className
      )}
      data-testid="row-check"
    >
      {checked && <CheckIcon className="size-3.5" strokeWidth={3} />}
    </button>
  )
}

export function SelectionBar({
  selection,
  busy,
  children,
  className,
}: {
  selection: Selection
  busy?: boolean
  children: React.ReactNode
  className?: string
}) {
  const { t } = useLingui()
  const picked = selection.count
  useHotkeys({ Escape: selection.stop }, selection.selecting)
  if (!selection.selecting) return null
  return (
    <div
      className={cn("flex shrink-0 flex-wrap items-center gap-1.5", className)}
      role="toolbar"
      aria-label={t`Selection`}
      aria-busy={busy}
      data-testid="selection-bar"
    >
      <span className="me-1 min-w-0 truncate text-small font-medium" aria-live="polite">
        <Trans>Selected: {picked}</Trans>
      </span>
      <Button
        variant="outline"
        className={look.btnSm}
        onClick={selection.toggleAll}
        aria-pressed={selection.all}
        data-testid="select-all"
      >
        <Trans>Select all</Trans>
      </Button>
      {children}
      <Button
        variant="ghost"
        className="h-[30px] rounded-lg px-2"
        onClick={selection.stop}
        title={t`Clear the selection (Esc)`}
        data-testid="select-cancel"
      >
        <Trans>Cancel</Trans>
      </Button>
    </div>
  )
}

export function BulkButton({
  selection,
  destructive,
  className,
  ...props
}: React.ComponentProps<typeof Button> & { selection: Selection; destructive?: boolean }) {
  return (
    <Button
      variant="outline"
      className={cn(look.btnSm, destructive && "text-destructive", className)}
      {...props}
      disabled={selection.count === 0 || props.disabled}
    />
  )
}

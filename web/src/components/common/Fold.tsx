import { ChevronDownIcon } from "lucide-react"

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { cn } from "@/lib/utils"

export function FoldPanel({
  label,
  className,
  children,
  testId,
}: {
  label: string
  className?: string
  children: React.ReactNode
  testId?: string
}) {
  return (
    <aside
      aria-label={label}
      className={cn("flex min-h-0 flex-col overflow-y-auto rounded-xl border bg-card", className)}
      data-testid={testId}
    >
      {children}
    </aside>
  )
}

export function Fold({
  title,
  summary,
  open,
  onOpenChange,
  children,
  testId,
}: {
  title: React.ReactNode
  summary?: React.ReactNode
  open: boolean
  onOpenChange: (open: boolean) => void
  children: React.ReactNode
  testId?: string
}) {
  return (
    <Collapsible
      open={open}
      onOpenChange={onOpenChange}
      render={<section className="border-t border-line-soft first:border-t-0" data-testid={testId} />}
    >
      <CollapsibleTrigger
        className="flex h-[42px] w-full items-center gap-2 px-3.5 text-start text-body font-semibold outline-none hover:bg-subtle focus-visible:bg-subtle"
        data-testid={testId ? `${testId}-toggle` : undefined}
      >
        <ChevronDownIcon
          aria-hidden
          className={cn("size-3.5 shrink-0 text-muted-foreground transition-transform", !open && "-rotate-90")}
        />
        <span className="shrink-0">{title}</span>
        {summary !== undefined && (
          <span className="ms-auto min-w-0 truncate text-small font-normal text-muted-foreground">{summary}</span>
        )}
      </CollapsibleTrigger>
      <CollapsibleContent className="px-3.5 pt-0.5 pb-3.5 text-small">{children}</CollapsibleContent>
    </Collapsible>
  )
}

export function FoldSub({ children }: { children: React.ReactNode }) {
  return <div className="mt-3.5 mb-1.5 text-caption font-semibold text-muted-foreground first:mt-0">{children}</div>
}

export function FoldRow({ label, value }: { label: React.ReactNode; value: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-3 py-1 text-small">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-end font-medium">{value}</span>
    </div>
  )
}

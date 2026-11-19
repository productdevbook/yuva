import { Trans, useLingui } from "@lingui/react/macro"

import { Kbd } from "@/components/common/Kbd"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Empty, EmptyContent, EmptyDescription } from "@/components/ui/empty"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

export function PageTitle({
  title,
  sub,
  actions,
  testId,
}: {
  title: React.ReactNode
  sub?: React.ReactNode
  actions?: React.ReactNode
  testId?: string
}) {
  return (
    <header className="flex flex-wrap items-center gap-x-4 gap-y-3 pt-6" data-testid={testId}>
      <div className="flex min-w-0 flex-[1_1_240px] flex-wrap items-baseline gap-x-2.5 gap-y-0.5">
        <h1 className="m-0 min-w-0 truncate text-[22px] leading-7 font-semibold tracking-[-0.01em]">{title}</h1>
        {sub && <p className="m-0 min-w-0 truncate text-body text-muted-foreground">{sub}</p>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}

export function FilterRow({ children, testId }: { children: React.ReactNode; testId?: string }) {
  return (
    <div className="relative mt-4 flex flex-wrap items-center gap-2" data-testid={testId}>
      {children}
    </div>
  )
}

export const filterButton = "h-8 gap-1.5 px-2.5 font-normal"
export const filterButtonOn = "border-brand/40 bg-brand-wash font-medium text-brand hover:border-brand/60"

export function ListBox({
  bar,
  children,
  className,
  testId,
}: {
  bar?: React.ReactNode
  children: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <Card className={cn("mt-3 gap-0 py-0 shadow-none", className)} data-testid={testId}>
      {bar}
      <div className="flex min-w-0 flex-col [&>[data-row]+[data-row]]:border-t">{children}</div>
    </Card>
  )
}

export function ListBoxSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} data-row className="flex items-center gap-3 border-line-soft px-3 py-3.5">
          <Skeleton className="size-4 rounded-[5px]" />
          <Skeleton className="h-4 w-36" />
          <Skeleton className="h-4 flex-1" />
        </div>
      ))}
    </>
  )
}

export function ListBoxEmpty({
  children,
  action,
  testId,
}: {
  children: React.ReactNode
  action?: React.ReactNode
  testId?: string
}) {
  return (
    <Empty className="gap-3 p-10" data-testid={testId}>
      <EmptyDescription className="text-body">{children}</EmptyDescription>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  )
}

export function boxRowClass({ current, cursor, checked }: { current?: boolean; cursor?: boolean; checked?: boolean }) {
  return cn(
    "relative flex min-w-0 scroll-mt-3 scroll-mb-6 items-start gap-3 border-line-soft ps-3 transition-colors hover:bg-subtle",
    (current || cursor) && "bg-subtle shadow-[inset_2px_0_0_var(--brand)]",
    checked && "bg-brand-wash hover:bg-brand-wash"
  )
}

export function RowCheckbox({
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
    <Checkbox
      checked={checked}
      onClick={(e) => {
        e.preventDefault()
        onToggle(e.shiftKey)
      }}
      aria-label={t`Select ${name}`}
      className={cn("mt-[15px] bg-card", className)}
      data-testid="row-check"
    />
  )
}

export function BulkBar({
  picked,
  total,
  onToggleAll,
  onCancel,
  children,
  busy,
}: {
  picked: number
  total: number
  onToggleAll: () => void
  onCancel: () => void
  children: React.ReactNode
  busy?: boolean
}) {
  const { t } = useLingui()
  const all = total > 0 && picked === total
  return (
    <div
      role="toolbar"
      aria-label={t`Selection`}
      aria-busy={busy}
      className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b bg-brand-wash/70 py-2 ps-3 pe-2.5"
      data-testid="selection-bar"
    >
      <Checkbox
        checked={all}
        indeterminate={picked > 0 && !all}
        disabled={total === 0}
        onClick={(e) => {
          e.preventDefault()
          onToggleAll()
        }}
        aria-label={t`Select all`}
        className="bg-card"
        data-testid="select-all"
      />
      <span className="min-w-16 text-small font-semibold tabular-nums" aria-live="polite">
        <Trans>{picked} selected</Trans>
      </span>
      <div className="flex min-w-0 flex-wrap items-center gap-2">{children}</div>
      <Button
        variant="outline"
        size="sm"
        className="ms-auto font-normal"
        onClick={onCancel}
        title={t`Clear the selection (Esc)`}
        data-testid="cancel-selection"
      >
        <Trans>Cancel</Trans>
      </Button>
    </div>
  )
}

export function bulkButton(destructive?: boolean) {
  return cn(
    "h-8 bg-card px-3 font-normal",
    destructive && "border-destructive/50 text-destructive hover:border-destructive hover:text-destructive"
  )
}

export function KeyHints({ items }: { items: [string[], React.ReactNode][] }) {
  return (
    <p className="m-0 mt-2.5 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-caption text-muted-foreground phone:hidden">
      {items.map(([keys, label], i) => (
        <span key={i} className="inline-flex items-center gap-1">
          {keys.map((k) => (
            <Kbd key={k}>{k}</Kbd>
          ))}
          {label}
        </span>
      ))}
    </p>
  )
}

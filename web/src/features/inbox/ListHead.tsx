import { useLingui } from "@lingui/react/macro"

import { cn } from "@/lib/utils"

export function ListHead({
  title,
  sub,
  action,
  children,
  className,
  testId,
}: {
  title: React.ReactNode
  sub?: React.ReactNode
  action?: React.ReactNode
  children?: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <header
      className={cn("flex shrink-0 flex-col border-b px-4 pt-4", !children && "pb-3.5", className)}
      data-testid={testId}
    >
      <div className="flex min-h-8 min-w-0 items-center gap-2">
        <h1 className="min-w-0 flex-1 truncate text-title">{title}</h1>
        {action && <div className="flex shrink-0 items-center gap-1">{action}</div>}
      </div>
      {sub && <p className="truncate text-small text-muted-foreground">{sub}</p>}
      {children}
    </header>
  )
}

export type TabItem<T extends string> = {
  value: T
  label: React.ReactNode
  count?: number
  title?: string
  testId?: string
}

export function UnderlineTabs<T extends string>({
  value,
  onChange,
  items,
  label,
  className,
}: {
  value: T | null
  onChange: (value: T) => void
  items: readonly TabItem<T>[]
  label: string
  className?: string
}) {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <div
      role="toolbar"
      aria-label={label}
      className={cn("-mx-1 mt-2.5 flex gap-3.5 overflow-x-auto px-1 [scrollbar-width:none]", className)}
    >
      {items.map((it) => {
        const on = it.value === value
        return (
          <button
            key={it.value}
            type="button"
            aria-pressed={on}
            title={it.title}
            onClick={() => onChange(it.value)}
            className={cn(
              "flex h-9 shrink-0 items-center gap-1.5 text-body whitespace-nowrap outline-none transition-colors focus-visible:text-foreground focus-visible:underline",
              on
                ? "font-medium text-foreground shadow-[inset_0_-2px_0_var(--foreground)]"
                : "text-muted-foreground hover:text-foreground"
            )}
            data-testid={it.testId}
          >
            {it.label}
            {!!it.count && (
              <span className={cn("text-small tabular-nums", on ? "text-muted-foreground" : "text-faint")}>
                {fmt.format(it.count)}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

export function FilterChip({
  active,
  onClick,
  children,
  title,
  testId,
}: {
  active: boolean
  onClick: () => void
  children: React.ReactNode
  title?: string
  testId?: string
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      title={title}
      onClick={onClick}
      className={cn(
        "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border px-3 text-small transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active
          ? "border-foreground bg-foreground text-background"
          : "border-border text-muted-foreground hover:border-faint hover:text-foreground"
      )}
      data-testid={testId}
    >
      {children}
    </button>
  )
}

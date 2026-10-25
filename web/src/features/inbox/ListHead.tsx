import { useLingui } from "@lingui/react/macro"
import { useCallback, useEffect, useRef, useState } from "react"

import { OverflowTabs } from "@/components/common/OverflowTabs"
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

const columnTabClass =
  "flex h-9 shrink-0 items-center gap-1.5 text-body whitespace-nowrap text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:underline data-popup-open:text-foreground aria-[current=page]:font-medium aria-[current=page]:text-foreground aria-[current=page]:shadow-[inset_0_-2px_0_var(--foreground)]"

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
  return (
    <OverflowTabs
      label={label}
      current={value}
      tabClass={columnTabClass}
      gap="gap-3.5"
      className={cn("mt-2.5", className)}
      items={items.map((it) => ({
        key: it.value,
        label: it.label,
        count: it.count || undefined,
        title: it.title,
        testId: it.testId,
        onSelect: () => onChange(it.value),
      }))}
    />
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

export function ViewChips<T extends string>({
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
  const row = useRef<HTMLDivElement>(null)
  const [fade, setFade] = useState({ left: false, right: false })
  const measure = useCallback(() => {
    const el = row.current
    if (!el) return
    const away = Math.abs(el.scrollLeft)
    const more = el.scrollWidth - el.clientWidth - away > 1
    const rtl = getComputedStyle(el).direction === "rtl"
    const next = rtl ? { left: more, right: away > 1 } : { left: away > 1, right: more }
    setFade((f) => (f.left === next.left && f.right === next.right ? f : next))
  }, [])
  useEffect(() => {
    const el = row.current
    if (!el) return
    el.querySelector<HTMLElement>("[aria-pressed=true]")?.scrollIntoView({ inline: "nearest", block: "nearest" })
    measure()
    const watch = new ResizeObserver(measure)
    watch.observe(el)
    return () => watch.disconnect()
  }, [value, items.length, measure])
  const edge = "1.5rem"
  const mask =
    fade.left || fade.right
      ? `linear-gradient(to right, ${fade.left ? "transparent" : "#000"}, #000 ${edge}, #000 calc(100% - ${edge}), ${fade.right ? "transparent" : "#000"})`
      : undefined
  return (
    <div
      ref={row}
      role="toolbar"
      aria-label={label}
      onScroll={measure}
      style={{ maskImage: mask, WebkitMaskImage: mask }}
      className={cn(
        "flex gap-1.5 overflow-x-auto px-3 pb-2.5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
        className
      )}
      data-testid="page-tabs"
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
              "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border px-2.5 text-small whitespace-nowrap outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              on
                ? "border-foreground bg-foreground text-background"
                : "bg-card text-muted-foreground hover:text-foreground"
            )}
            data-testid={it.testId}
          >
            {it.label}
            {!!it.count && <span className="font-mono tabular-nums">{fmt.format(it.count)}</span>}
          </button>
        )
      })}
    </div>
  )
}

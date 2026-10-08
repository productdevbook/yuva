import { cn } from "@/lib/utils"

export function Segmented<T extends string>({
  value,
  onChange,
  items,
  label,
  className,
}: {
  value: T
  onChange: (value: T) => void
  items: readonly { value: T; label: React.ReactNode }[]
  label: string
  className?: string
}) {
  const move = (e: React.KeyboardEvent, i: number) => {
    const step = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = items[(i + step + items.length) % items.length]
    onChange(next.value)
    const buttons = e.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>("button")
    buttons?.[(i + step + items.length) % items.length]?.focus()
  }
  return (
    <div role="tablist" aria-label={label} className={cn("flex items-center gap-1 overflow-x-auto", className)}>
      {items.map((it, i) => {
        const on = it.value === value
        return (
          <button
            key={it.value}
            type="button"
            role="tab"
            aria-selected={on}
            tabIndex={on ? 0 : -1}
            onClick={() => onChange(it.value)}
            onKeyDown={(e) => move(e, i)}
            className={cn(
              "h-7 shrink-0 rounded-full px-2.5 text-[0.8rem] whitespace-nowrap transition-colors",
              on ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:text-foreground",
            )}
          >
            {it.label}
          </button>
        )
      })}
    </div>
  )
}

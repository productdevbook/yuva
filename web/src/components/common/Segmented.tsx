import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

const item = "h-8 min-w-0 px-3 text-body font-medium text-muted-foreground hover:bg-transparent hover:text-foreground aria-pressed:text-foreground"

const looks = {
  soft: { group: "gap-1", item: cn(item, "rounded-full aria-pressed:bg-muted") },
  segment: {
    group: "gap-0.5 rounded-[10px] border bg-card p-[3px]",
    item: cn(item, "h-7 rounded-[7px] aria-pressed:bg-background aria-pressed:shadow-[0_1px_2px_rgb(0_0_0/0.06)]"),
  },
  chip: {
    group: "flex-wrap gap-1.5",
    item: cn(item, "rounded-full border bg-card hover:bg-card aria-pressed:border-foreground aria-pressed:bg-foreground aria-pressed:text-background aria-pressed:hover:bg-foreground"),
  },
}

export function Segmented<T extends string>({
  value,
  onChange,
  items,
  label,
  look = "soft",
  className,
  itemClassName,
}: {
  value: T
  onChange: (value: T) => void
  items: readonly { value: T; label: React.ReactNode; testId?: string; title?: string }[]
  label: string
  look?: keyof typeof looks
  className?: string
  itemClassName?: string
}) {
  const l = looks[look]
  return (
    <ToggleGroup
      value={[value]}
      onValueChange={(v) => {
        if (v[0]) onChange(v[0] as T)
      }}
      aria-label={label}
      className={cn(l.group, className)}
    >
      {items.map((it) => (
        <ToggleGroupItem key={it.value} value={it.value} className={cn(l.item, itemClassName)} title={it.title} data-testid={it.testId}>
          {it.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

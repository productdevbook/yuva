import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

const looks = {
  soft: {
    group: "gap-1",
    item: "h-7 min-w-0 rounded-full px-2.5 text-[0.8rem] font-normal text-muted-foreground hover:bg-transparent hover:text-foreground aria-pressed:bg-muted aria-pressed:font-medium aria-pressed:text-foreground",
  },
  segment: {
    group: "gap-0.5 rounded-[10px] border bg-card p-[3px]",
    item: "h-auto min-w-0 rounded-[7px] px-3 py-1.5 text-[13px] font-normal text-muted-foreground hover:bg-transparent hover:text-foreground aria-pressed:bg-background aria-pressed:font-medium aria-pressed:text-foreground aria-pressed:shadow-[0_1px_2px_rgb(0_0_0/0.06)]",
  },
  chip: {
    group: "flex-wrap gap-1.5",
    item: "h-auto min-w-0 rounded-full border bg-card px-3 py-1.5 text-[13px] font-normal text-muted-foreground hover:bg-card hover:text-foreground aria-pressed:border-foreground aria-pressed:bg-foreground aria-pressed:text-background aria-pressed:hover:bg-foreground",
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

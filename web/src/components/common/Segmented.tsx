import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"

const segmentTrack = "gap-0.5 rounded-[10px] bg-muted p-[3px]"
const segmentItem =
  "h-7 min-w-0 rounded-[7px] border border-transparent px-3 text-body font-medium text-muted-foreground hover:bg-transparent hover:text-foreground"
const pressed =
  "aria-pressed:border-border aria-pressed:bg-card aria-pressed:text-foreground aria-pressed:shadow-[0_1px_2px_rgb(0_0_0/0.08)] dark:aria-pressed:border-transparent dark:aria-pressed:bg-input"

export function Segmented<T extends string>({
  value,
  onChange,
  items,
  label,
  className,
  itemClassName,
}: {
  value: T
  onChange: (value: T) => void
  items: readonly { value: T; label: React.ReactNode; testId?: string; title?: string }[]
  label: string
  className?: string
  itemClassName?: string
}) {
  return (
    <ToggleGroup
      value={[value]}
      onValueChange={(v) => {
        if (v[0]) onChange(v[0] as T)
      }}
      aria-label={label}
      className={cn(segmentTrack, className)}
    >
      {items.map((it) => (
        <ToggleGroupItem
          key={it.value}
          value={it.value}
          className={cn(segmentItem, pressed, itemClassName)}
          title={it.title}
          onFocus={(e) => e.currentTarget.scrollIntoView({ block: "nearest", inline: "nearest" })}
          data-testid={it.testId}
        >
          {it.label}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

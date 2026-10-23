import { cn } from "@/lib/utils"

function toggled<T>(list: T[], v: T) {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v]
}

export function ChipMulti<T extends string>({
  value,
  onChange,
  options,
  label,
}: {
  value: T[]
  onChange: (next: T[]) => void
  options: readonly (readonly [T, string])[]
  label: string
}) {
  return (
    <div className="flex flex-wrap gap-1.5" role="group" aria-label={label}>
      {options.map(([v, text]) => {
        const on = value.includes(v)
        return (
          <button
            key={v}
            type="button"
            aria-pressed={on}
            onClick={() => onChange(toggled(value, v))}
            className={cn(
              "inline-flex h-[30px] max-w-full items-center rounded-lg border px-2.5 text-body outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring",
              on
                ? "border-foreground bg-surface font-medium text-foreground"
                : "border-input bg-background text-muted-foreground hover:border-faint hover:text-foreground"
            )}
          >
            <span className="truncate">{text}</span>
          </button>
        )
      })}
    </div>
  )
}

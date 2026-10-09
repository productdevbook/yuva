import { cn } from "@/lib/utils"

export function Dot({ color, className }: { color?: string | null; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn("inline-block size-2 shrink-0 rounded-full bg-faint", className)}
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}

export function LabelChip({ name, color, className }: { name: string; color: string; className?: string }) {
  return (
    <span className={cn("inline-flex max-w-40 items-center gap-1.5 text-caption text-muted-foreground", className)}>
      <Dot color={color} className="size-1.5" />
      <span className="truncate">{name}</span>
    </span>
  )
}

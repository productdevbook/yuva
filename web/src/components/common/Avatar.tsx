import { initials } from "@/components/common/text"
import { cn } from "@/lib/utils"

export function PersonAvatar({ name, className }: { name: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-medium text-muted-foreground",
        className,
      )}
    >
      {initials(name)}
    </span>
  )
}

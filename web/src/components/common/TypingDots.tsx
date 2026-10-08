import { cn } from "@/lib/utils"

export function TypingDots({ className }: { className?: string }) {
  return (
    <span aria-hidden className={cn("inline-flex items-center gap-0.5", className)}>
      {[0, 200, 400].map((d) => (
        <span key={d} className="size-1 animate-pulse rounded-full bg-current" style={{ animationDelay: `${d}ms` }} />
      ))}
    </span>
  )
}

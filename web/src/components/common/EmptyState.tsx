import { cn } from "@/lib/utils"

export function EmptyState({
  icon: Icon,
  title,
  children,
  className,
}: {
  icon?: React.ComponentType<{ className?: string }>
  title: React.ReactNode
  children?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center", className)}>
      {Icon && (
        <span className="mb-2 grid size-11 place-items-center rounded-full border text-faint">
          <Icon className="size-5" />
        </span>
      )}
      <p className="text-sm font-medium">{title}</p>
      {children && <div className="max-w-xs text-sm leading-relaxed text-muted-foreground">{children}</div>}
    </div>
  )
}

import { cn } from "@/lib/utils"

const tones = {
  info: "bg-surface text-muted-foreground",
  warning: "bg-warning/8 text-foreground",
  danger: "bg-destructive/6 text-foreground",
}

export function Notice({
  tone = "info",
  icon: Icon,
  children,
  className,
  ...props
}: {
  tone?: keyof typeof tones
  icon?: React.ComponentType<{ className?: string }>
  children: React.ReactNode
  className?: string
} & Omit<React.ComponentProps<"div">, "children">) {
  return (
    <div className={cn("flex gap-2.5 rounded-xl px-3.5 py-3 text-sm leading-relaxed", tones[tone], className)} {...props}>
      {Icon && (
        <Icon
          className={cn(
            "mt-0.5 size-4 shrink-0",
            tone === "danger" ? "text-destructive" : tone === "warning" ? "text-warning" : "text-faint",
          )}
        />
      )}
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">{children}</div>
    </div>
  )
}

import { Kbd as UiKbd } from "@/components/ui/kbd"
import { cn } from "@/lib/utils"

export function Kbd({ children, className }: { children: React.ReactNode; className?: string }) {
  return <UiKbd className={cn("rounded-md border bg-surface text-caption", className)}>{children}</UiKbd>
}

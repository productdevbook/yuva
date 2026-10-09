import { useErrorText } from "@/components/common/text"
import { cn } from "@/lib/utils"

export function ErrorLine({ error, className }: { error: unknown; className?: string }) {
  const text = useErrorText()
  if (!error) return null
  return (
    <p role="alert" className={cn("text-body text-destructive", className)}>
      {text(error)}
    </p>
  )
}

import { Input as InputPrimitive } from "@base-ui/react/input"
import { cn } from "@/lib/utils"

export const fieldClass =
  "w-full min-w-0 rounded-xl border border-input bg-background px-3 text-base transition-colors outline-none placeholder:text-faint hover:border-foreground/25 focus-visible:border-ring focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/15 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive md:text-body"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return <InputPrimitive type={type} data-slot="input" className={cn(fieldClass, "h-9", className)} {...props} />
}

export { Input }

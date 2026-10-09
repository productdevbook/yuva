import { cn } from "@/lib/utils"

import { fieldClass } from "@/components/ui/input"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return <textarea data-slot="textarea" className={cn(fieldClass, "field-sizing-content min-h-16 py-2", className)} {...props} />
}

export { Textarea }

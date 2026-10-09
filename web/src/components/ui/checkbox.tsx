import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox"
import { cn } from "@/lib/utils"
import { CheckIcon, MinusIcon } from "lucide-react"

function Checkbox({ className, ...props }: CheckboxPrimitive.Root.Props) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border border-input bg-background transition-colors outline-none after:absolute after:-inset-2 hover:border-foreground/40 disabled:cursor-not-allowed disabled:opacity-60 aria-invalid:border-destructive data-checked:border-primary data-checked:bg-primary data-checked:text-primary-foreground data-indeterminate:border-primary data-indeterminate:bg-primary data-indeterminate:text-primary-foreground",
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        className="grid place-content-center [&>svg]:size-3"
        render={(p, state) => <span {...p}>{state.indeterminate ? <MinusIcon /> : <CheckIcon strokeWidth={3} />}</span>}
      />
    </CheckboxPrimitive.Root>
  )
}

export { Checkbox }

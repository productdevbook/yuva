import { Checkbox as CheckboxPrimitive } from "@base-ui/react/checkbox"
import { cn } from "cn"
import { CheckIcon, MinusIcon } from "lucide-react"

function Checkbox({ className, ...props }: CheckboxPrimitive.Root.Props) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        "relative flex size-4 shrink-0 items-center justify-center rounded-[5px] border border-input bg-background transition-colors outline-none after:absolute after:-inset-2 hover:border-foreground/40 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive data-checked:border-foreground data-checked:bg-foreground data-checked:text-background data-indeterminate:border-foreground data-indeterminate:bg-foreground data-indeterminate:text-background",
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

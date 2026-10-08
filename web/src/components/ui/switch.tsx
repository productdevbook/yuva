import { Switch as SwitchPrimitive } from "@base-ui/react/switch"
import { cn } from "cn"

function Switch({ className, ...props }: SwitchPrimitive.Root.Props) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        "relative inline-flex h-5 w-8 shrink-0 items-center rounded-full p-0.5 transition-colors outline-none after:absolute after:-inset-2 data-checked:bg-primary data-unchecked:bg-input data-disabled:cursor-not-allowed data-disabled:opacity-50",
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="pointer-events-none block size-4 rounded-full bg-white shadow-sm data-checked:translate-x-3 rtl:data-checked:-translate-x-3" />
    </SwitchPrimitive.Root>
  )
}

export { Switch }

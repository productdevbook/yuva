import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "@/lib/utils"

const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center rounded-full border border-transparent text-body font-medium whitespace-nowrap transition-colors outline-none select-none focus-visible:ring-3 focus-visible:ring-ring/30 disabled:pointer-events-none disabled:opacity-60 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90 disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100",
        outline: "border-border bg-card text-foreground hover:border-faint aria-expanded:border-faint",
        secondary: "bg-muted text-foreground hover:bg-muted/80 aria-expanded:bg-muted/80",
        ghost: "text-muted-foreground hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground",
        destructive: "bg-destructive/10 text-destructive hover:bg-destructive/15",
        link: "h-auto! rounded-none px-0! text-brand underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 gap-1.5 px-4",
        sm: "h-8 gap-1.5 px-3",
        lg: "h-10 gap-2 px-5",
        icon: "size-9",
        "icon-sm": "size-8",
        "icon-xs": "size-6 [&_svg]:size-3.5",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      nativeButton={!props.render}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }

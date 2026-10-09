import { Dialog as SheetPrimitive } from "@base-ui/react/dialog"
import { Trans } from "@lingui/react/macro"
import { cn } from "@/lib/utils"
import { XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { overlayClass } from "@/components/ui/dialog"

const Sheet = SheetPrimitive.Root

function SheetContent({
  className,
  children,
  side = "right",
  showCloseButton = true,
  ...props
}: SheetPrimitive.Popup.Props & { side?: "left" | "right"; showCloseButton?: boolean }) {
  return (
    <SheetPrimitive.Portal>
      <SheetPrimitive.Backdrop className={overlayClass} />
      <SheetPrimitive.Popup
        data-side={side}
        className={cn(
          "fixed inset-y-0 z-50 flex w-[min(22rem,88vw)] flex-col bg-card text-body text-foreground shadow-[0_0_60px_-20px_rgb(15_23_42/0.4)] outline-none",
          side === "right" ? "right-0 border-l" : "left-0 border-r",
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <SheetPrimitive.Close render={<Button variant="ghost" size="icon-sm" className="absolute top-3 right-3" />}>
            <XIcon />
            <span className="sr-only">
              <Trans>Close</Trans>
            </span>
          </SheetPrimitive.Close>
        )}
      </SheetPrimitive.Popup>
    </SheetPrimitive.Portal>
  )
}

function SheetHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("flex flex-col gap-1 border-b px-5 py-4 pe-14", className)} {...props} />
}

function SheetTitle({ className, ...props }: SheetPrimitive.Title.Props) {
  return <SheetPrimitive.Title className={cn("", className)} {...props} />
}

function SheetDescription({ className, ...props }: SheetPrimitive.Description.Props) {
  return <SheetPrimitive.Description className={cn("text-body text-muted-foreground", className)} {...props} />
}

export { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle }

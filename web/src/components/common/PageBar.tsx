import { ArrowLeftIcon } from "lucide-react"
import { Link } from "react-router"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export function PageBar({
  back,
  label,
  children,
  className,
}: {
  back: string
  label: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <header
      className={cn(
        "sticky top-0 z-10 flex shrink-0 flex-wrap items-center gap-x-3 gap-y-2 border-b bg-background px-6 py-3 in-data-frame:static phone:px-4",
        className
      )}
      data-testid="page-bar"
    >
      <Button
        variant="outline"
        size="icon-sm"
        className="rounded-lg border-input"
        render={<Link to={back} />}
        aria-label={label}
        title={label}
        data-testid="page-bar-back"
      >
        <ArrowLeftIcon className="rtl:rotate-180" />
      </Button>
      {children}
    </header>
  )
}

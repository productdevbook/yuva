import { type To } from "react-router"

import { PaneBack } from "@/components/common/Column"
import { OverflowTabs } from "@/components/common/OverflowTabs"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export type FrameTab = {
  key: string
  to: To
  label: React.ReactNode
  count?: number
  testId?: string
}

export function FrameTabs({
  items,
  current,
  label,
  moreTestId,
}: {
  items: FrameTab[]
  current: string
  label: string
  moreTestId?: string
}) {
  return <OverflowTabs label={label} current={current} items={items} moreTestId={moreTestId} />
}

export function FrameButton({ ink, className, ...props }: React.ComponentProps<typeof Button> & { ink?: boolean }) {
  return (
    <Button
      variant={ink ? "default" : "outline"}
      className={cn(
        "h-[34px] gap-1.5 rounded-lg px-3.5",
        ink ? "bg-foreground text-background hover:bg-foreground/85" : "border-input bg-background",
        className
      )}
      {...props}
    />
  )
}

export function PageFrame({
  title,
  description,
  actions,
  tabs,
  back,
  notice,
  testId,
  bodyClassName,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  actions?: React.ReactNode
  tabs?: React.ReactNode
  back?: { to: string; label: string }
  notice?: React.ReactNode
  testId?: string
  bodyClassName?: string
  children: React.ReactNode
}) {
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid={testId}>
      <header className="flex shrink-0 flex-col gap-3 border-b px-6 pt-5 phone:px-4 phone:pt-3">
        {back && (
          <div className="-mb-3">
            <PaneBack to={back.to} label={back.label} />
          </div>
        )}
        <div className={cn("flex flex-wrap items-center gap-x-3 gap-y-2", !tabs && "pb-4")}>
          <div className="flex min-w-0 flex-1 basis-64 flex-col leading-[1.3]">
            <h1 className="text-[22px] leading-[1.3] font-semibold tracking-[-0.02em] break-words">{title}</h1>
            {description && <div className="text-body text-muted-foreground">{description}</div>}
          </div>
          {actions && <div className="flex shrink-0 flex-wrap items-center gap-3 phone:gap-2">{actions}</div>}
        </div>
        {notice}
        {tabs}
      </header>
      <div className={cn("min-h-0 flex-1 in-data-frame:overflow-y-auto", bodyClassName)}>{children}</div>
    </div>
  )
}

export const framePad = "flex flex-col gap-4 p-6 pb-16 phone:px-4 phone:pt-4"

export function ProductTag({ name, color, className }: { name: string; color?: string; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center rounded-[4px] border px-1.5 text-[11px] leading-4 font-medium whitespace-nowrap text-muted-foreground",
        className
      )}
      style={color ? { color, borderColor: `${color}40` } : undefined}
    >
      {name}
    </span>
  )
}

export type PillTone = "success" | "warning" | "danger" | "info" | "muted" | "brand"

const pillTone: Record<PillTone, string> = {
  success: "border-success/35 text-success",
  warning: "border-warning/40 text-warning",
  danger: "border-destructive/35 text-destructive",
  info: "border-blue-600/35 text-blue-600 dark:border-blue-400/35 dark:text-blue-400",
  muted: "border-faint/40 text-faint",
  brand: "border-brand/35 text-brand",
}

export function Pill({ tone, className, children, ...props }: React.ComponentProps<"span"> & { tone: PillTone }) {
  return (
    <span
      className={cn(
        "inline-flex w-fit shrink-0 items-center rounded-full border px-2 py-px text-caption font-medium whitespace-nowrap",
        pillTone[tone],
        className
      )}
      {...props}
    >
      {children}
    </span>
  )
}

export function Chip({ className, children }: { className?: string; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "inline-flex w-fit shrink-0 items-center rounded-[4px] border bg-surface px-1.5 text-[11px] leading-4 whitespace-nowrap text-muted-foreground",
        className
      )}
    >
      {children}
    </span>
  )
}

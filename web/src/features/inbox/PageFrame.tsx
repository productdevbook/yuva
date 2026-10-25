import { useLingui } from "@lingui/react/macro"
import { type To } from "react-router"

import { OverflowTabs } from "@/components/common/OverflowTabs"
import { formatDateTime, formatDuration, formatShort } from "@/components/common/text"
import { useNow } from "@/hooks/use-now"
import { cn } from "@/lib/utils"

export const look = {
  btn: "h-[34px] rounded-lg border-input px-3.5 font-normal",
  btnInk: "h-[34px] rounded-lg bg-foreground px-3.5 text-background hover:bg-foreground/90",
  btnSm: "h-[30px] rounded-lg border-input px-2.5 font-normal",
  btnInkSm: "h-[30px] rounded-lg bg-foreground px-3 text-background hover:bg-foreground/90",
  btnPSm: "h-[30px] rounded-lg px-3",
  chip: "inline-flex h-[30px] shrink-0 items-center gap-1.5 rounded-lg border border-input bg-background px-2.5 text-body text-foreground outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring data-popup-open:border-faint",
  chipDash:
    "inline-flex h-[30px] shrink-0 items-center gap-1 rounded-lg border border-dashed border-input bg-transparent px-2.5 text-body text-muted-foreground outline-none transition-colors hover:border-faint hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-popup-open:border-faint",
  chipOn:
    "inline-flex h-[30px] shrink-0 items-center gap-1.5 rounded-lg border border-brand/40 bg-brand-wash px-2.5 text-body font-medium text-brand outline-none focus-visible:ring-2 focus-visible:ring-ring",
  listChip:
    "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border bg-card px-2.5 text-small text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-muted data-popup-open:text-foreground",
  listChipOn:
    "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-foreground bg-foreground px-2.5 text-small text-background outline-none focus-visible:ring-2 focus-visible:ring-ring",
  over: "text-[11px] leading-4 font-medium tracking-[0.06em] text-faint uppercase",
  card: "overflow-hidden rounded-xl border bg-background",
  th: "text-caption font-medium text-faint",
  faint: "text-caption text-faint",
  pill: "inline-flex w-fit shrink-0 items-center rounded-full border px-2 py-px text-caption font-medium whitespace-nowrap",
}

export type FrameTab<T extends string> = {
  key: T
  label: React.ReactNode
  count?: number
  to?: To
  title?: string
  testId?: string
}

export function FrameTabs<T extends string>({
  items,
  value,
  onChange,
  label,
  moreTestId,
}: {
  items: readonly FrameTab<T>[]
  value: T | null
  onChange?: (key: T) => void
  label: string
  moreTestId?: string
}) {
  return (
    <OverflowTabs
      label={label}
      current={value}
      moreTestId={moreTestId}
      items={items.map((tab) => ({ ...tab, onSelect: onChange && (() => onChange(tab.key)) }))}
    />
  )
}

export function PageTop({
  title,
  sub,
  actions,
  tabs,
  aside,
  testId,
  className,
}: {
  title: React.ReactNode
  sub?: React.ReactNode
  actions?: React.ReactNode
  tabs?: React.ReactNode
  aside?: React.ReactNode
  testId?: string
  className?: string
}) {
  return (
    <header
      className={cn(
        "flex shrink-0 flex-col gap-3 border-b px-6 pt-5 phone:px-4 phone:pt-4",
        !tabs && "pb-4",
        className
      )}
      data-testid={testId}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-1 basis-56 flex-col leading-[1.3]">
          <h1 className="truncate text-[22px] leading-[1.3] font-semibold tracking-[-0.02em]">{title}</h1>
          {sub && <p className="truncate text-body text-muted-foreground">{sub}</p>}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-3">{actions}</div>}
      </div>
      {tabs && (
        <div className="flex min-w-0 items-end gap-4 phone:flex-col-reverse phone:items-stretch phone:gap-1">
          <div className="min-w-0 flex-1">{tabs}</div>
          {aside && <div className="flex shrink-0 items-center gap-2 pb-1.5 phone:pb-0">{aside}</div>}
        </div>
      )}
    </header>
  )
}

export function Overline({ children, className }: { children: React.ReactNode; className?: string }) {
  return <h3 className={cn(look.over, className)}>{children}</h3>
}

export function PageBody({
  children,
  className,
  testId,
}: {
  children: React.ReactNode
  className?: string
  testId?: string
}) {
  return (
    <div className={cn("min-h-0 flex-1 in-data-frame:overflow-y-auto", className)} data-testid={testId}>
      {children}
    </div>
  )
}

export function LabelTag({ name }: { name: string }) {
  return (
    <span className="max-w-24 shrink-0 truncate rounded-[4px] border bg-surface px-1.5 text-[11px] leading-4 text-muted-foreground">
      {name}
    </span>
  )
}

export function Ago({ iso, className }: { iso: string; className?: string }) {
  const { i18n } = useLingui()
  const now = useNow(60_000)
  const sec = (now - Date.parse(iso)) / 1000
  return (
    <time className={className} dateTime={iso} title={formatDateTime(iso, i18n.locale)}>
      {sec < 7 * 86400 ? formatDuration(Math.max(60, sec), i18n.locale) : formatShort(iso, i18n.locale)}
    </time>
  )
}

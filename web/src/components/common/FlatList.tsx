import { useLingui } from "@lingui/react/macro"

import { cn } from "@/lib/utils"

export function useDayLabel() {
  const { t, i18n } = useLingui()
  return (iso: string) => {
    const d = new Date(iso)
    const today = new Date()
    const yesterday = new Date()
    yesterday.setDate(today.getDate() - 1)
    if (d.toDateString() === today.toDateString()) return t`Today`
    if (d.toDateString() === yesterday.toDateString()) return t`Yesterday`
    const sameYear = d.getFullYear() === today.getFullYear()
    return new Intl.DateTimeFormat(
      i18n.locale,
      sameYear ? { weekday: "long", day: "numeric", month: "long" } : { dateStyle: "long" }
    ).format(d)
  }
}

export function groupRuns<T>(items: T[], keyOf: (item: T) => string) {
  const out: { key: string; items: T[] }[] = []
  for (const item of items) {
    const key = keyOf(item)
    const last = out.at(-1)
    if (last && last.key === key) last.items.push(item)
    else out.push({ key, items: [item] })
  }
  return out
}

export function dayOf(iso: string) {
  return new Date(iso).toDateString()
}

type ButtonProps = Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "className" | "children" | "type"> &
  Record<`data-${string}`, string | boolean | undefined>

export function FlatRow({
  color,
  who,
  children,
  end,
  lead,
  after,
  current,
  cursor,
  checked,
  className,
  button,
  ...rest
}: {
  color?: string | null
  who: React.ReactNode
  children?: React.ReactNode
  end?: React.ReactNode
  lead?: React.ReactNode
  after?: React.ReactNode
  current?: boolean
  cursor?: boolean
  checked?: boolean
  className?: string
  button?: ButtonProps
} & Omit<React.HTMLAttributes<HTMLDivElement>, "children" | "className"> &
  Record<`data-${string}`, string | boolean | undefined>) {
  return (
    <div
      className={cn(
        "relative flex min-w-0 scroll-mt-3 scroll-mb-6 items-center border-t border-line-soft transition-colors hover:bg-subtle",
        (current || cursor) && "bg-subtle shadow-[inset_2px_0_0_var(--brand)]",
        checked && "bg-brand-wash",
        className
      )}
      {...rest}
    >
      <button
        type="button"
        className={cn(
          "grid min-w-0 flex-1 items-center gap-x-3.5 px-1 py-[13px] text-start text-body leading-5 text-fg2 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
          lead
            ? "grid-cols-[auto_7px_minmax(0,170px)_minmax(0,1fr)_auto]"
            : "grid-cols-[7px_minmax(0,170px)_minmax(0,1fr)_auto]"
        )}
        {...button}
      >
        {lead}
        <span
          aria-hidden
          className="size-[7px] rounded-full bg-muted2"
          style={color ? { backgroundColor: color } : undefined}
        />
        <span className="flex min-w-0 items-center gap-1.5">{who}</span>
        <span className="min-w-0 truncate">{children}</span>
        <span className="flex min-w-0 items-center justify-end gap-2">{end}</span>
      </button>
      {after}
    </div>
  )
}

export function FlatWho({ strong, children }: { strong?: boolean; children: React.ReactNode }) {
  return <span className={cn("min-w-0 truncate", strong && "font-semibold text-foreground")}>{children}</span>
}

export function FlatTime({ iso, children, className }: { iso: string; children: React.ReactNode; className?: string }) {
  return (
    <time className={cn("font-mono text-xs whitespace-nowrap text-muted2 tabular-nums", className)} dateTime={iso}>
      {children}
    </time>
  )
}

export function FlatTag({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "max-w-28 shrink-0 truncate rounded-[4px] border px-1.5 text-[11px] leading-[18px] text-muted-foreground",
        className
      )}
    >
      {children}
    </span>
  )
}

export function UnreadBadge({
  label,
  children,
  testId,
}: {
  label: string
  children?: React.ReactNode
  testId?: string
}) {
  if (children === undefined)
    return <span className="size-2 shrink-0 rounded-full bg-brand" role="img" aria-label={label} data-testid={testId} />
  return (
    <span
      className="h-[18px] min-w-[18px] shrink-0 rounded-full bg-brand px-[5px] text-center text-[11px] leading-[18px] font-semibold text-white"
      aria-label={label}
      data-testid={testId}
    >
      {children}
    </span>
  )
}

export function FlatEmpty({ children, testId }: { children: React.ReactNode; testId?: string }) {
  return (
    <div
      className="mt-7 flex flex-col items-start gap-3 border-t border-line-soft pt-4 text-body text-muted-foreground"
      data-testid={testId}
    >
      {children}
    </div>
  )
}

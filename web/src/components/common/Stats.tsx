import { Link } from "react-router"

import { cn } from "@/lib/utils"

export function Stat({
  value,
  label,
  hint,
  to,
  quiet,
  tone,
  live,
  title,
  testId,
}: {
  value: React.ReactNode
  label: React.ReactNode
  hint?: React.ReactNode
  to?: string
  quiet?: boolean
  tone?: "brand"
  live?: boolean
  title?: string
  testId?: string
}) {
  const body = (
    <>
      <b
        className={cn(
          "text-[26px] leading-8 font-semibold tracking-[-0.02em] whitespace-nowrap tabular-nums",
          quiet && "text-muted2",
          tone === "brand" && "text-brand"
        )}
      >
        {value}
      </b>
      <span className="flex min-w-0 items-center gap-1.5 text-caption text-muted-foreground">
        {live && <span aria-hidden className="size-2 shrink-0 rounded-full bg-ok" />}
        <span className="line-clamp-2 min-w-0 [overflow-wrap:anywhere]">{label}</span>
      </span>
      {hint && <span className="line-clamp-2 font-mono text-[11px] text-muted2">{hint}</span>}
    </>
  )
  const className = "flex min-w-0 flex-[1_1_150px] flex-col gap-1 border-e border-b border-line-soft px-[18px] py-4"
  if (to)
    return (
      <Link
        to={to}
        title={title}
        className={cn(className, "outline-none transition-colors hover:bg-subtle focus-visible:bg-subtle")}
        data-testid={testId}
      >
        {body}
      </Link>
    )
  return (
    <div className={className} title={title} data-testid={testId}>
      {body}
    </div>
  )
}

export function Stats({ children, label, testId }: { children: React.ReactNode; label?: string; testId?: string }) {
  return (
    <div
      role={label ? "group" : undefined}
      aria-label={label}
      className="-me-px -mb-px flex flex-wrap"
      data-testid={testId}
    >
      {children}
    </div>
  )
}

export function StatCard({
  children,
  label,
  testId,
  className,
}: {
  children: React.ReactNode
  label?: string
  testId?: string
  className?: string
}) {
  return (
    <div className={cn("overflow-hidden rounded-[10px] border bg-background", className)}>
      <Stats label={label} testId={testId}>
        {children}
      </Stats>
    </div>
  )
}

export const chartDots =
  "bg-subtle bg-[radial-gradient(var(--input)_1px,transparent_1px)] bg-size-[18px_18px] [&_.recharts-cartesian-axis-line]:stroke-input"

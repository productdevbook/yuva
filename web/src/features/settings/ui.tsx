import { Trans } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronRightIcon } from "lucide-react"
import { Link, NavLink } from "react-router"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

export function PageHeader({
  title,
  description,
  back,
  action,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  back?: { to: string; label: string }
  action?: React.ReactNode
  children?: React.ReactNode
}) {
  return (
    <header className="flex flex-col gap-1.5">
      {back && (
        <Link to={back.to} className="mb-2 inline-flex w-fit items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground">
          <ArrowLeftIcon className="size-4 rtl:rotate-180" />
          {back.label}
        </Link>
      )}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="min-w-0 text-2xl font-semibold tracking-[-0.02em] break-words">{title}</h1>
        {action}
      </div>
      {description && <p className="max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>}
      {children}
    </header>
  )
}

export function Section({
  title,
  description,
  action,
  children,
  className,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  action?: React.ReactNode
  children?: React.ReactNode
  className?: string
}) {
  return (
    <section className={cn("flex flex-col gap-4", className)}>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="text-base font-semibold tracking-tight">{title}</h2>
          {description && <p className="max-w-2xl text-sm leading-relaxed text-muted-foreground">{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

type CardProps = {
  children: React.ReactNode
  footer?: React.ReactNode
  flush?: boolean
  tone?: "danger"
  className?: string
}

const cardClass = (p: CardProps) =>
  cn("flex flex-col overflow-hidden rounded-2xl border bg-card", p.tone === "danger" && "border-destructive/30", p.className)

function CardBody(p: CardProps) {
  return (
    <>
      <div className={cn(!p.flush && "flex flex-col gap-5 p-5 sm:p-6")}>{p.children}</div>
      {p.footer && <div className="flex flex-wrap items-center gap-3 border-t bg-surface px-5 py-3 sm:px-6">{p.footer}</div>}
    </>
  )
}

export function Card({ children, footer, flush, tone, className, ...rest }: CardProps & Omit<React.ComponentProps<"div">, "children">) {
  const p = { children, footer, flush, tone, className }
  return (
    <div className={cardClass(p)} {...rest}>
      <CardBody {...p} />
    </div>
  )
}

export function FormCard({ onSubmit, ...p }: CardProps & { onSubmit: () => void }) {
  return (
    <form
      className={cardClass(p)}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
    >
      <CardBody {...p} />
    </form>
  )
}

export function FormActions({
  pending,
  saved,
  error,
  disabled,
  label,
}: {
  pending: boolean
  saved?: boolean
  error?: unknown
  disabled?: boolean
  label?: React.ReactNode
}) {
  return (
    <>
      <ErrorLine error={error} className="min-w-0 flex-1" />
      {saved && !error && (
        <span className="flex-1 text-sm text-muted-foreground" role="status">
          <Trans>Saved</Trans>
        </span>
      )}
      <Button type="submit" size="sm" className="ms-auto" disabled={pending || disabled}>
        {label ?? <Trans>Save</Trans>}
      </Button>
    </>
  )
}

export function Field({
  label,
  htmlFor,
  hint,
  error,
  children,
  className,
}: {
  label: React.ReactNode
  htmlFor?: string
  hint?: React.ReactNode
  error?: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("flex min-w-0 flex-col gap-2", className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : (
        hint && <p className="text-xs leading-relaxed text-muted-foreground">{hint}</p>
      )}
    </div>
  )
}

export function ToggleRow({
  title,
  hint,
  children,
}: {
  title: React.ReactNode
  hint?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <label className="flex items-start justify-between gap-4 text-sm">
      <span className="flex flex-col gap-0.5">
        <span className="font-medium">{title}</span>
        {hint && <span className="text-xs leading-relaxed text-muted-foreground">{hint}</span>}
      </span>
      {children}
    </label>
  )
}

export function Rows({ children, className, ...props }: React.ComponentProps<"ul">) {
  return (
    <ul className={cn("divide-y", className)} {...props}>
      {children}
    </ul>
  )
}

export function Row({ children, className, ...props }: React.ComponentProps<"li">) {
  return (
    <li className={cn("flex items-center gap-3 px-5 py-3.5", className)} {...props}>
      {children}
    </li>
  )
}

export function LinkRow({ to, children, testId }: { to: string; children: React.ReactNode; testId?: string }) {
  return (
    <li>
      <Link to={to} className="flex items-center gap-3 px-5 py-3.5 transition-colors hover:bg-surface" data-testid={testId}>
        {children}
        <ChevronRightIcon className="size-4 shrink-0 text-faint rtl:rotate-180" />
      </Link>
    </li>
  )
}

export function RowText({ title, detail }: { title: React.ReactNode; detail?: React.ReactNode }) {
  return (
    <div className="min-w-0 flex-1">
      <p className="truncate text-sm font-medium">{title}</p>
      {detail && <p className="truncate text-xs text-muted-foreground">{detail}</p>}
    </div>
  )
}

export function EmptyRow({ children }: { children: React.ReactNode }) {
  return <p className="px-5 py-6 text-center text-sm text-muted-foreground">{children}</p>
}

export function StatusTag({ tone = "muted", children, ...props }: { tone?: "muted" | "success" | "warning" | "danger"; children: React.ReactNode } & React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-xs font-medium",
        tone === "success" ? "text-success" : tone === "warning" ? "text-warning" : tone === "danger" ? "text-destructive" : "text-muted-foreground",
      )}
      {...props}
    >
      <span className="size-1.5 rounded-full bg-current" />
      {children}
    </span>
  )
}

export function TabNav({ items, label }: { items: { to: string; label: React.ReactNode; end?: boolean }[]; label: string }) {
  return (
    <nav aria-label={label} className="-mx-1 flex items-center gap-1 overflow-x-auto pt-3">
      {items.map((it) => (
        <NavLink
          key={it.to}
          to={it.to}
          end={it.end}
          className={({ isActive }) =>
            cn(
              "h-8 shrink-0 rounded-full px-3 text-sm leading-8 whitespace-nowrap transition-colors",
              isActive ? "bg-muted font-medium text-foreground" : "text-muted-foreground hover:text-foreground",
            )
          }
        >
          {it.label}
        </NavLink>
      ))}
    </nav>
  )
}

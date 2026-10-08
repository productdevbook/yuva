import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronRightIcon } from "lucide-react"
import { Link } from "react-router"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

export function PageHeader({
  title,
  description,
  back,
  action,
  eyebrow,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  eyebrow?: React.ReactNode
  back?: { to: string; label: string } | false
  action?: React.ReactNode
  children?: React.ReactNode
}) {
  const { t } = useLingui()
  const to = back === undefined ? { to: "/settings", label: t`Settings` } : back
  return (
    <header className="flex flex-col">
      {to && (
        <Link to={to.to} className="mb-4.5 inline-flex w-fit items-center gap-1.5 text-sm text-faint transition-colors hover:text-foreground" data-testid="settings-back">
          <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
          {to.label}
        </Link>
      )}
      {eyebrow && <p className="mb-2.5 text-[13px] text-faint">{eyebrow}</p>}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="min-w-0 text-[28px] leading-tight font-semibold tracking-[-0.02em] break-words">{title}</h1>
        {action}
      </div>
      {description && <p className="mt-1.5 max-w-2xl text-[15px] leading-relaxed text-muted-foreground">{description}</p>}
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
  title?: React.ReactNode
  description?: React.ReactNode
  action?: React.ReactNode
  children?: React.ReactNode
  className?: string
}) {
  return (
    <section className={cn("flex flex-col gap-2", className)}>
      {(title || action) && (
        <div className="mx-1 flex flex-wrap items-end justify-between gap-3">
          {title && <h2 className="text-[13px] font-medium text-faint">{title}</h2>}
          {action}
        </div>
      )}
      {children}
      {description && <p className="mx-1 mt-0.5 max-w-2xl text-[13px] leading-relaxed text-faint">{description}</p>}
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
      <div className={cn(!p.flush && "flex flex-col gap-5 p-4")}>{p.children}</div>
      {p.footer && <div className="flex flex-wrap items-center gap-3 border-t px-4 py-3">{p.footer}</div>}
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
    <li className={cn("flex min-h-[52px] items-center gap-3 px-4 py-3", className)} {...props}>
      {children}
    </li>
  )
}

export function LinkRow({ to, children, value, testId }: { to: string; children: React.ReactNode; value?: React.ReactNode; testId?: string }) {
  return (
    <li>
      <Link to={to} className="flex min-h-[52px] items-center gap-3 px-4 py-3 transition-colors hover:bg-background" data-testid={testId}>
        {children}
        {value !== undefined && <span className="shrink-0 text-sm whitespace-nowrap text-faint">{value}</span>}
        <ChevronRightIcon className="size-4 shrink-0 text-faint rtl:rotate-180" />
      </Link>
    </li>
  )
}

export function RowText({ title, detail }: { title: React.ReactNode; detail?: React.ReactNode }) {
  return (
    <div className="min-w-0 flex-1">
      <p className="truncate text-sm">{title}</p>
      {detail && <p className="mt-px truncate text-[13px] text-faint">{detail}</p>}
    </div>
  )
}

export function EmptyRow({ children }: { children: React.ReactNode }) {
  return <p className="px-4 py-5 text-sm text-faint">{children}</p>
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


export function RowIcon({ children }: { children: React.ReactNode }) {
  return (
    <span className="grid size-[34px] shrink-0 place-items-center rounded-[10px] border bg-background text-[13px] font-medium text-muted-foreground [&_svg]:size-4">
      {children}
    </span>
  )
}

export function SettingRow({ title, hint, children, htmlFor }: { title: React.ReactNode; hint?: React.ReactNode; children?: React.ReactNode; htmlFor?: string }) {
  return (
    <li className="flex min-h-[52px] items-center gap-3 px-4 py-3 phone:flex-wrap">
      <label htmlFor={htmlFor} className="min-w-0 flex-1 text-sm phone:min-w-[55%]">
        {title}
        {hint && <small className="mt-px block text-[13px] text-faint">{hint}</small>}
      </label>
      {children}
    </li>
  )
}

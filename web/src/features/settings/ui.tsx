import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronRightIcon } from "lucide-react"
import { createContext, useContext } from "react"
import { Link, type To } from "react-router"

import { colorFor, ErrorLine } from "@/components/common"
import { OverflowTabs, pageTabClass } from "@/components/common/OverflowTabs"
import { useSaveBarEntry } from "@/components/common/SaveBar"
import { initials } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { cn } from "@/lib/utils"

const Band = createContext(false)

export function Page({
  testId,
  width = "full",
  className,
  children,
}: {
  testId?: string
  width?: "full" | "form"
  className?: string
  children: React.ReactNode
}) {
  return (
    <main className="min-h-0 flex-1 bg-background in-data-frame:overflow-y-auto" data-testid={testId}>
      <Band.Provider value>
        <div
          className={cn(
            "flex w-full flex-col gap-4 px-6 pb-16 phone:px-4",
            width === "form" && "[&>:not([data-page-header])]:w-full [&>:not([data-page-header])]:max-w-[760px]",
            className
          )}
        >
          {children}
        </div>
      </Band.Provider>
    </main>
  )
}

export const headButton = {
  primary: "h-[34px] rounded-lg bg-foreground px-3.5 text-background hover:bg-foreground/85",
  secondary: "h-[34px] rounded-lg border-input bg-background px-3.5 font-normal hover:border-faint",
}

export function PageHeader({
  title,
  description,
  back,
  action,
  eyebrow,
  tabs,
  children,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  eyebrow?: React.ReactNode
  back?: { to: string; label: string } | false
  action?: React.ReactNode
  tabs?: React.ReactNode
  children?: React.ReactNode
}) {
  const { t } = useLingui()
  const band = useContext(Band)
  const to = back === undefined ? { to: "/settings", label: t`Settings` } : back
  return (
    <header
      data-page-header
      className={cn(
        "flex flex-col",
        band &&
          "sticky top-0 z-10 -mx-6 mb-2 gap-3 border-b bg-background px-6 pt-5 phone:static phone:-mx-4 phone:px-4 phone:pt-3"
      )}
    >
      {to && (
        <Link
          to={to.to}
          className={cn(
            "w-fit items-center gap-1.5 text-body text-faint transition-colors hover:text-foreground",
            band ? "-mb-1" : "mb-4.5",
            back === undefined ? "hidden phone:inline-flex" : "inline-flex"
          )}
          data-testid="settings-back"
        >
          <ArrowLeftIcon className="size-3.5 rtl:rotate-180" />
          {to.label}
        </Link>
      )}
      {eyebrow && <p className={cn("text-small text-faint", !band && "mb-2.5")}>{eyebrow}</p>}
      <div className={cn("flex flex-wrap items-center gap-3", band && !tabs && "pb-4")}>
        <div className="flex min-w-0 flex-1 basis-64 flex-col">
          <h1 className="min-w-0 text-[22px] leading-[1.3] font-semibold tracking-[-0.02em] break-words">{title}</h1>
          {description && <div className="max-w-2xl text-body leading-[1.3] text-muted-foreground">{description}</div>}
        </div>
        {action && <div className="flex shrink-0 flex-wrap items-center gap-3">{action}</div>}
      </div>
      {tabs && <div className={cn(!band && "mt-3")}>{tabs}</div>}
      {children}
    </header>
  )
}

export type PageTab = {
  key: string
  to: To
  label: React.ReactNode
  count?: number
  testId?: string
}

export { pageTabClass }

export function PageTabs({ items, current, label }: { items: PageTab[]; current: string; label: string }) {
  const band = useContext(Band)
  return <OverflowTabs label={label} current={current} items={items} className={cn(!band && "border-b")} />
}

export function StatTile({
  label,
  value,
  hint,
  live,
  testId,
  title,
  hintClassName,
}: {
  label: React.ReactNode
  value: React.ReactNode
  hint?: React.ReactNode
  live?: boolean
  testId?: string
  title?: string
  hintClassName?: string
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-xl border bg-card p-4" title={title} data-testid={testId}>
      <span className="flex min-w-0 items-center gap-1.5 text-body text-muted-foreground">
        {live && <span className="size-2 shrink-0 rounded-full bg-success" aria-hidden />}
        <span className="truncate">{label}</span>
      </span>
      <b className="text-[26px] leading-[1.25] font-semibold tracking-[-0.02em] tabular-nums">{value}</b>
      {hint && <span className={cn("truncate text-caption text-faint", hintClassName)}>{hint}</span>}
    </div>
  )
}

export function CardHead({ children, action }: { children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="flex min-h-11 items-center gap-2 border-b px-4 py-2.5">
      <h2 className="flex min-w-0 flex-1 items-center gap-2 truncate text-body font-semibold">{children}</h2>
      {action}
    </div>
  )
}

export function HeadNote({ children }: { children: React.ReactNode }) {
  return <span className="shrink-0 text-caption whitespace-nowrap text-faint">{children}</span>
}

export function HeadLink({ to, children, testId }: { to: To; children: React.ReactNode; testId?: string }) {
  return (
    <Link
      to={to}
      className="shrink-0 text-body whitespace-nowrap text-muted-foreground transition-colors hover:text-foreground"
      data-testid={testId}
    >
      {children}
    </Link>
  )
}

export function mix(color: string, percent: number) {
  return `color-mix(in srgb, ${color} ${percent}%, transparent)`
}

export function ProductTag({ name, color, className }: { name: string; color?: string; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex max-w-32 min-w-0 shrink-0 items-center rounded border px-1.5 text-[11px] leading-[17px] font-medium text-muted-foreground",
        className
      )}
      style={color ? { color, borderColor: mix(color, 25) } : undefined}
      data-testid="product-tag"
    >
      <span className="truncate">{name}</span>
    </span>
  )
}

export function Chip({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center rounded border bg-surface px-1.5 text-[11px] leading-[17px] whitespace-nowrap text-muted-foreground",
        className
      )}
    >
      {children}
    </span>
  )
}

export function Pill({
  tone = "success",
  children,
  className,
  ...props
}: { tone?: "success" | "warning" | "danger" | "muted"; children: React.ReactNode } & React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center rounded-full border px-2 py-px text-caption font-medium whitespace-nowrap",
        tone === "success" && "border-success/35 text-success",
        tone === "warning" && "border-warning/35 text-warning",
        tone === "danger" && "border-destructive/35 text-destructive",
        tone === "muted" && "border-input text-muted-foreground",
        className
      )}
      {...props}
    >
      {children}
    </span>
  )
}

export function Bars({
  values,
  color = "var(--brand)",
  className,
  barClassName,
  fade = 0.45,
  label,
  min = 2,
}: {
  values: number[]
  color?: string
  className?: string
  barClassName?: string
  fade?: number
  label?: string
  min?: number
}) {
  const max = Math.max(1, ...values)
  return (
    <span
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn("flex items-end gap-0.5", className)}
    >
      {values.map((v, i) => (
        <span
          key={i}
          className={cn("flex-1 rounded-t-[2px]", barClassName)}
          style={{
            height: `max(${min}px, ${(v / max) * 100}%)`,
            backgroundColor: color,
            opacity: i === values.length - 1 ? 1 : fade,
          }}
        />
      ))}
    </span>
  )
}

export function Meter({
  value,
  max,
  color = "var(--brand)",
  className,
}: {
  value: number
  max: number
  color?: string
  className?: string
}) {
  return (
    <span className={cn("flex h-1.5 overflow-hidden rounded-full bg-border", className)} aria-hidden>
      <span
        className="rounded-full"
        style={{ width: `${max > 0 ? (value / max) * 100 : 0}%`, backgroundColor: color }}
      />
    </span>
  )
}

export function Face({
  id,
  name,
  online,
  away,
  className,
}: {
  id: string
  name: string
  online?: boolean
  away?: boolean
  className?: string
}) {
  const here = online && !away
  return (
    <span
      aria-hidden
      className={cn(
        "relative grid size-8 shrink-0 place-items-center rounded-full text-[11px] font-semibold text-white",
        className
      )}
      style={{ backgroundColor: colorFor(id) }}
    >
      {initials(name)}
      {online !== undefined && (
        <span
          className={cn(
            "absolute -end-px -bottom-px size-[30%] max-h-[11px] min-h-[9px] max-w-[11px] min-w-[9px] rounded-full border-2 border-card",
            here ? "bg-success" : "bg-input"
          )}
        />
      )}
    </span>
  )
}

export const tableHeadClass = "bg-surface text-caption font-medium text-faint"

export function Block({
  title,
  description,
  footer,
  tone,
  children,
  testId,
  onSubmit,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  footer?: React.ReactNode
  tone?: "danger"
  children: React.ReactNode
  testId?: string
  onSubmit?: () => void
}) {
  const body = (
    <>
      <div className="flex flex-col border-b px-[18px] py-3.5 leading-[1.3]">
        <h2 className="font-semibold">{title}</h2>
        {description && <p className="text-caption text-faint">{description}</p>}
      </div>
      {children}
      {footer && <div className="flex flex-wrap items-center justify-end gap-3 px-[18px] py-3">{footer}</div>}
    </>
  )
  const cls = cn(
    "flex flex-col overflow-hidden rounded-xl border bg-card",
    tone === "danger" && "border-destructive/30"
  )
  return onSubmit ? (
    <form
      className={cls}
      data-testid={testId}
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit()
      }}
    >
      {body}
    </form>
  ) : (
    <section className={cls} data-testid={testId}>
      {body}
    </section>
  )
}

export function BlockRow({
  label,
  htmlFor,
  hint,
  children,
}: {
  label: React.ReactNode
  htmlFor?: string
  hint?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[200px_minmax(0,1fr)] items-center gap-4 border-b px-[18px] py-3 phone:grid-cols-1 phone:gap-1.5">
      <Label htmlFor={htmlFor} className="font-medium">
        {label}
      </Label>
      <div className="flex min-w-0 flex-col gap-1">
        {children}
        {hint && <p className="text-caption text-faint">{hint}</p>}
      </div>
    </div>
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
          {title && <h2 className="text-small font-medium text-faint">{title}</h2>}
          {action}
        </div>
      )}
      {children}
      {description && <p className="mx-1 mt-0.5 max-w-2xl text-small text-faint">{description}</p>}
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
  cn(
    "flex flex-col overflow-hidden rounded-xl border bg-card",
    p.tone === "danger" && "border-destructive/30",
    p.className
  )

function CardBody(p: CardProps) {
  return (
    <>
      <div className={cn(!p.flush && "flex flex-col gap-5 p-4")}>{p.children}</div>
      {p.footer && (
        <div className="flex flex-wrap items-center gap-3 border-t px-4 py-3 [&:not(:has(>:not([hidden])))]:hidden">
          {p.footer}
        </div>
      )}
    </>
  )
}

export function Card({
  children,
  footer,
  flush,
  tone,
  className,
  ...rest
}: CardProps & Omit<React.ComponentProps<"div">, "children">) {
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
  const inBar = useSaveBarEntry({ enabled: !disabled && !pending, pending })
  if (inBar)
    return (
      <>
        <ErrorLine error={error} className="min-w-0 flex-1" />
        <button type="submit" hidden ref={inBar} tabIndex={-1} aria-hidden />
      </>
    )
  return (
    <>
      <ErrorLine error={error} className="min-w-0 flex-1" />
      {saved && !error && (
        <span className="flex-1 text-body text-muted-foreground" role="status">
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
        <p role="alert" className="text-small text-destructive">
          {error}
        </p>
      ) : (
        hint && <p className="text-small text-faint">{hint}</p>
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
    <Label className="items-start justify-between gap-4 font-normal">
      <span className="flex flex-col gap-0.5">
        <span className="font-medium">{title}</span>
        {hint && <span className="text-small text-faint">{hint}</span>}
      </span>
      {children}
    </Label>
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

export function LinkRow({
  to,
  state,
  children,
  value,
  testId,
}: {
  to: string
  state?: unknown
  children: React.ReactNode
  value?: React.ReactNode
  testId?: string
}) {
  return (
    <li>
      <Link
        to={to}
        state={state}
        className="flex min-h-[52px] items-center gap-3 px-4 py-3 transition-colors hover:bg-muted"
        data-testid={testId}
      >
        {children}
        {value !== undefined && <span className="shrink-0 text-body whitespace-nowrap text-faint">{value}</span>}
        <ChevronRightIcon className="size-4 shrink-0 text-faint rtl:rotate-180" />
      </Link>
    </li>
  )
}

export function RowText({ title, detail }: { title: React.ReactNode; detail?: React.ReactNode }) {
  return (
    <div className="min-w-0 flex-1">
      <p className="truncate text-body">{title}</p>
      {detail && <p className="mt-px truncate text-small text-faint">{detail}</p>}
    </div>
  )
}

export function EmptyRow({ children }: { children: React.ReactNode }) {
  return <p className="px-4 py-5 text-body text-faint">{children}</p>
}

export function StatusTag({
  tone = "muted",
  children,
  ...props
}: { tone?: "muted" | "success" | "warning" | "danger"; children: React.ReactNode } & React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-caption font-medium",
        tone === "success"
          ? "text-success"
          : tone === "warning"
            ? "text-warning"
            : tone === "danger"
              ? "text-destructive"
              : "text-muted-foreground"
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
    <span className="grid size-[34px] shrink-0 place-items-center rounded-[10px] border bg-surface text-small font-medium text-muted-foreground [&_svg]:size-4">
      {children}
    </span>
  )
}

export function SettingRow({
  title,
  hint,
  children,
  htmlFor,
}: {
  title: React.ReactNode
  hint?: React.ReactNode
  children?: React.ReactNode
  htmlFor?: string
}) {
  return (
    <li className="flex min-h-[52px] items-center gap-3 px-4 py-3 phone:flex-wrap">
      <Label htmlFor={htmlFor} className="block min-w-0 flex-1 font-normal phone:min-w-[55%]">
        {title}
        {hint && <small className="mt-px block text-small text-faint">{hint}</small>}
      </Label>
      {children}
    </li>
  )
}

export function ChoiceSelect<T extends string>({
  id,
  value,
  onChange,
  options,
  label,
  disabled,
  className,
  placeholder,
}: {
  id?: string
  value: T
  onChange: (value: T) => void
  options: readonly (readonly [T, string])[]
  label?: string
  placeholder?: string
  disabled?: boolean
  className?: string
}) {
  const items = Object.fromEntries(options) as Record<T, string>
  return (
    <Select
      value={placeholder && !(value in items) ? null : value}
      items={items}
      onValueChange={(v) => v !== null && onChange(v as T)}
      disabled={disabled}
    >
      <SelectTrigger id={id} aria-label={label} className={cn("max-w-full min-w-0 bg-background", className)}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent className="min-w-48">
        {options.map(([v, l]) => (
          <SelectItem key={v} value={v}>
            {l}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

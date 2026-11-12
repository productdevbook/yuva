import { useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, MoonIcon, SearchIcon, SunIcon } from "lucide-react"
import { createContext, useContext, useLayoutEffect, useRef } from "react"
import { createPortal } from "react-dom"
import { Link, type To } from "react-router"

import { moveFocus } from "@/app/Rail"
import { useShell } from "@/app/shell"
import { KeyHint } from "@/app/Sidebar"
import { useDark } from "@/components/common/EmailHtml"
import { mod } from "@/components/common/ShortcutSheet"
import { setTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export function PageLayout({
  sidebar,
  aside,
  label,
  testId,
  children,
}: {
  sidebar?: React.ReactNode
  aside?: React.ReactNode
  label?: string
  testId?: string
  children: React.ReactNode
}) {
  return (
    <div
      className="flex min-w-0 flex-1 flex-col bg-background in-data-frame:min-h-0 in-data-frame:overflow-y-auto"
      data-testid={testId}
    >
      <div aria-hidden className="h-0.5 shrink-0 bg-brand max-md:hidden" />
      <div className="mx-auto flex w-full max-w-[1440px] flex-1 items-stretch">
        {sidebar}
        <main aria-label={label} className="min-w-0 flex-[999_1_560px] ps-8 pe-7 pb-16 max-md:px-6 phone:px-4">
          {children}
        </main>
        {aside}
      </div>
    </div>
  )
}

export function SideColumn({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <aside
      aria-label={label}
      className="relative flex w-60 shrink-0 flex-col gap-5 border-e border-line-soft pt-5 ps-5 pe-4 pb-8 max-md:hidden"
      data-testid="page-sidebar"
    >
      {children}
    </aside>
  )
}

export function InfoColumn({ label, children }: { label: string; children?: React.ReactNode }) {
  return (
    <aside aria-label={label} className="w-60 shrink-0 pt-6 ps-2 pe-6 pb-8 max-[1180px]:hidden" data-testid="page-info">
      <div className="mb-3 flex justify-end">
        <ThemeToggle />
      </div>
      {children}
    </aside>
  )
}

export function ThemeToggle() {
  const { t } = useLingui()
  const dark = useDark()
  return (
    <button
      type="button"
      onClick={() => setTheme(dark ? "light" : "dark")}
      aria-pressed={dark}
      aria-label={t`Dark mode`}
      title={t`Dark mode`}
      className="grid size-8 place-items-center rounded-lg border bg-background text-fg2 outline-none transition-colors hover:bg-raise hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      data-testid="theme-toggle"
    >
      {dark ? <SunIcon className="size-4" /> : <MoonIcon className="size-4" />}
    </button>
  )
}

export function BackArrow({ className }: { className?: string }) {
  return <ArrowLeftIcon aria-hidden className={cn("shrink-0 rtl:-scale-x-100", className)} />
}

export function SideTitle({ title, back }: { title: React.ReactNode; back?: { to: To; label: string } }) {
  return (
    <div className="flex flex-col gap-0.5 px-2.5" data-testid="page-title">
      {back && (
        <Link
          to={back.to}
          className="inline-flex items-center gap-1.5 self-start rounded-sm text-caption leading-[18px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          data-testid="page-title-back"
        >
          <BackArrow className="size-3" />
          {back.label}
        </Link>
      )}
      <div className="text-title text-foreground">{title}</div>
    </div>
  )
}

export function PageSearch({ label }: { label?: string }) {
  const { t } = useLingui()
  const { openPalette } = useShell()
  const text = label ?? t`Search`
  return (
    <button
      type="button"
      onClick={openPalette}
      aria-label={`${text} ${mod} K`}
      title={text}
      className="flex h-9 w-full items-center gap-2 rounded-md border bg-subtle px-3.5 text-body text-muted-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring"
      data-testid="page-search"
    >
      <SearchIcon aria-hidden className="size-[15px] shrink-0" />
      <span className="min-w-0 flex-1 truncate text-start">{t`Search`}</span>
      <KeyHint keys={[mod, "K"]} className="rounded border border-input bg-background px-1.5 text-muted-foreground" />
    </button>
  )
}

export type NavItem = {
  key: string
  label: string
  count?: number
  hot?: boolean
  current?: boolean
  sub?: boolean
  color?: string
  to?: To
  state?: unknown
  onSelect?: () => void
  ariaLabel?: string
  title?: string
  testId?: string
}

const rowClass =
  "group flex w-full items-center gap-2 rounded-sm px-2.5 py-1 text-start text-body leading-5 text-fg2 outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:font-medium aria-[current=page]:text-foreground"

const labelClass =
  "-mb-0.5 min-w-0 flex-1 truncate pb-0.5 underline-offset-4 group-hover:underline group-aria-[current=page]:underline group-aria-[current=page]:decoration-brand group-aria-[current=page]:decoration-2"

export function NavRow({ item }: { item: NavItem }) {
  const current = item.current ? ("page" as const) : undefined
  const className = cn(rowClass, item.sub && "py-[3px] ps-[26px] text-small leading-[18px] text-muted-foreground")
  const inner = (
    <>
      {item.color !== undefined && (
        <span
          aria-hidden
          className="size-[7px] shrink-0 rounded-full bg-muted2"
          style={item.color ? { backgroundColor: item.color } : undefined}
        />
      )}
      <span className={labelClass}>{item.label}</span>
      {!!item.count && (
        <span
          className={cn("shrink-0 font-mono text-[11px] text-muted-foreground", item.hot && "font-semibold text-brand")}
          data-testid={item.testId ? `${item.testId}-count` : undefined}
        >
          {item.count > 999 ? "999+" : item.count}
        </span>
      )}
    </>
  )
  if (item.to !== undefined)
    return (
      <Link
        to={item.to}
        state={item.state}
        onClick={item.onSelect}
        aria-current={current}
        aria-label={item.ariaLabel}
        title={item.title}
        className={className}
        data-rail-item
        data-testid={item.testId}
      >
        {inner}
      </Link>
    )
  return (
    <button
      type="button"
      onClick={item.onSelect}
      aria-current={current}
      aria-label={item.ariaLabel}
      title={item.title}
      className={className}
      data-rail-item
      data-testid={item.testId}
    >
      {inner}
    </button>
  )
}

export function NavGroupLabel({ children }: { children: React.ReactNode }) {
  return <div className="px-2.5 pb-1.5 text-caption font-medium text-muted2">{children}</div>
}

export function NavGroup({ title, items, children }: { title?: string; items: NavItem[]; children?: React.ReactNode }) {
  return (
    <div role="group" aria-label={title}>
      {title && <NavGroupLabel>{title}</NavGroupLabel>}
      {items.map((i) => (
        <NavRow key={i.key} item={i} />
      ))}
      {children}
    </div>
  )
}

export function NavList({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <nav aria-label={label} onKeyDown={moveFocus} className="flex flex-col gap-[22px]" data-testid="page-nav">
      {children}
    </nav>
  )
}

const TopSlotContext = createContext<HTMLElement | null>(null)

export const TopSlotProvider = TopSlotContext.Provider

export function TopPortal({ children }: { children: React.ReactNode }) {
  const slot = useContext(TopSlotContext)
  return slot ? createPortal(children, slot) : children
}

export function PageTop({
  back,
  actions,
  slot,
  children,
}: {
  back?: { label: string; to?: To; onClick?: () => void; testId?: string }
  actions?: React.ReactNode
  slot?: (el: HTMLElement | null) => void
  children?: React.ReactNode
}) {
  const bar = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const el = bar.current
    if (!el || typeof ResizeObserver === "undefined") return
    const root = document.documentElement
    const io = new ResizeObserver(() => root.style.setProperty("--page-top-h", `${el.offsetHeight}px`))
    io.observe(el)
    return () => {
      io.disconnect()
      root.style.removeProperty("--page-top-h")
    }
  }, [])
  const backClass =
    "-ms-2 inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2 text-small text-fg2 outline-none hover:bg-line-soft hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
  return (
    <div
      ref={bar}
      className="sticky top-0 z-20 flex min-h-8 flex-wrap items-center gap-x-3 gap-y-2 bg-background pt-3.5 pb-2.5"
      data-testid="page-top"
    >
      {back &&
        (back.to !== undefined ? (
          <Link to={back.to} onClick={back.onClick} className={backClass} data-testid={back.testId ?? "page-back"}>
            <BackArrow className="size-3.5" />
            {back.label}
          </Link>
        ) : (
          <button type="button" onClick={back.onClick} className={backClass} data-testid={back.testId ?? "page-back"}>
            <BackArrow className="size-3.5" />
            {back.label}
          </button>
        ))}
      <div className="flex min-w-0 flex-1 basis-40 flex-wrap items-center gap-x-2 gap-y-0.5 font-mono text-caption tracking-[0.04em] text-muted2 uppercase">
        {children}
      </div>
      {(slot || actions) && (
        <div className="ms-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5">
          {slot && <div ref={slot} className="flex min-w-0 flex-wrap items-center gap-1.5 empty:hidden" />}
          {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
        </div>
      )}
    </div>
  )
}

export const topButton = "h-8 rounded-[9px] border-input bg-card px-3 font-normal"
export const topInk = "h-8 rounded-[9px] bg-foreground px-3.5 font-semibold text-background hover:bg-foreground/90"

export function TopSep() {
  return (
    <span aria-hidden className="text-input">
      ·
    </span>
  )
}

export function PageHeading({ title, lede }: { title: React.ReactNode; lede?: React.ReactNode }) {
  return (
    <header className="flex flex-col">
      <h1 className="m-0 text-page leading-[34px]">{title}</h1>
      {lede && <p className="m-0 mt-1.5 max-w-[680px] text-reading text-muted-foreground">{lede}</p>}
    </header>
  )
}

export function SectionHeading({
  id,
  title,
  lead,
  more,
}: {
  id?: string
  title: React.ReactNode
  lead?: React.ReactNode
  more?: React.ReactNode
}) {
  return (
    <div className="mt-9 mb-3 flex flex-col gap-1.5">
      <h2 id={id} className="m-0 flex scroll-mt-16 items-baseline gap-3 text-lg font-semibold tracking-[-0.01em]">
        <span className="min-w-0">{title}</span>
        {more && <span className="ms-auto text-small font-normal">{more}</span>}
      </h2>
      {lead && <p className="m-0 text-body leading-[22px] text-muted-foreground">{lead}</p>}
    </div>
  )
}

export function MoreLink({ to, children, testId }: { to: To; children: React.ReactNode; testId?: string }) {
  return (
    <Link
      to={to}
      className="rounded-sm whitespace-nowrap text-fg2 underline-offset-4 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
      data-testid={testId}
    >
      {children}
    </Link>
  )
}

export function ListGroup({ label, children }: { label?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="mt-7 first:mt-0">
      {label && <div className="pb-2 text-caption font-medium text-muted2">{label}</div>}
      {children}
    </div>
  )
}

export const listRowClass =
  "flex w-full min-w-0 items-center gap-3.5 border-t border-line-soft px-1 py-[13px] text-start text-body text-fg2 outline-none transition-colors hover:bg-subtle focus-visible:bg-subtle"

export function ListRow({
  to,
  onClick,
  className,
  children,
  ...rest
}: {
  to?: To
  onClick?: () => void
  className?: string
  children: React.ReactNode
} & Omit<React.HTMLAttributes<HTMLElement>, "onClick" | "className" | "children"> &
  Record<`data-${string}`, string | undefined>) {
  if (to !== undefined)
    return (
      <Link to={to} onClick={onClick} className={cn(listRowClass, className)} {...rest}>
        {children}
      </Link>
    )
  if (onClick)
    return (
      <button type="button" onClick={onClick} className={cn(listRowClass, className)} {...rest}>
        {children}
      </button>
    )
  return (
    <div className={cn(listRowClass, "hover:bg-transparent", className)} {...rest}>
      {children}
    </div>
  )
}

export function InfoSection({
  title,
  action,
  children,
  testId,
}: {
  title: React.ReactNode
  action?: React.ReactNode
  children: React.ReactNode
  testId?: string
}) {
  return (
    <section
      className="border-t border-line-soft py-3.5 first-of-type:border-t-0 first-of-type:pt-0"
      data-testid={testId}
    >
      <h3 className="m-0 mb-2 flex items-center gap-2 text-caption font-medium text-muted2">
        <span className="min-w-0 flex-1 truncate">{title}</span>
        {action}
      </h3>
      {children}
    </section>
  )
}

export function KV({
  label,
  value,
  tone,
  to,
  testId,
}: {
  label: React.ReactNode
  value: React.ReactNode
  tone?: "ok" | "warn" | "bad" | "brand"
  to?: To
  testId?: string
}) {
  const toneClass = { ok: "text-ok", warn: "text-warn", bad: "text-bad", brand: "text-brand" }
  const body = (
    <>
      <span className="shrink-0 whitespace-nowrap text-muted-foreground">{label}</span>
      <span
        className={cn(
          "min-w-0 flex-1 text-end [overflow-wrap:anywhere]",
          typeof value === "string" && value.length <= 24 && "whitespace-nowrap",
          tone && toneClass[tone]
        )}
      >
        {value}
      </span>
    </>
  )
  const className = "flex justify-between gap-3 py-[3px] text-small leading-5"
  if (to !== undefined)
    return (
      <Link
        to={to}
        className={cn(
          className,
          "rounded-sm outline-none hover:[&>span:first-child]:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
        )}
        data-testid={testId}
      >
        {body}
      </Link>
    )
  return (
    <div className={className} data-testid={testId}>
      {body}
    </div>
  )
}

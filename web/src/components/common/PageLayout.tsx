import { useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, CheckIcon, ChevronDownIcon, MoonIcon, SunIcon } from "lucide-react"
import { createContext, useContext, useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react"
import { createPortal } from "react-dom"
import { Link, useNavigate, type To } from "react-router"

import { useDark } from "@/components/common/EmailHtml"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { setTheme } from "@/lib/theme"
import { cn } from "@/lib/utils"

export function PageLayout({
  nav,
  aside,
  label,
  testId,
  children,
}: {
  nav?: React.ReactNode
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
      <div className="mx-auto flex w-full max-w-[1440px] flex-1 items-stretch">
        <main aria-label={label} className="min-w-0 flex-[999_1_560px] ps-8 pe-7 pb-16 max-md:px-6 phone:px-4">
          {nav}
          {children}
        </main>
        {aside}
      </div>
    </div>
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

export function BackButton({ to, label, className }: { to: To; label: string; className?: string }) {
  return (
    <Link
      to={to}
      aria-label={label}
      title={label}
      className={cn(
        "grid size-9 shrink-0 place-items-center rounded-md border bg-subtle text-fg2 outline-none transition-colors hover:bg-raise hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring",
        className
      )}
      data-testid="page-title-back"
    >
      <BackArrow className="size-4" />
    </Link>
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

const MENU_AT = 7

function Count({ item }: { item: NavItem }) {
  if (!item.count) return null
  return (
    <span
      className={cn(
        "shrink-0 text-caption font-normal text-muted-foreground tabular-nums",
        item.hot && "font-semibold text-brand"
      )}
      data-testid={item.testId ? `${item.testId}-count` : undefined}
    >
      {item.count > 999 ? "999+" : item.count}
    </span>
  )
}

function ColorDot({ color }: { color?: string }) {
  if (color === undefined) return null
  return (
    <span
      aria-hidden
      className="size-[7px] shrink-0 rounded-full bg-muted2"
      style={color ? { backgroundColor: color } : undefined}
    />
  )
}

function useOpen() {
  const navigate = useNavigate()
  return (item: NavItem) => {
    item.onSelect?.()
    if (item.to !== undefined) navigate(item.to, item.state === undefined ? undefined : { state: item.state })
  }
}

function MenuGroup({ title, items }: { title?: string; items: NavItem[] }) {
  const { t } = useLingui()
  const open = useOpen()
  const current = items.find((i) => i.current)
  const name = current?.label ?? title ?? t`Show`
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="sm" className="h-8 max-w-64 gap-1.5 bg-card font-normal" />}
        aria-label={title ? `${title}: ${name}` : name}
        data-testid="page-nav-menu"
      >
        <ColorDot color={current?.color} />
        {title && current && <span className="shrink-0 text-muted-foreground">{title}</span>}
        <span className="min-w-0 truncate font-medium">{name}</span>
        <ChevronDownIcon className="size-3.5 text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-[60svh] min-w-56">
        <DropdownMenuGroup>
          {title && <DropdownMenuLabel>{title}</DropdownMenuLabel>}
          {items.map((i) => (
            <DropdownMenuItem
              key={i.key}
              onClick={() => open(i)}
              aria-current={i.current ? "page" : undefined}
              aria-label={i.ariaLabel}
              title={i.title}
              className={cn(i.sub && "ps-6")}
              data-testid={i.testId}
            >
              <ColorDot color={i.color} />
              <span className="min-w-0 flex-1 truncate">{i.label}</span>
              <Count item={i} />
              <CheckIcon className={cn("size-3.5", !i.current && "invisible")} />
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ToggleRow({ title, items }: { title?: string; items: NavItem[] }) {
  const { t } = useLingui()
  const open = useOpen()
  const current = items.find((i) => i.current)?.key
  return (
    <ToggleGroup
      value={current ? [current] : []}
      onValueChange={(v) => {
        const item = items.find((i) => i.key === v[0])
        if (item) open(item)
      }}
      aria-label={title ?? t`Show`}
      size="sm"
      spacing={1}
      className="flex-wrap"
    >
      {items.map((i) => (
        <ToggleGroupItem
          key={i.key}
          value={i.key}
          aria-label={i.ariaLabel}
          title={i.title}
          className="h-8 gap-1.5 rounded-[7px] px-2.5 font-normal text-fg2 aria-pressed:bg-raise aria-pressed:font-semibold aria-pressed:text-foreground"
          data-testid={i.testId}
        >
          <ColorDot color={i.color} />
          {i.label}
          <Count item={i} />
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

export function NavGroup({
  title,
  items,
  menu: asMenu,
  children,
}: {
  title?: string
  items: NavItem[]
  menu?: boolean
  children?: React.ReactNode
}) {
  if (items.length === 0 && !children) return null
  const menu = asMenu || items.length >= MENU_AT || items.some((i) => i.color !== undefined)
  return (
    <div role="group" aria-label={title} className="flex min-w-0 items-center gap-2">
      {items.length > 0 &&
        (menu ? <MenuGroup title={title} items={items} /> : <ToggleRow title={title} items={items} />)}
      {children}
    </div>
  )
}

export function NavList({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <nav
      aria-label={label}
      className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2 pt-3 empty:hidden"
      data-testid="page-nav"
    >
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

export type PhoneBack = { label: string; to?: To; run: () => void }

let phoneBack: PhoneBack | undefined
const phoneBackListeners = new Set<() => void>()

function setPhoneBack(next: PhoneBack | undefined) {
  phoneBack = next
  phoneBackListeners.forEach((l) => l())
}

export function usePhoneBack() {
  return useSyncExternalStore(
    (l) => {
      phoneBackListeners.add(l)
      return () => phoneBackListeners.delete(l)
    },
    () => phoneBack
  )
}

export function usePhoneBackTarget(back?: { label: string; to?: To; onClick?: () => void }) {
  const latest = useRef(back)
  useEffect(() => {
    latest.current = back
  })
  const key = back ? `${JSON.stringify(back.to ?? null)}|${back.label}` : ""
  useEffect(() => {
    if (!key) return
    const { label, to } = latest.current!
    const mine: PhoneBack = { label, to, run: () => latest.current?.onClick?.() }
    setPhoneBack(mine)
    return () => {
      if (phoneBack === mine) setPhoneBack(undefined)
    }
  }, [key])
}

export function PageBackLink({
  to,
  label,
  className,
  testId,
}: {
  to: string
  label: string
  className?: string
  testId?: string
}) {
  usePhoneBackTarget({ to, label })
  return (
    <Link to={to} className={cn("max-md:hidden", className)} data-testid={testId}>
      <BackArrow className="size-3.5" />
      {label}
    </Link>
  )
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
  usePhoneBackTarget(back)
  const backClass =
    "-ms-2 inline-flex max-md:hidden h-7 shrink-0 items-center gap-1.5 rounded-md px-2 text-small text-fg2 outline-none hover:bg-line-soft hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
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

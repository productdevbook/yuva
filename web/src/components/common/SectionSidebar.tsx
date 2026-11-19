import { useEffect, useRef } from "react"
import { Link, type To } from "react-router"

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"

export type Place = {
  key: string
  label: string
  to?: To
  state?: unknown
  onSelect?: () => void
  count?: number
  hot?: boolean
  current?: boolean
  color?: string
  title?: string
  ariaLabel?: string
  testId?: string
}

export type PlaceGroup = { key: string; title?: string; items?: Place[]; node?: React.ReactNode }

export const placeLabelClass =
  "h-auto px-2.5 pt-3.5 pb-1 text-[11.5px] font-semibold tracking-[0.02em] text-muted-foreground group-first/places:pt-0 max-[899px]:hidden"

export const placeButtonClass =
  "h-8 gap-2 rounded-[7px] px-2.5 text-[13.5px] text-fg2 hover:bg-raise hover:text-foreground data-active:bg-brand-wash data-active:font-semibold data-active:text-foreground max-[899px]:w-auto max-[899px]:whitespace-nowrap"

export const placeMenuClass = "gap-0.5 max-[899px]:contents"

export const placeItemClass = "max-[899px]:shrink-0"

export function PlaceCount({ n, hot, testId }: { n?: number; hot?: boolean; testId?: string }) {
  if (!n) return null
  return (
    <span
      className={cn(
        "ms-auto shrink-0 ps-2 text-caption font-medium text-muted-foreground tabular-nums",
        hot && "font-semibold text-brand"
      )}
      data-testid={testId}
    >
      {n > 999 ? "999+" : n}
    </span>
  )
}

export function PlaceButton({ place }: { place: Place }) {
  const current = !!place.current
  const inner = (
    <>
      {place.color !== undefined && (
        <span
          aria-hidden
          className="size-2 shrink-0 rounded-[3px] bg-muted2"
          style={place.color ? { backgroundColor: place.color } : undefined}
        />
      )}
      <span className="min-w-0 flex-1 truncate">{place.label}</span>
      <PlaceCount n={place.count} hot={place.hot} testId={place.testId ? `${place.testId}-count` : undefined} />
    </>
  )
  return (
    <SidebarMenuButton
      render={place.to !== undefined ? <Link to={place.to} state={place.state} /> : undefined}
      onClick={place.onSelect}
      isActive={current}
      aria-current={current ? "page" : undefined}
      aria-label={place.ariaLabel}
      title={place.title}
      className={placeButtonClass}
      data-place
      data-testid={place.testId}
    >
      {inner}
    </SidebarMenuButton>
  )
}

function moveFocus(e: React.KeyboardEvent<HTMLElement>) {
  if (e.key !== "ArrowDown" && e.key !== "ArrowUp" && e.key !== "ArrowLeft" && e.key !== "ArrowRight") return
  if (e.altKey) return
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("[data-place]"))
  const i = items.indexOf(document.activeElement as HTMLElement)
  if (i < 0) return
  e.preventDefault()
  const step = e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : -1
  items[(i + step + items.length) % items.length].focus()
}

export function SectionSidebar({ label, groups }: { label: string; groups: PlaceGroup[] }) {
  const nav = useRef<HTMLElement>(null)
  const active = groups.flatMap((g) => g.items ?? []).find((p) => p.current)?.key
  useEffect(() => {
    const el = nav.current?.querySelector<HTMLElement>("[aria-current=page]")
    if (el && nav.current && nav.current.scrollWidth > nav.current.clientWidth)
      el.scrollIntoView({ block: "nearest", inline: "center" })
  }, [active])
  return (
    <Sidebar
      collapsible="none"
      className="h-auto w-[232px] shrink-0 border-e border-line-soft bg-background max-[899px]:w-full max-[899px]:border-e-0 max-[899px]:border-b"
      data-testid="section-sidebar"
    >
      <SidebarContent className="sticky top-0 max-h-svh flex-none overflow-y-auto in-data-frame:max-h-full max-[899px]:static max-[899px]:max-h-none max-[899px]:overflow-visible">
        <nav
          ref={nav}
          aria-label={label}
          onKeyDown={moveFocus}
          className="flex flex-col px-2.5 pt-4 pb-6 max-[899px]:flex-row max-[899px]:items-center max-[899px]:gap-1 max-[899px]:overflow-x-auto max-[899px]:px-3 max-[899px]:py-2 max-[899px]:[scrollbar-width:none]"
          data-testid="section-nav"
        >
          {groups.map((g) => (
            <SidebarGroup
              key={g.key}
              role="group"
              aria-label={g.title}
              className="group/places p-0 max-[899px]:contents"
              data-testid={`section-group-${g.key}`}
            >
              {g.title && <SidebarGroupLabel className={placeLabelClass}>{g.title}</SidebarGroupLabel>}
              {g.node ?? (
                <SidebarMenu className={placeMenuClass}>
                  {(g.items ?? []).map((p) => (
                    <SidebarMenuItem key={p.key} className={placeItemClass}>
                      <PlaceButton place={p} />
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              )}
            </SidebarGroup>
          ))}
        </nav>
      </SidebarContent>
    </Sidebar>
  )
}

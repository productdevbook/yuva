import { useLingui } from "@lingui/react/macro"
import { ChevronDownIcon } from "lucide-react"
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react"
import { Link, type To } from "react-router"

import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

export const pageTabClass =
  "flex shrink-0 items-center gap-1.5 pt-1.5 pb-2.5 text-body whitespace-nowrap text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:text-foreground focus-visible:underline data-popup-open:text-foreground aria-[current=page]:font-medium aria-[current=page]:text-foreground aria-[current=page]:shadow-[inset_0_-2px_0_currentColor]"

export type OverflowTab = {
  key: string
  label: React.ReactNode
  count?: number | string
  to?: To
  title?: string
  testId?: string
  onSelect?: () => void
}

type Sizes = { avail: number; gap: number; widths: number[] }

function sameSizes(a: Sizes | null, b: Sizes) {
  return (
    a !== null &&
    a.avail === b.avail &&
    a.gap === b.gap &&
    a.widths.length === b.widths.length &&
    a.widths.every((w, i) => w === b.widths[i])
  )
}

// The tabs that stay in the row, as indexes into items; null when everything fits.
// An active tab that would not fit takes the last slot before More.
export function visibleTabs(sizes: Sizes, active: number, count: number): number[] | null {
  const { avail, gap, widths } = sizes
  const tabs = widths.slice(0, count)
  const more = widths[count]
  const total = tabs.reduce((sum, w) => sum + w, 0) + gap * (count - 1)
  if (total <= avail) return null
  for (let k = count - 1; k >= 1; k--) {
    const idx = Array.from({ length: k }, (_, i) => i)
    if (active >= k) idx[k - 1] = active
    const width = idx.reduce((sum, i) => sum + tabs[i], 0) + gap * k + more
    if (width <= avail) return idx
  }
  return [Math.max(active, 0)]
}

function Count({ n }: { n: number | string }) {
  const { i18n } = useLingui()
  return (
    <span className="text-caption font-normal text-faint tabular-nums">
      {typeof n === "number" ? new Intl.NumberFormat(i18n.locale).format(n) : n}
    </span>
  )
}

function TabItem({
  tab,
  current,
  probe,
  tabClass,
}: {
  tab: OverflowTab
  current: boolean
  probe?: boolean
  tabClass: string
}) {
  const inner = (
    <>
      {tab.label}
      {tab.count !== undefined && <Count n={tab.count} />}
    </>
  )
  const common = {
    "aria-current": current ? ("page" as const) : undefined,
    className: tabClass,
    title: probe ? undefined : tab.title,
    tabIndex: probe ? -1 : undefined,
    "data-testid": probe ? undefined : tab.testId,
  }
  if (tab.to !== undefined)
    return (
      <Link to={tab.to} onClick={tab.onSelect} {...common}>
        {inner}
      </Link>
    )
  return (
    <button type="button" aria-pressed={current} onClick={tab.onSelect} {...common}>
      {inner}
    </button>
  )
}

export function OverflowTabs({
  items,
  current,
  label,
  moreTestId,
  testId = "page-tabs",
  tabClass = pageTabClass,
  gap = "gap-5",
  className,
}: {
  items: readonly OverflowTab[]
  current: string | null
  label: string
  moreTestId?: string
  testId?: string
  tabClass?: string
  gap?: string
  className?: string
}) {
  const { t } = useLingui()
  const box = useRef<HTMLDivElement>(null)
  const probe = useRef<HTMLUListElement>(null)
  const [sizes, setSizes] = useState<Sizes | null>(null)

  const measure = useCallback(() => {
    if (!box.current || !probe.current) return
    const cells = Array.from(probe.current.children) as HTMLElement[]
    const gap = parseFloat(getComputedStyle(probe.current).columnGap) || 0
    const widths = cells.map((c) => c.getBoundingClientRect().width)
    const next = { avail: box.current.clientWidth, gap, widths }
    setSizes((prev) => (sameSizes(prev, next) ? prev : next))
  }, [])

  useLayoutEffect(measure)

  useEffect(() => {
    if (!box.current || !probe.current) return
    const observer = new ResizeObserver(measure)
    observer.observe(box.current)
    observer.observe(probe.current)
    return () => observer.disconnect()
  }, [measure])

  const active = items.findIndex((i) => i.key === current)
  const fits = sizes && sizes.widths.length === items.length + 1 ? sizes : null
  const shown = fits ? visibleTabs(fits, active, items.length) : null
  const row = shown ? shown.map((i) => items[i]) : items
  const rest = shown ? items.filter((_, i) => !shown.includes(i)) : []

  return (
    <nav aria-label={label} className={cn("-mx-1 px-1", className)} data-testid={testId}>
      <div ref={box} className="relative overflow-hidden">
        <ul className={cn("flex", gap)}>
          {row.map((tab) => (
            <li key={tab.key} className="shrink-0">
              <TabItem tab={tab} current={tab.key === current} tabClass={tabClass} />
            </li>
          ))}
          {rest.length > 0 && (
            <li className="shrink-0">
              <DropdownMenu>
                <DropdownMenuTrigger className={tabClass} data-testid={moreTestId}>
                  {t`More`}
                  <ChevronDownIcon className="size-3.5" aria-hidden />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="min-w-48">
                  {rest.map((tab) => (
                    <DropdownMenuItem
                      key={tab.key}
                      render={tab.to !== undefined ? <Link to={tab.to} /> : undefined}
                      onClick={tab.onSelect}
                      title={tab.title}
                      data-testid={tab.testId}
                      className="justify-between"
                    >
                      <span className="truncate">{tab.label}</span>
                      {tab.count !== undefined && <Count n={tab.count} />}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            </li>
          )}
        </ul>
        <ul
          ref={probe}
          aria-hidden
          inert
          className={cn("pointer-events-none invisible absolute top-0 left-0 flex w-max", gap)}
        >
          {items.map((tab) => (
            <li key={tab.key} className="shrink-0">
              <TabItem tab={tab} current={tab.key === current} tabClass={tabClass} probe />
            </li>
          ))}
          <li className="shrink-0">
            <span className={tabClass}>
              {t`More`}
              <ChevronDownIcon className="size-3.5" />
            </span>
          </li>
        </ul>
      </div>
    </nav>
  )
}

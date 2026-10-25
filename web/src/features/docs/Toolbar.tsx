import { Plural, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon, GlobeIcon } from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { hostOf, Segmented } from "@/components/common"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { SORTS, useDocsDays, useDocsSite, useDocsSites, useDocsSort, WINDOWS } from "@/features/docs/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look } from "@/features/inbox/PageFrame"
import type { DocsPageSort } from "@/lib/api"
import { cn } from "@/lib/utils"

export function usePickedSite(inboxId: string) {
  const [days] = useDocsDays()
  const [site] = useDocsSite()
  const sites = useDocsSites(inboxId, days).data?.items
  return site && sites && !sites.some((s) => s.origin === site) ? "" : site
}

const ALL_SITES = "all"

export function DocsSitePicker({ className }: { className?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const [, setSite] = useDocsSite()
  const site = usePickedSite(inboxId)
  const all = useDocsSites(inboxId, days).data?.items ?? []
  const sites = all.filter((s, i) => all.findIndex((x) => x.origin === s.origin) === i)
  if (sites.length === 0) return null
  const nameOf = (origin: string) => all.find((x) => x.origin === origin && x.channel_name)?.channel_name
  const items: Record<string, string> = {
    [ALL_SITES]: t`All sites`,
    ...Object.fromEntries(sites.map((s) => [s.origin, nameOf(s.origin) || hostOf(s.origin)])),
  }
  return (
    <Select
      value={site || ALL_SITES}
      items={items}
      onValueChange={(v) => {
        const next = !v || v === ALL_SITES ? "" : String(v)
        setSite(next)
        if (pathname === "/docs/page") navigate("/docs")
      }}
    >
      <SelectTrigger
        aria-label={t`Site`}
        className={cn("h-8 w-auto max-w-64 rounded-lg bg-card", className)}
        data-testid="docs-site-picker"
      >
        <GlobeIcon className="size-4 text-faint" />
        <SelectValue className="min-w-0 flex-1 truncate text-start" />
      </SelectTrigger>
      <SelectContent className="min-w-64">
        <SelectItem value={ALL_SITES}>{t`All sites`}</SelectItem>
        {sites.map((s) => {
          const name = nameOf(s.origin)
          return (
            <SelectItem key={s.origin} value={s.origin} data-testid="docs-site-option">
              <span className="flex min-w-0 flex-col">
                <span className="truncate">{name || hostOf(s.origin)}</span>
                {name && <small className="truncate text-caption text-faint">{hostOf(s.origin)}</small>}
              </span>
            </SelectItem>
          )
        })}
      </SelectContent>
    </Select>
  )
}

export function DocsWindow({ className }: { className?: string }) {
  const { t } = useLingui()
  const [days, setDays] = useDocsDays()
  return (
    <Segmented
      value={String(days)}
      onChange={(v) => setDays(Number(v) as (typeof WINDOWS)[number])}
      label={t`Ratings over`}
      className={className}
      items={WINDOWS.map((d) => ({
        value: String(d),
        label: <Plural value={d} one="# day" other="# days" />,
        testId: `docs-days-${d}`,
      }))}
    />
  )
}

export function DocsSort() {
  const { t } = useLingui()
  const [sort, setSort] = useDocsSort()
  const labels: Record<DocsPageSort, string> = {
    down: t`Most disliked`,
    up: t`Most liked`,
    helpful: t`Most helpful`,
    activity: t`Most active`,
  }
  const hints: Record<DocsPageSort, string> = {
    down: t`Most “not helpful” ratings first`,
    up: t`Most “helpful” ratings first`,
    helpful: t`Highest share of “helpful” first, among pages with at least 5 ratings`,
    activity: t`Most ratings, open feedback, open discussions and waiting posts first`,
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<button type="button" className={look.listChip} />}
        aria-label={t`Sort`}
        data-testid="docs-sort"
      >
        <span className="truncate">{labels[sort]}</span>
        <ChevronDownIcon className="size-3.5 shrink-0 text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuRadioGroup value={sort} onValueChange={(v) => setSort(v as DocsPageSort)}>
          {SORTS.map((k) => (
            <DropdownMenuRadioItem key={k} value={k} data-testid={`docs-sort-${k}`}>
              <span className="flex min-w-0 flex-col leading-[1.3]">
                <span>{labels[k]}</span>
                <span className="text-caption text-faint">{hints[k]}</span>
              </span>
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

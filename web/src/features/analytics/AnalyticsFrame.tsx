import { Trans, useLingui } from "@lingui/react/macro"
import { ChartLineIcon, CheckIcon, ChevronDownIcon, PlusIcon } from "lucide-react"
import { useNavigate, useParams, useSearchParams } from "react-router"

import { Dot, PageLayout } from "@/components/common"
import { filterButton } from "@/components/common/ListBox"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useViewSearch } from "@/features/analytics/state"
import { useAnalyticsText } from "@/features/analytics/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import type { AnalyticsProperty } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export const VIEWS = [
  "overview",
  "realtime",
  "pages",
  "sources",
  "locations",
  "devices",
  "events",
  "goals",
  "funnels",
  "retention",
  "support",
  "settings",
] as const
export type ViewKey = (typeof VIEWS)[number]

export function useViewNames(): Record<ViewKey, string> {
  const { t } = useLingui()
  const text = useAnalyticsText()
  return {
    overview: t`Overview`,
    realtime: t`Realtime`,
    pages: text.title.pages,
    sources: t`Sources`,
    locations: t`Locations`,
    devices: t`Devices`,
    events: t`Events`,
    goals: t`Goals`,
    funnels: t`Funnels`,
    retention: t`Retention`,
    support: t`Support`,
    settings: t`Settings`,
  }
}

export function useViewLedes(): Record<ViewKey, string> {
  const { t } = useLingui()
  return {
    overview: t`Cookieless visitors across your sites and apps`,
    realtime: t`Who is here in the last 30 minutes.`,
    pages: t`Which pages people visit, and which lead to questions`,
    sources: t`Search, links, campaigns and the app stores.`,
    locations: t`Countries and cities, without storing any address.`,
    devices: t`Phones, computers, systems and browsers.`,
    events: t`What people did on your site and in your apps.`,
    goals: t`Visits and events that count as success`,
    funnels: t`Where people drop off on the way to a goal.`,
    retention: t`How many come back in the weeks after their first visit.`,
    support: t`Which visits end in a question to your team.`,
    settings: t`The script, alerts, sharing and what is kept.`,
  }
}

export function viewLink(id: string, v: ViewKey, search = "") {
  return `/analytics/${id}${v === "overview" ? "" : `/${v}`}${search}`
}

export function useScopedProperties() {
  const [inboxId] = useInboxFilter()
  const list = useAnalyticsProperties()
  const items = (list.data?.items ?? []).filter((p) => !inboxId || p.inbox_id === inboxId)
  return { list, items }
}

export function PropertyPicker({ property, current }: { property?: AnalyticsProperty; current: ViewKey }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const search = useViewSearch(true)
  const { items } = useScopedProperties()
  const inboxes = useInboxes().data ?? []
  const colors = new Map(inboxes.map((i) => [i.id, i.branding.color]))
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={cn(filterButton, "max-w-64 bg-card")} />}
        aria-label={t`Website or app`}
        data-testid="analytics-property-picker"
      >
        <ChartLineIcon className="text-muted-foreground" />
        <span className="truncate font-medium" data-testid="analytics-property-name">
          {property?.name ?? t`All sites and apps`}
        </span>
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Sites and apps</Trans>
          </DropdownMenuLabel>
          {items.length > 1 && (
            <DropdownMenuItem onClick={() => navigate("/analytics")} data-testid="analytics-nav-all">
              <Trans>All sites and apps</Trans>
            </DropdownMenuItem>
          )}
          {items.map((o) => (
            <DropdownMenuItem
              key={o.id}
              onClick={() =>
                navigate(
                  viewLink(o.id, current === "settings" ? "overview" : current, o.id === property?.id ? search : "")
                )
              }
              aria-current={o.id === property?.id ? "page" : undefined}
              className="aria-[current=page]:font-medium"
              data-testid="analytics-nav-property"
            >
              {inboxes.length > 1 && o.inbox_id && (
                <Dot color={colors.get(o.inbox_id)} className="size-[7px] shrink-0 rounded-[2px]" />
              )}
              <span className="min-w-0 flex-1 truncate">{o.name}</span>
              {o.id === property?.id && <CheckIcon className="text-brand" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        {canManage && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => navigate("/analytics/new")} data-testid="analytics-add">
              <PlusIcon />
              <Trans>Add a website or app</Trans>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AnalyticsLayout({ testId, children }: { testId?: string; children: React.ReactNode }) {
  const { t } = useLingui()
  return (
    <PageLayout label={t`Analytics`} testId={testId}>
      {children}
    </PageLayout>
  )
}

export function useDetailBack(current: ViewKey) {
  const names = useViewNames()
  const { propertyId = "" } = useParams()
  const [params] = useSearchParams()
  const key = current === "goals" ? "goal" : current === "funnels" ? "funnel" : ""
  const id = key ? params.get(key) : null
  if (!key || !id || id === "new") return undefined
  const next = new URLSearchParams(params)
  next.delete(key)
  const s = next.toString()
  return { label: names[current], to: viewLink(propertyId, current, s ? `?${s}` : "") }
}

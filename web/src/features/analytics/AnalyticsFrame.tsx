import { Trans, useLingui } from "@lingui/react/macro"
import { useParams, useSearchParams } from "react-router"

import { InfoSection, KV, NavGroup, SectionPage, TopSep, type NavItem } from "@/components/common"
import { formatDate } from "@/components/common/text"
import { filterLabel } from "@/features/analytics/Controls"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useRealtime } from "@/features/analytics/queries"
import { useView, useViewSearch } from "@/features/analytics/state"
import { useAnalyticsText } from "@/features/analytics/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import type { AnalyticsProperty } from "@/lib/api"
import { useSession } from "@/lib/session"
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

const GROUPS: { key: "traffic" | "behaviour" | "live"; views: ViewKey[] }[] = [
  { key: "traffic", views: ["overview", "pages", "sources", "locations", "devices"] },
  { key: "behaviour", views: ["events", "goals", "funnels", "retention", "support"] },
  { key: "live", views: ["realtime"] },
]

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

function AnalyticsNav({ property, current }: { property?: AnalyticsProperty; current?: ViewKey }) {
  const { t } = useLingui()
  const { canManage } = useSession()
  const names = useViewNames()
  const search = useViewSearch(true)
  const { items } = useScopedProperties()
  const live = useRealtime(property?.id ?? "", !!property).data?.visitors
  const target = property ?? items[0]
  const titles = { traffic: t`Traffic`, behaviour: t`Behaviour`, live: t`Live` }
  const keep = property ? search : ""
  const views = (vs: ViewKey[]): NavItem[] =>
    vs.map((v) => ({
      key: v,
      label: names[v],
      to: target ? viewLink(target.id, v, keep) : "/analytics",
      count: v === "realtime" ? live : undefined,
      hot: v === "realtime",
      current: !!property && current === v,
      testId: `analytics-nav-${v}`,
    }))
  const sites: NavItem[] = [
    ...(items.length > 1
      ? [{ key: "all", label: t`All sites`, to: "/analytics", current: !property, testId: "analytics-nav-all" }]
      : []),
    ...items.flatMap((p): NavItem[] => {
      const row: NavItem = {
        key: p.id,
        label: p.name,
        to: viewLink(
          p.id,
          current && current !== "settings" ? current : "overview",
          p.id === property?.id ? search : ""
        ),
        count: p.visitors_today,
        current: p.id === property?.id && current !== "settings",
        testId: "analytics-nav-property",
      }
      if (p.id !== property?.id) return [row]
      return [
        row,
        {
          key: `${p.id}-settings`,
          label: names.settings,
          to: viewLink(p.id, "settings"),
          current: current === "settings",
          sub: true,
          testId: "analytics-nav-settings",
        },
      ]
    }),
    ...(canManage
      ? [{ key: "add", label: t`Add a website`, to: "/analytics/new", testId: "analytics-add" } satisfies NavItem]
      : []),
  ]
  return (
    <>
      {target && GROUPS.map((g) => <NavGroup key={g.key} title={titles[g.key]} items={views(g.views)} />)}
      <NavGroup title={t`Sites and apps`} items={sites} />
    </>
  )
}

function useRangeText() {
  const { t, i18n } = useLingui()
  const text = useAnalyticsText()
  const { view } = useView()
  const r = view.range
  const range = r.period
    ? text.period[r.period]
    : `${formatDate(`${r.from}T12:00:00Z`, i18n.locale)} – ${formatDate(`${r.to}T12:00:00Z`, i18n.locale)}`
  const c = view.compare
  const compare = !c
    ? t`No comparison`
    : c.with === "previous"
      ? t`Compare with the period before`
      : c.with === "year"
        ? t`Compare with a year earlier`
        : t`Compare with ${formatDate(`${c.from}T12:00:00Z`, i18n.locale)} – ${formatDate(`${c.to}T12:00:00Z`, i18n.locale)}`
  return { range, compare }
}

function AnalyticsAside({ property }: { property?: AnalyticsProperty }) {
  const { i18n } = useLingui()
  const text = useAnalyticsText()
  const { view } = useView()
  const search = useViewSearch(true)
  const live = useRealtime(property?.id ?? "", !!property)
  const inbox = (useInboxes().data ?? []).find((i) => i.id === property?.inbox_id)
  const { range, compare } = useRangeText()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const privacy = (
    <InfoSection title={<Trans>Privacy</Trans>}>
      <p className="m-0 text-small text-fg2">
        <Trans>
          Pages, sources, places and devices, without cookies, without storing IP addresses and without a consent banner
          where the law allows it.
        </Trans>
      </p>
    </InfoSection>
  )
  if (!property) return privacy
  return (
    <>
      <InfoSection title={<Trans>Right now</Trans>} testId="analytics-aside-live">
        <KV
          label={<Trans>Visitors now</Trans>}
          value={live.data ? fmt.format(live.data.visitors) : "–"}
          tone={live.data?.visitors ? "ok" : undefined}
          to={viewLink(property.id, "realtime", search)}
        />
      </InfoSection>
      <InfoSection title={<Trans>Period</Trans>} testId="analytics-aside-period">
        <KV label={<Trans>Range</Trans>} value={range} />
        <KV label={<Trans>Comparison</Trans>} value={compare} />
      </InfoSection>
      {view.filters.length > 0 && (
        <InfoSection title={<Trans>Filters</Trans>} testId="analytics-aside-filters">
          <ul className="m-0 flex list-none flex-col gap-1 p-0 text-small leading-5 text-fg2">
            {view.filters.map((f, i) => (
              <li key={`${f.dimension}${i}`} className="[overflow-wrap:anywhere]" dir="auto">
                {filterLabel(text, f, i18n.locale)}
              </li>
            ))}
          </ul>
        </InfoSection>
      )}
      <InfoSection title={property.name}>
        <KV label={<Trans>Domains</Trans>} value={<span dir="ltr">{property.domains.join(", ") || "–"}</span>} />
        <KV label={<Trans>Time zone</Trans>} value={property.timezone} />
        {inbox && <KV label={<Trans>Product</Trans>} value={inbox.name} />}
      </InfoSection>
      {privacy}
    </>
  )
}

export function AnalyticsTrail({ property, current }: { property?: AnalyticsProperty; current?: ViewKey }) {
  const { t } = useLingui()
  const names = useViewNames()
  return (
    <>
      <span className="font-medium text-foreground">{t`Analytics`}</span>
      {current && (
        <>
          <TopSep />
          <span className="truncate">{names[current]}</span>
        </>
      )}
      {property && (
        <>
          <TopSep />
          <span className="truncate normal-case" data-testid="analytics-property-name">
            {property.name}
          </span>
        </>
      )}
    </>
  )
}

export function AnalyticsLayout({
  property,
  current,
  testId,
  children,
}: {
  property?: AnalyticsProperty
  current?: ViewKey
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <SectionPage
      title={t`Analytics`}
      nav={<AnalyticsNav property={property} current={current} />}
      aside={<AnalyticsAside property={property} />}
      testId={testId}
    >
      {children}
    </SectionPage>
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

import { useLingui } from "@lingui/react/macro"
import { useSearchParams } from "react-router"

import { NavGroup, SectionPage, TopSep, type NavItem } from "@/components/common"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { usePagesMonitors, statusLink } from "@/features/status/queries"
import { useScopeIncidents, useStatusScope } from "@/features/status/scope"
import { isClosed } from "@/features/status/text"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export type StatusPlace = "overview" | "components" | "subscribers" | "settings" | "pages" | "incidents" | "monitors"
export type IncidentView = "active" | "scheduled" | "past"

export function incidentsTo(v: IncidentView) {
  return v === "active" ? "/incidents" : `/incidents?view=${v}`
}

export function useStatusCounts() {
  const { items } = useStatusScope()
  const incidents = useScopeIncidents(items)
  const monitors = usePagesMonitors(items.map((p) => p.id))
  const live = incidents.data.filter((i) => !isClosed(i))
  return {
    pages: items.length,
    active: incidents.isPending ? undefined : live.filter((i) => i.state !== "scheduled").length,
    scheduled: incidents.isPending ? undefined : live.filter((i) => i.state === "scheduled").length,
    monitors: monitors.isPending ? undefined : monitors.data.length,
    down: monitors.data.filter((m) => m.state === "down" && !m.paused).length,
  }
}

export function useStatusNames(): Record<StatusPlace | IncidentView, string> {
  const { t } = useLingui()
  return {
    overview: t`Overview`,
    components: t`Components`,
    subscribers: t`Subscribers`,
    settings: t`Settings`,
    pages: t`Status pages`,
    incidents: t`Incidents`,
    monitors: t`Monitors`,
    active: t`Active`,
    scheduled: t`Scheduled`,
    past: t`Past`,
  }
}

function StatusNav({ place, view }: { place: StatusPlace; view?: IncidentView }) {
  const { canManage } = useSession()
  const [params] = useSearchParams()
  const { items } = useStatusScope()
  const counts = useStatusCounts()
  const names = useStatusNames()
  const page = items.find((p) => p.id === params.get("page")) ?? items[0]
  if (place === "monitors") return null
  if (place === "incidents") {
    const shown = view ?? "active"
    return (
      <NavGroup
        title={names.incidents}
        items={(["active", "scheduled", "past"] as const).map((v) => ({
          key: v,
          label: names[v],
          to: incidentsTo(v),
          count: v === "past" ? undefined : counts[v],
          current: shown === v,
          testId: `status-nav-incidents-${v}`,
        }))}
      />
    )
  }
  const pageViews: NavItem[] = page
    ? (["overview", "components", "subscribers", ...(canManage ? (["settings"] as const) : [])] as const).map((v) => ({
        key: v,
        label: names[v],
        to: statusLink(page.id, v === "overview" ? undefined : v),
        current: place === v,
        testId: `status-nav-${v}`,
      }))
    : []
  return (
    <>
      {page && <NavGroup title={page.name} items={pageViews} />}
      <NavGroup
        items={[
          {
            key: "pages",
            label: names.pages,
            to: "/status?tab=pages",
            count: counts.pages,
            current: place === "pages",
            testId: "status-nav-pages",
          },
        ]}
      />
    </>
  )
}

export function StatusLayout({
  place,
  view,
  aside,
  testId,
  children,
}: {
  place: StatusPlace
  view?: IncidentView
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <SectionPage title={t`Status`} nav={<StatusNav place={place} view={view} />} aside={aside} testId={testId}>
      {children}
    </SectionPage>
  )
}

export function StatusTrail({ place, view, name }: { place: StatusPlace; view?: IncidentView; name?: string }) {
  const { t } = useLingui()
  const names = useStatusNames()
  const [inboxId] = useInboxFilter()
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  return (
    <>
      <span className="font-medium text-foreground">{t`Status`}</span>
      <TopSep />
      <span className="truncate">{names[place]}</span>
      {view && (
        <>
          <TopSep />
          <span className="truncate">{names[view]}</span>
        </>
      )}
      <TopSep />
      <span className="truncate normal-case">{name ?? product ?? t`All products`}</span>
    </>
  )
}

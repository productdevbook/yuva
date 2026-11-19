import { usePagesMonitors } from "@/features/status/queries"
import { useScopeIncidents, useStatusScope } from "@/features/status/scope"
import { isClosed } from "@/features/status/text"

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

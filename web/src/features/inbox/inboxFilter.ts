import { useCallback, useSyncExternalStore } from "react"

import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

function filterKey(ws: string) {
  return `inbox-filter:${ws}`
}

const listeners = new Set<() => void>()

function readFilter(ws: string): string {
  try {
    return localStorage.getItem(filterKey(ws)) ?? ""
  } catch {
    return ""
  }
}

export function useInboxFilter() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data
  const stored = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => readFilter(ws),
  )
  const value = stored && inboxes && !inboxes.some((i) => i.id === stored) ? "" : stored
  const set = useCallback(
    (id: string) => {
      try {
        if (id) localStorage.setItem(filterKey(ws), id)
        else localStorage.removeItem(filterKey(ws))
      } catch {
        // Storage can be blocked; the filter then lasts until the next render only.
      }
      listeners.forEach((l) => l())
    },
    [ws],
  )
  return [value, set] as const
}

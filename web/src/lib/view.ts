import { useCallback, useSyncExternalStore } from "react"

import { useSession } from "@/lib/session"

export type View = "queue" | "list"

const listeners = new Set<() => void>()
const chosen = new Map<string, View>()

function read(key: string): View {
  const v = chosen.get(key)
  if (v) return v
  try {
    return localStorage.getItem(key) === "list" ? "list" : "queue"
  } catch {
    return "queue"
  }
}

export function useView() {
  const { membership } = useSession()
  const key = `view:${membership.member_id}`
  const view = useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => read(key),
  )
  const set = useCallback(
    (v: View) => {
      chosen.set(key, v)
      try {
        localStorage.setItem(key, v)
      } catch {
        // Storage can be blocked; the choice then lasts for this page only.
      }
      listeners.forEach((l) => l())
    },
    [key],
  )
  return [view, set] as const
}

import { useCallback, useSyncExternalStore } from "react"

import { useSession } from "@/lib/session"

export type View = "queue" | "list"

const listeners = new Set<() => void>()

function read(key: string): View {
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
      try {
        localStorage.setItem(key, v)
      } catch {
        // Storage can be blocked; the choice then lasts until the next render only.
      }
      listeners.forEach((l) => l())
    },
    [key],
  )
  return [view, set] as const
}

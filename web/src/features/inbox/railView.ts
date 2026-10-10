import { useSyncExternalStore } from "react"

export type RailView = "conversations" | "mentions" | "assistants"

let view: RailView = "conversations"
const listeners = new Set<() => void>()

export function setRailView(next: RailView) {
  view = next
  listeners.forEach((l) => l())
}

export function useRailView() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => view,
  )
}

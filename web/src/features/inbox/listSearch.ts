import { useSyncExternalStore } from "react"

let state = { query: "", focus: 0 }
const listeners = new Set<() => void>()

function set(next: typeof state) {
  state = next
  listeners.forEach((l) => l())
}

export function setListQuery(query: string) {
  set({ ...state, query })
}

export function focusListSearch() {
  set({ ...state, focus: state.focus + 1 })
}

export function useListSearch() {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}

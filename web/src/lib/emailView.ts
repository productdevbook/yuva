import { useSyncExternalStore } from "react"

export type EmailView = "original" | "reading"

const KEY = "email-view"
const listeners = new Set<() => void>()
let chosen: EmailView | undefined

function read(): EmailView {
  if (chosen) return chosen
  try {
    return localStorage.getItem(KEY) === "reading" ? "reading" : "original"
  } catch {
    return "original"
  }
}

export function setEmailView(v: EmailView) {
  chosen = v
  try {
    if (v === "original") localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, v)
  } catch {
    // Storage can be blocked; the choice then lasts for this page only.
  }
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  const storage = (e: StorageEvent) => {
    if (e.key !== KEY) return
    chosen = undefined
    l()
  }
  window.addEventListener("storage", storage)
  return () => {
    listeners.delete(l)
    window.removeEventListener("storage", storage)
  }
}

export function useEmailView(): EmailView {
  return useSyncExternalStore(subscribe, read)
}

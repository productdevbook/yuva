import { useSyncExternalStore } from "react"

export type Theme = "system" | "light" | "dark"

const KEY = "theme"
const listeners = new Set<() => void>()
const media = window.matchMedia("(prefers-color-scheme: dark)")

function read(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    return v === "light" || v === "dark" ? v : "system"
  } catch {
    return "system"
  }
}

function apply() {
  const t = read()
  const dark = t === "dark" || (t === "system" && media.matches)
  document.documentElement.classList.toggle("dark", dark)
  for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) meta.content = dark ? "#0a0a0c" : "#fbfaf9"
}

export function setTheme(t: Theme) {
  try {
    if (t === "system") localStorage.removeItem(KEY)
    else localStorage.setItem(KEY, t)
  } catch {
    // Storage can be blocked; the theme then lasts for this page only.
  }
  apply()
  listeners.forEach((l) => l())
}

export function startTheme() {
  apply()
  media.addEventListener("change", apply)
  window.addEventListener("storage", (e) => {
    if (e.key !== KEY) return
    apply()
    listeners.forEach((l) => l())
  })
}

export function useTheme(): Theme {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    read,
  )
}

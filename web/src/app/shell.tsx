import { createContext, useContext } from "react"

export type Shell = {
  openPalette: () => void
  openShortcuts: () => void
}

export const ShellContext = createContext<Shell>({ openPalette: () => {}, openShortcuts: () => {} })

export function useShell() {
  return useContext(ShellContext)
}

export function isChats(pathname: string) {
  return pathname === "/" || pathname.startsWith("/conversations/")
}

export type Section = "conversations" | "mentions" | "assistants" | "contacts" | "team" | "reports" | "docs" | "settings"

const routed = ["mentions", "contacts", "team", "reports", "docs", "settings"] as const

export function sectionOf(pathname: string, view: Section): Section {
  const routedSection = routed.find((s) => pathname === `/${s}` || pathname.startsWith(`/${s}/`))
  if (routedSection) return routedSection
  return pathname === "/" && view === "mentions" ? "conversations" : view
}

export function isSectionHome(pathname: string) {
  return pathname === "/" || routed.some((s) => pathname === `/${s}`)
}

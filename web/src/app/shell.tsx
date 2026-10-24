import { createContext, useContext } from "react"

export type Shell = {
  openPalette: () => void
  openShortcuts: () => void
}

export const ShellContext = createContext<Shell>({ openPalette: () => {}, openShortcuts: () => {} })

export function useShell() {
  return useContext(ShellContext)
}

export function inboxRoot(manager: boolean) {
  return manager ? "/conversations" : "/"
}

export function isInboxRoot(pathname: string, manager: boolean) {
  return pathname === "/conversations" || (!manager && pathname === "/")
}

export function isChats(pathname: string, manager: boolean) {
  return isInboxRoot(pathname, manager) || pathname.startsWith("/conversations/")
}

export type Section =
  | "home"
  | "conversations"
  | "mail"
  | "mentions"
  | "feedback"
  | "assistants"
  | "contacts"
  | "team"
  | "reports"
  | "docs"
  | "broadcasts"
  | "status"
  | "knowledge"
  | "board"
  | "updates"
  | "analytics"
  | "settings"

const routed = [
  "mail",
  "mentions",
  "feedback",
  "contacts",
  "team",
  "reports",
  "docs",
  "broadcasts",
  "status",
  "knowledge",
  "board",
  "updates",
  "analytics",
  "settings",
] as const

export function sectionOf(pathname: string, view: Section, manager: boolean): Section {
  if (manager && pathname === "/") return "home"
  // The server answers /status/{slug} with the public page, so the panel keeps a status page under /status-pages.
  if (pathname.startsWith("/status-pages/") || pathname.startsWith("/incidents") || pathname.startsWith("/monitors"))
    return "status"
  const routedSection = routed.find((s) => pathname === `/${s}` || pathname.startsWith(`/${s}/`))
  if (routedSection) return routedSection
  return isInboxRoot(pathname, manager) && view === "mentions" ? "conversations" : view
}

const fullPage: Section[] = ["team", "reports", "docs", "broadcasts", "status", "updates", "analytics", "settings"]

export function isFullPage(section: Section) {
  return fullPage.includes(section)
}

export function isSectionHome(pathname: string) {
  return (
    pathname === "/" ||
    pathname === "/conversations" ||
    pathname === "/broadcasts/lists" ||
    routed.some((s) => pathname === `/${s}`)
  )
}

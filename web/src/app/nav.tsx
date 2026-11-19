import { useLingui } from "@lingui/react/macro"
import { useLocation, useNavigate } from "react-router"

import { inboxRoot, isChats, isFullPage, sectionOf, type Section } from "@/app/shell"
import { keyLabel, mod, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useAnalyticsVisible } from "@/features/analytics/properties"
import { useConversations, useCounts } from "@/features/inbox/queries"
import { setRailView, useRailView } from "@/features/inbox/railView"
import { useMailInboxes } from "@/features/mail/folders"
import { useMailUnread } from "@/features/mail/queries"
import { useSession } from "@/lib/session"

export type PageKey =
  | "home"
  | "inbox"
  | "mail"
  | "people"
  | "broadcasts"
  | "analytics"
  | "feedback"
  | "help"
  | "status"
  | "reports"
  | "team"
  | "settings"
  | "imports"

export type PageEntry = {
  key: PageKey
  label: string
  keys: string[]
  open: () => void
  count?: number
  countLabel?: string
}

export type PageGroup = { key: string; title: string; entries: PageEntry[] }

export function useSection() {
  const { apart, canManage } = useSession()
  return sectionOf(useLocation().pathname, useRailView(), apart, canManage)
}

export function useGo() {
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const section = useSection()
  const { apart, homePath } = useSession()
  return (key: Section) => {
    if (key === "home") {
      if (pathname !== homePath) navigate(homePath)
    } else if (key === "mentions" || key === "assistants") {
      setRailView(key)
      if (pathname !== `/${key}`) navigate(`/${key}`)
    } else if (key === "conversations") {
      setRailView(key)
      if (!isChats(pathname, apart) || search) navigate(inboxRoot(apart))
    } else if (section !== key || (isFullPage(key) && pathname !== `/${key}`)) {
      navigate(`/${key}`)
    }
  }
}

export function isImports(pathname: string) {
  return pathname === "/settings/migrations" || pathname.startsWith("/migrations/")
}

export function pageOf(section: Section, pathname: string): PageKey {
  if (isImports(pathname)) return "imports"
  if (pathname === "/settings/teams") return "team"
  switch (section) {
    case "home":
      return "home"
    case "mail":
      return "mail"
    case "contacts":
      return "people"
    case "broadcasts":
      return "broadcasts"
    case "analytics":
      return "analytics"
    case "feedback":
    case "board":
      return "feedback"
    case "knowledge":
    case "docs":
    case "updates":
      return "help"
    case "status":
      return "status"
    case "reports":
      return "reports"
    case "team":
      return "team"
    case "settings":
      return "settings"
    default:
      return "inbox"
  }
}

export function usePage(): PageKey {
  return pageOf(useSection(), useLocation().pathname)
}

export function useUnreadCount() {
  const list = useConversations({ kind: ["conversation"], purpose: "support" })
  return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => c.unread).length
}

export function usePageGroups(): PageGroup[] {
  const { t } = useLingui()
  const go = useGo()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const unread = useUnreadCount()
  const feedback = Number(useCounts().data?.feedback ?? 0)
  const analytics = useAnalyticsVisible()
  const mail = useMailInboxes().length > 0
  const mailUnread = useMailUnread()
  const entry = (key: PageKey, label: string, shortcut: string, section: Section): PageEntry => ({
    key,
    label,
    keys: keyLabel(shortcut),
    open: () => go(section),
  })
  const work: PageEntry[] = [
    ...(canManage ? [entry("home", t`Home`, SHORTCUTS.home, "home")] : []),
    {
      ...entry("inbox", t`Inbox`, SHORTCUTS.conversations, "conversations"),
      count: unread,
      countLabel: t`Conversations, ${unread} unread`,
    },
    ...(mail
      ? [
          {
            ...entry("mail", t`Mail`, SHORTCUTS.mail, "mail"),
            count: mailUnread,
            countLabel: t`Mail, ${mailUnread} unread`,
          },
        ]
      : []),
    entry("people", t`People`, SHORTCUTS.contacts, "contacts"),
  ]
  const reach: PageEntry[] = [
    entry("broadcasts", t`Broadcasts`, SHORTCUTS.broadcasts, "broadcasts"),
    ...(analytics ? [entry("analytics", t`Analytics`, SHORTCUTS.analytics, "analytics")] : []),
    {
      ...entry("feedback", t`Feedback and roadmap`, SHORTCUTS.feedback, "feedback"),
      count: feedback,
      countLabel: t`Feedback, ${feedback} open`,
    },
    entry("help", t`Help center`, SHORTCUTS.helpCenter, "knowledge"),
    entry("status", t`Status`, SHORTCUTS.status, "status"),
  ]
  const workspace: PageEntry[] = [
    entry("reports", t`Reports`, SHORTCUTS.reports, "reports"),
    entry("team", t`Team`, SHORTCUTS.team, "team"),
    { key: "settings", label: t`Settings`, keys: [mod, ","], open: () => navigate("/settings") },
    ...(canManage
      ? [{ key: "imports" as const, label: t`Imports`, keys: [], open: () => navigate("/settings/migrations") }]
      : []),
  ]
  return [
    { key: "work", title: t`Work`, entries: work },
    { key: "reach", title: t`Reach`, entries: reach },
    { key: "workspace", title: t`Workspace`, entries: workspace },
  ]
}

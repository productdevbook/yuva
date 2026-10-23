import { useEffect, useMemo, useState } from "react"
import { Outlet, useLocation, useNavigate } from "react-router"

import { ConnectionBanner } from "@/app/ConnectionBanner"
import { Palette } from "@/app/Palette"
import { useGo } from "@/app/Rail"
import { inboxRoot, isChats, ShellContext } from "@/app/shell"
import { Toaster } from "@/components/common"
import { RequiredFieldsHost } from "@/features/fields/RequiredFieldsHost"
import { SHORTCUTS, ShortcutSheet } from "@/components/common/ShortcutSheet"
import { focusListSearch } from "@/features/inbox/listSearch"
import { useCounts } from "@/features/inbox/queries"
import { useAnalyticsVisible } from "@/features/analytics/properties"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useRegisterPushOnStart } from "@/lib/push"
import { useRealtime } from "@/lib/realtime"
import { useSession } from "@/lib/session"

function Title() {
  const counts = useCounts().data
  const n = counts ? counts.mine + counts.unassigned : 0
  useEffect(() => {
    document.title = n ? `(${n}) Yuva` : "Yuva"
  }, [n])
  return null
}

export function AppShell() {
  const { workspaceId, membership, canManage } = useSession()
  const navigate = useNavigate()
  const [shortcuts, setShortcuts] = useState(false)
  const [palette, setPalette] = useState(false)
  useRealtime(workspaceId, membership.member_id)
  useRegisterPushOnStart()

  const shell = useMemo(
    () => ({
      openPalette: () => setPalette(true),
      openShortcuts: () => setShortcuts(true),
    }),
    []
  )

  const { pathname } = useLocation()
  const chats = isChats(pathname, canManage)
  const go = useGo()
  const analytics = useAnalyticsVisible()
  useHotkeys({
    [SHORTCUTS.help]: () => setShortcuts(true),
    [SHORTCUTS.search]: () => {
      const contacts =
        pathname.startsWith("/contacts") && document.querySelector<HTMLInputElement>("[data-testid=contacts-search]")
      if (contacts) return contacts.focus()
      const subscribers =
        pathname.startsWith("/broadcasts/") &&
        document.querySelector<HTMLInputElement>("[data-testid=subscribers-search]")
      if (subscribers) return subscribers.focus()
      if (!chats && !pathname.startsWith("/feedback")) navigate(inboxRoot(canManage))
      focusListSearch()
    },
    [SHORTCUTS.home]: () => canManage && go("home"),
    [SHORTCUTS.contacts]: () => go("contacts"),
    [SHORTCUTS.conversations]: () => go("conversations"),
    [SHORTCUTS.team]: () => go("team"),
    [SHORTCUTS.reports]: () => go("reports"),
    [SHORTCUTS.docs]: () => go("docs"),
    [SHORTCUTS.broadcasts]: () => go("broadcasts"),
    [SHORTCUTS.status]: () => go("status"),
    [SHORTCUTS.helpCenter]: () => go("knowledge"),
    [SHORTCUTS.roadmap]: () => go("board"),
    [SHORTCUTS.updates]: () => go("updates"),
    [SHORTCUTS.analytics]: () => analytics && go("analytics"),
    [SHORTCUTS.assistants]: () => go("assistants"),
    [SHORTCUTS.mentions]: () => go("mentions"),
    [SHORTCUTS.feedback]: () => go("feedback"),
  })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey) return
      const key = e.key.toLowerCase()
      if (key === "k") {
        e.preventDefault()
        setPalette((p) => !p)
      } else if (key === ",") {
        e.preventDefault()
        navigate("/settings")
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [navigate])

  return (
    <ShellContext.Provider value={shell}>
      <Title />
      <div className="min-h-svh bg-background">
        <Outlet />
      </div>
      <Palette open={palette} onOpenChange={setPalette} />
      <ShortcutSheet open={shortcuts} onOpenChange={setShortcuts} home={canManage} />
      <RequiredFieldsHost />
      <Toaster />
      <ConnectionBanner />
    </ShellContext.Provider>
  )
}

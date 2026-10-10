import { useEffect, useMemo, useState } from "react"
import { Outlet, useLocation, useNavigate } from "react-router"

import { ConnectionBanner } from "@/app/ConnectionBanner"
import { Palette } from "@/app/Palette"
import { useGo } from "@/app/Rail"
import { isChats, ShellContext } from "@/app/shell"
import { Toaster } from "@/components/common"
import { SHORTCUTS, ShortcutSheet } from "@/components/common/ShortcutSheet"
import { focusListSearch } from "@/features/inbox/listSearch"
import { useCounts } from "@/features/inbox/queries"
import { useOpenSetup } from "@/features/setup/setup"
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
  const { workspaceId, membership } = useSession()
  const navigate = useNavigate()
  const [shortcuts, setShortcuts] = useState(false)
  const [palette, setPalette] = useState(false)
  useRealtime(workspaceId, membership.member_id)
  useRegisterPushOnStart()
  useOpenSetup()

  const shell = useMemo(
    () => ({
      openPalette: () => setPalette(true),
      openShortcuts: () => setShortcuts(true),
    }),
    [],
  )

  const { pathname } = useLocation()
  const chats = isChats(pathname)
  const go = useGo()
  useHotkeys({
    [SHORTCUTS.help]: () => setShortcuts(true),
    [SHORTCUTS.search]: () => {
      const contacts = pathname.startsWith("/contacts") && document.querySelector<HTMLInputElement>("[data-testid=contacts-search]")
      if (contacts) return contacts.focus()
      if (!chats) navigate("/")
      focusListSearch()
    },
    [SHORTCUTS.contacts]: () => go("contacts"),
    [SHORTCUTS.conversations]: () => go("conversations"),
    [SHORTCUTS.team]: () => go("team"),
    [SHORTCUTS.reports]: () => go("reports"),
    [SHORTCUTS.assistants]: () => go("assistants"),
    [SHORTCUTS.mentions]: () => go("mentions"),
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
      <ShortcutSheet open={shortcuts} onOpenChange={setShortcuts} />
      <Toaster />
      <ConnectionBanner />
    </ShellContext.Provider>
  )
}

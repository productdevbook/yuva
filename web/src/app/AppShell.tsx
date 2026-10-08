import { useEffect, useMemo, useState } from "react"
import { Outlet, useNavigate } from "react-router"

import { ConnectionBanner } from "@/app/ConnectionBanner"
import { Palette } from "@/app/Palette"
import { ShellContext } from "@/app/shell"
import { TopBar } from "@/app/TopBar"
import { Toaster } from "@/components/common"
import { SHORTCUTS, ShortcutSheet } from "@/components/common/ShortcutSheet"
import { AllDrawer } from "@/features/inbox/AllDrawer"
import { QueueProvider, useQueue } from "@/features/inbox/queue"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useRegisterPushOnStart } from "@/lib/push"
import { useRealtime } from "@/lib/realtime"
import { useSession } from "@/lib/session"

function Title() {
  const { waiting } = useQueue()
  const n = waiting.length
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
  const [drawer, setDrawer] = useState<{ open: boolean; q: string; focus: number }>({ open: false, q: "", focus: 0 })
  useRealtime(workspaceId, membership.member_id)
  useRegisterPushOnStart()

  const shell = useMemo(
    () => ({
      openDrawer: (q?: string) => setDrawer((d) => ({ open: true, q: q ?? d.q, focus: q === undefined ? d.focus : d.focus + 1 })),
      openPalette: () => setPalette(true),
      openShortcuts: () => setShortcuts(true),
    }),
    [],
  )

  useHotkeys({
    [SHORTCUTS.help]: () => setShortcuts(true),
    [SHORTCUTS.search]: () => setDrawer((d) => ({ open: true, q: d.q, focus: d.focus + 1 })),
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
    <QueueProvider>
      <ShellContext.Provider value={shell}>
        <Title />
        <div className="min-h-svh bg-background">
          <TopBar />
          <Outlet />
        </div>
        <AllDrawer
          open={drawer.open}
          onOpenChange={(open) => setDrawer((d) => ({ ...d, open }))}
          query={drawer.q}
          onQuery={(q) => setDrawer((d) => ({ ...d, q }))}
          focusKey={drawer.focus}
        />
        <Palette open={palette} onOpenChange={setPalette} />
        <ShortcutSheet open={shortcuts} onOpenChange={setShortcuts} />
        <Toaster />
        <ConnectionBanner />
      </ShellContext.Provider>
    </QueueProvider>
  )
}

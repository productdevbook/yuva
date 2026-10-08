import { useLingui } from "@lingui/react/macro"
import { useEffect, useMemo, useState } from "react"
import { Outlet, useLocation } from "react-router"

import { ConnectionBanner } from "@/app/ConnectionBanner"
import { ShellContext } from "@/app/shell"
import { Sidebar } from "@/app/Sidebar"
import { ShortcutSheet, useShortcutSheet } from "@/components/common/ShortcutSheet"
import { Sheet, SheetContent } from "@/components/ui/sheet"
import { useRegisterPushOnStart } from "@/lib/push"
import { useRealtime } from "@/lib/realtime"
import { useSession } from "@/lib/session"

export function AppShell() {
  const { t } = useLingui()
  const sheet = useShortcutSheet()
  const { workspaceId, membership } = useSession()
  const [navOpen, setNavOpen] = useState(false)
  const { pathname } = useLocation()
  useRealtime(workspaceId, membership.member_id)
  useRegisterPushOnStart()
  useEffect(() => setNavOpen(false), [pathname])
  const shell = useMemo(() => ({ openNav: () => setNavOpen(true) }), [])
  const sidebar = <Sidebar onShortcuts={() => sheet.setOpen(true)} />
  return (
    <ShellContext.Provider value={shell}>
      <div className="flex h-svh overflow-hidden bg-background">
        <aside className="hidden w-60 shrink-0 md:block">{sidebar}</aside>
        <main className="flex min-h-0 min-w-0 flex-1 flex-col">
          <Outlet />
        </main>
      </div>
      <Sheet open={navOpen} onOpenChange={setNavOpen}>
        <SheetContent side="left" className="w-[min(18rem,85vw)] border-0" aria-label={t`Navigation`} showCloseButton={false}>
          {sidebar}
        </SheetContent>
      </Sheet>
      <ShortcutSheet open={sheet.open} onOpenChange={sheet.setOpen} />
      <ConnectionBanner />
    </ShellContext.Provider>
  )
}

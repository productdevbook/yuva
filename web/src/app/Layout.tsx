import { useEffect } from "react"
import { Outlet, useLocation } from "react-router"

import { Header } from "@/app/Header"
import { useSection } from "@/app/nav"
import { SectionPlacesSidebar } from "@/app/places"
import { isChats, isInboxRoot, isSectionHome, type Section } from "@/app/shell"
import { SidebarProvider } from "@/components/ui/sidebar"
import { AssistantsPanel } from "@/features/inbox/AssistantsPage"
import { setRailView } from "@/features/inbox/railView"
import { MentionsPanel } from "@/features/inbox/SidePanels"
import { useIsPhone } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const COLUMNS: Section[] = ["mentions", "assistants"]

function PhoneColumn({ section }: { section: Section }) {
  if (section === "mentions") return <MentionsPanel />
  return <AssistantsPanel />
}

function useMentionsView(pathname: string, manager: boolean) {
  useEffect(() => {
    if (pathname === "/mentions") setRailView("mentions")
    else if (pathname === "/assistants") setRailView("assistants")
    else if (isInboxRoot(pathname, manager)) setRailView("conversations")
  }, [pathname, manager])
}

function isPhoneDetail(pathname: string, search: string) {
  return (
    (pathname === "/status" && new URLSearchParams(search).has("page")) ||
    /^\/(conversations|feedback|contacts)\/[^/]+/.test(pathname) ||
    /^\/broadcasts\/(?!(lists|designs|triggered)$)[^/]+$/.test(pathname) ||
    /^\/broadcasts\/lists\/[^/]+/.test(pathname) ||
    (pathname.startsWith("/mail/") && pathname.split("/").length > 4) ||
    /^\/board\/[^/]+\/ideas\/[^/]+/.test(pathname) ||
    /^\/knowledge\/[^/]+\/articles\/[^/]+/.test(pathname) ||
    /^\/incidents\/[^/]+\/[^/]+/.test(pathname) ||
    /^\/monitors\/[^/]+/.test(pathname) ||
    /^\/updates\/[^/]+\/entries\/[^/]+/.test(pathname) ||
    /^\/docs\/discussions\/(?!queue$)[^/]+/.test(pathname) ||
    pathname.startsWith("/migrations/") ||
    /^\/settings\/(inboxes|connected-apps|webhooks)\/[^/]+/.test(pathname)
  )
}

function Phone() {
  const { pathname, search } = useLocation()
  const section = useSection()
  const column = isSectionHome(pathname) && COLUMNS.includes(section)
  return (
    <SidebarProvider className="contents">
      <div className="flex h-svh min-h-0 flex-col" data-frame data-testid="shell">
        <Header />
        {!isPhoneDetail(pathname, search) && <SectionPlacesSidebar />}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          {column ? <PhoneColumn section={section} /> : <Outlet />}
        </div>
      </div>
    </SidebarProvider>
  )
}

const width = { "--sidebar-width": "232px" } as React.CSSProperties

export function Layout() {
  const phone = useIsPhone()
  const { pathname } = useLocation()
  const { apart } = useSession()
  const section = useSection()
  useMentionsView(pathname, apart)
  if (phone) return <Phone />
  const frame = isChats(pathname, apart) || section === "docs" || /^\/mail\/[^/]+\/filters$/.test(pathname)
  return (
    <SidebarProvider
      className={cn("flex-col", frame && "h-svh min-h-0 overflow-hidden")}
      style={width}
      data-frame={frame || undefined}
      data-testid="shell"
    >
      <Header />
      <div className="flex min-h-0 min-w-0 flex-1 max-[899px]:flex-col">
        <SectionPlacesSidebar />
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <Outlet />
        </div>
      </div>
    </SidebarProvider>
  )
}

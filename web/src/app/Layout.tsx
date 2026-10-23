import { lazy, Suspense, useCallback, useEffect, useState } from "react"
import { Outlet, useLocation, useMatch, useNavigate } from "react-router"

import { TabBar, useSection } from "@/app/Rail"
import { isChats, isInboxRoot, isSectionHome } from "@/app/shell"
import { AppSidebar } from "@/app/Sidebar"
import { TopBar } from "@/app/TopBar"
import { SidebarProvider, useSidebar } from "@/components/ui/sidebar"
import { ContactsColumn } from "@/features/contact/ContactsColumn"
import { FeedbackList } from "@/features/feedback/FeedbackList"
import { ChatList } from "@/features/inbox/ChatList"
import { setRailView, useRailView } from "@/features/inbox/railView"
import { AssistantsPanel, MentionsPanel } from "@/features/inbox/SidePanels"
import { ReportsColumn } from "@/features/reports/ReportsColumn"
import { SettingsColumn } from "@/features/settings/SettingsColumn"
import { useIsPhone } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const HelpColumn = lazy(() => import("@/features/help/HelpColumn"))
const AnalyticsColumn = lazy(() => import("@/features/analytics/AnalyticsColumn"))

function LeftColumn() {
  const section = useSection()
  const navigate = useNavigate()
  const { search } = useLocation()
  const currentId = useMatch("/conversations/:conversationId")?.params.conversationId ?? null
  const open = useCallback((id: string) => navigate(`/conversations/${id}${search}`), [navigate, search])
  const chats = section === "conversations"
  return (
    <>
      {chats && (
        <div className="flex min-h-0 flex-1 flex-col">
          <ChatList currentId={currentId} onOpen={open} active />
        </div>
      )}
      <SectionColumn />
    </>
  )
}

function SectionColumn() {
  const section = useSection()
  const navigate = useNavigate()
  const feedbackId = useMatch("/feedback/:conversationId")?.params.conversationId ?? null
  const openFeedback = useCallback((id: string) => navigate(`/feedback/${id}`), [navigate])
  return (
    <>
      {section === "feedback" && <FeedbackList currentId={feedbackId} onOpen={openFeedback} />}
      {section === "mentions" && <MentionsPanel />}
      {section === "assistants" && <AssistantsPanel />}
      {section === "contacts" && <ContactsColumn />}
      {section === "reports" && <ReportsColumn />}
      {section === "knowledge" && (
        <Suspense fallback={<div className="flex-1 bg-background" />}>
          <HelpColumn />
        </Suspense>
      )}
      {section === "analytics" && (
        <Suspense fallback={<div className="flex-1 bg-background" />}>
          <AnalyticsColumn />
        </Suspense>
      )}
      {section === "settings" && <SettingsColumn />}
    </>
  )
}

function useMentionsView(pathname: string, manager: boolean) {
  const view = useRailView()
  useEffect(() => {
    if (pathname === "/mentions") setRailView("mentions")
    else if (isInboxRoot(pathname, manager) && view === "mentions") setRailView("conversations")
  }, [pathname, view, manager])
}

function sidebarOpenAtStart() {
  const saved = document.cookie.match(/(?:^|; )sidebar_state=(true|false)/)?.[1]
  return saved ? saved === "true" : window.matchMedia("(min-width: 1100px)").matches
}

function Desktop({ frame, children }: { frame: boolean; children: React.ReactNode }) {
  const [open] = useState(sidebarOpenAtStart)
  return (
    <SidebarProvider defaultOpen={open} className="flex-col" data-testid="shell">
      <TopBar />
      <div className="flex min-w-0 flex-1 *:data-[slot=sidebar]:bg-sidebar">
        <AppSidebar />
        <div
          className={cn("flex min-w-0 flex-1 flex-col", frame && "sticky top-11 h-[calc(100svh-2.75rem)]")}
          data-frame={frame || undefined}
        >
          {children}
        </div>
      </div>
    </SidebarProvider>
  )
}

function CloseOnNavigate() {
  const { pathname } = useLocation()
  const { setOpenMobile } = useSidebar()
  useEffect(() => setOpenMobile(false), [pathname, setOpenMobile])
  return null
}

function Phone({ children }: { children: React.ReactNode }) {
  return (
    <SidebarProvider className="contents">
      <AppSidebar />
      <CloseOnNavigate />
      <div className="flex h-svh min-h-0 flex-col" data-frame data-testid="shell">
        {children}
      </div>
    </SidebarProvider>
  )
}

export function Layout() {
  const phone = useIsPhone()
  const { pathname } = useLocation()
  const { canManage } = useSession()
  const section = useSection()
  useMentionsView(pathname, canManage)
  if (!phone) {
    const frame =
      isChats(pathname, canManage) ||
      pathname === "/feedback" ||
      pathname.startsWith("/feedback/") ||
      section === "board" ||
      section === "docs" ||
      section === "status"
    return (
      <Desktop frame={frame}>
        <Outlet />
      </Desktop>
    )
  }
  if (
    section === "home" ||
    section === "board" ||
    section === "docs" ||
    section === "status" ||
    pathname === "/team" ||
    pathname === "/updates" ||
    pathname === "/updates/changelogs" ||
    pathname === "/broadcasts" ||
    pathname === "/broadcasts/lists" ||
    pathname === "/broadcasts/designs"
  ) {
    return (
      <Phone>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <Outlet />
        </div>
        <TabBar />
      </Phone>
    )
  }
  if (!isSectionHome(pathname)) {
    return (
      <Phone>
        <Outlet />
      </Phone>
    )
  }
  return (
    <Phone>
      <div className="flex min-h-0 flex-1 flex-col">
        <LeftColumn />
      </div>
      <TabBar />
    </Phone>
  )
}

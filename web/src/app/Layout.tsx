import { lazy, Suspense, useCallback, useEffect } from "react"
import { Outlet, useLocation, useMatch, useNavigate } from "react-router"

import { Header } from "@/app/Header"
import { useSection } from "@/app/nav"
import { SectionPlacesSidebar } from "@/app/places"
import { isChats, isInboxRoot, isSectionHome, type Section } from "@/app/shell"
import { SidebarProvider } from "@/components/ui/sidebar"
import { ContactsColumn } from "@/features/contact/ContactsColumn"
import { FeedbackList } from "@/features/feedback/FeedbackList"
import { AssistantsPanel } from "@/features/inbox/AssistantsPage"
import { setRailView } from "@/features/inbox/railView"
import { MentionsPanel } from "@/features/inbox/SidePanels"
import { useIsPhone } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

const HelpColumn = lazy(() => import("@/features/help/HelpColumn"))
const AnalyticsColumn = lazy(() => import("@/features/analytics/AnalyticsColumn"))

const COLUMNS: Section[] = ["mentions", "assistants", "feedback", "contacts", "knowledge", "analytics"]

function PhoneColumn({ section }: { section: Section }) {
  const navigate = useNavigate()
  const feedbackId = useMatch("/feedback/:conversationId")?.params.conversationId ?? null
  const openFeedback = useCallback((id: string) => navigate(`/feedback/${id}`), [navigate])
  if (section === "feedback") return <FeedbackList currentId={feedbackId} onOpen={openFeedback} />
  if (section === "mentions") return <MentionsPanel />
  if (section === "assistants") return <AssistantsPanel />
  if (section === "contacts") return <ContactsColumn />
  return (
    <Suspense fallback={<div className="flex-1 bg-background" />}>
      {section === "knowledge" ? <HelpColumn /> : <AnalyticsColumn />}
    </Suspense>
  )
}

function useMentionsView(pathname: string, manager: boolean) {
  useEffect(() => {
    if (pathname === "/mentions") setRailView("mentions")
    else if (pathname === "/assistants") setRailView("assistants")
    else if (isInboxRoot(pathname, manager)) setRailView("conversations")
  }, [pathname, manager])
}

function isPhoneDetail(pathname: string) {
  return (
    /^\/(conversations|feedback|contacts)\/[^/]+/.test(pathname) ||
    (pathname.startsWith("/mail/") && pathname.split("/").length > 4)
  )
}

function Phone() {
  const { pathname } = useLocation()
  const section = useSection()
  const column = isSectionHome(pathname) && COLUMNS.includes(section)
  if (pathname.startsWith("/migrations/")) return <Outlet />
  return (
    <SidebarProvider className="contents">
      <div className="flex h-svh min-h-0 flex-col" data-frame data-testid="shell">
        <Header />
        {!isPhoneDetail(pathname) && <SectionPlacesSidebar />}
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

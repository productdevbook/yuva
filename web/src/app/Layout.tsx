import { useCallback, useEffect } from "react"
import { Outlet, useLocation, useMatch, useNavigate } from "react-router"

import { Rail, TabBar, useSection } from "@/app/Rail"
import { isSectionHome } from "@/app/shell"
import { ContactsColumn } from "@/features/contact/ContactsColumn"
import { ChatList } from "@/features/inbox/ChatList"
import { setRailView, useRailView } from "@/features/inbox/railView"
import { AssistantsPanel, MentionsPanel } from "@/features/inbox/SidePanels"
import { ReportsColumn } from "@/features/reports/ReportsColumn"
import { SettingsColumn } from "@/features/settings/SettingsColumn"
import { TeamColumn } from "@/features/team/TeamColumn"
import { useIsPhone } from "@/hooks/use-media-query"
import { cn } from "@/lib/utils"

function LeftColumn({ keepList }: { keepList: boolean }) {
  const section = useSection()
  const navigate = useNavigate()
  const currentId = useMatch("/conversations/:conversationId")?.params.conversationId ?? null
  const open = useCallback((id: string) => navigate(`/conversations/${id}`), [navigate])
  const chats = section === "conversations"
  return (
    <>
      {(chats || keepList) && (
        <div className={cn("flex min-h-0 flex-1 flex-col", !chats && "hidden")}>
          <ChatList currentId={currentId} onOpen={open} active={chats} />
        </div>
      )}
      {section === "mentions" && <MentionsPanel />}
      {section === "assistants" && <AssistantsPanel />}
      {section === "contacts" && <ContactsColumn />}
      {section === "team" && <TeamColumn />}
      {section === "reports" && <ReportsColumn />}
      {section === "settings" && <SettingsColumn />}
    </>
  )
}

function useMentionsView(pathname: string) {
  const view = useRailView()
  useEffect(() => {
    if (pathname === "/mentions") setRailView("mentions")
    else if (pathname === "/" && view === "mentions") setRailView("conversations")
  }, [pathname, view])
}

export function Layout() {
  const phone = useIsPhone()
  const { pathname } = useLocation()
  useMentionsView(pathname)
  if (phone) {
    if (!isSectionHome(pathname)) {
      return (
        <div className="flex h-svh min-h-0 flex-col" data-testid="shell">
          <Outlet />
        </div>
      )
    }
    return (
      <div className="flex h-svh min-h-0 flex-col" data-testid="shell">
        <div className="flex min-h-0 flex-1 flex-col">
          <LeftColumn keepList={false} />
        </div>
        <TabBar />
      </div>
    )
  }
  return (
    <div className="flex h-svh min-h-0" data-testid="shell">
      <Rail />
      <div className="flex w-[clamp(288px,30vw,420px)] shrink-0 flex-col border-e">
        <LeftColumn keepList />
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <Outlet />
      </div>
    </div>
  )
}

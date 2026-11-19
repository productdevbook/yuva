import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon } from "lucide-react"
import { useCallback, useLayoutEffect, useRef, useState } from "react"
import { useLocation, useNavigate, useParams } from "react-router"

import { inboxRoot } from "@/app/shell"
import { PageLayout, PageTop } from "@/components/common"
import { PageTitle } from "@/components/common/ListBox"
import { ChatView } from "@/features/conversation/ChatView"
import { FeedbackHome } from "@/features/feedback/FeedbackHome"
import { FeedbackPage } from "@/features/feedback/FeedbackPage"
import {
  ChatList,
  ListMenu,
  rememberInbox,
  useRememberedChip,
  useRememberedInbox,
  StartConversation,
  useChipLabels,
  type ListShown,
} from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { AssistantsPage } from "@/features/inbox/AssistantsPage"
import { useOpenOnly, useOrigin } from "@/features/inbox/SourceFilter"
import { MentionsPage } from "@/features/inbox/SidePanels"
import { useActiveView } from "@/features/inbox/views/useActiveView"
import { useViews } from "@/features/inbox/views/queries"
import { useRailView } from "@/features/inbox/railView"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"
import { Button } from "@/components/ui/button"

const PANEL_KEY = "contact-panel"

function readPanel() {
  try {
    return localStorage.getItem(PANEL_KEY) !== "closed"
  } catch {
    return true
  }
}

function usePanel() {
  const [open, setOpen] = useState(readPanel)
  const set = useCallback((v: boolean) => {
    setOpen(v)
    try {
      localStorage.setItem(PANEL_KEY, v ? "open" : "closed")
    } catch {
      // Storage can be blocked; the panel then stays as chosen for this page only.
    }
  }, [])
  return [open, set] as const
}

function backSearch(search: string) {
  const params = new URLSearchParams(search)
  params.delete("m")
  params.delete("compose")
  const out = params.toString()
  return out ? `?${out}` : ""
}

function InboxPage({ id }: { id: string | null }) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { search } = useLocation()
  const { apart } = useSession()
  const wide = useMediaQuery("(min-width: 1180px)")
  const [panel, setPanel] = usePanel()
  const view = useActiveView()
  const openOnly = useOpenOnly()
  const [origin] = useOrigin()
  const rail = useRailView()
  const chip = useRememberedChip()
  const [starting, setStarting] = useState(false)
  const [sidebarInbox] = useInboxFilter()
  const chipInbox = useRememberedInbox()
  const [pager, setPager] = useState<HTMLElement | null>(null)
  const inboxId = sidebarInbox || chipInbox
  const product = useInboxes().data?.find((i) => i.id === inboxId)?.name
  const labels = useChipLabels()
  const saved = useViews().data?.find((v) => v.id === view.id)
  const viewName = view.id
    ? (saved?.name ?? t`Conversations`)
    : origin === "outbound"
      ? t`Replies to mail we sent`
      : openOnly
        ? t`Open`
        : chipInbox && product
          ? product
          : labels[chip]
  const listY = useRef(0)
  const open = useCallback(
    (next: string) => {
      if (!id) listY.current = window.scrollY
      navigate(`/conversations/${next}${search}`)
    },
    [navigate, search, id]
  )
  useLayoutEffect(() => {
    window.scrollTo({ top: id ? 0 : listY.current })
  }, [id])
  const listPath = `${inboxRoot(apart)}${backSearch(search)}`
  const back =
    rail === "mentions"
      ? { label: t`Mentions`, to: "/mentions" }
      : rail === "assistants"
        ? { label: t`Assistants`, to: "/assistants" }
        : { label: viewName, to: listPath }
  useHotkeys({ Escape: () => navigate(back.to) }, !!id)
  const fmt = new Intl.NumberFormat(i18n.locale)
  const where = chipInbox && product ? labels[chip] : (product ?? t`All inboxes`)
  const newConversation = (
    <Button onClick={() => setStarting(true)} data-testid="new-conversation">
      <PlusIcon />
      <Trans>New conversation</Trans>
    </Button>
  )
  const heading = ({ shown, unread, more }: ListShown) => {
    const n = `${fmt.format(shown)}${more ? "+" : ""}`
    const unreadText = fmt.format(unread)
    return (
      <PageTitle
        title={viewName}
        sub={
          <span data-testid="list-lede">
            {where}
            {" · "}
            {more ? (
              <Trans>{n} conversations</Trans>
            ) : (
              <Plural value={shown} one="# conversation" other="# conversations" />
            )}
            {unread > 0 && (
              <>
                {" · "}
                <Trans>{unreadText} unread</Trans>
              </>
            )}
          </span>
        }
        actions={
          <>
            {newConversation}
            <ListMenu className="border bg-card" />
          </>
        }
      />
    )
  }
  const list = (
    <ChatList
      currentId={id}
      onOpen={open}
      chip={chip}
      inbox={chipInbox}
      onInbox={rememberInbox}
      hidden={!!id}
      head={heading}
      pagerSlot={pager}
      place={back.label}
    />
  )
  if (id)
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="chats">
        <div className="mx-auto flex min-h-0 w-full max-w-[1440px] flex-1 flex-col ps-8 pe-7 pb-4 max-md:px-6">
          <PageTop back={back} slot={setPager} />
          {list}
          <ChatView key={id} id={id} panel={panel} inlinePanel={wide} onPanel={setPanel} />
        </div>
        <StartConversation open={starting} onOpenChange={setStarting} />
      </div>
    )
  return (
    <PageLayout testId="inbox-page" label={t`Conversations`}>
      {list}
      <StartConversation open={starting} onOpenChange={setStarting} />
    </PageLayout>
  )
}

export function ChatsPage({ section = "conversations" }: { section?: "conversations" | "feedback" }) {
  const { conversationId } = useParams()
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const phone = useIsPhone()
  const [panel, setPanel] = usePanel()
  const { apart } = useSession()
  const rail = useRailView()
  const back =
    section === "feedback" ? "/feedback" : rail === "mentions" || rail === "assistants" ? `/${rail}` : inboxRoot(apart)
  const id = conversationId ?? null
  useViewing(id ?? undefined)

  if (section === "feedback") return id ? <FeedbackPage id={id} /> : <FeedbackHome />
  if (!phone) {
    if (pathname === "/mentions") return <MentionsPage />
    if (pathname === "/assistants") return <AssistantsPage />
    return <InboxPage id={id} />
  }
  if (!id) {
    if (pathname === "/mentions") return <MentionsPage />
    if (pathname === "/assistants") return <AssistantsPage />
    return <InboxPage id={null} />
  }
  return (
    <main className="flex min-h-0 flex-1 flex-col px-3 pt-2.5 pb-2" data-testid="chats">
      <ChatView
        key={id}
        id={id}
        onBack={() => navigate(`${back}${search}`)}
        panel={panel}
        inlinePanel={false}
        onPanel={setPanel}
      />
    </main>
  )
}

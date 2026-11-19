import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, MessagesSquareIcon, PlusIcon } from "lucide-react"
import { useCallback, useLayoutEffect, useRef, useState } from "react"
import { useLocation, useNavigate, useParams } from "react-router"

import { inboxRoot } from "@/app/shell"
import { InfoColumn, PageHeading, PageLayout, PageTop, TopSep, TopSlotProvider } from "@/components/common"
import { formatDuration } from "@/components/common/text"
import { ChatView } from "@/features/conversation/ChatView"
import { EmptyFeedback } from "@/features/feedback/FeedbackList"
import { FeedbackPage } from "@/features/feedback/FeedbackPage"
import {
  ChatList,
  rememberChip,
  rememberInbox,
  useRememberedChip,
  useRememberedInbox,
  StartConversation,
  useChipLabels,
  type ListShown,
} from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { AssistantsPage } from "@/features/inbox/AssistantsPage"
import { ConversationEyebrow, ConversationInfo, ListInfo } from "@/features/inbox/InboxSide"
import { useOpenOnly, useOrigin } from "@/features/inbox/SourceFilter"
import { MentionsPage } from "@/features/inbox/SidePanels"
import { useCounts, useStats } from "@/features/inbox/queries"
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

function DayStats() {
  const { i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const stats = useStats(inboxId).data
  if (!stats) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const median = stats.median_first_reply_seconds
  const tiles: [string, React.ReactNode][] = [
    [fmt.format(stats.replies), <Trans key="replies">replies today</Trans>],
    [median !== undefined ? formatDuration(median, i18n.locale) : "–", <Trans key="median">median first reply</Trans>],
    [fmt.format(stats.closed), <Trans key="closed">closed today</Trans>],
  ]
  const good = stats.ratings.good
  const bad = stats.ratings.bad
  if (good + bad > 0) {
    const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(good / (good + bad))
    tiles.push([
      pct,
      <Trans key="rating">
        satisfied · 👍 {good} 👎 {bad}
      </Trans>,
    ])
  }
  return (
    <div
      className="mx-auto mt-7 grid max-w-[560px] auto-cols-fr grid-flow-col rounded-xl border bg-card [&>div+div]:border-s"
      data-testid="day-stats"
    >
      {tiles.map(([value, label], i) => (
        <div key={i} className="grid gap-0.5 px-3 py-4">
          <b className="text-title tabular-nums">{value}</b>
          <span className="text-caption text-faint">{label}</span>
        </div>
      ))}
    </div>
  )
}

function EmptyChat() {
  const counts = useCounts(["conversation"]).data
  const answered = counts !== undefined && counts.mine + counts.unassigned === 0
  return (
    <div
      className="flex min-h-0 flex-1 flex-col items-center justify-center bg-background px-8 text-center"
      data-testid="chat-empty"
    >
      <div className="mb-4 grid size-14 place-items-center rounded-[18px] bg-muted text-faint">
        {answered ? <CheckIcon className="size-6" /> : <MessagesSquareIcon className="size-6" />}
      </div>
      <h1 className="text-[18px] leading-6 font-semibold">
        {answered ? <Trans>Everyone has been answered.</Trans> : <Trans>Pick a conversation</Trans>}
      </h1>
      <p className="mt-2 max-w-sm text-small text-faint">
        {answered ? (
          <Trans>When a new message arrives, it will be in the list.</Trans>
        ) : (
          <Trans>Choose one from the list, or move with ↑ ↓ and open it with Enter.</Trans>
        )}
      </p>
      <DayStats />
    </div>
  )
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
  const [slot, setSlot] = useState<HTMLElement | null>(null)
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
  const where = product ?? t`every product`
  const heading = ({ shown, unread, more }: ListShown) => {
    const n = `${fmt.format(shown)}${more ? "+" : ""}`
    const unreadText = fmt.format(unread)
    return (
      <PageHeading
        title={viewName}
        lede={
          <span data-testid="list-lede">
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
            {" · "}
            {where}
          </span>
        }
      />
    )
  }
  return (
    <PageLayout
      testId={id ? "chats" : "inbox-page"}
      label={t`Conversations`}
      aside={
        <InfoColumn label={t`Details`}>
          {id ? (
            <ConversationInfo id={id} panel={panel} onPanel={setPanel} />
          ) : (
            <ListInfo title={viewName} inboxId={inboxId} />
          )}
        </InfoColumn>
      }
    >
      <PageTop
        back={id ? back : undefined}
        slot={id ? setSlot : undefined}
        actions={
          <Button
            className="h-8 gap-1.5 rounded-[9px] bg-foreground px-3 text-background hover:bg-foreground/90"
            onClick={() => setStarting(true)}
            title={t`New conversation`}
            data-testid="new-conversation"
          >
            <PlusIcon />
            <span className={id ? "sr-only" : "max-lg:sr-only"}>
              <Trans>New conversation</Trans>
            </span>
          </Button>
        }
      >
        {id ? (
          <ConversationEyebrow id={id} />
        ) : (
          <>
            <span className="font-medium text-foreground">
              <Trans>Conversations</Trans>
            </span>
            <TopSep />
            <span className="truncate">{viewName}</span>
            <TopSep />
            <span className="truncate">{product ?? t`All products`}</span>
          </>
        )}
      </PageTop>
      <div className="max-w-[860px]">
        <ChatList
          currentId={id}
          onOpen={open}
          chip={chip}
          onChip={rememberChip}
          inbox={chipInbox}
          onInbox={rememberInbox}
          flat
          hidden={!!id}
          head={heading}
        />
        {id && (
          <TopSlotProvider value={slot}>
            <ChatView key={id} id={id} flat panel={panel} inlinePanel={wide} onPanel={setPanel} />
          </TopSlotProvider>
        )}
      </div>
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

  if (!phone) {
    if (section === "feedback") return <FeedbackPage id={id} />
    if (pathname === "/mentions") return <MentionsPage />
    if (pathname === "/assistants") return <AssistantsPage />
    return <InboxPage id={id} />
  }
  if (!id) return section === "feedback" ? <EmptyFeedback /> : <EmptyChat />
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="chats">
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

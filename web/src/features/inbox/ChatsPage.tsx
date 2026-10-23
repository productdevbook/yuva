import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, MessagesSquareIcon } from "lucide-react"
import { useCallback, useState } from "react"
import { useDefaultLayout } from "react-resizable-panels"
import { useLocation, useNavigate, useParams } from "react-router"

import { inboxRoot } from "@/app/shell"
import { formatDuration } from "@/components/common/text"
import { ContactPanel } from "@/features/contact/ContactSheet"
import { ChatView } from "@/features/conversation/ChatView"
import { useConversation } from "@/features/conversation/queries"
import { EmptyFeedback } from "@/features/feedback/FeedbackList"
import { FeedbackPage } from "@/features/feedback/FeedbackPage"
import {
  ChatList,
  isWaiting,
  rememberChip,
  rememberedChip,
  StartConversation,
  useChipLabels,
  useChips,
  type Chip,
} from "@/features/inbox/ChatList"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { AssistantsPage } from "@/features/inbox/AssistantsPage"
import { MentionsPage } from "@/features/inbox/SidePanels"
import { useCounts, useLoadedCount, useStats } from "@/features/inbox/queries"
import { useRailView } from "@/features/inbox/railView"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"
import { Button } from "@/components/ui/button"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"

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
      className="mx-auto mt-7 grid max-w-[560px] auto-cols-fr grid-flow-col rounded-2xl border bg-card [&>div+div]:border-s"
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
      className="flex min-h-0 flex-1 flex-col items-center justify-center bg-surface px-8 text-center"
      data-testid="chat-empty"
    >
      <div className="mb-5 grid size-16 place-items-center rounded-[20px] bg-brand-wash text-brand">
        {answered ? <CheckIcon className="size-7" /> : <MessagesSquareIcon className="size-7" />}
      </div>
      <h1 className="text-title">
        {answered ? <Trans>Everyone has been answered.</Trans> : <Trans>Pick a conversation</Trans>}
      </h1>
      <p className="mt-1.5 text-body text-muted-foreground">
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

const handle = cn(
  "split-handle cursor-split bg-border bg-clip-content transition-colors duration-150 after:w-4 data-[separator=hover]:bg-brand/60 data-[separator=active]:bg-brand",
  "data-[separator=active]:z-10 data-[separator=active]:-mx-6 data-[separator=active]:px-6 data-[separator=active]:after:w-full",
  "data-[separator=hover]:z-10 data-[separator=hover]:-mx-6 data-[separator=hover]:px-6 data-[separator=hover]:after:w-full"
)

function followGrip(e: React.PointerEvent<HTMLDivElement>) {
  const el = e.currentTarget
  el.style.setProperty("--grip-y", `${e.clientY - el.getBoundingClientRect().top}px`)
  for (const sep of el.querySelectorAll<HTMLElement>("[data-slot=resizable-handle]")) {
    const r = sep.getBoundingClientRect()
    const dx = Math.max(-24, Math.min(24, e.clientX - (r.left + r.width / 2)))
    sep.style.setProperty("--grip-dx", `${dx}px`)
  }
}

function InboxPage({ id }: { id: string | null }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const roomy = useMediaQuery("(min-width: 1200px)")
  const [panel, setPanel] = usePanel()
  const [chip, setChip] = useState<Chip>(rememberedChip)
  const [starting, setStarting] = useState(false)
  const [sidebarInbox] = useInboxFilter()
  const [chipInbox, setChipInbox] = useState("")
  const inboxId = sidebarInbox || chipInbox
  const product = useInboxes().data?.find((i) => i.id === sidebarInbox)?.name
  const counts = useCounts(["conversation"]).data
  const scope = { kind: ["conversation" as const], inbox_id: inboxId || undefined }
  const waiting = useLoadedCount({ ...scope, status: ["open"] }, isWaiting)
  const unassigned = useLoadedCount({ ...scope, status: ["open"], assignee: "unassigned" })
  const labels = useChipLabels()
  const tabs = useChips()
  const contactId = useConversation(id ?? undefined).data?.contact_id
  const open = useCallback((next: string) => navigate(`/conversations/${next}`), [navigate])
  const count = (k: Chip) =>
    k === "waiting" ? waiting : k === "unassigned" ? (inboxId ? unassigned : counts?.unassigned) : undefined
  const inline = roomy && panel && !!contactId && !!id
  const layout = useDefaultLayout({
    id: "inbox-panes",
    panelIds: inline ? ["list", "chat", "contact"] : ["list", "chat"],
  })
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid={id ? "chats" : "inbox-page"}>
      <PageTop
        title={<Trans>Inbox</Trans>}
        sub={product ? <Trans>{product}’s conversations</Trans> : <Trans>Conversations from every product</Trans>}
        actions={
          <Button
            className={look.btnInk}
            onClick={() => setStarting(true)}
            title={t`New conversation`}
            data-testid="new-conversation"
          >
            <Trans>New conversation</Trans>
          </Button>
        }
        tabs={
          <FrameTabs
            label={t`Filter`}
            value={chip}
            onChange={(k) => {
              rememberChip(k)
              setChip(k)
            }}
            items={tabs.map((k) => ({ key: k, label: labels[k], count: count(k), testId: `chip-${k}` }))}
          />
        }
      />
      <ResizablePanelGroup
        className="min-h-0 flex-1"
        disableCursor
        onPointerMoveCapture={followGrip}
        resizeTargetMinimumSize={{ fine: 16, coarse: 32 }}
        defaultLayout={layout.defaultLayout}
        onLayoutChanged={layout.onLayoutChanged}
      >
        <ResizablePanel
          id="list"
          className="flex flex-col"
          defaultSize={340}
          minSize={260}
          maxSize={560}
          groupResizeBehavior="preserve-pixel-size"
        >
          <ChatList
            currentId={id}
            onOpen={open}
            chip={chip}
            onChip={setChip}
            inbox={chipInbox}
            onInbox={setChipInbox}
            embedded
          />
        </ResizablePanel>
        <ResizableHandle withHandle className={handle} />
        <ResizablePanel id="chat" className="flex flex-col" minSize={440}>
          {id ? <ChatView key={id} id={id} panel={panel} inlinePanel={roomy} onPanel={setPanel} /> : <EmptyChat />}
        </ResizablePanel>
        {inline && (
          <>
            <ResizableHandle withHandle className={handle} />
            <ResizablePanel
              id="contact"
              className="flex flex-col"
              defaultSize={280}
              minSize={240}
              maxSize={480}
              groupResizeBehavior="preserve-pixel-size"
              data-contact-panel
            >
              <ContactPanel key={contactId} contactId={contactId} conversationId={id} onClose={() => setPanel(false)} />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      <StartConversation open={starting} onOpenChange={setStarting} />
    </main>
  )
}

export function ChatsPage({ section = "conversations" }: { section?: "conversations" | "feedback" }) {
  const { conversationId } = useParams()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const phone = useIsPhone()
  const [panel, setPanel] = usePanel()
  const { canManage } = useSession()
  const rail = useRailView()
  const back = section === "feedback" ? "/feedback" : rail === "mentions" ? "/mentions" : inboxRoot(canManage)
  const id = conversationId ?? null
  useViewing(id ?? undefined)

  if (!phone) {
    if (section === "feedback") return <FeedbackPage id={id} />
    if (pathname === "/mentions") return <MentionsPage />
    if (!id && rail === "assistants") return <AssistantsPage />
    return <InboxPage id={id} />
  }
  if (!id) return section === "feedback" ? <EmptyFeedback /> : <EmptyChat />
  return (
    <main className="flex min-h-0 flex-1 flex-col" data-testid="chats">
      <ChatView key={id} id={id} onBack={() => navigate(back)} panel={panel} inlinePanel={false} onPanel={setPanel} />
    </main>
  )
}

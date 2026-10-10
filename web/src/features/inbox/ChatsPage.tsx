import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, MessagesSquareIcon } from "lucide-react"
import { useCallback, useState } from "react"
import { useNavigate, useParams } from "react-router"

import { formatDuration } from "@/components/common/text"
import { ContactPanel } from "@/features/contact/ContactSheet"
import { ChatView } from "@/features/conversation/ChatView"
import { useConversation } from "@/features/conversation/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useCounts, useStats } from "@/features/inbox/queries"
import { useRailView } from "@/features/inbox/railView"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import { useViewing } from "@/lib/realtime"

const PANEL_KEY = "contact-panel"

function readPanel() {
  try {
    return localStorage.getItem(PANEL_KEY) === "open"
  } catch {
    return false
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
    [fmt.format(stats.replies), <Trans>replies today</Trans>],
    [median !== undefined ? formatDuration(median, i18n.locale) : "–", <Trans>median first reply</Trans>],
    [fmt.format(stats.closed), <Trans>closed today</Trans>],
  ]
  const good = stats.ratings.good
  const bad = stats.ratings.bad
  if (good + bad > 0) {
    const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(good / (good + bad))
    tiles.push([
      pct,
      <Trans>
        satisfied · 👍 {good} 👎 {bad}
      </Trans>,
    ])
  }
  return (
    <div className="mx-auto mt-7 grid max-w-[560px] auto-cols-fr grid-flow-col rounded-2xl border bg-card [&>div+div]:border-s" data-testid="day-stats">
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
  const counts = useCounts().data
  const answered = counts !== undefined && counts.mine + counts.unassigned === 0
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center bg-surface px-8 text-center" data-testid="chat-empty">
      <div className="mb-5 grid size-16 place-items-center rounded-[20px] bg-brand-wash text-brand">
        {answered ? <CheckIcon className="size-7" /> : <MessagesSquareIcon className="size-7" />}
      </div>
      <h1 className="text-title">{answered ? <Trans>Everyone has been answered.</Trans> : <Trans>Pick a conversation</Trans>}</h1>
      <p className="mt-1.5 text-body text-muted-foreground">
        {answered ? <Trans>When a new message arrives, it will be in the list.</Trans> : <Trans>Choose one from the list, or move with ↑ ↓ and open it with Enter.</Trans>}
      </p>
      <DayStats />
    </div>
  )
}

export function ChatsPage() {
  const { conversationId } = useParams()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const roomy = useMediaQuery("(min-width: 1200px)")
  const [panel, setPanel] = usePanel()
  const back = useRailView() === "mentions" ? "/mentions" : "/"
  const id = conversationId ?? null
  const contactId = useConversation(id ?? undefined).data?.contact_id
  useViewing(id ?? undefined)

  if (!id) return <EmptyChat />
  if (phone) {
    return (
      <main className="flex min-h-0 flex-1 flex-col" data-testid="chats">
        <ChatView key={id} id={id} onBack={() => navigate(back)} panel={panel} inlinePanel={false} onPanel={setPanel} />
      </main>
    )
  }
  const inline = roomy && panel && !!contactId
  return (
    <main className="flex min-h-0 flex-1" data-testid="chats">
      <div className="flex min-w-0 flex-1 flex-col">
        <ChatView key={id} id={id} panel={panel} inlinePanel={roomy} onPanel={setPanel} />
      </div>
      {inline && (
        <div className="flex w-[360px] shrink-0 flex-col border-s">
          <ContactPanel key={contactId} contactId={contactId} conversationId={id} onClose={() => setPanel(false)} />
        </div>
      )}
    </main>
  )
}

import { useLingui } from "@lingui/react/macro"
import {
  ActivityIcon,
  AtSignIcon,
  BarChart3Icon,
  BookOpenTextIcon,
  BookUserIcon,
  BotIcon,
  ChartLineIcon,
  HouseIcon,
  KanbanIcon,
  LifeBuoyIcon,
  MailIcon,
  MegaphoneIcon,
  MessageSquareHeartIcon,
  MessagesSquareIcon,
  SparklesIcon,
  UsersIcon,
  MenuIcon,
} from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { inboxRoot, isChats, isFullPage, sectionOf, type Section } from "@/app/shell"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useSidebar } from "@/components/ui/sidebar"
import { useAnalyticsVisible } from "@/features/analytics/properties"
import { useConversations, useCounts, useDrafts, useMentions } from "@/features/inbox/queries"
import { setRailView, useRailView } from "@/features/inbox/railView"
import { useMailInboxes } from "@/features/mail/folders"
import { useMailUnread } from "@/features/mail/queries"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

type Entry = {
  key: Section
  label: string
  icon: typeof MessagesSquareIcon
  shortcut: string
  badge?: number
  badgeLabel?: string
  hot?: boolean
}

export function mainKeys(mailFirst: boolean): Section[] {
  return mailFirst
    ? ["home", "mail", "conversations", "mentions", "assistants"]
    : ["home", "conversations", "mail", "mentions", "assistants"]
}

export function useUnreadCount() {
  const list = useConversations({ kind: ["conversation"], purpose: "support" })
  return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => c.unread).length
}

export function useEntries(): Entry[] {
  const { t } = useLingui()
  const unread = useUnreadCount()
  const feedback = Number(useCounts().data?.feedback ?? 0)
  const unseen = Number(useMentions().data?.pages[0]?.unseen ?? 0)
  const botDrafts = Number(useDrafts({ author: "bot" }).data?.pages[0]?.total ?? 0)
  const assistantDrafts = Number(useDrafts({ author: "assistant" }).data?.pages[0]?.total ?? 0)
  const drafts = botDrafts + assistantDrafts
  const analytics = useAnalyticsVisible()
  const mail = useMailInboxes().length > 0
  const mailUnread = useMailUnread()
  const { canManage, mailFirst } = useSession()
  const mailEntry: Entry[] = mail
    ? [
        {
          key: "mail",
          label: t`Mail`,
          icon: MailIcon,
          shortcut: SHORTCUTS.mail,
          badge: mailUnread,
          badgeLabel: t`Mail, ${mailUnread} unread`,
        },
      ]
    : []
  const entries: Entry[] = [
    ...(canManage ? [{ key: "home" as const, label: t`Home`, icon: HouseIcon, shortcut: SHORTCUTS.home }] : []),
    ...(mailFirst ? mailEntry : []),
    {
      key: "conversations",
      label: t`Inbox`,
      icon: MessagesSquareIcon,
      shortcut: SHORTCUTS.conversations,
      badge: unread,
      badgeLabel: t`Conversations, ${unread} unread`,
      hot: true,
    },
    ...(mailFirst ? [] : mailEntry),
    {
      key: "mentions",
      label: t`Mentions`,
      icon: AtSignIcon,
      shortcut: SHORTCUTS.mentions,
      badge: unseen,
      badgeLabel: t`Mentions, ${unseen} unseen`,
      hot: true,
    },
    {
      key: "feedback",
      label: t`Feedback`,
      icon: MessageSquareHeartIcon,
      shortcut: SHORTCUTS.feedback,
      badge: feedback,
      badgeLabel: t`Feedback, ${feedback} open`,
    },
    { key: "contacts", label: t`Contacts`, icon: BookUserIcon, shortcut: SHORTCUTS.contacts },
    { key: "team", label: t`Team`, icon: UsersIcon, shortcut: SHORTCUTS.team },
    { key: "reports", label: t`Reports`, icon: BarChart3Icon, shortcut: SHORTCUTS.reports },
    { key: "docs", label: t`Docs`, icon: BookOpenTextIcon, shortcut: SHORTCUTS.docs },
    { key: "broadcasts", label: t`Broadcasts`, icon: MegaphoneIcon, shortcut: SHORTCUTS.broadcasts },
    { key: "status", label: t`Status pages`, icon: ActivityIcon, shortcut: SHORTCUTS.status },
    { key: "knowledge", label: t`Help center`, icon: LifeBuoyIcon, shortcut: SHORTCUTS.helpCenter },
    { key: "board", label: t`Roadmap`, icon: KanbanIcon, shortcut: SHORTCUTS.roadmap },
    { key: "updates", label: t`What’s new`, icon: SparklesIcon, shortcut: SHORTCUTS.updates },
    { key: "analytics", label: t`Analytics`, icon: ChartLineIcon, shortcut: SHORTCUTS.analytics },
    {
      key: "assistants",
      label: t`Assistants`,
      icon: BotIcon,
      shortcut: SHORTCUTS.assistants,
      badge: drafts,
      badgeLabel: t`Assistants, ${drafts} drafts waiting`,
    },
  ]
  return analytics ? entries : entries.filter((e) => e.key !== "analytics")
}

export function useSection() {
  const { apart, canManage } = useSession()
  return sectionOf(useLocation().pathname, useRailView(), apart, canManage)
}

export function useGo() {
  const navigate = useNavigate()
  const { pathname, search } = useLocation()
  const section = useSection()
  const { apart, homePath } = useSession()
  return (key: Section) => {
    if (key === "home") {
      if (pathname !== homePath) navigate(homePath)
    } else if (key === "mentions" || key === "assistants") {
      setRailView(key)
      if (pathname !== `/${key}`) navigate(`/${key}`)
    } else if (key === "conversations") {
      setRailView(key)
      if (!isChats(pathname, apart) || search) navigate(inboxRoot(apart))
    } else if (section !== key || (isFullPage(key) && pathname !== `/${key}`)) {
      navigate(`/${key}`)
    }
  }
}

function Badge({ n, id }: { n: number; id: string }) {
  if (!n) return null
  return (
    <span
      className="absolute -top-0.5 end-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-brand-solid px-1 text-[10px] leading-none font-semibold text-white tabular-nums"
      data-testid={`rail-badge-${id}`}
    >
      {n > 99 ? "99+" : n}
    </span>
  )
}

export function moveFocus(e: React.KeyboardEvent<HTMLElement>) {
  if (e.key !== "ArrowDown" && e.key !== "ArrowUp" && e.key !== "ArrowLeft" && e.key !== "ArrowRight") return
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("[data-rail-item]"))
  const i = items.indexOf(document.activeElement as HTMLElement)
  if (i < 0) return
  e.preventDefault()
  const step = e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : -1
  items[(i + step + items.length) % items.length].focus()
}

const tabClass =
  "flex min-w-0 flex-1 basis-0 flex-col items-center gap-0.5 pt-2 pb-1.5 text-[11px] text-sidebar-muted outline-none focus-visible:bg-sidebar-accent aria-[current=page]:font-semibold aria-[current=page]:text-brand-ink"

const tabActiveClass = "bg-sidebar-active shadow-[inset_0_0_0_1px_var(--sidebar-active-ring)]"

export function TabBar() {
  const { t } = useLingui()
  const view = useSection()
  const go = useGo()
  const all = useEntries()
  const { mailFirst } = useSession()
  const { setOpenMobile } = useSidebar()
  const entries = mainKeys(mailFirst)
    .flatMap((k) => all.find((e) => e.key === k) ?? [])
    .slice(0, 4)
  const elsewhere = !entries.some((e) => e.key === view)
  return (
    <nav
      aria-label={t`Sections`}
      onKeyDown={moveFocus}
      className="flex shrink-0 items-stretch border-t border-sidebar-border bg-card ps-[max(0.5rem,env(safe-area-inset-left))] pe-[max(0.5rem,env(safe-area-inset-right))] pb-[env(safe-area-inset-bottom)]"
      data-testid="tab-bar"
    >
      {entries.map((e) => (
        <button
          key={e.key}
          type="button"
          onClick={() => go(e.key)}
          aria-current={e.key === view ? "page" : undefined}
          aria-label={e.badge ? e.badgeLabel : e.label}
          className={tabClass}
          data-rail-item
          data-testid={`tab-${e.key}`}
        >
          <span
            className={cn(
              "relative grid h-7 w-full max-w-12 place-items-center rounded-full",
              e.key === view && tabActiveClass
            )}
          >
            <e.icon className="size-[19px]" />
            <Badge n={e.badge ?? 0} id={e.key} />
          </span>
          <span aria-hidden className="max-w-full truncate">
            {e.label}
          </span>
        </button>
      ))}
      <button
        type="button"
        onClick={() => setOpenMobile(true)}
        aria-current={elsewhere ? "page" : undefined}
        aria-label={t`Menu`}
        className={tabClass}
        data-rail-item
        data-testid="tab-menu"
      >
        <span className={cn("grid h-7 w-full max-w-12 place-items-center rounded-full", elsewhere && tabActiveClass)}>
          <MenuIcon className="size-[19px]" />
        </span>
        <span aria-hidden className="max-w-full truncate">
          {t`Menu`}
        </span>
      </button>
    </nav>
  )
}

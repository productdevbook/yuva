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
  const drafts = Number(useDrafts({}).data?.pages[0]?.total ?? 0)
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
      label: t`Conversations`,
      icon: MessagesSquareIcon,
      shortcut: SHORTCUTS.conversations,
      badge: unread,
      badgeLabel: t`Conversations, ${unread} unread`,
    },
    ...(mailFirst ? [] : mailEntry),
    {
      key: "mentions",
      label: t`Mentions`,
      icon: AtSignIcon,
      shortcut: SHORTCUTS.mentions,
      badge: unseen,
      badgeLabel: t`Mentions, ${unseen} unseen`,
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
    { key: "status", label: t`Status`, icon: ActivityIcon, shortcut: SHORTCUTS.status },
    { key: "knowledge", label: t`Help`, icon: LifeBuoyIcon, shortcut: SHORTCUTS.helpCenter },
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
    } else if (key === "mentions") {
      setRailView(key)
      if (pathname !== "/mentions") navigate("/mentions")
    } else if (key === "conversations" || key === "assistants") {
      setRailView(key)
      if (!isChats(pathname, apart) || search) navigate(inboxRoot(apart))
    } else if (section !== key || (isFullPage(key) && pathname !== `/${key}`)) {
      navigate(`/${key}`)
    }
  }
}

function Badge({ n, id, className }: { n: number; id: string; className?: string }) {
  if (!n) return null
  return (
    <span
      className={cn(
        "absolute -top-1 -end-1 grid h-[18px] min-w-[18px] place-items-center rounded-full bg-brand px-1 text-[11px] leading-none font-semibold text-white tabular-nums",
        className
      )}
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

export function TabBar() {
  const { t } = useLingui()
  const view = useSection()
  const go = useGo()
  const all = useEntries()
  const { setOpenMobile } = useSidebar()
  const entries = all.slice(0, 4)
  const elsewhere = !entries.some((e) => e.key === view)
  return (
    <nav
      aria-label={t`Sections`}
      onKeyDown={moveFocus}
      className="flex shrink-0 items-stretch border-t bg-background ps-[max(0.5rem,env(safe-area-inset-left))] pe-[max(0.5rem,env(safe-area-inset-right))] pb-[env(safe-area-inset-bottom)]"
      data-testid="tab-bar"
    >
      {entries.map((e) => (
        <button
          key={e.key}
          type="button"
          onClick={() => go(e.key)}
          aria-current={e.key === view ? "page" : undefined}
          aria-label={e.badge ? e.badgeLabel : e.label}
          className="flex min-w-0 flex-1 basis-0 flex-col items-center gap-0.5 pt-2 pb-1.5 text-[11px] text-muted-foreground outline-none focus-visible:bg-muted aria-[current=page]:min-w-max aria-[current=page]:px-1 aria-[current=page]:text-brand"
          data-rail-item
          data-testid={`tab-${e.key}`}
        >
          <span
            className={cn(
              "relative grid h-7 w-full max-w-12 place-items-center rounded-full",
              e.key === view && "bg-brand-wash"
            )}
          >
            <e.icon className="size-5" />
            <Badge n={e.badge ?? 0} id={e.key} className="-top-1 end-auto start-[calc(50%+6px)]" />
          </span>
          <span
            aria-hidden
            className={cn("font-medium whitespace-nowrap", e.key !== view && "invisible max-w-full overflow-hidden")}
          >
            {e.label}
          </span>
        </button>
      ))}
      <button
        type="button"
        onClick={() => setOpenMobile(true)}
        aria-current={elsewhere ? "page" : undefined}
        aria-label={t`Menu`}
        className="flex min-w-0 flex-1 basis-0 flex-col items-center gap-0.5 pt-2 pb-1.5 text-[11px] text-muted-foreground outline-none focus-visible:bg-muted aria-[current=page]:text-brand"
        data-rail-item
        data-testid="tab-menu"
      >
        <span className={cn("grid h-7 w-full max-w-12 place-items-center rounded-full", elsewhere && "bg-brand-wash")}>
          <MenuIcon className="size-5" />
        </span>
        <span aria-hidden className={cn("font-medium whitespace-nowrap", !elsewhere && "invisible")}>
          {t`Menu`}
        </span>
      </button>
    </nav>
  )
}

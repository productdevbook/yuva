import { useLingui } from "@lingui/react/macro"
import { AtSignIcon, BarChart3Icon, BookUserIcon, BotIcon, MessagesSquareIcon, SettingsIcon, UsersIcon } from "lucide-react"
import { useLocation, useNavigate } from "react-router"

import { isChats, sectionOf, type Section } from "@/app/shell"
import { UserMenu } from "@/app/UserMenu"
import { mod, SHORTCUTS, keyLabel } from "@/components/common/ShortcutSheet"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useConversations, useDrafts, useMentions } from "@/features/inbox/queries"
import { setRailView, useRailView } from "@/features/inbox/railView"
import { cn } from "@/lib/utils"

type Entry = { key: Section; label: string; icon: typeof MessagesSquareIcon; shortcut: string; badge?: number; badgeLabel?: string }

export function useUnreadCount() {
  const list = useConversations({})
  return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => c.unread).length
}

function useEntries(): Entry[] {
  const { t } = useLingui()
  const unread = useUnreadCount()
  const unseen = Number(useMentions().data?.pages[0]?.unseen ?? 0)
  const drafts = Number(useDrafts({}).data?.pages[0]?.total ?? 0)
  return [
    { key: "conversations", label: t`Conversations`, icon: MessagesSquareIcon, shortcut: SHORTCUTS.conversations, badge: unread, badgeLabel: t`Conversations, ${unread} unread` },
    { key: "mentions", label: t`Mentions`, icon: AtSignIcon, shortcut: SHORTCUTS.mentions, badge: unseen, badgeLabel: t`Mentions, ${unseen} unseen` },
    { key: "contacts", label: t`Contacts`, icon: BookUserIcon, shortcut: SHORTCUTS.contacts },
    { key: "team", label: t`Team`, icon: UsersIcon, shortcut: SHORTCUTS.team },
    { key: "reports", label: t`Reports`, icon: BarChart3Icon, shortcut: SHORTCUTS.reports },
    { key: "assistants", label: t`Assistants`, icon: BotIcon, shortcut: SHORTCUTS.assistants, badge: drafts, badgeLabel: t`Assistants, ${drafts} drafts waiting` },
  ]
}

export function useSection() {
  return sectionOf(useLocation().pathname, useRailView())
}

export function useGo() {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const section = useSection()
  return (key: Section) => {
    if (key === "mentions") {
      setRailView(key)
      if (pathname !== "/mentions") navigate("/mentions")
    } else if (key === "conversations" || key === "assistants") {
      setRailView(key)
      if (!isChats(pathname)) navigate("/")
    } else if (section !== key) {
      navigate(`/${key}`)
    }
  }
}

function Badge({ n, id, className }: { n: number; id: string; className?: string }) {
  if (!n) return null
  return (
    <span
      className={cn("absolute -top-1 -end-1 grid h-[18px] min-w-[18px] place-items-center rounded-full bg-brand px-1 text-[11px] leading-none font-semibold text-white tabular-nums", className)}
      data-testid={`rail-badge-${id}`}
    >
      {n > 99 ? "99+" : n}
    </span>
  )
}

function moveFocus(e: React.KeyboardEvent<HTMLElement>) {
  if (e.key !== "ArrowDown" && e.key !== "ArrowUp" && e.key !== "ArrowLeft" && e.key !== "ArrowRight") return
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>("[data-rail-item]"))
  const i = items.indexOf(document.activeElement as HTMLElement)
  if (i < 0) return
  e.preventDefault()
  const step = e.key === "ArrowDown" || e.key === "ArrowRight" ? 1 : -1
  items[(i + step + items.length) % items.length].focus()
}

const itemClass =
  "relative grid size-10 place-items-center rounded-xl text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-brand-wash aria-[current=page]:text-brand"

export function Rail() {
  const { t } = useLingui()
  const section = useSection()
  const go = useGo()
  const entries = useEntries()
  return (
    <nav aria-label={t`Sections`} onKeyDown={moveFocus} className="flex w-16 shrink-0 flex-col items-center gap-1.5 border-e bg-surface py-3" data-testid="rail">
      {entries.map((e) => (
        <Tooltip key={e.key}>
          <TooltipTrigger
            render={
              <button
                type="button"
                className={itemClass}
                aria-current={section === e.key ? "page" : undefined}
                aria-label={e.badge ? e.badgeLabel : e.label}
                onClick={() => go(e.key)}
                data-rail-item
                data-testid={`rail-${e.key}`}
              />
            }
          >
            <e.icon className="size-5" />
            <Badge n={e.badge ?? 0} id={e.key} />
          </TooltipTrigger>
          <TooltipContent side="inline-end">
            {e.label}
            <span className="text-background/60">{keyLabel(e.shortcut).join(" ")}</span>
          </TooltipContent>
        </Tooltip>
      ))}
      <span className="flex-1" />
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              className={itemClass}
              aria-current={section === "settings" ? "page" : undefined}
              aria-label={t`Settings`}
              onClick={() => go("settings")}
              data-rail-item
              data-testid="rail-settings"
            />
          }
        >
          <SettingsIcon className="size-5" />
        </TooltipTrigger>
        <TooltipContent side="inline-end">
          {t`Settings`}
          <span className="text-background/60">{mod} ,</span>
        </TooltipContent>
      </Tooltip>
      <UserMenu align="end" side="inline-end" />
    </nav>
  )
}

export function TabBar() {
  const { t } = useLingui()
  const view = useSection()
  const go = useGo()
  const entries = useEntries()
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
          className="flex min-w-0 flex-1 basis-0 flex-col items-center gap-0.5 pt-2 pb-1.5 text-[11px] text-muted-foreground outline-none focus-visible:bg-muted aria-[current=page]:text-brand"
          data-rail-item
          data-testid={`tab-${e.key}`}
        >
          <span className={cn("relative grid h-7 w-12 place-items-center rounded-full", e.key === view && "bg-brand-wash")}>
            <e.icon className="size-5" />
            <Badge n={e.badge ?? 0} id={e.key} className="-top-1 end-auto start-[calc(50%+6px)]" />
          </span>
          <span aria-hidden className={cn("font-medium whitespace-nowrap", e.key !== view && "invisible")}>
            {e.label}
          </span>
        </button>
      ))}
    </nav>
  )
}

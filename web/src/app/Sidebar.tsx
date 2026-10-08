import { Trans, useLingui } from "@lingui/react/macro"
import { InboxIcon, MessageSquareHeartIcon, SettingsIcon, ShieldAlertIcon, UserIcon, UserXIcon } from "lucide-react"
import { Link, useLocation } from "react-router"

import { UserMenu } from "@/app/UserMenu"
import { WorkspaceSwitcher } from "@/app/WorkspaceSwitcher"
import { Dot } from "@/components/common"
import { FEEDBACK_CATEGORIES, useEnumText } from "@/components/common/text"
import { useCounts } from "@/features/inbox/queries"
import { useViewLabels, VIEWS, type View } from "@/features/inbox/views"
import { useRealtimeStatus } from "@/lib/realtime"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels } from "@/lib/workspace"

const viewIcons: Record<View, React.ComponentType<{ className?: string }>> = {
  all: InboxIcon,
  mine: UserIcon,
  unassigned: UserXIcon,
  spam: ShieldAlertIcon,
}

function NavItem({
  to,
  active,
  count,
  icon,
  children,
  className,
  testId,
}: {
  to: string
  active: boolean
  count?: number
  icon?: React.ReactNode
  children: React.ReactNode
  className?: string
  testId?: string
}) {
  const { i18n } = useLingui()
  return (
    <li>
      <Link
        to={to}
        aria-current={active ? "page" : undefined}
        data-testid={testId}
        className={cn(
          "flex h-8 items-center gap-2.5 rounded-lg px-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
          active && "bg-background font-medium text-foreground shadow-[0_1px_2px_rgb(15_23_42/0.06)] ring-1 ring-border hover:bg-background",
          className,
        )}
      >
        {icon && <span className="flex size-4 shrink-0 items-center justify-center [&_svg]:size-4">{icon}</span>}
        <span className="min-w-0 flex-1 truncate">{children}</span>
        {!!count && (
          <span className="text-xs text-faint tabular-nums" data-testid="nav-count">
            {new Intl.NumberFormat(i18n.locale, { notation: "compact" }).format(count)}
          </span>
        )}
      </Link>
    </li>
  )
}

function Group({ title, children }: { title: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="mt-6">
      <h2 className="mb-1 px-2.5 text-xs font-medium text-faint">{title}</h2>
      <ul className="flex flex-col gap-px">{children}</ul>
    </div>
  )
}

function Nav() {
  const { pathname } = useLocation()
  const labels = useViewLabels()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const tags = useLabels().data ?? []
  const counts = useCounts().data
  const inboxCounts = new Map(counts?.inboxes.map((x) => [x.id, x.count]))
  const labelCounts = new Map(counts?.labels.map((x) => [x.id, x.count]))
  const categoryCounts = new Map(counts?.feedback_categories.map((x) => [x.category, x.count]))
  const [, first, second] = pathname.split("/")
  const onFeedback = first === "feedback"
  const categories = FEEDBACK_CATEGORIES.filter((c) => categoryCounts.has(c))
  const view = (v: View) => {
    const Icon = viewIcons[v]
    return (
      <NavItem key={v} to={`/${v}`} active={first === v} count={counts?.[v]} icon={<Icon />}>
        {labels[v]}
      </NavItem>
    )
  }
  return (
    <>
      <ul className="flex flex-col gap-px">
        {VIEWS.filter((v) => v !== "spam").map(view)}
        <NavItem
          to="/feedback/all"
          active={onFeedback && second === "all"}
          count={counts?.feedback}
          icon={<MessageSquareHeartIcon />}
          testId="nav-feedback"
        >
          <Trans>Feedback</Trans>
        </NavItem>
        {onFeedback &&
          categories.map((c) => (
            <NavItem
              key={c}
              to={`/feedback/${c}`}
              active={second === c}
              count={categoryCounts.get(c)}
              className="ps-9"
              testId="nav-feedback-category"
            >
              {text.category[c]}
            </NavItem>
          ))}
        {view("spam")}
      </ul>
      {inboxes.length > 0 && (
        <Group title={<Trans>Inboxes</Trans>}>
          {inboxes.map((inbox) => (
            <NavItem
              key={inbox.id}
              to={`/inbox/${inbox.id}`}
              active={first === "inbox" && second === inbox.id}
              count={inboxCounts.get(inbox.id)}
              icon={<Dot color={inbox.branding.color} />}
            >
              {inbox.name}
            </NavItem>
          ))}
        </Group>
      )}
      {tags.length > 0 && (
        <Group title={<Trans>Labels</Trans>}>
          {tags.map((label) => (
            <NavItem
              key={label.id}
              to={`/label/${label.id}`}
              active={first === "label" && second === label.id}
              count={labelCounts.get(label.id)}
              icon={<Dot color={label.color} className="size-1.5" />}
            >
              {label.name}
            </NavItem>
          ))}
        </Group>
      )}
    </>
  )
}

function ConnectionLine() {
  const status = useRealtimeStatus()
  return (
    <p
      className={cn("flex items-center gap-2 px-2.5 pb-1 text-xs text-faint", status === "live" && "sr-only")}
      data-testid="realtime-status"
      data-status={status}
      role="status"
    >
      <span className={cn("size-1.5 rounded-full", status === "live" ? "bg-success" : status === "connecting" ? "bg-warning" : "bg-destructive")} />
      {status === "live" ? <Trans>Live</Trans> : status === "connecting" ? <Trans>Connecting…</Trans> : <Trans>Offline</Trans>}
    </p>
  )
}

export function Sidebar({ onShortcuts }: { onShortcuts: () => void }) {
  const { t } = useLingui()
  const { pathname } = useLocation()
  return (
    <div className="flex h-full min-h-0 flex-col bg-surface">
      <div className="flex h-14 shrink-0 items-center px-3">
        <WorkspaceSwitcher />
      </div>
      <nav className="min-h-0 flex-1 overflow-y-auto px-3 pt-1 pb-4" aria-label={t`Navigation`}>
        <Nav />
      </nav>
      <div className="flex shrink-0 flex-col gap-1 px-3 pb-3">
        <ConnectionLine />
        <ul>
          <NavItem to="/settings" active={pathname.startsWith("/settings")} icon={<SettingsIcon />}>
            <Trans>Settings</Trans>
          </NavItem>
        </ul>
        <UserMenu onShortcuts={onShortcuts} />
      </div>
    </div>
  )
}

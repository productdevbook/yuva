import { Trans, useLingui } from "@lingui/react/macro"
import { SettingsIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link, NavLink } from "react-router"

import { useShell } from "@/app/shell"
import { TeamMenu } from "@/app/TeamMenu"
import { UserMenu } from "@/app/UserMenu"
import { YuvaMark } from "@/components/common"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { useCounts } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { WaitingPill } from "@/features/inbox/WaitingPill"
import { cn } from "@/lib/utils"

export const navLinkClass =
  "inline-flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"

export function TopBar() {
  const { t, i18n } = useLingui()
  const { openDrawer } = useShell()
  const { inboxId } = useQueue()
  const counts = useCounts().data
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4)
    window.addEventListener("scroll", onScroll, { passive: true })
    return () => window.removeEventListener("scroll", onScroll)
  }, [])
  const all = inboxId ? (counts?.inboxes.find((x) => x.id === inboxId)?.count ?? 0) : (counts?.all ?? 0)
  return (
    <nav
      className={cn(
        "sticky top-0 z-30 grid grid-cols-[1fr_auto_1fr] items-center gap-4 border-b border-transparent bg-background/90 px-5 py-3 backdrop-blur-md phone:grid-cols-[auto_1fr_auto] phone:gap-2 phone:px-3 phone:py-2.5",
        scrolled && "border-border",
      )}
      data-testid="topbar"
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <Link
          to="/"
          className="flex items-center gap-2 rounded-lg py-1 ps-1 pe-2 text-[15px] font-semibold tracking-tight transition-colors hover:bg-muted phone:pe-1"
          aria-label={t`Back to the queue`}
        >
          <YuvaMark className="size-6" />
          <span className="phone:hidden">Yuva</span>
        </Link>
        <span aria-hidden className="mx-1 h-[18px] w-px bg-border phone:hidden" />
        <InboxPicker className="phone:hidden" />
      </div>
      <WaitingPill />
      <div className="flex items-center justify-end gap-1">
        <TeamMenu />
        <button type="button" className={navLinkClass} onClick={() => openDrawer()} data-testid="open-all">
          <span className="phone:hidden">
            <Trans>All</Trans>
          </span>
          <span className="text-xs text-faint tabular-nums">{new Intl.NumberFormat(i18n.locale, { notation: "compact" }).format(all)}</span>
        </button>
        <NavLink
          to="/settings"
          className={({ isActive }) => cn(navLinkClass, "w-8 justify-center px-0", isActive && "bg-muted text-foreground")}
          aria-label={t`Settings`}
          title={t`Settings`}
          data-testid="open-settings"
        >
          <SettingsIcon className="size-[17px]" />
        </NavLink>
        <UserMenu />
      </div>
    </nav>
  )
}

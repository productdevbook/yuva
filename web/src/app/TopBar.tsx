import { Trans, useLingui } from "@lingui/react/macro"
import { SettingsIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation } from "react-router"

import { useShell } from "@/app/shell"
import { TeamMenu } from "@/app/TeamMenu"
import { UserMenu } from "@/app/UserMenu"
import { ViewToggle } from "@/app/ViewToggle"
import { YuvaMark } from "@/components/common"
import { Button } from "@/components/ui/button"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { ListSearchBox } from "@/features/inbox/ListSearchBox"
import { useCounts } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { WaitingPill } from "@/features/inbox/WaitingPill"
import { cn } from "@/lib/utils"
import { useView } from "@/lib/view"

export function TopBar() {
  const { t, i18n } = useLingui()
  const { openDrawer } = useShell()
  const { inboxId } = useQueue()
  const counts = useCounts().data
  const [scrolled, setScrolled] = useState(false)
  const bar = useRef<HTMLElement>(null)
  useEffect(() => {
    const el = bar.current
    if (!el) return
    const ro = new ResizeObserver(() => document.documentElement.style.setProperty("--topbar", `${el.offsetHeight}px`))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4)
    window.addEventListener("scroll", onScroll, { passive: true })
    return () => window.removeEventListener("scroll", onScroll)
  }, [])
  const [view] = useView()
  const { pathname } = useLocation()
  const listing = view === "list" && pathname === "/"
  const all = inboxId ? (counts?.inboxes.find((x) => x.id === inboxId)?.count ?? 0) : (counts?.all ?? 0)
  return (
    <nav
      ref={bar}
      className={cn(
        "sticky top-0 z-30 grid grid-cols-[1fr_auto_1fr] items-center gap-4 border-b border-transparent bg-background/90 px-5 py-3 backdrop-blur-md phone:grid-cols-[auto_1fr_auto] phone:gap-2 phone:px-3 phone:py-2.5",
        (scrolled || listing) && "border-border",
      )}
      data-testid="topbar"
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <Link
          to="/"
          className="flex items-center gap-2 rounded-lg py-1 ps-1 pe-2 text-reading font-medium transition-colors hover:bg-muted phone:pe-1"
          aria-label={t`Home`}
        >
          <YuvaMark className="size-6" />
          <span className="phone:hidden">Yuva</span>
        </Link>
        <span aria-hidden className="mx-1 h-[18px] w-px bg-border phone:hidden" />
        <ViewToggle className="phone:hidden" />
        <InboxPicker className="phone:hidden" />
      </div>
      {listing ? <ListSearchBox /> : <WaitingPill />}
      <div className="flex items-center justify-end gap-1">
        <TeamMenu />
        {!listing && (
          <Button variant="ghost" size="sm" onClick={() => openDrawer()} data-testid="open-all">
            <span className="phone:hidden">
              <Trans>All</Trans>
            </span>
            <span className="text-caption text-faint tabular-nums">{new Intl.NumberFormat(i18n.locale, { notation: "compact" }).format(all)}</span>
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          className={cn(pathname.startsWith("/settings") && "bg-muted text-foreground")}
          render={<Link to="/settings" />}
          aria-label={t`Settings`}
          title={t`Settings`}
          data-testid="open-settings"
        >
          <SettingsIcon className="size-[17px]" />
        </Button>
        <UserMenu />
      </div>
    </nav>
  )
}

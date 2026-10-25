import { useLingui } from "@lingui/react/macro"
import { MenuIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { useSidebar } from "@/components/ui/sidebar"
import { useSession } from "@/lib/session"

export function TopBar() {
  const { t } = useLingui()
  const { isMobile, setOpenMobile } = useSidebar()
  const name = useSession().membership.workspace.name
  if (!isMobile) return null
  return (
    <header
      className="flex h-[49px] w-full shrink-0 items-center gap-2.5 border-b border-sidebar-border bg-sidebar px-4 text-sidebar-accent-foreground"
      data-testid="top-bar"
    >
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setOpenMobile(true)}
        aria-label={t`Menu`}
        className="-ms-1.5 size-8 rounded-lg text-sidebar-foreground hover:bg-sidebar-accent"
        data-testid="sidebar-toggle"
      >
        <MenuIcon />
      </Button>
      <span
        aria-hidden
        className="grid size-7 shrink-0 place-items-center rounded-lg bg-brand-solid text-small font-bold text-white uppercase"
      >
        {Array.from(name.trim())[0] ?? ""}
      </span>
      <span className="min-w-0 flex-1 truncate font-semibold">{name}</span>
    </header>
  )
}

import { useLingui } from "@lingui/react/macro"
import { MenuIcon, SearchIcon } from "lucide-react"
import { Link } from "react-router"

import { useSection } from "@/app/Rail"
import { useShell } from "@/app/shell"
import { BackArrow, usePhoneBack } from "@/components/common/PageLayout"
import { PhoneSelector } from "@/components/common/WorkspaceSelector"
import { useSidebar } from "@/components/ui/sidebar"
import { useSession } from "@/lib/session"

const buttonClass =
  "grid size-11 shrink-0 place-items-center rounded-lg text-foreground outline-none transition-colors hover:bg-raise focus-visible:ring-2 focus-visible:ring-ring"

export function TopBar() {
  const { t } = useLingui()
  const { isMobile, setOpenMobile } = useSidebar()
  const { homePath, canManage } = useSession()
  const section = useSection()
  const { openPalette } = useShell()
  const detail = usePhoneBack()
  if (!isMobile) return null
  const back = !!detail || (canManage && section !== "home")
  return (
    <header
      className="flex shrink-0 items-center gap-1 border-b border-line-soft bg-background px-2 py-1.5 pt-[max(0.375rem,env(safe-area-inset-top))]"
      data-testid="top-bar"
    >
      {detail && detail.to === undefined ? (
        <button
          type="button"
          onClick={detail.run}
          aria-label={detail.label}
          className={buttonClass}
          data-testid="top-back"
        >
          <BackArrow className="size-5" />
        </button>
      ) : back ? (
        <Link
          to={detail?.to ?? homePath}
          onClick={detail?.run}
          aria-label={detail ? detail.label : t`Back to Home`}
          className={buttonClass}
          data-testid="top-back"
        >
          <BackArrow className="size-5" />
        </Link>
      ) : (
        <button
          type="button"
          onClick={() => setOpenMobile(true)}
          aria-label={t`Menu`}
          className={buttonClass}
          data-testid="sidebar-toggle"
        >
          <MenuIcon className="size-5" />
        </button>
      )}
      <PhoneSelector />
      <button
        type="button"
        onClick={openPalette}
        aria-label={t`Search`}
        className={buttonClass}
        data-testid="top-bar-search"
      >
        <SearchIcon className="size-5" />
      </button>
    </header>
  )
}

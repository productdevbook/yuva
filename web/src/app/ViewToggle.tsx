import { Trans, useLingui } from "@lingui/react/macro"
import { useLocation, useNavigate } from "react-router"

import { cn } from "@/lib/utils"
import { useView, type View } from "@/lib/view"

export function ViewToggle({ className }: { className?: string }) {
  const { t } = useLingui()
  const [view, setView] = useView()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const pick = (v: View) => {
    setView(v)
    if (pathname !== "/") navigate("/")
  }
  const tabs: [View, React.ReactNode][] = [
    ["queue", <Trans>Queue</Trans>],
    ["list", <Trans>List</Trans>],
  ]
  return (
    <div role="tablist" aria-label={t`View`} className={cn("flex gap-0.5 rounded-[10px] border bg-card p-[3px]", className)} data-testid="view-toggle">
      {tabs.map(([v, label]) => {
        const on = view === v && (pathname === "/" || pathname.startsWith("/conversations/"))
        return (
          <button
            key={v}
            type="button"
            role="tab"
            aria-selected={on}
            onClick={() => pick(v)}
            title={t`Switch view (V)`}
            className={cn(
              "rounded-[7px] px-3 py-1.5 text-[13px] text-muted-foreground transition-colors hover:text-foreground",
              on && "bg-background font-medium text-foreground shadow-[0_1px_2px_rgb(0_0_0/0.06)]",
            )}
            data-testid={`view-${v}`}
          >
            {label}
          </button>
        )
      })}
    </div>
  )
}

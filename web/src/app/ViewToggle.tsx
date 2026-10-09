import { useLingui } from "@lingui/react/macro"
import { useLocation, useNavigate } from "react-router"

import { Segmented } from "@/components/common"
import { useView, type View } from "@/lib/view"

export function ViewToggle({ className }: { className?: string }) {
  const { t } = useLingui()
  const [view, setView] = useView()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const here = pathname === "/" || pathname.startsWith("/conversations/")
  return (
    <Segmented<View | "">
      look="segment"
      label={t`View`}
      value={here ? view : ""}
      onChange={(v) => {
        if (!v) return
        setView(v)
        if (pathname !== "/") navigate("/")
      }}
      items={[
        { value: "queue", label: t`Queue`, testId: "view-queue", title: t`Switch view (V)` },
        { value: "list", label: t`List`, testId: "view-list", title: t`Switch view (V)` },
      ]}
      className={className}
    />
  )
}

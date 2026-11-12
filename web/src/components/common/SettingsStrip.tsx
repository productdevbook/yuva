import { Trans } from "@lingui/react/macro"
import { Link } from "react-router"

import { useFlatSettings } from "@/features/settings/ui"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export type SettingsArea = "products" | "channels" | "developer" | "general"

export function SettingsStrip({ current }: { current: SettingsArea }) {
  const { canManage } = useSession()
  const flat = useFlatSettings()
  if (flat) return null
  const items: { key: SettingsArea; to: string; title: React.ReactNode; detail: React.ReactNode }[] = [
    {
      key: "products",
      to: "/settings",
      title: <Trans>Products</Trans>,
      detail: <Trans>Who you support: name, hours, team, sender</Trans>,
    },
    {
      key: "channels",
      to: "/settings?view=channels",
      title: <Trans>Channels</Trans>,
      detail: <Trans>How messages arrive: chat, e-mail, apps</Trans>,
    },
    {
      key: "developer",
      to: canManage ? "/settings/api-keys" : "/settings/connected-apps",
      title: canManage ? <Trans>API & MCP</Trans> : <Trans>Connected apps</Trans>,
      detail: <Trans>How software connects: keys, hooks, assistants</Trans>,
    },
    {
      key: "general",
      to: "/settings/workspace",
      title: <Trans>General</Trans>,
      detail: <Trans>The workspace: labels, replies, data</Trans>,
    },
  ]
  return (
    <nav className="grid grid-cols-4 gap-3 max-lg:grid-cols-2" data-testid="settings-strip">
      {items.map((x) => (
        <Link
          key={x.key}
          to={x.to}
          aria-current={x.key === current ? "page" : undefined}
          className={cn(
            "flex min-w-0 flex-col gap-0.5 rounded-[10px] border bg-background px-3 py-2.5 outline-none transition-colors hover:border-faint focus-visible:ring-2 focus-visible:ring-ring",
            x.key === current && "border-foreground bg-surface hover:border-foreground"
          )}
        >
          <span className="truncate font-medium">{x.title}</span>
          <span className="text-caption text-faint">{x.detail}</span>
        </Link>
      ))}
    </nav>
  )
}

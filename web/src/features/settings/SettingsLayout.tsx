import { Trans, useLingui } from "@lingui/react/macro"
import { BellIcon, BuildingIcon, InboxIcon, KeyIcon, MessageSquareTextIcon, TagIcon, UserIcon, UsersIcon, WebhookIcon } from "lucide-react"
import { NavLink, Outlet } from "react-router"

import { PaneHeader } from "@/app/shell"
import { Label } from "@/components/ui/label"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function SettingsLayout() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const items = [
    { to: "profile", label: t`My profile`, icon: UserIcon },
    { to: "notifications", label: t`Notifications`, icon: BellIcon },
    { to: "workspace", label: t`Workspace`, icon: BuildingIcon },
    { to: "members", label: t`Members`, icon: UsersIcon },
    { to: "inboxes", label: t`Inboxes`, icon: InboxIcon },
    { to: "labels", label: t`Labels`, icon: TagIcon },
    { to: "canned-replies", label: t`Canned replies`, icon: MessageSquareTextIcon },
    ...(canManage
      ? [
          { to: "api-keys", label: t`API keys`, icon: KeyIcon },
          { to: "webhooks", label: t`Webhooks`, icon: WebhookIcon },
        ]
      : []),
  ]
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PaneHeader title={<Trans>Settings</Trans>} />
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <nav
          aria-label={t`Settings`}
          className="flex shrink-0 gap-1 overflow-x-auto border-b px-3 py-2 md:w-56 md:flex-col md:overflow-visible md:border-r md:border-b-0 md:py-4"
        >
          {items.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                cn(
                  "flex shrink-0 items-center gap-2 rounded-md px-2.5 py-1.5 text-sm whitespace-nowrap transition-colors hover:bg-muted",
                  isActive && "bg-accent font-medium text-accent-foreground hover:bg-accent",
                )
              }
            >
              <Icon className="size-4" />
              {label}
            </NavLink>
          ))}
        </nav>
        <div className="min-h-0 flex-1 overflow-y-auto" data-testid="settings-scroll">
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-6 md:px-8">
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  )
}

export function Section({
  title,
  description,
  action,
  children,
  className,
}: {
  title: React.ReactNode
  description?: React.ReactNode
  action?: React.ReactNode
  children?: React.ReactNode
  className?: string
}) {
  return (
    <section className={cn("flex flex-col gap-4 rounded-xl border bg-card p-4 md:p-5", className)}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="text-base font-semibold">{title}</h2>
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  )
}

export function Field({
  label,
  htmlFor,
  hint,
  children,
  className,
}: {
  label: React.ReactNode
  htmlFor?: string
  hint?: React.ReactNode
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

export function PageTitle({ children }: { children: React.ReactNode }) {
  return <h1 className="text-xl font-semibold">{children}</h1>
}

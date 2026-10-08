import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, ChevronRightIcon } from "lucide-react"
import { Link, Navigate, NavLink, Outlet, useLocation } from "react-router"

import { PaneHeader } from "@/app/shell"
import { Button } from "@/components/ui/button"
import { useIsMobile } from "@/hooks/use-media-query"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function useGroups() {
  const { t } = useLingui()
  const { canManage } = useSession()
  return [
    {
      title: t`Account`,
      items: [
        { to: "profile", label: t`My profile` },
        { to: "notifications", label: t`Notifications` },
      ],
    },
    {
      title: t`Workspace`,
      items: [
        { to: "workspace", label: t`General` },
        { to: "members", label: t`Members` },
        { to: "inboxes", label: t`Inboxes` },
        { to: "labels", label: t`Labels` },
        { to: "canned-replies", label: t`Canned replies` },
        ...(canManage
          ? [
              { to: "api-keys", label: t`API keys` },
              { to: "webhooks", label: t`Webhooks` },
            ]
          : []),
      ],
    },
  ]
}

function SettingsNav({ list }: { list?: boolean }) {
  const { t } = useLingui()
  const groups = useGroups()
  return (
    <nav aria-label={t`Settings`} className="flex flex-col gap-6">
      {groups.map((g) => (
        <div key={g.title}>
          <h2 className={cn("mb-1 text-xs font-medium text-faint", list ? "px-1" : "px-2.5")}>{g.title}</h2>
          <ul className={cn("flex flex-col", list ? "divide-y rounded-2xl border" : "gap-px")}>
            {g.items.map((it) => (
              <li key={it.to}>
                <NavLink
                  to={`/settings/${it.to}`}
                  className={({ isActive }) =>
                    list
                      ? "flex h-12 items-center justify-between px-4 text-[0.95rem]"
                      : cn(
                          "flex h-8 items-center rounded-lg px-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                          isActive && "bg-muted font-medium text-foreground",
                        )
                  }
                >
                  {it.label}
                  {list && <ChevronRightIcon className="size-4 text-faint rtl:rotate-180" />}
                </NavLink>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  )
}

export function SettingsIndex() {
  const isMobile = useIsMobile()
  if (!isMobile) return <Navigate to="profile" replace />
  return null
}

export function SettingsLayout() {
  const { t } = useLingui()
  const isMobile = useIsMobile()
  const { pathname } = useLocation()
  const atIndex = pathname.replace(/\/$/, "") === "/settings"
  if (isMobile && atIndex) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <PaneHeader title={<Trans>Settings</Trans>} />
        <div className="min-h-0 flex-1 overflow-y-auto px-4 pt-2 pb-8">
          <SettingsNav list />
        </div>
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-1">
      <aside className="hidden w-56 shrink-0 flex-col border-e md:flex">
        <PaneHeader title={<Trans>Settings</Trans>} />
        <div className="min-h-0 flex-1 overflow-y-auto px-3 pt-1 pb-6">
          <SettingsNav />
        </div>
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PaneHeader
          className="md:hidden"
          title={<Trans>Settings</Trans>}
          leading={
            <Button variant="ghost" size="icon-sm" className="-ms-1.5" render={<Link to="/settings" />} aria-label={t`Back`}>
              <ArrowLeftIcon />
            </Button>
          }
        />
        <div className="min-h-0 flex-1 overflow-y-auto" data-testid="settings-scroll">
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-10 px-4 pt-4 pb-16 sm:px-8 md:pt-10">
            <Outlet />
          </div>
        </div>
      </div>
    </div>
  )
}

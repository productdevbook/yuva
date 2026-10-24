import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, ChevronRightIcon, PlusIcon, SettingsIcon, XIcon } from "lucide-react"
import { Fragment, useState } from "react"
import { Link, useLocation, useMatch, useNavigate } from "react-router"

import { moveFocus, useEntries, useGo, useSection } from "@/app/Rail"
import { ViewsGroup } from "@/app/ViewsGroup"
import type { Section } from "@/app/shell"
import { keyLabel, mod } from "@/components/common/ShortcutSheet"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
  useSidebar,
} from "@/components/ui/sidebar"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useViewSearch } from "@/features/analytics/state"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { ProductSwitcher } from "@/features/inbox/ProductSwitcher"
import { useVersion } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function useOpenGroups(section: string) {
  const [state, setState] = useState<{ section: string; open: Record<string, boolean> }>({ section, open: {} })
  const open = state.section === section ? state.open : {}
  const set = (key: string, value: boolean) => setState({ section, open: { ...open, [key]: value } })
  return [open, set] as const
}

const rowClass =
  "h-8 gap-2.5 px-2.5 text-body in-data-[mobile=true]:h-12 in-data-[mobile=true]:gap-3.5 in-data-[mobile=true]:px-3 in-data-[mobile=true]:text-base in-data-[mobile=true]:[&_svg]:size-5 text-sidebar-foreground data-active:bg-sidebar-primary data-active:font-medium data-active:text-sidebar-primary-foreground data-active:hover:bg-sidebar-primary data-active:hover:text-sidebar-primary-foreground [&_svg]:size-4.5"

function Count({ n, id }: { n: number; id: string }) {
  if (!n) return null
  return (
    <SidebarMenuBadge
      className="right-1.5 h-[18px] in-data-[mobile=true]:top-3.5! in-data-[mobile=true]:right-3 in-data-[mobile=true]:h-5 in-data-[mobile=true]:min-w-5 in-data-[mobile=true]:text-xs min-w-[18px] rounded-full bg-sidebar-badge px-1.5 text-[11px] leading-none font-semibold text-white peer-hover/menu-button:text-white peer-data-active/menu-button:text-white group-data-[collapsible=icon]:-right-1 group-data-[collapsible=icon]:-top-1! group-data-[collapsible=icon]:flex group-data-[collapsible=icon]:h-4 group-data-[collapsible=icon]:min-w-4 group-data-[collapsible=icon]:px-1 group-data-[collapsible=icon]:text-[10px]"
      data-testid={`rail-badge-${id}`}
    >
      {n > 99 ? "99+" : n}
    </SidebarMenuBadge>
  )
}

function WorkspaceMark({ name, className }: { name: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "grid size-6 shrink-0 place-items-center rounded-md bg-brand text-caption font-semibold text-white uppercase",
        className
      )}
    >
      {Array.from(name.trim())[0] ?? ""}
    </span>
  )
}

function WorkspaceSwitcher() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { me, membership, switchWorkspace } = useSession()
  const signup = useVersion().data?.signup
  const name = membership.workspace.name
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                size="lg"
                className="h-10 gap-2.5 px-2 data-popup-open:bg-sidebar-accent in-data-[mobile=true]:h-12 in-data-[mobile=true]:text-base group-data-[collapsible=icon]:justify-center"
              />
            }
            aria-label={t`Workspace: ${name}`}
            data-testid="workspace-switcher"
          >
            <WorkspaceMark name={name} />
            <span
              className="min-w-0 flex-1 truncate font-semibold text-sidebar-accent-foreground group-data-[collapsible=icon]:hidden"
              data-testid="sidebar-workspace"
            >
              {name}
            </span>
            <ChevronDownIcon className="text-sidebar-muted group-data-[collapsible=icon]:hidden" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-(--anchor-width) min-w-60" data-testid="workspace-menu">
            <DropdownMenuGroup>
              <DropdownMenuLabel>
                <Trans>Workspaces</Trans>
              </DropdownMenuLabel>
              {me.memberships.map((m) => {
                const current = m.workspace.id === membership.workspace.id
                return (
                  <DropdownMenuItem
                    key={m.workspace.id}
                    onClick={() => !current && switchWorkspace(m.workspace.id)}
                    aria-current={current ? "true" : undefined}
                    data-testid="workspace-option"
                  >
                    <WorkspaceMark name={m.workspace.name} className="size-5 rounded-[5px] text-[10px]" />
                    <span className="min-w-0 flex-1 truncate">{m.workspace.name}</span>
                    <CheckIcon className={cn("size-3.5 text-foreground", !current && "invisible")} />
                  </DropdownMenuItem>
                )
              })}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            {signup === "open" && (
              <DropdownMenuItem onClick={() => navigate("/workspaces/new")} data-testid="workspace-create">
                <PlusIcon />
                <Trans>Create a workspace</Trans>
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onClick={() => navigate("/settings/workspace")} data-testid="workspace-settings">
              <SettingsIcon />
              <Trans>Workspace settings</Trans>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}

export type Child = { key: string; label: string; to: string; current: boolean }

function useStatusChildren(): Child[] {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const at = (path: string) => pathname === path || pathname.startsWith(`${path}/`)
  return [
    { key: "pages", label: t`Pages`, to: "/status", current: !at("/incidents") && !at("/monitors") },
    { key: "incidents", label: t`Incidents`, to: "/incidents", current: at("/incidents") },
    { key: "monitors", label: t`Monitors`, to: "/monitors", current: at("/monitors") },
  ]
}

function useBroadcastChildren(): Child[] {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const current = pathname.startsWith("/broadcasts/lists")
    ? "lists"
    : pathname.startsWith("/broadcasts/designs")
      ? "designs"
      : "broadcasts"
  return [
    { key: "broadcasts", label: t`Broadcasts`, to: "/broadcasts" },
    { key: "lists", label: t`Lists`, to: "/broadcasts/lists" },
    { key: "designs", label: t`Designs`, to: "/broadcasts/designs" },
  ].map((c) => ({ ...c, current: pathname.startsWith("/broadcasts") && current === c.key }))
}

function useAnalyticsChildren(): Child[] {
  const { t } = useLingui()
  const { pathname } = useLocation()
  const search = useViewSearch(true)
  const [inboxId] = useInboxFilter()
  const properties = (useAnalyticsProperties().data?.items ?? []).filter((p) => !inboxId || p.inbox_id === inboxId)
  const matched = useMatch("/analytics/:propertyId/*")?.params.propertyId
  const propertyId = matched && matched !== "new" ? matched : undefined
  const target = propertyId ?? properties[0]?.id
  const view = propertyId ? (pathname.split("/")[3] ?? "overview") : pathname === "/analytics" ? "overview" : ""
  const keep = propertyId ? search : ""
  const to = (v: string) => (target ? `/analytics/${target}${v ? `/${v}` : ""}${keep}` : "/analytics")
  return [
    { key: "overview", label: t`Overview`, to: to(""), current: view === "overview" },
    { key: "realtime", label: t`Realtime`, to: to("realtime"), current: view === "realtime" },
    { key: "pages", label: t`Pages`, to: to("pages"), current: view === "pages" },
    { key: "sources", label: t`Sources`, to: to("sources"), current: view === "sources" },
    { key: "locations", label: t`Locations`, to: to("locations"), current: view === "locations" },
    { key: "devices", label: t`Devices`, to: to("devices"), current: view === "devices" },
    { key: "events", label: t`Events`, to: to("events"), current: view === "events" },
    { key: "goals", label: t`Goals`, to: to("goals"), current: view === "goals" },
    { key: "funnels", label: t`Funnels`, to: to("funnels"), current: view === "funnels" },
    { key: "retention", label: t`Retention`, to: to("retention"), current: view === "retention" },
    { key: "support", label: t`Support`, to: to("support"), current: view === "support" },
    { key: "settings", label: t`Settings`, to: to("settings"), current: view === "settings" },
  ]
}

function useSettingsChildren(): Child[] {
  const { t } = useLingui()
  const { pathname, search } = useLocation()
  const { canManage } = useSession()
  const channels = pathname.includes("/channels/") || (pathname === "/settings" && search.includes("view=channels"))
  const at = (...paths: string[]) => paths.some((p) => pathname === p || pathname.startsWith(`${p}/`))
  return [
    {
      key: "products",
      label: t`Products`,
      to: "/settings",
      current: !channels && (pathname === "/settings" || at("/settings/inboxes")),
    },
    { key: "channels", label: t`Channels`, to: "/settings?view=channels", current: channels },
    {
      key: "developer",
      label: canManage ? t`API & MCP` : t`Connected apps`,
      to: canManage ? "/settings/api-keys" : "/settings/connected-apps",
      current: at("/settings/api-keys", "/settings/webhooks", "/settings/connected-apps"),
    },
    {
      key: "workspace",
      label: t`General`,
      to: "/settings/workspace",
      current: at(
        "/settings/workspace",
        "/settings/labels",
        "/settings/canned-replies",
        "/settings/migrations",
        "/settings/audit-log"
      ),
    },
  ]
}

export type NavRow = {
  key: Section
  label: string
  icon: React.ComponentType<{ className?: string }>
  title: string
  badge?: number
  badgeLabel?: string
  children?: Child[]
}

export type NavGroup = { title?: string; rows: NavRow[] }

export function useNavGroups(): NavGroup[] {
  const { mailFirst } = useSession()
  const { t } = useLingui()
  const entries = useEntries()
  const children: Partial<Record<Section, Child[]>> = {
    status: useStatusChildren(),
    broadcasts: useBroadcastChildren(),
    analytics: useAnalyticsChildren(),
    settings: useSettingsChildren(),
  }
  const labels: Partial<Record<Section, string>> = {
    conversations: t`Inbox`,
    knowledge: t`Help center`,
    status: t`Status pages`,
  }
  const rows = new Map<Section, NavRow>(
    entries.map((e) => [
      e.key,
      {
        ...e,
        label: labels[e.key] ?? e.label,
        title: keyLabel(e.shortcut).join(" "),
        children: children[e.key],
      },
    ])
  )
  rows.set("settings", {
    key: "settings",
    label: t`Settings`,
    icon: SettingsIcon,
    title: `${mod} ,`,
    children: children.settings,
  })
  const groups: { title?: string; keys: Section[] }[] = [
    {
      keys: mailFirst
        ? ["home", "mail", "conversations", "mentions", "assistants"]
        : ["home", "conversations", "mail", "mentions", "assistants"],
    },
    { title: t`Customers`, keys: ["contacts", "feedback", "knowledge"] },
    {
      title: t({ message: "Publish", context: "sidebar group" }),
      keys: ["status", "updates", "board", "broadcasts", "docs"],
    },
    { title: t`Insights`, keys: ["analytics", "reports"] },
    { title: t`Workspace`, keys: ["team", "settings"] },
  ]
  return groups
    .map((g) => ({ title: g.title, rows: g.keys.flatMap((k) => rows.get(k) ?? []) }))
    .filter((g) => g.rows.length > 0)
}

function Children({ section, items }: { section: string; items: Child[] }) {
  return (
    <SidebarMenuSub
      className="mx-0 mt-0.5 mb-1 translate-x-0 gap-px rounded-md border-s-0 bg-sidebar-accent py-1 ps-8 pe-1 in-data-[mobile=true]:ps-11"
      data-testid={`rail-${section}-children`}
    >
      {items.map((c) => (
        <SidebarMenuSubItem key={c.key}>
          <SidebarMenuSubButton
            render={<Link to={c.to} />}
            isActive={c.current}
            aria-current={c.current ? "page" : undefined}
            className="h-7 translate-x-0 text-small in-data-[mobile=true]:h-11 in-data-[mobile=true]:text-base text-sidebar-foreground/80 hover:bg-transparent hover:text-sidebar-accent-foreground data-active:bg-transparent data-active:font-semibold data-active:text-sidebar-accent-foreground"
            data-rail-item
            data-testid={`rail-${section}-${c.key}`}
          >
            <span>{c.label}</span>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
      ))}
    </SidebarMenuSub>
  )
}

function Row({
  row,
  active,
  open,
  onOpen,
}: {
  row: NavRow
  active: boolean
  open: boolean
  onOpen: (open: boolean) => void
}) {
  const go = useGo()
  const { state } = useSidebar()
  const collapsed = state === "collapsed"
  const kids = row.children
  const tooltip = {
    children: (
      <>
        {row.label}
        <span className="text-background/60">{row.title}</span>
      </>
    ),
  }
  const common = {
    tooltip,
    title: collapsed ? undefined : `${row.label} · ${row.title}`,
    "aria-label": row.badge ? row.badgeLabel : undefined,
    "data-rail-item": true,
    "data-testid": `rail-${row.key}`,
  }
  if (!kids)
    return (
      <SidebarMenuItem>
        <SidebarMenuButton
          {...common}
          className={rowClass}
          isActive={active}
          aria-current={active ? "page" : undefined}
          onClick={() => go(row.key)}
        >
          <row.icon />
          <span>{row.label}</span>
        </SidebarMenuButton>
        <Count n={row.badge ?? 0} id={row.key} />
      </SidebarMenuItem>
    )
  return (
    <Collapsible
      open={open && !collapsed}
      onOpenChange={(next) => !collapsed && onOpen(next)}
      render={<SidebarMenuItem />}
    >
      <CollapsibleTrigger
        render={
          <SidebarMenuButton
            {...common}
            className={cn(rowClass, active && "font-medium text-sidebar-accent-foreground")}
            onClick={collapsed ? () => go(row.key) : undefined}
          />
        }
      >
        <row.icon />
        <span className="min-w-0 flex-1 truncate">{row.label}</span>
        <ChevronRightIcon
          aria-hidden
          className="ms-auto size-4! text-sidebar-muted transition-transform group-data-panel-open/menu-button:rotate-90 rtl:-scale-x-100 rtl:group-data-panel-open/menu-button:-rotate-90"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Children section={row.key} items={kids} />
      </CollapsibleContent>
    </Collapsible>
  )
}

export function AppSidebar() {
  const { t } = useLingui()
  const section = useSection()
  const groups = useNavGroups()
  const [open, setOpen] = useOpenGroups(section)
  const { state, isMobile, setOpenMobile } = useSidebar()
  const collapsed = state === "collapsed"
  const toggleLabel = collapsed ? t`Expand the sidebar` : t`Collapse the sidebar`
  return (
    <Sidebar
      collapsible="icon"
      className="relative h-auto group-data-[side=left]:border-r-0 group-data-[side=right]:border-l-0"
      data-testid="sidebar"
    >
      {isMobile && (
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-sidebar-border ps-5 pe-2 pt-[env(safe-area-inset-top)]">
          <span className="text-base font-semibold text-sidebar-accent-foreground">{t`Menu`}</span>
          <button
            type="button"
            onClick={() => setOpenMobile(false)}
            className="grid size-11 place-items-center rounded-md text-sidebar-foreground outline-none hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            aria-label={t`Close`}
            data-testid="sidebar-close"
          >
            <XIcon className="size-5" />
          </button>
        </div>
      )}
      <SidebarHeader className="gap-2 px-3 pt-3 pb-1 group-data-[collapsible=icon]:px-2">
        <WorkspaceSwitcher />
        <div className="empty:hidden group-data-[collapsible=icon]:hidden">
          <ProductSwitcher />
        </div>
      </SidebarHeader>
      <SidebarContent className="overflow-visible in-data-[mobile=true]:overflow-y-auto in-data-[mobile=true]:pb-[max(1rem,env(safe-area-inset-bottom))]">
        <nav aria-label={t`Sections`} onKeyDown={moveFocus} className="flex flex-col" data-testid="rail">
          {groups.map((g, at) => (
            <Fragment key={g.title ?? "main"}>
              <SidebarGroup className="px-3 py-1 group-data-[collapsible=icon]:px-2" role="group" aria-label={g.title}>
                {g.title && (
                  <SidebarGroupLabel className="h-7 px-2.5 text-[11px] tracking-[0.06em] text-sidebar-muted uppercase">
                    {g.title}
                  </SidebarGroupLabel>
                )}
                <SidebarMenu className="gap-px">
                  {g.rows.map((r) => (
                    <Row
                      key={r.key}
                      row={r}
                      active={section === r.key}
                      open={open[r.key] ?? section === r.key}
                      onOpen={(v) => setOpen(r.key, v)}
                    />
                  ))}
                </SidebarMenu>
              </SidebarGroup>
              {at === 0 && <ViewsGroup />}
            </Fragment>
          ))}
        </nav>
      </SidebarContent>
      <SidebarRail aria-label={toggleLabel} title={`${toggleLabel} · ${mod} \\`} />
    </Sidebar>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import {
  CheckIcon,
  ChevronRightIcon,
  ChevronsUpDownIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  XIcon,
} from "lucide-react"
import { Fragment, useCallback, useEffect, useRef, useState } from "react"
import { Link, useLocation, useMatch, useNavigate } from "react-router"

import { mainKeys, moveFocus, useEntries, useGo, useSection } from "@/app/Rail"
import { useShell, type Section } from "@/app/shell"
import { UserMenu } from "@/app/UserMenu"
import { ViewsGroup } from "@/app/ViewsGroup"
import { keyLabel, mod } from "@/components/common/ShortcutSheet"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { Kbd, KbdGroup } from "@/components/ui/kbd"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarRail,
  SidebarSeparator,
  useSidebar,
} from "@/components/ui/sidebar"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useViewSearch } from "@/features/analytics/state"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { ProductScope, ProductSwitcher } from "@/features/inbox/ProductSwitcher"
import { useVersion } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

function useOpenGroups(section: string) {
  const [state, setState] = useState<{ section: string; open: Record<string, boolean> }>({ section, open: {} })
  const open = state.section === section ? state.open : {}
  const set = (key: string, value: boolean) => setState({ section, open: { ...open, [key]: value } })
  return [open, set] as const
}

const collapsedButton =
  "group-data-[collapsible=icon]:size-full! group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:p-2!"

export const rowClass = cn(
  "relative h-[34px] gap-2.5 rounded-lg px-2.5 text-body text-sidebar-foreground [&_svg]:size-[17px] [&_svg]:text-sidebar-foreground",
  "data-active:bg-sidebar-active data-active:font-semibold data-active:text-brand-ink data-active:shadow-[inset_0_0_0_1px_var(--sidebar-active-ring)] data-active:hover:bg-sidebar-active data-active:hover:text-brand-ink data-active:[&_svg]:text-brand-ink",
  "data-active:before:absolute data-active:before:inset-y-2 data-active:before:start-0 data-active:before:w-[3px] data-active:before:rounded-e-[3px] data-active:before:bg-brand",
  "in-data-[mobile=true]:h-11 in-data-[mobile=true]:gap-3.5 in-data-[mobile=true]:px-3 in-data-[mobile=true]:text-base in-data-[mobile=true]:[&_svg]:size-5",
  collapsedButton
)

const quietRowClass =
  "data-active:bg-transparent data-active:text-sidebar-accent-foreground data-active:shadow-none data-active:before:hidden data-active:hover:bg-sidebar-accent data-active:hover:text-sidebar-accent-foreground"

export const groupLabelClass =
  "h-auto px-2.5 pt-1 pb-1.5 text-[11px] font-semibold tracking-[0.06em] text-sidebar-muted uppercase"

export function KeyHint({ keys, className }: { keys: string[]; className?: string }) {
  return (
    <KbdGroup aria-hidden className={className}>
      {keys.map((k) => (
        <Kbd key={k} className="h-auto min-w-0 bg-transparent px-0 font-mono text-[11px] font-normal text-inherit">
          {k}
        </Kbd>
      ))}
    </KbdGroup>
  )
}

function Count({ n, hot, id }: { n: number; hot?: boolean; id: string }) {
  if (!n) return null
  return (
    <Badge
      className={cn(
        "h-5 min-w-5 shrink-0 rounded-full px-1.5 text-caption font-semibold tabular-nums group-data-[collapsible=icon]:hidden",
        hot ? "bg-sidebar-badge text-white" : "bg-sidebar-chip text-sidebar-foreground"
      )}
      data-testid={`rail-badge-${id}`}
    >
      {n > 99 ? "99+" : n}
    </Badge>
  )
}

function WorkspaceMark({ name, className }: { name: string; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "grid size-7 shrink-0 place-items-center rounded-lg bg-brand-solid text-small font-bold text-white uppercase",
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
                tooltip={name}
                className={cn(
                  "h-11 gap-2.5 rounded-[10px] px-2 data-popup-open:bg-sidebar-accent in-data-[mobile=true]:h-12",
                  collapsedButton,
                  "group-data-[collapsible=icon]:p-1!"
                )}
              />
            }
            aria-label={t`Workspace: ${name}`}
            data-testid="workspace-switcher"
          >
            <WorkspaceMark name={name} />
            <span className="flex min-w-0 flex-1 flex-col leading-4 group-data-[collapsible=icon]:hidden">
              <span className="truncate font-semibold text-sidebar-accent-foreground" data-testid="sidebar-workspace">
                {name}
              </span>
              <span className="truncate text-caption text-sidebar-muted">
                <Trans>Workspace</Trans>
              </span>
            </span>
            <ChevronsUpDownIcon className="text-sidebar-muted group-data-[collapsible=icon]:hidden" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="w-(--anchor-width) min-w-62" data-testid="workspace-menu">
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
                    <WorkspaceMark name={m.workspace.name} className="size-[22px] rounded-md text-[11px]" />
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

function SearchButton() {
  const { t } = useLingui()
  const { openPalette } = useShell()
  const { isMobile, setOpenMobile } = useSidebar()
  const label = t`Search or jump to…`
  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          variant="outline"
          tooltip={{
            children: (
              <>
                {label}
                <KeyHint keys={[mod, "K"]} className="text-background/60" />
              </>
            ),
          }}
          aria-label={`${label} ${mod} K`}
          onClick={() => {
            if (isMobile) setOpenMobile(false)
            openPalette()
          }}
          className={cn(
            "h-9 gap-2 rounded-[10px] bg-card ps-2.5 pe-2 text-small text-sidebar-muted hover:text-sidebar-accent-foreground in-data-[mobile=true]:h-11 in-data-[mobile=true]:text-base [&_svg]:size-[15px]",
            collapsedButton
          )}
          data-testid="top-search"
        >
          <SearchIcon />
          <span className="min-w-0 flex-1 truncate group-data-[collapsible=icon]:hidden">{label}</span>
          <KeyHint
            keys={[mod, "K"]}
            className="rounded-[5px] border border-sidebar-border bg-sidebar px-1.5 py-px text-sidebar-muted group-data-[collapsible=icon]:hidden in-data-[mobile=true]:hidden"
          />
        </SidebarMenuButton>
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
      : pathname.startsWith("/broadcasts/triggered")
        ? "triggered"
        : "broadcasts"
  return [
    { key: "broadcasts", label: t`Broadcasts`, to: "/broadcasts" },
    { key: "triggered", label: t`Triggered`, to: "/broadcasts/triggered" },
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

function useReportsChildren(): Child[] {
  const { t } = useLingui()
  const { pathname } = useLocation()
  return [
    { key: "today", label: t`Today` },
    { key: "trends", label: t`Trends` },
    { key: "ratings", label: t`Ratings` },
    { key: "members", label: t`Members` },
  ].map((c) => ({
    ...c,
    to: `/reports/${c.key}`,
    current: pathname === `/reports/${c.key}` || (c.key === "today" && pathname === "/reports"),
  }))
}

export type NavRow = {
  key: Section
  label: string
  icon: React.ComponentType<{ className?: string }>
  keys: string[]
  badge?: number
  badgeLabel?: string
  hot?: boolean
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
    reports: useReportsChildren(),
    settings: useSettingsChildren(),
  }
  const rows = new Map<Section, NavRow>(
    entries.map((e) => [e.key, { ...e, keys: keyLabel(e.shortcut), children: children[e.key] }])
  )
  rows.set("settings", {
    key: "settings",
    label: t`Settings`,
    icon: SettingsIcon,
    keys: [mod, ","],
    children: children.settings,
  })
  const groups: { title?: string; keys: Section[] }[] = [
    { keys: mainKeys(mailFirst) },
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
      className="relative mx-0 ms-[18px] mt-px mb-1 translate-x-0 gap-px border-l-0 ps-3 pe-0 py-0 before:absolute before:inset-y-0 before:start-0 before:w-px before:bg-sidebar-border"
      data-testid={`rail-${section}-children`}
    >
      {items.map((c) => (
        <SidebarMenuSubItem key={c.key}>
          <SidebarMenuSubButton
            render={<Link to={c.to} />}
            isActive={c.current}
            aria-current={c.current ? "page" : undefined}
            className="relative h-[30px] translate-x-0 overflow-visible rounded-lg px-2.5 text-small text-sidebar-foreground in-data-[mobile=true]:h-11 in-data-[mobile=true]:text-base data-active:bg-transparent data-active:font-semibold data-active:text-brand-ink data-active:before:absolute data-active:before:inset-y-1.5 data-active:before:-start-3 data-active:before:w-0.5 data-active:before:rounded-full data-active:before:bg-brand data-active:hover:bg-sidebar-accent data-active:hover:text-brand-ink"
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

const revealGap = 20

function revealGroup(group: HTMLElement | null) {
  const scroller = group?.closest<HTMLElement>('[data-sidebar="content"]')
  if (!group || !scroller) return
  const g = group.getBoundingClientRect()
  const s = scroller.getBoundingClientRect()
  let delta = 0
  if (g.height + 2 * revealGap >= s.height || g.top < s.top + revealGap) delta = g.top - s.top - revealGap
  else if (g.bottom > s.bottom - revealGap) delta = g.bottom - s.bottom + revealGap
  if (Math.abs(delta) > 1) scroller.scrollBy({ top: delta, behavior: "auto" })
}

function RailScroll({ children }: { children: React.ReactNode }) {
  const scroller = useRef<HTMLDivElement>(null)
  const [edges, setEdges] = useState({ top: false, bottom: false })
  const update = useCallback(() => {
    const el = scroller.current
    if (!el) return
    const next = { top: el.scrollTop > 1, bottom: el.scrollTop + el.clientHeight < el.scrollHeight - 1 }
    setEdges((prev) => (prev.top === next.top && prev.bottom === next.bottom ? prev : next))
  }, [])
  useEffect(() => {
    const el = scroller.current
    if (!el) return
    const observer = new ResizeObserver(update)
    observer.observe(el)
    if (el.firstElementChild) observer.observe(el.firstElementChild)
    update()
    return () => observer.disconnect()
  }, [update])
  return (
    <div
      className={cn(
        "relative flex min-h-0 flex-1 flex-col",
        "before:pointer-events-none before:absolute before:inset-x-0 before:top-0 before:z-10 before:h-7 before:bg-linear-to-b before:from-sidebar before:to-transparent before:opacity-0 before:shadow-[inset_0_1px_0_var(--sidebar-border)] before:transition-opacity data-[top=true]:before:opacity-100",
        "after:pointer-events-none after:absolute after:inset-x-0 after:bottom-0 after:z-10 after:h-7 after:bg-linear-to-t after:from-sidebar after:to-transparent after:opacity-0 after:shadow-[inset_0_-1px_0_var(--sidebar-border)] after:transition-opacity data-[bottom=true]:after:opacity-100"
      )}
      data-top={edges.top}
      data-bottom={edges.bottom}
      data-testid="rail-scroll"
    >
      <SidebarContent
        ref={scroller}
        onScroll={update}
        className="pt-3.5 pb-3 group-data-[collapsible=icon]:overflow-y-auto in-data-[mobile=true]:pb-[max(1rem,env(safe-area-inset-bottom))]"
      >
        {children}
      </SidebarContent>
    </div>
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
  const { t } = useLingui()
  const go = useGo()
  const { state } = useSidebar()
  const collapsed = state === "collapsed"
  const kids = row.children
  const quiet = !!kids?.some((c) => c.current)
  const item = useRef<HTMLElement | null>(null)
  const shown = !!kids && open && !collapsed
  useEffect(() => {
    if (!shown || !item.current) return
    const el = item.current
    revealGroup(el)
    const observer = new ResizeObserver(() => revealGroup(el))
    observer.observe(el)
    const stop = setTimeout(() => observer.disconnect(), 400)
    return () => {
      clearTimeout(stop)
      observer.disconnect()
    }
  }, [shown])
  const leaf = !kids
  useEffect(() => {
    if (active && leaf) revealGroup(item.current)
  }, [active, leaf])
  const n = row.badge ?? 0
  const label = row.label
  const button = (
    <SidebarMenuButton
      tooltip={{
        children: (
          <>
            {n ? row.badgeLabel : label}
            <KeyHint keys={row.keys} className="text-background/60" />
          </>
        ),
      }}
      aria-label={n ? row.badgeLabel : label}
      className={cn(rowClass, quiet && quietRowClass, kids && "pe-9")}
      isActive={active}
      aria-current={active ? "page" : undefined}
      onClick={() => go(row.key)}
      data-rail-item
      data-testid={`rail-${row.key}`}
    >
      <span className="relative flex shrink-0">
        <row.icon />
        {n > 0 && (
          <span
            aria-hidden
            className={cn(
              "absolute -top-[3px] -end-1 hidden size-2 rounded-full ring-2 ring-sidebar group-data-[collapsible=icon]:block",
              row.hot ? "bg-sidebar-badge" : "bg-sidebar-key"
            )}
          />
        )}
      </span>
      <span className="min-w-0 flex-1 truncate group-data-[collapsible=icon]:hidden">{label}</span>
      <KeyHint
        keys={row.keys}
        className="text-sidebar-key opacity-0 transition-opacity group-hover/menu-button:opacity-100 group-focus-visible/menu-button:opacity-100 group-data-[collapsible=icon]:hidden in-data-[mobile=true]:hidden"
      />
      <Count n={n} hot={row.hot} id={row.key} />
    </SidebarMenuButton>
  )
  const setItem = (el: HTMLElement | null) => {
    item.current = el
  }
  if (!kids) return <SidebarMenuItem ref={setItem}>{button}</SidebarMenuItem>
  return (
    <Collapsible
      ref={setItem}
      open={open && !collapsed}
      onOpenChange={(next) => !collapsed && onOpen(next)}
      render={<SidebarMenuItem />}
    >
      {button}
      <CollapsibleTrigger
        render={
          <SidebarMenuAction className="top-0 right-auto end-0.5 h-[34px] w-7 text-sidebar-muted peer-data-[size=default]/menu-button:top-0 hover:bg-transparent in-data-[mobile=true]:h-11 in-data-[mobile=true]:w-9" />
        }
        aria-label={t`Sub-pages of ${label}`}
        data-testid={`rail-${row.key}-toggle`}
      >
        <ChevronRightIcon
          aria-hidden
          className="size-[13px]! transition-transform in-data-panel-open:rotate-90 rtl:-scale-x-100 rtl:in-data-panel-open:-rotate-90"
        />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <Children section={row.key} items={kids} />
      </CollapsibleContent>
    </Collapsible>
  )
}

function CollapseButton() {
  const { t } = useLingui()
  const { state, toggleSidebar } = useSidebar()
  const collapsed = state === "collapsed"
  const label = collapsed ? t`Expand the sidebar` : t`Collapse the sidebar`
  const Icon = collapsed ? PanelLeftOpenIcon : PanelLeftCloseIcon
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            onClick={toggleSidebar}
            aria-label={label}
            aria-pressed={!collapsed}
            className="size-9 shrink-0 rounded-lg text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground in-data-[mobile=true]:hidden rtl:[&_svg]:-scale-x-100"
            data-testid="sidebar-toggle"
          />
        }
      >
        <Icon className="size-[17px]" />
      </TooltipTrigger>
      <TooltipContent side={collapsed ? "right" : "top"}>
        {label}
        <KeyHint keys={[mod, "\\"]} className="text-background/60" />
      </TooltipContent>
    </Tooltip>
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
      className="sticky top-0 h-svh border-e border-sidebar-border group-data-[side=left]:border-r-0 group-data-[side=right]:border-l-0"
      data-testid="sidebar"
    >
      {isMobile && (
        <div className="flex h-14 shrink-0 items-center justify-between ps-5 pe-2 pt-[env(safe-area-inset-top)]">
          <span className="text-base font-semibold text-sidebar-accent-foreground">{t`Menu`}</span>
          <button
            type="button"
            onClick={() => setOpenMobile(false)}
            className="grid size-11 place-items-center rounded-lg text-sidebar-foreground outline-none hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-sidebar-ring"
            aria-label={t`Close`}
            data-testid="sidebar-close"
          >
            <XIcon className="size-5" />
          </button>
        </div>
      )}
      <SidebarHeader className="shrink-0 gap-3.5 px-2.5 pt-3 pb-0">
        <WorkspaceSwitcher />
        <ProductScope />
        <SearchButton />
        <div className="empty:hidden group-data-[collapsible=icon]:hidden">
          <ProductSwitcher />
        </div>
      </SidebarHeader>
      <RailScroll>
        <nav aria-label={t`Sections`} onKeyDown={moveFocus} className="flex flex-col gap-3.5" data-testid="rail">
          {groups.map((g, at) => (
            <Fragment key={g.title ?? "main"}>
              <SidebarGroup className="px-2.5 py-0" role="group" aria-label={g.title}>
                {g.title && (
                  <>
                    <SidebarGroupLabel className={groupLabelClass}>{g.title}</SidebarGroupLabel>
                    <SidebarSeparator className="mx-2 mt-0.5 mb-1.5 hidden group-data-[collapsible=icon]:block" />
                  </>
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
                {at === 0 && <ViewsGroup />}
              </SidebarGroup>
            </Fragment>
          ))}
        </nav>
      </RailScroll>
      <SidebarFooter className="shrink-0 flex-row items-center gap-1.5 border-t border-sidebar-border py-3 ps-4 pe-2.5 group-data-[collapsible=icon]:flex-col group-data-[collapsible=icon]:gap-1 group-data-[collapsible=icon]:px-0 group-data-[collapsible=icon]:py-2.5 in-data-[mobile=true]:pb-[max(0.75rem,env(safe-area-inset-bottom))]">
        <div className="flex min-w-0 flex-1 group-data-[collapsible=icon]:flex-none">
          <UserMenu row align="start" side={collapsed && !isMobile ? "inline-end" : "top"} />
        </div>
        <CollapseButton />
      </SidebarFooter>
      <SidebarRail aria-label={toggleLabel} title={`${toggleLabel} · ${mod} \\`} />
    </Sidebar>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import {
  BuildingIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  InboxIcon,
  KeyboardIcon,
  MessageSquareHeartIcon,
  LogOutIcon,
  RefreshCwIcon,
  WifiOffIcon,
  SettingsIcon,
  ShieldAlertIcon,
  TagIcon,
  UserIcon,
  UserXIcon,
} from "lucide-react"
import { Fragment, useCallback, useEffect, useState } from "react"
import { Link, Navigate, Outlet, useLocation, useNavigate } from "react-router"

import { EmptyState, PersonAvatar } from "@/components/common"
import { FEEDBACK_CATEGORIES, useEnumText } from "@/components/common/text"
import { categoryIcons } from "@/components/inbox/Feedback"
import { ShortcutSheet, useShortcutSheet } from "@/components/common/ShortcutSheet"
import { LanguageMenu } from "@/components/LanguageMenu"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar"
import { Skeleton } from "@/components/ui/skeleton"
import { api, unwrap, useVersion, type Availability, type Me } from "@/lib/api"
import { useCounts, useInboxes, useLabels } from "@/lib/queries"
import { reconnectNow, useRealtime, useRealtimeStatus } from "@/lib/realtime"
import { cn } from "@/lib/utils"
import {
  meKey,
  pickMembership,
  SessionProvider,
  useMe,
  useSession,
  useSignOut,
  useWorkspaceChoice,
} from "@/lib/session"

export const VIEWS = ["all", "mine", "unassigned", "spam"] as const
export type View = (typeof VIEWS)[number]

export function useViewLabels(): Record<View, string> {
  const { t } = useLingui()
  return { all: t`All`, mine: t`Mine`, unassigned: t`Unassigned`, spam: t`Spam` }
}

const viewIcons: Record<View, React.ComponentType> = {
  all: InboxIcon,
  mine: UserIcon,
  unassigned: UserXIcon,
  spam: ShieldAlertIcon,
}

function NavLink({
  to,
  active,
  count,
  children,
}: {
  to: string
  active: boolean
  count?: number
  children: React.ReactNode
}) {
  const { isMobile, setOpenMobile } = useSidebar()
  const { i18n } = useLingui()
  return (
    <SidebarMenuItem>
      <SidebarMenuButton isActive={active} render={<Link to={to} onClick={() => isMobile && setOpenMobile(false)} />}>
        {children}
      </SidebarMenuButton>
      {!!count && (
        <SidebarMenuBadge className="text-muted-foreground" data-testid="nav-count">
          {new Intl.NumberFormat(i18n.locale, { notation: "compact" }).format(count)}
        </SidebarMenuBadge>
      )}
    </SidebarMenuItem>
  )
}

function compact(n: number, locale: string) {
  return new Intl.NumberFormat(locale, { notation: "compact" }).format(n)
}

function FeedbackNav({ active, category }: { active: boolean; category?: string }) {
  const { isMobile, setOpenMobile } = useSidebar()
  const { i18n } = useLingui()
  const text = useEnumText()
  const counts = useCounts().data
  const byCategory = new Map(counts?.feedback_categories.map((x) => [x.category, x.count]))
  const close = () => isMobile && setOpenMobile(false)
  const shown = FEEDBACK_CATEGORIES.filter((c) => byCategory.has(c))
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={active && category === "all"}
        render={<Link to="/feedback/all" onClick={close} />}
        data-testid="nav-feedback"
      >
        <MessageSquareHeartIcon />
        <span>
          <Trans>Feedback</Trans>
        </span>
      </SidebarMenuButton>
      {!!counts?.feedback && (
        <SidebarMenuBadge className="text-muted-foreground" data-testid="nav-count">
          {compact(counts.feedback, i18n.locale)}
        </SidebarMenuBadge>
      )}
      {shown.length > 0 && (
        <SidebarMenuSub>
          {shown.map((c) => {
            const Icon = categoryIcons[c]
            return (
              <SidebarMenuSubItem key={c}>
                <SidebarMenuSubButton
                  isActive={active && category === c}
                  render={<Link to={`/feedback/${c}`} onClick={close} />}
                  data-testid="nav-feedback-category"
                >
                  <Icon />
                  <span className="flex-1">{text.category[c]}</span>
                  <span className="text-xs text-muted-foreground tabular-nums">
                    {compact(byCategory.get(c) ?? 0, i18n.locale)}
                  </span>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            )
          })}
        </SidebarMenuSub>
      )}
    </SidebarMenuItem>
  )
}

function Nav() {
  const { pathname } = useLocation()
  const labels = useViewLabels()
  const inboxes = useInboxes().data ?? []
  const tags = useLabels().data ?? []
  const counts = useCounts().data
  const inboxCounts = new Map(counts?.inboxes.map((x) => [x.id, x.count]))
  const labelCounts = new Map(counts?.labels.map((x) => [x.id, x.count]))
  const first = pathname.split("/")[1]
  const second = pathname.split("/")[2]
  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {VIEWS.map((v) => {
              const Icon = viewIcons[v]
              return (
                <Fragment key={v}>
                  {v === "spam" && <FeedbackNav active={first === "feedback"} category={second} />}
                  <NavLink to={`/${v}`} active={first === v} count={counts?.[v]}>
                    <Icon />
                    <span>{labels[v]}</span>
                  </NavLink>
                </Fragment>
              )
            })}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
      {inboxes.length > 0 && (
        <SidebarGroup>
          <SidebarGroupLabel>
            <Trans>Inboxes</Trans>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {inboxes.map((inbox) => (
                <NavLink
                  key={inbox.id}
                  to={`/inbox/${inbox.id}`}
                  active={first === "inbox" && second === inbox.id}
                  count={inboxCounts.get(inbox.id)}
                >
                  <span
                    className="size-2.5 shrink-0 rounded-sm"
                    style={{ backgroundColor: inbox.branding.color ?? "var(--muted-foreground)" }}
                  />
                  <span>{inbox.name}</span>
                </NavLink>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      )}
      {tags.length > 0 && (
        <SidebarGroup>
          <SidebarGroupLabel>
            <Trans>Labels</Trans>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {tags.map((label) => (
                <NavLink
                  key={label.id}
                  to={`/label/${label.id}`}
                  active={first === "label" && second === label.id}
                  count={labelCounts.get(label.id)}
                >
                  <TagIcon style={{ color: label.color }} />
                  <span>{label.name}</span>
                </NavLink>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      )}
      <SidebarGroup className="mt-auto">
        <SidebarGroupContent>
          <SidebarMenu>
            <NavLink to="/settings" active={first === "settings"}>
              <SettingsIcon />
              <span>
                <Trans>Settings</Trans>
              </span>
            </NavLink>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}

function WorkspaceSwitcher() {
  const { me, membership, switchWorkspace } = useSession()
  const name = membership.workspace.name
  const header = (
    <>
      <img src="/favicon.svg" alt="" className="size-7 rounded-md" />
      <span className="min-w-0 flex-1 truncate text-left font-semibold">{name}</span>
    </>
  )
  if (me.memberships.length < 2) {
    return <div className="flex items-center gap-2 px-2 py-1.5">{header}</div>
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<SidebarMenuButton size="lg" />}
        aria-label={name}
        data-testid="workspace-switcher"
      >
        {header}
        <ChevronsUpDownIcon className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Workspaces</Trans>
          </DropdownMenuLabel>
          {me.memberships.map((m) => (
            <DropdownMenuItem key={m.workspace.id} onClick={() => switchWorkspace(m.workspace.id)}>
              <BuildingIcon />
              <span className="flex-1 truncate">{m.workspace.name}</span>
              {m.workspace.id === membership.workspace.id && <CheckIcon />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function availabilityClass(a: Availability, live: boolean) {
  if (a === "away") return "bg-amber-500"
  return live ? "bg-success" : "bg-muted-foreground"
}

function UserMenu({ onShortcuts }: { onShortcuts: () => void }) {
  const { t } = useLingui()
  const { me } = useSession()
  const qc = useQueryClient()
  const live = useRealtimeStatus() === "live"
  const availability = me.person.availability
  const setAvailability = useMutation({
    mutationFn: (value: Availability) => unwrap(api.PATCH("/v1/me", { body: { availability: value } })),
    onSuccess: (data) => qc.setQueryData(meKey, data),
  })
  const statusText = availability === "away" ? t`Away` : live ? t`Available` : t`Offline`
  const signOut = useSignOut()
  const navigate = useNavigate()
  const { isMobile, setOpenMobile } = useSidebar()
  const name = me.person.name || me.person.email
  const go = (to: string) => {
    if (isMobile) setOpenMobile(false)
    navigate(to)
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<SidebarMenuButton size="lg" />} data-testid="user-menu">
        <span className="relative shrink-0">
          <PersonAvatar name={name} />
          <span
            className={cn(
              "absolute -right-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-sidebar",
              availabilityClass(availability, live),
            )}
            title={statusText}
            data-testid="availability-dot"
            data-availability={availability}
          />
          <span className="sr-only">{statusText}</span>
        </span>
        <span className="flex min-w-0 flex-1 flex-col text-left leading-tight">
          <span className="truncate text-sm font-medium">{name}</span>
          <span className="truncate text-xs text-muted-foreground">{me.person.email}</span>
        </span>
        <ChevronsUpDownIcon className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="min-w-64">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Status</Trans>
          </DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={availability}
            onValueChange={(v) => {
              if (v !== availability) setAvailability.mutate(v as Availability)
            }}
          >
            <DropdownMenuRadioItem value="auto" data-testid="availability-auto">
              <span className={cn("size-2 shrink-0 rounded-full", availabilityClass("auto", true))} />
              <span className="flex flex-col">
                <Trans>Available</Trans>
                <span className="text-xs text-muted-foreground">
                  <Trans>While the panel is open, in business hours</Trans>
                </span>
              </span>
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="away" data-testid="availability-away">
              <span className={cn("size-2 shrink-0 rounded-full", availabilityClass("away", true))} />
              <span className="flex flex-col">
                <Trans>Away</Trans>
                <span className="text-xs text-muted-foreground">
                  <Trans>Live chat shows nobody available</Trans>
                </span>
              </span>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          {setAvailability.error && (
            <p role="alert" className="px-2 py-1 text-xs text-destructive">
              <Trans>The status was not changed. Try again.</Trans>
            </p>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => go("/settings/profile")}>
          <UserIcon />
          <Trans>My profile</Trans>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onShortcuts}>
          <KeyboardIcon />
          <Trans>Keyboard shortcuts</Trans>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={async () => {
            await signOut()
            navigate("/sign-in", { replace: true })
          }}
        >
          <LogOutIcon />
          <Trans>Sign out</Trans>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Footer() {
  const version = useVersion().data?.version
  const live = useRealtimeStatus()
  return (
    <div className="flex items-center gap-2 px-2 pb-1 text-xs text-muted-foreground">
      <span className="min-w-0 flex-1 truncate">{version && <Trans>Yuva {version}</Trans>}</span>
      <span className="flex items-center gap-1.5" data-testid="realtime-status" data-status={live}>
        <span
          className={cn(
            "size-2 rounded-full",
            live === "live" ? "bg-success" : live === "connecting" ? "bg-amber-500" : "bg-destructive",
          )}
        />
        {live === "live" ? <Trans>Live</Trans> : live === "connecting" ? <Trans>Connecting…</Trans> : <Trans>Offline</Trans>}
      </span>
    </div>
  )
}

function OfflineBanner() {
  const live = useRealtimeStatus()
  const [shown, setShown] = useState(false)
  useEffect(() => {
    if (live !== "offline") {
      setShown(false)
      return
    }
    const id = setTimeout(() => setShown(true), 1500)
    return () => clearTimeout(id)
  }, [live])
  if (!shown) return null
  return (
    <div
      role="status"
      data-testid="offline-banner"
      className="pointer-events-none fixed inset-x-0 top-14 z-50 flex justify-center px-4"
    >
      <div className="pointer-events-auto flex items-center gap-2 rounded-full border bg-background py-1 pr-1 pl-3 text-sm shadow-md">
        <WifiOffIcon className="size-4 shrink-0 text-destructive" />
        <span>
          <Trans>Connection lost. Reconnecting…</Trans>
        </span>
        <Button variant="ghost" size="sm" className="rounded-full" onClick={reconnectNow}>
          <Trans>Retry now</Trans>
        </Button>
      </div>
    </div>
  )
}

export function AppShell() {
  const sheet = useShortcutSheet()
  const { workspaceId, membership } = useSession()
  useRealtime(workspaceId, membership.member_id)
  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <Sidebar>
        <SidebarHeader>
          <WorkspaceSwitcher />
        </SidebarHeader>
        <SidebarContent>
          <Nav />
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <UserMenu onShortcuts={() => sheet.setOpen(true)} />
            </SidebarMenuItem>
          </SidebarMenu>
          <Footer />
        </SidebarFooter>
      </Sidebar>
      <SidebarInset className="min-h-0 min-w-0 overflow-hidden">
        <Outlet />
      </SidebarInset>
      <ShortcutSheet open={sheet.open} onOpenChange={sheet.setOpen} />
      <OfflineBanner />
    </SidebarProvider>
  )
}

export function TopBar({ title, children }: { title: React.ReactNode; children?: React.ReactNode }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3">
      <SidebarTrigger />
      <h1 className="min-w-0 truncate text-sm font-medium">{title}</h1>
      <div className="ml-auto flex items-center gap-1">
        {children}
        <LanguageMenu />
      </div>
    </header>
  )
}

function FullPage({ children }: { children: React.ReactNode }) {
  return <div className="flex min-h-svh flex-col bg-background">{children}</div>
}

function WorkspacePicker({ me, onChoose }: { me: Me; onChoose: (id: string) => void }) {
  return (
    <FullPage>
      <header className="flex h-12 items-center justify-end px-3">
        <LanguageMenu />
      </header>
      <main className="flex flex-1 items-start justify-center px-4 pt-[12vh]">
        <div className="w-full max-w-sm">
          <h1 className="mb-6 text-center text-xl font-semibold">
            <Trans>Choose a workspace</Trans>
          </h1>
          <ul className="flex flex-col gap-2">
            {me.memberships.map((m) => (
              <li key={m.workspace.id}>
                <button
                  type="button"
                  onClick={() => onChoose(m.workspace.id)}
                  className="flex w-full items-center gap-3 rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:bg-muted"
                >
                  <BuildingIcon className="size-5 text-muted-foreground" />
                  <span className="flex-1 truncate font-medium">{m.workspace.name}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </main>
    </FullPage>
  )
}

export function Gate() {
  const me = useMe()
  const location = useLocation()
  const navigate = useNavigate()
  const [chosen, choose] = useWorkspaceChoice()
  const onSwitch = useCallback(
    (id: string) => {
      choose(id)
      navigate("/all")
    },
    [choose, navigate],
  )

  if (me.isPending) {
    return (
      <FullPage>
        <div className="flex flex-1 items-center justify-center">
          <Skeleton className="h-8 w-40" />
        </div>
      </FullPage>
    )
  }
  if (me.error && !me.data) {
    return (
      <FullPage>
        <EmptyState icon={RefreshCwIcon} title={<Trans>Could not reach the server</Trans>}>
          <Button className="mt-2" onClick={() => me.refetch()}>
            <Trans>Try again</Trans>
          </Button>
        </EmptyState>
      </FullPage>
    )
  }
  if (!me.data) return <Navigate to="/sign-in" replace state={{ from: location.pathname }} />
  if (me.data.memberships.length === 0) {
    return (
      <FullPage>
        <EmptyState icon={BuildingIcon} title={<Trans>You are not a member of any workspace</Trans>}>
          <Trans>Ask an owner or admin of your workspace to invite you.</Trans>
        </EmptyState>
      </FullPage>
    )
  }
  const membership = pickMembership(me.data, chosen)
  if (!membership) return <WorkspacePicker me={me.data} onChoose={choose} />
  return (
    <SessionProvider key={membership.workspace.id} me={me.data} membership={membership} onSwitch={onSwitch}>
      <AppShell />
    </SessionProvider>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, InboxIcon, LanguagesIcon, MessagesSquareIcon, UserIcon, UserXIcon } from "lucide-react"
import { Link, Navigate, Outlet, useParams } from "react-router"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from "@/components/ui/sidebar"
import { activate, locales, type Locale } from "@/i18n"
import { useVersion } from "@/lib/api"

const VIEWS = ["all", "mine", "unassigned"] as const
type View = (typeof VIEWS)[number]

function useViewLabels(): Record<View, string> {
  const { t } = useLingui()
  return { all: t`All`, mine: t`Mine`, unassigned: t`Unassigned` }
}

const viewIcons: Record<View, React.ComponentType> = {
  all: InboxIcon,
  mine: UserIcon,
  unassigned: UserXIcon,
}

function isView(v: string | undefined): v is View {
  return VIEWS.includes(v as View)
}

function Nav() {
  const { view } = useParams()
  const labels = useViewLabels()
  const { isMobile, setOpenMobile } = useSidebar()
  return (
    <SidebarMenu>
      {VIEWS.map((v) => {
        const Icon = viewIcons[v]
        return (
          <SidebarMenuItem key={v}>
            <SidebarMenuButton
              isActive={view === v}
              render={<Link to={`/${v}`} onClick={() => isMobile && setOpenMobile(false)} />}
            >
              <Icon />
              <span>{labels[v]}</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        )
      })}
    </SidebarMenu>
  )
}

function LanguageMenu() {
  const { t, i18n } = useLingui()
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="sm" aria-label={t`Language`} />}>
        <LanguagesIcon />
        <span className="uppercase">{i18n.locale}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        {Object.entries(locales).map(([k, v]) => (
          <DropdownMenuItem key={k} onClick={() => activate(k as Locale)}>
            <span className="flex-1">{v}</span>
            {i18n.locale === k && <CheckIcon />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function VersionFooter() {
  const version = useVersion().data?.version
  return (
    <footer className="flex h-10 shrink-0 items-center border-t px-4 text-xs text-muted-foreground">
      {version && <Trans>Yuva {version}</Trans>}
    </footer>
  )
}

export function AppShell() {
  return (
    <SidebarProvider>
      <Sidebar>
        <SidebarHeader>
          <div className="flex items-center gap-2 px-2 py-1.5">
            <img src="/favicon.svg" alt="" className="size-7 rounded-md" />
            <span className="font-semibold">Yuva</span>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <Nav />
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>
      <SidebarInset className="min-h-svh">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3">
          <SidebarTrigger />
          <ViewTitle />
          <div className="ml-auto">
            <LanguageMenu />
          </div>
        </header>
        <main className="flex flex-1 flex-col">
          <Outlet />
        </main>
        <VersionFooter />
      </SidebarInset>
    </SidebarProvider>
  )
}

function ViewTitle() {
  const { view } = useParams()
  const labels = useViewLabels()
  return <h1 className="truncate text-sm font-medium">{isView(view) ? labels[view] : ""}</h1>
}

export function ConversationsPage() {
  const { view } = useParams()
  if (!isView(view)) return <Navigate to="/all" replace />
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <MessagesSquareIcon className="size-6" />
      </div>
      <p className="text-sm text-muted-foreground">
        <Trans>No conversations yet</Trans>
      </p>
    </div>
  )
}

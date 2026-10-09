import { Trans, useLingui } from "@lingui/react/macro"
import { BuildingIcon, DownloadIcon, KeyboardIcon, LanguagesIcon, ListIcon, LogOutIcon, MonitorIcon, MoonIcon, RowsIcon, SunIcon, UserIcon } from "lucide-react"
import { useNavigate } from "react-router"

import { useShell } from "@/app/shell"
import { PersonAvatar } from "@/components/common"
import { useChangeLocale } from "@/components/common/LanguageMenu"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { locales, type Locale } from "@/i18n"
import { useVersion, type Availability } from "@/lib/api"
import { useSetAvailability } from "@/lib/availability"
import { useInstallPrompt } from "@/lib/pwa"
import { useRealtimeStatus } from "@/lib/realtime"
import { useSession, useSignOut } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useView } from "@/lib/view"
import { setTheme, useTheme, type Theme } from "@/lib/theme"

function statusClass(a: Availability, live: boolean) {
  if (a === "away") return "bg-faint"
  return live ? "bg-success" : "bg-faint"
}

export function UserMenu() {
  const { t, i18n } = useLingui()
  const { me, membership, switchWorkspace } = useSession()
  const { openShortcuts } = useShell()
  const [view, setView] = useView()
  const theme = useTheme()
  const navigate = useNavigate()
  const signOut = useSignOut()
  const install = useInstallPrompt()
  const changeLocale = useChangeLocale()
  const version = useVersion().data?.version
  const live = useRealtimeStatus() === "live"
  const availability = me.person.availability
  const setAvailability = useSetAvailability()

  const statusText = availability === "away" ? t`Away` : live ? t`Available` : t`Offline`
  const name = me.person.name || me.person.email
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="relative ms-1 rounded-full p-0.5 outline-none"
        aria-label={t`${name} · ${statusText}`}
        title={statusText}
        data-testid="user-menu"
      >
        <PersonAvatar name={name} className="size-[30px] bg-muted-foreground text-[11px] font-semibold text-background" />
        <span
          className={cn("absolute end-px bottom-px size-[9px] rounded-full ring-2 ring-background", statusClass(availability, live))}
          data-testid="availability-dot"
          data-availability={availability}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="truncate">{me.person.email}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={availability}
            onValueChange={(v) => {
              if (v !== availability) setAvailability.mutate(v as Availability)
            }}
          >
            <DropdownMenuRadioItem value="auto" className="items-start" data-testid="availability-auto">
              <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", statusClass("auto", true))} />
              <span className="flex flex-col gap-0.5">
                <Trans>Available</Trans>
                <span className="text-xs text-muted-foreground">
                  <Trans>While the panel is open, in business hours</Trans>
                </span>
              </span>
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="away" className="items-start" data-testid="availability-away">
              <span className={cn("mt-1.5 size-2 shrink-0 rounded-full", statusClass("away", true))} />
              <span className="flex flex-col gap-0.5">
                <Trans>Away</Trans>
                <span className="text-xs text-muted-foreground">
                  <Trans>Live chat shows nobody available. Notifications pause, except for conversations assigned to you.</Trans>
                </span>
              </span>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          {setAvailability.error && (
            <p role="alert" className="px-2.5 py-1 text-xs text-destructive">
              <Trans>The status was not changed. Try again.</Trans>
            </p>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => navigate("/settings/profile")}>
          <UserIcon />
          <Trans>My profile</Trans>
        </DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <LanguagesIcon />
            <Trans>Language</Trans>
            <span className="ms-auto text-xs text-faint uppercase">{i18n.locale}</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="min-w-36">
            <DropdownMenuRadioGroup value={i18n.locale} onValueChange={(v) => changeLocale(v as Locale)}>
              {Object.entries(locales).map(([k, v]) => (
                <DropdownMenuRadioItem key={k} value={k}>
                  {v}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        {me.memberships.length > 1 && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <BuildingIcon />
              <span className="min-w-0 flex-1 truncate">{membership.workspace.name}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent className="min-w-56">
              <DropdownMenuRadioGroup value={membership.workspace.id} onValueChange={(id) => switchWorkspace(String(id))}>
                {me.memberships.map((m) => (
                  <DropdownMenuRadioItem key={m.workspace.id} value={m.workspace.id}>
                    <span className="truncate">{m.workspace.name}</span>
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            {theme === "dark" ? <MoonIcon /> : theme === "light" ? <SunIcon /> : <MonitorIcon />}
            <Trans>Appearance</Trans>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="min-w-36">
            <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
              <DropdownMenuRadioItem value="system" data-testid="theme-system">
                <Trans>System</Trans>
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="light" data-testid="theme-light">
                <Trans>Light</Trans>
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark" data-testid="theme-dark">
                <Trans>Dark</Trans>
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem
          onClick={() => {
            setView(view === "list" ? "queue" : "list")
            navigate("/")
          }}
          data-testid="menu-switch-view"
        >
          {view === "list" ? <RowsIcon /> : <ListIcon />}
          {view === "list" ? <Trans>Switch to the queue</Trans> : <Trans>Switch to the list</Trans>}
          <span className="ms-auto text-xs text-faint">V</span>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={openShortcuts}>
          <KeyboardIcon />
          <Trans>Keyboard shortcuts</Trans>
          <span className="ms-auto text-xs text-faint">?</span>
        </DropdownMenuItem>
        {install && (
          <DropdownMenuItem onClick={() => void install()} data-testid="install-app">
            <DownloadIcon />
            <Trans>Install app</Trans>
          </DropdownMenuItem>
        )}
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
        {version && (
          <p className="px-2.5 pt-1.5 pb-1 text-xs text-faint">
            <Trans>Yuva {version}</Trans>
          </p>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import {
  BellIcon,
  BuildingIcon,
  CompassIcon,
  DownloadIcon,
  LanguagesIcon,
  LogOutIcon,
  KeyboardIcon,
  PlusIcon,
  UserIcon,
} from "lucide-react"
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
  DropdownMenuShortcut,
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
import { setTheme, useTheme, type Theme } from "@/lib/theme"
import { Button } from "@/components/ui/button"

function statusClass(a: Availability, live: boolean) {
  if (a === "away") return "bg-warning"
  return live ? "bg-success" : "bg-faint"
}

export function UserMenu() {
  const { t, i18n } = useLingui()
  const { me, membership, switchWorkspace, canManage } = useSession()
  const { openShortcuts } = useShell()
  const theme = useTheme()
  const navigate = useNavigate()
  const signOut = useSignOut()
  const install = useInstallPrompt()
  const changeLocale = useChangeLocale()
  const { version, signup } = useVersion().data ?? {}
  const live = useRealtimeStatus() === "live"
  const availability = me.person.availability
  const setAvailability = useSetAvailability()

  const statusText = availability === "away" ? t`Away` : live ? t`Available` : t`Offline`
  const name = me.person.name || me.person.email
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="ghost" size="icon-sm" className="relative ms-1 size-7 rounded-full" />}
        aria-label={t`${name} · ${statusText}`}
        title={statusText}
        data-testid="user-menu"
      >
        <PersonAvatar name={name} className="size-6 bg-foreground text-[10px] text-background" />
        <span
          className={cn(
            "absolute end-0 bottom-0 size-2 rounded-full ring-2 ring-background",
            statusClass(availability, live)
          )}
          data-testid="availability-dot"
          data-availability={availability}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center gap-2.5 py-2">
            <PersonAvatar name={name} className="size-8 bg-muted-foreground text-background" />
            <span className="min-w-0">
              <span className="block truncate text-body font-medium text-foreground">{name}</span>
              <span className="block truncate text-caption font-normal text-muted-foreground">{me.person.email}</span>
            </span>
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
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
              <span className={cn("size-2 shrink-0 rounded-full", statusClass("auto", true))} />
              <Trans>Available</Trans>
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="away" data-testid="availability-away">
              <span className={cn("size-2 shrink-0 rounded-full", statusClass("away", true))} />
              <Trans>Away</Trans>
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          {setAvailability.error && (
            <p role="alert" className="px-2.5 py-1 text-caption text-destructive">
              <Trans>The status was not changed. Try again.</Trans>
            </p>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        {canManage && (
          <DropdownMenuItem onClick={() => navigate("/setup")} data-testid="menu-setup">
            <CompassIcon />
            <Trans>Getting started</Trans>
          </DropdownMenuItem>
        )}
        <DropdownMenuItem onClick={() => navigate("/settings/profile")}>
          <UserIcon />
          <Trans>My profile</Trans>
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => navigate("/settings/notifications")} data-testid="menu-notifications">
          <BellIcon />
          <Trans>Notifications</Trans>
        </DropdownMenuItem>
        <DropdownMenuGroup className="flex items-center gap-2 py-1 ps-2.5 pe-1.5" aria-label={t`Appearance`}>
          <span className="flex-1 text-body text-muted-foreground">
            <Trans>Appearance</Trans>
          </span>
          <DropdownMenuRadioGroup
            value={theme}
            onValueChange={(v) => setTheme(v as Theme)}
            className="inline-flex rounded-lg bg-muted p-0.5"
          >
            {(
              [
                ["system", t`System`],
                ["light", t`Light`],
                ["dark", t`Dark`],
              ] as const
            ).map(([value, label]) => (
              <DropdownMenuRadioItem
                key={value}
                value={value}
                closeOnClick={false}
                className="h-[26px] rounded-md px-2 pe-2 text-caption text-muted-foreground data-checked:bg-card data-checked:font-semibold data-checked:text-foreground data-checked:shadow-[0_0_0_1px_var(--border)] [&>span]:hidden"
                data-testid={`theme-${value}`}
              >
                {label}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <LanguagesIcon />
            <span className="flex-1">
              <Trans>Language</Trans>
            </span>
            <span className="text-caption text-muted-foreground">{locales[i18n.locale as Locale]}</span>
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="max-h-80 min-w-44 overflow-y-auto">
            <DropdownMenuRadioGroup value={i18n.locale} onValueChange={(v) => changeLocale(v as Locale)}>
              {Object.entries(locales)
                .sort(([a], [b]) => Number(b === i18n.locale) - Number(a === i18n.locale))
                .map(([k, v]) => (
                  <DropdownMenuRadioItem key={k} value={k}>
                    {v}
                  </DropdownMenuRadioItem>
                ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem onClick={openShortcuts} data-testid="menu-shortcuts">
          <KeyboardIcon />
          <Trans>Keyboard shortcuts</Trans>
          <DropdownMenuShortcut>?</DropdownMenuShortcut>
        </DropdownMenuItem>
        {(me.memberships.length > 1 || signup === "open") && (
          <DropdownMenuSub>
            <DropdownMenuSubTrigger>
              <BuildingIcon />
              <span className="min-w-0 flex-1 truncate">{membership.workspace.name}</span>
            </DropdownMenuSubTrigger>
            <DropdownMenuSubContent className="min-w-56">
              <DropdownMenuRadioGroup
                value={membership.workspace.id}
                onValueChange={(id) => switchWorkspace(String(id))}
              >
                {me.memberships.map((m) => (
                  <DropdownMenuRadioItem key={m.workspace.id} value={m.workspace.id}>
                    <span className="truncate">{m.workspace.name}</span>
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
              {signup === "open" && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => navigate("/workspaces/new")} data-testid="create-workspace">
                    <PlusIcon />
                    <Trans>Create workspace</Trans>
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuSubContent>
          </DropdownMenuSub>
        )}
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
          <p className="px-2.5 pt-1 pb-1.5 text-[11px] text-faint">
            <Trans>Yuva {version}</Trans>
          </p>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { DownloadIcon, KeyboardIcon, LanguagesIcon, LogOutIcon, UserIcon } from "lucide-react"
import { useNavigate } from "react-router"

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
import { api, unwrap, useVersion, type Availability } from "@/lib/api"
import { useInstallPrompt } from "@/lib/pwa"
import { useRealtimeStatus } from "@/lib/realtime"
import { meKey, useSession, useSignOut } from "@/lib/session"
import { cn } from "@/lib/utils"

function statusClass(a: Availability, live: boolean) {
  if (a === "away") return "bg-warning"
  return live ? "bg-success" : "bg-faint"
}

export function UserMenu({ onShortcuts }: { onShortcuts: () => void }) {
  const { t, i18n } = useLingui()
  const { me } = useSession()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const signOut = useSignOut()
  const install = useInstallPrompt()
  const changeLocale = useChangeLocale()
  const version = useVersion().data?.version
  const live = useRealtimeStatus() === "live"
  const availability = me.person.availability
  const setAvailability = useMutation({
    mutationFn: (value: Availability) => unwrap(api.PATCH("/v1/me", { body: { availability: value } })),
    onSuccess: (data) => qc.setQueryData(meKey, data),
  })
  const statusText = availability === "away" ? t`Away` : live ? t`Available` : t`Offline`
  const name = me.person.name || me.person.email
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="flex h-11 w-full items-center gap-2.5 rounded-lg px-2 text-start transition-colors outline-none hover:bg-muted aria-expanded:bg-muted"
        data-testid="user-menu"
      >
        <span className="relative shrink-0">
          <PersonAvatar name={name} />
          <span
            className={cn("absolute -end-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 ring-surface", statusClass(availability, live))}
            title={statusText}
            data-testid="availability-dot"
            data-availability={availability}
          />
        </span>
        <span className="flex min-w-0 flex-1 flex-col leading-tight">
          <span className="truncate text-sm font-medium">{name}</span>
          <span className="truncate text-xs text-faint">{statusText}</span>
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" className="w-72">
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
        <DropdownMenuItem onClick={onShortcuts}>
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

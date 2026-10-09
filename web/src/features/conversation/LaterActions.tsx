import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ClockIcon, UserRoundPlusIcon } from "lucide-react"
import { useState } from "react"

import { Kbd, MemberAvatar } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { popupClass } from "@/components/ui/dropdown-menu"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { firstName, type QueueActions } from "@/features/conversation/actions"
import { useCounts } from "@/features/inbox/queries"
import type { Conversation } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAssignableMembers } from "@/lib/workspace"

export type LaterMenu = "snooze" | "hand" | null

const link = <Button variant="ghost" size="sm" className="rounded-lg px-2.5 text-[13px] font-normal [&_svg]:size-[13px]" />

function Key({ children }: { children: React.ReactNode }) {
  return <Kbd className="h-4 min-w-4 rounded-[5px] bg-transparent font-mono text-[10px] text-faint phone:hidden">{children}</Kbd>
}

function useSnoozeTimes() {
  const { t, i18n } = useLingui()
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const now = new Date()
  const hour = new Date(now.getTime() + 60 * 60 * 1000)
  const evening = new Date(now)
  evening.setHours(17, 0, 0, 0)
  const morning = new Date(now)
  morning.setDate(now.getDate() + 1)
  morning.setHours(9, 0, 0, 0)
  const list = [{ at: hour, label: t`In 1 hour`, when: t`in an hour`, detail: time.format(hour) }]
  if (evening.getTime() - now.getTime() > 30 * 60 * 1000) {
    const at = time.format(evening)
    list.push({ at: evening, label: t`Today at ${at}`, when: t`today at ${at}`, detail: "" })
  }
  const at = time.format(morning)
  list.push({ at: morning, label: t`Tomorrow at ${at}`, when: t`tomorrow at ${at}`, detail: "" })
  return list
}

export function LaterActions({
  c,
  name,
  actions,
  menu,
  setMenu,
}: {
  c: Conversation
  name: string
  actions: QueueActions
  menu: LaterMenu
  setMenu: (m: LaterMenu) => void
}) {
  const { t } = useLingui()
  const { membership } = useSession()
  const times = useSnoozeTimes()
  const here = (m: { online: boolean; availability: string }) => (m.online && m.availability === "auto" ? 0 : m.availability === "away" ? 2 : 1)
  const people = useAssignableMembers(c.inbox_id)
    .filter((m) => m.id !== membership.member_id)
    .sort((a, b) => here(a) - here(b))
  const [note, setNote] = useState("")
  const load = new Map((useCounts().data?.assignees ?? []).map((a) => [a.id, a.count]))
  const first = firstName(name)
  return (
    <div className="mt-3 flex flex-wrap justify-center gap-0.5 phone:gap-0" data-testid="later-actions">
      <DropdownMenu open={menu === "snooze"} onOpenChange={(o) => setMenu(o ? "snooze" : null)}>
        <DropdownMenuTrigger render={link} data-testid="snooze-menu">
          <ClockIcon className="size-[13px]" />
          <Trans>Later</Trans>
          <Key>S</Key>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="center" side="top" className="w-[280px] max-w-[calc(100vw-32px)]">
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <Trans>When should it come back?</Trans>
            </DropdownMenuLabel>
            {times.map((o) => (
              <DropdownMenuItem key={o.label} onClick={() => actions.snooze(o.at, o.when)}>
                <span className="flex-1">{o.label}</span>
                {o.detail && <span className="text-xs text-faint">{o.detail}</span>}
              </DropdownMenuItem>
            ))}
            <DropdownMenuItem onClick={actions.untilReply}>
              <Trans>When {first} writes again</Trans>
            </DropdownMenuItem>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={actions.later}>
            <Trans>Just move it to the end of the queue</Trans>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Popover
        open={menu === "hand"}
        onOpenChange={(o) => {
          setMenu(o ? "hand" : null)
          if (!o) setNote("")
        }}
      >
        <PopoverTrigger render={link} data-testid="hand-menu">
          <UserRoundPlusIcon className="size-[13px]" />
          <Trans>Hand to a teammate</Trans>
          <Key>A</Key>
        </PopoverTrigger>
        <PopoverContent side="top" align="center" sideOffset={6} className={cn(popupClass, "w-[300px] max-w-[calc(100vw-32px)] gap-0 rounded-xl p-1.5 ring-0")} data-testid="hand-popup">
              <p className="px-2.5 pt-1.5 pb-1 text-xs font-medium text-faint">
                <Trans>Who should take it?</Trans>
              </p>
              {people.length === 0 && (
                <p className="px-2.5 py-2 text-sm text-muted-foreground">
                  <Trans>Nobody else can see this inbox yet.</Trans>
                </p>
              )}
              {people.map((m) => {
                const who = m.name || m.email
                return (
                  <Button
                    key={m.id}
                    variant="ghost"
                    onClick={() => {
                      setMenu(null)
                      actions.hand(m, note)
                      setNote("")
                    }}
                    className="h-auto w-full justify-start gap-2.5 rounded-lg px-2.5 py-2 text-start font-normal text-foreground"
                  >
                    <MemberAvatar name={who} online={m.online} away={m.availability === "away"} ring="ring-card" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{who}</span>
                      <small className="block truncate text-xs text-faint">
                        {m.availability === "away" ? <Trans>Away</Trans> : m.online ? <Trans>Online</Trans> : <Trans>Offline</Trans>}
                        {" · "}
                        <Plural value={load.get(m.id) ?? 0} one="# conversation" other="# conversations" />
                      </small>
                    </span>
                  </Button>
                )
              })}
              {people.length > 0 && (
                <Input
                  autoFocus
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder={t`Add a note (optional)`}
                  aria-label={t`Note for the teammate`}
                  className="mt-1.5 rounded-[10px]"
                  data-testid="hand-note"
                />
              )}
      </PopoverContent>
      </Popover>

      <Button variant="ghost" size="sm" className="rounded-lg px-2.5 text-[13px] font-normal [&_svg]:size-[13px]" onClick={actions.close} data-testid="close-conversation">
        <CheckIcon />
        <Trans>Close without reply</Trans>
        <Key>E</Key>
      </Button>
    </div>
  )
}

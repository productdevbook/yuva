import { Popover } from "@base-ui/react/popover"
import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ClockIcon, UserRoundPlusIcon } from "lucide-react"
import { useState } from "react"

import { PersonAvatar } from "@/components/common"
import { useEnumText } from "@/components/common/text"
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
import type { Conversation } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAssignableMembers } from "@/lib/workspace"

export type LaterMenu = "snooze" | "hand" | null

const link =
  "inline-flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-[13px] text-muted-foreground transition-colors outline-none hover:bg-muted hover:text-foreground aria-expanded:bg-muted aria-expanded:text-foreground"

function Key({ children }: { children: React.ReactNode }) {
  return <kbd className="rounded-[5px] border px-1 font-mono text-[10px] font-medium text-faint phone:hidden">{children}</kbd>
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
  const text = useEnumText()
  const { membership } = useSession()
  const times = useSnoozeTimes()
  const people = useAssignableMembers(c.inbox_id).filter((m) => m.id !== membership.member_id)
  const [note, setNote] = useState("")
  const first = firstName(name)
  return (
    <div className="mt-3 flex flex-wrap justify-center gap-0.5 phone:gap-0" data-testid="later-actions">
      <DropdownMenu open={menu === "snooze"} onOpenChange={(o) => setMenu(o ? "snooze" : null)}>
        <DropdownMenuTrigger className={link} data-testid="snooze-menu">
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

      <Popover.Root
        open={menu === "hand"}
        onOpenChange={(o) => {
          setMenu(o ? "hand" : null)
          if (!o) setNote("")
        }}
      >
        <Popover.Trigger className={link} data-testid="hand-menu">
          <UserRoundPlusIcon className="size-[13px]" />
          <Trans>Hand to a teammate</Trans>
          <Key>A</Key>
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner side="top" align="center" sideOffset={6} className="z-50">
            <Popover.Popup className={cn(popupClass, "w-[300px] max-w-[calc(100vw-32px)]")} data-testid="hand-popup">
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
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => {
                      setMenu(null)
                      actions.hand(m, note)
                      setNote("")
                    }}
                    className="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-start text-sm outline-none hover:bg-muted focus-visible:bg-muted"
                  >
                    <PersonAvatar name={who} className="size-[26px] bg-mate text-[10px] font-semibold text-white" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{who}</span>
                      <small className="block truncate text-xs text-faint">{text.role[m.role]}</small>
                    </span>
                  </button>
                )
              })}
              {people.length > 0 && (
                <input
                  autoFocus
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder={t`Add a note (optional)`}
                  aria-label={t`Note for the teammate`}
                  className="mt-1.5 w-full rounded-[10px] border bg-background px-3 py-2 text-base outline-none focus:border-brand md:text-sm"
                  data-testid="hand-note"
                />
              )}
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>
      </Popover.Root>

      <button type="button" className={link} onClick={actions.close} data-testid="close-conversation">
        <CheckIcon className="size-[13px]" />
        <Trans>Close without reply</Trans>
        <Key>E</Key>
      </button>
    </div>
  )
}

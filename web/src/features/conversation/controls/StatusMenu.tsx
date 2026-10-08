import { Trans, useLingui } from "@lingui/react/macro"

import { Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { STATUSES, useEnumText } from "@/components/common/text"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { SnoozeItems } from "@/features/conversation/controls/SnoozeItems"
import { menuProps, pillClass, statusIcons, type ControlProps } from "@/features/conversation/controls/shared"

export function StatusMenu(p: ControlProps) {
  const { t } = useLingui()
  const text = useEnumText()
  const Icon = statusIcons[p.conversation.status]
  return (
    <DropdownMenu {...menuProps("status", p)}>
      <DropdownMenuTrigger className={pillClass} aria-label={t`Change status`} data-testid="status-menu">
        <Icon />
        {text.status[p.conversation.status]}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Status</Trans>
            <Kbd>{SHORTCUTS.status}</Kbd>
          </DropdownMenuLabel>
          {STATUSES.filter((s) => s !== "snoozed").map((s) => {
            const I = statusIcons[s]
            return (
              <DropdownMenuItem key={s} onClick={() => p.update({ status: s })}>
                <I />
                <span className="flex-1">{text.status[s]}</span>
                {s === "closed" && <Kbd>{SHORTCUTS.close}</Kbd>}
                {s === "open" && <Kbd>{SHORTCUTS.reopen}</Kbd>}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <SnoozeItems onPick={(at) => p.update({ status: "snoozed", snooze_until: at.toISOString() })} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import { Trans } from "@lingui/react/macro"

import { DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel } from "@/components/ui/dropdown-menu"
import { useSnoozeOptions } from "@/features/conversation/controls/shared"

export function SnoozeItems({ onPick }: { onPick: (at: Date) => void }) {
  const options = useSnoozeOptions()
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>
        <Trans>Snooze until</Trans>
      </DropdownMenuLabel>
      {options.map((o) => (
        <DropdownMenuItem key={o.label} onClick={() => onPick(o.at)}>
          <span className="flex-1">{o.label}</span>
          <span className="text-xs text-faint">{o.when}</span>
        </DropdownMenuItem>
      ))}
    </DropdownMenuGroup>
  )
}

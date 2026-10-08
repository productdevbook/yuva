import { Trans, useLingui } from "@lingui/react/macro"
import { FlagIcon } from "lucide-react"

import { Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { PRIORITIES, useEnumText } from "@/components/common/text"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { menuProps, pillClass, priorityClass, type ControlProps } from "@/features/conversation/controls/shared"
import { cn } from "@/lib/utils"

export function PriorityMenu(p: ControlProps) {
  const { t } = useLingui()
  const text = useEnumText()
  const priority = p.conversation.priority
  const notable = priority === "urgent" || priority === "high"
  return (
    <DropdownMenu {...menuProps("priority", p)}>
      <DropdownMenuTrigger
        className={cn(pillClass, !notable && "w-7 justify-center px-0")}
        aria-label={t`Change priority`}
        title={text.priority[priority]}
        data-testid="priority-menu"
      >
        <FlagIcon className={priorityClass[priority]} />
        {notable && <span className={priorityClass[priority]}>{text.priority[priority]}</span>}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-44">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Priority</Trans>
            <Kbd>{SHORTCUTS.priority}</Kbd>
          </DropdownMenuLabel>
          {PRIORITIES.map((pr) => (
            <DropdownMenuItem key={pr} onClick={() => p.update({ priority: pr })}>
              <FlagIcon className={priorityClass[pr]} />
              <span className="flex-1">{text.priority[pr]}</span>
              {pr === priority && <span className="size-1.5 rounded-full bg-brand" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

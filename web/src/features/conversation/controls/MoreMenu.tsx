import { Trans, useLingui } from "@lingui/react/macro"
import { EllipsisIcon, ShieldAlertIcon, ShieldCheckIcon } from "lucide-react"

import { Dot, Kbd } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { menuProps, pillClass, type ControlProps } from "@/features/conversation/controls/shared"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export function MoreMenu({ move, moving, ...p }: ControlProps & { move: (inboxId: string) => void; moving: boolean }) {
  const { t } = useLingui()
  const spam = p.conversation.spam
  const others = (useInboxes().data ?? []).filter((i) => i.id !== p.conversation.inbox_id)
  return (
    <DropdownMenu {...menuProps("move", p)}>
      <DropdownMenuTrigger
        className={cn(pillClass, "w-7 justify-center px-0")}
        disabled={moving}
        aria-label={t`More actions`}
        data-testid="move-menu"
      >
        <EllipsisIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[60svh] min-w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Move to inbox</Trans>
            <Kbd>{SHORTCUTS.move}</Kbd>
          </DropdownMenuLabel>
          {others.length === 0 && (
            <p className="px-2.5 py-1.5 text-sm text-muted-foreground">
              <Trans>You cannot see any other inbox.</Trans>
            </p>
          )}
          {others.map((i) => (
            <DropdownMenuItem key={i.id} onClick={() => move(i.id)}>
              <Dot color={i.branding.color} />
              <span className="flex-1 truncate">{i.name}</span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => p.update({ spam: !spam })} data-testid="spam-toggle">
          {spam ? <ShieldCheckIcon /> : <ShieldAlertIcon />}
          <span className="flex-1">{spam ? <Trans>Not spam</Trans> : <Trans>Mark as spam</Trans>}</span>
          <Kbd>{SHORTCUTS.spam}</Kbd>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { UserXIcon } from "lucide-react"

import { Kbd, PersonAvatar } from "@/components/common"
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
import type { Member } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useAssignableMembers, useMemberMap } from "@/lib/workspace"

export function MemberItems({ members, onPick, current }: { members: Member[]; onPick: (id: string) => void; current?: string }) {
  const { membership } = useSession()
  return members.map((m) => (
    <DropdownMenuItem key={m.id} onClick={() => onPick(m.id)}>
      <PersonAvatar name={m.name || m.email} className="size-5 text-[9px]" />
      <span className="flex-1 truncate">
        {m.name || m.email}
        {m.id === membership.member_id && (
          <span className="text-faint">
            {" "}
            <Trans>(you)</Trans>
          </span>
        )}
      </span>
      {m.id === current && <span className="size-1.5 rounded-full bg-brand" />}
    </DropdownMenuItem>
  ))
}

export function AssigneeMenu(p: ControlProps) {
  const { t } = useLingui()
  const members = useAssignableMembers(p.conversation.inbox_id)
  const byId = useMemberMap()
  const assignee = p.conversation.assignee_id ? byId.get(p.conversation.assignee_id) : undefined
  const name = assignee ? assignee.name || assignee.email : null
  return (
    <DropdownMenu {...menuProps("assign", p)}>
      <DropdownMenuTrigger className={pillClass} aria-label={t`Assign`} data-testid="assign-menu">
        {name ? <PersonAvatar name={name} className="-ms-1.5 size-5 text-[9px]" /> : <UserXIcon />}
        <span className="truncate">{name ?? <Trans>Unassigned</Trans>}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="max-h-[60svh] min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Assign to</Trans>
            <Kbd>{SHORTCUTS.assign}</Kbd>
          </DropdownMenuLabel>
          <MemberItems members={members} current={p.conversation.assignee_id} onPick={(id) => p.update({ assignee_id: id })} />
        </DropdownMenuGroup>
        {p.conversation.assignee_id && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => p.update({ assignee_id: null })}>
              <UserXIcon />
              <Trans>Unassign</Trans>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

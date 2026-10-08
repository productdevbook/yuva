import { Trans, useLingui } from "@lingui/react/macro"
import { AlarmClockIcon, CheckIcon, RotateCcwIcon, TagIcon, UserPlusIcon, UserXIcon, XIcon } from "lucide-react"

import { Dot, ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { MemberItems } from "@/features/conversation/controls/AssigneeMenu"
import { SnoozeItems } from "@/features/conversation/controls/SnoozeItems"
import type { ConversationBulkUpdate } from "@/lib/api"
import { useLabels, useMembers } from "@/lib/workspace"

const MAX_BULK = 100

export type BulkChange = Omit<ConversationBulkUpdate, "conversation_ids">

function Action({ label, testId, ...props }: { label: string; testId: string } & React.ComponentProps<typeof Button>) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon-sm" aria-label={label} data-testid={testId} {...props} />} />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function MenuAction({ label, testId, disabled, icon, children }: { label: string; testId: string; disabled: boolean; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" disabled={disabled} />} aria-label={label} title={label} data-testid={testId}>
        {icon}
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-[60svh] min-w-56">
        {children}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function BulkBar({
  total,
  count,
  pending,
  error,
  failed,
  onSelectAll,
  onClear,
  onApply,
}: {
  total: number
  count: number
  pending: boolean
  error: unknown
  failed: number
  onSelectAll: (on: boolean) => void
  onClear: () => void
  onApply: (change: BulkChange) => void
}) {
  const { t } = useLingui()
  const members = useMembers().data ?? []
  const labels = useLabels().data ?? []
  const tooMany = count > MAX_BULK
  const off = pending || tooMany
  const all = total > 0 && count === total
  return (
    <div className="flex shrink-0 flex-col border-b bg-surface" data-testid="bulk-bar">
      <div className="flex h-11 items-center gap-0.5 ps-[1.6rem] pe-2">
        <Checkbox
          checked={all}
          indeterminate={count > 0 && !all}
          onCheckedChange={(on) => onSelectAll(on)}
          aria-label={t`Select all on this page`}
          data-testid="select-all"
        />
        <span className="ms-3 min-w-0 flex-1 truncate text-sm font-medium" data-testid="bulk-count">
          <Trans>{count} selected</Trans>
        </span>
        <Action label={t`Close`} onClick={() => onApply({ status: "closed" })} disabled={off} testId="bulk-close">
          <CheckIcon />
        </Action>
        <Action label={t`Reopen`} onClick={() => onApply({ status: "open" })} disabled={off} testId="bulk-reopen">
          <RotateCcwIcon />
        </Action>
        <MenuAction label={t`Snooze`} testId="bulk-snooze" disabled={off} icon={<AlarmClockIcon />}>
          <SnoozeItems onPick={(at) => onApply({ status: "snoozed", snooze_until: at.toISOString() })} />
        </MenuAction>
        <MenuAction label={t`Assign`} testId="bulk-assign" disabled={off} icon={<UserPlusIcon />}>
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <Trans>Assign to</Trans>
            </DropdownMenuLabel>
            <MemberItems members={members} onPick={(id) => onApply({ assignee_id: id })} />
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => onApply({ assignee_id: null })}>
            <UserXIcon />
            <Trans>Unassign</Trans>
          </DropdownMenuItem>
        </MenuAction>
        <MenuAction label={t`Labels`} testId="bulk-labels" disabled={off || labels.length === 0} icon={<TagIcon />}>
          {(
            [
              ["add_labels", <Trans>Add label</Trans>, "bulk-add-label"],
              ["remove_labels", <Trans>Remove label</Trans>, "bulk-remove-label"],
            ] as const
          ).map(([field, text, testId]) => (
            <DropdownMenuSub key={field}>
              <DropdownMenuSubTrigger data-testid={testId}>{text}</DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="max-h-[60svh]">
                {labels.map((l) => (
                  <DropdownMenuItem key={l.id} onClick={() => onApply({ [field]: [l.id] })}>
                    <Dot color={l.color} />
                    {l.name}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          ))}
        </MenuAction>
        <Action label={t`Clear the selection`} onClick={onClear} testId="bulk-clear">
          <XIcon />
        </Action>
      </div>
      {tooMany && (
        <p role="alert" className="px-4 pb-2 text-xs text-destructive">
          <Trans>Select at most {MAX_BULK} conversations at a time.</Trans>
        </p>
      )}
      {failed > 0 && (
        <p role="alert" className="px-4 pb-2 text-xs text-destructive" data-testid="bulk-failed">
          <Trans>{failed} could not be changed; they stay selected.</Trans>
        </p>
      )}
      <ErrorLine error={error} className="px-4 pb-2 text-xs" />
    </div>
  )
}

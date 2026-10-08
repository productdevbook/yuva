import { Trans, useLingui } from "@lingui/react/macro"
import {
  AlarmClockIcon,
  CheckCircle2Icon,
  CircleDotIcon,
  ClockIcon,
  MinusIcon,
  PlusIcon,
  TagIcon,
  UserPlusIcon,
  UserXIcon,
  XIcon,
} from "lucide-react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { snoozeTimes } from "@/components/inbox/ConversationControls"
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
import type { ConversationBulkUpdate } from "@/lib/api"
import { useLabels, useMembers } from "@/lib/queries"
import { useSession } from "@/lib/session"

export const MAX_BULK = 100

export type BulkChange = Omit<ConversationBulkUpdate, "conversation_ids">

function IconAction({
  label,
  onClick,
  disabled,
  children,
  testId,
}: {
  label: string
  onClick: () => void
  disabled: boolean
  children: React.ReactNode
  testId: string
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button variant="ghost" size="icon-sm" onClick={onClick} disabled={disabled} aria-label={label} data-testid={testId} />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
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
  const { t, i18n } = useLingui()
  const { membership } = useSession()
  const members = useMembers().data ?? []
  const labels = useLabels().data ?? []
  const times = snoozeTimes()
  const fmt = (d: Date) =>
    new Intl.DateTimeFormat(i18n.locale, { weekday: "short", hour: "2-digit", minute: "2-digit" }).format(d)
  const tooMany = count > MAX_BULK
  const off = pending || tooMany
  const all = total > 0 && count === total
  return (
    <div className="flex flex-col border-b" data-testid="bulk-bar">
      <div className="flex min-h-10 items-center gap-1 px-3 py-1">
        <Checkbox
          checked={all}
          indeterminate={count > 0 && !all}
          onCheckedChange={(on) => onSelectAll(on)}
          aria-label={t`Select all on this page`}
          data-testid="select-all"
        />
        {count === 0 ? (
          <span className="ml-2 text-xs text-muted-foreground">
            <Trans>Select all on this page</Trans>
          </span>
        ) : (
          <>
            <span className="ml-2 min-w-0 flex-1 truncate text-xs font-medium" data-testid="bulk-count">
              <Trans>{count} selected</Trans>
            </span>
            <IconAction label={t`Close`} onClick={() => onApply({ status: "closed" })} disabled={off} testId="bulk-close">
              <CheckCircle2Icon />
            </IconAction>
            <IconAction label={t`Reopen`} onClick={() => onApply({ status: "open" })} disabled={off} testId="bulk-reopen">
              <CircleDotIcon />
            </IconAction>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" disabled={off} />}
                aria-label={t`Snooze`}
                data-testid="bulk-snooze"
              >
                <AlarmClockIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-56">
                <DropdownMenuGroup>
                  <DropdownMenuLabel>
                    <Trans>Snooze until</Trans>
                  </DropdownMenuLabel>
                  {(
                    [
                      [times.inHour, t`In an hour`],
                      [times.tomorrow, t`Tomorrow morning`],
                      [times.nextWeek, t`Next week`],
                    ] as const
                  ).map(([d, label]) => (
                    <DropdownMenuItem key={label} onClick={() => onApply({ status: "snoozed", snooze_until: d.toISOString() })}>
                      <ClockIcon />
                      <span className="flex-1">{label}</span>
                      <span className="text-xs text-muted-foreground">{fmt(d)}</span>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" disabled={off} />}
                aria-label={t`Assign`}
                data-testid="bulk-assign"
              >
                <UserPlusIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="max-h-[60svh] min-w-56 overflow-y-auto">
                <DropdownMenuGroup>
                  <DropdownMenuLabel>
                    <Trans>Assign to</Trans>
                  </DropdownMenuLabel>
                  {members.map((m) => (
                    <DropdownMenuItem key={m.id} onClick={() => onApply({ assignee_id: m.id })}>
                      <PersonAvatar name={m.name || m.email} className="size-5 text-[9px]" />
                      <span className="flex-1 truncate">
                        {m.name || m.email}
                        {m.id === membership.member_id && (
                          <span className="text-muted-foreground">
                            {" "}
                            <Trans>(you)</Trans>
                          </span>
                        )}
                      </span>
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => onApply({ assignee_id: null })}>
                  <UserXIcon />
                  <Trans>Unassign</Trans>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" disabled={off || labels.length === 0} />}
                aria-label={t`Labels`}
                data-testid="bulk-labels"
              >
                <TagIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-48">
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger data-testid="bulk-add-label">
                    <PlusIcon />
                    <Trans>Add label</Trans>
                  </DropdownMenuSubTrigger>
                  <DropdownMenuSubContent className="max-h-[60svh] min-w-44 overflow-y-auto">
                    {labels.map((l) => (
                      <DropdownMenuItem key={l.id} onClick={() => onApply({ add_labels: [l.id] })}>
                        <span className="size-2.5 rounded-full" style={{ backgroundColor: l.color }} />
                        {l.name}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuSubContent>
                </DropdownMenuSub>
                <DropdownMenuSub>
                  <DropdownMenuSubTrigger data-testid="bulk-remove-label">
                    <MinusIcon />
                    <Trans>Remove label</Trans>
                  </DropdownMenuSubTrigger>
                  <DropdownMenuSubContent className="max-h-[60svh] min-w-44 overflow-y-auto">
                    {labels.map((l) => (
                      <DropdownMenuItem key={l.id} onClick={() => onApply({ remove_labels: [l.id] })}>
                        <span className="size-2.5 rounded-full" style={{ backgroundColor: l.color }} />
                        {l.name}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuSubContent>
                </DropdownMenuSub>
              </DropdownMenuContent>
            </DropdownMenu>
            <IconAction label={t`Clear the selection`} onClick={onClear} disabled={false} testId="bulk-clear">
              <XIcon />
            </IconAction>
          </>
        )}
      </div>
      {tooMany && (
        <p role="alert" className="px-3 pb-2 text-xs text-destructive">
          <Trans>Select at most {MAX_BULK} conversations at a time.</Trans>
        </p>
      )}
      {failed > 0 && (
        <p role="alert" className="px-3 pb-2 text-xs text-destructive" data-testid="bulk-failed">
          <Trans>{failed} could not be changed; they stay selected.</Trans>
        </p>
      )}
      <ErrorLine error={error} className="px-3 pb-2 text-xs" />
    </div>
  )
}

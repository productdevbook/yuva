import { Trans, useLingui } from "@lingui/react/macro"
import {
  AlarmClockIcon,
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleDotIcon,
  ClockIcon,
  FlagIcon,
  TagIcon,
  UserXIcon,
} from "lucide-react"

import { Kbd, LabelChip, PersonAvatar } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { PRIORITIES, STATUSES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Conversation, ConversationStatus, ConversationUpdate, Priority } from "@/lib/api"
import { useAssignableMembers, useLabels, useMemberMap } from "@/lib/queries"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export type MenuName = "assign" | "status" | "priority" | "labels"

type Props = {
  conversation: Conversation
  update: (body: ConversationUpdate) => void
  openMenu: MenuName | null
  setOpenMenu: (m: MenuName | null) => void
}

export const statusIcons: Record<ConversationStatus, React.ComponentType<{ className?: string }>> = {
  open: CircleDotIcon,
  pending: CircleDashedIcon,
  snoozed: AlarmClockIcon,
  closed: CheckCircle2Icon,
}

export const priorityClass: Record<Priority, string> = {
  urgent: "text-destructive",
  high: "text-orange-600 dark:text-orange-400",
  normal: "text-muted-foreground",
  low: "text-muted-foreground/60",
}

function menuProps(name: MenuName, p: Props) {
  return {
    open: p.openMenu === name,
    onOpenChange: (open: boolean) => p.setOpenMenu(open ? name : null),
  }
}

function snoozeTimes() {
  const now = new Date()
  const inHour = new Date(now.getTime() + 60 * 60 * 1000)
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  tomorrow.setHours(9, 0, 0, 0)
  const nextWeek = new Date(now)
  nextWeek.setDate(now.getDate() + ((8 - now.getDay()) % 7 || 7))
  nextWeek.setHours(9, 0, 0, 0)
  return { inHour, tomorrow, nextWeek }
}

export function AssigneeMenu(p: Props) {
  const { t } = useLingui()
  const { membership } = useSession()
  const members = useAssignableMembers(p.conversation.inbox_id)
  const byId = useMemberMap()
  const assignee = p.conversation.assignee_id ? byId.get(p.conversation.assignee_id) : undefined
  const name = assignee ? assignee.name || assignee.email : null
  return (
    <DropdownMenu {...menuProps("assign", p)}>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="sm" className="max-w-48" />}
        aria-label={t`Assign`}
        data-testid="assign-menu"
      >
        {name ? <PersonAvatar name={name} className="size-5 text-[9px]" /> : <UserXIcon />}
        <span className="truncate">{name ?? <Trans>Unassigned</Trans>}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-56">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Assign to</Trans>
            <Kbd>{SHORTCUTS.assign}</Kbd>
          </DropdownMenuLabel>
          {members.map((m) => (
            <DropdownMenuItem key={m.id} onClick={() => p.update({ assignee_id: m.id })}>
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
              {m.id === p.conversation.assignee_id && <span className="size-1.5 rounded-full bg-primary" />}
            </DropdownMenuItem>
          ))}
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

export function StatusMenu(p: Props) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const Icon = statusIcons[p.conversation.status]
  const times = snoozeTimes()
  const fmt = (d: Date) =>
    new Intl.DateTimeFormat(i18n.locale, { weekday: "short", hour: "2-digit", minute: "2-digit" }).format(d)
  return (
    <DropdownMenu {...menuProps("status", p)}>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="sm" />}
        aria-label={t`Change status`}
        data-testid="status-menu"
      >
        <Icon />
        {text.status[p.conversation.status]}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-56">
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
            <DropdownMenuItem
              key={label}
              onClick={() => p.update({ status: "snoozed", snooze_until: d.toISOString() })}
            >
              <ClockIcon />
              <span className="flex-1">{label}</span>
              <span className="text-xs text-muted-foreground">{fmt(d)}</span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function PriorityMenu(p: Props) {
  const { t } = useLingui()
  const text = useEnumText()
  return (
    <DropdownMenu {...menuProps("priority", p)}>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="sm" />}
        aria-label={t`Change priority`}
        data-testid="priority-menu"
      >
        <FlagIcon className={priorityClass[p.conversation.priority]} />
        {text.priority[p.conversation.priority]}
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
              {pr === p.conversation.priority && <span className="size-1.5 rounded-full bg-primary" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function LabelsMenu(p: Props) {
  const { t } = useLingui()
  const labels = useLabels().data ?? []
  const current = new Set(p.conversation.labels)
  const chosen = labels.filter((l) => current.has(l.id))
  return (
    <DropdownMenu {...menuProps("labels", p)}>
      <DropdownMenuTrigger
        render={<Button variant="outline" size="sm" className={cn(chosen.length > 0 && "h-auto min-h-8 py-1")} />}
        aria-label={t`Edit labels`}
        data-testid="labels-menu"
      >
        <TagIcon />
        {chosen.length === 0 ? (
          <Trans>Labels</Trans>
        ) : (
          <span className="flex flex-wrap gap-1">
            {chosen.map((l) => (
              <LabelChip key={l.id} name={l.name} color={l.color} />
            ))}
          </span>
        )}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-52">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="flex items-center justify-between">
            <Trans>Labels</Trans>
            <Kbd>{SHORTCUTS.labels}</Kbd>
          </DropdownMenuLabel>
          {labels.length === 0 && (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              <Trans>No labels yet. Add them in settings.</Trans>
            </p>
          )}
          {labels.map((l) => (
            <DropdownMenuCheckboxItem
              key={l.id}
              checked={current.has(l.id)}
              onCheckedChange={(on) => {
                const next = new Set(current)
                if (on) next.add(l.id)
                else next.delete(l.id)
                p.update({ labels: [...next] })
              }}
            >
              <span className="size-2.5 rounded-full" style={{ backgroundColor: l.color }} />
              {l.name}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

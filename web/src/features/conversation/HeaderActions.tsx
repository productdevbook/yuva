import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ChevronDownIcon, FlagIcon, UserRoundCheckIcon, UsersRoundIcon } from "lucide-react"
import { useState, type RefObject } from "react"

import { ErrorLine, MemberAvatar } from "@/components/common"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  popupClass,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Item } from "@/components/ui/item"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Separator } from "@/components/ui/separator"
import { firstName, type ConversationActions } from "@/features/conversation/actions"
import { progressIcons } from "@/features/conversation/Feedback"
import { useUpdateConversation } from "@/features/conversation/queries"
import { look } from "@/features/inbox/PageFrame"
import { useCounts } from "@/features/inbox/queries"
import type { Conversation, ConversationUpdate, FeedbackProgress } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useAssignableMembers, useInboxes, useTeams } from "@/lib/workspace"

export type HeaderMenu = "snooze" | "hand" | "team" | "progress" | "sla" | null

export function useSnoozeTimes() {
  const { t, i18n } = useLingui()
  const time = new Intl.DateTimeFormat(i18n.locale, { hour: "2-digit", minute: "2-digit" })
  const [now] = useState(() => new Date())
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

export function SnoozeMenu({
  name,
  actions,
  open,
  onOpenChange,
  anchor,
}: {
  name: string
  actions: ConversationActions
  open: boolean
  onOpenChange: (open: boolean) => void
  anchor?: RefObject<Element | null>
}) {
  const { t } = useLingui()
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={look.btnSm} />}
        title={t`Snooze (S)`}
        data-testid="snooze-menu"
      >
        <Trans>Snooze</Trans>
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" anchor={anchor} className="w-[280px] max-w-[calc(100vw-32px)]">
        <SnoozeTimes name={name} actions={actions} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function SnoozeTimes({ name, actions }: { name: string; actions: ConversationActions }) {
  const times = useSnoozeTimes()
  const first = firstName(name)
  return (
    <DropdownMenuGroup>
      <DropdownMenuLabel>
        <Trans>When should it come back?</Trans>
      </DropdownMenuLabel>
      {times.map((o) => (
        <DropdownMenuItem key={o.label} onClick={() => actions.snooze(o.at, o.when)}>
          <span className="flex-1">{o.label}</span>
          {o.detail && <span className="text-caption text-faint">{o.detail}</span>}
        </DropdownMenuItem>
      ))}
      <DropdownMenuItem onClick={actions.untilReply}>
        <Trans>When {first} writes again</Trans>
      </DropdownMenuItem>
    </DropdownMenuGroup>
  )
}

export function useInboxTeams(inboxId: string) {
  return (useTeams().data ?? []).filter((team) => team.inbox_ids.includes(inboxId))
}

function useTeamChoice(c: Conversation, close: () => void) {
  const update = useUpdateConversation(c.id)
  const choose = (teamId: string | null) => update.mutate({ team_id: teamId }, { onSuccess: close })
  return { choose, error: update.error, pending: update.isPending }
}

function TeamChoices({
  c,
  teams,
  error,
  pending,
  choose,
}: {
  c: Conversation
  teams: { id: string; name: string }[]
  error: unknown
  pending: boolean
  choose: (teamId: string | null) => void
}) {
  return (
    <>
      {teams.map((team) => (
        <Item
          key={team.id}
          render={<button type="button" disabled={pending} />}
          size="xs"
          onClick={() => choose(team.id)}
          className={cn("flex-nowrap", c.team_id === team.id && "bg-muted")}
          data-testid="team-choice"
        >
          <UsersRoundIcon className="size-4 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate text-start">{team.name}</span>
          {c.team_id === team.id && <CheckIcon className="size-4 text-brand" />}
        </Item>
      ))}
      <ErrorLine error={error} className="px-2.5 py-1.5 text-small" />
    </>
  )
}

export function TeamMenu({
  c,
  open,
  onOpenChange,
  anchor,
}: {
  c: Conversation
  open: boolean
  onOpenChange: (open: boolean) => void
  anchor?: RefObject<Element | null>
}) {
  const { t } = useLingui()
  const all = useTeams().data ?? []
  const teams = useInboxTeams(c.inbox_id)
  const current = all.find((x) => x.id === c.team_id)
  const { choose, error, pending } = useTeamChoice(c, () => onOpenChange(false))
  if (teams.length === 0 && !current) return null
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={<Button variant="outline" className={cn(look.btnSm, "max-w-40 gap-1")} />}
        title={t`Team (M)`}
        data-testid="team-menu"
      >
        <UsersRoundIcon className="text-faint" />
        <span className="min-w-0 truncate">{current ? current.name : <Trans>No team</Trans>}</span>
        <ChevronDownIcon className="text-faint" />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        anchor={anchor}
        sideOffset={6}
        className={cn(popupClass, "w-[260px] max-w-[calc(100vw-32px)] gap-0 rounded-xl p-1.5 ring-0")}
        data-testid="team-popup"
      >
        <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
          <Trans>Team</Trans>
        </p>
        <TeamChoices c={c} teams={teams} error={error} pending={pending} choose={choose} />
        {current && (
          <Item
            render={<button type="button" disabled={pending} />}
            size="xs"
            onClick={() => choose(null)}
            data-testid="team-clear"
          >
            <Trans>Remove the team</Trans>
          </Item>
        )}
      </PopoverContent>
    </Popover>
  )
}

export function HandMenu({
  c,
  actions,
  open,
  onOpenChange,
  anchor,
  label,
}: {
  c: Conversation
  actions: ConversationActions
  open: boolean
  onOpenChange: (open: boolean) => void
  anchor?: RefObject<Element | null>
  label: React.ReactNode
}) {
  const { t } = useLingui()
  const { membership } = useSession()
  const here = (m: { online: boolean; availability: string }) =>
    m.online && m.availability === "auto" ? 0 : m.availability === "away" ? 2 : 1
  const people = useAssignableMembers(c.inbox_id)
    .filter((m) => m.id !== membership.member_id)
    .sort((a, b) => here(a) - here(b))
  const [note, setNote] = useState("")
  const load = new Map((useCounts().data?.assignees ?? []).map((a) => [a.id, a.count]))
  const mine = c.assignee_id === membership.member_id
  const teams = useInboxTeams(c.inbox_id)
  const inboxCap = (useInboxes().data ?? []).find((i) => i.id === c.inbox_id)?.assignment_cap
  const teamPool = teams.find((x) => x.id === c.team_id && x.assignment_mode !== "off")
  const cap = teamPool ? teamPool.assignment_cap : inboxCap
  const teamChoice = useTeamChoice(c, () => onOpenChange(false))
  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) setNote("")
      }}
    >
      <PopoverTrigger
        render={<Button variant="outline" className={cn(look.btnSm, "max-w-44 gap-1")} />}
        title={t`Assign (A)`}
        data-testid="hand-menu"
      >
        <span className="min-w-0 truncate">{label}</span>
        <ChevronDownIcon className="text-faint" />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        anchor={anchor}
        sideOffset={6}
        className={cn(popupClass, "w-[300px] max-w-[calc(100vw-32px)] gap-0 rounded-xl p-1.5 ring-0")}
        data-testid="hand-popup"
      >
        {!mine && (
          <>
            <Item
              render={<button type="button" />}
              size="xs"
              className="flex-nowrap"
              onClick={() => {
                onOpenChange(false)
                actions.claim()
              }}
              data-testid="assign-me"
            >
              <UserRoundCheckIcon className="size-4 text-muted-foreground" />
              <span className="flex-1 text-start">
                <Trans>Assign to me</Trans>
              </span>
            </Item>
            <Separator className="mx-1.5 my-1 w-auto" />
          </>
        )}
        <p className="px-2.5 pt-1.5 pb-1 text-caption font-medium text-faint">
          <Trans>Who should take it?</Trans>
        </p>
        {people.length === 0 && (
          <p className="px-2.5 py-2 text-body text-muted-foreground">
            <Trans>Nobody else can see this inbox yet.</Trans>
          </p>
        )}
        {people.map((m) => {
          const who = m.name || m.email
          return (
            <Item
              key={m.id}
              render={<button type="button" />}
              size="xs"
              onClick={() => {
                onOpenChange(false)
                actions.hand(m, note)
                setNote("")
              }}
              className={cn("flex-nowrap", c.assignee_id === m.id && "bg-muted")}
            >
              <MemberAvatar name={who} online={m.online} away={m.availability === "away"} ring="ring-card" />
              <span className="min-w-0 flex-1 text-start">
                <span className="block truncate">{who}</span>
                <small className="block truncate text-caption text-faint">
                  {m.availability === "away" ? (
                    <Trans>Away</Trans>
                  ) : m.online ? (
                    <Trans>Online</Trans>
                  ) : (
                    <Trans>Offline</Trans>
                  )}
                  {" · "}
                  <Plural value={load.get(m.id) ?? 0} one="# conversation" other="# conversations" />
                  {cap !== undefined && (load.get(m.id) ?? 0) >= cap && (
                    <span className="text-warning" data-testid="over-cap">
                      {" · "}
                      <Trans>over cap</Trans>
                    </span>
                  )}
                </small>
              </span>
            </Item>
          )
        })}
        {teams.length > 0 && (
          <>
            <p className="px-2.5 pt-2.5 pb-1 text-caption font-medium text-faint">
              <Trans>Teams</Trans>
            </p>
            <TeamChoices
              c={c}
              teams={teams}
              error={teamChoice.error}
              pending={teamChoice.pending}
              choose={teamChoice.choose}
            />
          </>
        )}
        {people.length > 0 && (
          <Input
            name="hand-note"
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
  )
}

const PROGRESS: FeedbackProgress[] = ["planned", "in_progress", "done", "wont_do"]

export function ProgressMenu({
  c,
  update,
  open,
  onOpenChange,
}: {
  c: Conversation
  update: (body: ConversationUpdate) => void
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const current = c.feedback?.progress
  const Icon = current ? progressIcons[current] : FlagIcon
  return (
    <DropdownMenu open={open} onOpenChange={onOpenChange}>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={look.btnSm} />}
        title={t`Progress (O)`}
        data-testid="progress-menu"
        data-progress={current}
      >
        <Icon />
        {current ? text.progress[current] : <Trans>Set progress</Trans>}
        <ChevronDownIcon className="text-faint" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="min-w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            <Trans>Progress</Trans>
          </DropdownMenuLabel>
          {PROGRESS.map((p) => {
            const PIcon = progressIcons[p]
            return (
              <DropdownMenuItem key={p} onClick={() => update({ feedback_progress: p })} data-testid={`progress-${p}`}>
                <PIcon className="size-4 text-muted-foreground" />
                <span className="flex-1">{text.progress[p]}</span>
                {current === p && <CheckIcon className="size-4 text-brand" />}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>
        {current && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => update({ feedback_progress: null })} data-testid="progress-clear">
              <Trans>Clear</Trans>
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function CloseButton({ c, actions }: { c: Conversation; actions: ConversationActions }) {
  const { t } = useLingui()
  const feedback = c.kind === "feedback"
  const closed = c.status === "closed"
  return (
    <Button
      className={look.btnInkSm}
      onClick={closed ? actions.reopen : actions.close}
      title={closed ? undefined : feedback ? t`Resolve (E)` : t`Close (E)`}
      data-testid={closed ? "reopen-conversation" : "close-conversation"}
    >
      {closed ? <Trans>Reopen</Trans> : feedback ? <Trans>Resolve</Trans> : <Trans context="conversation">Close</Trans>}
    </Button>
  )
}

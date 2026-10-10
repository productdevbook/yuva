import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { UsersIcon } from "lucide-react"

import { byPresence, TeamSummary, useMemberStatus } from "@/app/TeamMenu"
import { ErrorLine, MemberAvatar } from "@/components/common"
import { Column, ColumnHeading, ColumnLink, PaneEmpty } from "@/components/common/Column"
import { Skeleton } from "@/components/ui/skeleton"
import { useCounts } from "@/features/inbox/queries"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useMembers } from "@/lib/workspace"

export function useTeamLoad() {
  const counts = useCounts().data
  return { counts, load: new Map((counts?.assignees ?? []).map((a) => [a.id, a.count])) }
}

export function TeamColumn() {
  const { t } = useLingui()
  const { membership } = useSession()
  const members = useMembers()
  const { counts, load } = useTeamLoad()
  const status = useMemberStatus()
  const order = [...(members.data ?? [])].sort((a, b) => byPresence(a, b) || (load.get(b.id) ?? 0) - (load.get(a.id) ?? 0))
  return (
    <Column title={<Trans>Team</Trans>} testId="team-panel">
      <div className="grid grid-cols-2 gap-2 px-1 pt-1">
        <div className="rounded-xl border bg-card px-3 py-2.5">
          <b className="block text-title tabular-nums">{counts?.unassigned ?? "–"}</b>
          <span className="text-caption text-faint">
            <Trans>open, unassigned</Trans>
          </span>
        </div>
        <div className="rounded-xl border bg-card px-3 py-2.5">
          <b className="block text-title tabular-nums">{order.filter((m) => m.online && m.availability === "auto").length}</b>
          <span className="text-caption text-faint">
            <Trans>available now</Trans>
          </span>
        </div>
      </div>
      <ColumnHeading>
        <Trans>Who is here</Trans>
      </ColumnHeading>
      {members.isPending && <Skeleton className="mx-2 h-10" />}
      <ErrorLine error={members.error} className="px-2.5" />
      <ul className="flex flex-col" data-testid="team-members">
        {order.map((m) => {
          const name = m.name || m.email
          const open = load.get(m.id) ?? 0
          return (
            <li key={m.id} data-testid="team-member">
              <ColumnLink to={`/team/${m.id}`}>
                <MemberAvatar name={name} online={m.online} away={m.availability === "away"} className="size-9" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-body">{m.id === membership.member_id ? t`${name} (you)` : name}</span>
                  <small className="block truncate text-caption text-faint">{status(m).text}</small>
                </span>
                <span className={cn("shrink-0 text-caption tabular-nums", open ? "text-foreground" : "text-faint")} title={t`Open conversations assigned`} data-testid="team-load">
                  <Plural value={open} one="# open" other="# open" />
                </span>
              </ColumnLink>
            </li>
          )
        })}
      </ul>
    </Column>
  )
}

export function TeamHome() {
  return (
    <PaneEmpty icon={UsersIcon} title={<Trans>Pick a teammate</Trans>} testId="team-home">
      <p>
        <Trans>See where they are, what they hold open and what they did today.</Trans>
      </p>
      <p className="mt-3 text-faint">
        <Trans>The team today:</Trans> <TeamSummary />
      </p>
    </PaneEmpty>
  )
}

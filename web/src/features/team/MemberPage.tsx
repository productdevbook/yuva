import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon } from "lucide-react"
import { Link, useParams } from "react-router"

import { useMemberStatus } from "@/app/TeamMenu"
import { MemberAvatar } from "@/components/common"
import { Pane, PaneBack } from "@/components/common/Column"
import { useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { ConversationRows } from "@/features/contact/ContactDetails"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useConversations, useStats } from "@/features/inbox/queries"
import { Section } from "@/features/settings/ui"
import { useTeamLoad } from "@/features/team/TeamColumn"
import { useSession } from "@/lib/session"
import { useMembers } from "@/lib/workspace"

function Tile({ value, label }: { value: React.ReactNode; label: React.ReactNode }) {
  return (
    <div className="rounded-2xl border bg-card px-4 py-3">
      <b className="block text-title tabular-nums">{value}</b>
      <span className="text-caption text-faint">{label}</span>
    </div>
  )
}

export function MemberPage() {
  const { memberId = "" } = useParams()
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const { membership } = useSession()
  const members = useMembers()
  const status = useMemberStatus()
  const { load } = useTeamLoad()
  const [inboxId] = useInboxFilter()
  const stats = useStats(inboxId).data
  const open = useConversations({ assignee: memberId, status: "open" })
  const m = members.data?.find((x) => x.id === memberId)
  const fmt = new Intl.NumberFormat(i18n.locale)

  if (!m) {
    return (
      <Pane testId="member-page">
        <PaneBack to="/team" label={t`Team`} />
        {members.isPending ? (
          <Skeleton className="h-16 w-full rounded-2xl" />
        ) : (
          <p className="text-body text-muted-foreground">
            <Trans>This member is not in the workspace any more.</Trans>
          </p>
        )}
      </Pane>
    )
  }
  const name = m.name || m.email
  const s = status(m)
  const today = stats?.members.find((x) => x.member_id === m.id)
  const items = open.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Pane testId="member-page">
      <header className="flex flex-col">
        <PaneBack to="/team" label={t`Team`} />
        <div className="flex flex-wrap items-center gap-4">
          <MemberAvatar name={name} online={m.online} away={m.availability === "away"} className="size-16" />
          <div className="min-w-0 flex-1">
            <h1 className="text-page break-words" data-testid="member-name">
              {m.id === membership.member_id ? t`${name} (you)` : name}
            </h1>
            <p className="mt-0.5 truncate text-small text-muted-foreground">
              {text.role[m.role]} · {s.text}
            </p>
          </div>
          {s.conversationId && m.id !== membership.member_id && (
            <Button variant="outline" render={<Link to={`/conversations/${s.conversationId}`} />} data-testid="member-viewing">
              <Trans>Open what they see</Trans>
              <ArrowUpRightIcon />
            </Button>
          )}
        </div>
      </header>
      <div className="grid grid-cols-3 gap-2 phone:grid-cols-2" data-testid="member-tiles">
        <Tile value={fmt.format(load.get(m.id) ?? 0)} label={<Trans>open, assigned</Trans>} />
        <Tile value={today ? fmt.format(today.replies) : "–"} label={<Trans>replies today</Trans>} />
        <Tile value={today ? fmt.format(today.closed) : "–"} label={<Trans>closed today</Trans>} />
      </div>
      <Section title={<Trans>Open conversations assigned</Trans>}>
        <ConversationRows items={items} roomy pending={open.isPending} />
        {open.hasNextPage && (
          <Button variant="ghost" size="sm" className="self-center" onClick={() => void open.fetchNextPage()} disabled={open.isFetchingNextPage}>
            <Trans>Load more</Trans>
          </Button>
        )}
      </Section>
    </Pane>
  )
}

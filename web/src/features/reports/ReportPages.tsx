import { Trans, useLingui } from "@lingui/react/macro"
import { ThumbsDownIcon, ThumbsUpIcon } from "lucide-react"
import { Outlet } from "react-router"

import { Dot, ErrorLine, MemberAvatar } from "@/components/common"
import { Pane, PaneBack } from "@/components/common/Column"
import { formatDuration } from "@/components/common/text"
import { Skeleton } from "@/components/ui/skeleton"
import { useTodayStats } from "@/features/reports/ReportsColumn"
import { Card, EmptyRow } from "@/features/settings/ui"
import type { Stats } from "@/lib/api"
import { useInboxes, useMembers } from "@/lib/workspace"

export function ReportsPane() {
  return (
    <Pane testId="reports">
      <Outlet />
    </Pane>
  )
}

function Report({ title, children }: { title: React.ReactNode; children: (s: Stats) => React.ReactNode }) {
  const { t } = useLingui()
  const stats = useTodayStats()
  return (
    <>
      <header className="flex flex-col">
        <PaneBack to="/reports" label={t`Reports`} />
        <h1 className="min-w-0 text-page break-words">{title}</h1>
        <p className="mt-1.5 text-reading text-muted-foreground">
          <Trans>Today since midnight, over the inboxes you can see.</Trans>
        </p>
      </header>
      {stats.isPending && <Skeleton className="h-28 rounded-2xl" />}
      <ErrorLine error={stats.error} />
      {stats.data && children(stats.data)}
    </>
  )
}

export function TodayReport() {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <Report title={<Trans>Replies and closes</Trans>}>
      {(s) => {
        const tiles: [string, string][] = [
          [fmt.format(s.replies), t`replies`],
          [fmt.format(s.closed), t`closed`],
          [fmt.format(s.first_replies), t`first replies`],
          [s.median_first_reply_seconds !== undefined ? formatDuration(s.median_first_reply_seconds, i18n.locale) : "–", t`median first reply`],
        ]
        return (
          <div className="grid grid-cols-2 gap-2" data-testid="report-tiles">
            {tiles.map(([value, label]) => (
              <div key={label} className="rounded-2xl border bg-card px-4 py-3.5">
                <b className="block text-page tabular-nums">{value}</b>
                <span className="text-small text-faint">{label}</span>
              </div>
            ))}
          </div>
        )
      }}
    </Report>
  )
}

export function RatingsReport() {
  const { t, i18n } = useLingui()
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const pct = new Intl.NumberFormat(i18n.locale, { style: "percent" })
  return (
    <Report title={<Trans>Ratings</Trans>}>
      {(s) => {
        const rated = s.ratings.good + s.ratings.bad
        if (rated === 0) {
          return (
            <Card flush>
              <EmptyRow>
                <Trans>No ratings today.</Trans>
              </EmptyRow>
            </Card>
          )
        }
        return (
          <Card>
            <div className="flex flex-col gap-3" data-testid="report-ratings">
              <div className="flex items-center gap-3 text-body">
                <b className="text-page tabular-nums">{pct.format(s.ratings.good / rated)}</b>
                <span className="text-muted-foreground">
                  <Trans>satisfied</Trans>
                </span>
                <span className="ms-auto inline-flex items-center gap-1 text-success">
                  <ThumbsUpIcon className="size-3.5" />
                  {fmt.format(s.ratings.good)}
                </span>
                <span className="inline-flex items-center gap-1 text-destructive">
                  <ThumbsDownIcon className="size-3.5" />
                  {fmt.format(s.ratings.bad)}
                </span>
              </div>
              <div className="flex h-2 overflow-hidden rounded-full bg-destructive/25" aria-hidden>
                <span className="bg-success" style={{ width: `${(s.ratings.good / rated) * 100}%` }} />
              </div>
              {s.ratings.inboxes.length > 1 &&
                s.ratings.inboxes.map((r) => {
                  const inbox = inboxes.get(r.inbox_id)
                  return (
                    <div key={r.inbox_id} className="flex items-center gap-2 text-body">
                      {inbox && <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />}
                      <span className="min-w-0 flex-1 truncate">{inbox?.name ?? t`Deleted inbox`}</span>
                      <span className="text-success tabular-nums">👍 {fmt.format(r.good)}</span>
                      <span className="text-destructive tabular-nums">👎 {fmt.format(r.bad)}</span>
                    </div>
                  )
                })}
            </div>
          </Card>
        )
      }}
    </Report>
  )
}

export function MembersReport() {
  const { t, i18n } = useLingui()
  const members = new Map((useMembers().data ?? []).map((m) => [m.id, m]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <Report title={<Trans>By member</Trans>}>
      {(s) => {
        const people = [...s.members].sort((a, b) => b.replies - a.replies || b.closed - a.closed)
        if (people.length === 0) {
          return (
            <Card flush>
              <EmptyRow>
                <Trans>Nobody has replied or closed anything yet today.</Trans>
              </EmptyRow>
            </Card>
          )
        }
        return (
          <Card flush>
            <table className="w-full text-body" data-testid="report-members">
              <thead className="bg-surface text-caption text-faint">
                <tr>
                  <th className="px-4 py-2 text-start font-normal">
                    <Trans>Member</Trans>
                  </th>
                  <th className="px-2 py-2 text-end font-normal">
                    <Trans>Replies</Trans>
                  </th>
                  <th className="px-4 py-2 text-end font-normal">
                    <Trans>Closed</Trans>
                  </th>
                </tr>
              </thead>
              <tbody>
                {people.map((p) => {
                  const m = members.get(p.member_id)
                  const name = m ? m.name || m.email : t`Deleted member`
                  return (
                    <tr key={p.member_id} className="border-t">
                      <td className="px-4 py-2.5">
                        <span className="flex min-w-0 items-center gap-2">
                          <MemberAvatar name={name} className="size-7" />
                          <span className="truncate">{name}</span>
                        </span>
                      </td>
                      <td className="px-2 py-2.5 text-end tabular-nums">{fmt.format(p.replies)}</td>
                      <td className="px-4 py-2.5 text-end tabular-nums">{fmt.format(p.closed)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </Card>
        )
      }}
    </Report>
  )
}

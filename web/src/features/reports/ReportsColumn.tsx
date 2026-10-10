import { Trans, useLingui } from "@lingui/react/macro"
import { BarChart3Icon, ThumbsUpIcon, UsersIcon } from "lucide-react"

import { Column, ColumnHeading, ColumnLink } from "@/components/common/Column"
import { ErrorLine } from "@/components/common"
import { RowIcon, RowText } from "@/features/settings/ui"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { useStats } from "@/features/inbox/queries"

export function useTodayStats() {
  const [inboxId] = useInboxFilter()
  return useStats(inboxId)
}

export function ReportsColumn() {
  const { t, i18n } = useLingui()
  const stats = useTodayStats()
  const s = stats.data
  const fmt = new Intl.NumberFormat(i18n.locale)
  const rated = s ? s.ratings.good + s.ratings.bad : 0
  const pct = s && rated ? new Intl.NumberFormat(i18n.locale, { style: "percent" }).format(s.ratings.good / rated) : "–"
  const rows: [string, React.ReactNode, React.ReactNode, React.ReactNode, string][] = [
    ["today", <BarChart3Icon />, <Trans>Replies and closes</Trans>, s ? t`${fmt.format(s.replies)} replies · ${fmt.format(s.closed)} closed` : " ", "report-link-today"],
    ["ratings", <ThumbsUpIcon />, <Trans>Ratings</Trans>, s ? (rated ? t`${pct} satisfied` : t`No ratings today`) : " ", "report-link-ratings"],
    ["members", <UsersIcon />, <Trans>By member</Trans>, s ? t`${fmt.format(s.members.length)} active today` : " ", "report-link-members"],
  ]
  return (
    <Column title={<Trans>Reports</Trans>} testId="reports-panel" actions={<InboxPicker className="max-w-40" />}>
      <ColumnHeading>
        <Trans>Today</Trans>
      </ColumnHeading>
      <ErrorLine error={stats.error} className="px-2.5" />
      <ul className="flex flex-col">
        {rows.map(([to, icon, title, detail, testId]) => (
          <li key={to}>
            <ColumnLink to={`/reports/${to}`} testId={testId}>
              <RowIcon>{icon}</RowIcon>
              <RowText title={title} detail={detail} />
            </ColumnLink>
          </li>
        ))}
      </ul>
    </Column>
  )
}

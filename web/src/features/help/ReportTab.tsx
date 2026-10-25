import { Trans, useLingui } from "@lingui/react/macro"
import { useState } from "react"
import { Link } from "react-router"

import { Segmented } from "@/components/common"
import { Skeleton } from "@/components/ui/skeleton"
import { articleLink, useHelpReport } from "@/features/help/queries"
import { ErrorLine, useLocaleName } from "@/features/help/text"
import { Card, EmptyRow, Section } from "@/features/settings/ui"
import type { HelpCenter, HelpReportArticle } from "@/lib/api"

const WINDOWS = [7, 30, 90] as const

function Tile({ label, value, testId }: { label: string; value: number; testId: string }) {
  const { i18n } = useLingui()
  return (
    <div className="flex flex-col gap-1 rounded-[14px] border bg-card px-4 py-3.5" data-testid={testId}>
      <span className="text-small text-faint">{label}</span>
      <span className="text-[24px] leading-[30px] font-semibold tracking-[-0.01em] tabular-nums">
        {new Intl.NumberFormat(i18n.locale).format(value)}
      </span>
    </div>
  )
}

function ArticleTable({ center, items }: { center: HelpCenter; items: HelpReportArticle[] }) {
  const { i18n } = useLingui()
  const number = new Intl.NumberFormat(i18n.locale)
  const percent = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[34rem] text-body">
        <thead>
          <tr className="border-b text-start text-[11px] tracking-[0.05em] text-faint uppercase">
            <th className="px-4 py-2 text-start font-medium">
              <Trans>Article</Trans>
            </th>
            <th className="px-2 py-2 text-end font-medium">
              <Trans>Views</Trans>
            </th>
            <th className="px-2 py-2 text-end font-medium">
              <Trans>Helpful</Trans>
            </th>
            <th className="px-2 py-2 text-end font-medium">
              <Trans>Chats started</Trans>
            </th>
            <th className="px-4 py-2 text-end font-medium">
              <Trans>Sent</Trans>
            </th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {items.map((a) => {
            const votes = a.helpful_up + a.helpful_down
            return (
              <tr key={a.article_id} data-testid="help-report-article">
                <td className="max-w-0 px-4 py-2.5">
                  <Link
                    to={articleLink(center.id, a.article_id, center.default_locale)}
                    className="block truncate hover:underline"
                  >
                    {a.title}
                  </Link>
                </td>
                <td className="px-2 py-2.5 text-end tabular-nums">{number.format(a.views)}</td>
                <td className="px-2 py-2.5 text-end tabular-nums">
                  {votes > 0 ? (
                    <span title={`${number.format(a.helpful_up)} / ${number.format(votes)}`}>
                      {percent.format(a.helpful_up / votes)}
                    </span>
                  ) : (
                    <span className="text-faint">–</span>
                  )}
                </td>
                <td className="px-2 py-2.5 text-end tabular-nums">{number.format(a.conversations)}</td>
                <td className="px-4 py-2.5 text-end tabular-nums">{number.format(a.sent)}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

export function ReportTab({ center }: { center: HelpCenter }) {
  const { t, i18n } = useLingui()
  const localeName = useLocaleName()
  const [days, setDays] = useState<(typeof WINDOWS)[number]>(30)
  const report = useHelpReport(center.id, days)
  const r = report.data
  const number = new Intl.NumberFormat(i18n.locale)
  return (
    <div className="flex flex-col gap-6" data-testid="help-report">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-60 flex-1 text-body text-muted-foreground">
          <Trans>
            What visitors read and search for. Views are approximate, and counts reach this page within a minute.
          </Trans>
        </p>
        <Segmented
          value={String(days)}
          onChange={(v) => setDays(Number(v) as (typeof WINDOWS)[number])}
          label={t`Report period`}
          items={WINDOWS.map((d) => ({ value: String(d), label: t`Last ${d} days`, testId: `help-days-${d}` }))}
        />
      </div>
      {report.isPending ? (
        <Skeleton className="h-40 w-full rounded-[14px]" />
      ) : report.error || !r ? (
        <ErrorLine error={report.error} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Tile label={t`Article views`} value={r.views} testId="help-report-views" />
            <Tile label={t`Searches`} value={r.searches} testId="help-report-searches" />
            <Tile label={t`Searches with no result`} value={r.no_result_searches} testId="help-report-no-result" />
            <Tile label={t`Results opened`} value={r.clicks} testId="help-report-clicks" />
          </div>
          <Section title={<Trans>Top articles</Trans>}>
            <Card flush>
              {r.top_articles.length === 0 ? (
                <EmptyRow>
                  <Trans>No article was read in this period.</Trans>
                </EmptyRow>
              ) : (
                <ArticleTable center={center} items={r.top_articles} />
              )}
            </Card>
          </Section>
          <Section
            title={<Trans>Searches with no result</Trans>}
            description={
              <Trans>Words visitors typed that found nothing. A new article for each is the fastest fix.</Trans>
            }
          >
            <Card flush>
              {r.no_result_queries.length === 0 ? (
                <EmptyRow>
                  <Trans>Every search found something.</Trans>
                </EmptyRow>
              ) : (
                <ul className="divide-y" data-testid="help-report-queries">
                  {r.no_result_queries.map((q) => (
                    <li key={`${q.locale}:${q.query}`} className="flex items-center gap-3 px-4 py-2.5">
                      <span className="min-w-0 flex-1 truncate text-body" dir="auto">
                        {q.query}
                      </span>
                      <span className="shrink-0 text-small text-faint">{localeName(q.locale)}</span>
                      <span className="w-12 shrink-0 text-end text-body tabular-nums">{number.format(q.searches)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </Section>
          <Section title={<Trans>Articles that did not help</Trans>}>
            <Card flush>
              {r.most_down.length === 0 ? (
                <EmptyRow>
                  <Trans>No article got a “no” in this period.</Trans>
                </EmptyRow>
              ) : (
                <ArticleTable center={center} items={r.most_down} />
              )}
            </Card>
          </Section>
        </>
      )}
    </div>
  )
}

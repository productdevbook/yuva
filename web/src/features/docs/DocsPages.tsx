import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { BookOpenTextIcon, ExternalLinkIcon } from "lucide-react"
import { Link, useSearchParams } from "react-router"

import { ContactAvatar, Dot, ErrorLine } from "@/components/common"
import { Pane, PaneBack, PaneEmpty } from "@/components/common/Column"
import { formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import type { ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import { CategoryChip } from "@/features/conversation/Feedback"
import { AnswerCard, pageLabel, useAnswerActions } from "@/features/docs/AnswerDialog"
import { DailyChart, zeroFill } from "@/features/docs/DailyChart"
import { DocsGuide, DocsWindow, helpfulShare } from "@/features/docs/DocsColumn"
import { PageRatingMark } from "@/features/docs/PageContext"
import { useDocsDays, useDocsPage, useDocsPages, useDocsSort, useDocsSummary, usePageAnswers } from "@/features/docs/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useConversations } from "@/features/inbox/queries"
import { Card, EmptyRow, Section } from "@/features/settings/ui"
import type { ConversationListItem, DocsPageDay } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export function DocsHome() {
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const [sort] = useDocsSort()
  const pages = useDocsPages(inboxId, days, sort)
  const none = !!pages.data && pages.data.pages[0]?.items.length === 0
  if (none) {
    return (
      <PaneEmpty icon={BookOpenTextIcon} title={<Trans>Feedback and questions from your documentation</Trans>} testId="docs-home">
        <DocsGuide />
      </PaneEmpty>
    )
  }
  return <DocsOverview />
}

function Tiles({ tiles, testId }: { tiles: [string, string, string?][]; testId: string }) {
  return (
    <div className="grid grid-cols-3 gap-2 phone:grid-cols-2" data-testid={testId}>
      {tiles.map(([value, label, tone]) => (
        <div key={label} className="rounded-2xl border bg-card px-4 py-3.5">
          <b className={cn("block text-page tabular-nums", tone)}>{value}</b>
          <span className="text-small text-faint">{label}</span>
        </div>
      ))}
    </div>
  )
}

export function DocsOverview() {
  const { t, i18n } = useLingui()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const summary = useDocsSummary(inboxId, days)
  const ratings = useRatingConfig()
  const conversations: ChartConfig = {
    feedback: { label: t`Feedback`, theme: { light: "#c2410c", dark: "#ea580c" } },
    questions: { label: t`Questions`, theme: { light: "#0891b2", dark: "#0891b2" } },
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const s = summary.data
  return (
    <Pane testId="docs-overview">
      <header className="flex flex-col">
        <PaneBack to="/docs" label={t`Docs`} />
        <h1 className="min-w-0 text-page break-words">
          <Trans>Documentation pages</Trans>
        </h1>
        <p className="mt-1.5 text-reading text-muted-foreground">
          <Plural value={days} one="The last day, over the inboxes you can see." other="The last # days, over the inboxes you can see." />
        </p>
      </header>
      <DocsWindow className="hidden phone:flex" />
      {summary.isPending && <Skeleton className="h-28 rounded-2xl" />}
      <ErrorLine error={summary.error} />
      {s && (
        <>
          <Tiles
            testId="docs-summary-tiles"
            tiles={[
              [fmt.format(s.totals.up), t`helpful`, "text-success"],
              [fmt.format(s.totals.down), t`not helpful`, "text-destructive"],
              [helpfulShare(s.totals, i18n.locale), t`helpful share`],
              [fmt.format(s.totals.feedback), t`feedback`],
              [fmt.format(s.totals.questions), t`questions`],
              [fmt.format(s.totals.published_answers), t`answers published`],
            ]}
          />
          <Section title={<Trans>Ratings per day</Trans>}>
            <Card>
              <DailyChart rows={s.days} config={ratings} stacked testId="docs-summary-ratings" />
            </Card>
          </Section>
          <Section title={<Trans>Feedback and questions per day</Trans>}>
            <Card>
              <DailyChart rows={s.days.map(({ day, feedback, questions }) => ({ day, feedback, questions }))} config={conversations} className="h-36" testId="docs-summary-conversations" />
            </Card>
          </Section>
        </>
      )}
    </Pane>
  )
}

export function useRatingConfig(): ChartConfig {
  const { t } = useLingui()
  return {
    up: { label: t`Helpful`, theme: { light: "#15803d", dark: "#0d9488" } },
    down: { label: t`Not helpful`, theme: { light: "#dc2626", dark: "#ef4444" } },
  }
}

function Days({ days, window }: { days: DocsPageDay[]; window: number }) {
  const config = useRatingConfig()
  if (!days.some((d) => d.up + d.down > 0)) {
    return (
      <Card flush>
        <EmptyRow>
          <Trans>No ratings in this window.</Trans>
        </EmptyRow>
      </Card>
    )
  }
  return (
    <Card>
      <DailyChart rows={zeroFill(days, window, { up: 0, down: 0 })} config={config} stacked testId="docs-days" />
    </Card>
  )
}

function ConversationRow({ c }: { c: ConversationListItem }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const name = c.contact.name || c.contact.email || t`Visitor`
  return (
    <li>
      <Link to={`/conversations/${c.id}`} className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted" data-testid="docs-conversation">
        <ContactAvatar id={c.contact_id} name={name} className="size-9 shrink-0" />
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-body font-medium">{name}</span>
            <time className="shrink-0 text-caption text-faint" dateTime={c.last_activity_at}>
              {formatShort(c.last_activity_at, i18n.locale)}
            </time>
          </span>
          <span className="flex items-center gap-2 text-small">
            <span className="min-w-0 flex-1 truncate text-muted-foreground">{c.last_message?.text || c.subject}</span>
            {c.feedback?.rating && <PageRatingMark rating={c.feedback.rating} />}
            {c.feedback?.category && c.feedback.category !== "other" && <CategoryChip category={c.feedback.category} />}
            <span className={cn("shrink-0 text-caption", c.status === "open" ? "text-brand" : "text-faint")}>{text.status[c.status]}</span>
          </span>
        </span>
      </Link>
    </li>
  )
}

function Conversations({ inboxId, page, kind, empty, title }: { inboxId: string; page: string; kind: "feedback" | "question"; empty: React.ReactNode; title: React.ReactNode }) {
  const list = useConversations({ inbox_id: inboxId, page, kind })
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Section title={title}>
      <Card flush data-testid={`docs-${kind}`}>
        {list.isPending ? (
          <Skeleton className="m-4 h-10" />
        ) : list.error ? (
          <ErrorLine error={list.error} className="p-4" />
        ) : items.length === 0 ? (
          <EmptyRow>{empty}</EmptyRow>
        ) : (
          <ul className="divide-y">
            {items.map((c) => (
              <ConversationRow key={c.id} c={c} />
            ))}
          </ul>
        )}
        {list.hasNextPage && (
          <div className="flex justify-center border-t p-2">
            <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </Card>
    </Section>
  )
}

function Answers({ inboxId, page }: { inboxId: string; page: string }) {
  const answers = usePageAnswers({ inbox_id: inboxId, page })
  const actions = useAnswerActions()
  const items = answers.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <Section title={<Trans>Published answers</Trans>} description={<Trans>Shown on the page by the questions element, newest first, without the visitor's name or address.</Trans>}>
      <Card flush data-testid="docs-answers">
        {answers.isPending ? (
          <Skeleton className="m-4 h-16" />
        ) : answers.error ? (
          <ErrorLine error={answers.error} className="p-4" />
        ) : items.length === 0 ? (
          <EmptyRow>
            <Trans>Nothing published yet. Open a question you answered and publish it to the page.</Trans>
          </EmptyRow>
        ) : (
          <div className="divide-y">
            {items.map((a) => (
              <AnswerCard
                key={a.id}
                a={a}
                onEdit={() => actions.edit(a)}
                onUnpublish={() => actions.unpublish(a)}
                footer={
                  a.conversation_id && (
                    <Button variant="ghost" size="sm" render={<Link to={`/conversations/${a.conversation_id}`} />} data-testid="answer-conversation">
                      <Trans>Conversation</Trans>
                    </Button>
                  )
                }
              />
            ))}
          </div>
        )}
        {answers.hasNextPage && (
          <div className="flex justify-center border-t p-2">
            <Button variant="ghost" size="sm" onClick={() => void answers.fetchNextPage()} disabled={answers.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </Card>
      {actions.dialogs}
    </Section>
  )
}

export function DocsPagePane() {
  const { t, i18n } = useLingui()
  const [params] = useSearchParams()
  const inboxId = params.get("inbox") ?? ""
  const page = params.get("url") ?? ""
  const [days] = useDocsDays()
  const detail = useDocsPage(inboxId, page, days)
  const inbox = (useInboxes().data ?? []).find((i) => i.id === inboxId)
  const d = detail.data?.page
  const fmt = new Intl.NumberFormat(i18n.locale)
  const tiles: [string, string, string?][] = d
    ? [
        [fmt.format(d.up), t`helpful`, "text-success"],
        [fmt.format(d.down), t`not helpful`, "text-destructive"],
        [helpfulShare(d, i18n.locale), t`helpful share`],
        [fmt.format(d.open_feedback), t`open feedback`],
        [fmt.format(d.open_questions), t`open questions`],
        [fmt.format(d.published_answers), t`published answers`],
      ]
    : []
  return (
    <Pane testId="docs-page" key={`${inboxId} ${page}`}>
      <header className="flex flex-col">
        <PaneBack to="/docs" label={t`Docs`} />
        <h1 className="min-w-0 text-page break-words">{d?.title || pageLabel(page)}</h1>
        <p className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-body text-muted-foreground">
          {inbox && (
            <span className="inline-flex items-center gap-1.5">
              <Dot color={inbox.branding.color} className="size-[7px] rounded-[2px]" />
              {inbox.name}
            </span>
          )}
          <a href={page} target="_blank" rel="noreferrer" className="inline-flex min-w-0 items-center gap-1 break-all text-brand underline-offset-4 hover:underline" dir="ltr" data-testid="docs-page-url">
            {page}
            <ExternalLinkIcon className="size-3.5 shrink-0" />
          </a>
        </p>
      </header>
      <DocsWindow className="hidden phone:flex" />
      {detail.isPending && <Skeleton className="h-28 rounded-2xl" />}
      <ErrorLine error={detail.error} />
      {d && (
        <>
          <Tiles tiles={tiles} testId="docs-tiles" />
          <Section title={<Trans>Ratings per day</Trans>}>
            <Days days={detail.data?.days ?? []} window={days} />
          </Section>
          <Conversations inboxId={inboxId} page={page} kind="question" title={<Trans>Questions</Trans>} empty={<Trans>No questions from this page.</Trans>} />
          <Conversations inboxId={inboxId} page={page} kind="feedback" title={<Trans>Feedback</Trans>} empty={<Trans>No feedback from this page.</Trans>} />
          <Answers inboxId={inboxId} page={page} />
        </>
      )}
    </Pane>
  )
}

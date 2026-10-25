import { Trans, useLingui } from "@lingui/react/macro"
import {
  AlertCircleIcon,
  ExternalLinkIcon,
  FileTextIcon,
  HourglassIcon,
  MessageSquareTextIcon,
  MessagesSquareIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
} from "lucide-react"
import { useEffect, useRef } from "react"
import { Link, useLocation, useNavigate, useSearchParams } from "react-router"

import { ContactAvatar, Dot, ErrorLine, InboxMark } from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { DailyChart, zeroFill } from "@/components/common/DailyChart"
import { DISCUSSION_KEYS } from "@/components/common/ShortcutSheet"
import { formatShort, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import type { ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"
import { useAnalyticsProperties } from "@/features/analytics/properties"
import { useAnalytics } from "@/features/analytics/queries"
import { CategoryChip } from "@/features/conversation/Feedback"
import { ThreadList } from "@/features/docs/discussions/DiscussionsPane"
import { pageLabel, PageRatingMark } from "@/features/docs/PageContext"
import {
  docsPageLink,
  useDocsDays,
  useDocsPage,
  useDocsPages,
  useDocsSort,
  useDocsSummary,
} from "@/features/docs/queries"
import { usePickedSite } from "@/features/docs/Toolbar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { look } from "@/features/inbox/PageFrame"
import { useConversations } from "@/features/inbox/queries"
import { Card, EmptyRow, Section } from "@/features/settings/ui"
import type { AnalyticsFilter, ConversationListItem, DocsPage, DocsPageDay } from "@/lib/api"
import type { DocsDays } from "@/lib/keys"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import { useResetState } from "@/hooks/use-reset-state"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export const GUIDE_URL = "https://github.com/productdevbook/yuva/blob/main/docs/documentation-pages.md"

export function ShareBar({ up, down, className }: { up: number; down: number; className?: string }) {
  const rated = up + down
  return (
    <span
      className={cn("flex h-1 overflow-hidden rounded-full", rated ? "gap-px" : "bg-muted", className)}
      aria-hidden
      data-testid="share-bar"
    >
      {rated > 0 && (
        <>
          {up > 0 && <span className="rounded-full bg-success" style={{ width: `${(up / rated) * 100}%` }} />}
          {down > 0 && <span className="flex-1 rounded-full bg-destructive" />}
        </>
      )}
    </span>
  )
}

export function helpfulShare(p: { up: number; down: number }, locale: string) {
  const rated = p.up + p.down
  return rated ? new Intl.NumberFormat(locale, { style: "percent" }).format(p.up / rated) : "–"
}

function Count({
  n,
  icon: Icon,
  label,
  className,
}: {
  n: number
  icon: React.ComponentType<{ className?: string }>
  label: string
  className?: string
}) {
  return (
    <span
      className={cn("inline-flex shrink-0 items-center gap-1 tabular-nums", n === 0 && "opacity-45", className)}
      title={label}
    >
      <Icon className="size-3.5" />
      <span className="sr-only">{label}</span>
      <span aria-hidden>{n}</span>
    </span>
  )
}

const ROW_COLS =
  "grid grid-cols-[minmax(0,2.2fr)_minmax(150px,1.2fr)_72px_104px] items-center gap-3 px-4 @max-3xl:grid-cols-[minmax(0,1fr)_auto]"

function PageRow({
  p,
  current,
  cursor,
  inboxColor,
}: {
  p: DocsPage
  current: boolean
  cursor: boolean
  inboxColor?: string
}) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const share = helpfulShare(p, i18n.locale)
  const [up, down, feedback, threads, pending] = [p.up, p.down, p.open_feedback, p.open_threads, p.pending_posts].map(
    (n) => fmt.format(n)
  )
  const rated = p.up + p.down
  const low = rated > 0 && p.up / rated < 0.7
  return (
    <li>
      <Link
        to={docsPageLink(p.inbox_id, p.page)}
        className={cn(
          ROW_COLS,
          "min-h-[54px] border-b py-2 text-small outline-none transition-colors hover:bg-surface/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
          current && "bg-brand/5",
          cursor && "bg-surface ring-2 ring-ring ring-inset"
        )}
        aria-current={current ? "page" : undefined}
        data-testid="docs-row"
      >
        <span className="flex min-w-0 flex-col">
          <span className="flex items-center gap-2">
            {inboxColor && <Dot color={inboxColor} className="size-[7px] shrink-0 rounded-[2px]" />}
            <span className="min-w-0 flex-1 truncate text-body font-semibold">{p.title || pageLabel(p.page)}</span>
          </span>
          <span className="truncate text-caption text-faint" dir="ltr">
            {pageLabel(p.page)}
          </span>
        </span>
        <span className="flex items-center gap-2.5">
          <ShareBar up={p.up} down={p.down} className="h-2 w-[84px] shrink-0 @max-3xl:hidden" />
          <span
            className={cn(
              "tabular-nums",
              rated === 0 ? "text-faint" : low ? "font-semibold text-destructive" : "text-foreground"
            )}
            title={t`${share} helpful`}
            data-testid="helpful-share"
          >
            <span className="sr-only">{t`${share} helpful`}</span>
            <span aria-hidden>{share}</span>
          </span>
          <span className="flex items-center gap-2 text-caption @max-3xl:hidden">
            <Count n={p.up} icon={ThumbsUpIcon} label={t`${up} helpful`} className="text-success" />
            <Count n={p.down} icon={ThumbsDownIcon} label={t`${down} not helpful`} className="text-destructive" />
          </span>
        </span>
        <span className="@max-3xl:hidden">
          <Count
            n={p.open_feedback}
            icon={MessageSquareTextIcon}
            label={t`${feedback} open feedback`}
            className={p.open_feedback ? "text-brand" : undefined}
          />
        </span>
        <span className="flex items-center gap-2 @max-3xl:hidden">
          <Count
            n={p.open_threads}
            icon={MessagesSquareIcon}
            label={t`${threads} open discussions`}
            className={p.unanswered_threads ? "text-brand" : undefined}
          />
          {p.pending_posts > 0 && (
            <span
              className="inline-flex h-5 items-center gap-1 rounded-md bg-warning/10 px-1.5 text-[11.5px] font-semibold text-warning tabular-nums"
              title={t`${pending} posts waiting for approval`}
            >
              <HourglassIcon className="size-3" />
              {pending}
            </span>
          )}
        </span>
      </Link>
    </li>
  )
}

function Tiles({ tiles, testId }: { tiles: [string, string, string?][]; testId: string }) {
  return (
    <div className="grid grid-cols-3 gap-2.5 phone:grid-cols-2" data-testid={testId}>
      {tiles.map(([value, label, tone]) => (
        <div key={label} className="flex min-w-0 flex-col gap-0.5 rounded-[14px] border bg-card px-3.5 py-3">
          <span className="truncate text-small text-faint">{label}</span>
          <b className={cn("block text-[20px] leading-7 font-semibold tracking-[-0.01em] tabular-nums", tone)}>
            {value}
          </b>
        </div>
      ))}
    </div>
  )
}

function SummaryTiles({ inboxId, site, days }: { inboxId: string; site: string; days: DocsDays }) {
  const { t, i18n } = useLingui()
  const summary = useDocsSummary(inboxId, site, days)
  const totals = summary.data?.totals
  if (summary.isPending) {
    return (
      <div className="grid grid-cols-4 gap-2.5 @max-3xl:grid-cols-2" aria-busy="true">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-[92px] rounded-[14px]" />
        ))}
      </div>
    )
  }
  if (!totals) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const [up, down] = [totals.up, totals.down].map((n) => fmt.format(n))
  const figures: { label: string; value: string; sub?: string; tone?: string }[] = [
    {
      label: t`helpful share`,
      value: helpfulShare(totals, i18n.locale),
      sub: totals.up + totals.down > 0 ? `${t`${up} helpful`} · ${t`${down} not helpful`}` : undefined,
    },
    { label: t`feedback`, value: fmt.format(totals.feedback) },
    { label: t`new discussions`, value: fmt.format(totals.threads) },
    {
      label: t`waiting for approval`,
      value: fmt.format(totals.pending_posts),
      tone: totals.pending_posts ? "text-warning" : undefined,
    },
  ]
  return (
    <div className="grid grid-cols-4 gap-2.5 @max-3xl:grid-cols-2" data-testid="docs-summary">
      {figures.map((f) => (
        <div key={f.label} className="flex min-w-0 flex-col gap-0.5 rounded-[14px] border bg-card px-4 py-3.5">
          <span className="block truncate text-small text-faint first-letter:uppercase">{f.label}</span>
          <b className={cn("block text-[24px] leading-[30px] font-semibold tracking-[-0.01em] tabular-nums", f.tone)}>
            {f.value}
          </b>
          <span className="truncate text-caption text-faint">{f.sub ?? "\u00a0"}</span>
        </div>
      ))}
    </div>
  )
}

function useRatingConfig(): ChartConfig {
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
      <Link
        to={`/conversations/${c.id}`}
        className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted"
        data-testid="docs-conversation"
      >
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
            {c.feedback?.category && c.feedback.category !== "other" && (
              <CategoryChip category={c.feedback.category} inboxId={c.inbox_id} />
            )}
            <span className={cn("shrink-0 text-caption", c.status === "open" ? "text-brand" : "text-faint")}>
              {text.statusOf(c.status, c.kind)}
            </span>
          </span>
        </span>
      </Link>
    </li>
  )
}

function Conversations({
  inboxId,
  page,
  kind,
  empty,
  title,
}: {
  inboxId: string
  page: string
  kind: "feedback" | "conversation"
  empty: React.ReactNode
  title: React.ReactNode
}) {
  const list = useConversations({ inbox_id: inboxId, page, kind: [kind] })
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
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void list.fetchNextPage()}
              disabled={list.isFetchingNextPage}
            >
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </Card>
    </Section>
  )
}

function hostMatches(domain: string, host: string) {
  const d = domain.toLowerCase()
  return d.startsWith("*.") ? host.endsWith(d.slice(1)) : host === d
}

function PageTraffic({
  inboxId,
  page,
  days,
  ratings,
}: {
  inboxId: string
  page: string
  days: DocsDays
  ratings: number
}) {
  const { t, i18n } = useLingui()
  const url = URL.canParse(page) ? new URL(page) : null
  const host = url?.hostname.toLowerCase() ?? ""
  const linked = (useAnalyticsProperties().data?.items ?? []).filter((p) => p.inbox_id === inboxId)
  const property = linked.find((p) => p.domains.some((d) => hostMatches(d, host))) ?? linked[0]
  const period = `${days}d` as const
  const filters: AnalyticsFilter[] = [
    { dimension: "page", op: "is", values: [url?.pathname ?? ""] },
    { dimension: "hostname", op: "is", values: [host] },
  ]
  const q = useAnalytics(property?.id ?? "", { metrics: ["pageviews", "visitors"], range: { period }, filters }, !!url)
  if (!property || !url) return null
  const fmt = new Intl.NumberFormat(i18n.locale)
  const views = q.data?.totals.pageviews ?? 0
  const visitors = q.data?.totals.visitors ?? 0
  const rate =
    views > 0 ? new Intl.NumberFormat(i18n.locale, { maximumFractionDigits: 1 }).format((ratings / views) * 1000) : "–"
  const search = new URLSearchParams([
    ["period", period],
    ["f", `page:is:${url.pathname}`],
    ["f", `hostname:is:${host}`],
  ])
  return (
    <Section
      title={<Trans>Views</Trans>}
      action={
        <Link
          to={`/analytics/${property.id}/pages?${search}`}
          className="text-small text-brand underline-offset-4 hover:underline"
          data-testid="docs-page-analytics"
        >
          <Trans>Open in Analytics</Trans>
        </Link>
      }
    >
      {q.isPending ? (
        <Skeleton className="h-20 rounded-[14px]" />
      ) : q.error ? (
        <ErrorLine error={q.error} />
      ) : (
        <Tiles
          testId="docs-page-traffic"
          tiles={[
            [fmt.format(views), t`views`],
            [fmt.format(visitors), t`visitors`],
            [rate, t`ratings per 1,000 views`],
          ]}
        />
      )}
    </Section>
  )
}

function PageDetail({
  inboxId,
  page,
  sheetHref,
  back,
  embedded,
}: {
  inboxId: string
  page: string
  sheetHref: (id: string) => string
  back?: { to: string; label: string }
  embedded?: boolean
}) {
  const { t, i18n } = useLingui()
  const [days] = useDocsDays()
  const site = usePickedSite("")
  const detail = useDocsPage(inboxId, page, site, days)
  const inbox = (useInboxes().data ?? []).find((i) => i.id === inboxId)
  const d = detail.data?.page
  const fmt = new Intl.NumberFormat(i18n.locale)
  const tiles: [string, string, string?][] = d
    ? [
        [fmt.format(d.up), t`helpful`, "text-success"],
        [fmt.format(d.down), t`not helpful`, "text-destructive"],
        [helpfulShare(d, i18n.locale), t`helpful share`],
        [fmt.format(d.open_feedback), t`open feedback`],
        [fmt.format(d.open_threads), t`open discussions`],
        [fmt.format(d.pending_posts), t`waiting for approval`, d.pending_posts ? "text-warning" : undefined],
      ]
    : []
  return (
    <div className={cn("flex flex-col gap-4", embedded ? "min-w-0" : "p-4 pb-16 phone:pt-4")} data-testid="docs-page">
      {back && (
        <div className="-mb-3">
          <PaneBack to={back.to} label={back.label} always />
        </div>
      )}
      <div className="flex flex-col gap-0.5">
        <h2 className="text-[18px] leading-6 font-semibold tracking-[-0.01em] break-words">
          {d?.title || pageLabel(page)}
        </h2>
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-body text-muted-foreground">
          {inbox && (
            <span className="inline-flex items-center gap-1.5">
              <InboxMark name={inbox.name} color={inbox.branding.color} className="size-4 text-[9px]" />
              {inbox.name}
            </span>
          )}
          <a
            href={page}
            target="_blank"
            rel="noreferrer"
            className="inline-flex min-w-0 items-center gap-1 break-all text-brand underline-offset-4 hover:underline"
            dir="ltr"
            data-testid="docs-page-url"
          >
            {page}
            <ExternalLinkIcon className="size-3.5 shrink-0" />
          </a>
        </span>
      </div>
      {detail.isPending && <Skeleton className="h-28 rounded-[14px]" />}
      <ErrorLine error={detail.error} />
      {d && (
        <>
          <Tiles tiles={tiles} testId="docs-tiles" />
          <PageTraffic inboxId={inboxId} page={page} days={days} ratings={d.up + d.down} />
          <Section title={<Trans>Ratings per day</Trans>}>
            <Days days={detail.data?.days ?? []} window={days} />
          </Section>
          <Section title={<Trans>Discussions</Trans>}>
            <ThreadList
              filters={{ inbox_id: inboxId, page }}
              showPage={false}
              link={(x) => sheetHref(x.id)}
              empty={<Trans>No discussions on this page yet.</Trans>}
            />
          </Section>
          <Conversations
            inboxId={inboxId}
            page={page}
            kind="conversation"
            title={<Trans>Conversations</Trans>}
            empty={<Trans>No conversations from this page.</Trans>}
          />
          <Conversations
            inboxId={inboxId}
            page={page}
            kind="feedback"
            title={<Trans>Feedback</Trans>}
            empty={<Trans>No feedback from this page.</Trans>}
          />
        </>
      )}
    </div>
  )
}

function DocsEmpty({ onInstall }: { onInstall: () => void }) {
  const steps = [
    {
      title: <Trans>Add the two elements under every docs page</Trans>,
      text: (
        <Trans>
          Pick a chat channel of the product and paste the snippet. Ratings and questions land in its inbox.
        </Trans>
      ),
    },
    {
      title: <Trans>Decide who may write in discussions</Trans>,
      text: (
        <Trans>A new reader’s first post waits for your approval. You can change this in the inbox settings.</Trans>
      ),
    },
    {
      title: <Trans>Open a page of your docs</Trans>,
      text: <Trans>The first rating makes the page appear here.</Trans>,
    },
  ]
  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-surface px-6 py-8 phone:px-4" data-testid="docs-empty">
      <div className="mx-auto flex max-w-[760px] flex-col gap-5">
        <div className="flex flex-col items-start gap-1.5">
          <span className="mb-1 grid size-12 place-items-center rounded-2xl bg-muted text-faint">
            <FileTextIcon className="size-5" />
          </span>
          <h2 className="text-[20px] leading-[1.3] font-semibold tracking-[-0.01em]">
            <Trans>No pages yet</Trans>
          </h2>
          <p className="text-body text-muted-foreground">
            <Trans>
              Docs are the pages you keep on your own site and host yourself. Once Yuva’s two elements are on them,
              every page shows up here with its ratings, feedback and discussions.
            </Trans>
          </p>
        </div>
        <div className="grid grid-cols-2 gap-3 phone:grid-cols-1">
          <div className={cn(look.card, "flex flex-col gap-1.5 rounded-[14px] border-brand p-4")}>
            <span className={look.over}>
              <Trans>Docs</Trans>
            </span>
            <span className="font-semibold">
              <Trans>Pages on your own site</Trans>
            </span>
            <span className="text-muted-foreground">
              <Trans>
                You write and host them. Yuva adds a rating and a discussion under each page and shows the results here.
              </Trans>
            </span>
          </div>
          <div className={cn(look.card, "flex flex-col gap-1.5 rounded-[14px] p-4")}>
            <span className={look.over}>
              <Trans>Help center</Trans>
            </span>
            <span className="font-semibold">
              <Trans>Articles you write in Yuva</Trans>
            </span>
            <span className="text-muted-foreground">
              <Trans>
                Hosted by Yuva on its own address. Searched from the chat widget and the reply box. For support answers.
              </Trans>
            </span>
            <span className="mt-1">
              <Button variant="outline" className={look.btnSm} render={<Link to="/knowledge" />}>
                <Trans>Open Help center</Trans>
              </Button>
            </span>
          </div>
        </div>
        <div className={cn(look.card, "rounded-[14px]")}>
          <ol className="flex flex-col gap-3 p-4">
            {steps.map((s, i) => (
              <li key={i} className="flex items-start gap-3">
                <span className="grid size-[22px] shrink-0 place-items-center rounded-full bg-brand-wash text-small font-semibold text-brand">
                  {i + 1}
                </span>
                <span className="flex flex-col leading-[1.4]">
                  <span className="font-medium">{s.title}</span>
                  <span className="text-muted-foreground">{s.text}</span>
                </span>
              </li>
            ))}
          </ol>
          <div className="flex flex-wrap gap-2 border-t px-4 py-3">
            <Button className={look.btnInk} onClick={onInstall} data-testid="docs-empty-install">
              <Trans>How to add</Trans>
            </Button>
            <Button
              variant="outline"
              className={look.btn}
              render={<a href={GUIDE_URL} target="_blank" rel="noreferrer" />}
              data-testid="docs-guide"
            >
              <Trans>Read the guide</Trans>
              <ExternalLinkIcon className="text-faint" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function PagesTab({
  sheetHref,
  sheetOpen,
  onInstall,
}: {
  sheetHref: (id: string) => string
  sheetOpen: boolean
  onInstall: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const [params] = useSearchParams()
  const { pathname } = useLocation()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const [sort] = useDocsSort()
  const site = usePickedSite(inboxId)
  const inboxes = useInboxes().data ?? []
  const colors = new Map(inboxes.map((i) => [i.id, i.branding.color]))
  const pages = useDocsPages(inboxId, site, days, sort)
  const items = pages.data?.pages.flatMap((p) => p.items) ?? []
  const pickedInbox = pathname === "/docs/page" ? params.get("inbox") : null
  const pickedUrl = pathname === "/docs/page" ? params.get("url") : null
  const hasPick = !!pickedInbox && !!pickedUrl
  const first = items[0]
  const selected = hasPick
    ? { inbox: pickedInbox, url: pickedUrl }
    : !phone && first
      ? { inbox: first.inbox_id, url: first.page }
      : null
  const [cursor, setCursor] = useResetState(-1, JSON.stringify([inboxId, site, days, sort]))
  const list = useRef<HTMLUListElement>(null)
  const down = () => setCursor((i) => Math.min(items.length - 1, i + 1))
  const up = () => setCursor((i) => Math.max(0, i - 1))
  useHotkeys(
    {
      [DISCUSSION_KEYS.next]: down,
      [DISCUSSION_KEYS.previous]: up,
      ArrowDown: down,
      ArrowUp: up,
      Enter: () => {
        const p = items[cursor]
        if (p) navigate(docsPageLink(p.inbox_id, p.page))
      },
    },
    !sheetOpen && items.length > 0
  )
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=docs-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  if (pages.isPending) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto bg-surface" aria-busy="true">
        <div className="mx-auto flex max-w-[1360px] flex-col gap-4 px-6 py-5 phone:px-4">
          <div className="grid grid-cols-4 gap-2.5 phone:grid-cols-2">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-[92px] rounded-[14px]" />
            ))}
          </div>
          <Skeleton className="h-[420px] rounded-[14px]" />
        </div>
      </div>
    )
  }
  if (pages.error)
    return (
      <div className="min-h-0 flex-1 overflow-y-auto bg-surface px-6 py-12 phone:px-4">
        <div className="mx-auto flex max-w-sm flex-col items-center gap-2.5 text-center">
          <span className="mb-1 grid size-10 place-items-center rounded-full bg-destructive/10 text-destructive">
            <AlertCircleIcon className="size-5" />
          </span>
          <ErrorLine error={pages.error} />
        </div>
      </div>
    )
  if (items.length === 0 && !hasPick) return <DocsEmpty onInstall={onInstall} />

  if (phone && selected) {
    return (
      <div className="min-h-0 flex-1 overflow-y-auto bg-surface">
        <PageDetail
          inboxId={selected.inbox}
          page={selected.url}
          sheetHref={sheetHref}
          back={{ to: "/docs", label: t`Pages` }}
          key={`${selected.inbox} ${selected.url}`}
        />
      </div>
    )
  }
  const currentKey = selected ? `${selected.inbox} ${selected.url}` : ""
  return (
    <div className="@container min-h-0 flex-1 overflow-y-auto bg-surface">
      <div className="mx-auto flex max-w-[1360px] flex-col gap-4 px-6 py-5 phone:px-4">
        <SummaryTiles inboxId={inboxId} site={site} days={days} />
        <div className="grid grid-cols-[minmax(0,1fr)_420px] items-start gap-4 @max-5xl:grid-cols-1">
          <section aria-label={t`Documentation pages`} className={cn(look.card, "rounded-[14px] bg-card")}>
            <div className={cn(ROW_COLS, "h-9 border-b bg-surface/50", look.over, "@max-3xl:hidden")} aria-hidden>
              <span>
                <Trans>Page</Trans>
              </span>
              <span>
                <Trans>Helpful</Trans>
              </span>
              <span>
                <Trans>Feedback</Trans>
              </span>
              <span>
                <Trans>Discussions</Trans>
              </span>
            </div>
            <ul ref={list} className="flex flex-col [&>li:last-child>a]:border-b-0">
              {items.map((p, i) => (
                <PageRow
                  key={`${p.inbox_id} ${p.page}`}
                  p={p}
                  current={`${p.inbox_id} ${p.page}` === currentKey}
                  cursor={i === cursor}
                  inboxColor={inboxes.length > 1 && !inboxId ? colors.get(p.inbox_id) : undefined}
                />
              ))}
            </ul>
            {pages.hasNextPage && (
              <div className="flex justify-center border-t p-3">
                <Button
                  variant="outline"
                  className={look.btnSm}
                  onClick={() => void pages.fetchNextPage()}
                  disabled={pages.isFetchingNextPage}
                >
                  <Trans>Load more</Trans>
                </Button>
              </div>
            )}
            <p className="border-t px-4 py-2.5 text-caption text-faint">
              <Trans>
                Pages live on your own site. Articles hosted by Yuva are in{" "}
                <Link to="/knowledge" className="text-brand hover:underline">
                  Help center
                </Link>
                .
              </Trans>
            </p>
          </section>
          {!phone && (
            <div className="min-w-0">
              {selected ? (
                <PageDetail
                  embedded
                  inboxId={selected.inbox}
                  page={selected.url}
                  sheetHref={sheetHref}
                  key={`${selected.inbox} ${selected.url}`}
                />
              ) : (
                <p className="px-6 py-12 text-center text-body text-faint">
                  <Trans>Pick a page to see its ratings, discussions and questions.</Trans>
                </p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

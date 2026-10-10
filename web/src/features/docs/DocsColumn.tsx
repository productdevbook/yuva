import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { BadgeCheckIcon, BarChart3Icon, MessageCircleQuestionMarkIcon, MessageSquareTextIcon, ThumbsDownIcon, ThumbsUpIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useLocation, useNavigate, useSearchParams } from "react-router"

import { Dot, ErrorLine, Segmented } from "@/components/common"
import { Column, ColumnLink } from "@/components/common/Column"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { pageLabel } from "@/features/docs/AnswerDialog"
import { docsPageLink, SORTS, useDocsDays, useDocsPages, useDocsSort, WINDOWS } from "@/features/docs/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { InboxPicker } from "@/features/inbox/InboxPicker"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { DocsPage, DocsPageSort } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export const GUIDE_URL = "https://github.com/productdevbook/yuva/blob/main/docs/documentation-pages.md"

export function DocsGuide() {
  return (
    <>
      <p>
        <Trans>
          Add the page feedback and questions elements to your documentation site, on one of the inbox's chat channels. Ratings, feedback and questions then show up here per page.
        </Trans>
      </p>
      <a href={GUIDE_URL} target="_blank" rel="noreferrer" className="mt-2 inline-block text-brand underline-offset-4 hover:underline" data-testid="docs-guide">
        <Trans>How to add them</Trans>
      </a>
    </>
  )
}

export function ShareBar({ up, down, className }: { up: number; down: number; className?: string }) {
  const rated = up + down
  return (
    <span className={cn("flex h-1 overflow-hidden rounded-full", rated ? "gap-px" : "bg-muted", className)} aria-hidden data-testid="share-bar">
      {rated > 0 && (
        <>
          {up > 0 && <span className="rounded-full bg-success" style={{ width: `${(up / rated) * 100}%` }} />}
          {down > 0 && <span className="flex-1 rounded-full bg-destructive" />}
        </>
      )}
    </span>
  )
}

export function downShare(p: { up: number; down: number }, locale: string) {
  const rated = p.up + p.down
  return rated ? new Intl.NumberFormat(locale, { style: "percent" }).format(p.down / rated) : "–"
}

function Count({ n, icon: Icon, label, className }: { n: number; icon: React.ComponentType<{ className?: string }>; label: string; className?: string }) {
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1 tabular-nums", n === 0 && "opacity-45", className)} title={label}>
      <Icon className="size-3.5" />
      <span className="sr-only">{label}</span>
      <span aria-hidden>{n}</span>
    </span>
  )
}

function Row({ p, current, cursor, inboxColor }: { p: DocsPage; current: boolean; cursor: boolean; inboxColor?: string }) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const share = downShare(p, i18n.locale)
  const [up, down, feedback, questions, answers] = [p.up, p.down, p.open_feedback, p.open_questions, p.published_answers].map((n) => fmt.format(n))
  return (
    <li>
      <Link
        to={docsPageLink(p.inbox_id, p.page)}
        className={cn(
          "flex flex-col gap-1 rounded-xl px-2.5 py-2.5 outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
          current && "bg-muted",
          cursor && "ring-2 ring-ring",
        )}
        aria-current={current ? "page" : undefined}
        data-testid="docs-row"
      >
        <span className="flex items-center gap-2">
          {inboxColor && <Dot color={inboxColor} className="size-[7px] shrink-0 rounded-[2px]" />}
          <span className="min-w-0 flex-1 truncate text-body font-medium">{p.title || pageLabel(p.page)}</span>
          <span className="flex shrink-0 items-center gap-2.5 text-small">
            <Count n={p.up} icon={ThumbsUpIcon} label={t`${up} helpful`} className="text-success" />
            <Count n={p.down} icon={ThumbsDownIcon} label={t`${down} not helpful`} className="text-destructive" />
            <span className="w-10 text-end text-muted-foreground tabular-nums" title={t`Share of not helpful`}>
              {share}
            </span>
          </span>
        </span>
        <span className="flex items-center gap-2 text-caption text-faint">
          <span className="min-w-0 flex-1 truncate" dir="ltr">
            {pageLabel(p.page)}
          </span>
          <span className="flex shrink-0 items-center gap-2.5">
            <Count n={p.open_feedback} icon={MessageSquareTextIcon} label={t`${feedback} open feedback`} className={p.open_feedback ? "text-brand" : undefined} />
            <Count n={p.open_questions} icon={MessageCircleQuestionMarkIcon} label={t`${questions} open questions`} className={p.open_questions ? "text-brand" : undefined} />
            <Count n={p.published_answers} icon={BadgeCheckIcon} label={t`${answers} published answers`} />
          </span>
        </span>
        <ShareBar up={p.up} down={p.down} className="mt-0.5" />
      </Link>
    </li>
  )
}

export function DocsWindow({ className }: { className?: string }) {
  const { t } = useLingui()
  const [days, setDays] = useDocsDays()
  return (
    <Segmented
      value={String(days)}
      onChange={(v) => setDays(Number(v) as (typeof WINDOWS)[number])}
      label={t`Ratings over`}
      className={className}
      itemClassName="flex-1"
      items={WINDOWS.map((d) => ({ value: String(d), label: <Plural value={d} one="# day" other="# days" />, testId: `docs-days-${d}` }))}
    />
  )
}

export function DocsSort({ className }: { className?: string }) {
  const { t } = useLingui()
  const [sort, setSort] = useDocsSort()
  const labels: Record<DocsPageSort, string> = { down: t`Most disliked`, up: t`Most liked`, helpful: t`Most helpful`, activity: t`Most active` }
  const titles: Record<DocsPageSort, string> = {
    down: t`Most “not helpful” ratings first`,
    up: t`Most “helpful” ratings first`,
    helpful: t`Highest share of “helpful” first, among pages with at least 5 ratings`,
    activity: t`Most ratings, open feedback and questions first`,
  }
  return (
    <div className={cn("flex flex-wrap gap-1.5", className)} role="toolbar" aria-label={t`Sort`}>
      {SORTS.map((k) => (
        <button
          key={k}
          type="button"
          aria-pressed={sort === k}
          title={titles[k]}
          onClick={() => setSort(k)}
          className={cn(
            "h-7 shrink-0 rounded-full px-3 text-small font-medium transition-colors",
            sort === k ? "bg-brand-wash text-brand" : "bg-muted text-muted-foreground hover:text-foreground",
          )}
          data-testid={`docs-sort-${k}`}
        >
          {labels[k]}
        </button>
      ))}
    </div>
  )
}

export function DocsColumn() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [params] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const [sort] = useDocsSort()
  const phone = useIsPhone()
  const inboxes = useInboxes().data ?? []
  const colors = new Map(inboxes.map((i) => [i.id, i.branding.color]))
  const pages = useDocsPages(inboxId, days, sort)
  const items = pages.data?.pages.flatMap((p) => p.items) ?? []
  const currentKey = pathname === "/docs/page" ? `${params.get("inbox")} ${params.get("url")}` : ""
  const [cursor, setCursor] = useState(-1)
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => setCursor(-1), [inboxId, days, sort])

  useHotkeys({
    j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
    Enter: () => cursor >= 0 && items[cursor] && navigate(docsPageLink(items[cursor].inbox_id, items[cursor].page)),
  })
  useEffect(() => {
    if (cursor >= 0) list.current?.querySelectorAll("[data-testid=docs-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  return (
    <Column title={<Trans>Docs</Trans>} testId="docs-panel" actions={<InboxPicker className="max-w-40" />} bodyClassName="px-2">
      <nav aria-label={t`Documentation pages`} className="flex flex-col">
        <DocsWindow className="mx-1 mb-2.5 flex" />
        <DocsSort className="mx-1 mb-2" />
        <ColumnLink to={phone ? "/docs/overview" : "/docs"} end testId="docs-overview-link">
          <span className="grid size-[34px] shrink-0 place-items-center rounded-[10px] border bg-surface text-muted-foreground">
            <BarChart3Icon className="size-4" />
          </span>
          <span className="text-body">
            <Trans>Overview</Trans>
          </span>
        </ColumnLink>
        {pages.isPending ? (
          <div className="flex flex-col gap-2 p-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : pages.error ? (
          <ErrorLine error={pages.error} className="p-3" />
        ) : items.length === 0 ? (
          <div className="px-4 py-10 text-center text-body text-faint" data-testid="docs-empty">
            <p className="mb-2 font-medium text-foreground">
              <Trans>No pages yet</Trans>
            </p>
            <DocsGuide />
          </div>
        ) : (
          <ul ref={list} className="flex flex-col">
            {items.map((p, i) => (
              <Row
                key={`${p.inbox_id} ${p.page}`}
                p={p}
                current={`${p.inbox_id} ${p.page}` === currentKey}
                cursor={i === cursor}
                inboxColor={inboxes.length > 1 && !inboxId ? colors.get(p.inbox_id) : undefined}
              />
            ))}
          </ul>
        )}
        {pages.hasNextPage && (
          <div className="mt-2 flex justify-center">
            <Button variant="ghost" size="sm" onClick={() => void pages.fetchNextPage()} disabled={pages.isFetchingNextPage}>
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
      </nav>
    </Column>
  )
}

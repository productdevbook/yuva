import { Trans, useLingui } from "@lingui/react/macro"
import { FileTextIcon, LoaderIcon, PlusIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  articleLink,
  centerLink,
  useCreateArticle,
  useHelpArticles,
  useHelpCollections,
  useHelpReport,
} from "@/features/help/queries"
import { ArticleBulkBar } from "@/features/help/Bulk"
import { articleTitle, collectionName, ErrorLine, useLocaleName, useStateText } from "@/features/help/text"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import { formatDateTime, formatShort } from "@/components/common/text"
import { look } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
import { useInboxes } from "@/lib/workspace"
import type {
  HelpArticleSummary,
  HelpCenter,
  HelpCollection,
  HelpReport,
  HelpReportArticle,
  HelpTranslationState,
} from "@/lib/api"
import type { HelpArticleFilters } from "@/lib/keys"
import { cn } from "@/lib/utils"

export function NewArticleDialog({
  center,
  collections,
  collectionId,
  initialTitle = "",
  onClose,
}: {
  center: HelpCenter
  collections: HelpCollection[]
  collectionId: string
  initialTitle?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const create = useCreateArticle(center.id)
  const [title, setTitle] = useState(initialTitle)
  const [locale, setLocale] = useState(center.default_locale)
  const [collection, setCollection] = useState(collectionId)
  const ok = title.trim().length >= 3
  const prefer = [center.default_locale]
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-article-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!ok) return
            create.mutate(
              { locale, title: title.trim(), collection_id: collection || undefined },
              { onSuccess: (a) => navigate(articleLink(center.id, a.id, locale)) }
            )
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>New article</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>Start with a title. You write the text next, and it stays a draft until it is published.</Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Title</Trans>} htmlFor="new-article-title" hint={<Trans>At least 3 characters.</Trans>}>
            <Input
              id="new-article-title"
              value={title}
              maxLength={200}
              autoFocus
              required
              placeholder={t`How do I reset my password?`}
              onChange={(e) => setTitle(e.target.value)}
              data-testid="new-article-title"
            />
          </Field>
          <div className="grid gap-5 sm:grid-cols-2">
            <Field label={<Trans>Language</Trans>} htmlFor="new-article-locale">
              <ChoiceSelect
                id="new-article-locale"
                value={locale}
                onChange={setLocale}
                options={center.locales.map((l) => [l, localeName(l)] as const)}
                label={t`Language`}
              />
            </Field>
            <Field label={<Trans>Collection</Trans>} htmlFor="new-article-collection">
              <ChoiceSelect
                id="new-article-collection"
                value={collection}
                onChange={setCollection}
                options={[
                  ["", t`No collection`] as const,
                  ...collections.map((c) => [c.id, collectionName(c, ...prefer)] as const),
                ]}
                label={t`Collection`}
              />
            </Field>
          </div>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || create.isPending} data-testid="create-article">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create article</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Stat({
  label,
  value,
  detail,
  tone,
  testId,
}: {
  label: React.ReactNode
  value: string
  detail?: React.ReactNode
  tone?: string
  testId: string
}) {
  return (
    <div className="flex min-w-0 flex-col rounded-xl border bg-background px-4 pt-3.5 pb-3" data-testid={testId}>
      <span className="truncate text-muted-foreground">{label}</span>
      <span className={cn("mt-1.5 text-[26px] leading-8 font-semibold tracking-[-0.02em] tabular-nums", tone)}>
        {value}
      </span>
      {detail && <span className="mt-1.5 truncate text-caption text-faint">{detail}</span>}
    </div>
  )
}

function rated(report?: HelpReport) {
  const seen = new Map<string, HelpReportArticle>()
  for (const r of [...(report?.most_down ?? []), ...(report?.top_articles ?? [])]) seen.set(r.article_id, r)
  let up = 0
  let all = 0
  for (const r of seen.values()) {
    up += r.helpful_up
    all += r.helpful_up + r.helpful_down
  }
  return { up, all }
}

function Stats({ center, report }: { center: HelpCenter; report?: HelpReport }) {
  const { i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const percent = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const drafts = n.format(Math.max(0, center.counts.articles - center.counts.published_articles))
  const missed = report?.no_result_searches ?? 0
  const searches = report ? n.format(report.searches) : ""
  const { up, all } = rated(report)
  const answers = n.format(all)
  return (
    <div className="grid grid-cols-4 gap-4 max-[900px]:grid-cols-2" data-testid="help-stats">
      <Stat
        label={<Trans>Articles</Trans>}
        value={n.format(center.counts.articles)}
        detail={<Trans>{drafts} drafts</Trans>}
        testId="help-stat-articles"
      />
      <Stat
        label={<Trans>Views · 7 days</Trans>}
        value={report ? n.format(report.views) : "–"}
        detail={report && <Trans>{searches} searches</Trans>}
        testId="help-stat-views"
      />
      <Stat
        label={<Trans>Helpful</Trans>}
        value={all ? percent.format(up / all) : "–"}
        detail={all > 0 && <Trans>of {answers} answers</Trans>}
        tone={all ? (up / all < 0.7 ? "text-warning" : "text-success") : undefined}
        testId="help-stat-helpful"
      />
      <Stat
        label={<Trans>Found nothing</Trans>}
        value={report ? n.format(missed) : "–"}
        detail={<Trans>searches this week</Trans>}
        tone={missed > 0 ? "text-warning" : undefined}
        testId="help-stat-missed"
      />
    </div>
  )
}

function Misses({ report, onWrite }: { report?: HelpReport; onWrite: (query: string) => void }) {
  const { i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const items = (report?.no_result_queries ?? []).filter((q) => q.query !== "(scrubbed)").slice(0, 8)
  if (items.length === 0) return null
  return (
    <section className={look.card} data-testid="help-misses">
      <h2 className="flex items-baseline justify-between gap-2 border-b px-4 py-3 font-semibold">
        <Trans>Searched for, found nothing</Trans>
        <span className="text-caption font-normal text-faint">
          <Trans>last 7 days</Trans>
        </span>
      </h2>
      <ul className="flex flex-wrap gap-2 p-4">
        {items.map((q) => (
          <li
            key={`${q.locale}:${q.query}`}
            className="inline-flex h-[34px] items-center gap-2 rounded-full border border-input ps-3 pe-1"
          >
            <span dir="auto" className="max-w-48 truncate">
              “{q.query}”
            </span>
            <span className="text-caption text-faint tabular-nums">{n.format(q.searches)}×</span>
            <Button
              size="sm"
              className="h-[26px] rounded-full px-2.5 text-caption"
              onClick={() => onWrite(q.query)}
              data-testid="help-miss-write"
            >
              <Trans>Write</Trans>
            </Button>
          </li>
        ))}
      </ul>
    </section>
  )
}

const STATE_TONE: Record<HelpTranslationState | "missing", string> = {
  published: "border-success/40 text-success",
  changed: "border-warning/50 text-warning",
  draft: "border-input text-muted-foreground",
  missing: "border-dashed border-input text-faint",
}

function StatePill({ state }: { state?: HelpTranslationState }) {
  const { t } = useLingui()
  const text = useStateText()
  return (
    <span className={cn(look.pill, STATE_TONE[state ?? "missing"])} data-testid="help-article-state">
      {state === "changed" ? t`Changed` : state ? text[state] : t`Not written`}
    </span>
  )
}

function Locales({ a, center }: { a: HelpArticleSummary; center: HelpCenter }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const stateText = useStateText()
  const written = center.locales.filter((l) => a.translations.some((x) => x.locale === l))
  return (
    <span className="flex min-w-0 items-center text-caption text-faint" data-testid="help-article-locales">
      {written.map((l, i) => {
        const tr = a.translations.find((x) => x.locale === l)!
        const label = `${name(l)}: ${stateText[tr.state]}${tr.outdated ? `, ${t`out of date`}` : ""}`
        return (
          <span key={l} className="flex items-center">
            {i > 0 && (
              <span className="px-1" aria-hidden>
                ·
              </span>
            )}
            <Link
              to={articleLink(center.id, a.id, l)}
              title={label}
              aria-label={label}
              className={cn(
                "uppercase outline-none hover:text-foreground focus-visible:underline",
                tr.outdated && "text-warning"
              )}
              data-testid={`help-locale-${l}`}
            >
              {l}
            </Link>
          </span>
        )
      })}
    </span>
  )
}

function CollectionsCard({
  center,
  cols,
  value,
  onChange,
}: {
  center: HelpCenter
  cols: HelpCollection[]
  value: string
  onChange: (id: string) => void
}) {
  const { t, i18n } = useLingui()
  const color = useInboxes().data?.find((i) => i.id === center.inbox_id)?.branding.color
  const fmt = new Intl.NumberFormat(i18n.locale)
  const prefer = [i18n.locale, center.default_locale]
  return (
    <section className={look.card} aria-label={t`Collections`} data-testid="help-collections">
      <h2 className="border-b px-4 py-3 font-semibold">
        <Trans>Collections</Trans>
      </h2>
      <ul className="flex flex-col">
        {cols.map((c) => {
          const on = value === c.id
          return (
            <li key={c.id} className="border-b">
              <button
                type="button"
                aria-pressed={on}
                onClick={() => onChange(on ? "" : c.id)}
                className={cn(
                  "flex w-full items-center gap-2.5 px-4 py-2.5 text-start outline-none transition-colors hover:bg-surface focus-visible:bg-surface",
                  on && "bg-surface font-medium shadow-[inset_2px_0_0_var(--brand)]"
                )}
                data-testid="help-collection"
              >
                <span
                  className="size-2 shrink-0 rounded-[2px] bg-brand"
                  style={color ? { backgroundColor: color } : undefined}
                  aria-hidden
                />
                <span className="min-w-0 flex-1 truncate">{collectionName(c, ...prefer)}</span>
                <span className="text-caption text-faint tabular-nums">{fmt.format(c.article_count)}</span>
              </button>
            </li>
          )
        })}
      </ul>
      <Link
        to={`${centerLink(center.id, "collections")}&new=1`}
        className="flex items-center gap-1 px-4 py-3 text-muted-foreground outline-none hover:text-foreground focus-visible:underline"
        data-testid="help-new-collection"
      >
        <PlusIcon className="size-3.5" />
        <Trans>New collection</Trans>
      </Link>
    </section>
  )
}

const grid =
  "grid grid-cols-[minmax(0,1fr)_120px_100px_130px_70px_70px] items-center gap-3 px-4 phone:grid-cols-[minmax(0,1fr)_auto]"

export function ArticlesTab({
  center,
  drafts,
  q,
  onWrite,
}: {
  center: HelpCenter
  drafts?: boolean
  q: string
  onWrite: (title: string, collectionId: string) => void
}) {
  const { i18n } = useLingui()
  const navigate = useNavigate()
  const [collection, setCollection] = useState("")
  const [cursor, setCursor] = useState(-1)
  const report = useHelpReport(center.id, 7).data
  const list = useRef<HTMLUListElement>(null)
  const debounced = useDebounced(q.trim(), 300)
  const collections = useHelpCollections(center.id)
  const cols = collections.data ?? []
  const color = useInboxes().data?.find((i) => i.id === center.inbox_id)?.branding.color
  const filters: HelpArticleFilters = {
    q: debounced || undefined,
    collection_id: collection || undefined,
    locale: drafts ? center.default_locale : undefined,
    state: drafts ? "draft" : undefined,
  }
  const articles = useHelpArticles(center.id, filters)
  const items = articles.data?.pages.flatMap((p) => p.items) ?? []
  const prefer = [i18n.locale, center.default_locale]
  const selection = useSelection(
    items.map((a) => a.id),
    JSON.stringify(filters)
  )

  useHotkeys({
    j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
    ...(cursor >= 0 && items[cursor]
      ? { Enter: () => navigate(articleLink(center.id, items[cursor].id, center.default_locale)) }
      : {}),
  })
  useEffect(() => {
    if (cursor >= 0) list.current?.children[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  const fmt = new Intl.NumberFormat(i18n.locale)
  const percent = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const stats = new Map<string, HelpReportArticle>()
  for (const r of [...(report?.most_down ?? []), ...(report?.top_articles ?? [])]) stats.set(r.article_id, r)
  const complete = !!report && report.top_articles.length < 20
  const maxViews = Math.max(0, ...(report?.top_articles ?? []).map((r) => r.views))
  const collectionOf = (a: HelpArticleSummary) => {
    const c = cols.find((x) => x.id === a.collection_id)
    return c ? collectionName(c, ...prefer) : undefined
  }

  return (
    <div className="flex flex-col gap-4" data-testid="help-articles">
      <Stats center={center} report={report} />
      <div className="grid grid-cols-[240px_minmax(0,1fr)] items-start gap-4 max-[1100px]:grid-cols-1">
        <CollectionsCard
          center={center}
          cols={cols}
          value={collection}
          onChange={(id) => {
            setCollection(id)
            setCursor(-1)
          }}
        />
        <div className="flex min-w-0 flex-col gap-4">
          <ArticleBulkBar center={center} items={items} cols={cols} selection={selection} />
          {!selection.selecting && items.length > 0 && (
            <div className="flex justify-end">
              <SelectToggle selection={selection} />
            </div>
          )}
          <div className={look.card}>
            <div className={cn(grid, "h-[38px]", look.th)} aria-hidden>
              <span>
                <Trans>Article</Trans>
              </span>
              <span className="phone:hidden">
                <Trans>Collection</Trans>
              </span>
              <span>
                <Trans>Status</Trans>
              </span>
              <span className="phone:hidden">
                <Trans>Views · 7 days</Trans>
              </span>
              <span className="phone:hidden">
                <Trans>Helpful</Trans>
              </span>
              <span className="phone:hidden">
                <Trans>Updated</Trans>
              </span>
            </div>
            {articles.isPending ? (
              <div className="flex flex-col gap-2 border-t p-3">
                <Skeleton className="h-10 w-full" />
                <Skeleton className="h-10 w-full" />
              </div>
            ) : articles.error ? (
              <ErrorLine error={articles.error} className="border-t p-4" />
            ) : items.length === 0 ? (
              <div
                className="flex flex-col items-center gap-2 border-t px-6 py-10 text-center"
                data-testid="help-articles-empty"
              >
                <FileTextIcon className="size-6 text-faint" />
                <p className="text-muted-foreground">
                  {debounced || collection ? (
                    <Trans>No article matches.</Trans>
                  ) : drafts ? (
                    <Trans>No drafts. Everything written is published.</Trans>
                  ) : (
                    <Trans>No articles yet. Write the first answer your customers keep asking for.</Trans>
                  )}
                </p>
              </div>
            ) : (
              <ul ref={list} className="flex flex-col">
                {items.map((a, i) => {
                  const r = stats.get(a.id)
                  const votes = r ? r.helpful_up + r.helpful_down : 0
                  const views = r ? r.views : complete ? 0 : undefined
                  const state = a.translations.find((x) => x.locale === center.default_locale)?.state
                  return (
                    <li
                      key={a.id}
                      className={cn(grid, "border-t py-2.5", i === cursor && "bg-surface")}
                      data-testid="help-article-row"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        {selection.selecting && (
                          <RowCheck
                            checked={selection.has(a.id)}
                            name={articleTitle(a, ...prefer)}
                            onToggle={(range) => selection.toggle(a.id, range)}
                          />
                        )}
                        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                          <Link
                            to={articleLink(center.id, a.id, center.default_locale)}
                            className="block truncate font-medium outline-none hover:underline focus-visible:underline"
                          >
                            {articleTitle(a, ...prefer)}
                          </Link>
                          <span className="flex min-w-0 items-center gap-1.5">
                            <ProductTag id={center.inbox_id} always />
                            <Locales a={a} center={center} />
                          </span>
                        </div>
                      </div>
                      <span className="truncate phone:hidden">
                        {collectionOf(a) ?? <span className="text-faint">–</span>}
                      </span>
                      <span>
                        <StatePill state={state} />
                      </span>
                      <span className="flex items-center gap-2 tabular-nums phone:hidden">
                        <span className="h-1.5 w-[70px] shrink-0 overflow-hidden rounded-full bg-muted" aria-hidden>
                          {views !== undefined && maxViews > 0 && (
                            <span
                              className="block h-full rounded-full bg-brand"
                              style={{
                                width: `${(views / maxViews) * 100}%`,
                                ...(color ? { backgroundColor: color } : {}),
                              }}
                            />
                          )}
                        </span>
                        <span className="min-w-0 flex-1 truncate text-end text-caption text-muted-foreground">
                          {views !== undefined && views > 0 ? fmt.format(views) : "–"}
                        </span>
                      </span>
                      <span className="tabular-nums phone:hidden">
                        {votes > 0 && r ? (
                          <span
                            className={cn(r.helpful_up / votes < 0.7 && "font-medium text-warning")}
                            title={`${fmt.format(r.helpful_up)} / ${fmt.format(votes)}`}
                          >
                            {percent.format(r.helpful_up / votes)}
                          </span>
                        ) : (
                          <span className="text-faint">–</span>
                        )}
                      </span>
                      <time
                        className="text-caption text-muted-foreground phone:hidden"
                        dateTime={a.updated_at}
                        title={formatDateTime(a.updated_at, i18n.locale)}
                      >
                        {formatShort(a.updated_at, i18n.locale)}
                      </time>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
          {articles.hasNextPage && (
            <Button
              variant="outline"
              size="sm"
              className="self-center"
              disabled={articles.isFetchingNextPage}
              onClick={() => articles.fetchNextPage()}
            >
              <Trans>Load more</Trans>
            </Button>
          )}
          <Misses report={report} onWrite={(query) => onWrite(query, collection)} />
        </div>
      </div>
    </div>
  )
}

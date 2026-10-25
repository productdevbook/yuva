import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { FileTextIcon, FoldersIcon, LoaderIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
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
import { ICON_COMPONENTS } from "@/features/help/icons"
import { articleTitle, collectionName, ErrorLine, useLocaleName, useStateText } from "@/features/help/text"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import { formatDateTime, formatShort } from "@/components/common/text"
import { look } from "@/features/inbox/PageFrame"
import { ProductTag } from "@/features/inbox/ProductTag"
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
    <div
      className="flex min-w-0 flex-col gap-0.5 border-e px-4 py-3 last:border-e-0 @max-3xl:border-b @max-3xl:border-e-0 @max-3xl:odd:border-e @max-3xl:last:border-b-0 @max-3xl:nth-last-2:border-b-0 @min-[1241px]:border-b @min-[1241px]:border-e-0 @min-[1241px]:odd:border-e @min-[1241px]:last:border-b-0 @min-[1241px]:nth-last-2:border-b-0"
      data-testid={testId}
    >
      <span className="truncate text-caption text-faint">{label}</span>
      <span className={cn("text-[18px] leading-6 font-semibold tabular-nums", tone)}>{value}</span>
      {detail && <span className="truncate text-caption text-faint">{detail}</span>}
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

function Stats({ center, report, className }: { center: HelpCenter; report?: HelpReport; className?: string }) {
  const { i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const percent = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const drafts = Math.max(0, center.counts.articles - center.counts.published_articles)
  const missed = report?.no_result_searches ?? 0
  const searches = report ? n.format(report.searches) : ""
  const { up, all } = rated(report)
  const answers = n.format(all)
  return (
    <div
      className={cn(
        look.card,
        "grid grid-cols-4 rounded-[14px] @max-3xl:grid-cols-2 @min-[1241px]:grid-cols-2",
        className
      )}
      data-testid="help-stats"
    >
      <Stat
        label={<Trans>Articles</Trans>}
        value={n.format(center.counts.articles)}
        detail={<Plural value={drafts} one="# draft" other="# drafts" />}
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

function Misses({
  report,
  onWrite,
  className,
}: {
  report?: HelpReport
  onWrite: (query: string) => void
  className?: string
}) {
  const { i18n } = useLingui()
  const n = new Intl.NumberFormat(i18n.locale)
  const items = (report?.no_result_queries ?? []).filter((q) => q.query !== "(scrubbed)").slice(0, 8)
  if (items.length === 0) return null
  return (
    <section className={cn(look.card, "rounded-[14px]", className)} data-testid="help-misses">
      <h2 className="flex items-baseline justify-between gap-2 px-4 pt-3.5 pb-2.5 text-body font-semibold">
        <Trans>Searched for, found nothing</Trans>
        <span className="text-caption font-normal text-faint">
          <Trans>last 7 days</Trans>
        </span>
      </h2>
      <ul>
        {items.map((q) => (
          <li
            key={`${q.locale}:${q.query}`}
            className="flex items-center gap-2.5 border-t px-4 py-2.5"
            data-testid="help-miss"
          >
            <span dir="auto" className="min-w-0 flex-1 truncate text-body">
              “{q.query}”
            </span>
            <span className="text-caption text-faint tabular-nums">{n.format(q.searches)}×</span>
            <Button
              variant="ghost"
              className="h-7 rounded-[7px] px-2 text-small font-medium"
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
  published: "text-success before:bg-current",
  changed: "text-warning before:bg-[linear-gradient(90deg,currentColor_50%,transparent_50%)]",
  draft: "text-faint",
  missing: "text-faint",
}

function StatePill({ state }: { state?: HelpTranslationState }) {
  const { t } = useLingui()
  const text = useStateText()
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-caption font-medium whitespace-nowrap before:size-[7px] before:shrink-0 before:rounded-full before:border-[1.5px] before:border-current before:content-['']",
        STATE_TONE[state ?? "missing"]
      )}
      data-testid="help-article-state"
    >
      {state === "changed" ? t`Changed` : state ? text[state] : t`Not written`}
    </span>
  )
}

const LANG_CHIP =
  "inline-flex h-5 items-center rounded-md px-1.5 text-[11px] font-semibold tracking-[0.02em] uppercase outline-none focus-visible:ring-2 focus-visible:ring-ring"

function Locales({ a, center }: { a: HelpArticleSummary; center: HelpCenter }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const stateText = useStateText()
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-1" data-testid="help-article-locales">
      {center.locales.map((l) => {
        const tr = a.translations.find((x) => x.locale === l)
        if (!tr) {
          return (
            <span
              key={l}
              className={cn(LANG_CHIP, "border border-dashed border-input text-faint")}
              title={`${name(l)}: ${t`Not written`}`}
            >
              {l}
            </span>
          )
        }
        const label = `${name(l)}: ${stateText[tr.state]}${tr.outdated ? `, ${t`out of date`}` : ""}`
        return (
          <Link
            key={l}
            to={articleLink(center.id, a.id, l)}
            title={label}
            aria-label={label}
            className={cn(
              LANG_CHIP,
              tr.outdated ? "bg-warning/15 text-warning" : "bg-surface text-muted-foreground hover:text-foreground"
            )}
            data-testid={`help-locale-${l}`}
          >
            {l}
          </Link>
        )
      })}
    </span>
  )
}

const NODE =
  "flex h-8 w-full items-center gap-2 rounded-lg px-2.5 text-start text-small text-muted-foreground outline-none transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring aria-pressed:bg-card aria-pressed:font-semibold aria-pressed:text-foreground aria-pressed:shadow-[0_0_0_1px_var(--border)]"

function CollectionsCard({
  className,
  center,
  cols,
  value,
  onChange,
}: {
  className?: string
  center: HelpCenter
  cols: HelpCollection[]
  value: string
  onChange: (id: string) => void
}) {
  const { t, i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  const prefer = [i18n.locale, center.default_locale]
  const ordered = cols
    .filter((c) => !c.parent_id)
    .sort((a, b) => a.position - b.position || a.number - b.number)
    .flatMap((top) => [
      top,
      ...cols.filter((c) => c.parent_id === top.id).sort((a, b) => a.position - b.position || a.number - b.number),
    ])
  return (
    <nav
      className={cn("flex flex-col gap-px @3xl:sticky @3xl:top-4", className)}
      aria-label={t`Collections`}
      data-testid="help-collections"
    >
      <h2 className="px-2.5 pt-1 pb-1.5 text-[11px] leading-4 font-semibold tracking-[0.05em] text-faint uppercase @max-3xl:hidden">
        <Trans>Collections</Trans>
      </h2>
      <div className="flex flex-col gap-px @max-3xl:flex-row @max-3xl:gap-1 @max-3xl:overflow-x-auto @max-3xl:[scrollbar-width:none]">
        <button
          type="button"
          aria-pressed={value === ""}
          onClick={() => onChange("")}
          className={cn(NODE, "@max-3xl:w-auto @max-3xl:shrink-0")}
        >
          <span className="min-w-0 flex-1 truncate">
            <Trans>All</Trans>
          </span>
          <span className="text-caption font-normal text-faint tabular-nums">{fmt.format(center.counts.articles)}</span>
        </button>
        {ordered.map((c) => {
          const on = value === c.id
          const Icon = c.icon ? ICON_COMPONENTS[c.icon] : FoldersIcon
          return (
            <button
              key={c.id}
              type="button"
              aria-pressed={on}
              onClick={() => onChange(on ? "" : c.id)}
              className={cn(NODE, c.parent_id && "ps-7 @max-3xl:hidden", "@max-3xl:w-auto @max-3xl:shrink-0")}
              data-testid="help-collection"
            >
              <Icon className="size-3.5 shrink-0 text-faint" aria-hidden />
              <span className="min-w-0 flex-1 truncate">{collectionName(c, ...prefer)}</span>
              <span className="text-caption font-normal text-faint tabular-nums">{fmt.format(c.article_count)}</span>
            </button>
          )
        })}
        <Link
          to={`${centerLink(center.id, "collections")}&new=1`}
          className={cn(NODE, "text-faint hover:text-foreground @max-3xl:w-auto @max-3xl:shrink-0")}
          data-testid="help-new-collection"
        >
          <PlusIcon className="size-3.5" aria-hidden />
          <Trans>New collection</Trans>
        </Link>
      </div>
    </nav>
  )
}

const grid =
  "grid grid-cols-[var(--cb,0px)_minmax(0,1fr)_96px_80px_52px_80px_68px] items-center gap-3 px-4 @max-3xl:grid-cols-[minmax(0,1fr)_auto]"

export function ArticlesTab({
  center,
  drafts,
  q,
  onQ,
  search,
  onWrite,
}: {
  center: HelpCenter
  drafts?: boolean
  q: string
  onQ: (q: string) => void
  search: React.RefObject<HTMLInputElement | null>
  onWrite: (title: string, collectionId: string) => void
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const [collection, setCollection] = useState("")
  const [cursor, setCursor] = useState(-1)
  const report = useHelpReport(center.id, 7).data
  const list = useRef<HTMLUListElement>(null)
  const debounced = useDebounced(q.trim(), 300)
  const collections = useHelpCollections(center.id)
  const cols = collections.data ?? []
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
  const findKey = keyLabel(HELP_KEYS.search).join("")

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
  const collectionOf = (a: HelpArticleSummary) => {
    const c = cols.find((x) => x.id === a.collection_id)
    return c ? collectionName(c, ...prefer) : undefined
  }

  return (
    <div
      className="grid grid-cols-[200px_minmax(0,1fr)] items-start gap-4 @max-3xl:grid-cols-1 @min-[1241px]:grid-cols-[220px_minmax(0,1fr)_290px]"
      data-testid="help-articles"
    >
      <CollectionsCard
        className="row-span-3 @max-3xl:row-span-1 @min-[1241px]:row-span-2 @min-[1241px]:row-start-1"
        center={center}
        cols={cols}
        value={collection}
        onChange={(id) => {
          setCollection(id)
          setCursor(-1)
        }}
      />
      <Stats
        center={center}
        report={report}
        className="col-start-2 @max-3xl:col-start-1 @min-[1241px]:col-start-3 @min-[1241px]:row-start-1"
      />
      <div className="flex min-w-0 flex-col gap-4 col-start-2 @max-3xl:col-start-1 @min-[1241px]:row-span-2 @min-[1241px]:row-start-1">
        <div
          className={cn(look.card, "rounded-[14px]")}
          style={selection.selecting ? { ["--cb" as string]: "20px" } : undefined}
        >
          <ArticleBulkBar center={center} items={items} cols={cols} selection={selection} />
          <div className="flex flex-wrap items-center gap-2 px-4 py-3">
            <label className="flex h-[34px] min-w-0 max-w-sm flex-1 basis-52 items-center gap-2 rounded-[9px] border border-input bg-card px-2.5 focus-within:border-brand focus-within:ring-[3px] focus-within:ring-brand/20">
              <SearchIcon className="size-3.5 shrink-0 text-faint" aria-hidden />
              <input
                ref={search}
                value={q}
                onChange={(e) => onQ(e.target.value)}
                onKeyDown={(e) => e.key === "Escape" && e.currentTarget.blur()}
                placeholder={t`Search titles (${findKey})`}
                aria-label={t`Search titles`}
                className="min-w-0 flex-1 bg-transparent text-small outline-none placeholder:text-faint"
                data-testid="help-article-search"
              />
            </label>
            <span className="flex-1" />
            {!selection.selecting && items.length > 0 && <SelectToggle selection={selection} />}
          </div>
          <div className={cn(grid, "h-9 border-y bg-surface/50", look.over, "@max-3xl:hidden")} aria-hidden>
            <span />
            <span>
              <Trans>Article</Trans>
            </span>
            <span>
              <Trans>Status</Trans>
            </span>
            <span>
              <Trans>Languages</Trans>
            </span>
            <span className="text-end">
              <Trans>Views</Trans>
            </span>
            <span>
              <Trans>Helpful</Trans>
            </span>
            <span>
              <Trans>Updated</Trans>
            </span>
          </div>
          {articles.isPending ? (
            <div className="flex flex-col gap-3 p-4" aria-busy="true">
              {[0, 1, 2, 3, 4].map((i) => (
                <Skeleton key={i} className="h-10 w-full rounded-lg" />
              ))}
            </div>
          ) : articles.error ? (
            <ErrorLine error={articles.error} className="p-4" />
          ) : items.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-6 py-12 text-center" data-testid="help-articles-empty">
              <span className="mb-1 grid size-12 place-items-center rounded-2xl bg-muted text-faint">
                <FileTextIcon className="size-5" />
              </span>
              <p className="max-w-sm text-body text-muted-foreground">
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
                const col = collectionOf(a)
                return (
                  <li
                    key={a.id}
                    className={cn(
                      grid,
                      "min-h-[58px] border-b py-2 last:border-b-0 hover:bg-surface/60",
                      i === cursor && "bg-surface",
                      selection.has(a.id) && "bg-brand/5"
                    )}
                    data-testid="help-article-row"
                  >
                    <span className="@max-3xl:hidden">
                      {selection.selecting && (
                        <RowCheck
                          checked={selection.has(a.id)}
                          name={articleTitle(a, ...prefer)}
                          onToggle={(range) => selection.toggle(a.id, range)}
                        />
                      )}
                    </span>
                    <div className="flex min-w-0 items-center gap-3">
                      {selection.selecting && (
                        <span className="hidden @max-3xl:block">
                          <RowCheck
                            checked={selection.has(a.id)}
                            name={articleTitle(a, ...prefer)}
                            onToggle={(range) => selection.toggle(a.id, range)}
                          />
                        </span>
                      )}
                      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <Link
                          to={articleLink(center.id, a.id, center.default_locale)}
                          className="block truncate font-semibold outline-none hover:underline focus-visible:underline"
                        >
                          {articleTitle(a, ...prefer)}
                        </Link>
                        <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
                          <ProductTag id={center.inbox_id} always />
                          <span className="truncate">{col ?? "–"}</span>
                        </span>
                      </div>
                    </div>
                    <span>
                      <StatePill state={state} />
                    </span>
                    <span className="@max-3xl:hidden">
                      <Locales a={a} center={center} />
                    </span>
                    <span className="text-end text-small text-muted-foreground tabular-nums @max-3xl:hidden">
                      {views !== undefined && views > 0 ? fmt.format(views) : "–"}
                    </span>
                    <span className="flex items-center gap-1.5 text-small tabular-nums @max-3xl:hidden">
                      {votes > 0 && r ? (
                        <>
                          <span className="h-1.5 w-[30px] shrink-0 overflow-hidden rounded-full bg-muted" aria-hidden>
                            <span
                              className={cn(
                                "block h-full rounded-full",
                                r.helpful_up / votes < 0.7 ? "bg-warning" : "bg-success"
                              )}
                              style={{ width: `${(r.helpful_up / votes) * 100}%` }}
                            />
                          </span>
                          <span
                            className={cn(r.helpful_up / votes < 0.7 && "font-medium text-warning")}
                            title={`${fmt.format(r.helpful_up)} / ${fmt.format(votes)}`}
                          >
                            {percent.format(r.helpful_up / votes)}
                          </span>
                        </>
                      ) : (
                        <span className="text-faint">–</span>
                      )}
                    </span>
                    <time
                      className="text-caption text-muted-foreground @max-3xl:hidden"
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
          {articles.hasNextPage && (
            <div className="flex justify-center border-t p-3">
              <Button
                variant="outline"
                className={look.btnSm}
                disabled={articles.isFetchingNextPage}
                onClick={() => articles.fetchNextPage()}
              >
                <Trans>Load more</Trans>
              </Button>
            </div>
          )}
        </div>
      </div>
      <Misses
        report={report}
        onWrite={(query) => onWrite(query, collection)}
        className="col-start-2 @max-3xl:col-start-1 @min-[1241px]:col-start-3 @min-[1241px]:row-start-2"
      />
    </div>
  )
}

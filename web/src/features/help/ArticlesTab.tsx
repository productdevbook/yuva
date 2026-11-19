import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { InfoSection, KV } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import { boxRowClass, KeyHints, ListBox, ListBoxEmpty, ListBoxSkeleton, RowCheckbox } from "@/components/common/ListBox"
import { HELP_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDateTime, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ArticleBulkBar } from "@/features/help/Bulk"
import {
  articleLink,
  useCreateArticle,
  useHelpArticles,
  useHelpCollections,
  useHelpReport,
} from "@/features/help/queries"
import { articleTitle, collectionName, ErrorLine, useLocaleName, useStateText } from "@/features/help/text"
import { useListCursor } from "@/features/inbox/listCursor"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
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

export function NewArticleForm({
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
    <FormCard
      title={<Trans>New article</Trans>}
      description={
        <Trans>Start with a title. You write the text next, and it stays a draft until it is published.</Trans>
      }
      submit={<Trans>Create article</Trans>}
      ready={ok}
      busy={create.isPending}
      error={create.error}
      onCancel={onClose}
      onSubmit={() =>
        create.mutate(
          { locale, title: title.trim(), collection_id: collection || undefined },
          { onSuccess: (a) => navigate(articleLink(center.id, a.id, locale)) }
        )
      }
      testId="new-article-form"
    >
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
      <FormGrid>
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
      </FormGrid>
    </FormCard>
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

export type ArticleState = "" | "published" | "draft"

function ArticleRow({
  a,
  center,
  collection,
  stats,
  complete,
  cursor,
  checked,
  onToggle,
  onOpen,
}: {
  a: HelpArticleSummary
  center: HelpCenter
  collection?: string
  stats?: HelpReportArticle
  complete: boolean
  cursor: boolean
  checked: boolean
  onToggle: (range: boolean) => void
  onOpen: () => void
}) {
  const { i18n } = useLingui()
  const prefer = [i18n.locale, center.default_locale]
  const title = articleTitle(a, ...prefer)
  const views = stats ? stats.views : complete ? 0 : undefined
  const state = a.translations.find((x) => x.locale === center.default_locale)?.state
  return (
    <div
      data-row
      data-row-id={a.id}
      className={boxRowClass({ cursor, checked })}
      data-testid="help-article-row"
      aria-selected={checked || undefined}
    >
      <RowCheckbox checked={checked} name={title} onToggle={onToggle} />
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-4 py-2.5 pe-3.5 text-start outline-none focus-visible:underline"
      >
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="truncate text-body font-semibold">{title}</span>
          <span className="truncate text-small text-muted-foreground">
            {collection ?? "–"}
            <span className="@xl/list:hidden">
              {" · "}
              {formatShort(a.updated_at, i18n.locale)}
            </span>
          </span>
        </span>
        <span className="shrink-0">
          <StatePill state={state} />
        </span>
        <span className="hidden shrink-0 @3xl/list:block" onClick={(e) => e.stopPropagation()}>
          <Locales a={a} center={center} />
        </span>
        <span className="hidden w-24 shrink-0 text-end text-small text-muted-foreground tabular-nums @2xl/list:block">
          {views !== undefined && plural(views, { one: "# view", other: "# views" })}
        </span>
        <time
          className="hidden w-16 shrink-0 text-end text-small text-muted-foreground @xl/list:block"
          dateTime={a.updated_at}
          title={formatDateTime(a.updated_at, i18n.locale)}
        >
          {formatShort(a.updated_at, i18n.locale)}
        </time>
      </button>
    </div>
  )
}

export function ArticlesTab({
  center,
  state,
  collection,
  q,
  active,
  onClear,
}: {
  center: HelpCenter
  state: ArticleState
  collection: string
  q: string
  active: boolean
  onClear?: () => void
}) {
  const { i18n } = useLingui()
  const navigate = useNavigate()
  const report = useHelpReport(center.id, 7).data
  const debounced = useDebounced(q.trim(), 300)
  const cols = useHelpCollections(center.id).data ?? []
  const filters: HelpArticleFilters = {
    q: debounced || undefined,
    collection_id: collection || undefined,
    locale: state ? center.default_locale : undefined,
    state: state || undefined,
  }
  const articles = useHelpArticles(center.id, filters)
  const items = articles.data?.pages.flatMap((p) => p.items) ?? []
  const prefer = [i18n.locale, center.default_locale]
  const scope = JSON.stringify(filters)
  const selection = useSelection(
    items.map((a) => a.id),
    scope
  )
  const listRef = useRef<HTMLDivElement>(null)
  const open = (id: string) => navigate(articleLink(center.id, id, center.default_locale))
  const { cursorId } = useListCursor(items, null, open, active, listRef, true, true)
  useHotkeys(
    {
      [SHORTCUTS.select]: () => cursorId && selection.toggle(cursorId),
      ...(selection.selecting ? { Escape: selection.stop } : {}),
    },
    active
  )

  const stats = new Map<string, HelpReportArticle>()
  for (const r of [...(report?.most_down ?? []), ...(report?.top_articles ?? [])]) stats.set(r.article_id, r)
  const complete = !!report && report.top_articles.length < 20
  const collectionOf = (a: HelpArticleSummary) => {
    const c = cols.find((x) => x.id === a.collection_id)
    return c ? collectionName(c, ...prefer) : undefined
  }
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shown = `${fmt.format(items.length)}${articles.hasNextPage ? "+" : ""}`

  return (
    <div ref={listRef} className="@container/list min-w-0" data-testid="help-articles">
      <ListBox
        testId="list"
        bar={selection.selecting && <ArticleBulkBar center={center} items={items} cols={cols} selection={selection} />}
      >
        {articles.isPending ? (
          <ListBoxSkeleton />
        ) : articles.error && items.length === 0 ? (
          <ListBoxEmpty
            action={
              <Button variant="outline" size="sm" onClick={() => void articles.refetch()}>
                <Trans>Try again</Trans>
              </Button>
            }
          >
            <ErrorLine error={articles.error} />
          </ListBoxEmpty>
        ) : items.length === 0 ? (
          <ListBoxEmpty
            testId="help-articles-empty"
            action={
              onClear && (
                <Button variant="outline" size="sm" onClick={onClear}>
                  <Trans>Clear filters</Trans>
                </Button>
              )
            }
          >
            {debounced || collection ? (
              <Trans>No article matches.</Trans>
            ) : state === "draft" ? (
              <Trans>No drafts. Everything written is published.</Trans>
            ) : (
              <Trans>No articles yet. Write the first answer your customers keep asking for.</Trans>
            )}
          </ListBoxEmpty>
        ) : (
          items.map((a) => (
            <ArticleRow
              key={a.id}
              a={a}
              center={center}
              collection={collectionOf(a)}
              stats={stats.get(a.id)}
              complete={complete}
              cursor={a.id === cursorId}
              checked={selection.has(a.id)}
              onToggle={(range) => selection.toggle(a.id, range)}
              onOpen={() => (selection.selecting ? selection.toggle(a.id) : open(a.id))}
            />
          ))
        )}
      </ListBox>
      {articles.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          disabled={articles.isFetchingNextPage}
          onClick={() => void articles.fetchNextPage()}
        >
          <Trans>Load more</Trans>
        </Button>
      )}
      {items.length > 0 && (
        <p className="m-0 mt-3 text-small text-muted-foreground" data-testid="list-lede">
          {plural(items.length, { one: `${shown} article`, other: `${shown} articles` })}
        </p>
      )}
      <KeyHints
        items={[
          [[SHORTCUTS.next, SHORTCUTS.previous], <Trans key="move">move</Trans>],
          [
            ["↵"],
            <Trans key="open" context="keyboard hint">
              open
            </Trans>,
          ],
          [[SHORTCUTS.select], <Trans key="select">select</Trans>],
          [[HELP_KEYS.search], <Trans key="search">search</Trans>],
          [[HELP_KEYS.newArticle], <Trans key="new">new article</Trans>],
        ]}
      />
    </div>
  )
}

export function HelpAside({ center, onWrite }: { center: HelpCenter; onWrite: (query: string) => void }) {
  const { t, i18n } = useLingui()
  const report = useHelpReport(center.id, 7).data
  const n = new Intl.NumberFormat(i18n.locale)
  const percent = new Intl.NumberFormat(i18n.locale, { style: "percent", maximumFractionDigits: 0 })
  const drafts = Math.max(0, center.counts.articles - center.counts.published_articles)
  const missed = report?.no_result_searches ?? 0
  const { up, all } = rated(report)
  const misses = (report?.no_result_queries ?? []).filter((q) => q.query !== "(scrubbed)").slice(0, 8)
  return (
    <>
      <InfoSection title={<Trans>Site</Trans>}>
        <KV label={<Trans>Address</Trans>} value={<span dir="ltr">{center.url.replace(/^https?:\/\//, "")}</span>} />
        <KV
          label={<Trans>Visibility</Trans>}
          value={center.published ? t`Published` : t`Unpublished`}
          tone={center.published ? "ok" : undefined}
        />
        <KV label={<Trans>Articles</Trans>} value={n.format(center.counts.articles)} testId="help-stat-articles" />
        <KV label={<Trans>Drafts</Trans>} value={n.format(drafts)} tone={drafts ? "warn" : undefined} />
      </InfoSection>
      <InfoSection title={<Trans>Last 7 days</Trans>} testId="help-stats">
        <KV label={<Trans>Views</Trans>} value={report ? n.format(report.views) : "–"} testId="help-stat-views" />
        <KV label={<Trans>Searches</Trans>} value={report ? n.format(report.searches) : "–"} />
        <KV
          label={<Trans>Helpful</Trans>}
          value={all ? percent.format(up / all) : "–"}
          tone={all ? (up / all < 0.7 ? "warn" : "ok") : undefined}
          testId="help-stat-helpful"
        />
        <KV
          label={<Trans>Found nothing</Trans>}
          value={report ? n.format(missed) : "–"}
          tone={missed > 0 ? "warn" : undefined}
          testId="help-stat-missed"
        />
      </InfoSection>
      {misses.length > 0 && (
        <InfoSection title={<Trans>Searched for, found nothing</Trans>} testId="help-misses">
          {misses.map((q) => (
            <div
              key={`${q.locale}:${q.query}`}
              className="flex items-center gap-2 py-[3px] text-small"
              data-testid="help-miss"
            >
              <span dir="auto" className="min-w-0 flex-1 truncate text-muted-foreground">
                “{q.query}”
              </span>
              <span className="font-mono text-[11px] text-muted2 tabular-nums">{n.format(q.searches)}×</span>
              <button
                type="button"
                className="rounded-sm text-fg2 underline-offset-4 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                onClick={() => onWrite(q.query)}
                data-testid="help-miss-write"
              >
                <Trans>Write</Trans>
              </button>
            </div>
          ))}
        </InfoSection>
      )}
    </>
  )
}

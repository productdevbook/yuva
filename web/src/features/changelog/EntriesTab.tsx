import { plural } from "@lingui/core/macro"
import { Trans, useLingui } from "@lingui/react/macro"
import { TriangleAlertIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { InboxMark } from "@/components/common"
import { FormCard, FormGrid } from "@/components/common/FormCard"
import {
  boxRowClass,
  FilterMenu,
  FilterRow,
  KeyHints,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  RowCheckbox,
} from "@/components/common/ListBox"
import { CHANGELOG_KEYS, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { formatDate, formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { EntryBulkBar } from "@/features/changelog/Bulk"
import { entryLink, useCreateEntry, useEntries, useEntry } from "@/features/changelog/queries"
import {
  entryTitle,
  ErrorLine,
  LABELS,
  LABEL_TONES,
  useEntryStateText,
  useFailureText,
  useLabelText,
  useTranslationStateText,
} from "@/features/changelog/text"
import { useLocaleName } from "@/features/help/text"
import { useListCursor } from "@/features/inbox/listCursor"
import { Chip, Pill, ProductTag } from "@/features/status/PageFrame"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Changelog, ChangelogEntrySummary, ChangelogLabel, Inbox } from "@/lib/api"
import type { ChangelogEntryFilters } from "@/lib/keys"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function chipTone(state: string | undefined) {
  if (state === "published") return "border-success/40 bg-success/10 text-success"
  if (state === "changed") return "border-warning/50 bg-warning/10 text-warning"
  if (state === "draft") return "border-border bg-muted text-muted-foreground"
  return "border-dashed border-border text-faint"
}

function LocaleChips({ e, changelog }: { e: ChangelogEntrySummary; changelog: Changelog }) {
  const { t } = useLingui()
  const name = useLocaleName()
  const stateText = useTranslationStateText()
  return (
    <span className="flex flex-wrap gap-1" data-testid="entry-locales">
      {changelog.locales.map((l) => {
        const tr = e.translations.find((x) => x.locale === l)
        const label = tr
          ? `${name(l)}: ${stateText[tr.state]}${tr.outdated ? `, ${t`out of date`}` : ""}`
          : `${name(l)}: ${t`not written`}`
        return (
          <Link
            key={l}
            to={entryLink(changelog.id, e.id, l)}
            title={label}
            aria-label={label}
            className={cn(
              "relative z-10 inline-flex h-5 items-center rounded-md border px-1.5 text-[11px] leading-none font-semibold tracking-[0.02em] uppercase outline-none focus-visible:ring-2 focus-visible:ring-ring",
              chipTone(tr?.state),
              tr?.outdated && "underline decoration-warning decoration-2 underline-offset-2"
            )}
            data-testid={`entry-locale-${l}`}
          >
            {l}
          </Link>
        )
      })}
    </span>
  )
}

export function NewEntryForm({
  changelogs,
  initialId,
  onClose,
}: {
  changelogs: Changelog[]
  initialId?: string
  onClose: () => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const labelText = useLabelText()
  const [changelogId, setChangelogId] = useState(
    changelogs.find((c) => c.id === initialId)?.id ?? changelogs[0]?.id ?? ""
  )
  const changelog = changelogs.find((c) => c.id === changelogId) ?? changelogs[0]
  const create = useCreateEntry(changelog?.id ?? "")
  const [title, setTitle] = useState("")
  const [locale, setLocale] = useState(changelog?.default_locale ?? "en")
  const [labels, setLabels] = useState<ChangelogLabel[]>(["new"])
  const ok = !!changelog && title.trim().length >= 3 && labels.length > 0
  const toggle = (l: ChangelogLabel) =>
    setLabels((cur) =>
      cur.includes(l) ? cur.filter((x) => x !== l) : LABELS.filter((x) => x === l || cur.includes(x))
    )
  const locales = changelog?.locales ?? []
  return (
    <FormCard
      title={<Trans>New post</Trans>}
      description={
        <Trans>Start with a title. You write the text next, and it stays a draft until it is published.</Trans>
      }
      submit={<Trans>Create entry</Trans>}
      ready={ok}
      busy={create.isPending}
      error={create.error}
      onCancel={onClose}
      onSubmit={() =>
        changelog &&
        create.mutate(
          { locale, title: title.trim(), labels },
          { onSuccess: (entry) => navigate(entryLink(changelog.id, entry.id, locale)) }
        )
      }
      testId="new-entry-form"
    >
      <Field label={<Trans>Title</Trans>} htmlFor="new-entry-title" hint={<Trans>At least 3 characters.</Trans>}>
        <Input
          id="new-entry-title"
          value={title}
          maxLength={200}
          autoFocus
          required
          onChange={(e) => setTitle(e.target.value)}
          data-testid="new-entry-title"
        />
      </Field>
      <Field label={<Trans>Labels</Trans>}>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t`Labels`}>
          {LABELS.map((l) => (
            <button
              key={l}
              type="button"
              aria-pressed={labels.includes(l)}
              onClick={() => toggle(l)}
              className={cn(
                "rounded-lg border px-3 py-1.5 text-body font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring",
                labels.includes(l) ? LABEL_TONES[l] : "border-border text-muted-foreground"
              )}
              data-testid={`new-entry-label-${l}`}
            >
              {labelText[l]}
            </button>
          ))}
        </div>
      </Field>
      <FormGrid>
        {changelogs.length > 1 && (
          <Field label={<Trans>Changelog</Trans>} htmlFor="new-entry-changelog">
            <ChoiceSelect
              id="new-entry-changelog"
              value={changelogId}
              onChange={(id) => {
                setChangelogId(id)
                const next = changelogs.find((c) => c.id === id)
                if (next && !next.locales.includes(locale)) setLocale(next.default_locale)
              }}
              options={changelogs.map((c) => [c.id, c.name] as const)}
              label={t`Changelog`}
            />
          </Field>
        )}
        {locales.length > 1 && (
          <Field label={<Trans>Language</Trans>} htmlFor="new-entry-locale">
            <ChoiceSelect
              id="new-entry-locale"
              value={locale}
              onChange={setLocale}
              options={locales.map((l) => [l, localeName(l)] as const)}
              label={t`Language`}
            />
          </Field>
        )}
      </FormGrid>
    </FormCard>
  )
}

function plain(md: string) {
  return md
    .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[#>*_`~-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
}

function useEntryText(changelog: Changelog, entry: ChangelogEntrySummary) {
  const { i18n } = useLingui()
  const full = useEntry(changelog.id, entry.id).data
  const tr =
    full?.translations.find((x) => x.locale === i18n.locale) ??
    full?.translations.find((x) => x.locale === changelog.default_locale) ??
    full?.translations[0]
  const title =
    tr?.published?.title ?? tr?.title ?? entryTitle(entry.translations, i18n.locale, changelog.default_locale)
  const body = tr?.published?.excerpt || plain(tr?.body ?? "")
  return { title, body, loading: !full }
}

function DateBlock({ iso }: { iso: string }) {
  const { i18n } = useLingui()
  const d = new Date(iso)
  return (
    <time
      dateTime={iso}
      className="flex size-12 shrink-0 flex-col items-center justify-center rounded-xl border bg-surface leading-[1.15] @max-xl:size-11"
      title={formatDate(iso, i18n.locale)}
    >
      <span className="sr-only">{formatDate(iso, i18n.locale)}</span>
      <span className="text-[10.5px] tracking-[0.04em] text-faint uppercase" aria-hidden>
        {new Intl.DateTimeFormat(i18n.locale, { month: "short" }).format(d)}
      </span>
      <span className="text-[18px] leading-5 font-semibold tabular-nums" aria-hidden>
        {new Intl.DateTimeFormat(i18n.locale, { day: "2-digit" }).format(d)}
      </span>
    </time>
  )
}

const pillOf = { published: "success", scheduled: "brand", draft: "muted" } as const

export function EntryRow({
  e,
  changelog,
  inbox,
  cursor,
  checked,
  onToggle,
  onOpen,
}: {
  e: ChangelogEntrySummary
  changelog: Changelog
  inbox?: Inbox
  cursor: boolean
  checked: boolean
  onToggle: (range: boolean) => void
  onOpen: () => void
}) {
  const { i18n } = useLingui()
  const stateText = useEntryStateText()
  const labelText = useLabelText()
  const failureText = useFailureText()
  const { title, body, loading } = useEntryText(changelog, e)
  const when =
    e.state === "published" && e.published_at ? (
      formatShort(e.published_at, i18n.locale)
    ) : e.state === "scheduled" && e.publish_at ? (
      formatShort(e.publish_at, i18n.locale)
    ) : (
      <Trans>not published</Trans>
    )
  return (
    <div
      data-row
      data-row-id={e.id}
      className={boxRowClass({ cursor, checked })}
      aria-selected={checked || undefined}
      data-testid="entry-row"
    >
      <RowCheckbox checked={checked} name={title} onToggle={onToggle} className="mt-[30px]" />
      <div
        role="button"
        tabIndex={-1}
        onClick={onOpen}
        className="flex min-w-0 flex-1 cursor-pointer items-start gap-3.5 py-3.5 pe-3.5 outline-none"
      >
        <DateBlock iso={e.published_at ?? e.publish_at ?? e.created_at} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <span className="flex min-w-0 flex-wrap items-center gap-1.5">
            <Pill tone={pillOf[e.state]} data-testid="entry-state" data-state={e.state}>
              {stateText[e.state]}
            </Pill>
            {e.labels.map((l) => (
              <Chip key={l}>{labelText[l]}</Chip>
            ))}
            {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
          </span>
          <Link
            to={entryLink(changelog.id, e.id, changelog.default_locale)}
            onClick={(ev) => ev.stopPropagation()}
            className="line-clamp-2 text-[15px] leading-[21px] font-semibold break-words outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
          >
            {title}
          </Link>
          {loading ? (
            <Skeleton className="my-1 h-3.5 w-72 max-w-full" />
          ) : (
            <span className="line-clamp-2 text-body leading-[19px] break-words text-muted-foreground">
              {body || title}
            </span>
          )}
          {e.schedule_failed && (
            <span className="flex items-center gap-1.5 text-small text-warning" data-testid="entry-schedule-failed">
              <TriangleAlertIcon className="size-3.5 shrink-0" />
              <span>
                <Trans>Publishing on schedule failed.</Trans> {failureText[e.schedule_failed.reason]}
              </span>
            </span>
          )}
          <span
            className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-caption text-faint tabular-nums"
            onClick={(ev) => ev.stopPropagation()}
          >
            {changelog.locales.length > 1 && <LocaleChips e={e} changelog={changelog} />}
            <span>{when}</span>
          </span>
        </div>
      </div>
    </div>
  )
}

export function EntryPreview({
  changelog,
  entry,
  flat,
}: {
  changelog: Changelog
  entry: ChangelogEntrySummary
  flat?: boolean
}) {
  const inbox = (useInboxes().data ?? []).find((i) => i.id === changelog.inbox_id)
  const labelText = useLabelText()
  const { title, body } = useEntryText(changelog, entry)
  const color = inbox?.branding.color ?? "var(--brand)"
  const host = changelog.url.replace(/^https?:\/\//, "").replace(/\/$/, "")
  return (
    <aside
      className={cn("flex flex-col gap-3", !flat && "w-[320px] shrink-0 rounded-[14px] border bg-card p-4")}
      aria-label={title}
      data-testid="entry-preview"
    >
      <h3
        className={
          flat
            ? "m-0 text-caption font-medium text-muted2"
            : "text-[11px] font-semibold tracking-[0.06em] text-faint uppercase"
        }
      >
        {inbox ? <Trans>How it looks in {inbox.name}</Trans> : <Trans>How it looks</Trans>}
      </h3>
      <div className="overflow-hidden rounded-xl border bg-surface">
        <div className="flex items-center gap-2 px-4 py-3">
          {inbox && <InboxMark name={inbox.name} color={inbox.branding.color} className="size-[22px] text-caption" />}
          <span className="flex-1 text-body font-semibold">
            <Trans>What’s new</Trans>
          </span>
        </div>
        <div className="relative h-[84px]">
          {entry.image ? (
            <img src={entry.image.url} alt="" className="size-full object-cover" />
          ) : (
            <div
              className="size-full"
              style={{ background: `linear-gradient(135deg, ${color}, color-mix(in oklab, ${color} 45%, white))` }}
            />
          )}
          {entry.labels[0] && (
            <span className="absolute start-3.5 bottom-3.5 rounded-full bg-white px-2 py-0.5 text-caption font-semibold text-black">
              {labelText[entry.labels[0]]}
            </span>
          )}
        </div>
        <div className="flex flex-col gap-1 p-3">
          <p className="text-[13.5px] leading-[18px] font-semibold break-words">{title}</p>
          {body ? (
            <p className="line-clamp-4 text-[12.5px] leading-[18px] break-words text-muted-foreground">{body}</p>
          ) : (
            <p className="text-body text-faint">
              <Trans>No text yet.</Trans>
            </p>
          )}
        </div>
      </div>
      <p className="text-small text-faint">
        <Trans>Shown in the chat bubble, the apps and on {host}.</Trans>
      </p>
    </aside>
  )
}

export function EntryList({
  rows,
  changelogs,
  pending,
  error,
  onRetry,
  hasMore,
  loadingMore,
  onMore,
  empty,
  onClear,
  scope,
  active,
  onCursor,
}: {
  rows: ChangelogEntrySummary[]
  changelogs: Map<string, Changelog>
  pending: boolean
  error: unknown
  onRetry: () => void
  hasMore?: boolean
  loadingMore?: boolean
  onMore?: () => void
  empty: React.ReactNode
  onClear?: () => void
  scope: string
  active: boolean
  onCursor?: (e: ChangelogEntrySummary | undefined) => void
}) {
  const { i18n } = useLingui()
  const navigate = useNavigate()
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const selection = useSelection(
    rows.map((e) => e.id),
    scope
  )
  const listRef = useRef<HTMLDivElement>(null)
  const open = (id: string) => {
    const e = rows.find((x) => x.id === id)
    const c = e && changelogs.get(e.changelog_id)
    if (e && c) navigate(entryLink(c.id, e.id, c.default_locale))
  }
  const { cursorId, setCursorId } = useListCursor(rows, null, open, active, listRef, true, true)
  const [seen, setSeen] = useState(scope)
  if (seen !== scope) {
    setSeen(scope)
    setCursorId(null)
  }
  const shown = rows.find((e) => e.id === cursorId) ?? rows[0]
  useEffect(() => onCursor?.(shown), [shown, onCursor])
  useHotkeys(
    {
      [SHORTCUTS.select]: () => cursorId && selection.toggle(cursorId),
      ...(selection.selecting ? { Escape: selection.stop } : {}),
    },
    active
  )
  const fmt = new Intl.NumberFormat(i18n.locale)
  const shownCount = `${fmt.format(rows.length)}${hasMore ? "+" : ""}`
  const many = changelogs.size > 1
  return (
    <div ref={listRef} className="@container min-w-0">
      <ListBox
        testId="list"
        bar={selection.selecting && <EntryBulkBar changelogs={changelogs} items={rows} selection={selection} />}
      >
        {pending ? (
          <ListBoxSkeleton rows={3} />
        ) : error && rows.length === 0 ? (
          <ListBoxEmpty
            action={
              <Button variant="outline" size="sm" onClick={onRetry}>
                <Trans>Try again</Trans>
              </Button>
            }
          >
            <ErrorLine error={error} />
          </ListBoxEmpty>
        ) : rows.length === 0 ? (
          <ListBoxEmpty
            testId="changelog-entries-empty"
            action={
              onClear && (
                <Button variant="outline" size="sm" onClick={onClear}>
                  <Trans>Clear filters</Trans>
                </Button>
              )
            }
          >
            {empty}
          </ListBoxEmpty>
        ) : (
          rows.map((e) => {
            const c = changelogs.get(e.changelog_id)
            if (!c) return null
            return (
              <EntryRow
                key={e.id}
                e={e}
                changelog={c}
                inbox={many ? inboxes.get(c.inbox_id) : undefined}
                cursor={e.id === cursorId}
                checked={selection.has(e.id)}
                onToggle={(range) => selection.toggle(e.id, range)}
                onOpen={() => (selection.selecting ? selection.toggle(e.id) : setCursorId(e.id))}
              />
            )
          })
        )}
      </ListBox>
      {hasMore && (
        <Button variant="outline" size="sm" className="mt-3" disabled={loadingMore} onClick={onMore}>
          <Trans>Load more</Trans>
        </Button>
      )}
      {rows.length > 0 && (
        <p className="m-0 mt-3 text-small text-muted-foreground" data-testid="list-lede">
          {plural(rows.length, { one: `${shownCount} entry`, other: `${shownCount} entries` })}
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
          [[CHANGELOG_KEYS.newEntry], <Trans key="new">new post</Trans>],
        ]}
      />
    </div>
  )
}

export function EntriesTab({
  changelog,
  active,
  onCursor,
}: {
  changelog: Changelog
  active: boolean
  onCursor?: (e: ChangelogEntrySummary | undefined) => void
}) {
  const { t } = useLingui()
  const stateText = useEntryStateText()
  const translationText = useTranslationStateText()
  const labelText = useLabelText()
  const [state, setState] = useState<NonNullable<ChangelogEntryFilters["state"]> | "">("")
  const [label, setLabel] = useState<ChangelogLabel | "">("")
  const filters: ChangelogEntryFilters = {
    state: state || undefined,
    label: label || undefined,
  }
  const entries = useEntries(changelog.id, filters)
  const items = entries.data?.pages.flatMap((p) => p.items) ?? []
  const filtering = !!(state || label)
  const clear = () => {
    setState("")
    setLabel("")
  }
  return (
    <div data-testid="changelog-entries">
      <FilterRow testId="changelog-filters">
        <FilterMenu
          value={state}
          onChange={(v) => setState(v as NonNullable<ChangelogEntryFilters["state"]> | "")}
          options={[
            { value: "draft", label: stateText.draft },
            { value: "scheduled", label: stateText.scheduled },
            { value: "published", label: stateText.published },
            { value: "changed", label: translationText.changed },
          ]}
          anyLabel={t`Any state`}
          label={t`State`}
          testId="changelog-state"
        />
        <FilterMenu
          value={label}
          onChange={(v) => setLabel(v as ChangelogLabel | "")}
          options={LABELS.map((l) => ({ value: l, label: labelText[l] }))}
          anyLabel={t`All labels`}
          label={t`Label`}
          testId="changelog-label"
        />
        {filtering && (
          <Button variant="ghost" size="sm" className="h-8" onClick={clear}>
            <Trans>Clear filters</Trans>
          </Button>
        )}
      </FilterRow>
      <EntryList
        rows={items}
        changelogs={new Map([[changelog.id, changelog]])}
        pending={entries.isPending}
        error={entries.error}
        onRetry={() => void entries.refetch()}
        hasMore={entries.hasNextPage}
        loadingMore={entries.isFetchingNextPage}
        onMore={() => void entries.fetchNextPage()}
        empty={
          filtering ? (
            <Trans>No entry matches.</Trans>
          ) : (
            <Trans>No entries yet. Tell your customers what changed in your latest release.</Trans>
          )
        }
        onClear={filtering ? clear : undefined}
        scope={`${state}|${label}`}
        active={active}
        onCursor={onCursor}
      />
    </div>
  )
}

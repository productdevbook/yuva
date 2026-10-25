import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon, PlusIcon, SparklesIcon, TriangleAlertIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate } from "react-router"

import { InboxMark } from "@/components/common"
import { RowCheck, SelectToggle } from "@/components/common/Selection"
import { CHANGELOG_KEYS } from "@/components/common/ShortcutSheet"
import { formatDate, formatShort } from "@/components/common/text"
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
import { Chip, FrameButton, Pill, ProductTag } from "@/features/status/PageFrame"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useSelection } from "@/hooks/use-selection"
import type { Changelog, ChangelogEntrySummary, ChangelogLabel, Inbox } from "@/lib/api"
import type { ChangelogEntryFilters } from "@/lib/keys"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type StateFilter = NonNullable<ChangelogEntryFilters["state"]> | "all"

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

export function NewEntryDialog({ changelog, onClose }: { changelog: Changelog; onClose: () => void }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const labelText = useLabelText()
  const create = useCreateEntry(changelog.id)
  const [title, setTitle] = useState("")
  const [locale, setLocale] = useState(changelog.default_locale)
  const [labels, setLabels] = useState<ChangelogLabel[]>(["new"])
  const ok = title.trim().length >= 3 && labels.length > 0
  const toggle = (l: ChangelogLabel) =>
    setLabels((cur) =>
      cur.includes(l) ? cur.filter((x) => x !== l) : LABELS.filter((x) => x === l || cur.includes(x))
    )
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-entry-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!ok) return
            create.mutate(
              { locale, title: title.trim(), labels },
              { onSuccess: (entry) => navigate(entryLink(changelog.id, entry.id, locale)) }
            )
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>New entry</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>Start with a title. You write the text next, and it stays a draft until it is published.</Trans>
            </DialogDescription>
          </DialogHeader>
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
          <Field label={<Trans>Language</Trans>} htmlFor="new-entry-locale">
            <ChoiceSelect
              id="new-entry-locale"
              value={locale}
              onChange={setLocale}
              options={changelog.locales.map((l) => [l, localeName(l)] as const)}
              label={t`Language`}
              className="w-64"
            />
          </Field>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || create.isPending} data-testid="create-entry">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create entry</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
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
  current,
  onPick,
  locales,
  check,
}: {
  e: ChangelogEntrySummary
  changelog: Changelog
  inbox?: Inbox
  current: boolean
  onPick: () => void
  locales?: boolean
  check?: { checked: boolean; onToggle: (range: boolean) => void }
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
    <li
      className={cn(
        "relative flex items-start gap-3.5 rounded-[14px] border bg-card p-4 transition-colors hover:border-faint @max-xl:gap-3 @max-xl:p-3",
        current &&
          "border-brand shadow-[0_0_0_3px_color-mix(in_srgb,var(--brand)_16%,transparent)] before:absolute before:inset-y-4 before:start-1 before:w-[3px] before:rounded-[3px] before:bg-brand"
      )}
      onClick={onPick}
      data-testid="entry-row"
    >
      {check && <RowCheck checked={check.checked} name={title} onToggle={check.onToggle} className="mt-3.5" />}
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
        <span className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1.5 text-caption text-faint tabular-nums">
          {locales && changelog.locales.length > 1 && <LocaleChips e={e} changelog={changelog} />}
          <span>{when}</span>
        </span>
      </div>
    </li>
  )
}

export function EntryPreview({ changelog, entry }: { changelog: Changelog; entry: ChangelogEntrySummary }) {
  const inbox = (useInboxes().data ?? []).find((i) => i.id === changelog.inbox_id)
  const labelText = useLabelText()
  const { title, body } = useEntryText(changelog, entry)
  const color = inbox?.branding.color ?? "var(--brand)"
  const host = changelog.url.replace(/^https?:\/\//, "").replace(/\/$/, "")
  return (
    <aside
      className="flex w-[320px] shrink-0 flex-col gap-3 rounded-[14px] border bg-card p-4"
      aria-label={title}
      data-testid="entry-preview"
    >
      <h3 className="text-[11px] font-semibold tracking-[0.06em] text-faint uppercase">
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

export function EntriesTab({ changelog }: { changelog: Changelog }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const stateText = useEntryStateText()
  const translationText = useTranslationStateText()
  const labelText = useLabelText()
  const [state, setState] = useState<StateFilter>("all")
  const [label, setLabel] = useState<ChangelogLabel | "">("")
  const [creating, setCreating] = useState(false)
  const [cursor, setCursor] = useState(-1)
  const [picked, setPicked] = useState("")
  const list = useRef<HTMLUListElement>(null)
  const filters: ChangelogEntryFilters = {
    state: state === "all" ? undefined : state,
    label: label || undefined,
  }
  const entries = useEntries(changelog.id, filters)
  const items = entries.data?.pages.flatMap((p) => p.items) ?? []
  const shown = items[cursor] ?? items.find((e) => e.id === picked) ?? items[0]
  const selection = useSelection(
    items.map((e) => e.id),
    `${state}|${label}`
  )

  useHotkeys({
    [CHANGELOG_KEYS.newEntry]: () => setCreating(true),
    j: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    k: () => setCursor((i) => Math.max(0, i - 1)),
    ArrowDown: () => setCursor((i) => Math.min(items.length - 1, i + 1)),
    ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
    ...(cursor >= 0 && items[cursor]
      ? { Enter: () => navigate(entryLink(changelog.id, items[cursor].id, changelog.default_locale)) }
      : {}),
  })
  useEffect(() => {
    if (cursor >= 0) list.current?.children[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])

  const states: [StateFilter, string][] = [
    ["all", t`Any state`],
    ["draft", stateText.draft],
    ["scheduled", stateText.scheduled],
    ["published", stateText.published],
    ["changed", translationText.changed],
  ]

  return (
    <div className="@container flex min-h-0 flex-1" data-testid="changelog-entries">
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <div className="flex flex-wrap items-center gap-2 border-b px-6 py-3 phone:px-4">
          <div className="flex items-center gap-2 phone:w-full">
            <ChoiceSelect
              value={state}
              onChange={(v) => {
                setState(v)
                setCursor(-1)
              }}
              options={states}
              label={t`State`}
              className="w-48 phone:min-w-0 phone:flex-1"
            />
            <ChoiceSelect
              value={label}
              onChange={(v) => {
                setLabel(v)
                setCursor(-1)
              }}
              options={[["", t`All labels`] as const, ...LABELS.map((l) => [l, labelText[l]] as const)]}
              label={t`Label`}
              className="w-40 phone:min-w-0 phone:flex-1"
            />
          </div>
          <span className="flex-1 phone:hidden" />
          <div className="flex items-center gap-2 phone:w-full">
            {items.length > 0 && !selection.selecting && <SelectToggle selection={selection} />}
            <FrameButton className="phone:flex-1" onClick={() => setCreating(true)} data-testid="new-entry">
              <PlusIcon />
              <Trans>New entry</Trans>
            </FrameButton>
          </div>
        </div>
        <EntryBulkBar changelog={changelog} items={items} selection={selection} />
        {entries.isPending ? (
          <div className="flex flex-col gap-2 p-6">
            <Skeleton className="h-14 w-full rounded-xl" />
            <Skeleton className="h-14 w-full rounded-xl" />
          </div>
        ) : entries.error ? (
          <ErrorLine error={entries.error} className="m-6" />
        ) : items.length === 0 ? (
          <div
            className="m-6 flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-12 text-center"
            data-testid="changelog-entries-empty"
          >
            <SparklesIcon className="size-6 text-faint" />
            <p className="text-body text-muted-foreground">
              {state !== "all" || label ? (
                <Trans>No entry matches.</Trans>
              ) : (
                <Trans>No entries yet. Tell your customers what changed in your latest release.</Trans>
              )}
            </p>
          </div>
        ) : (
          <ul ref={list} className="flex flex-col gap-2.5 p-6 @max-xl:p-4">
            {items.map((e, i) => (
              <EntryRow
                key={e.id}
                e={e}
                changelog={changelog}
                locales
                check={
                  selection.selecting
                    ? { checked: selection.has(e.id), onToggle: (range) => selection.toggle(e.id, range) }
                    : undefined
                }
                current={i === cursor || (cursor < 0 && e.id === shown?.id)}
                onPick={() => {
                  setPicked(e.id)
                  setCursor(-1)
                }}
              />
            ))}
          </ul>
        )}
        {entries.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            className="m-6 self-center"
            disabled={entries.isFetchingNextPage}
            onClick={() => entries.fetchNextPage()}
          >
            <Trans>Load more</Trans>
          </Button>
        )}
      </div>
      {shown && (
        <div className="hidden min-h-0 pt-6 pe-6 @min-[1000px]:flex">
          <EntryPreview changelog={changelog} entry={shown} />
        </div>
      )}
      {creating && <NewEntryDialog changelog={changelog} onClose={() => setCreating(false)} />}
    </div>
  )
}

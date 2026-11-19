import "@/features/changelog/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"
import { ArrowUpRightIcon, EyeIcon, PlusIcon, TriangleAlertIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { InboxMark, InfoColumn, PageLayout, PageTop } from "@/components/common"
import { FilterMenu, FilterRow, FilterSearch, ListBox, ListBoxEmpty, PageTitle } from "@/components/common/ListBox"
import { CHANGELOG_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CreateChangelogDialog } from "@/features/changelog/CreateChangelogDialog"
import { EntriesTab, EntryList, EntryPreview, NewEntryForm } from "@/features/changelog/EntriesTab"
import {
  changelogLink,
  changelogOfInbox,
  entryLink,
  useAllEntries,
  useChangelog,
  useChangelogs,
  useUpdateChangelog,
} from "@/features/changelog/queries"
import { PreviewDialog } from "@/features/changelog/PreviewDialog"
import { SettingsTab } from "@/features/changelog/SettingsTab"
import { entryTitle, ErrorLine, LABELS, useFailureText, useLabelText } from "@/features/changelog/text"
import { useLocaleName } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { keys } from "@/lib/keys"
import type { ChangelogEntrySummary, ChangelogLabel, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export { EntryEditorPage } from "@/features/changelog/EntryEditor"

const TABS = ["entries", "settings"] as const
type Tab = (typeof TABS)[number]

const VIEWS = ["published", "scheduled", "draft"] as const
type View = (typeof VIEWS)[number]

export function ChangelogHome() {
  const { t, i18n } = useLingui()
  const queryClient = useQueryClient()
  const [inboxId] = useInboxFilter()
  const [params, setParams] = useSearchParams()
  const changelogs = useChangelogs()
  const items = (changelogs.data ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  const byId = new Map(items.map((c) => [c.id, c]))
  const inboxes = useInboxes().data ?? []
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const labelText = useLabelText()
  const failureText = useFailureText()
  const { canManage, workspaceId } = useSession()
  const [creating, setCreating] = useState<Inbox | null>(null)
  const [writing, setWriting] = useState(false)
  const [shown, setShown] = useState<ChangelogEntrySummary | undefined>()
  const search = useRef<HTMLInputElement>(null)
  const raw = params.get("state") as View | null
  const view: View | "" = raw && VIEWS.includes(raw) ? raw : ""
  const query = params.get("q") ?? ""
  const label = (params.get("label") ?? "") as ChangelogLabel | ""
  const locale = params.get("locale") ?? ""
  const set = (key: string, value: string) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p)
        if (value) next.set(key, value)
        else next.delete(key)
        return next
      },
      { replace: true }
    )
  const entries = useAllEntries(
    items.map((c) => c.id),
    { ...(view ? { state: view } : {}), ...(label ? { label } : {}) }
  )
  const when = (e: ChangelogEntrySummary) => e.published_at ?? e.publish_at ?? e.created_at
  const needle = query.trim().toLowerCase()
  const rows = entries.data
    .filter((e) => byId.has(e.changelog_id))
    .filter((e) => !locale || e.translations.some((x) => x.locale === locale))
    .filter((e) => !needle || e.translations.some((x) => x.title.toLowerCase().includes(needle)))
    .sort((a, b) => when(b).localeCompare(when(a)))
  const filtering = !!(needle || label || locale || view)
  const failed = rows.filter((e) => e.schedule_failed)
  const shownLog = shown && byId.get(shown.changelog_id)
  const shownInboxes = inboxId ? inboxes.filter((i) => i.id === inboxId) : inboxes
  const missing =
    canManage && changelogs.data ? shownInboxes.find((i) => !changelogOfInbox(changelogs.data, i.id)) : undefined
  const createKey = keyLabel(CHANGELOG_KEYS.create).join("")
  const newKey = keyLabel(CHANGELOG_KEYS.newEntry).join("")
  const manageKey = keyLabel(CHANGELOG_KEYS.manage).join("")
  const open = !creating && !writing
  const counts = items.reduce(
    (n, c) => ({
      published: n.published + c.counts.published,
      scheduled: n.scheduled + c.counts.scheduled,
      all: n.all + c.counts.entries,
    }),
    { all: 0, published: 0, scheduled: 0 }
  )
  const viewCounts: Record<View, number> = {
    published: counts.published,
    scheduled: counts.scheduled,
    draft: counts.all - counts.published - counts.scheduled,
  }
  const viewNames: Record<View, string> = { published: t`Published`, scheduled: t`Scheduled`, draft: t`Drafts` }
  const locales = [...new Set(items.flatMap((c) => c.locales))]
  const fmt = new Intl.NumberFormat(i18n.locale)
  const reset = () => setParams(new URLSearchParams(), { replace: true })
  useHotkeys(
    {
      ...(missing ? { [CHANGELOG_KEYS.create]: () => setCreating(missing) } : {}),
      ...(items.length > 0 ? { [CHANGELOG_KEYS.newEntry]: () => setWriting(true) } : {}),
      ...(items.length > 0 || missing ? { [CHANGELOG_KEYS.manage]: () => navigate("/updates/changelogs") } : {}),
      "/": () => search.current?.focus(),
    },
    open
  )
  const emptyText = filtering ? <Trans>No entry matches.</Trans> : <Trans>Write the first entry with New post.</Trans>
  const firstFailed = failed[0]
  const firstFailedLog = firstFailed && byId.get(firstFailed.changelog_id)
  return (
    <PageLayout
      testId="changelog-home"
      label={t`Updates`}
      aside={
        <InfoColumn label={t`Preview`}>
          {shown && shownLog && <EntryPreview changelog={shownLog} entry={shown} flat />}
        </InfoColumn>
      }
    >
      <section className="flex min-w-0 flex-col">
        <PageTitle
          title={<Trans>Updates</Trans>}
          sub={<Trans>Release notes shown in your products and on the web</Trans>}
          actions={
            <>
              {(items.length > 0 || missing) && (
                <Button
                  variant="outline"
                  render={<Link to="/updates/changelogs" />}
                  title={t`Changelogs (${manageKey})`}
                  data-testid="changelog-home-menu"
                >
                  <Trans>Changelogs</Trans>
                </Button>
              )}
              {items.length > 0 ? (
                <Button
                  onClick={() => setWriting(true)}
                  disabled={writing}
                  title={t`New post (${newKey})`}
                  data-testid="new-entry"
                >
                  <PlusIcon />
                  <Trans>New post</Trans>
                </Button>
              ) : (
                missing && (
                  <Button onClick={() => setCreating(missing)} title={t`Create changelog (${createKey})`}>
                    <PlusIcon />
                    <Trans>Create changelog</Trans>
                  </Button>
                )
              )}
            </>
          }
          testId="changelog-head"
        />
        {writing && <NewEntryForm changelogs={items} initialId={shownLog?.id} onClose={() => setWriting(false)} />}
        {items.length > 0 && (
          <FilterRow testId="changelog-filters">
            <FilterSearch
              inputRef={search}
              value={query}
              onChange={(v) => set("q", v)}
              placeholder={t`Search entries…`}
              label={t`Search entries`}
              testId="changelog-search"
            />
            <FilterMenu
              value={view}
              onChange={(v) => set("state", v)}
              options={VIEWS.map((v) => ({ value: v, label: `${viewNames[v]} · ${fmt.format(viewCounts[v])}` }))}
              anyLabel={t`Every state`}
              label={t`State`}
              testId="changelog-state"
            />
            <FilterMenu
              value={label}
              onChange={(v) => set("label", v)}
              options={LABELS.map((l) => ({ value: l, label: labelText[l] }))}
              anyLabel={t`All labels`}
              label={t`Label`}
              testId="changelog-label"
            />
            {locales.length > 1 && (
              <FilterMenu
                value={locale}
                onChange={(v) => set("locale", v)}
                options={locales.map((l) => ({ value: l, label: localeName(l) }))}
                anyLabel={t`All languages`}
                label={t`Language`}
                testId="changelog-locale"
              />
            )}
            {filtering && (
              <Button variant="ghost" size="sm" className="h-8" onClick={reset} data-testid="changelog-clear">
                <Trans>Clear filters</Trans>
              </Button>
            )}
          </FilterRow>
        )}
        {failed.length > 0 && firstFailed && firstFailedLog && (
          <div
            role="alert"
            className="mt-3 flex flex-wrap items-center gap-3 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2.5"
            data-testid="changelog-failed-banner"
          >
            <TriangleAlertIcon className="size-4 shrink-0 text-warning" />
            <p className="m-0 min-w-48 flex-1 text-body">
              <span className="font-semibold text-foreground">
                <Trans>Publishing on schedule failed.</Trans>
              </span>{" "}
              <span className="text-muted-foreground">
                {entryTitle(firstFailed.translations, firstFailedLog.default_locale)}
                {": "}
                {firstFailed.schedule_failed && failureText[firstFailed.schedule_failed.reason]}{" "}
                <Trans>Publish it now or schedule it again.</Trans>
              </span>
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                navigate(entryLink(firstFailed.changelog_id, firstFailed.id, firstFailedLog.default_locale))
              }
            >
              <Trans>Review</Trans>
            </Button>
          </div>
        )}
        {!changelogs.isPending && items.length === 0 ? (
          <ListBox>
            <ListBoxEmpty
              action={
                missing && (
                  <Button variant="outline" size="sm" onClick={() => setCreating(missing)}>
                    <PlusIcon />
                    <Trans>Create changelog</Trans>
                  </Button>
                )
              }
            >
              <ErrorLine error={changelogs.error} />
              <Trans>
                Tell your customers what is new, improved and fixed, on a public page, in the chat widget and by mail.
              </Trans>
            </ListBoxEmpty>
          </ListBox>
        ) : (
          <EntryList
            rows={rows}
            changelogs={byId}
            pending={changelogs.isPending || entries.isPending}
            error={changelogs.error ?? entries.error}
            onRetry={() => void queryClient.invalidateQueries({ queryKey: keys.changelog(workspaceId) })}
            empty={emptyText}
            onClear={filtering ? reset : undefined}
            scope={`${view}|${label}|${locale}|${needle}|${inboxId}`}
            active={open}
            onCursor={setShown}
          />
        )}
      </section>
      {creating && (
        <CreateChangelogDialog
          inboxId={creating.id}
          inboxName={creating.name}
          onClose={() => setCreating(null)}
          onCreated={(c) => {
            setCreating(null)
            navigate(changelogLink(c.id, "settings"))
          }}
        />
      )}
    </PageLayout>
  )
}

export function ChangelogPane() {
  const { t } = useLingui()
  const { changelogId = "" } = useParams()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const changelog = useChangelog(changelogId)
  const patch = useUpdateChangelog(changelogId)
  const { canManage } = useSession()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === changelog.data?.inbox_id)
  const [preview, setPreview] = useState(false)
  const [writing, setWriting] = useState(false)
  const [shown, setShown] = useState<ChangelogEntrySummary | undefined>()
  const raw = params.get("tab")
  const tab: Tab = raw === "settings" && canManage ? "settings" : "entries"
  const setTab = (v: Tab) => navigate(changelogLink(changelogId, v === "entries" ? undefined : v), { replace: true })
  const c = changelog.data
  const busy = preview || writing

  useHotkeys(
    {
      [CHANGELOG_KEYS.entries]: () => setTab("entries"),
      [CHANGELOG_KEYS.preview]: () => setPreview(true),
      ...(tab === "entries" ? { [CHANGELOG_KEYS.newEntry]: () => setWriting(true) } : {}),
      ...(canManage ? { [CHANGELOG_KEYS.settings]: () => setTab("settings") } : {}),
      ...(c?.published ? { [CHANGELOG_KEYS.openPage]: () => window.open(c.url, "_blank", "noopener,noreferrer") } : {}),
      ...(canManage && c ? { [CHANGELOG_KEYS.publish]: () => patch.mutate({ published: !c.published }) } : {}),
    },
    !!c && !busy
  )

  const back = { label: t`Updates`, to: "/updates" }
  if (!c)
    return (
      <PageLayout testId="changelog" label={t`Updates`}>
        <PageTop back={back} />
        {changelog.isPending ? (
          <div className="mt-3 flex max-w-[760px] flex-col gap-4">
            <Skeleton className="h-8 w-60" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : (
          <ErrorLine error={changelog.error} className="mt-3" />
        )}
      </PageLayout>
    )
  const previewKey = keyLabel(CHANGELOG_KEYS.preview).join("")
  const publishKey = keyLabel(CHANGELOG_KEYS.publish).join("")
  const settingsKey = keyLabel(CHANGELOG_KEYS.settings).join("")
  const entriesKey = keyLabel(CHANGELOG_KEYS.entries).join("")
  const newKey = keyLabel(CHANGELOG_KEYS.newEntry).join("")
  return (
    <PageLayout
      testId="changelog"
      label={c.name}
      aside={
        <InfoColumn label={t`Preview`}>
          {tab === "entries" && shown && <EntryPreview changelog={c} entry={shown} flat />}
        </InfoColumn>
      }
    >
      <PageTop back={back} />
      <section key={c.id} className="flex min-w-0 flex-col">
        <PageTitle
          title={
            <span className="inline-flex items-center gap-2.5">
              {inbox && <InboxMark name={inbox.name} color={inbox.branding.color} className="size-6 text-caption" />}
              <span data-testid="changelog-name">{c.name}</span>
            </span>
          }
          sub={
            <span data-testid="changelog-published">
              {c.published ? <Trans>Published</Trans> : <Trans>Unpublished, only your team can see it</Trans>}
            </span>
          }
          actions={
            <>
              <Button
                variant="outline"
                onClick={() => setPreview(true)}
                title={t`Preview (${previewKey})`}
                data-testid="changelog-preview"
              >
                <EyeIcon />
                <Trans>Preview</Trans>
              </Button>
              {c.published && (
                <Button
                  variant="outline"
                  render={<a href={c.url} target="_blank" rel="noreferrer noopener" />}
                  data-testid="changelog-open-public"
                >
                  <Trans>View changelog</Trans>
                  <ArrowUpRightIcon className="rtl:-scale-x-100" />
                </Button>
              )}
              {canManage && (
                <Button
                  variant="outline"
                  disabled={patch.isPending}
                  onClick={() => patch.mutate({ published: !c.published })}
                  title={c.published ? t`Unpublish (${publishKey})` : t`Publish (${publishKey})`}
                  data-testid="changelog-publish"
                >
                  {c.published ? <Trans>Unpublish</Trans> : <Trans>Publish</Trans>}
                </Button>
              )}
              {canManage &&
                (tab === "settings" ? (
                  <Button
                    variant="outline"
                    onClick={() => setTab("entries")}
                    title={t`Entries (${entriesKey})`}
                    data-testid="changelog-tab-entries"
                  >
                    <Trans>Entries</Trans>
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    onClick={() => setTab("settings")}
                    title={t`Settings (${settingsKey})`}
                    data-testid="changelog-tab-settings"
                  >
                    <Trans>Settings</Trans>
                  </Button>
                ))}
              {tab === "entries" && (
                <Button
                  onClick={() => setWriting(true)}
                  disabled={writing}
                  title={t`New post (${newKey})`}
                  data-testid="new-entry"
                >
                  <PlusIcon />
                  <Trans>New post</Trans>
                </Button>
              )}
            </>
          }
          testId="changelog-head"
        />
        <ErrorLine error={patch.error} className="mt-3" />
        {tab === "entries" && (
          <>
            {writing && <NewEntryForm changelogs={[c]} onClose={() => setWriting(false)} />}
            <EntriesTab changelog={c} active={!busy} onCursor={setShown} />
          </>
        )}
        {tab === "settings" && canManage && (
          <div className="mt-4 max-w-[1040px]">
            <SettingsTab changelog={c} />
          </div>
        )}
      </section>
      {preview && <PreviewDialog changelog={c} onClose={() => setPreview(false)} />}
    </PageLayout>
  )
}

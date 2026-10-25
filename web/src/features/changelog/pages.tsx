import "@/features/changelog/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"
import { ArrowUpRightIcon, CircleAlertIcon, EyeIcon, PlusIcon, SparklesIcon, TriangleAlertIcon } from "lucide-react"
import { cloneElement, useEffect, useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { InboxMark } from "@/components/common"
import { CHANGELOG_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { CreateChangelogDialog } from "@/features/changelog/CreateChangelogDialog"
import { EntriesTab, EntryPreview, EntryRow, NewEntryDialog } from "@/features/changelog/EntriesTab"
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
import { ViewChips } from "@/features/inbox/ListHead"
import { FrameTabs as ViewTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { ChoiceSelect } from "@/features/settings/ui"
import { FrameButton, FrameTabs, framePad, PageFrame } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { keys } from "@/lib/keys"
import type { Changelog, ChangelogEntrySummary, ChangelogLabel, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export { EntryEditorPage } from "@/features/changelog/EntryEditor"

const TABS = ["entries", "settings"] as const
type Tab = (typeof TABS)[number]

const VIEWS = ["all", "published", "scheduled", "draft"] as const
type View = (typeof VIEWS)[number]

function ChangelogPicker({
  items,
  inboxOf,
  onPick,
  button,
  children,
}: {
  items: Changelog[]
  inboxOf: (c: Changelog) => Inbox | undefined
  onPick: (c: Changelog) => void
  button: React.ReactElement<{ onClick?: () => void }>
  children: React.ReactNode
}) {
  if (items.length === 1) return cloneElement(button, { onClick: () => onPick(items[0]) }, children)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={button}>{children}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-52">
        {items.map((c) => {
          const inbox = inboxOf(c)
          return (
            <DropdownMenuItem key={c.id} onClick={() => onPick(c)}>
              <InboxMark name={inbox?.name ?? c.name} color={inbox?.branding.color} className="size-4 text-[9px]" />
              {c.name}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function ChangelogHome() {
  const { t, i18n } = useLingui()
  const queryClient = useQueryClient()
  const [inboxId] = useInboxFilter()
  const [params] = useSearchParams()
  const inboxes = useInboxes().data ?? []
  const changelogs = useChangelogs()
  const byInbox = new Map(inboxes.map((i) => [i.id, i]))
  const inboxOf = (c: Changelog) => byInbox.get(c.inbox_id)
  const items = (changelogs.data ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  const byId = new Map(items.map((c) => [c.id, c]))
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const labelText = useLabelText()
  const failureText = useFailureText()
  const { canManage, workspaceId } = useSession()
  const [creating, setCreating] = useState<Inbox | null>(null)
  const [writing, setWriting] = useState<Changelog | null>(null)
  const [cursor, setCursor] = useState(-1)
  const [picked, setPicked] = useState("")
  const [query, setQuery] = useState("")
  const [label, setLabel] = useState<ChangelogLabel | "">("")
  const [locale, setLocale] = useState("")
  const list = useRef<HTMLDivElement>(null)
  const raw = params.get("state") as View | null
  const view: View = raw && VIEWS.includes(raw) ? raw : "all"
  const entries = useAllEntries(
    items.map((c) => c.id),
    { ...(view === "all" ? {} : { state: view }), ...(label ? { label } : {}) }
  )
  const when = (e: ChangelogEntrySummary) => e.published_at ?? e.publish_at ?? e.created_at
  const needle = query.trim().toLowerCase()
  const rows = entries.data
    .filter((e) => byId.has(e.changelog_id))
    .filter((e) => !locale || e.translations.some((x) => x.locale === locale))
    .filter((e) => !needle || e.translations.some((x) => x.title.toLowerCase().includes(needle)))
    .sort((a, b) => when(b).localeCompare(when(a)))
  const months = rows.reduce<{ key: string; name: string; rows: { e: ChangelogEntrySummary; i: number }[] }[]>(
    (groups, e, i) => {
      const d = new Date(when(e))
      const key = `${d.getFullYear()}-${d.getMonth()}`
      const last = groups[groups.length - 1]
      if (last?.key === key) last.rows.push({ e, i })
      else
        groups.push({
          key,
          name: new Intl.DateTimeFormat(i18n.locale, { month: "long", year: "numeric" }).format(d),
          rows: [{ e, i }],
        })
      return groups
    },
    []
  )
  const filtering = !!needle || !!label || !!locale
  const failed = rows.filter((e) => e.schedule_failed)
  const shown = rows[cursor] ?? rows.find((e) => e.id === picked) ?? rows[0]
  const shownLog = shown && byId.get(shown.changelog_id)
  const shownInboxes = inboxId ? inboxes.filter((i) => i.id === inboxId) : inboxes
  const missing =
    canManage && changelogs.data ? shownInboxes.find((i) => !changelogOfInbox(changelogs.data, i.id)) : undefined
  const createKey = keyLabel(CHANGELOG_KEYS.create).join("")
  const newKey = keyLabel(CHANGELOG_KEYS.newEntry).join("")
  const open = !creating && !writing
  const counts = items.reduce(
    (n, c) => ({
      all: n.all + c.counts.entries,
      published: n.published + c.counts.published,
      scheduled: n.scheduled + c.counts.scheduled,
    }),
    { all: 0, published: 0, scheduled: 0 }
  )
  const tabCounts: Record<View, number> = {
    all: counts.all,
    published: counts.published,
    scheduled: counts.scheduled,
    draft: counts.all - counts.published - counts.scheduled,
  }
  const locales = [...new Set(items.flatMap((c) => c.locales))]
  const reset = () => {
    setQuery("")
    setLabel("")
    setLocale("")
    setCursor(-1)
  }
  useHotkeys(
    {
      ...(missing ? { [CHANGELOG_KEYS.create]: () => setCreating(missing) } : {}),
      ...(items.length > 0 ? { [CHANGELOG_KEYS.newEntry]: () => setWriting(shownLog ?? items[0]) } : {}),
      ...(items.length > 0 || missing ? { [CHANGELOG_KEYS.manage]: () => navigate("/updates/changelogs") } : {}),
      j: () => setCursor((i) => Math.min(rows.length - 1, i + 1)),
      k: () => setCursor((i) => Math.max(0, i - 1)),
      ArrowDown: () => setCursor((i) => Math.min(rows.length - 1, i + 1)),
      ArrowUp: () => setCursor((i) => Math.max(0, i - 1)),
      ...(cursor >= 0 && rows[cursor] && byId.get(rows[cursor].changelog_id)
        ? {
            Enter: () =>
              navigate(
                entryLink(
                  rows[cursor].changelog_id,
                  rows[cursor].id,
                  byId.get(rows[cursor].changelog_id)?.default_locale
                )
              ),
          }
        : {}),
    },
    open
  )
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=entry-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])
  const tabs = VIEWS.map((v) => ({
    key: v,
    to: v === "all" ? "/updates" : `/updates?state=${v}`,
    label: { all: t`All`, published: t`Published`, scheduled: t`Scheduled`, draft: t`Drafts` }[v],
    count: items.length > 0 ? tabCounts[v] : undefined,
    testId: `changelog-view-${v}`,
  }))
  const emptyText = {
    all: t`Write the first entry with New post.`,
    published: t`Entries show on the public page once they are published.`,
    scheduled: t`Pick a time when you write an entry and it goes public by itself.`,
    draft: t`Entries you have not published yet show up here.`,
  }[view]
  const firstFailed = failed[0]
  const firstFailedLog = firstFailed && byId.get(firstFailed.changelog_id)
  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="changelog-home">
      <PageTop
        className="bg-surface"
        title={<Trans>What’s new</Trans>}
        sub={<Trans>Release notes shown in your products and on the web</Trans>}
        actions={
          <>
            {(items.length > 0 || missing) && (
              <Button
                variant="outline"
                className={look.btn}
                render={<Link to="/updates/changelogs" />}
                title={t`Changelogs`}
                data-testid="changelog-home-menu"
              >
                <Trans>Changelogs</Trans>
              </Button>
            )}
            {items.length > 0 && (
              <ChangelogPicker
                items={items}
                inboxOf={inboxOf}
                onPick={setWriting}
                button={<Button className={look.btnInk} title={t`New post (${newKey})`} data-testid="new-entry" />}
              >
                <Trans>New post</Trans>
              </ChangelogPicker>
            )}
          </>
        }
        tabs={<ViewTabs label={t`What’s new`} value={view} items={tabs} />}
      />
      {items.length > 0 && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b bg-surface px-6 py-3 phone:px-4">
          <Input
            type="search"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setCursor(-1)
            }}
            placeholder={t`Search entries`}
            aria-label={t`Search entries`}
            className="h-[34px] w-64 rounded-[9px] bg-card phone:w-full"
            data-testid="changelog-search"
          />
          <ViewChips<ChangelogLabel | "">
            label={t`Label`}
            value={label}
            onChange={(v) => {
              setLabel(v)
              setCursor(-1)
            }}
            className="p-0"
            items={[{ value: "", label: t`All labels` }, ...LABELS.map((l) => ({ value: l, label: labelText[l] }))]}
          />
          {locales.length > 1 && (
            <ChoiceSelect
              value={locale}
              onChange={(v) => {
                setLocale(v)
                setCursor(-1)
              }}
              options={[["", t`All languages`] as const, ...locales.map((l) => [l, localeName(l)] as const)]}
              label={t`Language`}
              className="w-44 rounded-lg bg-card"
            />
          )}
        </div>
      )}
      <div className="@container flex min-h-0 flex-1 overflow-y-auto bg-surface">
        <div className="mx-auto flex w-full max-w-[1320px] items-start gap-4 p-6 @max-xl:p-4">
          <div className="flex min-w-0 flex-1 flex-col gap-3">
            {changelogs.isPending || (items.length > 0 && entries.isPending) ? (
              <div className="flex flex-col gap-2.5" aria-busy data-testid="changelog-loading">
                {[104, 104, 88, 104].map((h, n) => (
                  <Skeleton key={n} className="w-full rounded-[14px]" style={{ height: h }} />
                ))}
              </div>
            ) : changelogs.error || entries.error ? (
              <div className="mx-auto my-12 flex max-w-sm flex-col items-center gap-3 px-4 text-center">
                <span className="grid size-8 place-items-center rounded-full bg-destructive/10 text-destructive">
                  <CircleAlertIcon className="size-4" aria-hidden />
                </span>
                <ErrorLine error={changelogs.error ?? entries.error} />
                <Button
                  variant="outline"
                  className={look.btn}
                  onClick={() => void queryClient.invalidateQueries({ queryKey: keys.changelog(workspaceId) })}
                >
                  <Trans>Try again</Trans>
                </Button>
              </div>
            ) : items.length === 0 ? (
              <div className="mx-auto my-8 flex w-[min(560px,100%)] flex-col items-center gap-3 rounded-[14px] border bg-card px-6 py-10 text-center">
                <span className="grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
                  <SparklesIcon className="size-5" aria-hidden />
                </span>
                <p className="max-w-md text-body text-muted-foreground">
                  <Trans>
                    Tell your customers what is new, improved and fixed, on a public page, in the chat widget and by
                    mail.
                  </Trans>
                </p>
                {missing && (
                  <Button
                    className={look.btnInk}
                    onClick={() => setCreating(missing)}
                    title={t`Create changelog (${createKey})`}
                  >
                    <PlusIcon />
                    <Trans>Create changelog</Trans>
                  </Button>
                )}
              </div>
            ) : (
              <>
                {failed.length > 0 && firstFailed && firstFailedLog && (
                  <div
                    role="alert"
                    className="flex flex-wrap items-center gap-3 rounded-[14px] border border-warning/40 bg-warning/10 px-4 py-3"
                    data-testid="changelog-failed-banner"
                  >
                    <TriangleAlertIcon className="size-[18px] shrink-0 text-warning" />
                    <p className="min-w-48 flex-1 leading-[1.4]">
                      <span className="font-semibold">
                        <Trans>Publishing on schedule failed.</Trans>
                      </span>
                      <br />
                      <span className="text-muted-foreground">
                        {entryTitle(firstFailed.translations, firstFailedLog.default_locale)}
                        {": "}
                        {firstFailed.schedule_failed && failureText[firstFailed.schedule_failed.reason]}{" "}
                        <Trans>Publish it now or schedule it again.</Trans>
                      </span>
                    </p>
                    <Button
                      variant="outline"
                      className={look.btnSm}
                      onClick={() =>
                        navigate(entryLink(firstFailed.changelog_id, firstFailed.id, firstFailedLog.default_locale))
                      }
                    >
                      <Trans>Review</Trans>
                    </Button>
                  </div>
                )}
                {rows.length === 0 ? (
                  <div
                    className="flex flex-col items-center gap-2 rounded-[14px] border bg-card px-6 py-10 text-center"
                    data-testid="changelog-entries-empty"
                  >
                    <span className="mb-1 grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
                      <SparklesIcon className="size-5" aria-hidden />
                    </span>
                    <p className="font-semibold">
                      {filtering ? <Trans>No entry matches.</Trans> : <Trans>Nothing here.</Trans>}
                    </p>
                    <p className="max-w-sm text-body text-faint">
                      {filtering ? <Trans>Try another word, label or language.</Trans> : emptyText}
                    </p>
                    {filtering && (
                      <Button variant="outline" className={look.btnSm} onClick={reset}>
                        <Trans>Clear filters</Trans>
                      </Button>
                    )}
                  </div>
                ) : (
                  <div ref={list} className="flex flex-col gap-3">
                    {months.map((m) => (
                      <section key={m.key} className="flex flex-col gap-2.5">
                        <h2 className="px-0.5 pt-2 text-[11px] font-semibold tracking-[0.05em] text-faint uppercase">
                          {m.name}
                        </h2>
                        <ul className="flex flex-col gap-2.5" aria-label={m.name}>
                          {m.rows.map(({ e, i }) => {
                            const c = byId.get(e.changelog_id)
                            if (!c) return null
                            return (
                              <EntryRow
                                key={e.id}
                                e={e}
                                changelog={c}
                                inbox={items.length > 1 ? inboxOf(c) : undefined}
                                locales
                                current={i === cursor || (cursor < 0 && e.id === shown?.id)}
                                onPick={() => {
                                  setPicked(e.id)
                                  setCursor(-1)
                                }}
                              />
                            )
                          })}
                        </ul>
                      </section>
                    ))}
                  </div>
                )}
              </>
            )}
          </div>
          {shown && shownLog && (
            <div className="sticky top-6 hidden self-start @min-[1000px]:flex">
              <EntryPreview changelog={shownLog} entry={shown} />
            </div>
          )}
        </div>
      </div>
      {writing && <NewEntryDialog changelog={writing} onClose={() => setWriting(null)} />}
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
    </main>
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
  const raw = params.get("tab")
  const tab: Tab = raw === "settings" && canManage ? "settings" : "entries"
  const setTab = (v: Tab) => navigate(changelogLink(changelogId, v === "entries" ? undefined : v), { replace: true })
  const c = changelog.data

  useHotkeys(
    {
      [CHANGELOG_KEYS.entries]: () => setTab("entries"),
      [CHANGELOG_KEYS.preview]: () => setPreview(true),
      ...(canManage ? { [CHANGELOG_KEYS.settings]: () => setTab("settings") } : {}),
      ...(c?.published ? { [CHANGELOG_KEYS.openPage]: () => window.open(c.url, "_blank", "noopener,noreferrer") } : {}),
      ...(canManage && c ? { [CHANGELOG_KEYS.publish]: () => patch.mutate({ published: !c.published }) } : {}),
    },
    !!c
  )

  if (changelog.isPending) {
    return (
      <div className="flex flex-col gap-4 p-6">
        <Skeleton className="h-8 w-60" />
        <Skeleton className="h-24 w-full rounded-xl" />
      </div>
    )
  }
  if (!c) {
    return (
      <PageFrame title={<Trans>What’s new</Trans>} back={{ to: "/updates", label: t`What’s new` }}>
        <ErrorLine error={changelog.error} className="m-6" />
      </PageFrame>
    )
  }
  const names: Record<Tab, string> = { entries: t`Entries`, settings: t`Settings` }
  const previewKey = keyLabel(CHANGELOG_KEYS.preview).join("")
  const publishKey = keyLabel(CHANGELOG_KEYS.publish).join("")
  const tabs = TABS.filter((v) => v !== "settings" || canManage).map((v) => ({
    key: v,
    to: changelogLink(changelogId, v === "entries" ? undefined : v),
    label: names[v],
    count: v === "entries" ? c.counts.entries : undefined,
    testId: `changelog-tab-${v}`,
  }))
  return (
    <PageFrame
      testId="changelog"
      key={c.id}
      back={{ to: "/updates", label: t`What’s new` }}
      title={
        <span className="flex items-center gap-2.5">
          {inbox && <InboxMark name={inbox.name} color={inbox.branding.color} className="size-6 text-caption" />}
          <span data-testid="changelog-name">{c.name}</span>
        </span>
      }
      description={
        <span data-testid="changelog-published">
          {c.published ? <Trans>Published</Trans> : <Trans>Unpublished, only your team can see it</Trans>}
        </span>
      }
      actions={
        <>
          <FrameButton
            onClick={() => setPreview(true)}
            title={t`Preview (${previewKey})`}
            data-testid="changelog-preview"
          >
            <EyeIcon />
            <Trans>Preview</Trans>
          </FrameButton>
          {c.published && (
            <FrameButton
              render={<a href={c.url} target="_blank" rel="noreferrer noopener" />}
              data-testid="changelog-open-public"
            >
              <Trans>View changelog</Trans>
              <ArrowUpRightIcon className="rtl:-scale-x-100" />
            </FrameButton>
          )}
          {canManage && (
            <FrameButton
              ink={!c.published}
              disabled={patch.isPending}
              onClick={() => patch.mutate({ published: !c.published })}
              title={c.published ? t`Unpublish (${publishKey})` : t`Publish (${publishKey})`}
              data-testid="changelog-publish"
            >
              {c.published ? <Trans>Unpublish</Trans> : <Trans>Publish</Trans>}
            </FrameButton>
          )}
        </>
      }
      notice={<ErrorLine error={patch.error} />}
      tabs={<FrameTabs label={t`Changelog`} items={tabs} current={tab} />}
      bodyClassName={tab === "entries" ? "flex overflow-hidden" : undefined}
    >
      {tab === "entries" && <EntriesTab changelog={c} />}
      {tab === "settings" && canManage && (
        <div className={cn(framePad, "max-w-[1040px]")}>
          <SettingsTab changelog={c} />
        </div>
      )}
      {preview && <PreviewDialog changelog={c} onClose={() => setPreview(false)} />}
    </PageFrame>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import {
  CalendarClockIcon,
  CheckIcon,
  ColumnsIcon,
  EyeIcon,
  ImageIcon,
  LoaderIcon,
  MailIcon,
  MoreHorizontalIcon,
  SendIcon,
  TriangleAlertIcon,
  ExternalLinkIcon,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router"

import { Notice, Segmented, toast, useConfirm } from "@/components/common"
import { Pane, PaneBack } from "@/components/common/Column"
import { alt, ENTRY_KEYS, keyLabel, mod } from "@/components/common/ShortcutSheet"
import { formatDateTime } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import { broadcastLink, useBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { StateBadge } from "@/features/broadcasts/BroadcastParts"
import { MailDialog } from "@/features/changelog/MailDialog"
import {
  changelogLink,
  useChangelog,
  useEntry,
  useEntryMutations,
  useRenderedMarkdown,
  useUploadEntryImage,
} from "@/features/changelog/queries"
import { PreviewDialog } from "@/features/changelog/PreviewDialog"
import { ScheduleDialog } from "@/features/changelog/ScheduleDialog"
import {
  ErrorLine,
  LABEL_TONES,
  LABELS,
  useEntryStateText,
  useFailureText,
  useLabelText,
  useTranslationStateText,
} from "@/features/changelog/text"
import { ImageLibrary } from "@/features/designs/designer/ImageLibrary"
import { EditorToolbar } from "@/features/help/EditorToolbar"
import { format, insertBlock, type Format } from "@/features/help/markdown"
import { OriginalText, PreviewPane } from "@/features/help/Preview"
import { useLocaleName } from "@/features/help/text"
import { Field } from "@/features/settings/ui"
import { useDebounced } from "@/hooks/use-debounced"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useLatest } from "@/hooks/use-latest"
import {
  ApiError,
  type Changelog,
  type ChangelogEntry,
  type ChangelogEntryTranslation,
  type ChangelogLabel,
} from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

type Fields = { title: string; body: string; slug: string }
type PaneName = "write" | "preview" | "original"

const fieldsOf = (x?: ChangelogEntryTranslation): Fields => ({
  title: x?.title ?? "",
  body: x?.body ?? "",
  slug: x?.slug ?? "",
})

const same = (a: Fields, b: Fields) => a.title === b.title && a.body === b.body && a.slug === b.slug

function conflictOf(err: unknown) {
  if (!(err instanceof ApiError) || err.code !== "version_conflict") return null
  return (err.body as { translation?: ChangelogEntryTranslation } | undefined)?.translation ?? null
}

function StateDot({ state }: { state?: ChangelogEntryTranslation["state"] }) {
  return (
    <span
      className={cn(
        "size-1.5 rounded-full",
        state === "published" && "bg-success",
        state === "changed" && "bg-warning",
        state === "draft" && "bg-muted-foreground/60",
        !state && "border border-dashed border-faint"
      )}
      aria-hidden
    />
  )
}

function Sent({ changelog, entry }: { changelog: Changelog; entry: ChangelogEntry }) {
  const broadcasts = useBroadcasts(changelog.inbox_id)
  const mine = (broadcasts.data?.pages.flatMap((p) => p.items) ?? []).filter((b) => b.changelog_entry_id === entry.id)
  if (mine.length === 0) return null
  return (
    <Field label={<Trans>Mailed to subscribers</Trans>}>
      <ul className="flex flex-col divide-y rounded-xl border" data-testid="entry-broadcasts">
        {mine.map((b) => (
          <li key={b.id} className="flex items-center gap-3 px-3 py-2">
            <Link
              to={broadcastLink(b.id)}
              className="min-w-0 flex-1 truncate text-body outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              {b.name}
            </Link>
            <StateBadge state={b.state} />
          </li>
        ))}
      </ul>
    </Field>
  )
}

function Editor({
  changelog,
  entry,
  locale,
  onLocale,
}: {
  changelog: Changelog
  entry: ChangelogEntry
  locale: string
  onLocale: (l: string) => void
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const localeName = useLocaleName()
  const entryState = useEntryStateText()
  const translationState = useTranslationStateText()
  const labelText = useLabelText()
  const failureText = useFailureText()
  const m = useEntryMutations(changelog.id, entry.id)
  const upload = useUploadEntryImage(changelog.id)
  const [confirm, confirmDialog] = useConfirm()
  const tr = entry.translations.find((x) => x.locale === locale)
  const source =
    locale !== changelog.default_locale
      ? entry.translations.find((x) => x.locale === changelog.default_locale)
      : undefined
  const canPublish = changelog.can_publish
  const readOnly = !canPublish && entry.state === "scheduled"
  const [fields, setFields] = useState<Fields>(() => fieldsOf(tr))
  const [saved, setSaved] = useState<Fields>(() => fieldsOf(tr))
  const [base, setBase] = useState(tr?.version ?? 0)
  const [conflict, setConflict] = useState<ChangelogEntryTranslation | null>(null)
  const [saveError, setSaveError] = useState<unknown>(null)
  const [showPreview, setShowPreview] = useState(true)
  const [showSource, setShowSource] = useState(true)
  const [pane, setPane] = useState<PaneName>("write")
  const [scheduling, setScheduling] = useState(false)
  const [mailing, setMailing] = useState(false)
  const [pagePreview, setPagePreview] = useState(false)
  const [picking, setPicking] = useState(false)
  const text = useRef<HTMLTextAreaElement>(null)
  const files = useRef<HTMLInputElement>(null)
  const inflight = useRef<Promise<ChangelogEntryTranslation | null> | null>(null)
  const live = useLatest({ fields, saved, base, conflict, tr })

  const titleOk = fields.title.trim().length >= 3
  const dirty = !same(fields, saved)
  const set = (v: Partial<Fields>) => setFields((f) => ({ ...f, ...v }))

  const adopt = (x: ChangelogEntryTranslation) => {
    setFields(fieldsOf(x))
    setSaved(fieldsOf(x))
    setBase(x.version)
    setConflict(null)
    setSaveError(null)
  }

  const save = async (): Promise<ChangelogEntryTranslation | null> => {
    while (inflight.current) await inflight.current.catch(() => null)
    const { fields: f, saved: s, base: b, conflict: c, tr: current } = live.current
    if (c || readOnly) return null
    if (same(f, s)) return current ?? null
    if (f.title.trim().length < 3) return null
    const run = (async () => {
      try {
        const sent = { ...f, title: f.title.trim() }
        const res = await m.save.mutateAsync({
          locale,
          base_version: b,
          title: sent.title,
          body: sent.body,
          slug: sent.slug || undefined,
        })
        setBase(res.version)
        setSaved({ ...sent, slug: res.slug })
        setFields((now) => (now.slug ? now : { ...now, slug: res.slug }))
        setSaveError(null)
        return res
      } catch (err) {
        const theirs = conflictOf(err)
        if (theirs) setConflict(theirs)
        else setSaveError(err)
        return null
      }
    })()
    inflight.current = run
    try {
      return await run
    } finally {
      inflight.current = null
    }
  }
  const latest = useLatest({ save, adopt, dirty, titleOk })

  useEffect(() => {
    if (!dirty || !titleOk || conflict || readOnly) return
    const id = setTimeout(() => void latest.current.save(), 1500)
    return () => clearTimeout(id)
  }, [fields, dirty, titleOk, conflict, readOnly, latest])

  const version = tr?.version
  useEffect(() => {
    const { saved: s, fields: f, base: b, tr: current } = live.current
    if (!current || current.version <= b) return
    if (!inflight.current && same(f, s)) latest.current.adopt(current)
    else setConflict(current)
  }, [version, live, latest])

  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => {
      if (latest.current.dirty) e.preventDefault()
    }
    const flush = () => {
      if (latest.current.dirty && latest.current.titleOk) void latest.current.save()
    }
    window.addEventListener("beforeunload", warn)
    return () => {
      window.removeEventListener("beforeunload", warn)
      flush()
    }
  }, [latest])

  const switchTo = async (l: string) => {
    if (l === locale) return
    await save()
    if (!same(live.current.fields, live.current.saved) && live.current.fields.title.trim()) return
    onLocale(l)
  }

  const isPublic = entry.state === "published"
  const translationPublic = tr?.state === "published" || tr?.state === "changed"
  const effective = !tr ? "draft" : tr.state === "published" && dirty ? "changed" : tr.state

  const publish = async () => {
    if (readOnly) return
    const res = await save()
    if (!res) return
    if (!canPublish) {
      toast(t`Draft submitted. An owner or admin can publish it.`)
      navigate(changelogLink(changelog.id))
      return
    }
    m.publish.mutate(isPublic ? { locale, base_version: res.version } : {}, {
      onSuccess: () => toast(t`Published`),
      onError: (err) => {
        const theirs = conflictOf(err)
        if (theirs) setConflict(theirs)
      },
    })
  }
  const schedule = async (iso: string) => {
    const res = await save()
    if (dirty && !res) return
    m.schedule.mutate(iso, {
      onSuccess: () => {
        setScheduling(false)
        toast(t`Scheduled`)
      },
    })
  }
  const unpublish = () =>
    confirm({
      title: <Trans>Unpublish this entry?</Trans>,
      description: (
        <Trans>
          The entry disappears from the public page, the feed and the widget. The drafts stay, and you can publish it
          again.
        </Trans>
      ),
      confirm: <Trans>Unpublish</Trans>,
      run: () => m.unpublish.mutate(undefined, { onSuccess: () => toast(t`Unpublished`) }),
    })
  const unschedule = () => m.unschedule.mutate()
  const markCurrent = async () => {
    await save()
    const { saved: s, base: b, conflict: c } = live.current
    if (c || s.title.trim().length < 3) return
    try {
      const res = await m.save.mutateAsync({
        locale,
        base_version: b,
        title: s.title,
        body: s.body,
        slug: s.slug || undefined,
      })
      setBase(res.version)
      setSaved(fieldsOf(res))
    } catch (err) {
      const theirs = conflictOf(err)
      if (theirs) setConflict(theirs)
      else setSaveError(err)
    }
  }
  const removeEntry = () =>
    confirm({
      title: <Trans>Delete this entry?</Trans>,
      description: <Trans>Every translation is deleted. Broadcasts made from it stay.</Trans>,
      confirm: <Trans>Delete entry</Trans>,
      run: () =>
        m.remove.mutate(undefined, {
          onSuccess: () => navigate(changelogLink(changelog.id), { replace: true }),
        }),
    })
  const removeTranslation = () => {
    const name = localeName(locale)
    confirm({
      title: <Trans>Delete the {name} translation?</Trans>,
      description: <Trans>Its draft is deleted. The other languages stay.</Trans>,
      confirm: <Trans>Delete translation</Trans>,
      run: () =>
        m.removeTranslation.mutate(locale, {
          onSuccess: () => {
            toast(t`Translation deleted`)
            onLocale(
              changelog.default_locale === locale
                ? (entry.translations.find((x) => x.locale !== locale)?.locale ?? changelog.default_locale)
                : changelog.default_locale
            )
          },
        }),
    })
  }

  const applyFormat = (f: Format) => text.current && !readOnly && format(text.current, f)
  const addImage = async (file: File) => {
    const el = text.current
    if (!el || readOnly) return
    try {
      const image = await upload.mutateAsync(file)
      insertBlock(el, image.markdown)
    } catch (err) {
      setSaveError(err)
    }
  }
  const pickImage = () => files.current?.click()

  const keys = useLatest({ publish, applyFormat, pickImage, save })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const withMod = (e.metaKey || e.ctrlKey) && !e.altKey
      const inText = e.target === text.current
      if (withMod && !e.shiftKey && e.code === "KeyS") {
        e.preventDefault()
        void keys.current.save()
      } else if (withMod && e.key === "Enter") {
        e.preventDefault()
        void keys.current.publish()
      } else if (withMod && !e.shiftKey && inText && e.code === "KeyB") {
        e.preventDefault()
        keys.current.applyFormat("bold")
      } else if (withMod && !e.shiftKey && inText && e.code === "KeyI") {
        e.preventDefault()
        keys.current.applyFormat("italic")
      } else if (e.altKey && !e.metaKey && !e.ctrlKey) {
        const action: Record<string, () => void> = {
          KeyK: () => keys.current.applyFormat("link"),
          KeyI: () => keys.current.pickImage(),
          KeyP: () => setShowPreview((v) => !v),
          KeyC: () => setShowSource((v) => !v),
        }
        const run = action[e.code]
        if (!run) return
        e.preventDefault()
        run()
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [keys])

  const others = changelog.locales
  const idx = others.indexOf(locale)
  const canMail = canManage && isPublic
  useHotkeys({
    [ENTRY_KEYS.previous]: () => idx > 0 && void switchTo(others[idx - 1]),
    [ENTRY_KEYS.next]: () => idx >= 0 && idx < others.length - 1 && void switchTo(others[idx + 1]),
    ...(canPublish && entry.state !== "published" ? { [ENTRY_KEYS.schedule]: () => setScheduling(true) } : {}),
    ...(canPublish && isPublic ? { [ENTRY_KEYS.unpublish]: unpublish } : {}),
    ...(canPublish && entry.state === "scheduled" ? { [ENTRY_KEYS.unpublish]: unschedule } : {}),
    ...(canMail ? { [ENTRY_KEYS.mail]: () => setMailing(true) } : {}),
    ...(canPublish || entry.state === "draft" ? { [ENTRY_KEYS.remove]: removeEntry } : {}),
  })

  const localeItems = [
    ...changelog.locales,
    ...entry.translations.map((x) => x.locale).filter((l) => !changelog.locales.includes(l)),
  ]
  const localeState = (l: string) => entry.translations.find((x) => x.locale === l)?.state

  const status = m.save.isPending ? (
    <span className="inline-flex items-center gap-1.5">
      <LoaderIcon className="size-3.5 animate-spin" />
      <Trans>Saving…</Trans>
    </span>
  ) : !titleOk && fields.title ? (
    <Trans>The title needs at least 3 characters</Trans>
  ) : dirty ? (
    <Trans>Unsaved changes</Trans>
  ) : tr ? (
    <span className="inline-flex items-center gap-1.5">
      <CheckIcon className="size-3.5" />
      <Trans>Saved</Trans>
    </span>
  ) : null

  const saveKey = `${mod} S`
  const publishKey = `${mod} ↵`
  const canDeleteEntry = canPublish || entry.state === "draft"
  const lockedDefault = entry.state !== "draft" && locale === changelog.default_locale
  const canDeleteTranslation =
    entry.translations.length > 1 && !lockedDefault && (canPublish || !translationPublic) && !!tr && !readOnly
  const labelsLocked = readOnly || (!canPublish && entry.state !== "draft")

  const originalLanguage = localeName(changelog.default_locale)
  const sourceVisible = !!source || locale !== changelog.default_locale
  const columns = (sourceVisible && showSource ? 1 : 0) + 1 + (showPreview ? 1 : 0)
  const hide = (name: PaneName, desktop: boolean) =>
    cn(pane === name ? "block" : "hidden", desktop ? "lg:block" : "lg:hidden")
  const body = useDebounced(fields.body, 700)
  const rendered = useRenderedMarkdown(body)

  const toggleLabel = (l: ChangelogLabel) => {
    const next = entry.labels.includes(l)
      ? entry.labels.filter((x) => x !== l)
      : LABELS.filter((x) => x === l || entry.labels.includes(x))
    if (next.length > 0) m.update.mutate({ labels: next })
  }

  return (
    <main className="min-h-0 flex-1 overflow-y-auto bg-background" data-testid="entry-editor">
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-4 px-6 pt-6 pb-24 phone:px-4 phone:pt-3">
        <PaneBack to={changelogLink(changelog.id)} label={t`What’s new`} always />
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <div className="-my-1 max-w-full overflow-x-auto py-1">
            <Segmented
              value={locale}
              onChange={(l) => void switchTo(l)}
              label={t`Language`}
              items={localeItems.map((l) => ({
                value: l,
                label: (
                  <span className="inline-flex items-center gap-1.5">
                    <StateDot state={localeState(l)} />
                    {localeName(l)}
                  </span>
                ),
                testId: `entry-editor-locale-${l}`,
              }))}
            />
          </div>
          <span className="text-small text-muted-foreground" data-testid="entry-editor-state">
            {entryState[entry.state]}
            {entry.state === "scheduled" && entry.publish_at && ` · ${formatDateTime(entry.publish_at, i18n.locale)}`}
            {isPublic && tr && effective !== "published" && ` · ${translationState[effective]}`}
            {tr?.outdated && (
              <span className="ms-2 inline-flex items-center gap-1 text-warning">
                <TriangleAlertIcon className="size-3.5" />
                <Trans>Out of date</Trans>
              </span>
            )}
          </span>
          <span className="text-small text-faint" role="status" data-testid="entry-editor-saving">
            {status}
          </span>
          <span className="flex-1 phone:hidden" />
          <div className="flex flex-wrap items-center gap-2 phone:w-full">
            <Button
              variant={showPreview ? "secondary" : "outline"}
              size="sm"
              aria-pressed={showPreview}
              onClick={() => {
                setShowPreview((v) => !v)
                setPane("preview")
              }}
              title={t`Preview (${alt} P)`}
              data-testid="entry-toggle-preview"
            >
              <EyeIcon />
              <span className="phone:sr-only">
                <Trans>Preview</Trans>
              </span>
            </Button>
            {sourceVisible && (
              <Button
                variant={showSource ? "secondary" : "outline"}
                size="sm"
                aria-pressed={showSource}
                onClick={() => {
                  setShowSource((v) => !v)
                  setPane("original")
                }}
                title={t`Original text (${alt} C)`}
                data-testid="entry-toggle-original"
              >
                <ColumnsIcon />
                <span className="phone:sr-only">
                  <Trans>Original</Trans>
                </span>
              </Button>
            )}
            {tr?.url && (
              <Button
                variant="outline"
                size="sm"
                render={<a href={tr.url} target="_blank" rel="noreferrer noopener" />}
                data-testid="entry-view"
              >
                <ExternalLinkIcon />
                <span className="phone:sr-only">
                  <Trans>View</Trans>
                </span>
              </Button>
            )}
            {canMail && (
              <Button variant="outline" size="sm" onClick={() => setMailing(true)} data-testid="entry-mail">
                <MailIcon />
                <span className="phone:sr-only">
                  <Trans>Mail this to subscribers</Trans>
                </span>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" aria-label={t`More`} />}
                data-testid="entry-editor-menu"
              >
                <MoreHorizontalIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {!readOnly && (
                  <DropdownMenuItem onClick={() => void keys.current.save()}>
                    <Trans>Save now</Trans>
                    <span className="ms-auto text-caption text-faint">{saveKey}</span>
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onClick={() => setPagePreview(true)}>
                  <Trans>Preview</Trans>
                </DropdownMenuItem>
                {canPublish && entry.state !== "published" && (
                  <DropdownMenuItem onClick={() => setScheduling(true)}>
                    {entry.state === "scheduled" ? <Trans>Change the time</Trans> : <Trans>Schedule</Trans>}
                    <span className="ms-auto text-caption text-faint">{keyLabel(ENTRY_KEYS.schedule).join("")}</span>
                  </DropdownMenuItem>
                )}
                {canPublish && entry.state === "scheduled" && (
                  <DropdownMenuItem onClick={unschedule}>
                    <Trans>Unschedule</Trans>
                    <span className="ms-auto text-caption text-faint">{keyLabel(ENTRY_KEYS.unpublish).join("")}</span>
                  </DropdownMenuItem>
                )}
                {canPublish && isPublic && (
                  <DropdownMenuItem onClick={unpublish}>
                    <Trans>Unpublish</Trans>
                    <span className="ms-auto text-caption text-faint">{keyLabel(ENTRY_KEYS.unpublish).join("")}</span>
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                {canDeleteTranslation && (
                  <DropdownMenuItem variant="destructive" onClick={removeTranslation}>
                    <Trans>Delete this translation</Trans>
                  </DropdownMenuItem>
                )}
                {canDeleteEntry && (
                  <DropdownMenuItem variant="destructive" onClick={removeEntry}>
                    <Trans>Delete entry</Trans>
                    <span className="ms-auto text-caption text-faint">{keyLabel(ENTRY_KEYS.remove).join("")}</span>
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
            {canPublish && entry.state !== "published" && (
              <Button variant="outline" size="sm" onClick={() => setScheduling(true)} data-testid="entry-schedule">
                <CalendarClockIcon />
                <Trans>Schedule</Trans>
              </Button>
            )}
            {!readOnly && (
              <Button
                size="sm"
                className="phone:flex-1"
                disabled={
                  !titleOk || m.publish.isPending || (canPublish && isPublic && tr?.state === "published" && !dirty)
                }
                onClick={() => void publish()}
                title={canPublish ? t`Publish (${publishKey})` : t`Submit draft (${publishKey})`}
                data-testid="entry-publish"
              >
                {m.publish.isPending ? <LoaderIcon className="animate-spin" /> : !canPublish && <SendIcon />}
                {canPublish ? (
                  isPublic && tr?.state === "published" && !dirty ? (
                    <Trans>Published</Trans>
                  ) : (
                    <Trans>Publish</Trans>
                  )
                ) : (
                  <Trans>Submit draft</Trans>
                )}
              </Button>
            )}
          </div>
        </div>
        {readOnly && (
          <Notice tone="warning" icon={CalendarClockIcon} data-testid="entry-readonly">
            <p className="font-medium">
              <Trans>This entry is scheduled. A member who may publish does it.</Trans>
            </p>
          </Notice>
        )}
        {entry.schedule_failed && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="entry-schedule-failed">
            <p className="font-medium">
              <Trans>Publishing on schedule failed.</Trans>
            </p>
            <p>{failureText[entry.schedule_failed.reason]}</p>
            {canPublish && (
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!titleOk || m.publish.isPending}
                  onClick={() => void publish()}
                  data-testid="entry-failed-publish"
                >
                  <Trans>Publish now</Trans>
                </Button>
                <Button size="sm" variant="outline" onClick={() => setScheduling(true)}>
                  <Trans>Schedule again</Trans>
                </Button>
              </div>
            )}
          </Notice>
        )}
        {canPublish && entry.state === "scheduled" && entry.publish_at && (
          <Notice tone="info" icon={CalendarClockIcon} data-testid="entry-scheduled">
            <p className="font-medium">
              <Trans>This entry is scheduled for {formatDateTime(entry.publish_at, i18n.locale)}.</Trans>
            </p>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => setScheduling(true)}>
                <Trans>Change the time</Trans>
              </Button>
              <Button size="sm" variant="outline" disabled={m.unschedule.isPending} onClick={unschedule}>
                <Trans>Unschedule</Trans>
              </Button>
            </div>
          </Notice>
        )}
        {tr?.outdated && !readOnly && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="entry-outdated">
            <p className="font-medium">
              <Trans>The original changed after this translation was saved.</Trans>
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setShowSource(true)
                  setPane("original")
                }}
              >
                <Trans>Show the original</Trans>
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={m.save.isPending || !!conflict}
                onClick={() => void markCurrent()}
              >
                <Trans>Mark up to date</Trans>
              </Button>
            </div>
          </Notice>
        )}
        {conflict && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="entry-conflict">
            <p className="font-medium">
              <Trans>Someone saved this translation while you were editing.</Trans>
            </p>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => adopt(conflict)}>
                <Trans>Use their version</Trans>
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setBase(conflict.version)
                  setConflict(null)
                }}
              >
                <Trans>Keep mine</Trans>
              </Button>
            </div>
          </Notice>
        )}
        <ErrorLine
          error={
            saveError ??
            m.publish.error ??
            m.unpublish.error ??
            m.unschedule.error ??
            m.update.error ??
            m.remove.error ??
            m.removeTranslation.error
          }
        />
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_300px]">
          <div className="flex min-w-0 flex-col gap-4">
            <Input
              value={fields.title}
              maxLength={200}
              readOnly={readOnly}
              onChange={(e) => set({ title: e.target.value })}
              placeholder={t`Title`}
              aria-label={t`Title`}
              dir="auto"
              className="h-12 text-title font-semibold"
              data-testid="entry-editor-title"
            />
            <div className="flex flex-col gap-4">
              <div className="-my-1 overflow-x-auto py-1 lg:hidden">
                <Segmented
                  value={pane}
                  onChange={setPane}
                  label={t`Show`}
                  items={[
                    { value: "write" as const, label: t`Write` },
                    { value: "preview" as const, label: t`Preview` },
                    ...(sourceVisible ? [{ value: "original" as const, label: t`Original` }] : []),
                  ]}
                />
              </div>
              <div
                className="grid gap-4 lg:[grid-template-columns:var(--columns)]"
                style={{ "--columns": `repeat(${columns}, minmax(0, 1fr))` } as React.CSSProperties}
              >
                {sourceVisible && (
                  <div className={hide("original", showSource)}>
                    <OriginalText
                      source={source}
                      language={originalLanguage}
                      empty={<Trans>The main language has no text for this entry yet.</Trans>}
                    />
                  </div>
                )}
                <div className={cn("flex min-w-0 flex-col gap-2", hide("write", true))}>
                  {!readOnly && (
                    <EditorToolbar onFormat={applyFormat} onImage={pickImage} uploading={upload.isPending} />
                  )}
                  <input
                    ref={files}
                    type="file"
                    accept="image/png,image/jpeg,image/gif,image/webp"
                    hidden
                    data-testid="entry-image-input"
                    onChange={(e) => {
                      const file = e.target.files?.[0]
                      e.target.value = ""
                      if (file) void addImage(file)
                    }}
                  />
                  <Textarea
                    ref={text}
                    value={fields.body}
                    readOnly={readOnly}
                    onChange={(e) => set({ body: e.target.value })}
                    placeholder={t`Write what changed in Markdown…`}
                    aria-label={t`Text`}
                    spellCheck
                    dir="auto"
                    className="min-h-[55vh] resize-y bg-card font-mono text-[13.5px] leading-relaxed"
                    onPaste={(e) => {
                      const image = Array.from(e.clipboardData.files).find((f) => f.type.startsWith("image/"))
                      if (!image) return
                      e.preventDefault()
                      void addImage(image)
                    }}
                    onDrop={(e) => {
                      const image = Array.from(e.dataTransfer.files).find((f) => f.type.startsWith("image/"))
                      if (!image) return
                      e.preventDefault()
                      void addImage(image)
                    }}
                    data-testid="entry-editor-body"
                  />
                </div>
                {showPreview && (
                  <div className={hide("preview", showPreview)}>
                    <PreviewPane
                      title={fields.title}
                      html={rendered.data ?? ""}
                      fetching={rendered.isFetching}
                      error={rendered.error}
                      frameTitle={t`Preview of the entry`}
                    />
                  </div>
                )}
              </div>
            </div>
          </div>
          <aside
            className="flex flex-col gap-5 rounded-2xl border bg-card p-4 lg:self-start"
            data-testid="entry-editor-details"
          >
            <Field
              label={<Trans>Labels</Trans>}
              hint={
                labelsLocked && !readOnly ? (
                  <Trans>A member who may publish changes the labels of a published entry.</Trans>
                ) : undefined
              }
            >
              <div className="flex flex-wrap gap-2" role="group" aria-label={t`Labels`}>
                {LABELS.map((l) => (
                  <button
                    key={l}
                    type="button"
                    disabled={labelsLocked || m.update.isPending}
                    aria-pressed={entry.labels.includes(l)}
                    onClick={() => toggleLabel(l)}
                    className={cn(
                      "rounded-lg border px-3 py-1.5 text-body font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60",
                      entry.labels.includes(l) ? LABEL_TONES[l] : "border-border text-muted-foreground"
                    )}
                    data-testid={`entry-label-${l}`}
                  >
                    {labelText[l]}
                  </button>
                ))}
              </div>
            </Field>
            <Field
              label={<Trans>Address ending</Trans>}
              htmlFor="entry-slug"
              hint={<Trans>Part of this translation’s web address. Made from the title when empty.</Trans>}
            >
              <Input
                id="entry-slug"
                value={fields.slug}
                maxLength={100}
                readOnly={readOnly}
                spellCheck={false}
                autoCapitalize="none"
                onChange={(e) => set({ slug: e.target.value.toLowerCase() })}
                data-testid="entry-editor-slug"
              />
            </Field>
            <Field
              label={<Trans>Picture</Trans>}
              hint={<Trans>Shown with the entry on the page, in the widget and in the mail.</Trans>}
            >
              <div className="flex items-center gap-3">
                {entry.image ? (
                  <img
                    src={entry.image.url}
                    alt={entry.image.filename}
                    className="h-16 w-24 rounded-lg border object-cover"
                    data-testid="entry-image"
                  />
                ) : (
                  <span className="grid h-16 w-24 place-items-center rounded-lg border border-dashed text-faint">
                    <ImageIcon className="size-5" />
                  </span>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={labelsLocked}
                    onClick={() => setPicking(true)}
                    data-testid="entry-pick-image"
                  >
                    {entry.image ? <Trans>Change</Trans> : <Trans>Choose a picture</Trans>}
                  </Button>
                  {entry.image && (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={labelsLocked}
                      onClick={() => m.update.mutate({ image_id: null })}
                    >
                      <Trans>Remove</Trans>
                    </Button>
                  )}
                </div>
              </div>
            </Field>
            {canMail && <Sent changelog={changelog} entry={entry} />}
          </aside>
        </div>
      </div>
      <ImageLibrary
        open={picking}
        onOpenChange={setPicking}
        current={entry.image?.id}
        onPick={(image) => {
          setPicking(false)
          m.update.mutate({ image_id: image.id })
        }}
      />
      {scheduling && (
        <ScheduleDialog
          initial={entry.publish_at}
          pending={m.schedule.isPending}
          error={m.schedule.error}
          onSchedule={(iso) => void schedule(iso)}
          onClose={() => {
            m.schedule.reset()
            setScheduling(false)
          }}
        />
      )}
      {pagePreview && (
        <PreviewDialog changelog={changelog} entry={entry.id} locale={locale} onClose={() => setPagePreview(false)} />
      )}
      {mailing && <MailDialog changelog={changelog} entry={entry} locale={locale} onClose={() => setMailing(false)} />}
      {confirmDialog}
    </main>
  )
}

export function EntryEditorPage() {
  const { t } = useLingui()
  const { changelogId = "", entryId = "" } = useParams()
  const [params, setParams] = useSearchParams()
  const changelog = useChangelog(changelogId)
  const entry = useEntry(changelogId, entryId)
  const c = changelog.data
  const e = entry.data
  if (changelog.isPending || entry.isPending) {
    return (
      <Pane wide className="gap-4">
        <Skeleton className="h-8 w-60" />
        <Skeleton className="h-64 w-full rounded-2xl" />
      </Pane>
    )
  }
  if (!c || !e) {
    return (
      <Pane wide>
        <PaneBack to={changelogLink(changelogId)} label={t`What’s new`} always />
        <ErrorLine error={changelog.error ?? entry.error} />
      </Pane>
    )
  }
  const wanted = params.get("locale")
  const known = (l: string | null): l is string =>
    !!l && (c.locales.includes(l) || e.translations.some((x) => x.locale === l))
  const locale = known(wanted)
    ? wanted
    : e.translations.some((x) => x.locale === c.default_locale)
      ? c.default_locale
      : (e.translations[0]?.locale ?? c.default_locale)
  return (
    <Editor
      key={`${e.id}:${locale}`}
      changelog={c}
      entry={e}
      locale={locale}
      onLocale={(l) => setParams({ locale: l }, { replace: true })}
    />
  )
}

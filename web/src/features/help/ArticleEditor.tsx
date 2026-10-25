import { Trans, useLingui } from "@lingui/react/macro"
import {
  CheckIcon,
  ColumnsIcon,
  ExternalLinkIcon,
  EyeIcon,
  HistoryIcon,
  LoaderIcon,
  MoreHorizontalIcon,
  SendIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router"

import { Notice, Segmented, toast, useConfirm } from "@/components/common"
import { Pane, PaneBack } from "@/components/common/Column"
import { alt, HELP_KEYS, keyLabel, mod } from "@/components/common/ShortcutSheet"
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
import { EditorToolbar } from "@/features/help/EditorToolbar"
import { format, insertBlock, type Format } from "@/features/help/markdown"
import { OriginalText, ArticlePreview } from "@/features/help/Preview"
import {
  centerLink,
  useArticleMutations,
  useHelpArticle,
  useHelpCenter,
  useHelpCollections,
  useUploadHelpImage,
} from "@/features/help/queries"
import { Revisions } from "@/features/help/Revisions"
import { collectionName, ErrorLine, useLocaleName, useStateText } from "@/features/help/text"
import { ChoiceSelect, Field } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useLatest } from "@/hooks/use-latest"
import {
  ApiError,
  type HelpArticle,
  type HelpArticleRevision,
  type HelpArticleTranslation,
  type HelpCenter,
} from "@/lib/api"
import { cn } from "@/lib/utils"

type Fields = { title: string; body: string; keywords: string; slug: string }
type PaneName = "write" | "preview" | "original"

const fieldsOf = (x?: HelpArticleTranslation): Fields => ({
  title: x?.title ?? "",
  body: x?.body ?? "",
  keywords: x?.keywords ?? "",
  slug: x?.slug ?? "",
})

const same = (a: Fields, b: Fields) =>
  a.title === b.title && a.body === b.body && a.keywords === b.keywords && a.slug === b.slug

function conflictOf(err: unknown) {
  if (!(err instanceof ApiError) || err.code !== "version_conflict") return null
  return (err.body as { translation?: HelpArticleTranslation } | undefined)?.translation ?? null
}

function StateDot({ state }: { state?: HelpArticleTranslation["state"] }) {
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

function Editor({
  center,
  article,
  locale,
  onLocale,
}: {
  center: HelpCenter
  article: HelpArticle
  locale: string
  onLocale: (l: string) => void
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const localeName = useLocaleName()
  const stateText = useStateText()
  const m = useArticleMutations(center.id, article.id)
  const upload = useUploadHelpImage(center.id)
  const collections = useHelpCollections(center.id).data ?? []
  const [confirm, confirmDialog] = useConfirm()
  const tr = article.translations.find((x) => x.locale === locale)
  const source =
    locale !== center.default_locale ? article.translations.find((x) => x.locale === center.default_locale) : undefined
  const [fields, setFields] = useState<Fields>(() => fieldsOf(tr))
  const [saved, setSaved] = useState<Fields>(() => fieldsOf(tr))
  const [base, setBase] = useState(tr?.version ?? 0)
  const [conflict, setConflict] = useState<HelpArticleTranslation | null>(null)
  const [saveError, setSaveError] = useState<unknown>(null)
  const [paths, setPaths] = useState(article.paths.join("\n"))
  const [showPreview, setShowPreview] = useState(true)
  const [showSource, setShowSource] = useState(true)
  const [pane, setPane] = useState<PaneName>("write")
  const [history, setHistory] = useState(false)
  const text = useRef<HTMLTextAreaElement>(null)
  const files = useRef<HTMLInputElement>(null)
  const inflight = useRef<Promise<HelpArticleTranslation | null> | null>(null)
  const live = useLatest({ fields, saved, base, conflict, tr })

  const titleOk = fields.title.trim().length >= 3
  const dirty = !same(fields, saved)
  const set = (v: Partial<Fields>) => setFields((f) => ({ ...f, ...v }))

  const adopt = (x: HelpArticleTranslation) => {
    setFields(fieldsOf(x))
    setSaved(fieldsOf(x))
    setBase(x.version)
    setConflict(null)
    setSaveError(null)
  }

  const save = async (): Promise<HelpArticleTranslation | null> => {
    while (inflight.current) await inflight.current.catch(() => null)
    const { fields: f, saved: s, base: b, conflict: c, tr: current } = live.current
    if (c) return null
    if (same(f, s)) return current ?? null
    if (f.title.trim().length < 3) return null
    const run = (async () => {
      try {
        const sent = { ...f, title: f.title.trim(), keywords: f.keywords.trim() }
        const res = await m.save.mutateAsync({
          locale,
          base_version: b,
          title: sent.title,
          body: sent.body,
          keywords: sent.keywords,
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
    if (!dirty || !titleOk || conflict) return
    const id = setTimeout(() => void latest.current.save(), 1500)
    return () => clearTimeout(id)
  }, [fields, dirty, titleOk, conflict, latest])

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

  const canPublish = center.can_publish
  const effective = !tr ? "draft" : tr.state === "published" && dirty ? "changed" : tr.state
  const published = tr?.state === "published" || tr?.state === "changed"

  const publish = async () => {
    const res = await save()
    if (!res) return
    if (!canPublish) {
      toast(t`Draft submitted. An owner or admin can publish it.`)
      navigate(centerLink(center.id))
      return
    }
    m.publish.mutate(
      { locale, base_version: res.version },
      {
        onSuccess: () => toast(t`Published`),
        onError: (err) => {
          const theirs = conflictOf(err)
          if (theirs) setConflict(theirs)
        },
      }
    )
  }
  const unpublish = () =>
    confirm({
      title: <Trans>Unpublish this translation?</Trans>,
      description: <Trans>The page stops working for visitors. The draft stays, and you can publish it again.</Trans>,
      confirm: <Trans>Unpublish</Trans>,
      run: () => m.unpublish.mutate(locale, { onSuccess: () => toast(t`Unpublished`) }),
    })
  const removeArticle = () =>
    confirm({
      title: <Trans>Delete this article?</Trans>,
      description: <Trans>Every translation, its revisions and its counters are deleted.</Trans>,
      confirm: <Trans>Delete article</Trans>,
      run: () =>
        m.remove.mutate(undefined, {
          onSuccess: () => {
            toast(t`Article deleted`)
            navigate(centerLink(center.id), { replace: true })
          },
        }),
    })
  const removeTranslation = () => {
    const name = localeName(locale)
    confirm({
      title: <Trans>Delete the {name} translation?</Trans>,
      description: <Trans>Its draft and revisions are deleted. The other languages stay.</Trans>,
      confirm: <Trans>Delete translation</Trans>,
      run: () =>
        m.removeTranslation.mutate(locale, {
          onSuccess: () => {
            toast(t`Translation deleted`)
            onLocale(
              center.default_locale === locale
                ? (article.translations.find((x) => x.locale !== locale)?.locale ?? center.default_locale)
                : center.default_locale
            )
          },
        }),
    })
  }

  const applyFormat = (f: Format) => text.current && format(text.current, f)
  const addImage = async (file: File) => {
    const el = text.current
    if (!el) return
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
          KeyH: () => setHistory(true),
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

  const others = center.locales
  const idx = others.indexOf(locale)
  useHotkeys({
    [HELP_KEYS.previous]: () => idx > 0 && void switchTo(others[idx - 1]),
    [HELP_KEYS.next]: () => idx >= 0 && idx < others.length - 1 && void switchTo(others[idx + 1]),
    ...(canPublish && published ? { [HELP_KEYS.unpublish]: unpublish } : {}),
    [HELP_KEYS.remove]: removeArticle,
  })

  const localeItems = [
    ...center.locales,
    ...article.translations.map((x) => x.locale).filter((l) => !center.locales.includes(l)),
  ]
  const localeState = (l: string) => article.translations.find((x) => x.locale === l)?.state
  const localeLabel = (l: string) => {
    const x = article.translations.find((y) => y.locale === l)
    if (!x) return t`Not written`
    if (x.outdated) return t`Out of date`
    return x.state === "changed" ? t`Changed` : stateText[x.state]
  }

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
  const canDeleteArticle = canPublish || !article.translations.some((x) => x.state !== "draft")
  const canDeleteTranslation = article.translations.length > 1 && (canPublish || !published) && !!tr

  const markUpToDate = async () => {
    const current = await save()
    if (!current) return
    try {
      const res = await m.save.mutateAsync({
        locale,
        base_version: current.version,
        title: current.title,
        body: current.body,
        keywords: current.keywords,
        slug: current.slug || undefined,
      })
      setBase(res.version)
      setSaved(fieldsOf(res))
      setSaveError(null)
    } catch (err) {
      const theirs = conflictOf(err)
      if (theirs) setConflict(theirs)
      else setSaveError(err)
    }
  }

  const originalLanguage = localeName(center.default_locale)
  const sourceVisible = !!source || locale !== center.default_locale
  const columns = (sourceVisible && showSource ? 1 : 0) + 1 + (showPreview ? 1 : 0)
  const hide = (name: PaneName, desktop: boolean) =>
    cn(pane === name ? "block" : "hidden", desktop ? "lg:block" : "lg:hidden")

  return (
    <main className="min-h-0 flex-1 overflow-y-auto bg-surface" data-testid="help-editor">
      <div className="mx-auto flex w-full max-w-[1400px] flex-col gap-4 px-6 pt-6 pb-24 phone:px-4 phone:pt-3">
        <PaneBack to={centerLink(center.id)} label={t`Articles`} always />
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <div className="-my-1 max-w-full overflow-x-auto py-1 phone:basis-full">
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
                    <span
                      className={cn(
                        "text-caption font-normal",
                        article.translations.find((y) => y.locale === l)?.outdated ? "text-warning" : "text-faint"
                      )}
                    >
                      {localeLabel(l)}
                    </span>
                  </span>
                ),
                testId: `help-editor-locale-${l}`,
              }))}
            />
          </div>
          <span className="text-small text-muted-foreground" data-testid="help-editor-state">
            {stateText[effective]}
            {tr?.outdated && (
              <span className="ms-2 inline-flex items-center gap-1 text-warning">
                <TriangleAlertIcon className="size-3.5" />
                <Trans>Out of date</Trans>
              </span>
            )}
          </span>
          <span className="text-small text-faint" role="status" data-testid="help-editor-saving">
            {status}
          </span>
          <span className="flex-1 phone:hidden" />
          <div className="flex items-center gap-2 phone:basis-full phone:justify-end">
            <Button
              variant={showPreview ? "secondary" : "outline"}
              size="sm"
              aria-pressed={showPreview}
              onClick={() => {
                setShowPreview((v) => !v)
                setPane("preview")
              }}
              title={t`Preview (${alt} P)`}
              data-testid="help-toggle-preview"
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
                data-testid="help-toggle-original"
              >
                <ColumnsIcon />
                <span className="phone:sr-only">
                  <Trans>Original</Trans>
                </span>
              </Button>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={!tr}
              onClick={() => setHistory(true)}
              title={t`Revisions (${alt} H)`}
              data-testid="help-history"
            >
              <HistoryIcon />
              <span className="phone:sr-only">
                <Trans>Revisions</Trans>
              </span>
            </Button>
            {tr?.url && (
              <Button
                variant="outline"
                size="sm"
                render={<a href={tr.url} target="_blank" rel="noreferrer noopener" />}
                data-testid="help-view-article"
              >
                <ExternalLinkIcon />
                <span className="phone:sr-only">
                  <Trans>View</Trans>
                </span>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger
                render={<Button variant="ghost" size="icon-sm" aria-label={t`More`} />}
                data-testid="help-editor-menu"
              >
                <MoreHorizontalIcon />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => void keys.current.save()}>
                  <Trans>Save now</Trans>
                  <span className="ms-auto text-caption text-faint">{saveKey}</span>
                </DropdownMenuItem>
                {canPublish && published && (
                  <DropdownMenuItem onClick={unpublish}>
                    <Trans>Unpublish</Trans>
                    <span className="ms-auto text-caption text-faint">{keyLabel(HELP_KEYS.unpublish).join("")}</span>
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                {canDeleteTranslation && (
                  <DropdownMenuItem variant="destructive" onClick={removeTranslation}>
                    <Trans>Delete this translation</Trans>
                  </DropdownMenuItem>
                )}
                {canDeleteArticle && (
                  <DropdownMenuItem variant="destructive" onClick={removeArticle}>
                    <Trans>Delete article</Trans>
                    <span className="ms-auto text-caption text-faint">{keyLabel(HELP_KEYS.remove).join("")}</span>
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              size="sm"
              disabled={!titleOk || m.publish.isPending || (canPublish && tr?.state === "published" && !dirty)}
              onClick={() => void publish()}
              title={canPublish ? t`Publish (${publishKey})` : t`Submit draft (${publishKey})`}
              data-testid="help-publish-article"
            >
              {m.publish.isPending ? <LoaderIcon className="animate-spin" /> : !canPublish && <SendIcon />}
              {canPublish ? (
                tr?.state === "published" && !dirty ? (
                  <Trans>Published</Trans>
                ) : (
                  <Trans>Publish</Trans>
                )
              ) : (
                <Trans>Submit draft</Trans>
              )}
            </Button>
          </div>
        </div>
        {tr?.outdated && source && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="help-outdated">
            <p className="font-medium">
              <Trans>
                The original changed after this translation was written. Compare them, then mark it up to date.
              </Trans>
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setShowSource(true)
                  setPane("original")
                }}
                data-testid="help-show-original"
              >
                <Trans>Show original</Trans>
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={m.save.isPending || !titleOk}
                onClick={() => void markUpToDate()}
                data-testid="help-mark-current"
              >
                <Trans>Mark up to date</Trans>
              </Button>
            </div>
          </Notice>
        )}
        {conflict && (
          <Notice tone="warning" icon={TriangleAlertIcon} data-testid="help-conflict">
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
        <ErrorLine error={saveError ?? m.publish.error ?? m.unpublish.error ?? m.update.error ?? m.remove.error} />
        <Input
          value={fields.title}
          maxLength={200}
          onChange={(e) => set({ title: e.target.value })}
          placeholder={t`Article title`}
          aria-label={t`Title`}
          dir="auto"
          className="h-12 text-title font-semibold"
          data-testid="help-editor-title"
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
                <OriginalText source={source} language={originalLanguage} />
              </div>
            )}
            <div className={cn("flex min-w-0 flex-col gap-2", hide("write", true))}>
              <EditorToolbar onFormat={applyFormat} onImage={pickImage} uploading={upload.isPending} />
              <input
                ref={files}
                type="file"
                accept="image/png,image/jpeg,image/gif,image/webp"
                hidden
                data-testid="help-image-input"
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  e.target.value = ""
                  if (file) void addImage(file)
                }}
              />
              <Textarea
                ref={text}
                value={fields.body}
                onChange={(e) => set({ body: e.target.value })}
                placeholder={t`Write the answer in Markdown…`}
                aria-label={t`Article text`}
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
                data-testid="help-editor-body"
              />
            </div>
            {showPreview && (
              <div className={hide("preview", showPreview)}>
                <ArticlePreview centerId={center.id} title={fields.title} body={fields.body} />
              </div>
            )}
          </div>
        </div>
        <div className="grid gap-5 rounded-[14px] border bg-card p-4 sm:grid-cols-2" data-testid="help-editor-details">
          <Field
            label={<Trans>Address ending</Trans>}
            htmlFor="help-article-slug"
            hint={<Trans>Part of this translation’s web address. Made from the title when empty.</Trans>}
          >
            <Input
              id="help-article-slug"
              value={fields.slug}
              maxLength={100}
              spellCheck={false}
              autoCapitalize="none"
              onChange={(e) => set({ slug: e.target.value.toLowerCase() })}
              data-testid="help-editor-slug"
            />
          </Field>
          <Field
            label={<Trans>Search words</Trans>}
            htmlFor="help-article-keywords"
            hint={<Trans>Other words visitors might type, such as synonyms.</Trans>}
          >
            <Input
              id="help-article-keywords"
              value={fields.keywords}
              maxLength={500}
              dir="auto"
              onChange={(e) => set({ keywords: e.target.value })}
              data-testid="help-editor-keywords"
            />
          </Field>
          <Field label={<Trans>Collection</Trans>} htmlFor="help-article-collection">
            <ChoiceSelect
              id="help-article-collection"
              value={article.collection_id ?? ""}
              onChange={(v) => m.update.mutate({ collection_id: v || null })}
              options={[
                ["", t`No collection`] as const,
                ...collections.map((c) => [c.id, collectionName(c, i18n.locale, center.default_locale)] as const),
              ]}
              label={t`Collection`}
            />
          </Field>
          <Field
            label={<Trans>Suggest on pages</Trans>}
            htmlFor="help-article-paths"
            hint={
              <Trans>
                One path per line, such as /billing/*. The chat widget suggests this article on matching pages.
              </Trans>
            }
          >
            <Textarea
              id="help-article-paths"
              value={paths}
              rows={2}
              spellCheck={false}
              onChange={(e) => setPaths(e.target.value)}
              onBlur={() => {
                const next = paths
                  .split("\n")
                  .map((p) => p.trim())
                  .filter(Boolean)
                if (next.join("\n") !== article.paths.join("\n")) m.update.mutate({ paths: next })
              }}
              className="font-mono text-small"
              data-testid="help-editor-paths"
            />
          </Field>
        </div>
      </div>
      {history && tr && (
        <Revisions
          centerId={center.id}
          articleId={article.id}
          locale={locale}
          restoring={m.restore.isPending}
          error={m.restore.error}
          onClose={() => setHistory(false)}
          onRestore={async (r: HelpArticleRevision) => {
            const current = await save()
            m.restore.mutate(
              { locale, revisionId: r.id, base_version: current?.version ?? live.current.base },
              {
                onSuccess: (restored) => {
                  adopt(restored)
                  setHistory(false)
                  toast(t`Version restored to the draft`)
                },
              }
            )
          }}
        />
      )}
      {confirmDialog}
    </main>
  )
}

export function ArticleEditorPage() {
  const { t } = useLingui()
  const { centerId = "", articleId = "" } = useParams()
  const [params, setParams] = useSearchParams()
  const center = useHelpCenter(centerId)
  const article = useHelpArticle(centerId, articleId)
  const c = center.data
  const a = article.data
  if (center.isPending || article.isPending) {
    return (
      <Pane wide className="gap-4">
        <Skeleton className="h-8 w-60" />
        <Skeleton className="h-64 w-full rounded-[14px]" />
      </Pane>
    )
  }
  if (!c || !a) {
    return (
      <Pane wide>
        <PaneBack to={centerLink(centerId)} label={t`Articles`} always />
        <ErrorLine error={center.error ?? article.error} />
      </Pane>
    )
  }
  const wanted = params.get("locale")
  const known = (l: string | null): l is string =>
    !!l && (c.locales.includes(l) || a.translations.some((x) => x.locale === l))
  const locale = known(wanted)
    ? wanted
    : a.translations.some((x) => x.locale === c.default_locale)
      ? c.default_locale
      : (a.translations[0]?.locale ?? c.default_locale)
  return (
    <Editor
      key={`${a.id}:${locale}`}
      center={c}
      article={a}
      locale={locale}
      onLocale={(l) => setParams({ locale: l }, { replace: true })}
    />
  )
}

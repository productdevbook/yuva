import "@/features/help/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, LoaderIcon, PlusIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { InfoColumn, PageLayout } from "@/components/common"
import {
  FilterMenu,
  FilterRow,
  FilterSearch,
  ListBox,
  ListBoxEmpty,
  ListBoxSkeleton,
  PageTitle,
} from "@/components/common/ListBox"
import { toast } from "@/components/common/Toast"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { ArticlesTab, HelpAside, NewArticleForm, type ArticleState } from "@/features/help/ArticlesTab"
import { DocsNote, usePublishToggle } from "@/features/help/CenterBar"
import { CollectionsTab, type CollectionFormState } from "@/features/help/CollectionsTab"
import { CreateCenterDialog } from "@/features/help/CreateCenterDialog"
import { centerLink, useHelpCenter, useHelpCenters, useHelpCollections, useHelpPreview } from "@/features/help/queries"
import { ReportTab } from "@/features/help/ReportTab"
import { SettingsTab } from "@/features/help/SettingsTab"
import { collectionName, ErrorLine } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { HelpCenter, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

export { ArticleEditorPage } from "@/features/help/ArticleEditor"

const TABS = ["articles", "collections", "report", "settings"] as const
type Tab = (typeof TABS)[number]

function useCenterChoice() {
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes()
  const centers = useHelpCenters()
  const all = (inboxes.data ?? []).map((inbox) => ({
    inbox,
    center: centers.data?.find((c) => c.inbox_id === inbox.id),
  }))
  const groups = all.filter((g) => !inboxId || g.inbox.id === inboxId)
  return { all, groups, pending: inboxes.isPending || centers.isPending, error: centers.error }
}

function useCreateCenter() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [creating, setCreating] = useState<Inbox | null>(null)
  const dialog = creating && (
    <CreateCenterDialog
      inboxId={creating.id}
      inboxName={creating.name}
      onClose={() => setCreating(null)}
      onCreated={(c) => {
        setCreating(null)
        toast(t`Help center created. Write the first article, then publish it.`)
        navigate(centerLink(c.id))
      }}
    />
  )
  return [setCreating, dialog] as const
}

function CreateCenterButtons({ inboxes, onCreate }: { inboxes: Inbox[]; onCreate: (i: Inbox) => void }) {
  return inboxes.map((i) => {
    const product = i.name
    return (
      <Button
        key={i.id}
        variant="ghost"
        size="sm"
        className="h-8"
        onClick={() => onCreate(i)}
        data-testid="help-nav-create"
      >
        <PlusIcon />
        <Trans>New help center for {product}</Trans>
      </Button>
    )
  })
}

export function HelpHome() {
  const { t } = useLingui()
  const { canManage } = useSession()
  const { all, groups, pending, error } = useCenterChoice()
  const [create, dialog] = useCreateCenter()
  const [params] = useSearchParams()
  const first = groups.find((g) => g.center)?.center
  if (first) return <Navigate to={centerLink(first.id, params.get("tab") ?? undefined)} replace />
  const target = groups.find((g) => !g.center) ?? all.find((g) => !g.center)
  return (
    <PageLayout testId="help-home" label={t`Help center`}>
      <section className="flex min-w-0 flex-col" data-testid="help-panel">
        <PageTitle
          title={<Trans>Help center</Trans>}
          sub={<Trans>Articles people read before they write to you</Trans>}
          testId="help-head"
        />
        <ListBox>
          {pending ? (
            <ListBoxSkeleton rows={3} />
          ) : error ? (
            <ListBoxEmpty>
              <ErrorLine error={error} />
            </ListBoxEmpty>
          ) : target ? (
            <ListBoxEmpty
              action={
                canManage ? (
                  <Button onClick={() => create(target.inbox)} data-testid="create-help-center-open">
                    <Trans>Create help center</Trans>
                  </Button>
                ) : (
                  <span className="text-caption" data-testid="help-none">
                    <Trans>No help center yet. Owners and admins create it.</Trans>
                  </span>
                )
              }
            >
              <span className="block font-medium text-foreground">
                <Trans>No help center for {target.inbox.name} yet</Trans>
              </span>
              <Trans>
                Answer common questions once, in writing. Each product gets its own help center with articles,
                collections and a report of what people search for. Pages are hosted by Yuva, so there is nothing to
                install on your site.
              </Trans>
            </ListBoxEmpty>
          ) : (
            <ListBoxEmpty testId="help-empty">
              <Trans>No inbox to show a help center for.</Trans>
            </ListBoxEmpty>
          )}
        </ListBox>
        <div className="mt-6">
          <DocsNote />
        </div>
      </section>
      {dialog}
    </PageLayout>
  )
}

function PreviewDialog({ center, onClose }: { center: HelpCenter; onClose: () => void }) {
  const { t } = useLingui()
  const preview = useHelpPreview(center.id, true)
  const html = preview.data ? `<base target="_blank">${preview.data}` : undefined
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="h-[calc(100svh-2rem)] grid-rows-[auto_1fr] sm:max-w-4xl"
        data-testid="help-preview-dialog"
      >
        <DialogHeader>
          <DialogTitle>
            <Trans>Preview</Trans>
          </DialogTitle>
          <DialogDescription>
            {center.published ? (
              <Trans>The help center as visitors see it now. Drafts are left out.</Trans>
            ) : (
              <Trans>The help center is not published. Only your team sees this.</Trans>
            )}
          </DialogDescription>
        </DialogHeader>
        <div className="relative min-h-0 overflow-hidden rounded-xl border bg-background">
          {preview.isPending && (
            <div className="grid h-full place-items-center text-faint">
              <LoaderIcon className="animate-spin" />
            </div>
          )}
          {preview.error && <ErrorLine error={preview.error} className="p-4" />}
          {html && (
            <iframe
              title={t`Preview of ${center.name}`}
              srcDoc={html}
              sandbox="allow-popups allow-popups-to-escape-sandbox"
              className="size-full bg-white"
              data-testid="help-preview-frame"
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

export function CenterPane() {
  const { t, i18n } = useLingui()
  const { centerId = "" } = useParams()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const center = useHelpCenter(centerId)
  const { canManage } = useSession()
  const [inboxFilter] = useInboxFilter()
  const [create, createDialog] = useCreateCenter()
  const centers = useHelpCenters().data
  const inboxes = (useInboxes().data ?? []).filter(isSupport)
  const collections = useHelpCollections(centerId).data ?? []
  const [preview, setPreview] = useState(false)
  const [creating, setCreating] = useState<{ title: string; collectionId: string } | null>(null)
  const [colForm, setColForm] = useState<CollectionFormState>(params.get("new") ? {} : null)
  const search = useRef<HTMLInputElement>(null)
  const raw = params.get("tab")
  const tab: Tab =
    raw === "drafts"
      ? "articles"
      : TABS.includes(raw as Tab) && (raw !== "settings" || canManage)
        ? (raw as Tab)
        : "articles"
  const q = params.get("q") ?? ""
  const rawState = raw === "drafts" ? "draft" : params.get("state")
  const state: ArticleState = rawState === "published" || rawState === "draft" ? rawState : ""
  const picked = params.get("collection") ?? ""
  const set = (key: string, value: string) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p)
        if (value) next.set(key, value)
        else next.delete(key)
        if (next.get("tab") === "drafts") next.delete("tab")
        return next
      },
      { replace: true }
    )
  const setTab = (v: Tab) => navigate(centerLink(centerId, v === "articles" ? undefined : v), { replace: true })
  const c = center.data
  const listing = tab === "articles"
  const pub = usePublishToggle(centerId, c?.published)
  const togglePublish = pub.toggle
  const busy = !!creating || !!colForm || preview

  useHotkeys(
    {
      [HELP_KEYS.articles]: () => setTab("articles"),
      [HELP_KEYS.collections]: () => setTab("collections"),
      [HELP_KEYS.report]: () => setTab("report"),
      ...(canManage ? { [HELP_KEYS.settings]: () => setTab("settings") } : {}),
      [HELP_KEYS.preview]: () => setPreview(true),
      ...(listing
        ? {
            [HELP_KEYS.newArticle]: () => setCreating({ title: "", collectionId: picked }),
            [HELP_KEYS.search]: () => search.current?.focus(),
            "/": () => search.current?.focus(),
          }
        : {}),
      ...(tab === "collections" ? { [HELP_KEYS.newCollection]: () => setColForm({}) } : {}),
      ...(c?.published ? { [HELP_KEYS.openPage]: () => window.open(c.url, "_blank", "noopener,noreferrer") } : {}),
      ...(canManage && c ? { [HELP_KEYS.publishCenter]: togglePublish } : {}),
    },
    !!c && !busy
  )

  const previewKey = keyLabel(HELP_KEYS.preview).join("")
  const newKey = keyLabel(HELP_KEYS.newArticle).join("")
  const colKey = keyLabel(HELP_KEYS.newCollection).join("")
  const openKey = keyLabel(HELP_KEYS.openPage).join("")
  const publishKey = keyLabel(HELP_KEYS.publishCenter).join("")
  const switched = inboxFilter && c && c.inbox_id !== inboxFilter && centers?.find((x) => x.inbox_id === inboxFilter)
  if (switched) return <Navigate to={centerLink(switched.id)} replace />

  const names: Record<Tab, string> = {
    articles: t`Articles`,
    collections: t`Collections`,
    report: t`Search gaps`,
    settings: t`Settings`,
  }
  if (!c)
    return (
      <PageLayout testId={center.isPending ? undefined : "help-center-error"} label={t`Help center`}>
        <PageTitle title={names[tab]} />
        {center.isPending ? (
          <div className="mt-4 flex flex-col gap-4" aria-busy="true">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        ) : (
          <ListBox>
            <ListBoxEmpty>
              <ErrorLine error={center.error} />
            </ListBoxEmpty>
          </ListBox>
        )}
      </PageLayout>
    )

  const others = (centers ?? []).filter((x) => x.id !== c.id && (!inboxFilter || x.inbox_id === inboxFilter))
  const missing = canManage && centers ? inboxes.filter((i) => !centers.some((x) => x.inbox_id === i.id)) : []
  const anyFilter = !!(q || state || picked)
  const clearFilters = () => setParams(new URLSearchParams(), { replace: true })
  const prefer = [i18n.locale, c.default_locale]
  const centerName = c.name
  return (
    <PageLayout
      testId="help-center"
      label={t`Help center`}
      aside={
        <InfoColumn label={t`Help center`}>
          <HelpAside center={c} onWrite={(title) => setCreating({ title, collectionId: picked })} />
        </InfoColumn>
      }
    >
      <section key={c.id} className="flex min-w-0 flex-col">
        <PageTitle
          title={names[tab]}
          sub={
            <span data-testid="help-published">
              <span data-testid="help-center-name">{centerName}</span>
              {" · "}
              {c.published ? <Trans>Published</Trans> : <Trans>Unpublished</Trans>}
            </span>
          }
          actions={
            <>
              <Button
                variant="outline"
                onClick={() => setPreview(true)}
                title={t`Preview (${previewKey})`}
                data-testid="help-preview"
              >
                <Trans>Preview</Trans>
              </Button>
              {c.published && (
                <Button
                  variant="outline"
                  render={<a href={c.url} target="_blank" rel="noreferrer noopener" />}
                  title={t`View site (${openKey})`}
                  data-testid="help-open-public"
                >
                  <Trans>View site</Trans>
                  <ArrowUpRightIcon className="rtl:-scale-x-100" />
                </Button>
              )}
              {canManage && (
                <Button
                  variant="outline"
                  disabled={pub.pending}
                  onClick={togglePublish}
                  title={c.published ? t`Unpublish (${publishKey})` : t`Publish (${publishKey})`}
                  data-testid="help-publish"
                >
                  {c.published ? <Trans>Unpublish</Trans> : <Trans>Publish</Trans>}
                </Button>
              )}
              {listing && (
                <Button
                  onClick={() => setCreating({ title: "", collectionId: picked })}
                  disabled={!!creating}
                  title={t`New article (${newKey})`}
                  data-testid="new-article"
                >
                  <PlusIcon />
                  <Trans>New article</Trans>
                </Button>
              )}
              {tab === "collections" && (
                <Button
                  onClick={() => setColForm({})}
                  disabled={!!colForm}
                  title={t`New collection (${colKey})`}
                  data-testid="new-collection"
                >
                  <PlusIcon />
                  <Trans>New collection</Trans>
                </Button>
              )}
            </>
          }
          testId="help-head"
        />
        <ErrorLine error={pub.error} className="mt-3" />
        {listing && (
          <>
            {creating && (
              <NewArticleForm
                key={creating.title}
                center={c}
                collections={collections}
                collectionId={creating.collectionId}
                initialTitle={creating.title}
                onClose={() => setCreating(null)}
              />
            )}
            <FilterRow testId="help-filters">
              <FilterSearch
                inputRef={search}
                value={q}
                onChange={(v) => set("q", v)}
                placeholder={t`Search articles…`}
                label={t`Search titles`}
                testId="help-article-search"
              />
              {others.length > 0 && (
                <FilterMenu
                  value=""
                  onChange={(id) => id && navigate(centerLink(id))}
                  options={others.map((x) => ({ value: x.id, label: x.name }))}
                  anyLabel={c.name}
                  label={t`Help center`}
                  testId="help-center-picker"
                />
              )}
              <FilterMenu
                value={state}
                onChange={(v) => set("state", v)}
                options={[
                  { value: "published", label: t`Published` },
                  { value: "draft", label: t`Drafts` },
                ]}
                anyLabel={t`Every status`}
                label={t`Filter by status`}
                testId="help-state"
              />
              {collections.length > 0 && (
                <FilterMenu
                  value={picked}
                  onChange={(v) => set("collection", v)}
                  options={collections.map((x) => ({ value: x.id, label: collectionName(x, ...prefer) }))}
                  anyLabel={t`Every collection`}
                  label={t`Filter by collection`}
                  testId="help-collection"
                />
              )}
              {anyFilter && (
                <Button variant="ghost" size="sm" className="h-8" onClick={clearFilters} data-testid="help-clear">
                  <Trans>Clear filters</Trans>
                </Button>
              )}
            </FilterRow>
            <ArticlesTab
              key={c.id}
              center={c}
              state={state}
              collection={picked}
              q={q}
              active={!busy}
              onClear={anyFilter ? clearFilters : undefined}
            />
          </>
        )}
        {tab === "collections" && <CollectionsTab center={c} form={colForm} onForm={setColForm} />}
        {tab === "report" && (
          <div className="mt-4">
            <ReportTab center={c} />
          </div>
        )}
        {tab === "settings" && canManage && missing.length > 0 && (
          <FilterRow testId="help-create-centers">
            <CreateCenterButtons inboxes={missing} onCreate={create} />
          </FilterRow>
        )}
        {tab === "settings" && canManage && (
          <div className="mt-4 max-w-[1040px]">
            <SettingsTab center={c} />
          </div>
        )}
        <div className="mt-8">
          <DocsNote />
        </div>
      </section>
      {createDialog}
      {pub.dialog}
      {preview && <PreviewDialog center={c} onClose={() => setPreview(false)} />}
    </PageLayout>
  )
}

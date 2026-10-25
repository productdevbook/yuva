import "@/features/help/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { LifeBuoyIcon, LoaderIcon } from "lucide-react"
import { useRef, useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { PaneEmpty } from "@/components/common/Column"
import { toast } from "@/components/common/Toast"
import { HELP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { ArticlesTab, NewArticleDialog } from "@/features/help/ArticlesTab"
import { CenterBar, DocsNote, ProductChips, usePublishToggle } from "@/features/help/CenterBar"
import { CollectionsTab } from "@/features/help/CollectionsTab"
import { CreateCenterDialog } from "@/features/help/CreateCenterDialog"
import { centerLink, useHelpCenter, useHelpCenters, useHelpCollections, useHelpPreview } from "@/features/help/queries"
import { ReportTab } from "@/features/help/ReportTab"
import { SettingsTab } from "@/features/help/SettingsTab"
import { ErrorLine } from "@/features/help/text"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { HelpCenter, Inbox } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

export { ArticleEditorPage } from "@/features/help/ArticleEditor"

const TABS = ["articles", "drafts", "collections", "report", "settings"] as const
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

export function HelpHome() {
  const phone = useIsPhone()
  const { canManage } = useSession()
  const { all, groups, pending, error } = useCenterChoice()
  const [create, dialog] = useCreateCenter()
  const [picked, setPicked] = useState("")
  if (phone) {
    return (
      <PaneEmpty icon={LifeBuoyIcon} title={<Trans>Help centers</Trans>} testId="help-home">
        <Trans>
          Answer common questions once, in writing. Pick an inbox’s help center to write articles, group them in
          collections and see what customers search for.
        </Trans>
      </PaneEmpty>
    )
  }
  const first = groups.find((g) => g.center)?.center
  if (first) return <Navigate to={centerLink(first.id)} replace />
  const target =
    all.find((g) => g.inbox.id === picked && !g.center) ?? groups.find((g) => !g.center) ?? all.find((g) => !g.center)
  return (
    <main className="flex min-h-0 flex-1 flex-col bg-surface" data-testid="help-home">
      <PageTop
        className="bg-surface"
        title={<Trans>Help center</Trans>}
        sub={<Trans>Articles people read before they write to you</Trans>}
      />
      <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5 phone:px-4">
        <div className="flex max-w-[720px] flex-col gap-4">
          {pending ? (
            <Skeleton className="h-40 w-full rounded-[14px]" />
          ) : error ? (
            <ErrorLine error={error} />
          ) : (
            <>
              <ProductChips
                groups={all}
                currentInboxId={target?.inbox.id}
                canManage={canManage}
                onCreate={(i) => setPicked(i.id)}
              />
              <div className={`${look.card} flex flex-col gap-3 rounded-[14px] p-6`} data-testid="help-panel">
                {target ? (
                  <>
                    <h2 className="text-reading font-semibold">
                      <Trans>No help center for {target.inbox.name} yet</Trans>
                    </h2>
                    <p className="text-muted-foreground">
                      <Trans>
                        Answer common questions once, in writing. Each product gets its own help center with articles,
                        collections and a report of what people search for. Pages are hosted by Yuva, so there is
                        nothing to install on your site.
                      </Trans>
                    </p>
                    <div className="flex items-center gap-3">
                      {canManage ? (
                        <Button
                          className={look.btnInk}
                          onClick={() => create(target.inbox)}
                          data-testid="create-help-center-open"
                        >
                          <Trans>Create help center</Trans>
                        </Button>
                      ) : (
                        <span className="text-caption text-faint" data-testid="help-none">
                          <Trans>No help center yet. Owners and admins create it.</Trans>
                        </span>
                      )}
                    </div>
                  </>
                ) : (
                  <p className="text-center text-faint" data-testid="help-empty">
                    <Trans>No inbox to show a help center for.</Trans>
                  </p>
                )}
              </div>
              <DocsNote />
            </>
          )}
        </div>
      </div>
      {dialog}
    </main>
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
  const { t } = useLingui()
  const { centerId = "" } = useParams()
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const center = useHelpCenter(centerId)
  const { canManage } = useSession()
  const { all } = useCenterChoice()
  const [inboxFilter] = useInboxFilter()
  const [create, createDialog] = useCreateCenter()
  const centers = useHelpCenters().data
  const collections = useHelpCollections(centerId).data ?? []
  const [preview, setPreview] = useState(false)
  const [q, setQ] = useState("")
  const [creating, setCreating] = useState<{ title: string; collectionId: string } | null>(null)
  const search = useRef<HTMLInputElement>(null)
  const raw = params.get("tab")
  const tab: Tab = TABS.includes(raw as Tab) && (raw !== "settings" || canManage) ? (raw as Tab) : "articles"
  const setTab = (v: Tab) => navigate(centerLink(centerId, v === "articles" ? undefined : v), { replace: true })
  const c = center.data
  const listing = tab === "articles" || tab === "drafts"
  const pub = usePublishToggle(centerId, c?.published)
  const togglePublish = pub.toggle

  useHotkeys(
    {
      [HELP_KEYS.articles]: () => setTab("articles"),
      [HELP_KEYS.collections]: () => setTab("collections"),
      [HELP_KEYS.report]: () => setTab("report"),
      ...(canManage ? { [HELP_KEYS.settings]: () => setTab("settings") } : {}),
      [HELP_KEYS.preview]: () => setPreview(true),
      ...(listing
        ? {
            [HELP_KEYS.newArticle]: () => setCreating({ title: "", collectionId: "" }),
            [HELP_KEYS.search]: () => search.current?.focus(),
          }
        : {}),
      ...(c?.published ? { [HELP_KEYS.openPage]: () => window.open(c.url, "_blank", "noopener,noreferrer") } : {}),
      ...(canManage && c ? { [HELP_KEYS.publishCenter]: togglePublish } : {}),
    },
    !!c
  )

  const switched = inboxFilter && c && c.inbox_id !== inboxFilter && centers?.find((x) => x.inbox_id === inboxFilter)
  if (switched) return <Navigate to={centerLink(switched.id)} replace />

  if (center.isPending) {
    return (
      <main className="flex flex-1 flex-col gap-4 bg-surface px-6 py-5 phone:px-4" aria-busy="true">
        <Skeleton className="h-8 w-60" />
        <div className="grid grid-cols-[220px_minmax(0,1fr)_290px] gap-4 phone:grid-cols-1">
          <Skeleton className="h-64 rounded-xl" />
          <Skeleton className="h-[480px] rounded-[14px]" />
          <Skeleton className="h-72 rounded-[14px] phone:hidden" />
        </div>
      </main>
    )
  }
  if (!c) {
    return (
      <main
        className="flex flex-1 flex-col items-center gap-2.5 bg-surface px-6 py-12 text-center"
        data-testid="help-center-error"
      >
        <span
          className="grid size-8 place-items-center rounded-full bg-destructive/10 text-body font-bold text-destructive"
          aria-hidden
        >
          !
        </span>
        <ErrorLine error={center.error} />
      </main>
    )
  }
  const names: Record<Tab, string> = {
    articles: t`Articles`,
    drafts: t`Drafts`,
    collections: t`Collections`,
    report: t`Report`,
    settings: t`Settings`,
  }
  const keys: Partial<Record<Tab, string>> = {
    articles: HELP_KEYS.articles,
    collections: HELP_KEYS.collections,
    report: HELP_KEYS.report,
    settings: HELP_KEYS.settings,
  }
  const previewKey = keyLabel(HELP_KEYS.preview).join("")
  const newKey = keyLabel(HELP_KEYS.newArticle).join("")
  return (
    <main className="flex min-h-0 flex-1 flex-col bg-surface" data-testid="help-center" key={c.id}>
      <PageTop
        className="bg-surface"
        title={<Trans>Help center</Trans>}
        sub={<Trans>Articles people read before they write to you</Trans>}
        actions={
          <>
            <Button
              variant="outline"
              className={look.btn}
              onClick={() => setPreview(true)}
              title={t`Preview (${previewKey})`}
              data-testid="help-preview"
            >
              <Trans>Preview</Trans>
            </Button>
            <Button
              className={look.btnInk}
              onClick={() => setCreating({ title: "", collectionId: "" })}
              title={t`New article (${newKey})`}
              data-testid="new-article"
            >
              <Trans>New article</Trans>
            </Button>
          </>
        }
        tabs={
          <FrameTabs<Tab>
            label={t`Help center`}
            value={tab}
            onChange={setTab}
            items={TABS.filter((v) => v !== "settings" || canManage).map((v) => ({
              key: v,
              label: names[v],
              title: keys[v] ? `${names[v]} (${keyLabel(keys[v]).join("")})` : undefined,
              testId: `help-tab-${v}`,
            }))}
          />
        }
      />
      <CenterBar
        center={c}
        groups={all}
        canManage={canManage}
        busy={pub.pending}
        onCreate={create}
        onTogglePublish={togglePublish}
      />
      <ErrorLine error={pub.error} className="px-6 pt-2" />
      <div className="@container min-h-0 flex-1 overflow-y-auto px-6 pt-4 pb-12 phone:px-4">
        <div className="max-w-[1240px]">
          {listing && (
            <ArticlesTab
              key={tab}
              center={c}
              drafts={tab === "drafts"}
              q={q}
              onQ={setQ}
              search={search}
              onWrite={(title, collectionId) => setCreating({ title, collectionId })}
            />
          )}
          {tab === "collections" && <CollectionsTab center={c} />}
          {tab === "report" && <ReportTab center={c} />}
          {tab === "settings" && canManage && <SettingsTab center={c} />}
        </div>
        <div className="mt-6">
          <DocsNote />
        </div>
      </div>
      {createDialog}
      {pub.dialog}
      {preview && <PreviewDialog center={c} onClose={() => setPreview(false)} />}
      {creating && (
        <NewArticleDialog
          center={c}
          collections={collections}
          collectionId={creating.collectionId}
          initialTitle={creating.title}
          onClose={() => setCreating(null)}
        />
      )}
    </main>
  )
}

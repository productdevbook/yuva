import { Trans, useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"
import { useState } from "react"
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router"

import { DOCS_KEYS } from "@/components/common/ShortcutSheet"
import { Segmented } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Chips, useViewText } from "@/features/docs/discussions/DiscussionsPane"
import { Thread } from "@/features/docs/discussions/ThreadPane"
import { THREAD_SORTS, THREAD_VIEWS, useThreadSort, useThreadView } from "@/features/docs/discussions/queries"
import { QueueList } from "@/features/docs/discussions/QueuePane"
import { DiscussionsTab } from "@/features/docs/DiscussionsTab"
import { FromDocsDialog } from "@/features/docs/FromDocs"
import { InstallDialog } from "@/features/docs/InstallDialog"
import { PagesTab } from "@/features/docs/PagesTab"
import { useDocsDays, useDocsSummary } from "@/features/docs/queries"
import { DocsSitePicker, DocsSort, DocsWindow, usePickedSite } from "@/features/docs/Toolbar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { ideaLink } from "@/features/roadmap/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Discussion, DiscussionTeamSort } from "@/lib/api"

export type DocsTabKey = "pages" | "discussions" | "queue"

function ThreadSheet({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useLingui()
  return (
    <>
      <button
        type="button"
        aria-label={t`Close`}
        className="absolute inset-0 z-20 cursor-default bg-foreground/25"
        onClick={onClose}
        data-testid="docs-sheet-backdrop"
      />
      <aside
        role="region"
        aria-label={t`Discussion`}
        className="absolute inset-y-0 end-0 z-30 flex w-[min(640px,100%)] flex-col border-s bg-card shadow-[0_0_60px_-20px_rgb(15_23_42/0.4)]"
        data-testid="docs-sheet"
      >
        <div className="flex shrink-0 justify-end px-3 pt-2">
          <Button variant="ghost" size="icon-sm" onClick={onClose} aria-label={t`Close`} data-testid="docs-sheet-close">
            <XIcon />
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-24 phone:px-4">
          <Thread embedded discussionId={id} key={id} />
        </div>
      </aside>
    </>
  )
}

function ThreadFilters({ pending }: { pending: number }) {
  const { t, i18n } = useLingui()
  const [view, setView] = useThreadView()
  const [sort, setSort] = useThreadSort()
  const viewText = useViewText()
  const sortText: Record<DiscussionTeamSort, string> = {
    activity: t`Recent activity`,
    new: t`Newest`,
    top: t`Most votes`,
    votes: t`Most votes`,
  }
  const count = new Intl.NumberFormat(i18n.locale).format(pending)
  return (
    <>
      <Chips
        value={view}
        onChange={setView}
        label={t`Show`}
        testId="thread-view"
        items={THREAD_VIEWS.map((v) => ({
          value: v,
          label:
            v === "pending" && pending > 0 ? (
              <span className="inline-flex items-center gap-1.5">
                {viewText[v]}
                <span className="tabular-nums opacity-70">{count}</span>
              </span>
            ) : (
              viewText[v]
            ),
        }))}
      />
      <Segmented
        value={sort}
        onChange={setSort}
        label={t`Sort`}
        items={THREAD_SORTS.map((s) => ({ value: s, label: sortText[s], testId: `thread-sort-${s}` }))}
      />
    </>
  )
}

export function DocsPage({ tab }: { tab: DocsTabKey }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const { pathname, search } = useLocation()
  const { discussionId } = useParams()
  const [params, setParams] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const site = usePickedSite(inboxId)
  const pending = useDocsSummary(inboxId, site, days).data?.totals.pending_posts ?? 0
  const [dialog, setDialog] = useState<"from" | "install" | null>(null)
  const sheetId = tab === "discussions" ? "" : (params.get("thread") ?? "")
  const sheetHref = (id: string) => {
    const next = new URLSearchParams(search)
    next.set("thread", id)
    return `${pathname}?${next}`
  }
  const closeSheet = () =>
    setParams(
      (p) => {
        p.delete("thread")
        return p
      },
      { replace: true }
    )
  const queueLink = (d: Discussion) => (d.kind === "idea" && d.board_id ? ideaLink(d.board_id, d.id) : sheetHref(d.id))

  useHotkeys(
    {
      [DOCS_KEYS.pages]: () => navigate("/docs"),
      [DOCS_KEYS.discussions]: () => navigate("/docs/discussions"),
      [DOCS_KEYS.queue]: () => navigate("/docs/discussions/queue"),
      [DOCS_KEYS.fromConversation]: () => setDialog("from"),
      [DOCS_KEYS.install]: () => setDialog("install"),
    },
    !dialog
  )
  useHotkeys({ Escape: closeSheet }, !!sheetId && !dialog)

  const detailOnPhone =
    phone && ((tab === "pages" && pathname === "/docs/page") || (tab === "discussions" && !!discussionId))
  const tabs = [
    { key: "pages" as const, to: "/docs", label: t`Pages`, testId: "docs-overview-link" },
    { key: "discussions" as const, to: "/docs/discussions", label: t`Discussions`, testId: "docs-discussions-link" },
    {
      key: "queue" as const,
      to: "/docs/discussions/queue",
      label: t`Waiting for approval`,
      count: pending > 0 ? pending : undefined,
      testId: "docs-queue-link",
    },
  ]

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface" data-testid="docs-home">
      {!detailOnPhone && (
        <>
          <PageTop
            className="bg-surface"
            title={<Trans>Docs</Trans>}
            sub={<Trans>Ratings, questions and discussions from the documentation pages on your own site</Trans>}
            actions={
              <>
                <Button
                  variant="outline"
                  className={look.btn}
                  onClick={() => setDialog("from")}
                  title={`${t`From a conversation`} (${DOCS_KEYS.fromConversation})`}
                  data-testid="docs-from-conversation"
                >
                  <Trans>From a conversation</Trans>
                </Button>
                <Button
                  className={look.btnInk}
                  onClick={() => setDialog("install")}
                  title={`${t`How to add`} (${DOCS_KEYS.install})`}
                  data-testid="docs-install"
                >
                  <Trans>How to add</Trans>
                </Button>
              </>
            }
            tabs={<FrameTabs label={t`Docs`} value={tab} items={tabs} />}
          />
          <div
            className="flex shrink-0 flex-wrap items-center gap-2.5 border-b bg-surface px-6 py-3 phone:px-4"
            data-testid="docs-toolbar"
          >
            <DocsSitePicker />
            {tab === "pages" && (
              <>
                <DocsWindow />
                <DocsSort />
              </>
            )}
            {tab === "discussions" && <ThreadFilters pending={pending} />}
          </div>
        </>
      )}
      <div className="relative flex min-h-0 flex-1 flex-col">
        {tab === "pages" && (
          <PagesTab sheetHref={sheetHref} sheetOpen={!!sheetId} onInstall={() => setDialog("install")} />
        )}
        {tab === "discussions" && <DiscussionsTab discussionId={discussionId} />}
        {tab === "queue" && (
          <div className="min-h-0 flex-1 overflow-y-auto bg-surface">
            <QueueList hrefOf={queueLink} keyboard={!sheetId} />
          </div>
        )}
        {sheetId && <ThreadSheet id={sheetId} onClose={closeSheet} />}
      </div>
      {dialog === "from" && <FromDocsDialog onClose={() => setDialog(null)} />}
      {dialog === "install" && <InstallDialog onClose={() => setDialog(null)} />}
    </main>
  )
}

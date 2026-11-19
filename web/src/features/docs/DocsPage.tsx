import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { XIcon } from "lucide-react"
import { useState } from "react"
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router"

import { PageLayout, PageTop } from "@/components/common"
import { FilterMenu, FilterRow, PageTitle } from "@/components/common/ListBox"
import { DOCS_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { ThreadList, useViewText } from "@/features/docs/discussions/DiscussionsPane"
import { Thread } from "@/features/docs/discussions/ThreadPane"
import {
  THREAD_SORTS,
  THREAD_VIEWS,
  threadLink,
  useDiscussions,
  useThreadSort,
  useThreadView,
  type ThreadView,
} from "@/features/docs/discussions/queries"
import { QueueList } from "@/features/docs/discussions/QueuePane"
import { useThreadFilters } from "@/features/docs/DiscussionsTab"
import { FromDocsDialog } from "@/features/docs/FromDocs"
import { InstallDialog } from "@/features/docs/InstallDialog"
import { PagesTab } from "@/features/docs/PagesTab"
import { useDocsDays, useDocsSummary } from "@/features/docs/queries"
import { DocsSitePicker, DocsSort, DocsWindow, usePickedSite } from "@/features/docs/Toolbar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { ideaLink } from "@/features/roadmap/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Discussion, DiscussionTeamSort } from "@/lib/api"

export type DocsTabKey = "pages" | "discussions" | "queue"

function ThreadSheet({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useLingui()
  return (
    <>
      <button
        type="button"
        aria-label={t`Close`}
        className="fixed inset-0 z-40 cursor-default bg-foreground/25"
        onClick={onClose}
        data-testid="docs-sheet-backdrop"
      />
      <aside
        role="region"
        aria-label={t`Discussion`}
        className="fixed inset-y-0 end-0 z-50 flex w-[min(640px,100%)] flex-col border-s bg-card shadow-[0_0_60px_-20px_rgb(15_23_42/0.4)]"
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
      <FilterMenu
        value={view === "all" ? "" : view}
        onChange={(v) => setView((v || "all") as ThreadView)}
        options={THREAD_VIEWS.filter((v) => v !== "all").map((v) => ({
          value: v,
          label: v === "pending" && pending > 0 ? `${viewText[v]} · ${count}` : viewText[v],
        }))}
        anyLabel={viewText.all}
        label={t`Show`}
        testId="thread-view"
      />
      <FilterMenu
        value={sort === "activity" ? "" : sort}
        onChange={(v) => setSort((v || "activity") as DiscussionTeamSort)}
        options={THREAD_SORTS.filter((s) => s !== "activity").map((s) => ({ value: s, label: sortText[s] }))}
        anyLabel={sortText.activity}
        label={t`Sort`}
        testId="thread-sort"
      />
    </>
  )
}

export function DocsPage({ tab }: { tab: DocsTabKey }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const location = useLocation()
  const { pathname, search } = location
  const { discussionId } = useParams()
  const [params, setParams] = useSearchParams()
  const [inboxId] = useInboxFilter()
  const [days] = useDocsDays()
  const site = usePickedSite(inboxId)
  const pending = useDocsSummary(inboxId, site, days).data?.totals.pending_posts ?? 0
  const filters = useThreadFilters()
  const [view] = useThreadView()
  const threads = useDiscussions(filters).data?.pages.flatMap((p) => p.items) ?? []
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
  const detail = tab === "discussions" && !!discussionId
  const at = threads.findIndex((d) => d.id === discussionId)
  const step = (by: number) => {
    const next = threads[at + by]
    if (next) navigate(threadLink(next.id), { replace: true })
  }
  const fromQueue = (location.state as { from?: string } | null)?.from === "/docs/discussions/queue"
  const back = fromQueue
    ? { to: "/docs/discussions/queue", label: t`Waiting for approval` }
    : { to: "/docs/discussions", label: t`Discussions` }

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
  useHotkeys(
    {
      [DOCS_KEYS.previousThread]: () => step(-1),
      [DOCS_KEYS.nextThread]: () => step(1),
      Escape: () => navigate(back.to),
    },
    detail && !dialog
  )

  const dialogs = (
    <>
      {dialog === "from" && <FromDocsDialog onClose={() => setDialog(null)} />}
      {dialog === "install" && <InstallDialog onClose={() => setDialog(null)} />}
    </>
  )
  if (detail && discussionId)
    return (
      <PageLayout testId="docs-home" label={t`Discussion`}>
        <PageTop back={back} />
        <div className="max-w-[760px]">
          <Thread embedded discussionId={discussionId} key={discussionId} />
        </div>
        {dialogs}
      </PageLayout>
    )

  const titles: Record<DocsTabKey, string> = {
    pages: t`Docs`,
    discussions: t`Discussions`,
    queue: t`Waiting for approval`,
  }
  const subs: Record<DocsTabKey, React.ReactNode> = {
    pages: <Trans>Ratings, questions and discussions from the documentation pages on your own site</Trans>,
    discussions: <Trans>Questions readers ask under your documentation pages</Trans>,
    queue: <Trans>Posts from new readers wait here until a member approves them</Trans>,
  }
  const empty = {
    all: <Trans>No discussions yet. Readers start them under your documentation pages.</Trans>,
    unanswered: <Trans>Every open discussion has an answer.</Trans>,
    pending: <Trans>Nothing is waiting for approval.</Trans>,
    locked: <Trans>No locked discussions.</Trans>,
    hidden: <Trans>No hidden discussions.</Trans>,
  }[view]
  const count = pending
  return (
    <PageLayout testId="docs-home" label={titles[tab]}>
      {tab === "queue" && <PageTop back={{ to: "/docs/discussions", label: t`Discussions` }} />}
      <PageTitle
        title={titles[tab]}
        sub={subs[tab]}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => setDialog("from")}
              title={`${t`From a conversation`} (${DOCS_KEYS.fromConversation})`}
              data-testid="docs-from-conversation"
            >
              <Trans>From a conversation</Trans>
            </Button>
            <Button
              variant={tab === "pages" ? "default" : "outline"}
              onClick={() => setDialog("install")}
              title={`${t`How to add`} (${DOCS_KEYS.install})`}
              data-testid="docs-install"
            >
              <Trans>How to add</Trans>
            </Button>
          </>
        }
        testId="docs-head"
      />
      <FilterRow testId="docs-toolbar">
        <DocsSitePicker />
        {tab === "pages" && (
          <>
            <DocsWindow />
            <DocsSort />
          </>
        )}
        {tab === "discussions" && (
          <>
            <ThreadFilters pending={pending} />
            {pending > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="h-8"
                render={<Link to="/docs/discussions/queue" />}
                data-testid="docs-queue-link"
              >
                <Plural value={count} one="# post waiting for approval" other="# posts waiting for approval" />
              </Button>
            )}
          </>
        )}
      </FilterRow>
      {tab === "pages" && (
        <PagesTab sheetHref={sheetHref} sheetOpen={!!sheetId} onInstall={() => setDialog("install")} />
      )}
      {tab === "discussions" && <ThreadList filters={filters} showPage keyboard={!dialog} selectable empty={empty} />}
      {tab === "queue" && <QueueList hrefOf={queueLink} keyboard={!sheetId && !dialog} />}
      {sheetId && <ThreadSheet id={sheetId} onClose={closeSheet} />}
      {dialogs}
    </PageLayout>
  )
}

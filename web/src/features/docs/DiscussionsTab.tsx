import { Trans, useLingui } from "@lingui/react/macro"
import { useLocation, useNavigate } from "react-router"

import { DOCS_KEYS } from "@/components/common/ShortcutSheet"
import { ThreadList } from "@/features/docs/discussions/DiscussionsPane"
import { Thread } from "@/features/docs/discussions/ThreadPane"
import {
  threadLink,
  useDiscussions,
  useThreadSort,
  useThreadView,
  viewFilters,
} from "@/features/docs/discussions/queries"
import { usePickedSite } from "@/features/docs/Toolbar"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { DiscussionFilters } from "@/lib/keys"

export function useThreadFilters(): DiscussionFilters {
  const [inboxId] = useInboxFilter()
  const site = usePickedSite(inboxId)
  const [view] = useThreadView()
  const [sort] = useThreadSort()
  return { inbox_id: inboxId || undefined, site: site || undefined, sort, ...viewFilters(view) }
}

export function DiscussionsTab({ discussionId }: { discussionId?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const phone = useIsPhone()
  const location = useLocation()
  const [view] = useThreadView()
  const filters = useThreadFilters()
  const items = useDiscussions(filters).data?.pages.flatMap((p) => p.items) ?? []
  const at = items.findIndex((d) => d.id === discussionId)
  const step = (by: number) => {
    const next = items[at + by]
    if (next) navigate(threadLink(next.id), { replace: true })
  }
  useHotkeys(
    {
      [DOCS_KEYS.previousThread]: () => step(-1),
      [DOCS_KEYS.nextThread]: () => step(1),
      Escape: () => navigate("/docs/discussions"),
    },
    !!discussionId
  )
  const empty = {
    all: <Trans>No discussions yet. Readers start them under your documentation pages.</Trans>,
    unanswered: <Trans>Every open discussion has an answer.</Trans>,
    pending: <Trans>Nothing is waiting for approval.</Trans>,
    locked: <Trans>No locked discussions.</Trans>,
    hidden: <Trans>No hidden discussions.</Trans>,
  }[view]
  const fromQueue = (location.state as { from?: string } | null)?.from === "/docs/discussions/queue"
  const back = fromQueue
    ? { to: "/docs/discussions/queue", label: t`Waiting for approval` }
    : { to: "/docs/discussions", label: t`Discussions` }

  if (phone) {
    return discussionId ? (
      <div className="min-h-0 flex-1 overflow-y-auto px-4 pt-4 pb-24">
        <Thread embedded discussionId={discussionId} back={back} key={discussionId} />
      </div>
    ) : (
      <div className="min-h-0 flex-1 overflow-y-auto">
        <ThreadList filters={filters} showPage bare keyboard selectable empty={empty} />
      </div>
    )
  }
  return (
    <div className="flex min-h-0 flex-1">
      <div className="min-h-0 w-[420px] shrink-0 overflow-y-auto border-e">
        <ThreadList
          filters={filters}
          showPage
          bare
          keyboard={!discussionId}
          currentId={discussionId}
          selectable
          empty={empty}
        />
      </div>
      <div className="min-w-0 flex-1 overflow-y-auto">
        {discussionId ? (
          <div className="mx-auto w-full max-w-[760px] px-6 pt-5 pb-24">
            <Thread embedded discussionId={discussionId} key={discussionId} />
          </div>
        ) : (
          <p className="px-6 py-12 text-center text-body text-faint">
            <Trans>Pick a discussion to read and answer it.</Trans>
          </p>
        )}
      </div>
    </div>
  )
}

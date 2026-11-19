import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { MessageSquareIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { boxRowClass, ListBox, ListBoxEmpty, ListBoxSkeleton } from "@/components/common/ListBox"
import { Button } from "@/components/ui/button"
import { boardLink, ideaLink, useScopeIdeas } from "@/features/roadmap/queries"
import { STAGE_DOT } from "@/features/roadmap/stages"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { IdeaStage, Inbox, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"

export function stageTo(all: boolean, boardId: string | undefined, stage?: IdeaStage) {
  const base = all || !boardId ? "/board" : boardLink(boardId)
  return stage ? `${base}${base.includes("?") ? "&" : "?"}stage=${stage}` : base
}

export function StageList({
  boards,
  stage,
  all,
  inboxes,
  active,
}: {
  boards: RoadmapBoard[]
  stage: IdeaStage
  all: boolean
  inboxes: Map<string, Inbox>
  active: boolean
}) {
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const scope = boards.length === 1 ? boards[0].id : undefined
  const ids = new Set(boards.map((b) => b.id))
  const list = useScopeIdeas(scope, { stage, state: "published" })
  const items = (list.data?.pages.flatMap((p) => p.items) ?? []).filter((i) => !!i.board_id && ids.has(i.board_id))
  const inboxOf = new Map(boards.map((b) => [b.id, inboxes.get(b.inbox_id)]))
  const fmt = new Intl.NumberFormat(i18n.locale)
  const [cursor, setCursor] = useState(-1)
  const ref = useRef<HTMLDivElement>(null)
  const open = (n: number) => {
    const idea = items[n]
    if (idea) navigate(ideaLink(idea.board_id ?? "", idea.id, all))
  }
  const down = () => setCursor((n) => Math.min(items.length - 1, n + 1))
  const up = () => setCursor((n) => Math.max(0, n - 1))
  useHotkeys(
    { j: down, k: up, ArrowDown: down, ArrowUp: up, ...(cursor >= 0 ? { Enter: () => open(cursor) } : {}) },
    active
  )
  useEffect(() => {
    if (cursor >= 0)
      ref.current?.querySelectorAll("[data-testid=idea-row]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])
  if (list.isPending)
    return (
      <ListBox>
        <ListBoxSkeleton rows={3} />
      </ListBox>
    )
  if (items.length === 0)
    return (
      <ListBox>
        <ListBoxEmpty testId="stage-empty">
          <ErrorLine error={list.error} />
          {stage === "ideas" ? <Trans>No ideas yet. New ones land here.</Trans> : <Trans>Nothing here yet.</Trans>}
        </ListBoxEmpty>
      </ListBox>
    )
  return (
    <div ref={ref}>
      <ListBox testId="stage-column">
        {items.map((idea, n) => {
          const votes = fmt.format(idea.votes)
          const linked = idea.linked_count ?? 0
          const inbox = boards.length > 1 && idea.board_id ? inboxOf.get(idea.board_id) : undefined
          return (
            <button
              key={idea.id}
              type="button"
              data-row
              onClick={() => open(n)}
              className={cn(
                boxRowClass({ cursor: n === cursor }),
                "w-full items-center gap-3.5 py-2.5 pe-3 text-start text-body text-fg2 outline-none focus-visible:bg-subtle"
              )}
              data-testid="idea-row"
              data-idea-id={idea.id}
            >
              <span
                className="flex h-11 w-12 shrink-0 flex-col items-center justify-center rounded-lg border bg-background font-mono text-[13px] font-medium text-foreground tabular-nums"
                title={t`${votes} votes`}
              >
                <span className="text-[8px] text-muted-foreground" aria-hidden>
                  ▲
                </span>
                {votes}
              </span>
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate font-medium text-foreground">{idea.title}</span>
                <span className="flex min-w-0 items-center gap-2 text-small text-muted-foreground">
                  <span aria-hidden className={cn("size-[7px] shrink-0 rounded-full", STAGE_DOT[stage])} />
                  {inbox && <span className="truncate">{inbox.name}</span>}
                  <span className={cn("inline-flex items-center gap-1", linked > 0 && "text-brand")}>
                    <MessageSquareIcon className="size-3.5" aria-hidden />
                    <Plural value={linked} one="# feedback" other="# feedback" />
                  </span>
                </span>
              </span>
            </button>
          )
        })}
      </ListBox>
      {list.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => void list.fetchNextPage()}
          disabled={list.isFetchingNextPage}
        >
          <Trans>Load more</Trans>
        </Button>
      )}
    </div>
  )
}

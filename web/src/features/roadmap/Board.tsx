import {
  DndContext,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightLeftIcon, CheckIcon, HourglassIcon, MessageSquareIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine, toast } from "@/components/common"
import { ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { IDEA_STAGES, useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { AuthorName } from "@/features/docs/discussions/parts"
import { useModerate } from "@/features/docs/discussions/queries"
import { Ago, look } from "@/features/inbox/PageFrame"
import { MoveDialog } from "@/features/roadmap/MoveDialog"
import { ideaLink, useScopeIdeas } from "@/features/roadmap/queries"
import { STAGE_DOT } from "@/features/roadmap/stages"
import { ProductTag } from "@/features/status/PageFrame"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone } from "@/hooks/use-media-query"
import type { Discussion, Inbox, IdeaStage, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

type Move = { idea: Discussion; to: IdeaStage }

function useScopeLists(scope?: string) {
  const f = (stage: IdeaStage) => ({ stage, state: "published" as const })
  return {
    ideas: useScopeIdeas(scope, f("ideas")),
    planned: useScopeIdeas(scope, f("planned")),
    in_progress: useScopeIdeas(scope, f("in_progress")),
    shipped: useScopeIdeas(scope, f("shipped")),
    closed: useScopeIdeas(scope, f("closed")),
  }
}

function Card({
  idea,
  inbox,
  current,
  cursor,
  all,
  onMove,
}: {
  idea: Discussion
  inbox?: Inbox
  current: boolean
  cursor: boolean
  all: boolean
  onMove: (to: IdeaStage) => void
}) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const { setNodeRef, listeners, isDragging } = useDraggable({ id: idea.id, data: { idea } })
  const fmt = new Intl.NumberFormat(i18n.locale)
  const votes = fmt.format(idea.votes)
  const linked = idea.linked_count ?? 0
  const pending = idea.pending_posts ?? 0
  const title = idea.title
  return (
    <li
      ref={setNodeRef}
      {...listeners}
      className={cn(
        "group relative flex cursor-grab flex-col gap-2 rounded-[11px] border bg-card p-3 shadow-[0_1px_2px_rgb(0_0_0/0.04)] transition-colors hover:border-faint active:cursor-grabbing",
        current &&
          "border-brand shadow-[0_0_0_3px_color-mix(in_srgb,var(--brand)_18%,transparent)] before:absolute before:inset-y-3 before:start-1 before:w-[3px] before:rounded-[3px] before:bg-brand",
        cursor && "ring-2 ring-ring",
        isDragging && "opacity-40"
      )}
      data-testid="idea-card"
      data-idea-id={idea.id}
    >
      <Link
        to={ideaLink(idea.board_id ?? "", idea.id, all)}
        className="line-clamp-3 pe-6 text-[13.5px] leading-[19px] font-semibold break-words outline-none hover:underline focus-visible:underline"
        data-testid="idea-card-link"
      >
        {idea.title}
      </Link>
      <span className="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-caption text-faint tabular-nums">
        <span
          className="inline-flex h-[22px] items-center gap-1 rounded-[7px] bg-muted px-[7px] font-semibold text-foreground"
          title={t`${votes} votes`}
          data-testid="idea-card-votes"
        >
          <span className="text-[8px]" aria-hidden>
            ▲
          </span>
          <span className="sr-only">{t`${votes} votes`}</span>
          <span aria-hidden>{votes}</span>
        </span>
        <span
          className={cn("inline-flex items-center gap-1", linked > 0 && "font-medium text-brand")}
          data-testid="idea-card-linked"
        >
          <MessageSquareIcon className="size-3.5" aria-hidden />
          <Plural value={linked} one="# feedback" other="# feedback" />
        </span>
        {pending > 0 && (
          <span
            className="inline-flex items-center gap-1 font-medium text-warning"
            title={t`${pending} waiting for approval`}
            data-testid="idea-card-pending"
          >
            <HourglassIcon className="size-3.5" />
            {pending}
          </span>
        )}
        {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-xs"
              className="absolute end-1.5 top-1.5 text-faint opacity-0 group-hover:opacity-100 focus-visible:opacity-100 aria-expanded:opacity-100 [@media(hover:none)]:opacity-100"
            />
          }
          aria-label={t`Move ${title} to a stage`}
          title={t`Move to a stage`}
          data-testid="idea-card-stage"
        >
          <ArrowRightLeftIcon />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          {IDEA_STAGES.map((s) => (
            <DropdownMenuItem key={s} disabled={s === idea.stage} onClick={() => onMove(s)}>
              {text.stage[s]}
              {s === idea.stage && <CheckIcon className="ms-auto" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  )
}

type Ideas = ReturnType<typeof useScopeIdeas>

function StageColumn({
  stage,
  count,
  list,
  items,
  inboxOf,
  ideaId,
  cursorId,
  all,
  onMove,
}: {
  stage: IdeaStage
  count: number
  list: Ideas
  items: Discussion[]
  inboxOf: (idea: Discussion) => Inbox | undefined
  ideaId?: string
  cursorId: string | null
  all: boolean
  onMove: (m: Move) => void
}) {
  const text = useEnumText()
  const { setNodeRef, isOver } = useDroppable({ id: stage })
  return (
    <section
      ref={setNodeRef}
      className={cn(
        "flex min-w-64 flex-1 basis-0 snap-start flex-col gap-2 rounded-[14px] border bg-surface p-2.5 transition-colors max-md:min-w-[82%] max-md:flex-none",
        stage === "closed" && "opacity-80",
        isOver && "bg-brand-wash"
      )}
      aria-label={text.stage[stage]}
      data-testid="stage-column"
      data-stage={stage}
    >
      <h2 className="flex items-center gap-2 px-1 pt-0.5 pb-1 text-body font-semibold">
        <span aria-hidden className={cn("size-[7px] shrink-0 rounded-full", STAGE_DOT[stage])} />
        <span className="min-w-0 flex-1 truncate">{text.stage[stage]}</span>
        <span className="text-caption font-normal text-faint tabular-nums" data-testid="stage-count">
          {count}
        </span>
      </h2>
      {list.isPending ? (
        <div className="flex flex-col gap-2" aria-busy>
          <Skeleton className="h-[86px] rounded-[11px]" />
          <Skeleton className="h-16 rounded-[11px]" />
        </div>
      ) : (
        <ul className="flex min-h-12 flex-col gap-2">
          {items.map((idea) => (
            <Card
              key={idea.id}
              idea={idea}
              inbox={inboxOf(idea)}
              current={idea.id === ideaId}
              cursor={idea.id === cursorId}
              all={all}
              onMove={(to) => onMove({ idea, to })}
            />
          ))}
          {items.length === 0 && (
            <li className="px-2 py-4 text-center text-small text-faint" data-testid="stage-empty">
              {stage === "ideas" ? <Trans>No ideas yet. New ones land here.</Trans> : <Trans>Nothing here yet.</Trans>}
            </li>
          )}
        </ul>
      )}
      {list.hasNextPage && (
        <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
          <Trans>Load more</Trans>
        </Button>
      )}
      <ErrorLine error={list.error} className="px-1.5 text-caption" />
    </section>
  )
}

function Pending({
  scope,
  inScope,
  inboxOf,
  all,
}: {
  scope?: string
  inScope: (idea: Discussion) => boolean
  inboxOf: (idea: Discussion) => Inbox | undefined
  all: boolean
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const list = useScopeIdeas(scope, { state: "pending" })
  const moderate = useModerate()
  const phone = useIsPhone()
  const [folded, setFolded] = useState<boolean | null>(null)
  const open = !(folded ?? phone)
  const items = (list.data?.pages.flatMap((p) => p.items) ?? []).filter(inScope)
  if (items.length === 0) return null
  return (
    <section
      className="flex shrink-0 flex-col gap-3 rounded-[14px] border bg-card px-4 py-3"
      aria-label={t`Waiting for approval`}
      data-testid="pending-strip"
    >
      <h2 className="flex flex-wrap items-center gap-3 text-body">
        <span className="grid h-5.5 min-w-8 place-items-center rounded-full bg-brand px-1.5 text-caption font-semibold text-white tabular-nums">
          {items.length}
        </span>
        <span className="min-w-0 flex-1 basis-64">
          <Trans>Readers suggested new ideas. Review them before they show on the public board.</Trans>
        </span>
        <Button
          variant="outline"
          className={look.btnSm}
          aria-expanded={open}
          onClick={() => setFolded(open)}
          data-testid="pending-review"
        >
          {open ? <Trans>Done</Trans> : <Trans>Review</Trans>}
        </Button>
      </h2>
      {open && (
        <ul className="flex gap-2.5 overflow-x-auto pb-1">
          {items.map((idea) => {
            const inbox = inboxOf(idea)
            return (
              <li
                key={idea.id}
                className="flex w-72 shrink-0 flex-col gap-1.5 rounded-[10px] border bg-background p-3"
                data-testid="pending-idea"
              >
                <span className="flex min-w-0 items-center gap-1.5 text-caption text-faint">
                  {inbox && <ProductTag name={inbox.name} color={inbox.branding.color} />}
                  <span className="min-w-0 truncate">
                    <AuthorName author={idea.author} className="font-normal" />
                  </span>
                  <span aria-hidden>·</span>
                  <Ago iso={idea.created_at} className="shrink-0" />
                </span>
                <Link
                  to={ideaLink(idea.board_id ?? "", idea.id, all)}
                  className="line-clamp-2 leading-[1.35] font-medium break-words hover:underline"
                >
                  {idea.title}
                </Link>
                {idea.body && (
                  <p className="line-clamp-3 text-small leading-[1.4] break-words text-muted-foreground">{idea.body}</p>
                )}
                <span className="-mx-1 mt-auto flex gap-1.5 pt-1">
                  <Button
                    size="sm"
                    className="h-[30px] rounded-lg px-3"
                    disabled={moderate.isPending}
                    onClick={() =>
                      moderate.mutate(
                        { postId: idea.opening_post_id, to: "published" },
                        { onSuccess: () => toast(t`Approved`), onError: (e) => toast(errorText(e)) }
                      )
                    }
                    data-testid="pending-approve"
                  >
                    <Trans>Approve</Trans>
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-[30px] rounded-lg px-2.5 font-normal"
                    disabled={moderate.isPending}
                    onClick={() =>
                      moderate.mutate(
                        { postId: idea.opening_post_id, to: "hidden" },
                        { onSuccess: () => toast(t`Hidden`), onError: (e) => toast(errorText(e)) }
                      )
                    }
                    data-testid="pending-hide"
                  >
                    <Trans>Hide</Trans>
                  </Button>
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

export function Board({
  boards,
  ideaId,
  all,
  moving,
  onMoving,
  onMoved,
}: {
  boards: RoadmapBoard[]
  ideaId?: string
  all: boolean
  moving: Move | null
  onMoving: (m: Move | null) => void
  onMoved: (idea: Discussion, stage: IdeaStage) => void
}) {
  const navigate = useNavigate()
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const scope = boards.length === 1 ? boards[0].id : undefined
  const ids = new Set(boards.map((b) => b.id))
  const inScope = (idea: Discussion) => !!idea.board_id && ids.has(idea.board_id)
  const inboxes = new Map((useInboxes().data ?? []).map((i) => [i.id, i]))
  const inboxOfBoard = new Map(boards.map((b) => [b.id, boards.length > 1 ? inboxes.get(b.inbox_id) : undefined]))
  const inboxOf = (idea: Discussion) => (idea.board_id ? inboxOfBoard.get(idea.board_id) : undefined)
  const lists = useScopeLists(scope)
  const count = (stage: IdeaStage) => boards.reduce((n, b) => n + b.counts[stage], 0)
  const stages = IDEA_STAGES.filter((s) => s !== "closed" || count(s) > 0)
  const columns = stages.map((stage) => (lists[stage].data?.pages.flatMap((p) => p.items) ?? []).filter(inScope))
  const [pos, setPos] = useState<{ col: number; row: number } | null>(null)
  const size = (col: number) => columns[col]?.length ?? 0
  const cursorIdea = pos ? columns[pos.col]?.[pos.row] : undefined
  const cursorId = cursorIdea?.id ?? null
  const step = (dc: number, dr: number) => {
    if (!pos) {
      const first = stages.findIndex((_, i) => size(i) > 0)
      if (first >= 0) setPos({ col: first, row: 0 })
      return
    }
    if (dc === 0) {
      const len = size(pos.col)
      if (len > 0) setPos({ col: pos.col, row: Math.min(Math.max(pos.row + dr, 0), len - 1) })
      return
    }
    for (let i = 1; i <= stages.length; i++) {
      const col = (pos.col + dc * i + stages.length * i) % stages.length
      if (size(col) > 0) return setPos({ col, row: Math.min(pos.row, size(col) - 1) })
    }
  }
  useEffect(() => {
    if (cursorId)
      document.querySelector(`[data-idea-id="${cursorId}"]`)?.scrollIntoView({ block: "nearest", inline: "nearest" })
  }, [cursorId])

  const onDragEnd = (e: DragEndEvent) => {
    const idea = e.active.data.current?.idea as Discussion | undefined
    const to = e.over?.id as IdeaStage | undefined
    if (idea && to && IDEA_STAGES.includes(to) && to !== idea.stage) onMoving({ idea, to })
  }

  const down = () => step(0, 1)
  const up = () => step(0, -1)
  const shift = (d: number) => {
    const to = cursorIdea && IDEA_STAGES[IDEA_STAGES.indexOf(cursorIdea.stage ?? "ideas") + d]
    if (cursorIdea && to) onMoving({ idea: cursorIdea, to })
  }
  useHotkeys(
    {
      j: down,
      k: up,
      ArrowDown: down,
      ArrowUp: up,
      [ROADMAP_KEYS.nextColumn]: () => step(1, 0),
      [ROADMAP_KEYS.previousColumn]: () => step(-1, 0),
      ...(cursorIdea
        ? {
            [ROADMAP_KEYS.open]: () => navigate(ideaLink(cursorIdea.board_id ?? "", cursorIdea.id, all)),
            [ROADMAP_KEYS.move]: () => onMoving({ idea: cursorIdea, to: cursorIdea.stage ?? "ideas" }),
            [ROADMAP_KEYS.moveBack]: () => shift(-1),
            [ROADMAP_KEYS.moveForward]: () => shift(1),
          }
        : {}),
    },
    !ideaId && !moving
  )

  return (
    <>
      <div className="@container min-h-0 min-w-0 flex-1 overflow-y-auto">
        <div className="flex min-h-full flex-col gap-4 p-6 @max-3xl:px-4 @max-3xl:pt-4">
          <Pending scope={scope} inScope={inScope} inboxOf={inboxOf} all={all} />
          <DndContext sensors={sensors} onDragEnd={onDragEnd}>
            <div className="flex min-h-0 flex-1 snap-x items-stretch gap-3 overflow-x-auto" data-testid="board-columns">
              {stages.map((stage, i) => (
                <StageColumn
                  key={stage}
                  stage={stage}
                  count={count(stage)}
                  inboxOf={inboxOf}
                  list={lists[stage]}
                  items={columns[i] ?? []}
                  ideaId={ideaId}
                  cursorId={cursorId}
                  all={all}
                  onMove={onMoving}
                />
              ))}
            </div>
          </DndContext>
        </div>
      </div>
      {moving && (
        <MoveDialog
          idea={moving.idea}
          to={moving.to}
          onClose={() => onMoving(null)}
          onMoved={(stage) => {
            onMoving(null)
            onMoved(moving.idea, stage)
          }}
        />
      )}
    </>
  )
}

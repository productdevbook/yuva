import "@/features/roadmap/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, KanbanIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { ErrorLine } from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { useDiscussion } from "@/features/docs/discussions/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { Board } from "@/features/roadmap/Board"
import { BoardBar } from "@/features/roadmap/BoardBar"
import { CreateBoardDialog } from "@/features/roadmap/CreateBoardDialog"
import { IdeaPane } from "@/features/roadmap/IdeaPane"
import { NewIdeaDialog } from "@/features/roadmap/NewIdeaDialog"
import { boardLink, boardOfInbox, ideaLink, useBoards, useUpdateBoard } from "@/features/roadmap/queries"
import { Settings } from "@/features/roadmap/Settings"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useMediaQuery } from "@/hooks/use-media-query"
import type { Discussion, IdeaStage, Inbox, RoadmapBoard } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

type Tab = "board" | "settings"

function PublicBoards({ boards }: { boards: RoadmapBoard[] }) {
  const open = boards.filter((b) => b.public)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="outline" className={look.btn} />} data-testid="board-open-public">
        <Trans>View board</Trans>
        <ArrowUpRightIcon className="rtl:-scale-x-100" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        {open.map((b) => (
          <DropdownMenuItem
            key={b.id}
            render={<a href={b.url} target="_blank" rel="noreferrer noopener" />}
            data-testid="board-open-public-item"
          >
            <span className="flex min-w-0 flex-1 flex-col leading-[1.3]">
              <span className="truncate font-medium">{b.name}</span>
              <span className="truncate text-caption text-faint" dir="ltr">
                {b.url.replace(/^https?:\/\//, "")}
              </span>
            </span>
            <ArrowUpRightIcon className="text-faint rtl:-scale-x-100" />
          </DropdownMenuItem>
        ))}
        {open.length === 0 && (
          <p className="px-2 py-1.5 text-body text-faint">
            <Trans>No board is public yet. Make one public under Settings.</Trans>
          </p>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function RoadmapPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const [search] = useSearchParams()
  const [boardId = "", , ideaId] = (useParams()["*"] ?? "").split("/")
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const inboxById = new Map(inboxes.map((i) => [i.id, i]))
  const boards = useBoards()
  const split = useMediaQuery("(min-width: 1180px)")
  const [creating, setCreating] = useState<Inbox | null>(null)
  const [writing, setWriting] = useState(false)
  const [moving, setMoving] = useState<{ idea: Discussion; to: IdeaStage } | null>(null)

  const scoped = (boards.data ?? []).filter((b) => !inboxId || b.inbox_id === inboxId)
  const shown = (inboxId ? inboxes.filter((i) => i.id === inboxId) : inboxes).filter(isSupport)
  const missing = canManage && boards.data ? shown.filter((i) => !boardOfInbox(boards.data, i.id)) : []
  const firstMissing = missing[0]
  const missingName = firstMissing?.name ?? ""
  const urlBoard = boards.data?.find((b) => b.id === boardId)
  const tab: Tab = canManage && search.get("tab") === "settings" && scoped.length > 0 ? "settings" : "board"
  const all = tab === "board" && (!urlBoard || search.get("scope") === "all")
  const selected = tab === "settings" ? (urlBoard ?? scoped[0]) : all ? undefined : urlBoard
  const patch = useUpdateBoard(selected?.id ?? "")
  const inView = selected ? [selected] : scoped
  const showIdea = tab === "board" && !!ideaId && !!urlBoard
  const phoneIdea = showIdea && !split
  const missingBoard = !!boardId && !boards.isPending && !urlBoard

  const togglePublic = (b: RoadmapBoard) => {
    if (patch.isPending) return
    patch.mutate({ public: !b.public })
  }
  const boardTab = all ? "/board" : boardLink(selected?.id ?? boardId)
  const settingsTab = boardLink(selected?.id ?? scoped[0]?.id ?? boardId, "settings")

  useHotkeys(
    {
      ...(firstMissing ? { [ROADMAP_KEYS.newBoard]: () => setCreating(firstMissing) } : {}),
      [ROADMAP_KEYS.board]: () => navigate(boardTab, { replace: true }),
      ...(canManage && scoped.length > 0
        ? { [ROADMAP_KEYS.settings]: () => navigate(settingsTab, { replace: true }) }
        : {}),
      ...(canManage && selected ? { [ROADMAP_KEYS.publicBoard]: () => togglePublic(selected) } : {}),
      ...(selected?.public && !ideaId
        ? { [ROADMAP_KEYS.openPage]: () => window.open(selected.url, "_blank", "noopener,noreferrer") }
        : {}),
    },
    !creating && !writing && !moving
  )

  if (phoneIdea && urlBoard)
    return (
      <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="roadmap-home">
        <aside className="min-h-0 flex-1 overflow-y-auto px-4 pt-4 pb-24" data-testid="idea-pane">
          <IdeaPane board={urlBoard} ideaId={ideaId} all={search.get("scope") === "all"} key={ideaId} />
        </aside>
      </main>
    )

  return (
    <main className="flex min-h-0 min-w-0 flex-1 flex-col bg-background" data-testid="roadmap-home">
      <PageTop
        title={<Trans>Roadmap</Trans>}
        sub={<Trans>Ideas your customers vote on, with the feedback behind them</Trans>}
        actions={
          scoped.length > 0 && (
            <>
              <PublicBoards boards={scoped} />
              <Button className={look.btnInk} onClick={() => setWriting(true)} data-testid="new-idea">
                <Trans>New idea</Trans>
              </Button>
            </>
          )
        }
        tabs={
          canManage &&
          scoped.length > 0 && (
            <FrameTabs
              label={t`Roadmap`}
              value={tab}
              items={[
                { key: "board", to: boardTab, label: t`Board`, testId: "board-tab-board" },
                { key: "settings", to: settingsTab, label: t`Settings`, testId: "board-tab-settings" },
              ]}
            />
          )
        }
      />
      {boards.isPending ? (
        <div className="flex flex-1 gap-4 p-6">
          <Skeleton className="h-64 flex-1 rounded-xl" />
          <Skeleton className="h-64 flex-1 rounded-xl" />
          <Skeleton className="h-64 flex-1 rounded-xl" />
          <Skeleton className="h-64 flex-1 rounded-xl" />
        </div>
      ) : boards.error ? (
        <ErrorLine error={boards.error} className="m-6" />
      ) : missingBoard ? (
        <div className="flex flex-1 flex-col gap-3 p-6">
          <PaneBack to="/board" label={t`Roadmap`} />
          <p className="text-body text-muted-foreground">
            <Trans>This board no longer exists or you cannot see it.</Trans>
          </p>
        </div>
      ) : scoped.length === 0 ? (
        <div className="m-6 flex flex-col items-center gap-3 self-start rounded-xl border border-dashed px-6 py-12 text-center phone:m-4">
          <KanbanIcon className="size-6 text-faint" />
          <p className="max-w-md text-body text-muted-foreground">
            {firstMissing ? (
              <Trans>
                Show your customers what you are building, and link the feedback behind each idea. Create a board for{" "}
                {missingName} to sort ideas into stages.
              </Trans>
            ) : (
              <Trans>No board yet. Owners and admins create it.</Trans>
            )}
          </p>
          {firstMissing && (
            <Button className={look.btnInk} onClick={() => setCreating(firstMissing)} data-testid="create-board-empty">
              <PlusIcon />
              <Trans>Create board</Trans>
            </Button>
          )}
        </div>
      ) : (
        <>
          <BoardBar
            boards={scoped}
            inboxes={inboxById}
            selected={selected}
            all={all}
            settings={tab === "settings"}
            missing={missing}
            canManage={canManage}
            busy={patch.isPending}
            onCreate={setCreating}
            onTogglePublic={togglePublic}
          />
          <ErrorLine error={patch.error} className="mx-6 mt-2 phone:mx-4" />
          {tab === "settings" && selected ? (
            <div className="min-h-0 flex-1 overflow-y-auto pt-4" data-testid="board-settings-page">
              <Settings board={selected} key={selected.id} />
            </div>
          ) : (
            <div className="flex min-h-0 flex-1">
              <Board
                boards={inView}
                ideaId={showIdea ? ideaId : undefined}
                all={all}
                moving={moving}
                onMoving={setMoving}
                onMoved={(idea, stage) => {
                  if (stage === "shipped" && (idea.linked_count ?? 0) > 0 && !showIdea && idea.board_id)
                    navigate(ideaLink(idea.board_id, idea.id, all), { state: { offerReply: true } })
                }}
              />
              {showIdea && urlBoard && (
                <aside
                  className={cn(
                    "min-h-0 w-[min(520px,45%)] shrink-0 overflow-y-auto border-s bg-background px-6 pt-4 pb-24"
                  )}
                  data-testid="idea-pane"
                >
                  <IdeaPane board={urlBoard} ideaId={ideaId} all={search.get("scope") === "all"} key={ideaId} />
                </aside>
              )}
            </div>
          )}
        </>
      )}
      {writing && (
        <NewIdeaDialog
          boards={scoped}
          initialBoardId={selected?.id ?? urlBoard?.id}
          onClose={() => setWriting(false)}
        />
      )}
      {creating && (
        <CreateBoardDialog
          inboxId={creating.id}
          inboxName={creating.name}
          onClose={() => setCreating(null)}
          onCreated={(b) => {
            setCreating(null)
            navigate(boardLink(b.id, "settings"))
          }}
        />
      )}
    </main>
  )
}

export function IdeaRedirect() {
  const { ideaId = "" } = useParams()
  const idea = useDiscussion(ideaId)
  const d = idea.data?.discussion
  if (d?.board_id) return <Navigate to={ideaLink(d.board_id, d.id)} replace />
  return idea.isPending ? (
    <div className="flex-1 bg-background" />
  ) : (
    <div className="flex flex-1 flex-col bg-background p-6">
      <PaneBack to="/board" label="" />
      <ErrorLine error={idea.error} />
    </div>
  )
}

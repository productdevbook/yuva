import "@/features/roadmap/i18n"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, CircleAlertIcon, KanbanIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import {
  ErrorLine,
  FlatEmpty,
  InfoSection,
  KV,
  PageHeading,
  PageTop as FlatTop,
  topButton,
  topInk,
} from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { keyLabel, ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, IDEA_STAGES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import { useDiscussion } from "@/features/docs/discussions/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { FrameTabs, look, PageTop } from "@/features/inbox/PageFrame"
import { Board } from "@/features/roadmap/Board"
import { BoardBar, openIdeas } from "@/features/roadmap/BoardBar"
import { CreateBoardDialog } from "@/features/roadmap/CreateBoardDialog"
import { IdeaPane } from "@/features/roadmap/IdeaPane"
import { NewIdeaDialog } from "@/features/roadmap/NewIdeaDialog"
import { boardLink, boardOfInbox, ideaLink, useBoards, useUpdateBoard } from "@/features/roadmap/queries"
import { RoadmapLayout, RoadmapTrail, StageList, stageTo } from "@/features/roadmap/RoadmapFrame"
import { Settings } from "@/features/roadmap/Settings"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsPhone, useMediaQuery } from "@/hooks/use-media-query"
import type { Discussion, IdeaStage, Inbox, RoadmapBoard } from "@/lib/api"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { isSupport, useInboxes } from "@/lib/workspace"

type Tab = "board" | "settings"

function PublicBoards({ boards, className }: { boards: RoadmapBoard[]; className?: string }) {
  const open = boards.filter((b) => b.public)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant="outline" className={className ?? look.btn} />}
        data-testid="board-open-public"
      >
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

function BoardInfo({ board }: { board: RoadmapBoard }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <InfoSection title={<Trans>Board</Trans>}>
      <KV
        label={<Trans>Visibility</Trans>}
        value={board.public ? t`Public` : t`Private`}
        tone={board.public ? "ok" : undefined}
      />
      {board.public && (
        <KV label={<Trans>Address</Trans>} value={<span dir="ltr">{board.url.replace(/^https?:\/\//, "")}</span>} />
      )}
      {IDEA_STAGES.map((s) => (
        <KV key={s} label={text.stage[s]} value={fmt.format(board.counts[s])} to={stageTo(false, board.id, s)} />
      ))}
      {board.counts.pending > 0 && (
        <KV label={<Trans>Waiting for approval</Trans>} value={fmt.format(board.counts.pending)} tone="warn" />
      )}
    </InfoSection>
  )
}

function BoardsInfo({ boards }: { boards: RoadmapBoard[] }) {
  const { i18n } = useLingui()
  const fmt = new Intl.NumberFormat(i18n.locale)
  return (
    <InfoSection title={<Trans>Boards</Trans>}>
      {boards.map((b) => (
        <KV key={b.id} label={b.name} value={fmt.format(openIdeas(b))} to={boardLink(b.id)} />
      ))}
    </InfoSection>
  )
}

function IdeaInfo({ board, ideaId }: { board: RoadmapBoard; ideaId: string }) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const idea = useDiscussion(ideaId).data?.discussion
  const fmt = new Intl.NumberFormat(i18n.locale)
  if (!idea) return null
  return (
    <InfoSection title={<Trans>Idea</Trans>}>
      {idea.stage && <KV label={<Trans>Stage</Trans>} value={text.stage[idea.stage]} />}
      <KV label={<Trans>Votes</Trans>} value={fmt.format(idea.votes)} />
      <KV label={<Trans>Feedback</Trans>} value={fmt.format(idea.linked_count ?? 0)} />
      <KV label={<Trans>Board</Trans>} value={board.name} to={boardLink(board.id)} />
      <KV label={<Trans>Created</Trans>} value={formatDateTime(idea.created_at, i18n.locale)} />
    </InfoSection>
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
  const phone = useIsPhone()
  const text = useEnumText()
  const rawStage = search.get("stage") as IdeaStage | null
  const stage = rawStage && IDEA_STAGES.includes(rawStage) ? rawStage : undefined
  const publicKey = keyLabel(ROADMAP_KEYS.publicBoard).join("")
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
      ...(!phone && ideaId ? { Escape: () => navigate(all ? "/board" : boardLink(urlBoard?.id ?? boardId)) } : {}),
      ...(selected?.public && !ideaId
        ? { [ROADMAP_KEYS.openPage]: () => window.open(selected.url, "_blank", "noopener,noreferrer") }
        : {}),
    },
    !creating && !writing && !moving
  )

  if (!phone) {
    const navProps = {
      boards: scoped,
      all,
      selected,
      inboxes: inboxById,
      stage: tab === "board" ? stage : undefined,
      settings: tab === "settings",
      settingsTo: canManage && scoped.length > 0 ? settingsTab : undefined,
      missing,
      onCreate: setCreating,
    }
    const dialogs = (
      <>
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
      </>
    )
    const boardBack = { label: t`Board`, to: all ? "/board" : boardLink(urlBoard?.id ?? boardId) }
    if (showIdea && urlBoard)
      return (
        <RoadmapLayout {...navProps} testId="roadmap-home" aside={<IdeaInfo board={urlBoard} ideaId={ideaId} />}>
          <FlatTop back={boardBack}>
            <RoadmapTrail place={t`Idea`} name={urlBoard.name} />
          </FlatTop>
          <div className="mt-3 max-w-[760px]" data-testid="idea-pane">
            <IdeaPane board={urlBoard} ideaId={ideaId} all={search.get("scope") === "all"} key={ideaId} noBack />
          </div>
          {dialogs}
        </RoadmapLayout>
      )
    const stageName = stage ? text.stage[stage] : t`All stages`
    return (
      <RoadmapLayout
        {...navProps}
        testId="roadmap-home"
        aside={selected && !all ? <BoardInfo board={selected} /> : scoped.length > 0 && <BoardsInfo boards={scoped} />}
      >
        <FlatTop
          actions={
            scoped.length > 0 && (
              <>
                {canManage && selected && !all && (
                  <Button
                    variant="outline"
                    className={topButton}
                    disabled={patch.isPending}
                    onClick={() => togglePublic(selected)}
                    title={selected.public ? t`Make private (${publicKey})` : t`Make public (${publicKey})`}
                    data-testid="board-toggle-public"
                  >
                    {selected.public ? <Trans>Make private</Trans> : <Trans>Make public</Trans>}
                  </Button>
                )}
                <PublicBoards boards={scoped} className={topButton} />
                <Button className={topInk} onClick={() => setWriting(true)} data-testid="new-idea">
                  <PlusIcon className="size-3.5" aria-hidden />
                  <Trans>New idea</Trans>
                </Button>
              </>
            )
          }
        >
          <RoadmapTrail place={tab === "settings" ? t`Settings` : stageName} name={all ? undefined : selected?.name} />
        </FlatTop>
        <section className="@container flex min-w-0 flex-col">
          <PageHeading
            title={tab === "settings" ? t`Settings` : stage ? stageName : all || !selected ? t`Roadmap` : selected.name}
            lede={
              selected && !all && tab === "board" ? (
                <span data-testid="board-state">
                  <span data-testid="board-visibility">
                    {selected.public ? <Trans>Public</Trans> : <Trans>Private</Trans>}
                  </span>
                  {selected.public && (
                    <>
                      {" · "}
                      <a
                        href={selected.url}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="hover:text-foreground hover:underline"
                        dir="ltr"
                        data-testid="board-url"
                      >
                        {selected.url.replace(/^https?:\/\//, "")}
                      </a>
                    </>
                  )}
                  {". "}
                  <Trans>Ideas your customers vote on, with the feedback behind them</Trans>
                </span>
              ) : (
                <Trans>Ideas your customers vote on, with the feedback behind them</Trans>
              )
            }
          />
          <ErrorLine error={patch.error} className="mt-3" />
          {boards.isPending ? (
            <div className="mt-7 flex flex-col gap-3" aria-busy data-testid="roadmap-loading">
              {[0, 1, 2].map((n) => (
                <Skeleton key={n} className="h-5 w-full" />
              ))}
            </div>
          ) : boards.error ? (
            <FlatEmpty>
              <ErrorLine error={boards.error} />
              <Button variant="outline" className={topButton} onClick={() => void boards.refetch()}>
                <Trans>Try again</Trans>
              </Button>
            </FlatEmpty>
          ) : missingBoard ? (
            <FlatEmpty>
              <Trans>This board no longer exists or you cannot see it.</Trans>
            </FlatEmpty>
          ) : scoped.length === 0 ? (
            <FlatEmpty>
              <p className="m-0 max-w-xl">
                {firstMissing ? (
                  <Trans>
                    Show your customers what you are building, and link the feedback behind each idea. Create a board
                    for {missingName} to sort ideas into stages.
                  </Trans>
                ) : (
                  <Trans>No board yet. Owners and admins create it.</Trans>
                )}
              </p>
              {firstMissing && (
                <Button
                  variant="outline"
                  className={topButton}
                  onClick={() => setCreating(firstMissing)}
                  data-testid="create-board-empty"
                >
                  <PlusIcon />
                  <Trans>Create board</Trans>
                </Button>
              )}
            </FlatEmpty>
          ) : tab === "settings" && selected ? (
            <div className="mt-6 max-w-[1040px]" data-testid="board-settings-page">
              <Settings board={selected} key={selected.id} />
            </div>
          ) : stage ? (
            <StageList
              boards={inView}
              stage={stage}
              all={all}
              inboxes={inboxById}
              active={!creating && !writing && !moving}
            />
          ) : (
            <Board
              boards={inView}
              all={all}
              moving={moving}
              onMoving={setMoving}
              flat
              onMoved={(idea, s) => {
                if (s === "shipped" && (idea.linked_count ?? 0) > 0 && idea.board_id)
                  navigate(ideaLink(idea.board_id, idea.id, all), { state: { offerReply: true } })
              }}
            />
          )}
        </section>
        {dialogs}
      </RoadmapLayout>
    )
  }

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
        <div className="@container min-w-0 flex-1 overflow-hidden" aria-busy data-testid="roadmap-loading">
          <div className="flex gap-3 p-6 @max-3xl:p-4">
            {[96, 64, 120, 40].map((h, n) => (
              <div
                key={n}
                className="flex min-w-64 flex-1 basis-0 flex-col gap-2 rounded-[14px] border bg-surface p-2.5 max-md:min-w-[82%] max-md:flex-none"
              >
                <Skeleton className="m-1 h-4 w-24" />
                <Skeleton className="h-[86px] rounded-[11px]" />
                <Skeleton className="rounded-[11px]" style={{ height: h }} />
              </div>
            ))}
          </div>
        </div>
      ) : boards.error ? (
        <div className="mx-auto my-12 flex max-w-sm flex-col items-center gap-3 px-4 text-center">
          <span className="grid size-8 place-items-center rounded-full bg-destructive/10 text-destructive">
            <CircleAlertIcon className="size-4" aria-hidden />
          </span>
          <ErrorLine error={boards.error} />
          <Button variant="outline" className={look.btn} onClick={() => void boards.refetch()}>
            <Trans>Try again</Trans>
          </Button>
        </div>
      ) : missingBoard ? (
        <div className="flex flex-1 flex-col gap-3 p-6">
          <PaneBack to="/board" label={t`Roadmap`} />
          <p className="text-body text-muted-foreground">
            <Trans>This board no longer exists or you cannot see it.</Trans>
          </p>
        </div>
      ) : scoped.length === 0 ? (
        <div className="mx-auto my-8 flex w-[min(560px,calc(100%-2rem))] flex-col items-center gap-3 rounded-[14px] border bg-card px-6 py-10 text-center">
          <span className="grid size-10 place-items-center rounded-xl bg-brand-wash text-brand">
            <KanbanIcon className="size-5" aria-hidden />
          </span>
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
          <div className="@container shrink-0">
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
          </div>
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

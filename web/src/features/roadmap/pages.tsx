import "@/features/roadmap/i18n"
import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { ArrowUpRightIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { Link, Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { ErrorLine, InfoColumn, InfoSection, KV, PageLayout, PageTop } from "@/components/common"
import { PaneBack } from "@/components/common/Column"
import { FilterMenu, FilterRow, ListBox, ListBoxEmpty, ListBoxSkeleton, PageTitle } from "@/components/common/ListBox"
import { keyLabel, ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { formatDateTime, IDEA_STAGES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useDiscussion } from "@/features/docs/discussions/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { Board } from "@/features/roadmap/Board"
import { openIdeas } from "@/features/roadmap/BoardBar"
import { CreateBoardDialog } from "@/features/roadmap/CreateBoardDialog"
import { IdeaPane } from "@/features/roadmap/IdeaPane"
import { NewIdeaForm } from "@/features/roadmap/NewIdeaForm"
import { boardLink, boardOfInbox, ideaLink, useBoards, useUpdateBoard } from "@/features/roadmap/queries"
import { StageList, stageTo } from "@/features/roadmap/RoadmapFrame"
import { Settings } from "@/features/roadmap/Settings"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Discussion, IdeaStage, Inbox, RoadmapBoard } from "@/lib/api"
import { useSession } from "@/lib/session"
import { isSupport, useInboxes } from "@/lib/workspace"

type Tab = "board" | "settings"

function PublicBoards({ boards, className }: { boards: RoadmapBoard[]; className?: string }) {
  const open = boards.filter((b) => b.public)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="outline" className={className} />} data-testid="board-open-public">
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
  const { t, i18n } = useLingui()
  const navigate = useNavigate()
  const { canManage } = useSession()
  const [search] = useSearchParams()
  const [boardId = "", , ideaId] = (useParams()["*"] ?? "").split("/")
  const [inboxId] = useInboxFilter()
  const inboxes = useInboxes().data ?? []
  const inboxById = new Map(inboxes.map((i) => [i.id, i]))
  const boards = useBoards()
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
  const missingBoard = !!boardId && !boards.isPending && !urlBoard
  const fmt = new Intl.NumberFormat(i18n.locale)

  const togglePublic = (b: RoadmapBoard) => {
    if (patch.isPending) return
    patch.mutate({ public: !b.public })
  }
  const boardTab = all ? "/board" : boardLink(selected?.id ?? boardId)
  const settingsTab = boardLink(selected?.id ?? scoped[0]?.id ?? boardId, "settings")
  const busy = !!creating || writing || !!moving

  useHotkeys(
    {
      ...(firstMissing ? { [ROADMAP_KEYS.newBoard]: () => setCreating(firstMissing) } : {}),
      [ROADMAP_KEYS.board]: () => navigate(boardTab, { replace: true }),
      ...(canManage && scoped.length > 0
        ? { [ROADMAP_KEYS.settings]: () => navigate(settingsTab, { replace: true }) }
        : {}),
      ...(canManage && selected ? { [ROADMAP_KEYS.publicBoard]: () => togglePublic(selected) } : {}),
      ...(ideaId ? { Escape: () => navigate(all ? "/board" : boardLink(urlBoard?.id ?? boardId)) } : {}),
      ...(selected?.public && !ideaId
        ? { [ROADMAP_KEYS.openPage]: () => window.open(selected.url, "_blank", "noopener,noreferrer") }
        : {}),
      ...(scoped.length > 0 && !ideaId ? { [ROADMAP_KEYS.newIdea]: () => setWriting(true) } : {}),
    },
    !busy
  )

  const createDialog = creating && (
    <CreateBoardDialog
      inboxId={creating.id}
      inboxName={creating.name}
      onClose={() => setCreating(null)}
      onCreated={(b) => {
        setCreating(null)
        navigate(boardLink(b.id, "settings"))
      }}
    />
  )

  if (showIdea && urlBoard)
    return (
      <PageLayout
        testId="roadmap-home"
        label={t`Roadmap`}
        aside={
          <InfoColumn label={t`Details`}>
            <IdeaInfo board={urlBoard} ideaId={ideaId} />
          </InfoColumn>
        }
      >
        <PageTop back={{ label: t`Roadmap`, to: all ? "/board" : boardLink(urlBoard.id) }} />
        <div className="max-w-[760px]" data-testid="idea-pane">
          <IdeaPane board={urlBoard} ideaId={ideaId} all={search.get("scope") === "all"} key={ideaId} noBack />
        </div>
      </PageLayout>
    )

  const openCount = inView.reduce((n, b) => n + openIdeas(b), 0)
  const stageCount = (s: IdeaStage) => inView.reduce((n, b) => n + b.counts[s], 0)
  const sub =
    selected && !all ? (
      <span data-testid="board-state">
        <span data-testid="board-visibility">{selected.public ? <Trans>Public</Trans> : <Trans>Private</Trans>}</span>
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
      </span>
    ) : scoped.length > 0 ? (
      <Plural value={openCount} one="# idea" other="# ideas" />
    ) : (
      <Trans>Ideas your customers vote on, with the feedback behind them</Trans>
    )
  return (
    <PageLayout testId="roadmap-home" label={t`Roadmap`}>
      <section className="@container flex min-w-0 flex-col">
        <PageTitle
          title={tab === "settings" ? t`Board settings` : t`Roadmap`}
          sub={sub}
          actions={
            scoped.length > 0 && (
              <>
                {canManage &&
                  (tab === "settings" ? (
                    <Button variant="outline" render={<Link to={boardTab} />} data-testid="board-tab-board">
                      <Trans>Board</Trans>
                    </Button>
                  ) : (
                    <Button variant="outline" render={<Link to={settingsTab} />} data-testid="board-tab-settings">
                      <Trans>Settings</Trans>
                    </Button>
                  ))}
                {canManage && selected && !all && (
                  <Button
                    variant="outline"
                    disabled={patch.isPending}
                    onClick={() => togglePublic(selected)}
                    title={selected.public ? t`Make private (${publicKey})` : t`Make public (${publicKey})`}
                    data-testid="board-toggle-public"
                  >
                    {selected.public ? <Trans>Make private</Trans> : <Trans>Make public</Trans>}
                  </Button>
                )}
                <PublicBoards boards={scoped} />
                {tab === "board" && (
                  <Button
                    onClick={() => setWriting(true)}
                    disabled={writing}
                    title={t`New idea (${ROADMAP_KEYS.newIdea.toUpperCase()})`}
                    data-testid="new-idea"
                  >
                    <PlusIcon />
                    <Trans>New idea</Trans>
                  </Button>
                )}
              </>
            )
          }
          testId="roadmap-head"
        />
        {writing && (
          <NewIdeaForm
            boards={scoped}
            initialBoardId={selected?.id ?? urlBoard?.id}
            onClose={() => setWriting(false)}
          />
        )}
        {(scoped.length > 1 || (tab === "board" && scoped.length > 0) || missing.length > 0) && (
          <FilterRow testId="roadmap-filters">
            {scoped.length > 1 && (
              <FilterMenu
                value={all ? "" : (selected?.id ?? "")}
                onChange={(id) =>
                  navigate(
                    id
                      ? boardLink(id, tab === "settings" ? "settings" : undefined)
                      : tab === "settings"
                        ? settingsTab
                        : "/board"
                  )
                }
                options={scoped.map((b) => ({
                  value: b.id,
                  label: `${inboxById.get(b.inbox_id)?.name ?? b.name} · ${fmt.format(openIdeas(b))}`,
                }))}
                anyLabel={tab === "settings" ? (selected?.name ?? t`Every board`) : t`Every board`}
                label={t`Board`}
                testId="board-chip"
              />
            )}
            {tab === "board" && scoped.length > 0 && (
              <FilterMenu
                value={stage ?? ""}
                onChange={(v) => navigate(stageTo(all, selected?.id, (v || undefined) as IdeaStage | undefined))}
                options={IDEA_STAGES.map((s) => ({
                  value: s,
                  label: `${text.stage[s]} · ${fmt.format(stageCount(s))}`,
                }))}
                anyLabel={t`Every stage`}
                label={t`Stage`}
                testId="board-stage"
              />
            )}
            {missing.map((i) => {
              const name = i.name
              return (
                <Button
                  key={i.id}
                  variant="ghost"
                  size="sm"
                  className="h-8"
                  onClick={() => setCreating(i)}
                  title={t`Create board (${keyLabel(ROADMAP_KEYS.newBoard).join("")})`}
                  data-testid="create-board-open"
                >
                  <PlusIcon />
                  <Trans>Create board for {name}</Trans>
                </Button>
              )
            })}
          </FilterRow>
        )}
        <ErrorLine error={patch.error} className="mt-3" />
        {boards.isPending ? (
          <ListBox>
            <ListBoxSkeleton />
          </ListBox>
        ) : boards.error ? (
          <ListBox>
            <ListBoxEmpty
              action={
                <Button variant="outline" size="sm" onClick={() => void boards.refetch()}>
                  <Trans>Try again</Trans>
                </Button>
              }
            >
              <ErrorLine error={boards.error} />
            </ListBoxEmpty>
          </ListBox>
        ) : missingBoard ? (
          <ListBox>
            <ListBoxEmpty>
              <Trans>This board no longer exists or you cannot see it.</Trans>
            </ListBoxEmpty>
          </ListBox>
        ) : scoped.length === 0 ? (
          <ListBox>
            <ListBoxEmpty
              action={
                firstMissing && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setCreating(firstMissing)}
                    data-testid="create-board-empty"
                  >
                    <PlusIcon />
                    <Trans>Create board</Trans>
                  </Button>
                )
              }
            >
              {firstMissing ? (
                <Trans>
                  Show your customers what you are building, and link the feedback behind each idea. Create a board for{" "}
                  {missingName} to sort ideas into stages.
                </Trans>
              ) : (
                <Trans>No board yet. Owners and admins create it.</Trans>
              )}
            </ListBoxEmpty>
          </ListBox>
        ) : tab === "settings" && selected ? (
          <div className="mt-4 max-w-[1040px]" data-testid="board-settings-page">
            <Settings board={selected} key={selected.id} />
          </div>
        ) : stage ? (
          <StageList boards={inView} stage={stage} all={all} inboxes={inboxById} active={!busy} />
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
      {createDialog}
    </PageLayout>
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

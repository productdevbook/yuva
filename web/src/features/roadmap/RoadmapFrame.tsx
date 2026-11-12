import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { MessageSquareIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, FlatEmpty, NavGroup, SectionPage, TopSep, type NavItem } from "@/components/common"
import { IDEA_STAGES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { look } from "@/features/inbox/PageFrame"
import { openIdeas } from "@/features/roadmap/BoardBar"
import { boardLink, ideaLink, useScopeIdeas } from "@/features/roadmap/queries"
import { STAGE_DOT } from "@/features/roadmap/stages"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { IdeaStage, Inbox, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"

export function stageTo(all: boolean, boardId: string | undefined, stage?: IdeaStage) {
  const base = all || !boardId ? "/board" : boardLink(boardId)
  return stage ? `${base}${base.includes("?") ? "&" : "?"}stage=${stage}` : base
}

function RoadmapNav({
  boards,
  all,
  selected,
  inboxes,
  stage,
  settings,
  settingsTo,
  missing,
  onCreate,
}: {
  boards: RoadmapBoard[]
  all: boolean
  selected?: RoadmapBoard
  inboxes: Map<string, Inbox>
  stage?: IdeaStage
  settings: boolean
  settingsTo?: string
  missing: Inbox[]
  onCreate: (i: Inbox) => void
}) {
  const { t } = useLingui()
  const text = useEnumText()
  const inView = selected ? [selected] : boards
  const count = (s: IdeaStage) => inView.reduce((n, b) => n + b.counts[s], 0)
  const id = selected?.id
  const stages: NavItem[] = [
    {
      key: "all",
      label: t`All stages`,
      to: stageTo(all, id),
      count: inView.reduce((n, b) => n + openIdeas(b), 0),
      current: !settings && !stage,
      testId: "board-tab-board",
    },
    ...IDEA_STAGES.map((s) => ({
      key: s,
      label: text.stage[s],
      to: stageTo(all, id, s),
      count: count(s),
      current: !settings && stage === s,
      testId: `board-stage-${s}`,
    })),
  ]
  const pending = inView.reduce((n, b) => n + b.counts.pending, 0)
  const extra: NavItem[] = [
    ...(pending > 0
      ? [{ key: "pending", label: t`Waiting for approval`, to: stageTo(all, id), count: pending, hot: true }]
      : []),
    ...(settingsTo
      ? [{ key: "settings", label: t`Settings`, to: settingsTo, current: settings, testId: "board-tab-settings" }]
      : []),
  ]
  const rows: NavItem[] = [
    ...(boards.length > 1 && !settings
      ? [
          {
            key: "all",
            label: t`All boards`,
            color: "",
            to: "/board",
            count: boards.reduce((n, b) => n + openIdeas(b), 0),
            current: all,
            testId: "board-chip-all",
          },
        ]
      : []),
    ...boards.map((b) => ({
      key: b.id,
      label: b.name,
      color: inboxes.get(b.inbox_id)?.branding.color ?? "",
      to: boardLink(b.id, settings ? "settings" : undefined),
      count: openIdeas(b),
      current: !all && selected?.id === b.id,
      testId: "board-chip",
    })),
    ...missing.map((i) => {
      const name = i.name
      return {
        key: `new-${i.id}`,
        label: t`Create board for ${name}`,
        color: "",
        onSelect: () => onCreate(i),
        testId: "create-board-open",
      }
    }),
  ]
  return (
    <>
      {boards.length > 0 && <NavGroup items={stages} />}
      {extra.length > 0 && <NavGroup items={extra} />}
      {rows.length > 0 && <NavGroup title={t`Boards`} items={rows} />}
    </>
  )
}

export function RoadmapLayout({
  aside,
  testId,
  children,
  ...nav
}: React.ComponentProps<typeof RoadmapNav> & {
  aside?: React.ReactNode
  testId?: string
  children: React.ReactNode
}) {
  const { t } = useLingui()
  return (
    <SectionPage title={t`Roadmap`} nav={<RoadmapNav {...nav} />} aside={aside} testId={testId}>
      {children}
    </SectionPage>
  )
}

export function RoadmapTrail({ place, name }: { place: string; name?: string }) {
  const { t } = useLingui()
  return (
    <>
      <span className="font-medium text-foreground">{t`Roadmap`}</span>
      <TopSep />
      <span className="truncate">{place}</span>
      <TopSep />
      <span className="truncate normal-case">{name ?? t`All boards`}</span>
    </>
  )
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
      <div className="mt-7 flex flex-col gap-3" aria-busy>
        {[0, 1, 2].map((n) => (
          <Skeleton key={n} className="h-5 w-full" />
        ))}
      </div>
    )
  if (items.length === 0)
    return (
      <FlatEmpty testId="stage-empty">
        <ErrorLine error={list.error} />
        {stage === "ideas" ? <Trans>No ideas yet. New ones land here.</Trans> : <Trans>Nothing here yet.</Trans>}
      </FlatEmpty>
    )
  return (
    <div ref={ref} className="mt-7" data-testid="stage-column" data-stage={stage}>
      {items.map((idea, n) => {
        const votes = fmt.format(idea.votes)
        const linked = idea.linked_count ?? 0
        const inbox = boards.length > 1 && idea.board_id ? inboxOf.get(idea.board_id) : undefined
        return (
          <button
            key={idea.id}
            type="button"
            onClick={() => open(n)}
            className={cn(
              "flex w-full min-w-0 items-center gap-3.5 border-t border-line-soft px-1 py-3 text-start text-body text-fg2 outline-none transition-colors hover:bg-subtle focus-visible:bg-subtle",
              n === cursor && "bg-subtle shadow-[inset_2px_0_0_var(--brand)]"
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
      {list.hasNextPage && (
        <div className="flex border-t border-line-soft pt-4">
          <Button
            variant="outline"
            className={look.btnSm}
            onClick={() => void list.fetchNextPage()}
            disabled={list.isFetchingNextPage}
          >
            <Trans>Load more</Trans>
          </Button>
        </div>
      )}
    </div>
  )
}

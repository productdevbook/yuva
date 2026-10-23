import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon, ExternalLinkIcon, GlobeIcon } from "lucide-react"
import { useState } from "react"
import { useLocation } from "react-router"

import { ROADMAP_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { formatDateTime, IDEA_STAGES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Thread } from "@/features/docs/discussions/ThreadPane"
import { useDiscussion } from "@/features/docs/discussions/queries"
import { IdeaIssues } from "@/features/issues/IdeaIssues"
import { IdeaFeedback } from "@/features/roadmap/IdeaFeedback"
import { MoveDialog } from "@/features/roadmap/MoveDialog"
import { boardLink, useStageChanges } from "@/features/roadmap/queries"
import { STAGE_DOT } from "@/features/roadmap/stages"
import { Card, EmptyRow, Section } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Discussion, IdeaStage, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useMemberMap } from "@/lib/workspace"

function History({ idea }: { idea: Discussion }) {
  const { i18n } = useLingui()
  const text = useEnumText()
  const changes = useStageChanges(idea.id)
  const members = useMemberMap()
  const items = changes.data ?? []
  return (
    <Section title={<Trans>Stage history</Trans>}>
      <Card flush data-testid="stage-history">
        {changes.isPending ? (
          <Skeleton className="m-4 h-10" />
        ) : items.length === 0 ? (
          <EmptyRow>
            <Trans>The idea has not been moved yet.</Trans>
          </EmptyRow>
        ) : (
          <ul className="divide-y">
            {items.map((c) => {
              const who = c.member_id ? members.get(c.member_id) : undefined
              const progress = c.progress ? text.progress[c.progress] : ""
              const set = c.progress_set
              return (
                <li key={c.id} className="flex flex-col gap-0.5 px-4 py-2.5" data-testid="stage-change">
                  <span className="flex items-center gap-2 text-body font-medium">
                    {text.stage[c.from_stage]}
                    <ArrowRightIcon className="size-3.5 text-faint rtl:rotate-180" />
                    {text.stage[c.to_stage]}
                    <time className="ms-auto text-caption font-normal text-faint" dateTime={c.created_at}>
                      {formatDateTime(c.created_at, i18n.locale)}
                    </time>
                  </span>
                  <span className="text-small text-muted-foreground">
                    {who && <span>{who.name || who.email}</span>}
                    {who && c.progress && " · "}
                    {c.progress && (
                      <Trans>
                        Linked feedback set to {progress}: {set}
                      </Trans>
                    )}
                  </span>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </Section>
  )
}

export function IdeaPane({ board, ideaId, all }: { board: RoadmapBoard; ideaId: string; all: boolean }) {
  const { t } = useLingui()
  const location = useLocation()
  const text = useEnumText()
  const detail = useDiscussion(ideaId)
  const idea = detail.data?.discussion
  const [moving, setMoving] = useState<IdeaStage | null>(null)
  const [offer, setOffer] = useState((location.state as { offerReply?: boolean } | null)?.offerReply ? 1 : 0)
  const stage = idea?.stage
  const shift = (d: number) => {
    const to = stage && IDEA_STAGES[IDEA_STAGES.indexOf(stage) + d]
    if (to) setMoving(to)
  }
  useHotkeys(
    {
      [ROADMAP_KEYS.move]: () => stage && setMoving(stage),
      [ROADMAP_KEYS.moveBack]: () => shift(-1),
      [ROADMAP_KEYS.moveForward]: () => shift(1),
      [ROADMAP_KEYS.openPage]: () =>
        board.public && window.open(`${board.url}/ideas/${ideaId}`, "_blank", "noopener,noreferrer"),
    },
    !!idea && !moving
  )
  return (
    <>
      <Thread
        embedded
        discussionId={ideaId}
        back={{ to: all ? "/board" : boardLink(board.id), label: t`Board` }}
        actions={(d) => (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => d.stage && setMoving(d.stage)}
              title={`${t`Move to a stage`} (${keyLabel(ROADMAP_KEYS.move).join("")})`}
              data-testid="idea-stage"
            >
              {d.stage && <span aria-hidden className={cn("size-[7px] shrink-0 rounded-full", STAGE_DOT[d.stage])} />}
              {d.stage ? text.stage[d.stage] : null}
              <ArrowRightIcon className="rtl:rotate-180" />
            </Button>
            {board.public && d.state === "published" && (
              <Button
                variant="outline"
                size="sm"
                render={<a href={`${board.url}/ideas/${d.id}`} target="_blank" rel="noreferrer noopener" />}
                title={`${t`Open on the public board`} (${keyLabel(ROADMAP_KEYS.openPage).join("")})`}
                data-testid="idea-public"
              >
                <GlobeIcon />
                <Trans>Public page</Trans>
                <ExternalLinkIcon className="opacity-60" />
              </Button>
            )}
          </>
        )}
        before={(d) => (
          <>
            <IdeaIssues idea={d} hotkeys={!moving} />
            <IdeaFeedback idea={d} board={board} offer={offer} />
          </>
        )}
        after={(d) => <History idea={d} />}
      />
      {moving && idea && (
        <MoveDialog
          idea={idea}
          to={moving}
          onClose={() => setMoving(null)}
          onMoved={(to) => {
            setMoving(null)
            if (to === "shipped" && (idea.linked_count ?? 0) > 0) setOffer((n) => n + 1)
          }}
        />
      )}
    </>
  )
}

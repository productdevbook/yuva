import { Trans, useLingui } from "@lingui/react/macro"

import { MoreLink, SectionHeading, toast } from "@/components/common"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useDiscussion } from "@/features/docs/discussions/queries"
import { look } from "@/features/inbox/PageFrame"
import { boardOfInbox, ideaLink, useBoards, useLinkIdea } from "@/features/roadmap/queries"
import type { Conversation } from "@/lib/api"

export function RoadmapCard({ c, onLink, onNew }: { c: Conversation; onLink: () => void; onNew: () => void }) {
  const { t, i18n } = useLingui()
  const text = useEnumText()
  const errorText = useErrorText()
  const { unlink } = useLinkIdea()
  const boards = useBoards()
  const idea = c.idea
  const detail = useDiscussion(idea?.id ?? "").data?.discussion
  const board = boards.data?.find((b) => b.id === detail?.board_id) ?? boardOfInbox(boards.data, c.inbox_id)
  const others = Math.max(0, (detail?.linked_count ?? 1) - 1)
  const votes = new Intl.NumberFormat(i18n.locale).format(detail?.votes ?? 0)
  const to = idea && (detail?.board_id ? ideaLink(detail.board_id, idea.id) : `/board/ideas/${idea.id}`)
  return (
    <section data-testid="feedback-roadmap">
      <SectionHeading
        title={<Trans>Roadmap</Trans>}
        more={
          to && (
            <MoreLink to={to} testId="idea-open">
              <Trans>Open idea</Trans>
            </MoreLink>
          )
        }
        lead={
          idea ? (
            <Trans>When this idea ships, you can answer everyone linked to it in one reply.</Trans>
          ) : (
            <Trans>Not on the roadmap yet. Link it to an idea, so you can tell everyone who asked when it ships.</Trans>
          )
        }
      />
      {idea ? (
        <div
          className="flex flex-wrap items-center gap-x-3.5 gap-y-2 border-y border-line-soft py-3.5"
          data-testid="idea-chip"
        >
          <span className="min-w-11 shrink-0 rounded-md border border-line-strong py-1 text-center font-mono text-small font-medium">
            {`▲ ${votes}`}
          </span>
          <span className="flex min-w-0 flex-[1_1_240px] flex-col">
            <span className="font-medium break-words text-foreground" dir="auto">
              {idea.title}
            </span>
            <span className="text-small text-muted-foreground">
              <span data-testid="idea-stage">{text.stage[idea.stage]}</span>
              {board ? ` · ${board.name}` : ""}
              {` · ${t`linked with ${others} other feedback`}`}
            </span>
          </span>
          <span className="flex gap-1.5">
            <Button variant="outline" className={look.btnSm} onClick={onLink} data-testid="idea-change">
              <Trans>Change</Trans>
            </Button>
            <Button
              variant="ghost"
              className={look.btnSm}
              disabled={unlink.isPending}
              onClick={() =>
                unlink.mutate({ ideaId: idea.id, conversationId: c.id }, { onError: (e) => toast(errorText(e)) })
              }
              data-testid="idea-chip-unlink"
            >
              <Trans>Unlink</Trans>
            </Button>
          </span>
        </div>
      ) : (
        <div className="flex flex-wrap gap-2" data-testid="roadmap-empty">
          <Button
            variant="outline"
            className={look.btnSm}
            onClick={onLink}
            title={t`Link to an idea (B)`}
            data-testid="header-idea"
          >
            <Trans>Link to an idea</Trans>
          </Button>
          <Button variant="ghost" className={look.btnSm} onClick={onNew} data-testid="idea-new">
            <Trans>New idea from this</Trans>
          </Button>
        </div>
      )}
    </section>
  )
}

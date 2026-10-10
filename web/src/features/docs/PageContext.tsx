import { Trans, useLingui } from "@lingui/react/macro"
import { BadgeCheckIcon, GlobeIcon, MessageCircleQuestionMarkIcon, ThumbsDownIcon, ThumbsUpIcon, UploadIcon } from "lucide-react"
import { useState } from "react"

import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { Button } from "@/components/ui/button"
import { pageLabel, PublishDialog, useAnswerActions } from "@/features/docs/AnswerDialog"
import { usePageAnswers } from "@/features/docs/queries"
import type { Conversation, Message, PageRating } from "@/lib/api"
import { cn } from "@/lib/utils"

const small = "h-6 gap-1 px-2 text-caption [&_svg]:size-3.5"

export function PageLink({ url, title, className }: { url: string; title?: string; className?: string }) {
  return (
    <a href={url} target="_blank" rel="noreferrer" title={url} className={cn("min-w-0 truncate text-muted-foreground underline-offset-4 hover:text-foreground hover:underline", className)} data-testid="page-link">
      {pageLabel(url, title)}
    </a>
  )
}

export function PageRatingMark({ rating }: { rating: PageRating }) {
  return rating === "up" ? (
    <span className="inline-flex shrink-0 items-center gap-1 text-success" data-testid="page-rating" data-rating="up">
      <ThumbsUpIcon className="size-3.5" />
      <Trans>Helpful</Trans>
    </span>
  ) : (
    <span className="inline-flex shrink-0 items-center gap-1 text-destructive" data-testid="page-rating" data-rating="down">
      <ThumbsDownIcon className="size-3.5" />
      <Trans>Not helpful</Trans>
    </span>
  )
}

const isMemberReply = (m: Message) => m.kind === "message" && m.direction === "out" && m.author.type === "member" && !m.draft

export function usePagePublishing(c: Conversation, items: Message[], allLoaded: boolean) {
  const { t } = useLingui()
  const question = c.kind === "question" ? c.question : undefined
  const answers = usePageAnswers({ conversation_id: c.id }, !!question)
  const published = answers.data?.pages[0]?.items[0]
  const reply = items.findLast(isMemberReply)
  const asked = allLoaded ? items.find((m) => m.kind === "message" && m.direction !== "out") : undefined
  const [publishing, setPublishing] = useState(false)
  const actions = useAnswerActions()
  const run = () => {
    if (!question || answers.isPending) return
    if (published) actions.edit(published)
    else if (reply) setPublishing(true)
  }
  const key = keyLabel(SHORTCUTS.publish).join("")

  const line = question ? (
    <span className="inline-flex min-w-0 items-center gap-1.5" data-testid="question-page">
      <MessageCircleQuestionMarkIcon className="size-3.5 shrink-0" />
      <span className="min-w-0 truncate">
        <Trans>
          Asked on <PageLink url={question.page_url} title={question.page_title} />
        </Trans>
      </span>
      {published ? (
        <>
          <span aria-hidden>·</span>
          <span className="inline-flex shrink-0 items-center gap-1 text-success" data-testid="question-published">
            <BadgeCheckIcon className="size-3.5" />
            <Trans>Published</Trans>
          </span>
          <Button variant="ghost" className={small} onClick={() => actions.edit(published)} title={t`Edit the published answer (${key})`} data-testid="question-edit">
            <Trans>Edit</Trans>
          </Button>
          <Button variant="ghost" className={cn(small, "text-destructive hover:text-destructive")} onClick={() => actions.unpublish(published)} data-testid="question-unpublish">
            <Trans>Unpublish</Trans>
          </Button>
        </>
      ) : reply ? (
        <Button variant="outline" className={cn(small, "ms-1")} onClick={run} disabled={answers.isPending} title={t`Publish to the page (${key})`} data-testid="question-publish">
          <UploadIcon />
          <Trans>Publish to page</Trans>
        </Button>
      ) : (
        !answers.isPending && (
          <span className="shrink-0">
            · <Trans>reply first to publish it</Trans>
          </span>
        )
      )}
    </span>
  ) : c.kind === "feedback" && c.feedback?.page_url ? (
    <span className="inline-flex min-w-0 items-center gap-1.5" data-testid="feedback-page">
      <GlobeIcon className="size-3.5 shrink-0" />
      <PageLink url={c.feedback.page_url} title={c.feedback.page_title} />
      {c.feedback.rating && (
        <>
          <span aria-hidden>·</span>
          <PageRatingMark rating={c.feedback.rating} />
        </>
      )}
    </span>
  ) : null

  const dialogs = (
    <>
      {actions.dialogs}
      {publishing && question && (
        <PublishDialog
          conversationId={c.id}
          page={question.page_url}
          initial={{ question: asked?.body.trim() || c.subject, answer: reply?.body.trim() ?? "" }}
          onClose={() => setPublishing(false)}
        />
      )}
    </>
  )
  return { line, dialogs, run }
}

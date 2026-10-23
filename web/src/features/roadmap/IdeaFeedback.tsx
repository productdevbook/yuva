import { Trans, useLingui } from "@lingui/react/macro"
import { CornerUpLeftIcon, LinkIcon, XIcon } from "lucide-react"
import { useEffect, useState } from "react"
import { Link } from "react-router"

import { ErrorLine, Kbd, toast, useContactName } from "@/components/common"
import { ReplyDialog, ReplyOutcome, type LinkedConversation } from "@/components/common/Linked"
import { keyLabel, ROADMAP_KEYS } from "@/components/common/ShortcutSheet"
import { useEnumText, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CategoryPill, ProgressPill } from "@/features/feedback/FeedbackList"
import { Ago, look, Overline } from "@/features/inbox/PageFrame"
import {
  useLinkedFeedback,
  useLinkIdea,
  useReplyCounts,
  useReplyOutcomes,
  useReplyToLinked,
  useUnlinkedFeedback,
} from "@/features/roadmap/queries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Discussion, RoadmapBoard } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChannelMap, useInboxes } from "@/lib/workspace"

function FeedbackRow({ c, action }: { c: LinkedConversation; action: React.ReactNode }) {
  const text = useEnumText()
  const channel = useChannelMap().get(c.channel_id ?? "")
  const contactName = useContactName()
  const who = contactName(c, channel, c.contact.name, c.contact.email)
  const f = c.feedback
  const line = ((f?.text || c.subject).split("\n").find((l) => l.trim()) ?? "").trim()
  return (
    <li
      className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3.5 py-2.5"
      data-testid="linked-conversation"
      data-conversation-id={c.id}
    >
      {f && <CategoryPill category={f.category} inboxId={c.inbox_id} />}
      <span className="flex min-w-0 flex-1 basis-40 flex-col leading-[1.3]">
        <span className="truncate font-medium" dir="auto">
          {line}
        </span>
        <span className={cn(look.faint, "flex min-w-0 items-center gap-1.5")}>
          <span className="max-w-32 min-w-0 truncate">{who}</span>
          <span aria-hidden>·</span>
          <Ago iso={c.created_at} className="shrink-0" />
          <span aria-hidden>·</span>
          <span
            className={cn("shrink-0 font-medium", c.status === "closed" ? "text-success" : "text-warning")}
            data-testid="linked-state"
          >
            {text.statusOf(c.status, "feedback")}
          </span>
        </span>
      </span>
      <span className="flex shrink-0 items-center gap-2 phone:basis-full phone:justify-end">
        {f?.progress && <ProgressPill progress={f.progress} />}
        {action}
      </span>
    </li>
  )
}

function Picker({ idea, board }: { idea: Discussion; board: RoadmapBoard }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const list = useUnlinkedFeedback(board.inbox_id, true)
  const { link } = useLinkIdea()
  const inbox = (useInboxes().data ?? []).find((i) => i.id === board.inbox_id)
  const inboxName = inbox?.name ?? board.name
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <div className="overflow-hidden rounded-xl border bg-background" data-testid="link-feedback-picker">
      <p className={cn(look.faint, "border-b bg-surface px-3.5 py-2")}>
        <Trans>Unlinked feedback from {inboxName}</Trans>
      </p>
      <ul className="max-h-72 divide-y overflow-y-auto">
        {list.isPending ? (
          <li className="p-3.5">
            <Skeleton className="h-9" />
          </li>
        ) : (
          items.map((c) => (
            <FeedbackRow
              key={c.id}
              c={c}
              action={
                <Button
                  variant="outline"
                  className={look.btnSm}
                  disabled={link.isPending}
                  onClick={() =>
                    link.mutate(
                      { ideaId: idea.id, conversationId: c.id },
                      { onSuccess: () => toast(t`Linked`), onError: (e) => toast(errorText(e)) }
                    )
                  }
                  data-testid="link-feedback"
                >
                  <Trans context="link feedback to the idea">Link</Trans>
                </Button>
              }
            />
          ))
        )}
        {!list.isPending && items.length === 0 && (
          <li className="px-4 py-4 text-center text-faint">
            <Trans>All feedback from {inboxName} that fits is already linked.</Trans>
          </li>
        )}
      </ul>
      <ErrorLine error={list.error} className="border-t px-3.5 py-2.5" />
    </div>
  )
}

export function IdeaFeedback({ idea, board, offer }: { idea: Discussion; board: RoadmapBoard; offer: number }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const list = useLinkedFeedback(idea.id)
  const { unlink } = useLinkIdea()
  const [picking, setPicking] = useState(false)
  const [replying, setReplying] = useState(false)
  const [replyId, setReplyId] = useState("")
  const [dismissed, setDismissed] = useState(0)
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = list
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage) void fetchNextPage()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])
  const items = list.data?.pages.flatMap((p) => p.items) ?? []
  const counting = list.isPending || !!hasNextPage
  const open = items.filter((c) => c.status !== "closed").length
  const closed = items.length - open
  const canReply = !counting && open > 0
  const key = keyLabel(ROADMAP_KEYS.replyLinked).join("")
  const linkKey = keyLabel(ROADMAP_KEYS.linkFeedback).join("")
  const title = idea.title
  const offering = offer > dismissed && canReply
  useHotkeys({
    [ROADMAP_KEYS.linkFeedback]: () => setPicking((p) => !p),
    ...(canReply ? { [ROADMAP_KEYS.replyLinked]: () => setReplying(true) } : {}),
  })
  return (
    <section className="flex flex-col gap-3" data-testid="idea-feedback">
      {offering && (
        <div
          className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-[10px] border border-brand/35 bg-brand-wash px-3.5 py-3"
          role="status"
          data-testid="ship-offer"
        >
          <p className="min-w-0 flex-1 basis-56">
            <Trans>Shipped. Open feedback linked to it: {open}. A reply is ready to send.</Trans>
          </p>
          <Button className={look.btnPSm} onClick={() => setReplying(true)} data-testid="ship-offer-review">
            <Trans>Review the reply</Trans>
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => setDismissed(offer)}
            aria-label={t`Dismiss`}
            title={t`Dismiss`}
          >
            <XIcon />
          </Button>
        </div>
      )}
      <div className="flex items-center gap-2">
        <Overline>
          <Trans>Linked feedback</Trans>
        </Overline>
        <span className={cn(look.faint, "flex-1 tabular-nums")} data-testid="linked-counts">
          {counting ? "…" : items.length}
        </span>
        <Button
          variant="outline"
          className={look.btnSm}
          aria-expanded={picking}
          onClick={() => setPicking((p) => !p)}
          title={`${t`Link feedback`} (${linkKey})`}
          data-testid="link-feedback-open"
        >
          <LinkIcon />
          <Trans>Link feedback</Trans>
        </Button>
      </div>
      {picking && <Picker idea={idea} board={board} />}
      {list.isPending ? (
        <Skeleton className="h-12 rounded-xl" />
      ) : items.length === 0 ? (
        <div className="rounded-xl border border-dashed p-4 text-muted-foreground" data-testid="linked-empty">
          <Trans>
            No feedback is linked yet. Use Link feedback, or open a piece of feedback and choose Link to an idea.
          </Trans>
        </div>
      ) : (
        <ul className={cn(look.card, "divide-y")} data-testid="linked-conversations">
          {items.map((c) => (
            <FeedbackRow
              key={c.id}
              c={c}
              action={
                <span className="flex shrink-0 gap-1.5">
                  <Button
                    variant="outline"
                    className={look.btnSm}
                    render={<Link to={`/feedback/${c.id}`} />}
                    data-testid="linked-open"
                  >
                    <Trans context="open the feedback">Open</Trans>
                  </Button>
                  <Button
                    variant="outline"
                    className={look.btnSm}
                    disabled={unlink.isPending}
                    onClick={() =>
                      unlink.mutate({ ideaId: idea.id, conversationId: c.id }, { onError: (e) => toast(errorText(e)) })
                    }
                    data-testid="linked-unlink"
                  >
                    <Trans>Unlink</Trans>
                  </Button>
                </span>
              }
            />
          ))}
        </ul>
      )}
      <ErrorLine error={list.error} />
      {items.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="font-medium">
            <Trans>Reply to everyone</Trans>
          </span>
          <p className={look.faint}>
            <Trans>
              One reply, written once, goes to every open feedback conversation linked to this idea, as you, on each
              conversation’s own channel.
            </Trans>
            {closed > 0 && (
              <>
                {" "}
                <Trans>Resolved feedback does not get it: {closed}</Trans>
              </>
            )}
          </p>
          <div>
            <Button
              variant="outline"
              className={look.btn}
              disabled={!canReply}
              onClick={() => setReplying(true)}
              title={t`Reply to linked (${key})`}
              data-testid="linked-reply"
            >
              <CornerUpLeftIcon />
              <Trans>Reply to everyone</Trans>
              <Kbd className="phone:hidden">{key}</Kbd>
            </Button>
          </div>
        </div>
      )}
      {replyId && (
        <ReplyOutcome
          hooks={hooks(idea.id)}
          replyId={replyId}
          unlinked={t`It was unlinked from the idea.`}
          onDismiss={() => setReplyId("")}
        />
      )}
      {replying && (
        <ReplyDialog
          hooks={hooks(idea.id)}
          description={
            <Trans>
              One reply, written once, goes to every open feedback conversation linked to this idea, as you, on each
              conversation’s own channel.
            </Trans>
          }
          open={open}
          initialBody={offering ? t`Good news: “${title}” has shipped. Thank you for asking for it.` : ""}
          onClose={() => setReplying(false)}
          onSent={(id) => {
            setReplying(false)
            setReplyId(id)
            setDismissed(offer)
          }}
        />
      )}
    </section>
  )
}

function hooks(ideaId: string) {
  return {
    useSend: () => useReplyToLinked(ideaId),
    useCounts: (id: string) => useReplyCounts(ideaId, id),
    useOutcomes: (id: string, enabled: boolean) => useReplyOutcomes(ideaId, id, enabled),
  }
}

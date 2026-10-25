import { Trans, useLingui } from "@lingui/react/macro"
import {
  ArrowBigUpIcon,
  CircleCheckIcon,
  CircleDotIcon,
  EyeOffIcon,
  HourglassIcon,
  LockIcon,
  MessageSquareIcon,
} from "lucide-react"
import { Link } from "react-router"

import { Dot, toast, useConfirm } from "@/components/common"
import { RowCheck } from "@/components/common/Selection"
import { useAuthorName } from "@/components/common/DiscussionAuthor"
import { formatShort, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { pageLabel } from "@/features/docs/PageContext"
import { threadLink, useModerate, type Moderation } from "@/features/docs/discussions/queries"
import type { Discussion, DiscussionAuthor, DiscussionPost } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

export function AuthorName({ author, className }: { author: DiscussionAuthor; className?: string }) {
  const name = useAuthorName()(author)
  const cls = cn("min-w-0 truncate font-medium", author.deleted && "text-faint italic", className)
  if (author.kind === "reader" && author.contact_id && !author.deleted)
    return (
      <Link to={`/contacts/${author.contact_id}`} className={cn(cls, "hover:underline")} data-testid="post-author">
        {name}
      </Link>
    )
  return (
    <span className={cls} data-testid="post-author">
      {name}
    </span>
  )
}

export function TeamTag() {
  return (
    <span className="shrink-0 rounded-full border px-1.5 py-px text-caption text-muted-foreground">
      <Trans>Team</Trans>
    </span>
  )
}

export function StateTag({ state }: { state: DiscussionPost["state"] }) {
  if (state === "published") return null
  return state === "pending" ? (
    <span
      className="inline-flex shrink-0 items-center gap-1 rounded-full bg-warning/10 px-2 py-px text-caption font-medium text-warning"
      data-testid="state-pending"
    >
      <HourglassIcon className="size-3" />
      <Trans>Waiting for approval</Trans>
    </span>
  ) : (
    <span
      className="inline-flex shrink-0 items-center gap-1 rounded-full bg-muted px-2 py-px text-caption font-medium text-muted-foreground"
      data-testid="state-hidden"
    >
      <EyeOffIcon className="size-3" />
      <Trans>Hidden</Trans>
    </span>
  )
}

export function ThreadIcon({ d, className }: { d: Discussion; className?: string }) {
  const { t } = useLingui()
  const [Icon, tone, label] =
    d.state === "hidden"
      ? [EyeOffIcon, "text-faint", t`Hidden`]
      : d.state === "pending"
        ? [HourglassIcon, "text-warning", t`Waiting for approval`]
        : d.answer_post_id
          ? [CircleCheckIcon, "text-success", t`Answered`]
          : d.locked
            ? [LockIcon, "text-faint", t`Locked`]
            : [CircleDotIcon, "text-brand", t`Unanswered`]
  return (
    <span title={label} className={cn("shrink-0", tone, className)}>
      <Icon className="size-4" />
      <span className="sr-only">{label}</span>
    </span>
  )
}

export function ThreadTags({ d }: { d: Discussion }) {
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {d.state === "published" &&
        d.kind !== "idea" &&
        (d.answer_post_id ? (
          <span
            className="inline-flex items-center gap-1 rounded-full bg-success/10 px-2 py-0.5 text-caption font-medium text-success"
            data-testid="thread-answered"
          >
            <CircleCheckIcon className="size-3.5" />
            <Trans>Answered</Trans>
          </span>
        ) : (
          <span
            className="inline-flex items-center gap-1 rounded-full bg-brand-wash px-2 py-0.5 text-caption font-medium text-brand"
            data-testid="thread-unanswered"
          >
            <CircleDotIcon className="size-3.5" />
            <Trans>Unanswered</Trans>
          </span>
        ))}
      <StateTag state={d.state} />
      {d.locked && (
        <span
          className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-caption font-medium text-muted-foreground"
          data-testid="thread-locked"
        >
          <LockIcon className="size-3" />
          <Trans>Locked</Trans>
        </span>
      )}
    </span>
  )
}

export function PostBody({ post, className }: { post: { body_html: string; deleted?: boolean }; className?: string }) {
  if (post.deleted)
    return (
      <p className={cn("text-body text-faint italic", className)}>
        <Trans>Deleted by its author.</Trans>
      </p>
    )
  return (
    <div
      className={cn("rich-text text-reading", className)}
      dangerouslySetInnerHTML={{ __html: post.body_html }}
      onClick={(e) => {
        const a = (e.target as HTMLElement).closest("a")
        if (!a?.href) return
        e.preventDefault()
        window.open(a.href, "_blank", "noopener,noreferrer")
      }}
      data-testid="post-body"
    />
  )
}

function Count({
  n,
  icon: Icon,
  label,
}: {
  n: number
  icon: React.ComponentType<{ className?: string }>
  label: string
}) {
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1 tabular-nums", n === 0 && "opacity-45")} title={label}>
      <Icon className="size-3.5" />
      <span className="sr-only">{label}</span>
      <span aria-hidden>{n}</span>
    </span>
  )
}

export function ThreadRow({
  d,
  cursor,
  showPage,
  from,
  to,
  current,
  check,
}: {
  d: Discussion
  cursor: boolean
  showPage: boolean
  from?: string
  to?: string
  current?: boolean
  check?: { checked: boolean; onToggle: (range: boolean) => void }
}) {
  const { t, i18n } = useLingui()
  const authorName = useAuthorName()(d.author)
  const inboxes = useInboxes().data ?? []
  const inbox = inboxes.length > 1 ? inboxes.find((i) => i.id === d.inbox_id) : undefined
  const fmt = new Intl.NumberFormat(i18n.locale)
  const [votes, replies, pending] = [d.votes, d.reply_count, d.pending_posts ?? 0].map((n) => fmt.format(n))
  const when = formatShort(d.last_activity_at, i18n.locale)
  return (
    <li className="flex items-start">
      {check && <RowCheck className="mt-3.5 ms-4" checked={check.checked} name={d.title} onToggle={check.onToggle} />}
      <Link
        to={to ?? threadLink(d.id)}
        state={from ? { from } : undefined}
        aria-current={current ? "page" : undefined}
        className={cn(
          "relative flex min-w-0 flex-1 items-start gap-3 px-4 py-3 outline-none transition-colors hover:bg-surface/60 focus-visible:bg-surface",
          current && "bg-brand/5 before:absolute before:inset-y-0 before:start-0 before:w-[3px] before:bg-brand",
          cursor && "bg-surface ring-2 ring-ring ring-inset"
        )}
        data-testid="thread-row"
      >
        <ThreadIcon d={d} className="mt-0.5" />
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex items-center gap-2">
            <span className={cn("min-w-0 flex-1 truncate text-body font-medium", d.state === "hidden" && "text-faint")}>
              {d.title}
            </span>
            <time className="shrink-0 text-caption text-faint" dateTime={d.last_activity_at}>
              {when}
            </time>
          </span>
          <span className="flex items-center gap-2 text-caption text-faint">
            <span className="flex min-w-0 flex-1 items-center gap-1.5 truncate">
              {inbox && <Dot color={inbox.branding.color} className="size-[7px] shrink-0 rounded-[2px]" />}
              <span className="truncate">
                {showPage ? (
                  <span dir="ltr">{pageLabel(d.page, d.page_title)}</span>
                ) : (
                  <Trans>Started by {authorName}</Trans>
                )}
              </span>
            </span>
            <span className="flex shrink-0 items-center gap-2.5">
              {(d.pending_posts ?? 0) > 0 && (
                <span
                  className="inline-flex items-center gap-1 font-medium text-warning tabular-nums"
                  title={t`${pending} waiting for approval`}
                  data-testid="thread-pending"
                >
                  <HourglassIcon className="size-3.5" />
                  {pending}
                </span>
              )}
              <Count n={d.votes} icon={ArrowBigUpIcon} label={t`${votes} votes`} />
              <Count n={d.reply_count} icon={MessageSquareIcon} label={t`${replies} replies`} />
            </span>
          </span>
        </span>
      </Link>
    </li>
  )
}

export function ListSkeleton() {
  return (
    <div className="flex flex-col gap-2 p-4">
      <Skeleton className="h-12 w-full rounded-lg" />
      <Skeleton className="h-12 w-full rounded-lg" />
      <Skeleton className="h-12 w-full rounded-lg" />
    </div>
  )
}

export function LoadMore({
  list,
}: {
  list: { hasNextPage: boolean; isFetchingNextPage: boolean; fetchNextPage: () => unknown }
}) {
  if (!list.hasNextPage) return null
  return (
    <div className="flex justify-center border-t p-2">
      <Button variant="ghost" size="sm" onClick={() => void list.fetchNextPage()} disabled={list.isFetchingNextPage}>
        <Trans>Load more</Trans>
      </Button>
    </div>
  )
}

export function useModeration() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const moderate = useModerate()
  const [confirm, dialog] = useConfirm()
  const done: Record<Moderation, string> = {
    published: t`Approved`,
    hidden: t`Hidden`,
    spam: t`Marked as spam`,
  }
  const run = (post: DiscussionPost, to: Moderation) =>
    moderate.mutate({ postId: post.id, to }, { onSuccess: () => toast(done[to]), onError: (e) => toast(errorText(e)) })
  const act = (post: DiscussionPost, to: Moderation) => {
    if (to === "published" && post.state === "published") return
    if (to === "hidden" && post.state === "hidden") return
    if (to === "spam") {
      if (post.author.kind === "member") return
      confirm({
        title: <Trans>Mark this post as spam?</Trans>,
        description: (
          <Trans>
            It is hidden with every other post of its author still waiting for approval, and their later posts always
            wait for a member.
          </Trans>
        ),
        confirm: <Trans>Mark as spam</Trans>,
        run: () => run(post, "spam"),
      })
      return
    }
    run(post, to)
  }
  return { act, dialog, pending: moderate.isPending }
}

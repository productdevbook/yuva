import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, EyeOffIcon, ShieldAlertIcon } from "lucide-react"
import { useEffect, useRef } from "react"
import { Link, useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import { AuthorAvatar } from "@/components/common/DiscussionAuthor"
import { DISCUSSION_KEYS, keyLabel } from "@/components/common/ShortcutSheet"
import { formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { usePickedSite } from "@/features/docs/Toolbar"
import { pageLabel } from "@/features/docs/PageContext"
import { AuthorName, ListSkeleton, LoadMore, PostBody, TeamTag, useModeration } from "@/features/docs/discussions/parts"
import { useQueue } from "@/features/docs/discussions/queries"
import { useInboxFilter } from "@/features/inbox/inboxFilter"
import { EmptyRow } from "@/features/settings/ui"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useResetState } from "@/hooks/use-reset-state"
import type { Discussion, DiscussionPostItem } from "@/lib/api"
import { cn } from "@/lib/utils"

const FROM = "/docs/discussions/queue"

function QueueItem({
  item,
  cursor,
  href,
  onAct,
}: {
  item: DiscussionPostItem
  cursor: boolean
  href: string
  onAct: (to: "published" | "hidden" | "spam") => void
}) {
  const { t, i18n } = useLingui()
  const { post, discussion: d } = item
  const hint = (label: string, key: string) => `${label} (${keyLabel(key).join("")})`
  return (
    <li
      className={cn("flex flex-col gap-2.5 rounded-[14px] border bg-card px-4 py-3.5", cursor && "ring-2 ring-ring")}
      data-testid="queue-item"
      data-post-id={post.id}
    >
      <div className="flex min-w-0 flex-col gap-0.5">
        <span className="flex items-center gap-2 text-caption font-medium text-faint">
          {post.opening ? (
            d.kind === "idea" ? (
              <Trans>New idea</Trans>
            ) : (
              <Trans>New discussion</Trans>
            )
          ) : (
            <Trans>Reply</Trans>
          )}
          <Link
            to={href}
            state={{ from: FROM }}
            className="ms-auto font-normal text-muted-foreground hover:text-foreground hover:underline"
            data-testid="queue-open"
          >
            <Trans>Open discussion</Trans>
          </Link>
        </span>
        <Link
          to={href}
          state={{ from: FROM }}
          className="min-w-0 truncate text-body font-medium hover:underline"
          data-testid="queue-thread"
        >
          {d.title}
        </Link>
        {d.page && (
          <span className="truncate text-caption text-faint" dir="ltr">
            {pageLabel(d.page, d.page_title)}
          </span>
        )}
      </div>
      <div className="flex min-w-0 items-center gap-2 text-body">
        <AuthorAvatar author={post.author} className="size-6" />
        <AuthorName author={post.author} />
        {post.author.kind === "member" && <TeamTag />}
        <time className="ms-auto shrink-0 text-caption text-faint" dateTime={post.created_at}>
          {formatShort(post.created_at, i18n.locale)}
        </time>
      </div>
      <PostBody post={post} className="max-h-60 overflow-y-auto" />
      <div className="-mx-2 flex flex-wrap items-center gap-0.5">
        <Button
          variant="ghost"
          size="sm"
          className="text-success hover:text-success"
          onClick={() => onAct("published")}
          title={hint(t`Approve`, DISCUSSION_KEYS.approve)}
          data-testid="queue-approve"
        >
          <CheckIcon />
          <Trans>Approve</Trans>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onAct("hidden")}
          title={hint(t`Hide`, DISCUSSION_KEYS.hide)}
          data-testid="queue-hide"
        >
          <EyeOffIcon />
          <Trans>Hide</Trans>
        </Button>
        {post.author.kind !== "member" && (
          <Button
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            onClick={() => onAct("spam")}
            title={hint(t`Spam`, DISCUSSION_KEYS.spam)}
            data-testid="queue-spam"
          >
            <ShieldAlertIcon />
            <Trans>Spam</Trans>
          </Button>
        )}
      </div>
    </li>
  )
}

export function QueueList({ hrefOf, keyboard }: { hrefOf: (d: Discussion) => string; keyboard: boolean }) {
  const navigate = useNavigate()
  const [inboxId] = useInboxFilter()
  const site = usePickedSite(inboxId)
  const queue = useQueue({ inbox_id: inboxId, site })
  const items = queue.data?.pages.flatMap((p) => p.items) ?? []
  const moderation = useModeration()
  const [picked, setCursor] = useResetState(-1, JSON.stringify([inboxId, site]))
  const cursor = Math.min(picked, items.length - 1)
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => {
    if (cursor >= 0)
      list.current?.querySelectorAll("[data-testid=queue-item]")[cursor]?.scrollIntoView({ block: "nearest" })
  }, [cursor])
  const target = items[Math.max(0, cursor)]
  const act = (to: "published" | "hidden" | "spam") => target && moderation.act(target.post, to)
  const down = () => setCursor(Math.min(items.length - 1, cursor + 1))
  const up = () => setCursor(Math.max(0, cursor - 1))
  useHotkeys(
    {
      [DISCUSSION_KEYS.next]: down,
      [DISCUSSION_KEYS.previous]: up,
      ArrowDown: down,
      ArrowUp: up,
      Enter: () => target && navigate(hrefOf(target.discussion), { state: { from: FROM } }),
      [DISCUSSION_KEYS.approve]: () => act("published"),
      [DISCUSSION_KEYS.hide]: () => act("hidden"),
      [DISCUSSION_KEYS.spam]: () => act("spam"),
    },
    keyboard
  )
  return (
    <div className="mx-auto flex w-full max-w-[860px] flex-col gap-3 p-6 pb-16 phone:px-4 phone:pt-4">
      <p className="text-body text-muted-foreground">
        <Trans>
          Readers' posts wait here until a member approves one of theirs; after that, they publish at once. Oldest
          first.
        </Trans>
      </p>
      <div className="flex flex-col gap-3" data-testid="queue">
        {queue.isPending ? (
          <ListSkeleton />
        ) : queue.error ? (
          <ErrorLine error={queue.error} className="p-4" />
        ) : items.length === 0 ? (
          <div className="rounded-[14px] border bg-card">
            <EmptyRow>
              <Trans>Nothing is waiting for approval.</Trans>
            </EmptyRow>
          </div>
        ) : (
          <ul ref={list} className="flex flex-col gap-3">
            {items.map((it, i) => (
              <QueueItem
                key={it.post.id}
                item={it}
                cursor={i === cursor}
                href={hrefOf(it.discussion)}
                onAct={(to) => {
                  setCursor(i)
                  moderation.act(it.post, to)
                }}
              />
            ))}
          </ul>
        )}
        <LoadMore list={queue} />
      </div>
      {moderation.dialog}
    </div>
  )
}

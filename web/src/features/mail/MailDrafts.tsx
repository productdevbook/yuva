import { Trans, useLingui } from "@lingui/react/macro"
import { PaperclipIcon, TriangleAlertIcon } from "lucide-react"
import { useMemo, useRef } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { ErrorLine, toast } from "@/components/common"
import { formatShort, useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useListCursor } from "@/features/inbox/listCursor"
import { useDrafts } from "@/features/inbox/queries"
import { useMailInboxes } from "@/features/mail/folders"
import { api, unwrap, type DraftItem } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { cn } from "@/lib/utils"
import { useInboxes } from "@/lib/workspace"

function Row({
  item,
  current,
  cursor,
  showInbox,
  onOpen,
}: {
  item: DraftItem
  current: boolean
  cursor: boolean
  showInbox: boolean
  onOpen: () => void
}) {
  const { t, i18n } = useLingui()
  const inbox = useInboxes().data?.find((i) => i.id === item.conversation.inbox_id)
  const d = item.draft
  const who = item.conversation.contact.name || item.conversation.contact.email || t`Unknown sender`
  const subject = item.conversation.subject.trim() || t`(no subject)`
  const line = d.body.replace(/\s+/g, " ").trim()
  return (
    <div
      className={cn(
        "group relative flex w-full items-start border-b transition-colors",
        current ? "bg-surface" : "hover:bg-surface",
        cursor && !current && "bg-surface",
        (current || cursor) && "shadow-[inset_2px_0_0_var(--brand)] rtl:shadow-[inset_-2px_0_0_var(--brand)]"
      )}
      data-testid="mail-row"
      data-conversation-id={item.conversation.id}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-start gap-2.5 px-3.5 py-3 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        aria-current={current || undefined}
        data-testid="mail-open"
      >
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          <span className="flex min-w-0 items-baseline gap-2">
            <span className="min-w-0 flex-1 truncate text-body font-medium" data-testid="mail-sender">
              {t`To: ${who}`}
            </span>
            {d.attachments.length > 0 && (
              <PaperclipIcon className="size-3.5 shrink-0 self-center text-faint" aria-label={t`Has attachments`} />
            )}
            <time className="shrink-0 text-caption text-faint" dateTime={d.created_at}>
              {formatShort(d.created_at, i18n.locale)}
            </time>
          </span>
          <span className="min-w-0 truncate text-body text-foreground/85" data-testid="mail-subject">
            {subject}
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-small text-muted-foreground">
            {showInbox && inbox && (
              <span className="shrink-0 rounded border px-1 text-caption text-faint">{inbox.name}</span>
            )}
            {d.send_error ? (
              <span className="flex min-w-0 items-center gap-1 text-destructive" data-testid="mail-draft-failed">
                <TriangleAlertIcon className="size-3.5 shrink-0" />
                <span className="min-w-0 truncate">{d.send_error.detail}</span>
              </span>
            ) : (
              <span className="min-w-0 truncate" data-testid="mail-line">
                <span className="font-medium text-brand">
                  <Trans>Draft:</Trans>
                </span>{" "}
                {line}
              </span>
            )}
          </span>
        </span>
      </button>
    </div>
  )
}

export function MailDrafts({
  box,
  currentId,
  onOpen,
  onCompose,
  active,
}: {
  box: string
  currentId: string | null
  onOpen: (id: string) => void
  onCompose: (item: DraftItem) => void
  active: boolean
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const mailBoxes = useMailInboxes()
  const scroller = useRef<HTMLDivElement>(null)
  const list = useDrafts({ inbox_id: box === "all" ? undefined : box, mine: true, scheduled: false })
  const items = useMemo(() => {
    const ids = new Set(mailBoxes.map((i) => i.id))
    return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((i) => ids.has(i.conversation.inbox_id))
  }, [list.data, mailBoxes])
  const rows = useMemo(() => items.map((i) => ({ id: i.conversation.id })), [items])

  const open = async (item: DraftItem) => {
    const id = item.conversation.id
    try {
      const c = await qc.fetchQuery({
        queryKey: keys.conversation(ws, id),
        queryFn: () =>
          unwrap(api.GET("/v1/conversations/{conversationId}", { params: { path: { conversationId: id } } })),
        staleTime: 0,
      })
      if (c.last_message_at) onOpen(id)
      else onCompose(item)
    } catch (e) {
      toast(errorText(e))
    }
  }
  const { cursorId } = useListCursor(
    rows,
    currentId,
    (id) => {
      const item = items.find((i) => i.conversation.id === id)
      if (item) void open(item)
    },
    active,
    scroller
  )

  return (
    <nav aria-label={t`Drafts`} className="flex h-full min-h-0 flex-col bg-background" data-testid="mail-list">
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto border-t" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1 p-3">
            {[0, 1, 2].map((i) => (
              <div key={i} className="flex flex-col gap-2 py-2">
                <Skeleton className="h-4 w-1/2" />
                <Skeleton className="h-3 w-3/4" />
              </div>
            ))}
          </div>
        ) : items.length === 0 && !list.hasNextPage ? (
          <div className="flex flex-col items-center gap-3 px-4 py-10 text-center" data-testid="list-empty">
            <p className="text-body text-faint">
              <Trans>You have no drafts. Mail you start writing is kept here.</Trans>
            </p>
          </div>
        ) : (
          items.map((item) => (
            <Row
              key={item.draft.id}
              item={item}
              current={item.conversation.id === currentId}
              cursor={item.conversation.id === cursorId}
              showInbox={box === "all"}
              onOpen={() => void open(item)}
            />
          ))
        )}
        {list.hasNextPage && (
          <div className="flex justify-center p-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => void list.fetchNextPage()}
              disabled={list.isFetchingNextPage}
            >
              <Trans>Load more</Trans>
            </Button>
          </div>
        )}
        <ErrorLine error={list.error} className="px-4 py-2" />
      </div>
    </nav>
  )
}

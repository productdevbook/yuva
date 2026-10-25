import { Trans, useLingui } from "@lingui/react/macro"
import { PaperclipIcon, TriangleAlertIcon } from "lucide-react"
import { useMemo, useRef } from "react"

import { ContactAvatar, ErrorLine } from "@/components/common"
import { formatShort } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useListCursor } from "@/features/inbox/listCursor"
import { useDrafts } from "@/features/inbox/queries"
import { useMailInboxes } from "@/features/mail/folders"
import type { DraftItem } from "@/lib/api"
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
        "group relative mx-2 flex w-[calc(100%-1rem)] items-start rounded-xl transition-colors",
        current ? "bg-card shadow-[0_1px_2px_rgb(0_0_0/0.05),0_0_0_1px_var(--border)]" : "hover:bg-muted/60",
        cursor && !current && "bg-muted/60",
        (current || cursor) &&
          "before:absolute before:inset-y-3 before:-start-2 before:w-[3px] before:rounded-e-[3px] before:bg-brand"
      )}
      data-testid="mail-row"
      data-conversation-id={item.conversation.id}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-start gap-3 rounded-xl px-2.5 py-2.5 text-start outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-current={current || undefined}
        data-testid="mail-open"
      >
        <ContactAvatar id={item.conversation.contact.id} name={who} className="size-[38px] font-semibold" />
        <span className="flex min-w-0 flex-1 flex-col gap-px">
          <span className="min-w-0 truncate text-body font-medium text-foreground/80" data-testid="mail-sender">
            {t`To: ${who}`}
          </span>
          <span className="min-w-0 truncate text-body text-foreground/85" data-testid="mail-subject">
            {subject}
          </span>
          <span className="min-w-0 truncate text-small text-faint">
            {d.send_error ? (
              <span className="flex min-w-0 items-center gap-1 text-destructive" data-testid="mail-draft-failed">
                <TriangleAlertIcon className="size-3.5 shrink-0" />
                <span className="min-w-0 truncate">{d.send_error.detail}</span>
              </span>
            ) : (
              <span data-testid="mail-line">
                <span className="font-semibold text-destructive">
                  <Trans>Draft:</Trans>
                </span>{" "}
                {line}
              </span>
            )}
          </span>
          {showInbox && inbox && (
            <span className="mt-1 flex min-w-0">
              <span className="max-w-full truncate rounded-full bg-muted px-2 text-[11px] leading-[18px] text-muted-foreground">
                {inbox.name}
              </span>
            </span>
          )}
        </span>
        <span className="flex shrink-0 flex-col items-end gap-1.5 pt-px">
          <time className="font-mono text-[12px] whitespace-nowrap text-faint tabular-nums" dateTime={d.created_at}>
            {formatShort(d.created_at, i18n.locale)}
          </time>
          {d.attachments.length > 0 && (
            <PaperclipIcon className="size-3.5 text-faint" aria-label={t`Has attachments`} />
          )}
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
  head,
}: {
  box: string
  currentId: string | null
  onOpen: (id: string, editDraft?: string) => void
  onCompose: (item: DraftItem) => void
  active: boolean
  head?: React.ReactNode
}) {
  const { t } = useLingui()
  const mailBoxes = useMailInboxes()
  const scroller = useRef<HTMLDivElement>(null)
  const list = useDrafts({ inbox_id: box === "all" ? undefined : box, mine: true, scheduled: false })
  const items = useMemo(() => {
    const ids = new Set(mailBoxes.map((i) => i.id))
    return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((i) => ids.has(i.conversation.inbox_id))
  }, [list.data, mailBoxes])
  const rows = useMemo(() => items.map((i) => ({ id: i.conversation.id })), [items])

  const open = (item: DraftItem) => {
    if (item.composed) onCompose(item)
    else onOpen(item.conversation.id, item.draft.send_error ? item.draft.id : undefined)
  }
  const { cursorId } = useListCursor(
    rows,
    currentId,
    (id) => {
      const item = items.find((i) => i.conversation.id === id)
      if (item) open(item)
    },
    active,
    scroller
  )

  return (
    <nav aria-label={t`Drafts`} className="flex h-full min-h-0 flex-col bg-surface" data-testid="mail-list">
      {head}
      <div ref={scroller} className="min-h-0 flex-1 overflow-y-auto pb-3" data-testid="list">
        {list.isPending ? (
          <div className="grid gap-1.5 px-4 py-2">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex items-start gap-3 py-2">
                <Skeleton className="size-9 rounded-full" />
                <div className="flex flex-1 flex-col gap-1.5">
                  <Skeleton className="h-3 w-[45%]" />
                  <Skeleton className="h-3 w-4/5" />
                  <Skeleton className="h-3 w-[95%]" />
                </div>
              </div>
            ))}
          </div>
        ) : items.length === 0 && !list.hasNextPage ? (
          <div className="flex flex-col items-start gap-1.5 px-6 py-10" data-testid="list-empty">
            <p className="text-body text-muted-foreground">
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
              onOpen={() => open(item)}
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

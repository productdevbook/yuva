import { Trans, useLingui } from "@lingui/react/macro"
import { CheckIcon, ClockIcon } from "lucide-react"
import { useEffect } from "react"
import { useNavigate, useParams } from "react-router"

import { useShell } from "@/app/shell"
import { ContactAvatar, toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { Skeleton } from "@/components/ui/skeleton"
import { QueueConversation } from "@/features/conversation/QueueConversation"
import { useBulkUpdateConversations, useConversations, useCounts } from "@/features/inbox/queries"
import { useQueue } from "@/features/inbox/queue"
import { useSetAvailability } from "@/lib/availability"
import { useViewing } from "@/lib/realtime"
import { useSession } from "@/lib/session"

const banner = "mb-4.5 flex items-center gap-3 rounded-[14px] py-2.5 ps-3.5 pe-2.5 text-sm"
const bannerButton = "h-8 shrink-0 rounded-full border bg-card px-3 text-[13px] hover:border-faint disabled:opacity-50"

function AwayBanner() {
  const { me } = useSession()
  const set = useSetAvailability()
  if (me.person.availability !== "away") return null
  return (
    <div className={`${banner} border bg-card text-muted-foreground`} data-testid="away-banner">
      <span className="inline-flex min-w-0 items-center gap-2">
        <ClockIcon className="size-[13px] shrink-0" />
        <Trans>You are away. Live chat shows nobody available and notifications pause.</Trans>
      </span>
      <button type="button" className={`${bannerButton} ms-auto`} onClick={() => set.mutate("auto")} disabled={set.isPending}>
        <Trans>I'm back</Trans>
      </button>
    </div>
  )
}

function CleanupBanner() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const errorText = useErrorText()
  const { inboxId } = useQueue()
  const counts = useCounts().data
  const spam = useConversations({ status: "open", spam: true }, !!counts?.spam)
  const bulk = useBulkUpdateConversations()
  const items = (spam.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
  if (!counts?.spam || items.length === 0) return null
  const ids = items.slice(0, 100).map((c) => c.id)
  const closeAll = () =>
    bulk.mutate(
      { conversation_ids: ids, status: "closed" },
      {
        onSuccess: (r) => {
          const done = r.updated.map((c) => c.id)
          toast(t`${done.length} closed`, () =>
            bulk.mutate({ conversation_ids: done, status: "open" }, { onError: (e) => toast(errorText(e)) }),
          )
        },
        onError: (e) => toast(errorText(e)),
      },
    )
  return (
    <div className={`${banner} border border-dashed bg-card`} data-testid="cleanup-banner">
      <span className="flex shrink-0">
        {items.slice(0, 4).map((c) => (
          <ContactAvatar
            key={c.id}
            id={c.contact.id}
            name={c.contact.name || c.contact.email || "?"}
            className="-ms-1.5 size-6 text-[9px] ring-2 ring-card first:ms-0"
          />
        ))}
      </span>
      <span className="min-w-0">
        <b className="block font-medium">
          <Trans>Quick cleanup</Trans>
        </b>
        <small className="block text-xs text-faint">
          <Trans>{items.length} marked as spam · no reply needed</Trans>
        </small>
      </span>
      <button type="button" className={`${bannerButton} ms-auto`} onClick={() => navigate(`/conversations/${items[0].id}`)}>
        <Trans>Review</Trans>
      </button>
      <button type="button" className={bannerButton} onClick={closeAll} disabled={bulk.isPending} data-testid="cleanup-close-all">
        <Trans>Close all</Trans>
      </button>
    </div>
  )
}

function EmptyQueue() {
  const { openDrawer } = useShell()
  return (
    <div className="pt-[16vh] text-center" data-testid="queue-empty">
      <div className="mx-auto mb-5.5 grid size-[72px] place-items-center rounded-[22px] bg-brand-wash text-brand">
        <CheckIcon className="size-8" />
      </div>
      <h1 className="text-[30px] font-semibold tracking-[-0.025em]">
        <Trans>Everyone has been answered.</Trans>
      </h1>
      <p className="mt-2 text-[15px] text-muted-foreground">
        <Trans>When a new message arrives, it will be here.</Trans>
      </p>
      <button type="button" onClick={() => openDrawer()} className="mt-6 h-9 rounded-full border bg-card px-3.5 text-sm hover:border-faint">
        <Trans>All conversations</Trans>
      </button>
    </div>
  )
}

export function QueuePage() {
  const { conversationId } = useParams()
  const queue = useQueue()
  const head = queue.waiting[0]?.id
  const id = conversationId ?? queue.currentId ?? head ?? null
  const { setCurrent, currentId } = queue
  useEffect(() => {
    if (!conversationId && !currentId && head) setCurrent(head)
  }, [conversationId, currentId, head, setCurrent])
  useViewing(id ?? undefined)
  return (
    <main className="mx-auto max-w-[700px] px-6 pt-7 pb-24 phone:px-4 phone:pt-4 phone:pb-[120px]" data-testid="queue">
      <AwayBanner />
      <CleanupBanner />
      {id ? (
        <QueueConversation key={id} id={id} />
      ) : queue.open.isPending ? (
        <div className="flex items-center gap-4">
          <Skeleton className="size-14 rounded-full" />
          <Skeleton className="h-7 w-48" />
        </div>
      ) : (
        <EmptyQueue />
      )}
    </main>
  )
}

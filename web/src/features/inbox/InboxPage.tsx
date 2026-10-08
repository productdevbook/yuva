import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, MessagesSquareIcon } from "lucide-react"
import { useCallback, useMemo, useRef, useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { PaneHeader } from "@/app/shell"
import { EmptyState } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { FEEDBACK_CATEGORIES, STATUSES, useEnumText } from "@/components/common/text"
import { ContactPanel } from "@/features/contact/ContactPanel"
import {
  ConversationList,
  type ListBase,
  type ListFilters,
} from "@/features/inbox/ConversationList"
import { ThreadView } from "@/features/conversation/ThreadView"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useViewLabels, VIEWS, type View } from "@/features/inbox/views"
import { useMediaQuery } from "@/hooks/use-media-query"
import { useIsMobile } from "@/hooks/use-media-query"
import { isGone, type ConversationStatus, type FeedbackCategory } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useConversation } from "@/features/conversation/queries"
import { useConversations } from "@/features/inbox/queries"
import { useInboxes, useLabels } from "@/lib/workspace"
import { useViewing } from "@/lib/realtime"
import { cn } from "@/lib/utils"

const FILTER_KEYS = ["status", "q", "inbox", "label", "assignee"] as const

function readFilters(sp: URLSearchParams): ListFilters {
  const status = sp.get("status")
  return {
    status: status === "all" || STATUSES.includes(status as ConversationStatus) ? (status as ListFilters["status"]) : "open",
    q: sp.get("q") ?? "",
    inbox: sp.get("inbox") ?? "",
    label: sp.get("label") ?? "",
    assignee: sp.get("assignee") ?? "",
  }
}

function toQuery(base: ListBase, f: ListFilters): ConversationFilters {
  const q: ConversationFilters = {}
  if (f.status !== "all") q.status = f.status
  if (f.q) q.q = f.q
  if (base.kind === "inbox") q.inbox_id = base.id
  else if (f.inbox) q.inbox_id = f.inbox
  if (base.kind === "label") q.label_id = base.id
  else if (f.label) q.label_id = f.label
  if (base.kind === "feedback") {
    q.kind = "feedback"
    if (base.id !== "all") q.category = base.id as FeedbackCategory
  }
  if (base.kind === "view") {
    if (base.id === "mine") q.assignee = "me"
    if (base.id === "unassigned") q.assignee = "unassigned"
    if (base.id === "spam") q.spam = true
  } else if (f.assignee) q.assignee = f.assignee
  return q
}

export function OpenConversation() {
  const { conversationId } = useParams()
  return <Navigate to={`/all/${conversationId}`} replace />
}

export function InboxPage() {
  const params = useParams()
  if (params.view !== undefined && !VIEWS.includes(params.view as View)) return <Navigate to="/all" replace />
  if (
    params.category !== undefined &&
    params.category !== "all" &&
    !FEEDBACK_CATEGORIES.includes(params.category as FeedbackCategory)
  ) {
    return <Navigate to="/feedback/all" replace />
  }
  const base: ListBase = params.inboxId
    ? { kind: "inbox", id: params.inboxId }
    : params.labelId
      ? { kind: "label", id: params.labelId }
      : params.category
        ? { kind: "feedback", id: params.category }
        : { kind: "view", id: params.view ?? "all" }
  return <Inbox key={`${base.kind}:${base.id}`} base={base} conversationId={params.conversationId} />
}

function Inbox({ base, conversationId }: { base: ListBase; conversationId?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const isMobile = useIsMobile()
  const isWide = useMediaQuery("(min-width: 1280px)")
  const [paneOpen, setPaneOpen] = useState(true)
  const [sheetOpen, setSheetOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const viewLabels = useViewLabels()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []

  const filters = readFilters(sp)
  const list = useConversations(toQuery(base, filters))
  const conversations = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const selected = useConversation(conversationId)
  useViewing(conversationId)

  const basePath = base.kind === "view" ? `/${base.id}` : `/${base.kind}/${base.id}`
  const listSearch = useMemo(() => {
    const s = new URLSearchParams()
    for (const k of FILTER_KEYS) {
      const v = sp.get(k)
      if (v) s.set(k, v)
    }
    const str = s.toString()
    return str ? `?${str}` : ""
  }, [sp])
  const hrefFor = useCallback((id: string) => `${basePath}/${id}${listSearch}`, [basePath, listSearch])
  const backHref = `${basePath}${listSearch}`

  const setFilters = useCallback(
    (patch: Partial<ListFilters>) => {
      const next = new URLSearchParams(sp)
      for (const [k, v] of Object.entries(patch)) {
        if (v === "" || (k === "status" && v === "open")) next.delete(k)
        else next.set(k, v)
      }
      setSp(next, { replace: true })
    },
    [sp, setSp],
  )

  const mobileContact = isMobile && sp.get("contact") === "1"
  const toggleContact = () => {
    if (isMobile) {
      if (mobileContact) navigate(-1)
      else {
        const next = new URLSearchParams(sp)
        next.set("contact", "1")
        setSp(next)
      }
    } else if (isWide) setPaneOpen((o) => !o)
    else setSheetOpen((o) => !o)
  }

  const move = (delta: number) => {
    if (conversations.length === 0) return
    const i = conversations.findIndex((c) => c.id === conversationId)
    const next = i === -1 ? 0 : Math.min(Math.max(i + delta, 0), conversations.length - 1)
    navigate(hrefFor(conversations[next].id), { replace: true })
  }
  useHotkeys({
    [SHORTCUTS.next]: () => move(1),
    [SHORTCUTS.previous]: () => move(-1),
    [SHORTCUTS.search]: () => searchRef.current?.focus(),
  })

  const category = base.kind === "feedback" && base.id !== "all" ? text.category[base.id as FeedbackCategory] : ""
  const title =
    base.kind === "feedback"
      ? base.id === "all"
        ? t`Feedback`
        : t`Feedback: ${category}`
      : base.kind === "view"
      ? viewLabels[base.id as View]
      : base.kind === "inbox"
        ? (inboxes.find((i) => i.id === base.id)?.name ?? "")
        : (labels.find((l) => l.id === base.id)?.name ?? "")

  const contactId =
    selected.data?.id === conversationId && !isGone(selected.error) ? selected.data?.contact_id : undefined
  const contactPanel =
    conversationId && contactId ? (
      <ContactPanel contactId={contactId} conversationId={conversationId} hrefFor={hrefFor} />
    ) : null

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PaneHeader title={title} />
      <div className="flex min-h-0 flex-1">
        <aside
          className={cn(
            "flex min-h-0 w-full flex-col border-r md:w-80 md:shrink-0 lg:w-96",
            conversationId && "hidden md:flex",
          )}
          aria-label={t`Conversations`}
        >
          <ConversationList
            ref={searchRef}
            base={base}
            filters={filters}
            setFilters={setFilters}
            conversations={conversations}
            selectedId={conversationId}
            hrefFor={hrefFor}
            isPending={list.isPending}
            error={list.error}
            hasNextPage={list.hasNextPage}
            isFetchingNextPage={list.isFetchingNextPage}
            fetchNextPage={() => void list.fetchNextPage()}
          />
        </aside>
        {conversationId ? (
          <>
            <section className={cn("flex min-h-0 min-w-0 flex-1", mobileContact && "hidden")}>
              <ThreadView
                key={conversationId}
                conversationId={conversationId}
                backHref={backHref}
                hrefFor={hrefFor}
                onToggleContact={toggleContact}
                contactShown={isMobile ? mobileContact : isWide ? paneOpen : sheetOpen}
              />
            </section>
            {mobileContact && (
              <section className="flex min-h-0 min-w-0 flex-1 flex-col md:hidden">
                <div className="flex h-12 shrink-0 items-center gap-2 border-b px-3">
                  <Button variant="ghost" size="icon-sm" onClick={() => navigate(-1)} aria-label={t`Back`}>
                    <ArrowLeftIcon />
                  </Button>
                  <h2 className="text-sm font-medium">
                    <Trans>Contact</Trans>
                  </h2>
                </div>
                {contactPanel}
              </section>
            )}
            {isWide && paneOpen && (
              <aside className="flex min-h-0 w-80 shrink-0 flex-col border-l" aria-label={t`Contact`}>
                {contactPanel}
              </aside>
            )}
            {!isMobile && !isWide && (
              <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
                <SheetContent side="right" className="w-80 gap-0 p-0 sm:max-w-80">
                  <SheetHeader className="border-b">
                    <SheetTitle>
                      <Trans>Contact</Trans>
                    </SheetTitle>
                  </SheetHeader>
                  {contactPanel}
                </SheetContent>
              </Sheet>
            )}
          </>
        ) : (
          <EmptyState
            icon={MessagesSquareIcon}
            title={<Trans>Pick a conversation</Trans>}
            className="hidden md:flex"
          >
            <Trans>Use j and k to move through the list, ? for all shortcuts.</Trans>
          </EmptyState>
        )}
      </div>
    </div>
  )
}

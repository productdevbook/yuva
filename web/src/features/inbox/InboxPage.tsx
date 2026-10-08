import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, MessagesSquareIcon } from "lucide-react"
import { useCallback, useMemo, useRef, useState } from "react"
import { Navigate, useNavigate, useParams, useSearchParams } from "react-router"

import { PaneHeader } from "@/app/shell"
import { EmptyState } from "@/components/common"
import { SHORTCUTS } from "@/components/common/ShortcutSheet"
import { FEEDBACK_CATEGORIES, useEnumText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { ContactPanel } from "@/features/contact/ContactPanel"
import { useConversation } from "@/features/conversation/queries"
import { ThreadView } from "@/features/conversation/ThreadView"
import { ConversationList } from "@/features/inbox/ConversationList"
import { usePanePreference } from "@/features/inbox/usePanePreference"
import { useConversations } from "@/features/inbox/queries"
import { basePath, FILTER_KEYS, readFilters, toQuery, useViewLabels, VIEWS, type ListBase, type ListFilters, type View } from "@/features/inbox/views"
import { useHotkeys } from "@/hooks/use-hotkeys"
import { useIsMobile, useMediaQuery } from "@/hooks/use-media-query"
import { isGone, type FeedbackCategory } from "@/lib/api"
import { useViewing } from "@/lib/realtime"
import { cn } from "@/lib/utils"
import { useInboxes, useLabels } from "@/lib/workspace"

export function OpenConversation() {
  const { conversationId } = useParams()
  return <Navigate to={`/all/${conversationId}`} replace />
}

export function InboxPage() {
  const params = useParams()
  if (params.view !== undefined && !VIEWS.includes(params.view as View)) return <Navigate to="/all" replace />
  if (params.category !== undefined && params.category !== "all" && !FEEDBACK_CATEGORIES.includes(params.category as FeedbackCategory)) {
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

function useTitle(base: ListBase) {
  const { t } = useLingui()
  const viewLabels = useViewLabels()
  const text = useEnumText()
  const inboxes = useInboxes().data ?? []
  const labels = useLabels().data ?? []
  if (base.kind === "feedback") {
    if (base.id === "all") return t`Feedback`
    const category = text.category[base.id as FeedbackCategory]
    return t`Feedback: ${category}`
  }
  if (base.kind === "view") return viewLabels[base.id as View]
  if (base.kind === "inbox") return inboxes.find((i) => i.id === base.id)?.name ?? ""
  return labels.find((l) => l.id === base.id)?.name ?? ""
}

function Inbox({ base, conversationId }: { base: ListBase; conversationId?: string }) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const [sp, setSp] = useSearchParams()
  const isMobile = useIsMobile()
  const isWide = useMediaQuery("(min-width: 1280px)")
  const [paneOpen, togglePane] = usePanePreference()
  const [sheetOpen, setSheetOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const title = useTitle(base)

  const filters = readFilters(sp)
  const list = useConversations(toQuery(base, filters))
  const conversations = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  const selected = useConversation(conversationId)
  useViewing(conversationId)

  const listSearch = useMemo(() => {
    const s = new URLSearchParams()
    for (const k of FILTER_KEYS) {
      const v = sp.get(k)
      if (v) s.set(k, v)
    }
    const str = s.toString()
    return str ? `?${str}` : ""
  }, [sp])
  const root = basePath(base)
  const hrefFor = useCallback((id: string) => `${root}/${id}${listSearch}`, [root, listSearch])
  const backHref = `${root}${listSearch}`

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
    } else if (isWide) togglePane()
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

  const contactId = selected.data?.id === conversationId && !isGone(selected.error) ? selected.data?.contact_id : undefined
  const contactPanel =
    conversationId && contactId ? <ContactPanel contactId={contactId} conversationId={conversationId} hrefFor={hrefFor} /> : null

  return (
    <div className="flex min-h-0 flex-1">
      <section
        className={cn("flex min-h-0 w-full flex-col border-e md:w-[22rem] md:shrink-0 xl:w-96", conversationId && "hidden md:flex")}
        aria-label={t`Conversations`}
      >
        <ConversationList
          ref={searchRef}
          base={base}
          title={title}
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
      </section>
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
              <PaneHeader
                title={<Trans>Contact</Trans>}
                leading={
                  <Button variant="ghost" size="icon-sm" className="-ms-1.5" onClick={() => navigate(-1)} aria-label={t`Back`}>
                    <ArrowLeftIcon />
                  </Button>
                }
                className="border-b"
              />
              {contactPanel}
            </section>
          )}
          {isWide && paneOpen && (
            <aside className="flex min-h-0 w-80 shrink-0 flex-col border-s" aria-label={t`Contact`}>
              {contactPanel}
            </aside>
          )}
          {!isMobile && !isWide && (
            <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
              <SheetContent side="right">
                <SheetHeader>
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
        <EmptyState icon={MessagesSquareIcon} title={<Trans>Pick a conversation</Trans>} className="hidden md:flex">
          <Trans>Use j and k to move through the list, ? for all shortcuts.</Trans>
        </EmptyState>
      )}
    </div>
  )
}

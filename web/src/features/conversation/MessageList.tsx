import { Trans, useLingui } from "@lingui/react/macro"
import { useCallback, useEffect, useLayoutEffect, useRef } from "react"

import { ErrorLine } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { ThreadContext } from "@/features/conversation/messages/context"
import { MessageItem } from "@/features/conversation/messages/MessageItem"
import { useMarkRead, useMessages } from "@/features/conversation/queries"
import type { Message } from "@/lib/api"
import { useSession } from "@/lib/session"

function dayLabel(iso: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: "full" }).format(new Date(iso))
}

const nearBottom = (el: HTMLElement) => el.scrollHeight - el.scrollTop - el.clientHeight < 48

export function MessageList({
  conversationId,
  messages,
  items,
  ctx,
}: {
  conversationId: string
  messages: ReturnType<typeof useMessages>
  items: Message[]
  ctx: ThreadContext
}) {
  const { i18n } = useLingui()
  const { membership } = useSession()
  const scroller = useRef<HTMLDivElement>(null)
  const top = useRef<HTMLDivElement>(null)
  const firstId = items[0]?.id
  const lastId = items[items.length - 1]?.id
  const lastMine = items[items.length - 1]?.author.member_id === membership.member_id
  const atBottom = useRef(true)
  const shown = useRef<{ first?: string; last?: string; height: number }>({ height: 0 })

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || !lastId) return
    const prev = shown.current
    if (!prev.last) el.scrollTop = el.scrollHeight
    else if (firstId !== prev.first && lastId === prev.last) el.scrollTop += el.scrollHeight - prev.height
    else if (lastId !== prev.last && (atBottom.current || lastMine)) el.scrollTop = el.scrollHeight
    shown.current = { first: firstId, last: lastId, height: el.scrollHeight }
    atBottom.current = nearBottom(el)
  }, [firstId, lastId, lastMine])

  const { mutate: mark } = useMarkRead(conversationId)
  const markedId = useRef<string | undefined>(undefined)
  const maybeMarkRead = useCallback(() => {
    if (!lastId || !scroller.current || markedId.current === lastId) return
    if (!atBottom.current || document.visibilityState !== "visible") return
    markedId.current = lastId
    mark(lastId)
  }, [lastId, mark])
  useEffect(() => {
    maybeMarkRead()
    document.addEventListener("visibilitychange", maybeMarkRead)
    return () => document.removeEventListener("visibilitychange", maybeMarkRead)
  }, [maybeMarkRead])

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = messages
  useEffect(() => {
    const el = top.current
    if (!el || !hasNextPage || isFetchingNextPage) return
    const io = new IntersectionObserver((entries) => entries[0]?.isIntersecting && void fetchNextPage(), {
      root: scroller.current,
      rootMargin: "300px 0px 0px 0px",
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, firstId])

  let lastDay = ""
  return (
    <div
      ref={scroller}
      onScroll={() => {
        const el = scroller.current
        if (!el) return
        atBottom.current = nearBottom(el)
        if (atBottom.current) maybeMarkRead()
      }}
      className="min-h-0 flex-1 overflow-y-auto [overflow-anchor:none]"
      data-testid="messages"
    >
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-6 sm:px-6">
        {hasNextPage && (
          <div ref={top} className="flex justify-center">
            {isFetchingNextPage ? (
              <Skeleton className="h-4 w-32" />
            ) : (
              <Button variant="ghost" size="sm" onClick={() => void fetchNextPage()}>
                <Trans>Load older messages</Trans>
              </Button>
            )}
          </div>
        )}
        {messages.isPending ? (
          <>
            <Skeleton className="h-14 w-2/3" />
            <Skeleton className="ms-auto h-14 w-1/2" />
          </>
        ) : messages.error ? (
          <ErrorLine error={messages.error} />
        ) : (
          items.map((m) => {
            const day = dayLabel(m.created_at, i18n.locale)
            const sep = day !== lastDay
            lastDay = day
            return (
              <div key={m.id} className="flex flex-col gap-4">
                {sep && <p className="pt-2 text-center text-xs text-faint">{day}</p>}
                <MessageItem m={m} ctx={ctx} />
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}

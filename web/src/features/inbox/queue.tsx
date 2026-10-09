import { createContext, useCallback, useContext, useEffect, useMemo, useState, useSyncExternalStore } from "react"
import { useLocation, useNavigate } from "react-router"

import { useConversations } from "@/features/inbox/queries"
import type { ConversationListItem } from "@/lib/api"
import { useSession } from "@/lib/session"
import { useInboxes } from "@/lib/workspace"

const MAX_PAGES = 5

function filterKey(ws: string) {
  return `inbox-filter:${ws}`
}

const filterListeners = new Set<() => void>()

function readFilter(ws: string): string {
  try {
    return localStorage.getItem(filterKey(ws)) ?? ""
  } catch {
    return ""
  }
}

export function useInboxFilter() {
  const { workspaceId: ws } = useSession()
  const inboxes = useInboxes().data
  const stored = useSyncExternalStore(
    (l) => {
      filterListeners.add(l)
      return () => filterListeners.delete(l)
    },
    () => readFilter(ws),
  )
  const value = stored && inboxes && !inboxes.some((i) => i.id === stored) ? "" : stored
  const set = useCallback(
    (id: string) => {
      try {
        if (id) localStorage.setItem(filterKey(ws), id)
        else localStorage.removeItem(filterKey(ws))
      } catch {
        // Storage can be blocked; the filter then lasts until the next render only.
      }
      filterListeners.forEach((l) => l())
    },
    [ws],
  )
  return [value, set] as const
}

export function waitingSince(c: ConversationListItem) {
  return c.last_message_at ?? c.created_at
}

function byWait(a: ConversationListItem, b: ConversationListItem) {
  const x = waitingSince(a)
  const y = waitingSince(b)
  if (x !== y) return x < y ? -1 : 1
  return a.id < b.id ? -1 : 1
}

type Queue = {
  waiting: ConversationListItem[]
  team: ConversationListItem[]
  open: ReturnType<typeof useConversations>
  inboxId: string
  currentId: string | null
  setCurrent: (id: string | null) => void
  leaving: Set<string>
  leave: (id: string) => void
  unleave: (id: string) => void
  defer: (id: string) => void
  nextAfter: (id: string | null) => ConversationListItem | undefined
  advance: (fromId: string) => void
  show: (id: string) => void
}

const QueueContext = createContext<Queue | null>(null)

export function useQueue() {
  const q = useContext(QueueContext)
  if (!q) throw new Error("useQueue outside QueueProvider")
  return q
}

export function QueueProvider({ children }: { children: React.ReactNode }) {
  const { membership } = useSession()
  const me = membership.member_id
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const [inboxId] = useInboxFilter()
  const open = useConversations({ status: "open" })
  const [currentId, setCurrent] = useState<string | null>(null)
  const [leaving, setLeaving] = useState<Set<string>>(() => new Set())
  const [deferred, setDeferred] = useState<string[]>([])

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = open
  const pages = open.data?.pages.length ?? 0
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage && pages < MAX_PAGES) void fetchNextPage()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, pages])

  const { waiting, team } = useMemo(() => {
    const all = (open.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => !inboxId || c.inbox_id === inboxId)
    const mine = all.filter((c) => (!c.assignee_id || c.assignee_id === me) && !leaving.has(c.id)).sort(byWait)
    const rank = (c: ConversationListItem) => deferred.indexOf(c.id)
    const fresh = mine.filter((c) => rank(c) < 0)
    const later = mine.filter((c) => rank(c) >= 0).sort((a, b) => rank(a) - rank(b))
    return {
      waiting: [...fresh, ...later],
      team: all.filter((c) => c.assignee_id && c.assignee_id !== me).sort(byWait),
    }
  }, [open.data, inboxId, me, leaving, deferred])

  const leave = useCallback((id: string) => setLeaving((s) => new Set(s).add(id)), [])
  const unleave = useCallback(
    (id: string) =>
      setLeaving((s) => {
        const n = new Set(s)
        n.delete(id)
        return n
      }),
    [],
  )
  const defer = useCallback((id: string) => setDeferred((d) => [...d.filter((x) => x !== id), id]), [])

  const nextAfter = useCallback(
    (id: string | null) => {
      const rest = waiting.filter((c) => c.id !== id)
      const i = waiting.findIndex((c) => c.id === id)
      if (i < 0) return rest[0]
      return waiting.slice(i + 1).find((c) => c.id !== id) ?? rest[0]
    },
    [waiting],
  )

  const show = useCallback(
    (id: string) => {
      setCurrent(id)
      if (pathname !== "/") navigate("/")
    },
    [navigate, pathname],
  )

  const advance = useCallback(
    (fromId: string) => {
      const next = nextAfter(fromId)
      setCurrent(next?.id ?? null)
      if (pathname !== "/") navigate("/")
    },
    [nextAfter, navigate, pathname],
  )

  const value = useMemo<Queue>(
    () => ({ waiting, team, open, inboxId, currentId, setCurrent, leaving, leave, unleave, defer, nextAfter, advance, show }),
    [waiting, team, open, inboxId, currentId, leaving, leave, unleave, defer, nextAfter, advance, show],
  )
  return <QueueContext.Provider value={value}>{children}</QueueContext.Provider>
}

import { keepPreviousData, useQueries, useQuery } from "@tanstack/react-query"

import {
  api,
  unwrap,
  type ChannelKind,
  type ConversationListItem,
  type ReportGranularity,
  type ReportGroupBy,
} from "@/lib/api"
import { saveDownload } from "@/lib/download"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

const PAGES = 10

export function dayStart(daysAgo: number, from = new Date()) {
  const d = new Date(from)
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() - daysAgo)
  return d
}

export function dayKey(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

export function statsQuery(ws: string, inboxId: string, since: string | undefined) {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return {
    queryKey: since ? [...keys.stats(ws), inboxId, timezone, since] : [...keys.stats(ws), inboxId, timezone],
    queryFn: () =>
      unwrap(api.GET("/v1/stats", { params: { query: { inbox_id: inboxId || undefined, timezone, since } } })),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  }
}

export function useWindowStats(inboxId: string, since: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery(statsQuery(ws, inboxId, since))
}

export function useInboxStats(inboxIds: string[], since: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQueries({ queries: inboxIds.map((id) => statsQuery(ws, id, since)) })
}

export function useRecentConversations(since: string, inboxId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.stats(ws), "recent", since, inboxId],
    queryFn: async () => {
      const items: ConversationListItem[] = []
      let cursor: string | undefined
      for (let i = 0; i < PAGES; i++) {
        const page = await unwrap(
          api.GET("/v1/conversations", { params: { query: { inbox_id: inboxId || undefined, cursor, limit: 100 } } })
        )
        const fresh = page.items.filter((c) => (c.last_activity_at ?? c.created_at ?? "") >= since)
        items.push(...fresh)
        cursor = page.next_cursor
        if (!cursor || fresh.length < page.items.length) return { items, complete: true }
      }
      return { items, complete: false }
    },
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })
}

export function createdSince(items: ConversationListItem[], since: string) {
  return items.filter((c) => (c.created_at ?? "") >= since)
}

export function ratedSince(items: ConversationListItem[], since: string) {
  return items
    .filter((c) => c.rating && c.rating.rated_at >= since)
    .sort((a, b) => b.rating!.rated_at.localeCompare(a.rating!.rated_at))
}

export function share(good: number, bad: number) {
  return good + bad > 0 ? good / (good + bad) : undefined
}

export type ReportQuery = {
  from: string
  to: string
  granularity: ReportGranularity
  group_by?: ReportGroupBy
  inbox_id?: string
  channel_kind?: ChannelKind
  label_id?: string
  team_id?: string
  member_id?: string
}

function reportParams(q: ReportQuery) {
  return {
    ...q,
    inbox_id: q.inbox_id || undefined,
    label_id: q.label_id || undefined,
    team_id: q.team_id || undefined,
    member_id: q.member_id || undefined,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  }
}

export function useReport(q: ReportQuery, enabled = true) {
  const { workspaceId: ws } = useSession()
  const query = reportParams(q)
  return useQuery({
    queryKey: [...keys.stats(ws), "report", query],
    queryFn: () => unwrap(api.GET("/v1/reports", { params: { query } })),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
    enabled,
  })
}

export function downloadReport(q: ReportQuery) {
  return saveDownload(
    api.GET("/v1/reports/export", { params: { query: reportParams(q) }, parseAs: "blob" }),
    `yuva-report-${q.from}-${q.to}.csv`
  )
}

export function addDays(key: string, n: number) {
  const [y, m, d] = key.split("-").map(Number)
  return dayKey(new Date(y!, m! - 1, d! + n))
}

export function daysBetween(from: string, to: string) {
  const a = new Date(`${from}T00:00:00Z`).getTime()
  const b = new Date(`${to}T00:00:00Z`).getTime()
  return Math.round((b - a) / 86400000) + 1
}

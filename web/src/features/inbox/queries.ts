import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { patchConversation } from "@/features/conversation/queries"
import { api, unwrap, type ConversationBulkUpdate, type ConversationKind, type ConversationListItem } from "@/lib/api"
import { keys, type ConversationFilters, type DraftFilters } from "@/lib/keys"
import { applyEvent } from "@/lib/live"
import { isLive } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { useLiveContext } from "@/lib/workspace"

export function useConversations(filters: ConversationFilters, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.conversations(ws, filters),
    queryFn: ({ pageParam }) => {
      const { fields, ...rest } = filters
      const dynamic = Object.fromEntries(Object.entries(fields ?? {}).map(([k, v]) => [`field.${k}`, v]))
      return unwrap(
        api.GET("/v1/conversations", { params: { query: { ...rest, ...dynamic, cursor: pageParam, limit: 100 } } })
      )
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

export function useLoadedCount(filters: ConversationFilters, keep?: (c: ConversationListItem) => boolean) {
  const q = useConversations(filters)
  if (!q.data || q.hasNextPage) return undefined
  const items = q.data.pages.flatMap((p) => p.items)
  return keep ? items.filter(keep).length : items.length
}

export function useCounts(kind?: ConversationKind[]) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.counts(ws), kind ?? "all"],
    queryFn: () => unwrap(api.GET("/v1/conversations/counts", { params: { query: { kind } } })),
  })
}

export function useBulkUpdateConversations() {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (body: ConversationBulkUpdate) => unwrap(api.POST("/v1/conversations/bulk", { body })),
    onSuccess: (data, body) => {
      const type =
        body.trashed === undefined
          ? "conversation.updated"
          : body.trashed
            ? "conversation.trashed"
            : "conversation.restored"
      for (const c of data.updated) applyEvent(qc, live, { type, data: c })
      if (!isLive(live.ws)) {
        for (const c of data.updated) void qc.invalidateQueries({ queryKey: keys.messages(live.ws, c.id) })
      }
    },
  })
}

export function useBulkTeam() {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: async ({ ids, teamId }: { ids: string[]; teamId: string | null }) => {
      const results = await Promise.allSettled(ids.map((id) => patchConversation(id, { team_id: teamId })))
      for (const r of results) {
        if (r.status === "fulfilled") applyEvent(qc, live, { type: "conversation.updated", data: r.value })
      }
      const failed = results.find((r): r is PromiseRejectedResult => r.status === "rejected")
      if (failed) throw failed.reason
      return results.length
    },
  })
}

export function useStats(inboxId: string) {
  const { workspaceId: ws } = useSession()
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return useQuery({
    queryKey: [...keys.stats(ws), inboxId, timezone],
    queryFn: () => unwrap(api.GET("/v1/stats", { params: { query: { inbox_id: inboxId || undefined, timezone } } })),
    staleTime: 60_000,
  })
}

export function useMentions(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.mentions(ws),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/me/mentions", { params: { query: { cursor: pageParam, limit: 50 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

export function useMarkMentionSeen() {
  const { workspaceId: ws } = useSession()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (noteId: string) => unwrap(api.POST("/v1/me/mentions/{noteId}/seen", { params: { path: { noteId } } })),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.mentions(ws) }),
  })
}

export function useMarkAllMentionsSeen() {
  const { workspaceId: ws } = useSession()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => unwrap(api.POST("/v1/me/mentions/seen")),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.mentions(ws) }),
  })
}

export function useDrafts(filters: DraftFilters) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.drafts(ws, filters),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/drafts", { params: { query: { ...filters, cursor: pageParam, limit: 50 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

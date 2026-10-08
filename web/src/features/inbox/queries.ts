import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type ConversationBulkUpdate } from "@/lib/api"
import { keys, type ConversationFilters } from "@/lib/keys"
import { applyEvent } from "@/lib/live"
import { isLive } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { useLiveContext } from "@/lib/workspace"

export function useConversations(filters: ConversationFilters, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.conversations(ws, filters),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/conversations", { params: { query: { ...filters, cursor: pageParam, limit: 100 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled,
  })
}

export function useCounts() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.counts(ws),
    queryFn: () => unwrap(api.GET("/v1/conversations/counts")),
  })
}

export function useBulkUpdateConversations() {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (body: ConversationBulkUpdate) => unwrap(api.POST("/v1/conversations/bulk", { body })),
    onSuccess: (data) => {
      for (const c of data.updated) applyEvent(qc, live, { type: "conversation.updated", data: c })
      if (!isLive(live.ws)) {
        for (const c of data.updated) void qc.invalidateQueries({ queryKey: keys.messages(live.ws, c.id) })
      }
    },
  })
}

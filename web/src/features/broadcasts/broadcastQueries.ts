import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  api,
  ApiError,
  unwrap,
  type Broadcast,
  type BroadcastBulkChange,
  type BroadcastDeliveryState,
  type BroadcastProblem,
  type BroadcastState,
  type BroadcastUpdate,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { setBroadcast } from "@/lib/live"
import { meKey, useSession } from "@/lib/session"

export function broadcastLink(id: string, tab?: string) {
  return `/broadcasts/${id}${tab ? `?tab=${tab}` : ""}`
}

export const ACTIVE: BroadcastState[] = ["preparing", "sending", "paused"]
export const FINISHED: BroadcastState[] = ["sent", "cancelled", "failed"]

export function isActive(b: Broadcast) {
  return ACTIVE.includes(b.state)
}

export function useBroadcasts(inboxId: string, states?: BroadcastState[]) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.broadcastPages(ws, inboxId, states?.join(",") ?? ""),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/broadcasts", {
          params: { query: { inbox_id: inboxId || undefined, state: states, cursor: pageParam, limit: 100 } },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useBroadcast(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.broadcast(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/broadcasts/{broadcastId}", { params: { path: { broadcastId: id! } } })),
    enabled: !!id,
    retry: (n, err) => !(err instanceof ApiError && err.status < 500) && n < 2,
  })
}

function useSet() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return (b: Broadcast) => setBroadcast(qc, ws, b)
}

export function useCreateBroadcast() {
  const set = useSet()
  return useMutation({
    mutationFn: (body: { inbox_id: string; name: string; channel_id?: string }) =>
      unwrap(api.POST("/v1/broadcasts", { body })),
    onSuccess: set,
  })
}

export function useUpdateBroadcast(id: string) {
  const set = useSet()
  return useMutation({
    mutationFn: (body: BroadcastUpdate) =>
      unwrap(api.PATCH("/v1/broadcasts/{broadcastId}", { params: { path: { broadcastId: id } }, body })),
    onSuccess: set,
  })
}

export function useDeleteBroadcast() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (id: string) =>
      unwrap(api.DELETE("/v1/broadcasts/{broadcastId}", { params: { path: { broadcastId: id } } })),
    onSuccess: (_, id) => {
      qc.removeQueries({ queryKey: keys.broadcast(ws, id) })
      void qc.invalidateQueries({ queryKey: [...keys.broadcasts(ws), "pages"] })
    },
  })
}

export function useBulkBroadcasts() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: BroadcastBulkChange) => unwrap(api.POST("/v1/broadcasts/bulk", { body })),
    onSuccess: (result) => {
      for (const id of result.succeeded) qc.removeQueries({ queryKey: keys.broadcast(ws, id) })
      void qc.invalidateQueries({ queryKey: [...keys.broadcasts(ws), "pages"] })
    },
  })
}

export function useAudienceCount(b: Broadcast, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.broadcastAudience(ws, b.id, JSON.stringify([b.audience, b.design_id, b.subject, b.updated_at])),
    queryFn: () =>
      unwrap(api.POST("/v1/broadcasts/{broadcastId}/audience", { params: { path: { broadcastId: b.id } } })),
    enabled: enabled && b.audience.list_ids.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  })
}

export function useReview(b: Broadcast, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.broadcastReview(ws, b.id), b.updated_at],
    queryFn: () => unwrap(api.GET("/v1/broadcasts/{broadcastId}/review", { params: { path: { broadcastId: b.id } } })),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useSendTest(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: { member_ids?: string[]; contact_id?: string }) =>
      unwrap(api.POST("/v1/broadcasts/{broadcastId}/test", { params: { path: { broadcastId: id } }, body })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.broadcast(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.broadcastReview(ws, id) })
    },
  })
}

export function useSchedule(id: string) {
  const set = useSet()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: { send_at?: string; confirm_recipients?: number }) =>
      unwrap(api.POST("/v1/broadcasts/{broadcastId}/schedule", { params: { path: { broadcastId: id } }, body })),
    onSuccess: set,
    onError: () => {
      void qc.invalidateQueries({ queryKey: keys.broadcast(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.broadcastReview(ws, id) })
      void qc.invalidateQueries({ queryKey: [...keys.broadcasts(ws), "audience", id] })
    },
  })
}

export function problemOf(err: unknown): BroadcastProblem | undefined {
  return err instanceof ApiError ? (err.body as BroadcastProblem | undefined) : undefined
}

export type BroadcastAction = "unschedule" | "pause" | "resume" | "cancel"

export function useBroadcastAction(id: string) {
  const set = useSet()
  return useMutation({
    mutationFn: (action: BroadcastAction) => {
      const params = { params: { path: { broadcastId: id } } }
      switch (action) {
        case "unschedule":
          return unwrap(api.POST("/v1/broadcasts/{broadcastId}/unschedule", params))
        case "pause":
          return unwrap(api.POST("/v1/broadcasts/{broadcastId}/pause", params))
        case "resume":
          return unwrap(api.POST("/v1/broadcasts/{broadcastId}/resume", params))
        case "cancel":
          return unwrap(api.POST("/v1/broadcasts/{broadcastId}/cancel", params))
      }
    },
    onSuccess: set,
  })
}

export function useReport(id: string, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.broadcastReport(ws, id),
    queryFn: () => unwrap(api.GET("/v1/broadcasts/{broadcastId}/report", { params: { path: { broadcastId: id } } })),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useDeliveries(id: string, state: BroadcastDeliveryState | "", q: string, replied: boolean) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.broadcastDeliveries(ws, id, state, q, replied),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/broadcasts/{broadcastId}/deliveries", {
          params: {
            path: { broadcastId: id },
            query: {
              state: state || undefined,
              q: q || undefined,
              replied: replied || undefined,
              cursor: pageParam,
              limit: 50,
            },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    placeholderData: keepPreviousData,
  })
}

export function useRequestApproval() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: { website: string; sender_description: string; consent_description: string }) =>
      unwrap(api.POST("/v1/workspace/broadcast-approval", { body })),
    onSuccess: (w) => {
      qc.setQueryData(keys.workspace(ws), w)
      void qc.invalidateQueries({ queryKey: meKey })
    },
  })
}

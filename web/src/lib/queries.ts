import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  api,
  unwrap,
  type ConversationUpdate,
  type MessageCreate,
} from "@/lib/api"
import { keys, type ConversationFilters } from "@/lib/keys"
import { applyEvent, applyRead } from "@/lib/live"
import { isLive } from "@/lib/realtime"
import { useSession } from "@/lib/session"

export function useMembers() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.members(ws),
    queryFn: () => unwrap(api.GET("/v1/members")).then((r) => r.items),
    staleTime: 60_000,
    refetchOnWindowFocus: "always",
  })
}

export function useMemberMap() {
  const members = useMembers().data
  return new Map((members ?? []).map((m) => [m.id, m]))
}

export function useInboxes() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.inboxes(ws),
    queryFn: () => unwrap(api.GET("/v1/inboxes")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useInboxMembers(inboxId: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.inboxMembers(ws, inboxId ?? ""),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/members", { params: { path: { inboxId: inboxId! } } })).then(
        (r) => r.items,
      ),
    enabled: !!inboxId,
    staleTime: 60_000,
  })
}

export function useAssignableMembers(inboxId: string | undefined) {
  const members = useMembers().data ?? []
  const granted = new Set((useInboxMembers(inboxId).data ?? []).map((m) => m.id))
  return members.filter((m) => m.role !== "agent" || granted.has(m.id))
}

export function useLabels() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.labels(ws),
    queryFn: () => unwrap(api.GET("/v1/labels")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useCannedReplies() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.cannedReplies(ws),
    queryFn: () => unwrap(api.GET("/v1/canned-replies")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useContact(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contact(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/contacts/{contactId}", { params: { path: { contactId: id! } } })),
    enabled: !!id,
    staleTime: 60_000,
  })
}

export function useConversations(filters: ConversationFilters) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.conversations(ws, filters),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/conversations", { params: { query: { ...filters, cursor: pageParam, limit: 25 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useCounts() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.counts(ws),
    queryFn: () => unwrap(api.GET("/v1/conversations/counts")),
  })
}

export function useConversation(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.conversation(ws, id ?? ""),
    queryFn: () =>
      unwrap(api.GET("/v1/conversations/{conversationId}", { params: { path: { conversationId: id! } } })),
    enabled: !!id,
  })
}

export const MESSAGE_PAGE = 50

export function useMessages(conversationId: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.messages(ws, conversationId ?? ""),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/conversations/{conversationId}/messages", {
          params: {
            path: { conversationId: conversationId! },
            query: { order: "desc", cursor: pageParam, limit: MESSAGE_PAGE },
          },
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    enabled: !!conversationId,
  })
}

export function useMarkRead(conversationId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (messageId: string) =>
      unwrap(
        api.POST("/v1/conversations/{conversationId}/read", {
          params: { path: { conversationId } },
          body: { message_id: messageId },
        }),
      ),
    onSuccess: (data) => applyRead(qc, ws, data),
  })
}

export function useUpdateConversation(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws, membership } = useSession()
  const live = { ws, memberId: membership.member_id }
  return useMutation({
    mutationFn: (body: ConversationUpdate) =>
      unwrap(api.PATCH("/v1/conversations/{conversationId}", { params: { path: { conversationId: id } }, body })),
    onSuccess: (data) => {
      applyEvent(qc, live, { type: "conversation.updated", data })
      if (!isLive(ws)) void qc.invalidateQueries({ queryKey: keys.messages(ws, id) })
    },
  })
}

export type Outgoing = MessageCreate & { client_id: string; files: File[] }

export function useSendMessage(conversationId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws, membership } = useSession()
  const live = { ws, memberId: membership.member_id }
  return useMutation({
    mutationFn: ({ files, ...body }: Outgoing) => {
      const path = { params: { path: { conversationId } } }
      if (files.length === 0) {
        return unwrap(api.POST("/v1/conversations/{conversationId}/messages", { ...path, body }))
      }
      return unwrap(
        api.POST("/v1/conversations/{conversationId}/messages", {
          ...path,
          body: { ...body, files: [] },
          bodySerializer: () => {
            const form = new FormData()
            for (const [k, v] of Object.entries(body)) if (v !== undefined && v !== "") form.append(k, String(v))
            for (const f of files) form.append("files", f, f.name)
            return form
          },
        }),
      )
    },
    onSuccess: (data) => applyEvent(qc, live, { type: "message.created", data }),
  })
}

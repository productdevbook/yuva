import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type ConversationUpdate, type MessageCreate } from "@/lib/api"
import { keys } from "@/lib/keys"
import { applyEvent, applyRead } from "@/lib/live"
import { isLive } from "@/lib/realtime"
import { useSession } from "@/lib/session"
import { useLiveContext } from "@/lib/workspace"

export function useConversation(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.conversation(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/conversations/{conversationId}", { params: { path: { conversationId: id! } } })),
    enabled: !!id,
  })
}

const MESSAGE_PAGE = 50

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

export function useMessageEmail(id: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.messageEmail(ws, id),
    queryFn: () => unwrap(api.GET("/v1/messages/{messageId}/email", { params: { path: { messageId: id } } })),
    enabled,
    staleTime: Infinity,
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
  const live = useLiveContext()
  return useMutation({
    mutationFn: (body: ConversationUpdate) =>
      unwrap(api.PATCH("/v1/conversations/{conversationId}", { params: { path: { conversationId: id } }, body })),
    onSuccess: (data) => {
      applyEvent(qc, live, { type: "conversation.updated", data })
      if (!isLive(live.ws)) void qc.invalidateQueries({ queryKey: keys.messages(live.ws, id) })
    },
  })
}

export function useMoveConversation(id: string) {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (inboxId: string) =>
      unwrap(
        api.POST("/v1/conversations/{conversationId}/move", {
          params: { path: { conversationId: id } },
          body: { inbox_id: inboxId },
        }),
      ),
    onSuccess: (data) => {
      applyEvent(qc, live, { type: "conversation.updated", data })
      if (!isLive(live.ws)) void qc.invalidateQueries({ queryKey: keys.messages(live.ws, id) })
    },
  })
}

type Outgoing = MessageCreate & { client_id: string; files: File[] }

export function useSendMessage(conversationId: string) {
  const qc = useQueryClient()
  const live = useLiveContext()
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

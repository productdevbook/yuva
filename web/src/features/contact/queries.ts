import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type ContactUpdate, type ConversationCreate } from "@/lib/api"
import { keys } from "@/lib/keys"
import { applyEvent } from "@/lib/live"
import { useSession } from "@/lib/session"
import { useLiveContext } from "@/lib/workspace"

export function useContact(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contact(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/contacts/{contactId}", { params: { path: { contactId: id! } } })),
    enabled: !!id,
    staleTime: 60_000,
  })
}

export function useContactPresence(id: string | undefined) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contactPresence(ws, id ?? ""),
    queryFn: () => unwrap(api.GET("/v1/contacts/{contactId}/presence", { params: { path: { contactId: id! } } })),
    enabled: !!id,
    refetchOnWindowFocus: "always",
  })
}

export function useContactSearch(q: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.contactSearch(ws), q],
    queryFn: () =>
      unwrap(api.GET("/v1/contacts", { params: { query: { q: q || undefined, limit: 20 } } })).then((r) => r.items),
    enabled,
    placeholderData: (prev) => prev,
  })
}

export function useContactConversationCount(contactId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contactConversationCount(ws, contactId),
    queryFn: () =>
      unwrap(api.GET("/v1/conversations", { params: { query: { contact_id: contactId, limit: 100 } } })).then((r) => ({
        count: r.items.length,
        more: !!r.next_cursor,
      })),
  })
}

export function useMergeContact(targetId: string) {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (sourceId: string) =>
      unwrap(
        api.POST("/v1/contacts/{contactId}/merge", {
          params: { path: { contactId: targetId } },
          body: { source_id: sourceId },
        }),
      ),
    onSuccess: (data, sourceId) => {
      applyEvent(qc, live, { type: "contact.updated", data })
      applyEvent(qc, live, { type: "contact.deleted", data: { id: sourceId, merged_into_id: data.id } })
      void qc.invalidateQueries({ queryKey: keys.contactSearch(live.ws) })
    },
  })
}

export function useClearUndeliverable(contactId: string) {
  const update = useUpdateContact(contactId)
  return { ...update, mutate: (email: string) => update.mutate({ clear_undeliverable: [email] }) }
}

export type DirectoryFilter = "all" | "open" | "known" | "visitor"

export function useContactDirectory(q: string, filter: DirectoryFilter) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: [...keys.contactSearch(ws), "directory", q, filter],
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/contacts", {
          params: {
            query: {
              q: q || undefined,
              kind: filter === "known" || filter === "visitor" ? filter : undefined,
              has_open: filter === "open" ? true : undefined,
              sort: q ? undefined : "last_seen",
              cursor: pageParam,
              limit: 50,
            },
          },
        }),
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
    placeholderData: (prev) => prev,
  })
}

export function useContactSummary(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contactSummary(ws, id),
    queryFn: () => unwrap(api.GET("/v1/contacts/{contactId}/summary", { params: { path: { contactId: id } } })),
  })
}

export function useContactNotes(id: string) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.contactNotes(ws, id),
    queryFn: ({ pageParam }) =>
      unwrap(api.GET("/v1/contacts/{contactId}/notes", { params: { path: { contactId: id }, query: { cursor: pageParam, limit: 50 } } })),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useAddContactNote(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: string) => unwrap(api.POST("/v1/contacts/{contactId}/notes", { params: { path: { contactId: id } }, body: { body } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.contactNotes(ws, id) }),
  })
}

export function useDeleteContactNote(id: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (noteId: string) =>
      unwrap(api.DELETE("/v1/contacts/{contactId}/notes/{noteId}", { params: { path: { contactId: id, noteId } } })),
    onSettled: () => void qc.invalidateQueries({ queryKey: keys.contactNotes(ws, id) }),
  })
}

export function useUpdateContact(contactId: string) {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (body: ContactUpdate) => unwrap(api.PATCH("/v1/contacts/{contactId}", { params: { path: { contactId } }, body })),
    onSuccess: (data) => applyEvent(qc, live, { type: "contact.updated", data }),
  })
}

export function useCreateConversation() {
  const qc = useQueryClient()
  const live = useLiveContext()
  return useMutation({
    mutationFn: (body: ConversationCreate) => unwrap(api.POST("/v1/conversations", { body })),
    onSuccess: () => void qc.invalidateQueries({ queryKey: keys.conversationLists(live.ws) }),
  })
}

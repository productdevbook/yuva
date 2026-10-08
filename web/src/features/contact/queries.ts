import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Contact } from "@/lib/api"
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
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (email: string) =>
      unwrap(
        api.PATCH("/v1/contacts/{contactId}", {
          params: { path: { contactId } },
          body: { clear_undeliverable: [email] },
        }),
      ),
    onSuccess: (data) => qc.setQueryData<Contact>(keys.contact(ws, contactId), data),
  })
}

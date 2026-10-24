import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { store } from "@/features/docs/queries"
import { api, unwrap, type MailingList, type SubscriptionBulkChange, type SubscriptionState } from "@/lib/api"
import { keys, type GrowthDays } from "@/lib/keys"
import { useSession } from "@/lib/session"

export const GROWTH_WINDOWS: GrowthDays[] = [30, 90]

export const useGrowthDays = store<GrowthDays>(30)

export function listLink(id: string, tab?: string) {
  return `/broadcasts/lists/${id}${tab ? `?tab=${tab}` : ""}`
}

export function useLists(inboxId: string, archived: boolean) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.listPages(ws, inboxId, archived),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/lists", {
          params: {
            query: {
              inbox_id: inboxId || undefined,
              archived: archived || undefined,
              cursor: pageParam,
              limit: 100,
            },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useList(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.list(ws, id),
    queryFn: () => unwrap(api.GET("/v1/lists/{listId}", { params: { path: { listId: id } } })),
    enabled: !!id,
  })
}

export function useSetList() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return (l: MailingList) => {
    qc.setQueryData(keys.list(ws, l.id), l)
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "pages"] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "contact"] })
  }
}

export function useArchiveList(listId: string) {
  const setList = useSetList()
  return useMutation({
    mutationFn: (archived: boolean) =>
      unwrap(api.PATCH("/v1/lists/{listId}", { params: { path: { listId } }, body: { archived } })),
    onSuccess: setList,
  })
}

export function useGrowth(id: string, days: GrowthDays) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.listGrowth(ws, id, days),
    queryFn: () => unwrap(api.GET("/v1/lists/{listId}/growth", { params: { path: { listId: id }, query: { days } } })),
  })
}

export function useSubscriptions(listId: string, state: SubscriptionState | "", q: string) {
  const { workspaceId: ws } = useSession()
  return useInfiniteQuery({
    queryKey: keys.subscriptions(ws, listId, state, q),
    queryFn: ({ pageParam }) =>
      unwrap(
        api.GET("/v1/lists/{listId}/subscriptions", {
          params: {
            path: { listId },
            query: { state: state || undefined, q: q || undefined, cursor: pageParam, limit: 50 },
          },
        })
      ),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  })
}

export function useBulkSubscriptions(listId: string) {
  const refresh = useRefreshList(listId)
  return useMutation({
    mutationFn: (body: SubscriptionBulkChange) =>
      unwrap(api.POST("/v1/lists/{listId}/subscriptions/bulk", { params: { path: { listId } }, body })),
    onSuccess: () => refresh(),
  })
}

export function useSubscription(listId: string, id: string | null, enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.subscription(ws, listId, id ?? ""),
    queryFn: () =>
      unwrap(
        api.GET("/v1/lists/{listId}/subscriptions/{subscriptionId}", {
          params: { path: { listId, subscriptionId: id! } },
        })
      ),
    enabled: !!id && enabled,
  })
}

export function useRefreshList(listId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => {
    void qc.invalidateQueries({ queryKey: keys.list(ws, listId) })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "pages"] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "subscriptions", listId] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "subscription", listId] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "growth", listId] })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "lists", "contact"] })
  }
}

export function useContactSubscriptions(contactId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.contactSubscriptions(ws, contactId),
    queryFn: () =>
      unwrap(api.GET("/v1/contacts/{contactId}/subscriptions", { params: { path: { contactId } } })).then(
        (r) => r.items
      ),
  })
}

export function useInboxChannels(inboxId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.channels(ws, inboxId),
    queryFn: () =>
      unwrap(api.GET("/v1/inboxes/{inboxId}/channels", { params: { path: { inboxId } } })).then((r) => r.items),
    enabled: !!inboxId,
  })
}

export function useSender(inboxId: string, channelId?: string) {
  const channels = useInboxChannels(inboxId).data ?? []
  const c = channels.find((x) => x.id === channelId) ?? channels.find((x) => x.kind === "email")
  return c?.email ? c.email.from_address || c.email.address : undefined
}

export function useDeleteList() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/lists/{listId}", { params: { path: { listId: id } } })),
    onSuccess: (_, id) => {
      qc.removeQueries({ queryKey: keys.list(ws, id) })
      void qc.invalidateQueries({ queryKey: keys.lists(ws) })
    },
  })
}

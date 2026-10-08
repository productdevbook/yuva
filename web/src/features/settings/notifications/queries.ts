import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type NotificationEventsUpdate, type NotificationSettings } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useNotificationSettings() {
  const { workspaceId: ws } = useSession()
  return useQuery({ queryKey: keys.notifications(ws), queryFn: () => unwrap(api.GET("/v1/me/notifications")) })
}

export function useSaveSettings() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: { events?: NotificationEventsUpdate; email_delay_minutes?: number }) => unwrap(api.PATCH("/v1/me/notifications", { body })),
    onMutate: (body) => {
      const prev = qc.getQueryData<NotificationSettings>(keys.notifications(ws))
      if (prev) {
        qc.setQueryData<NotificationSettings>(keys.notifications(ws), {
          ...prev,
          events: { ...prev.events, ...body.events },
          email_delay_minutes: body.email_delay_minutes ?? prev.email_delay_minutes,
        })
      }
      return prev
    },
    onError: (_, __, prev) => prev && qc.setQueryData(keys.notifications(ws), prev),
    onSuccess: (data) => qc.setQueryData(keys.notifications(ws), data),
  })
}

export function useSetInboxOverride(inboxId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const set = (fn: (prev: NotificationSettings) => NotificationSettings) =>
    qc.setQueryData<NotificationSettings>(keys.notifications(ws), (prev) => prev && fn(prev))
  const put = useMutation({
    mutationFn: (events: NotificationEventsUpdate) =>
      unwrap(api.PUT("/v1/me/notifications/inboxes/{inboxId}", { params: { path: { inboxId } }, body: { events } })),
    onSuccess: (data) => set((prev) => ({ ...prev, inboxes: [...prev.inboxes.filter((o) => o.inbox_id !== inboxId), data] })),
  })
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/me/notifications/inboxes/{inboxId}", { params: { path: { inboxId } } })),
    onSuccess: () => set((prev) => ({ ...prev, inboxes: prev.inboxes.filter((o) => o.inbox_id !== inboxId) })),
  })
  return { put, remove }
}

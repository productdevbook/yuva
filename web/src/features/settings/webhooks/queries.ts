import { useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type WebhookEndpoint } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useWebhooks(inboxId?: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.webhookList(ws, inboxId ?? ""),
    queryFn: () => unwrap(api.GET("/v1/webhooks", { params: { query: inboxId ? { inbox_id: inboxId } : {} } })).then((r) => r.items),
  })
}

export function useWebhook(id: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.webhook(ws, id),
    queryFn: () => unwrap(api.GET("/v1/webhooks/{webhookId}", { params: { path: { webhookId: id } } })),
  })
}

export function useSetEndpoint() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return (e: WebhookEndpoint) => {
    qc.setQueryData(keys.webhook(ws, e.id), e)
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "list"] })
  }
}

export function useRefreshLog(endpointId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return () => {
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "deliveries", endpointId] })
    void qc.invalidateQueries({ queryKey: keys.webhookAttempts(ws, endpointId) })
    void qc.invalidateQueries({ queryKey: ["ws", ws, "webhooks", "delivery", endpointId] })
    void qc.invalidateQueries({ queryKey: keys.webhook(ws, endpointId) })
  }
}

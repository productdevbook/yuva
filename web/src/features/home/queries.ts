import { keepPreviousData, useQuery } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export type HomeScope = { days?: 7 | 14 | 30; inboxId?: string; timezone?: string }

export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"
  } catch {
    return "UTC"
  }
}

export function useHome(enabled = true, scope: HomeScope = {}) {
  const { workspaceId: ws } = useSession()
  const { days = 14, inboxId, timezone = browserTimezone() } = scope
  return useQuery({
    queryKey: [...keys.home(ws), days, inboxId ?? "", timezone],
    queryFn: () =>
      unwrap(api.GET("/v1/home", { params: { query: { days, inbox_id: inboxId || undefined, timezone } } })),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useActivity(inboxId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.home(ws), "activity", inboxId],
    queryFn: () => unwrap(api.GET("/v1/activity", { params: { query: { inbox_id: inboxId || undefined, limit: 6 } } })),
    placeholderData: keepPreviousData,
  })
}

export function useMigrationsNeedingAttention() {
  const { workspaceId: ws, canManage } = useSession()
  return useQuery({
    queryKey: keys.migrationAttention(ws),
    queryFn: () =>
      unwrap(api.GET("/v1/migrations", { params: { query: { state: ["needs_attention"], limit: 20 } } })).then(
        (r) => r.items
      ),
    enabled: canManage,
  })
}

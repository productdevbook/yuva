import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type InboxSlackUpdate } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useSlack(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.slack(ws),
    queryFn: () => unwrap(api.GET("/v1/slack")),
    enabled,
    retry: false,
  })
}

export function useSlackChannels(enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.slackChannels(ws),
    queryFn: () => unwrap(api.GET("/v1/slack/channels")).then((r) => r.items),
    enabled,
    retry: false,
  })
}

export function useSlackMutations() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.slack(ws) })
  return {
    connect: useMutation({ mutationFn: () => unwrap(api.POST("/v1/slack/connect")) }),
    update: useMutation({
      mutationFn: (reply_command: string) => unwrap(api.PATCH("/v1/slack", { body: { reply_command } })),
      onSuccess: refresh,
    }),
    disconnect: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/slack")),
      onSuccess: refresh,
    }),
  }
}

export function useInboxSlack(inbox: string, enabled: boolean) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.inboxSlack(ws, inbox),
    queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}/slack", { params: { path: { inboxId: inbox } } })),
    enabled,
    retry: false,
  })
}

export function useSetInboxSlack(inbox: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: InboxSlackUpdate) =>
      unwrap(api.PUT("/v1/inboxes/{inboxId}/slack", { params: { path: { inboxId: inbox } }, body })),
    onSuccess: (data) => qc.setQueryData(keys.inboxSlack(ws, inbox), data),
  })
}

export function useMySlack(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.mySlack(ws),
    queryFn: () => unwrap(api.GET("/v1/me/slack")),
    enabled,
    retry: false,
  })
}

export function useMySlackMutations() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.mySlack(ws) })
  return {
    start: useMutation({
      mutationFn: (email?: string) => unwrap(api.POST("/v1/me/slack/link", { body: email ? { email } : {} })),
      onSuccess: refresh,
    }),
    confirm: useMutation({
      mutationFn: (code: string) => unwrap(api.POST("/v1/me/slack/link/confirm", { body: { code } })),
      onSuccess: refresh,
    }),
    unlink: useMutation({
      mutationFn: () => unwrap(api.DELETE("/v1/me/slack")),
      onSuccess: refresh,
    }),
  }
}

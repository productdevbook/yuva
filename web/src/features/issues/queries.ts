import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  api,
  unwrap,
  type IssueIntegration,
  type IssueIntegrationCreate,
  type IssueIntegrationUpdate,
  type IssueLinkCreate,
} from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export type IssueSubject = { kind: "conversation" | "idea"; id: string }

export function useIssueIntegrations(enabled = true) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.issueIntegrations(ws),
    queryFn: () => unwrap(api.GET("/v1/issue-integrations")).then((r) => r.items),
    enabled,
    retry: false,
  })
}

export function useIssueIntegrationMutations() {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () => qc.invalidateQueries({ queryKey: keys.issueIntegrations(ws) })
  return {
    connect: useMutation({
      mutationFn: (body: IssueIntegrationCreate) => unwrap(api.POST("/v1/issue-integrations", { body })),
      onSuccess: refresh,
    }),
    update: useMutation({
      mutationFn: (v: { id: string; body: IssueIntegrationUpdate }) =>
        unwrap(
          api.PATCH("/v1/issue-integrations/{integrationId}", {
            params: { path: { integrationId: v.id } },
            body: v.body,
          })
        ),
      onSuccess: refresh,
    }),
    rotate: useMutation({
      mutationFn: (integration: IssueIntegration) =>
        unwrap(
          api.POST("/v1/issue-integrations/{integrationId}/webhook-secret", {
            params: { path: { integrationId: integration.id } },
          })
        ),
      onSuccess: refresh,
    }),
    disconnect: useMutation({
      mutationFn: (integration: IssueIntegration) =>
        unwrap(
          api.DELETE("/v1/issue-integrations/{integrationId}", {
            params: { path: { integrationId: integration.id } },
          })
        ),
      onSuccess: refresh,
    }),
  }
}

export function useIssueLinkMutations({ kind, id }: IssueSubject) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const refresh = () =>
    qc.invalidateQueries({ queryKey: kind === "conversation" ? keys.conversation(ws, id) : keys.discussions(ws) })
  return {
    add: useMutation({
      mutationFn: (body: IssueLinkCreate) =>
        unwrap(
          kind === "conversation"
            ? api.POST("/v1/conversations/{conversationId}/issues", {
                params: { path: { conversationId: id } },
                body,
              })
            : api.POST("/v1/discussions/{discussionId}/issues", { params: { path: { discussionId: id } }, body })
        ),
      onSuccess: refresh,
    }),
    remove: useMutation({
      mutationFn: (linkId: string) =>
        unwrap(
          kind === "conversation"
            ? api.DELETE("/v1/conversations/{conversationId}/issues/{issueLinkId}", {
                params: { path: { conversationId: id, issueLinkId: linkId } },
              })
            : api.DELETE("/v1/discussions/{discussionId}/issues/{issueLinkId}", {
                params: { path: { discussionId: id, issueLinkId: linkId } },
              })
        ),
      onSuccess: refresh,
    }),
  }
}

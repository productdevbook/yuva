import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type MemberSignature } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useMySignatures() {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: keys.signatures(ws),
    queryFn: () => unwrap(api.GET("/v1/me/signatures")).then((r) => r.items),
    staleTime: 60_000,
  })
}

export function useSetSignature(inboxId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: string) =>
      unwrap(api.PUT("/v1/me/signatures/{inboxId}", { params: { path: { inboxId } }, body: { body } })),
    onSuccess: (saved) =>
      qc.setQueryData<MemberSignature[]>(keys.signatures(ws), (all) =>
        (all ?? []).map((s) => (s.inbox_id === inboxId ? saved : s))
      ),
  })
}

export function useResetSignature(inboxId: string) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/me/signatures/{inboxId}", { params: { path: { inboxId } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.signatures(ws) }),
  })
}

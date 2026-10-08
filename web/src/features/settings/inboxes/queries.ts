import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type Inbox, type InboxUpdate } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function useInbox(inboxId: string) {
  const { workspaceId: ws } = useSession()
  return useQuery({
    queryKey: [...keys.inboxes(ws), inboxId],
    queryFn: () => unwrap(api.GET("/v1/inboxes/{inboxId}", { params: { path: { inboxId } } })),
  })
}

export function useSaveInbox(inbox: Inbox) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (body: InboxUpdate) => unwrap(api.PATCH("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } }, body })),
    onSuccess: (data) => {
      qc.setQueryData([...keys.inboxes(ws), inbox.id], data)
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws), exact: true })
    },
  })
}

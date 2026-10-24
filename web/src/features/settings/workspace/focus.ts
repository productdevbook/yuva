import { useMutation, useQueryClient } from "@tanstack/react-query"

import { api, unwrap, type WorkspaceFocus } from "@/lib/api"
import { keys } from "@/lib/keys"
import { meKey, useSession } from "@/lib/session"

export function useSaveFocus(onSaved?: () => void) {
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  return useMutation({
    mutationFn: (focus: WorkspaceFocus) => unwrap(api.PATCH("/v1/workspace", { body: { focus } })),
    onSuccess: async (w) => {
      qc.setQueryData(keys.workspace(ws), w)
      await qc.invalidateQueries({ queryKey: meKey })
      void qc.invalidateQueries({ queryKey: keys.home(ws) })
      onSaved?.()
    },
  })
}

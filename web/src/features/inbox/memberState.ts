import { useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { useConversations } from "@/features/inbox/queries"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { applyPin, applyRead } from "@/lib/live"
import { useSession } from "@/lib/session"

export function usePinnedIds() {
  const list = useConversations({ pinned: true })
  return new Set((list.data?.pages ?? []).flatMap((p) => p.items.map((c) => c.id)))
}

export function useListActions() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const qc = useQueryClient()
  const { workspaceId: ws, membership } = useSession()
  const me = membership.member_id
  const path = (id: string) => ({ params: { path: { conversationId: id } } })
  const fail = (err: unknown) => {
    toast(errorText(err))
    void qc.invalidateQueries({ queryKey: keys.conversationLists(ws) })
  }
  return {
    pin: (id: string, on: boolean) => {
      applyPin(qc, ws, { conversation_id: id, member_id: me, pinned_at: on ? new Date().toISOString() : undefined })
      const req = on ? unwrap(api.PUT("/v1/conversations/{conversationId}/pin", path(id))) : unwrap(api.DELETE("/v1/conversations/{conversationId}/pin", path(id)))
      req.then((pin) => {
        applyPin(qc, ws, pin)
        toast(on ? t`Pinned to the top of your list` : t`Unpinned`)
      }, fail)
    },
    markUnread: (id: string) => {
      applyRead(qc, ws, { conversation_id: id, member_id: me, unread: true })
      unwrap(api.POST("/v1/conversations/{conversationId}/unread", path(id))).then((read) => applyRead(qc, ws, read), fail)
    },
    markRead: (id: string) => {
      applyRead(qc, ws, { conversation_id: id, member_id: me, unread: false })
      unwrap(api.POST("/v1/conversations/{conversationId}/read", path(id))).then((read) => applyRead(qc, ws, read), fail)
    },
  }
}

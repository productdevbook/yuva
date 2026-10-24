import { useLingui } from "@lingui/react/macro"
import { useQueryClient } from "@tanstack/react-query"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { mailFilters, useMailInboxes } from "@/features/mail/folders"
import { useConversations, useCounts } from "@/features/inbox/queries"
import { api, unwrap } from "@/lib/api"
import { applyStar } from "@/lib/live"
import { useSession } from "@/lib/session"

export function useMailCounts(enabled = true) {
  const counts = useCounts(undefined, "mail")
  return enabled ? counts.data : undefined
}

export function useMailUnread() {
  const any = useMailInboxes().length > 0
  const list = useConversations(mailFilters("all", "inbox"), any)
  if (!any) return 0
  return (list.data?.pages.flatMap((p) => p.items) ?? []).filter((c) => c.unread).length
}

export function useStar() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const set = (ids: string[], on: boolean) =>
    Promise.all(
      ids.map((id) => {
        applyStar(qc, ws, id, on)
        const path = { params: { path: { conversationId: id } } }
        const req = on
          ? unwrap(api.PUT("/v1/conversations/{conversationId}/star", path))
          : unwrap(api.DELETE("/v1/conversations/{conversationId}/star", path))
        return req.catch((err: unknown) => {
          applyStar(qc, ws, id, !on)
          throw err
        })
      })
    )
  return (ids: string[], on: boolean) => {
    if (ids.length === 0) return
    const n = ids.length
    set(ids, on).then(
      () =>
        toast(
          on ? (n === 1 ? t`Starred` : t`${n} starred`) : n === 1 ? t`Star removed` : t`${n} unstarred`,
          () => void set(ids, !on).catch((e) => toast(errorText(e)))
        ),
      (e) => toast(errorText(e))
    )
  }
}

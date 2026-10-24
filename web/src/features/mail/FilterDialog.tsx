import { useQuery } from "@tanstack/react-query"
import { useEffect } from "react"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { RuleDialog } from "@/features/settings/automation/RuleDialog"
import { api, unwrap } from "@/lib/api"
import { useSession } from "@/lib/session"

export function FilterDialog({ messageId, onClose }: { messageId: string; onClose: () => void }) {
  const errorText = useErrorText()
  const { workspaceId: ws } = useSession()
  const suggestion = useQuery({
    queryKey: ["ws", ws, "filter-suggestion", messageId],
    queryFn: () => unwrap(api.GET("/v1/messages/{messageId}/filter-suggestion", { params: { path: { messageId } } })),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  })
  useEffect(() => {
    if (!suggestion.error) return
    toast(errorText(suggestion.error))
    onClose()
  }, [suggestion.error, errorText, onClose])
  if (!suggestion.data) return null
  return (
    <RuleDialog rule={null} prefill={suggestion.data} inboxId={suggestion.data.inbox_id} filter onClose={onClose} />
  )
}

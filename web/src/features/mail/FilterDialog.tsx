import { Trans } from "@lingui/react/macro"
import { useQuery } from "@tanstack/react-query"
import { useEffect } from "react"

import { toast } from "@/components/common"
import { useErrorText } from "@/components/common/text"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { RuleForm } from "@/features/settings/automation/RuleForm"
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
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] gap-0 overflow-y-auto p-0 sm:max-w-2xl" showCloseButton={false}>
        <DialogTitle className="sr-only">
          <Trans>New filter</Trans>
        </DialogTitle>
        <RuleForm
          rule={null}
          prefill={suggestion.data}
          inboxId={suggestion.data.inbox_id}
          filter
          onClose={onClose}
          className="mt-0 ring-0"
        />
      </DialogContent>
    </Dialog>
  )
}

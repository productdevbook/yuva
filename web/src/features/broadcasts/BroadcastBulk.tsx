import { Trans } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { useBulkBroadcasts } from "@/features/broadcasts/broadcastQueries"
import type { Selection } from "@/hooks/use-selection"
import type { Broadcast } from "@/lib/api"

export function BroadcastBulkBar({ items, selection }: { items: Broadcast[]; selection: Selection }) {
  const errorText = useErrorText()
  const bulk = useBulkBroadcasts()
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const n = selection.count
  return (
    <>
      <SelectionBar selection={selection} busy={bulk.isPending}>
        <BulkButton
          selection={selection}
          destructive
          disabled={bulk.isPending}
          onClick={() =>
            confirm({
              title: <Trans>Delete the selected drafts ({n})?</Trans>,
              description: (
                <Trans>
                  Their reports and the lists of recipients are deleted. Conversations opened by replies stay.
                </Trans>
              ),
              confirm: <Trans>Delete</Trans>,
              run: () =>
                bulk.mutate(
                  { ids: selection.chosen, action: "delete" },
                  {
                    onSuccess: (r) => {
                      report(r, (id) => items.find((b) => b.id === id)?.name ?? id)
                      selection.stop()
                    },
                    onError: (e) => toast(errorText(e)),
                  }
                ),
            })
          }
          data-testid="delete-selected"
        >
          <Trans>Delete</Trans>
        </BulkButton>
      </SelectionBar>
      {confirmDialog}
      {reportDialog}
    </>
  )
}

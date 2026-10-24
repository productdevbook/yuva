import { Trans } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { useBulkSubscriptions } from "@/features/broadcasts/queries"
import type { Selection } from "@/hooks/use-selection"
import type { Subscription } from "@/lib/api"

export function SubscriberBulkBar({
  listId,
  items,
  selection,
}: {
  listId: string
  items: Subscription[]
  selection: Selection
}) {
  const errorText = useErrorText()
  const bulk = useBulkSubscriptions(listId)
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const nameOf = (id: string) => items.find((s) => s.id === id)?.email ?? id
  const send = (action: "unsubscribe" | "delete") =>
    bulk.mutate(
      { ids: selection.chosen, action },
      {
        onSuccess: (r) => {
          report(r, nameOf)
          selection.stop()
        },
        onError: (e) => toast(errorText(e)),
      }
    )
  const n = selection.count
  return (
    <>
      <SelectionBar selection={selection} busy={bulk.isPending}>
        <BulkButton
          selection={selection}
          disabled={bulk.isPending}
          onClick={() =>
            confirm({
              title: <Trans>Unsubscribe the selected subscribers ({n})?</Trans>,
              description: (
                <Trans>
                  They get no more mail from this list, and it is recorded that the team unsubscribed them. Only they
                  can subscribe again, by confirming a mail.
                </Trans>
              ),
              confirm: <Trans>Unsubscribe</Trans>,
              run: () => send("unsubscribe"),
            })
          }
          data-testid="unsubscribe-selected"
        >
          <Trans>Unsubscribe</Trans>
        </BulkButton>
        <BulkButton
          selection={selection}
          destructive
          disabled={bulk.isPending}
          onClick={() =>
            confirm({
              title: <Trans>Delete the selected subscriptions ({n})?</Trans>,
              description: (
                <Trans>
                  Their consent records are deleted with them, so you can no longer show how they agreed. To stop
                  mailing them, unsubscribe them instead.
                </Trans>
              ),
              confirm: <Trans>Delete</Trans>,
              run: () => send("delete"),
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

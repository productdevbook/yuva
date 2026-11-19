import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkBar, bulkButton } from "@/components/common/ListBox"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { isActive, useBulkBroadcasts } from "@/features/broadcasts/broadcastQueries"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Selection } from "@/hooks/use-selection"
import type { Broadcast } from "@/lib/api"

export function deletable(b: Broadcast) {
  return b.state !== "scheduled" && !isActive(b)
}

export function BroadcastBulkBar({ items, selection }: { items: Broadcast[]; selection: Selection }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const bulk = useBulkBroadcasts()
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const n = selection.count
  const ask = () => {
    const ids = [...selection.chosen]
    if (ids.length === 0) return
    confirm({
      title: <Trans>Delete the selected broadcasts ({n})?</Trans>,
      description: (
        <Trans>Their reports and the lists of recipients are deleted. Conversations opened by replies stay.</Trans>
      ),
      confirm: <Trans>Delete</Trans>,
      run: () =>
        bulk.mutate(
          { ids, action: "delete" },
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
  useHotkeys({ Delete: ask }, selection.selecting && n > 0)
  useHotkeys({ Escape: selection.stop }, selection.selecting)
  return (
    <>
      {selection.selecting && (
        <BulkBar
          picked={n}
          total={items.filter(deletable).length}
          onToggleAll={selection.toggleAll}
          onCancel={selection.stop}
          busy={bulk.isPending}
        >
          <Button
            variant="outline"
            className={bulkButton(true)}
            disabled={n === 0 || bulk.isPending}
            onClick={ask}
            title={t`Delete the selected broadcasts (Del)`}
            data-testid="delete-selected"
          >
            <Trash2Icon />
            <Trans>Delete</Trans>
          </Button>
        </BulkBar>
      )}
      {confirmDialog}
      {reportDialog}
    </>
  )
}

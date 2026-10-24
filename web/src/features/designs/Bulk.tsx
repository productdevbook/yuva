import { Trans } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { useBulkDesigns, useBulkImages } from "@/features/designs/queries"
import type { Selection } from "@/hooks/use-selection"
import type { EmailDesignSummary, EmailImage } from "@/lib/api"

export function DesignBulkBar({ items, selection }: { items: EmailDesignSummary[]; selection: Selection }) {
  const errorText = useErrorText()
  const bulk = useBulkDesigns()
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
              title: <Trans>Archive the selected designs ({n})?</Trans>,
              description: (
                <Trans>
                  They leave the list of designs. Their versions are kept. Drafts that use them still send, and you can
                  restore them from the archive.
                </Trans>
              ),
              confirm: <Trans>Archive</Trans>,
              run: () =>
                bulk.mutate(
                  { ids: selection.chosen, action: "delete" },
                  {
                    onSuccess: (r) => {
                      report(r, (id) => items.find((d) => d.id === id)?.name ?? id)
                      selection.stop()
                    },
                    onError: (e) => toast(errorText(e)),
                  }
                ),
            })
          }
          data-testid="archive-selected"
        >
          <Trans>Archive</Trans>
        </BulkButton>
      </SelectionBar>
      {confirmDialog}
      {reportDialog}
    </>
  )
}

export function ImageBulkBar({ items, selection }: { items: EmailImage[]; selection: Selection }) {
  const errorText = useErrorText()
  const bulk = useBulkImages()
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
              title: <Trans>Delete the selected images ({n})?</Trans>,
              description: <Trans>Their addresses stop working at once.</Trans>,
              confirm: <Trans>Delete</Trans>,
              run: () =>
                bulk.mutate(
                  { ids: selection.chosen, action: "delete" },
                  {
                    onSuccess: (r) => {
                      report(r, (id) => items.find((i) => i.id === id)?.filename ?? id)
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

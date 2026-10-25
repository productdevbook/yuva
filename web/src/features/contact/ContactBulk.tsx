import { Trans, useLingui } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { useDeleteContacts } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Selection } from "@/hooks/use-selection"
import type { Contact } from "@/lib/api"

export function ContactBulkBar({
  items,
  selection,
  className,
}: {
  items: Contact[]
  selection: Selection
  className?: string
}) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const bulk = useDeleteContacts()
  const [confirm, confirmDialog] = useConfirm()
  const [, reportDialog, showDeleted] = useBulkReport()
  const n = selection.count
  const unnamed = t`Unnamed contact`
  const ask = () => {
    const ids = [...selection.chosen]
    if (ids.length === 0) return
    confirm({
      title: <Trans>Delete the selected contacts ({n})?</Trans>,
      description: (
        <Trans>The contacts, their conversations, notes and files are deleted for good. This cannot be undone.</Trans>
      ),
      confirm: <Trans>Delete</Trans>,
      run: () =>
        bulk.mutate(ids, {
          onSuccess: (r) => {
            showDeleted(r.deleted, r.failed, (id) => {
              const c = items.find((x) => x.id === id)
              return c ? contactName(c, unnamed) : id
            })
            selection.stop()
          },
          onError: (e) => toast(errorText(e)),
        }),
    })
  }
  useHotkeys({ Delete: ask }, selection.selecting && n > 0)
  return (
    <>
      <SelectionBar selection={selection} busy={bulk.isPending} className={className}>
        <BulkButton
          selection={selection}
          destructive
          disabled={bulk.isPending}
          onClick={ask}
          title={t`Delete the selected contacts (Del)`}
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

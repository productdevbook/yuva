import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkBar, bulkButton } from "@/components/common/ListBox"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useDeleteContacts } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { Selection } from "@/hooks/use-selection"
import type { Contact } from "@/lib/api"

export function ContactBulkBar({ items, selection }: { items: Contact[]; selection: Selection }) {
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
  useHotkeys({ Escape: selection.stop }, selection.selecting)
  return (
    <>
      {selection.selecting && (
        <BulkBar
          picked={n}
          total={items.length}
          onToggleAll={selection.toggleAll}
          onCancel={selection.stop}
          busy={bulk.isPending}
        >
          <Button
            variant="outline"
            className={bulkButton(true)}
            disabled={n === 0 || bulk.isPending}
            onClick={ask}
            title={t`Delete the selected contacts (Del)`}
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

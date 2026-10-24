import { Trans, useLingui } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { useBulkEntries } from "@/features/changelog/queries"
import { entryTitle } from "@/features/changelog/text"
import type { Selection } from "@/hooks/use-selection"
import type { Changelog, ChangelogEntrySummary } from "@/lib/api"

export function EntryBulkBar({
  changelog,
  items,
  selection,
}: {
  changelog: Changelog
  items: ChangelogEntrySummary[]
  selection: Selection
}) {
  const { i18n } = useLingui()
  const errorText = useErrorText()
  const bulk = useBulkEntries(changelog.id)
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const nameOf = (id: string) => {
    const e = items.find((x) => x.id === id)
    return (e && entryTitle(e.translations, i18n.locale, changelog.default_locale)) || id
  }
  const send = (action: "publish" | "delete") =>
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
  const remove = () => {
    const n = selection.count
    confirm({
      title: <Trans>Delete the selected entries ({n})?</Trans>,
      description: <Trans>Every translation is deleted. Broadcasts made from it stay.</Trans>,
      confirm: <Trans>Delete</Trans>,
      run: () => send("delete"),
    })
  }
  return (
    <>
      <SelectionBar selection={selection} busy={bulk.isPending} className="border-b px-6 py-2 phone:px-4">
        {changelog.can_publish && (
          <BulkButton
            selection={selection}
            disabled={bulk.isPending}
            onClick={() => send("publish")}
            data-testid="publish-selected"
          >
            <Trans>Publish</Trans>
          </BulkButton>
        )}
        <BulkButton
          selection={selection}
          destructive
          disabled={bulk.isPending}
          onClick={remove}
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

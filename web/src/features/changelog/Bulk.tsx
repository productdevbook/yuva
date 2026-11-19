import { Trans, useLingui } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkBar, bulkButton } from "@/components/common/ListBox"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { useBulkEntries } from "@/features/changelog/queries"
import { entryTitle } from "@/features/changelog/text"
import type { Selection } from "@/hooks/use-selection"
import type { Changelog, ChangelogEntrySummary } from "@/lib/api"

export function EntryBulkBar({
  changelogs,
  items,
  selection,
}: {
  changelogs: Map<string, Changelog>
  items: ChangelogEntrySummary[]
  selection: Selection
}) {
  const { i18n } = useLingui()
  const errorText = useErrorText()
  const bulk = useBulkEntries()
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const byId = new Map(items.map((e) => [e.id, e]))
  const nameOf = (id: string) => {
    const e = byId.get(id)
    const c = e && changelogs.get(e.changelog_id)
    return (e && c && entryTitle(e.translations, i18n.locale, c.default_locale)) || id
  }
  const canPublish = [...changelogs.values()].some((c) => c.can_publish)
  const send = (action: "publish" | "delete") => {
    const groups = new Map<string, string[]>()
    for (const id of selection.chosen) {
      const c = byId.get(id)?.changelog_id
      if (c) groups.set(c, [...(groups.get(c) ?? []), id])
    }
    bulk.mutate(
      { groups, action },
      {
        onSuccess: (r) => {
          report(r, nameOf)
          selection.stop()
        },
        onError: (e) => toast(errorText(e)),
      }
    )
  }
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
      <BulkBar
        picked={selection.count}
        total={items.length}
        onToggleAll={selection.toggleAll}
        onCancel={selection.stop}
        busy={bulk.isPending}
      >
        {canPublish && (
          <Button
            variant="outline"
            className={bulkButton()}
            disabled={selection.count === 0 || bulk.isPending}
            onClick={() => send("publish")}
            data-testid="publish-selected"
          >
            <Trans>Publish</Trans>
          </Button>
        )}
        <Button
          variant="outline"
          className={bulkButton(true)}
          disabled={selection.count === 0 || bulk.isPending}
          onClick={remove}
          data-testid="delete-selected"
        >
          <Trans>Delete</Trans>
        </Button>
      </BulkBar>
      {confirmDialog}
      {reportDialog}
    </>
  )
}

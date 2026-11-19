import { Trans, useLingui } from "@lingui/react/macro"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkBar, bulkButton } from "@/components/common/ListBox"
import { Button } from "@/components/ui/button"
import { useErrorText } from "@/components/common/text"
import { useBulkDiscussions } from "@/features/docs/discussions/queries"
import type { Selection } from "@/hooks/use-selection"
import type { Discussion, DiscussionBulkChange } from "@/lib/api"
import { useSession } from "@/lib/session"

export function DiscussionBulkBar({ items, selection }: { items: Discussion[]; selection: Selection }) {
  const { t } = useLingui()
  const { canManage, membership } = useSession()
  const errorText = useErrorText()
  const bulk = useBulkDiscussions()
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const byId = new Map(items.map((d) => [d.id, d]))
  const nameOf = (id: string) => byId.get(id)?.title ?? id
  const mine = selection.chosen.every((id) => byId.get(id)?.author.member_id === membership.member_id)
  const n = selection.count

  const send = (action: DiscussionBulkChange["action"], undo?: (succeeded: string[]) => (() => void) | undefined) =>
    bulk.mutate(
      { ids: selection.chosen, action },
      {
        onSuccess: (r) => {
          report(r, nameOf, undo?.(r.succeeded))
          selection.stop()
        },
        onError: (e) => toast(errorText(e)),
      }
    )
  const hide = () =>
    send("hide", (succeeded) => {
      const ids = succeeded.filter((id) => byId.get(id)?.state === "published")
      if (ids.length === 0) return undefined
      return () =>
        bulk.mutate(
          { ids, action: "unhide" },
          { onSuccess: (r) => report(r, nameOf), onError: (e) => toast(errorText(e)) }
        )
    })

  return (
    <>
      <BulkBar
        picked={selection.count}
        total={items.length}
        onToggleAll={selection.toggleAll}
        onCancel={selection.stop}
        busy={bulk.isPending}
      >
        {canManage && (
          <>
            <Button
              variant="outline"
              className={bulkButton()}
              disabled={n === 0 || bulk.isPending}
              onClick={hide}
              data-testid="hide-selected"
            >
              <Trans>Hide</Trans>
            </Button>
            <Button
              variant="outline"
              className={bulkButton(true)}
              disabled={n === 0 || bulk.isPending}
              onClick={() =>
                confirm({
                  title: <Trans>Mark the selected discussions as spam ({n})?</Trans>,
                  description: (
                    <Trans>
                      They are hidden with every other post of their authors still waiting for approval, and the
                      authors’ later posts always wait for a member.
                    </Trans>
                  ),
                  confirm: <Trans>Mark as spam</Trans>,
                  run: () => send("spam"),
                })
              }
              data-testid="spam-selected"
            >
              <Trans>Spam</Trans>
            </Button>
          </>
        )}
        <Button
          variant="outline"
          className={bulkButton(true)}
          disabled={n === 0 || bulk.isPending || !mine}
          title={mine ? undefined : t`You can delete only discussions you started`}
          onClick={() =>
            confirm({
              title: <Trans>Delete the selected discussions ({n})?</Trans>,
              description: <Trans>A deleted post cannot be brought back.</Trans>,
              confirm: <Trans>Delete</Trans>,
              run: () => send("delete"),
            })
          }
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

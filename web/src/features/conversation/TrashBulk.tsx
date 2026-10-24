import { Trans, useLingui } from "@lingui/react/macro"
import { Trash2Icon } from "lucide-react"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import { useCounts, useDeleteConversations, useEmptyTrash } from "@/features/inbox/queries"
import { ApiError } from "@/lib/api"
import { cn } from "@/lib/utils"

export function useTrashTools() {
  const { t } = useLingui()
  const errorText = useErrorText()
  const [confirm, confirmDialog] = useConfirm()
  const [, reportDialog, showDeleted] = useBulkReport()
  const del = useDeleteConversations()
  const empty = useEmptyTrash()

  const deleteForever = (ids: string[], name: (id: string) => string, done: () => void) => {
    const n = ids.length
    if (n === 0) return
    confirm({
      title: <Trans>Delete the selected conversations ({n}) for good?</Trans>,
      description: (
        <Trans>
          They are deleted at once with their messages and files. This cannot be undone, and nobody can restore them.
        </Trans>
      ),
      confirm: <Trans>Delete for good</Trans>,
      run: () =>
        del.mutate(ids, {
          onSuccess: (r) => {
            showDeleted(r.deleted, r.failed, name)
            done()
          },
          onError: (e) => toast(errorText(e)),
        }),
    })
  }

  const emptyTrash = (count: number, changed = false) => {
    if (count === 0) {
      toast(t`The Trash is empty`)
      return
    }
    confirm({
      title: <Trans>Empty the Trash?</Trans>,
      description: (
        <>
          {changed && (
            <span className="mb-2 block font-medium text-foreground" data-testid="trash-count-changed">
              <Trans>The Trash changed. It now holds {count}.</Trans>
            </span>
          )}
          <Trans>
            Every conversation in the Trash that you may delete ({count}) is deleted at once with its messages and
            files, in conversation and mail inboxes alike. This cannot be undone.
          </Trans>
        </>
      ),
      confirm: <Trans>Empty the Trash</Trans>,
      run: () =>
        empty.mutate(count, {
          onSuccess: (r) => toast(t`Emptying the Trash: ${r.count} are being deleted`),
          onError: (e) => {
            const now =
              e instanceof ApiError && e.code === "count_changed" ? Number((e.body as { count?: number }).count) : NaN
            if (Number.isNaN(now)) toast(errorText(e))
            else emptyTrash(now, true)
          },
        }),
    })
  }

  const dialogs = (
    <>
      {confirmDialog}
      {reportDialog}
    </>
  )
  return { deleteForever, emptyTrash, dialogs, pending: del.isPending || empty.isPending }
}

export function TrashStrip({ onEmpty, busy }: { onEmpty: (count: number) => void; busy: boolean }) {
  const count = useCounts().data?.trash ?? 0
  if (count === 0) return null
  return (
    <div
      role="status"
      className="mx-3 mb-2 flex flex-wrap items-center gap-x-2 gap-y-1.5 rounded-xl border border-border bg-card py-2 ps-3 pe-2 text-small"
      data-testid="trash-strip"
    >
      <Trash2Icon className="size-3.5 shrink-0 text-faint" />
      <span className="min-w-0 flex-1 basis-40">
        <Trans>{count} in the Trash across your inboxes. They are deleted for good after 30 days.</Trans>
      </span>
      <Button
        variant="outline"
        size="sm"
        className={cn(look.btnSm, "ms-auto text-destructive")}
        onClick={() => onEmpty(count)}
        disabled={busy}
        data-testid="empty-trash"
      >
        <Trans>Empty the Trash</Trans>
      </Button>
    </div>
  )
}

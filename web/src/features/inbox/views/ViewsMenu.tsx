import { Trans, useLingui } from "@lingui/react/macro"
import { BookmarkIcon, BookmarkPlusIcon, PencilIcon, RefreshCwIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { useSearchParams } from "react-router"

import { toast, useConfirm } from "@/components/common"
import { keyLabel, SHORTCUTS } from "@/components/common/ShortcutSheet"
import { useErrorText } from "@/components/common/text"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { look } from "@/features/inbox/PageFrame"
import { toViewFilters, viewSearch } from "@/features/inbox/views/filters"
import { canEditView, useView, useViewMutations } from "@/features/inbox/views/queries"
import { useActiveView } from "@/features/inbox/views/useActiveView"
import { ViewDialog } from "@/features/inbox/views/ViewDialog"
import { useHotkeys } from "@/hooks/use-hotkeys"
import type { SavedView } from "@/lib/api"
import type { ConversationFilters } from "@/lib/keys"
import { useSession } from "@/lib/session"

export function ViewsMenu({ filters, active }: { filters: ConversationFilters; active: boolean }) {
  const { t } = useLingui()
  const errorText = useErrorText()
  const { membership, canManage } = useSession()
  const [, setParams] = useSearchParams()
  const { id, clear } = useActiveView()
  const current = useView(id).data
  const { update, remove } = useViewMutations()
  const [dialog, setDialog] = useState<"new" | "edit" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const editable = !!current && canEditView(current, membership.member_id, canManage)
  const saved = toViewFilters(filters)
  const show = (v: SavedView) => setParams(new URLSearchParams(viewSearch(v.id, v.filters)), { replace: true })
  useHotkeys({ [SHORTCUTS.saveView]: () => setDialog("new") }, active)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<button type="button" className={id ? look.listChipOn : look.listChip} />}
          aria-label={t`Saved views`}
          title={t`Saved views`}
          data-testid="views-menu"
        >
          <BookmarkIcon className="size-3.5" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-60">
          <DropdownMenuItem onClick={() => setDialog("new")} data-testid="view-new">
            <BookmarkPlusIcon />
            <Trans>Save as view</Trans>
            <DropdownMenuShortcut>{keyLabel(SHORTCUTS.saveView).join(" ")}</DropdownMenuShortcut>
          </DropdownMenuItem>
          {current && editable && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog("edit")} data-testid="view-edit">
                <PencilIcon />
                <Trans>Edit view</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() =>
                  update.mutate(
                    { id: current.id, body: { filters: saved } },
                    { onSuccess: show, onError: (e) => toast(errorText(e)) }
                  )
                }
                data-testid="view-update"
              >
                <RefreshCwIcon />
                <Trans>Update the view from this list</Trans>
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => {
                  const name = current.name
                  confirm({
                    title: <Trans>Delete the view {name}?</Trans>,
                    description: <Trans>The conversations stay where they are. Only the saved view goes.</Trans>,
                    confirm: <Trans>Delete</Trans>,
                    run: () => remove.mutate(current.id, { onSuccess: clear, onError: (e) => toast(errorText(e)) }),
                  })
                }}
                data-testid="view-delete"
              >
                <Trash2Icon />
                <Trans>Delete view</Trans>
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {dialog && (
        <ViewDialog
          view={dialog === "edit" ? current : undefined}
          filters={saved}
          onClose={() => setDialog(null)}
          onDone={(v) => {
            setDialog(null)
            show(v)
          }}
        />
      )}
      {confirmDialog}
    </>
  )
}

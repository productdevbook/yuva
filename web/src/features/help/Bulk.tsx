import { Trans, useLingui } from "@lingui/react/macro"
import { ChevronDownIcon } from "lucide-react"

import { toast, useConfirm } from "@/components/common"
import { useBulkReport } from "@/components/common/BulkReport"
import { BulkButton, SelectionBar } from "@/components/common/Selection"
import { useErrorText } from "@/components/common/text"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { useBulkArticles } from "@/features/help/queries"
import { articleTitle, collectionName } from "@/features/help/text"
import type { Selection } from "@/hooks/use-selection"
import type { BulkResult, HelpArticleBulkChange, HelpArticleSummary, HelpCenter, HelpCollection } from "@/lib/api"
import { look } from "@/features/inbox/PageFrame"

export function ArticleBulkBar({
  center,
  items,
  cols,
  selection,
}: {
  center: HelpCenter
  items: HelpArticleSummary[]
  cols: HelpCollection[]
  selection: Selection
}) {
  const { t, i18n } = useLingui()
  const errorText = useErrorText()
  const bulk = useBulkArticles(center.id)
  const [confirm, confirmDialog] = useConfirm()
  const [report, reportDialog] = useBulkReport()
  const locale = center.default_locale
  const prefer = [i18n.locale, locale]
  const byId = new Map(items.map((a) => [a.id, a]))
  const nameOf = (id: string) => {
    const a = byId.get(id)
    return (a && articleTitle(a, ...prefer)) || id
  }
  const live = (id: string) => {
    const state = byId.get(id)?.translations.find((x) => x.locale === locale)?.state
    return state === "published" || state === "changed"
  }

  const send = (body: Omit<HelpArticleBulkChange, "ids">, undo?: (r: BulkResult) => (() => void) | undefined) =>
    bulk.mutate(
      { ids: selection.chosen, ...body },
      {
        onSuccess: (r) => {
          report(r, nameOf, undo?.(r))
          selection.stop()
        },
        onError: (e) => toast(errorText(e)),
      }
    )

  const swap = (ids: string[], action: "publish" | "unpublish") =>
    ids.length === 0
      ? undefined
      : () =>
          bulk.mutate(
            { ids, action, locale },
            { onSuccess: (r) => report(r, nameOf), onError: (e) => toast(errorText(e)) }
          )

  const publish = () =>
    send({ action: "publish", locale }, (r) =>
      swap(
        r.succeeded.filter((id) => !live(id)),
        "unpublish"
      )
    )
  const unpublish = () =>
    confirm({
      title: <Trans>Unpublish the selected translations?</Trans>,
      description: <Trans>The pages stop working for visitors. The drafts stay, and you can publish them again.</Trans>,
      confirm: <Trans>Unpublish</Trans>,
      run: () =>
        send({ action: "unpublish", locale }, (r) =>
          swap(
            r.succeeded.filter((id) => live(id)),
            "publish"
          )
        ),
    })
  const move = (collectionId: string | null) =>
    send({ action: "move", collection_id: collectionId }, (r) => {
      const groups = new Map<string | null, string[]>()
      for (const id of r.succeeded) {
        const from = byId.get(id)?.collection_id ?? null
        groups.set(from, [...(groups.get(from) ?? []), id])
      }
      return async () => {
        const merged: BulkResult = { succeeded: [], failed: [] }
        try {
          for (const [from, ids] of groups) {
            const back = await bulk.mutateAsync({ ids, action: "move", collection_id: from })
            merged.succeeded.push(...back.succeeded)
            merged.failed.push(...back.failed)
          }
        } catch (e) {
          toast(errorText(e))
        }
        report(merged, nameOf)
      }
    })
  const remove = () => {
    const n = selection.count
    confirm({
      title: <Trans>Delete the selected articles ({n})?</Trans>,
      description: <Trans>Every translation, its revisions and its counters are deleted.</Trans>,
      confirm: <Trans>Delete</Trans>,
      run: () => send({ action: "delete" }),
    })
  }

  return (
    <>
      <SelectionBar selection={selection} busy={bulk.isPending}>
        {center.can_publish && (
          <>
            <BulkButton
              selection={selection}
              disabled={bulk.isPending}
              onClick={publish}
              data-testid="publish-selected"
            >
              <Trans>Publish</Trans>
            </BulkButton>
            <BulkButton
              selection={selection}
              disabled={bulk.isPending}
              onClick={unpublish}
              data-testid="unpublish-selected"
            >
              <Trans>Unpublish</Trans>
            </BulkButton>
          </>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                className={look.btnSm}
                disabled={selection.count === 0 || bulk.isPending}
                title={t`Move to collection`}
              />
            }
            data-testid="move-selected"
          >
            <Trans>Move to collection</Trans>
            <ChevronDownIcon className="text-faint" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-56">
            <DropdownMenuItem onClick={() => move(null)}>
              <Trans>No collection</Trans>
            </DropdownMenuItem>
            {cols.map((c) => (
              <DropdownMenuItem key={c.id} onClick={() => move(c.id)}>
                {collectionName(c, ...prefer)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
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

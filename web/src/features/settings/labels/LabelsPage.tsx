import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { Dot, ErrorLine, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { LabelDialog } from "@/features/settings/labels/LabelDialog"
import { Card, EmptyRow, PageHeader, Row, Rows, RowText } from "@/features/settings/ui"
import { api, unwrap, type Label } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useLabels } from "@/lib/workspace"

export function LabelsPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const labels = useLabels()
  const [editing, setEditing] = useState<Label | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/labels/{labelId}", { params: { path: { labelId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.labels(ws) }),
  })
  return (
    <>
      <PageHeader
        title={<Trans>Labels</Trans>}
        description={<Trans>Sort conversations by topic. Each label also gets its own view in the sidebar.</Trans>}
        action={
          canManage && (
            <Button size="sm" onClick={() => setEditing("new")}>
              <Trans>New label</Trans>
            </Button>
          )
        }
      />
      <Card flush>
        {labels.data && labels.data.length === 0 ? (
          <EmptyRow>
            <Trans>No labels yet</Trans>
          </EmptyRow>
        ) : (
          <Rows>
            {(labels.data ?? []).map((l) => (
              <Row key={l.id} data-testid="label-row">
                <Dot color={l.color} className="size-2.5" />
                <RowText title={l.name} />
                <span className="font-mono text-xs text-faint">{l.color}</span>
                {canManage && (
                  <>
                    <Button variant="ghost" size="icon-sm" aria-label={t`Edit`} onClick={() => setEditing(l)}>
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t`Delete`}
                      onClick={() => {
                        const name = l.name
                        confirm({
                          title: <Trans>Delete the label {name}?</Trans>,
                          description: <Trans>It is removed from every conversation.</Trans>,
                          confirm: <Trans>Delete</Trans>,
                          run: () => remove.mutate(l.id),
                        })
                      }}
                    >
                      <Trash2Icon />
                    </Button>
                  </>
                )}
              </Row>
            ))}
          </Rows>
        )}
        <ErrorLine error={labels.error ?? remove.error} className="px-5 pb-4" />
      </Card>
      {editing && <LabelDialog label={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}

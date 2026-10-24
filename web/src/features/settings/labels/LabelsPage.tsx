import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, TagIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, useConfirm } from "@/components/common"
import { EmptyBox } from "@/components/common/EmptyBox"
import { Button } from "@/components/ui/button"
import { useCounts } from "@/features/inbox/queries"
import { GeneralHeader, Intro } from "@/features/settings/GeneralTabs"
import { LabelDialog } from "@/features/settings/labels/LabelDialog"
import { Card, headButton, mix, Row, Rows } from "@/features/settings/ui"
import { api, unwrap, type Label } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { useLabels } from "@/lib/workspace"

export function LabelsPage() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws, canManage } = useSession()
  const labels = useLabels()
  const counts = useCounts(["conversation"], null)
  const open = new Map((counts.data?.labels ?? []).map((c) => [c.id, c.count]))
  const [editing, setEditing] = useState<Label | "new" | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: (id: string) => unwrap(api.DELETE("/v1/labels/{labelId}", { params: { path: { labelId: id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.labels(ws) }),
  })
  const add = canManage && (
    <Button className={headButton.primary} onClick={() => setEditing("new")} data-testid="label-new">
      <Trans>New label</Trans>
    </Button>
  )
  return (
    <>
      <GeneralHeader current="labels" />
      <Intro action={labels.data && labels.data.length > 0 ? add : undefined}>
        <Trans>Sort conversations by topic. Each label also gets its own view in the sidebar.</Trans>
      </Intro>
      {labels.data && labels.data.length === 0 ? (
        <EmptyBox icon={TagIcon} title={<Trans>No labels yet</Trans>} actions={add} testId="labels-empty">
          <Trans>Labels sort conversations by topic, such as billing or bugs.</Trans>
        </EmptyBox>
      ) : (
        <Card flush>
          <Rows>
            {(labels.data ?? []).map((l) => {
              const n = open.get(l.id) ?? 0
              return (
                <Row key={l.id} data-testid="label-row">
                  <span
                    className="inline-flex max-w-48 min-w-0 items-center rounded-md border px-2 py-0.5 text-body font-medium"
                    style={{ color: l.color, borderColor: mix(l.color, 35), backgroundColor: mix(l.color, 8) }}
                  >
                    <span className="truncate">{l.name}</span>
                  </span>
                  <span className="min-w-0 flex-1 text-caption text-faint" data-testid="label-count">
                    {counts.data && <Plural value={n} one="# open conversation" other="# open conversations" />}
                  </span>
                  <span className="font-mono text-caption text-faint phone:hidden">{l.color}</span>
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
              )
            })}
          </Rows>
          <ErrorLine error={labels.error ?? remove.error} className="px-5 pb-4" />
        </Card>
      )}
      {editing && <LabelDialog label={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PencilIcon, PlusIcon, TagIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"

import { EmptyState, ErrorLine, LabelChip, useConfirm } from "@/components/common"
import { Field, PageTitle, Section } from "@/features/settings/SettingsLayout"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { api, unwrap, type Label } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useLabels } from "@/lib/workspace"
import { useSession } from "@/lib/session"

const PALETTE = ["#6b7280", "#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6", "#3b82f6", "#8b5cf6", "#ec4899"]

export function LabelsSettings() {
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
      <div className="flex items-center justify-between gap-3">
        <PageTitle>
          <Trans>Labels</Trans>
        </PageTitle>
        {canManage && (
          <Button onClick={() => setEditing("new")}>
            <PlusIcon />
            <Trans>New label</Trans>
          </Button>
        )}
      </div>
      <Section
        title={<Trans>Workspace labels</Trans>}
        description={<Trans>Sort conversations by topic. Each label also gets its own view in the sidebar.</Trans>}
      >
        {labels.data && labels.data.length === 0 ? (
          <EmptyState icon={TagIcon} title={<Trans>No labels yet</Trans>} className="py-8" />
        ) : (
          <ul className="flex flex-col divide-y rounded-lg border">
            {(labels.data ?? []).map((l) => (
              <li key={l.id} className="flex items-center gap-3 px-3 py-2.5" data-testid="label-row">
                <LabelChip name={l.name} color={l.color} />
                <span className="flex-1 font-mono text-xs text-muted-foreground">{l.color}</span>
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
              </li>
            ))}
          </ul>
        )}
        <ErrorLine error={labels.error ?? remove.error} />
      </Section>
      {editing && <LabelDialog label={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {confirmDialog}
    </>
  )
}

function LabelDialog({ label, onClose }: { label: Label | null; onClose: () => void }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState(label?.name ?? "")
  const [color, setColor] = useState(label?.color ?? PALETTE[6])
  const save = useMutation({
    mutationFn: () =>
      label
        ? unwrap(api.PATCH("/v1/labels/{labelId}", { params: { path: { labelId: label.id } }, body: { name, color } }))
        : unwrap(api.POST("/v1/labels", { body: { name, color } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.labels(ws) })
      onClose()
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{label ? <Trans>Edit label</Trans> : <Trans>New label</Trans>}</DialogTitle>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="label-name">
            <Input
              id="label-name"
              required
              maxLength={64}
              value={name}
              placeholder={t`Billing`}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label={<Trans>Color</Trans>}>
            <div className="flex flex-wrap items-center gap-2">
              {PALETTE.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={c}
                  aria-pressed={c === color}
                  onClick={() => setColor(c)}
                  className="size-7 rounded-full border-2 border-transparent ring-offset-2 ring-offset-background aria-pressed:ring-2 aria-pressed:ring-ring"
                  style={{ backgroundColor: c }}
                />
              ))}
              <input
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                aria-label={t`Pick a color`}
                className="h-7 w-10 cursor-pointer rounded border bg-transparent p-0.5"
              />
            </div>
          </Field>
          <div>
            <LabelChip name={name || t`Preview`} color={color} />
          </div>
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="submit" disabled={save.isPending}>
              <Trans>Save</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

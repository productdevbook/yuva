import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { ErrorLine, LabelChip } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type Label } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

const PALETTE = ["#71717a", "#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6", "#3b82f6", "#8b5cf6", "#ec4899"]

export function LabelDialog({ label, onClose }: { label: Label | null; onClose: () => void }) {
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
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <DialogHeader>
            <DialogTitle>{label ? <Trans>Edit label</Trans> : <Trans>New label</Trans>}</DialogTitle>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="label-name">
            <Input id="label-name" required maxLength={64} value={name} placeholder={t`Billing`} onChange={(e) => setName(e.target.value)} />
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
                  className="size-6 rounded-full ring-offset-2 ring-offset-card aria-pressed:ring-2 aria-pressed:ring-foreground"
                  style={{ backgroundColor: c }}
                />
              ))}
              <input
                type="color"
                value={color}
                onChange={(e) => setColor(e.target.value)}
                aria-label={t`Pick a color`}
                className="h-7 w-10 cursor-pointer rounded-lg border bg-transparent p-0.5"
              />
            </div>
          </Field>
          <LabelChip name={name || t`Preview`} color={color} className="text-sm" />
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

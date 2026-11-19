import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { LabelChip } from "@/components/common"
import { FormCard } from "@/components/common/FormCard"
import { Input } from "@/components/ui/input"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type Label } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"

const PALETTE = ["#71717a", "#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6", "#3b82f6", "#8b5cf6", "#ec4899"]

export function LabelForm({ label, onClose }: { label: Label | null; onClose: () => void }) {
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
    <FormCard
      title={label ? <Trans>Edit label</Trans> : <Trans>New label</Trans>}
      submit={label ? <Trans>Save</Trans> : <Trans>Add label</Trans>}
      busy={save.isPending}
      ready={!!name.trim()}
      error={save.error}
      onSubmit={() => save.mutate()}
      onCancel={onClose}
      testId="label-form"
    >
      <Field label={<Trans>Name</Trans>} htmlFor="label-name">
        <Input
          id="label-name"
          required
          autoFocus
          maxLength={64}
          value={name}
          placeholder={t`Billing`}
          onChange={(e) => setName(e.target.value)}
          className="sm:max-w-80"
        />
      </Field>
      <Field label={<Trans>Color</Trans>}>
        <div className="flex flex-wrap items-center gap-2">
          <ToggleGroup
            value={[color]}
            onValueChange={(v) => v[0] && setColor(String(v[0]))}
            aria-label={t`Color`}
            className="flex-wrap gap-2"
          >
            {PALETTE.map((c) => (
              <ToggleGroupItem
                key={c}
                value={c}
                aria-label={c}
                className="size-6 min-w-6 rounded-full p-0 ring-offset-2 ring-offset-card hover:bg-transparent aria-pressed:bg-transparent aria-pressed:ring-2 aria-pressed:ring-foreground"
                style={{ backgroundColor: c }}
              />
            ))}
          </ToggleGroup>
          <Input
            type="color"
            value={color}
            onChange={(e) => setColor(e.target.value)}
            aria-label={t`Pick a color`}
            className="h-7 w-10 cursor-pointer rounded-lg bg-transparent p-0.5"
          />
          <LabelChip name={name || t`Preview`} color={color} className="ms-2 text-body" />
        </div>
      </Field>
    </FormCard>
  )
}

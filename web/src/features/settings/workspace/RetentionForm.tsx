import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import { Field, FormActions, FormCard, Section } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"

export function RetentionForm() {
  const qc = useQueryClient()
  const { membership } = useSession()
  const current = membership.workspace.retention_days?.toString() ?? ""
  const [days, setDays] = useState(current)
  const isOwner = membership.role === "owner"
  const save = useMutation({
    mutationFn: () => unwrap(api.PATCH("/v1/workspace", { body: { retention_days: days === "" ? null : Number(days) } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  })
  const dirty = days !== current
  return (
    <Section title={<Trans>Data retention</Trans>}>
      <FormCard
        onSubmit={() => save.mutate()}
        footer={
          isOwner ? (
            <FormActions pending={save.isPending} disabled={!dirty} saved={save.isSuccess && !dirty} error={save.error} />
          ) : (
            <p className="text-sm text-muted-foreground">
              <Trans>Only owners can change this.</Trans>
            </p>
          )
        }
      >
        <fieldset disabled={!isOwner} className="contents">
          <Field
            label={<Trans>Delete after (days)</Trans>}
            htmlFor="retention-days"
            hint={
              <Trans>
                Closed conversations untouched for this many days are deleted with their messages and attachments,
                and raw e-mails older than this are deleted. Leave empty to keep everything.
              </Trans>
            }
          >
            <Input id="retention-days" type="number" min={1} max={36500} value={days} onChange={(e) => setDays(e.target.value)} className="sm:max-w-40" />
          </Field>
        </fieldset>
      </FormCard>
    </Section>
  )
}

import { Trans, useLingui } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"

import { FormCard } from "@/components/common/FormCard"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Field } from "@/features/settings/ui"
import { api, unwrap, type Channel } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function parseSettings(raw: string): Record<string, unknown> | null {
  if (raw.trim() === "") return {}
  try {
    const v = JSON.parse(raw)
    return v && typeof v === "object" && !Array.isArray(v) ? v : null
  } catch {
    return null
  }
}

export function ApiChannelForm({
  inboxId,
  onClose,
  onCreated,
  className,
}: {
  inboxId: string
  onClose: () => void
  onCreated: (ch: Channel) => void
  className?: string
}) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { workspaceId: ws } = useSession()
  const [name, setName] = useState("")
  const [raw, setRaw] = useState("{}")
  const settings = parseSettings(raw)
  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/v1/inboxes/{inboxId}/channels", {
          params: { path: { inboxId } },
          body: { kind: "api", name, settings: settings ?? {} },
        })
      ),
    onSuccess: (saved) => {
      void qc.invalidateQueries({ queryKey: keys.channels(ws, inboxId) })
      onCreated(saved)
    },
  })
  return (
    <FormCard
      title={<Trans>Add an API channel</Trans>}
      description={<Trans>The kind cannot be changed later.</Trans>}
      submit={<Trans>Save</Trans>}
      ready={!!settings && !!name.trim()}
      busy={save.isPending}
      error={save.error}
      onCancel={onClose}
      onSubmit={() => save.mutate()}
      className={className}
      testId="add-api-channel"
    >
      <Field label={<Trans>Name</Trans>} htmlFor="channel-name">
        <Input
          id="channel-name"
          required
          autoFocus
          maxLength={200}
          value={name}
          placeholder={t`Billing events`}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field
        label={<Trans>Settings (JSON)</Trans>}
        htmlFor="channel-settings"
        error={settings ? undefined : <Trans>This is not a JSON object.</Trans>}
      >
        <Textarea
          id="channel-settings"
          rows={6}
          spellCheck={false}
          className="font-mono text-caption"
          value={raw}
          aria-invalid={!settings}
          onChange={(e) => setRaw(e.target.value)}
        />
      </Field>
    </FormCard>
  )
}

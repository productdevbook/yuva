import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trash2Icon } from "lucide-react"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, TypeToConfirmDialog } from "@/components/common"
import { Field, PageTitle, Section } from "@/features/settings/SettingsLayout"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, setWorkspace, unwrap } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"

export function WorkspaceSettings() {
  const { membership } = useSession()
  return (
    <>
      <PageTitle>
        <Trans>Workspace</Trans>
      </PageTitle>
      <RetentionForm key={membership.workspace.retention_days ?? 0} />
      <DangerZone />
    </>
  )
}

function RetentionForm() {
  const qc = useQueryClient()
  const { membership } = useSession()
  const current = membership.workspace.retention_days?.toString() ?? ""
  const [days, setDays] = useState(current)
  const isOwner = membership.role === "owner"
  const save = useMutation({
    mutationFn: () =>
      unwrap(api.PATCH("/v1/workspace", { body: { retention_days: days === "" ? null : Number(days) } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: meKey }),
  })
  const dirty = days !== current
  return (
    <Section title={<Trans>Data retention</Trans>} description={membership.workspace.name}>
      <form
        className="grid gap-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
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
            className="sm:col-span-2"
          >
            <Input
              id="retention-days"
              type="number"
              min={1}
              max={36500}
              value={days}
              onChange={(e) => setDays(e.target.value)}
              className="sm:max-w-40"
            />
          </Field>
          {isOwner ? (
            <div className="flex items-center gap-3 sm:col-span-2">
              <Button type="submit" disabled={!dirty || save.isPending}>
                <Trans>Save</Trans>
              </Button>
              {save.isSuccess && !dirty && (
                <span className="text-sm text-muted-foreground">
                  <Trans>Saved</Trans>
                </span>
              )}
              <ErrorLine error={save.error} />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground sm:col-span-2">
              <Trans>Only owners can change this.</Trans>
            </p>
          )}
        </fieldset>
      </form>
    </Section>
  )
}

function DangerZone() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { membership } = useSession()
  const [open, setOpen] = useState(false)
  const name = membership.workspace.name
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/workspace", { body: { name } })),
    onSuccess: async () => {
      setOpen(false)
      setWorkspace(null)
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== meKey[0] })
      await qc.invalidateQueries({ queryKey: meKey })
      navigate("/", { replace: true })
    },
  })
  return (
    <Section
      title={<Trans>Danger zone</Trans>}
      description={
        <Trans>
          Deleting {name} removes its inboxes, channels, conversations, contacts, files, labels, webhooks and API keys
          for everyone, at once. Members keep their accounts. This cannot be undone.
        </Trans>
      }
      className="border-destructive/40"
    >
      {membership.role === "owner" ? (
        <Button
          type="button"
          variant="destructive"
          className="self-start"
          onClick={() => {
            remove.reset()
            setOpen(true)
          }}
          data-testid="delete-workspace"
        >
          <Trash2Icon />
          <Trans>Delete this workspace</Trans>
        </Button>
      ) : (
        <p className="text-sm text-muted-foreground">
          <Trans>Only owners can delete the workspace.</Trans>
        </p>
      )}
      <TypeToConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={<Trans>Delete {name}?</Trans>}
        description={
          <Trans>
            Everything in this workspace is deleted for every member, and its widgets, apps, e-mail addresses and API
            keys stop working immediately.
          </Trans>
        }
        label={<Trans>Type the workspace name to confirm</Trans>}
        expected={name}
        confirm={<Trans>Delete this workspace</Trans>}
        pending={remove.isPending}
        error={<ErrorLine error={remove.error} />}
        onConfirm={() => remove.mutate()}
      />
    </Section>
  )
}

import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { Navigate, useNavigate } from "react-router"

import { ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxLayout"
import { Card, Section } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"
import { keys } from "@/lib/keys"
import { useSession } from "@/lib/session"

function IdentitySecret() {
  const { inbox } = useInboxOutlet()
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const rotate = useMutation({
    mutationFn: () => unwrap(api.POST("/v1/inboxes/{inboxId}/identity-secret", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: (r) => setSecret(r.identity_secret),
  })
  return (
    <Section
      title={<Trans>Identity secret</Trans>}
      description={
        <Trans>
          Your backend signs identity tokens with this secret so Yuva knows who your signed-in users are. It is shown
          only when created or rotated.
        </Trans>
      }
    >
      <Card>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            disabled={rotate.isPending}
            onClick={() =>
              confirm({
                title: <Trans>Rotate the identity secret?</Trans>,
                description: <Trans>Tokens signed with the current secret stop working at once.</Trans>,
                confirm: <Trans>Rotate</Trans>,
                run: () => rotate.mutate(),
              })
            }
          >
            <Trans>Rotate secret</Trans>
          </Button>
          <ErrorLine error={rotate.error} />
        </div>
      </Card>
      <SecretDialog
        secret={secret}
        title={<Trans>New identity secret</Trans>}
        description={<Trans>Copy it now and update your backend; it is not shown again.</Trans>}
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </Section>
  )
}

function DeleteInbox() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { inbox } = useInboxOutlet()
  const { workspaceId: ws } = useSession()
  const [confirm, confirmDialog] = useConfirm()
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/inboxes/{inboxId}", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.inboxes(ws) })
      navigate("/settings/inboxes")
    },
  })
  const name = inbox.name
  return (
    <Section title={<Trans>Delete inbox</Trans>} description={<Trans>Deletes its channels, conversations, messages and attachments. This cannot be undone.</Trans>}>
      <Card tone="danger">
        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="destructive"
            size="sm"
            onClick={() =>
              confirm({
                title: <Trans>Delete {name}?</Trans>,
                description: <Trans>All of its conversations are deleted with it.</Trans>,
                confirm: <Trans>Delete</Trans>,
                run: () => remove.mutate(),
              })
            }
          >
            <Trans>Delete inbox</Trans>
          </Button>
          <ErrorLine error={remove.error} />
        </div>
      </Card>
      {confirmDialog}
    </Section>
  )
}

export function AdvancedPage() {
  const { canManage } = useSession()
  const { inbox } = useInboxOutlet()
  if (!canManage) return <Navigate to={`/settings/inboxes/${inbox.id}`} replace />
  return (
    <>
      <IdentitySecret />
      <DeleteInbox />
    </>
  )
}

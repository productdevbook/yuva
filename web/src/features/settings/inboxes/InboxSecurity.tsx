import { Trans } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, SecretDialog, useConfirm } from "@/components/common"
import { Button } from "@/components/ui/button"
import { useInboxOutlet } from "@/features/settings/inboxes/InboxPage"
import { DeleteMailboxDialog } from "@/features/settings/inboxes/Mailbox"
import { Block, BlockRow } from "@/features/settings/ui"
import { api, unwrap } from "@/lib/api"

export function IdentitySecretRow() {
  const { inbox } = useInboxOutlet()
  const [secret, setSecret] = useState<string | null>(null)
  const [confirm, confirmDialog] = useConfirm()
  const rotate = useMutation({
    mutationFn: () =>
      unwrap(api.POST("/v1/inboxes/{inboxId}/identity-secret", { params: { path: { inboxId: inbox.id } } })),
    onSuccess: (r) => setSecret(r.identity_secret),
  })
  return (
    <BlockRow
      label={<Trans>Identity secret</Trans>}
      hint={
        <Trans>
          Your backend signs identity tokens with this secret so Yuva knows who your signed-in users are. It is shown
          only when created or rotated.
        </Trans>
      }
    >
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
      <SecretDialog
        secret={secret}
        title={<Trans>New identity secret</Trans>}
        description={<Trans>Copy it now and update your backend; it is not shown again.</Trans>}
        onClose={() => setSecret(null)}
      />
      {confirmDialog}
    </BlockRow>
  )
}

export function DeleteProduct() {
  const navigate = useNavigate()
  const { inbox } = useInboxOutlet()
  const [open, setOpen] = useState(false)
  const name = inbox.name
  const personal = !!inbox.mailbox
  return (
    <Block
      tone="danger"
      title={personal ? <Trans>Delete mailbox</Trans> : <Trans>Delete product</Trans>}
      description={<Trans>Deletes its channels, conversations, messages and attachments. This cannot be undone.</Trans>}
      footer={
        <Button variant="destructive" size="sm" onClick={() => setOpen(true)} data-testid="delete-product">
          <Trans>Delete {name}…</Trans>
        </Button>
      }
    >
      <DeleteMailboxDialog inbox={inbox} open={open} onOpenChange={setOpen} onDeleted={() => navigate("/settings")} />
    </Block>
  )
}

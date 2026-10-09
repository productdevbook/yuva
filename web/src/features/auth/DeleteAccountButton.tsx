import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, TypeToConfirmDialog } from "@/components/common"
import { Button } from "@/components/ui/button"
import { api, ApiError, setWorkspace, unwrap } from "@/lib/api"
import { meKey } from "@/lib/session"

export function DeleteAccountButton({ email, size }: { email: string; size?: "sm" | "lg" }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const remove = useMutation({
    mutationFn: () => unwrap(api.DELETE("/v1/me", { body: { email } })),
    onSuccess: () => {
      setOpen(false)
      setWorkspace(null)
      qc.clear()
      qc.setQueryData(meKey, null)
      navigate("/sign-in", { replace: true })
    },
  })
  const lastOwner = remove.error instanceof ApiError && remove.error.code === "last_owner"
  return (
    <>
      <Button
        type="button"
        variant="destructive"
        size={size}
        onClick={() => {
          remove.reset()
          setOpen(true)
        }}
        data-testid="delete-account"
      >
        <Trans>Delete my account</Trans>
      </Button>
      <TypeToConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={<Trans>Delete your account?</Trans>}
        description={
          <Trans>
            You leave every workspace and lose your sign-ins, passkeys and notifications. Messages you wrote stay in
            their conversations, shown as from a deleted member. This cannot be undone.
          </Trans>
        }
        label={<Trans>Type your e-mail address to confirm</Trans>}
        expected={email}
        ignoreCase
        confirm={<Trans>Delete my account</Trans>}
        pending={remove.isPending}
        error={
          lastOwner ? (
            <p role="alert" className="text-body text-destructive">
              <Trans>
                You are the only owner of a workspace. Make someone else an owner there, or delete that workspace,
                before you delete your account.
              </Trans>
            </p>
          ) : (
            <ErrorLine error={remove.error} />
          )
        }
        onConfirm={() => remove.mutate()}
      />
    </>
  )
}

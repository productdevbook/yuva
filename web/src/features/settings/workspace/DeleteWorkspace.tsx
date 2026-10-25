import { Trans } from "@lingui/react/macro"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useNavigate } from "react-router"

import { ErrorLine, TypeToConfirmDialog } from "@/components/common"
import { Button } from "@/components/ui/button"
import { look } from "@/features/inbox/PageFrame"
import { Sec, secCard } from "@/features/settings/workspace/parts"
import { api, setWorkspace, unwrap } from "@/lib/api"
import { meKey, useSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function DeleteWorkspace() {
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
    <>
      <Sec
        id="w-danger"
        tone="danger"
        title={<Trans>Danger zone</Trans>}
        description={<Trans>These cannot be undone</Trans>}
      >
        <div className={cn(secCard, "border-destructive/30")}>
          <div className="flex items-center gap-4 p-5 @max-[760px]:flex-col @max-[760px]:items-start @max-[760px]:p-4">
            <span className="flex min-w-0 flex-1 flex-col gap-0.5">
              <b className="font-semibold">
                <Trans>Delete workspace</Trans>
              </b>
              <span className="text-body text-muted-foreground">
                <Trans>
                  Removes {name} with its products, channels, conversations, contacts, files, labels, webhooks and API
                  keys for everyone. Members keep their accounts.
                </Trans>
              </span>
            </span>
            {membership.role === "owner" ? (
              <Button
                type="button"
                variant="outline"
                className={cn(look.btn, "border-destructive/35 text-destructive hover:border-destructive")}
                onClick={() => {
                  remove.reset()
                  setOpen(true)
                }}
                data-testid="delete-workspace"
              >
                <Trans>Delete workspace…</Trans>
              </Button>
            ) : (
              <p className="text-caption text-faint">
                <Trans>Only owners can delete the workspace.</Trans>
              </p>
            )}
          </div>
        </div>
      </Sec>
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
    </>
  )
}

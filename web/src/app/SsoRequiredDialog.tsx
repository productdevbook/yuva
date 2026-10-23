import { Trans } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { ArrowLeftIcon, BuildingIcon, LogInIcon } from "lucide-react"
import { useNavigate } from "react-router"

import { ErrorLine } from "@/components/common"
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { storeWorkspace, useSignOut } from "@/lib/session"
import { clearSsoPrompt, startSso, useSsoPrompt } from "@/lib/sso"

export function SsoRequiredDialog() {
  const prompt = useSsoPrompt()
  const navigate = useNavigate()
  const signOut = useSignOut()
  const email = prompt?.kind === "required" ? prompt.email : null
  const go = useMutation({
    mutationFn: async (address: string) => {
      const r = await startSso(address, window.location.pathname + window.location.search)
      if (r.sso && r.authorization_url) window.location.assign(r.authorization_url)
    },
  })
  const signIn = useMutation({
    mutationFn: async () => {
      const next = window.location.pathname + window.location.search
      await signOut()
      clearSsoPrompt()
      navigate(`/sign-in?next=${encodeURIComponent(next)}`, { replace: true })
    },
  })
  if (prompt?.kind === "scope") {
    return (
      <AlertDialog open>
        <AlertDialogContent data-testid="sso-scope-dialog">
          <AlertDialogHeader>
            <AlertDialogTitle>
              <Trans>Sign in to this workspace</Trans>
            </AlertDialogTitle>
            <AlertDialogDescription>
              <Trans>
                You signed in through another workspace's single sign-on, and that sign-in works only there. Sign in
                with an e-mailed code or a passkey to work in this workspace.
              </Trans>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <ErrorLine error={signIn.error} />
          <AlertDialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                storeWorkspace(prompt.workspaceId)
                window.location.assign("/")
              }}
              data-testid="sso-scope-back"
            >
              <ArrowLeftIcon />
              <Trans>Back to the other workspace</Trans>
            </Button>
            <Button disabled={signIn.isPending} onClick={() => signIn.mutate()} data-testid="sso-scope-sign-in">
              <LogInIcon />
              <Trans>Sign in to this workspace</Trans>
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    )
  }
  return (
    <AlertDialog open={email !== null}>
      <AlertDialogContent data-testid="sso-required-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>
            <Trans>Sign in through your organisation</Trans>
          </AlertDialogTitle>
          <AlertDialogDescription>
            <Trans>
              This workspace requires single sign-on. Continue with your organisation's sign-in as {email} to keep
              working here.
            </Trans>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <ErrorLine error={go.error} />
        <AlertDialogFooter>
          <Button
            disabled={go.isPending || !email}
            onClick={() => email && go.mutate(email)}
            data-testid="sso-required-continue"
          >
            <BuildingIcon />
            <Trans>Continue with your organisation's sign-in</Trans>
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

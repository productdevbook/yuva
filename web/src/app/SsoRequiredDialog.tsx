import { Trans } from "@lingui/react/macro"
import { useMutation } from "@tanstack/react-query"
import { BuildingIcon } from "lucide-react"

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
import { startSso, useSsoRequired } from "@/lib/sso"

export function SsoRequiredDialog() {
  const email = useSsoRequired()
  const go = useMutation({
    mutationFn: async (address: string) => {
      const r = await startSso(address, window.location.pathname + window.location.search)
      if (r.sso && r.authorization_url) window.location.assign(r.authorization_url)
    },
  })
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

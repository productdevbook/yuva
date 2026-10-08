import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"

type Request = {
  title: React.ReactNode
  description?: React.ReactNode
  confirm: React.ReactNode
  run: () => void
}

export function useConfirm() {
  const [state, setState] = useState<Request | null>(null)
  const dialog = (
    <AlertDialog open={state !== null} onOpenChange={(open) => !open && setState(null)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{state?.title}</AlertDialogTitle>
          {state?.description && <AlertDialogDescription>{state.description}</AlertDialogDescription>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>
            <Trans>Cancel</Trans>
          </AlertDialogCancel>
          <Button
            variant="destructive"
            onClick={() => {
              state?.run()
              setState(null)
            }}
          >
            {state?.confirm}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
  return [setState, dialog] as const
}

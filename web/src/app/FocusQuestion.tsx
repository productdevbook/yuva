import { Trans } from "@lingui/react/macro"
import { useState } from "react"

import { ErrorLine } from "@/components/common"
import { FocusChoice } from "@/components/common/FocusChoice"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { useSaveFocus } from "@/features/settings/workspace/focus"
import type { WorkspaceFocus } from "@/lib/api"
import { useSession } from "@/lib/session"

function deferredKey(ws: string) {
  return `yuva:focus-later:${ws}`
}

function wasDeferred(ws: string) {
  try {
    return sessionStorage.getItem(deferredKey(ws)) === "1"
  } catch {
    return false
  }
}

export function FocusQuestion() {
  const { membership, canManage, workspaceId: ws } = useSession()
  const [later, setLater] = useState(() => wasDeferred(ws))
  const [picked, setPicked] = useState<WorkspaceFocus>()
  const save = useSaveFocus()
  if (!canManage || membership.workspace.focus || later) return null
  const defer = () => {
    try {
      sessionStorage.setItem(deferredKey(ws), "1")
    } catch {
      // the question comes back on the next load
    }
    setLater(true)
  }
  return (
    <Dialog open disablePointerDismissal onOpenChange={() => {}}>
      <DialogContent showCloseButton={false} data-testid="focus-question">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (picked) save.mutate(picked)
          }}
        >
          <DialogHeader className="pe-0">
            <DialogTitle>
              <Trans>What will you use Yuva for?</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>This sets what you see first. You can change it later in the workspace settings.</Trans>
            </DialogDescription>
          </DialogHeader>
          <FocusChoice value={picked} onChange={setPicked} disabled={save.isPending} />
          <ErrorLine error={save.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={defer} disabled={save.isPending} data-testid="focus-later">
              <Trans>Decide later</Trans>
            </Button>
            <Button type="submit" disabled={!picked || save.isPending} data-testid="focus-save">
              <Trans>Continue</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

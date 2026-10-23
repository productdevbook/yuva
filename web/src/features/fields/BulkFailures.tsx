import { Trans } from "@lingui/react/macro"
import { Link } from "react-router"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { ConversationBulkFailure, ConversationListItem } from "@/lib/api"
import { useCustomFields } from "@/lib/workspace"

function Reason({ failure }: { failure: ConversationBulkFailure }) {
  const fields = useCustomFields("conversation").data
  const name = fields?.find((f) => f.key === failure.field)?.name ?? failure.field
  if (failure.code === "field_required") return <Trans>Needs “{name}” before it can close</Trans>
  if (failure.code === "field_invalid") return <Trans>“{name}” holds a value that does not fit</Trans>
  return <>{failure.detail ?? failure.code}</>
}

export function BulkFailuresDialog({
  failures,
  rows,
  onClose,
}: {
  failures: ConversationBulkFailure[]
  rows: ConversationListItem[]
  onClose: () => void
}) {
  const count = failures.length
  return (
    <Dialog open={count > 0} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-md" data-testid="bulk-failures">
        <DialogHeader>
          <DialogTitle>
            <Trans>{count} could not be closed</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>The others were closed. Open one to fill in what it needs, then close it.</Trans>
          </DialogDescription>
        </DialogHeader>
        <ul className="flex flex-col gap-2.5">
          {failures.map((f) => {
            const row = rows.find((c) => c.id === f.id)
            const title = row?.subject || row?.contact.name || row?.last_message?.text
            return (
              <li key={f.id} className="flex min-w-0 flex-col" data-testid="bulk-failure">
                <Link to={`/conversations/${f.id}`} className="truncate font-medium hover:underline" onClick={onClose}>
                  {title || <Trans>No subject</Trans>}
                </Link>
                <span className="text-small text-destructive">
                  <Reason failure={f} />
                </span>
              </li>
            )
          })}
        </ul>
        <DialogFooter>
          <Button onClick={onClose}>
            <Trans>Done</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

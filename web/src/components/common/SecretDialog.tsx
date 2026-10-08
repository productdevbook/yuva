import { Trans } from "@lingui/react/macro"

import { CopyButton } from "@/components/common/CopyButton"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

export function SecretDialog({
  secret,
  title,
  description,
  onClose,
}: {
  secret: string | null
  title: React.ReactNode
  description: React.ReactNode
  onClose: () => void
}) {
  return (
    <Dialog open={secret !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 rounded-xl border bg-surface px-3 py-2 font-mono text-xs break-all">{secret}</code>
          {secret && <CopyButton value={secret} />}
        </div>
        <DialogFooter>
          <Button onClick={onClose}>
            <Trans>I saved it</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

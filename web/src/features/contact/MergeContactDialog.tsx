import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowRightIcon } from "lucide-react"
import { useState } from "react"

import { ErrorLine, PersonAvatar } from "@/components/common"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { ContactSearch } from "@/features/contact/ContactSearch"
import { MergePreview } from "@/features/contact/MergePreview"
import { useMergeContact } from "@/features/contact/queries"
import { contactName } from "@/features/contact/Section"
import type { Contact } from "@/lib/api"

function Who({ name }: { name: string }) {
  return (
    <span className="flex max-w-[calc(50%-1rem)] min-w-0 items-center gap-2">
      <PersonAvatar name={name} className="size-6 text-[10px]" />
      <span className="min-w-0 truncate font-medium">{name}</span>
    </span>
  )
}

export function MergeContactDialog({ target, open, onOpenChange }: { target: Contact; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useLingui()
  const [source, setSource] = useState<Contact | null>(null)
  const merge = useMergeContact(target.id)
  const unnamed = t`Unnamed contact`
  const targetName = contactName(target, unnamed)
  const sourceName = source ? contactName(source, unnamed) : ""

  const close = (next: boolean) => {
    if (!next) {
      setSource(null)
      merge.reset()
    }
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="grid-cols-[minmax(0,1fr)] sm:max-w-lg" data-testid="merge-dialog">
        <DialogHeader>
          <DialogTitle className="[overflow-wrap:anywhere]">
            <Trans>Merge a contact into {targetName}</Trans>
          </DialogTitle>
          <DialogDescription className="[overflow-wrap:anywhere]">
            {source ? (
              <Trans>
                Everything below moves from {sourceName} to {targetName}, and {sourceName} is deleted. This cannot be
                undone.
              </Trans>
            ) : (
              <Trans>Pick the other contact. It is merged into this one and then deleted.</Trans>
            )}
          </DialogDescription>
        </DialogHeader>
        {open && (
          <div hidden={!!source}>
            <ContactSearch exclude={target.id} onPick={setSource} />
          </div>
        )}
        {source && (
          <>
            <div className="flex min-w-0 items-center gap-3 text-sm">
              <Who name={sourceName} />
              <ArrowRightIcon className="size-4 shrink-0 text-faint rtl:rotate-180" />
              <Who name={targetName} />
            </div>
            <MergePreview source={source} target={target} />
            <ErrorLine error={merge.error} />
            <DialogFooter>
              <Button variant="outline" onClick={() => setSource(null)} disabled={merge.isPending}>
                <Trans>Back</Trans>
              </Button>
              <Button
                variant="destructive"
                onClick={() => merge.mutate(source.id, { onSuccess: () => close(false) })}
                disabled={merge.isPending}
                data-testid="merge-confirm"
              >
                <Trans>Merge</Trans>
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

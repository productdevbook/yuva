import { Plural, Trans, useLingui } from "@lingui/react/macro"
import { useNavigate } from "react-router"

import { ErrorLine, toast } from "@/components/common"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { useContactDeletionPreview, useDeleteContact } from "@/features/contact/queries"
import type { Contact } from "@/lib/api"

export function DeleteContactDialog({
  contact,
  name,
  open,
  onOpenChange,
}: {
  contact: Contact
  name: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useLingui()
  const navigate = useNavigate()
  const preview = useContactDeletionPreview(contact.id, open)
  const del = useDeleteContact(contact.id)
  const p = preview.data

  const close = (next: boolean) => {
    if (!next) del.reset()
    onOpenChange(next)
  }
  const remove = () =>
    del.mutate(undefined, {
      onSuccess: () => {
        toast(t`${name} deleted`)
        onOpenChange(false)
        navigate("/contacts")
      },
    })

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="grid-cols-[minmax(0,1fr)] sm:max-w-md" data-testid="delete-contact-dialog">
        <DialogHeader>
          <DialogTitle className="[overflow-wrap:anywhere]">
            <Trans>Delete {name}?</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              The contact, their notes and everything they wrote are deleted for good. This cannot be undone.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        {preview.isPending ? (
          <Skeleton className="h-10 w-full" />
        ) : p ? (
          <ul className="flex list-disc flex-col gap-1 ps-5 text-body" data-testid="delete-contact-preview">
            {p.conversation_count > 0 && (
              <li>
                <Plural
                  value={p.conversation_count}
                  one="# conversation is deleted with it"
                  other="# conversations are deleted with it"
                />
              </li>
            )}
            {p.has_hidden_conversations && (
              <li>
                {p.conversation_count > 0 ? (
                  <Trans>Also conversations in mailboxes you cannot read</Trans>
                ) : (
                  <Trans>Conversations in mailboxes you cannot read are deleted with it</Trans>
                )}
              </li>
            )}
            {p.conversation_count === 0 && !p.has_hidden_conversations && (
              <li>
                <Trans>It has no conversations</Trans>
              </li>
            )}
          </ul>
        ) : (
          <ErrorLine error={preview.error} />
        )}
        <ErrorLine error={del.error} />
        <DialogFooter>
          <Button variant="outline" onClick={() => close(false)} disabled={del.isPending}>
            <Trans>Cancel</Trans>
          </Button>
          <Button
            variant="destructive"
            onClick={remove}
            disabled={del.isPending || !p}
            data-testid="delete-contact-confirm"
          >
            <Trans>Delete contact</Trans>
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

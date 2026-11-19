import { Trans, useLingui } from "@lingui/react/macro"
import { LoaderIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { useCreateHelpCenter } from "@/features/help/queries"
import { ErrorLine } from "@/features/help/text"
import { Field } from "@/features/settings/ui"
import { slugify, validSlug } from "@/features/status/CreatePageForm"
import type { HelpCenter } from "@/lib/api"

export function CreateCenterDialog({
  inboxId,
  inboxName,
  onClose,
  onCreated,
}: {
  inboxId: string
  inboxName: string
  onClose: () => void
  onCreated: (c: HelpCenter) => void
}) {
  const { t } = useLingui()
  const [name, setName] = useState(inboxName)
  const [slug, setSlug] = useState(slugify(inboxName))
  const [edited, setEdited] = useState(false)
  const create = useCreateHelpCenter()
  const ok = name.trim() && validSlug(slug)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-help-center-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (ok) create.mutate({ inbox_id: inboxId, name: name.trim(), slug }, { onSuccess: onCreated })
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Create help center</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                Articles that answer your customers’ questions for {inboxName}. It stays unpublished until you publish
                it.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="help-center-name">
            <Input
              id="help-center-name"
              value={name}
              maxLength={100}
              autoFocus
              required
              placeholder={t`Acme help`}
              onChange={(e) => {
                setName(e.target.value)
                if (!edited) setSlug(slugify(e.target.value))
              }}
              data-testid="new-help-center-name"
            />
          </Field>
          <Field
            label={<Trans>Address</Trans>}
            htmlFor="help-center-slug"
            hint={<Trans>3 to 48 letters, numbers and hyphens. It is unique on this server.</Trans>}
          >
            <Input
              id="help-center-slug"
              value={slug}
              maxLength={48}
              required
              spellCheck={false}
              autoCapitalize="none"
              onChange={(e) => {
                setEdited(true)
                setSlug(e.target.value.toLowerCase())
              }}
              data-testid="new-help-center-slug"
            />
          </Field>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || create.isPending} data-testid="create-help-center">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create help center</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

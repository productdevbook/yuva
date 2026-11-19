import { Trans } from "@lingui/react/macro"
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
import { useCreateChangelog } from "@/features/changelog/queries"
import { ErrorLine } from "@/features/changelog/text"
import { Field } from "@/features/settings/ui"
import { slugify, validSlug } from "@/features/status/CreatePageForm"
import type { Changelog } from "@/lib/api"

export function CreateChangelogDialog({
  inboxId,
  inboxName,
  onClose,
  onCreated,
}: {
  inboxId: string
  inboxName: string
  onClose: () => void
  onCreated: (c: Changelog) => void
}) {
  const [name, setName] = useState(inboxName)
  const [slug, setSlug] = useState(slugify(inboxName))
  const [edited, setEdited] = useState(false)
  const create = useCreateChangelog()
  const ok = name.trim() && validSlug(slug)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent data-testid="new-changelog-dialog">
        <form
          className="flex flex-col gap-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (ok) create.mutate({ inbox_id: inboxId, name: name.trim(), slug }, { onSuccess: onCreated })
          }}
        >
          <DialogHeader>
            <DialogTitle>
              <Trans>Create changelog</Trans>
            </DialogTitle>
            <DialogDescription>
              <Trans>
                Tell the customers of {inboxName} what is new, improved and fixed. It stays unpublished until you
                publish it.
              </Trans>
            </DialogDescription>
          </DialogHeader>
          <Field label={<Trans>Name</Trans>} htmlFor="changelog-name">
            <Input
              id="changelog-name"
              value={name}
              maxLength={100}
              autoFocus
              required
              onChange={(e) => {
                setName(e.target.value)
                if (!edited) setSlug(slugify(e.target.value))
              }}
              data-testid="new-changelog-name"
            />
          </Field>
          <Field
            label={<Trans>Address</Trans>}
            htmlFor="changelog-slug"
            hint={<Trans>3 to 48 letters, numbers and hyphens. It is unique on this server.</Trans>}
          >
            <Input
              id="changelog-slug"
              value={slug}
              maxLength={48}
              required
              spellCheck={false}
              autoCapitalize="none"
              onChange={(e) => {
                setEdited(true)
                setSlug(e.target.value.toLowerCase())
              }}
              data-testid="new-changelog-slug"
            />
          </Field>
          <ErrorLine error={create.error} />
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={!ok || create.isPending} data-testid="create-changelog">
              {create.isPending && <LoaderIcon className="animate-spin" />}
              <Trans>Create changelog</Trans>
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
